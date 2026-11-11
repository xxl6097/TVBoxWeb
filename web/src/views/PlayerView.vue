<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, proxied, remuxed, humanizeApiError } from '@/api'
import { useAppStore } from '@/stores/app'
import { usePlayerStore } from '@/stores/player'
import { fmtMs } from '@/utils/format'
import { fromHlsLevels, labelOfLevel, labelOf, AUTO, TOP, QLevel } from '@/utils/quality'
import { usePlayerSubtitles } from '@/composables/usePlayerSubtitles'
import { usePlayerGestures } from '@/composables/usePlayerGestures'
import PlayerSettingsPanel from '@/components/player/PlayerSettingsPanel.vue'
import EpisodeDrawer from '@/components/player/EpisodeDrawer.vue'

// hls.js 动态加载（410KB → 只在进播放页时才请求）
let Hls: any = null
async function loadHls(): Promise<any> {
  if (Hls) return Hls
  const mod = await import('hls.js')
  Hls = mod.default || mod
  return Hls
}

const route = useRoute()
const router = useRouter()
const app = useAppStore()
const player = usePlayerStore()

const SEEK_STEP = 10000
const LIVE_SKIP_MAX = 4

const rawUrl0 = (route.query.url as string) || ''
const siteKey = (route.query.site as string) || ''
const vodId = (route.query.id as string) || ''
const vodName = (route.query.name as string) || (route.query.vodTitle as string) || ''
const vodPic = (route.query.pic as string) || ''
const vodSiteName = (route.query.siteName as string) || ''

// 转封装模式：MKV 等容器由服务端实时转成 fMP4。
// 这种流不支持 Range，所以拖进度条要带 seek 重新起转。
const remuxMode = ref(false)
const remuxSeek = ref(0)

let rawUrl = rawUrl0
let finalUrl = ''
let isLive = player.isLive
let directMode = player.directMode || route.query.direct === '1'
let qCap = 0
let levelIdx = -1
const levels = ref<QLevel[]>([])
let liveFail = 0

const videoEl = ref<HTMLVideoElement>()
let hls: any | null = null
let hlsRecover = 0 // hls fatal 错误恢复次数，避免 seek/网络抖动直接判死

const title = ref((route.query.title as string) || '')
const epLabel = ref('')
const resolving = ref(true)
const prepared = ref(false)
const playing = ref(false)
const buffering = ref(false)
const curMs = ref(0)
const durMs = ref(0)
const showOsd = ref(true)
const showQ = ref(false)
const showVol = ref(false)
const volume = ref(1)
const muted = ref(false)
const qLabel = ref('自动')
const via = ref('')
const errMsg = ref('')
const hint = ref('')
const playFlash = ref('')
const skipShow = ref('')
// 画面比例：0 拉伸铺满 / 1 裁剪铺满 / 2 等比适应
const scaleMode = ref(0)
const skipLeft = ref(0)
const introSkipSec = ref(0)
const outroSkipSec = ref(0)
// ===== 连续播放 =====
const autoPlayNext = ref(true)  // 自动连播开关，默认开启
// ===== 字幕支持（usePlayerSubtitles 组合式函数，见 composables/usePlayerSubtitles.ts）=====
const {
  subtitles,
  currentSubIdx,
  subDelay,
  subFontSize,
  subLabel,
  externalSubActive,
  loadSubtitlesFromHls,
  switchSubtitle,
  applySubStyle,
  setSubFontSize,
  removeExternalSub,
  setExternalSub,
  onSubFile,
  promptSubUrl,
  setSubDelay,
  disposeSubtitles
} = usePlayerSubtitles({
  videoEl,
  getHls: () => hls,
  tip: (m: string) => app.showTip(m),
  proxy: (u: string) => proxied(u)
})
void switchSubtitle // 内嵌字幕轨道切换，保留给将来 UI 使用

// ===== 倍速播放 =====
const playbackRate = ref(1.0)
const PLAYBACK_RATES = [0.5, 0.75, 1, 1.25, 1.5, 2]
let introDone = false
let outroFired = false
let skipTimer = 0
let osdTimer = 0
let flashTimer = 0
let volTimer = 0
let leave = false
let lastSaveMs = 0
let saveTimer = 0 // savePos 防抖 timer

// ===== Seek 防抖优化（三层防抖）=====
const SEEK_DEBOUNCE = 300      // 拖拽时防抖 300ms，给用户反馈
const SEEK_THROTTLE = 5000     // 最终落盘节流 5s，避免频繁网络请求
let lastSeekMs = 0             // 上次成功 seek 的时间戳
let seekPendingTimer = 0       // 等待中的 seek timer

const isLivePlaying = computed(() => isLive)
const playPauseText = computed(() => (playing.value ? '暂停' : '播放'))
const muteText = computed(() => (muted.value ? '取消静音' : '静音'))
const volPct = computed(() => (muted.value ? '静音' : Math.round(volume.value * 100) + '%'))
const progressPct = computed(() => (durMs.value > 0 ? (curMs.value / durMs.value) * 100 : 0))

// ===== 移动端检测 =====
const isMobile = ref(false)
function checkMobile() { isMobile.value = window.innerWidth <= 640 }

// ===== 画中画 =====
const isPip = ref(false)
function supportsPip(): boolean {
  return !!(document as any).pictureInPictureEnabled
}
async function togglePip() {
  const vid = videoEl.value
  if (!vid) return
  try {
    const doc = document as any
    if (doc.pictureInPictureElement) {
      await doc.exitPictureInPicture()
    } else {
      await (vid as any).requestPictureInPicture()
    }
  } catch (e) {
    app.showTip('此设备不支持画中画')
  }
}

// ===== 投屏（Remote Playback API）=====
const castConnected = ref(false)
function remoteApi(): any {
  return (videoEl.value as any)?.remote
}
function supportsCast(): boolean {
  return !!remoteApi()
}
async function toggleCast() {
  const remote = remoteApi()
  if (!remote) {
    app.showTip('当前浏览器不支持投屏，可以用 Chrome / Edge 连接 Chromecast 设备')
    return
  }
  try {
    if (remote.state === 'connected') {
      await remote.disconnect()
      app.showTip('已断开投屏')
    } else {
      app.showTip('正在搜索投屏设备…')
      await remote.prompt()
    }
  } catch {
    app.showTip('投屏已取消，或没有可用设备')
  }
}

// ===== 全屏 =====
const playerEl = ref<HTMLElement>()
const isFullscreen = ref(false)

function tryLockLandscape() {
  try {
    const p = (screen as any)?.orientation?.lock?.('landscape')
    if (p?.catch) p.catch(() => {})
  } catch { /* 部分浏览器不支持，忽略 */ }
}

function tryUnlockOrientation() {
  try { (screen as any)?.orientation?.unlock?.() } catch { /* ignore */ }
}

function toggleFullscreen() {
  // 移动端统一走 CSS 全屏类，避免原生 Fullscreen API 与 isFullscreen 状态不同步
  if (isMobile.value) {
    isFullscreen.value = !isFullscreen.value
    if (isFullscreen.value) tryLockLandscape()
    else tryUnlockOrientation()
    return
  }
  const el = playerEl.value
  if (!el) return
  const doc = document as any
  const isFs =
    doc.fullscreenElement ||
    doc.webkitFullscreenElement ||
    doc.mozFullScreenElement ||
    doc.msFullscreenElement

  if (isFs) {
    ;(doc.exitFullscreen || doc.webkitExitFullscreen || doc.mozCancelFullScreen || doc.msExitFullscreen)?.call(doc)
  } else {
    const req = (el as any).requestFullscreen ||
      (el as any).webkitRequestFullscreen ||
      (el as any).mozRequestFullScreen ||
      (el as any).msRequestFullscreen
    req?.call(el)
    tryLockLandscape()
  }
}

function onFullscreenChange() {
  const doc = document as any
  isFullscreen.value = !!(
    doc.fullscreenElement ||
    doc.webkitFullscreenElement ||
    doc.mozFullScreenElement ||
    doc.msFullscreenElement
  )
}

function syncEpLabel() {
  const n = player.count
  if (isLive) {
    epLabel.value = n > 1 ? `第 ${player.idx + 1}/${n} 台` : ''
    return
  }
  epLabel.value = n > 1 ? `第 ${player.idx + 1}/${n} 集` : ''
}

onMounted(async () => {
  const prefs = await api.prefs().catch(() => ({ qPref: 0, speedPref: 1, volumePref: 1, introSkip: 0, outroSkip: 0, tapSound: 1, tapVibrate: 1 }))
  qCap = prefs.qPref || 0
  introSkipSec.value = prefs.introSkip || 0
  outroSkipSec.value = prefs.outroSkip || 0
  // 应用倍速偏好（持久化）
  const sp = prefs.speedPref || 1
  if (PLAYBACK_RATES.includes(sp)) {
    playbackRate.value = sp
  }
  // 应用音量偏好（持久化，0=未设置用 1.0）
  volume.value = Math.min(1, Math.max(0, prefs.volumePref || 1))
  syncEpLabel()
  await resolveUrl()
  resolving.value = false
  initPlayer()
  if (videoEl.value) videoEl.value.volume = volume.value
  // 监听投屏连接状态（Remote Playback API）
  const remote = remoteApi()
  if (remote) {
    const onConnect = () => { castConnected.value = true; app.showTip('已连接投屏设备') }
    const onDisconnect = () => { castConnected.value = false; app.showTip('投屏已断开') }
    remote.addEventListener?.('connect', onConnect)
    remote.addEventListener?.('disconnect', onDisconnect)
    castHandlers = { onConnect, onDisconnect }
    castConnected.value = remote.state === 'connected'
  }
})

onUnmounted(() => {
  leave = true
  // 清理所有 timers
  if (saveTimer) clearTimeout(saveTimer)
  if (seekPendingTimer) clearTimeout(seekPendingTimer)
  if (volSaveTimer) clearTimeout(volSaveTimer)
  removeExternalSub()
  disposeSubtitles()
  if (autoplayTimer) { clearInterval(autoplayTimer); autoplayTimer = 0 }
  if (castHandlers) {
    const remote = remoteApi()
    remote?.removeEventListener?.('connect', castHandlers.onConnect)
    remote?.removeEventListener?.('disconnect', castHandlers.onDisconnect)
    castHandlers = null
  }
  tryUnlockOrientation()
  // 页面卸载时确保最终进度被保存
  savePos()
  destroyPlayer()
})

// ---------- 解析 ----------

function fixPlayable(u: string): string {
  if (!u || isDirectUrl(u) || !u.includes('://')) return u
  const noQuery = u.split('?')[0].replace(/\/+$/, '')
  const last = noQuery.slice(noQuery.lastIndexOf('/') + 1)
  if (last.length > 0 && !last.includes('.')) return noQuery + '/index.m3u8'
  return u
}

function isDirectUrl(u: string): boolean {
  const lu = u.toLowerCase()
  return ['.m3u8', '.mp4', '.flv', '.mkv', '.ts', '.m4v'].some((e) => lu.includes(e))
}

async function resolveUrl() {
  finalUrl = ''
  via.value = ''
  hint.value = ''
  levels.value = []
  levelIdx = -1
  errMsg.value = ''

  if (isLive) {
    const url = await pickPlayableLive()
    if (!url) {
      errMsg.value = '这一组频道都打不开。可以在直播页换一个分组，或去设置页换一个直播源。'
      return
    }
    finalUrl = url
    via.value = '直播直连'
    return
  }

  if (directMode) {
    finalUrl = rawUrl
    via.value = '直连'
    return
  }

  // 补全 m3u8
  const guess = fixPlayable(rawUrl)
  if (guess !== rawUrl) {
    try {
      const ok = await api.peek(guess)
      if (ok.alive) {
        finalUrl = guess
        via.value = '补全 m3u8'
        return
      }
    } catch {
      /* ignore */
    }
  }
  try {
    const r = await api.resolve(rawUrl)
    via.value = r.via
    finalUrl = fixPlayable(r.url)
    if (!r.ok) hint.value = '未能解析出真实播放地址（' + r.via + '），仍尝试直接播放'
  } catch {
    finalUrl = rawUrl
    via.value = '直链'
  }
}

async function probeLive(u: string): Promise<boolean> {
  try {
    const r = await api.peek(u)
    return r.alive
  } catch {
    return false
  }
}

async function pickPlayableLive(): Promise<string> {
  let skip = 0
  let tries = 0
  // 直播探活最多重试 4 次（含备用线路），每次 3-4 秒，合计约 12-16 秒
  // 超过上限后显示提示让用户手动换源，而不是无限等待
  const MAX_TRIES = 4
  while (skip <= LIVE_SKIP_MAX && tries < MAX_TRIES) {
    tries++
    if (await probeLive(rawUrl)) {
      liveFail = 0
      return rawUrl
    }
    // 同频道备用线路
    const ep = player.current
    if (ep) {
      for (const alt of ep.urls) {
        if (alt === rawUrl) continue
        tries++
        if (await probeLive(alt)) {
          rawUrl = alt
          liveFail = 0
          app.showTip('已切到该频道的备用线路')
          return alt
        }
      }
    }
    if (skip >= LIVE_SKIP_MAX || !player.hasNext) return ''
    const next = player.step(1)
    if (!next) return ''
    skip++
    liveFail = skip
    rawUrl = next.url
    title.value = next.name
    syncEpLabel()
    app.showTip('跳过失效频道 → ' + next.name)
  }
  return ''
}

// ---------- 播放器 ----------

// 浏览器 <video>/MSE 不支持这些容器：即使里面的 H.264/AAC 能解也播不了，
// 必须由服务端转封装成 fMP4（/api/remux）。
const REMUX_EXTS = ['.mkv', '.avi', '.wmv', '.rmvb', '.rm', '.mov', '.ts', '.m2ts', '.mpg', '.mpeg', '.vob']

/** 从 URL 取容器扩展名（忽略 query/hash） */
function containerExt(u: string): string {
  const clean = u.split('?')[0].split('#')[0].toLowerCase()
  const dot = clean.lastIndexOf('.')
  return dot >= 0 ? clean.slice(dot) : ''
}

function needsRemux(u: string): boolean {
  const ext = containerExt(u)
  // 无扩展名（网盘常见，如「异星战境 Atlas」）无法从名字判断，先交给浏览器试；
  // 真失败时 onPlayError 会走「尝试转封装」的兜底。
  return ext !== '' && REMUX_EXTS.includes(ext)
}

function proxiedFinal(): string {
  const auth = player.header?.auth || undefined
  // MKV 之类：服务端转封装。输出是分片 fMP4 流、不支持 Range，
  // 拖进度条时会用 seek 参数重新起转（见 reloadAt）。
  if (needsRemux(finalUrl) && app.status?.remux) {
    remuxMode.value = true
    return remuxed(finalUrl, auth, remuxSeek.value)
  }
  remuxMode.value = false
  // 认证信息与「是否直连」无关：只要上游需要 Basic Auth（WebDAV 网盘就是），
  // 无论直连还是走 /api/proxy 都必须带上。
  // 之前写成 directMode ? auth : undefined —— 网盘走的是普通代理模式，
  // 代理拿不到认证 → 上游 401 → <video> 触发 error → 被误报成「视频解码失败」。
  return proxied(finalUrl, auth)
}

async function initPlayer() {
  if (!finalUrl || !videoEl.value) return
  const vid = videoEl.value
  const src = proxiedFinal()
  if (src.includes('.m3u8') || via.value === '直播直连' || via.value === '补全 m3u8') {
    const HlsCls = await loadHls()
    if (HlsCls.isSupported()) {
      // 缓冲策略：直播追求低延迟用小缓冲 + 限制最大缓冲防内存膨胀；
      // 点播用较大缓冲保证网络抖动时的流畅度。
      // maxBufferLength 默认 30s（1080p 会占 100MB+ 内存），这里收紧到 10s。
      const hlsCfg: any = isLive
        ? { maxBufferLength: 10, maxMaxBufferLength: 30 } // 直播：最多缓冲 30s
        : { maxBufferLength: 30 }                         // 点播：缓冲 30s 保流畅
      const h = new HlsCls(hlsCfg)
      hls = h
      h.loadSource(src)
      h.attachMedia(vid)
      h.on(HlsCls.Events.MANIFEST_PARSED, () => {
        if (h.levels && h.levels.length > 1) {
          levels.value = fromHlsLevels(h.levels)
          applyInitialLevel()
        }
        // 尝试从 m3u8 中加载字幕轨道
        loadSubtitlesFromHls(h)
      })
      h.on(HlsCls.Events.ERROR, (_e: any, data: any) => {
        if (!data.fatal) return
        if (hlsRecover < 3) {
          hlsRecover++
          if (data.type === HlsCls.ErrorTypes.NETWORK_ERROR) {
            app.showTip('网络抖动，正在恢复…')
            h.startLoad()
            return
          }
          if (data.type === HlsCls.ErrorTypes.MEDIA_ERROR) {
            app.showTip('解码异常，正在恢复…')
            h.recoverMediaError()
            return
          }
        }
        onPlayError(data.details || data.type)
      })
    } else if (vid.canPlayType('application/vnd.apple.mpegurl')) {
      vid.src = src
    }
  } else {
    vid.src = src
  }
  bindVideoEvents(vid)
  // 应用倍速偏好（已从 prefs 读取）
  if (playbackRate.value !== 1) {
    vid.playbackRate = playbackRate.value
    if (hls) hls.config.playbackRate = playbackRate.value
  }
  vid.play().catch(() => {})
}

function bindVideoEvents(vid: HTMLVideoElement) {
  vid.onloadedmetadata = () => {
    if (isLive) {
      durMs.value = 0
    } else if (remuxMode.value) {
      // 转封装流从 seek 点起转，服务端只知剩余时长，补回起点才是整片时长
      durMs.value = (remuxSeek.value + vid.duration) * 1000
    } else {
      durMs.value = vid.duration * 1000
    }
    prepared.value = true
    buffering.value = false
    playing.value = true
    popOsd()
    // 网盘续播：起播后 seek 到上次进度。
    // 转封装模式下走 applySeek（要重新请求），不能直接改 currentTime。
    // player.seek 先清零再调用，避免重新加载后 loadedmetadata 再次触发造成循环。
    if (player.seek > 0 && !isLive) {
      const t = player.seek / 1000
      player.seek = 0
      app.showTip('已从上次位置继续播放')
      applySeek(t)
    }
    if (introSkipSec.value > 0 && !introDone && !isLive) {
      const skipMs = introSkipSec.value * 1000
      if (durMs.value <= 0 || skipMs < durMs.value - 30000) showSkipBtn('intro', 5)
    }
  }
  vid.ontimeupdate = () => {
    // 转封装模式下服务端从 seek 点起转，视频时间戳从 0 开始，
    // 这里把跳转起点加回去，进度条与时间显示才是真实位置。
    curMs.value = (remuxMode.value ? remuxSeek.value : 0) * 1000 + vid.currentTime * 1000
    // 进度保存：节流（5 秒）+ 防抖（300ms），保证最后一次 seek 后一定保存
    if (curMs.value - lastSaveMs > 5000) {
      lastSaveMs = curMs.value
      if (saveTimer) clearTimeout(saveTimer)
      saveTimer = window.setTimeout(() => { savePos(); saveTimer = 0 }, 300)
    }
    if (!isLive && outroSkipSec.value > 0 && durMs.value > 0 && !outroFired && curMs.value >= durMs.value - outroSkipSec.value * 1000) {
      outroFired = true
      showSkipBtn('outro', 8)
    }
  }
  vid.onwaiting = () => {
    buffering.value = true
  }
  vid.onplaying = () => {
    buffering.value = false
    playing.value = true
    hlsRecover = 0
  }
  vid.onpause = () => {
    playing.value = false
    buffering.value = false
  }
  vid.onended = () => {
    playing.value = false
    if (player.hasNext && !isLive && autoPlayNext.value) {
      // 自动连播：3 秒倒计时后切下一集
      let countdown = 3
      hint.value = `即将播放下一集（${countdown}s）`
      if (autoplayTimer) clearInterval(autoplayTimer)
      autoplayTimer = window.setInterval(() => {
        countdown--
        if (countdown > 0) {
          hint.value = `即将播放下一集（${countdown}s）`
        } else {
          clearInterval(autoplayTimer)
          autoplayTimer = 0
          hint.value = ''
          switchEpisode(1)
        }
      }, 1000)
    } else {
      showOsd.value = true
      if (!player.hasNext && !isLive) {
        app.showTip('播放列表已全部完成')
      }
    }
  }
  vid.onerror = () => {
    // 区分真正的解码失败与「上游返回了错误页」：
    // <video> 对 401/403/404 的响应体同样会抛 MEDIA_ERR_SRC_NOT_SUPPORTED，
    // 一律说成「解码失败」会把人往编码格式上带偏（实际是认证或地址问题）。
    const code = vid.error?.code
    const msg = vid.error?.message || ''
    if (code === MediaError.MEDIA_ERR_NETWORK) {
      onPlayError('networkError')
      return
    }
    if (code === MediaError.MEDIA_ERR_DECODE) {
      onPlayError('mediaError')
      return
    }
    // MEDIA_ERR_SRC_NOT_SUPPORTED：地址不通/需要认证/格式不支持，三种都可能
    onPlayError(/401|403/.test(msg) ? 'authError' : 'srcNotSupported')
  }
  vid.onenterpictureinpicture = () => { isPip.value = true }
  vid.onleavepictureinpicture = () => { isPip.value = false }
}

function onPlayError(errKind?: string) {
  if (isLive && liveFail < 3 && player.hasNext) {
    liveFail++
    app.showTip('该频道无法播放，自动换下一个')
    switchEpisode(1)
    return
  }
  // 按错误类型给出友好提示
  // 容器不支持（MKV 等）且服务端没有 ffmpeg：给出可操作的指引，
  // 而不是让人以为文件坏了。这是网盘播放最常见的失败原因。
  const isContainerIssue = needsRemux(finalUrl) || /\.(mkv|avi|wmv|rmvb|rm|ts|mov|m2ts|mpg|mpeg|vob)\b/i.test(finalUrl)
  if (isContainerIssue && !app.status?.remux) {
    errMsg.value =
      (app.status?.remuxReason || '服务端缺少 ffmpeg，无法转换该视频容器') +
      '。装好后重启服务即可播放，画质不受影响（只换容器不重新编码）。'
    showOsd.value = true
    return
  }

  const tips: Record<string, string> = {
    networkError: '网络连接失败，请检查网络后点击重试',
    mediaError: '视频解码失败，可能是编码格式不支持',
    manifestLoadError: '视频地址失效或已下线',
    levelLoadError: '视频分片加载失败，源站可能不稳定',
    fragLoadError: '视频分片加载中断，可能是网络波动',
    manifestIncompatibleCodecsError: '编码格式不支持当前浏览器',
    authError: '资源需要认证或没有访问权限，请检查网盘账号密码',
    srcNotSupported: needsRemux(finalUrl)
      ? '该容器转换失败。可能是源不是 H.264/AAC（如 HEVC/AC3），无法无损转封装'
      : '这个文件无法直接播放。可能是编码格式浏览器不支持，或地址已失效',
  }
  errMsg.value = tips[errKind || ''] || '播放失败，可能是链接失效或编码不支持'
  showOsd.value = true
}

function applyInitialLevel() {
  if (!hls || !levels.value.length) return
  if (qCap > 0) {
    const i = pickByCap(qCap, levels.value)
    levelIdx = i
    hls.currentLevel = i
  } else {
    // 自动：起播取 720P 附近
    let best = -1
    levels.value.forEach((l, i) => {
      if (l.height > 0 && l.height <= 720) best = i
    })
    if (best >= 0) {
      levelIdx = best
      hls.currentLevel = best
    }
  }
  syncQLabel()
}

function pickByCap(cap: number, lv: QLevel[]): number {
  if (!lv.length) return -1
  if (cap >= TOP) return lv.length - 1
  let best = -1
  lv.forEach((l, i) => {
    if (l.height <= 0 || l.height <= cap) best = i
  })
  return best < 0 ? 0 : best
}

function syncQLabel() {
  if (levelIdx >= 0 && levelIdx < levels.value.length) {
    const name = labelOfLevel(levels.value[levelIdx])
    if (levels.value.length <= 1) {
      qLabel.value = '自动'
      return
    }
    qLabel.value = qCap > 0 ? name : '自动 ' + name
    return
  }
  if (qCap >= TOP) qLabel.value = '原画'
  else if (qCap > 0) qLabel.value = labelOf(qCap, 0)
  else qLabel.value = '自动'
}

function chooseLevel(cap: number) {
  qCap = cap
  api.setPrefs({ qPref: cap }).catch(() => {})
  showQ.value = false
  if (hls && levels.value.length > 1) {
    const next = cap > 0 ? pickByCap(cap, levels.value) : -1
    if (cap === 0) {
      hls.currentLevel = -1
      levelIdx = -1
    } else {
      levelIdx = next
      hls.currentLevel = next
    }
  }
  syncQLabel()
}

// 画面比例
const scaleFit = computed(() => {
  if (scaleMode.value === 0) return 'fill'
  if (scaleMode.value === 1) return 'cover'
  return 'contain'
})

const scaleNames = ['拉伸铺满', '裁剪铺满（等比不变形）', '等比适应（留黑边）']

function setScale(mode: number) {
  scaleMode.value = mode
  app.showTip('画面比例：' + scaleNames[mode])
}

// 跳过片头/片尾档位设置
const SKIP_PRESETS = [0, 15, 30, 60, 90]

function setIntroSkip(sec: number) {
  introSkipSec.value = sec
  api.setPrefs({ introSkip: sec }).catch(() => {})
  if (sec > 0) app.showTip(`已设片头跳过 ${fmtSec(sec)}，起播自动生效`)
  else app.showTip('已关闭片头跳过')
}

function setOutroSkip(sec: number) {
  outroSkipSec.value = sec
  api.setPrefs({ outroSkip: sec }).catch(() => {})
  if (sec > 0) app.showTip(`已设片尾跳过 ${fmtSec(sec)}，播到片尾自动切`)
  else app.showTip('已关闭片尾跳过')
}

// 标当前：片头=当前播放位置，片尾=距结尾距离
function markSkip(kind: 'intro' | 'outro') {
  const vid = videoEl.value
  if (!vid || !prepared.value || isLive) return
  if (kind === 'intro') {
    const t = Math.floor(vid.currentTime)
    if (t <= 0) {
      app.showTip('当前位置是片头起点，无法标记')
      return
    }
    introSkipSec.value = t
    api.setPrefs({ introSkip: t }).catch(() => {})
    app.showTip(`已标记片头结束点 ${fmtSec(t)}，下次起播跳到这里`)
  } else {
    if (durMs.value <= 0) return
    const remain = Math.floor((durMs.value - vid.currentTime * 1000) / 1000)
    if (remain <= 0) {
      app.showTip('已到片尾，无法标记')
      return
    }
    outroSkipSec.value = remain
    api.setPrefs({ outroSkip: remain }).catch(() => {})
    app.showTip(`已标记片尾起点（距结尾 ${fmtSec(remain)}），播到这里自动切`)
  }
}

// ---------- 播放控制 ----------

function togglePlay() {
  const vid = videoEl.value
  if (!vid || !prepared.value) return
  if (vid.paused) {
    vid.play()
    flashPlay('play')
    popOsd()
  } else {
    vid.pause()
    flashPlay('pause')
    showOsd.value = true
  }
}

// 点画面：控制条收着时先唤出，再点才切播放/暂停。
function onSurfaceClick() {
  if (!showOsd.value) {
    popOsd()
    return
  }
  togglePlay()
}

function onMouseMove() {
  if (!showOsd.value && prepared.value) popOsd()
}

// ---------- 进度条拖动定位 ----------

const dragging = ref(false)
const previewMs = ref(-1)
// 进度条 DOM 引用：拖拽时用它算比例（window 监听拿不到 currentTarget）。
// 用函数式 ref 而不是 ref="xxx"：桌面上有一个进度条，移动端模板里有两处，
// 同名 ref 只会保留最后一个，容易取错元素。
const progressEl = ref<HTMLElement>()
function setProgressEl(el: any) {
  if (el instanceof HTMLElement) progressEl.value = el
}
// ?debug=1 打开拖拽诊断日志
const DEBUG = route.query.debug === '1'

function ratioFromEvent(e: PointerEvent): number {
  const el = e.currentTarget as HTMLElement
  const rect = el.getBoundingClientRect()
  return Math.min(1, Math.max(0, (e.clientX - rect.left) / rect.width))
}

function onProgressDown(e: PointerEvent) {
  if (DEBUG) console.log('[seek] down', { prepared: prepared.value, dur: durMs.value, live: isLive })
  if (!prepared.value || durMs.value <= 0 || isLive) return
  e.preventDefault()
  dragging.value = true
  previewMs.value = durMs.value * ratioFromEvent(e)

  // move/up 绑到 window 而不是进度条自己：
  // 进度条只有 ~26px 高，拖快一点指针就跑出去了。虽然 setPointerCapture
  // 理论上能兜住，但元素被重渲染、或在某些浏览器里捕获失败时，
  // 一旦移出元素就再也收不到 pointermove/pointerup，表现为「拖不动」。
  // 绑 window 后无论指针移到哪里都能跟住，这是最稳的做法。
  window.addEventListener('pointermove', onProgressMove, { passive: true })
  window.addEventListener('pointerup', onProgressUp, { passive: true })
  window.addEventListener('pointercancel', onProgressCancel, { passive: true })
}

function ratioFromPoint(clientX: number, el: HTMLElement): number {
  const rect = el.getBoundingClientRect()
  if (rect.width <= 0) return 0
  return Math.min(1, Math.max(0, (clientX - rect.left) / rect.width))
}

function onProgressMove(e: PointerEvent) {
  if (!dragging.value) return
  const el = progressEl.value
  if (!el) return
  previewMs.value = durMs.value * ratioFromPoint(e.clientX, el)
}

function onProgressCancel() {
  // 被系统手势/来电等打断：撤销拖动，不改播放位置
  if (!dragging.value) return
  dragging.value = false
  previewMs.value = -1
  detachProgressListeners()
}

function detachProgressListeners() {
  window.removeEventListener('pointermove', onProgressMove)
  window.removeEventListener('pointerup', onProgressUp)
  window.removeEventListener('pointercancel', onProgressCancel)
}

function onProgressUp(e: PointerEvent) {
  if (DEBUG) console.log('[seek] up', { dragging: dragging.value, preview: previewMs.value })
  if (!dragging.value) return
  dragging.value = false
  detachProgressListeners()

  const t = previewMs.value / 1000
  if (videoEl.value && t >= 0 && durMs.value > 0) {
    // 统一走 applySeek：普通流直接改 currentTime，转封装流要带 seek 重新起转。
    applySeek(t)
    curMs.value = previewMs.value

    // 三层防抖：只在 5s 间隔后才真正落盘进度记录
    clearTimeout(seekPendingTimer)
    const now = Date.now()
    const timeSinceLastSeek = now - lastSeekMs

    if (timeSinceLastSeek >= SEEK_THROTTLE) {
      // 距上次 seek 已超过 5s，直接落盘
      lastSeekMs = now
      savePos()
    } else {
      // 在 5s 范围内，等待 300ms 后若无新 seek 才落盘
      seekPendingTimer = window.setTimeout(() => {
        const elapsed = Date.now() - lastSeekMs
        if (elapsed >= SEEK_THROTTLE) {
          lastSeekMs = Date.now()
          savePos()
        }
      }, SEEK_DEBOUNCE)
    }
  }
  const target = fmtMs(previewMs.value)
  previewMs.value = -1
  app.showTip('已定位到 ' + target)
  popOsd()
}

// ---------- 音量滑块 ----------

const volDragging = ref(false)
let volSaveTimer = 0

// 音量变化后防抖落盘，避免拖动时每帧都请求后端
function persistVolume() {
  if (volSaveTimer) clearTimeout(volSaveTimer)
  volSaveTimer = window.setTimeout(() => {
    api.setPrefs({ volumePref: Math.round(volume.value * 100) / 100 }).catch(() => {})
  }, 500)
}

function setVolumeFromEvent(e: PointerEvent) {
  const el = e.currentTarget as HTMLElement
  const rect = el.getBoundingClientRect()
  let r = (e.clientX - rect.left) / rect.width
  r = Math.min(1, Math.max(0, r))
  volume.value = r
  if (r > 0) muted.value = false
  if (videoEl.value) videoEl.value.volume = muted.value ? 0 : r
  persistVolume()
  showVol.value = true
  if (volTimer) clearTimeout(volTimer)
  volTimer = window.setTimeout(() => (showVol.value = false), 1600)
}

function onVolDown(e: PointerEvent) {
  volDragging.value = true
  setVolumeFromEvent(e)
  ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
}

function onVolMove(e: PointerEvent) {
  if (volDragging.value) setVolumeFromEvent(e)
}

function onVolUp() {
  volDragging.value = false
}

function seekBy(dir: number) {
  const vid = videoEl.value
  if (!vid || !prepared.value || isLive) return
  let t = vid.currentTime + (dir * SEEK_STEP) / 1000
  if (t < 0) t = 0
  if (durMs.value > 0 && t > durMs.value / 1000 - 1) t = durMs.value / 1000 - 1
  applySeek(t)
}

/**
 * 执行一次跳转。
 *
 * 转封装流（/api/remux）是服务端线性吐出来的分片 MP4，体积索引不完整，
 * 直接改 video.currentTime 浏览器会原地重试甚至报错。这里改为带 seek 参数
 * 重新请求，让 ffmpeg 从目标位置重新转封装。
 */
let seekingRemux = false

function applySeek(t: number) {
  const vid = videoEl.value
  if (!vid) return
  if (remuxMode.value) {
    // 防重入：重新设置 src 会再触发 loadedmetadata，
    // 若此时还有未消化的 player.seek 就会反复重载。
    if (seekingRemux) return
    seekingRemux = true
    window.setTimeout(() => (seekingRemux = false), 1500)
    remuxSeek.value = Math.max(0, Math.floor(t))
    const wasPlaying = playing.value
    const src = proxiedFinal()
    vid.src = src
    curMs.value = t * 1000
    vid.addEventListener('loadedmetadata', () => {
      if (wasPlaying) vid.play().catch(() => {})
    }, { once: true })
    popOsd()
    app.showTip('正在跳转…')
    return
  }
  vid.currentTime = t
  curMs.value = t * 1000
  popOsd()
}

// ===== 倍速播放 =====

function setPlaybackRate(rate: number) {
  const vid = videoEl.value
  if (!vid || !prepared.value) return
  if (!PLAYBACK_RATES.includes(rate)) return
  playbackRate.value = rate
  vid.playbackRate = rate
  // 同步到 HLS 配置（防止切换线路时重置）
  if (hls) hls.config.playbackRate = rate
  // 持久化倍速偏好
  api.setPrefs({ speedPref: rate }).catch(() => {})
  app.showTip(`${rate}x 倍速播放`)
}

// 循环切换倍速（0.5x → 0.75x → 1x → 1.25x → 1.5x → 2x → 0.5x）
function togglePlaySpeed() {
  const idx = PLAYBACK_RATES.indexOf(playbackRate.value)
  const nextIdx = (idx + 1) % PLAYBACK_RATES.length
  setPlaybackRate(PLAYBACK_RATES[nextIdx])
}

// ===== 移动端手势（usePlayerGestures 组合式函数）=====
const { gestureHint, brightness, onTouchStart, onTouchMove, onTouchEnd, disposeGestures } =
  usePlayerGestures({
    videoEl,
    volume,
    muted,
    curMs,
    durMs,
    showOsd,
    isLive: () => isLive,
    seekBy,
    togglePlay,
    flashPlay,
    popOsd,
    persistVolume,
    tip: (m: string) => app.showTip(m)
  })

function seekToRatio(ratio: number) {
  const vid = videoEl.value
  if (!vid || !prepared.value || durMs.value <= 0) return
  const r = Math.min(1, Math.max(0, ratio))
  applySeek((durMs.value * r) / 1000)
}

function changeVolume(delta: number) {
  let v = volume.value + delta
  v = Math.min(1, Math.max(0, v))
  volume.value = v
  if (v > 0) muted.value = false
  if (videoEl.value) videoEl.value.volume = muted.value ? 0 : v
  persistVolume()
  showVol.value = true
  if (volTimer) clearTimeout(volTimer)
  volTimer = window.setTimeout(() => (showVol.value = false), 1600)
}

function toggleMute() {
  muted.value = !muted.value
  if (videoEl.value) videoEl.value.volume = muted.value ? 0 : volume.value
  showVol.value = true
  if (volTimer) clearTimeout(volTimer)
  volTimer = window.setTimeout(() => (showVol.value = false), 1600)
}

// ===== 选集抽屉 =====
const showEps = ref(false)
let autoplayTimer = 0
let castHandlers: { onConnect: () => void; onDisconnect: () => void } | null = null

async function jumpToEpisode(i: number) {
  if (i === player.idx) { showEps.value = false; return }
  await switchEpisode(i - player.idx)
  showEps.value = false
}

async function switchEpisode(delta: number) {
  if (!player.count) {
    app.showTip(isLive ? '当前分组没有其他频道' : '当前没有可切换的剧集')
    return
  }
  savePos()
  // 清除进度条拖动状态，避免切集/切台后进度条显示混乱
  dragging.value = false
  previewMs.value = -1
  const ep = player.step(delta)
  if (!ep) {
    app.showTip(delta > 0 ? '已经是最后一个了' : '已经是第一个了')
    return
  }
  destroyPlayer()
  curMs.value = 0
  durMs.value = 0
  prepared.value = false
  playing.value = false
  introDone = false
  outroFired = false
  skipShow.value = ''
  rawUrl = ep.url
  if (isLive) {
    title.value = ep.name
    app.showTip('换台：' + ep.name)
  } else {
    title.value = player.vodTitle ? `${player.vodTitle} ${ep.name}` : ep.name
    app.showTip(`第 ${player.idx + 1} 集  ${ep.name}`)
  }
  syncEpLabel()
  resolving.value = true
  await resolveUrl()
  resolving.value = false
  initPlayer()
}

function goBack() {
  savePos()
  if (window.history.length > 1) {
    router.back()
  } else {
    router.replace('/')
  }
}

// 手动切换直播线路（同一频道的多线路）
async function switchLine(url: string) {
  if (url === rawUrl) {
    app.showTip('已经是当前线路')
    return
  }
  destroyPlayer()
  curMs.value = 0
  durMs.value = 0
  prepared.value = false
  playing.value = false
  introDone = false
  outroFired = false
  skipShow.value = ''
  liveFail = 0
  rawUrl = url
  app.showTip('切换线路中…')
  resolving.value = true
  await resolveUrl()
  resolving.value = false
  initPlayer()
}

function retry() {
  destroyPlayer()
  liveFail = 0
  prepared.value = false
  playing.value = false
  curMs.value = 0
  durMs.value = 0
  errMsg.value = ''
  resolving.value = true
  resolveUrl().then(() => {
    resolving.value = false
    initPlayer()
  })
}

function destroyPlayer() {
  if (hls) {
    hls.destroy()
    hls = null
  }
  if (videoEl.value) {
    videoEl.value.pause()
    videoEl.value.removeAttribute('src')
    videoEl.value.load()
  }
  if (skipTimer) clearInterval(skipTimer)
  if (flashTimer) clearTimeout(flashTimer)
  if (autoplayTimer) { clearInterval(autoplayTimer); autoplayTimer = 0 }
  playFlash.value = ''
}

function savePos() {
  if (isLive) return
  if (curMs.value <= 0) return
  // 网盘播放：保存续播记录（网盘页「继续观看」）
  if (siteKey === 'webdav') {
    if (!player.wdVod) return
    api
      .updateWdHistory({
        srcId: player.wdSrcId,
        srcName: player.wdSrcName,
        vodName: player.wdVod,
        dir: player.wdDir,
        epIndex: player.idx,
        epName: player.current?.name || '',
        url: player.current?.url || '',
        pos: Math.floor(curMs.value),
        dur: durMs.value > 0 ? Math.floor(durMs.value) : 0,
        time: Date.now()
      })
      .catch(() => {})
    return
  }
  if (!siteKey || !vodId) return
  api
    .updateHistoryPos({
      siteKey,
      siteName: vodSiteName,
      vodId,
      name: vodName,
      pic: vodPic,
      epName: player.current?.name || '',
      epIndex: player.idx,
      from: '',
      pos: Math.floor(curMs.value),
      dur: durMs.value > 0 ? Math.floor(durMs.value) : 0,
      time: Date.now()
    })
    .catch(() => {})
}

// ---------- 提示 ----------

function popOsd() {
  curMs.value = videoEl.value?.currentTime ? videoEl.value.currentTime * 1000 : curMs.value
  showOsd.value = true
  if (osdTimer) clearTimeout(osdTimer)
  osdTimer = window.setTimeout(() => {
    if (playing.value) showOsd.value = false
  }, 12000)
}

function flashPlay(kind: string) {
  playFlash.value = kind
  if (flashTimer) clearTimeout(flashTimer)
  flashTimer = window.setTimeout(() => (playFlash.value = ''), 700)
}

// ---------- 跳过片头片尾 ----------

function showSkipBtn(kind: string, totalSec: number) {
  skipShow.value = kind
  skipLeft.value = totalSec
  if (skipTimer) clearInterval(skipTimer)
  skipTimer = window.setInterval(() => {
    skipLeft.value -= 0.1
    if (skipLeft.value <= 0) {
      clearInterval(skipTimer)
      doSkip(kind)
    }
  }, 100)
}

function doSkip(kind: string) {
  if (skipShow.value !== kind) return
  skipShow.value = ''
  if (kind === 'intro') {
    introDone = true
    const skipMs = introSkipSec.value * 1000
    if (videoEl.value && prepared.value && skipMs > 0) {
      // 同样要走 applySeek：转封装流直接改 currentTime 无效
      applySeek(skipMs / 1000)
      curMs.value = skipMs
    }
    app.showTip('已跳过片头 ' + fmtSec(introSkipSec.value))
  } else {
    outroFired = true
    if (player.hasNext) {
      app.showTip('片尾，跳下一集')
      switchEpisode(1)
    } else {
      app.showTip('已跳过片尾')
    }
  }
}

function fmtSec(sec: number): string {
  const m = Math.floor(sec / 60)
  const s = sec % 60
  if (m > 0 && s > 0) return m + '分' + s + '秒'
  if (m > 0) return m + '分钟'
  return s + '秒'
}

// 键盘
function onKey(e: KeyboardEvent) {
  if (e.key === ' ' || e.key === 'Enter') {
    e.preventDefault()
    if (!showOsd.value) popOsd()
    else togglePlay()
  } else if (e.key === 'ArrowLeft') {
    if (isLive) switchEpisode(-1)
    else seekBy(-1)
  } else if (e.key === 'ArrowRight') {
    if (isLive) switchEpisode(1)
    else seekBy(1)
  } else if (e.key === 'ArrowUp') changeVolume(0.05)
  else if (e.key === 'ArrowDown') changeVolume(-0.05)
  else if (e.key === 'm') toggleMute()
  else if (e.key === 'f' || e.key === 'F') toggleFullscreen()
  else if (e.key === 'p' || e.key === 'P') togglePip()
}

onMounted(() => {
  window.addEventListener('keydown', onKey)
  document.addEventListener('fullscreenchange', onFullscreenChange)
  document.addEventListener('webkitfullscreenchange', onFullscreenChange)
  document.addEventListener('mozfullscreenchange', onFullscreenChange)
  document.addEventListener('MSFullscreenChange', onFullscreenChange)
  checkMobile()
  window.addEventListener('resize', checkMobile)
})
onUnmounted(() => {
  window.removeEventListener('keydown', onKey)
  document.removeEventListener('fullscreenchange', onFullscreenChange)
  document.removeEventListener('webkitfullscreenchange', onFullscreenChange)
  document.removeEventListener('mozfullscreenchange', onFullscreenChange)
  document.removeEventListener('MSFullscreenChange', onFullscreenChange)
  window.removeEventListener('resize', checkMobile)
})
</script>

<template>
  <!-- 桌面端：全屏固定播放器 -->
  <div v-if="!isMobile" ref="playerEl" class="player" @mousemove="onMouseMove" @click="onSurfaceClick" @dblclick="toggleFullscreen">
    <video ref="videoEl" class="video" :style="{ objectFit: scaleFit }"></video>

    <!-- 中央播放/暂停图标 -->
    <transition name="flash">
      <div v-if="playFlash" class="play-flash">{{ playFlash === 'play' ? '▶' : '❚❚' }}</div>
    </transition>

    <!-- 加载/缓冲 -->
    <div v-if="resolving" class="center-tip">正在解析播放地址…</div>
    <div v-else-if="buffering" class="center-tip">缓冲中…</div>
    <div v-else-if="errMsg" class="center-tip err">
      <p>{{ errMsg }}</p>
      <div class="err-actions">
        <el-button @click="retry">重试</el-button>
        <el-button @click="goBack">返回</el-button>
      </div>
    </div>

    <!-- 跳过片头/片尾按钮 -->
    <div v-if="skipShow" class="skip-btn" @click="doSkip(skipShow)">
      <span class="ring" :style="{ '--p': skipLeft }"></span>
      {{ skipShow === 'intro' ? '跳过片头' : player.hasNext ? '下一集' : '跳过片尾' }}
    </div>

    <!-- 桌面端右上角画中画按钮 -->
  <button
    v-if="prepared && supportsPip()"
    class="pip-btn"
    :title="isPip ? '退出画中画 (P)' : '画中画 (P)'"
    @click="togglePip"
  >
    <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <rect x="2" y="3" width="20" height="14" rx="2"/>
      <rect x="12" y="10" width="9" height="7" rx="1"/>
    </svg>
  </button>

  <!-- 控制条 -->
    <div v-if="showOsd && prepared" class="osd" @click.stop>
      <div class="osd-top">
        <button class="back-btn" @click="goBack">
          <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 18l-6-6 6-6" /></svg>
          <span>返回</span>
        </button>
        <span class="osd-title">{{ title }}</span>
        <span class="osd-meta">{{ epLabel }} {{ via ? '· ' + via : '' }}</span>
      </div>
      <div :ref="setProgressEl" class="osd-progress" @pointerdown="onProgressDown">
        <div class="progress-track">
          <div class="progress-fill" :style="{ width: (dragging ? (previewMs / durMs) * 100 : progressPct) + '%' }"></div>
        </div>
      </div>
      <div v-if="dragging" class="seek-hint">{{ fmtMs(previewMs) }} / {{ fmtMs(durMs) }}</div>
      <div class="osd-bottom">
        <span class="time">{{ dragging ? fmtMs(previewMs) : fmtMs(curMs) }}</span>
        <div class="controls">
          <button class="ctl" @click="switchEpisode(-1)">{{ isLive ? '上一台' : '上一集' }}</button>
          <button class="ctl" @click="seekBy(-1)">−10 秒</button>
          <button class="ctl primary" @click="togglePlay">{{ playPauseText }}</button>
          <button class="ctl" @click="seekBy(1)">+10 秒</button>
          <button class="ctl" @click="toggleMute">{{ muteText }}</button>
          <div class="vol-slider" @pointerdown="onVolDown" @pointermove="onVolMove" @pointerup="onVolUp">
            <div class="vol-track"><div class="vol-fill" :style="{ width: (muted ? 0 : volume * 100) + '%' }"></div></div>
          </div>
          <button class="ctl" @click="showQ = !showQ">画质</button>
          <button v-if="!isLive && player.count > 1" class="ctl" @click="showEps = true">选集</button>
          <button v-if="supportsCast()" class="ctl" :class="{ primary: castConnected }" @click="toggleCast">{{ castConnected ? '断开投屏' : '投屏' }}</button>
          <button class="ctl" @click="switchEpisode(1)">{{ isLive ? '下一台' : '下一集' }}</button>
          <button class="ctl fullscreen-btn" :title="isFullscreen ? '退出全屏 (F)' : '全屏 (F)'" @click="toggleFullscreen">
            {{ isFullscreen ? '⤓' : '⛶' }}
          </button>
        </div>
        <span class="time">{{ durMs > 0 ? fmtMs(durMs) : (isLive ? '直播' : '') }}</span>
        <span v-if="showVol" class="vol">{{ volPct }}</span>
      </div>
    </div>

    <!-- 画质面板（桌面浮层，结构见 PlayerSettingsPanel） -->
    <PlayerSettingsPanel
      v-model="showQ"
      variant="desktop"
      :is-live="isLive"
      :levels="levels"
      :q-cap="qCap"
      :level-idx="levelIdx"
      :lines="player.current?.urls || []"
      :raw-url="rawUrl"
      :sub-delay="subDelay"
      :sub-font-size="subFontSize"
      :external-sub-active="externalSubActive"
      :sub-label="subLabel"
      :scale-names="scaleNames"
      :scale-mode="scaleMode"
      :intro-skip-sec="introSkipSec"
      :outro-skip-sec="outroSkipSec"
      @choose-level="chooseLevel"
      @switch-line="switchLine"
      @sub-file="onSubFile"
      @sub-url="promptSubUrl"
      @remove-sub="removeExternalSub"
      @sub-delay="setSubDelay"
      @sub-size="setSubFontSize"
      @scale="setScale"
      @intro-skip="setIntroSkip"
      @outro-skip="setOutroSkip"
      @mark-skip="markSkip"
    />
  </div>

  <!-- 移动端：视频 + 控制面板垂直流式布局 -->
  <div v-else ref="playerEl" class="m-player" :class="{ 'm-fullscreen': isFullscreen }">
    <!-- 视频区域 -->
    <div
      class="m-video-wrap"
      @click="onSurfaceClick"
      @dblclick="toggleFullscreen"
      @touchstart="onTouchStart"
      @touchmove="onTouchMove"
      @touchend="onTouchEnd"
    >
      <video ref="videoEl" class="video" :style="{ objectFit: scaleFit }"></video>

      <!-- 模拟亮度遮罩（左滑调节，覆盖在视频上方） -->
      <div class="brightness-mask" :style="{ opacity: brightness }"></div>

      <!-- 手势提示 -->
      <div v-if="gestureHint" class="m-gesture-hint">{{ gestureHint }}</div>

      <!-- 加载/缓冲（视频区内）-->
      <div v-if="resolving" class="m-center-tip">正在解析…</div>
      <div v-else-if="buffering" class="m-center-tip">缓冲中…</div>
      <div v-else-if="errMsg" class="m-center-tip m-err">
        <p>{{ errMsg }}</p>
        <div class="err-actions">
          <el-button size="small" @click="retry">重试</el-button>
          <el-button size="small" @click="goBack">返回</el-button>
        </div>
      </div>

      <!-- 播放/暂停动画 -->
      <transition name="flash">
        <div v-if="playFlash" class="play-flash" style="font-size:56px">{{ playFlash === 'play' ? '▶' : '❚❚' }}</div>
      </transition>

      <!-- 跳过按钮 -->
      <div v-if="skipShow" class="skip-btn" @click.stop="doSkip(skipShow)">
        <span class="ring"></span>
        {{ skipShow === 'intro' ? '跳过片头' : player.hasNext ? '下一集' : '跳过片尾' }}
      </div>

      <!-- 全屏时的浮动 OSD（覆盖在视频上方） -->
      <div v-if="isFullscreen && showOsd && prepared" class="m-osd-fullscreen" @click.stop>
        <div class="m-fs-top">
          <button class="m-back-btn" @click="goBack">
            <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M15 18l-6-6 6-6" /></svg>
          </button>
          <span class="m-fs-title">{{ title }}</span>
          <button class="m-icon-btn" @click="toggleFullscreen">⤓</button>
        </div>
        <div class="m-fs-bottom">
          <div :ref="setProgressEl" class="m-progress-wrap" @pointerdown="onProgressDown">
            <div class="m-progress-track">
              <div class="m-progress-fill" :style="{ width: (dragging ? (previewMs / durMs) * 100 : progressPct) + '%' }"></div>
            </div>
          </div>
          <div v-if="dragging" class="m-seek-hint">{{ fmtMs(previewMs) }}</div>
          <div class="m-fs-controls">
            <span class="m-time">{{ fmtMs(curMs) }}</span>
            <button class="m-ctl-icon" @click="seekBy(-1)">
              <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="11 17 2 12 11 7"/><line x1="22" y1="7" x2="22" y2="17"/><polyline points="22 17 13 12 22 7"/></svg>
            </button>
            <button class="m-play-btn" @click="togglePlay">
              <svg v-if="!playing" viewBox="0 0 24 24" width="32" height="32" fill="currentColor"><polygon points="5 3 19 12 5 21"/></svg>
              <svg v-else viewBox="0 0 24 24" width="32" height="32" fill="currentColor"><rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/></svg>
            </button>
            <button class="m-ctl-icon" @click="seekBy(1)">
              <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="13 17 22 12 13 7"/><line x1="2" y1="7" x2="2" y2="17"/><polyline points="2 17 11 12 2 7"/></svg>
            </button>
            <span class="m-time">{{ durMs > 0 ? fmtMs(durMs) : (isLive ? '直播' : '--:--') }}</span>
            <button class="m-icon-btn" style="margin-left:auto" @click="showQ = !showQ">⚙</button>
          </div>
        </div>
      </div>
    </div>

    <!-- 非全屏时：视频下方控制面板 -->
    <div v-if="!isFullscreen" class="m-panel">
      <!-- 标题栏 -->
      <div class="m-panel-title">
        <button class="m-back-btn" @click="goBack">
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M15 18l-6-6 6-6" /></svg>
        </button>
        <div class="m-title-info">
          <div class="m-title-name">{{ title }}</div>
          <div v-if="epLabel" class="m-title-meta">{{ epLabel }}<span v-if="via"> · {{ via }}</span></div>
        </div>
        <button class="m-icon-btn" @click="toggleFullscreen">⛶</button>
      </div>

      <!-- 进度条 -->
      <div v-if="!isLive" class="m-progress-row">
        <span class="m-time-sm">{{ fmtMs(curMs) }}</span>
        <div :ref="setProgressEl" class="m-progress-wrap" @pointerdown="onProgressDown">
          <div class="m-progress-track">
            <div class="m-progress-fill" :style="{ width: (dragging ? (previewMs / durMs) * 100 : progressPct) + '%' }"></div>
          </div>
        </div>
        <span class="m-time-sm">{{ durMs > 0 ? fmtMs(durMs) : '--:--' }}</span>
      </div>
      <div v-if="dragging" class="m-seek-hint">{{ fmtMs(previewMs) }}</div>

      <!-- 主控制按钮行 -->
      <div class="m-main-controls">
        <button class="m-ctl-icon" @click="switchEpisode(-1)">
          <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="11 17 2 12 11 7"/><line x1="22" y1="7" x2="22" y2="17"/><polyline points="22 17 13 12 22 7"/></svg>
        </button>
        <button class="m-ctl-icon" @click="seekBy(-1)">
          <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 1 0 .49-3.96"/></svg>
        </button>
        <button class="m-play-btn" @click="togglePlay">
          <svg v-if="!playing" viewBox="0 0 24 24" width="36" height="36" fill="currentColor"><polygon points="5 3 19 12 5 21"/></svg>
          <svg v-else viewBox="0 0 24 24" width="36" height="36" fill="currentColor"><rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/></svg>
        </button>
        <button class="m-ctl-icon" @click="seekBy(1)">
          <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-.49-3.96"/></svg>
        </button>
        <button class="m-ctl-icon" @click="switchEpisode(1)">
          <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="13 17 22 12 13 7"/><line x1="2" y1="7" x2="2" y2="17"/><polyline points="2 17 11 12 2 7"/></svg>
        </button>
      </div>

      <!-- 辅助按钮行 -->
      <div class="m-aux-controls">
        <button class="m-aux-btn" @click="toggleMute">
          <span class="m-aux-icon">{{ muted ? '🔇' : '🔊' }}</span>
          <span>{{ muted ? '已静音' : Math.round(volume * 100) + '%' }}</span>
        </button>
        <button v-if="!isLive && player.hasNext" class="m-aux-btn" @click="autoPlayNext = !autoPlayNext">
          <span class="m-aux-icon">{{ autoPlayNext ? '▶' : '⏸' }}</span>
          <span>{{ autoPlayNext ? '自动连播' : '手动切集' }}</span>
        </button>
        <button class="m-aux-btn" @click="showQ = !showQ">
          <span class="m-aux-icon">⚙</span>
          <span>画质</span>
        </button>
        <button v-if="!isLive && player.count > 1" class="m-aux-btn" @click="showEps = true">
          <span class="m-aux-icon">☷</span>
          <span>选集</span>
        </button>
        <button v-if="supportsCast()" class="m-aux-btn" @click="toggleCast">
          <span class="m-aux-icon">⧉</span>
          <span>{{ castConnected ? '断开投屏' : '投屏' }}</span>
        </button>
        <button v-if="supportsPip()" class="m-aux-btn" @click="togglePip">
          <span class="m-aux-icon">{{ isPip ? '⤦' : '⧉' }}</span>
          <span>{{ isPip ? '退出PiP' : '画中画' }}</span>
        </button>
        <button class="m-aux-btn" @click="toggleFullscreen">
          <span class="m-aux-icon">⛶</span>
          <span>全屏</span>
        </button>
      </div>
    </div>

    <!-- 画质面板（移动端底部 sheet，结构见 PlayerSettingsPanel） -->
    <PlayerSettingsPanel
      v-model="showQ"
      variant="mobile"
      :is-live="isLive"
      :levels="levels"
      :q-cap="qCap"
      :level-idx="levelIdx"
      :lines="player.current?.urls || []"
      :raw-url="rawUrl"
      :sub-delay="subDelay"
      :sub-font-size="subFontSize"
      :external-sub-active="externalSubActive"
      :sub-label="subLabel"
      :scale-names="scaleNames"
      :scale-mode="scaleMode"
      :intro-skip-sec="introSkipSec"
      :outro-skip-sec="outroSkipSec"
      @choose-level="chooseLevel"
      @switch-line="switchLine"
      @sub-file="onSubFile"
      @sub-url="promptSubUrl"
      @remove-sub="removeExternalSub"
      @sub-delay="setSubDelay"
      @sub-size="setSubFontSize"
      @scale="setScale"
      @intro-skip="setIntroSkip"
      @outro-skip="setOutroSkip"
      @mark-skip="markSkip"
    />

    <!-- 选集抽屉 -->
    <EpisodeDrawer
      v-model="showEps"
      :title="`选集 · ${player.vodTitle || ''}（${player.count} 集）`"
      :episodes="player.eps"
      :current-idx="player.idx"
      @select="jumpToEpisode"
    />
  </div>
</template>

<style scoped>
/* ===== 桌面端播放器 ===== */
.player {
  position: fixed;
  inset: 0;
  background: #000;
  overflow: hidden;
}
.video {
  width: 100%;
  height: 100%;
  object-fit: contain;
  display: block;
}
.play-flash {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  font-size: 90px;
  color: rgba(255, 255, 255, 0.9);
  pointer-events: none;
}
.flash-enter-active, .flash-leave-active { transition: opacity 0.3s; }
.flash-enter-from, .flash-leave-to { opacity: 0; }

.center-tip {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  color: #fff;
  font-size: 18px;
  text-align: center;
}
.center-tip.err p { margin-bottom: 12px; }
.err-actions { display: flex; justify-content: center; gap: 12px; }

.skip-btn {
  position: absolute;
  right: 48px;
  bottom: 96px;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 26px 12px 20px;
  background: rgba(24, 24, 24, 0.8);
  border: 1.5px solid rgba(255, 255, 255, 0.5);
  border-radius: 40px;
  color: #fff;
  font-size: 18px;
  cursor: pointer;
}
.ring {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  border: 2.5px solid rgba(255, 255, 255, 0.3);
  border-top-color: #fff;
  animation: spin 1s linear infinite;
}
@keyframes spin { to { transform: rotate(360deg); } }

.osd {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  padding: 24px 24px 16px;
  background: linear-gradient(transparent, rgba(0, 0, 0, 0.85));
}
.osd-top {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 10px;
}
.back-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 16px 8px 12px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.12);
  border: 1px solid rgba(255, 255, 255, 0.35);
  color: #fff;
  font-size: 15px;
  font-weight: 500;
  cursor: pointer;
  flex-shrink: 0;
  transition: background 0.15s, border-color 0.15s;
}
.back-btn:hover {
  background: rgba(255, 255, 255, 0.24);
  border-color: #fff;
}
.osd-title { font-size: 18px; font-weight: 600; }
.osd-meta { font-size: 13px; color: var(--sub); }

.osd-progress {
  padding: 10px 0;
  cursor: pointer;
  touch-action: none;
}
.progress-track {
  height: 6px;
  border-radius: 3px;
  background: rgba(255, 255, 255, 0.25);
  position: relative;
}
.progress-fill {
  height: 100%;
  border-radius: 3px;
  background: var(--live);
}
.osd-progress:hover .progress-track { height: 8px; }
.seek-hint {
  text-align: center;
  font-size: 14px;
  color: #ffeb3b;
  margin-bottom: 4px;
}

.osd-bottom {
  display: flex;
  align-items: center;
  gap: 12px;
}
.time {
  font-size: 13px;
  color: #fff;
  min-width: 52px;
}
.controls {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  flex: 1;
}
.ctl {
  padding: 8px 12px;
  border-radius: 4px;
  background: transparent;
  border: 1px solid transparent;
  color: #fff;
  font-size: 14px;
  cursor: pointer;
  white-space: nowrap;
}
.ctl:hover {
  background: var(--card-hi);
  border-color: #fff;
}
.ctl.primary {
  background: #fff;
  color: #141218;
  font-weight: 600;
}
.vol-slider {
  width: 80px;
  padding: 10px 0;
  cursor: pointer;
  touch-action: none;
}
.vol-track {
  height: 5px;
  border-radius: 3px;
  background: rgba(255, 255, 255, 0.25);
}
.vol-fill {
  height: 100%;
  border-radius: 3px;
  background: #fff;
}
.vol { font-size: 13px; color: #fff; }
.fullscreen-btn {
  font-size: 18px;
  padding: 6px 10px;
  margin-left: 4px;
  min-width: 36px;
}
.fullscreen-btn:hover {
  background: rgba(255, 255, 255, 0.2);
  border-color: rgba(255, 255, 255, 0.6);
}

/* 桌面端画中画按钮 */
.pip-btn {
  position: absolute;
  top: 16px;
  right: 16px;
  width: 36px;
  height: 36px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.12);
  border: 1px solid rgba(255, 255, 255, 0.35);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  z-index: 20;
  transition: background 0.15s, border-color 0.15s;
}
.pip-btn:hover {
  background: rgba(255, 255, 255, 0.24);
  border-color: #fff;
}

:fullscreen .osd,
:-webkit-full-screen .osd,
:-moz-full-screen .osd { position: fixed; }

/* ===== 移动端播放器 ===== */
.m-player {
  position: fixed;
  inset: 0;
  background: #000;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.m-player.m-fullscreen {
  /* 全屏时回归单一视频容器 */
}

.m-video-wrap {
  position: relative;
  width: 100%;
  flex-shrink: 0;
  background: #000;
}
.m-player:not(.m-fullscreen) .m-video-wrap {
  aspect-ratio: 16 / 9;
}
.m-player.m-fullscreen .m-video-wrap {
  flex: 1;
  height: 100%;
}

.m-center-tip {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  color: #fff;
  font-size: 15px;
  text-align: center;
  z-index: 10;
}
.m-center-tip.m-err p { margin-bottom: 10px; font-size: 14px; }

.m-panel {
  flex: 1;
  background: var(--surface);
  padding: 12px 16px 16px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.m-panel-title {
  display: flex;
  align-items: center;
  gap: 10px;
}
.m-back-btn {
  width: 36px;
  height: 36px;
  border-radius: 8px;
  background: var(--card);
  border: none;
  color: var(--text);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  flex-shrink: 0;
}
.m-title-info {
  flex: 1;
  min-width: 0;
}
.m-title-name {
  font-size: 15px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.m-title-meta {
  font-size: 12px;
  color: var(--sub);
  margin-top: 2px;
}
.m-icon-btn {
  width: 36px;
  height: 36px;
  border-radius: 8px;
  background: var(--card);
  border: none;
  color: var(--text);
  font-size: 18px;
  cursor: pointer;
  flex-shrink: 0;
}

.m-progress-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.m-time-sm {
  font-size: 11px;
  color: var(--sub);
  min-width: 36px;
  text-align: center;
}
.m-progress-wrap {
  flex: 1;
  padding: 8px 0;
  cursor: pointer;
  touch-action: none;
}
.m-progress-track {
  height: 4px;
  border-radius: 2px;
  background: var(--card-hi);
  position: relative;
}
.m-progress-fill {
  height: 100%;
  border-radius: 2px;
  background: var(--live);
}
.m-seek-hint {
  text-align: center;
  font-size: 13px;
  color: #ffeb3b;
  margin: -6px 0 6px;
}

.m-main-controls {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 16px;
  padding: 8px 0;
}
.m-ctl-icon {
  width: 44px;
  height: 44px;
  border-radius: 8px;
  background: var(--card);
  border: none;
  color: var(--text);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  flex-shrink: 0;
}
.m-play-btn {
  width: 64px;
  height: 64px;
  border-radius: 50%;
  background: var(--live);
  border: none;
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  flex-shrink: 0;
  box-shadow: 0 4px 12px rgba(0,0,0,0.15);
}

.m-aux-controls {
  display: flex;
  gap: 8px;
}
.m-aux-btn {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 10px 8px;
  border-radius: 8px;
  background: var(--card);
  border: none;
  color: var(--text);
  font-size: 12px;
  cursor: pointer;
}
.m-aux-icon {
  font-size: 20px;
}

/* 全屏时浮动 OSD */
.m-osd-fullscreen {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  padding: 12px;
  background: linear-gradient(rgba(0,0,0,0.5) 0%, transparent 20%, transparent 80%, rgba(0,0,0,0.7) 100%);
  pointer-events: none;
}
.m-osd-fullscreen > * { pointer-events: auto; }

.m-fs-top {
  display: flex;
  align-items: center;
  gap: 10px;
}
.m-fs-title {
  flex: 1;
  color: #fff;
  font-size: 15px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.m-fs-bottom {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.m-fs-controls {
  display: flex;
  align-items: center;
  gap: 12px;
}
.m-time {
  font-size: 12px;
  color: #fff;
  min-width: 40px;
  text-align: center;
}

/* ===== 手势提示覆盖层 ===== */
.m-gesture-hint {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  background: rgba(0, 0, 0, 0.65);
  color: #fff;
  font-size: 16px;
  font-weight: 600;
  padding: 10px 18px;
  border-radius: 8px;
  pointer-events: none;
  z-index: 20;
  white-space: nowrap;
}

/* ===== 模拟亮度遮罩 ===== */
.brightness-mask {
  position: absolute;
  inset: 0;
  background: #000;
  pointer-events: none;
  z-index: 5;
  transition: opacity 0.1s;
}
</style>
