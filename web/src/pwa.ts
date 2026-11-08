/**
 * PWA 注册与离线状态管理
 * 
 * 功能：
 * 1. 注册 Service Worker（应用壳离线缓存）
 * 2. 离线状态检测（online/offline 事件 + 响应式状态）
 * 3. IndexedDB 后台清理
 */

import { ref } from 'vue'
import { idbCleanup, idbPersist } from './idb'

// ===== 响应式离线状态 =====
export const isOffline = ref(!navigator.onLine)

// ===== SW 注册状态 =====
export const swSupported = ref('serviceWorker' in navigator)
export const swRegistered = ref(false)

let swRegistration: ServiceWorkerRegistration | null = null
let listenersAttached = false

/**
 * 注册 Service Worker
 * 只在生产环境（构建后）注册，开发环境跳过（避免缓存干扰热更新）
 */
export async function registerSW(): Promise<void> {
  if (!swSupported.value) {
    console.warn('[PWA] Service Worker 不支持')
    return
  }

  // 开发环境（vite dev server）跳过，避免缓存干扰
  if (import.meta.env.DEV) {
    console.info('[PWA] 开发环境跳过 SW 注册')
    return
  }

  try {
    swRegistration = await navigator.serviceWorker.register('/sw.js')
    swRegistered.value = true
    console.info('[PWA] Service Worker 注册成功', swRegistration.scope)

    // 监听 SW 更新
    swRegistration.addEventListener('updatefound', () => {
      const newWorker = swRegistration!.installing
      if (!newWorker) return
      newWorker.addEventListener('statechange', () => {
        if (newWorker.state === 'installed' && navigator.serviceWorker.controller) {
          console.info('[PWA] 新版本已就绪，刷新页面即可生效')
        }
      })
    })
  } catch (err) {
    console.error('[PWA] Service Worker 注册失败:', err)
    swRegistered.value = false
  }
}

/**
 * 附加离线状态监听
 */
function attachOfflineListeners(): void {
  if (listenersAttached) return
  listenersAttached = true

  window.addEventListener('online', () => {
    isOffline.value = false
    console.info('[PWA] 网络已恢复')
  })

  window.addEventListener('offline', () => {
    isOffline.value = true
    console.warn('[PWA] 网络已断开')
  })
}

/**
 * 初始化 PWA（注册 SW + 离线监听 + IndexedDB 清理）
 * 应在应用启动时调用
 */
export async function initPWA(): Promise<void> {
  attachOfflineListeners()

  // 请求持久化存储（防止浏览器自动清理 IndexedDB）
  idbPersist().then((granted) => {
    if (granted) console.info('[PWA] 持久化存储已授权')
  })

  // 后台清理过期缓存
  idbCleanup().catch(() => {})

  // 注册 Service Worker
  await registerSW()
}

/**
 * 检查是否有待更新的 SW（手动触发更新）
 */
export async function checkUpdate(): Promise<boolean> {
  if (!swRegistration) return false
  try {
    await swRegistration.update()
    return true
  } catch {
    return false
  }
}

/**
 * 手动刷新以应用新版本
 */
export function applyUpdate(): void {
  // 跳过等待，让新 SW 立即生效
  if (swRegistration?.waiting) {
    swRegistration.waiting.postMessage({ type: 'SKIP_WAITING' })
  }
  window.location.reload()
}

export default {
  isOffline,
  swSupported,
  swRegistered,
  registerSW,
  initPWA,
  checkUpdate,
  applyUpdate
}
