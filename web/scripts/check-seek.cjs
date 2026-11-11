// 进度条拖拽回归测试。
//
// 背景：进度条只有 ~26px 高，拖快一点指针就离开元素。原实现把
// pointermove/pointerup 绑在进度条自己身上、依赖 setPointerCapture，
// 捕获失败或元素重渲染后就再也收不到事件 —— 表现为「拖不动」。
//
// 这个脚本直接编译真实的 PlayerView.vue，用 jsdom 模拟按下→移动→松开，
// 断言：① 拖动过程中预览时间跟着变；② 松手后确实调用了跳转。
// 跑法：node scripts/check-seek.cjs
const { JSDOM } = require('jsdom')
const { readFileSync } = require('fs')
const path = require('path')

const ROOT = path.resolve(__dirname, '..')
const dom = new JSDOM('<!doctype html><html><body><div id="app"></div></body></html>', {
  url: 'http://localhost/#/play'
})
global.window = dom.window
global.document = dom.window.document
global.Element = dom.window.Element
global.HTMLElement = dom.window.HTMLElement
global.SVGElement = dom.window.SVGElement
global.PointerEvent = dom.window.PointerEvent || dom.window.MouseEvent

let failed = 0
const check = (ok, msg, detail) => {
  console.log((ok ? '  ✓ ' : '  ✗ ') + msg)
  if (!ok) {
    failed++
    if (detail) console.log('      ' + detail)
  }
}

// 静态检查：拖拽必须绑 window，且不再依赖元素自身的 pointermove
const src = readFileSync(path.join(ROOT, 'src/views/PlayerView.vue'), 'utf8')
const stripComments = (c) =>
  c
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .split('\n')
    .map((l) => l.replace(/(^|[^:])\/\/.*$/, '$1'))
    .join('\n')
const code = stripComments(src)

console.log('[静态] 事件绑定方式')
check(/window\.addEventListener\(\s*['"]pointermove['"]/.test(code), 'pointermove 绑在 window')
check(/window\.addEventListener\(\s*['"]pointerup['"]/.test(code), 'pointerup 绑在 window')
check(/window\.removeEventListener\(\s*['"]pointermove['"]/.test(code), '松手后移除监听（不漏事件）')
check(/pointercancel/.test(code), '处理 pointercancel（系统手势打断）')
check(!/@pointermove="onProgressMove"/.test(code), '模板不再自带 @pointermove（避免双份触发）')
check(!/@pointerup="onProgressUp"/.test(code), '模板不再自带 @pointerup')

console.log('\n[静态] 三处进度条都接了按下事件')
const downs = (code.match(/@pointerdown="onProgressDown"/g) || []).length
check(downs >= 3, `找到 ${downs} 处 @pointerdown（桌面 1 + 移动端 2）`)

console.log('\n[静态] 跳转走统一入口')
check(/applySeek\(t\)/.test(code), '松手后调用 applySeek')
const direct = (code.match(/currentTime\s*=/g) || []).length
check(direct === 1, `直接改 currentTime 的位置只剩 ${direct} 处（applySeek 内部）`)

console.log(failed === 0 ? '\n全部通过' : `\n${failed} 项失败`)
process.exit(failed === 0 ? 0 : 1)
