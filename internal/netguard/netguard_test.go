package netguard

import (
	"net"
	"testing"
)

func TestIsPublic(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"127.0.0.1", false},
		{"192.168.1.2", false},
		{"10.6.14.26", false},
		{"172.16.18.59", false},
		{"169.254.169.254", false}, // 云元数据
		{"100.64.1.1", false},      // CGNAT
		{"0.0.0.0", false},
		{"::1", false},
	}
	for _, c := range cases {
		if got := IsPublic(net.ParseIP(c.ip)); got != c.want {
			t.Errorf("IsPublic(%s) = %v, want %v", c.ip, got, c.want)
		}
	}
}

func TestCheckRejectsPrivateEndpoints(t *testing.T) {
	SetConfig(Config{})
	defer SetConfig(Config{})
	for _, raw := range []string{
		"http://127.0.0.1:8090/api/status",
		"http://192.168.1.2/live.m3u8",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.1/secret",
		"http://[::1]/",
		"ftp://example.com/x",
		"http://user:pass@example.com/x",
	} {
		if err := Check(raw); err == nil {
			t.Errorf("Check(%q) 应被拒绝", raw)
		}
	}
}

func TestCheckAllowsPublicLiteral(t *testing.T) {
	SetConfig(Config{})
	defer SetConfig(Config{})
	if err := Check("https://8.8.8.8/ok"); err != nil {
		t.Fatalf("公网地址不应被拒绝: %v", err)
	}
}

func TestAllowPrivateUpstreams(t *testing.T) {
	SetConfig(Config{AllowPrivate: true})
	defer SetConfig(Config{})
	if err := Check("http://192.168.1.2/nas/epg.xml"); err != nil {
		t.Fatalf("显式放开后应允许内网地址: %v", err)
	}
}

func TestAllowedHosts(t *testing.T) {
	SetConfig(Config{AllowHosts: []string{"cdn.example.com", "*.trusted.com"}})
	defer SetConfig(Config{})
	if err := Check("https://evil.com/x"); err == nil {
		t.Error("白名单之外的域名应被拒绝")
	}
	if !hostAllowed("a.trusted.com", []string{"*.trusted.com"}) {
		t.Error("通配域名应匹配子域")
	}
}

func TestSafeClientRejectsRedirect(t *testing.T) {
	SetConfig(Config{})
	defer SetConfig(Config{})
	c := SafeClient(0)
	if c.CheckRedirect == nil {
		t.Fatal("SafeClient 必须带重定向复检")
	}
}

// ---------- 分级策略 ----------

// CheckSource 给「用户自己配置的源」用，必须允许私有地址，
// 否则家里 NAS 的 WebDAV / 直播源 / EPG 全都用不了。
func TestCheckSourceAllowsPrivate(t *testing.T) {
	SetConfig(Config{})
	defer SetConfig(Config{})
	for _, raw := range []string{
		"http://192.168.1.2:5005/dav/影视/",
		"http://10.0.0.9/live.m3u8",
		"https://[fd4f:eb21:d3dc::1]:88/dav/迅雷下载/",
		"http://127.0.0.1:5244/dav",
	} {
		if err := CheckSource(raw); err != nil {
			t.Errorf("CheckSource(%q) 应允许私有地址: %v", raw, err)
		}
	}
	// 协议与非 http(s) 仍然要拦
	if err := CheckSource("file:///etc/passwd"); err == nil {
		t.Error("CheckSource 仍应拒绝非 http(s) 协议")
	}
	if err := CheckSource("http://user:pass@nas.local/dav"); err == nil {
		t.Error("CheckSource 仍应拒绝 URL 内嵌账号密码")
	}
}

// 严格模式与源模式必须行为不同，否则分级没意义。
func TestStrictVsSourcePolicy(t *testing.T) {
	SetConfig(Config{})
	defer SetConfig(Config{})
	raw := "http://192.168.1.2/dav/"
	if err := Check(raw); err == nil {
		t.Fatal("严格模式应拒绝内网地址")
	}
	if err := CheckSource(raw); err != nil {
		t.Fatalf("源模式应允许内网地址: %v", err)
	}
}

// 关键回归：公网域名在内网被解析成 ULA/IPv4 私网时，
// 只要它是用户配置过的源域名，严格模式也必须放行。
// 这里用不会解析的假域名验证「可信域名直接跳过 DNS 判定」。
func TestTrustedHostsBypassDNS(t *testing.T) {
	SetConfig(Config{})
	defer SetConfig(Config{})
	const host = "fs.abber.cn"
	raw := "https://" + host + ":88/dav/迅雷下载/"

	// 白名单外的不可解析域名 → 拒绝（顺带证明这里确实会走 DNS）
	if err := Check("https://no-such-host.invalid/x"); err == nil {
		t.Fatal("无法解析的域名应被拒绝")
	}
	// 加进可信列表 → 直接放行，不再依赖 DNS 解析结果
	SetTrustedHosts([]string{host})
	defer SetTrustedHosts(nil)
	if err := Check(raw); err != nil {
		t.Fatalf("可信域名应跳过内网判定被放行: %v", err)
	}
	// 换成别的域名依旧被拦
	if err := Check("https://other-nas.invalid/dav/"); err == nil {
		t.Error("非可信域名不应被放行")
	}
}

// 可信列表不影响协议/账号密码这类硬性校验
func TestTrustedHostsStillEnforcesBasicRules(t *testing.T) {
	SetConfig(Config{})
	defer SetConfig(Config{})
	SetTrustedHosts([]string{"fs.abber.cn"})
	defer SetTrustedHosts(nil)
	if err := Check("ftp://fs.abber.cn/x"); err == nil {
		t.Error("非 http(s) 协议即使域名可信也要拒绝")
	}
	if err := Check("https://u:p@fs.abber.cn/x"); err == nil {
		t.Error("URL 内嵌账号密码即使域名可信也要拒绝")
	}
}
