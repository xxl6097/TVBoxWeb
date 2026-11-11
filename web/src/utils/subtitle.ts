// 字幕格式转换：SRT / ASS / SSA → WebVTT。
// 纯函数，不依赖 DOM，便于单测与复用。

// ASS/SSA 时间轴 `H:MM:SS.cc` → WebVTT 的 `HH:MM:SS.mmm`
export function assTime(t: string): string {
  const m = t.trim().match(/^(\d+):(\d{2}):(\d{2})(?:\.(\d{1,2}))?$/)
  if (!m) return ''
  return `${m[1].padStart(2, '0')}:${m[2]}:${m[3]}.${(m[4] || '0').padEnd(3, '0')}`
}

// ASS/SSA 只抽取 Dialogue 对白文本（忽略特效标签），基础可用。
export function assToVtt(text: string): string {
  const cues: string[] = ['WEBVTT', '']
  for (const line of text.split('\n')) {
    if (!/^Dialogue:/i.test(line)) continue
    const p = line.replace(/^Dialogue:\s*/i, '').split(',')
    if (p.length < 10) continue
    const st = assTime(p[1])
    const sp = assTime(p[2])
    const body = p
      .slice(9)
      .join(',')
      .replace(/\{[^}]*\}/g, '')
      .replace(/\\[Nn]/g, '\n')
      .trim()
    if (st && sp && body) cues.push(`${st} --> ${sp}\n${body}\n`)
  }
  return cues.join('\n') + '\n'
}

// 任意常见字幕文本 → WebVTT。
// 已带 WEBVTT 头的原样返回；ASS/SSA 走 Dialogue 抽取；其余按 SRT 处理
// （时间码逗号改句点、去掉纯数字序号行）。
export function toWebVtt(text: string): string {
  const clean = text.replace(/^\uFEFF/, '').replace(/\r\n?/g, '\n')
  if (/\[Script Info\]/i.test(clean)) return assToVtt(clean)
  if (/^\s*WEBVTT/i.test(clean)) return clean
  const body = clean
    .replace(/(\d{2}:\d{2}:\d{2}),(\d{3})/g, '$1.$2')
    .replace(/^\s*\d+\s*$/gm, '')
  return 'WEBVTT\n\n' + body.trim() + '\n'
}
