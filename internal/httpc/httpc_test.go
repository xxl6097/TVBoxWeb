package httpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xxl6097/TVBoxWeb/internal/netguard"
)

// slowServer 模拟一个「很慢才返回」的上游，用来验证 ctx 取消能真正中断请求。
func slowServer(delay time.Duration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(delay):
			_, _ = w.Write([]byte("late body"))
		case <-r.Context().Done():
			// 上游感知到客户端断开，立即收工
		}
	}))
}

func TestTextCtxCancelsUpstream(t *testing.T) {
	srv := slowServer(3 * time.Second)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	// 传入 10s 的请求超时，若 ctx 未透传则至少要等 3s 才返回
	_, err := TextCtx(ctx, srv.URL, nil, 10*time.Second)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("ctx 已超时，TextCtx 应当返回错误")
	}
	if elapsed > time.Second {
		t.Fatalf("ctx 取消没有中断上游请求，耗时 %v", elapsed)
	}
}

func TestTextCtxReturnsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	got, err := TextCtx(context.Background(), srv.URL, nil, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Fatalf("body = %q, want hello", got)
	}
}

func TestPeekCtxCancelsUpstream(t *testing.T) {
	srv := slowServer(3 * time.Second)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _ = PeekCtx(ctx, srv.URL, 10*time.Second)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("PeekCtx 未透传 ctx，耗时 %v", elapsed)
	}
}

func TestPropfindCtxCancelsUpstream(t *testing.T) {
	srv := slowServer(3 * time.Second)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := PropfindCtx(ctx, srv.URL, nil, 10*time.Second)
	if err == nil {
		t.Fatal("ctx 已超时，PropfindCtx 应当返回错误")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("PropfindCtx 未透传 ctx，耗时 %v", elapsed)
	}
}

// Text 无 ctx 版本必须保持原有行为（重定向跟随 + 4xx 中文提示）。
func TestTextLegacyBehaviourUnchanged(t *testing.T) {
	// 重定向复检会拒绝 127.0.0.1（SSRF 防护），单测里显式放开内网
	netguard.SetConfig(netguard.Config{AllowPrivate: true})
	defer netguard.SetConfig(netguard.Config{})

	mux := http.NewServeMux()
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ok", http.StatusFound)
	})
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("after redirect"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	got, err := Text(srv.URL+"/redirect", nil, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "after redirect") {
		t.Fatalf("重定向后应拿到最终内容，得到 %q", got)
	}
}
