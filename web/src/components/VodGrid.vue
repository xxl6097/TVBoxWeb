<script setup lang="ts">
import { useRouter } from 'vue-router'
import type { Vod } from '@/types'
import VodCard from './VodCard.vue'

const props = defineProps<{
  vods: Vod[]
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
  <div v-if="vods.length" class="vod-grid">
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
</style>
