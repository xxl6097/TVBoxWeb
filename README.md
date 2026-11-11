# 夜帧影视 · TVBoxWeb

将鸿蒙原生（ArkTS）电视端影视客户端 **TVBoxTV** 全部功能，用 **Go 后端 + Vue3 前端** 完整复刻的 Web 应用。

同时，它也是一个**可复用的 Go 依赖库**：第三方开发者可 `go get` 引用，一行代码在自有应用中启动一个完整、内嵌前端的影视聚合服务。

---

## ✨ 特性

- **Go 库 + 内嵌前端**：`go get` 即可用，前端静态资源通过 `go:embed` 内嵌，无外部文件依赖
- **后端**：Golang 标准库 `net/http` 为主，仅依赖 `gorilla/websocket` + `google/uuid`
- **前端**：Vue3 + Vite4 + TypeScript + Element Plus + Pinia + Vue Router
- **播放**：hls.js 播放 HLS/m3u8，`/api/proxy` 视频代理绕开跨域并重写 m3u8 分片地址
- **功能**：点播 / 直播 / 网盘 / 搜索（含拼音）/ 收藏分组 / 历史 / 倍速 / 字幕 / 连播 / 画中画 / PWA 离线 / 性能监控等

> ⚠️ 本库**不含任何影片内容**，只有界面、协议实现与播放器。看什么取决于你导入的 TVBox 配置（JSON）。

---

## 📦 作为 Go 库使用

### 安装

```bash
go get github.com/xxl6097/TVBoxWeb
```

### 最简用法

```go
package main

import (
	"log"

	tvbox "github.com/xxl6097/TVBoxWeb"
)

func main() {
	// 默认监听 :8090，数据存到 ./data 目录
	if err := tvbox.ListenAndServe(":8090"); err != nil {
		log.Fatal(err)
	}
}
```

### 自定义配置

```go
srv, err := tvbox.New(
	tvbox.WithDataDir("./mydata"),   // 数据目录
	tvbox.WithLogger(log.Default()),  // 日志器
)
if err != nil {
	log.Fatal(err)
}
defer srv.Close()

// 拿完整 Handler（含全部 API + 内嵌前端）
http.Handle("/", srv.Handler())

log.Fatal(http.ListenAndServe(":8090", nil))
```

### 嵌入自定义 mux / 框架

`srv.Handler()` 返回标准 `http.Handler`，可挂到任意路由、套中间件，或嵌入 Gin / Echo 等框架：

```go
srv, _ := tvbox.New()
handler := srv.Handler()  // 标准 http.Handler

// 套一个日志中间件
wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	log.Printf("[%s] %s", r.Method, r.URL.Path)
	handler.ServeHTTP(w, r)
})
http.ListenAndServe(":8090", wrapped)
```

完整示例见 [`example/`](./example) 目录。

---

## 🔌 库 API

| 函数 | 说明 |
|------|------|
| `tvbox.New(opts ...Option) (*Server, error)` | 创建服务器实例 |
| `tvbox.ListenAndServe(addr string, opts ...Option) error` | 便捷：直接启动并阻塞 |
| `tvbox.Handler(opts ...Option) (http.Handler, error)` | 便捷：只拿 Handler |
| `(*Server).Handler() http.Handler` | 返回完整 Handler（API + 前端） |
| `(*Server).ListenAndServe(addr string) error` | 在指定地址启动并阻塞 |
| `(*Server).Close() error` | 释放资源 |

**Option 函数式选项：**

| 选项 | 说明 |
|------|------|
| `tvbox.WithDataDir(dir string)` | 设置数据目录（默认 `data`） |
| `tvbox.WithLogger(l *log.Logger)` | 设置日志器（默认 `log.Default()`） |

---

## 🖥️ 作为独立程序使用

### 快速运行

```bash
./run.sh                 # 一键构建前端 + 编译 + 启动 :8090
```

或手动：

```bash
# 1. 构建前端
cd web && npm install && npm run build && cd ..

# 2. 编译（前端已内嵌）
go build -o yezhen ./cmd/tvbox

# 3. 运行
./yezhen -addr :8090 -data ./data
```

### 命令行参数

| 参数 | 默认 | 说明 |
|------|------|------|
| `-addr` | `:8090` | 监听地址 |
| `-data` | `data` | 数据目录（存 `store.json`） |

### 跨平台打包

```bash
./build.sh v1.1.0    # 产出 release/v1.1.0/ 下 6 平台压缩包
```

### Docker

```bash
docker build -t yezhen .
docker run -p 8090:8090 yezhen
```

---

## 📂 目录结构

```
TVBoxWeb/
├── tvbox.go                # 库入口（package tvbox）：embed 前端 + Handler/Serve
├── go.mod / go.sum         # module github.com/xxl6097/TVBoxWeb
├── cmd/tvbox/main.go       # 独立可执行入口（调用 tvbox 库）
├── internal/               # 内部实现（不对外暴露）
│   ├── api/                # HTTP API + 视频代理
│   ├── store/              # JSON 文件持久化
│   ├── cms/                # 苹果CMS 采集协议 JSON/XML
│   ├── resolve/            # 播放地址解析
│   ├── live/               # 直播源解析 M3U/TXT
│   ├── liveprobe/          # 直播源测速 + 频道探活
│   ├── crawler/            # 定时抓取
│   ├── quality/            # HLS 画质解析 + 网速实测
│   ├── webdav/             # 网盘（WebDAV）
│   ├── pinyin/             # 拼音首字母搜索
│   ├── ws/                 # WebSocket 实时推送
│   └── metrics/            # 性能指标采集
├── web/                    # Vue3 前端源码
├── dist/                   # 前端构建产物（go:embed 内嵌，随库分发）
├── example/                # 库使用示例
└── .github/workflows/      # CI：自动编译打包 + 发布 Release
```

---

---

## 🆕 v1.4.0 · 三批体验优化

**第一批（基础体验）**
- 音量偏好持久化：`/api/prefs` 新增 `volumePref`，起播恢复、调整后 500ms 防抖落盘
- 结构化错误码：后端所有错误响应返回 `{ error, code }`（`ERR_TIMEOUT` / `ERR_UPSTREAM` / `ERR_SITE` / `ERR_AUTH` / `ERR_NETWORK` / `ERR_BAD_REQUEST` / `ERR_NOT_FOUND` / `ERR_INTERNAL`），前端 `humanizeApiError()` 统一转成带解决建议的中文提示
- 空态文案统一为「暂无 X，可以 Y」并补引导按钮（首页 / 点播 / 直播 / 收藏 / 历史 / 网盘 / 设置）

**第二批（播放与内容）**
- 外挂字幕：播放设置里可选本地字幕文件或填网络地址，SRT / VTT / ASS 转 WebVTT 后用 `<track>` 渲染；支持 16/20/26/32px 字号、±0.5s 同步、移除字幕；文件只在本机解析，不上传
- 观看历史：按「今天 / 昨天 / 近 7 天 / 更早」分组显示，兼容秒级与毫秒级时间戳
- 直播 EPG 节目单：直播页「节目单」按钮，按频道名宽松匹配（忽略大小写、空格、清晰度后缀），显示正在播出、接下来与当天节目表；XMLTV 源 30 分钟内存缓存，避免每次切台重复下载
  - 配置：面板内填 XMLTV 地址并保存（写入 `prefs.epgUrl`）
  - 接口：`GET /api/live/epg?name=频道名&url=EPG地址`（`url` 可省略，用已保存地址）

**第三批（大列表与外设）**
- 虚拟滚动：新增 `web/src/components/VirtualGrid.vue`（零依赖、固定行高网格，只渲染可视区 + overscan），直播频道列表已接入，上千频道不再一次性挂载 DOM
- 投屏：基于浏览器 Remote Playback API，桌面控制条与移动端辅助按钮行出现「投屏 / 断开投屏」；仅当浏览器暴露 `video.remote` 时显示（Chrome / Edge + Chromecast），不支持时自动隐藏

**验证**：`go build ./...`、`go vet ./...`、`go test ./internal/live`（EPG 解析 + 缓存单测）、`npm run build`（含 `vue-tsc --noEmit`）全部通过。

---

---

## 🆕 v1.5.0 · 三批加固与打磨

**第四批（止血：可用性 / 安全 / 可靠性）**
- **修复在线升级死链**：升级器原先找 `TVBoxWeb_<ver>_<os>_<arch>`，但 CI 产物是 `yezhen-<os>-<arch>`，v1.3.1 起在线升级一直报「未找到安装包」。现已统一产物名，并消费 Release 里的 `checksums.txt` 做 SHA256 校验、semver 只升不降、保留 `.backup` 到下次启动自检通过
- 可选访问口令：`-token`（或 `tvbox.WithAuthToken`），`/api/*` 与 `/ws` 用 `X-Auth-Token`/`Bearer`/query 传递；口令会透传进 m3u8 分片地址。设置页「数据 → 访问口令」配置。默认关闭，兼容原有单机部署
- CORS 由 `*` 收紧为同源 + 白名单；WebSocket 校验 Origin；`/api/proxy`、`/api/peek`、EPG、清晰度、WebDAV 等统一做 SSRF 校验（拒绝内网 / 云元数据 / 保留 IP，重定向逐跳复检），`-allow-private` 可放行家用 NAS；WebDAV 密码回显脱敏
- `store.json` 改为「临时文件 + fsync + rename」原子写，保留 `.bak`；主文件损坏时启动自动回滚备份；读侧 RWMutex、写侧串行化
- `http.Server` 显式超时 + SIGINT/SIGTERM 优雅退出；定时抓取改为常驻 ticker（原先只在启动时检查一次）；HTTP 连接池复用 + 文本响应 8MB 上限

**第五批（状态一致性）**
- 路由 `scrollBehavior` + 点播页把站点/分类/筛选/页码写进 URL，返回列表不再丢状态与滚动位置
- 任何写操作成功后自动失效 IndexedDB / Service Worker / 内存缓存；相同 GET 并发去重；幂等 GET 网络抖动自动重试一次
- 搜索真正透传 `AbortSignal`，快速连搜不再出现旧结果覆盖新结果
- 播放页新增「选集」抽屉（可按集数/名称过滤，桌面与移动端均可进入）
- 统一 `ErrorState` 错误占位与重试按钮（点播 / 详情）
- 设置页新增整站备份导出 / 导入 / 恢复出厂（`/api/backup`、`/api/restore`、`/api/reset`）

**第六批（体验与工程化）**
- 浅色主题 / 深色 / 跟随系统（设置页「数据 → 外观」，持久化到 localStorage）；全局 `:focus-visible` 描边；导航与影片卡片支持键盘 Enter/Space
- 大列表渐进渲染（首屏 60 张 + IntersectionObserver 追加 + `content-visibility`），点播/首页/收藏/搜索不再一次性挂载上千 DOM
- 移动端全屏统一走 CSS 状态并尝试锁定横屏；修复 VodCard 点击态选择器、LiveView WebSocket 回调泄漏、连播倒计时与投屏监听未清理
- 单测覆盖升级校验 / SSRF / 鉴权 / m3u8 口令透传 / store 原子写与并发落盘（`go test -race ./...` 全通过）；CI 新增 vet + race 测试 job；API 请求日志带 requestID / 状态码 / 耗时，panic 打印堆栈
- 版本号统一为 v1.5.0（源码 / Dockerfile / build.sh）；Docker 镜像改为非 root 并声明 `VOLUME /app/data`

**验证**：`go build ./...`、`go vet ./...`、`go test -race ./...`、`npm run build`（含 `vue-tsc --noEmit`）全部通过。

**公网部署建议**：务必加 `-token <口令>`；需要访问内网订阅/网盘源时才加 `-allow-private`；跨域调试用 `-origins https://your-dev-host`；用 Docker 时挂载 `-v $(pwd)/data:/app/data`。

---

## 🆕 v1.5.1 · 在线升级下载通道加固

- **修复**：v1.5.0 虽已能正确找到安装包，但校验文件与二进制都走 `github.com/.../releases/download/...` 直链；该域名在部分网络（尤其国内代理）会被网关拦截返回 502，导致升级卡在下载阶段
- 升级器改为**双通道**：优先 `api.github.com/repos/<owner>/<repo>/releases/assets/<id>`（带 `Accept: application/octet-stream`，公开仓库无需 token），失败自动回退 `browser_download_url`；`Check()` 解析 Release 时保留 asset id
- 二进制与 `checksums.txt` 两个下载点共用同一套双通道逻辑；全部通道失败时错误信息会列出所有尝试过的地址，便于排障
- 新增 6 个单测：通道顺序与去重、API URL 构造、Accept 头、502 自动回退、全失败错误聚合、校验和优先通道
- 真实链路验证：模拟 v1.4.0 客户端检查 → 经 API 通道下载 `yezhen-darwin-arm64`（9,064,898 B）→ SHA256 与 `checksums.txt` 一致；Go 客户端跟随 302 到 `release-assets.githubusercontent.com` 成功

**说明**：断点续传仍未实现；GitHub 未认证 API 限流 60 次/小时，单次升级仅 2 个请求，正常无影响。

---

## 🚀 开发

```bash
# 终端 1：后端
go run ./cmd/tvbox -addr :8090

# 终端 2：前端热更新（Vite 代理 /api → :8090）
cd web && npm run dev
# 打开 http://localhost:5173
```

---

## 📄 免责声明

本项目仅用于技术学习与交流，不含任何影视内容，不提供、不存储、不分发任何版权资源。使用本库时请遵守当地法律法规，勿用于任何侵权用途。因使用本库产生的一切后果由使用者自行承担。

## 📜 License

[MIT](./LICENSE) © 2026 xxl6097
