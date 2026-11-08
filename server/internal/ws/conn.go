package ws

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

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
			// 允许同源 + 本地调试
			return true
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, fmt.Errorf("upgrade websocket: %w", err)
	}

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
