// Package httpc 提供精简 HTTP 客户端：GET + 手写重定向跟随 + 中文错误提示，
// 与鸿蒙影视 Http.ets 行为一致。
package httpc

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const UA = "Mozilla/5.0 (Linux; HarmonyOS; SmartScreen) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// WebUA 桌面浏览器 UA：视频流代理专用。
// 鸿蒙 SmartScreen UA 会被部分视频 CDN（如 Fastly/媒体站）判定为异常设备返回 403，
// 导致分片/整片拉取失败、进度条 seek 卡死；视频流改用通用桌面 Chrome UA 保证兼容。
const WebUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// StreamClient 视频流代理专用：连接池复用 + 流式长连接。
// 关键点：不设置 Client.Timeout（视频分片/长流会被整体超时切断），
// 只在握手阶段（拨号 / TLS / 响应头）设超时，避免单个请求永久挂起。
var StreamClient = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DisableCompression:    true, // 视频已是压缩格式，且需要透传 Content-Length/Range
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   16, // 同源 seek/分片请求复用连接
		MaxConnsPerHost:       32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	},
}

// Text 拉取文本，自动跟随最多 3 次重定向，4xx 给出中文提示。
func Text(u string, extra map[string]string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	cur := u
	for hop := 0; hop < 3; hop++ {
		code, body, loc, err := once(cur, extra, timeout)
		if err != nil {
			return "", fmt.Errorf("网络请求失败：%v", err)
		}
		if code >= 300 && code < 400 && loc != "" {
			cur = abs(cur, loc)
			continue
		}
		if code == 0 {
			return body, nil
		}
		if code >= 400 {
			hint := ""
			switch code {
			case 403:
				hint = "，被站点拒绝（可能需要 Referer 或已限制地区）"
			case 429:
				hint = "，请求过于频繁被限流"
			case 404:
				hint = "，接口不存在（站源可能已失效）"
			}
			return "", fmt.Errorf("HTTP %d%s", code, hint)
		}
		return body, nil
	}
	return "", fmt.Errorf("网络请求失败：重定向次数过多")
}

// Peek 只抓首包（前 4KB），用来判断流地址「活不活」「快不快」。
// 抛异常 = 连不上；返回空串 = 服务端给了 4xx/5xx。
func Peek(u string, ms time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ms)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", UA)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Range", "bytes=0-4095")
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("连接失败：%v", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return "", nil
	}
	buf := make([]byte, 4096)
	n, _ := io.ReadFull(res.Body, buf)
	if n == 0 {
		return "", nil
	}
	ct := res.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/") && !strings.Contains(ct, "mpegurl") &&
		!strings.Contains(ct, "javascript") && !strings.Contains(ct, "json") {
		return "#BIN", nil
	}
	s := string(buf[:n])
	if len(s) > 4096 {
		s = s[:4096]
	}
	return s, nil
}

// Propfind 发送 WebDAV PROPFIND 请求，Depth: 1 列当前目录直接子项。
func Propfind(u string, extra map[string]string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", UA)
	req.Header.Set("Depth", "1")
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("WebDAV 请求失败：%v", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		hint := ""
		switch res.StatusCode {
		case 401:
			hint = "，用户名或密码错误"
		case 403:
			hint = "，无权限访问该路径"
		case 404:
			hint = "，路径不存在"
		}
		return "", fmt.Errorf("WebDAV HTTP %d%s", res.StatusCode, hint)
	}
	return string(body), nil
}

// once 单次 GET，返回状态码、body、location。
func once(u string, extra map[string]string, ms time.Duration) (int, string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ms)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return 0, "", "", err
	}
	req.Header.Set("User-Agent", UA)
	req.Header.Set("Accept", "*/*")
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	res, err := client.Do(req)
	if err != nil {
		return 0, "", "", err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(body), res.Header.Get("Location"), nil
}

// abs 相对地址拼绝对地址（与 Http.abs 一致）。
func abs(base, loc string) string {
	if strings.HasPrefix(loc, "http://") || strings.HasPrefix(loc, "https://") {
		return loc
	}
	i := strings.Index(base, "://")
	if i < 0 {
		return loc
	}
	slash := strings.Index(base[i+3:], "/")
	if strings.HasPrefix(loc, "/") {
		if slash < 0 {
			return base + loc
		}
		return base[:i+3+slash] + loc
	}
	if slash < 0 {
		return base + "/" + loc
	}
	return base[:i+3+slash+1] + loc
}

// AbsURL 计算绝对地址（公开给其他包用）。
func AbsURL(base, ref string) string { return abs(base, ref) }

// EncodePath 逐段 URL 编码路径，保留 '/'。
func EncodePath(p string) string {
	if p == "" {
		return "/"
	}
	parts := strings.Split(p, "/")
	out := make([]string, 0, len(parts))
	for _, s := range parts {
		if s != "" {
			out = append(out, url.PathEscape(s))
		}
	}
	return "/" + strings.Join(out, "/")
}
