// 网盘手机端回归：弹窗宽度、亮度遮罩、历史合并、画面比例默认值。
//
// 背景（用户报障）：
//   ① 网盘视频有时闪成纯色 —— 左半屏亮度手势的黑色遮罩上限 0.7，
//      误触后画面几乎全黑，且没有复位入口；
//   ② 进度条还是有两条；
//   ③ 「继续观看」里没有网盘播放记录 —— 网盘历史和点播历史是两套存储，
//      首页/历史页只读 /api/history，永远看不到网盘那套；
//   ④ 选集弹窗没有适配手机屏幕 —— width 写死 480px，390 屏溢出 90px。
//
// 跑法：node scripts/check-webdav-mobile.cjs
const { readFileSync } = require('fs')
const path = require('path')

const ROOT = path.resolve(__dirname, '..')
const read = (p) => readFileSync(path.join(ROOT, p), 'utf8')
const stripComments = (c) =>
  c.replace(/\/\*[\s\S]*?\*\//g, '').split('\n')
    .map((l) => l.replace(/(^|[^:])\/\/.*$/, '$1')).join('\n')

let failed = 0
function expect(cond, msg, detail) {
  console.log((cond ? '  ✓ ' : '  ✗ ') + msg)
  if (!cond) {
    failed++
    if (detail) console.log('      ' + detail)
  }
}

const wd = stripComments(read('src/views/WebdavView.vue'))
const gestures = stripComments(read('src/composables/usePlayerGestures.ts'))
const player = stripComments(read('src/views/PlayerView.vue'))
const home = stripComments(read('src/views/HomeView.vue'))
const history = stripComments(read('src/views/HistoryView.vue'))

console.log('[1] 网盘弹窗宽度响应式')
{
  // 只挑 el-dialog 上的 width=，label-width="80px" 这类表单布局不算
  const dialogs = wd.match(/<el-dialog[^>]*>/g) || []
  const fixed = dialogs.filter((d) => /width="\d+px"/.test(d))
  expect(fixed.length === 0, `弹窗没写死 px 宽度（发现 ${fixed.length} 处）`, fixed.join('\n      '))
  const mins = dialogs.filter((d) => /width="min\(/.test(d))
  expect(mins.length >= 2, `选集弹窗与源表单弹窗都用 min()（${mins.length}/${dialogs.length}）`)
}

console.log('\n[2] 亮度遮罩不会把画面压成纯色')
{
  expect(/const BRIGHT_MAX = 0\.55/.test(gestures), '亮度上限收到 0.55（原来 0.7 一划就七成黑）')
  expect(/function resetBrightness/.test(gestures), '有 resetBrightness()')
  expect(/\bBRIGHT_MAX\b/.test(gestures) && !/Math\.min\(0\.7,/.test(gestures),
    '不再有硬编码 0.7')
  expect(/resetBrightness,/.test(gestures) || /return \{[^}]*resetBrightness/.test(gestures),
    'resetBrightness 已导出')
  const pv = player
  const resetCalls = (pv.match(/resetBrightness\(\)/g) || []).length
  expect(resetCalls >= 2, `切集/换源都会复位亮度（找到 ${resetCalls} 处）`)
  // 双击左半屏恢复
  expect(/brightness\.value > 0\.05[\s\S]{0,120}?resetBrightness\(\)/.test(gestures),
    '双击左半屏可恢复亮度（误触后的救急入口）')
}

console.log('\n[3] 横向滑动 seek 必须走 applySeek')
{
  expect(/host\.applySeek\(newMs \/ 1000\)/.test(gestures),
    '横滑结束调用 host.applySeek')
  expect(!/videoEl\.value\.currentTime = newMs/.test(gestures),
    '不再直接改 currentTime（转封装流会失效/重载，表现为画面黑一下）')
  expect(/applySeek: \(t: number\) => void/.test(gestures), 'GestureHost 声明了 applySeek')
  expect(/applySeek,\s*\n\s*togglePlay/.test(player) || /^\s*applySeek,$/m.test(player),
    'PlayerView 把手势 host 的 applySeek 接上了')
}

console.log('\n[4] 画面比例默认不留拉伸')
{
  expect(/const scaleMode = ref\(2\)/.test(player),
    '默认 scaleMode = 2（等比适应），不再是 0（fill 拉伸）')
  expect(!/const scaleMode = ref\(0\)/.test(player), '没有残留的 ref(0) 声明')
  expect(/if \(scaleMode\.value === 0\) return 'fill'/.test(player),
    '仍保留「拉伸铺满」选项供手动选择')
}

console.log('\n[5] 网盘历史合并进「继续观看」')
{
  expect(/api\.webdavHistory\(\)/.test(home), '首页读取网盘历史')
  expect(/siteKey: 'webdav'/.test(home), '网盘记录带 siteKey=webdav 标记')
  expect(/api\.webdavHistory\(\)/.test(history), '历史页也读取网盘历史')
  expect(/siteKey === 'webdav'/.test(home), '首页点击网盘记录走网盘分支')
  expect(/siteKey === 'webdav'/.test(history), '历史页点击网盘记录走网盘分支')
  expect(/continue: h\.vodId/.test(home), '首页跳转带 continue 参数')
  expect(/clearWebdavHistory/.test(home), '首页「清空」同时清网盘历史')
  expect(/clearWebdavHistory/.test(history), '历史页「清空」同时清网盘历史')
  expect(/removeWdHistory/.test(home), '首页删除单条时调网盘删除接口')
  expect(/removeWdHistory/.test(history), '历史页删除单条时调网盘删除接口')
}

console.log('\n[6] 网盘页支持 ?continue= 自动续播')
{
  expect(/route\.query\.continue/.test(wd), '网盘页解析 continue 参数')
  expect(/route\.query\.continue[\s\S]{0,400}?await continueWatch\(h\)/.test(wd),
    '命中记录后直接续播')
}

console.log(failed === 0 ? '\n全部通过' : `\n${failed} 项失败`)
process.exit(failed === 0 ? 0 : 1)
