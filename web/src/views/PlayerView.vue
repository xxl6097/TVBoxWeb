<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, proxied } from '@/api'
import { useAppStore } from '@/stores/app'
import { usePlayerStore } from '@/stores/player'
import { fmtMs } from '@/utils/format'
import { fromHlsLevels, labelOfLevel, labelOf, AUTO, TOP, QLevel } from '@/utils/quality'

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
let introDone = false
let outroFired = false
let skipTimer = 0
let osdTimer = 0
let flashTimer = 0
let volTimer = 0
let leave = false
let lastSaveMs = 0
let saveTimer = 0 // savePos 防抖 timer

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
  if (saveTimer) clearTimeout(saveTimer)
  savePos() // 退出时立即保存最终进度
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

function proxiedFinal(): string {
  return proxied(finalUrl, directMode ? player.header?.auth : undefined)
}

async function initPlayer() {
  if (!finalUrl || !videoEl.value) return
  const vid = videoEl.value
  const src = proxiedFinal()
  if (src.includes('.m3u8') || via.value === '直播直连' || via.value === '补全 m3u8') {
    const HlsCls = await loadHls()
    if (HlsCls.isSupported()) {
      // maxBufferLength: 10 秒缓冲（原来 30 秒，在 1080p 流上会占 100MB+ 内存）
      const h = new HlsCls({ maxBufferLength: 10 })
      hls = h
      h.loadSource(src)
      h.attachMedia(vid)
      h.on(HlsCls.Events.MANIFEST_PARSED, () => {
        if (h.levels && h.levels.length > 1) {
          levels.value = fromHlsLevels(h.levels)
          applyInitialLevel()
        }
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
    if (player.hasNext && !isLive) {
      switchEpisode(1)
    } else {
      showOsd.value = true
    }
  }
  vid.onerror = () => {
    onPlayError()
  }
  vid.onenterpictureinpicture = () => { isPip.value = true }
  vid.onleavepictureinpicture = () => { isPip.value = false }
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

// ===== 移动端手势 =====
let touchStartX = 0
let touchStartY = 0
let touchStartVol = 0
let touchStartMs = 0
let touchDir: 'horizontal' | 'vertical-left' | 'vertical-right' | '' = ''
let touchLastTap = 0
let touchTapTimer = 0
const gestureHint = ref('')
let gestureTimer = 0

function showGestureHint(msg: string) {
  gestureHint.value = msg
  if (gestureTimer) clearTimeout(gestureTimer)
  gestureTimer = window.setTimeout(() => (gestureHint.value = ''), 1200)
}

function onTouchStart(e: TouchEvent) {
  if (e.touches.length !== 1) return
  const t = e.touches[0]
  touchStartX = t.clientX
  touchStartY = t.clientY
  touchStartVol = volume.value
  touchStartMs = curMs.value
  touchDir = ''
}

function onTouchMove(e: TouchEvent) {
  if (e.touches.length !== 1) return
  const t = e.touches[0]
  const dx = t.clientX - touchStartX
  const dy = t.clientY - touchStartY
  const adx = Math.abs(dx)
  const ady = Math.abs(dy)

  if (!touchDir && (adx > 8 || ady > 8)) {
    // 判断滑动方向：横向 or 纵向（左半屏亮度，右半屏音量）
    if (adx > ady * 1.5) {
      touchDir = 'horizontal'
    } else {
      const isRightHalf = touchStartX > window.innerWidth / 2
      touchDir = isRightHalf ? 'vertical-right' : 'vertical-left'
    }
  }

  if (touchDir === 'horizontal') {
    // 横向滑动：快进退（1px = 约 0.5s，最大 ±90s）
    if (isLive || durMs.value <= 0) return
    e.preventDefault()
    const seekDelta = (dx / window.innerWidth) * 90000 // 90s range
    const newMs = Math.max(0, Math.min(durMs.value, touchStartMs + seekDelta))
    const diff = Math.round((newMs - touchStartMs) / 1000)
    const sign = diff >= 0 ? '+' : ''
    showGestureHint(`${sign}${diff}s → ${fmtMs(newMs)}`)
  } else if (touchDir === 'vertical-right') {
    // 右半屏上下：音量
    e.preventDefault()
    const delta = -dy / 200 // 上划增大
    const newVol = Math.min(1, Math.max(0, touchStartVol + delta))
    volume.value = newVol
    if (newVol > 0) muted.value = false
    if (videoEl.value) videoEl.value.volume = muted.value ? 0 : newVol
    showGestureHint(`音量 ${Math.round(newVol * 100)}%`)
  } else if (touchDir === 'vertical-left') {
    // 左半屏上下：预留亮度（Web API 不支持屏幕亮度，显示提示）
    e.preventDefault()
    showGestureHint('亮度调节请用系统控制中心')
  }
}

function onTouchEnd(e: TouchEvent) {
  if (!touchDir) {
    // 无方向滑动 = 点击
    const now = Date.now()
    const t = e.changedTouches[0]
    if (now - touchLastTap < 300) {
      // 双击：左侧 -10s，右侧 +10s
      clearTimeout(touchTapTimer)
      const isRight = t.clientX > window.innerWidth / 2
      if (!isLive && durMs.value > 0) {
        seekBy(isRight ? 1 : -1)
        flashPlay(isRight ? 'play' : 'pause')
        showGestureHint(isRight ? '+10s' : '-10s')
      } else {
        togglePlay()
      }
      touchLastTap = 0
    } else {
      touchLastTap = now
      touchTapTimer = window.setTimeout(() => {
        // 单击：切换 OSD
        touchLastTap = 0
        if (!showOsd.value) popOsd()
        else showOsd.value = false
      }, 280)
    }
    return
  }

  // 横向滑动结束：实际 seek
  if (touchDir === 'horizontal' && !isLive && durMs.value > 0) {
    const dx = e.changedTouches[0].clientX - touchStartX
    const seekDelta = (dx / window.innerWidth) * 90000
    const newMs = Math.max(0, Math.min(durMs.value, touchStartMs + seekDelta))
    if (videoEl.value) {
      videoEl.value.currentTime = newMs / 1000
      curMs.value = newMs
    }
    app.showTip('已跳到 ' + fmtMs(newMs))
  }
  touchDir = ''
  gestureHint.value = ''
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
      <div class="osd-progress" @pointerdown="onProgressDown" @pointermove="onProgressMove" @pointerup="onProgressUp">
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
          <span v-for="(lv, i) in levels" :key="i" class="chip" :class="{ active: qCap > 0 && qCap < TOP && levelIdx === i }" @click="chooseLevel(lv.height > 0 ? lv.height : TOP)">{{ labelOfLevel(lv) }}</span>
        </div>
        <div v-if="!levels.length" class="q-hint">源只提供这一档清晰度。</div>
        <div class="q-section">画面比例</div>
        <div class="q-chips">
          <span v-for="(n, i) in scaleNames" :key="'sc' + i" class="chip" :class="{ active: scaleMode === i }" @click="setScale(i)">{{ n }}</span>
        </div>
        <div class="q-section">跳过片头</div>
        <div class="q-chips">
          <span v-for="s in SKIP_PRESETS" :key="'i' + s" class="chip" :class="{ active: introSkipSec === s }" @click="setIntroSkip(s)">{{ s === 0 ? '关' : s + ' 秒' }}</span>
          <span class="chip" @click="markSkip('intro')">标当前</span>
        </div>
        <div class="q-section">跳过片尾</div>
        <div class="q-chips">
          <span v-for="s in SKIP_PRESETS" :key="'o' + s" class="chip" :class="{ active: outroSkipSec === s }" @click="setOutroSkip(s)">{{ s === 0 ? '关' : s + ' 秒' }}</span>
          <span class="chip" @click="markSkip('outro')">标当前</span>
        </div>
        <div class="q-footer"><el-button size="small" @click="showQ = false">关闭</el-button></div>
      </div>
    </div>
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
          <div class="m-progress-wrap" @pointerdown="onProgressDown" @pointermove="onProgressMove" @pointerup="onProgressUp">
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
        <div class="m-progress-wrap" @pointerdown="onProgressDown" @pointermove="onProgressMove" @pointerup="onProgressUp">
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
        <button class="m-aux-btn" @click="showQ = !showQ">
          <span class="m-aux-icon">⚙</span>
          <span>画质</span>
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

    <!-- 画质面板（移动端底部弹出） -->
    <div v-if="showQ" class="m-q-mask" @click.self="showQ = false">
      <div class="m-q-sheet">
        <div class="m-q-handle"></div>
        <div class="m-q-title">播放设置</div>
        <div class="m-q-section">画质</div>
        <div class="m-q-chips">
          <span class="chip" :class="{ active: qCap === AUTO }" @click="chooseLevel(AUTO)">自动</span>
          <span class="chip" :class="{ active: qCap >= TOP }" @click="chooseLevel(TOP)">原画</span>
          <span v-for="(lv, i) in levels" :key="i" class="chip" :class="{ active: qCap > 0 && qCap < TOP && levelIdx === i }" @click="chooseLevel(lv.height > 0 ? lv.height : TOP)">{{ labelOfLevel(lv) }}</span>
        </div>
        <div class="m-q-section">画面比例</div>
        <div class="m-q-chips">
          <span v-for="(n, i) in scaleNames" :key="'sc' + i" class="chip" :class="{ active: scaleMode === i }" @click="setScale(i)">{{ n }}</span>
        </div>
        <div class="m-q-section">跳过片头</div>
        <div class="m-q-chips">
          <span v-for="s in SKIP_PRESETS" :key="'i' + s" class="chip" :class="{ active: introSkipSec === s }" @click="setIntroSkip(s)">{{ s === 0 ? '关' : s + ' 秒' }}</span>
          <span class="chip" @click="markSkip('intro')">标当前</span>
        </div>
        <div class="m-q-section">跳过片尾</div>
        <div class="m-q-chips">
          <span v-for="s in SKIP_PRESETS" :key="'o' + s" class="chip" :class="{ active: outroSkipSec === s }" @click="setOutroSkip(s)">{{ s === 0 ? '关' : s + ' 秒' }}</span>
          <span class="chip" @click="markSkip('outro')">标当前</span>
        </div>
        <div class="m-q-close"><el-button size="small" @click="showQ = false" style="width:100%">关闭</el-button></div>
      </div>
    </div>
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
.q-title { font-size: 16px; font-weight: 600; margin-bottom: 6px; }
.q-section { font-size: 13px; color: var(--sub); margin: 16px 0 8px; }
.q-chips { display: flex; flex-wrap: wrap; gap: 10px; }
.q-hint { margin-top: 8px; color: var(--dim); font-size: 13px; }
.q-footer { margin-top: 20px; text-align: right; }

.chip {
  padding: 8px 16px;
  border-radius: 20px;
  background: var(--card);
  border: 1px solid transparent;
  font-size: 14px;
  cursor: pointer;
  transition: all 0.15s;
}
.chip:hover { background: var(--card-hi); }
.chip.active {
  background: var(--live);
  color: #fff;
  font-weight: 600;
}

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

/* 移动端画质弹窗（底部弹出） */
.m-q-mask {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: flex-end;
  z-index: 100;
}
.m-q-sheet {
  width: 100%;
  background: var(--surface);
  border-radius: 16px 16px 0 0;
  padding: 20px 16px;
  max-height: 80vh;
  overflow-y: auto;
}
.m-q-handle {
  width: 36px;
  height: 4px;
  border-radius: 2px;
  background: var(--card-hi);
  margin: 0 auto 16px;
}
.m-q-title {
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 16px;
  text-align: center;
}
.m-q-section {
  font-size: 13px;
  color: var(--sub);
  margin: 16px 0 8px;
}
.m-q-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.m-q-close {
  margin-top: 16px;
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
</style>
