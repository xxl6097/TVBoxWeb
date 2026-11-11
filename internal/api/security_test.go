package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOriginAllowed(t *testing.T) {
	SetSecurity(SecurityConfig{AllowedOrigins: []string{"https://dev.example.com"}})
	defer SetSecurity(SecurityConfig{})

	if !OriginAllowed("http://localhost:8090", "localhost:8090") {
		t.Error("同源请求必须允许")
	}
	if !OriginAllowed("https://dev.example.com", "example.com") {
		t.Error("白名单来源必须允许")
	}
	if OriginAllowed("https://evil.com", "example.com") {
		t.Error("未授权来源必须拒绝")
	}
}

func TestNeedsAuth(t *testing.T) {
	for _, p := range []string{"/api/status", "/api/backup", "/api/upgrade/run", "/ws"} {
		if !needsAuth(p) {
			t.Errorf("%s 应纳入鉴权范围", p)
		}
	}
	for _, p := range []string{"/", "/assets/index.js", "/sw.js", "/index.html"} {
		if needsAuth(p) {
			t.Errorf("%s 是静态资源，不应要求鉴权", p)
		}
	}
}

func TestAuthOK(t *testing.T) {
	SetSecurity(SecurityConfig{AuthToken: "s3cr3t"})
	defer SetSecurity(SecurityConfig{})

	mk := func(path, header, value string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		if header != "" {
			r.Header.Set(header, value)
		}
		return r
	}
	if authOK(mk("/api/status", "", "")) {
		t.Error("缺少口令应被拒绝")
	}
	if !authOK(mk("/api/status", "X-Auth-Token", "s3cr3t")) {
		t.Error("X-Auth-Token 应通过")
	}
	if !authOK(mk("/api/status", "Authorization", "Bearer s3cr3t")) {
		t.Error("Bearer 应通过")
	}
	if authOK(mk("/api/status", "X-Auth-Token", "wrong")) {
		t.Error("错误口令应被拒绝")
	}
	// 健康检查不鉴权（容器 HEALTHCHECK 需要）
	if !authOK(mk("/api/health", "", "")) {
		t.Error("健康检查应放行")
	}
	// 视频分片通过 query 传递
	if !authOK(mk("/api/proxy?url=x&token=s3cr3t", "", "")) {
		t.Error("query token 应通过")
	}
}

func TestSecureRejectsWithoutToken(t *testing.T) {
	SetSecurity(SecurityConfig{AuthToken: "s3cr3t"})
	defer SetSecurity(SecurityConfig{})

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := Secure(next)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("无口令请求应返回 401，得到 %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ERR_AUTH") {
		t.Fatalf("错误响应应带结构化的 code，得到 %s", w.Body.String())
	}

	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	r2.Header.Set("X-Auth-Token", "s3cr3t")
	h.ServeHTTP(w2, r2)
	if w2.Code != http.StatusOK {
		t.Fatalf("带口令请求应放行，得到 %d", w2.Code)
	}
}

func TestRewriteM3u8CarriesToken(t *testing.T) {
	out := rewriteM3u8("#EXTM3U\nseg0.ts\nseg1.ts\n", "https://cdn.example.com/hls/", "", "abc123")
	if !strings.Contains(out, "token=abc123") {
		t.Fatalf("m3u8 分片地址应透传访问口令，得到:\n%s", out)
	}
	if !strings.Contains(out, "/api/proxy?url=") {
		t.Fatal("分片地址应走代理")
	}
	// 未开启口令时不应出现 token 参数
	plain := rewriteM3u8("seg0.ts\n", "https://cdn.example.com/hls/", "", "")
	if strings.Contains(plain, "token=") {
		t.Fatal("未开启口令时不应附加 token")
	}
}
