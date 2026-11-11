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

### 账号密码登录（v1.7.0）

上层库可以自己决定账号密码，**不配置就保持原来的免登录行为**：

```go
srv, err := tvbox.New(
	tvbox.WithDataDir("./mydata"),
	// 用户名与密码都非空才启用；任一项为空视为不启用
	tvbox.WithLogin("admin", os.Getenv("YEZHEN_PASS")),
)
```

启用后：

- 访问 `/api/*` 与 `/ws` 必须先 `POST /api/login {user, pass}` 换取会话 Cookie（`yz_session`，HttpOnly + SameSite=Lax，7 天有效）；
- 前端会自动跳到 `/#/login`，登录成功回跳到原页面；任意接口返回 401 会自动踢回登录页；
- 会话签名密钥每个进程随机生成，**重启后需重新登录**（密钥不落盘）；
- 同一 IP 10 分钟内失败 8 次会临时锁定，防暴力破解；
- 与旧的访问口令 `WithAuthToken` 可以并存，任一凭证通过即放行。

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
| `-user` | 空 | 登录用户名（与 `-pass` 同时设置才启用登录页；也可用环境变量 `YZ_USER`） |
| `-pass` | 空 | 登录密码（与 `-user` 同时设置才启用登录页；也可用环境变量 `YZ_PASS`） |
| `-secure-cookie` | 关 | 会话 Cookie 加 `Secure`（HTTPS 但反代不透传 `X-Forwarded-Proto` 时才需要；HTTP 下开启会导致登录失效） |

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
- 可选账号密码登录：`-user` / `-pass`（或 `tvbox.WithLogin`），启用后未登录会被路由守卫拦到登录页
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

**公网部署建议**：务必加 `-token <口令>`；跨域调试用 `-origins https://your-dev-host`；用 Docker 时挂载 `-v $(pwd)/data:/app/data`。

**关于内网地址（NAS / 路由器）**：SSRF 防护分两级——
- **源配置类**接口（WebDAV、订阅、直播源、EPG）允许指向内网，因为这些地址本来就是你自己填的；
- **内容回显类**接口（`/api/proxy`、m3u8 解析）默认拒绝内网，避免把内网暴露成开放代理；
- 例外是**你保存过的源域名**：即使它解析到内网（家里 NAS 的公网域名在内网被 DNS 解成 ULA `fd00::/8` 或 `192.168.x.x` 很常见），也会被自动加入可信列表并放行，所以 NAS 上的视频能正常经 `/api/proxy` 播放。
- 只有「临时用一次、没配置成源」的内网地址才需要 `-allow-private` 兜底。

---

## 🆕 v1.9.2 · 修复「登录后网盘视频播不了」

这是**登录功能引入的回归**，不是网盘或播放器退化。

### 原因：`<video>` 的请求带不了会话 Cookie

启用 `-user/-pass` 后，`/api/proxy` 需要会话 Cookie 才能通过鉴权。但浏览器加载 `<video src="...">` 是子资源请求：

- 没加 `crossorigin` → 不是 CORS 凭证请求，**不带 Cookie**
- 加了 `crossorigin` 又要求 `SameSite=None; Secure`，而 HTTP 部署设不了 `Secure`

于是：**页面本身一切正常（fetch 同源自动带 Cookie），唯独视频请求 401**。浏览器再把 401 返回的 JSON 当视频数据去解码，报成「这个文件无法直接播放」。

### 修复：登录时下发「媒体令牌」

- `POST /api/login` 响应增加 `mediaToken` 字段（与会话同源、同有效期，但值独立于 Cookie）。
- 前端存进 `sessionStorage`，`proxied()` / `remuxed()` 统一经 `withToken()` 拼成 `&token=`。
- 后端 `authOK` 接受该令牌，**但只对 `/api/proxy`、`/api/remux` 生效**——URL 泄露也只暴露「能看这个视频」，拿不到改配置、看历史等管理权限。
- 未启用登录时不下发令牌；登出时清除。
- 旧的 `-token` 访问口令仍然可用（两者取其一即可）。

### 验证

| 场景（不带 Cookie，模拟 `<video>`） | 结果 |
|---|---|
| MP4 + mediaToken | 206 `video/mp4` |
| MKV + mediaToken（转封装） | 200 `video/mp4` |
| 无 token | 401 |
| 伪造 token | 401 |
| mediaToken 调 `/api/status` | 401（被拒，符合预期） |

新增 3 组单测（媒体令牌可用 / 不可越权 / 端点白名单最小化）+ 5 项前端检查。已验证：临时关掉媒体令牌分支时，测试精确失败在「应接受媒体令牌」。

---

## 🆕 v1.9.1 · 修复进度条拖不动

- **现象**：进度条按住拖动没反应，所有文件都这样（不只是 MKV）。
- **根因**：`pointermove` / `pointerup` 绑在进度条元素自己身上，靠 `setPointerCapture` 兜底。但进度条只有 ~26px 高，拖快一点指针就离开元素；一旦捕获失败或元素被重渲染，`pointermove`/`pointerup` 就再也收不到 —— 表现为「拖不动」。
- **修复**：按下时把 `pointermove` / `pointerup` / `pointercancel` **绑到 window**，松手后立即移除。这样指针移到哪里都能跟住。
- **顺带**：
  - 三处进度条（桌面 1 + 移动端 2）改用**函数式 ref**（`setProgressEl`）——同名 `ref="xxx"` 在移动端模板里会互相覆盖，取到错元素。
  - 拖动落点、跳过片头统一走 `applySeek()`，转封装模式（MKV）也能正常拖。
  - 加 `?debug=1` 诊断开关，打印 `[seek] down/up` 便于以后排查。
- **防回归**：新增 `web/scripts/check-seek.cjs`（10 项静态断言），接入 CI。已验证：回退修复后 6 项失败。
- 另用 jsdom 实测确认：指针移出元素后 window 仍能收到 `pointermove`/`pointerup`，拖动比例与松手结果都正确。

---

## 🆕 v1.9.0 · MKV 等容器转封装播放（需要 ffmpeg）

### 为什么之前的版本「能播」，现在「不行」

不是回归，是**以前那个文件根本没进列表**。v1.8.0 的识别逻辑按「所在目录」分组，`凡人/07 外海风云/143.mkv` 会把整组归成一部叫「外海风云」的剧；v1.8.0 起改为按「剧名」归并，`143.mkv` 才第一次显示在「凡人」的第 7 季里。文件一直在，只是以前没露出来。

### 浏览器为什么播不了 MKV

`143.mkv` 里面是 **H.264 High + AAC-LC（4K）**，编码本身浏览器完全能解。**卡在容器**：Chrome / Edge / Firefox 的 `<video>` 与 MSE 都不支持 Matroska。

### 解决方案：服务端转封装（不是转码）

新增 `GET /api/remux?url=&auth=&seek=`：

- 用 `ffmpeg -c copy` 把 MKV 里的音视频流**原样搬进 fMP4**，**不重新编码，画质零损失**，CPU 几乎不花（实测 2.5GB 4K 文件取 20 秒约 6 秒，主要是网络 IO）。
- `frag_keyframe+empty_moov` → **边转边播**，不用等整片转完。
- 等第一块数据到了才写响应头：源不可解时返回 JSON 错误而不是空流。
- `context` 跟随客户端：关掉页面 ffmpeg 立即退出，不留孤儿进程。
- 复用 `/api/proxy` 的 SSRF 校验（内网地址默认拒绝）。
- `-ss` 快速定位支持拖进度条；输出时间戳从 0 起算，前端用 `remuxSeek` 补回偏移，进度条与总时长都正确。

**前端行为**：`.mkv/.avi/.wmv/.rmvb/.rm/.mov/.ts/.m2ts/.mpg/.mpeg/.vob` 自动走转封装；`.mp4/.webm` 仍走原来的代理直放。`/api/status` 新增 `remux` / `remuxReason` 字段，没有 ffmpeg 时前端会明确提示「安装 ffmpeg 后重启即可，画质不受影响」。

### 需要装 ffmpeg

```bash
# macOS
brew install ffmpeg
# Debian/Ubuntu
apt install ffmpeg
# 群晖/威联通：套件中心装 ffmpeg，或 docker 镜像里带上
```

没装也能正常用，只是 MKV 播不了，会给出明确提示（而不是笼统的「格式不支持」）。

> Docker 镜像目前不含 ffmpeg（会显著增大体积）。需要 MKV 支持请在镜像里加装，或把容器换成自带 ffmpeg 的基础镜像。

---

## 🆕 v1.8.2 · 修复网盘视频播放报「解码失败」

- **现象**：网盘里的视频点播放，提示「视频解码失败，可能是编码格式不支持」。
- **根因**：`PlayerView` 里写成 `proxied(finalUrl, directMode ? player.header?.auth : undefined)` —— **只有直链模式才带认证**。网盘播走的是普通代理模式，认证被丢掉 → `/api/proxy` 不带 Basic Auth → 上游返回 401 → `<video>` 抛 `MEDIA_ERR_SRC_NOT_SUPPORTED` → 被统一误报成「解码失败」。
- **排查结论**：源文件本身完全正常（H.264 High / 4K / AAC-LC，`moov` 在文件头可边下边播，大文件 Range 请求返回 206）。问题只在认证透传。
- **修复**：`proxied(finalUrl, player.header?.auth || undefined)` —— 认证与「是否直连」无关，只要上游需要就带。
- **顺带改进错误提示**：`vid.onerror` 不再一律报 `mediaError`，改为按 `MediaError.code` 区分：
  - `MEDIA_ERR_NETWORK` → 网络问题
  - `MEDIA_ERR_DECODE` → 真的解码失败
  - `MEDIA_ERR_SRC_NOT_SUPPORTED` → 进一步看是否 401/403，提示「需要认证」或「格式不支持（MKV/HEVC 等）」

  旧行为会把人往「编码格式」方向带偏，而实际可能是认证或地址问题。
- **防回归**：新增 `web/scripts/check-wd-auth.cjs`，静态检查「代理 URL 必须透传 auth」「video error 不能一进来就报 mediaError」，接入 CI。已验证：回退修复后 4 项失败。
## 🆕 v1.8.1 · 安全加固 + 清晰度解析修复 + 补测试

### 安全：会话 Cookie 支持 Secure

- 新增 `-secure-cookie` / `tvbox.WithCookieSecure(true)`。默认关闭以兼容家用 HTTP 部署。
- **自动判断**：直连 TLS（`r.TLS != nil`）或反代带来 `X-Forwarded-Proto: https` 时自动加 `Secure`，一般部署无需配置。
- **HTTP 下强制不加**：`Secure` Cookie 在 HTTP 页面会被浏览器**静默丢弃**，配错的表现是「密码正确 → 提示成功 → 立刻回登录页」，用户完全无从排查。所以宁可弱一点也不锁死自己。
- 登出时的清除指令也带同样的 `Secure`，否则浏览器可能不认这条删除。

### 修复：HLS 多档码率解析取错值

`strAttr` 用 `strings.Index(line, key+"=")` 查找，查 `BANDWIDTH` 会先命中 `AVERAGE-BANDWIDTH=` 里的片段：

```
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=800000,BANDWIDTH=1200000
                                    ↑ 错误命中这里
```

结果取到 800000 而非 1200000，导致 `PickBySpeed` 选档偏低。改为校验 key 前一个字符必须是 `,` / `:` / 行首。同时更正一处：`0` 之前被当成「无数据」返回空串，现保持原行为并加注释与测试锁定。

### 登录页显示版本号

`/api/version` 加入鉴权豁免名单（只含版本/构建时间/平台，无用户数据）。登录页底部显示 `夜帧影视 · Yezhen v1.8.1`，排查「用户在旧版本上反馈问题」时很省事。新增测试断言豁免名单**只放行必要接口**，其余 8 个接口未登录必须 401。

### 其他

- `quality` 包从 0 测试补到 9 组（清晰度标签、master playlist 解析、按速率/分辨率选档、首分片提取、速率格式化）。
- 清理 `TopBar` 里只写不读的 `clock` 变量（模板实际用 `clockParts`），改名为名副其实的 `clockText` 供无障碍标签使用。

---

## 🆕 v1.8.0 · 设置页「识别影视」+ 剧集归并修复

### 新增：设置 → 网盘 → 识别影视

对齐鸿蒙版：一条入口进去，选网盘、定扫描深度，递归扫目录树把视频文件聚合成「电影 / 剧集」，扫完直接跳网盘页选集播放。入口上会显示「已识别 N 部」摘要；没扫过时显示引导文案；一个网盘都没有时该项置灰并提示先添加挂载。

- 扫描结果由后端缓存（`/api/webdav/scan` → `store.SaveWebdavScan`），进网盘页优先读缓存，不重复扫。
- 网盘页新增 `?src=<id>` 直接进入某源的识别结果，供「查看结果」按钮使用。

### 修复：一部剧被拆成十几部

真实 NAS 验证时发现的三个问题：

1. **「剧名/01 篇章/1.mp4」结构被按目录拆开** —— 目录 `凡人/01 风起天南`…`09 慕兰之战` 各算一部，一部 197 集的剧变成 11 条记录。改为**按「剧名」这一级归并**，篇章目录作为季（Season），集号保持原著连续编号。修复后：18 部 → 10 部，「凡人」合并为 197 集 / 9 篇章，总集数 206 一集未丢。
2. **`reDirPrefix` 把四位年份当季号** —— 正则 `^\d{1,3}` 会把「2024」截成 202。改为要求数字后有分隔符或空白，四位年份不再被误判。
3. **聚合逻辑不可单测** —— 抽出 `aggregate()` 独立函数，新增 5 组单测（篇章归并、电影不合并、单目录剧集、集号解析、季号提取），并补 `cleanDirName` 兼容性测试确认改动没破坏旧行为。

---

## 🆕 v1.7.2 · 加载体验优化 + 时钟精确到秒

### 加载体验

- **首屏骨架前移到 index.html**：在 `<head>` 内联样式 + `<body>` 里放品牌标记与细进度条，JS 还没下载解析就能画出底色与动画，不再先白/黑屏再突然出内容。样式内联、不依赖外部 CSS；`main.ts` 挂载前移除它，由 App 的启动态无缝接力（两者同构，用户看不到跳变）。
- **启动态不再「等死」**：原来 `loadStatus()` 与 `privacy()` **串行** await，任意一个慢都会让「加载中…」一直挂着。改为并行 + 3.5s 兜底超时——超时先放界面出来，数据到位后靠响应式更新，不再拿加载态挡人。
- **路由切换有反馈**：新增顶部 2px 进度条，`beforeResolve` 起、`afterEach`/`onError` 收；延迟 180ms 才出现，已缓存的页面切换不会闪一下，只有首次加载 chunk 真慢时才提示。
- 启动态视觉与首屏骨架统一（品牌标记 + 滑动进度条），全部支持 `prefers-reduced-motion`。

### 时钟

- 右上角时间由 `HH:mm` 改为 **`HH:mm:ss`**，刷新间隔 30s → 每秒，并对齐到整秒再起定时器，避免累积漂移。
- 时分与秒拆成两个节点：秒透明度 0.65 做视觉弱化；配合 `font-variant-numeric: tabular-nums` 与固定宽度，秒数跳动时不会左右抖动或推挤相邻元素。

---

## 🆕 v1.7.1 · 修复播放设置面板打不开关不掉

- **现象**：进播放页就弹出「播放设置」，点「关闭」没反应，点遮罩也关不掉。
- **根因**：`PlayerSettingsPanel` 两个根节点写的是 `v-if="variant === 'desktop'"`，**只判断了形态、没判断 `modelValue`**。组件内部从不观察 `modelValue`，父组件把 `showQ` 置 false 时只有父层 `v-if` 才能卸载它；这里没写父层 `v-if`，于是面板一旦渲染就永久驻留 —— 关闭按钮其实一直在正常 emit，只是组件不接受。
- **修复**：两个分支的可见性都改成 `v-if="modelValue && …"` / `v-else-if="modelValue"`，组件自己遵守 `v-model` 契约。
- **防回归**：新增 `web/scripts/check-panel.cjs`（jsdom 直接编译真实 .vue，点击真实按钮与遮罩），覆盖桌面/移动两种形态共 14 项断言，并接入 CI 的 `npm run check:panel`。已验证：回退到修复前版本时 8 项失败（「初始不显示」「点关闭后消失」全部命中），修复后全绿。

---

## 🆕 v1.7.0 · 账号密码登录

- **新增登录页**：`/#/login`，深色极简风格（光晕 + 网格底纹 + 玻璃卡片刻度），跟随现有主题令牌，亮色主题同样适配；移动端单独调过间距。
- **没登录进不去首页**：路由守卫在每次跳转前确认会话，未登录一律重定向到登录页并带上 `?redirect=`，登录后回跳原页面；任意接口返回 401 也会自动踢回登录页。
- **账号密码由上层决定**：`tvbox.WithLogin(user, pass)`，命令行对应 `-user` / `-pass`（也认 `YZ_USER` / `YZ_PASS`）。**两项都非空才启用**，不配置时行为与之前完全一致（免登录）。
- **会话**：`POST /api/login` 校验后下发 HttpOnly Cookie `yz_session`（HMAC-SHA256 签名，7 天有效）。密钥每进程随机，重启即失效，不落盘；密码只做等时比较，不写日志、不回显。
- **防爆破**：同一 IP 10 分钟内失败 8 次临时锁定（429）。
- **与旧口令并存**：`WithAuthToken` 仍然有效，任一凭证通过即放行。
- **设置页**：启用登录后，「关于」里显示当前账号并提供「退出登录」。

---

## 🆕 v1.6.3 · 修复 WebDAV 密码被掩码覆盖导致 401

- **现象**：进「网盘 → 编辑挂载」，密码框里自动显示成 6 个星号；点「测试连接」报 `WebDAV HTTP 401，用户名或密码错误`。
- **原因**：列表接口出于安全只回显 `******`，前端把整个对象回填进表单，掩码就进了密码框；点测试连接时原样提交，服务端拿 `******` 当密码去认证 → 401。保存时同理会把真实密码覆盖成星号。
- **修复（服务端兜底 + 前端体验）**：
  - 新增 `resolveWdPass`：**掩码和留空都解释成「沿用已保存的密码」**，按 `id`（其次按「地址 + 用户名」，忽略尾部斜杠）找回；找不到对应已存源（属于新增）则保持为空，即匿名访问。
  - `probe` / `sources` POST / `list` / `scan` 四个接口统一走这套还原逻辑——原来只有 POST 做了掩码判断，`probe`、`list`、`scan` 都会拿星号去认证。
  - 前端编辑时不再把掩码填进密码框，改为留空 + 提示「密码已保存在服务端，留空即不修改」。
- **升级完成后自动刷新界面**：`done` 事件后 1.6 秒自动 `location.reload()`，完成后一行显示「升级到 vX 成功 · 正在刷新界面…（重启服务后新版本生效）」。
- 新增 2 个单测：`resolveWdPass` 五种分支（掩码还原 / 留空沿用 / 无 id 按地址兜底 / 新密码不覆盖 / 匿名源不串密码），以及「用掩码再保存不会把真实密码写成星号」的接口级回归。

---

## 🆕 v1.6.1 · 修复内网 NAS 无法挂载 WebDAV（SSRF 策略分级）

### 顺带修掉：升级进度显示成一个「1」，进度条根本不动

- **原因**：`UpgradeStage` 是 `int`，虽然实现了 `String()`，但 `encoding/json` 不认它，SSE 推的是 `{"stage":1}`。前端查表 `stageMap[1]` 落空，退回渲染原始值 → 界面上孤零零一个「1」；同时 `stage === 'downloading'` 恒为 false，**已下载字节与进度条从未更新过**，你看到的「已下载 28.0%」只是后端塞在 message 里的文字。
- **后端**：给 `UpgradeStage` 加 `MarshalJSON`/`UnmarshalJSON`，阶段以名字（`"downloading"`）出现在 JSON 里；反序列化同时兼容旧版数字码，库使用者不受影响。
- **前端**：加阶段归一化，数字码 / 数字字符串 / 名字三种写法都能识别（旧服务端配新前端也不会退化成数字）；未知阶段显示「处理中…」而不是原始值。
- **顺带把进度区做完整**：标题行左边阶段、右边「28% · 4.2 MB / 15.1 MB」，百分比用后端 `loaded/total` 真实计算；下载阶段不再重复显示「已下载 xx%」的 message，校验/替换阶段才显示文字说明；进度条补 `role="progressbar"` 与 `aria-valuenow`。


- **问题**：家里 NAS 的公网域名在内网会被 DNS 解析出一个 ULA 内网 IPv6（如 `fd4f:eb21:d3dc::1`）和一个公网 IPv4。v1.6.0 的 SSRF 校验遍历全部解析结果，只要有一条是私网就拒绝，于是保存 WebDAV 源直接报「目标地址不安全」。地址和密码本身没问题（PROPFIND 返回 207）。
- **修复**：把校验拆成两级策略，而不是一个全局开关：
  - `netguard.Check`（严格）：只用于内容回显型接口 `/api/proxy`、`/api/quality/master`；
  - `netguard.CheckSource`（源模式）：WebDAV / 订阅 / 直播源 / EPG / 探活等「用户指向自己服务器」的接口，允许私有地址；
  - 新增**可信域名列表**：用户保存过的源域名（网盘源、站源 API、直播源、EPG、订阅）解析到内网也放行，并在严格模式下同样生效——这样挂载成功后视频经 `/api/proxy` 播放不会被拦。
- 可信域名在启动时以及每次「导入订阅 / 保存或删除网盘源 / 切换直播源 / 改 EPG」后自动重建，变更时清空 DNS 判定缓存。
- 被拦时的报错改成可操作提示：说明可以「保存为源」或加 `-allow-private`，不再只甩一句「不安全」。
- 新增 7 个单测：源模式允许私网（含 ULA 字面量）、严格/源模式行为差异、可信域名跳过 DNS、可信域名仍拒绝非 http(s) 与内嵌账号密码，以及 3 个接口级回归（真实场景保存成功、`/api/proxy` 对未配置的内网地址仍拒绝、非 http 协议仍拒绝）。

---

## 🆕 v1.6.0 · 请求可取消 + 升级续传 + 播放页拆分 + 真虚拟滚动

**后端**

- **`context` 全链路透传**：`httpc` / `cms` / `live` / `webdav` / `resolve` / `crawler` / `quality` / `upgrade` 全部新增 `XxxCtx` 变体，HTTP handler 统一传 `r.Context()`。客户端关页面或断网后，正在跑的上游请求会立刻中止，不再白等满超时（原先多站并发搜索/直播源拉取会一直烧到超时）。旧的 `Xxx` 函数保留为 `context.Background()` 包装，第三方调用不受影响。
- **在线升级断点续传**：下载改为 `.part` 文件 + `Range: bytes=<offset>-` 续传，中断后最多自动重试 3 次；服务端不支持 Range（回 200）时自动整包重下；`.part` 比目标大视为脏数据丢弃；已下完但没走完校验的直接复用。缓存目录在用户缓存区（`<cache>/yezhen-upgrade/`），换版本时清掉旧 `.part`。
- **历史记录索引**：点播历史与网盘历史各建 `map[key]下标`，`UpdateHistoryPos`（播放中每 5 秒调一次）从线性扫描变 O(1)；新增 `FindHistory` / `FindWdHistory`。网盘历史上限 500 条，避免 `store.json` 无限膨胀。

**前端**

- **播放页拆分**：`PlayerView.vue` 从 2356 行降到 1924 行。字幕状态机 → `composables/usePlayerSubtitles.ts`，移动端手势 → `composables/usePlayerGestures.ts`，字幕格式转换 → `utils/subtitle.ts`，播放设置面板与选集抽屉 → `components/player/`。桌面浮层与移动 sheet 原本是两段几乎重复的模板，现在共用 `PlayerSettingsPanel.vue`。
- **`VodGrid` 真虚拟滚动**：DOM 节点数恒定在 2~3 屏（原来首屏 60 条、滚动不断累积）。列数/列宽/行高全部从真实布局测量（`gridTemplateColumns`），响应式断点不用写死数字；从详情页返回时按滚动位置还原窗口，不再依赖已卸载节点。
- **站源健康度**：`/api/sites/health` 原本没有消费方，现在点播页站源下拉用绿/黄/红/灰圆点显示正常/偏慢/不可用/未知。

**工程化**

- CI actions 全部升到 Node 24 运行时：`checkout@v7`、`setup-go@v7`、`setup-node@v7`、`upload-artifact@v7`、`download-artifact@v8`、`action-gh-release@v3`，消除「forced to run on Node.js 24」弃用告警；每个 job 补 `timeout-minutes`。
- 新增单测：`httpc` ctx 取消能真正中断上游（4 个）、升级断点续传（6 个）、历史索引一致性（2 个）。
- 优化档案归档到 `docs/optimization/`，`OPTIMIZATION_TASKS.md` 逐条回勾（12 项里 11 项完成，剩余 3 条已标注原因）。

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
