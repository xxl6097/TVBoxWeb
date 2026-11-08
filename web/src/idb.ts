/**
 * IndexedDB 离线数据层
 * 
 * 用途：
 * 1. 大容量 KV 存储（localStorage 仅 5MB，IndexedDB 可到数百 MB）
 * 2. API GET 响应离线缓存（SW 缓存之外的第二层兜底）
 * 3. 播放进度、搜索历史等本地数据持久化
 * 
 * 特性：
 * - Promise 化 API（async/await）
 * - 带 TTL 的缓存项
 * - 自动清理过期数据
 * - 类型安全
 */

interface CacheItem<T> {
  value: T
  expireAt: number // 过期时间戳（毫秒），0 表示永不过期
}

const DB_NAME = 'yezhen-film'
const DB_VERSION = 1
const STORE = 'kv'

let dbPromise: Promise<IDBDatabase> | null = null

/**
 * 打开（或创建）数据库
 */
function openDB(): Promise<IDBDatabase> {
  if (dbPromise) return dbPromise

  dbPromise = new Promise((resolve, reject) => {
    // IndexedDB 在部分隐私模式下不可用，优雅降级
    if (typeof indexedDB === 'undefined') {
      reject(new Error('IndexedDB not supported'))
      return
    }

    const request = indexedDB.open(DB_NAME, DB_VERSION)

    request.onupgradeneeded = () => {
      const db = request.result
      if (!db.objectStoreNames.contains(STORE)) {
        // keyPath 用 key，value 存 JSON
        db.createObjectStore(STORE)
      }
    }

    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error)
  })

  // 失败后重置，允许重试
  dbPromise.catch(() => {
    dbPromise = null
  })

  return dbPromise
}

/**
 * 通用请求封装
 */
function request<T>(mode: IDBTransactionMode, action: (store: IDBObjectStore) => IDBRequest): Promise<T> {
  return openDB().then(
    (db) =>
      new Promise<T>((resolve, reject) => {
        const tx = db.transaction(STORE, mode)
        const store = tx.objectStore(STORE)
        const req = action(store)
        req.onsuccess = () => resolve(req.result as T)
        req.onerror = () => reject(req.error)
      })
  )
}

/**
 * 写入缓存（带 TTL）
 */
export async function idbSet<T>(key: string, value: T, ttlMs = 0): Promise<void> {
  const item: CacheItem<T> = {
    value,
    expireAt: ttlMs > 0 ? Date.now() + ttlMs : 0
  }
  try {
    await request<IDBValidKey>('readwrite', (store) => store.put(item, key))
  } catch {
    // IndexedDB 不可用时静默失败（不阻塞主流程）
  }
}

/**
 * 读取缓存（自动清理过期项）
 */
export async function idbGet<T>(key: string): Promise<T | null> {
  try {
    const item = await request<CacheItem<T> | undefined>('readonly', (store) => store.get(key))
    if (!item) return null

    // 检查是否过期
    if (item.expireAt > 0 && item.expireAt < Date.now()) {
      // 异步删除过期项（不阻塞返回）
      idbDelete(key)
      return null
    }

    return item.value
  } catch {
    return null
  }
}

/**
 * 删除缓存
 */
export async function idbDelete(key: string): Promise<void> {
  try {
    await request<undefined>('readwrite', (store) => store.delete(key))
  } catch {
    /* ignore */
  }
}

/**
 * 清空所有缓存
 */
export async function idbClear(): Promise<void> {
  try {
    await request<undefined>('readwrite', (store) => store.clear())
  } catch {
    /* ignore */
  }
}

/**
 * 获取所有键
 */
export async function idbKeys(): Promise<string[]> {
  try {
    return await request<string[]>('readonly', (store) => store.getAllKeys())
  } catch {
    return []
  }
}

/**
 * 清理过期项（后台调用）
 */
export async function idbCleanup(): Promise<void> {
  try {
    const keys = await idbKeys()
    const now = Date.now()
    await Promise.all(
      keys.map(async (key) => {
        const item = await idbGet<CacheItem<unknown>>(key)
        if (item && (item as any).expireAt > 0 && (item as any).expireAt < now) {
          await idbDelete(key)
        }
      })
    )
  } catch {
    /* ignore */
  }
}

/**
 * 估算存储占用（字节）
 */
export async function idbSize(): Promise<number> {
  try {
    if (navigator.storage?.estimate) {
      const estimate = await navigator.storage.estimate()
      return estimate.usage || 0
    }
  } catch {
    /* ignore */
  }
  return 0
}

/**
 * 持久化存储授权（防止浏览器自动清理）
 */
export async function idbPersist(): Promise<boolean> {
  try {
    if (navigator.storage?.persist) {
      return await navigator.storage.persist()
    }
  } catch {
    /* ignore */
  }
  return false
}

export default {
  set: idbSet,
  get: idbGet,
  delete: idbDelete,
  clear: idbClear,
  keys: idbKeys,
  cleanup: idbCleanup,
  size: idbSize,
  persist: idbPersist
}
