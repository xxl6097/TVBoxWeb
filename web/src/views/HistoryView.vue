<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/api'
import { useAppStore } from '@/stores/app'
import type { HistItem } from '@/types'
import TopBar from '@/components/TopBar.vue'

const router = useRouter()
const app = useAppStore()
const hist = ref<HistItem[]>([])
const confirming = ref(false)
const confirmDel = ref('')
let confirmTimer = 0
let delTimer = 0

onMounted(load)

async function load() {
  const all = await api.history()
  // 过滤空名称的历史记录（旧格式或测试遗留）
  hist.value = all.filter((h: HistItem) => h.name && h.name.trim())
}

function progressOf(h: HistItem): number {
  if (!h.dur || h.dur <= 0 || h.pos <= 0) return 0
  const r = h.pos / h.dur
  return r > 0.99 ? 0 : r < 0.02 ? 0.02 : r
}

function open(h: HistItem) {
  router.push({ name: 'detail', query: { site: h.siteKey, siteName: h.siteName, id: h.vodId } })
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
  return Math.floor(hh / 24) + ' 天前'
}

// 单条删除二次确认
function remove(h: HistItem) {
  const key = h.siteKey + '|' + h.vodId
  if (confirmDel.value !== key) {
    confirmDel.value = key
    app.showTip(`再点一次确认删除「${h.name}」`)
    if (delTimer) clearTimeout(delTimer)
    delTimer = window.setTimeout(() => (confirmDel.value = ''), 6000)
    return
  }
  confirmDel.value = ''
  if (delTimer) clearTimeout(delTimer)
  api.removeHistory(h).then(() => {
    app.showTip('已删除「' + h.name + '」')
    load()
  })
}

function clearAll() {
  if (!confirming.value) {
    confirming.value = true
    app.showTip(`再点一次确认清空（共 ${hist.value.length} 条）`)
    if (confirmTimer) clearTimeout(confirmTimer)
    confirmTimer = window.setTimeout(() => (confirming.value = false), 6000)
    return
  }
  confirming.value = false
  if (confirmTimer) clearTimeout(confirmTimer)
  api.clearHistory().then(() => {
    app.showTip('历史已清空')
    load()
  })
}
</script>

<template>
  <div class="page">
    <TopBar />
    <div class="section-title">
      观看历史 <span class="count">{{ hist.length }} 条</span>
      <el-button size="small" class="more" :type="confirming ? 'danger' : 'default'" @click="clearAll">
        {{ confirming ? '确认清空？' : '清空历史' }}
      </el-button>
    </div>

    <div v-if="hist.length" class="list">
      <div v-for="h in hist" :key="h.siteKey + h.vodId" class="row" @click="open(h)">
        <div class="thumb">
          <img :src="h.pic" :alt="h.name" loading="lazy" />
        </div>
        <div class="info">
          <div class="name">{{ h.name }}</div>
          <div class="meta">
            {{ h.epName || '未开始' }}<span v-if="h.siteName"> · {{ h.siteName }}</span>
            <span v-if="ago(h.time)"> · {{ ago(h.time) }}</span>
          </div>
          <div v-if="progressOf(h) > 0" class="progress">
            <div class="bar" :style="{ width: progressOf(h) * 100 + '%' }"></div>
          </div>
        </div>
        <div
          class="del"
          :class="{ danger: confirmDel === h.siteKey + '|' + h.vodId }"
          @click.stop="remove(h)"
        >
          {{ confirmDel === h.siteKey + '|' + h.vodId ? '确认删除？' : '删除' }}
        </div>
      </div>
    </div>
    <div v-else class="empty">还没有播放记录</div>
  </div>
</template>

<style scoped>
.more {
  margin-left: auto;
}
.list {
  padding: 0 24px;
}
.row {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 10px;
  border-radius: 8px;
  cursor: pointer;
}
.row:hover {
  background: var(--card);
}
.thumb {
  width: 120px;
  aspect-ratio: 16 / 9;
  border-radius: 6px;
  overflow: hidden;
  background: var(--card);
  flex-shrink: 0;
}
.thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.info {
  flex: 1;
  min-width: 0;
}
.name {
  font-size: 15px;
  font-weight: 600;
}
.meta {
  font-size: 13px;
  color: var(--dim);
  margin-top: 4px;
}
.time {
  font-size: 13px;
  color: var(--dim);
}
.del {
  font-size: 13px;
  color: var(--sub);
  cursor: pointer;
  padding: 4px 8px;
  white-space: nowrap;
}
.del:hover {
  color: var(--warn);
}
.del.danger {
  color: var(--warn);
}
.progress {
  height: 3px;
  background: var(--white-15);
  border-radius: 2px;
  margin-top: 6px;
  overflow: hidden;
}
.progress .bar {
  height: 100%;
  background: var(--live);
  border-radius: 2px;
}
.empty {
  text-align: center;
  padding: 60px 24px;
  color: var(--dim);
}

/* ===== 移动端响应式 ===== */
@media (max-width: 640px) {
  .top { padding: 10px 16px 6px; flex-wrap: wrap; gap: 8px; }
  .count-hint { font-size: 13px; }
  .list { padding: 0 12px; }
  .row {
    padding: 10px 12px;
    gap: 10px;
  }
  .thumb { width: 80px; flex-shrink: 0; }
  .name { font-size: 14px; }
  .meta { font-size: 12px; }
  .del { font-size: 12px; padding: 4px; }
  .time-lbl { font-size: 12px; }
}
</style>
