import { createRouter, createWebHashHistory } from 'vue-router'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
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

export default router
