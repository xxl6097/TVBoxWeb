<script setup lang="ts">
import { useRouter } from 'vue-router'
import type { Vod } from '@/types'
import VodCard from './VodCard.vue'

const props = defineProps<{
  vods: Vod[]
  loading?: boolean
  skeletonCount?: number
  showProgress?: boolean
  progressOf?: (v: Vod) => number
}>()

const router = useRouter()

function open(v: Vod) {
  router.push({
    name: 'detail',
    query: { site: v.siteKey, siteName: v.siteName, id: v.id }
  })
}
</script>

<template>
  <!-- 加载中：骨架屏 -->
  <div v-if="loading" class="vod-grid">
    <div v-for="n in (skeletonCount ?? 20)" :key="n" class="skeleton-card">
      <div class="skeleton-poster"></div>
      <div class="skeleton-name"></div>
    </div>
  </div>
  <!-- 有内容 -->
  <div v-else-if="vods.length" class="vod-grid">
    <VodCard
      v-for="v in vods"
      :key="v.siteKey + v.id"
      :vod="v"
      :show-progress="showProgress ? progressOf?.(v) : undefined"
      @click="open"
    />
  </div>
  <div v-else class="empty">暂无内容</div>
</template>

<style scoped>
.vod-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 16px;
  padding: 0 24px;
}
.empty {
  color: var(--dim);
  text-align: center;
  padding: 60px 0;
}
/* 骨架屏 */
@keyframes shimmer {
  0% { background-position: -200% 0; }
  100% { background-position: 200% 0; }
}
.skeleton-card {
  cursor: default;
}
.skeleton-poster {
  aspect-ratio: 2 / 3;
  border-radius: var(--radius);
  background: linear-gradient(90deg, var(--card) 25%, var(--card-hi) 50%, var(--card) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.4s infinite;
}
.skeleton-name {
  margin-top: 8px;
  height: 14px;
  border-radius: 4px;
  width: 70%;
  background: linear-gradient(90deg, var(--card) 25%, var(--card-hi) 50%, var(--card) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.4s infinite 0.2s;
}

@media (max-width: 640px) {
  .vod-grid {
    grid-template-columns: repeat(auto-fill, minmax(110px, 1fr));
    gap: 10px;
    padding: 0 12px;
  }
}
</style>
