<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { api } from '@/api'
import { useAppStore } from '@/stores/app'
import { isOffline } from '@/pwa'
import { applyTheme, watchSystemTheme } from '@/theme'
import { authState, ensureSession } from '@/auth'
import { routeLoading } from '@/router'
import TipToast from '@/components/TipToast.vue'
import PrivacyPanel from '@/components/PrivacyPanel.vue'

const app = useAppStore()
const router = useRouter()
const route = useRoute()

const booting = ref(true)
const agreed = ref(false)
const refused = ref(false)

// 已登录后才拉业务数据（未登录时这些接口会 401，没必要打）
// 两个请求并行：原来串行等待，状态接口慢一点就会连带拖住隐私判断，首屏白等。
async function loadAuthedData() {
  const statusP = app.loadStatus().catch(() => {})
  const privacyP = api
    .privacy()
    .then((p) => {
      agreed.value = p.agreed
    })
    .catch(() => {
      // 取不到隐私设置时不拦人，直接放行
      agreed.value = true
    })
  await Promise.all([statusP, privacyP])
}

// 登录成功后再补一次数据加载（onMounted 那次因为未登录跳过了）
watch(
  () => authState.authed,
  async (authed) => {
    if (authed) await loadAuthedData()
  }
)

// ===== 移动端左滑返回手势 =====
let edgeStartX = 0
let edgeStartY = 0
let edgeTracking = false

function onTouchStart(e: TouchEvent) {
  if (window.innerWidth > 640) return
  const t = e.touches[0]
  // 只从屏幕左边缘（<28px）开始追踪
  if (t.clientX < 28) {
    edgeTracking = true
    edgeStartX = t.clientX
    edgeStartY = t.clientY
  }
}

function onTouchMove(e: TouchEvent) {
  if (!edgeTracking) return
  const t = e.touches[0]
  const dx = t.clientX - edgeStartX
  const dy = t.clientY - edgeStartY
  // 明显横向且向右滑动时，阻止页面横向滚动
  if (dx > 0 && Math.abs(dx) > Math.abs(dy)) {
    e.preventDefault()
  }
}

function onTouchEnd(e: TouchEvent) {
  if (!edgeTracking) return
  edgeTracking = false
  const t = e.changedTouches[0]
  const dx = t.clientX - edgeStartX
  const dy = t.clientY - edgeStartY
  // 右滑超过 70px 且横向为主 → 返回（播放页有自己的手势系统，跳过）
  if (dx > 70 && Math.abs(dx) > Math.abs(dy) * 1.5 && route.name !== 'play') {
    router.back()
  }
}

onMounted(async () => {
  applyTheme()
  watchSystemTheme()

  // 启动态最多等这么久：网络慢/接口挂住时先让界面出来，
  // 后续数据到位再交给响应式更新，不要用「加载中…」把人挡在外面。
  const BOOT_TIMEOUT = 3500
  const bootTask = (async () => {
    // 先确认登录态：未登录时不打业务接口，直接交给路由守卫跳到登录页
    const authed = await ensureSession()
    if (authed) await loadAuthedData()
  })()

  await Promise.race([
    bootTask.catch(() => {}),
    new Promise((r) => window.setTimeout(r, BOOT_TIMEOUT))
  ])
  booting.value = false
  // 超时后任务仍在跑，跑完若拿到数据也不会再挡界面
  bootTask.catch(() => {})

  // 全局左滑返回手势
  window.addEventListener('touchstart', onTouchStart, { passive: true })
  window.addEventListener('touchmove', onTouchMove, { passive: false })
  window.addEventListener('touchend', onTouchEnd, { passive: true })
})

onUnmounted(() => {
  window.removeEventListener('touchstart', onTouchStart)
  window.removeEventListener('touchmove', onTouchMove)
  window.removeEventListener('touchend', onTouchEnd)
})

async function agree() {
  try {
    await api.setPrivacy(true, false)
  } catch {
    /* ignore */
  }
  agreed.value = true
  await app.loadStatus()
}

function disagree() {
  refused.value = true
}
</script>

<template>
  <!-- 启动态：与 index.html 的内联骨架保持同一套视觉，避免「骨架 → 文字 → 内容」的跳变 -->
  <div v-if="booting" class="boot">
    <div class="boot-mark" aria-hidden="true">
      <svg viewBox="0 0 48 48" width="34" height="34" fill="none">
        <rect x="5" y="9" width="30" height="21" rx="4" stroke="currentColor" stroke-width="2.2" />
        <path d="M38 17l5.5-3.5v21L38 31" stroke="currentColor" stroke-width="2.2"
              stroke-linecap="round" stroke-linejoin="round" />
        <path d="M13 38h14" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" />
      </svg>
    </div>
    <div class="boot-bar"><i></i></div>
  </div>

  <!-- 隐私政策：只在已登录、且尚未同意时拦一层，避免压住登录页 -->
  <PrivacyPanel v-else-if="authState.authed && !agreed && !refused" @agree="agree" @disagree="disagree" />

  <div v-else-if="refused" class="boot-loading">
    <p>你需要同意隐私政策后才能使用本应用</p>
    <el-button type="primary" @click="refused = false">重新查看</el-button>
  </div>

  <template v-else>
    <!-- 路由 chunk 首次下载期间显示细进度条；延迟 180ms 才出现，
         已缓存的页面切换不会闪一下，只有真慢时才提示。 -->
    <transition name="route-fade">
      <div v-if="routeLoading" class="route-bar"><i></i></div>
    </transition>

    <router-view v-slot="{ Component }">
      <component :is="Component" />
    </router-view>

    <TipToast />
    <!-- 离线提示横幅 -->
    <transition name="offline-fade">
      <div v-if="isOffline" class="offline-banner">
        <span class="dot"></span>
        当前网络已断开，正在展示缓存内容
      </div>
    </transition>
  </template>
</template>

<style scoped>
.boot-loading {
  position: fixed;
  inset: 0;
  background: var(--bg);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 16px;
  color: var(--sub);
}

/* 启动骨架（与 index.html 内联版本同构，颜色改用主题令牌） */
.boot {
  position: fixed;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 20px;
  background: var(--bg);
  color: var(--selected);
}
.boot-mark {
  width: 56px;
  height: 56px;
  display: grid;
  place-items: center;
  border-radius: 16px;
  border: 1px solid var(--border);
  background: var(--card);
  animation: boot-in 0.5s ease-out both;
}
.boot-bar {
  width: 92px;
  height: 2px;
  border-radius: 2px;
  background: var(--white-15);
  overflow: hidden;
}
.boot-bar i {
  display: block;
  width: 40%;
  height: 100%;
  border-radius: 2px;
  background: currentColor;
  animation: boot-slide 1.1s ease-in-out infinite;
}
@keyframes boot-in {
  from { opacity: 0; transform: scale(0.94); }
  to { opacity: 1; transform: none; }
}
@keyframes boot-slide {
  0% { transform: translateX(-100%); }
  100% { transform: translateX(250%); }
}
@media (prefers-reduced-motion: reduce) {
  .boot-mark, .boot-bar i { animation: none; }
  .boot-bar i { width: 100%; }
}

/* 路由切换进度条：顶部一条 2px 细线，不遮挡内容、不触发重排 */
.route-bar {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  height: 2px;
  z-index: 9000;
  pointer-events: none;
  background: var(--white-15);
  overflow: hidden;
}
.route-bar i {
  display: block;
  height: 100%;
  width: 35%;
  background: var(--selected);
  box-shadow: 0 0 8px var(--selected);
  animation: route-slide 0.9s ease-in-out infinite;
}
@keyframes route-slide {
  0% { transform: translateX(-100%); }
  100% { transform: translateX(320%); }
}
.route-fade-enter-active, .route-fade-leave-active { transition: opacity 0.18s; }
.route-fade-enter-from, .route-fade-leave-to { opacity: 0; }
@media (prefers-reduced-motion: reduce) {
  .route-bar i { animation: none; width: 100%; }
}

/* 离线提示横幅 */
.offline-banner {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  z-index: 9999;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 8px 16px;
  font-size: 13px;
  color: #fff;
  background: rgba(200, 60, 35, 0.92);
  backdrop-filter: blur(8px);
  text-align: center;
}
.offline-banner .dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #fff;
  animation: offline-blink 1.2s ease-in-out infinite;
}
@keyframes offline-blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.3; }
}
.offline-fade-enter-active,
.offline-fade-leave-active {
  transition: opacity 0.3s ease, transform 0.3s ease;
}
.offline-fade-enter-from,
.offline-fade-leave-to {
  opacity: 0;
  transform: translateY(-100%);
}
</style>
