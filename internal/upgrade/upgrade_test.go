package upgrade

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
	got, err := u.fetchChecksum(context.Background(), srv.URL, "yezhen-linux-amd64")
	if err != nil {
		t.Fatal(err)
	}
	if got != sum {
		t.Fatalf("checksum = %q, want %q", got, sum)
	}
	if _, err := u.fetchChecksum(context.Background(), srv.URL, "not-exist"); err == nil {
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
