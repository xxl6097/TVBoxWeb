package live

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestFetchEPGXMLTV 用本地 httptest 提供一份 XMLTV，验证频道匹配、
// 时间解析、正在播出/下一档判定，以及 30 分钟缓存是否生效。
func TestFetchEPGXMLTV(t *testing.T) {
	now := time.Now()
	f := func(d time.Duration) string { return now.Add(d).Format("20060102150405 -0700") }
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="cctv1">
    <display-name>CCTV1 综合</display-name>
  </channel>
  <channel id="cctv5">
    <display-name>CCTV5 体育</display-name>
  </channel>
  <programme start="%s" stop="%s" channel="cctv1">
    <title lang="zh">新闻联播</title><desc lang="zh">当日要闻</desc>
  </programme>
  <programme start="%s" stop="%s" channel="cctv1">
    <title lang="zh">焦点访谈</title>
  </programme>
  <programme start="%s" stop="%s" channel="cctv5">
    <title lang="zh">体育新闻</title>
  </programme>
</tv>`, f(-10*time.Minute), f(20*time.Minute),
		f(20*time.Minute), f(50*time.Minute),
		f(-10*time.Minute), f(20*time.Minute))

	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()

	// 频道名带清晰度后缀，应能宽松匹配到 cctv1
	res, err := FetchEPG(srv.URL, "CCTV1 高清")
	if err != nil {
		t.Fatalf("FetchEPG 失败: %v", err)
	}
	if len(res.Programs) != 2 {
		t.Fatalf("节目数期望 2，实际 %d", len(res.Programs))
	}
	if res.Now == nil || res.Now.Title != "新闻联播" {
		t.Fatalf("正在播出判定错误: %+v", res.Now)
	}
	if res.Next == nil || res.Next.Title != "焦点访谈" {
		t.Fatalf("下一档判定错误: %+v", res.Next)
	}
	if res.Now.Start <= 0 || res.Now.Stop <= res.Now.Start {
		t.Fatalf("XMLTV 时间解析错误: %+v", res.Now)
	}

	// 第二次请求应命中内存缓存，不再访问源站
	if _, err := FetchEPG(srv.URL, "CCTV1"); err != nil {
		t.Fatalf("缓存读取失败: %v", err)
	}
	if hits != 1 {
		t.Fatalf("EPG 缓存未生效，源站被请求 %d 次", hits)
	}

	// 未配置地址应给出明确错误
	if _, err := FetchEPG("", "CCTV1"); err == nil {
		t.Fatal("空 EPG 地址应返回错误")
	}
}
