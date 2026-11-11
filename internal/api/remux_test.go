package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// FFmpegAvailable 至少要能稳定返回一对值，不能 panic 或每次结果不一致。
func TestFFmpegAvailableStable(t *testing.T) {
	p1, r1 := FFmpegAvailable()
	p2, r2 := FFmpegAvailable()
	if p1 != p2 || r1 != r2 {
		t.Fatalf("两次调用结果不一致：(%q,%q) vs (%q,%q)", p1, r1, p2, r2)
	}
	// 两者必须互斥：有路径就没有原因，有原因就没有路径
	if (p1 == "") == (r1 == "") {
		t.Fatalf("路径与原因应互斥，实际 path=%q reason=%q", p1, r1)
	}
}

// 缺 url / 协议不对：必须 400，且不能被当成有效转封装请求。
func TestRemuxRejectsBadParams(t *testing.T) {
	cases := []struct {
		query string
		want  int
	}{
		{"", http.StatusBadRequest},
		{"?url=", http.StatusBadRequest},
		{"?url=ftp://x/y.mkv", http.StatusBadRequest},
		{"?url=" + url.QueryEscape("/local/file.mkv"), http.StatusBadRequest},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", "/api/remux"+c.query, nil)
		rec := httptest.NewRecorder()
		hRemux(rec, req)
		if rec.Code != c.want {
			t.Errorf("query=%q 期望 %d，实际 %d（body=%s）", c.query, c.want, rec.Code, rec.Body.String())
		}
	}
}

// 内网地址在严格模式下必须被拒绝，避免 /api/remux 变成 SSRF 入口。
func TestRemuxBlocksPrivateTarget(t *testing.T) {
	saved := sec
	defer func() { sec = saved }()
	// 明确关闭内网放行
	SetSecurity(SecurityConfig{AllowPrivateUpstreams: false})
	defer SetSecurity(SecurityConfig{})

	req := httptest.NewRequest("GET", "/api/remux?url="+url.QueryEscape("http://127.0.0.1:9/a.mkv"), nil)
	rec := httptest.NewRecorder()
	hRemux(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("内网地址应被拒，实际 %d（body=%s）", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "不安全") {
		t.Errorf("错误信息应说明地址不安全，实际 %s", rec.Body.String())
	}
}

// limitedWriter 不能让 ffmpeg 的日志把内存吃满。
func TestLimitedWriterCap(t *testing.T) {
	var sb strings.Builder
	lw := &limitedWriter{w: &sb, max: 16}
	for i := 0; i < 100; i++ {
		n, err := lw.Write([]byte("0123456789"))
		if err != nil || n != 10 {
			t.Fatalf("Write 应返回全部长度，n=%d err=%v", n, err)
		}
	}
	if sb.Len() > 16 {
		t.Fatalf("输出应被限制在 16 字节内，实际 %d", sb.Len())
	}
}

// firstLine 取第一行有效信息，避免把整段日志抛给用户。
func TestFirstLine(t *testing.T) {
	cases := map[string]string{
		"":                         "",
		"\n\n  \n实际上有内容":           "实际上有内容",
		"第一行\n第二行":                 "第一行",
		"  Invalid data found  \n": "Invalid data found",
	}
	for in, want := range cases {
		if got := firstLine(in); got != want {
			t.Errorf("firstLine(%q) = %q，期望 %q", in, got, want)
		}
	}
}
