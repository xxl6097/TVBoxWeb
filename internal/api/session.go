package api

// 账号密码登录与会话。
//
// 设计取舍：
//   - 会话放在 HttpOnly Cookie 里，不用 localStorage 存口令——前端脚本读不到，
//     XSS 也偷不走；视频分片、WebSocket 这类浏览器直发的请求天然带得上。
//   - 签名密钥每个进程随机生成：进程重启（含升级替换后重启）所有会话失效，
//     代价是重新登录一次，换来「密钥永不落盘」。
//   - 密码只做等时比较，不落盘、不进日志；错误信息不区分「用户不存在」和
//     「密码错误」，避免被用来枚举用户名。

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	sessionCookieName = "yz_session"
	sessionTTL        = 7 * 24 * time.Hour

	// 登录失败限流：同一 IP 在窗口期内失败这么多次就暂时锁住
	loginMaxFails   = 8
	loginFailWindow = 10 * time.Minute
)

var sessionKey = func() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// 极端情况下退化成一个固定串也无所谓：只影响会话签名强度，
		// 不影响口令校验本身（口令仍然走等时比较）。
		return []byte("yezhen-session-key-fallback")
	}
	return b
}()

// ---------- 会话签名 ----------

func signSession(user string, exp time.Time) string {
	payload := fmt.Sprintf("%s|%d", user, exp.Unix())
	mac := hmac.New(sha256.New, sessionKey)
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sig
}

func verifySession(v string) (string, bool) {
	dot := strings.LastIndex(v, ".")
	if dot <= 0 {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(v[:dot])
	if err != nil {
		return "", false
	}
	payload := string(raw)
	user, expStr, ok := strings.Cut(payload, "|")
	if !ok {
		return "", false
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return "", false
	}
	want := signSession(user, time.Unix(exp, 0))
	if subtle.ConstantTimeCompare([]byte(v), []byte(want)) != 1 {
		return "", false
	}
	return user, true
}

// sessionUser 从请求里取出已登录用户（空表示未登录）。
func sessionUser(r *http.Request) string {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return ""
	}
	user, ok := verifySession(c.Value)
	if !ok {
		return ""
	}
	return user
}

// loginEnabled 是否配置了登录账号（两者都非空才启用）。
func loginEnabled() bool {
	return sec.LoginUser != "" && sec.LoginPass != ""
}

// ---------- 登录失败限流 ----------

type failRecord struct {
	count int
	first time.Time
}

var (
	failMu sync.Mutex
	fails  = map[string]*failRecord{}
)

func clientIP(r *http.Request) string {
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return h
	}
	return r.RemoteAddr
}

// tooManyFails 判断该来源是否已被临时锁定。
func tooManyFails(ip string) bool {
	failMu.Lock()
	defer failMu.Unlock()
	rec, ok := fails[ip]
	if !ok {
		return false
	}
	if time.Since(rec.first) > loginFailWindow {
		delete(fails, ip)
		return false
	}
	return rec.count >= loginMaxFails
}

func recordFail(ip string) {
	failMu.Lock()
	defer failMu.Unlock()
	rec, ok := fails[ip]
	if !ok || time.Since(rec.first) > loginFailWindow {
		fails[ip] = &failRecord{count: 1, first: time.Now()}
		return
	}
	rec.count++
	// 简单容量控制，避免被大量来源灌满
	if len(fails) > 4096 {
		for k, v := range fails {
			if time.Since(v.first) > loginFailWindow {
				delete(fails, k)
			}
		}
	}
}

func clearFails(ip string) {
	failMu.Lock()
	delete(fails, ip)
	failMu.Unlock()
}

// ---------- HTTP 处理器 ----------

// hSession GET: 返回当前登录状态，前端路由守卫用它决定是否放行。
func hSession(w http.ResponseWriter, r *http.Request) {
	user := sessionUser(r)
	ok(w, map[string]any{
		"needLogin": loginEnabled(),
		"authed":    !loginEnabled() || user != "",
		"user":      user,
	})
}

// hLogin POST: 账号密码换会话 Cookie。
func hLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "仅支持 POST")
		return
	}
	if !loginEnabled() {
		// 没启用登录时直接给个成功，别让前端卡在登录页
		ok(w, map[string]any{"ok": true, "user": ""})
		return
	}
	ip := clientIP(r)
	if tooManyFails(ip) {
		fail(w, http.StatusTooManyRequests, "尝试次数过多，请 10 分钟后再试")
		return
	}

	var req struct {
		User string `json:"user"`
		Pass string `json:"pass"`
	}
	if err := body(r, &req); err != nil {
		fail(w, http.StatusBadRequest, "参数错误")
		return
	}

	userOK := subtle.ConstantTimeCompare([]byte(strings.TrimSpace(req.User)), []byte(sec.LoginUser)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(req.Pass), []byte(sec.LoginPass)) == 1
	if !userOK || !passOK {
		recordFail(ip)
		// 不区分「用户不存在」与「密码错误」，避免被用来枚举用户名
		fail(w, http.StatusUnauthorized, "账号或密码不正确")
		return
	}
	clearFails(ip)

	exp := time.Now().Add(sessionTTL)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    signSession(sec.LoginUser, exp),
		Path:     "/",
		Expires:  exp,
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		// 不设 Secure：家用 HTTP 部署也要能用；上 HTTPS 后由反代保证链路加密
	})
	ok(w, map[string]any{"ok": true, "user": sec.LoginUser})
}

// hLogout POST: 清除会话。
func hLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	ok(w, map[string]any{"ok": true})
}
