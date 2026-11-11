package api

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xxl6097/TVBoxWeb/internal/netguard"
	"github.com/xxl6097/TVBoxWeb/internal/store"
)

// 复现并锁住用户实际遇到的问题：
// 家里 NAS 的公网域名在内网被 DNS 解析成 ULA（fd4f:...）/IPv4 私网，
// 保存 WebDAV 源时不能因为「解析到内网」就被 SSRF 防护拦下。
func TestWebdavSourceAcceptsPrivateResolvingHost(t *testing.T) {
	if err := store.Init(filepath.Join(t.TempDir(), "store.json")); err != nil {
		t.Fatal(err)
	}
	store.Reset()
	SetSecurity(SecurityConfig{})
	defer netguard.SetTrustedHosts(nil)

	const davURL = "https://fs.abber.cn:88/dav/迅雷下载/"
	body := `{"name":"极空间","url":"` + davURL + `","user":"webdav","pass":"het002402"}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/webdav/sources", strings.NewReader(body))
	Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("保存 WebDAV 源应成功，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if n := len(store.WebdavSources()); n != 1 {
		t.Fatalf("应保存 1 个网盘源，实际 %d", n)
	}

	// 保存后该域名进入可信列表：严格模式（/api/proxy 用的就是它）也必须放行，
	// 否则挂载成功但视频播放仍会被拦。
	if err := netguard.Check("https://fs.abber.cn:88/public/movie.mkv"); err != nil {
		t.Fatalf("已保存的源域名应被可信放行，实际被拒: %v", err)
	}
}

// 安全底线：没配置过的内网地址，/api/proxy 仍必须拒绝，不能因为上面那条修复而放开代理。
func TestProxyStillRejectsUnknownPrivateTarget(t *testing.T) {
	if err := store.Init(filepath.Join(t.TempDir(), "store.json")); err != nil {
		t.Fatal(err)
	}
	store.Reset()
	SetSecurity(SecurityConfig{})
	defer netguard.SetTrustedHosts(nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/proxy?url=http://192.168.1.1/secret", nil)
	Handler().ServeHTTP(rec, req)
	if rec.Code == 200 {
		t.Fatalf("未配置过的内网地址不应被代理，实际 200：%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "不安全") {
		t.Fatalf("错误信息应提示地址不安全，实际：%s", rec.Body.String())
	}
}

// 端口/协议这类硬规则在源模式下依然生效
func TestWebdavSourceRejectsNonHTTPScheme(t *testing.T) {
	if err := store.Init(filepath.Join(t.TempDir(), "store.json")); err != nil {
		t.Fatal(err)
	}
	store.Reset()
	SetSecurity(SecurityConfig{})

	body := `{"name":"坏源","url":"file:///etc/passwd"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/webdav/sources", strings.NewReader(body))
	Handler().ServeHTTP(rec, req)
	if rec.Code == 200 {
		t.Fatal("非 http(s) 协议不应被接受")
	}
}
