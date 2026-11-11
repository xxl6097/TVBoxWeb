// Package api 的可选安全层：访问口令、CORS 白名单、panic 堆栈与请求指标。
package api

import (
	"crypto/subtle"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"github.com/xxl6097/TVBoxWeb/internal/metrics"
	"github.com/xxl6097/TVBoxWeb/internal/netguard"
)

// SecurityConfig 由 tvbox.Server 在构造时注入。默认零值 = 与原行为兼容
// （无口令、仅同源 CORS、禁止代理内网地址）。
type SecurityConfig struct {
	// AuthToken 非空时，所有 /api/* 与 /ws 请求必须携带该口令。
	AuthToken string
	// AllowedOrigins 额外的 CORS 允许来源（开发时前后端分离用）。
	// 同源请求始终允许。
	AllowedOrigins []string
	// AllowPrivateUpstreams 允许服务端请求内网地址（家用 NAS/WebDAV 场景）。
	AllowPrivateUpstreams bool
	// AllowedHosts 上游域名白名单，为空表示不限制（仍拒绝内网 IP）。
	AllowedHosts []string
	// LoginUser / LoginPass 账号密码登录。两者都非空时启用：
	// /api/* 与 /ws 需要先 POST /api/login 换取会话 Cookie。
	// 未设置时行为与旧版本一致（不启用登录页）。
	LoginUser string
	LoginPass string
	// Logger 用于记录 panic 堆栈，nil 时用标准日志。
	Logger *log.Logger
}

var sec SecurityConfig

// SetSecurity 注入安全配置（由 tvbox.New 调用）。
func SetSecurity(c SecurityConfig) {
	sec = c
	netguard.SetConfig(netguard.Config{
		AllowPrivate: c.AllowPrivateUpstreams,
		AllowHosts:   c.AllowedHosts,
	})
}

// statusRecorder 只用于普通 API 响应；视频流 / WebSocket 直接用原生
// ResponseWriter，避免破坏 Hijack / Flush / 流式转发。
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}
func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = 200
	}
	return s.ResponseWriter.Write(b)
}
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Secure 是完整的外层中间件：CORS 收紧 + 可选口令鉴权 + panic 恢复 + 请求日志 + 指标。
func Secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		isAPI := strings.HasPrefix(r.URL.Path, "/api/")
		requestID := fmt.Sprintf("%x", start.UnixNano())
		lg := sec.Logger
		if lg == nil {
			lg = log.Default()
		}
		var rw http.ResponseWriter = w
		var recorder *statusRecorder
		if isAPI && r.URL.Path != "/api/proxy" {
			recorder = &statusRecorder{ResponseWriter: w}
			rw = recorder
			w.Header().Set("X-Request-ID", requestID)
		}

		defer func() {
			status := 200
			if recorder != nil && recorder.status != 0 {
				status = recorder.status
			}
			if panicVal := recover(); panicVal != nil {
				lg.Printf("PANIC %s %s: %v\n%s", r.Method, r.URL.Path, panicVal, debug.Stack())
				metrics.RecordError()
				failWith(rw, 500, ErrInternal, "内部错误")
				status = 500
			}
			if isAPI {
				ms := time.Since(start).Milliseconds()
				metrics.Record(r.URL.Path, ms)
				lg.Printf("[%s] %s %s %d %dms", requestID, r.Method, r.URL.Path, status, ms)
			}
		}()

		// --- CORS：同源放行，白名单放行，其余不返回 CORS 头 ---
		origin := r.Header.Get("Origin")
		if origin != "" && originAllowed(origin, r.Host) {
			rw.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Auth-Token, Range")
		if r.Method == http.MethodOptions {
			rw.WriteHeader(http.StatusNoContent)
			return
		}

		// --- 可选鉴权（只保护 API 与 WebSocket，静态资源必须能匿名加载）---
		// 两种方式并存：旧的访问口令（-token / Bearer / X-Auth-Token），
		// 以及新的账号密码登录（会话 Cookie）。
		if needsAuth(r.URL.Path) && !authOK(r) {
			if loginEnabled() && sessionUser(r) == "" {
				failWith(rw, 401, ErrAuth, "请先登录")
				return
			}
			if !loginEnabled() && sec.AuthToken != "" {
				failWith(rw, 401, ErrAuth, "需要访问口令，请在「设置 → 访问口令」中配置")
				return
			}
		}
		next.ServeHTTP(rw, r)
	})
}

// CORS 保留旧名称，供直接引用 api.CORS 的第三方代码无缝升级。
func CORS(next http.Handler) http.Handler { return Secure(next) }

// OriginAllowed 供 WebSocket 的 CheckOrigin 复用同一套来源判定。
func OriginAllowed(origin, host string) bool { return originAllowed(origin, host) }

func originAllowed(origin, host string) bool {
	if origin == "" {
		return false
	}
	// 同源：比较 origin 的 host 部分与请求 Host（含端口或仅主机名）
	if u, err := url.Parse(origin); err == nil && u.Host != "" {
		if strings.EqualFold(u.Host, host) || strings.EqualFold(u.Hostname(), hostOnly(host)) {
			return true
		}
	}
	for _, o := range sec.AllowedOrigins {
		if strings.EqualFold(strings.TrimSuffix(strings.TrimSpace(o), "/"), strings.TrimSuffix(origin, "/")) {
			return true
		}
	}
	return false
}

func hostOnly(h string) string {
	if strings.HasPrefix(h, "[") { // IPv6
		if i := strings.Index(h, "]"); i >= 0 {
			return h[1:i]
		}
	}
	if i := strings.LastIndex(h, ":"); i >= 0 {
		return h[:i]
	}
	return h
}

func needsAuth(path string) bool {
	// 登录相关端点必须匿名可达，否则拿不到会话
	switch path {
	case "/api/health", "/api/login", "/api/logout", "/api/session":
		return false
	}
	return strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/ws")
}

// authOK 判断请求是否已通过鉴权：口令正确，或持有有效的登录会话。
// 未启用任何鉴权时恒为 true。
func authOK(r *http.Request) bool {
	// 健康检查不鉴权，保证容器 HEALTHCHECK 可用
	if r.URL.Path == "/api/health" {
		return true
	}
	// 已登录（会话 Cookie 有效）
	if sessionUser(r) != "" {
		return true
	}
	if sec.AuthToken == "" && !loginEnabled() {
		return true
	}
	token := ""
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		token = strings.TrimPrefix(h, "Bearer ")
	}
	if token == "" {
		token = r.Header.Get("X-Auth-Token")
	}
	if token == "" {
		// 视频分片 / WebSocket 由浏览器直接发起，无法带自定义头，
		// 允许通过 query 传递（仅在已开启口令时生效）。
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		return false
	}
	if sec.AuthToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(sec.AuthToken)) == 1
}
