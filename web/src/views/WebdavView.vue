<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, humanizeApiError } from '@/api'
import { useAppStore } from '@/stores/app'
import { usePlayerStore } from '@/stores/player'
import { fmtSize } from '@/utils/format'
import type { WebdavSource, WebdavVod, WebdavEntry, WebdavEp, WdHistory } from '@/types'
import TopBar from '@/components/TopBar.vue'

const router = useRouter()
const route = useRoute()
const app = useAppStore()
const player = usePlayerStore()

// 三种视图：sources（源列表）/ browse（浏览目录）/ scan（识别结果）
const view = ref<'sources' | 'browse' | 'scan'>('sources')
const sources = ref<WebdavSource[]>([])
const loading = ref(false)
const msg = ref('')

// 当前浏览的源与路径
const cur = ref<WebdavSource>({ id: '', name: '', url: '', user: '', pass: '', root: '' })
const curPath = ref('/')
const entries = ref<WebdavEntry[]>([])

// 识别结果 + 续播记录
const vods = ref<WebdavVod[]>([])
const wdHist = ref<WdHistory[]>([])

// 选集弹窗
const epOpen = ref(false)
const selVod = ref<WebdavVod | null>(null)

// 源表单
const formOpen = ref(false)
const form = ref<WebdavSource>({ id: '', name: '', url: '', user: '', pass: '', root: '' })

onMounted(async () => {
  await refreshData()
  // 从设置页「识别影视 → 查看结果」跳过来时带 ?src=<id>：
  // 该源已有识别缓存就直接进识别视图（不重扫）；缓存为空就停在源列表并说明。
  const srcId = route.query.src as string | undefined
  if (!srcId) return
  const s = sources.value.find((x) => x.id === srcId)
  if (!s) return
  const cache = await api.webdavVods(s.id).catch(() => [])
  if (cache.length > 0) {
    cur.value = s
    curPath.value = '/'
    vods.value = cache
    resetCollapsed()
    view.value = 'scan'
    msg.value = `已识别 ${cache.length} 部影视`
  } else {
    msg.value = `「${s.name}」还没有识别结果，点它进入即可开始识别`
  }
})

async function refreshData() {
  sources.value = await api.webdavSources()
  wdHist.value = await api.webdavHistory()
}

// UTF-8 base64（兼容中文用户名/密码）
function b64Utf8(s: string): string {
  return btoa(unescape(encodeURIComponent(s)))
}

function authOf(src: WebdavSource): string {
  return b64Utf8(`${src.user}:${src.pass}`)
}

/**
 * 取播放用的 Basic Auth。
 *
 * 不能用列表里拿到的 pass：GET /webdav/sources 出于安全返回掩码 ******，
 * 拿它拼认证会被上游 401（这就是「加了登录后网盘视频播不了」的直接原因——
 * 登录会清 IndexedDB，缓存里那份未掩码的旧数据没了）。
 * 改为向服务端按 id 换真实凭据。
 */
async function authOfSrc(src: WebdavSource): Promise<string> {
  try {
    const r = await api.webdavAuth(src.id)
    if (r?.auth) return r.auth
  } catch {
    /* 回退到本地拼接（无密码或旧版后端） */
  }
  return src.pass && src.pass !== '******' ? authOf(src) : ''
}

// ---------- 源管理 ----------

function openForm(src?: WebdavSource) {
  form.value = src ? { ...src } : { id: '', name: '', url: '', user: '', pass: '', root: '' }
  formOpen.value = true
}

async function saveForm() {
  const f = form.value
  if (!f.url) {
    app.showTip('请填写服务器地址')
    return
  }
  if (!f.name) {
    try {
      f.name = new URL(f.url).host
    } catch {
      f.name = f.url
    }
  }
  await api.saveWebdavSource(f)
  formOpen.value = false
  app.showTip('已保存')
  await refreshData()
}

async function removeSource(s: WebdavSource) {
  await api.removeWebdavSource(s.id)
  app.showTip('已删除')
  await refreshData()
}

async function probe(s: WebdavSource) {
  try {
    const r = await api.webdavProbe(s)
    app.showTip(`连接成功，${r.count} 个条目`)
  } catch (e: any) {
    app.showTip('连接失败：' + humanizeApiError(e))
  }
}

// ---------- 进入源 ----------

async function enterSource(src: WebdavSource) {
  cur.value = src
  curPath.value = '/'
  vods.value = []
  msg.value = ''
  const cache = await api.webdavVods(src.id).catch(() => [])
  if (cache.length > 0) {
    vods.value = cache
    resetCollapsed()
    view.value = 'scan'
    msg.value = `上次识别到 ${cache.length} 部影视，点「重新扫描」可刷新`
    return
  }
  // 无缓存 → 切到识别视图自动扫描
  view.value = 'scan'
  await scanVods()
}

// ---------- 浏览 ----------

async function loadDir() {
  loading.value = true
  entries.value = []
  try {
    entries.value = await api.webdavList(cur.value, curPath.value)
  } catch (e: any) {
    msg.value = humanizeApiError(e)
  }
  loading.value = false
}

async function openBrowse() {
  view.value = 'browse'
  msg.value = ''
  await loadDir()
}

async function enterDir(e: WebdavEntry) {
  curPath.value = e.href.replace(/\/+$/, '')
  await loadDir()
}

async function goUp() {
  const p = curPath.value.replace(/\/+$/, '')
  const i = p.lastIndexOf('/')
  curPath.value = i <= 0 ? '/' : p.substring(0, i)
  await loadDir()
}

function pathLabel(): string {
  let p = curPath.value
  try {
    p = decodeURIComponent(p)
  } catch {
    /* keep */
  }
  return p
}

// ---------- 播放 ----------

function epNameOf(vod: WebdavVod, ep: WebdavEp, i: number): string {
  if (vod.kind === 'series') {
    if (ep.season > 0 && ep.episode > 0) {
      return 'S' + pad2(ep.season) + 'E' + pad2(ep.episode)
    }
    if (ep.episode > 0) {
      return '第 ' + ep.episode + ' 集'
    }
  }
  return stripExt(ep.name)
}

function pad2(n: number): string {
  return n < 10 ? '0' + n : String(n)
}

function stripExt(name: string): string {
  const dot = name.lastIndexOf('.')
  return dot > 0 ? name.substring(0, dot) : name
}

function playFile(entry: WebdavEntry) {
  const v: WebdavVod = {
    name: stripExt(entry.name),
    kind: 'movie',
    year: '',
    dir: curPath.value.replace(/\/+$/, ''),
    eps: [{ name: entry.name, url: entry.href, href: entry.href, season: 0, episode: 0 }]
  }
  playEp(v, 0)
}

function openEp(vod: WebdavVod) {
  if (vod.eps.length === 1) {
    playEp(vod, 0)
    return
  }
  selVod.value = vod
  epOpen.value = true
}

async function playEp(vod: WebdavVod, idx: number) {
  if (idx < 0 || idx >= vod.eps.length) return
  // 交给播放列表，剧集可切上/下一集
  const eps = vod.eps.map((e, i) => ({ name: epNameOf(vod, e, i), url: e.url, urls: [e.url] }))
  player.set(eps, idx, vod.name, false, true)
  player.header = { auth: await authOfSrc(cur.value) }
  player.seek = 0
  player.wdSrcId = cur.value.id
  player.wdSrcName = cur.value.name
  player.wdVod = vod.name
  player.wdDir = vod.dir
  player.wdSeries = vod.eps.length > 1
  epOpen.value = false
  router.push({
    name: 'play',
    query: {
      url: vod.eps[idx].url,
      title: vod.name + (vod.eps.length > 1 ? ' ' + eps[idx].name : ''),
      site: 'webdav',
      id: 'webdav:' + cur.value.id + ':' + vod.name,
      from: 'direct',
      direct: '1',
      epIndex: String(idx)
    }
  })
}

// ---------- 续播 ----------

async function continueWatch(h: WdHistory) {
  const src = sources.value.find((s) => s.id === h.srcId)
  if (!src) {
    // 源已删除，记录失效，清掉
    await api.removeWdHistory(h)
    wdHist.value = await api.webdavHistory()
    return
  }
  // 尝试从识别缓存重建整部剧的选集，续播时也能切上/下一集
  let eps: { name: string; url: string; urls: string[] }[] = []
  let startIdx = 0
  let playUrl = h.url
  let title = h.vodName
  const cache = await api.webdavVods(src.id).catch(() => [])
  const vod = cache.find((v) => v.name === h.vodName)
  if (vod && vod.eps.length > 0) {
    eps = vod.eps.map((e, i) => ({ name: epNameOf(vod, e, i), url: e.url, urls: [e.url] }))
    startIdx = h.epIndex >= 0 && h.epIndex < vod.eps.length ? h.epIndex : 0
    playUrl = vod.eps[startIdx].url
    title = vod.name + (vod.eps.length > 1 ? ' ' + eps[startIdx].name : '')
  } else if (h.epName) {
    title = h.vodName + ' ' + h.epName
  }
  player.set(eps, startIdx, h.vodName, false, true)
  player.header = { auth: await authOfSrc(src) }
  player.seek = h.pos
  player.wdSrcId = src.id
  player.wdSrcName = src.name
  player.wdVod = h.vodName
  player.wdDir = h.dir
  player.wdSeries = vod && vod.eps.length > 0 ? vod.eps.length > 1 : h.epIndex > 0 || h.epName.length > 0
  router.push({
    name: 'play',
    query: {
      url: playUrl,
      title,
      site: 'webdav',
      id: 'webdav:' + src.id + ':' + h.vodName,
      from: 'direct',
      direct: '1',
      epIndex: String(startIdx)
    }
  })
}

async function removeWd(h: WdHistory) {
  await api.removeWdHistory(h)
  wdHist.value = await api.webdavHistory()
}

async function clearWd() {
  await api.clearWebdavHistory()
  wdHist.value = []
}

// ---------- 识别影视 ----------

async function scanVods() {
  loading.value = true
  msg.value = '正在扫描目录识别影视资源…'
  try {
    const result = await api.webdavScan(cur.value, 6)
    vods.value = result
    resetCollapsed()
    view.value = 'scan'
    msg.value = `识别到 ${result.length} 部影视`
  } catch (e: any) {
    msg.value = humanizeApiError(e)
  }
  loading.value = false
}

// ---------- 识别结果的目录层级 ----------
//
// 扫描结果本身是平铺的（每条一个 dir），直接列出来会丢掉用户自己的目录组织。
// 这里按 relDir 还原成树：同一层的条目归到一个分组，可以在界面上折叠展开。
// 例：
//   迅雷下载/凡人/01 风起天南/1.mp4   → 目录「凡人 / 01 风起天南」
//   迅雷下载/电影/某片.mp4            → 目录「电影」

interface VodNode {
  /** 本层目录名（最后一段） */
  label: string
  /** 从根到本层的完整相对路径，用于 key 与默认展开判断 */
  path: string
  /** 深度，用于缩进 */
  depth: number
  /** 本层直接包含的影视 */
  vods: WebdavVod[]
  /** 子目录 */
  children: VodNode[]
}

/** 取条目的相对目录路径；老后端没有 relDir 时按 dir 的最后一级兜底 */
function relDirOf(v: WebdavVod): string {
  if (typeof v.relDir === 'string') return v.relDir
  // 退化：用 dir 的最后一段
  const d = (v.dir || '').replace(/\/+$/, '')
  const seg = d.slice(d.lastIndexOf('/') + 1)
  try {
    return decodeURIComponent(seg)
  } catch {
    return seg
  }
}

/** 把平铺结果按目录层级组装成树 */
function buildVodTree(list: WebdavVod[]): VodNode[] {
  const root: VodNode = { label: '', path: '', depth: 0, vods: [], children: [] }
  const byPath = new Map<string, VodNode>([['', root]])

  const ensure = (path: string): VodNode => {
    const hit = byPath.get(path)
    if (hit) return hit
    const segs = path.split('/')
    const parentPath = segs.slice(0, -1).join('/')
    const parent = ensure(parentPath)
    const node: VodNode = {
      label: segs[segs.length - 1],
      path,
      depth: segs.length,
      vods: [],
      children: [],
    }
    parent.children.push(node)
    byPath.set(path, node)
    return node
  }

  for (const v of list) {
    const rel = relDirOf(v).replace(/^\/+|\/+$/g, '')
    ensure(rel).vods.push(v)
  }

  // 同一层的目录按自然顺序（01 < 02 < 10），条目按名称
  const sortNode = (n: VodNode) => {
    n.children.sort((a, b) => a.label.localeCompare(b.label, 'zh-CN', { numeric: true }))
    n.vods.sort((a, b) => a.name.localeCompare(b.name, 'zh-CN', { numeric: true }))
    n.children.forEach(sortNode)
  }
  sortNode(root)
  return root.children
}

// 折叠状态：key = 节点完整路径，true 表示已折叠
const collapsed = ref<Set<string>>(new Set())

const vodTree = computed(() => buildVodTree(vods.value))

/** 统计某个节点下（含子目录）的影视总数与集数 */
function countUnder(n: VodNode): { vods: number; eps: number } {
  let nv = n.vods.length
  let ne = n.vods.reduce((a, v) => a + v.eps.length, 0)
  for (const c of n.children) {
    const r = countUnder(c)
    nv += r.vods
    ne += r.eps
  }
  return { vods: nv, eps: ne }
}

function toggleNode(path: string) {
  const s = new Set(collapsed.value)
  if (s.has(path)) s.delete(path)
  else s.add(path)
  collapsed.value = s
}

const isCollapsed = (path: string) => collapsed.value.has(path)

/** 正在扫描或刚换源时，重置折叠状态（全部展开） */
function resetCollapsed() {
  collapsed.value = new Set()
}

/** 一行渲染项：目录行或影视行 */
type Row =
  | { type: 'dir'; node: VodNode }
  | { type: 'vod'; vod: WebdavVod; depth: number }

/**
 * 把树按当前折叠状态压平成渲染列表。
 * 不用递归组件：单文件里递归要额外抽组件，压平更简单也不会踩栈深度。
 */
const vodRows = computed<Row[]>(() => {
  const out: Row[] = []
  const walk = (nodes: VodNode[]) => {
    for (const n of nodes) {
      out.push({ type: 'dir', node: n })
      if (!isCollapsed(n.path)) {
        // 目录下直接挂的影视（如「电影」目录里的一堆片子）
        for (const v of n.vods) out.push({ type: 'vod', vod: v, depth: n.depth })
        walk(n.children)
      }
    }
  }
  walk(vodTree.value)
  return out
})

/** 目录行显示的统计文案 */
function dirMeta(n: VodNode): string {
  const c = countUnder(n)
  return `${c.vods} 部 · ${c.eps} 集`
}

function vodMeta(v: WebdavVod): string {
  return (
    (v.kind === 'series' ? '剧集 · ' : '电影 · ') +
    (v.year ? v.year + ' · ' : '') +
    v.eps.length +
    ' 集'
  )
}

function fmtPos(ms: number): string {
  if (ms <= 0) return '0:00'
  const t = Math.floor(ms / 1000)
  const h = Math.floor(t / 3600)
  const m = Math.floor((t % 3600) / 60)
  const s = t % 60
  const mm = (m < 10 ? '0' : '') + m
  const ss = (s < 10 ? '0' : '') + s
  return h > 0 ? `${h}:${mm}:${ss}` : `${mm}:${ss}`
}

function back() {
  if (epOpen.value) {
    epOpen.value = false
    return
  }
  if (view.value !== 'sources') {
    view.value = 'sources'
    entries.value = []
    vods.value = []
    msg.value = ''
    return
  }
  router.back()
}
</script>

<template>
  <div class="page">
    <TopBar />
    <div class="wd-head">
      <span class="back" @click="back">‹ 返回</span>
      <span class="wd-title">
        {{ view === 'sources' ? 'WebDAV 网盘' : view === 'browse' ? '浏览文件' : '识别影视' }}
      </span>
      <span v-if="view === 'browse'" class="wd-sub">{{ cur.name }} · {{ pathLabel() }}</span>
      <span v-else-if="view === 'scan'" class="wd-sub">{{ cur.name }}</span>
    </div>

    <!-- 源列表视图 -->
    <div v-if="view === 'sources'" class="wd-body">
      <template v-if="wdHist.length">
        <div class="section-title">
          继续观看
          <span class="count">{{ wdHist.length }} 条</span>
          <span class="more" @click="clearWd">清空</span>
        </div>
        <div class="wd-hist">
          <div v-for="h in wdHist" :key="h.srcId + h.vodName" class="hist-row" @click="continueWatch(h)">
            <div class="hist-info">
              <div class="hist-name">{{ h.vodName }}</div>
              <div class="hist-meta">
                {{ h.epName ? '看到 ' + h.epName : '电影' }}
                <span v-if="h.pos > 0" class="hist-pos">
                  · 已看 {{ fmtPos(h.pos) }}{{ h.dur > 0 ? ' / ' + fmtPos(h.dur) : '' }}
                </span>
              </div>
            </div>
            <span class="hist-go">继续播放 ›</span>
            <span class="hist-del" @click.stop="removeWd(h)">✕</span>
          </div>
        </div>
      </template>

      <p class="wd-note">挂载的网盘在这里浏览目录、识别并播放影视资源。</p>

      <div class="wd-toolbar">
        <el-button type="primary" @click="openForm()">添加网盘源</el-button>
      </div>

      <p v-if="!sources.length" class="wd-empty">暂无挂载的网盘，可以点上方「添加网盘源」，填服务器地址、账号密码即可。</p>
      <div v-else class="src-list">
        <div v-for="s in sources" :key="s.id" class="src-row">
          <div class="src-info" @click="enterSource(s)">
            <div class="src-name">{{ s.name }}</div>
            <div class="src-url">{{ s.url }}{{ s.root }}</div>
          </div>
          <span class="src-go" @click="enterSource(s)">进入 ›</span>
          <el-button size="small" @click="probe(s)">测试</el-button>
          <el-button size="small" @click="openForm(s)">编辑</el-button>
          <el-button size="small" type="danger" @click="removeSource(s)">删除</el-button>
        </div>
      </div>
    </div>

    <!-- 浏览视图 -->
    <div v-else-if="view === 'browse'" class="wd-body">
      <div class="wd-toolbar">
        <el-button type="primary" @click="scanVods">识别影视</el-button>
        <el-button @click="goUp">上级目录</el-button>
        <el-button @click="loadDir">刷新</el-button>
      </div>

      <div v-if="loading" class="wd-loading">加载中…</div>
      <div v-else-if="!entries.length" class="wd-empty">{{ msg || '这个目录是空的。' }}</div>
      <div v-else class="entry-list">
        <div v-for="(e, i) in entries" :key="e.href + i" class="entry-row" @click="e.isDir ? enterDir(e) : playFile(e)">
          <span class="e-icon">{{ e.isDir ? '📁' : '🎬' }}</span>
          <div class="e-info">
            <div class="e-name">{{ e.name }}</div>
            <div class="e-meta">{{ e.isDir ? '目录' : fmtSize(e.size) + (e.mtime ? ' · ' + e.mtime : '') }}</div>
          </div>
          <span class="e-go">{{ e.isDir ? '进入' : '播放' }}</span>
        </div>
      </div>
    </div>

    <!-- 识别视图 -->
    <div v-else class="wd-body">
      <div class="wd-toolbar">
        <el-button @click="openBrowse">浏览文件</el-button>
        <el-button type="primary" @click="scanVods">重新扫描</el-button>
      </div>

      <div v-if="loading" class="wd-loading">
        <p>扫描中…</p>
        <p v-if="msg" class="wd-loading-msg">{{ msg }}</p>
      </div>
      <div v-else-if="!vods.length" class="wd-empty">{{ msg || '没有识别到视频资源。' }}</div>
      <div v-else class="vod-list">
        <!-- 保留原目录层级：目录行可折叠，影视行缩进在所属目录下 -->
        <template v-for="(row, i) in vodRows" :key="row.type === 'dir' ? 'd:' + row.node.path : 'v:' + (row.vod.dir + '#') + i">
          <div
            v-if="row.type === 'dir'"
            class="dir-row"
            :style="{ paddingLeft: 14 + (row.node.depth - 1) * 16 + 'px' }"
            @click="toggleNode(row.node.path)"
          >
            <span class="dir-caret" :class="{ closed: isCollapsed(row.node.path) }">▾</span>
            <span class="dir-name">{{ row.node.label }}</span>
            <span class="dir-meta">{{ dirMeta(row.node) }}</span>
          </div>
          <div
            v-else
            class="vod-row"
            :style="{ marginLeft: 14 + row.depth * 16 + 'px' }"
            @click="openEp(row.vod)"
          >
            <span class="v-kind">{{ row.vod.kind === 'series' ? '剧集' : '电影' }}</span>
            <div class="v-info">
              <div class="v-name">{{ row.vod.name }}</div>
              <div class="v-meta">{{ vodMeta(row.vod) }}</div>
            </div>
            <span class="v-go">播放</span>
          </div>
        </template>
      </div>
    </div>

    <!-- 选集弹窗 -->
    <el-dialog v-model="epOpen" :title="selVod?.name" width="480px" append-to-body>
      <div class="ep-list">
        <div
          v-for="(ep, i) in selVod?.eps || []"
          :key="ep.href + i"
          class="ep-row"
          @click="playEp(selVod!, i)"
        >
          <span class="ep-name">{{ epNameOf(selVod!, ep, i) }}</span>
          <span class="ep-go">播放</span>
        </div>
      </div>
    </el-dialog>

    <!-- 源表单弹窗 -->
    <el-dialog v-model="formOpen" title="网盘源" width="480px" append-to-body>
      <el-form label-width="80px">
        <el-form-item label="名称">
          <el-input v-model="form.name" placeholder="留空自动用主机名" />
        </el-form-item>
        <el-form-item label="地址">
          <el-input v-model="form.url" placeholder="http://192.168.1.10:5244/dav" />
        </el-form-item>
        <el-form-item label="用户名">
          <el-input v-model="form.user" placeholder="匿名可留空" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="form.pass" type="password" show-password />
        </el-form-item>
        <el-form-item label="根路径">
          <el-input v-model="form.root" placeholder="/影视，留空从根开始" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="formOpen = false">取消</el-button>
        <el-button type="primary" @click="saveForm">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.wd-head {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 32px 4px;
}
.back {
  color: var(--sub);
  cursor: pointer;
  font-size: 15px;
}
.wd-title {
  font-size: 22px;
  font-weight: 600;
}
.wd-sub {
  font-size: 14px;
  color: var(--dim);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
}
.wd-body {
  width: 100%;
  padding: 8px 32px 40px;
}
.wd-body :deep(.section-title) {
  padding: 0;
  margin: 20px 0 14px;
}
.wd-note {
  font-size: 14px;
  color: var(--sub);
  line-height: 22px;
  margin: 6px 0 4px;
}
.wd-empty {
  font-size: 14px;
  color: var(--dim);
  margin-top: 16px;
}
.wd-toolbar {
  display: flex;
  gap: 8px;
  margin: 8px 0 14px;
}
.wd-loading {
  color: var(--dim);
  font-size: 14px;
  margin-top: 20px;
}
.wd-loading-msg {
  font-size: 13px;
  margin-top: 8px;
}
.wd-hist {
  margin-bottom: 8px;
}
.hist-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 14px;
  border-radius: 4px;
  background: var(--card);
  cursor: pointer;
  margin-bottom: 8px;
}
.hist-row:hover {
  background: var(--card-hi);
}
.hist-info {
  flex: 1;
  min-width: 0;
}
.hist-name {
  font-size: 15px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.hist-meta {
  font-size: 12px;
  color: var(--dim);
  margin-top: 3px;
}
.hist-pos {
  color: var(--selected);
}
.hist-go {
  font-size: 13px;
  color: var(--selected);
  white-space: nowrap;
}
.hist-del {
  font-size: 15px;
  color: var(--dim);
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 18px;
  cursor: pointer;
}
.hist-del:hover {
  color: #fff;
  background: var(--card-hi);
}
.src-list {
  margin-top: 8px;
}
.src-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 0;
  border-bottom: 1px solid var(--border);
}
.src-info {
  flex: 1;
  min-width: 0;
  cursor: pointer;
}
.src-name {
  font-size: 15px;
  font-weight: 600;
}
.src-url {
  font-size: 12px;
  color: var(--dim);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.src-go {
  font-size: 13px;
  color: var(--selected);
  cursor: pointer;
  white-space: nowrap;
}
.entry-list,
.vod-list {
  max-height: calc(100vh - 200px);
  overflow-y: auto;
}
.entry-row,
/* 目录行：表示一层目录，可折叠 */
.dir-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 9px 14px;
  margin-bottom: 6px;
  border-radius: 4px;
  cursor: pointer;
  background: var(--card);
  border-left: 2px solid var(--border);
  user-select: none;
}
.dir-row:hover { border-left-color: var(--selected); }
.dir-caret {
  font-size: 12px;
  color: var(--dim);
  width: 12px;
  flex-shrink: 0;
  transition: transform 0.15s;
  display: inline-block;
}
.dir-caret.closed { transform: rotate(-90deg); }
.dir-name {
  font-size: 14px;
  color: var(--txt);
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dir-meta {
  font-size: 11.5px;
  color: var(--dim);
  white-space: nowrap;
  flex-shrink: 0;
}

.vod-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  border-radius: 4px;
  background: var(--card);
  cursor: pointer;
  margin-bottom: 8px;
}
.entry-row:hover,
.vod-row:hover {
  background: var(--card-hi);
}
.e-icon {
  font-size: 18px;
}
.e-info,
.v-info {
  flex: 1;
  min-width: 0;
}
.e-name,
.v-name {
  font-size: 15px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.e-meta,
.v-meta {
  font-size: 12px;
  color: var(--dim);
  margin-top: 3px;
}
.e-go,
.v-go {
  font-size: 13px;
  color: var(--selected);
  white-space: nowrap;
}
.v-kind {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 4px;
  background: var(--white-15);
  white-space: nowrap;
}
.ep-list {
  max-height: 360px;
  overflow-y: auto;
}
.ep-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 14px;
  border-radius: 4px;
  background: var(--card);
  cursor: pointer;
  margin-bottom: 8px;
}
.ep-row:hover {
  background: var(--card-hi);
}
.ep-name {
  font-size: 14px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ep-go {
  font-size: 13px;
  color: var(--selected);
}

/* ===== 移动端响应式 ===== */
@media (max-width: 640px) {
  .wd-head { padding: 8px 16px 4px; gap: 8px; flex-wrap: wrap; }
  .wd-body { padding: 8px 16px 40px; }
  /* 继续观看网格：手机 2 列 */
  .continue-grid { grid-template-columns: repeat(2, 1fr); gap: 10px; }
  /* 文件列表 */
  .entry-row { padding: 10px 12px; gap: 10px; }
  .entry-name { font-size: 14px; }
  .entry-size { font-size: 11px; }
  /* 网盘源行 */
  .src-row { padding: 12px 14px; gap: 8px; flex-wrap: wrap; }
  .src-name { font-size: 15px; }
  .src-url { font-size: 11px; }
  /* 识别结果网格：手机 2 列 */
  .vod-grid-wd { grid-template-columns: repeat(2, 1fr); gap: 10px; }
}
</style>
