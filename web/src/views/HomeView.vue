<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/api'
import { useAppStore } from '@/stores/app'
import { fmtMs } from '@/utils/format'
import type { Vod, HistItem, Category } from '@/types'
import TopBar from '@/components/TopBar.vue'
import VodGrid from '@/components/VodGrid.vue'
import SkeletonList from '@/components/SkeletonList.vue'

const router = useRouter()
const app = useAppStore()

const history = ref<HistItem[]>([])
const feed = ref<Vod[]>([])
const feedSummary = ref('')
const loading = ref(true)
const hasSites = ref(false)
const confirmClear = ref(false)
let confirmTimer = 0

// 首页分类导航：取第一个可用站源的顶层分类
const cats = ref<Category[]>([])
const catSiteKey = ref('')

async function loadCats() {
  try {
    const sites = await api.sites()
    const usable = sites.filter((s) => s.usable)
    if (!usable.length) return
    catSiteKey.value = usable[0].key
    const all = await api.categories(usable[0].key)
    cats.value = all.filter((c) => c.pid === '' && c.id !== '').slice(0, 12)
  } catch {
    /* ignore */
  }
}

function goCat(tid: string) {
  router.push({ name: 'vod', query: { site: catSiteKey.value, tid } })
}

async function loadFeed() {
  try {
    const [h, f] = await Promise.all([api.history(), api.feed()])
    // 过滤掉影片名为空的记录（来自旧格式或测试遗留）
    history.value = h.filter((x: HistItem) => x.name && x.name.trim())
    feed.value = f.feed
    feedSummary.value = f.summary
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await app.loadStatus()
  hasSites.value = (app.status?.totalSites ?? 0) > 0
  await loadFeed()
  loadCats()
  // 到期自动抓一轮（后台）
  api.triggerCrawl().catch(() => {})
})

// 监听订阅/站源变更信号，自动重载 Feed 和状态
watch(() => app.feedVersion, async () => {
  loading.value = true
  await app.loadStatus()
  hasSites.value = (app.status?.totalSites ?? 0) > 0
  await loadFeed()
})

// 进度钳制：>0.99 视为看完（不再显示继续观看），<0.02 抬到 0.02（区分「刚开始看」与「没看过」）
function histRatio(h: HistItem): number {
  if (!h.dur || h.dur <= 0 || h.pos <= 0) return 0
  const r = h.pos / h.dur
  if (r > 0.99) return 0
  return r < 0.02 ? 0.02 : r
}

function progressOf(h: HistItem): number {
  return histRatio(h)
}

// 相对时间
function ago(ms: number): string {
  if (!ms) return ''
  const diff = Date.now() - ms
  const m = Math.floor(diff / 60000)
  if (m < 1) return '刚刚'
  if (m < 60) return m + ' 分钟前'
  const hh = Math.floor(m / 60)
  if (hh < 24) return hh + ' 小时前'
  const d = Math.floor(hh / 24)
  return d + ' 天前'
}

function continueVods(): Vod[] {
  return history.value
    .filter((h) => h.pos > 0)
    .slice(0, 20)
    .map((h) => ({
      id: h.vodId,
      name: h.name,
      pic: h.pic,
      remark: '',
      year: '',
      area: '',
      actor: '',
      director: '',
      content: '',
      typeName: '',
      from: h.from,
      playUrl: '',
      siteKey: h.siteKey,
      siteName: h.siteName
    }))
}

function openHistory(h: HistItem) {
  router.push({ name: 'detail', query: { site: h.siteKey, siteName: h.siteName, id: h.vodId } })
}

async function removeHist(h: HistItem) {
  await api.removeHistory(h)
  history.value = await api.history()
  app.showTip('已删除「' + h.name + '」')
}

async function clearHist() {
  if (!history.value.length) return
  if (!confirmClear.value) {
    confirmClear.value = true
    app.showTip(`再点一次确认清空（共 ${history.value.length} 条）`)
    if (confirmTimer) clearTimeout(confirmTimer)
    confirmTimer = window.setTimeout(() => (confirmClear.value = false), 6000)
    return
  }
  confirmClear.value = false
  if (confirmTimer) clearTimeout(confirmTimer)
  const n = history.value.length
  await api.clearHistory()
  history.value = []
  app.showTip('已清空 ' + n + ' 条记录')
}

// 内容更新进度
const updating = ref(false)
const updatePct = ref(0)
let updateTimer = 0

function refresh() {
  if (updating.value) return
  api.triggerCrawl().then(() => {
    updating.value = true
    updatePct.value = 0
    // 轮询进度，直到抓取完成
    updateTimer = window.setInterval(async () => {
      try {
        const r = await api.crawlSummary()
        if (r.total > 0) {
          updatePct.value = Math.min(100, Math.round((r.processed / r.total) * 100))
        }
        if (!r.running) {
          clearInterval(updateTimer)
          updating.value = false
          updatePct.value = 0
          await loadFeed()
          app.showTip(r.summary || '更新完成')
        }
      } catch {
        /* ignore */
      }
    }, 800)
  })
}
</script>

<template>
  <div class="page">
    <TopBar />

    <!-- 分类导航 -->
    <div v-if="cats.length" class="cat-nav">
      <span
        v-for="c in cats"
        :key="c.id"
        class="cat-chip"
        @click="goCat(c.id)"
      >{{ c.name }}</span>
    </div>

    <!-- 继续观看 -->
    <template v-if="loading">
      <div class="section-title">继续观看</div>
      <SkeletonList type="card" :count="6" />
    </template>
    <template v-else-if="history.length">
      <div class="section-title">
        继续观看
        <span class="count">{{ history.length }} 部</span>
        <div class="title-right">
          <span class="more" :class="{ danger: confirmClear }" @click="clearHist">{{ confirmClear ? '确认清空？' : '清空' }}</span>
          <span class="more" @click="router.push('/history')">全部 ›</span>
        </div>
      </div>
      <div class="continue-row">
        <div
          v-for="h in history.slice(0, 20)"
          :key="h.siteKey + h.vodId"
          class="continue-item"
          @click="openHistory(h)"
        >
          <div class="poster">
            <img :src="h.pic" :alt="h.name" loading="lazy" />
            <div v-if="histRatio(h) > 0" class="progress"><div class="bar" :style="{ width: histRatio(h) * 100 + '%' }"></div></div>
            <div class="ep">{{ h.epName || '继续观看' }}</div>
            <div class="del" @click.stop="removeHist(h)">✕</div>
          </div>
          <div class="name">{{ h.name }}</div>
          <div class="c-meta">
            <span v-if="h.epName">{{ h.epName }} · </span>
            <span v-if="histRatio(h) > 0">已看 {{ Math.round(histRatio(h) * 100) }}%</span>
            <span v-if="ago(h.time)"> · {{ ago(h.time) }}</span>
          </div>
        </div>
      </div>
    </template>

    <!-- 最近更新 -->
    <div class="section-title">
      最近更新
      <span v-if="feed.length" class="count">{{ feed.length }} 条</span>
      <span v-if="feedSummary" class="summary">{{ feedSummary }}</span>
      <span class="more" :class="{ updating }" @click="refresh">{{ updating ? `更新中 ${updatePct}%` : '更新 ›' }}</span>
    </div>
    <VodGrid :vods="feed" :loading="loading" />
    <div v-if="!loading && !feed.length" class="empty">
      <template v-if="hasSites">
        <p>暂无内容，正在拉取最新影片…</p>
        <p class="hint">也可以点上方「点播」按分类浏览，或到「设置 → 内容更新 → 立即更新」</p>
      </template>
      <template v-else>
        <p>暂无内容</p>
        <p class="hint">可以进「设置」点「订阅」，一个链接同时导入点播 + 直播源</p>
        <el-button type="primary" @click="router.push('/settings')">去订阅</el-button>
      </template>
    </div>
  </div>
</template>

<style scoped>
.cat-nav {
  display: flex;
  gap: 8px;
  padding: 4px 24px 16px;
  overflow-x: auto;
  flex-wrap: wrap;
}
.cat-chip {
  padding: 5px 14px;
  border-radius: 16px;
  font-size: 13px;
  color: var(--sub);
  background: var(--card);
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.14s;
}
.cat-chip:hover {
  color: #fff;
  background: var(--card-hi);
}
.more.updating {
  color: var(--live, #ff6b6b);
  cursor: default;
}
.continue-row {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 16px;
  padding: 0 24px;
}
.continue-item {
  cursor: pointer;
}
.continue-item .poster {
  position: relative;
  aspect-ratio: 16 / 9;
  border-radius: var(--radius);
  overflow: hidden;
  background: var(--card);
  border: 2px solid transparent;
}
.continue-item:hover .poster {
  border-color: #fff;
}
.poster img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.progress {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 4px;
  background: rgba(255, 255, 255, 0.2);
}
.progress .bar {
  height: 100%;
  background: var(--live);
}
.ep {
  position: absolute;
  right: 6px;
  bottom: 6px;
  font-size: 11px;
  color: #fff;
  background: rgba(0, 0, 0, 0.5);
  padding: 2px 6px;
  border-radius: 4px;
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
  opacity: 0;
  transition: opacity 0.14s;
}
.continue-item:hover .del {
  opacity: 1;
}
.del:hover {
  background: var(--live);
}
.name {
  margin-top: 6px;
  font-size: 14px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.c-meta {
  font-size: 12px;
  color: var(--dim);
  margin-top: 2px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.summary {
  font-size: 12px;
  color: var(--dim);
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.title-right {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 12px;
}
/* 取消 .more 自带的 margin-left: auto，由父容器统一控制 */
.title-right .more {
  margin-left: 0;
}
.more.danger {
  color: var(--warn);
}
.empty {
  text-align: center;
  padding: 80px 24px;
  color: var(--dim);
}
.empty .hint {
  font-size: 13px;
  margin-bottom: 16px;
}

/* 移动端 */
@media (max-width: 640px) {
  .continue-row {
    grid-template-columns: repeat(2, 1fr);
    gap: 10px;
    padding: 0 12px;
  }
  /* 移动端删除按钮始终可见（hover 在触摸屏无效） */
  .del {
    opacity: 1;
  }
}
</style>
