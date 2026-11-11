<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { api } from '@/api'
import { useAppStore } from '@/stores/app'
import { isOffline } from '@/pwa'
import { applyTheme, watchSystemTheme } from '@/theme'
import { authState, ensureSession } from '@/auth'
import TipToast from '@/components/TipToast.vue'
import PrivacyPanel from '@/components/PrivacyPanel.vue'

const app = useAppStore()
const router = useRouter()
const route = useRoute()

const booting = ref(true)
const agreed = ref(false)
const refused = ref(false)

// 已登录后才拉业务数据（未登录时这些接口会 401，没必要打）
async function loadAuthedData() {
  try {
    await app.loadStatus()
  } catch {
    /* 状态接口失败不阻塞界面 */
  }
  try {
    const p = await api.privacy()
    agreed.value = p.agreed
  } catch {
    // 取不到隐私设置时不拦人，直接放行
    agreed.value = true
  }
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
  // 先确认登录态：未登录时不打业务接口，直接交给路由守卫跳到登录页
  const authed = await ensureSession()
  if (authed) await loadAuthedData()
  booting.value = false
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
  <div v-if="booting" class="boot-loading">
    <span>加载中…</span>
  </div>

  <!-- 隐私政策：只在已登录、且尚未同意时拦一层，避免压住登录页 -->
  <PrivacyPanel v-else-if="authState.authed && !agreed && !refused" @agree="agree" @disagree="disagree" />

  <div v-else-if="refused" class="boot-loading">
    <p>你需要同意隐私政策后才能使用本应用</p>
    <el-button type="primary" @click="refused = false">重新查看</el-button>
  </div>

  <template v-else>
    <router-view />
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
