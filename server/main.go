// 夜帧影视 · Web 版（Go 后端）
//
// 单文件二进制 + 内嵌前端静态资源，或开发时前后端分离（Vite dev server 代理）。
package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"tvbox/internal/api"
	"tvbox/internal/crawler"
	"tvbox/internal/store"
	"tvbox/internal/ws"
)

//go:embed all:dist
var distFS embed.FS

// startTime 服务启动时间，用于 /api/health 返回 uptime。
var startTime = time.Now()

func main() {
	addr := flag.String("addr", ":8090", "监听地址")
	dataDir := flag.String("data", "data", "数据目录")
	flag.Parse()

	// 记录启动时间
	api.SetStartTime(startTime)

	// 初始化 WebSocket Hub
	ws.Init()
	api.SetWSHub(ws.GlobalHub)
	defer ws.Shutdown()

	// 数据文件
	dataFile := filepath.Join(*dataDir, "store.json")
	if err := store.Init(dataFile); err != nil {
		log.Printf("加载数据失败：%v", err)
	}

	// 启动时恢复缓存站点列表
	if store.TotalSites() == 0 {
		crawler.Restore()
	}

	mux := api.Handler()

	// WebSocket 端点
	wsHandler := ws.NewHandler(ws.GlobalHub)
	mux.HandleFunc("/ws", wsHandler.ServeWS)

	// 静态资源：优先本地 dist，其次内嵌
	if _, err := os.Stat("dist"); err == nil {
		mux.Handle("/", http.FileServer(http.Dir("dist")))
		log.Println("使用本地 dist 静态资源")
	} else {
		if sub, err := fs.Sub(distFS, "dist"); err == nil {
			mux.Handle("/", http.FileServer(http.FS(sub)))
			log.Println("使用内嵌 dist 静态资源")
		}
	}

	log.Printf("夜帧影视 Web 版已启动：http://localhost%s", *addr)

	// 后台定时抓取（首次启动先触发一轮，非阻塞）
	go func() {
		if store.TotalSites() > 0 {
			crawler.MaybeRun()
		}
	}()

	if err := http.ListenAndServe(*addr, api.CORS(mux)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
