/**
 * WebSocket 客户端 - 订阅实时推送（站源健康度、信息流等）
 */

export interface HealthUpdate {
  siteKey: string
  health: 'healthy' | 'slow' | 'dead'
  errorRate: number
  avgMs: number
  lastCheck: number
  lastSuccess: number
}

export interface WSMessage {
  type: 'health' | 'feed' | 'pong'
  timestamp: number
  data: any
}

type HealthCallback = (update: HealthUpdate) => void
type FeedCallback = (data: any) => void

class WSClient {
  private ws: WebSocket | null = null
  private url: string
  private reconnectDelay = 1000
  private reconnectAttempts = 0
  private maxReconnectAttempts = 10
  private healthCallbacks: HealthCallback[] = []
  private feedCallbacks: FeedCallback[] = []
  private heartbeatTimer: number | null = null

  constructor(url?: string) {
    // 自动推断 WebSocket URL
    if (!url) {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const base = `${protocol}//${window.location.host}/ws`
      const t = (() => { try { return localStorage.getItem('yz_auth_token') || '' } catch { return '' } })()
      this.url = t ? `${base}?token=${encodeURIComponent(t)}` : base
    } else {
      this.url = url
    }
  }

  /**
   * 连接到 WebSocket 服务器
   */
  connect(): Promise<void> {
    return new Promise((resolve, reject) => {
      try {
        this.ws = new WebSocket(this.url)

        this.ws.onopen = () => {
          console.log('[WS] Connected to', this.url)
          this.reconnectAttempts = 0
          this.reconnectDelay = 1000
          this.startHeartbeat()
          resolve()
        }

        this.ws.onmessage = (event) => {
          try {
            const msg: WSMessage = JSON.parse(event.data)
            this.handleMessage(msg)
          } catch (e) {
            console.error('[WS] Parse message error:', e)
          }
        }

        this.ws.onerror = (error) => {
          console.error('[WS] Connection error:', error)
          reject(error)
        }

        this.ws.onclose = () => {
          console.log('[WS] Disconnected')
          this.stopHeartbeat()
          // 自动重连
          if (this.reconnectAttempts < this.maxReconnectAttempts) {
            this.reconnectAttempts++
            setTimeout(() => this.connect().catch(console.error), this.reconnectDelay)
            this.reconnectDelay = Math.min(this.reconnectDelay * 2, 30000)
          }
        }
      } catch (e) {
        reject(e)
      }
    })
  }

  /**
   * 断开连接
   */
  disconnect(): void {
    this.stopHeartbeat()
    if (this.ws) {
      this.ws.close()
      this.ws = null
    }
  }

  /**
   * 订阅站源健康度更新
   */
  onHealth(callback: HealthCallback): void {
    this.healthCallbacks.push(callback)
  }

  /**
   * 订阅信息流更新
   */
  onFeed(callback: FeedCallback): void {
    this.feedCallbacks.push(callback)
  }

  /**
   * 移除订阅
   */
  offHealth(callback: HealthCallback): void {
    this.healthCallbacks = this.healthCallbacks.filter((cb) => cb !== callback)
  }

  offFeed(callback: FeedCallback): void {
    this.feedCallbacks = this.feedCallbacks.filter((cb) => cb !== callback)
  }

  /**
   * 处理接收到的消息
   */
  private handleMessage(msg: WSMessage): void {
    switch (msg.type) {
      case 'health':
        this.healthCallbacks.forEach((cb) => {
          try {
            cb(msg.data as HealthUpdate)
          } catch (e) {
            console.error('[WS] Health callback error:', e)
          }
        })
        break

      case 'feed':
        this.feedCallbacks.forEach((cb) => {
          try {
            cb(msg.data)
          } catch (e) {
            console.error('[WS] Feed callback error:', e)
          }
        })
        break

      case 'pong':
        // 心跳响应
        break
    }
  }

  /**
   * 启动心跳（定期 Ping）
   */
  private startHeartbeat(): void {
    this.stopHeartbeat()
    this.heartbeatTimer = window.setInterval(() => {
      if (this.ws?.readyState === WebSocket.OPEN) {
        this.ws.send(JSON.stringify({ type: 'ping' }))
      }
    }, 30000) // 30 秒一次 Ping
  }

  /**
   * 停止心跳
   */
  private stopHeartbeat(): void {
    if (this.heartbeatTimer !== null) {
      clearInterval(this.heartbeatTimer)
      this.heartbeatTimer = null
    }
  }

  /**
   * 获取连接状态
   */
  isConnected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN
  }
}

// 创建全局单例
let instance: WSClient | null = null

/**
 * 获取全局 WebSocket 客户端实例
 */
export function getWSClient(): WSClient {
  if (!instance) {
    instance = new WSClient()
  }
  return instance
}

/**
 * 初始化 WebSocket 连接（应在应用启动时调用）
 */
export async function initWS(): Promise<void> {
  const client = getWSClient()
  try {
    await client.connect()
  } catch (e) {
    console.error('[WS] Initialize failed:', e)
    // 允许失败，重连会自动进行
  }
}

/**
 * 关闭 WebSocket 连接
 */
export function closeWS(): void {
  if (instance) {
    instance.disconnect()
    instance = null
  }
}

export default {
  getWSClient,
  initWS,
  closeWS,
}
