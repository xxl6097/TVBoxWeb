package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xxl6097/TVBoxWeb/internal/model"
)

// 「加了登录后网盘视频播不了」的回归测试之二：
// 播放需要的 Basic Auth 必须由服务端用**真实密码**生成。
//
// 背景：GET /api/webdav/sources 出于安全返回掩码 ******，
// 前端拿它拼 auth 会得到 base64("user:******") → 上游 401。
// 早先看起来能用，是因为列表被缓存进 IndexedDB，
// 缓存里还是掩码上线前的旧数据；加登录后每次登录都清缓存，问题才暴露。
func TestWdAuthReturnsRealPassword(t *testing.T) {
	SetSecurity(SecurityConfig{})
	defer SetSecurity(SecurityConfig{})

	// 造一个带真实密码的源
	src := model.WebdavSource{
		ID: "wd_test", Name: "t", URL: "https://dav.example.com",
		User: "webdav", Pass: "het002402",
	}
	// 直接走 HTTP 保存，确保进入 store
	body, _ := json.Marshal(src)
	req := httptest.NewRequest("POST", "/api/webdav/sources", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("保存源失败 %d: %s", rec.Code, rec.Body.String())
	}

	// ① 列表接口必须掩码
	req = httptest.NewRequest("GET", "/api/webdav/sources", nil)
	rec = httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)
	var list []model.WebdavSource
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) == 0 {
		t.Fatal("列表为空")
	}
	var id string
	for _, s := range list {
		if s.Name == "t" {
			id = s.ID
			if s.Pass != maskedPassword {
				t.Errorf("列表接口应返回掩码，实际 %q（泄漏了真实密码）", s.Pass)
			}
		}
	}
	if id == "" {
		t.Fatal("找不到刚保存的源")
	}

	// ② auth 接口必须给真实密码
	req = httptest.NewRequest("GET", "/api/webdav/auth?id="+id, nil)
	rec = httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("auth 接口应 200，实际 %d: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Auth string `json:"auth"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Auth == "" {
		t.Fatal("auth 为空")
	}
	raw, err := base64.StdEncoding.DecodeString(got.Auth)
	if err != nil {
		t.Fatalf("auth 不是合法 base64: %v", err)
	}
	if string(raw) != "webdav:het002402" {
		t.Fatalf("auth 解码后应为 user:真实密码，实际 %q", string(raw))
	}
	if strings.Contains(string(raw), maskedPassword) {
		t.Fatalf("auth 里不能出现掩码：%q", string(raw))
	}
}

func TestWdAuthRejectsBadID(t *testing.T) {
	SetSecurity(SecurityConfig{})
	defer SetSecurity(SecurityConfig{})
	for _, q := range []string{"", "?id=", "?id=not-exist"} {
		req := httptest.NewRequest("GET", "/api/webdav/auth"+q, nil)
		rec := httptest.NewRecorder()
		Handler().ServeHTTP(rec, req)
		if rec.Code == http.StatusOK {
			t.Errorf("query=%q 不应返回 200", q)
		}
	}
}

// 该接口必须受鉴权保护：否则等于匿名可查任何源的凭据。
func TestWdAuthRequiresLogin(t *testing.T) {
	SetSecurity(SecurityConfig{LoginUser: "u", LoginPass: "p"})
	defer SetSecurity(SecurityConfig{})
	req := httptest.NewRequest("GET", "/api/webdav/auth?id=x", nil)
	rec := httptest.NewRecorder()
	secured().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未登录访问 auth 接口应 401，实际 %d", rec.Code)
	}
}
