/**
 * 自动换源：历史记录里的源站失效后，自动在现存源里找到同一部片、
 * 定位到同一集，并把原来的播放进度带过去。
 *
 * 为什么需要单独一套匹配逻辑，而不是直接用搜索结果第一条：
 * - 搜「三十而已」可能返回「三十而已」「三十而已 花絮」「三十而已 番外」，
 *   也可能返回别的剧里带这几个字的条目；
 * - 同一个片名在不同源里的集名格式五花八门（「第01集」「01」「Ep01」「第一集」），
 *   不能靠字符串相等去对；
 * - 搜不到时不能死循环，必须能明确告诉调用方「换不了」。
 *
 * 所以这里做三件事：① 按片名相似度选最优候选；② 按集序号/集名双路兜底定位；
 * ③ 全过程可中断（用户连点、离开页面）。
 */
import { api } from '@/api'
import { splitLines } from '@/utils/lines'
import type { HistItem, Vod, Line, Episode } from '@/types'

export interface SwitchTarget {
  /** 新源站点 key */
  siteKey: string
  siteName: string
  /** 新源的影片 id */
  vodId: string
  /** 匹配到的影片 */
  vod: Vod
  /** 命中线路 */
  line: Line
  /** 该集在线路里的真实下标 */
  epIndex: number
  /** 该集 */
  ep: Episode
  /** 带过去的播放进度（毫秒），0 表示从头播 */
  posMs: number
  /** 片名相似度（0~1），用于提示「已换到 xx 源」 */
  score: number
}

export interface SwitchResult {
  ok: boolean
  target?: SwitchTarget
  /** 命中候选的片名相似度（0~1），便于调用方决定提示语气 */
  score?: number
  /** 失败原因，直接可展示 */
  reason?: string
  /** 搜到但匹配不上时，给出片名供用户手动搜 */
  fallbackKw?: string
}

/** 归一化片名：去空格、标点、全角转半角，便于比较 */
function norm(s: string): string {
  return (s || '')
    .replace(/[\uff01-\uff5e]/g, (c) => String.fromCharCode(c.charCodeAt(0) - 0xfee0))
    .replace(/[\s\-_·:：,，.。!！?？'"“”‘’()（）[\]【】《》<>]/g, '')
    .toLowerCase()
}

/**
 * 片名相似度 0~1。
 * 完全相等给 1；包含关系给 0.8~0.95（短的那个是长的的前缀时更像同一部）；
 * 其余按字符重合度给分。刻意不给「包含即满分」——
 * 「三十而已」和「三十而已花絮」必须能区分开。
 */
export function titleScore(a: string, b: string): number {
  const x = norm(a)
  const y = norm(b)
  if (!x || !y) return 0
  if (x === y) return 1
  const [shorter, longer] = x.length <= y.length ? [x, y] : [y, x]
  if (longer.includes(shorter)) {
    // 短名是长名的一部分：越接近越长越像同一部
    const ratio = shorter.length / longer.length
    // 长名多出「花絮/预告/解说」这类后缀时进一步降权
    if (/花絮|预告|解说|番外|剪辑|片段|抢先|特辑|幕后/.test(longer)) return 0.35
    return 0.8 + 0.15 * ratio
  }
  // 字符集合重合度（Dice 系数）
  const setA = new Set(x.split(''))
  const setB = new Set(y.split(''))
  let inter = 0
  setA.forEach((c) => {
    if (setB.has(c)) inter++
  })
  return (2 * inter) / (setA.size + setB.size)
}

/**
 * 从集名里抽出集号，用于跨源对齐。
 * 兼容：第01集 / 第1集 / 01 / 1 / EP01 / E01 / 01-xxx / 第十二集（中文数字暂不转）。
 */
export function epNumber(name: string): number | null {
  const s = (name || '').trim()
  if (!s) return null
  // 优先「第N集/期/话/篇」
  let m = s.match(/第\s*(\d+)\s*[集期话篇回]/)
  if (m) return parseInt(m[1], 10)
  // EP01 / E01 / EP.01
  m = s.match(/\b(?:ep|e)\.?\s*(\d+)\b/i)
  if (m) return parseInt(m[1], 10)
  // 兜底：取第一段连续数字
  m = s.match(/(\d+)/)
  if (m) return parseInt(m[1], 10)
  return null
}

/**
 * 在给定线路里定位「同一集」。
 * 优先集号对齐（跨源最可靠），其次集名归一化后相等，最后退回原下标。
 */
export function locateEpisode(eps: Episode[], wantIndex: number, wantName: string): number {
  if (!eps.length) return -1
  const wantNum = epNumber(wantName)
  if (wantNum !== null) {
    const byNum = eps.findIndex((e) => epNumber(e.name) === wantNum)
    if (byNum >= 0) return byNum
  }
  const wn = norm(wantName)
  if (wn) {
    const byName = eps.findIndex((e) => norm(e.name) === wn)
    if (byName >= 0) return byName
  }
  // 集号对不上（有的源把第一集标成「正片」）：下标在范围内就用原下标
  if (wantIndex >= 0 && wantIndex < eps.length) return wantIndex
  return eps.length ? 0 : -1
}

/** 挑一条可用线路：优先有直链的，其次集数最多的（通常最完整） */
function pickLine(lines: Line[]): Line | null {
  if (!lines.length) return null
  const withEps = lines.filter((l) => l.eps.length > 0)
  if (!withEps.length) return null
  return withEps.slice().sort((a, b) => {
    if (a.direct !== b.direct) return a.direct ? -1 : 1
    return b.eps.length - a.eps.length
  })[0]
}

/** 候选里排除「明显不是同一部」的（花絮/预告/解说等） */
function isNoise(name: string): boolean {
  return /花絮|预告|解说|番外|剪辑|片段|抢先|特辑|幕后|合集|盘点/.test(name)
}

/**
 * 主流程：给一条失效的历史记录，找一个能接着播的新源。
 *
 * @param signal 可选中断信号（用户二次点击、组件卸载时用）
 */
export async function autoSwitchSource(
  h: HistItem,
  signal?: AbortSignal
): Promise<SwitchResult> {
  const kw = (h.name || '').trim()
  if (!kw) return { ok: false, reason: '这条记录没有片名，无法自动换源' }

  // 1) 按片名搜全部活跃源
  let list: Vod[] = []
  try {
    const r = await api.search(kw, signal)
    list = r.list || []
  } catch (e: any) {
    if (e?.name === 'AbortError') return { ok: false, reason: '已取消' }
    return { ok: false, reason: '搜索失败，请检查网络后重试', fallbackKw: kw }
  }

  // 2) 排除原站点（已经失效没必要再试）和噪音条目，按相似度排序
  const cands = list
    .filter((v) => v.siteKey !== h.siteKey)
    .filter((v) => !isNoise(v.name))
    .map((v) => ({ v, score: titleScore(h.name, v.name) }))
    // 0.6 以下基本是「沾了个字」，不值得试
    .filter((x) => x.score >= 0.6)
    .sort((a, b) => b.score - a.score)

  if (!cands.length) {
    return {
      ok: false,
      reason: `没在现存的源里找到「${kw}」`,
      fallbackKw: kw
    }
  }

  // 3) 逐个试，直到拿到能定位到剧集的线路。
  //    只试前几个，避免用户等太久。
  for (const { v, score } of cands.slice(0, 3)) {
    try {
      const detail = await api.vodDetail(v.siteKey, v.id)
      const lines = splitLines(detail.from, detail.playUrl)
      const line = pickLine(lines)
      if (!line) continue

      const idx = locateEpisode(line.eps, h.epIndex, h.epName)
      if (idx < 0) continue

      return {
        ok: true,
        score,
        target: {
          siteKey: v.siteKey,
          siteName: v.siteName || detail.siteName || v.siteKey,
          vodId: v.id,
          vod: detail,
          line,
          epIndex: idx,
          ep: line.eps[idx],
          score,
          // 进度只在「同一集」时才有意义：集号对不上说明可能串集了，
          // 这种情况从这一集开头播，比跳到一半更安全。
          posMs: sameEpisode(h, line.eps[idx]) ? Math.max(0, h.pos || 0) : 0
        }
      }
    } catch {
      // 这个源详情拉不到，换下一个
      continue
    }
  }

  return {
    ok: false,
    reason: `找到了「${kw}」，但都没法定位到第 ${h.epIndex + 1} 集`,
    fallbackKw: kw
  }
}

/** 新源的这一集是否就是原来那一集（集号相同即视为同一集） */
function sameEpisode(h: HistItem, ep: Episode): boolean {
  const a = epNumber(h.epName || '')
  const b = epNumber(ep.name || '')
  if (a !== null && b !== null) return a === b
  // 集名都抽不出数字时，只能认为「下标一致即同一集」
  return true
}

/** 把换源结果拼成播放页需要的路由参数 */
export function buildPlayQuery(t: SwitchTarget, posMs: number) {
  return {
    url: t.ep.url,
    title: `${t.vod.name} ${t.ep.name}`,
    site: t.siteKey,
    id: t.vodId,
    name: t.vod.name,
    pic: t.vod.pic || '',
    siteName: t.siteName,
    from: '',
    epIndex: String(t.epIndex),
    vodTitle: t.vod.name,
    lineIdx: '0',
    direct: t.line.direct ? '1' : '0',
    // 跨页把进度带过去：新源没有这条历史，播放页查不到
    pos: String(Math.max(0, Math.floor(posMs)))
  }
}
