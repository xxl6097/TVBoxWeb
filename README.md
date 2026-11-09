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
