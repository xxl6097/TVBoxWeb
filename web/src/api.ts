// API 客户端：统一封装对 Go 后端的请求。

import type {
  Site,
  Category,
  Vod,
  VodPage,
  HistItem,
  FavItem,
  FavGroup,
  RawLive,
  LiveDir,
  ChannelGroup,
  Channel,
  LiveScore,
  QLevel,
  WebdavSource,
  WebdavEntry,
  WebdavVod
} from '@/types'
import { idbGet, idbSet } from '@/idb'

const BASE = '/api'

// 可离线缓存的 GET 路径（历史/收藏/分类/站源等只读数据）
// 视频详情、搜索等也缓存，但 TTL 较短
const CACHEABLE_PREFIXES = [
  '/status',
  '/sites',
  '/categories',
  '/history',
  '/favorites',
  '/keywords',
  '/feed',
  '/live/url',
  '/live/sources',
  '/live/channels',
  '/prefs',
  '/privacy',
  '/webdav/sources',
  '/webdav/vods',
  '/direct'
]

// 判断路径是否可缓存
function isCacheable(path: string): boolean {
  const clean = path.split('?')[0]
  return CACHEABLE_PREFIXES.some((p) => clean === p || clean.startsWith(p + '/'))
}

// 判断请求是否为 GET（只有 GET 可缓存）
function isGet(opts: RequestInit): boolean {
  return !opts.method || opts.method.toUpperCase() === 'GET'
}

async function request<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const cacheable = isGet(opts) && isCacheable(path)

  // 命中 IndexedDB 缓存则秒回（离线第二层兜底）
  if (cacheable) {
    const cacheKey = 'api:' + path
    const cached = await idbGet<T>(cacheKey)
    if (cached) {
      // 后台刷新缓存（不阻塞返回）
      refreshCache(path, cacheKey)
      return cached
    }
  }

  const res = await fetch(BASE + path, {
    headers: { 'Content-Type': 'application/json' },
    ...opts
  })
  if (!res.ok) {
    let msg = `请求失败 (${res.status})`
    try {
      // 检查 Content-Type，避免 HTML 错误页面解析失败导致错误信息丢失
      const ct = res.headers.get('Content-Type') || ''
      if (ct.includes('application/json')) {
        const j = await res.json()
        if (j && j.error) msg = j.error
      } else {
        const text = await res.text()
        // 截取前 200 字符作为提示，避免显示完整 HTML
        if (text && !text.trim().startsWith('<')) {
          msg = text.slice(0, 200)
        }
      }
    } catch {
      /* ignore */
    }
    throw new Error(msg)
  }

  const data = (await res.json()) as T

  // 缓存成功响应（IndexedDB）
  if (cacheable) {
    const cacheKey = 'api:' + path
    idbSet(cacheKey, data, 5 * 60 * 1000) // TTL 5 分钟
  }

  return data
}

// 后台刷新缓存（stale-while-revalidate）
async function refreshCache(path: string, cacheKey: string): Promise<void> {
  try {
    const res = await fetch(BASE + path, {
      headers: { 'Content-Type': 'application/json' }
    })
    if (res.ok) {
      const data = await res.json()
      idbSet(cacheKey, data, 5 * 60 * 1000)
    }
  } catch {
    /* 后台刷新失败忽略 */
  }
}

const get = <T>(path: string) => request<T>(path)
const post = <T>(path: string, body?: unknown) =>
  request<T>(path, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body) })
const del = <T>(path: string) => request<T>(path, { method: 'DELETE' })

export const api = {
  get,
  post,
  del,

  status: () => get<any>('/status'),
  sites: () => get<Site[]>('/sites'),
  setEnabled: (keys: string[]) => post('/sites/enabled', { keys }),
  sitesSpeed: () => get<{ key: string; name: string; ms: number; ok: boolean }[]>('/sites/speed'),
  loadConfig: (source: string) => post<{ count: number }>('/config', { source }),
  subscribe: (source: string) => post<{ name: string; sites: number; lives: number }>('/subscription', { source }),

  categories: (site: string) => get<Category[]>(`/categories?site=${encodeURIComponent(site)}`),
  vodList: (site: string, tid: string, page: number, ext = '', children = '') =>
    get<VodPage>(
      `/vod?site=${encodeURIComponent(site)}&tid=${encodeURIComponent(tid)}&page=${page}` +
        (ext ? `&ext=${encodeURIComponent(ext)}` : '') +
        (children ? `&children=${encodeURIComponent(children)}` : '')
    ),
  vodDetail: (site: string, id: string) =>
    get<Vod>(`/vod/detail?site=${encodeURIComponent(site)}&id=${encodeURIComponent(id)}`),
  search: (kw: string) => get<{ list: Vod[]; sites: number; total: number }>(`/search?kw=${encodeURIComponent(kw)}`),
  suggest: (kw: string) => get<string[]>(`/suggest?kw=${encodeURIComponent(kw)}`),
  resolve: (url: string) => post<{ url: string; header: Record<string, string>; via: string; ok: boolean }>('/resolve', { url }),

  history: () => get<HistItem[]>('/history'),
  updateHistoryPos: (h: HistItem) => post('/history/pos', h),
  removeHistory: (h: HistItem) => post('/history/remove', h),
  clearHistory: () => post('/history/clear'),

  favorites: () => get<FavItem[]>('/favorites'),
  toggleFavorite: (f: FavItem) => post<{ added: boolean }>('/favorites/toggle', f),
  // 收藏分组
  favGroups: () => get<{ groups: FavGroup[]; defaultCount: number; total: number }>('/favorites/groups'),
  createFavGroup: (name: string) => post<FavGroup>('/favorites/group/create', { name }),
  updateFavGroup: (id: string, name: string) => post<{ ok: boolean }>('/favorites/group/update', { id, name }),
  deleteFavGroup: (id: string) => post<{ ok: boolean }>('/favorites/group/delete', { id }),
  moveFav: (siteKey: string, vodId: string, groupId: string) =>
    post<{ ok: boolean }>('/favorites/move', { siteKey, vodId, groupId }),

  keywords: () => get<string[]>('/keywords'),
  pushKeyword: (kw: string) => post<string[]>('/keywords', { kw }),
  clearKeywords: () => del('/keywords'),

  liveUrl: () => get<{ url: string }>('/live/url'),
  setLiveUrl: (url: string) => post('/live/url', { url }),
  liveSources: () => get<RawLive[]>('/live/sources'),
  setLiveSources: (lives: RawLive[]) => post('/live/sources', { lives }),
  liveChannels: (url?: string) =>
    get<{ root: LiveDir; groups: ChannelGroup[]; url: string }>(
      `/live/channels${url ? `?url=${encodeURIComponent(url)}` : ''}`
    ),
  liveRankOne: (name: string, url: string) => post<LiveScore>('/live/rank/one', { name, url }),
  liveRank: (items: LiveScore[]) => post<LiveScore[]>('/live/rank', { items }),
  liveProbeChannels: (channels: Channel[]) => post<{ dead: number; total: number }>('/live/probe/channels', { channels }),
  peek: (url: string) => get<{ alive: boolean }>(`/peek?url=${encodeURIComponent(url)}`),

  crawlSummary: () => get<{ summary: string; last: number; running: boolean }>('/crawl'),
  triggerCrawl: () => post('/crawl', { force: true }),
  feed: () => get<{ feed: Vod[]; deadKeys: string[]; summary: string }>('/feed'),
  qualityMaster: (url: string) => post<QLevel[]>('/quality/master', { url }),

  prefs: () => get<{ qPref: number; introSkip: number; outroSkip: number; tapSound: number; tapVibrate: number }>('/prefs'),
  setPrefs: (p: Record<string, number>) => post('/prefs', p),

  webdavSources: () => get<WebdavSource[]>('/webdav/sources'),
  saveWebdavSource: (src: WebdavSource) => post('/webdav/sources', src),
  removeWebdavSource: (id: string) => del(`/webdav/sources?id=${encodeURIComponent(id)}`),
  webdavProbe: (src: WebdavSource) => post<{ count: number }>('/webdav/probe', src),
  webdavList: (src: WebdavSource, path: string) => post<WebdavEntry[]>('/webdav/list', { src, path }),
  webdavScan: (src: WebdavSource, maxDepth: number) => post<WebdavVod[]>('/webdav/scan', { src, maxDepth }),
  webdavVods: (srcId: string) => get<WebdavVod[]>(`/webdav/vods?srcId=${encodeURIComponent(srcId)}`),
  webdavHistory: () => get<any[]>('/webdav/history'),
  updateWdHistory: (h: any) => post('/webdav/history/update', h),
  removeWdHistory: (h: any) => post('/webdav/history/remove', h),
  clearWebdavHistory: () => post('/webdav/history/clear'),

  directList: () => get<string[]>('/direct'),
  pushDirect: (url: string) => post('/direct', { url }),
  privacy: () => get<{ agreed: boolean; guideDone: boolean }>('/privacy'),
  setPrivacy: (agree?: boolean, guide?: boolean) => post('/privacy', { agree, guide }),

  pinyinAbbr: (word: string) => get<{ abbrs: string[] }>(`/pinyin/abbr?word=${encodeURIComponent(word)}`)
}

// 视频地址走代理（绕开跨域，m3u8 分片也由代理重写）。
export function proxied(rawUrl: string, auth?: string): string {
  const u = `/api/proxy?url=${encodeURIComponent(rawUrl)}`
  return auth ? `${u}&auth=${encodeURIComponent(auth)}` : u
}
