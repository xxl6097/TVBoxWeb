<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '@/api'
import { useAppStore } from '@/stores/app'
import { usePlayerStore } from '@/stores/player'
import { splitLines } from '@/utils/lines'
import type { Vod, Line } from '@/types'
import TopBar from '@/components/TopBar.vue'
import SkeletonList from '@/components/SkeletonList.vue'

const route = useRoute()
const router = useRouter()
const app = useAppStore()
const player = usePlayerStore()

const siteKey = route.query.site as string
const siteName = (route.query.siteName as string) || ''
const vodId = route.query.id as string

const vod = ref<Vod | null>(null)
const lines = ref<Line[]>([])
const curLine = ref(0)
const expanded = ref(false)
const isFav = ref(false)
const reverse = ref(false)
const loading = ref(true)

// 上次看到的集（用于定位）
const watchedEp = ref(-1)
const posterFailed = ref(false)

onMounted(async () => {
  try {
    const detail = await api.vodDetail(siteKey, vodId)
    vod.value = detail
    lines.value = splitLines(detail.from, detail.playUrl)
    // 默认线路：优先选第一条「直链」线路（网页地址播不出，真流在后面）
    if (lines.value.length) {
      const dIdx = lines.value.findIndex((l) => l.direct)
      curLine.value = dIdx >= 0 ? dIdx : 0
    }
  } catch (e: any) {
    app.showTip('详情加载失败：' + e.message)
  } finally {
    loading.value = false
  }
  try {
    const favs = await api.favorites()
    isFav.value = favs.some((f) => f.siteKey === siteKey && f.vodId === vodId)
  } catch {
    /* ignore */
  }
  try {
    const hist = await api.history()
    const h = hist.find((x) => x.siteKey === siteKey && x.vodId === vodId)
    if (h) watchedEp.value = h.epIndex
  } catch {
    /* ignore */
  }
})

const eps = computed(() => {
  if (!lines.value.length) return []
  const list = lines.value[curLine.value].eps
  return reverse.value ? [...list].reverse() : list
})

// 倒序时显示下标要换算回真实集下标
function isWatched(displayIdx: number): boolean {
  const line = lines.value[curLine.value]
  if (!line) return false
  const realIdx = reverse.value ? line.eps.length - 1 - displayIdx : displayIdx
  return realIdx === watchedEp.value
}

const meta = computed(() => {
  if (!vod.value) return ''
  const parts = [vod.value.typeName, vod.value.year, vod.value.area, vod.value.remark].filter(Boolean)
  const line = parts.join(' · ')
  const actor = vod.value.actor ? '主演：' + vod.value.actor : ''
  return [line, actor].filter(Boolean).join(' · ') || '暂无资料'
})

function toggleFav() {
  if (!vod.value) return
  const f = {
    siteKey,
    siteName: vod.value.siteName || siteName,
    vodId,
    name: vod.value.name,
    pic: vod.value.pic
  }
  api.toggleFavorite(f).then((r) => {
    isFav.value = r.added
    app.showTip(r.added ? '已加入收藏' : '已取消收藏')
  })
}

function play(epIndex: number) {
  if (!vod.value || !lines.value.length) return
  const line = lines.value[curLine.value]
  const realIndex = reverse.value ? line.eps.length - 1 - epIndex : epIndex
  const ep = line.eps[realIndex]
  if (!ep) return
  // 把整条线路的剧集写入播放列表，播放页才能「上一集/下一集」
  player.set(
    line.eps.map((e) => ({ name: e.name, url: e.url, urls: [...e.urls] })),
    realIndex,
    vod.value.name,
    false,
    false
  )
  router.push({
    name: 'play',
    query: {
      url: ep.url,
      title: `${vod.value.name} ${ep.name}`,
      site: siteKey,
      id: vodId,
      name: vod.value.name,
      pic: vod.value.pic || '',
      siteName: vod.value.siteName || '',
      from: '',
      epIndex: String(realIndex),
      vodTitle: vod.value.name,
      lineIdx: String(curLine.value),
      direct: line.direct ? '1' : '0'
    }
  })
}
</script>

<template>
  <div class="page">
    <TopBar />
    <div class="back" @click="router.back()">‹ 返回</div>

    <!-- 加载中：骨架屏 -->
    <div v-if="loading" class="detail-loading">
      <div class="left">
        <div class="skeleton-poster"></div>
      </div>
      <div class="right">
        <div class="skeleton-title"></div>
        <div class="skeleton-meta"></div>
        <div class="skeleton-meta"></div>
        <div class="skeleton-content"></div>
        <div class="skeleton-content"></div>
        <div class="skeleton-content short"></div>
      </div>
    </div>

    <!-- 详情内容 -->
    <div v-else-if="vod" class="detail">
      <div class="left">
        <div class="poster">
          <img v-if="!posterFailed && vod.pic" :src="vod.pic" :alt="vod.name" @error="posterFailed = true" />
          <div v-else class="ph">
            <span class="ph-char">{{ vod.name ? vod.name[0] : '影' }}</span>
            <span class="ph-name">{{ vod.name }}</span>
          </div>
        </div>
      </div>
      <div class="right">
        <h1 class="title">{{ vod.name }}</h1>
        <div v-if="meta" class="meta">{{ meta }}</div>
        <div v-if="vod.actor" class="meta">主演：{{ vod.actor }}</div>
        <div v-if="vod.director" class="meta">导演：{{ vod.director }}</div>

        <div v-if="vod.content" class="content" :class="{ expanded }">
          {{ vod.content }}
        </div>
        <el-button v-if="vod.content" link class="toggle" @click="expanded = !expanded">
          {{ expanded ? '收起' : '展开' }}
        </el-button>

        <div class="actions">
          <el-button type="primary" @click="toggleFav">{{ isFav ? '已收藏' : '收藏' }}</el-button>
        </div>
      </div>
    </div>

    <!-- 线路 -->
    <div v-if="lines.length" class="lines">
      <div class="lines-head">
        <span class="lines-label">线路</span>
        <div class="lines-tabs">
          <span
            v-for="(l, i) in lines"
            :key="i"
            class="chip"
            :class="{ active: curLine === i }"
            @click="curLine = i"
          >
            {{ l.name }}{{ l.direct ? '' : ' ·解析' }}
          </span>
        </div>
      </div>

      <!-- 剧集 -->
      <div class="eps-head">
        <span class="eps-title">剧集</span>
        <span class="eps-count">共 {{ lines[curLine].eps.length }} 集</span>
        <span v-if="lines[curLine].eps.length > 32" class="eps-hint">· 可上下滚动查看更多</span>
        <span v-if="watchedEp >= 0" class="eps-seen">上次看到第 {{ watchedEp + 1 }} 集</span>
        <span class="eps-order" @click="reverse = !reverse">{{ reverse ? '正序' : '倒序' }}</span>
      </div>
      <div class="eps">
        <span
          v-for="(ep, i) in eps"
          :key="i"
          class="ep-chip"
          :class="{ watched: isWatched(i) }"
          @click="play(i)"
        >
          {{ ep.name }}
        </span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.back {
  padding: 8px 24px;
  color: var(--sub);
  cursor: pointer;
  font-size: 15px;
}
.detail {
  display: flex;
  gap: 24px;
  padding: 8px 24px 24px;
}
.poster {
  width: 220px;
  aspect-ratio: 2 / 3;
  border-radius: var(--radius);
  overflow: hidden;
  background: var(--card);
  flex-shrink: 0;
}
.poster img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.poster .ph {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  background: linear-gradient(160deg, #26242b 0%, #1c1b1f 100%);
  padding: 12px;
  text-align: center;
}
.poster .ph .ph-char {
  font-size: 56px;
  font-weight: 700;
  color: rgba(255, 255, 255, 0.18);
  line-height: 1;
}
.poster .ph .ph-name {
  font-size: 13px;
  color: rgba(255, 255, 255, 0.5);
  word-break: break-all;
}
.right {
  flex: 1;
  min-width: 0;
}
.title {
  margin: 0 0 12px;
  font-size: 26px;
}
.meta {
  color: var(--sub);
  font-size: 14px;
  margin-bottom: 6px;
}
.content {
  color: var(--sub);
  font-size: 14px;
  line-height: 1.6;
  margin-top: 12px;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.content.expanded {
  -webkit-line-clamp: unset;
}
.toggle {
  padding: 0;
  color: var(--selected);
}
.actions {
  margin-top: 16px;
}
.lines {
  padding: 0 24px 24px;
}
.lines-head {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 14px;
}
.lines-label {
  font-size: 16px;
  font-weight: 600;
}
.lines-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  flex: 1;
}
.eps-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}
.eps-title {
  font-size: 16px;
  font-weight: 600;
}
.eps-count {
  font-size: 13px;
  color: var(--dim);
}
.eps-hint {
  font-size: 12px;
  color: var(--dim);
}
.eps-seen {
  font-size: 13px;
  color: var(--selected);
}
.eps-order {
  margin-left: auto;
  font-size: 13px;
  color: var(--sub);
  cursor: pointer;
}
.eps-order:hover {
  color: #fff;
}
.eps {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
  gap: 8px;
  max-height: 220px;
  overflow-y: auto;
}
.ep-chip {
  padding: 8px 12px;
  border-radius: var(--radius-sm);
  background: var(--white-15);
  font-size: 13px;
  text-align: center;
  cursor: pointer;
  transition: all 0.14s;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ep-chip:hover {
  background: var(--card-hi);
}
.ep-chip.watched {
  color: var(--selected);
  border: 1px solid var(--selected);
}

/* ===== 移动端响应式 ===== */
@media (max-width: 640px) {
  .back { padding: 8px 16px; }
  /* 海报+详情从水平双栏 → 垂直单栏 */
  .detail {
    flex-direction: column;
    gap: 16px;
    padding: 8px 16px 16px;
  }
  .poster {
    width: 100%;
    max-width: 200px;
    align-self: center;
  }
  .title { font-size: 20px; }
  /* 剧集网格：手机 3 列 */
  .eps { grid-template-columns: repeat(3, 1fr); }
  /* 操作按钮换行 */
  .lines-head, .eps-head {
    flex-wrap: wrap;
    gap: 8px;
  }
  /* 线路 tab 横滑 */
  .lines-tabs {
    overflow-x: auto;
    white-space: nowrap;
    flex-wrap: nowrap;
    padding-bottom: 4px;
  }
  /* 骨架屏移动端 */
  .detail-loading {
    flex-direction: column;
    padding: 0 12px;
  }
  .detail-loading .left {
    width: 100%;
    max-width: 200px;
    margin: 0 auto;
  }
}

/* 详情页骨架屏 */
@keyframes shimmer {
  0% { background-position: -200% 0; }
  100% { background-position: 200% 0; }
}

.detail-loading {
  display: flex;
  gap: 24px;
  padding: 0 24px;
}

.detail-loading .left {
  width: 240px;
  flex-shrink: 0;
}

.detail-loading .right {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.skeleton-poster {
  aspect-ratio: 2 / 3;
  border-radius: var(--radius);
  background: linear-gradient(90deg, var(--card) 25%, var(--card-hi) 50%, var(--card) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.4s infinite;
}

.skeleton-title {
  height: 32px;
  width: 60%;
  border-radius: 6px;
  background: linear-gradient(90deg, var(--card) 25%, var(--card-hi) 50%, var(--card) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.4s infinite;
}

.skeleton-meta {
  height: 16px;
  width: 40%;
  border-radius: 4px;
  background: linear-gradient(90deg, var(--card) 25%, var(--card-hi) 50%, var(--card) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.4s infinite 0.2s;
}

.skeleton-content {
  height: 14px;
  width: 100%;
  border-radius: 4px;
  background: linear-gradient(90deg, var(--card) 25%, var(--card-hi) 50%, var(--card) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.4s infinite 0.4s;
}

.skeleton-content.short {
  width: 70%;
}
</style>
