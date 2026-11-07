import { defineStore } from 'pinia'
import type { Episode } from '@/types'

// 进程内播放列表单例，等价于鸿蒙影视 Playlist.ets：
// 避免用路由传几百集的序列化数据。
interface PlaylistState {
  eps: Episode[]
  idx: number
  vodTitle: string
  isLive: boolean
  directMode: boolean
  header: Record<string, string>
  seek: number
  wdSrcId: string
  wdSrcName: string
  wdVod: string
  wdDir: string
  wdSeries: boolean
}

export const usePlayerStore = defineStore('player', {
  state: (): PlaylistState => ({
    eps: [],
    idx: 0,
    vodTitle: '',
    isLive: false,
    directMode: false,
    header: {},
    seek: 0,
    wdSrcId: '',
    wdSrcName: '',
    wdVod: '',
    wdDir: '',
    wdSeries: false
  }),
  getters: {
    count: (s) => s.eps.length,
    current: (s): Episode | null => (s.idx >= 0 && s.idx < s.eps.length ? s.eps[s.idx] : null),
    hasNext: (s) => s.idx + 1 < s.eps.length,
    hasPrev: (s) => s.idx > 0
  },
  actions: {
    set(eps: Episode[], idx: number, vodTitle: string, isLive = false, directMode = false) {
      this.eps = eps.map((e) => ({ name: e.name, url: e.url, urls: [...(e.urls || [])] }))
      this.idx = idx >= 0 && idx < eps.length ? idx : 0
      this.vodTitle = vodTitle
      this.isLive = isLive
      this.directMode = directMode
    },
    step(delta: number): Episode | null {
      const t = this.idx + delta
      if (t < 0 || t >= this.eps.length) return null
      this.idx = t
      return this.eps[t]
    },
    clear() {
      this.eps = []
      this.idx = 0
      this.vodTitle = ''
      this.isLive = false
      this.directMode = false
    }
  }
})
