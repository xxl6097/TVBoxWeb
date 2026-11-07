// 清晰度工具（与后端 quality 包一致）。

export interface QLevel {
  height: number
  width: number
  bps: number
  url: string
  name: string
}

export const AUTO = 0
export const TOP = 9999

export function labelOf(h: number, bps: number): string {
  if (h >= 2160) return '4K'
  if (h >= 1440) return '2K'
  if (h >= 1080) return '1080P'
  if (h >= 720) return '720P'
  if (h >= 540) return '540P'
  if (h >= 480) return '480P'
  if (h >= 360) return '360P'
  if (bps >= 4000000) return '高清'
  if (bps >= 1500000) return '标清'
  return bps > 0 ? '流畅' : '原画'
}

export function labelOfLevel(l: QLevel): string {
  const s = labelOf(l.height, l.bps)
  if (s === '原画' && l.name) return l.name
  const vague = s === '流畅' || s === '标清' || s === '高清'
  if (vague && l.height > 0) return l.height + 'P'
  if (vague && l.bps > 0) return s + ' ' + Math.round(l.bps / 1000) + 'K'
  return s
}

export function pickByCap(cap: number, levels: QLevel[]): number {
  if (!levels.length) return -1
  if (cap >= TOP) return levels.length - 1
  let best = -1
  levels.forEach((l, i) => {
    if (l.height <= 0 || l.height <= cap) best = i
  })
  return best < 0 ? 0 : best
}

export function pickBySpeed(kbps: number, levels: QLevel[]): number {
  if (!levels.length) return -1
  if (kbps <= 0) return 0
  const budget = kbps * 0.7 * 1000
  let best = -1
  levels.forEach((l, i) => {
    const need = l.bps > 0 ? l.bps : l.height > 0 ? l.height * 4000 : 0
    if (need <= budget) best = i
  })
  return best < 0 ? 0 : best
}

// 从 hls.js levels 映射到 QLevel
export function fromHlsLevels(levels: any[]): QLevel[] {
  return levels.map((lv) => ({
    height: lv.height || 0,
    width: lv.width || 0,
    bps: lv.bitrate || 0,
    url: lv.url || '',
    name: lv.name || lv.attrs?.NAME || ''
  }))
}
