// Package githubupgrade 是 tvbox.Upgrader 的一个 GitHub Releases 实现。
//
// tvbox 库本身不内置更新逻辑（更新源与校验策略应由部署方决定），
// 这个包把「对接 GitHub Releases」这件常见的事做成可复用的实现：
//
//	up, err := githubupgrade.New(githubupgrade.Options{
//		Owner: "xxl6097",
//		Repo:  "TVBoxWeb",
//	})
//	srv, err := tvbox.New(tvbox.WithUpgrader(up))
//
// 它做的事：
//
//	检查   调 /repos/<owner>/<repo>/releases/latest，与 CurrentVersion 比较
//	升级   下载对应 GOOS/GOARCH 的裸二进制资产
//	       → 用同一 release 的 checksums.txt 校验 sha256
//	       → 原子替换当前可执行文件（失败则回滚）
//
// # 安全约束
//
//   - **先校验再替换**。摘要对不上就在落地前中止，一个文件都不动。
//   - **没有校验和文件就拒绝升级**。「校验不了就放行」等于没有校验。
//   - **临时文件建在目标二进制的同目录下**，保证 rename 是原子的。
//   - **替换前把原文件改成 .old**，rename 失败还能放回去。
//
// # 已知限制
//
//   - 只处理**裸二进制**资产（如 yezhen-linux-amd64），不支持 .tar.gz/.zip。
//   - 升级完成后**不会自动重启**：重启交给部署方式（systemd/docker）或调用方。
package githubupgrade

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tvbox "github.com/xxl6097/TVBoxWeb"
)

// DefaultAPIBase 是 github.com 的 API 地址。
const DefaultAPIBase = "https://api.github.com"

// DefaultChecksumsName 是默认的校验和文件名。
const DefaultChecksumsName = "checksums.txt"

// Options 用来构造 Upgrader。Owner 与 Repo 必填。
type Options struct {
	Owner string
	Repo  string

	// CurrentVersion 为当前运行的版本号，用来判断「有没有新版」。
	// 留空则用 tvbox.AppVersion。
	CurrentVersion string

	// APIBase 为 API 根地址，留空用 DefaultAPIBase。
	APIBase string

	// Client 为 HTTP 客户端，留空用 10 分钟超时的。
	Client *http.Client

	// AssetPrefix 为资产名前缀，默认 "yezhen"。
	// 资产名形如 <prefix>-<goos>-<goarch>[.exe]。
	AssetPrefix string

	// ChecksumsName 为校验和文件名，默认 checksums.txt。
	ChecksumsName string

	// DryRun 为 true 时只下载并校验，不替换自己。
	DryRun bool
}

// Upgrader 实现 tvbox.Upgrader。
type Upgrader struct {
	owner, repo    string
	currentVersion string
	apiBase        string
	assetPrefix    string
	checksumsName  string
	client         *http.Client
	dryRun         bool
}

// New 校验参数并构造 Upgrader。
func New(opts Options) (*Upgrader, error) {
	if strings.TrimSpace(opts.Owner) == "" || strings.TrimSpace(opts.Repo) == "" {
		return nil, errors.New("githubupgrade: 必须指定 Owner 与 Repo")
	}
	base := strings.TrimSuffix(opts.APIBase, "/")
	if base == "" {
		base = DefaultAPIBase
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("githubupgrade: APIBase %q 不是合法 URL", base)
	}
	if u.Scheme != "https" {
		return nil, errors.New("githubupgrade: APIBase 必须使用 HTTPS")
	}
	prefix := opts.AssetPrefix
	if prefix == "" {
		prefix = "yezhen"
	}
	sums := opts.ChecksumsName
	if sums == "" {
		sums = DefaultChecksumsName
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	current := strings.TrimSpace(opts.CurrentVersion)
	if current == "" {
		current = tvbox.Version()
	}
	return &Upgrader{
		owner:          strings.TrimSpace(opts.Owner),
		repo:           strings.TrimSpace(opts.Repo),
		currentVersion: strings.TrimPrefix(current, "v"),
		apiBase:        base,
		assetPrefix:    prefix,
		checksumsName:  sums,
		client:         client,
		dryRun:         opts.DryRun,
	}, nil
}

// ---------------------------------------------------------------------------
// GitHub API 响应
// ---------------------------------------------------------------------------

type ghAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type ghRelease struct {
	Tag         string    `json:"tag_name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []ghAsset `json:"assets"`
}

// assetName 返回本平台的资产名。
func (u *Upgrader) assetName() string {
	n := u.assetPrefix + "-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		n += ".exe"
	}
	return n
}

// pickAsset 从 release 里找本平台的资产。
func (u *Upgrader) pickAsset(rel ghRelease) (ghAsset, bool) {
	name := u.assetName()
	for _, a := range rel.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return ghAsset{}, false
}

// Check 查最新 release 并与当前版本比较；没有更新时返回 (nil, nil)。
func (u *Upgrader) Check(ctx context.Context) (*tvbox.Release, error) {
	rel, err := u.fetchRelease(ctx)
	if err != nil {
		return nil, err
	}
	latest := strings.TrimPrefix(rel.Tag, "v")
	if !versionLess(u.currentVersion, latest) {
		return nil, nil // 已是最新
	}
	asset, ok := u.pickAsset(*rel)
	if !ok {
		return nil, fmt.Errorf("版本 %s 里没有 %s/%s 的资产", rel.Tag, runtime.GOOS, runtime.GOARCH)
	}
	return &tvbox.Release{
		Version:     latest,
		Notes:       strings.TrimSpace(rel.Body),
		PublishedAt: rel.PublishedAt.Format("2006-01-02"),
		PageURL:     rel.HTMLURL,
		Size:        asset.Size,
	}, nil
}

// Upgrade 下载并替换当前可执行文件。
func (u *Upgrader) Upgrade(ctx context.Context, rel *tvbox.Release,
	onProgress func(tvbox.UpgradeProgress)) error {

	report := func(p tvbox.UpgradeProgress) {
		if onProgress != nil {
			onProgress(p)
		}
	}

	// 重新拉一次 release：资产地址以这一次为准，不信任前端传入的 URL。
	ghRel, err := u.fetchRelease(ctx)
	if err != nil {
		return err
	}
	asset, ok := u.pickAsset(*ghRel)
	if !ok {
		return fmt.Errorf("版本 %s 里没有 %s/%s 的资产", ghRel.Tag, runtime.GOOS, runtime.GOARCH)
	}

	target, err := executablePath()
	if err != nil {
		return err
	}

	// 临时文件建在目标同目录下：同一文件系统才能用 rename 原子替换。
	tmp, err := os.CreateTemp(filepath.Dir(target), ".yezhen-upgrade-*")
	if err != nil {
		return fmt.Errorf("创建临时文件失败（目录可能不可写）: %w", err)
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()

	report(tvbox.UpgradeProgress{
		Stage: tvbox.StageDownloading, Total: asset.Size, Message: asset.Name,
	})
	sum, err := u.download(ctx, asset, tmp, func(n int64) {
		report(tvbox.UpgradeProgress{
			Stage: tvbox.StageDownloading, Loaded: n, Total: asset.Size, Message: asset.Name,
		})
	})
	if err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("写出临时文件失败: %w", err)
	}

	// 先校验，再替换。
	report(tvbox.UpgradeProgress{Stage: tvbox.StageVerifying, Message: "校验 sha256"})
	want, err := u.fetchChecksum(ctx, *ghRel, asset.Name)
	if err != nil {
		return err
	}
	if !strings.EqualFold(want, sum) {
		return fmt.Errorf("摘要不匹配：期望 %s，实际 %s —— 已中止，未替换任何文件", want, sum)
	}

	if u.dryRun {
		report(tvbox.UpgradeProgress{
			Stage:   tvbox.StageDone,
			Message: "dry-run：已下载并校验通过，未替换（临时文件已清理）",
		})
		return nil
	}

	report(tvbox.UpgradeProgress{Stage: tvbox.StageReplacing, Message: "替换 " + filepath.Base(target)})
	if err := replaceFile(tmpName, target); err != nil {
		return err
	}
	committed = true

	report(tvbox.UpgradeProgress{
		Stage:   tvbox.StageDone,
		Message: "已替换为 " + rel.Version + "，重启进程后生效",
	})
	return nil
}

// ---------------------------------------------------------------------------
// 内部实现
// ---------------------------------------------------------------------------

// executablePath 返回当前可执行文件的真实路径（解析软链）。
func executablePath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("定位当前可执行文件失败: %w", err)
	}
	if real, err := filepath.EvalSymlinks(p); err == nil {
		p = real
	}
	return p, nil
}

// replaceFile 把 src 原子地换到 dst，失败则回滚。
func replaceFile(src, dst string) error {
	if err := os.Chmod(src, 0o755); err != nil {
		return fmt.Errorf("设置可执行位失败: %w", err)
	}
	backup := dst + ".old"
	_ = os.Remove(backup)
	if err := os.Rename(dst, backup); err != nil {
		return fmt.Errorf("备份原文件失败: %w", err)
	}
	if err := os.Rename(src, dst); err != nil {
		if rbErr := os.Rename(backup, dst); rbErr != nil {
			return fmt.Errorf("替换失败（%v），且回滚也失败（%v）—— 原文件在 %s", err, rbErr, backup)
		}
		return fmt.Errorf("替换失败，已回滚: %w", err)
	}
	return nil
}

// fetchRelease 取最新 release。
func (u *Upgrader) fetchRelease(ctx context.Context) (*ghRelease, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", u.apiBase, u.owner, u.repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "TVBoxWeb-upgrade")
	resp, err := u.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API 返回 %d", resp.StatusCode)
	}
	var rel ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

// download 下载资产到 dst，边下边算 sha256。
func (u *Upgrader) download(ctx context.Context, asset ghAsset, dst io.Writer,
	onProgress func(n int64)) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "TVBoxWeb-upgrade")
	resp, err := u.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载资产返回 %d", resp.StatusCode)
	}
	h := sha256.New()
	var loaded int64
	buf := make([]byte, 32<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return "", werr
			}
			h.Write(buf[:n])
			loaded += int64(n)
			if onProgress != nil {
				onProgress(loaded)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", rerr
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// fetchChecksum 从 release 的 checksums 资产里找目标资产的 sha256。
func (u *Upgrader) fetchChecksum(ctx context.Context, rel ghRelease, assetName string) (string, error) {
	var sumsURL string
	for _, a := range rel.Assets {
		if a.Name == u.checksumsName {
			sumsURL = a.URL
			break
		}
	}
	if sumsURL == "" {
		return "", fmt.Errorf("release 里没有 %s，拒绝无校验升级", u.checksumsName)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sumsURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "TVBoxWeb-upgrade")
	resp, err := u.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载校验和返回 %d", resp.StatusCode)
	}
	sc := bufio.NewScanner(io.LimitReader(resp.Body, 1<<20))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := filepath.Base(fields[len(fields)-1])
		if name == assetName {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("checksums 里没有 %s 的摘要", assetName)
}

// versionLess 比较语义化版本（简化：按 . 分段数字比较）。
func versionLess(a, b string) bool {
	as := splitVer(a)
	bs := splitVer(b)
	for i := 0; i < len(as) || i < len(bs); i++ {
		var av, bv int
		if i < len(as) {
			av = atoi(as[i])
		}
		if i < len(bs) {
			bv = atoi(bs[i])
		}
		if av != bv {
			return av < bv
		}
	}
	return false
}

func splitVer(v string) []string {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.Split(v, ".")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}
