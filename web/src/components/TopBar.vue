<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'

const router = useRouter()
const route = useRoute()

// 拆成 时/分/秒 三段渲染：秒用等宽字形，避免每秒跳动时整行数字左右晃
const clockParts = ref({ h: '00', m: '00', s: '00' })
// 屏幕阅读器用的完整时间串（模板里显示的是上面三段）
const clockText = ref('00:00:00')
const menuOpen = ref(false)
let timer = 0

const navs = [
  { name: 'home', label: '首页', path: '/' },
  { name: 'vod', label: '点播', path: '/vod' },
  { name: 'live', label: '直播', path: '/live' },
  { name: 'search', label: '搜索', path: '/search' },
  { name: 'collect', label: '收藏', path: '/collect' },
  { name: 'history', label: '历史', path: '/history' },
  { name: 'webdav', label: '网盘', path: '/webdav' },
  { name: 'settings', label: '设置', path: '/settings' }
]

const pad = (n: number) => (n < 10 ? '0' + n : '' + n)

function tick() {
  const d = new Date()
  const h = pad(d.getHours())
  const m = pad(d.getMinutes())
  const s = pad(d.getSeconds())
  clockParts.value = { h, m, s }
  clockText.value = `${h}:${m}:${s}`
}

onMounted(() => {
  tick()
  // 显示到秒，就必须每秒刷新；用 1s 间隔会累积漂移，
  // 这里对齐到下一个整秒再起定时器，保证翻秒尽量贴近真实时间。
  const startAligned = () => {
    timer = window.setInterval(tick, 1000)
  }
  const now = new Date()
  window.setTimeout(() => {
    tick()
    startAligned()
  }, 1000 - now.getMilliseconds())
})
onUnmounted(() => clearInterval(timer))

function active(path: string) {
  if (path === '/') return route.name === 'home'
  return route.path.startsWith(path)
}

function navigate(path: string) {
  menuOpen.value = false
  router.push(path)
}
</script>

<template>
  <header class="topbar">
    <div class="logo" @click="navigate('/')">
      <img class="logo-icon" src="/icons/app_icon.png" alt="夜帧影视" />
      <span class="title">夜帧影视</span>
    </div>

    <!-- 桌面导航 -->
    <nav class="navs desktop-only">
      <span
        v-for="n in navs"
        :key="n.name"
        class="nav-item"
        :class="{ active: active(n.path) }"
        role="button"
        tabindex="0"
        :aria-label="`前往${n.label}`"
        :aria-current="active(n.path) ? 'page' : undefined"
        @click="navigate(n.path)"
        @keydown.enter="navigate(n.path)"
        @keydown.space.prevent="navigate(n.path)"
      >
        {{ n.label }}
      </span>
    </nav>
    <span class="clock desktop-only" :aria-label="`当前时间 ${clockText}`">
      <span class="c-hm">{{ clockParts.h }}:{{ clockParts.m }}</span><span class="c-s">:{{ clockParts.s }}</span>
    </span>

    <!-- 移动端汉堡按钮 -->
    <button class="hamburger mobile-only" :class="{ open: menuOpen }" @click="menuOpen = !menuOpen" aria-label="菜单">
      <span></span><span></span><span></span>
    </button>
  </header>

  <!-- 移动端下拉菜单 -->
  <div v-if="menuOpen" class="mobile-menu mobile-only" @click="menuOpen = false">
    <div class="mobile-menu-inner" @click.stop>
      <span
        v-for="n in navs"
        :key="n.name"
        class="mobile-nav-item"
        :class="{ active: active(n.path) }"
        role="button"
        tabindex="0"
        :aria-label="`前往${n.label}`"
        @click="navigate(n.path)"
        @keydown.enter="navigate(n.path)"
        @keydown.space.prevent="navigate(n.path)"
      >
        {{ n.label }}
      </span>
    </div>
  </div>
</template>

<style scoped>
.topbar {
  display: flex;
  align-items: center;
  padding: 12px 20px 8px;
  gap: 20px;
  position: relative;
  z-index: 100;
}
.logo {
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
  flex-shrink: 0;
}
.logo-badge,
.logo-icon {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: #fff;
  color: #141218;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  object-fit: cover;
}
.logo-icon {
  background: transparent;
}
.title {
  font-size: 20px;
  font-weight: 700;
}
.navs {
  display: flex;
  gap: 4px;
  flex: 1;
  flex-wrap: wrap;
}
.nav-item {
  padding: 6px 14px;
  border-radius: 20px;
  font-size: 14px;
  color: var(--sub);
  cursor: pointer;
  transition: all 0.14s;
  white-space: nowrap;
}
.nav-item:hover {
  color: #fff;
}
.nav-item.active {
  color: var(--selected);
  background: var(--card-hi);
}
.clock {
  font-size: 15px;
  color: var(--dim);
  /* 固定宽 + 等宽数字：秒数每秒变化时不会推挤后面的元素，也不会左右抖动 */
  min-width: 76px;
  flex-shrink: 0;
  text-align: right;
  font-variant-numeric: tabular-nums;
  font-feature-settings: 'tnum' 1;
  letter-spacing: 0.02em;
  white-space: nowrap;
  user-select: none;
}
/* 秒比时分淡一档，视觉主体仍是时分 */
.clock .c-s { opacity: 0.65; }

/* 汉堡菜单 */
.hamburger {
  display: none;
  flex-direction: column;
  justify-content: space-between;
  width: 28px;
  height: 20px;
  background: none;
  border: none;
  cursor: pointer;
  padding: 0;
  margin-left: auto;
}
.hamburger span {
  display: block;
  height: 2px;
  background: #fff;
  border-radius: 2px;
  transition: all 0.25s;
  transform-origin: center;
}
.hamburger.open span:nth-child(1) { transform: translateY(9px) rotate(45deg); }
.hamburger.open span:nth-child(2) { opacity: 0; transform: scaleX(0); }
.hamburger.open span:nth-child(3) { transform: translateY(-9px) rotate(-45deg); }

/* 移动端下拉菜单 */
.mobile-menu {
  position: fixed;
  inset: 0;
  z-index: 200;
  background: rgba(0, 0, 0, 0.5);
  display: none;
}
.mobile-menu-inner {
  position: absolute;
  top: 56px;
  left: 0;
  right: 0;
  background: var(--surface);
  border-bottom: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  padding: 8px 0 16px;
}
.mobile-nav-item {
  padding: 14px 24px;
  font-size: 16px;
  color: var(--sub);
  cursor: pointer;
  border-left: 3px solid transparent;
  transition: all 0.14s;
}
.mobile-nav-item:active {
  background: var(--card);
}
.mobile-nav-item.active {
  color: var(--selected);
  border-left-color: var(--selected);
  background: var(--card);
}

/* 响应式断点 */
@media (max-width: 640px) {
  .desktop-only { display: none !important; }
  .mobile-only { display: flex !important; }
  .mobile-menu { display: block; }
}
</style>
