import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import App from './App.vue'
import router from './router'
import './styles/main.css'
import { initWS } from './ws'
import { initPWA } from './pwa'
import { setUnauthorizedHandler } from './api'
import { markUnauthorized } from './auth'

const app = createApp(App)

app.config.errorHandler = (err, _instance, info) => {
  console.error('[APP ERROR]', info, err, (err as Error)?.stack)
}

for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

// 全局兜底：任意图片加载失败时隐藏，露出父容器的占位底色（避免黑块）
document.addEventListener(
  'error',
  (e) => {
    const el = e.target as HTMLElement
    if (el && el.tagName === 'IMG') {
      el.style.display = 'none'
    }
  },
  true
)

app.use(createPinia())
app.use(router)
app.use(ElementPlus)

// 会话失效（任意 API 返回 401）时统一跳登录页，并带上当前路径方便登录后回跳
setUnauthorizedHandler(() => {
  markUnauthorized()
  const cur = router.currentRoute.value
  if (cur.name === 'login') return
  router.replace({ name: 'login', query: cur.fullPath !== '/' ? { redirect: cur.fullPath } : {} })
})

app.mount('#app')

// 初始化 WebSocket 连接（非阻塞）
initWS().catch(console.error)

// 初始化 PWA（注册 Service Worker + 离线状态检测，非阻塞）
initPWA().catch(console.error)
