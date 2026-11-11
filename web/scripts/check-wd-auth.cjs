// 播放链路回归：网盘需要 Basic Auth，认证必须透传到代理。
//
// 背景（v1.8.1 之前的 bug）：PlayerView 里写成
//     proxied(finalUrl, directMode ? player.header?.auth : undefined)
// 网盘播走的不是直链模式，auth 因此被丢掉 → 代理不带认证 → 上游 401
// → <video> 抛 MEDIA_ERR_SRC_NOT_SUPPORTED → 被误报成「视频解码失败」。
//
// 这个脚本盯两件事：
//   1. PlayerView 传给 proxied 的第二个参数不能出现 directMode 条件；
//   2. 网盘页设置 player.header 的地方确实带了 auth。
// 跑法：node scripts/check-wd-auth.cjs
const { readFileSync } = require('fs')
const path = require('path')

const ROOT = path.resolve(__dirname, '..')
const read = (p) => readFileSync(path.join(ROOT, p), 'utf8')

// 去掉 // 行注释与 /* */ 块注释，避免注释里提到的旧写法触发误判
function stripComments(code) {
  return code
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .split('\n')
    .map((l) => l.replace(/(^|[^:])\/\/.*$/, '$1'))
    .join('\n')
}

let failed = 0
const check = (ok, msg, detail) => {
  console.log((ok ? '  ✓ ' : '  ✗ ') + msg)
  if (!ok) {
    failed++
    if (detail) console.log('      ' + detail)
  }
}

console.log('[PlayerView] 代理 URL 必须透传认证')
const player = read('src/views/PlayerView.vue')

// 取出 proxiedFinal 函数体
const fn = player.match(/function proxiedFinal\(\)[^{]*\{([\s\S]*?)\n\}/)
if (!fn) {
  check(false, '找到 proxiedFinal 函数', '函数结构变了？请同步更新本脚本')
} else {
  const body = stripComments(fn[1])
  check(!/directMode/.test(body), '不再按 directMode 决定是否带 auth', '当前实现：' + body.trim())
  // 断言「传给 proxied 的第二个参数来自 header 的 auth」，
  // 不绑定具体写法（可能先取到局部变量 auth 再传）
  const usesHeaderAuth = /player\.header\?\.auth/.test(body)
  const passesAuthVar = /auth\s*=/.test(body) && /proxied\(\s*finalUrl\s*,\s*auth\s*\)/.test(body)
  check(usesHeaderAuth && (passesAuthVar || /proxied\(\s*finalUrl\s*,\s*player\.header\?\.auth/.test(body)),
    'proxied() 的 auth 参数来自 player.header.auth')
}

// 视频错误不能一律报「解码失败」
const onerror = player.match(/vid\.onerror\s*=\s*\(\)\s*=>\s*\{([\s\S]*?)\n  \}/)
if (onerror) {
  const b = stripComments(onerror[1])
  // 允许出现 onPlayError('mediaError')，但必须是被 if 条件包着的分支，
  // 不能是一进 onerror 就直接报解码失败
  const firstStmt = b.split('\n').map((l) => l.trim()).filter(Boolean)[0] || ''
  check(!/^onPlayError\('mediaError'\)/.test(firstStmt),
    '视频 error 不再一进来就报 mediaError', '首个语句：' + firstStmt)
  check(/MEDIA_ERR_NETWORK|MEDIA_ERR_DECODE|MediaError/.test(b),
    '区分了 MEDIA_ERR_* 错误码')
} else {
  check(false, '找到 vid.onerror', '结构变了？请同步更新本脚本')
}

console.log('\n[WebdavView] 播放入口必须设置 auth')
const webdav = read('src/views/WebdavView.vue')
const headerSets = webdav.match(/player\.header\s*=\s*\{[^}]*\}/g) || []
check(headerSets.length >= 2, `找到 ${headerSets.length} 处 player.header 赋值（进入剧集 + 继续观看）`)
check(headerSets.every((s) => /auth\s*:/.test(s)), '每处都包含 auth 字段')

console.log('\n[api.ts] proxied() 必须接受并拼上 auth')
const api = read('src/api.ts')
const px = api.match(/export function proxied\([^)]*\)[\s\S]*?\n\}/)
if (px) {
  const pxBody = stripComments(px[0])
  check(/auth/.test(pxBody) && /&auth=/.test(pxBody), 'proxied() 会把 auth 拼进 query')
} else {
  check(false, '找到 proxied() 实现')
}

console.log(failed === 0 ? '\n全部通过' : `\n${failed} 项失败`)
process.exit(failed === 0 ? 0 : 1)
