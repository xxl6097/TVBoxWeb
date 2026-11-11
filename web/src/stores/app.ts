import { defineStore } from 'pinia'
import { api } from '@/api'

interface Status {
  isCustom: boolean
  subName: string
  configUrl: string
  totalSites: number
  activeSites: number
  feedCount: number
  lastCrawl: number
  crawlSummary: string
  liveUrl: string
  privacyAgreed: boolean
  guideDone: boolean
  /** 服务端有 ffmpeg 时才能把 MKV 等容器转封装播放 */
  remux: boolean
  /** 不具备转封装能力时的原因说明（用于提示用户） */
  remuxReason: string
}

export const useAppStore = defineStore('app', {
  state: () => ({
    status: null as Status | null,
    // 全局提示（对应 Tip.ets）
    tip: '',
    tipTimer: 0 as ReturnType<typeof setTimeout> | 0,
    // 订阅刷新信号：每次订阅成功后自增，HomeView watch 它来重载 Feed
    feedVersion: 0
  }),
  actions: {
    async loadStatus() {
      try {
        this.status = await api.status()
      } catch {
        /* ignore */
      }
    },
    showTip(msg: string) {
      if (!msg) return
      this.tip = msg
      if (this.tipTimer) clearTimeout(this.tipTimer)
      this.tipTimer = setTimeout(() => {
        this.tip = ''
        this.tipTimer = 0
      }, 1800)
    },
    // notifyFeedRefresh 通知需要刷新首页 Feed（订阅/站源变更后调用）
    notifyFeedRefresh() {
      this.feedVersion++
    }
  }
})
