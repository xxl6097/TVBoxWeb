# 夜帧影视 · Web 版（TVBoxWeb）

将鸿蒙原生（ArkTS）电视端影视客户端 **TVBoxTV** 全部功能，用 **Go 后端 + Vue3 前端** 完整复刻的 Web 应用。

- **后端**：Golang（标准库 `net/http`，零第三方依赖），单二进制 + 内嵌前端静态资源
- **前端**：Vue3 + Vite4 + TypeScript + Element Plus + Pinia + Vue Router
- **播放**：hls.js 播放 HLS/m3u8，`/api/proxy` 视频代理绕开跨域并重写 m3u8 分片地址

> 本应用**不含任何影片内容**，只有界面、协议实现、播放器。看什么取决于你导入的 TVBox 配置（JSON）。

---

## 目录结构

```
TVBoxWeb/
├── server/                 # Go 后端
│   ├── main.go             # 入口：启动服务 + 静态资源 + 定时抓取
│   ├── go.mod
│   ├── dist/               # 前端构建产物（npm run build 生成，go:embed 内嵌）
│   └── internal/
│       ├── model/          # 数据模型 + 界面常量（对应 Model.ets）
│       ├── store/          # JSON 文件持久化（对应 Store.ets）
│       ├── httpc/          # HTTP 客户端：重定向/首包探测/PROPFIND（对应 Http.ets）
│       ├── cms/            # 苹果CMS 采集协议 JSON/XML（对应 Cms.ets）
│       ├── resolve/        # 播放地址解析 parses type=1（对应 Resolve.ets）
│       ├── live/           # 直播源解析 M3U/TXT + 目录树（对应 Live.ets）
│       ├── liveprobe/      # 直播源测速排行 + 频道探活（对应 LiveProbe.ets）
│       ├── crawler/        # 定时抓取（对应 Crawler.ets）
│       ├── quality/        # HLS 画质解析 + 网速实测（对应 Quality.ets）
│       ├── webdav/         # WebDAV 客户端 + 影视识别（对应 Webdav.ets）
│       ├── pinyin/         # 拼音首字母 + T9（对应 Pinyin.ets，全量 GB2312 表）
│       └── api/            # REST API + 视频代理
└── web/                    # Vue3 前端
    ├── vite.config.ts      # 代理 /api → :8080，构建输出到 ../server/dist
    └── src/
        ├── views/          # 11 个页面（见下）
        ├── components/     # TopBar / VodCard / VodGrid / TipToast
        ├── stores/         # app（全局状态）+ player（播放列表单例）
        ├── utils/          # lines / quality / format
        ├── api.ts          # 后端 API 客户端 + proxied()
        ├── types.ts        # 类型（对应 model.go）
        └── theme.ts        # 主题常量（FongMi/TV leanback 色板）
```

## 功能复刻对照表

| 模块 | 鸿蒙影视（源） | Web 版（本仓库） |
|---|---|---|
| 配置导入 | URL/粘贴 JSON，容错 BOM/注释/`class` | `cms.CleanJSON` 同逻辑 |
| 订阅 | 一份 JSON 同时带 sites + lives | `POST /api/subscription` |
| 站源管理 | 开关/可用标注/测速排行 | `/api/sites` `/api/sites/enabled` `/api/sites/speed` |
| 分类浏览 | 分类胶囊 + 海报网格 + 翻页 + 父分类聚合 | `/api/categories` `/api/vod?children=` |
| 筛选 | ext.filters | 前端 `parseFilters` + `/api/vod?ext=` |
| 详情 | 海报/元信息/简介展开/线路/剧集 | `/api/vod/detail` + 前端 `splitLines` |
| 全局搜索 | 最多 18 站分批并发（每批 6） | `/api/search` |
| 播放 | AVPlayer 硬解 | `<video>` + hls.js + `/api/proxy` |
| 播放地址解析 | parses type=1 轮询前 8 个 | `/api/resolve` |
| 历史/收藏 | 本地持久化 + 进度 | `/api/history` `/api/favorites`（JSON 文件） |
| 定时抓取 | 6 小时刷新 + 首页「最近更新」 | `crawler` + `/api/crawl` `/api/feed` |
| 直播 | M3U/TXT 解析 + 三级分组 + 测速 + 换线 | `/api/live/*` |
| 直播测速 | 可播率 70% + 速度 30% 评分 | `liveprobe` |
| 画质自适应 | master playlist + 缓冲水位 | hls.js 多档 + 自动/锁定档位 |
| 跳过片头片尾 | Apple TV 式按钮 + 倒计时 | PlayerView 同逻辑 |
| 直连播放 | 单地址 | `/direct` |
| WebDAV 网盘 | PROPFIND + 递归识别影视 | `/api/webdav/*` |
| 手机扫码录入 | 局域网 TCP 服务 | Web 端浏览器直接输入（无需扫码） |

## 运行

### 开发模式（前后端分离）

```bash
# 1. 启动 Go 后端（:8090）
cd server && go run . 

# 2. 启动 Vite 前端（:5173，自动代理 /api）
cd web && npm install && npm run dev
```

浏览器打开 http://localhost:5173 。

### 生产模式（单二进制）

```bash
# 1. 构建前端到 server/dist
cd web && npm install && npm run build

# 2. 编译 Go（内嵌 dist）
cd ../server && go build -o tvbox .

# 3. 运行
./tvbox -addr :8090 -data ./data
```

浏览器打开 http://localhost:8090 。

> `-addr` 默认 `:8090`（8080 常被 macOS 系统进程占用）；`-data` 指定数据目录，配置/历史/收藏/直播源等都存为 `data/store.json`。

### 跨平台打包（一键产出各平台二进制）

```bash
./build.sh v1.0.0
```

产物在 `release/v1.0.0/`，覆盖 6 个平台（前端已内嵌进二进制，单文件运行）：

| 平台 | 产物 |
|---|---|
| macOS · Apple Silicon (arm64) | `fanren-macos-arm64.tar.gz` |
| macOS · Intel (amd64) | `fanren-macos-amd64.tar.gz` |
| Linux · amd64 | `fanren-linux-amd64.tar.gz` |
| Linux · arm64 | `fanren-linux-arm64.tar.gz` |
| Windows · amd64 | `fanren-windows-amd64.zip` |
| Windows · arm64 | `fanren-windows-arm64.zip` |

> 纯 Go 标准库、`CGO_ENABLED=0`，任意平台可交叉编译；解压后直接运行（默认 http://localhost:8090 ）。

## 首次使用

打开后进入 **设置 → 播放源 → 订阅**，填入一个订阅链接（一份 JSON 同时含点播站源 `sites` 与直播源 `lives`），即可同时更新点播和直播：

```json
{
  "name": "订阅源",
  "sites":  [ /* TVBox 站源，type 0/1/4 协议型 */ ],
  "parses": [ /* 播放地址解析接口 type=1 */ ],
  "lives":  [ { "name": "央视", "url": "https://…/iptv.m3u" } ]
}
```

## 已知边界（与原版一致）

- 爬虫型站源（`type 3`，csp_/jar，背后是 Java 字节码）不支持，站点列表中会标注「需爬虫运行时」。
- 只支持 http/https；`proxy://` `tvbox://` 等伪协议未实现。
- 播放格式取决于浏览器：HLS/m3u8/mp4 正常，DTS/TrueHD 音轨大概率无声。
- 视频地址统一走 `/api/proxy`（含 m3u8 分片重写），绕开跨域。
