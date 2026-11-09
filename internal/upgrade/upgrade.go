// Package upgrade 实现基于 GitHub Release 的自动升级功能。
package upgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		} `json:"assets"`
	}

	if err := json.Unmarshal(body, &ghRelease); err != nil {
		return nil, fmt.Errorf("解析 GitHub Release 失败: %w", err)
	}

	// 检查版本是否更新
	if ghRelease.TagName == "" || ghRelease.TagName == u.currentVersion {
		return nil, nil // 已是最新版本
	}

	// 查找当前平台的资源文件
	assetName := u.getAssetName(ghRelease.TagName)
	var downloadURL string
	var size int64

	for _, asset := range ghRelease.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
			size = asset.Size
			break
		}
	}

	if downloadURL == "" {
		return nil, fmt.Errorf("未找到适用于 %s/%s 平台的安装包", runtime.GOOS, runtime.GOARCH)
	}

	return &Release{
		Version:     ghRelease.TagName,
		Notes:       ghRelease.Body,
		PublishedAt: ghRelease.PublishedAt,
		PageURL:     ghRelease.HTMLURL,
		DownloadURL: downloadURL,
		Size:        size,
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

	tmpFile, err := u.downloadRelease(ctx, rel.DownloadURL, func(loaded, total int64) {
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

	if err := u.verifyFile(tmpFile); err != nil {
		return fmt.Errorf("文件验证失败: %w", err)
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

// downloadRelease 下载 Release 文件
func (u *GitHubUpgrader) downloadRelease(ctx context.Context, url string, onProgress func(int64, int64)) (string, error) {
	client := &http.Client{Timeout: 5 * time.Minute}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载失败，HTTP 状态码: %d", resp.StatusCode)
	}

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

// verifyFile 验证下载的文件
func (u *GitHubUpgrader) verifyFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	// 基本验证：文件大小不为 0
	if info.Size() == 0 {
		return fmt.Errorf("下载的文件为空")
	}

	// 验证文件是否可执行（Unix 系统）
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0755); err != nil {
			return fmt.Errorf("设置执行权限失败: %w", err)
		}
	}

	return nil
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

	// 删除备份
	_ = os.Remove(backupPath)

	return nil
}

// getAssetName 根据当前平台生成资源文件名
func (u *GitHubUpgrader) getAssetName(version string) string {
	// 去掉版本号中的 'v' 前缀（如果有）
	version = strings.TrimPrefix(version, "v")

	osName := runtime.GOOS
	arch := runtime.GOARCH

	// 文件名格式: TVBoxWeb_v1.3.1_darwin_arm64
	fileName := fmt.Sprintf("TVBoxWeb_%s_%s_%s", version, osName, arch)

	// Windows 平台添加 .exe 扩展名
	if osName == "windows" {
		fileName += ".exe"
	}

	return fileName
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
