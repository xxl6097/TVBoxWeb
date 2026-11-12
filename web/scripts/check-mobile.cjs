#!/usr/bin/env node
/**
 * 设置页手机端布局回归检查（静态分析，不需要浏览器）。
 *
 * 覆盖线上真实出现过的问题：
 *  1. 设置行是 flex 单行 → 手机上标题被压到 32~96px、右侧只剩 8px
 *  2. .srow-right 硬编码 width:380px → 手机必然溢出
 *  3. 弹窗 width 写死 px → 窄屏溢出
 *  4. 访问口令输入框内联 max-width:260px → 手机上不跟随
 *  5. 关键断点缺失
 */
const { readFileSync } = require('fs')
const path = require('path')

const ROOT = path.resolve(__dirname, '..')
const read = (p) => readFileSync(path.join(ROOT, p), 'utf8')

function stripComments(code) {
  return code
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .split('\n')
    .map((l) => l.replace(/(^|[^:])\/\/.*$/, '$1'))
    .join('\n')
}

let failed = 0
let passed = 0
function check(ok, label) {
  if (ok) {
    passed++
    console.log('  ✓ ' + label)
  } else {
    failed++
    console.log('  ✗ ' + label)
  }
}

const src = read('src/views/SettingsView.vue')
const tpl = stripComments(src)

// 取出 @media (max-width: 640px) 块（用括号配对）
function extractMobileBlock(code) {
  const idx = code.indexOf('@media (max-width: 640px)')
  if (idx < 0) return ''
  let i = code.indexOf('{', idx)
  let depth = 0
  for (let j = i; j < code.length; j++) {
    if (code[j] === '{') depth++
    else if (code[j] === '}') {
      depth--
      if (depth === 0) return code.slice(i + 1, j)
    }
  }
  return ''
}
const mobile = extractMobileBlock(tpl)

console.log('\n[结构] 移动端断点与基础布局')
check(mobile.length > 0, '存在 @media (max-width: 640px) 断点')
check(/\.settings-layout\s*\{[^}]*flex-direction:\s*column/.test(mobile), '双栏转单栏（flex-direction: column）')
check(/\.settings-nav\s*\{[^}]*overflow-x:\s*auto/.test(mobile), '导航栏可横向滚动')

console.log('\n[关键] 设置行必须能换行（否则标题被挤压）')
check(
  /\.srow\s*\{[^}]*flex-wrap:\s*wrap/.test(mobile),
  '.srow 在手机上 flex-wrap: wrap（标题与内容分行）'
)
check(
  /\.srow-left\s*\{[^}]*flex:\s*1\s+1\s+100%/.test(mobile),
  '.srow-left 占满整行（flex: 1 1 100%）'
)
check(
  /\.srow-title\s*\{[^}]*word-break:\s*break-word/.test(mobile) ||
    /\.srow-title\s*\{[^}]*overflow-wrap/.test(mobile),
  '长标题允许折行，不截断'
)

console.log('\n[关键] 右侧内容不得硬编码宽度')
check(
  /\.srow-right\s*\{[^}]*width:\s*auto/.test(mobile),
  '.srow-right 在手机上 width: auto（覆盖桌面 380px）'
)
check(
  !/\.srow-right\s*\{[^}]*width:\s*\d+px/.test(mobile),
  '.srow-right 在手机上不再用固定 px 宽度'
)

console.log('\n[关键] 输入框与按钮铺满')
check(
  /\.srow-right\s+\.el-input[^{]*\{[^}]*width:\s*100%/.test(mobile),
  '右侧输入框宽度 100%'
)
check(/\.token-input\s*\{[^}]*max-width:\s*100%/.test(mobile), '访问口令输入框在手机上 max-width:100%')
check(!/style="max-width:\s*260px"/.test(tpl), '模板中不再有内联 max-width:260px')

console.log('\n[关键] 弹窗宽度响应式')
const dialogs = [...tpl.matchAll(/<el-dialog[^>]*>/g)].map((m) => m[0])
check(dialogs.length > 0, '找到 el-dialog')
for (const d of dialogs) {
  const fixed = /width="\d+px"/.test(d)
  check(!fixed, '弹窗宽度不用固定 px：' + (d.match(/width="[^"]*"/) || ['(无 width)'])[0])
  check(/width="min\(/.test(d) || /width="\d+%"/.test(d) || !/width=/.test(d), '弹窗宽度用 min()/百分比（' + (d.match(/title="[^"]*"/) || [''])[0] + '）')
}

console.log('\n[细节] 触控与可读性')
check(
  /\.nav-item\s*\{[^}]*min-height:\s*4[0-9]px/.test(mobile),
  '导航项高度 ≥40px（触控友好）'
)
check(
  /\.theme-chips\s+\.chip\s*\{[^}]*min-height/.test(mobile),
  '主题切换按钮加大触控区'
)
check(/\.srow-url\s*\{[^}]*word-break:\s*break-all/.test(mobile), '长 URL 允许折行（不再单行截断）')
check(/\.srow-title:first-child/.test(mobile), '只有标题+箭头的行保持单行（箭头贴右）')
check(/:has\(> \.srow-left\)/.test(mobile), '带副标题的行箭头绝对定位（不掉到第二行）')

console.log('\n[清理] 无重复样式定义')
const count = (re) => (tpl.match(re) || []).length
check(count(/^\.scan-label\s*\{/gm) === 1, '.scan-label 只定义一次')
check(count(/^\.scan-cached\s*\{/gm) === 1, '.scan-cached 只定义一次')

console.log('\n' + (failed === 0 ? '全部通过' : `${failed} 项失败`) + `（${passed} 通过）`)
process.exit(failed === 0 ? 0 : 1)
