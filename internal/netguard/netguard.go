// Package netguard 提供服务端请求上游 URL 的安全校验，防止 SSRF。
//
// 影视聚合类服务天然需要请求用户填写的订阅/直播源地址，因此采用分级策略：
//   - /api/proxy、/api/peek 属「内容回显型」接口，默认拒绝私有网络地址；
//   - 订阅、EPG、WebDAV 等允许用户指向内网 NAS，可通过 WithAllowPrivateUpstreams 放开。
package netguard

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Config 是全局安全配置，由 api.SetSecurity 注入。
type Config struct {
	// AllowPrivate 为 true 时允许目标解析到私有/保留 IP 段。
	AllowPrivate bool
	// AllowHosts 非空时只允许这些域名（支持 "example.com" 与 "*.example.com"），
	// 其余一律拒绝；为空表示不限制公网域名。
	AllowHosts []string
}

var (
	mu     sync.RWMutex
	config Config

	// DNS 判定结果短缓存，避免视频分片代理时每个请求都做一次解析。
	cacheMu sync.Mutex
	cache   = map[string]cacheItem{}
)

type cacheItem struct {
	err error
	at  time.Time
}

const cacheTTL = 30 * time.Second

// SetConfig 更新全局配置（启动时调用一次）。
func SetConfig(c Config) {
	mu.Lock()
	defer mu.Unlock()
	config = c
}

func current() Config {
	mu.RLock()
	defer mu.RUnlock()
	return config
}

// Check 校验一个上游 URL 是否允许被服务端请求。
func Check(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("URL 格式不合法：%v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("仅支持 http/https 协议")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL 缺少主机名")
	}
	if u.User != nil {
		return fmt.Errorf("URL 不允许携带账号密码")
	}

	cfg := current()
	if len(cfg.AllowHosts) > 0 && !hostAllowed(host, cfg.AllowHosts) {
		return fmt.Errorf("目标域名 %s 不在允许列表中", host)
	}
	if cfg.AllowPrivate {
		return nil
	}

	// 字面量 IP 直接判定，无需 DNS
	if ip := net.ParseIP(host); ip != nil {
		if !IsPublic(ip) {
			return fmt.Errorf("禁止访问内网/保留地址 %s", host)
		}
		return nil
	}

	cacheMu.Lock()
	ci, hit := cache[host]
	cacheMu.Unlock()
	if hit && time.Since(ci.at) < cacheTTL {
		return ci.err
	}

	err = checkResolved(context.Background(), host)
	cacheMu.Lock()
	cache[host] = cacheItem{err: err, at: time.Now()}
	// 简单容量控制，避免被大量不同域名灌满
	if len(cache) > 512 {
		for k, v := range cache {
			if time.Since(v.at) > cacheTTL {
				delete(cache, k)
			}
		}
	}
	cacheMu.Unlock()
	return err
}

func checkResolved(ctx context.Context, host string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("解析域名 %s 失败：%v", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("域名 %s 没有解析结果", host)
	}
	for _, a := range addrs {
		if !IsPublic(a.IP) {
			return fmt.Errorf("目标 %s 解析到内网/保留地址 %s，已拒绝", host, a.IP)
		}
	}
	return nil
}

// IsPublic 判断 IP 是否为可安全访问的公网地址。
func IsPublic(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	// 云元数据、运营商级 NAT、保留段等额外排除
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 169 && ip4[1] == 254 { // 169.254.0.0/16（含元数据）
			return false
		}
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 { // 100.64.0.0/10
			return false
		}
		if ip4[0] == 0 || ip4[0] >= 240 { // 0.0.0.0/8、240.0.0.0/4
			return false
		}
	}
	return true
}

func hostAllowed(host string, list []string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, raw := range list {
		p := strings.ToLower(strings.TrimSpace(raw))
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "*.") {
			if strings.HasSuffix(host, p[1:]) || host == p[2:] {
				return true
			}
		} else if host == p {
			return true
		}
	}
	return false
}

// SafeClient 返回带「重定向逐跳复检」的 HTTP 客户端，供代理类接口使用。
func SafeClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("重定向次数过多")
			}
			return Check(req.URL.String())
		},
	}
}
