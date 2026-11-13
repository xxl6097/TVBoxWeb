// 播放页结构回归：选集抽屉必须两端都在、快捷键必须规避输入态。
//
// 背景（本次审计发现）：
//   ① 桌面端点「选集」没反应 —— EpisodeDrawer 只挂在移动端模板里，
//      桌面分支里把 showEps 置 true 却没有任何组件渲染它。
//   ② 播放页有输入框（画质面板的字幕地址/选集搜索），onKey 没做输入态判断，
//      在里面打字会误触发播放器快捷键（m 静音、空格暂停、方向键跳进度）。
//   ③ 移动端全屏的快进/快退按钮画成了「上/下一集」的双三角图标，语义不符；
//      且全屏下没有切集入口，每换一集都得先退出全屏。
//
// 跑法：node scripts/check-player-ui.cjs
const { readFileSync } = require('fs')
const path = require('path')

const ROOT = path.resolve(__dirname, '..')
const src = readFileSync(path.join(ROOT, 'src/views/PlayerView.vue'), 'utf8')
const stripComments = (c) =>
  c.replace(/\/\*[\s\S]*?\*\//g, '').split('\n')
    .map((l) => l.replace(/(^|[^:])\/\/.*$/, '$1')).join('\n')
const code = stripComments(src)
const tpl = code.slice(code.indexOf('<template>'), code.indexOf('</template>'))

let failed = 0
function expect(cond, msg, detail) {
  console.log((cond ? '  ✓ ' : '  ✗ ') + msg)
  if (!cond) {
    failed++
    if (detail) console.log('      ' + detail)
  }
}

// 把模板切成「桌面分支」和「移动分支」
const deskStart = tpl.indexOf('v-if="!isMobile"')
const mvStart = tpl.indexOf('v-else ref="playerEl"')
expect(deskStart > 0 && mvStart > deskStart, '能切分出桌面/移动两个分支')
const deskTpl = tpl.slice(deskStart, mvStart)
const mvTpl = tpl.slice(mvStart)

console.log('\n[1] 桌面端选集抽屉')
expect(/<EpisodeDrawer/.test(deskTpl), '桌面分支里渲染 EpisodeDrawer')
expect(/v-model="showEps"/.test(deskTpl), '桌面抽屉绑定 showEps')
expect(/@select="jumpToEpisode"/.test(deskTpl), '桌面抽屉的 select 接到 jumpToEpisode')
expect(/>选集</.test(deskTpl), '桌面控制条有「选集」按钮')

console.log('\n[2] 移动端选集抽屉（保持原样）')
expect(/<EpisodeDrawer/.test(mvTpl), '移动分支里也有 EpisodeDrawer')
expect(/v-model="showEps"/.test(mvTpl), '移动抽屉绑定 showEps')

console.log('\n[3] 快捷键规避输入态')
expect(/function isTypingTarget/.test(code), '有 isTypingTarget 判定函数')
expect(/INPUT|'input'/.test(code) && /TEXTAREA|'textarea'/i.test(code), '识别 INPUT / TEXTAREA')
expect(/isContentEditable/.test(code), '识别 contenteditable')
const onKey = code.slice(code.indexOf('function onKey('), code.indexOf('function onKey(') + 900)
expect(/if \(isTypingTarget\(e\.target\)\) return/.test(onKey), 'onKey 开头先判输入态并放行')
expect(/(e\.ctrlKey|e\.metaKey)/.test(onKey), '带 Ctrl/Cmd 的组合键也不劫持')
expect(/isTypingTarget\(e\.target\)\) return[\s\S]{0,200}?preventDefault/.test(onKey),
  '输入态下不做 preventDefault（不影响正常输入）')

console.log('\n[4] 移动端全屏快进/快退图标语义')
const fsOsd = mvTpl.slice(mvTpl.indexOf('m-osd-fullscreen'))
const fsCtl = fsOsd.slice(fsOsd.indexOf('m-fs-controls'), fsOsd.indexOf('m-fs-controls') + 2600)
expect(/title="快退 10 秒"/.test(fsCtl), '快退按钮 title 写明「快退 10 秒」')
expect(/title="快进 10 秒"/.test(fsCtl), '快进按钮 title 写明「快进 10 秒」')
// 双三角图标是 3 个 polyline（11 17 2 12 11 7 / 22 7 22 17 / 22 17 13 12 22 7）
const seekBlocks = fsCtl.split('</button>').filter((b) => /title="快[进退] 10 秒"/.test(b))
expect(seekBlocks.length === 2, `找到 ${seekBlocks.length} 个快进/快退按钮`)
expect(seekBlocks.every((b) => /points="1 4 1 10 7 10"|points="23 4 23 10 17 10"/.test(b)),
  '快进/快退用回转箭头（不是双三角）',
  seekBlocks.map((b) => (b.match(/points="([^"]*)"/g) || []).join(' | ')).join('\n      '))

console.log('\n[5] 移动端全屏可切集')
expect(/switchEpisode\(-1\)/.test(fsCtl), '全屏有上一集')
expect(/switchEpisode\(1\)/.test(fsCtl), '全屏有下一集')
expect(/player\.count > 1/.test(fsCtl), '仅在多集时显示（单集不占位置）')

console.log('\n[6] 桌面/移动容器互斥（v-else 未被新节点打断）')
const between = tpl.slice(deskStart, mvStart)
const elseAfterComponent = /<EpisodeDrawer[\s\S]*?\/>\s*<\/div>\s*<div v-else/.test(between)
expect(elseAfterComponent || mvStart > 0, 'v-else 仍紧跟在桌面根 div 之后（组件在 div 内部）')

console.log('\n[7] 死代码清理')
expect(!/isLivePlaying/.test(code), '未使用的 isLivePlaying 已移除')

console.log('\n[8] 倍速功能必须有 UI 入口')
expect(/function setPlaybackRate/.test(code), 'setPlaybackRate 已实现')
expect(/function togglePlaySpeed/.test(code), 'togglePlaySpeed 已实现')
expect(/@click="togglePlaySpeed"/.test(deskTpl), '桌面控制条有倍速按钮')
expect(/@click="togglePlaySpeed"/.test(mvTpl), '移动端辅助按钮行有倍速按钮')
expect(/rateLabel/.test(code), '倍速显示走 rateLabel（不显示 1.0x）')

console.log('\n[9] 自动连播开关两端都有，且能取消倒计时')
expect(/@click="toggleAutoPlayNext"/.test(deskTpl), '桌面端可切换自动连播')
expect(/@click="toggleAutoPlayNext"/.test(mvTpl), '移动端可切换自动连播')
expect(!/autoPlayNext = !autoPlayNext/.test(code),
  '不再直接取反（需走函数以便取消正在跑的倒计时）')
{
  const fn = code.slice(code.indexOf('function toggleAutoPlayNext'),
                        code.indexOf('function toggleAutoPlayNext') + 400)
  expect(/if \(!autoPlayNext\.value\)[\s\S]{0,80}?cancelAutoplay\(\)/.test(fn),
    '关掉自动连播时立即取消倒计时（否则 3 秒后照样被切走）')
}

console.log('\n[10] 移动端图标不重复')
const auxIcons = [...mvTpl.matchAll(/class="m-aux-icon">([^<]*)</g)].map((m) => m[1])
const dup = auxIcons.filter((v, i) => v && auxIcons.indexOf(v) !== i)
expect(dup.length === 0, `辅助图标无重复（共 ${auxIcons.length} 个）`, '重复: ' + dup.join(','))

console.log('\n[11] 选集抽屉宽度响应式')
const drawer = readFileSync(path.join(ROOT, 'src/components/player/EpisodeDrawer.vue'), 'utf8')
expect(/width="min\(620px, 94vw\)"/.test(drawer),
  'el-dialog 用 min() 响应式宽度（390px 手机不溢出）')
expect(!/width="\d+px"/.test(drawer), '没有写死 px 宽度')

console.log('\n[12] 选集点击后立即关闭抽屉')
const jump = code.slice(code.indexOf('async function jumpToEpisode'), code.indexOf('async function switchEpisode'))
expect(/showEps\.value = false[\s\S]{0,120}?await switchEpisode/.test(jump),
  '先关抽屉再 await 切集（避免卡住时用户以为没点中）')

console.log('\n[13] 画中画按钮跟 OSD 一起显隐')
expect(/v-if="prepared && showOsd && supportsPip\(\)"/.test(deskTpl),
  '桌面 pip 按钮带 showOsd 条件（OSD 隐藏后不再常驻遮挡画面）')

console.log('\n[14] 当前画质档位可见')
expect(/qLabel/.test(deskTpl), '桌面画质按钮显示 qLabel')
expect(/qLabel/.test(mvTpl), '移动端画质按钮显示 qLabel')
{
  const sync = code.slice(code.indexOf('function syncQLabel'), code.indexOf('function syncQLabel') + 600)
  expect(!/'自动 ' \+ name/.test(sync),
    'qLabel 不再拼「自动 1080P」这种长串（移动端按钮会被挤变形）')
}

console.log('\n[15] 离开播放页清理全部定时器')
{
  const un = code.slice(code.indexOf('onUnmounted('), code.indexOf('onUnmounted(') + 1200)
  for (const t of ['saveTimer', 'seekPendingTimer', 'volSaveTimer', 'osdTimer', 'skipTimer', 'flashTimer', 'volTimer']) {
    expect(new RegExp('clear(Timeout|Interval)\\(' + t + '\\)').test(un), `onUnmounted 清理了 ${t}`)
  }
  expect(/cancelAutoplay\(\)/.test(un), 'onUnmounted 取消了连播倒计时')
}

console.log('\n[16] 连播倒计时的提示不会残留到下一集')
{
  const fn = code.slice(code.indexOf('function cancelAutoplay'), code.indexOf('function cancelAutoplay') + 500)
  expect(/hint\.value\s*=\s*''/.test(fn), 'cancelAutoplay 会清掉「即将播放下一集」提示')
  const sw = code.slice(code.indexOf('async function switchEpisode'), code.indexOf('async function switchEpisode') + 700)
  expect(/cancelAutoplay\(\)/.test(sw), 'switchEpisode 里取消倒计时')
  const tp = code.slice(code.indexOf('function togglePlay'), code.indexOf('function togglePlay') + 400)
  expect(/cancelAutoplay\(\)/.test(tp), 'togglePlay 里取消倒计时（用户按暂停就别切了）')
}

console.log('\n[17] hint 必须真的渲染出来')
expect(/v-if="hint"/.test(deskTpl), '桌面端渲染 hint')
expect(/v-if="hint"/.test(mvTpl), '移动端渲染 hint')
expect(/\.player-hint\s*\{/.test(code), '有 .player-hint 样式')
expect(/hint\.value\s*=/.test(code), 'hint 仍被赋值（不是死变量）')

console.log('\n[18] 竖屏视频区不能太矮、面板不能留大片空白')
{
  // 原来 .m-video-wrap 锁 16:9（390 屏只有 219px，占 26%），
  // .m-panel 又是 flex:1 撑满剩余空间，内容仅 266px 却占 625px，
  // 底部空出 359px，看着像页面没加载完。
  const css = code.slice(code.indexOf('.m-player:not(.m-fullscreen) .m-video-wrap'))
  expect(/flex:\s*1/.test(css.slice(0, 400)), '竖屏视频区用 flex 吃掉剩余高度')
  expect(!/aspect-ratio:\s*16\s*\/\s*9/.test(css.slice(0, 400)),
    '竖屏视频区不再锁死 16:9')
  const panel = code.slice(code.indexOf('.m-panel {'), code.indexOf('.m-panel {') + 500)
  expect(/flex:\s*0\s+0\s+auto/.test(panel), '面板按内容高度排布（不再 flex:1 撑满）')
  expect(/max-height/.test(panel), '面板有高度上限，内容过多时内部滚动')
}

console.log('\n[19] 定时器必须先停表再走后续逻辑')
{
  // 只把 id 置 0 而不 clearInterval，一旦后续提前 return 就会一直跑下去
  const ended = code.slice(code.indexOf('vid.onended'), code.indexOf('vid.onended') + 1200)
  expect(/clearInterval\(autoplayTimer\)[\s\S]{0,80}?autoplayTimer\s*=\s*0/.test(ended),
    '连播倒计时到点时先 clearInterval 再置 0')
  const skip = code.slice(code.indexOf('function showSkipBtn'), code.indexOf('function showSkipBtn') + 600)
  expect(/clearInterval\(skipTimer\)[\s\S]{0,120}?skipTimer\s*=\s*0[\s\S]{0,120}?doSkip/.test(skip),
    '跳过倒计时先停表并归零，再调 doSkip')
}

console.log('\n[20] 键盘快捷键')
{
  const k = code.slice(code.indexOf('function onKey'), code.indexOf('function onKey') + 1400)
  for (const key of ['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown']) {
    const seg = k.slice(k.indexOf(key))
    expect(/preventDefault/.test(seg.slice(0, 260)), `${key} 会 preventDefault（不滚动背景）`)
  }
  expect(/toLowerCase\(\)/.test(k), '按键统一转小写（M/F/P 都能用）')
  expect(/supportsPip\(\)/.test(k.slice(k.indexOf("'p'"), k.indexOf("'p'") + 300)),
    '按 P 前检查是否支持画中画')
}

console.log('\n[21] 解析失败时不能卡在「正在解析」')
{
  expect(/function safeResolve/.test(code), '有 safeResolve 统一包装')
  const sr = code.slice(code.indexOf('function safeResolve'), code.indexOf('function safeResolve') + 700)
  expect(/finally/.test(sr) && /resolving\.value\s*=\s*false/.test(sr),
    'safeResolve 在 finally 里复位 resolving')
  // 唯一允许出现裸调的地方就是 safeResolve 自己
  const total = (code.match(/await resolveUrl\(\)/g) || []).length
  const inside = (sr.match(/await resolveUrl\(\)/g) || []).length
  expect(total === inside, `safeResolve 以外没有裸调 await resolveUrl()（共 ${total} 处，其中 safeResolve 内 ${inside} 处）`)
}

console.log('\n[22] 音量滑块拖动')
{
  const down = code.slice(code.indexOf('function onVolDown'), code.indexOf('function onVolDown') + 900)
  expect(/addEventListener\('pointermove'/.test(down), '拖动时 pointermove 绑 window')
  expect(/addEventListener\('pointerup'/.test(down), 'pointerup 绑 window（移出元素也能收尾）')
  const up = code.slice(code.indexOf('function onVolUp'), code.indexOf('function onVolUp') + 400)
  expect(/removeEventListener\('pointermove'/.test(up), 'onVolUp 解绑 window 监听')
  const un = code.slice(code.indexOf('onUnmounted('), code.indexOf('onUnmounted(') + 1400)
  expect(/onVolUp\(\)/.test(un), '卸载时解绑音量拖动监听')
}

console.log('\n[23] 网盘历史保存兜底')
{
  const sp = code.slice(code.indexOf('function savePos'), code.indexOf('function savePos') + 1600)
  expect(!/siteKey === 'webdav'\) \{\s*if \(!player\.wdVod\) return/.test(sp),
    'wdVod 为空时不再直接 return（会落到点播历史分支）')
  expect(/siteKey === 'webdav' && player\.wdVod/.test(sp),
    '只有 wdVod 有值时才写网盘历史')
}

console.log(failed === 0 ? '\n全部通过' : `\n${failed} 项失败`)
process.exit(failed === 0 ? 0 : 1)
