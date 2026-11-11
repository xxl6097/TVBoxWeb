// Package tvbox 提供「夜帧影视」的 Web 服务库，内嵌前端静态资源。
//
// 第三方开发者可直接在自己的 Go 应用中引用本库，一行代码启动一个
// 完整可用的影视聚合点播/直播 Web 服务（Go 后端 + 内嵌 Vue3 前端）。
//
// 用法：
//
//	import "github.com/xxl6097/TVBoxWeb"
//
//	// 最简单：直接启动
//	if err := tvbox.ListenAndServe(":8090"); err != nil {
//		log.Fatal(err)
//	}
//
//	// 或接入自定义 mux / 中间件
//	srv, _ := tvbox.New(tvbox.WithDataDir("./mydata"))
//	http.Handle("/", srv.Handler())
package tvbox

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/xxl6097/TVBoxWeb/internal/api"
	"github.com/xxl6097/TVBoxWeb/internal/crawler"
	"github.com/xxl6097/TVBoxWeb/internal/store"
	"github.com/xxl6097/TVBoxWeb/internal/upgrade"
	"github.com/xxl6097/TVBoxWeb/internal/ws"
)

//go:embed all:dist
var distFS embed.FS

// 全局初始化保护：ws.Init / SetStartTime / SetWSHub 只需执行一次。
var (
	initOnce sync.Once
	initErr  error
)

// Server 代表一个「夜帧影视」Web 服务实例。
type Server struct {
	opts      Options
	mux       *http.ServeMux
	stop      chan struct{}
	closeOnce sync.Once
}

// Options 是库的配置项，通过 Option 函数式选项设置。
type Options struct {
	// DataDir 数据目录，配置/历史/收藏等存为 DataDir/store.json。
	// 默认 "data"。
	DataDir string

	// Logger 日志输出，默认 log.Default()。
	Logger *log.Logger

	// Upgrader 在线升级实现。未设置时升级接口返回「未启用」。
	Upgrader upgrade.Upgrader

	// Version 当前版本号，用于升级检测对比。默认 AppVersion。
	Version string

	// AuthToken 访问口令。非空时 /api/* 与 /ws 必须携带
	// Authorization: Bearer <token> 或 X-Auth-Token: <token>。
	// 默认空（兼容原有单机部署）。
	AuthToken string

	// AllowedOrigins 额外允许的 CORS / WebSocket 来源（同源始终允许）。
	AllowedOrigins []string

	// AllowPrivateUpstreams 允许服务端请求内网地址（家用 NAS / 内网订阅源）。
	// 默认 false：/api/proxy 等回显型接口拒绝私有 IP，防 SSRF。
	AllowPrivateUpstreams bool

	// AllowedHosts 上游域名白名单，空表示不限制（仍做私网校验）。
	AllowedHosts []string
}

// Option 函数式配置项。
type Option func(*Options)

// WithDataDir 设置数据目录。
func WithDataDir(dir string) Option {
	return func(o *Options) { o.DataDir = dir }
}

// WithLogger 设置日志器。
func WithLogger(l *log.Logger) Option {
	return func(o *Options) { o.Logger = l }
}

// 升级相关类型的公开别名，允许外部包实现 Upgrader 接口（同 v1.3.x 兼容层）。
type (
	Release         = upgrade.Release
	UpgradeProgress = upgrade.UpgradeProgress
	UpgradeStage    = upgrade.UpgradeStage
	Upgrader        = upgrade.Upgrader
)

// 升级阶段常量
const (
	StageChecking    = upgrade.StageChecking
	StageDownloading = upgrade.StageDownloading
	StageVerifying   = upgrade.StageVerifying
	StageReplacing   = upgrade.StageReplacing
	StageDone        = upgrade.StageDone
)

// WithUpgrader 注入在线升级实现（复刻 gofs.WithUpgrader）。
func WithUpgrader(u Upgrader) Option {
	return func(o *Options) { o.Upgrader = u }
}

// WithVersion 设置当前版本号（升级检测时与远端比较）。
func WithVersion(v string) Option {
	return func(o *Options) { o.Version = v }
}

// WithAuthToken 开启访问口令保护（公网部署建议设置）。
func WithAuthToken(token string) Option {
	return func(o *Options) { o.AuthToken = strings.TrimSpace(token) }
}

// WithAllowedOrigins 设置额外的 CORS / WebSocket 允许来源。
func WithAllowedOrigins(origins ...string) Option {
	return func(o *Options) { o.AllowedOrigins = origins }
}

// WithAllowPrivateUpstreams 允许请求内网上游（家庭 NAS 场景）。
func WithAllowPrivateUpstreams(allow bool) Option {
	return func(o *Options) { o.AllowPrivateUpstreams = allow }
}

// WithAllowedHosts 限制上游域名白名单。
func WithAllowedHosts(hosts ...string) Option {
	return func(o *Options) { o.AllowedHosts = hosts }
}

// Version 返回当前版本号（可通过 ldflags -X 注入 internal/api.AppVersion）。
func Version() string {
	return api.AppVersion
}

// 默认初始化：只跑一次，线程安全。
func ensureInit() error {
	initOnce.Do(func() {
		api.SetStartTime(time.Now())
		ws.Init()
		api.SetWSHub(ws.GlobalHub)
	})
	return initErr
}

// New 创建一个服务器实例。所有 Option 均在此时生效。
func New(opts ...Option) (*Server, error) {
	o := Options{
		DataDir: "data",
		Logger:  log.Default(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	if err := ensureInit(); err != nil {
		return nil, err
	}

	// 安全配置必须在任何请求进来之前注入
	api.SetSecurity(api.SecurityConfig{
		AuthToken:             o.AuthToken,
		AllowedOrigins:        o.AllowedOrigins,
		AllowPrivateUpstreams: o.AllowPrivateUpstreams,
		AllowedHosts:          o.AllowedHosts,
		Logger:                o.Logger,
	})
	ws.SetOriginChecker(api.OriginAllowed)
	// 新版本能正常启动才清理上一版 .backup，否则可用它手工回滚
	upgrade.ConfirmStartup()

	// 注入升级实现与版本号
	api.SetUpgrader(o.Upgrader)
	if o.Version != "" {
		api.AppVersion = o.Version
	}

	// 数据文件
	dataFile := filepath.Join(o.DataDir, "store.json")
	if err := store.Init(dataFile); err != nil {
		return nil, fmt.Errorf("tvbox: 加载数据失败: %w", err)
	}

	// 启动时恢复缓存站点列表
	if store.TotalSites() == 0 {
		crawler.Restore()
	}

	// store 就绪后再同步可信域名（含网盘源/订阅/直播/EPG），
	// 否则「NAS 公网域名在内网解析成 ULA」会被严格模式拒绝。
	api.RefreshTrustedHosts()

	s := &Server{opts: o, mux: api.Handler(), stop: make(chan struct{})}

	// WebSocket 端点
	wsHandler := ws.NewHandler(ws.GlobalHub)
	s.mux.HandleFunc("/ws", wsHandler.ServeWS)

	// 内嵌前端静态资源
	if err := s.mountStatic(); err != nil {
		return nil, err
	}

	return s, nil
}

// mountStatic 挂载内嵌前端静态资源到根路径。
func (s *Server) mountStatic() error {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return fmt.Errorf("tvbox: 内嵌前端资源不可用: %w", err)
	}
	s.mux.Handle("/", http.FileServer(http.FS(sub)))
	return nil
}

// Handler 返回完整的 http.Handler（含全部 API 与内嵌前端）。
//
// 可嵌入到自定义 mux、net/http 或任意支持 http.Handler 的框架中。
func (s *Server) Handler() http.Handler {
	return api.CORS(s.mux)
}

// ListenAndServe 在指定地址启动服务并阻塞，直到出错或服务关闭。
//
//	addr 形如 ":8090"、"127.0.0.1:9000"。
func (s *Server) ListenAndServe(addr string) error {
	// 后台定时抓取：启动先跑一轮，之后按 CrawlInterval 常驻调度
	s.startScheduler()

	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// 视频流为长连接，不设整体 WriteTimeout；用 IdleTimeout 兜底
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	// SIGINT/SIGTERM 优雅退出：先停止接收新请求，再让在途请求收尾
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		<-sigCh
		s.opts.Logger.Printf("收到退出信号，正在优雅关闭…")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	s.opts.Logger.Printf("夜帧影视 Web 服务已启动：http://localhost%s", addr)
	err := srv.ListenAndServe()
	if err == http.ErrServerClosed {
		err = nil
	}
	_ = s.Close()
	return err
}

// startScheduler 常驻定时抓取：修复原先「只在启动时检查一次」的问题。
func (s *Server) startScheduler() {
	go func() {
		if store.TotalSites() > 0 {
			crawler.MaybeRun()
		}
		t := time.NewTicker(crawler.CrawlInterval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				crawler.MaybeRun()
			case <-s.stop:
				return
			}
		}
	}()
}

// Close 释放资源（停止调度器、关闭 WebSocket Hub）。幂等，可重复调用。
func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		if s.stop != nil {
			close(s.stop)
		}
		ws.Shutdown()
	})
	return nil
}

// ListenAndServe 是包级便捷函数：用默认配置在 addr 启动服务并阻塞。
func ListenAndServe(addr string, opts ...Option) error {
	srv, err := New(opts...)
	if err != nil {
		return err
	}
	return srv.ListenAndServe(addr)
}

// Handler 是包级便捷函数：返回一个用默认配置构建的 http.Handler。
func Handler(opts ...Option) (http.Handler, error) {
	srv, err := New(opts...)
	if err != nil {
		return nil, err
	}
	return srv.Handler(), nil
}
