package ws

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Handler 包含 WebSocket 端点处理。
type Handler struct {
	hub *Hub
}

// NewHandler 创建 WebSocket 处理器。
func NewHandler(hub *Hub) *Handler {
	return &Handler{hub: hub}
}

// ServeWS 是 /ws 端点处理器。
func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request) {
	// 升级连接
	conn, err := NewConn(w, r, 60*time.Second, 10*time.Second)
	if err != nil {
		http.Error(w, fmt.Sprintf("upgrade error: %v", err), http.StatusBadRequest)
		return
	}

	// 生成客户端 ID
	clientID := uuid.New().String()

	// 创建客户端
	client := &Client{
		ID:   clientID,
		Conn: conn,
		send: make(chan Message, 10),
		done: make(chan struct{}),
	}

	// 注册到 Hub
	h.hub.register <- client

	// 启动读写 goroutines
	go h.readPump(client)
	go h.writePump(client)
}

// readPump 从客户端读取消息。
func (h *Handler) readPump(client *Client) {
	defer func() {
		h.hub.unregister <- client
		client.Conn.Close()
	}()

	for {
		select {
		case <-client.done:
			return
		default:
		}

		messageType, data, err := client.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				// 日志：异常关闭
			}
			return
		}

		if messageType != websocket.TextMessage {
			continue
		}

		// 可选：处理客户端消息（如订阅特定频道）
		var req struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &req); err != nil {
			continue
		}

		// 例如：客户端可以订阅特定的站源或类别
		switch req.Type {
		case "ping":
			// 响应 pong
			client.send <- Message{
				Type:      "pong",
				Timestamp: time.Now().UnixMilli(),
			}
		}
	}
}

// writePump 写消息到客户端。
func (h *Handler) writePump(client *Client) {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		client.Conn.Close()
	}()

	for {
		select {
		case msg, ok := <-client.send:
			if !ok {
				// Hub 关闭了 send 通道
				client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			data, err := json.Marshal(msg)
			if err != nil {
				return
			}

			if err := client.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}

		case <-ticker.C:
			// 定期发送 Ping 保活连接
			if err := client.Conn.Ping(); err != nil {
				return
			}

		case <-client.done:
			return
		}
	}
}

// GlobalHub 全局 WebSocket Hub（在 main 中初始化）。
var GlobalHub *Hub

// Init 初始化全局 Hub。
func Init() {
	GlobalHub = NewHub()
	GlobalHub.Start()
}

// Shutdown 关闭全局 Hub。
func Shutdown() {
	if GlobalHub != nil {
		GlobalHub.Shutdown()
	}
}
