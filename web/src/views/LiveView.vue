<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/api'
import { useAppStore } from '@/stores/app'
import { usePlayerStore } from '@/stores/player'
import type { LiveDir, Channel, RawLive, LiveScore } from '@/types'
import TopBar from '@/components/TopBar.vue'

const router = useRouter()
const app = useAppStore()
const player = usePlayerStore()

const loading = ref(false)
const root = ref<LiveDir | null>(null)
const currentUrl = ref('')
const selectedPath = ref<string[]>([]) // 当前分组路径
const deadSet = ref<Set<string>>(new Set())

const sources = ref<RawLive[]>([])
const probing = ref(false)
const ranking = ref(false)
const speedOpen = ref(false)
const checkedCount = ref(0)
const rankResults = ref<Record<string, LiveScore>>({})
const ranked = ref<LiveScore[]>([])
const filter = ref('')

// ===== 性能优化：频道索引缓存 =====
// 避免每次搜索都递归遍历全树，改用预构建的扁平列表 + 首字母索引
const flatChannels = ref<Channel[]>([])
const channelIndex = ref<Map<string, Channel[]>>(new Map())

onMounted(async () => {
  sources.value = await api.liveSources()
  const u = (await api.liveUrl()).url
  if (u) {
    currentUrl.value = u
    await loadChannels()
  }
})

async function loadChannels() {
  if (!currentUrl.value) return
  loading.value = true
  try {
    const r = await api.liveChannels(currentUrl.value)
    root.value = r.root
    currentUrl.value = r.url
    deadSet.value = new Set()
  } catch (e: any) {
    app.showTip('直播源加载失败：' + e.message)
  } finally {
    loading.value = false
  }
}

// 当前分组下的频道（选中路径下递归收集）
const currentDir = computed<LiveDir | null>(() => {
  if (!root.value) return null
  if (!selectedPath.value.length) return root.value
  let node = root.value
  for (const name of selectedPath.value) {
    const child = node.children.find((c) => c.name === name)
    if (!child) return root.value
    node = child
  }
  return node
})

// 构建频道索引（当分组改变时）
function buildChannelIndex() {
  flatChannels.value = []
  channelIndex.value.clear()
  if (currentDir.value) {
    collect(currentDir.value, flatChannels.value)
  }
  
  // 按首字母分桶索引，加速搜索
  for (const ch of flatChannels.value) {
    const firstChar = ch.name.charAt(0).toLowerCase()
    if (!channelIndex.value.has(firstChar)) {
      channelIndex.value.set(firstChar, [])
    }
    channelIndex.value.get(firstChar)!.push(ch)
  }
}

// 当 currentDir 变化时重建索引
watch(currentDir, () => {
  buildChannelIndex()
}, { deep: true })

// 优化搜索：先用首字母索引快速定位候选集，再精准过滤
const channels = computed<Channel[]>(() => {
  const kw = filter.value.trim().toLowerCase()
  if (!kw) return flatChannels.value
  
  // 用首字母快速查找候选集
  const firstChar = kw.charAt(0)
  const candidates = channelIndex.value.get(firstChar) || flatChannels.value
  
  // 在候选集内精准过滤
  return candidates.filter((c) => c.name.toLowerCase().includes(kw))
})

function collect(n: LiveDir, out: Channel[]) {
  out.push(...n.channels)
  n.children.forEach((c) => collect(c, out))
}

function selectGroup(path: string[]) {
  selectedPath.value = path
}

function pathLabel(): string {
  if (!selectedPath.value.length) return '全部频道'
  return selectedPath.value.join(' › ')
}

function playChannel(ch: Channel) {
  const list = channels.value
  const idx = list.findIndex((c) => c.name === ch.name && c.url === ch.url)
  player.set(
    list.map((c) => ({ name: c.name, url: c.url, urls: c.urls })),
    idx,
    '直播',
    true,
    false
  )
  router.push({ name: 'play', query: { url: ch.url, title: ch.name, site: 'live', id: '', from: 'live' } })
}

async function switchSource(url: string) {
  currentUrl.value = url
  await api.setLiveUrl(url)
  await loadChannels()
}

// 测速排行：逐个测订阅下发的源，结果进面板
async function rankSources() {
  if (!sources.value.length) {
    app.showTip('还没有直播源，先去「设置 → 订阅」导入')
    return
  }
  if (ranking.value) {
    app.showTip('正在测速，请稍等')
    return
  }
  ranking.value = true
  speedOpen.value = true
  rankResults.value = {}
  ranked.value = []
  app.showTip('开始测速 ' + sources.value.length + ' 个源，约 30 秒')
  for (const s of sources.value) {
    rankResults.value[s.url] = { name: s.name, url: s.url, ok: false, ms: -1, count: 0, alive: 0, sampled: 0, score: 0, state: '排队中' }
  }
  const results: LiveScore[] = []
  for (const s of sources.value) {
    const r = await api.liveRankOne(s.name, s.url)
    rankResults.value[s.url] = r
    results.push(r)
  }
  ranked.value = results
    .filter((x) => x.ok)
    .sort((a, b) => {
      if (a.score !== b.score) return b.score - a.score
      if (a.alive !== b.alive) return b.alive - a.alive
      return a.ms - b.ms
    })
  ranking.value = false
  const usable = results.filter((x) => x.ok).length
  if (ranked.value.length) {
    app.showTip(`测完 ${results.length} 个源，${usable} 个可用；建议用「${ranked.value[0].name}」`)
  } else {
    app.showTip('所有源都没测通，检查网络后重试')
  }
}

function useSource(url: string) {
  if (ranking.value) {
    app.showTip('正在测速，稍等一下再切换')
    return
  }
  if (url === currentUrl.value) {
    app.showTip('已经在用这个源')
    return
  }
  const s = sources.value.find((x) => x.url === url)
  switchSource(url)
  speedOpen.value = false
  app.showTip(`已切到「${s?.name || url}」，正在重新加载`)
}

// 检测当前分组频道可用性
async function probeChannels() {
  if (!channels.value.length) {
    app.showTip('当前分组没有频道')
    return
  }
  if (probing.value) {
    app.showTip('正在检测，请稍等')
    return
  }
  probing.value = true
  checkedCount.value = 0
  const list = channels.value.slice(0, 40)
  app.showTip('开始检测 ' + list.length + ' 个频道')
  let dead = 0
  for (const ch of list) {
    try {
      const r = await api.peek(ch.url)
      const key = ch.name + '|' + ch.url
      if (!r.alive) {
        dead++
        if (!deadSet.value.has(key)) {
          deadSet.value = new Set([...deadSet.value, key])
        }
      } else if (deadSet.value.has(key)) {
        const next = new Set(deadSet.value)
        next.delete(key)
        deadSet.value = next
      }
    } catch {
      /* ignore */
    }
    checkedCount.value++
  }
  probing.value = false
  if (dead === 0) {
    app.showTip(`这 ${list.length} 台全部可连`)
  } else {
    app.showTip(`${list.length} 台里 ${dead} 台连不上，已标灰${channels.value.length > 40 ? '（只测了前 40 台）' : ''}`)
  }
}

function isDead(ch: Channel): boolean {
  return deadSet.value.has(ch.name + '|' + ch.url)
}
</script>

<template>
  <div class="page">
    <TopBar />
    <div class="live-head">
      <div class="live-title">
        {{ root ? root.count + ' 台 · ' + root.children.length + ' 组' : '直播' }}
        <span v-if="pathLabel() !== '全部频道'" class="path">{{ pathLabel() }}</span>
      </div>
      <div class="live-actions">
        <el-select
          :model-value="currentUrl"
          size="small"
          placeholder="选择直播源"
          style="width: 240px"
          @change="switchSource"
        >
          <el-option v-for="s in sources" :key="s.url" :label="s.name" :value="s.url" />
        </el-select>
        <el-button size="small" :loading="probing" @click="probeChannels">检测</el-button>
        <el-button size="small" :loading="ranking" @click="rankSources">测速排行</el-button>
      </div>
    </div>

    <div v-if="!currentUrl" class="empty">
      <p>还没有配置直播源</p>
      <el-button type="primary" @click="router.push('/settings')">去设置</el-button>
    </div>

    <div v-else class="live-body" v-loading="loading">
      <!-- 左分组 -->
      <div class="groups">
        <div class="group" :class="{ active: !selectedPath.length }" @click="selectGroup([])">
          全部频道
        </div>
        <template v-if="root">
          <div v-for="g in root.children" :key="g.name">
            <div
              class="group"
              :class="{ active: selectedPath.length === 1 && selectedPath[0] === g.name }"
              @click="selectGroup([g.name])"
            >
              {{ g.name }} <span class="cnt">{{ g.count }}</span>
            </div>
            <div
              v-for="sub in g.children"
              :key="sub.name"
              class="group sub"
              :class="{ active: selectedPath.length === 2 && selectedPath[0] === g.name && selectedPath[1] === sub.name }"
              @click="selectGroup([g.name, sub.name])"
            >
              {{ sub.name }} <span class="cnt">{{ sub.count }}</span>
            </div>
          </div>
        </template>
      </div>

      <!-- 右频道 -->
      <div class="channels">
        <div class="ch-search">
          <el-input
            v-model="filter"
            size="small"
            clearable
            placeholder="搜索频道（如 CCTV / 卫视）"
            style="width: 260px"
          />
          <span v-if="filter" class="ch-search-count">{{ channels.length }} 台</span>
        </div>
        <div
          v-for="ch in channels"
          :key="ch.name + ch.url"
          class="channel"
          :class="{ dead: isDead(ch) }"
          @click="playChannel(ch)"
        >
          <span class="ch-name">{{ ch.name }}</span>
          <span v-if="ch.urls.length > 1" class="ch-alts">{{ ch.urls.length }} 线路</span>
        </div>
      </div>
    </div>

    <!-- 测速排行面板 -->
    <el-dialog v-model="speedOpen" title="直播源测速排行" width="620px" append-to-body :close-on-click-modal="!ranking">
      <div class="speed-sub">{{ ranking ? '测速中…' : '可播率 70% + 响应速度 30% 综合排序' }}</div>
      <div class="speed-list">
        <div v-for="(s, i) in sources" :key="s.url" class="speed-row">
          <span class="speed-rank" :class="{ top: i < 3 }">{{ i + 1 }}</span>
          <div class="speed-info">
            <div class="speed-name">
              {{ s.name }}
              <span v-if="s.url === currentUrl" class="tag">使用中</span>
              <span v-else-if="ranked.length && ranked[0].url === s.url" class="tag rec">推荐</span>
            </div>
            <div class="speed-state" :class="{ fail: rankResults[s.url] && !rankResults[s.url].ok }">
              {{ rankResults[s.url] ? rankResults[s.url].state : '待测' }}
              <span v-if="rankResults[s.url] && rankResults[s.url].ok" class="score">
                · {{ rankResults[s.url].score }} 分
              </span>
            </div>
          </div>
          <el-button size="small" :type="s.url === currentUrl ? 'primary' : 'default'" :disabled="ranking" @click="useSource(s.url)">
            {{ s.url === currentUrl ? '当前' : '使用' }}
          </el-button>
        </div>
      </div>
      <template #footer>
        <el-button :loading="ranking" @click="rankSources">{{ ranking ? '测速中…' : '重新测速' }}</el-button>
        <el-button type="primary" @click="speedOpen = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.live-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 24px 16px;
}
.live-title {
  font-size: 20px;
  font-weight: 600;
}
.path {
  font-size: 14px;
  color: var(--sub);
  font-weight: 400;
  margin-left: 12px;
}
.live-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.live-body {
  display: flex;
  padding: 0 24px;
  gap: 16px;
}
.groups {
  width: 220px;
  flex-shrink: 0;
  max-height: calc(100vh - 160px);
  overflow-y: auto;
}
.group {
  padding: 10px 14px;
  border-radius: 6px;
  cursor: pointer;
  color: var(--sub);
  font-size: 14px;
  display: flex;
  justify-content: space-between;
}
.group:hover {
  background: var(--card);
}
.group.active {
  color: var(--selected);
  background: var(--card-hi);
}
.group.sub {
  padding-left: 28px;
  font-size: 13px;
}
.cnt {
  color: var(--dim);
  font-size: 12px;
}
.channels {
  flex: 1;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 8px;
  align-content: start;
  max-height: calc(100vh - 160px);
  overflow-y: auto;
}
.ch-search {
  grid-column: 1 / -1;
  display: flex;
  align-items: center;
  gap: 10px;
  padding-bottom: 4px;
  position: sticky;
  top: 0;
  background: var(--bg);
  z-index: 1;
}
.ch-search-count {
  font-size: 12px;
  color: var(--dim);
  white-space: nowrap;
}
.channel {
  padding: 12px;
  border-radius: 6px;
  background: var(--white-15);
  cursor: pointer;
  font-size: 14px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  transition: all 0.14s;
}
.channel:hover {
  background: var(--card-hi);
}
.channel.dead {
  color: var(--dim);
  text-decoration: line-through;
}
.ch-alts {
  font-size: 11px;
  color: var(--dim);
}
.empty {
  text-align: center;
  padding: 80px 24px;
  color: var(--dim);
}
.speed-sub {
  font-size: 13px;
  color: var(--dim);
  margin-bottom: 12px;
}
.speed-list {
  max-height: 50vh;
  overflow-y: auto;
}
.speed-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px;
  border-radius: 6px;
}
.speed-row:hover {
  background: var(--card);
}
.speed-rank {
  width: 24px;
  height: 24px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  color: var(--sub);
  background: var(--white-15);
  flex-shrink: 0;
}
.speed-rank.top {
  color: #141218;
  background: #ffeb3b;
  font-weight: 600;
}
.speed-info {
  flex: 1;
  min-width: 0;
}
.speed-name {
  font-size: 14px;
  display: flex;
  align-items: center;
  gap: 8px;
}
.tag {
  font-size: 11px;
  padding: 1px 8px;
  border-radius: 4px;
  color: var(--selected);
  border: 1px solid var(--selected);
}
.tag.rec {
  color: #7ee787;
  border-color: #7ee787;
}
.speed-state {
  font-size: 12px;
  color: var(--dim);
  margin-top: 3px;
}
.speed-state.fail {
  color: var(--warn);
}
.speed-state .score {
  color: var(--selected);
}

/* ===== 移动端响应式 ===== */
@media (max-width: 640px) {
  .top { padding: 8px 16px; gap: 8px; }
  .groups { 
    overflow-x: auto; 
    flex-wrap: nowrap;
    padding: 4px 12px 8px;
  }
  .group-btn { font-size: 13px; padding: 6px 12px; }
  .channels {
    grid-template-columns: repeat(2, 1fr);
    gap: 8px;
    padding: 0 12px;
  }
  .channel { padding: 10px; font-size: 13px; }
  .empty { padding: 60px 16px; }
}
</style>
