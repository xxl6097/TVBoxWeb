# WebSocket 实时推送功能说明

## 概述

实现 WebSocket 实时推送，无需轮询即可向前端推送站源健康度更新、信息流等数据。

---

## 后端实现

### 1. **Hub 管理（server/internal/ws/hub.go）**

- 集中管理所有连接客户端
- 支持广播消息到所有客户端
- 自动广播站源请求的健康度更新

```go
// BroadcastHealth 广播站源健康度更新
func (h *Hub) BroadcastHealth(siteKey, health string, errorRate float64, 
    avgMs int, lastCheck, lastSuccess int64)

// 健康度分类：healthy（绿） / slow（橙） / dead（红）
```

### 2. **连接处理（server/internal/ws/conn.go）**

- 升级 HTTP 为 WebSocket
- 自动 Ping/Pong 心跳保活（30s）
- 可配置读写超时

```go
// NewConn 升级 HTTP 请求为 WebSocket
func NewConn(w http.ResponseWriter, r *http.Request, 
    readTimeout, writeTimeout time.Duration) (*Conn, error)
```

### 3. **HTTP 端点（server/internal/ws/handler.go）**

- `/ws` 端点处理连接升级
- 读泵（readPump）：从客户端读取消息
- 写泵（writePump）：向客户端写入消息

```go
// ServeWS 处理 /ws 端点
func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request)
```

### 4. **数据流集成（server/internal/store/store.go）**

在 `RecordSiteRequest` 中自动广播健康度更新：

```go
// 记录站源请求后广播 WebSocket
ws.BroadcastHealth(siteKey, health, errorRate, avgMs, now*1000, lastSuccess*1000)
```

---

## 前端实现

### 1. **WebSocket 客户端（web/src/ws.ts）**

```typescript
// 全局单例模式
const client = getWSClient()

// 自动连接 + 重连
await initWS()

// 订阅健康度更新
client.onHealth((update: HealthUpdate) => {
  console.log(update)  // { siteKey, health, errorRate, avgMs, ... }
})

// 断开连接
closeWS()
```

**特性：**
- 自动重连（指数退避，最多 10 次）
- 心跳保活（30s Ping）
- 类型安全的回调（TypeScript）

### 2. **LiveView 集成（web/src/views/LiveView.vue）**

在测速排行面板显示实时健康度指示器：

```vue
<!-- 健康度点 -->
<span
  v-if="getSiteHealth(s.name)"
  class="health-dot"
  :class="getSiteHealth(s.name)!.health"
  :title="`健康度: ${getSiteHealth(s.name)!.health} · 错误率 ${(getSiteHealth(s.name)!.errorRate * 100).toFixed(0)}% · 平均 ${getSiteHealth(s.name)!.avgMs}ms`"
></span>
```

**样式：**
- `healthy` 绿色 ✓ 稳定
- `slow` 橙色 ⚠ 缓慢
- `dead` 红色 ✗ 故障（脉冲动画）

### 3. **应用初始化（web/src/main.ts）**

```typescript
// 启动时自动连接
initWS().catch(console.error)
```

---

## 消息格式

### Health 消息

```json
{
  "type": "health",
  "timestamp": 1794178710000,
  "data": {
    "siteKey": "iFan-自建聚合",
    "health": "healthy",
    "errorRate": 0.05,
    "avgMs": 1200,
    "lastCheck": 1794178710000,
    "lastSuccess": 1794178710000
  }
}
```

### Feed 消息（预留）

```json
{
  "type": "feed",
  "timestamp": 1794178710000,
  "data": { "vods": [...] }
}
```

### Pong 消息

```json
{
  "type": "pong",
  "timestamp": 1794178710000,
  "data": null
}
```

---

## 使用流程

1. **后端启动**时自动初始化 Hub：
   ```go
   ws.Init()
   api.SetWSHub(ws.GlobalHub)
   ```

2. **前端启动**时自动连接：
   ```typescript
   initWS()
   ```

3. **每次 API 请求**（站源）：
   ```go
   store.RecordSiteRequest(siteKey, success, ms)
   // 自动计算健康度 + 广播 WebSocket
   ```

4. **前端接收更新**：
   ```typescript
   ws.onHealth((update) => {
     siteHealthStatus.set(update.siteKey, update)
   })
   ```

5. **UI 渲染**：
   ```typescript
   // LiveView 测速排行中显示实时指示器
   getSiteHealth(s.name)  // 返回最新的健康度信息
   ```

---

## 测试

**Python 测试脚本：**

```python
import websocket
import json

ws = websocket.create_connection("ws://localhost:8091/ws", timeout=5)
print("连接成功")

# 发送 ping
ws.send(json.dumps({"type": "ping"}))

# 接收 pong
data = ws.recv()
msg = json.loads(data)
print(f"收到: {msg}")  # 输出 pong 消息

ws.close()
```

**期望输出：**

```
✓ WebSocket 连接成功
📤 发送 ping 消息...
📨 收到消息: {
  "type": "pong",
  "timestamp": 1794178805245,
  "data": null
}
✓ Pong 响应收到，WebSocket 连接正常！
```

---

## 性能指标

| 指标 | 值 |
|-----|-----|
| 心跳间隔 | 30s |
| 消息队列缓冲 | 100（Hub）/ 10（Client） |
| 自动重连最多尝试 | 10 次 |
| 读写超时 | 60s / 10s |
| 指数退避最大延迟 | 30s |

---

## 未来扩展

1. **信息流实时推送** - `BroadcastFeed()` 推送新视频
2. **爬虫进度推送** - 实时爬虫状态 + 进度条
3. **多客户端消息** - 支持订阅特定频道/类别
4. **离线消息队列** - 持久化并在重连时补发

---

## 文件清单

```
server/
  ├─ internal/ws/
  │  ├─ hub.go        (Hub 核心 + BroadcastHealth)
  │  ├─ conn.go       (WebSocket 连接包装)
  │  ├─ handler.go    (HTTP 端点 + 读写泵)
  │  └─ ...
  ├─ main.go          (ws.Init() + SetWSHub)
  └─ internal/api/api.go  (SetWSHub + 全局访问)

web/src/
  ├─ ws.ts            (客户端 + 自动重连)
  ├─ views/LiveView.vue  (实时健康度指示器)
  ├─ main.ts          (initWS 启动)
  └─ ...
```

**总计：** 3 个新 Go 文件 + 1 个新 TS 文件 + 2 个文件修改 = 6 个文件变更
