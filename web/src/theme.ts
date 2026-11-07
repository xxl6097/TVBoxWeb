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
