<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api } from '@/api'
import { useAppStore } from '@/stores/app'
import { isOffline } from '@/pwa'
import TipToast from '@/components/TipToast.vue'
import PrivacyPanel from '@/components/PrivacyPanel.vue'

const app = useAppStore()

const checking = ref(true)
const agreed = ref(false)
const refused = ref(false)

onMounted(async () => {
  await app.loadStatus()
  try {
    const p = await api.privacy()
    agreed.value = p.agreed
  } catch {
    agreed.value = true
  }
  checking.value = false
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
  <div v-if="checking" class="boot-loading">
    <span>加载中…</span>
  </div>

  <PrivacyPanel v-else-if="!agreed && !refused" @agree="agree" @disagree="disagree" />

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
