// Package store 实现配置与本地数据持久化（JSON 文件），与鸿蒙影视 Store.ets 对应。
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"tvbox/internal/model"
)

// Data 是落盘结构，字段名与鸿蒙 preferences 键一一对应。
type Data struct {
	CfgText      string               `json:"cfgText"`
	CfgURL       string               `json:"cfgUrl"`
	CfgCustom    string               `json:"cfgCustom"`
	SubName      string               `json:"subName"`
	Enabled      []string             `json:"enabled"`
	Hist         []HistItem           `json:"hist"`
	Fav          []FavItem            `json:"fav"`
	KwHist       string               `json:"kwHist"`
	LiveURL      string               `json:"liveUrl"`
	SubLives     []model.RawLive      `json:"subLives"`
	QPref        int                  `json:"qPref"`
	IntroSkip    int                  `json:"introSkip"`
	OutroSkip    int                  `json:"outroSkip"`
	TapSound     int                  `json:"tapSound"`
	TapVibrate   int                  `json:"tapVibrate"`
	Feed         []model.Vod          `json:"feed"`
	CrawlAt      int64                `json:"crawlAt"`
	CrawlSummary string               `json:"crawlSummary"`
	DeadKeys     []string             `json:"deadKeys"`
	PrivacyAgreed bool                `json:"privacyAgreed"`
	GuideDone    bool                 `json:"guideDone"`
	DirectList   []string             `json:"directList"`
	WebdavList   []model.WebdavSource `json:"webdavList"`
	WebdavScan   map[string][]model.WebdavVod `json:"webdavScan"`
	WdHist       []model.WdHistory    `json:"wdHist"`
	SiteHealth   map[string]SiteHealthStat `json:"siteHealth"` // 站源健康度统计
	FavGroups    []FavGroup           `json:"favGroups"`      // 收藏分组列表
}

// SiteHealthStat 站源健康度统计数据。
type SiteHealthStat struct {
	LastSuccess int64 `json:"lastSuccess"` // 最后成功时间戳（秒）
	LastFail    int64 `json:"lastFail"`    // 最后失败时间戳（秒）
	TotalReq    int   `json:"totalReq"`    // 总请求数
	FailCount   int   `json:"failCount"`   // 失败次数
	AvgMs       int   `json:"avgMs"`       // 平均响应时间（毫秒）
}

type HistItem struct {
	SiteKey  string `json:"siteKey"`
	SiteName string `json:"siteName"`
	VodID    string `json:"vodId"`
	Name     string `json:"name"`
	Pic      string `json:"pic"`
	EpName   string `json:"epName"`
	EpIndex  int    `json:"epIndex"`
	From     string `json:"from"`
	Pos      int64  `json:"pos"`
	Dur      int64  `json:"dur"`
	Time     int64  `json:"time"`
}

type FavItem struct {
	SiteKey  string `json:"siteKey"`
	SiteName string `json:"siteName"`
	VodID    string `json:"vodId"`
	Name     string `json:"name"`
	Pic      string `json:"pic"`
	GroupID  string `json:"groupId"` // 所属分组 ID，空字符串表示默认分组
}

// FavGroup 收藏分组。
type FavGroup struct {
	ID   string `json:"id"`   // 分组 ID（UUID）
	Name string `json:"name"` // 分组名称
	Time int64  `json:"time"` // 创建时间
}

type SubResult struct {
	Name  string `json:"name"`
	Sites int    `json:"sites"`
	Lives int    `json:"lives"`
}

var (
	mu      sync.Mutex
	path    string
	data    = &Data{
		WebdavScan: map[string][]model.WebdavVod{},
		SiteHealth: map[string]SiteHealthStat{},
	}
	siteList []model.Site
	parseList []model.ParseRule
	rawSites int
	
	// 版本追踪用于 API 缓存（ETag）
	sitesVersion  int64 // 站源列表最后更新时间戳
	configVersion int64 // 配置最后更新时间戳
)

// Init 指定数据文件路径并加载。
func Init(p string) error {
	mu.Lock()
	defer mu.Unlock()
	path = p
	if p == "" {
		return nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(b) == 0 {
		return nil
	}
	d := &Data{}
	if err := json.Unmarshal(b, &d); err != nil {
		return err
	}
	if d.WebdavScan == nil {
		d.WebdavScan = map[string][]model.WebdavVod{}
	}
	data = d
	return nil
}

func save() {
	if path == "" {
		return
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	b, _ := json.MarshalIndent(data, "", "  ")
	_ = os.WriteFile(path, b, 0o600)
}

// flush 线程安全地把当前 data 序列化写入磁盘。
// 调用方不需要持锁；本函数内部加锁做快照，再在锁外写文件，
// 避免锁竞争同时保证写入的是一致快照。
func flush() {
	if path == "" {
		return
	}
	mu.Lock()
	b, _ := json.MarshalIndent(data, "", "  ")
	mu.Unlock()
	os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, b, 0o600)
}

// ---------- 配置 ----------

func IsCustom() bool { return data.CfgCustom == "1" }
func GetConfigURL() string { return data.CfgURL }
func SubName() string { return data.SubName }
func CachedText() string { return data.CfgText }

// SetCfg 保存配置原文与来源地址。
func SetCfg(text, source string) {
	mu.Lock()
	data.CfgText = text
	if strings.HasPrefix(source, "http") {
		data.CfgURL = source
		data.CfgCustom = "1"
	}
	mu.Unlock()
	flush()
}

func SetSubName(name string) {
	if name == "" {
		return
	}
	mu.Lock()
	data.SubName = name
	mu.Unlock()
	flush()
}

// ResetCrawlAt 站源列表变了，重置抓取时钟。
func ResetCrawlAt() {
	mu.Lock()
	data.CrawlAt = 0
	mu.Unlock()
	flush()
}

func BuildSites(cfg model.RawConfig) {
	mu.Lock()
	defer mu.Unlock()
	var out []model.Site
	for i, r := range cfg.Sites {
		s := model.Site{
			Key: r.Key, Name: r.Name, Type: r.Type, API: r.API,
			Searchable:  r.Searchable == 1,
			QuickSearch: r.QuickSearch == 1,
		}
		if s.Key == "" {
			s.Key = "site_" + itoa(i)
		}
		if s.Name == "" {
			s.Name = "站源 " + itoa(i+1)
		}
		if s.Type == 0 {
			s.Type = 1
		}
		switch e := r.Ext.(type) {
		case string:
			s.Ext = e
		case nil:
			s.Ext = ""
		default:
			b, _ := json.Marshal(e)
			s.Ext = string(b)
		}
		switch {
		case s.Type == 3:
			s.Usable = false
			s.Reason = "需爬虫运行时（Java/JS），Web 无法执行"
		case s.Type == 0 || s.Type == 1 || s.Type == 4:
			if hasPrefix(s.API) {
				s.Usable = true
			} else {
				s.Reason = "接口地址不是 HTTP"
			}
		default:
			s.Reason = "暂不支持的类型 type=" + itoa(s.Type)
		}
		out = append(out, s)
	}
	rawSites = len(out)
	siteList = out
	sitesVersion = time.Now().UnixNano() // 更新版本号
}

func BuildParses(cfg model.RawConfig) {
	mu.Lock()
	defer mu.Unlock()
	var out []model.ParseRule
	for i, p := range cfg.Parses {
		if p.Type == 1 && hasPrefix(p.URL) {
			name := p.Name
			if name == "" {
				name = "解析" + itoa(i)
			}
			out = append(out, model.ParseRule{Name: name, URL: p.URL})
		}
	}
	parseList = out
}

func Sites() []model.Site { return siteList }
func TotalSites() int     { return rawSites }
func Parses() []model.ParseRule { return parseList }
func SitesVersion() int64  { mu.Lock(); defer mu.Unlock(); return sitesVersion }
func ConfigVersion() int64 { mu.Lock(); defer mu.Unlock(); return configVersion }

func Site(key string) *model.Site {
	for i := range siteList {
		if siteList[i].Key == key {
			return &siteList[i]
		}
	}
	return nil
}

func UsableSites() []model.Site {
	var out []model.Site
	for _, s := range siteList {
		if s.Usable {
			out = append(out, s)
		}
	}
	return out
}

// ActiveSites 默认只启用协议型站点。
func ActiveSites() []model.Site {
	keys := data.Enabled
	if len(keys) == 0 {
		return UsableSites()
	}
	set := map[string]bool{}
	for _, k := range keys {
		set[k] = true
	}
	var out []model.Site
	for _, s := range siteList {
		if s.Usable && set[s.Key] {
			out = append(out, s)
		}
	}
	return out
}

func EnabledKeys() []string { return data.Enabled }
func SetEnabledKeys(keys []string) {
	mu.Lock()
	data.Enabled = keys
	mu.Unlock()
	flush()
}

// ---------- 历史 ----------

func History() []HistItem { return data.Hist }

func PushHistory(h HistItem) {
	mu.Lock()
	var out []HistItem
	out = append(out, h)
	key := h.SiteKey + "|" + h.VodID
	for _, it := range data.Hist {
		if it.SiteKey+"|"+it.VodID != key {
			out = append(out, it)
		}
		if len(out) >= 40 {
			break
		}
	}
	data.Hist = out
	mu.Unlock()
	flush()
}

func UpdateHistoryPos(h HistItem) {
	mu.Lock()
	found := false
	for i := range data.Hist {
		it := &data.Hist[i]
		if it.SiteKey == h.SiteKey && it.VodID == h.VodID {
			it.Pos, it.Dur, it.EpName, it.EpIndex, it.From, it.Time = h.Pos, h.Dur, h.EpName, h.EpIndex, h.From, time.Now().UnixMilli()
			// 补更新影片名/封面（早期记录可能为空）
			if h.Name != "" {
				it.Name = h.Name
			}
			if h.Pic != "" {
				it.Pic = h.Pic
			}
			if h.SiteName != "" {
				it.SiteName = h.SiteName
			}
			found = true
			break // 同站源同影片只有一条记录，找到后直接退出
		}
	}
	if !found {
		// 控制历史上限 40 条
		if len(data.Hist) >= 40 {
			data.Hist = data.Hist[:39]
		}
		data.Hist = append([]HistItem{h}, data.Hist...)
	}
	mu.Unlock()
	flush()
}

func RemoveHistory(h HistItem) {
	mu.Lock()
	var out []HistItem
	for _, it := range data.Hist {
		if !(it.SiteKey == h.SiteKey && it.VodID == h.VodID) {
			out = append(out, it)
		}
	}
	data.Hist = out
	mu.Unlock()
	flush()
}

func ClearHistory() { mu.Lock(); data.Hist = nil; mu.Unlock(); flush() }

// ---------- 收藏 ----------

func Favorites() []FavItem { return data.Fav }

func IsFavorite(siteKey, vodID string) bool {
	for _, f := range data.Fav {
		if f.SiteKey == siteKey && f.VodID == vodID {
			return true
		}
	}
	return false
}

func ToggleFavorite(f FavItem) bool {
	mu.Lock()
	removed := false
	var out []FavItem
	for _, it := range data.Fav {
		if it.SiteKey == f.SiteKey && it.VodID == f.VodID {
			removed = true
		} else {
			out = append(out, it)
		}
	}
	if !removed {
		out = append([]FavItem{f}, out...)
	}
	data.Fav = out
	mu.Unlock()
	flush()
	return !removed
}

// ---------- 收藏分组 ----------

// FavGroupList 获取所有分组。
func FavGroupList() []FavGroup {
	mu.Lock()
	defer mu.Unlock()
	if data.FavGroups == nil {
		return []FavGroup{}
	}
	out := make([]FavGroup, len(data.FavGroups))
	copy(out, data.FavGroups)
	return out
}

// CreateFavGroup 创建新分组，返回新建的分组。
func CreateFavGroup(name string) FavGroup {
	g := FavGroup{
		ID:   genID(),
		Name: name,
		Time: time.Now().Unix(),
	}
	mu.Lock()
	if data.FavGroups == nil {
		data.FavGroups = []FavGroup{}
	}
	data.FavGroups = append(data.FavGroups, g)
	mu.Unlock()
	flush()
	return g
}

// UpdateFavGroup 修改分组名称。
func UpdateFavGroup(id, name string) bool {
	mu.Lock()
	found := false
	for i := range data.FavGroups {
		if data.FavGroups[i].ID == id {
			data.FavGroups[i].Name = name
			found = true
			break
		}
	}
	mu.Unlock()
	if found {
		flush()
	}
	return found
}

// DeleteFavGroup 删除分组，该分组下的收藏自动归入默认分组（groupId=""）。
func DeleteFavGroup(id string) bool {
	mu.Lock()
	var groups []FavGroup
	found := false
	for _, g := range data.FavGroups {
		if g.ID == id {
			found = true
		} else {
			groups = append(groups, g)
		}
	}
	if found {
		data.FavGroups = groups
		// 收藏归入默认组
		for i := range data.Fav {
			if data.Fav[i].GroupID == id {
				data.Fav[i].GroupID = ""
			}
		}
	}
	mu.Unlock()
	if found {
		flush()
	}
	return found
}

// MoveFav 将收藏移入指定分组（groupId="" 为默认分组）。
func MoveFav(siteKey, vodID, groupID string) bool {
	mu.Lock()
	found := false
	for i := range data.Fav {
		if data.Fav[i].SiteKey == siteKey && data.Fav[i].VodID == vodID {
			data.Fav[i].GroupID = groupID
			found = true
			break
		}
	}
	mu.Unlock()
	if found {
		flush()
	}
	return found
}

// genID 生成简单 UUID（时间戳 + 随机后缀）。
func genID() string {
	// 使用 时间戳纳秒 作为简单唯一 ID（不需要真正的 UUID）
	ns := time.Now().UnixNano()
	return fmt.Sprintf("%x", ns)
}

// ---------- 搜索历史 ----------

func Keywords() []string {
	var out []string
	for _, p := range splitLines(data.KwHist) {
		p = trim(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func PushKeyword(kw string) []string {
	k := trim(kw)
	if k == "" {
		return Keywords()
	}
	old := Keywords()
	out := []string{k}
	for _, o := range old {
		if o != k && len(out) < 8 {
			out = append(out, o)
		}
	}
	mu.Lock()
	data.KwHist = joinLines(out)
	mu.Unlock()
	flush()
	return out
}

func ClearKeywords() { mu.Lock(); data.KwHist = ""; mu.Unlock(); flush() }

// ---------- 直播源 ----------

func LiveURL() string       { return data.LiveURL }
func SetLiveURL(u string)   { mu.Lock(); data.LiveURL = u; mu.Unlock(); flush() }
func SubLives() []model.RawLive { return data.SubLives }
func SetSubLives(list []model.RawLive) { mu.Lock(); data.SubLives = list; mu.Unlock(); flush() }

// ---------- 画质 / 跳过 / 反馈 ----------

func QPref() int            { return data.QPref }
func SetQPref(v int)        { mu.Lock(); data.QPref = v; mu.Unlock(); flush() }
func IntroSkipSec() int     { return data.IntroSkip }
func SetIntroSkipSec(v int) { mu.Lock(); data.IntroSkip = v; mu.Unlock(); flush() }
func OutroSkipSec() int     { return data.OutroSkip }
func SetOutroSkipSec(v int) { mu.Lock(); data.OutroSkip = v; mu.Unlock(); flush() }
func TapSound() int         { if data.TapSound == 0 { return 1 }; return data.TapSound }
func SetTapSound(v int)     { mu.Lock(); data.TapSound = v; mu.Unlock(); flush() }
func TapVibrate() int       { if data.TapVibrate == 0 { return 1 }; return data.TapVibrate }
func SetTapVibrate(v int)   { mu.Lock(); data.TapVibrate = v; mu.Unlock(); flush() }

// ---------- 定时抓取 ----------

func Feed() []model.Vod        { return data.Feed }
func SaveFeed(list []model.Vod) { mu.Lock(); data.Feed = list; mu.Unlock(); flush() }
func LastCrawlAt() int64       { return data.CrawlAt }
func SetLastCrawlAt(t int64)   { mu.Lock(); data.CrawlAt = t; mu.Unlock(); flush() }
func CrawlSummary() string     { return data.CrawlSummary }
func SaveCrawlSummary(s string) { mu.Lock(); data.CrawlSummary = s; mu.Unlock(); flush() }
func DeadKeys() []string       { return data.DeadKeys }
func SaveDeadKeys(keys []string) { mu.Lock(); data.DeadKeys = keys; mu.Unlock(); flush() }

// ---------- 首启同意与引导 ----------

func PrivacyAgreed() bool       { return data.PrivacyAgreed }
func SetPrivacyAgreed()         { mu.Lock(); data.PrivacyAgreed = true; mu.Unlock(); flush() }
func GuideDone() bool           { return data.GuideDone }
func SetGuideDone()             { mu.Lock(); data.GuideDone = true; mu.Unlock(); flush() }

// ---------- 直连播放 ----------

func DirectList() []string      { return data.DirectList }
func PushDirect(url string) {
	mu.Lock()
	out := []string{url}
	for _, o := range data.DirectList {
		if o != url && len(out) < 12 {
			out = append(out, o)
		}
	}
	data.DirectList = out
	mu.Unlock()
	flush()
}

// ---------- WebDAV ----------

func WebdavSources() []model.WebdavSource { return data.WebdavList }

func SaveWebdavSource(src model.WebdavSource) {
	mu.Lock()
	if src.ID == "" {
		src.ID = "wd_" + itoa(int(time.Now().UnixMilli()))
	}
	replaced := false
	for i := range data.WebdavList {
		if data.WebdavList[i].ID == src.ID {
			data.WebdavList[i] = src
			replaced = true
		}
	}
	if !replaced {
		data.WebdavList = append([]model.WebdavSource{src}, data.WebdavList...)
	}
	mu.Unlock()
	flush()
}

func RemoveWebdavSource(id string) {
	mu.Lock()
	var out []model.WebdavSource
	for _, s := range data.WebdavList {
		if s.ID != id {
			out = append(out, s)
		}
	}
	data.WebdavList = out
	delete(data.WebdavScan, id)
	mu.Unlock()
	flush()
}

func WebdavSource(id string) *model.WebdavSource {
	for i := range data.WebdavList {
		if data.WebdavList[i].ID == id {
			return &data.WebdavList[i]
		}
	}
	return nil
}

func WebdavScanCache(srcID string) []model.WebdavVod { return data.WebdavScan[srcID] }
func SaveWebdavScan(srcID string, vods []model.WebdavVod) {
	mu.Lock()
	data.WebdavScan[srcID] = vods
	mu.Unlock()
	flush()
}

func WdHistory() []model.WdHistory {
	list := append([]model.WdHistory(nil), data.WdHist...)
	sort.SliceStable(list, func(i, j int) bool { return list[i].Time > list[j].Time })
	return list
}

func UpdateWdHistory(h model.WdHistory) {
	mu.Lock()
	found := false
	for i := range data.WdHist {
		it := &data.WdHist[i]
		if it.SrcID == h.SrcID && it.VodName == h.VodName {
			*it = h
			found = true
		}
	}
	if !found {
		data.WdHist = append([]model.WdHistory{h}, data.WdHist...)
	}
	mu.Unlock()
	flush()
}

func RemoveWdHistory(h model.WdHistory) {
	mu.Lock()
	var out []model.WdHistory
	for _, it := range data.WdHist {
		if !(it.SrcID == h.SrcID && it.VodName == h.VodName) {
			out = append(out, it)
		}
	}
	data.WdHist = out
	mu.Unlock()
	flush()
}

func ClearWdHistory() { mu.Lock(); data.WdHist = nil; mu.Unlock(); flush() }

// ---------- 小工具 ----------

func hasPrefix(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	cur := ""
	for _, c := range s {
		if c == '\n' {
			out = append(out, cur)
			cur = ""
		} else {
			cur += string(c)
		}
	}
	out = append(out, cur)
	return out
}

func joinLines(list []string) string {
	out := ""
	for i, s := range list {
		if i > 0 {
			out += "\n"
		}
		out += s
	}
	return out
}

func trim(s string) string {
	return stringTrim(s)
}

func stringTrim(s string) string {
	start, end := 0, len(s)
	for start < end && isSpace(s[start]) {
		start++
	}
	for end > start && isSpace(s[end-1]) {
		end--
	}
	return s[start:end]
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// ---------- 站源健康度 ----------

// RecordSiteRequest 记录站源请求（成功/失败 + 响应时间）。
func RecordSiteRequest(siteKey string, success bool, ms int) {
	mu.Lock()
	defer mu.Unlock()
	if data.SiteHealth == nil {
		data.SiteHealth = make(map[string]SiteHealthStat)
	}
	stat := data.SiteHealth[siteKey]
	now := time.Now().Unix()
	stat.TotalReq++
	if success {
		stat.LastSuccess = now
	} else {
		stat.LastFail = now
		stat.FailCount++
	}
	// 移动平均：保留历史 90%，新值占 10%
	if stat.AvgMs == 0 {
		stat.AvgMs = ms
	} else {
		stat.AvgMs = (stat.AvgMs*9 + ms) / 10
	}
	data.SiteHealth[siteKey] = stat
	
	// 计算健康状态并广播 WebSocket
	health := calcHealth(stat)
	errorRate := float64(stat.FailCount) / float64(stat.TotalReq)
	if errorRate > 1.0 {
		errorRate = 1.0
	}
	
	// 异步广播与落盘，避免阻塞
	go func() {
		// 广播 WebSocket 消息
		if hub := getWSHub(); hub != nil {
			// 使用类型断言调用 BroadcastHealth
			if h, ok := hub.(interface{ BroadcastHealth(string, string, float64, int, int64, int64) }); ok {
				h.BroadcastHealth(siteKey, health, errorRate, stat.AvgMs, now*1000, stat.LastSuccess*1000)
			}
		}
		// 落盘
		flush()
	}()
}

// calcHealth 根据统计数据计算健康状态。
func calcHealth(stat SiteHealthStat) string {
	if stat.TotalReq == 0 {
		return "unknown"
	}
	errorRate := float64(stat.FailCount) / float64(stat.TotalReq)
	now := time.Now().Unix()
	daysSinceSuccess := (now - stat.LastSuccess) / 86400
	
	if daysSinceSuccess > 1 || errorRate > 0.8 {
		return "dead"
	}
	if errorRate > 0.5 || stat.AvgMs > 5000 {
		return "slow"
	}
	return "healthy"
}

// getWSHub 获取全局 WebSocket Hub（延迟导入，避免循环依赖）。
func getWSHub() interface{} {
	// 使用 api 包的 exported 函数获取 Hub
	// 为了避免循环依赖，我们在 api 包中导出一个访问器
	return getGlobalWSHub()
}

// SetGlobalWSHub 由 api 包调用，设置全局 Hub。
var getGlobalWSHub = func() interface{} { return nil }

// InitWSHub 由 main 或 api 包调用，初始化 Hub 访问器。
func InitWSHub(getter func() interface{}) {
	getGlobalWSHub = getter
}

// SiteHealthMap 返回所有站源健康度数据。
func SiteHealthMap() map[string]SiteHealthStat {
	mu.Lock()
	defer mu.Unlock()
	if data.SiteHealth == nil {
		return map[string]SiteHealthStat{}
	}
	// 复制一份避免并发修改
	out := make(map[string]SiteHealthStat, len(data.SiteHealth))
	for k, v := range data.SiteHealth {
		out[k] = v
	}
	return out
}

// SiteHealth 计算站源健康状态（healthy/slow/dead）。
type SiteHealthStatus struct {
	Key         string  `json:"key"`
	Status      string  `json:"status"`      // healthy / slow / dead
	LastSuccess int64   `json:"lastSuccess"` // 最后成功时间
	ErrorRate   float64 `json:"errorRate"`   // 错误率（0-1）
	AvgMs       int     `json:"avgMs"`       // 平均响应时间
}

func SiteHealthStatuses() []SiteHealthStatus {
	hm := SiteHealthMap()
	now := time.Now().Unix()
	var out []SiteHealthStatus
	for _, s := range siteList {
		stat, ok := hm[s.Key]
		if !ok {
			// 从未请求过，默认 unknown
			out = append(out, SiteHealthStatus{Key: s.Key, Status: "unknown", AvgMs: -1})
			continue
		}
		var errRate float64
		if stat.TotalReq > 0 {
			errRate = float64(stat.FailCount) / float64(stat.TotalReq)
		}
		status := "healthy"
		if stat.LastSuccess == 0 || now-stat.LastSuccess > 3600*24 {
			// 超过 24 小时没有成功过
			status = "dead"
		} else if errRate > 0.5 || stat.AvgMs > 5000 {
			// 错误率超过 50% 或平均响应超过 5s
			status = "slow"
		}
		out = append(out, SiteHealthStatus{
			Key:         s.Key,
			Status:      status,
			LastSuccess: stat.LastSuccess,
			ErrorRate:   errRate,
			AvgMs:       stat.AvgMs,
		})
	}
	return out
}
