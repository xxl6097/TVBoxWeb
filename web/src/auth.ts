// 登录态：与后端 /api/session 对齐的前端状态机。
//
// 之所以单独抽一个模块（而不是放进 Pinia store）：
// 路由守卫在应用启动早期就要判断登录态，这时 Pinia 可能还没装好；
// 一个模块级 reactive 对象在任何时机都能读。
import { reactive } from 'vue'

export interface AuthState {
  /** 服务端是否启用了账号密码登录 */
  needLogin: boolean
  /** 当前是否已通过登录 */
  authed: boolean
  /** 已登录用户名 */
  user: string
  /** 是否已经问过服务端（避免每次跳转都打一次） */
  checked: boolean
}

export const authState = reactive<AuthState>({
  needLogin: false,
  authed: true,
  user: '',
  checked: false
})

/** 清空缓存，避免登出后还从 IndexedDB 里读到上一个会话的数据 */
async function clearCaches(): Promise<void> {
  const mod = await import('@/api')
  mod.invalidateApiCache()
}

async function callSession(): Promise<AuthState> {
  const res = await fetch('/api/session', { credentials: 'same-origin' })
  if (!res.ok) throw new Error('session ' + res.status)
  const j = (await res.json()) as { needLogin: boolean; authed: boolean; user: string }
  authState.needLogin = !!j.needLogin
  authState.authed = !!j.authed
  authState.user = j.user || ''
  authState.checked = true
  return authState
}

/**
 * 确保登录态已知。失败（网络异常）时保守放行：
 * 宁可让用户看到「接口 401」的真实报错，也不要因为一次网络抖动把人锁在登录页出不去。
 */
export async function ensureSession(force = false): Promise<boolean> {
  if (authState.checked && !force) return authState.authed
  try {
    const st = await callSession()
    return st.authed
  } catch {
    authState.checked = true
    return true
  }
}

/**
 * 媒体令牌：给 <video>/MSE 这类「带不了 Cookie」的请求用。
 *
 * 会话 Cookie 在视频子资源请求上不会发送（没 crossorigin 就不走 CORS 凭证，
 * 加了又要求 SameSite=None; Secure，HTTP 部署设不了 Secure）。
 * 所以登录后额外拿一个令牌，拼进视频 URL 的 token 参数。
 */
const MEDIA_TOKEN_KEY = 'yz_media_token'

export function getMediaToken(): string {
  try {
    return sessionStorage.getItem(MEDIA_TOKEN_KEY) || ''
  } catch {
    return ''
  }
}

function setMediaToken(t: string): void {
  try {
    if (t) sessionStorage.setItem(MEDIA_TOKEN_KEY, t)
    else sessionStorage.removeItem(MEDIA_TOKEN_KEY)
  } catch {
    /* 隐私模式忽略 */
  }
}

/** 登录；失败时抛出带中文提示的 Error。 */
export async function login(user: string, pass: string): Promise<void> {
  const res = await fetch('/api/login', {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user, pass })
  })
  if (!res.ok) {
    let msg = '登录失败'
    try {
      const j = await res.json()
      if (j?.error) msg = j.error
    } catch {
      /* 非 JSON 响应，用默认文案 */
    }
    throw new Error(msg)
  }
  let mediaToken = ''
  try {
    const j = await res.json()
    mediaToken = j?.mediaToken || ''
  } catch {
    /* 老版本后端没有这个字段，忽略 */
  }
  setMediaToken(mediaToken)
  await clearCaches()
  await ensureSession(true)
}

/** 退出登录。 */
export async function logout(): Promise<void> {
  try {
    await fetch('/api/logout', { method: 'POST', credentials: 'same-origin' })
  } catch {
    /* 网络异常也继续清本地状态 */
  }
  authState.authed = !authState.needLogin
  authState.user = ''
  authState.checked = true
  setMediaToken('')
  await clearCaches()
}

/** 后端返回 401 时调用：标记为未登录并强制重新校验。 */
export function markUnauthorized(): void {
  if (!authState.needLogin) return
  authState.authed = false
  authState.user = ''
}
