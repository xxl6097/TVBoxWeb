// 播放设置面板的可见性回归测试（零框架，node 直接跑）。
//
// 背景：PlayerSettingsPanel 最初把「显示与否」写成只判断 variant，
// 没有绑定 modelValue，导致父组件把 showQ 置 false 后组件内部不接受：
// 面板一旦出现就关不掉，而且 v-if 与 v-model 脱节时还会「一进就弹」。
//
// 这个脚本直接编译真实的 .vue 文件，用 jsdom 点击真实的关闭按钮与遮罩，
// 断言面板确实消失。跑法：node scripts/check-panel.cjs
const { JSDOM } = require('jsdom')
const { readFileSync } = require('fs')
const path = require('path')

const ROOT = path.resolve(__dirname, '..')
const dom = new JSDOM('<!doctype html><html><body><div id="app"></div></body></html>', {
  url: 'http://localhost/'
})
global.window = dom.window
global.document = dom.window.document
global.Element = dom.window.Element
global.SVGElement = dom.window.SVGElement

const { parse, compileScript } = require(path.join(ROOT, 'node_modules/@vue/compiler-sfc'))
const esbuild = require('esbuild')
const V = require('vue')

// element-plus 的 el-button 不属于被测范围，用原生 button 顶替
const StubButton = V.defineComponent({
  setup(_, { slots }) {
    return () => V.h('button', { class: 'stub-btn' }, slots.default ? slots.default() : [])
  }
})

// 把 SFC 编译成可执行的组件
function loadSFC(file) {
  const abs = path.join(ROOT, file)
  const { descriptor } = parse(readFileSync(abs, 'utf8'), { filename: abs })
  const compiled = compileScript(descriptor, { id: 'panel', inlineTemplate: true })
  const src = compiled.content
    .replace(/^import .*$/gm, '') // 去掉 @/ 别名导入，下面用 prelude 顶替
    .replace(/export default/, 'module.exports =')
  const names = [
    'defineComponent', 'createElementBlock', 'createElementVNode', 'createBlock', 'openBlock',
    'toDisplayString', 'renderList', 'createCommentVNode', 'normalizeClass', 'withModifiers',
    'createTextVNode', 'Fragment', 'resolveComponent', 'withCtx', 'createVNode', 'mergeProps',
    'toHandlers', 'normalizeStyle', 'withDirectives', 'unref', 'isRef', 'resolveDynamicComponent'
  ]
  const prelude =
    `const { ${names.map((n) => `${n}: _${n}`).join(', ')} } = __VUE__;\n` +
    // 被测组件从 @/utils/quality 引入的三个常量
    `const AUTO = 0, TOP = 100000, labelOfLevel = (lv) => (lv.height ? lv.height + 'P' : '原画');\n`
  const js = esbuild.transformSync(prelude + src, { loader: 'ts', format: 'cjs' }).code
  const mod = { exports: {} }
  new Function('module', 'exports', 'require', '__VUE__', js)(mod, mod.exports, require, V)
  return mod.exports.default || mod.exports
}

const Panel = loadSFC('src/components/player/PlayerSettingsPanel.vue')

function makeParent(variant) {
  return V.defineComponent({
    setup() {
      const showQ = V.ref(false)
      const props = {
        isLive: false, levels: [], qCap: 0, levelIdx: -1, lines: [], rawUrl: '',
        subDelay: 0, subFontSize: 20, externalSubActive: false, subLabel: '',
        scaleNames: ['铺满'], scaleMode: 0, introSkipSec: 0, outroSkipSec: 0
      }
      return () =>
        V.h('div', [
          V.h('button', { class: 'open-btn', onClick: () => (showQ.value = !showQ.value) }, '画质'),
          V.h(Panel, {
            modelValue: showQ.value,
            'onUpdate:modelValue': (v) => (showQ.value = v),
            variant,
            ...props
          })
        ])
    }
  })
}

const tick = async () => {
  await V.nextTick()
  await V.nextTick()
}
const click = (el) => el.dispatchEvent(new dom.window.MouseEvent('click', { bubbles: true }))

let failed = 0
function expect(cond, msg) {
  if (cond) {
    console.log('  ✓ ' + msg)
  } else {
    console.log('  ✗ ' + msg)
    failed++
  }
}

async function runVariant(variant) {
  const mask = variant === 'mobile' ? '.m-q-mask' : '.q-panel'
  const closeBtn = variant === 'mobile' ? '.m-q-close button' : '.q-footer button'
  console.log(`\n[${variant}]`)

  document.body.innerHTML = '<div id="app"></div>'
  const app = V.createApp(makeParent(variant))
  app.component('el-button', StubButton)
  app.mount('#app')
  await tick()

  // 1) 初始必须是关闭的：面板不能自己弹出来
  expect(!document.querySelector(mask), '初始不显示（不会一进就弹）')

  // 2) 点「画质」能打开
  click(document.querySelector('.open-btn'))
  await tick()
  expect(!!document.querySelector(mask), '点「画质」后显示')

  // 3) 点「关闭」必须能关掉 —— 这就是用户报的 bug
  const btn = [...document.querySelectorAll(closeBtn)].find((b) => b.textContent.includes('关闭'))
  expect(!!btn, '能找到关闭按钮')
  if (btn) {
    click(btn)
    await tick()
    expect(!document.querySelector(mask), '点「关闭」后消失')
  }

  // 4) 点遮罩空白处也能关
  click(document.querySelector('.open-btn'))
  await tick()
  click(document.querySelector(mask))
  await tick()
  expect(!document.querySelector(mask), '点遮罩空白处后消失')

  // 5) 点面板内部（非关闭按钮）不能误关 —— 内部有 @click.stop，外层有 @click.self
  click(document.querySelector('.open-btn'))
  await tick()
  const title = document.querySelector(variant === 'mobile' ? '.m-q-title' : '.q-title')
  if (title) {
    click(title)
    await tick()
    expect(!!document.querySelector(mask), '点面板内部不会误关')
  }
  // 关掉，进入下一项
  click([...document.querySelectorAll(closeBtn)].find((b) => b.textContent.includes('关闭')))
  await tick()

  // 6) 反复开关不出错
  for (let i = 0; i < 3; i++) {
    click(document.querySelector('.open-btn'))
    await tick()
    click([...document.querySelectorAll(closeBtn)].find((b) => b.textContent.includes('关闭')))
    await tick()
  }
  expect(!document.querySelector(mask), '反复开关后仍能正常关闭')

  app.unmount()
}

;(async () => {
  await runVariant('desktop')
  await runVariant('mobile')
  console.log(failed === 0 ? '\n全部通过' : `\n${failed} 项失败`)
  process.exit(failed === 0 ? 0 : 1)
})().catch((e) => {
  console.error(e)
  process.exit(1)
})
