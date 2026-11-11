// 主题常量，取自 FongMi/TV leanback 资源，与鸿蒙影视 Model.ets 的 T 常量一致。

export const T = {
  BG: '#141218',
  SURFACE: '#1C1B1F',
  BLACK_20: '#33000000',
  BLACK_40: '#66000000',
  BLACK_50: '#80000000',
  BLACK_70: '#B3000000',
  BLACK_80: '#CC000000',
  BLACK_90: '#E6000000',
  WHITE_15: '#26FFFFFF',
  WHITE_30: '#4DFFFFFF',
  WHITE_70: '#B3FFFFFF',
  WHITE_80: '#CCFFFFFF',
  PRIMARY: '#FFFFFF',
  SELECTED: '#FFEB3B',
  CARD: '#33000000',
  CARD_HI: '#66000000',
  ACCENT: '#FFFFFF',
  ACCENT_DIM: '#4DFFFFFF',
  TXT: '#FFFFFF',
  SUB: '#B3FFFFFF',
  DIM: '#80FFFFFF',
  WARN: '#FFB4AB',
  ERR: '#FFB4AB',
  LIVE: '#F44336',
  ANIM: 140,
  BORDER: 2,
  RADIUS: 8,
  RADIUS_SM: 4,
  RADIUS_PILL: 28,
  HOVER_SCALE: 1.1
} as const

export type ThemeT = typeof T

// ===== 主题切换 =====
export type ThemeMode = 'dark' | 'light' | 'system'
const THEME_KEY = 'yz_theme'

export function getTheme(): ThemeMode {
  try {
    const v = localStorage.getItem(THEME_KEY)
    return v === 'light' || v === 'dark' || v === 'system' ? v : 'dark'
  } catch { return 'dark' }
}

export function setTheme(mode: ThemeMode): void {
  try { localStorage.setItem(THEME_KEY, mode) } catch { /* ignore */ }
  applyTheme(mode)
}

export function applyTheme(mode: ThemeMode = getTheme()): void {
  if (typeof document === 'undefined') return
  const prefersDark = typeof matchMedia === 'function' && matchMedia('(prefers-color-scheme: dark)').matches
  const dark = mode === 'dark' || (mode === 'system' && prefersDark)
  const root = document.documentElement
  root.classList.toggle('dark', dark)
  root.classList.toggle('light', !dark)
  document.querySelector('meta[name="theme-color"]')
    ?.setAttribute('content', dark ? '#141218' : '#f7f6f8')
}

export function watchSystemTheme(): void {
  if (typeof matchMedia !== 'function') return
  const mq = matchMedia('(prefers-color-scheme: dark)')
  const fn = () => { if (getTheme() === 'system') applyTheme('system') }
  if (mq.addEventListener) mq.addEventListener('change', fn)
  else mq.addListener(fn)
}
