// API 客户端：统一封装对 Go 后端的请求。

import type {
  Site,
  Category,
  Vod,
  VodPage,
  HistItem,
  FavItem,
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

const BASE = '/api'

async function request<T>(path: string, opts: RequestInit = {}): Promise<T> {
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
  return res.json() as Promise<T>
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
