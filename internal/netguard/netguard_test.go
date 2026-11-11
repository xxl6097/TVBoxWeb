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
