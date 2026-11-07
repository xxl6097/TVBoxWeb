// 时间/文案格式化工具。

export function fmtMs(ms: number): string {
  if (ms <= 0) return '00:00'
  const total = Math.floor(ms / 1000)
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  const mm = (m < 10 ? '0' : '') + m
  const ss = (s < 10 ? '0' : '') + s
  return h > 0 ? `${h}:${mm}:${ss}` : `${mm}:${ss}`
}

export function fmtTime(ms: number): string {
  if (ms <= 0) return '还没更新过'
  const d = new Date(ms)
  const mo = d.getMonth() + 1 < 10 ? '0' + (d.getMonth() + 1) : '' + (d.getMonth() + 1)
  const da = d.getDate() < 10 ? '0' + d.getDate() : '' + d.getDate()
  const h = d.getHours() < 10 ? '0' + d.getHours() : '' + d.getHours()
  const mi = d.getMinutes() < 10 ? '0' + d.getMinutes() : '' + d.getMinutes()
  return `${mo}-${da} ${h}:${mi}`
}

export function fmtSize(n: number): string {
  if (n <= 0) return ''
  if (n >= 1024 * 1024 * 1024) return (n / 1024 / 1024 / 1024).toFixed(1) + ' GB'
  if (n >= 1024 * 1024) return (n / 1024 / 1024).toFixed(1) + ' MB'
  if (n >= 1024) return (n / 1024).toFixed(0) + ' KB'
  return n + ' B'
}
