import type { Line, Episode } from '@/types'

// 与后端 cms.SplitLines 一致：vod_play_from / vod_play_url → 线路与剧集。
export function splitLines(from: string, playUrl: string): Line[] {
  const out: Line[] = []
  if (!playUrl) return out
  const fArr = from ? from.split('$$$') : []
  const uArr = playUrl.split('$$$')
  uArr.forEach((u, i) => {
    const line: Line = {
      name: i < fArr.length && fArr[i] ? fArr[i] : `线路 ${i + 1}`,
      direct: false,
      eps: []
    }
    u.split('#').forEach((seg, k) => {
      seg = seg.trim()
      if (!seg) return
      let name = `第 ${k + 1} 集`
      let url = seg
      const p = seg.indexOf('$')
      if (p > 0) {
        name = seg.slice(0, p).trim()
        url = seg.slice(p + 1).trim()
      }
      if (url && url.includes('://')) {
        line.eps.push({ name, url, urls: [url] })
      }
    })
    let directCount = 0
    line.eps.forEach((e) => {
      if (isDirect(e.url)) directCount++
    })
    line.direct = line.eps.length > 0 && directCount * 2 >= line.eps.length
    if (line.eps.length) out.push(line)
  })
  return out
}

export function isDirect(url: string): boolean {
  const u = url.toLowerCase()
  return (
    u.includes('.m3u8') ||
    u.includes('.mp4') ||
    u.includes('.flv') ||
    u.includes('.mkv') ||
    u.includes('.ts') ||
    u.includes('.m4v')
  )
}
