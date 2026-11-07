<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/api'
import { useAppStore } from '@/stores/app'
import { usePlayerStore } from '@/stores/player'
import TopBar from '@/components/TopBar.vue'

const router = useRouter()
const app = useAppStore()
const player = usePlayerStore()

const url = ref('')
const list = ref<string[]>([])

onMounted(async () => {
  list.value = await api.directList()
})

function play(u?: string) {
  const target = (u ?? url.value).trim()
  if (!target) {
    app.showTip('先填入视频地址')
    return
  }
  if (!/^https?:\/\//.test(target)) {
    app.showTip('只支持 http(s) 地址')
    return
  }
  player.set([{ name: '直连播放', url: target, urls: [target] }], 0, '直连播放', false, true)
  api.pushDirect(target)
  router.push({ name: 'play', query: { url: target, title: '直连播放', site: 'direct', id: '', from: 'direct', direct: '1' } })
}
</script>

<template>
  <div class="page">
    <TopBar />
    <div class="direct">
      <div class="card">
        <h2 class="card-title">直连播放</h2>
        <div class="input-row">
          <el-input v-model="url" size="large" placeholder="粘贴 http(s) 的 m3u8 / mp4 地址" @keyup.enter="play()" />
          <el-button type="primary" size="large" @click="play()">播放</el-button>
        </div>
        <div v-if="list.length" class="recent">
          <div class="recent-title">最近播放</div>
          <div v-for="u in list" :key="u" class="recent-item" @click="play(u)">{{ u }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.direct {
  max-width: 760px;
  padding: 8px 24px 40px;
}
.card {
  background: var(--surface);
  border-radius: 12px;
  padding: 20px;
}
.card-title {
  margin: 0 0 16px;
  font-size: 17px;
}
.input-row {
  display: flex;
  gap: 8px;
}
.recent {
  margin-top: 20px;
}
.recent-title {
  font-size: 13px;
  color: var(--dim);
  margin-bottom: 8px;
}
.recent-item {
  padding: 8px;
  border-radius: 6px;
  font-size: 13px;
  color: var(--sub);
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.recent-item:hover {
  background: var(--card);
  color: #fff;
}

/* ===== 移动端响应式 ===== */
@media (max-width: 640px) {
  .top { padding: 8px 16px; gap: 8px; }
  .top .el-input { width: 100%; }
  .recents { padding: 0 16px; }
}
</style>
