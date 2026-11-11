package ws

import (
	"fmt"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// originChecker 由 tvbox.New 注入（复用 api.OriginAllowed）。
// 未注入时默认只允许同源；浏览器之外的客户端不带 Origin，直接放行。
var originChecker atomic.Value

// SetOriginChecker 注入来源判定函数。
func SetOriginChecker(fn func(origin, host string) bool) {
	if fn != nil {
		originChecker.Store(fn)
	}
}

func sameOrigin(origin, host string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Host == host {
		return true
	}
	h := host
	if i := lastColon(h); i >= 0 {
		h = h[:i]
	}
	return u.Hostname() == h
}

func lastColon(h string) int {
	for i := len(h) - 1; i >= 0; i-- {
		if h[i] == ':' {
			return i
		}
	}
	return -1
}

// Conn 包装 gorilla/websocket.Conn，处理读写。
type Conn struct {
	conn *websocket.Conn

	// 缓冲读写超时
	readTimeout  time.Duration
	writeTimeout time.Duration
}

// NewConn 创建一个新的 Conn 包装。
func NewConn(w http.ResponseWriter, r *http.Request, readTimeout, writeTimeout time.Duration) (*Conn, error) {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true // 非浏览器客户端
			}
			if fn, ok := originChecker.Load().(func(string, string) bool); ok {
				return fn(origin, r.Host)
			}
			return sameOrigin(origin, r.Host)
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, fmt.Errorf("upgrade websocket: %w", err)
	}

	// 防止超大消息打满内存
	conn.SetReadLimit(1 << 20)
	// 设置 Pong 处理，响应 Ping
	conn.SetReadDeadline(time.Now().Add(readTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(readTimeout))
		return nil
	})

	return &Conn{
		conn:         conn,
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
	}, nil
}

// ReadMessage 从 WebSocket 读取消息。
func (c *Conn) ReadMessage() (int, []byte, error) {
	return c.conn.ReadMessage()
}

// WriteMessage 写消息到 WebSocket。
func (c *Conn) WriteMessage(messageType int, data []byte) error {
	c.conn.SetWriteDeadline(time.Now().Add(c.writeTimeout))
	return c.conn.WriteMessage(messageType, data)
}

// Close 关闭连接。
func (c *Conn) Close() error {
	return c.conn.Close()
}

// Ping 发送 Ping 帧。
func (c *Conn) Ping() error {
	c.conn.SetWriteDeadline(time.Now().Add(c.writeTimeout))
	return c.conn.WriteMessage(websocket.PingMessage, []byte{})
}
