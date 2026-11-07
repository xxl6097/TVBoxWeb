<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Hls from 'hls.js'
import { api, proxied } from '@/api'
import { useAppStore } from '@/stores/app'
import { usePlayerStore } from '@/stores/player'
import { fmtMs } from '@/utils/format'
import { fromHlsLevels, labelOfLevel, labelOf, AUTO, TOP, QLevel } from '@/utils/quality'

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

let rawUrl = rawUrl0
let finalUrl = ''
let isLive = player.isLive
let directMode = player.directMode || route.query.direct === '1'
let qCap = 0
let levelIdx = -1
const levels = ref<QLevel[]>([])
let liveFail = 0

const videoEl = ref<HTMLVideoElement>()
let hls: Hls | null = null
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
let introDone = false
let outroFired = false
let skipTimer = 0
let osdTimer = 0
let flashTimer = 0
let volTimer = 0
let leave = false
let lastSaveMs = 0

const isLivePlaying = computed(() => isLive)
const playPauseText = computed(() => (playing.value ? '暂停' : '播放'))
const muteText = computed(() => (muted.value ? '取消静音' : '静音'))
const volPct = computed(() => (muted.value ? '静音' : Math.round(volume.value * 100) + '%'))
const progressPct = computed(() => (durMs.value > 0 ? (curMs.value / durMs.value) * 100 : 0))

// ===== 全屏 =====
const playerEl = ref<HTMLElement>()
const isFullscreen = ref(false)

function toggleFullscreen() {
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

function onKeyDown(e: KeyboardEvent) {
  // 在输入框等焦点元素里不响应
  if ((e.target as HTMLElement).tagName === 'INPUT' || (e.target as HTMLElement).tagName === 'TEXTAREA') return
  if (e.key === 'f' || e.key === 'F') toggleFullscreen()
  if (e.key === ' ') { e.preventDefault(); togglePlay() }
  if (e.key === 'ArrowRight') seekBy(1)
  if (e.key === 'ArrowLeft') seekBy(-1)
  if (e.key === 'Escape' && isFullscreen.value) toggleFullscreen()
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
  const prefs = await api.prefs().catch(() => ({ qPref: 0, introSkip: 0, outroSkip: 0, tapSound: 1, tapVibrate: 1 }))
  qCap = prefs.qPref || 0
  introSkipSec.value = prefs.introSkip || 0
  outroSkipSec.value = prefs.outroSkip || 0
  syncEpLabel()
  await resolveUrl()
  resolving.value = false
  initPlayer()
})

onUnmounted(() => {
  leave = true
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
  while (skip <= LIVE_SKIP_MAX && tries < 7) {
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

function proxiedFinal(): string {
  return proxied(finalUrl, directMode ? player.header?.auth : undefined)
}

function initPlayer() {
  if (!finalUrl || !videoEl.value) return
  const vid = videoEl.value
  const src = proxiedFinal()
  if (src.includes('.m3u8') || src.includes('.m3u8?') || via.value === '直播直连' || via.value === '补全 m3u8') {
    if (Hls.isSupported()) {
      const h = new Hls({ maxBufferLength: 30 })
      hls = h
      h.loadSource(src)
      h.attachMedia(vid)
      h.on(Hls.Events.MANIFEST_PARSED, () => {
        if (h.levels && h.levels.length > 1) {
          levels.value = fromHlsLevels(h.levels)
          applyInitialLevel()
        }
      })
      h.on(Hls.Events.ERROR, (_e, data) => {
        if (!data.fatal) return
        // seek 或网络抖动引发的 fatal 错误先尝试恢复，而不是直接判死
        if (hlsRecover < 3) {
          hlsRecover++
          if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
            app.showTip('网络抖动，正在恢复…')
            h.startLoad()
            return
          }
          if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
            app.showTip('解码异常，正在恢复…')
            h.recoverMediaError()
            return
          }
        }
        onPlayError()
      })
    } else if (vid.canPlayType('application/vnd.apple.mpegurl')) {
      vid.src = src
    }
  } else {
    vid.src = src
  }
  bindVideoEvents(vid)
  vid.play().catch(() => {})
}

function bindVideoEvents(vid: HTMLVideoElement) {
  vid.onloadedmetadata = () => {
    durMs.value = isLive ? 0 : vid.duration * 1000
    prepared.value = true
    buffering.value = false
    playing.value = true
    popOsd()
    // 网盘续播：起播后 seek 到上次进度
    if (player.seek > 0 && !isLive) {
      vid.currentTime = player.seek / 1000
      curMs.value = player.seek
      player.seek = 0
      app.showTip('已从上次位置继续播放')
    }
    if (introSkipSec.value > 0 && !introDone && !isLive) {
      const skipMs = introSkipSec.value * 1000
      if (durMs.value <= 0 || skipMs < durMs.value - 30000) showSkipBtn('intro', 5)
    }
  }
  vid.ontimeupdate = () => {
    curMs.value = vid.currentTime * 1000
    // 每 5 秒节流保存一次进度（退出/换集也会保存）
    if (curMs.value - lastSaveMs > 5000) {
      lastSaveMs = curMs.value
      savePos()
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
    if (player.hasNext && !isLive) {
      switchEpisode(1)
    } else {
      showOsd.value = true
    }
  }
  vid.onerror = () => {
    onPlayError()
  }
}

function onPlayError() {
  if (isLive && liveFail < 3 && player.hasNext) {
    liveFail++
    app.showTip('该频道无法播放，自动换下一个')
    switchEpisode(1)
    return
  }
  errMsg.value = '播放失败，可能是链接失效或编码不支持'
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

function ratioFromEvent(e: PointerEvent): number {
  const el = e.currentTarget as HTMLElement
  const rect = el.getBoundingClientRect()
  return Math.min(1, Math.max(0, (e.clientX - rect.left) / rect.width))
}

function onProgressDown(e: PointerEvent) {
  if (!prepared.value || durMs.value <= 0 || isLive) return
  dragging.value = true
  previewMs.value = durMs.value * ratioFromEvent(e)
  ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
}

function onProgressMove(e: PointerEvent) {
  if (!dragging.value) return
  previewMs.value = durMs.value * ratioFromEvent(e)
}

function onProgressUp(e: PointerEvent) {
  if (!dragging.value) return
  dragging.value = false
  const t = previewMs.value / 1000
  if (videoEl.value && t >= 0 && durMs.value > 0) {
    videoEl.value.currentTime = t
    curMs.value = previewMs.value
  }
  const target = fmtMs(previewMs.value)
  previewMs.value = -1
  app.showTip('已定位到 ' + target)
  popOsd()
}

// ---------- 音量滑块 ----------

const volDragging = ref(false)

function setVolumeFromEvent(e: PointerEvent) {
  const el = e.currentTarget as HTMLElement
  const rect = el.getBoundingClientRect()
  let r = (e.clientX - rect.left) / rect.width
  r = Math.min(1, Math.max(0, r))
  volume.value = r
  if (r > 0) muted.value = false
  if (videoEl.value) videoEl.value.volume = muted.value ? 0 : r
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
  vid.currentTime = t
  curMs.value = t * 1000
  popOsd()
}

function seekToRatio(ratio: number) {
  const vid = videoEl.value
  if (!vid || !prepared.value || durMs.value <= 0) return
  let r = Math.min(1, Math.max(0, ratio))
  const t = (durMs.value * r) / 1000
  vid.currentTime = t
  curMs.value = t * 1000
  popOsd()
}

function changeVolume(delta: number) {
  let v = volume.value + delta
  v = Math.min(1, Math.max(0, v))
  volume.value = v
  if (v > 0) muted.value = false
  if (videoEl.value) videoEl.value.volume = muted.value ? 0 : v
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

async function switchEpisode(delta: number) {
  if (!player.count) {
    app.showTip(isLive ? '当前分组没有其他频道' : '当前没有可切换的剧集')
    return
  }
  savePos()
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
      videoEl.value.currentTime = skipMs / 1000
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
}

onMounted(() => {
  window.addEventListener('keydown', onKey)
  document.addEventListener('fullscreenchange', onFullscreenChange)
  document.addEventListener('webkitfullscreenchange', onFullscreenChange)
  document.addEventListener('mozfullscreenchange', onFullscreenChange)
  document.addEventListener('MSFullscreenChange', onFullscreenChange)
})
onUnmounted(() => {
  window.removeEventListener('keydown', onKey)
  document.removeEventListener('fullscreenchange', onFullscreenChange)
  document.removeEventListener('webkitfullscreenchange', onFullscreenChange)
  document.removeEventListener('mozfullscreenchange', onFullscreenChange)
  document.removeEventListener('MSFullscreenChange', onFullscreenChange)
})
</script>

<template>
  <div ref="playerEl" class="player" @mousemove="onMouseMove" @click="onSurfaceClick" @dblclick="toggleFullscreen">
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
      <div
        class="osd-progress"
        @pointerdown="onProgressDown"
        @pointermove="onProgressMove"
        @pointerup="onProgressUp"
      >
        <div class="progress-track">
          <div
            class="progress-fill"
            :style="{ width: (dragging ? (previewMs / durMs) * 100 : progressPct) + '%' }"
          ></div>
        </div>
      </div>
      <div v-if="dragging" class="seek-hint">
        {{ fmtMs(previewMs) }} / {{ fmtMs(durMs) }}
      </div>
      <div class="osd-bottom">
        <span class="time">{{ dragging ? fmtMs(previewMs) : fmtMs(curMs) }}</span>
        <div class="controls">
          <button class="ctl" @click="switchEpisode(-1)">{{ isLive ? '上一台' : '上一集' }}</button>
          <button class="ctl" @click="seekBy(-1)">−10 秒</button>
          <button class="ctl primary" @click="togglePlay">{{ playPauseText }}</button>
          <button class="ctl" @click="seekBy(1)">+10 秒</button>
          <button class="ctl" @click="toggleMute">{{ muteText }}</button>
          <div
            class="vol-slider"
            @pointerdown="onVolDown"
            @pointermove="onVolMove"
            @pointerup="onVolUp"
          >
            <div class="vol-track">
              <div class="vol-fill" :style="{ width: (muted ? 0 : volume * 100) + '%' }"></div>
            </div>
          </div>
          <button class="ctl" @click="showQ = !showQ">画质</button>
          <button class="ctl" @click="switchEpisode(1)">{{ isLive ? '下一台' : '下一集' }}</button>
          <button class="ctl fullscreen-btn" :title="isFullscreen ? '退出全屏 (F)' : '全屏 (F)'" @click="toggleFullscreen">
            {{ isFullscreen ? '⤓' : '⛶' }}
          </button>
        </div>
        <span class="time">{{ durMs > 0 ? fmtMs(durMs) : (isLive ? '直播' : '') }}</span>
        <span v-if="showVol" class="vol">{{ volPct }}</span>
      </div>
    </div>

    <!-- 画质面板 -->
    <div v-if="showQ" class="q-panel" @click.stop @click.self="showQ = false">
      <div class="q-content">
        <div class="q-title">播放设置</div>

        <div class="q-section">画质</div>
        <div class="q-chips">
          <span class="chip" :class="{ active: qCap === AUTO }" @click="chooseLevel(AUTO)">自动</span>
          <span class="chip" :class="{ active: qCap >= TOP }" @click="chooseLevel(TOP)">原画</span>
          <span
            v-for="(lv, i) in levels"
            :key="i"
            class="chip"
            :class="{ active: qCap > 0 && qCap < TOP && levelIdx === i }"
            @click="chooseLevel(lv.height > 0 ? lv.height : TOP)"
          >
            {{ labelOfLevel(lv) }}
          </span>
        </div>
        <div v-if="!levels.length" class="q-hint">源只提供这一档清晰度。网络差时播放器会自动加大缓冲。</div>

        <div class="q-section">画面比例</div>
        <div class="q-chips">
          <span
            v-for="(n, i) in scaleNames"
            :key="'sc' + i"
            class="chip"
            :class="{ active: scaleMode === i }"
            @click="setScale(i)"
          >
            {{ n }}
          </span>
        </div>

        <div class="q-section">跳过片头</div>
        <div class="q-chips">
          <span
            v-for="s in SKIP_PRESETS"
            :key="'i' + s"
            class="chip"
            :class="{ active: introSkipSec === s }"
            @click="setIntroSkip(s)"
          >
            {{ s === 0 ? '关' : s + ' 秒' }}
          </span>
          <span class="chip" @click="markSkip('intro')">标当前</span>
        </div>

        <div class="q-section">跳过片尾</div>
        <div class="q-chips">
          <span
            v-for="s in SKIP_PRESETS"
            :key="'o' + s"
            class="chip"
            :class="{ active: outroSkipSec === s }"
            @click="setOutroSkip(s)"
          >
            {{ s === 0 ? '关' : s + ' 秒' }}
          </span>
          <span class="chip" @click="markSkip('outro')">标当前</span>
        </div>

        <div class="q-footer">
          <el-button size="small" @click="showQ = false">关闭</el-button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
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
.flash-enter-active,
.flash-leave-active {
  transition: opacity 0.3s;
}
.flash-enter-from,
.flash-leave-to {
  opacity: 0;
}
.center-tip {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  color: #fff;
  font-size: 18px;
  text-align: center;
}
.center-tip.err p {
  margin-bottom: 12px;
}
.err-actions {
  display: flex;
  justify-content: center;
  gap: 12px;
}
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
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
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
.osd-title {
  font-size: 18px;
  font-weight: 600;
}
.osd-meta {
  font-size: 13px;
  color: var(--sub);
}
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
.osd-progress:hover .progress-track {
  height: 8px;
}
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
.vol {
  font-size: 13px;
  color: #fff;
}
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
/* 全屏状态下让 OSD 铺满全屏根元素 */
:fullscreen .osd,
:-webkit-full-screen .osd,
:-moz-full-screen .osd {
  position: fixed;
}
.q-panel {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
}
.q-content {
  background: var(--surface);
  border-radius: 16px;
  padding: 24px;
  max-width: 640px;
  width: 80%;
  max-height: 80vh;
  overflow-y: auto;
}
.q-title {
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 6px;
}
.q-section {
  font-size: 13px;
  color: var(--sub);
  margin: 16px 0 8px;
}
.q-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}
.q-hint {
  margin-top: 8px;
  color: var(--dim);
  font-size: 13px;
}
.q-footer {
  margin-top: 20px;
  text-align: right;
}

/* ===== 移动端响应式 ===== */
@media (max-width: 640px) {
  /* 控制条按钮紧凑：3行布局 */
  .controls {
    gap: 4px;
  }
  .ctl {
    padding: 6px 8px;
    font-size: 12px;
    min-width: 44px;
    text-align: center;
  }
  .ctl.fullscreen-btn { min-width: 44px; }
  /* 时间戳字体缩小 */
  .time { font-size: 11px; min-width: 44px; }
  /* 音量滑块在手机上宽度适配 */
  .vol-track { width: 80px; }
  /* OSD 顶栏标题截断 */
  .osd-title { font-size: 14px; max-width: 60vw; }
  /* 画质面板宽度 */
  .q-content { width: 92%; max-width: none; }
}
</style>
