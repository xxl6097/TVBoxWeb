package upgrade

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestIsNewerVersion(t *testing.T) {
	cases := []struct {
		remote, current string
		want            bool
	}{
		{"v1.5.0", "v1.4.0", true},
		{"v1.4.0", "v1.4.0", false},
		{"v1.3.9", "v1.4.0", false}, // 不允许降级
		{"v1.10.0", "v1.9.9", true},
		{"v2.0.0", "v1.99.99", true},
		{"1.4.1", "v1.4.0", true}, // 无 v 前缀
		{"v1.5.0-rc1", "v1.4.0", true},
		{"dev", "v1.4.0", true}, // 无法解析时回退字符串比较
	}
	for _, c := range cases {
		if got := isNewerVersion(c.remote, c.current); got != c.want {
			t.Errorf("isNewerVersion(%q, %q) = %v, want %v", c.remote, c.current, got, c.want)
		}
	}
}

func TestGetAssetNameMatchesCI(t *testing.T) {
	u := &GitHubUpgrader{}
	want := fmt.Sprintf("yezhen-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		want += ".exe"
	}
	if got := u.getAssetName(); got != want {
		t.Fatalf("getAssetName() = %q, want %q（必须与 build-release.yml 产物名一致）", got, want)
	}
	if strings.HasPrefix(u.getAssetName(), "TVBoxWeb_") {
		t.Fatal("资源名仍是旧的 TVBoxWeb_ 前缀，在线升级会找不到安装包")
	}
}

func TestFetchChecksum(t *testing.T) {
	const sum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  yezhen-linux-amd64\n%s *./yezhen-darwin-arm64\n", sum, strings.Repeat("a", 64))
	}))
	defer srv.Close()

	u := &GitHubUpgrader{}
	got, err := u.fetchChecksum(context.Background(), assetSources("", srv.URL), "yezhen-linux-amd64")
	if err != nil {
		t.Fatal(err)
	}
	if got != sum {
		t.Fatalf("checksum = %q, want %q", got, sum)
	}
	if _, err := u.fetchChecksum(context.Background(), assetSources("", srv.URL), "not-exist"); err == nil {
		t.Fatal("缺少目标文件时应返回错误")
	}
}

func TestVerifyFileRejectsMismatch(t *testing.T) {
	const good = "1111111111111111111111111111111111111111111111111111111111111111"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  payload\n", good)
	}))
	defer srv.Close()

	tmp := filepath.Join(t.TempDir(), "payload")
	if err := os.WriteFile(tmp, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	u := &GitHubUpgrader{}
	rel := &Release{Size: 5, assetName: "payload", checksumURL: srv.URL}
	if err := u.verifyFile(context.Background(), tmp, rel); err == nil || !strings.Contains(err.Error(), "SHA256 不匹配") {
		t.Fatalf("篡改文件应被拒绝，得到: %v", err)
	}
	// 空文件也必须拒绝
	if err := os.WriteFile(tmp, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := u.verifyFile(context.Background(), tmp, rel); err == nil {
		t.Fatal("空文件应被拒绝")
	}
}

func TestAssetSourcesOrder(t *testing.T) {
	s := assetSources("https://api.github.com/x", "https://github.com/x")
	if len(s) != 2 || !s[0].api || s[1].api {
		t.Fatalf("API 通道必须排第一位且标记 api=true: %+v", s)
	}
	if s[0].url != "https://api.github.com/x" || s[1].url != "https://github.com/x" {
		t.Fatalf("顺序错误: %+v", s)
	}
	if got := assetSources("", "https://github.com/x"); len(got) != 1 || got[0].api {
		t.Fatalf("无 API 通道时应只剩直链: %+v", got)
	}
	if got := assetSources("u", "u"); len(got) != 1 {
		t.Fatalf("相同地址应去重: %+v", got)
	}
	if got := assetSources("", ""); len(got) != 0 {
		t.Fatalf("空地址应为空列表: %+v", got)
	}
}

func TestAPIAssetURL(t *testing.T) {
	u := &GitHubUpgrader{owner: "xxl6097", repo: "TVBoxWeb"}
	want := "https://api.github.com/repos/xxl6097/TVBoxWeb/releases/assets/733906936"
	if got := u.apiAssetURL(733906936); got != want {
		t.Fatalf("apiAssetURL = %q, want %q", got, want)
	}
}

func TestOpenAssetPrefersAPIChannel(t *testing.T) {
	acceptCh := make(chan string, 1)
	var fallbackHits atomic.Int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acceptCh <- r.Header.Get("Accept")
		_, _ = w.Write([]byte("api-channel"))
	}))
	defer primary.Close()
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackHits.Add(1)
		_, _ = w.Write([]byte("fallback"))
	}))
	defer fallback.Close()

	resp, err := openAsset(context.Background(),
		[]downloadSource{{url: primary.URL, api: true}, {url: fallback.URL}}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "api-channel" {
		t.Fatalf("应优先使用 API 通道，得到 %q", b)
	}
	if got := <-acceptCh; got != "application/octet-stream" {
		t.Fatalf("API 通道必须带 Accept: application/octet-stream，得到 %q", got)
	}
	if n := fallbackHits.Load(); n != 0 {
		t.Fatalf("primary 成功时不应回退，命中 %d 次", n)
	}
}

func TestOpenAssetFallsBackOn502(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("CONNECT tunnel failed"))
	}))
	defer primary.Close()
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fallback-ok"))
	}))
	defer fallback.Close()

	resp, err := openAsset(context.Background(),
		[]downloadSource{{url: primary.URL, api: true}, {url: fallback.URL}}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "fallback-ok" {
		t.Fatalf("502 时应自动回退直链，得到 %q", b)
	}
}

func TestOpenAssetAllFail(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer primary.Close()
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer fallback.Close()

	_, err := openAsset(context.Background(),
		[]downloadSource{{url: primary.URL, api: true}, {url: fallback.URL}}, 5*time.Second)
	if err == nil {
		t.Fatal("全部失败时应返回错误")
	}
	if !strings.Contains(err.Error(), primary.URL) || !strings.Contains(err.Error(), fallback.URL) {
		t.Fatalf("错误信息应列出两个尝试过的地址: %v", err)
	}
}

func TestFetchChecksumPrefersAPIChannel(t *testing.T) {
	const apiSum = "abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"
	var fallbackHits atomic.Int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  yezhen-linux-amd64\n", apiSum)
	}))
	defer primary.Close()
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackHits.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer fallback.Close()

	u := &GitHubUpgrader{}
	got, err := u.fetchChecksum(context.Background(),
		assetSources(primary.URL, fallback.URL), "yezhen-linux-amd64")
	if err != nil {
		t.Fatal(err)
	}
	if got != apiSum {
		t.Fatalf("checksum = %q, want %q", got, apiSum)
	}
	if n := fallbackHits.Load(); n != 0 {
		t.Fatalf("API 通道成功时不应请求直链，命中 %d 次", n)
	}
}

// ---------- 断点续传 ----------

// isolateCache 把断点续传缓存目录重定向到测试临时目录，避免污染真实用户目录。
func isolateCache(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, ".cache"))
	t.Setenv("LocalAppData", dir)
	return dir
}

// rangeServer 模拟一个支持 HTTP Range 的文件服务器。
// truncateFirst > 0 时，第 1 个请求只写 truncateFirst 字节并强制断连，用来模拟网络中断。
func rangeServer(payload []byte, truncateFirst int) (*httptest.Server, *[]int64) {
	var mu sync.Mutex
	var ranges []int64
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		var start int64
		if rg := r.Header.Get("Range"); rg != "" {
			if s, ok := contentRangeStart(strings.Replace(rg, "bytes=", "bytes ", 1)); ok {
				start = s
			}
		}
		mu.Lock()
		ranges = append(ranges, start)
		mu.Unlock()

		body := payload[start:]
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		if start > 0 {
			w.Header().Set("Content-Range",
				fmt.Sprintf("bytes %d-%d/%d", start, len(payload)-1, len(payload)))
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		if n == 1 && truncateFirst > 0 && truncateFirst < len(body) {
			_, _ = w.Write(body[:truncateFirst])
			// 强行断开连接，让客户端读到 unexpected EOF
			if hj, ok := w.(http.Hijacker); ok {
				if conn, _, err := hj.Hijack(); err == nil {
					_ = conn.Close()
				}
			}
			return
		}
		_, _ = w.Write(body)
	}))
	return srv, &ranges
}

func TestContentRangeStart(t *testing.T) {
	cases := []struct {
		in    string
		want  int64
		valid bool
	}{
		{"bytes 1024-2047/2048", 1024, true},
		{"bytes   0-99/100", 0, true},
		{"bytes */100", 0, false},
		{"", 0, false},
		{"items 1-2/3", 0, false},
	}
	for _, c := range cases {
		got, ok := contentRangeStart(c.in)
		if ok != c.valid || (ok && got != c.want) {
			t.Errorf("contentRangeStart(%q) = (%d,%v), want (%d,%v)", c.in, got, ok, c.want, c.valid)
		}
	}
}

func TestDownloadResumesAfterInterruption(t *testing.T) {
	isolateCache(t)
	payload := bytes.Repeat([]byte("0123456789abcdef"), 8000) // 128KB
	const firstChunk = 40000

	srv, ranges := rangeServer(payload, firstChunk)
	defer srv.Close()

	u := &GitHubUpgrader{}
	rel := &Release{Version: "v9.9.9", assetName: "yezhen-test", Size: int64(len(payload))}

	part, err := u.downloadRelease(context.Background(), rel,
		[]downloadSource{{url: srv.URL}}, nil)
	if err != nil {
		t.Fatalf("续传下载应成功: %v", err)
	}
	got, err := os.ReadFile(part)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("续传后内容不一致：得到 %d 字节，期望 %d 字节", len(got), len(payload))
	}
	if len(*ranges) < 2 {
		t.Fatalf("应发生续传（至少 2 次请求），实际 %d 次", len(*ranges))
	}
	if (*ranges)[0] != 0 {
		t.Fatalf("首次请求不应带 Range，得到 offset=%d", (*ranges)[0])
	}
	if (*ranges)[1] == 0 {
		t.Fatal("第二次请求必须带 Range 从中断处续传，实际又从 0 开始")
	}
	if got := (*ranges)[1]; got != firstChunk {
		t.Fatalf("续传起点应为中断处 %d，实际 %d", firstChunk, got)
	}
}

func TestDownloadReusesCompletedPartFile(t *testing.T) {
	isolateCache(t)
	payload := []byte(strings.Repeat("x", 4096))

	srv, ranges := rangeServer(payload, 0)
	defer srv.Close()

	u := &GitHubUpgrader{}
	rel := &Release{Version: "v9.9.8", assetName: "yezhen-test", Size: int64(len(payload))}

	// 预置一个「已下载完整」的 .part 文件
	part, err := partialPath(rel.Version, rel.assetName)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(part, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := u.downloadRelease(context.Background(), rel,
		[]downloadSource{{url: srv.URL}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(*ranges) != 0 {
		t.Fatalf("已完整的 .part 不应再发起下载，实际请求 %d 次", len(*ranges))
	}
	if got != part {
		t.Fatalf("应复用同一个 .part 路径，得到 %q，期望 %q", got, part)
	}
}

func TestDownloadRestartsWhenServerIgnoresRange(t *testing.T) {
	isolateCache(t)
	payload := []byte(strings.Repeat("y", 8192))

	// 服务端永远忽略 Range，直接回 200 全量
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	u := &GitHubUpgrader{}
	rel := &Release{Version: "v9.9.7", assetName: "yezhen-test", Size: int64(len(payload))}

	// 预置脏的半个文件
	part, err := partialPath(rel.Version, rel.assetName)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(part, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := u.downloadRelease(context.Background(), rel,
		[]downloadSource{{url: srv.URL}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, payload) {
		t.Fatalf("服务端不支持 Range 时应从头覆盖，得到 %d 字节（前 8 字节 %q）", len(b), string(b[:min(8, len(b))]))
	}
}

func TestDownloadDiscardsOversizedPartFile(t *testing.T) {
	isolateCache(t)
	payload := []byte("small")

	srv, ranges := rangeServer(payload, 0)
	defer srv.Close()

	u := &GitHubUpgrader{}
	rel := &Release{Version: "v9.9.6", assetName: "yezhen-test", Size: int64(len(payload))}

	part, _ := partialPath(rel.Version, rel.assetName)
	if err := os.WriteFile(part, bytes.Repeat([]byte("z"), 100), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := u.downloadRelease(context.Background(), rel, []downloadSource{{url: srv.URL}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(got)
	if string(b) != "small" {
		t.Fatalf("超过目标的脏缓存应被丢弃重下，得到 %q", b)
	}
	if len(*ranges) == 0 || (*ranges)[0] != 0 {
		t.Fatalf("丢弃脏缓存后应从头请求，实际 %v", *ranges)
	}
}

func TestPrunePartsKeepsCurrent(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "v1.0.0-keep.part")
	old := filepath.Join(dir, "v0.9.0-old.part")
	other := filepath.Join(dir, "notes.txt")
	for _, p := range []string{keep, old, other} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pruneParts(dir, keep)
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("当前版本的 .part 不应被清理")
	}
	if _, err := os.Stat(old); err == nil {
		t.Fatal("其他版本的 .part 应被清理")
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("非 .part 文件不应被清理")
	}
}

// ---------- 进度序列化 ----------

// 关键回归：SSE 里的阶段必须是 "downloading" 这样的名字。
// 之前用 int 序列化，前端按字符串查表查不到，界面上直接显示一个「1」，
// 并且 stage === 'downloading' 恒为 false，进度条完全不动。
func TestStageMarshalsAsName(t *testing.T) {
	cases := map[UpgradeStage]string{
		StageChecking:    `"checking"`,
		StageDownloading: `"downloading"`,
		StageVerifying:   `"verifying"`,
		StageReplacing:   `"replacing"`,
		StageDone:        `"done"`,
	}
	for st, want := range cases {
		b, err := json.Marshal(st)
		if err != nil {
			t.Fatalf("Marshal(%v) 出错: %v", st, err)
		}
		if string(b) != want {
			t.Errorf("阶段序列化应为 %s，实际 %s", want, b)
		}
	}

	p := UpgradeProgress{Stage: StageDownloading, Loaded: 100, Total: 400, Message: "已下载 25.0%"}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"stage":"downloading"`) {
		t.Fatalf("进度 JSON 应含字符串阶段，实际 %s", b)
	}
}

func TestStageUnmarshalAcceptsNameAndLegacyInt(t *testing.T) {
	var st UpgradeStage
	if err := json.Unmarshal([]byte(`"verifying"`), &st); err != nil || st != StageVerifying {
		t.Fatalf("应能从名字还原，得到 %v err=%v", st, err)
	}
	// 兼容旧版本推送的数字码（v1.6.1 及更早）
	if err := json.Unmarshal([]byte(`1`), &st); err != nil || st != StageDownloading {
		t.Fatalf("应能从数字还原，得到 %v err=%v", st, err)
	}
}
