// Package cms 实现苹果CMS（采集源）协议客户端，与鸿蒙影视 Cms.ets 一致。
// 只实现 TVBox 的「协议型」站源：type 0（XML）、type 1（JSON）、type 4（data 包一层）。
package cms

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"tvbox/internal/httpc"
	"tvbox/internal/model"
)

// ---------- 响应缓存 ----------

type cmHit struct {
	key  string
	body string
	at   time.Time
}

const (
	hitMax    = 30
	hitChars  = 3000000
	ttlCat    = 300 * time.Second
	ttlList   = 90 * time.Second
	ttlDetail = 600 * time.Second
)

var (
	cacheMu sync.Mutex
	hits    []cmHit
)

func hit(url string, ttl time.Duration) string {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	for i := range hits {
		if hits[i].key == url {
			if time.Since(hits[i].at) <= ttl {
				return hits[i].body
			}
			hits = append(hits[:i], hits[i+1:]...)
			return ""
		}
	}
	return ""
}

func keep(url, body string) {
	if body == "" {
		return
	}
	cacheMu.Lock()
	defer cacheMu.Unlock()
	e := cmHit{key: url, body: body, at: time.Now()}
	for i := range hits {
		if hits[i].key == url {
			hits[i] = e
			return
		}
	}
	hits = append(hits, e)
	for len(hits) > hitMax {
		hits = hits[1:]
	}
	total := 0
	for _, h := range hits {
		total += len(h.body)
	}
	for total > hitChars && len(hits) > 1 {
		total -= len(hits[0].body)
		hits = hits[1:]
	}
}

// ClearCache 用户换了配置/站源后主动清空。
func ClearCache() {
	cacheMu.Lock()
	hits = nil
	cacheMu.Unlock()
}

// ---------- CMS 客户端 ----------

type Cms struct {
	site  model.Site
	extra string // 筛选条件尾巴，如 &area=大陆&year=2025
}

func New(site model.Site) *Cms { return &Cms{site: site} }

func (c *Cms) SetExtra(q string) { c.extra = q }

func fetch(url string, ttl time.Duration, ms time.Duration) (string, error) {
	if h := hit(url, ttl); h != "" {
		return h, nil
	}
	txt, err := httpc.Text(url, nil, ms)
	if err != nil {
		return "", err
	}
	keep(url, txt)
	return txt, nil
}

func (c *Cms) u(params string) string {
	a := c.site.API
	if strings.HasSuffix(a, "?") {
		return a + params
	}
	if strings.Contains(a, "?") {
		return a + "&" + params
	}
	return a + "?" + params
}

// APIURL 导出：拼接站源 API 与参数（供测速等外部使用）。
func (c *Cms) APIURL(params string) string { return c.u(params) }

// ---------- 公共入口 ----------

func (c *Cms) Categories() ([]model.Category, error) {
	txt, err := fetch(c.u("ac=list"), ttlCat, 15*time.Second)
	if err != nil {
		return nil, err
	}
	if c.site.Type == 0 || looksXML(txt) {
		cats := catFromXML(txt)
		if len(cats) > 0 {
			return prependAll(cats), nil
		}
		derived := catsFromVideos(txt)
		if len(derived) > 0 {
			return prependAll(derived), nil
		}
		return derived, nil
	}
	root, err := parse(txt)
	if err != nil {
		return nil, err
	}
	root = Root(root)
	var out []model.Category
	for _, cl := range root.CatList {
		cat := model.Category{
			ID:   strconv.Itoa(int(cl.TypeID)),
			Name: s(cl.TypeName),
		}
		if cl.TypePid > 0 {
			cat.PID = strconv.Itoa(int(cl.TypePid))
		}
		if cat.ID != "" && cat.Name != "" {
			out = append(out, cat)
		}
	}
	if len(out) == 0 && len(root.List) > 0 {
		seen := map[string]bool{}
		for _, it := range root.List {
			tid := strconv.Itoa(int(it.Tid))
			tn := s(it.TypeName)
			if tid != "" && tn != "" && !seen[tid] {
				seen[tid] = true
				out = append(out, model.Category{ID: tid, Name: tn})
			}
		}
	}
	if len(out) > 0 {
		out = prependAll(out)
	}
	return out, nil
}

func prependAll(cats []model.Category) []model.Category {
	return append([]model.Category{{ID: "", Name: "全部"}}, cats...)
}

func (c *Cms) List(tid string, page int) (model.VodPage, error) {
	q := "ac=videolist&t=" + urlEncode(tid) + "&pg=" + strconv.Itoa(page) + c.extra
	txt, err := fetch(c.u(q), ttlList, 18*time.Second)
	if err != nil {
		return model.VodPage{}, err
	}
	if c.site.Type == 0 || looksXML(txt) {
		return listFromXML(txt, page, c.site), nil
	}
	root, err := parse(txt)
	if err != nil {
		return model.VodPage{}, err
	}
	root = Root(root)
	pg := model.VodPage{Page: page, PageCount: 1}
	if root.Page > 0 {
		pg.Page = int(root.Page)
	}
	if root.PageCount > 0 {
		pg.PageCount = int(root.PageCount)
	}
	if root.Total > 0 {
		pg.Total = int(root.Total)
	}
	for _, it := range root.List {
		pg.Items = append(pg.Items, c.toVod(it))
	}
	return pg, nil
}

// ListMerged 把一个父分类和它的子分类合在一起取第一页（按 vod id 去重）。
func (c *Cms) ListMerged(ids []string, page int) (model.VodPage, error) {
	out := model.VodPage{Page: 1, PageCount: 1}
	seen := map[string]bool{}
	type res struct {
		pg  model.VodPage
		err error
	}
	ch := make(chan res, len(ids))
	for _, id := range ids {
		go func(id string) {
			pg, err := c.List(id, page)
			ch <- res{pg, err}
		}(id)
	}
	for range ids {
		r := <-ch
		if r.err != nil {
			continue
		}
		for _, it := range r.pg.Items {
			if !seen[it.ID] {
				seen[it.ID] = true
				out.Items = append(out.Items, it)
			}
		}
	}
	out.Total = len(out.Items)
	return out, nil
}

func (c *Cms) Detail(id string) (model.Vod, error) {
	q := "ac=detail&ids=" + urlEncode(id)
	txt, err := fetch(c.u(q), ttlDetail, 18*time.Second)
	if err != nil {
		return model.Vod{}, err
	}
	if c.site.Type == 0 || looksXML(txt) {
		return detailFromXML(txt, c.site, id)
	}
	root, err := parse(txt)
	if err != nil {
		return model.Vod{}, err
	}
	root = Root(root)
	if len(root.List) > 0 {
		return c.toVod(root.List[0]), nil
	}
	return model.Vod{}, fmt.Errorf("接口没有返回详情数据")
}

// Search 搜索；不同站对 ac / wd 的接受度不同，做两级兜底。
func (c *Cms) Search(kw string) []model.Vod {
	e := urlEncode(kw)
	var items []model.Vod
	if txt, err := httpc.Text(c.u("ac=videolist&wd="+e), nil, 12*time.Second); err == nil {
		items = c.pickList(txt)
	}
	if len(items) == 0 {
		if txt2, err := httpc.Text(c.u("wd="+e), nil, 12*time.Second); err == nil {
			items = c.pickList(txt2)
		}
	}
	return items
}

func (c *Cms) pickList(txt string) []model.Vod {
	if c.site.Type == 0 || looksXML(txt) {
		return listFromXML(txt, 1, c.site).Items
	}
	root, err := parse(txt)
	if err != nil {
		return nil
	}
	root = Root(root)
	var out []model.Vod
	for _, it := range root.List {
		out = append(out, c.toVod(it))
	}
	return out
}

func (c *Cms) toVod(j model.JVod) model.Vod {
	v := model.Vod{
		ID:       strconv.Itoa(int(j.VodID)),
		Name:     s(j.VodName),
		Pic:      s(j.VodPic),
		Remark:   s(j.VodRemarks),
		Year:     s(j.VodYear),
		Area:     s(j.VodArea),
		Actor:    s(j.VodActor),
		Director: s(j.VodDirector),
		TypeName: s(j.TypeName),
		From:     s(j.VodPlayFrom),
		PlayURL:  s(j.VodPlayURL),
		SiteKey:  c.site.Key,
		SiteName: c.site.Name,
	}
	content := s(j.VodContent)
	if content == "" {
		content = s(j.VodBlurb)
	}
	v.Content = plain(content)
	return v
}

// ---------- 静态工具 ----------

func s(v string) string { return strings.TrimSpace(v) }

func looksXML(txt string) bool {
	return strings.HasPrefix(strings.TrimLeft(txt, " \t\r\n"), "<")
}

// CleanJSON 配置 JSON 容错：去 BOM、去 // 注释行、把保留字 class 改名。
func CleanJSON(txt string) string {
	t := txt
	if strings.HasPrefix(t, "\uFEFF") {
		t = t[3:] // 去 UTF-8 BOM
	}
	lines := strings.Split(t, "\n")
	var keep []string
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "//") {
			continue
		}
		keep = append(keep, line)
	}
	return regexp.MustCompile(`"class"\s*:`).ReplaceAllString(strings.Join(keep, "\n"), `"catList":`)
}

func parse(txt string) (model.JResp, error) {
	t := CleanJSON(txt)
	if strings.TrimSpace(t) == "" {
		return model.JResp{}, fmt.Errorf("接口返回空内容")
	}
	var j model.JResp
	if err := json.Unmarshal([]byte(t), &j); err != nil {
		return model.JResp{}, fmt.Errorf("接口返回的不是合法 JSON（站源可能已失效）")
	}
	return j, nil
}

// Root type 4 站点返回体多包一层 data。
func Root(j model.JResp) model.JResp {
	if j.Data != nil {
		return *j.Data
	}
	return j
}

var (
	reBr      = regexp.MustCompile(`(?i)<br\s*/?>`)
	reP       = regexp.MustCompile(`(?i)</p>`)
	reTag     = regexp.MustCompile(`<[^>]+>`)
	reEntity  = regexp.MustCompile(`&(lt|gt|quot|nbsp|amp|#39);`)
	reNewline = regexp.MustCompile(`\n{3,}`)
)

// plain HTML 实体 + 标签清理，用于简介。
func plain(s0 string) string {
	r := reBr.ReplaceAllString(s0, "\n")
	r = reP.ReplaceAllString(r, "\n")
	r = reTag.ReplaceAllString(r, "")
	r = reEntity.ReplaceAllStringFunc(r, func(m string) string {
		switch m {
		case "&lt;":
			return "<"
		case "&gt;":
			return ">"
		case "&quot;":
			return `"`
		case "&#39;":
			return "'"
		case "&nbsp;":
			return " "
		case "&amp;":
			return "&"
		}
		return m
	})
	r = reNewline.ReplaceAllString(r, "\n\n")
	return strings.TrimSpace(r)
}

// node XML 节点取值：先取 CDATA 内容，再清标签与实体。
func node(block, tag string) string {
	re := regexp.MustCompile(`(?is)<` + tag + `>(.*?)</` + tag + `>`)
	m := re.FindStringSubmatch(block)
	if m == nil {
		return ""
	}
	return text(m[1])
}

func text(s0 string) string {
	r := s0
	if i := strings.Index(r, "<![CDATA["); i >= 0 {
		if e := strings.Index(r[i:], "]]>"); e > 0 {
			r = r[i+9 : i+e]
		}
	}
	return plain(r)
}

// catsFromVideos 从每条影片的 tid/type 反推分类。
func catsFromVideos(xml string) []model.Category {
	var out []model.Category
	seen := map[string]bool{}
	re := regexp.MustCompile(`(?is)<video>.*?</video>`)
	for _, b := range re.FindAllString(xml, -1) {
		tid := node(b, "tid")
		tn := node(b, "type")
		if tid != "" && tn != "" && !seen[tid] {
			seen[tid] = true
			out = append(out, model.Category{ID: tid, Name: tn})
		}
	}
	return out
}

func catFromXML(xml string) []model.Category {
	var out []model.Category
	re := regexp.MustCompile(`(?is)<ty\b[^>]*\bid="(\d+)"[^>]*>(.*?)</ty>`)
	for _, m := range re.FindAllStringSubmatch(xml, -1) {
		name := plain(m[2])
		if name != "" {
			out = append(out, model.Category{ID: m[1], Name: name})
		}
	}
	return out
}

func listFromXML(xml string, page int, site model.Site) model.VodPage {
	pg := model.VodPage{Page: page}
	if head := regexp.MustCompile(`(?i)<list\b[^>]*>`).FindString(xml); head != "" {
		if p := regexp.MustCompile(`(?i)\bpage="(\d+)"`).FindStringSubmatch(head); p != nil {
			pg.Page, _ = strconv.Atoi(p[1])
		}
		if pc := regexp.MustCompile(`(?i)\bpagecount="(\d+)"`).FindStringSubmatch(head); pc != nil {
			pg.PageCount, _ = strconv.Atoi(pc[1])
		}
		if rc := regexp.MustCompile(`(?i)\brecordcount="(\d+)"`).FindStringSubmatch(head); rc != nil {
			pg.Total, _ = strconv.Atoi(rc[1])
		}
	}
	re := regexp.MustCompile(`(?is)<video>.*?</video>`)
	for _, b := range re.FindAllString(xml, -1) {
		v := model.Vod{
			ID:       node(b, "id"),
			Name:     node(b, "name"),
			Pic:      node(b, "pic"),
			TypeName: node(b, "type"),
			SiteKey:  site.Key,
			SiteName: site.Name,
		}
		if note := node(b, "note"); note != "" {
			v.Remark = note
		} else {
			v.Remark = node(b, "dt")
		}
		if v.Name != "" {
			pg.Items = append(pg.Items, v)
		}
	}
	return pg
}

func detailFromXML(xml string, site model.Site, id string) (model.Vod, error) {
	re := regexp.MustCompile(`(?is)<video>.*?</video>`)
	blocks := re.FindAllString(xml, -1)
	if len(blocks) == 0 {
		return model.Vod{}, fmt.Errorf("接口没有返回详情数据")
	}
	b := blocks[0]
	v := model.Vod{
		ID:       node(b, "id"),
		Name:     node(b, "name"),
		Pic:      node(b, "pic"),
		Remark:   node(b, "note"),
		TypeName: node(b, "type"),
		Year:     node(b, "year"),
		Area:     node(b, "area"),
		Actor:    node(b, "actor"),
		Director: node(b, "director"),
		Content:  node(b, "des"),
		SiteKey:  site.Key,
		SiteName: site.Name,
	}
	if v.ID == "" {
		v.ID = id
	}
	var flags, urls []string
	ddRe := regexp.MustCompile(`(?is)<dd\b[^>]*\bflag="([^"]*)"[^>]*>(.*?)</dd>`)
	for _, m := range ddRe.FindAllStringSubmatch(b, -1) {
		flags = append(flags, m[1])
		urls = append(urls, text(m[2]))
	}
	if len(flags) == 0 {
		ddRe2 := regexp.MustCompile(`(?is)<dd\b[^>]*>(.*?)</dd>`)
		n := 1
		for _, m := range ddRe2.FindAllStringSubmatch(b, -1) {
			flags = append(flags, "线路"+strconv.Itoa(n))
			urls = append(urls, text(m[1]))
			n++
		}
	}
	v.From = strings.Join(flags, "$$$")
	v.PlayURL = strings.Join(urls, "$$$")
	return v, nil
}

// SplitLines vod_play_url / vod_play_from → 线路与剧集。
func SplitLines(from, playURL string) []model.Line {
	var out []model.Line
	if playURL == "" {
		return out
	}
	var fArr []string
	if from != "" {
		fArr = strings.Split(from, "$$$")
	}
	uArr := strings.Split(playURL, "$$$")
	for i, u := range uArr {
		ln := model.Line{Name: fmt.Sprintf("线路 %d", i+1)}
		if i < len(fArr) && fArr[i] != "" {
			ln.Name = fArr[i]
		}
		for k, seg := range strings.Split(u, "#") {
			seg = strings.TrimSpace(seg)
			if seg == "" {
				continue
			}
			ep := model.Episode{Name: fmt.Sprintf("第 %d 集", k+1), URL: seg}
			if p := strings.Index(seg, "$"); p > 0 {
				ep.Name = strings.TrimSpace(seg[:p])
				ep.URL = strings.TrimSpace(seg[p+1:])
			}
			if ep.URL != "" && strings.Contains(ep.URL, "://") {
				ln.Eps = append(ln.Eps, ep)
			}
		}
		directCount := 0
		for _, e := range ln.Eps {
			if IsDirect(e.URL) {
				directCount++
			}
		}
		ln.Direct = len(ln.Eps) > 0 && directCount*2 >= len(ln.Eps)
		if len(ln.Eps) > 0 {
			out = append(out, ln)
		}
	}
	return out
}

func IsDirect(u string) bool {
	lu := strings.ToLower(u)
	for _, ext := range []string{".m3u8", ".mp4", ".flv", ".mkv", ".ts", ".m4v"} {
		if strings.Contains(lu, ext) {
			return true
		}
	}
	return false
}

// ---------- 筛选（ext 里的 filters） ----------

type jExt struct {
	Filters map[string][]struct {
		Key   string `json:"key"`
		Name  string `json:"name"`
		Value []struct {
			N string `json:"n"`
			V string `json:"v"`
		} `json:"value"`
	} `json:"filters"`
}

// FiltersOf 从站点 ext 里取出该分类的筛选组。
func FiltersOf(ext, tid string) []model.FilterGroup {
	var out []model.FilterGroup
	txt := strings.TrimSpace(ext)
	if txt == "" || txt[0] != '{' {
		return out
	}
	var j jExt
	if err := json.Unmarshal([]byte(txt), &j); err != nil {
		return out
	}
	groups, ok := j.Filters[tid]
	if !ok {
		return out
	}
	for _, g := range groups {
		gg := model.FilterGroup{Key: g.Key, Name: g.Name}
		if gg.Name == "" {
			gg.Name = g.Key
		}
		for _, v := range g.Value {
			if v.N != "" {
				gg.Values = append(gg.Values, model.FilterValue{N: v.N, V: v.V})
			}
		}
		if gg.Key != "" && len(gg.Values) > 0 {
			out = append(out, gg)
		}
	}
	return out
}

// SortCategories 按 type_pid 层级稳定排序（父类在前）。
func SortCategories(cats []model.Category) []model.Category {
	sort.SliceStable(cats, func(i, j int) bool {
		return cats[i].PID < cats[j].PID
	})
	return cats
}

// urlEncode 用 Go 标准库转义（与 JS encodeURIComponent 语义接近）。
func urlEncode(s0 string) string {
	return url.QueryEscape(s0)
}
