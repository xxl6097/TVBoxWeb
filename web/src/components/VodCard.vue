<script setup lang="ts">
import { ref, computed } from 'vue'
import type { Vod } from '@/types'

const props = defineProps<{
  vod: Vod
  showProgress?: number // 0~1 观看进度，压在海报底部
  showRemark?: boolean
}>()

const emit = defineEmits<{ (e: 'click', vod: Vod): void }>()

const failed = ref(false)

function onErr() {
  failed.value = true
}

const firstChar = computed(() => (props.vod.name ? props.vod.name.trim()[0] || '影' : '影'))
</script>

<template>
  <div
    class="vod-card"
    role="button"
    tabindex="0"
    :aria-label="`打开 ${vod.name}`"
    @click="emit('click', vod)"
    @keydown.enter="emit('click', vod)"
    @keydown.space.prevent="emit('click', vod)"
  >
    <div class="poster">
      <img v-if="!failed && vod.pic" :src="vod.pic" :alt="vod.name" loading="lazy" @error="onErr" />
      <div v-else class="placeholder">
        <span class="ph-char">{{ firstChar }}</span>
        <span class="ph-name">{{ vod.name }}</span>
      </div>
      <div v-if="vod.remark && showRemark !== false" class="remark">{{ vod.remark }}</div>
      <div v-if="showProgress !== undefined && showProgress > 0" class="progress">
        <div class="progress-bar" :style="{ width: showProgress * 100 + '%' }"></div>
      </div>
    </div>
    <div class="name">{{ vod.name }}</div>
    <div v-if="vod.year" class="meta">{{ vod.year }}</div>
  </div>
</template>

<style scoped>
.vod-card {
  cursor: pointer;
  transition: transform 0.14s ease-out;
}
.vod-card:hover {
  transform: scale(1.06);
}
.poster {
  position: relative;
  aspect-ratio: 2 / 3;
  border-radius: var(--radius);
  overflow: hidden;
  background: var(--card);
  border: 2px solid transparent;
  transition: border-color 0.14s;
}
.vod-card:hover .poster {
  border-color: #fff;
}
.poster img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
  border-radius: var(--radius);
}
.placeholder {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  background: linear-gradient(160deg, #26242b 0%, #1c1b1f 100%);
  padding: 8px;
  text-align: center;
}
.ph-char {
  font-size: 40px;
  font-weight: 700;
  color: rgba(255, 255, 255, 0.18);
  line-height: 1;
}
.ph-name {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.5);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  word-break: break-all;
}
.remark {
  position: absolute;
  right: 0;
  top: 0;
  background: rgba(23, 117, 53, 0.7);
  color: #fff;
  font-size: 12px;
  padding: 4px;
  border-radius: 0 0 0 4px;
}
.progress {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 4px;
  background: rgba(255, 255, 255, 0.2);
}
.progress-bar {
  height: 100%;
  background: var(--live);
}
.name {
  margin-top: 8px;
  font-size: 14px;
  line-height: 1.3;
  color: var(--txt);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.meta {
  margin-top: 2px;
  font-size: 12px;
  color: var(--dim);
}

/* ===== 移动端：触摸替代 hover ===== */
@media (max-width: 640px) {
  .vod-card:active .poster {
    border-color: #fff;
    transform: scale(0.97);
  }
  /* 名称在手机上不限2行 */
  .name { -webkit-line-clamp: 2; font-size: 13px; }
  /* remark 标签在小屏不截断 */
  .remark { font-size: 10px; padding: 1px 5px; }
}
</style>
