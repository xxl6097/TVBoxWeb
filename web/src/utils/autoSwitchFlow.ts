/**
 * 失效记录的自动换源入口（HomeView / HistoryView 共用）。
 *
 * 为什么抽出来：两处原来各写了一遍「判断源站是否失效 → 跳转」，
 * 加了自动换源后逻辑更重（异步搜索 + 换源 + 带进度跳播放页），
 * 再复制一份必然会漂移。
 */
import { autoSwitchSource, buildPlayQuery } from '@/utils/autoSwitch'
import { usePlayerStore } from '@/stores/player'
import type { HistItem } from '@/types'

export interface SwitchDeps {
  push: (to: any) => void
  tip: (msg: string) => void
  /** 用来防止连点：同一个 key 正在换源时忽略后续点击 */
  switchingKey: { value: string }
}

/**
 * 自动换源并续播。
 * 成功 → 直接跳播放页（带 ?pos= 回到上次位置）；
 * 失败 → 降级到搜索页，并说明原因，不让用户卡在原地。
 */
export async function autoSwitchAndPlay(h: HistItem, deps: SwitchDeps): Promise<void> {
  const key = h.siteKey + '|' + h.vodId
  if (deps.switchingKey.value === key) return // 连点保护
  deps.switchingKey.value = key
  deps.tip('正在寻找可用的源…')

  try {
    const r = await autoSwitchSource(h)
    if (r.ok && r.target) {
      const t = r.target
      // 先把整条播放列表塞进 store，播放页才有「上一集/下一集」
      const player = usePlayerStore()
      player.set(
        t.line.eps.map((e) => ({ name: e.name, url: e.url, urls: [...e.urls] })),
        t.epIndex,
        t.vod.name,
        false,
        false
      )
      deps.push({ name: 'play', query: buildPlayQuery(t, t.posMs) })
      const epInfo = t.ep.name ? ` ${t.ep.name}` : ''
      deps.tip(
        t.posMs > 0
          ? `已换到「${t.siteName}」${epInfo}，从上次位置继续`
          : `已换到「${t.siteName}」${epInfo}`
      )
      return
    }
    // 换不了：说清原因，并送去搜索页让用户自己挑
    deps.tip(r.reason || '没找到可用的源')
    deps.push({ name: 'search', query: r.fallbackKw ? { kw: r.fallbackKw } : {} })
  } catch {
    deps.tip('换源失败，已转到搜索')
    deps.push({ name: 'search', query: { kw: h.name } })
  } finally {
    deps.switchingKey.value = ''
  }
}
