// 自动换源的匹配算法测试（零框架，node 直跑）。
//
// 这里不测 UI，只测最容易出错的三段纯逻辑：
//   ① titleScore 片名相似度 —— 决定「换到哪一部」
//   ② epNumber   集号抽取   —— 决定「换到第几集」
//   ③ locateEpisode 集定位   —— 决定「在目标线路里找哪一集」
// 这三个一旦错，用户会被换到别的剧 / 别的集，比不换源更糟。
const { execFileSync } = require('node:child_process')
const path = require('node:path')
const fs = require('node:fs')
const os = require('node:os')

const ROOT = path.join(__dirname, '..')
const SRC = path.join(ROOT, 'src/utils/autoSwitch.ts')

// 用 esbuild 把 TS 转成 CJS 再 require，测的就是真实实现而不是副本
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'asw-'))
const out = path.join(tmp, 'autoSwitch.cjs')
execFileSync(
  path.join(ROOT, 'node_modules/.bin/esbuild'),
  [SRC, '--format=cjs', '--platform=node', '--outfile=' + out, '--log-level=error'],
  { stdio: 'inherit' }
)
// 去掉对 api / splitLines 的真实依赖（测试只关心纯函数）
let code = fs.readFileSync(out, 'utf8')
code = code
  .replace(/require\("@\/api"\)/g, '({ api: { search: async () => ({ list: [] }), vodDetail: async () => ({}) } })')
  .replace(/require\("@\/utils\/lines"\)/g, '({ splitLines: () => [] })')
fs.writeFileSync(out, code)

const { titleScore, epNumber, locateEpisode } = require(out)

let failed = 0
function expect(cond, msg, detail) {
  console.log((cond ? '  ✓ ' : '  ✗ ') + msg + (detail !== undefined ? '  ' + JSON.stringify(detail) : ''))
  if (!cond) failed++
}

console.log('[1] 片名相似度：必须能把「同一部」和「沾边但不是」分开')
expect(titleScore('三十而已', '三十而已') === 1, '完全同名 = 1')
expect(titleScore('三十而已', '三十而已 ') > 0.95, '尾部空格不影响（归一化后同名）')
expect(titleScore('三十而已', '三十而已（粤语）') >= 0.8, '带语言后缀仍算同一部',
  titleScore('三十而已', '三十而已（粤语）'))
expect(titleScore('三十而已', '三十而已 花絮') < 0.5, '「花絮」必须降权',
  titleScore('三十而已', '三十而已 花絮'))
expect(titleScore('三十而已', '三十而已 预告') < 0.5, '「预告」必须降权',
  titleScore('三十而已', '三十而已 预告'))
expect(titleScore('三十而已', '三十而立') < 0.8, '「而已」vs「而立」不能算同一部',
  titleScore('三十而已', '三十而立'))
expect(titleScore('鬼灭之刃', '鬼灭之刃 第一季') >= 0.8, '带「第一季」仍算同一部',
  titleScore('鬼灭之刃', '鬼灭之刃 第一季'))
// 名字一长一短：短的是长的一部分时更像同一部
expect(titleScore('狂飙', '狂飙') === 1, '短名同名')
expect(titleScore('', '任何') === 0, '空片名给 0')

console.log('\n[2] 集号抽取：各源集名格式不一样，必须都能对上')
const cases = [
  ['第01集', 1], ['第1集', 1], ['第 12 集', 12],
  ['第十二集', null], // 中文数字不转，靠集名兜底
  ['01', 1], ['1', 1], ['12', 12],
  ['EP01', 1], ['ep3', 3], ['E12', 12], ['EP.05', 5],
  ['第100集', 100], ['01-高清', 1], ['第01期', 1], ['第3话', 3],
  ['正片', null], ['预告', null], ['', null]
]
for (const [name, want] of cases) {
  const got = epNumber(name)
  expect(got === want, `epNumber(${JSON.stringify(name)}) = ${want}`, got)
}

console.log('\n[3] 集定位：优先集号，其次集名，最后退回下标')
const mk = (names) => names.map((n) => ({ name: n, url: 'http://x/' + n, urls: [] }))

// 场景 A：目标源集名格式不同，但集号能对上
let eps = mk(['第01集', '第02集', '第03集', '第04集'])
expect(locateEpisode(eps, 2, '第3集') === 2, '原「第3集」→ 新源「第03集」下标 2',
  locateEpisode(eps, 2, '第3集'))

// 场景 B：新源多了一集预告，索引整体后移 —— 必须靠集号而不是下标
eps = mk(['预告', '第01集', '第02集', '第03集'])
expect(locateEpisode(eps, 1, '第2集') === 2, '有预告时靠集号定位到下标 2',
  locateEpisode(eps, 1, '第2集'))

// 场景 C：集名完全一致但抽不出数字
eps = mk(['上', '下'])
expect(locateEpisode(eps, 1, '下') === 1, '集名相同按名字定位', locateEpisode(eps, 1, '下'))

// 场景 D：抽不出数字也对不上名字，退回原下标
eps = mk(['正片A', '正片B', '正片C'])
expect(locateEpisode(eps, 2, '第9集') === 2, '对不上时退回原下标', locateEpisode(eps, 2, '第9集'))

// 场景 E：原下标越界时退回 0，不能返回 -1 让调用方崩
eps = mk(['第01集', '第02集'])
expect(locateEpisode(eps, 99, '第99集') === 0, '越界退回第一集', locateEpisode(eps, 99, '第99集'))

// 场景 F：空线路
expect(locateEpisode([], 0, '第1集') === -1, '空线路返回 -1')

console.log('\n[4] 集号对不上时不能乱带进度')
{
  // buildPlayQuery 会把 pos 写进 URL；进度只在同一集时有意义，
  // 这条规则在 autoSwitchSource 里用 sameEpisode 判断（见源码）。
  const src = fs.readFileSync(SRC, 'utf8')
  expect(/sameEpisode\(h, line\.eps\[idx\]\)/.test(src),
    '进度带上与否由 sameEpisode 判定')
  expect(/pos: String\(Math\.max\(0, Math\.floor\(posMs\)\)\)/.test(src),
    '进度通过 ?pos= 传给播放页')
}

console.log('\n[5] 候选筛选：排除噪音与失效源')
{
  const src = fs.readFileSync(SRC, 'utf8')
  expect(/v\.siteKey !== h\.siteKey/.test(src), '不再试已失效的原站点')
  expect(/!isNoise\(v\.name\)/.test(src), '排除花絮/预告等噪音条目')
  expect(/score >= 0\.6/.test(src), '相似度低于 0.6 的不试')
  expect(/cands\.slice\(0, 3\)/.test(src), '最多试 3 个候选，避免久等')
  expect(/return \{\s*ok: false[\s\S]{0,80}fallbackKw/.test(src),
    '找不到时回传片名供手动搜索')
}

console.log('\n[6] 接进页面：HomeView / HistoryView 都改走自动换源')
{
  const home = fs.readFileSync(path.join(ROOT, 'src/views/HomeView.vue'), 'utf8')
  const hist = fs.readFileSync(path.join(ROOT, 'src/views/HistoryView.vue'), 'utf8')
  const flow = fs.readFileSync(path.join(ROOT, 'src/utils/autoSwitchFlow.ts'), 'utf8')
  const play = fs.readFileSync(path.join(ROOT, 'src/views/PlayerView.vue'), 'utf8')

  // 两处入口都要用 autoSwitchAndPlay，而不是只跳搜索页
  if (true) {
    const t = (c, m, d) => expect(c, m, d)
    const p2 = (f) => fs.readFileSync(path.join(ROOT, f), 'utf8')
    t(/isDeadHist\(h\)[\s\S]{0,200}autoSwitchAndPlay\(/.test(home),
      '首页失效记录走 autoSwitchAndPlay')
    t(/isDead\(h\)[\s\S]{0,200}autoSwitchAndPlay\(/.test(hist),
      '历史页失效记录走 autoSwitchAndPlay')
    t(/switchingKey/.test(flow), '换源有连点保护')
    t(/player\.set\(/.test(flow), '换源前把播放列表写进 store（保留上一集/下一集）')
    t(/buildPlayQuery/.test(flow), '用 buildPlayQuery 拼播放参数')
    t(/name: 'search'/.test(flow), '换源失败时降级到搜索页')
    t(/urlResumeMs/.test(play), '播放页读取 ?pos=')
    t(/player\.seek > 0 \? player\.seek : urlResumeMs/.test(play),
      'player.seek 优先，?pos= 兜底')
    t(/上次的位置超出这一集的时长/.test(play),
      '进度超出本集时长时从头播（防止跳片尾触发连播）')
  }
}

fs.rmSync(tmp, { recursive: true, force: true })
console.log(failed === 0 ? '\n全部通过' : `\n${failed} 项失败`)
process.exit(failed === 0 ? 0 : 1)
