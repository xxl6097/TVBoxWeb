// 播放器字幕状态机：HLS 内嵌字幕轨道 + 外挂字幕（本地文件 / 网络地址），
// 含字号、延迟、样式注入与清理。
// 从 PlayerView.vue 抽出，避免播放页继续膨胀。
import { ref, type Ref } from 'vue'
import { humanizeApiError } from '@/api'
import { toWebVtt } from '@/utils/subtitle'

export interface SubtitleTrack {
  lang: string
  label: string
  url: string
  default?: boolean
}

export interface SubtitleHost {
  /** 播放器 video 元素 */
  videoEl: Ref<HTMLVideoElement | undefined>
  /** 当前 hls 实例（可能为 null，用于同步内嵌文字轨） */
  getHls: () => any
  /** 全局轻提示 */
  tip: (msg: string) => void
  /** 把原始地址转成带鉴权的代理地址 */
  proxy: (url: string) => string
}

export function usePlayerSubtitles(host: SubtitleHost) {
  const subtitles = ref<SubtitleTrack[]>([])
  const currentSubIdx = ref(-1) // -1 表示不加载内嵌字幕
  const subDelay = ref(0) // 字幕延迟（毫秒）
  const subFontSize = ref(20)
  const subLabel = ref('')
  const externalSubActive = ref(false)

  let extTrack: HTMLTrackElement | null = null
  let extTrackUrl = ''
  let subStyleEl: HTMLStyleElement | null = null
  const cueOrigins = new WeakMap<any, { s: number; e: number }>()
  // 记住用户挂过的字幕原文，切集时可以原样重新挂上。
  // 外挂字幕（尤其是同剧集的 srt）通常整季通用，切集就让人重选太折腾。
  let extText = ''
  let extLabel = ''
  // 转封装模式下视频时间轴从 seek 点重算，外挂字幕的 cue 是整片绝对时间，
  // 需要按这个偏移整体平移，否则字幕会提前 remuxSeek 秒出现。
  let timelineBaseMs = 0

  /** 从 hls.js 实例读取内嵌字幕轨道列表 */
  function loadSubtitlesFromHls(hls: any) {
    subtitles.value = []
    currentSubIdx.value = -1
    if (!hls || !hls.media) return
    try {
      if (hls.subtitleTracks && Array.isArray(hls.subtitleTracks)) {
        for (let i = 0; i < hls.subtitleTracks.length; i++) {
          const track = hls.subtitleTracks[i]
          if (track) {
            subtitles.value.push({
              lang: track.lang || `track_${i}`,
              label: track.name || `字幕 ${i + 1}`,
              url: track.url || '',
              default: track.default || false
            })
          }
        }
      }
    } catch {
      /* 轨道结构因 hls.js 版本而异，取不到就当没有内嵌字幕 */
    }
  }

  /** 切换内嵌字幕轨道 */
  function switchSubtitle(idx: number) {
    const hls = host.getHls()
    if (idx === -1) {
      currentSubIdx.value = -1
      if (hls && hls.media && hls.media.textTracks) {
        for (let i = 0; i < hls.media.textTracks.length; i++) {
          hls.media.textTracks[i].mode = 'hidden'
        }
      }
      host.tip('已关闭字幕')
      return
    }
    if (idx < 0 || idx >= subtitles.value.length) return
    currentSubIdx.value = idx
    if (hls && hls.media && hls.media.textTracks) {
      for (let i = 0; i < hls.media.textTracks.length; i++) {
        hls.media.textTracks[i].mode = i === idx ? 'showing' : 'hidden'
      }
    }
    host.tip(`已激活字幕：${subtitles.value[idx].label}`)
  }

  // ::cue 无法被 scoped 样式命中，动态注入一条全局规则
  function applySubStyle() {
    if (!subStyleEl) {
      subStyleEl = document.createElement('style')
      subStyleEl.id = 'yz-sub-style'
      document.head.appendChild(subStyleEl)
    }
    subStyleEl.textContent = `video::cue { font-size: ${subFontSize.value}px; color: #fff; text-shadow: 0 1px 3px #000, 0 0 6px #000; }`
  }

  function setSubFontSize(px: number) {
    subFontSize.value = px
    applySubStyle()
    host.tip(`字幕字号 ${px}px`)
  }

  /** 把 <track> 从 video 上摘掉，但保留字幕内容（切集时用） */
  function detachTrack() {
    if (extTrack) {
      extTrack.remove()
      extTrack = null
    }
    if (extTrackUrl) {
      URL.revokeObjectURL(extTrackUrl)
      extTrackUrl = ''
    }
  }

  /** 彻底移除外挂字幕（用户主动点「移除字幕」或离开页面时） */
  function removeExternalSub() {
    detachTrack()
    externalSubActive.value = false
    subLabel.value = ''
    extText = ''
    extLabel = ''
  }

  /**
   * 切集时调用：摘掉当前 track 并清掉激活态，但记住字幕原文。
   * 等新的 video 就绪后由 reattachExternalSub() 重新挂上。
   */
  function detachExternalSub() {
    if (!extText) {
      detachTrack()
      externalSubActive.value = false
      subLabel.value = ''
      return
    }
    detachTrack()
    externalSubActive.value = false
  }

  /** 新视频就绪后重新挂上之前用过的外挂字幕 */
  function reattachExternalSub() {
    if (!extText) return
    mountTrack(extText, extLabel)
  }

  function mountTrack(text: string, label: string) {
    const vid = host.videoEl.value
    if (!vid) return
    detachTrack()
    const url = URL.createObjectURL(new Blob([toWebVtt(text)], { type: 'text/vtt' }))
    const track = document.createElement('track')
    track.kind = 'subtitles'
    track.label = label
    track.srclang = 'zh'
    track.src = url
    track.default = true
    vid.appendChild(track)
    const show = () => {
      if (track.track) {
        track.track.mode = 'showing'
        // 重新挂载后 cue 的时间轴是全新的，需要按当前 base 再平移一次
        applyCueShift(track)
      }
    }
    track.addEventListener('load', show)
    window.setTimeout(show, 300)
    extTrack = track
    extTrackUrl = url
    externalSubActive.value = true
    subLabel.value = label
    applySubStyle()
  }

  /**
   * 按 timelineBaseMs 平移一条轨道里的 cue。
   * 用 cueOrigins 记住原始时间，重复调用不会累积漂移。
   */
  function applyCueShift(track: any) {
    const cues = track?.cues
    if (!cues) return
    const shift = -timelineBaseMs / 1000
    for (let i = 0; i < cues.length; i++) {
      const cue = cues[i]
      if (!cueOrigins.has(cue)) cueOrigins.set(cue, { s: cue.startTime, e: cue.endTime })
      const o = cueOrigins.get(cue)!
      // 延迟（用户手调）和转封装偏移要叠加，两者都以原始时间为基准
      const delta = shift + subDelay.value / 1000
      cue.startTime = Math.max(0, o.s + delta)
      cue.endTime = Math.max(0, o.e + delta)
    }
  }

  /** 切集 / seek 后更新转封装偏移并重算所有 track 的 cue */
  function setTimelineBase(ms: number) {
    timelineBaseMs = Math.max(0, ms)
    const tracks = host.videoEl.value?.textTracks
    if (!tracks) return
    for (let i = 0; i < tracks.length; i++) applyCueShift(tracks[i])
  }

  function setExternalSub(text: string, label: string) {
    if (!host.videoEl.value) {
      host.tip('播放器尚未就绪')
      return
    }
    extText = text
    extLabel = label
    mountTrack(text, label)
  }

  async function onSubFile(e: Event) {
    const input = e.target as HTMLInputElement
    const f = input.files?.[0]
    if (!f) return
    try {
      setExternalSub(await f.text(), f.name)
      host.tip('字幕已加载：' + f.name)
    } catch {
      host.tip('字幕读取失败，请检查文件编码')
    }
    input.value = ''
  }

  async function promptSubUrl() {
    const u = window.prompt('输入字幕文件地址（SRT / VTT / ASS）')
    if (!u || !u.trim()) return
    try {
      const res = await fetch(host.proxy(u.trim()))
      if (!res.ok) throw new Error('HTTP ' + res.status)
      setExternalSub(await res.text(), u.split('/').pop() || '网络字幕')
      host.tip('字幕已加载')
    } catch (e) {
      host.tip('字幕加载失败：' + humanizeApiError(e))
    }
  }

  // 调整字幕延迟：基于原始时间重算，避免多次调整累积漂移。
  // 转封装模式下还要叠加 timelineBase 偏移，所以统一走 applyCueShift。
  function setSubDelay(ms: number) {
    subDelay.value = ms
    const tracks = host.videoEl.value?.textTracks
    if (tracks) {
      for (let i = 0; i < tracks.length; i++) applyCueShift(tracks[i])
    }
    host.tip(`字幕延迟 ${ms > 0 ? '+' : ''}${(ms / 1000).toFixed(1)}s`)
  }

  /** 页面卸载时清理外挂轨道与注入的样式节点 */
  function disposeSubtitles() {
    removeExternalSub()
    if (subStyleEl) {
      subStyleEl.remove()
      subStyleEl = null
    }
  }
  return {
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
    detachExternalSub,
    reattachExternalSub,
    setExternalSub,
    setTimelineBase,
    onSubFile,
    promptSubUrl,
    setSubDelay,
    disposeSubtitles
  }
}
