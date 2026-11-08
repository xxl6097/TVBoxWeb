/**
 * 夜帧影视 Service Worker
 * 
 * 缓存策略：
 * 1. 应用壳（index.html / manifest / 图标）：预缓存 + Network-First
 * 2. 静态资源（/assets/* 带 hash 文件名）：Cache-First（不可变）
 * 3. API GET 请求（/api/*）：Stale-While-Revalidate（离线兜底）
 * 4. 视频代理流（/api/proxy/*）：不缓存（流媒体）
 * 5. WebSocket（/ws）：不拦截
 */

const VERSION = 'yzz-v1'

const CACHE_SHELL = `${VERSION}-shell`
const CACHE_STATIC = `${VERSION}-static`
const CACHE_API = `${VERSION}-api`

// 预缓存的应用壳资源
const PRECACHE_URLS = [
  '/',
  '/index.html',
  '/manifest.json',
  '/icons/icon-192.png',
  '/icons/icon-512.png',
  '/icons/icon-maskable-192.png',
  '/icons/favicon-32.png',
  '/icons/apple-touch-icon.png'
]

// install：预缓存应用壳
self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_SHELL)
      .then((cache) => cache.addAll(PRECACHE_URLS))
      .then(() => self.skipWaiting())
  )
})

// activate：清理旧版本缓存
self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys()
      .then((keys) => {
        const valid = [CACHE_SHELL, CACHE_STATIC, CACHE_API]
        return Promise.all(
          keys.filter((k) => !valid.includes(k)).map((k) => caches.delete(k))
        )
      })
      .then(() => self.clients.claim())
  )
})

// message：处理 SKIP_WAITING（手动更新时立即生效）
self.addEventListener('message', (event) => {
  if (event.data && event.data.type === 'SKIP_WAITING') {
    self.skipWaiting()
  }
})

// fetch：按 URL 类型分流处理
self.addEventListener('fetch', (event) => {
  const request = event.request

  // 只处理 GET 请求
  if (request.method !== 'GET') return

  const url = new URL(request.url)

  // 跨域请求不拦截（视频源、图片 CDN 等）
  if (url.origin !== self.location.origin) return

  // WebSocket 升级请求不拦截
  if (request.headers.get('upgrade') === 'websocket') return

  // 视频代理流：不缓存（流媒体，实时性优先）
  if (url.pathname.startsWith('/api/proxy')) return

  // API GET 请求：Stale-While-Revalidate（离线兜底）
  if (url.pathname.startsWith('/api/')) {
    event.respondWith(staleWhileRevalidate(request, CACHE_API))
    return
  }

  // 静态资源（带 hash 文件名，不可变）：Cache-First
  if (url.pathname.startsWith('/assets/') || url.pathname.startsWith('/icons/')) {
    event.respondWith(cacheFirst(request, CACHE_STATIC))
    return
  }

  // 导航请求（HTML 页面）：Network-First，离线回退缓存
  if (request.mode === 'navigate') {
    event.respondWith(networkFirst(request, CACHE_SHELL))
    return
  }
})

/**
 * Cache-First：先查缓存，命中即返回；未命中则请求并缓存。
 * 适用于带 hash 文件名的不可变静态资源。
 */
async function cacheFirst(request, cacheName) {
  const cache = await caches.open(cacheName)
  const cached = await cache.match(request)
  if (cached) return cached

  try {
    const response = await fetch(request)
    // 只缓存成功响应
    if (response.ok) {
      cache.put(request, response.clone())
    }
    return response
  } catch (err) {
    // 离线且无缓存：返回离线兜底（仅对导航请求有意义）
    return new Response('', { status: 503, statusText: 'offline' })
  }
}

/**
 * Network-First：优先请求网络，失败则回退缓存。
 * 适用于 HTML 导航请求（保证最新内容，离线兜底）。
 */
async function networkFirst(request, cacheName) {
  try {
    const response = await fetch(request)
    const cache = await caches.open(cacheName)
    cache.put(request, response.clone())
    return response
  } catch (err) {
    const cache = await caches.open(cacheName)
    const cached = await cache.match(request)
    if (cached) return cached
    // 离线且无缓存：返回 index.html 兜底
    const fallback = await cache.match('/index.html')
    if (fallback) return fallback
    return new Response('离线状态，请联网后重试', {
      status: 503,
      headers: { 'Content-Type': 'text/plain; charset=utf-8' }
    })
  }
}

/**
 * Stale-While-Revalidate：先返回缓存（秒开），后台更新缓存。
 * 适用于 API GET 请求（离线兜底 + 实时更新兼顾）。
 */
async function staleWhileRevalidate(request, cacheName) {
  const cache = await caches.open(cacheName)
  const cached = await cache.match(request)

  // 后台更新（不阻塞响应）
  const fetchPromise = fetch(request)
    .then((response) => {
      if (response.ok) {
        cache.put(request, response.clone())
      }
      return response
    })
    .catch(() => null)

  // 有缓存直接返回，否则等网络
  if (cached) {
    // 触发后台更新（不 await，避免阻塞）
    fetchPromise
    return cached
  }

  const response = await fetchPromise
  if (response) return response

  // 完全离线且无缓存
  return new Response(JSON.stringify({ error: 'offline', offline: true }), {
    status: 503,
    headers: { 'Content-Type': 'application/json' }
  })
}
