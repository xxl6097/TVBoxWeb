// Package crawler 实现定时抓取（远程配置刷新 + 逐站抓最新），与鸿蒙影视 Crawler.ets 一致。
package crawler

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/xxl6097/TVBoxWeb/internal/cms"
	"github.com/xxl6097/TVBoxWeb/internal/httpc"
	"github.com/xxl6097/TVBoxWeb/internal/model"
	"github.com/xxl6097/TVBoxWeb/internal/store"
)

var (
	errNoSites  = errors.New("配置里没有 sites 列表")
	errInvalid  = errors.New("订阅不是合法 JSON")
	errNoData   = errors.New("订阅里既没有站点也没有直播源")
)

func fetchText(source string) (string, error) {
	src := strings.TrimSpace(source)
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		return httpc.Text(src, nil, 20*time.Second)
	}
	if src != "" {
		return src, nil
	}
	text := store.CachedText()
	if text == "" {
		return "", errors.New("还没有配置站源")
	}
	return text, nil
}

func parseConfig(text string) (model.RawConfig, error) {
	cleaned := cms.CleanJSON(text)
	var cfg model.RawConfig
	if err := json.Unmarshal([]byte(cleaned), &cfg); err != nil {
		return cfg, errors.New("配置不是合法 JSON，请确认地址返回的是 TVBox 配置")
	}
	return cfg, nil
}

func trim(s string) string { return strings.TrimSpace(s) }

func hasHTTP(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}


const (
	CrawlInterval = 6 * time.Hour
	PerSite       = 10
	FeedMax       = 60
	Conc          = 4
)

type CrawlResult struct {
	At         int64  `json:"at"`
	Sites      int    `json:"sites"`
	Alive      int    `json:"alive"`
	Dead       int    `json:"dead"`
	Items      int    `json:"items"`
	CfgUpdated bool   `json:"cfgUpdated"`
	OK         bool   `json:"ok"`
	Msg        string `json:"msg"`
}

var (
	mu                sync.Mutex
	running           bool
	progressProcessed int
	progressTotal     int
)

func IsRunning() bool {
	mu.Lock()
	defer mu.Unlock()
	return running
}

// Progress 返回抓取进度（已处理数/总数/是否运行中）。
func Progress() (processed, total int, running bool) {
	mu.Lock()
	defer mu.Unlock()
	return progressProcessed, progressTotal, running
}

// Run 立即跑一轮；force=false 时受 6 小时间隔限制。
func Run(force bool) CrawlResult {
	mu.Lock()
	if running {
		mu.Unlock()
		return CrawlResult{OK: false, Msg: "正在更新中，请稍候"}
	}
	running = true
	mu.Unlock()
	defer func() {
		mu.Lock()
		running = false
		mu.Unlock()
	}()

	res := CrawlResult{}

	// 1) 远程配置刷新
	if store.IsCustom() {
		if u := store.GetConfigURL(); u != "" {
			if _, err := loadConfig(u); err == nil {
				res.CfgUpdated = true
			}
		}
	}

	// 2) 逐站抓最新
	sites := store.ActiveSites()
	res.Sites = len(sites)
	if len(sites) == 0 {
		res.Msg = "还没有站源，请先在「设置 → 播放源 → 订阅」导入"
		return res
	}

	// 初始化进度
	mu.Lock()
	progressTotal = len(sites)
	progressProcessed = 0
	mu.Unlock()
	defer func() {
		mu.Lock()
		progressProcessed = progressTotal
		mu.Unlock()
	}()

	var bag []model.Vod
	var dead []string
	var aliveCount int
	var cursor int
	var bagMu sync.Mutex

	worker := func() {
		for {
			bagMu.Lock()
			if cursor >= len(sites) {
				bagMu.Unlock()
				return
			}
			i := cursor
			cursor++
			bagMu.Unlock()

			s := sites[i]
			got := 0
			c := cms.New(s)
			if pg, err := c.List("", 1); err == nil {
				for k, v := range pg.Items {
					if k >= PerSite {
						break
					}
					if v.ID == "" || v.Name == "" {
						continue
					}
					item := model.Vod{
						ID: v.ID, Name: v.Name, Pic: v.Pic, Remark: v.Remark,
						Year: v.Year, Area: v.Area, TypeName: v.TypeName,
						SiteKey: s.Key, SiteName: s.Name,
					}
					bagMu.Lock()
					bag = append(bag, item)
					bagMu.Unlock()
					got++
				}
			}
			if got > 0 {
				bagMu.Lock()
				aliveCount++
				bagMu.Unlock()
			} else {
				bagMu.Lock()
				dead = append(dead, s.Key)
				bagMu.Unlock()
			}
			// 更新进度
			mu.Lock()
			progressProcessed++
			mu.Unlock()
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < Conc; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker()
		}()
	}
	wg.Wait()

	// 3) 按片名去重
	seen := map[string]bool{}
	var feed []model.Vod
	for _, v := range bag {
		k := v.Name + "|" + v.Year
		if seen[k] {
			continue
		}
		seen[k] = true
		feed = append(feed, v)
		if len(feed) >= FeedMax {
			break
		}
	}

	res.Alive = aliveCount
	res.Dead = len(dead)
	res.Items = len(feed)
	res.At = time.Now().UnixMilli()
	res.OK = len(feed) > 0
	if !res.OK {
		res.Msg = "这一轮没有抓到数据，可能是网络不通或站源集体失效"
	}

	store.SaveFeed(feed)
	store.SaveDeadKeys(dead)
	store.SetLastCrawlAt(res.At)
	store.SaveCrawlSummary(SummaryOf(res))
	return res
}

// MaybeRun 到期才跑；没到期返回 nil。
func MaybeRun() *CrawlResult {
	if IsRunning() {
		return nil
	}
	last := store.LastCrawlAt()
	if last > 0 && time.Now().UnixMilli()-last < CrawlInterval.Milliseconds() {
		return nil
	}
	r := Run(false)
	return &r
}

// loadConfig 拉取并载入远程配置。
func loadConfig(source string) (int, error) {
	return LoadConfig(source)
}

// LoadConfig 供 api 包复用：载入配置（URL 或 JSON 文本）。
func LoadConfig(source string) (int, error) {
	text, err := fetchText(source)
	if err != nil {
		return 0, err
	}
	cfg, err := parseConfig(text)
	if err != nil {
		return 0, err
	}
	if len(cfg.Sites) == 0 {
		return 0, errNoSites
	}
	store.BuildSites(cfg)
	store.BuildParses(cfg)
	store.SetCfg(text, source)
	store.ResetCrawlAt()
	return store.TotalSites(), nil
}

// LoadSubscription 订阅：一份 JSON 同时带点播站源与直播源。
func LoadSubscription(source string) (store.SubResult, error) {
	text, err := fetchText(source)
	if err != nil {
		return store.SubResult{}, err
	}
	cfg, err := parseConfig(text)
	if err != nil {
		return store.SubResult{}, errInvalid
	}
	r := store.SubResult{Name: cfg.Name}
	if len(cfg.Sites) > 0 {
		store.BuildSites(cfg)
		store.BuildParses(cfg)
		store.SetCfg(text, source)
		store.SetSubName(cfg.Name)
		store.ResetCrawlAt()
		// 站源列表变了（key 可能不同），清空旧 feed，避免首页旧影片点进详情报「站源不存在」
		store.SaveFeed(nil)
		r.Sites = store.TotalSites()
	}
	if len(cfg.Lives) > 0 {
		var lives []model.RawLive
		for i, l := range cfg.Lives {
			u := trim(l.URL)
			if hasHTTP(u) {
				name := l.Name
				if name == "" {
					name = "直播源" + itoa(i+1)
				}
				lives = append(lives, model.RawLive{Name: name, URL: u})
			}
		}
		store.SetSubLives(lives)
		r.Lives = len(lives)
		if len(lives) > 0 {
			cur := store.LiveURL()
			first := lives[0].URL
			inList := false
			for _, l := range lives {
				if l.URL == cur {
					inList = true
				}
			}
			if first != "" && !inList {
				store.SetLiveURL(first)
			}
		}
	}
	if r.Sites == 0 && r.Lives == 0 {
		return store.SubResult{}, errNoData
	}
	return r, nil
}

// Restore 启动时用缓存恢复站点列表。
func Restore() bool {
	text := store.CachedText()
	if text == "" {
		return false
	}
	cfg, err := parseConfig(text)
	if err != nil || len(cfg.Sites) == 0 {
		return false
	}
	store.BuildSites(cfg)
	store.BuildParses(cfg)
	return true
}

func SummaryOf(r CrawlResult) string {
	t := fmtTime(r.At)
	if !r.OK {
		return t + " 更新失败：" + r.Msg
	}
	s := t + " 更新完成 · " + itoa(r.Alive) + "/" + itoa(r.Sites) + " 个站源有响应 · 收到 " + itoa(r.Items) + " 条"
	if r.Dead > 0 {
		s += " · " + itoa(r.Dead) + " 个无响应"
	}
	if r.CfgUpdated {
		s += " · 配置已刷新"
	}
	return s
}

func fmtTime(ms int64) string {
	if ms <= 0 {
		return "还没更新过"
	}
	t := time.UnixMilli(ms)
	return t.Format("01-02 15:04")
}
