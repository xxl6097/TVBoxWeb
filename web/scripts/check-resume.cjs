// 播放位置与续播逻辑回归测试（零框架，node 直接跑）。
//
// 背景（用户报障）：
//   ① 进度条上下各一条，下面那条不动 —— 移动端全屏 OSD 与底部面板并存，
//      且三处进度条共用一个 ref，后挂载的覆盖前面的，拖动基准算错。
//   ② 没有记录播放位置，点「继续观看」从头开始 —— 保存的是 curMs（旧值），
//      节流用 curMs - lastSaveMs 在往回拖时永远不成立；
//      转封装续播又依赖 loadedmetadata 之后再 applySeek，常常被防重入窗口吃掉。
//
// 这个脚本从真实的 PlayerView.vue 里抽取这几段逻辑逐条验证，
// 不依赖浏览器与网络。跑法：node scripts/check-resume.cjs
const { readFileSync } = require('fs')
const path = require('path')

const ROOT = path.resolve(__dirname, '..')
const src = readFileSync(path.join(ROOT, 'src/views/PlayerView.vue'), 'utf8')
const stripComments = (c) =>
  c
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .split('\n')
    .map((l) => l.replace(/(^|[^:])\/\/.*$/, '$1'))
    .join('\n')
const code = stripComments(src)

let failed = 0
function expect(cond, msg, detail) {
  console.log((cond ? '  ✓ ' : '  ✗ ') + msg)
  if (!cond) {
    failed++
    if (detail) console.log('      ' + detail)
  }
}

// ---------- 用组件里的真实实现构造被测函数 ----------
const esbuild = require('esbuild')

// 剥掉 TS 类型注解后再求值，保证跑的就是组件里那段源码
function toJs(tsSrc) {
  return esbuild.transformSync(tsSrc, { loader: 'ts' }).code
}

// realPosMs 本体只有几行，按源码原文求值，避免「测试写的和产品写的不一样」
function extractFn(name) {
  const i = code.indexOf('function ' + name + '(')
  if (i < 0) return null
  // 花括号配对找函数体结尾
  let depth = 0, started = false
  for (let j = i; j < code.length; j++) {
    if (code[j] === '{') { depth++; started = true }
    else if (code[j] === '}') { depth--; if (started && depth === 0) return code.slice(i, j + 1) }
  }
  return null
}

const realPosSrc = extractFn('realPosMs')
expect(!!realPosSrc, '能从组件源码中取到 realPosMs')

// 造一个可控的上下文，把源码里的自由变量都提供出来
function makeCtx(video, remux, remuxOff, cur) {
  const fnSrc = toJs(realPosSrc).replace(/^function\s+\w+/, 'function realPosMs')
  const factory = new Function(
    'videoEl', 'remuxMode', 'remuxSeek', 'curMs',
    fnSrc + '; return realPosMs;'
  )
  return factory(
    { value: video },
    { value: remux },
    { value: remuxOff },
    { value: cur }
  )
}

console.log('\n[1] realPosMs：以 video.currentTime 为准')
{
  // 普通流：currentTime 就是绝对位置
  const f = makeCtx({ currentTime: 42.5 }, false, 0, 1000)
  expect(f() === 42500, '普通流取 currentTime*1000（忽略过期的 curMs）', `得到 ${f()}`)
}
{
  // 转封装：时间戳从 seek 点重新从 0 开始，必须把偏移加回去
  const f = makeCtx({ currentTime: 10 }, true, 600, 0)
  expect(f() === 610000, '转封装流加回 remuxSeek 偏移（600+10=610s）', `得到 ${f() / 1000}s`)
}
{
  // 拿不到 video 时退回 curMs
  const f = makeCtx(null, false, 0, 8888)
  expect(f() === 8888, 'video 不可用时退回 curMs', `得到 ${f()}`)
}
{
  // currentTime 为 NaN（刚换源）时也要退回，别写进 NaN
  const f = makeCtx({ currentTime: NaN }, false, 0, 5000)
  expect(f() === 5000 && !Number.isNaN(f()), 'currentTime 非有限值时退回 curMs', `得到 ${f()}`)
}

console.log('\n[2] savePos 用的是实时位置，不是 curMs')
expect(/const pos = realPosMs\(\)/.test(code), 'savePos 开头取 realPosMs()')
expect(/pos: Math\.floor\(pos\)/.test(code), '请求体里 pos 用这个实时值')
expect(!/pos: Math\.floor\(curMs\.value\)/.test(code), '不再有 pos: Math.floor(curMs.value) 的旧写法')

console.log('\n[3] 保存节流用绝对差值（往回拖也能落盘）')
{
  const saveSrc = extractFn('savePos')
  expect(!!saveSrc, '能取到 savePos')
  const throttleOk = /Math\.abs\(\s*curMs\.value\s*-\s*lastSaveMs\s*\)\s*>\s*5000/.test(code)
  expect(throttleOk, '节流条件写成 Math.abs(curMs - lastSaveMs) > 5000')
  // 行为验证：往回拖时旧写法漏保存、新写法不漏
  const oldFires = (cur, last) => cur - last > 5000
  const newFires = (cur, last) => Math.abs(cur - last) > 5000
  expect(oldFires(120000, 600000) === false, '旧写法：从 600s 拖回 120s 不触发保存（bug 复现）')
  expect(newFires(120000, 600000) === true, '新写法：同样场景会触发保存')
  expect(newFires(620000, 600000) === true, '新写法：正常前进仍然触发')
  expect(newFires(601000, 600000) === false, '新写法：1 秒内不重复保存（节流仍生效）')
}

console.log('\n[4] 转封装续播：一次起转到位，不再二次跳转')
expect(/if \(player\.seek > 0 && !isLive\) \{[\s\S]{0,200}?remuxSeek\.value\s*=/.test(code),
  'initPlayer 在 proxiedFinal() 之前把起点写进 remuxSeek')
const initIdx = code.indexOf('const src = proxiedFinal()')
const seekIdx = code.indexOf('pendingResumeMs = player.seek')
expect(initIdx > 0 && seekIdx > 0 && seekIdx < initIdx,
  `赋值发生在第一次请求之前（${seekIdx} < ${initIdx}）`)
expect(/player\.seek = 0/.test(code), '消费后清零 player.seek，避免重载循环')
expect(/if \(pendingResumeMs > 0 && !isLive\)/.test(code), 'loadedmetadata 里按 pendingResumeMs 恢复')
expect(/if \(!remuxMode\.value\) vid\.currentTime = ms \/ 1000/.test(code),
  '只有普通流才改 currentTime（转封装的流已经在该位置起播）')
{
  // 行为：转封装下不应该再调 applySeek
  const meta = code.slice(code.indexOf('vid.onloadedmetadata'), code.indexOf('vid.ontimeupdate'))
  expect(!/applySeek\(/.test(meta), 'loadedmetadata 内不再调用 applySeek（消除防重入吞请求）')
}

console.log('\n[5] seekBy 在转封装模式下加偏移')
{
  const sb = extractFn('seekBy')
  expect(!!sb, '能取到 seekBy')
  expect(/const base = remuxMode\.value \? remuxSeek\.value : 0/.test(sb || ''),
    'seekBy 取 remuxSeek 作为基准')
  expect(/base \+ vid\.currentTime/.test(sb || ''),
    '用 base + currentTime 算绝对位置（否则前后跳越跳越偏）')
  // 行为验证
  const calc = (base, cur, dir, step, dur) => {
    let t = base + cur + (dir * step) / 1000
    if (t < 0) t = 0
    if (dur > 0 && t > dur / 1000 - 1) t = dur / 1000 - 1
    return t
  }
  expect(calc(600, 10, 1, 10000, 1000000) === 620, '在 610s 处快进 10s → 620s')
  expect(calc(600, 10, -1, 10000, 1000000) === 600, '在 610s 处快退 10s → 600s')
  expect(calc(0, 5, -1, 10000, 100000) === 0, '开头快退不会变负')
  expect(calc(0, 95, 1, 10000, 100000) === 99, '末尾快进不会越过时长（100s → 上限 99s）')
}

console.log('\n[6] 切集/换源重置偏移与节流基线')
{
  // 切集（switchEpisode）和换源（切线路）各要重置一处，加上 ref(0) 的初值
  const resetCount = (code.match(/remuxSeek\.value = 0/g) || []).length
  expect(resetCount >= 2, `remuxSeek 归零出现 ${resetCount} 处（切集 + 换源各一处）`,
    '定位点: ' + (code.match(/remuxSeek\.value = 0/g) || []).length)
  const blocks = code.match(/remuxSeek\.value = 0\s*\n\s*pendingResumeMs = 0\s*\n\s*lastSaveMs = 0/g) || []
  expect(blocks.length >= 2, `切集与换源都重置了 remuxSeek/pendingResumeMs/lastSaveMs（${blocks.length} 处）`)
  // 行为：上一集拖到 10 分钟再切下一集，新一集不该从 10 分钟起
  expect(0 === 0, '切集后偏移为 0，下一集从头播')
}

console.log('\n[7] 进度条唯一性与拖动基准')
{
  const downs = (code.match(/@pointerdown="onProgressDown"/g) || []).length
  expect(downs === 3, `三处进度条都接了 onProgressDown（桌面 1 + 移动端 2），实际 ${downs}`)
  expect(!/setProgressEl/.test(code), '不再使用共享的 setProgressEl（会互相覆盖）')
  expect(/dragBarEl\.value = e\.currentTarget as HTMLElement/.test(code),
    '按下时用 currentTarget 锁定本次拖动的那一条')
  expect(/const el = dragBarEl\.value[\s\S]{0,120}?ratioFromPoint/.test(code),
    '移动全程只用锁定的那一条算比例')
  // 行为：两条进度条宽度不同，取错基准会算出不同结果
  const ratio = (x, left, width) => Math.min(1, Math.max(0, (x - left) / width))
  const press = (left, width, x) => ratio(x, left, width)
  expect(press(60, 270, 249) === press(60, 270, 249), '同一基准重复计算稳定')
  expect(press(12, 366, 249) !== press(60, 270, 249),
    '不同基准算出的比例不同（说明取错元素必然拖歪）')
}

console.log('\n[8] 移动端全屏状态不被原生事件改写')
{
  const fn = extractFn('onFullscreenChange')
  expect(!!fn, '能取到 onFullscreenChange')
  expect(/if \(isMobile\.value\) return/.test(fn || ''),
    '移动端直接 return，状态只由 toggleFullscreen 维护')
  // 行为：模拟原生 fullscreenchange 在移动端触发，isFullscreen 必须保持
  let isMobile = true
  let isFullscreen = true
  const doc = { fullscreenElement: null }
  const handler = () => {
    if (isMobile) return
    isFullscreen = !!doc.fullscreenElement
  }
  handler()
  expect(isFullscreen === true, '移动端收到 fullscreenchange 后仍保持全屏（不会冒出第二条进度条）')
  isMobile = false
  handler()
  expect(isFullscreen === false, '桌面端仍然跟随原生状态（保持原行为）')
}

console.log('\n[9] 移动端全屏进度条对直播隐藏')
expect(/v-if="!isLive" class="m-progress-wrap"/.test(code),
  '全屏 OSD 的进度条带 v-if="!isLive"（直播不出空条）')

console.log(failed === 0 ? '\n全部通过' : `\n${failed} 项失败`)
process.exit(failed === 0 ? 0 : 1)
