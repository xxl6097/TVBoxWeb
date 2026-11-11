package api

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xxl6097/TVBoxWeb/internal/model"
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

// 密码掩码回路：列表接口不回显密码（返回 ******），前端把它回填再提交时，
// 服务端必须认出这是「沿用已存密码」，否则会用 ****** 去认证 → 401，
// 或者把真实密码覆盖成星号。
func TestResolveWdPass(t *testing.T) {
	if err := store.Init(filepath.Join(t.TempDir(), "store.json")); err != nil {
		t.Fatal(err)
	}
	store.Reset()
	store.SaveWebdavSource(model.WebdavSource{
		ID: "a", Name: "极空间", URL: "https://fs.abber.cn:88/dav/", User: "webdav", Pass: "het002402",
	})

	// ① 掩码 → 还原成真实密码
	s := model.WebdavSource{ID: "a", URL: "https://fs.abber.cn:88/dav/", User: "webdav", Pass: maskedPassword}
	resolveWdPass(&s)
	if s.Pass != "het002402" {
		t.Fatalf("掩码应还原成真实密码，实际 %q", s.Pass)
	}

	// ② 留空（编辑时前端不回填）→ 同样沿用
	s2 := model.WebdavSource{ID: "a", URL: "https://fs.abber.cn:88/dav/", User: "webdav"}
	resolveWdPass(&s2)
	if s2.Pass != "het002402" {
		t.Fatalf("留空应沿用已存密码，实际 %q", s2.Pass)
	}

	// ③ 没带 id → 按「地址 + 用户名」兜底，并把 id 补上
	s3 := model.WebdavSource{URL: "https://fs.abber.cn:88/dav", User: "webdav", Pass: maskedPassword}
	resolveWdPass(&s3)
	if s3.Pass != "het002402" || s3.ID != "a" {
		t.Fatalf("按地址+用户名应能找回，实际 pass=%q id=%q", s3.Pass, s3.ID)
	}

	// ④ 用户真填了新密码 → 不能被覆盖
	s4 := model.WebdavSource{ID: "a", URL: "https://fs.abber.cn:88/dav/", User: "webdav", Pass: "newpass"}
	resolveWdPass(&s4)
	if s4.Pass != "newpass" {
		t.Fatalf("新密码不应被覆盖，实际 %q", s4.Pass)
	}

	// ⑤ 新增源 + 空密码 → 保持空（匿名访问），不能误用别人的密码
	s5 := model.WebdavSource{URL: "https://other.example.com/dav/", User: "x"}
	resolveWdPass(&s5)
	if s5.Pass != "" {
		t.Fatalf("新增匿名源不应被塞密码，实际 %q", s5.Pass)
	}
}

// 保存链路：用掩码再保存一次，已存的真实密码不能被写成星号
func TestSaveWebdavSourceKeepsPasswordWhenMasked(t *testing.T) {
	if err := store.Init(filepath.Join(t.TempDir(), "store.json")); err != nil {
		t.Fatal(err)
	}
	store.Reset()
	SetSecurity(SecurityConfig{})
	defer netguard.SetTrustedHosts(nil)

	save := func(body string) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/webdav/sources", strings.NewReader(body))
		Handler().ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("保存失败 %d: %s", rec.Code, rec.Body.String())
		}
	}
	save(`{"name":"极空间","url":"https://fs.abber.cn:88/dav/","user":"webdav","pass":"het002402"}`)
	before := store.WebdavSources()
	if len(before) != 1 || before[0].Pass != "het002402" {
		t.Fatalf("首次保存应写入真实密码，实际 %+v", before)
	}
	id := before[0].ID

	// 模拟前端把掩码原样回传
	save(`{"id":"` + id + `","name":"极空间改名","url":"https://fs.abber.cn:88/dav/","user":"webdav","pass":"` + maskedPassword + `"}`)
	after := store.WebdavSources()
	if after[0].Pass != "het002402" {
		t.Fatalf("掩码不应把真实密码覆盖掉，实际 %q", after[0].Pass)
	}
	if after[0].Name != "极空间改名" {
		t.Fatalf("其他字段应正常更新，实际 %q", after[0].Name)
	}
}
