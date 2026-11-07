<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api } from '@/api'
import { useAppStore } from '@/stores/app'
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
</style>
