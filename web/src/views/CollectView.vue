<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/api'
import { useAppStore } from '@/stores/app'
import type { FavItem } from '@/types'
import TopBar from '@/components/TopBar.vue'

const router = useRouter()
const app = useAppStore()
const favs = ref<FavItem[]>([])

onMounted(load)

async function load() {
  favs.value = await api.favorites()
}

function open(f: FavItem) {
  router.push({ name: 'detail', query: { site: f.siteKey, siteName: f.siteName, id: f.vodId } })
}

function remove(f: FavItem) {
  api.toggleFavorite(f).then(() => {
    app.showTip('已取消收藏')
    load()
  })
}
</script>

<template>
  <div class="page">
    <TopBar />
    <div class="section-title">我的收藏 <span class="count">{{ favs.length }} 部</span></div>
    <div v-if="favs.length" class="grid">
      <div v-for="f in favs" :key="f.siteKey + f.vodId" class="item">
        <div class="poster" @click="open(f)">
          <img :src="f.pic" :alt="f.name" loading="lazy" />
          <div class="del" @click.stop="remove(f)">✕</div>
        </div>
        <div class="name" @click="open(f)">{{ f.name }}</div>
      </div>
    </div>
    <div v-else class="empty">还没有收藏内容</div>
  </div>
</template>

<style scoped>
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 16px;
  padding: 0 24px;
}
.poster {
  position: relative;
  aspect-ratio: 2 / 3;
  border-radius: var(--radius);
  overflow: hidden;
  background: var(--card);
  cursor: pointer;
  border: 2px solid transparent;
}
.item:hover .poster {
  border-color: #fff;
}
.poster img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.del {
  position: absolute;
  right: 6px;
  top: 6px;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 12px;
}
.name {
  margin-top: 8px;
  font-size: 14px;
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.empty {
  text-align: center;
  padding: 60px 24px;
  color: var(--dim);
}

/* ===== 移动端响应式 ===== */
@media (max-width: 640px) {
  .top { padding: 8px 16px; }
  .grid { grid-template-columns: repeat(2, 1fr); gap: 10px; padding: 0 12px; }
  /* 移动端删除按钮始终可见 */
  .del-btn { opacity: 1; }
}
</style>
