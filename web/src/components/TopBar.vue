<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'

const router = useRouter()
const route = useRoute()

const clock = ref('')
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

function tick() {
  const d = new Date()
  const h = d.getHours() < 10 ? '0' + d.getHours() : '' + d.getHours()
  const m = d.getMinutes() < 10 ? '0' + d.getMinutes() : '' + d.getMinutes()
  clock.value = `${h}:${m}`
}

onMounted(() => {
  tick()
  timer = window.setInterval(tick, 30000)
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
        @click="navigate(n.path)"
      >
        {{ n.label }}
      </span>
    </nav>
    <span class="clock desktop-only">{{ clock }}</span>

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
        @click="navigate(n.path)"
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
  min-width: 44px;
  flex-shrink: 0;
}

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
