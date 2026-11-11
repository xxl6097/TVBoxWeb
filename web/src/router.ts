import { createRouter, createWebHashHistory } from 'vue-router'
import { ensureSession } from '@/auth'

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
