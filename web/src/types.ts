// 与 Go 后端 model 包一一对应的前端类型。

/** 站源健康度（GET /api/sites/health） */
export interface SiteHealthStatus {
  key: string
  status: 'healthy' | 'slow' | 'dead' | 'unknown' | string
  lastSuccess: number
  errorRate: number
  avgMs: number
}

export interface Site {
  key: string
  name: string
  type: number
  api: string
  searchable: boolean
  quickSearch: boolean
  ext: string
  usable: boolean
  reason: string
  enabled: boolean
}

export interface Category {
  id: string
  name: string
  pid: string
}

export interface Vod {
  id: string
  name: string
  pic: string
  remark: string
  year: string
  area: string
  actor: string
  director: string
  content: string
  typeName: string
  from: string
  playUrl: string
  siteKey: string
  siteName: string
}

export interface Episode {
  name: string
  url: string
  urls: string[]
}

export interface Line {
  name: string
  direct: boolean
  eps: Episode[]
}

export interface VodPage {
  items: Vod[]
  page: number
  pageCount: number
  total: number
}

export interface FilterValue {
  n: string
  v: string
}

export interface FilterGroup {
  key: string
  name: string
  values: FilterValue[]
}

export interface Channel {
  name: string
  url: string
  logo: string
  group: string
  urls: string[]
}

export interface ChannelGroup {
  name: string
  channels: Channel[]
}

export interface LiveDir {
  name: string
  count: number
  children: LiveDir[]
  channels: Channel[]
}

export interface LiveScore {
  name: string
  url: string
  ok: boolean
  ms: number
  count: number
  alive: number
  sampled: number
  score: number
  state: string
}

export interface HistItem {
  siteKey: string
  siteName: string
  vodId: string
  name: string
  pic: string
  epName: string
  epIndex: number
  from: string
  pos: number
  dur: number
  time: number
  watchedEps?: number[]  // 已看过的集数列表（分集管理）
}

export interface FavItem {
  siteKey: string
  siteName: string
  vodId: string
  name: string
  pic: string
  groupId?: string  // 所属分组 ID，空字符串表示默认分组
}

export interface FavGroup {
  id: string
  name: string
  time: number
  count?: number  // 该分组内收藏数量（由后端聚合计算）
}

export interface WebdavSource {
  id: string
  name: string
  url: string
  user: string
  pass: string
  root: string
}

export interface WebdavEntry {
  href: string
  name: string
  isDir: boolean
  size: number
  mtime: string
}

export interface WebdavEp {
  name: string
  url: string
  href: string
  season: number
  episode: number
}

export interface WebdavVod {
  name: string
  kind: string
  year: string
  /** 所在绝对目录（URL 路径） */
  dir: string
  /**
   * 相对挂载根的目录路径（已解码），如「凡人/01 风起天南」。
   * 用于还原目录层级分组；直接位于挂载根下时为空串。
   * 老版本后端不返回该字段，前端会退化为按 dir 推断。
   */
  relDir?: string
  eps: WebdavEp[]
}

export interface WdHistory {
  srcId: string
  srcName: string
  vodName: string
  dir: string
  epIndex: number
  epName: string
  url: string
  pos: number
  dur: number
  time: number
}

export interface RawLive {
  name: string
  url: string
}

export interface EpgProgram {
  start: number
  stop: number
  title: string
  desc: string
}

export interface EpgResult {
  channel: string
  programs: EpgProgram[]
  now: EpgProgram | null
  next: EpgProgram | null
}

export interface QLevel {
  height: number
  width: number
  bps: number
  url: string
  name: string
}

export interface CrawlResult {
  at: number
  sites: number
  alive: number
  dead: number
  items: number
  cfgUpdated: boolean
  ok: boolean
  msg: string
}
