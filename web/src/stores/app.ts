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
}

export const useAppStore = defineStore('app', {
  state: () => ({
    status: null as Status | null,
    // 全局提示（对应 Tip.ets）
    tip: '',
    tipTimer: 0 as ReturnType<typeof setTimeout> | 0
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
    }
  }
})
