import { createRouter, createWebHashHistory } from 'vue-router'
import { ref } from 'vue'
import { ensureSession } from '@/auth'

/**
 * 路由切换中的加载提示。
 * 只对「首次加载该 chunk」有意义：已缓存的页面 beforeResolve 几乎瞬时完成，
 * 所以用 180ms 延迟避免闪一下；真慢（弱网/首次进大页面）才显示细进度条。
 */
export const routeLoading = ref(false)
let loadingTimer = 0
let hideTimer = 0

function beginLoad() {
  if (hideTimer) {
    clearTimeout(hideTimer)
    hideTimer = 0
  }
  if (loadingTimer) return
  loadingTimer = window.setTimeout(() => {
    loadingTimer = 0
    routeLoading.value = true
  }, 180)
}

function endLoad() {
  if (loadingTimer) {
    clearTimeout(loadingTimer)
    loadingTimer = 0
  }
  // 已经显示出来的话，稍作停留再撤，避免进度条一闪而过
  hideTimer = window.setTimeout(() => {
    routeLoading.value = false
    hideTimer = 0
  }, 160)
}

const router = createRouter({
  history: createWebHashHistory(),
  scrollBehavior(to, from, savedPosition) {
    if (savedPosition) return savedPosition
    if (to.hash) return { el: to.hash, behavior: 'smooth' }
    // 换页回顶部；同页只换 query（筛选/翻页）时保持当前滚动位置
    if (to.path === from.path) return undefined
    return { top: 0 }
  },
  routes: [
    { path: '/login', name: 'login', component: () => import('@/views/LoginView.vue') },
    { path: '/', name: 'home', component: () => import('@/views/HomeView.vue') },
    { path: '/vod', name: 'vod', component: () => import('@/views/VodView.vue') },
    { path: '/detail', name: 'detail', component: () => import('@/views/DetailView.vue') },
    { path: '/search', name: 'search', component: () => import('@/views/SearchView.vue') },
    { path: '/play', name: 'play', component: () => import('@/views/PlayerView.vue') },
    { path: '/live', name: 'live', component: () => import('@/views/LiveView.vue') },
    { path: '/collect', name: 'collect', component: () => import('@/views/CollectView.vue') },
    { path: '/history', name: 'history', component: () => import('@/views/HistoryView.vue') },
    { path: '/settings', name: 'settings', component: () => import('@/views/SettingsView.vue') },
    { path: '/webdav', name: 'webdav', component: () => import('@/views/WebdavView.vue') },
    { path: '/direct', name: 'direct', component: () => import('@/views/DirectView.vue') },
    { path: '/metrics', name: 'metrics', component: () => import('@/views/MetricsView.vue') }
  ]
})

/**
 * 登录守卫：除登录页外，所有页面都要先确认会话。
 * ensureSession() 内部有一次性的结果缓存，正常翻页不会每次都打接口。
 */
router.beforeResolve(() => beginLoad())
router.afterEach(() => endLoad())
router.onError(() => endLoad())

router.beforeEach(async (to) => {
  if (to.name === 'login') {
    // 已登录还去登录页 → 回首页，避免重复登录
    const authed = await ensureSession()
    return authed ? { name: 'home' } : true
  }
  const authed = await ensureSession()
  if (authed) return true
  return {
    name: 'login',
    query: to.fullPath && to.fullPath !== '/' ? { redirect: to.fullPath } : {}
  }
})

export default router
