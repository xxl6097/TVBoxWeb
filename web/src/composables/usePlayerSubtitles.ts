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

  function removeExternalSub() {
    if (extTrack) {
      extTrack.remove()
      extTrack = null
    }
    if (extTrackUrl) {
      URL.revokeObjectURL(extTrackUrl)
      extTrackUrl = ''
    }
    externalSubActive.value = false
    subLabel.value = ''
  }

  function setExternalSub(text: string, label: string) {
    const vid = host.videoEl.value
    if (!vid) {
      host.tip('播放器尚未就绪')
      return
    }
    removeExternalSub()
    const url = URL.createObjectURL(new Blob([toWebVtt(text)], { type: 'text/vtt' }))
    const track = document.createElement('track')
    track.kind = 'subtitles'
    track.label = label
    track.srclang = 'zh'
    track.src = url
    track.default = true
    vid.appendChild(track)
    track.addEventListener('load', () => {
      if (track.track) track.track.mode = 'showing'
    })
    window.setTimeout(() => {
      if (track.track) track.track.mode = 'showing'
    }, 300)
    extTrack = track
    extTrackUrl = url
    externalSubActive.value = true
    subLabel.value = label
    applySubStyle()
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

  // 调整字幕延迟：基于原始时间重算，避免多次调整累积漂移
  function setSubDelay(ms: number) {
    subDelay.value = ms
    const tracks = host.videoEl.value?.textTracks
    if (tracks) {
      for (let i = 0; i < tracks.length; i++) {
        const cues = tracks[i].cues
        if (!cues) continue
        for (let j = 0; j < cues.length; j++) {
          const cue: any = cues[j]
          if (!cueOrigins.has(cue)) cueOrigins.set(cue, { s: cue.startTime, e: cue.endTime })
          const o = cueOrigins.get(cue)!
          cue.startTime = Math.max(0, o.s + ms / 1000)
          cue.endTime = Math.max(0, o.e + ms / 1000)
        }
      }
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
    setExternalSub,
    onSubFile,
    promptSubUrl,
    setSubDelay,
    disposeSubtitles
  }
}
