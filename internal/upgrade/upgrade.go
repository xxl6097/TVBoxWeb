// Package upgrade 实现基于 GitHub Release 的自动升级功能。
package upgrade

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Release 表示一个 GitHub Release 版本信息。
type Release struct {
	Version     string `json:"version"`
	Notes       string `json:"notes"`
	PublishedAt string `json:"publishedAt"`
	PageURL     string `json:"pageUrl"`
	DownloadURL string `json:"downloadUrl"`
	Size        int64  `json:"size"`

	// 内部字段：不暴露给前端，仅用于下载后做 SHA256 完整性校验
	assetName      string
	checksumURL    string // browser_download_url（回退通道）
	checksumAPIURL string // api.github.com releases/assets/<id>（优先通道）
	downloadAPIURL string // 裸二进制的 API asset 通道
}

// downloadSource 描述一个可尝试的下载地址。
// api=true 表示这是 GitHub API asset 通道，需要带 Accept: application/octet-stream
// 才会返回二进制内容（否则 API 默认返回 JSON 元数据）。
type downloadSource struct {
	url string
	api bool
}

// assetSources 按「API 通道优先、直链回退」组装候选地址。
// 国内网络/部分代理下 github.com/.../releases/download/... 常被网关拦截（502），
// 而 api.github.com + Accept: application/octet-stream 通常可达。
func assetSources(apiURL, browserURL string) []downloadSource {
	var out []downloadSource
	if apiURL != "" {
		out = append(out, downloadSource{url: apiURL, api: true})
	}
	if browserURL != "" && browserURL != apiURL {
		out = append(out, downloadSource{url: browserURL})
	}
	return out
}

func (u *GitHubUpgrader) apiAssetURL(id int64) string {
	return fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/assets/%d", u.owner, u.repo, id)
}

// openAsset 依次尝试候选下载地址，返回第一个成功（HTTP 200）的响应。
// 调用方负责关闭 resp.Body。
func openAsset(ctx context.Context, sources []downloadSource, timeout time.Duration) (*http.Response, error) {
	if len(sources) == 0 {
		return nil, fmt.Errorf("缺少下载地址")
	}
	client := &http.Client{Timeout: timeout}
	var errs []string
	for _, s := range sources {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", s.url, err))
			continue
		}
		if s.api {
			req.Header.Set("Accept", "application/octet-stream")
		}
		req.Header.Set("User-Agent", "yezhen-upgrader")

		resp, err := client.Do(req)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", s.url, err))
			continue
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		resp.Body.Close()
		detail := strings.TrimSpace(string(body))
		if detail != "" {
			errs = append(errs, fmt.Sprintf("%s: HTTP %d %s", s.url, resp.StatusCode, detail))
		} else {
			errs = append(errs, fmt.Sprintf("%s: HTTP %d", s.url, resp.StatusCode))
		}
	}
	return nil, fmt.Errorf("所有下载地址均失败：%s", strings.Join(errs, "；"))
}

// UpgradeStage 升级阶段
type UpgradeStage int

const (
	StageChecking UpgradeStage = iota
	StageDownloading
	StageVerifying
	StageReplacing
	StageDone
)

func (s UpgradeStage) String() string {
	switch s {
	case StageChecking:
		return "checking"
	case StageDownloading:
		return "downloading"
	case StageVerifying:
		return "verifying"
	case StageReplacing:
		return "replacing"
	case StageDone:
		return "done"
	default:
		return "unknown"
	}
}

// UpgradeProgress 升级进度信息
type UpgradeProgress struct {
	Stage   UpgradeStage `json:"stage"`
	Loaded  int64        `json:"loaded,omitempty"`
	Total   int64        `json:"total,omitempty"`
	Message string       `json:"message"`
}

// Upgrader 升级器接口
type Upgrader interface {
	// Check 检查是否有新版本
	Check(ctx context.Context) (*Release, error)
	// Upgrade 执行升级
	Upgrade(ctx context.Context, rel *Release, onProgress func(UpgradeProgress)) error
}

// GitHubUpgrader 基于 GitHub Release 的升级器
type GitHubUpgrader struct {
	owner          string // GitHub 用户名
	repo           string // GitHub 仓库名
	currentVersion string // 当前版本
	execPath       string // 当前可执行文件路径
}

// NewGitHubUpgrader 创建一个 GitHub 升级器
func NewGitHubUpgrader(owner, repo, currentVersion string) (*GitHubUpgrader, error) {
	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("无法获取当前执行文件路径: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return nil, fmt.Errorf("无法解析符号链接: %w", err)
	}

	return &GitHubUpgrader{
		owner:          owner,
		repo:           repo,
		currentVersion: currentVersion,
		execPath:       execPath,
	}, nil
}

// Check 检查是否有新版本
func (u *GitHubUpgrader) Check(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", u.owner, u.repo)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", fmt.Sprintf("%s-upgrade-check", u.repo))

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 GitHub API 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API 返回错误状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20)) // 最多读取 5MB
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var ghRelease struct {
		TagName     string `json:"tag_name"`
		Body        string `json:"body"`
		PublishedAt string `json:"published_at"`
		HTMLURL     string `json:"html_url"`
		Assets      []struct {
			ID                 int64  `json:"id"`
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		} `json:"assets"`
	}

	if err := json.Unmarshal(body, &ghRelease); err != nil {
		return nil, fmt.Errorf("解析 GitHub Release 失败: %w", err)
	}

	// 只升不降：用 semver 数字比较，避免远端 tag 异常时诱导降级
	if ghRelease.TagName == "" || !isNewerVersion(ghRelease.TagName, u.currentVersion) {
		return nil, nil // 已是最新版本（或远端版本更旧）
	}

	// 查找当前平台的裸二进制（CI 产物名 yezhen-<os>-<arch>）
	assetName := u.getAssetName()
	var downloadURL, checksumURL string
	var downloadAPIURL, checksumAPIURL string
	var size int64

	for _, asset := range ghRelease.Assets {
		switch asset.Name {
		case assetName:
			downloadURL = asset.BrowserDownloadURL
			size = asset.Size
			if asset.ID > 0 {
				downloadAPIURL = u.apiAssetURL(asset.ID)
			}
		case "checksums.txt":
			checksumURL = asset.BrowserDownloadURL
			if asset.ID > 0 {
				checksumAPIURL = u.apiAssetURL(asset.ID)
			}
		}
	}

	if downloadURL == "" {
		return nil, fmt.Errorf("未找到适用于 %s/%s 平台的安装包（%s）", runtime.GOOS, runtime.GOARCH, assetName)
	}
	if checksumURL == "" {
		return nil, fmt.Errorf("该版本缺少 checksums.txt，出于安全考虑已中止升级")
	}

	return &Release{
		Version:        ghRelease.TagName,
		Notes:          ghRelease.Body,
		PublishedAt:    ghRelease.PublishedAt,
		PageURL:        ghRelease.HTMLURL,
		DownloadURL:    downloadURL,
		Size:           size,
		assetName:      assetName,
		checksumURL:    checksumURL,
		checksumAPIURL: checksumAPIURL,
		downloadAPIURL: downloadAPIURL,
	}, nil
}

// Upgrade 执行升级
func (u *GitHubUpgrader) Upgrade(ctx context.Context, rel *Release, onProgress func(UpgradeProgress)) error {
	report := func(p UpgradeProgress) {
		if onProgress != nil {
			onProgress(p)
		}
	}

	// 1. 下载新版本
	report(UpgradeProgress{
		Stage:   StageDownloading,
		Total:   rel.Size,
		Message: fmt.Sprintf("正在下载 %s", rel.Version),
	})

	tmpFile, err := u.downloadRelease(ctx, assetSources(rel.downloadAPIURL, rel.DownloadURL), func(loaded, total int64) {
		report(UpgradeProgress{
			Stage:   StageDownloading,
			Loaded:  loaded,
			Total:   total,
			Message: fmt.Sprintf("已下载 %.1f%%", float64(loaded)/float64(total)*100),
		})
	})
	if err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}
	defer os.Remove(tmpFile)

	// 2. 验证文件
	report(UpgradeProgress{
		Stage:   StageVerifying,
		Message: "验证下载文件",
	})

	if err := u.verifyFile(tmpFile, rel); err != nil {
		return fmt.Errorf("文件完整性校验失败: %w", err)
	}

	// 3. 替换可执行文件
	report(UpgradeProgress{
		Stage:   StageReplacing,
		Message: "替换可执行文件",
	})

	if err := u.replaceExecutable(tmpFile); err != nil {
		return fmt.Errorf("替换文件失败: %w", err)
	}

	report(UpgradeProgress{
		Stage:   StageDone,
		Message: fmt.Sprintf("升级到 %s 成功", rel.Version),
	})

	return nil
}

// downloadRelease 下载 Release 文件，优先 GitHub API asset 通道，失败回退直链。
func (u *GitHubUpgrader) downloadRelease(ctx context.Context, sources []downloadSource, onProgress func(int64, int64)) (string, error) {
	resp, err := openAsset(ctx, sources, 5*time.Minute)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "tvboxweb-upgrade-*")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	// 带进度的复制
	contentLength := resp.ContentLength
	var written int64
	buf := make([]byte, 32*1024)

	lastReport := time.Now()
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		nr, er := resp.Body.Read(buf)
		if nr > 0 {
			nw, ew := tmpFile.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				return "", ew
			}
			if nr != nw {
				return "", io.ErrShortWrite
			}

			// 每 100ms 报告一次进度
			if time.Since(lastReport) > 100*time.Millisecond {
				if onProgress != nil {
					onProgress(written, contentLength)
				}
				lastReport = time.Now()
			}
		}
		if er != nil {
			if er != io.EOF {
				return "", er
			}
			break
		}
	}

	// 最终进度报告
	if onProgress != nil {
		onProgress(written, contentLength)
	}

	return tmpFile.Name(), nil
}

// verifyFile 验证下载文件：非空 + SHA256 与 Release 的 checksums.txt 一致。
func (u *GitHubUpgrader) verifyFile(path string, rel *Release) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return fmt.Errorf("下载的文件为空")
	}
	if rel == nil || rel.assetName == "" {
		return fmt.Errorf("缺少校验信息，拒绝替换可执行文件")
	}
	checksumSources := assetSources(rel.checksumAPIURL, rel.checksumURL)
	if len(checksumSources) == 0 {
		return fmt.Errorf("缺少校验信息，拒绝替换可执行文件")
	}

	want, err := u.fetchChecksum(context.Background(), checksumSources, rel.assetName)
	if err != nil {
		return err
	}
	got, err := sha256File(path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("SHA256 不匹配（下载包可能被篡改或损坏）：期望 %s，实际 %s", want, got)
	}
	if rel.Size > 0 && info.Size() != rel.Size {
		return fmt.Errorf("文件大小与 Release 记录不一致：期望 %d，实际 %d", rel.Size, info.Size())
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0755); err != nil {
			return fmt.Errorf("设置执行权限失败: %w", err)
		}
	}
	return nil
}

// fetchChecksum 下载 checksums.txt 并取出指定文件的 sha256。
// 与二进制下载一致，优先 API asset 通道、失败回退 browser_download_url。
func (u *GitHubUpgrader) fetchChecksum(ctx context.Context, sources []downloadSource, assetName string) (string, error) {
	res, err := openAsset(ctx, sources, 20*time.Second)
	if err != nil {
		return "", fmt.Errorf("下载 checksums.txt 失败：%w", err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && len(fields[0]) == 64 {
			name := strings.TrimPrefix(fields[len(fields)-1], "*")
			if name == assetName || strings.HasSuffix(name, "/"+assetName) {
				return fields[0], nil
			}
		}
	}
	return "", fmt.Errorf("checksums.txt 中没有 %s 的校验值", assetName)
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// isNewerVersion 只做 vX.Y.Z 数字比较；无法解析时回退字符串比较，
// 但相同版本一律视为不升级。预发布标签（-rc1 等）按基础版本比较。
func isNewerVersion(remote, current string) bool {
	norm := func(v string) []int {
		v = strings.TrimPrefix(strings.TrimSpace(v), "v")
		if i := strings.IndexAny(v, "-+"); i >= 0 {
			v = v[:i]
		}
		parts := strings.Split(v, ".")
		out := make([]int, 3)
		for i := 0; i < len(parts) && i < 3; i++ {
			n, err := strconv.Atoi(parts[i])
			if err != nil {
				return nil
			}
			out[i] = n
		}
		return out
	}
	r, c := norm(remote), norm(current)
	if r == nil || c == nil {
		return remote != current
	}
	for i := 0; i < 3; i++ {
		if r[i] != c[i] {
			return r[i] > c[i]
		}
	}
	return false
}

// ConfirmStartup 在进程成功启动后调用：新版本能跑起来，才清理上一版的备份。
// 若新版本无法启动，用户可直接用 <exe>.backup 手工回滚。
func ConfirmStartup() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	backup := exe + ".backup"
	if st, err := os.Stat(backup); err == nil && st.Size() > 0 {
		_ = os.Remove(backup)
	}
}

// replaceExecutable 替换当前可执行文件
func (u *GitHubUpgrader) replaceExecutable(newFile string) error {
	// 创建备份
	backupPath := u.execPath + ".backup"
	if err := os.Rename(u.execPath, backupPath); err != nil {
		return fmt.Errorf("备份当前文件失败: %w", err)
	}

	// 复制新文件
	if err := copyFile(newFile, u.execPath); err != nil {
		// 恢复备份
		_ = os.Rename(backupPath, u.execPath)
		return fmt.Errorf("复制新文件失败: %w", err)
	}

	// 设置执行权限（Unix 系统）
	if runtime.GOOS != "windows" {
		if err := os.Chmod(u.execPath, 0755); err != nil {
			// 恢复备份
			_ = os.Remove(u.execPath)
			_ = os.Rename(backupPath, u.execPath)
			return fmt.Errorf("设置执行权限失败: %w", err)
		}
	}

	// 备份保留到下次启动自检成功（ConfirmStartup）后再清理，
	// 这样新版本起不来时仍可用 <exe>.backup 手工回滚。
	return nil
}

// getAssetName 与 .github/workflows/build-release.yml 的产物名严格一致：
// yezhen-<os>-<arch>（Windows 带 .exe）。旧实现找 TVBoxWeb_<ver>_<os>_<arch>，
// 与 CI 产物永不匹配，导致在线升级一直报「未找到安装包」。
func (u *GitHubUpgrader) getAssetName() string {
	name := fmt.Sprintf("yezhen-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// copyFile 复制文件
func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}
