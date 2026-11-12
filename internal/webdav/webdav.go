// Package webdav 实现 WebDAV 客户端 + 影视资源识别，与鸿蒙影视 Webdav.ets 一致。
package webdav

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/xxl6097/TVBoxWeb/internal/httpc"
	"github.com/xxl6097/TVBoxWeb/internal/model"
)

var videoExt = map[string]bool{
	"mp4": true, "mkv": true, "avi": true, "ts": true, "m2ts": true, "mts": true,
	"mov": true, "wmv": true, "flv": true, "rmvb": true, "rm": true, "m4v": true,
	"webm": true, "3gp": true, "mpg": true, "mpeg": true, "vob": true, "m2t": true, "f4v": true,
}

type parsed struct {
	title    string
	season   int
	episode  int
	year     string
	isSeries bool
}

// AuthHeader Basic Auth header；用户名空则不给。
func AuthHeader(src model.WebdavSource) map[string]string {
	if src.User == "" {
		return nil
	}
	h := map[string]string{}
	h["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(src.User+":"+src.Pass))
	return h
}

// OriginOf 提取 origin：scheme://host:port。
func OriginOf(u string) string {
	i := strings.Index(u, "://")
	if i < 0 {
		return u
	}
	rest := u[i+3:]
	slash := strings.Index(rest, "/")
	if slash < 0 {
		return u
	}
	return u[:i+3+slash]
}

// URLPath url 的路径部分。
func URLPath(u string) string {
	i := strings.Index(u, "://")
	if i < 0 {
		return "/"
	}
	rest := u[i+3:]
	slash := strings.Index(rest, "/")
	if slash < 0 {
		return "/"
	}
	return rest[slash:]
}

func stripSlash(p string) string {
	if p == "/" || p == "" {
		return "/"
	}
	return strings.TrimSuffix(p, "/")
}

// safePath 已含 % 则保留，否则编码。
func safePath(p string) string {
	if strings.Contains(p, "%") {
		return p
	}
	return httpc.EncodePath(p)
}

// MountHref 挂载根对应的服务器绝对路径。
func MountHref(src model.WebdavSource) string {
	base := stripSlash(safePath(URLPath(src.URL)))
	root := strings.TrimSpace(src.Root)
	if root == "" || root == "/" {
		return base
	}
	r := root
	if !strings.HasPrefix(r, "/") {
		r = "/" + r
	}
	rootEnc := safePath(r)
	baseSlash := base
	if base != "/" {
		baseSlash = base + "/"
	}
	if rootEnc == base || strings.HasPrefix(rootEnc, baseSlash) {
		return stripSlash(rootEnc)
	}
	if base == "/" {
		return stripSlash(rootEnc)
	}
	return base + rootEnc
}

// AbsURL href → 完整 URL。
func AbsURL(src model.WebdavSource, href string) string {
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	origin := OriginOf(src.URL)
	if strings.HasPrefix(href, "/") {
		return origin + href
	}
	return origin + "/" + href
}

// List PROPFIND 列目录，返回直接子项。
func List(src model.WebdavSource, path string) ([]model.WebdavEntry, error) {
	return ListCtx(context.Background(), src, path)
}

// ListCtx 与 List 相同，ctx 可取消（扫描中途取消会及时中止后续 PROPFIND）。
func ListCtx(ctx context.Context, src model.WebdavSource, path string) ([]model.WebdavEntry, error) {
	origin := OriginOf(src.URL)
	u := origin + path
	xmlStr, err := httpc.PropfindCtx(ctx, u, AuthHeader(src), 15*time.Second)
	if err != nil {
		return nil, err
	}
	all := xmlList(xmlStr)
	self := stripSlash(path)
	var out []model.WebdavEntry
	for _, e := range all {
		if stripSlash(e.Href) == self {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// Probe 测试连接，返回条目数。
func Probe(src model.WebdavSource) (int, error) {
	return ProbeCtx(context.Background(), src)
}

// ProbeCtx 与 Probe 相同，ctx 可取消。
func ProbeCtx(ctx context.Context, src model.WebdavSource) (int, error) {
	root := MountHref(src)
	list, err := ListCtx(ctx, src, root)
	if err != nil {
		return 0, err
	}
	return len(list), nil
}

// ---------- 影视识别 ----------

func isVideo(name string) bool {
	dot := strings.LastIndex(name, ".")
	if dot < 0 {
		return true
	}
	ext := strings.ToLower(name[dot+1:])
	return videoExt[ext]
}

func dirName(href string) string {
	h := stripSlash(href)
	i := strings.LastIndex(h, "/")
	name := h
	if i >= 0 {
		name = h[i+1:]
	}
	if d, err := url.PathUnescape(name); err == nil {
		name = d
	}
	return name
}

// reDirPrefix 匹配目录名前缀序号，如「01 风起天南」「10-外海风云」。
// 用 \b 边界 + 长度 ≤3 限制：避免把「2024」这种四位年份当成序号。
var reDirPrefix = regexp.MustCompile(`^(\d{1,3})(?:\s*[-_. ]\s*|\s+)`)

// reSeasonName 匹配「带季特征」的目录名，用于识别「某长篇/第一季、第二季」这种结构：
// 这类目录下集号通常每季重新从 1 开始，无法靠集号连续性判断，只能看名字。
//
// 覆盖：第一季 / 第2季 / 第二部 / Season 1 / season02 / S01 / S2
// 不覆盖：普通剧名（如「风起天南」），避免把平铺的多部剧误合并。
var reSeasonName = regexp.MustCompile(`(?i)(第\s*[0-9一二三四五六七八九十百]+\s*[季部]|season\s*\d+|s\d{1,2}$)`)

func cleanDirName(name string) string {
	return strings.TrimSpace(reDirPrefix.ReplaceAllString(name, ""))
}

func dirOf(href string) string {
	i := strings.LastIndex(href, "/")
	if i > 0 {
		return href[:i]
	}
	return "/"
}

var (
	reYear  = regexp.MustCompile(`((?:19|20)\d{2})`)
	reSE    = regexp.MustCompile(`[Ss](\d{1,2})\s*[Ee](\d{1,3})`)
	reCN    = regexp.MustCompile(`第\s*(\d{1,3})\s*[集話话]`)
	reEP    = regexp.MustCompile(`[Ee][Pp]?\s*(\d{1,3})\b`)
	reTail  = regexp.MustCompile(`(?:[._\-\s])(\d{1,3})\s*$`)
	rePure  = regexp.MustCompile(`^\s*(\d{1,4})\s*$`)
	reTag   = regexp.MustCompile(`1080[pi]|720[pi]|2160[pi]|480[pi]|576[pi]|4[Kk]|8[Kk]`)
	reCodec = regexp.MustCompile(`x26[45]|h26[45]|hevc|hdr10?|10bit|8bit|aac|dts|dd5\.1|truehd|atmos|bluray|blu-ray|web-?dl|webrip|remux|proper|repack`)
	reSep   = regexp.MustCompile(`[._\[\]()（）\-]+`)
	reSpace = regexp.MustCompile(`\s{2,}`)
)

func parse(name string) parsed {
	p := parsed{}
	base := name
	if dot := strings.LastIndex(base, "."); dot > 0 {
		if ext := strings.ToLower(base[dot+1:]); videoExt[ext] {
			base = base[:dot]
		}
	}
	if m := rePure.FindStringSubmatch(base); m != nil {
		p.episode, _ = strconv.Atoi(m[1])
		p.isSeries = true
		return p
	}
	if m := reYear.FindStringSubmatch(base); m != nil {
		p.year = m[1]
		base = strings.Replace(base, m[1], " ", 1)
	}
	if m := reSE.FindStringSubmatch(base); m != nil {
		p.season, _ = strconv.Atoi(m[1])
		p.episode, _ = strconv.Atoi(m[2])
		p.isSeries = true
		base = strings.Replace(base, m[0], " ", 1)
	} else if m := reCN.FindStringSubmatch(base); m != nil {
		p.episode, _ = strconv.Atoi(m[1])
		p.isSeries = true
		base = strings.Replace(base, m[0], " ", 1)
	} else if m := reEP.FindStringSubmatch(base); m != nil {
		p.episode, _ = strconv.Atoi(m[1])
		p.isSeries = true
		base = strings.Replace(base, m[0], " ", 1)
	} else if m := reTail.FindStringSubmatch(base); m != nil {
		if n, _ := strconv.Atoi(m[1]); n > 0 && n <= 999 {
			p.episode = n
			p.isSeries = true
			base = base[:len(base)-len(m[0])]
		}
	}
	p.title = cleanTitle(base)
	return p
}

func cleanTitle(s string) string {
	t := reTag.ReplaceAllString(s, " ")
	t = reCodec.ReplaceAllString(t, " ")
	t = reSep.ReplaceAllString(t, " ")
	t = reSpace.ReplaceAllString(t, " ")
	return strings.TrimSpace(t)
}

// Scan 递归扫描目录树，识别影视并聚合。
func Scan(src model.WebdavSource, maxDepth int, onProgress func(dirs, files int)) []model.WebdavVod {
	return ScanCtx(context.Background(), src, maxDepth, onProgress)
}

// ScanCtx 与 Scan 相同，ctx 可取消。
func ScanCtx(ctx context.Context, src model.WebdavSource, maxDepth int, onProgress func(dirs, files int)) []model.WebdavVod {
	root := MountHref(src)
	var files []model.WebdavEntry
	const dirBudget = 200
	const batch = 6
	current := []string{root}
	depth := 0
	scanned := 0

	for len(current) > 0 && depth <= maxDepth && scanned < dirBudget {
		var next []string
		for b := 0; b < len(current); b += batch {
			end := b + batch
			if end > len(current) {
				end = len(current)
			}
			slice := current[b:end]
			type r struct {
				entries []model.WebdavEntry
			}
			results := make([]r, len(slice))
			ch := make(chan struct{})
			for i, dir := range slice {
				go func(i int, dir string) {
					defer func() { ch <- struct{}{} }()
					if entries, err := ListCtx(ctx, src, dir); err == nil {
						results[i].entries = entries
					}
				}(i, dir)
			}
			for range slice {
				<-ch
			}
			for _, rr := range results {
				for _, e := range rr.entries {
					if e.IsDir {
						next = append(next, e.Href)
					} else if isVideo(e.Name) {
						files = append(files, e)
					}
				}
			}
		}
		scanned += len(current)
		if onProgress != nil {
			onProgress(scanned, len(files))
		}
		current = next
		depth++
	}

	return aggregate(src, files)
}

// aggregate 把扫描到的视频文件聚合成「电影 / 剧集」。
//
// 抽成独立函数便于单测：归并规则写错时，一部剧会被拆成十几部，
// 或者不同电影被并成一部，这类退化必须能被测试发现。
func aggregate(src model.WebdavSource, files []model.WebdavEntry) []model.WebdavVod {
	var vods []model.WebdavVod
	idx := map[string]int{}

	// ---------- 归并策略 ----------
	//
	// 基本规则：**以「文件夹」为识别单位 —— 一个含视频的文件夹 = 一部影视。**
	//
	// 唯一的例外是「同一部剧按篇章/季度拆成多个子文件夹」：
	//
	//   迅雷下载/凡人/01 风起天南/1.mp4
	//   迅雷下载/凡人/02 燕家堡之战/18.mp4
	//   迅雷下载/凡人/03 魔道争锋/32.mp4
	//
	// 这种应合并成一部《凡人》，篇章目录作为季。而下面这种必须保持独立：
	//
	//   剧集/01 庆余年/1.mp4
	//   剧集/02 狂飙/1.mp4
	//
	// 两者目录名**都带序号前缀**，光看名字无法区分 —— 但集号可以：
	//   · 同一部剧的篇章：集号跨目录**连续**（1..17、18..31、32..56…）
	//   · 不同的剧：各自**从 1 重新开始**
	//
	// 所以判定改为「集号是否连续」，比看目录名可靠得多。
	// 同时保留一个更弱的辅助信号：子目录名去序号后如果都以同一剧名开头
	//（如「凡人 01 风起天南」），也认定为同一部剧。
	seasonSplit := map[string]bool{} // 剧名目录 → 是否属于「分季结构」
	{
		// 父目录 → 各子目录的集号区间
		type span struct {
			min, max int
			title    string // 子目录名去掉序号前缀后的部分
			raw      string // 子目录原始名（含序号）
		}
		spans := map[string]map[string]*span{} // parent → childDir → 区间
		for _, f := range files {
			p := parse(f.Name)
			if !p.isSeries || p.episode <= 0 {
				continue // 无法判断集号的文件不参与统计
			}
			child := dirOf(f.Href)
			parent := dirOf(child)
			if spans[parent] == nil {
				spans[parent] = map[string]*span{}
			}
			sp := spans[parent][child]
			if sp == nil {
				raw := dirName(child)
				sp = &span{min: p.episode, max: p.episode, title: cleanDirName(raw), raw: raw}
				spans[parent][child] = sp
				continue
			}
			if p.episode < sp.min {
				sp.min = p.episode
			}
			if p.episode > sp.max {
				sp.max = p.episode
			}
		}

		for parent, children := range spans {
			if len(children) < 2 {
				continue // 只有一个子目录，谈不上合并
			}
			// 条件一：集号首尾相接（后一段的起点紧接前一段的终点）
			// 条件二：所有子目录名去序号后共享同一前缀（剧名）
			//
			// 注意用切片而不是 map[min]max：两部不同的剧都从第 1 集开始时，
			// map 的键会互相覆盖，导致「看起来连续」的假象（踩过）。
			segs := make([]*span, 0, len(children))
			shared := ""
			sharedOK := true
			for _, sp := range children {
				segs = append(segs, sp)
				if sp.title == "" {
					sharedOK = false
					continue
				}
				if shared == "" {
					shared = sp.title
				} else if shared != sp.title {
					sharedOK = false
				}
			}

			// 集号连续：按起点排序后逐段检查衔接，且必须从第 1 集开始
			sort.Slice(segs, func(a, b int) bool { return segs[a].min < segs[b].min })
			contiguous := segs[0].min == 1
			prevMax := 0
			for _, sp := range segs {
				if sp.min != prevMax+1 {
					contiguous = false
					break
				}
				prevMax = sp.max
			}
			// 起点的重复也要判掉：若两段都从 1 开始，排序后第二段的 min 不等于 prevMax+1
			if contiguous && len(segs) > 1 {
				for i := 1; i < len(segs); i++ {
					if segs[i].min <= segs[i-1].max {
						contiguous = false
						break
					}
				}
			}

			// 条件三：子目录名带「季」特征（第一季 / Season 2 / S02 / 第二季…）
			// 这种结构下集号通常每季重新从 1 开始，只能靠名字判断。
			seasonNamed := 0
			for _, sp := range children {
				if reSeasonName.MatchString(sp.raw) {
					seasonNamed++
				}
			}

			if contiguous || sharedOK || seasonNamed == len(children) {
				seasonSplit[parent] = true
			}
		}
	}

	for i, f := range files {
		p := parse(f.Name)
		dir := dirOf(f.Href)

		title := p.title
		if title == "" && p.isSeries {
			title = cleanDirName(dirName(dir))
		}

		var key string
		if p.isSeries {
			// 默认：按所在文件夹分组（一个文件夹 = 一部）
			key = dir + "|" + title

			// 例外：父目录是「分季结构」时，向上归并成一部
			parent := dirOf(dir)
			if p.isSeries && seasonSplit[parent] {
				upTitle := cleanDirName(dirName(parent))
				if upTitle != "" {
					title = upTitle
				}
				key = parent + "|" + title
			}
		} else {
			key = "#" + strconv.Itoa(i)
		}

		pos, ok := idx[key]
		season := p.season
		if !ok {
			name := title
			if name == "" {
				name = stripExt(f.Name)
			}
			kind := "movie"
			if p.isSeries {
				kind = "series"
			}
			vods = append(vods, model.WebdavVod{Name: name, Kind: kind, Year: p.year, Dir: dir})
			pos = len(vods) - 1
			idx[key] = pos
			if p.isSeries {
				// 「01 风起天南」这类目录序号作为季号，便于在选集里按篇章分组
				if n := dirOrder(dirName(dir)); n > 0 {
					season = n
				}
			}
		} else if vods[pos].Kind == "series" && season == 0 {
			if n := dirOrder(dirName(dir)); n > 0 {
				season = n
			}
		}

		vod := &vods[pos]
		ep := model.WebdavEp{
			Name: f.Name, URL: AbsURL(src, f.Href), Href: f.Href,
			Season: season, Episode: p.episode,
		}
		vod.Eps = append(vod.Eps, ep)
	}

	for i := range vods {
		if vods[i].Kind == "series" {
			sort.SliceStable(vods[i].Eps, func(a, b int) bool {
				if vods[i].Eps[a].Season != vods[i].Eps[b].Season {
					return vods[i].Eps[a].Season < vods[i].Eps[b].Season
				}
				return vods[i].Eps[a].Episode < vods[i].Eps[b].Episode
			})
		}
	}
	return vods
}

func dirOrder(name string) int {
	m := reDirPrefix.FindString(name)
	if m == "" {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimRight(strings.TrimSpace(m), "-_ ")))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func stripExt(name string) string {
	dot := strings.LastIndex(name, ".")
	if dot > 0 {
		return name[:dot]
	}
	return name
}

// HostOf 从 URL 取主机名。
func HostOf(u string) string {
	s := u
	if p := strings.Index(s, "://"); p >= 0 {
		s = s[p+3:]
	}
	if slash := strings.Index(s, "/"); slash >= 0 {
		s = s[:slash]
	}
	return s
}

// ---------- XML 解析 ----------

type multistatus struct {
	Responses []response `xml:"response"`
}

type response struct {
	Href     string     `xml:"href"`
	Propstat []propstat `xml:"propstat"`
}

type propstat struct {
	Prop prop `xml:"prop"`
}

type prop struct {
	ResourceType     resourceType `xml:"resourcetype"`
	GetContentLength string       `xml:"getcontentlength"`
	GetLastModified  string       `xml:"getlastmodified"`
}

type resourceType struct {
	Collection *struct{} `xml:"collection"`
}

func xmlList(xmlStr string) []model.WebdavEntry {
	var out []model.WebdavEntry
	// 去掉命名空间前缀
	cleaned := regexp.MustCompile(`(</?)[A-Za-z]+:`).ReplaceAllString(xmlStr, "$1")
	var ms multistatus
	if err := xml.Unmarshal([]byte(cleaned), &ms); err != nil {
		return out
	}
	for _, r := range ms.Responses {
		href := strings.TrimSpace(r.Href)
		if href == "" {
			continue
		}
		e := model.WebdavEntry{Href: href}
		for _, ps := range r.Propstat {
			e.IsDir = e.IsDir || ps.Prop.ResourceType.Collection != nil
			if ps.Prop.GetContentLength != "" {
				e.Size, _ = strconv.ParseInt(ps.Prop.GetContentLength, 10, 64)
			}
			if ps.Prop.GetLastModified != "" {
				e.Mtime = ps.Prop.GetLastModified
			}
		}
		disp := href
		if d, err := url.PathUnescape(href); err == nil {
			disp = d
		}
		name := strings.TrimSuffix(disp, "/")
		if slash := strings.LastIndex(name, "/"); slash >= 0 {
			name = name[slash+1:]
		}
		if name == "" {
			name = href
		}
		e.Name = name
		out = append(out, e)
	}
	return out
}

var _ = fmt.Sprintf
