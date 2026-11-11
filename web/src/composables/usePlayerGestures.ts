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
  // 模拟亮度：遮罩不透明度 0~0.7（Web 无法调系统亮度，用黑色遮罩模拟）
  const brightness = ref(0)
  let touchStartBrightness = 0

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
      const newB = Math.min(0.7, Math.max(0, touchStartBrightness + delta))
      brightness.value = newB
      const pct = Math.round((1 - newB / 0.7) * 100)
      showGestureHint(`亮度 ${pct}%`)
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
      if (host.videoEl.value) {
        host.videoEl.value.currentTime = newMs / 1000
        host.curMs.value = newMs
      }
      host.tip('已跳到 ' + fmtMs(newMs))
    }
    touchDir = ''
    gestureHint.value = ''
  }

  function disposeGestures() {
    if (gestureTimer) clearTimeout(gestureTimer)
    if (touchTapTimer) clearTimeout(touchTapTimer)
  }

  return { gestureHint, brightness, onTouchStart, onTouchMove, onTouchEnd, disposeGestures }
}
