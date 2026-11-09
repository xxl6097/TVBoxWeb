package ws

import (
	"encoding/json"
	"sync"
	"time"
)

// Message 是 WebSocket 消息体。
type Message struct {
	Type      string      `json:"type"`      // "health" | "feed" | "crawl"
	Timestamp int64       `json:"timestamp"` // Unix 毫秒
	Data      interface{} `json:"data"`
}

// HealthUpdate 是站源健康度实时更新。
type HealthUpdate struct {
	SiteKey    string  `json:"siteKey"`
	Health     string  `json:"health"`      // "healthy" | "slow" | "dead"
	ErrorRate  float64 `json:"errorRate"`   // 0~1
	AvgMs      int     `json:"avgMs"`       // 平均响应时间
	LastCheck  int64   `json:"lastCheck"`   // Unix 毫秒
	LastSuccess int64  `json:"lastSuccess"` // Unix 毫秒
}

// Client 代表一个连接的 WebSocket 客户端。
type Client struct {
	ID   string
	Conn *Conn
	send chan Message
	done chan struct{}
}

// Hub 管理所有 WebSocket 连接。
type Hub struct {
	mu      sync.RWMutex
	clients map[string]*Client

	// 广播消息到所有连接
	broadcast chan Message

	// 管理连接的生命周期
	register   chan *Client
	unregister chan *Client

	// 用于优雅关闭
	shutdown chan struct{}
}

// NewHub 创建一个新的 WebSocket Hub。
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		broadcast:  make(chan Message, 100),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		shutdown:   make(chan struct{}),
	}
}

// Start 启动 Hub 的事件循环。
func (h *Hub) Start() {
	go func() {
		for {
			select {
			case client := <-h.register:
				h.mu.Lock()
				h.clients[client.ID] = client
				h.mu.Unlock()

			case client := <-h.unregister:
				h.mu.Lock()
				if _, ok := h.clients[client.ID]; ok {
					delete(h.clients, client.ID)
					close(client.send)
				}
				h.mu.Unlock()

			case msg := <-h.broadcast:
				h.mu.RLock()
				for _, client := range h.clients {
					select {
					case client.send <- msg:
					default:
						// 避免阻塞：如果客户端发送队列满，跳过此消息
					}
				}
				h.mu.RUnlock()

			case <-h.shutdown:
				return
			}
		}
	}()
}

// Broadcast 广播消息到所有连接的客户端。
func (h *Hub) Broadcast(msg Message) {
	select {
	case h.broadcast <- msg:
	case <-h.shutdown:
	}
}

// BroadcastHealth 广播站源健康度更新。
func (h *Hub) BroadcastHealth(siteKey, health string, errorRate float64, avgMs int, lastCheck, lastSuccess int64) {
	msg := Message{
		Type:      "health",
		Timestamp: time.Now().UnixMilli(),
		Data: HealthUpdate{
			SiteKey:     siteKey,
			Health:      health,
			ErrorRate:   errorRate,
			AvgMs:       avgMs,
			LastCheck:   lastCheck,
			LastSuccess: lastSuccess,
		},
	}
	h.Broadcast(msg)
}

// BroadcastFeed 广播新的信息流更新（可选）。
func (h *Hub) BroadcastFeed(data json.RawMessage) {
	msg := Message{
		Type:      "feed",
		Timestamp: time.Now().UnixMilli(),
		Data:      data,
	}
	h.Broadcast(msg)
}

// Shutdown 关闭 Hub 并断开所有连接。
func (h *Hub) Shutdown() {
	close(h.shutdown)
	h.mu.Lock()
	for _, client := range h.clients {
		close(client.done)
	}
	h.mu.Unlock()
}

// Size 返回当前连接数。
func (h *Hub) Size() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
