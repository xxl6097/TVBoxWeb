package upgrade

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	if err := u.verifyFile(tmp, rel); err == nil || !strings.Contains(err.Error(), "SHA256 不匹配") {
		t.Fatalf("篡改文件应被拒绝，得到: %v", err)
	}
	// 空文件也必须拒绝
	if err := os.WriteFile(tmp, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := u.verifyFile(tmp, rel); err == nil {
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
