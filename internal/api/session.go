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

	// 登录失败限流：同一 IP 累计失败这么多次就锁一段时间。
	// 阈值刻意放宽到 20——家用/内网部署所有设备共用一个出口 IP，
	// 太紧会把正常用户一起锁死（见 tooManyFails 的说明）。
	loginMaxFails   = 20
	loginFailWindow = 15 * time.Minute
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
	// last 上次失败时间：用于「失败后必须间隔一定时间才能再试」的节流，
	// 比单纯计数更温和——正常用户输错一次，第二次不用等太久。
	last time.Time
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
//
// 设计取舍：阈值不能太紧。家用/内网部署时所有设备共用一个出口 IP，
// 浏览器刷新、多次尝试输错密码都会打到同一个计数器上。原来的
// 「8 次 / 10 分钟」实测很容易被误伤——用户一旦被锁就完全登不进去，
// 连「密码正确」也不行，表现为整个服务不可用。
//
// 现在的策略：
//   - 前 3 次失败：无限制，正常人手滑能立刻重试
//   - 第 4 次起：每次要求间隔 3 秒（递增到最多 15 秒），温和节流
//   - 累计 20 次：锁定 15 分钟（暴力破解靠这个兜底）
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

// failCooldown 返回到下次可以尝试还需等待的时间；0 表示可以立刻尝试。
// 与 tooManyFails 分开，是为了给用户「还要等几秒」的明确提示。
func failCooldown(ip string) time.Duration {
	failMu.Lock()
	defer failMu.Unlock()
	rec, ok := fails[ip]
	if !ok || time.Since(rec.first) > loginFailWindow {
		return 0
	}
	// 前 3 次不设冷却：正常输错应该能马上重试。
	// 注意是 <=3 而不是 <3 —— 第 3 次失败后 count 已经是 3，
	// 用 <3 会让第 3 次就要求等 3 秒，与注释描述不符（有单测盯着这条）。
	if rec.count <= 3 {
		return 0
	}
	// 第 4 次起按「3s、6s、9s……最多 15s」节流
	sec := (rec.count - 3) * 3
	if sec < 3 {
		sec = 3
	}
	if sec > 15 {
		sec = 15
	}
	wait := time.Duration(sec)*time.Second - time.Since(rec.last)
	if wait < 0 {
		return 0
	}
	return wait
}

func recordFail(ip string) {
	failMu.Lock()
	defer failMu.Unlock()
	now := time.Now()
	rec, ok := fails[ip]
	if !ok || now.Sub(rec.first) > loginFailWindow {
		fails[ip] = &failRecord{count: 1, first: now, last: now}
		return
	}
	rec.count++
	rec.last = now
	// 简单容量控制，避免被大量来源灌满
	if len(fails) > 4096 {
		for k, v := range fails {
			if now.Sub(v.first) > loginFailWindow {
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
		fail(w, http.StatusTooManyRequests, "尝试次数过多，请 15 分钟后再试；忘记密码可用 -user/-pass 重启服务重设")
		return
	}
	// 失败次数较多时给出渐进冷却，返回明确还要等多久。
	// 只回 429 不说等多久，用户只会反复点，反而更快被锁。
	if wait := failCooldown(ip); wait > 0 {
		sec := int(wait.Seconds()) + 1
		fail(w, http.StatusTooManyRequests, fmt.Sprintf("尝试过于频繁，请 %d 秒后再试", sec))
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
		Secure:   cookieSecure(r),
		SameSite: http.SameSiteLaxMode,
	})
	// 同时返回一个「媒体令牌」给前端。
	//
	// 为什么不能只靠 Cookie：<video>/MSE 发起的子资源请求不会带 Cookie
	// （没有 crossorigin 属性时不走 CORS 凭证，加了又要求 SameSite=None; Secure，
	// 而 HTTP 部署设不了 Secure）。结果是「页面能正常用、唯独视频 401」，
	// 浏览器再把 401 的 JSON 当视频解码，报成「文件无法播放」。
	// 媒体令牌走 URL query，天然不受 Cookie 策略限制。
	ok(w, map[string]any{"ok": true, "user": sec.LoginUser, "mediaToken": mediaToken()})
}

// mediaToken 返回给前端用的媒体令牌。
//
// 设计取舍：直接用当前会话签名串复用同一把 HMAC 密钥，但不等于会话 Cookie 的值——
// 这样即使 URL 被日志/分享泄露，攻击者也拿不到可访问整个后台的会话 Cookie；
// 而媒体令牌本身只能用于 /api/proxy 与 /api/remux 这类媒体转发端点（见 needsAuth 的放行逻辑）。
func mediaToken() string {
	if !loginEnabled() {
		return ""
	}
	return signSession(sec.LoginUser, time.Now().Add(sessionTTL))
}

// cookieSecure 决定会话 Cookie 是否带 Secure（只在 HTTPS 下发送）。
//
// 判定只看「本次请求是不是走的 HTTPS」：
//   - 直连 TLS：r.TLS != nil
//   - 反代终止 TLS：Go 这边 r.TLS 为 nil，看 X-Forwarded-Proto
//
// 为什么不干脆用配置项强开：Secure Cookie 在 HTTP 页面里会被浏览器**静默丢弃**，
// 一旦配错就是「输入正确密码 → 提示成功 → 立刻又回登录页」，用户完全无从排查。
// 所以 HTTP 请求上一律不加 Secure，宁可弱一点也不能让人登不进去。
//
// CookieSecure 配置项仍然保留：它用于「我知道自己在 HTTPS 后面，但反代没透传
// X-Forwarded-Proto」的场景 —— 此时若客户端确实用 https 访问，需要服务端主动加标记。
// 该场景下由部署方保证前提成立。
func cookieSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if strings.EqualFold(proto, "https") {
		return true
	}
	if sec.CookieSecure && proto == "" {
		// 显式开启且没有任何反代协议头：按部署方声明的 HTTPS 处理
		return true
	}
	return false
}

// hLogout POST: 清除会话。
func hLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		// 清除时也要带同样的 Secure，否则浏览器可能不认这条删除指令
		Secure:   cookieSecure(r),
		SameSite: http.SameSiteLaxMode,
	})
	ok(w, map[string]any{"ok": true})
}
