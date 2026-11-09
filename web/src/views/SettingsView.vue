<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/api'
import { useAppStore } from '@/stores/app'
import type { Site, WebdavSource } from '@/types'
import TopBar from '@/components/TopBar.vue'
import UpgradePanel from '@/components/UpgradePanel.vue'

const app = useAppStore()
const router = useRouter()

// ===== 导航 =====
type NavKey = 'source' | 'update' | 'webdav' | 'sites' | 'data' | 'about'
const navKey = ref<NavKey>('source')
const navItems: { key: NavKey; label: string }[] = [
  { key: 'source', label: '播放源' },
  { key: 'update', label: '内容更新' },
  { key: 'webdav', label: '网盘' },
  { key: 'sites',  label: '站源' },
  { key: 'data',   label: '数据' },
  { key: 'about',  label: '关于' }
]

// ===== 播放源 =====
const subUrl      = ref('')
const subName     = ref('')
const cfgText     = ref('')
const cfgState    = ref('未配置')
const liveInput   = ref('')
const liveSources = ref<{ name: string; url: string }[]>([])
const curLiveUrl  = ref('')
const showPresets = ref(false)

// ===== 内容更新 =====
const crawlSummary = ref('')
const crawling     = ref(false)

// ===== 站源 =====
const sites        = ref<Site[]>([])
const enabledKeys  = ref<Set<string>>(new Set())
const totalSites   = ref(0)
const showSites    = ref(false)
const showAll      = ref(false)
const speedResults = ref<Record<string, { ms: number; ok: boolean }>>({})
const speedTested  = ref(false)
const testing      = ref(false)

// ===== 网盘 =====
const wdSources  = ref<WebdavSource[]>([])
const wdEditOpen = ref(false)
const wdForm     = ref<WebdavSource>({ id: '', name: '', url: '', user: '', pass: '', root: '' })
const wdMsg      = ref('')
const wdMsgOk    = ref(false)
const wdTesting  = ref(false)

// ===== 数据 =====
const confirmClear = ref(false)
let   ccTimer = 0

// ===== 关于 =====
const showPrivacy = ref(false)
const appVersion  = ref('加载中...')
const buildTime   = ref('')
const osArch      = ref('')

function goMetrics() {
  router.push('/metrics')
}

// ==============================
onMounted(load)

async function load() {
  const [st, list, wdList, liveList] = await Promise.all([
    api.status(), api.sites(), api.webdavSources(), api.liveSources()
  ])
  subName.value      = st.subName || ''
  totalSites.value   = st.totalSites
  crawlSummary.value = st.crawlSummary || ''
  // configUrl 既是点播配置地址，也是订阅地址（订阅时一起保存了）
  // 有 subName 说明是通过订阅导入的，把地址回填到 subUrl 方便用户查看/修改
  if (st.configUrl) {
    cfgText.value  = st.configUrl
    cfgState.value = '已配置'
    if (st.subName) subUrl.value = st.configUrl  // 订阅地址回填
  } else {
    cfgState.value = list.length > 0 ? '已导入' : '未配置'
  }
  curLiveUrl.value = st.liveUrl || ''
  liveInput.value  = st.liveUrl || ''
  sites.value      = list
  enabledKeys.value = new Set(list.filter((s: Site) => s.enabled).map((s: Site) => s.key))
  wdSources.value   = wdList
  liveSources.value = liveList

  // 加载版本信息
  loadVersionInfo()
}

async function loadVersionInfo() {
  try {
    const res = await fetch('/api/version')
    const data = await res.json()
    appVersion.value = data.appVersion || 'v1.0.0'
    buildTime.value = data.buildTime || ''
    osArch.value = `${data.osType}/${data.arch}`
  } catch (err) {
    console.error('获取版本信息失败:', err)
    appVersion.value = 'v1.0.0'
  }
}

// ---------- 播放源 ----------
async function doSubscribe() {
  const v = subUrl.value.trim()
  if (!v) { app.showTip('先填订阅链接'); return }
  try {
    const r = await api.subscribe(v)
    app.showTip(`订阅成功：${r.name || '订阅源'} · ${r.sites} 站点 · ${r.lives} 个直播源`)
    subName.value = r.name || ''
    // 不清空 subUrl，保留地址让用户可查看和再次修改
    await load()
    app.notifyFeedRefresh() // 通知首页重载 Feed
  } catch (e: any) { app.showTip('订阅失败：' + e.message) }
}

async function doImportCfg() {
  const v = cfgText.value.trim()
  if (!v) { app.showTip('先填配置地址或 JSON'); return }
  try {
    const r = await api.loadConfig(v)
    app.showTip(`导入成功：${r.count} 个站源`)
    cfgState.value = '已导入'
    await load()
  } catch (e: any) { app.showTip('导入失败：' + e.message) }
}

async function doSetLive() {
  const v = liveInput.value.trim()
  if (!v) { app.showTip('先填直播源地址'); return }
  await api.setLiveUrl(v)
  curLiveUrl.value = v
  app.showTip('直播源已保存')
}

async function selectLive(url: string) {
  await api.setLiveUrl(url)
  curLiveUrl.value = url
  liveInput.value  = url
  app.showTip('已切换直播源')
}

// ---------- 内容更新 ----------
async function doCrawl() {
  crawling.value = true
  await api.triggerCrawl()
  app.showTip('已开始更新…')
  setTimeout(async () => {
    const st = await api.status()
    crawlSummary.value = st.crawlSummary || ''
    crawling.value = false
  }, 3000)
}

// ---------- 站源 ----------
async function toggleSite(s: Site, val: boolean | string | number) {
  const on = val as boolean
  const next = new Set(enabledKeys.value)
  if (on) next.add(s.key); else next.delete(s.key)
  enabledKeys.value = next
  await api.setEnabled([...next])}

function setAllEnabled() {
  const keys = sites.value.filter((s: Site) => s.usable).map((s: Site) => s.key)
  enabledKeys.value = new Set(keys)
  api.setEnabled(keys)
  app.showTip('已启用全部可用站源')
}

async function speedTest() {
  testing.value = true
  speedResults.value = {}
  app.showTip('开始测速，约 40 秒…')
  try {
    const list = await api.sitesSpeed()
    for (const r of list) speedResults.value[r.key] = { ms: r.ms, ok: r.ok }
    speedTested.value = true
    const ok = list.filter((r: any) => r.ok).length
    app.showTip(`测速完成：${ok}/${list.length} 个可用，已按速度排序`)
    sites.value = [...sites.value].sort((a: Site, b: Site) => {
      const sa = speedResults.value[a.key]
      const sb = speedResults.value[b.key]
      if (!sa && !sb) return 0
      if (!sa) return 1; if (!sb) return -1
      if (sa.ok !== sb.ok) return sa.ok ? -1 : 1
      return sa.ms - sb.ms
    })
  } finally { testing.value = false }
}

function enableFastest5() {
  const okList = Object.entries(speedResults.value)
    .filter(([, v]) => v.ok).sort((a, b) => a[1].ms - b[1].ms).slice(0, 5).map(([k]) => k)
  if (!okList.length) { app.showTip('先测速再启用'); return }
  enabledKeys.value = new Set(okList)
  api.setEnabled(okList)
  app.showTip('已启用最快 ' + okList.length + ' 个站源')
}

function enabledCount() {
  return sites.value.filter((s: Site) => s.usable && enabledKeys.value.has(s.key)).length
}

function speedText(key: string) {
  const r = speedResults.value[key]
  if (!r) return ''
  return r.ok ? r.ms + ' ms' : '不通'
}

function speedOk(key: string): boolean | null {
  const r = speedResults.value[key]
  return r ? r.ok : null
}

// ---------- 网盘 ----------
function openWdAdd() {
  wdForm.value = { id: '', name: '', url: '', user: '', pass: '', root: '' }
  wdMsg.value = ''
  wdEditOpen.value = true
}

function openWdEdit(s: WebdavSource) {
  wdForm.value = { ...s }
  wdMsg.value = ''
  wdEditOpen.value = true
}

async function testWd() {
  if (!wdForm.value.url.trim()) { wdMsg.value = '先填服务器地址'; wdMsgOk.value = false; return }
  wdTesting.value = true
  try {
    const r = await api.webdavProbe(wdForm.value)
    wdMsg.value  = `连接成功，根目录有 ${r.count} 项`; wdMsgOk.value = true
  } catch (e: any) { wdMsg.value = e.message; wdMsgOk.value = false }
  wdTesting.value = false
}

async function saveWd() {
  const url = wdForm.value.url.trim()
  if (!url) { wdMsg.value = '请填服务器地址'; wdMsgOk.value = false; return }
  if (!/^https?:\/\//.test(url)) { wdMsg.value = '地址要以 http:// 或 https:// 开头'; wdMsgOk.value = false; return }
  const f = { ...wdForm.value, url }
  if (!f.name) { try { f.name = new URL(url).host } catch { f.name = url } }
  await api.saveWebdavSource(f)
  wdSources.value = await api.webdavSources()
  wdEditOpen.value = false
  app.showTip('已保存「' + f.name + '」')
}

async function removeWd(s: WebdavSource) {
  await api.removeWebdavSource(s.id)
  wdSources.value = await api.webdavSources()
  wdEditOpen.value = false
  app.showTip('已删除「' + s.name + '」')
}

// ---------- 数据 ----------
async function clearHistory() {
  if (!confirmClear.value) {
    confirmClear.value = true
    app.showTip('再点一次确认清除播放记录')
    if (ccTimer) clearTimeout(ccTimer)
    ccTimer = window.setTimeout(() => confirmClear.value = false, 6000)
    return
  }
  confirmClear.value = false
  if (ccTimer) clearTimeout(ccTimer)
  await api.clearHistory()
  app.showTip('已清除播放记录')
}
</script>

<template>
  <div class="page">
    <TopBar />
    <div class="settings-layout">

      <!-- 左侧导航 -->
      <nav class="settings-nav">
        <div
          v-for="n in navItems"
          :key="n.key"
          class="nav-item"
          :class="{ active: navKey === n.key }"
          @click="navKey = n.key"
        >
          <span class="nav-bar"></span>
          <span class="nav-label">{{ n.label }}</span>
          <span v-if="n.key === 'sites' && totalSites > 0" class="nav-badge">
            {{ enabledCount() }}/{{ totalSites }}
          </span>
        </div>
      </nav>

      <!-- 细分割线 -->
      <div class="nav-divider"></div>

      <!-- 右侧内容区 -->
      <div class="settings-content">

        <!-- ========== 播放源 ========== -->
        <template v-if="navKey === 'source'">
          <div class="section-label">播放源</div>
          <div class="card">
            <!-- 订阅 -->
            <div class="srow srow-sub">
              <div class="srow-sub-head">
                <span class="srow-title">订阅</span>
                <span v-if="subName" class="srow-val sub-name">{{ subName }}</span>
                <span v-else class="srow-dim">未订阅</span>
              </div>
              <div class="srow-sub-body">
                <el-input
                  v-model="subUrl"
                  placeholder="订阅链接（https://…/subscribe.json）"
                  clearable
                  size="small"
                  style="flex:1"
                />
                <el-button type="primary" size="small" @click="doSubscribe">
                  {{ subName ? '重新订阅' : '订阅' }}
                </el-button>
              </div>
            </div>
            <div class="sdiv"></div>
            <!-- 点播配置 -->
            <div class="srow">
              <div class="srow-left">
                <span class="srow-title">点播配置</span>
                <span :class="cfgState === '未配置' ? 'srow-dim' : 'srow-val'">{{ cfgState }}</span>
              </div>
              <div class="srow-right">
                <el-input v-model="cfgText" placeholder="配置地址 / 整段 JSON" clearable size="small" />
                <el-button size="small" @click="doImportCfg">导入</el-button>
              </div>
            </div>
            <div class="sdiv"></div>
            <!-- 直播源 -->
            <div class="srow">
              <div class="srow-left">
                <span class="srow-title">直播源</span>
                <span v-if="curLiveUrl" class="srow-dim srow-url">{{ curLiveUrl }}</span>
                <span v-else class="srow-dim">未设置</span>
              </div>
              <div class="srow-right">
                <el-input v-model="liveInput" placeholder="m3u / txt 直播源地址" clearable size="small" />
                <el-button size="small" @click="doSetLive">保存</el-button>
              </div>
            </div>
            <!-- 订阅直播源列表折叠 -->
            <template v-if="liveSources.length">
              <div class="sdiv"></div>
              <div class="srow srow-clickable" @click="showPresets = !showPresets">
                <div class="srow-left">
                  <span class="srow-title">订阅直播源</span>
                  <span class="srow-dim">{{ liveSources.length }} 个 · {{ showPresets ? '收起' : '展开' }}</span>
                </div>
                <span class="arrow">›</span>
              </div>
              <template v-if="showPresets">
                <div v-for="(p, i) in liveSources" :key="i" class="schild" :class="{ 'schild-hot': curLiveUrl === p.url }" @click="selectLive(p.url)">
                  <span class="schild-name">{{ p.name }}</span>
                  <span class="schild-val">{{ curLiveUrl === p.url ? '✓ 使用中' : '使用' }}</span>
                </div>
              </template>
            </template>
          </div>
        </template>

        <!-- ========== 内容更新 ========== -->
        <template v-else-if="navKey === 'update'">
          <div class="section-label">内容更新</div>
          <div class="card">
            <div class="srow">
              <div class="srow-left">
                <span class="srow-title">上次更新</span>
                <span class="srow-dim">{{ crawlSummary || '还没有更新过' }}</span>
              </div>
            </div>
            <div class="sdiv"></div>
            <div class="srow srow-clickable" @click="doCrawl">
              <span class="srow-title">{{ crawling ? '正在抓取最新影片…' : '立即更新' }}</span>
              <span class="arrow">›</span>
            </div>
          </div>
          <div class="note">每 6 小时自动刷新远程配置并抓取各站最新影片。</div>
        </template>

        <!-- ========== 网盘 ========== -->
        <template v-else-if="navKey === 'webdav'">
          <div class="section-label">网盘</div>
          <div class="card">
            <div class="srow srow-clickable" @click="openWdAdd">
              <div class="srow-left">
                <span class="srow-title">添加挂载</span>
                <span class="srow-dim">{{ wdSources.length }} 个已挂载</span>
              </div>
              <span class="arrow">›</span>
            </div>
            <template v-if="!wdSources.length">
              <div class="sdiv"></div>
              <p class="note" style="padding:12px 18px 16px">挂载 WebDAV 网盘（Alist / 群晖 / Nextcloud 等）后，可在网盘页浏览目录、识别并播放影视资源。</p>
            </template>
            <template v-else>
              <div v-for="(s, i) in wdSources" :key="s.id">
                <div class="sdiv"></div>
                <div class="srow srow-clickable" @click="openWdEdit(s)">
                  <div class="srow-left">
                    <span class="srow-title">{{ s.name }}</span>
                    <span class="srow-dim srow-url">{{ s.url }}</span>
                  </div>
                  <span class="arrow">›</span>
                </div>
              </div>
            </template>
          </div>
        </template>

        <!-- ========== 站源 ========== -->
        <template v-else-if="navKey === 'sites'">
          <div class="section-label">站源</div>
          <div class="card">
            <div class="srow srow-clickable" @click="showSites = !showSites">
              <div class="srow-left">
                <span class="srow-title">站源列表</span>
                <span class="srow-dim">{{ sites.length }} 个 · 已启用 {{ enabledCount() }} · {{ showSites ? '收起' : '展开' }}</span>
              </div>
              <span class="arrow">›</span>
            </div>
            <div class="sdiv"></div>
            <div class="srow srow-clickable" @click="speedTest">
              <span class="srow-title">{{ testing ? '正在测速…' : '测速排行' }}</span>
              <span class="arrow">›</span>
            </div>
            <template v-if="speedTested">
              <div class="sdiv"></div>
              <div class="srow srow-clickable" @click="enableFastest5">
                <span class="srow-title">启用最快的 5 个</span>
                <span class="arrow">›</span>
              </div>
              <div class="sdiv"></div>
              <div class="srow srow-clickable" @click="setAllEnabled">
                <span class="srow-title">全部启用</span>
                <span class="arrow">›</span>
              </div>
            </template>
            <div class="sdiv"></div>
            <div class="srow srow-clickable" @click="showAll = !showAll; load()">
              <div class="srow-left">
                <span class="srow-title">站源筛选</span>
                <span class="srow-dim">{{ showAll ? '全部' : '仅可用' }}</span>
              </div>
              <span class="arrow">›</span>
            </div>
            <!-- 站源详细列表 -->
            <template v-if="showSites">
              <div class="sdiv"></div>
              <p v-if="!sites.length" class="note" style="padding:12px 18px 16px">还没有导入配置，去「播放源 → 点播配置」导入。</p>
              <div v-for="s in sites" :key="s.key" class="site-row">
                <div class="site-bar">
                  <el-switch :model-value="enabledKeys.has(s.key)" :disabled="!s.usable" @change="(v: any) => toggleSite(s, v)" />
                </div>
                <div class="site-info">
                  <div class="site-name">
                    {{ s.name }}
                    <span v-if="speedResults[s.key]" class="speed-tag" :class="{ ok: speedOk(s.key) === true, fail: speedOk(s.key) === false }">
                      {{ speedText(s.key) }}
                    </span>
                  </div>
                  <div class="site-meta" :class="{ warn: !s.usable }">{{ s.usable ? s.api : s.reason }}</div>
                </div>
                <span v-if="!s.usable" class="unusable-tag">不可用</span>
              </div>
            </template>
          </div>
        </template>

        <!-- ========== 数据 ========== -->
        <template v-else-if="navKey === 'data'">
          <div class="section-label">数据</div>
          <div class="card">
            <div class="srow srow-clickable" :class="{ warn: confirmClear }" @click="clearHistory">
              <span class="srow-title" :class="{ warn: confirmClear }">
                {{ confirmClear ? '再次点击确认清除播放记录' : '清除播放记录' }}
              </span>
              <span class="arrow">›</span>
            </div>
          </div>
        </template>

        <!-- ========== 关于 ========== -->
        <template v-else-if="navKey === 'about'">
          <div class="section-label">关于</div>
          <div class="card">
            <div class="srow">
              <span class="srow-title">版本</span>
              <span class="srow-val">{{ appVersion }}</span>
            </div>
            <div class="sdiv" v-if="buildTime"></div>
            <div class="srow" v-if="buildTime">
              <span class="srow-title">构建时间</span>
              <span class="srow-dim">{{ buildTime }}</span>
            </div>
            <div class="sdiv" v-if="osArch"></div>
            <div class="srow" v-if="osArch">
              <span class="srow-title">平台架构</span>
              <span class="srow-dim">{{ osArch }}</span>
            </div>
            <div class="sdiv"></div>
            <div class="srow srow-clickable" @click="showPrivacy = !showPrivacy">
              <span class="srow-title">隐私政策</span>
              <span class="srow-val">{{ showPrivacy ? '收起' : '查看' }}</span>
              <span class="arrow" style="margin-left:8px">›</span>
            </div>
            <template v-if="showPrivacy">
              <div class="sdiv"></div>
              <div class="privacy-block">
                <p>本应用不收集任何个人信息，没有自己的服务器。</p>
                <p>· 本机保存：源地址、收藏、历史、播放进度——均不上传。</p>
                <p>· 网络请求只发往你填写的第三方地址，不经过开发者。</p>
                <p>· 无广告、无统计 SDK、无账号。</p>
                <p>· 本应用为本地播放器，不提供、不存储、不分发任何影视内容；内容全部来自你自行填写的第三方接口。</p>
              </div>
            </template>
            <div class="sdiv"></div>
            <div class="srow srow-clickable" @click="goMetrics">
              <span class="srow-title">性能监控</span>
              <span class="srow-val">查看运行指标</span>
              <span class="arrow" style="margin-left:8px">›</span>
            </div>
            <div class="sdiv"></div>
            <div class="srow">
              <span class="srow-title">技术栈</span>
              <span class="srow-dim">Vue3 + Vite4 + TypeScript + Element Plus + Go</span>
            </div>
          </div>

          <!-- 升级检测 -->
          <div class="section-label" style="margin-top: 20px">系统更新</div>
          <UpgradePanel />
        </template>

      </div>
    </div>
  </div>

  <!-- ========== 网盘编辑弹窗 ========== -->
  <el-dialog
    v-model="wdEditOpen"
    :title="wdForm.id ? '编辑挂载' : '添加挂载'"
    width="560px"
    append-to-body
  >
    <div class="wd-form">
      <el-input v-model="wdForm.name" placeholder="名称（可留空，默认取域名）" clearable class="wd-input" />
      <el-input v-model="wdForm.url" placeholder="服务器地址，如 https://dav.example.com/dav" clearable class="wd-input" />
      <el-input v-model="wdForm.user" placeholder="用户名（匿名可留空）" clearable class="wd-input" />
      <el-input v-model="wdForm.pass" placeholder="密码" type="password" show-password clearable class="wd-input" />
      <el-input v-model="wdForm.root" placeholder="挂载根路径，如 /影视（可留空 = 根目录）" clearable class="wd-input" />
      <p v-if="wdMsg" class="wd-msg" :class="{ ok: wdMsgOk }">{{ wdMsg }}</p>
    </div>
    <template #footer>
      <div class="wd-footer">
        <div>
          <el-button v-if="wdForm.id" type="danger" @click="removeWd(wdForm as WebdavSource)">删除</el-button>
          <el-button :loading="wdTesting" @click="testWd">测试连接</el-button>
        </div>
        <div>
          <el-button @click="wdEditOpen = false">取消</el-button>
          <el-button type="primary" @click="saveWd">保存</el-button>
        </div>
      </div>
    </template>
  </el-dialog>
</template>

<style scoped>
/* ===== 双栏布局 ===== */
.settings-layout {
  display: flex;
  height: calc(100vh - 72px);
  overflow: hidden;
}
.settings-nav {
  width: 220px;
  flex-shrink: 0;
  padding: 14px 12px;
  overflow-y: auto;
}
.nav-divider {
  width: 1px;
  background: rgba(255,255,255,0.08);
  flex-shrink: 0;
}
.settings-content {
  flex: 1;
  overflow-y: auto;
  padding: 0 36px 40px 28px;
}
/* ===== 导航项 ===== */
.nav-item {
  display: flex;
  align-items: center;
  height: 52px;
  padding: 0 14px;
  border-radius: 12px;
  cursor: pointer;
  transition: background 0.14s, color 0.14s;
  margin-bottom: 2px;
}
.nav-item:hover { background: rgba(255,255,255,0.08); }
.nav-item.active { background: rgba(255,255,255,0.2); }
.nav-bar {
  width: 3px;
  height: 22px;
  border-radius: 2px;
  background: transparent;
  flex-shrink: 0;
  transition: background 0.14s;
}
.nav-item.active .nav-bar { background: #fff; }
.nav-label {
  font-size: 17px;
  color: var(--sub);
  margin-left: 14px;
  flex: 1;
  transition: color 0.14s;
}
.nav-item.active .nav-label { color: #fff; font-weight: 500; }
.nav-badge {
  font-size: 12px;
  color: var(--dim);
  background: rgba(255,255,255,0.1);
  padding: 2px 8px;
  border-radius: 10px;
}
/* ===== 分组标题 ===== */
.section-label {
  font-size: 12px;
  font-weight: 500;
  color: rgba(255,255,255,0.55);
  letter-spacing: 0.8px;
  padding: 18px 18px 8px;
}
/* ===== 内容卡片 ===== */
.card {
  background: var(--surface);
  border-radius: 14px;
  overflow: hidden;
}
/* ===== 行 ===== */
.srow {
  display: flex;
  align-items: center;
  min-height: 52px;
  padding: 0 18px;
  gap: 12px;
}
.srow-clickable { cursor: pointer; transition: background 0.14s; }
.srow-clickable:hover { background: rgba(255,255,255,0.08); }
.srow-left {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 3px;
}
.srow-right {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-shrink: 0;
  min-width: 0;
  width: 380px;
}
.srow-title { font-size: 16px; color: #fff; }
.srow-title.warn { color: var(--warn); }
.srow-val  { font-size: 13px; color: var(--selected); }
.srow-dim  { font-size: 13px; color: var(--dim); }
.srow-url  { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 380px; }
.arrow { font-size: 20px; color: var(--dim); margin-left: auto; flex-shrink: 0; }
.warn.srow-clickable:hover { background: rgba(255,100,100,0.08); }
/* ===== 分割线 ===== */
.sdiv { height: 1px; background: rgba(255,255,255,0.12); margin: 0 18px; }
/* ===== 子项（直播源列表等） ===== */
.schild {
  display: flex;
  align-items: center;
  height: 42px;
  padding: 0 18px 0 30px;
  cursor: pointer;
  transition: background 0.14s;
}
.schild:hover { background: rgba(255,255,255,0.06); }
.schild-hot { background: rgba(255,235,59,0.06); }
.schild-name { font-size: 15px; color: var(--txt); flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.schild-hot .schild-name { color: var(--selected); }
.schild-val { font-size: 13px; color: var(--dim); flex-shrink: 0; }
.schild-hot .schild-val { color: var(--selected); }
/* ===== 站源列表 ===== */
.site-row {
  display: flex;
  align-items: center;
  min-height: 58px;
  padding: 8px 18px 8px 30px;
  border-top: 1px solid rgba(255,255,255,0.08);
  gap: 12px;
}
.site-bar { flex-shrink: 0; }
.site-info { flex: 1; min-width: 0; }
.site-name { font-size: 15px; color: #fff; display: flex; align-items: center; gap: 8px; }
.site-meta { font-size: 12px; color: var(--dim); margin-top: 3px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 520px; }
.site-meta.warn { color: var(--warn); }
.speed-tag { font-size: 12px; padding: 1px 8px; border-radius: 10px; background: rgba(255,255,255,0.1); color: var(--dim); }
.speed-tag.ok   { color: #b7f0bc; background: rgba(183,240,188,0.1); }
.speed-tag.fail { color: var(--warn); background: rgba(255,100,100,0.1); }
.unusable-tag {
  font-size: 11px; color: var(--warn); border: 1px solid var(--warn);
  border-radius: 4px; padding: 1px 6px; flex-shrink: 0;
}
/* ===== 说明文字 ===== */
.note { font-size: 13px; color: var(--sub); line-height: 1.6; padding: 12px 18px 0; }
/* ===== 隐私块 ===== */
.privacy-block {
  padding: 12px 18px 16px 30px;
  font-size: 13px;
  color: var(--sub);
  line-height: 22px;
}
.privacy-block p { margin: 4px 0; }
/* ===== 网盘弹窗 ===== */
.wd-form { display: flex; flex-direction: column; gap: 10px; }
.wd-input { }
.wd-msg { font-size: 13px; color: var(--warn); margin: 0; }
.wd-msg.ok { color: #b7f0bc; }
.wd-footer { display: flex; justify-content: space-between; align-items: center; gap: 8px; }

/* ===== 订阅行（两段布局：名称行 + 输入行） ===== */
.srow-sub {
  flex-direction: column;
  align-items: stretch;
  gap: 10px;
  padding: 14px 18px;
  min-height: unset;
}
.srow-sub-head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.sub-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.srow-sub-body {
  display: flex;
  gap: 8px;
  align-items: center;
}

/* ===== 移动端响应式 ===== */
@media (max-width: 640px) {
  /* 双栏布局 → 单栏（上方tab + 下方内容） */
  .settings-layout {
    flex-direction: column;
    height: auto;
    overflow: visible;
  }
  .nav-divider { display: none; }
  .settings-nav {
    width: 100%;
    padding: 8px 8px 0;
    overflow-x: auto;
    overflow-y: visible;
    display: flex;
    gap: 4px;
    white-space: nowrap;
  }
  /* 导航项横排 tab 样式 */
  .nav-item {
    height: auto;
    padding: 8px 12px;
    border-radius: 20px;
    flex-shrink: 0;
    margin-bottom: 0;
  }
  .nav-bar { display: none; }
  .nav-label { font-size: 13px; }
  .nav-badge { font-size: 10px; padding: 0 4px; }
  .settings-content {
    padding: 0 12px 40px 12px;
    overflow: visible;
  }
  .card { padding: 0; }
  .srow { padding: 12px 14px; min-height: 48px; }
  .srow-right {
    flex-direction: column;
    align-items: flex-start;
    gap: 8px;
    max-width: 100%;
  }
  .srow-right .el-input { width: 100% !important; }
  .section-label { padding: 12px 14px 4px; }
  .srow-sub-body { flex-wrap: wrap; }
  /* 站源列表在手机上紧凑 */
  .site-meta { max-width: none; }
  .site-row { padding: 10px 14px; }
}
</style>
