<script setup lang="ts">
defineProps<{
  count?: number
  type?: 'channel' | 'search' | 'card'
}>()
</script>

<template>
  <!-- 频道列表骨架屏 -->
  <div v-if="type === 'channel'" class="skeleton-channels">
    <div v-for="n in (count ?? 30)" :key="n" class="skeleton-channel">
      <div class="skeleton-text"></div>
    </div>
  </div>
  
  <!-- 搜索结果骨架屏 -->
  <div v-else-if="type === 'search'" class="skeleton-search">
    <div v-for="n in (count ?? 10)" :key="n" class="skeleton-search-item">
      <div class="skeleton-poster"></div>
      <div class="skeleton-info">
        <div class="skeleton-title"></div>
        <div class="skeleton-meta"></div>
        <div class="skeleton-meta short"></div>
      </div>
    </div>
  </div>
  
  <!-- 通用卡片骨架屏 -->
  <div v-else class="skeleton-cards">
    <div v-for="n in (count ?? 20)" :key="n" class="skeleton-card">
      <div class="skeleton-poster"></div>
      <div class="skeleton-name"></div>
    </div>
  </div>
</template>

<style scoped>
/* 统一 shimmer 动画 */
@keyframes shimmer {
  0% { background-position: -200% 0; }
  100% { background-position: 200% 0; }
}

.shimmer-bg {
  background: linear-gradient(90deg, var(--card) 25%, var(--card-hi) 50%, var(--card) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.4s infinite;
}

/* 频道列表骨架屏 */
.skeleton-channels {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 12px;
  padding: 16px 24px;
}

.skeleton-channel {
  padding: 12px 16px;
  border-radius: var(--radius);
  background: var(--card);
}

.skeleton-text {
  height: 20px;
  border-radius: 4px;
  background: linear-gradient(90deg, var(--card) 25%, var(--card-hi) 50%, var(--card) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.4s infinite;
}

/* 搜索结果骨架屏 */
.skeleton-search {
  padding: 16px 24px;
}

.skeleton-search-item {
  display: flex;
  gap: 16px;
  padding: 12px 0;
  border-bottom: 1px solid var(--border);
}

.skeleton-search-item:last-child {
  border-bottom: none;
}

.skeleton-poster {
  flex-shrink: 0;
  width: 100px;
  aspect-ratio: 2 / 3;
  border-radius: var(--radius);
  background: linear-gradient(90deg, var(--card) 25%, var(--card-hi) 50%, var(--card) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.4s infinite;
}

.skeleton-info {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.skeleton-title {
  height: 20px;
  width: 60%;
  border-radius: 4px;
  background: linear-gradient(90deg, var(--card) 25%, var(--card-hi) 50%, var(--card) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.4s infinite;
}

.skeleton-meta {
  height: 14px;
  width: 40%;
  border-radius: 4px;
  background: linear-gradient(90deg, var(--card) 25%, var(--card-hi) 50%, var(--card) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.4s infinite 0.2s;
}

.skeleton-meta.short {
  width: 30%;
  animation-delay: 0.4s;
}

/* 通用卡片骨架屏 */
.skeleton-cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 16px;
  padding: 0 24px;
}

.skeleton-card .skeleton-poster {
  width: 100%;
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
  .skeleton-channels {
    grid-template-columns: repeat(2, 1fr);
    gap: 8px;
    padding: 12px;
  }
  
  .skeleton-search {
    padding: 12px;
  }
  
  .skeleton-search-item {
    gap: 12px;
  }
  
  .skeleton-poster {
    width: 80px;
  }
  
  .skeleton-cards {
    grid-template-columns: repeat(auto-fill, minmax(110px, 1fr));
    gap: 10px;
    padding: 0 12px;
  }
}
</style>
