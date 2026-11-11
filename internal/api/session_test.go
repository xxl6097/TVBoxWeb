package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// secured 返回带上鉴权/CORS 中间件的处理器。
// Handler() 只返回裸 mux，中间件是在 tvbox.Server 里套的，
// 所以测鉴权必须自己套一层 Secure。
func secured() http.Handler { return Secure(Handler()) }

func loginReq(user, pass string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"user": user, "pass": pass})
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(string(body)))
	req.RemoteAddr = "203.0.113.9:12345"
	rec := httptest.NewRecorder()
	secured().ServeHTTP(rec, req)
	return rec
}

// flipLast 把最后一个字符换掉，用于制造「签名被改动」的场景。
func flipLast(v string) byte {
	if v == "" {
		return 'x'
	}
	c := v[len(v)-1]
	if c == 'A' {
		return 'B'
	}
	return 'A'
}

func sessionCookie(rec *httptest.ResponseRecorder) string {
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			return c.Value
		}
	}
	return ""
}

func apiWithCookie(path, cookie string) int {
	req := httptest.NewRequest("GET", path, nil)
	req.RemoteAddr = "203.0.113.9:12345"
	if cookie != "" {
		req.Header.Set("Cookie", sessionCookieName+"="+cookie)
	}
	rec := httptest.NewRecorder()
	secured().ServeHTTP(rec, req)
	return rec.Code
}

// 未配置账号密码时行为必须与旧版本一致：不拦任何 API。
func TestLoginDisabledByDefault(t *testing.T) {
	SetSecurity(SecurityConfig{})
	defer SetSecurity(SecurityConfig{})
	if code := apiWithCookie("/api/status", ""); code != 200 {
		t.Fatalf("未启用登录时 /api/status 应可匿名访问，实际 %d", code)
	}
	rec := httptest.NewRecorder()
	secured().ServeHTTP(rec, httptest.NewRequest("GET", "/api/session", nil))
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["needLogin"] != false || body["authed"] != true {
		t.Fatalf("未启用登录时应 needLogin=false authed=true，实际 %v", body)
	}
}

// 启用登录后：无会话被拦、错误密码被拒、正确密码放行、登出后再次被拦。
func TestLoginFlow(t *testing.T) {
	SetSecurity(SecurityConfig{LoginUser: "admin", LoginPass: "s3cr3t-pass"})
	defer SetSecurity(SecurityConfig{})

	// ① 未登录访问 API → 401
	if code := apiWithCookie("/api/status", ""); code != 401 {
		t.Fatalf("未登录应 401，实际 %d", code)
	}

	// ② 登录端点本身不能被鉴权拦住
	sess := httptest.NewRecorder()
	secured().ServeHTTP(sess, httptest.NewRequest("GET", "/api/session", nil))
	if sess.Code != 200 {
		t.Fatalf("/api/session 应匿名可达，实际 %d", sess.Code)
	}

	// ③ 密码错误 → 401 且不下发 Cookie
	bad := loginReq("admin", "wrong")
	if bad.Code != 401 {
		t.Fatalf("密码错误应 401，实际 %d", bad.Code)
	}
	if sessionCookie(bad) != "" {
		t.Fatal("密码错误不应下发会话 Cookie")
	}

	// ④ 用户名错误 → 401
	if c := loginReq("root", "s3cr3t-pass"); c.Code != 401 {
		t.Fatalf("用户名错误应 401，实际 %d", c.Code)
	}

	// ⑤ 正确密码 → 200 + HttpOnly Cookie
	good := loginReq("admin", "s3cr3t-pass")
	if good.Code != 200 {
		t.Fatalf("正确密码应 200，实际 %d: %s", good.Code, good.Body.String())
	}
	var httpOnly, sameSite bool
	for _, c := range good.Result().Cookies() {
		if c.Name == sessionCookieName {
			httpOnly = c.HttpOnly
			sameSite = c.SameSite == 2 // Lax
		}
	}
	if !httpOnly {
		t.Fatal("会话 Cookie 必须是 HttpOnly（前端脚本不可读）")
	}
	if !sameSite {
		t.Fatal("会话 Cookie 应为 SameSite=Lax")
	}

	// ⑥ 带 Cookie 访问 API → 放行
	ck := sessionCookie(good)
	if ck == "" {
		t.Fatal("应下发会话 Cookie")
	}
	if code := apiWithCookie("/api/status", ck); code != 200 {
		t.Fatalf("已登录应放行，实际 %d", code)
	}

	// ⑦ 伪造 Cookie → 401
	if code := apiWithCookie("/api/status", ck+"tampered"); code != 401 {
		t.Fatalf("篡改的 Cookie 应被拒，实际 %d", code)
	}
	if code := apiWithCookie("/api/status", "aaa.bbb"); code != 401 {
		t.Fatalf("随便编的 Cookie 应被拒，实际 %d", code)
	}
}

// 会话签名本身：过期要失效、篡改要失效。
func TestSessionSignVerify(t *testing.T) {
	v := signSession("admin", time.Now().Add(time.Hour))
	if u, ok := verifySession(v); !ok || u != "admin" {
		t.Fatalf("正常会话应校验通过，得到 %q %v", u, ok)
	}
	// 过期
	old := signSession("admin", time.Now().Add(-time.Minute))
	if _, ok := verifySession(old); ok {
		t.Fatal("过期会话应失效")
	}
	// 篡改签名（末尾改一个字符，HMAC 必然对不上）
	tampered := v[:len(v)-1] + string(flipLast(v))
	if _, ok := verifySession(tampered); ok {
		t.Fatal("篡改签名应失效")
	}
	// 篡改载荷：把 base64 段解出来改用户名再拼回去
	dot := strings.LastIndex(v, ".")
	raw, _ := base64.RawURLEncoding.DecodeString(v[:dot])
	parts := strings.SplitN(string(raw), "|", 2)
	forged := base64.RawURLEncoding.EncodeToString([]byte("root|"+parts[1])) + v[dot:]
	if _, ok := verifySession(forged); ok {
		t.Fatal("篡改载荷应失效")
	}
	// 垃圾值
	for _, bad := range []string{"", ".", "abc", "x.y", ".."} {
		if _, ok := verifySession(bad); ok {
			t.Fatalf("非法值 %q 不应通过", bad)
		}
	}
}

// 访问口令与登录可以并存：任一种凭证都能过。
func TestTokenAndLoginCoexist(t *testing.T) {
	SetSecurity(SecurityConfig{AuthToken: "tok-123", LoginUser: "admin", LoginPass: "pw"})
	defer SetSecurity(SecurityConfig{})

	// 无凭证 → 401
	if code := apiWithCookie("/api/status", ""); code != 401 {
		t.Fatalf("无凭证应 401，实际 %d", code)
	}
	// 只带口令 → 放行
	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("X-Auth-Token", "tok-123")
	rec := httptest.NewRecorder()
	secured().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("口令应放行，实际 %d", rec.Code)
	}
}

// 连续失败要触发限流，避免公网被暴力破解。
func TestLoginRateLimit(t *testing.T) {
	SetSecurity(SecurityConfig{LoginUser: "admin", LoginPass: "pw"})
	defer SetSecurity(SecurityConfig{})
	clearFails("198.51.100.7")

	body := `{"user":"admin","pass":"bad"}`
	var last int
	for i := 0; i < loginMaxFails+2; i++ {
		req := httptest.NewRequest("POST", "/api/login", strings.NewReader(body))
		req.RemoteAddr = "198.51.100.7:999"
		rec := httptest.NewRecorder()
		secured().ServeHTTP(rec, req)
		last = rec.Code
	}
	if last != 429 {
		t.Fatalf("超过失败次数应返回 429，实际 %d", last)
	}
	// 正确密码也解不开（窗口内锁定），这是有意的：防止绕过暴力穷举
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"user":"admin","pass":"pw"}`))
	req.RemoteAddr = "198.51.100.7:999"
	rec := httptest.NewRecorder()
	secured().ServeHTTP(rec, req)
	if rec.Code != 429 {
		t.Fatalf("锁定期间正确密码也应被挡，实际 %d", rec.Code)
	}
	clearFails("198.51.100.7")
}
