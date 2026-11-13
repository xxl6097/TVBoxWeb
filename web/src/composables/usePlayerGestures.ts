// 移动端播放器手势：横向快进退、左半屏亮度、右半屏音量、单击切 OSD、双击快进退。
// 从 PlayerView.vue 抽出，宿主通过回调注入副作用（seek / 换台 / 提示等）。
import { ref, type Ref } from 'vue'
import { fmtMs } from '@/utils/format'

export interface GestureHost {
  videoEl: Ref<HTMLVideoElement | undefined>
  volume: Ref<number>
  muted: Ref<boolean>
  curMs: Ref<number>
  durMs: Ref<number>
  showOsd: Ref<boolean>
  isLive: () => boolean
  /** 相对快进退（-1 后退 10s，1 前进 10s） */
  seekBy: (dir: number) => void
  /**
   * 跳到指定秒数。转封装流必须走它（内部会带 seek 参数重新起转），
   * 直接改 currentTime 会失效甚至重载整条流。
   */
  applySeek: (t: number) => void
  /** 播放/暂停 */
  togglePlay: () => void
  /** 中央播放/暂停动画 */
  flashPlay: (kind: 'play' | 'pause') => void
  /** 唤起 OSD 控制条 */
  popOsd: () => void
  /** 音量变化后持久化 */
  persistVolume: () => void
  tip: (msg: string) => void
}

export function usePlayerGestures(host: GestureHost) {
  let touchStartX = 0
  let touchStartY = 0
  let touchStartVol = 0
  let touchStartMs = 0
  let touchDir: 'horizontal' | 'vertical-left' | 'vertical-right' | '' = ''
  let touchLastTap = 0
  let touchTapTimer = 0
  let gestureTimer = 0

  const gestureHint = ref('')
  // 模拟亮度：黑色遮罩的不透明度（Web 无法调系统亮度）。
  // 上限压到 0.55：原来允许到 0.7，左半屏稍微一划就压到七成黑，
  // 画面糊成一片纯色，用户会以为视频坏了；而且没有复位入口，
  // 只能退出重进。现在双击左半屏可以立刻恢复。
  const BRIGHT_MAX = 0.55
  const brightness = ref(0)
  let touchStartBrightness = 0

  /** 恢复 100% 亮度（清掉黑色遮罩）。切集、换源、离开页面都要调。 */
  function resetBrightness() {
    brightness.value = 0
  }

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
    touchStartVol = host.volume.value
    touchStartMs = host.curMs.value
    touchStartBrightness = brightness.value
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
      if (host.isLive() || host.durMs.value <= 0) return
      e.preventDefault()
      const seekDelta = (dx / window.innerWidth) * 90000
      const newMs = Math.max(0, Math.min(host.durMs.value, touchStartMs + seekDelta))
      const diff = Math.round((newMs - touchStartMs) / 1000)
      const sign = diff >= 0 ? '+' : ''
      showGestureHint(`${sign}${diff}s → ${fmtMs(newMs)}`)
    } else if (touchDir === 'vertical-right') {
      // 右半屏上下：音量
      e.preventDefault()
      const delta = -dy / 200 // 上划增大
      const newVol = Math.min(1, Math.max(0, touchStartVol + delta))
      host.volume.value = newVol
      if (newVol > 0) host.muted.value = false
      if (host.videoEl.value) host.videoEl.value.volume = host.muted.value ? 0 : newVol
      host.persistVolume()
      showGestureHint(`音量 ${Math.round(newVol * 100)}%`)
    } else if (touchDir === 'vertical-left') {
      // 左半屏上下：模拟亮度（黑色遮罩，Web 无法调系统屏幕亮度）
      e.preventDefault()
      const delta = -dy / 300 // 上划变亮（遮罩变淡）
      const newB = Math.min(BRIGHT_MAX, Math.max(0, touchStartBrightness + delta))
      brightness.value = newB
      const pct = Math.round((1 - newB / BRIGHT_MAX) * 100)
      showGestureHint(`亮度 ${pct}%`)
    }
  }

  function onTouchEnd(e: TouchEvent) {
    if (!touchDir) {
      // 无方向滑动 = 点击
      const now = Date.now()
      const t = e.changedTouches[0]
      if (now - touchLastTap < 300) {
        // 双击：左侧恢复亮度 / 退 10 秒，右侧进 10 秒
        clearTimeout(touchTapTimer)
        const isRight = t.clientX > window.innerWidth / 2
        // 左半屏且画面被压暗时，双击优先「恢复亮度」——
        // 遮罩压到深处又找不到开关，用户只能退出重进，得留个救急手段。
        if (!isRight && brightness.value > 0.05) {
          resetBrightness()
          showGestureHint('亮度 100%')
          touchLastTap = 0
          return
        }
        if (!host.isLive() && host.durMs.value > 0) {
          host.seekBy(isRight ? 1 : -1)
          host.flashPlay(isRight ? 'play' : 'pause')
          showGestureHint(isRight ? '+10s' : '-10s')
        } else {
          host.togglePlay()
        }
        touchLastTap = 0
      } else {
        touchLastTap = now
        touchTapTimer = window.setTimeout(() => {
          // 单击：切换 OSD
          touchLastTap = 0
          if (!host.showOsd.value) host.popOsd()
          else host.showOsd.value = false
        }, 280)
      }
      return
    }

    // 横向滑动结束：实际 seek
    if (touchDir === 'horizontal' && !host.isLive() && host.durMs.value > 0) {
      const dx = e.changedTouches[0].clientX - touchStartX
      const seekDelta = (dx / window.innerWidth) * 90000
      const newMs = Math.max(0, Math.min(host.durMs.value, touchStartMs + seekDelta))
      // 必须走 host.applySeek 而不是直接改 currentTime：
      // 转封装流（网盘 MKV 之类）直接改 currentTime 无效甚至重载整条流，
      // 表现为画面黑一下再回来。applySeek 会按模式分别处理。
      host.applySeek(newMs / 1000)
      host.curMs.value = newMs
      host.tip('已跳到 ' + fmtMs(newMs))
    }
    touchDir = ''
    gestureHint.value = ''
  }

  function disposeGestures() {
    if (gestureTimer) clearTimeout(gestureTimer)
    if (touchTapTimer) clearTimeout(touchTapTimer)
  }

  return { gestureHint, brightness, resetBrightness, onTouchStart, onTouchMove, onTouchEnd, disposeGestures }
}
