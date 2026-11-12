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
		// 没有扩展名：可能是无扩展名的视频文件（网盘里常见），
		// 也可能是目录。这里保持「当视频」的宽松语义，
		// 供 aggregate 兜住无扩展名文件。
		// ⚠️ 判断「是不是目录」时不能用这个函数，见 hasVideoExt。
		return true
	}
	ext := strings.ToLower(name[dot+1:])
	return videoExt[ext]
}

// hasVideoExt 严格判断名字是否带已知视频扩展名（无扩展名返回 false）。
//
// 专门用于「IsDir 标记是否可信」的判定：有些网盘会把 .mp4 标成目录，
// 需要按扩展名纠正；但目录通常没有扩展名，此时必须返回 false，
// 否则所有目录都会被当成视频，扫描直接瘫痪（踩过）。
func hasVideoExt(name string) bool {
	dot := strings.LastIndex(name, ".")
	if dot < 0 || dot == len(name)-1 {
		return false
	}
	return videoExt[strings.ToLower(name[dot+1:])]
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
					// 有些网盘（实测极空间的某些文件）会把 .mp4 之类**标成目录**返回：
					// isDir=true 但名字带视频扩展名。按目录处理会继续下钻，
					// 拿到的是「文件内部的虚拟目录」，文件本身反而漏掉，
					// 最终把文件名当成目录名。
					//
					// 用 hasVideoExt 而不是 isVideo：后者对无扩展名返回 true，
					// 会把普通目录也当成视频（直接导致扫描只认出顶层 2 项）。
					switch {
					case e.IsDir && hasVideoExt(e.Name):
						files = append(files, e) // 被误标为目录的视频文件
					case e.IsDir:
						next = append(next, e.Href)
					case isVideo(e.Name):
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

// aggregate 把扫描到的视频文件整理成「电影 / 剧集」，严格按目录结构，不做跨目录聚合。
//
// 抽成独立函数便于单测：归类逻辑写错时，整部剧会被拆散或不同内容被并在一起，
// 这类退化必须能被测试发现。
func aggregate(src model.WebdavSource, files []model.WebdavEntry) []model.WebdavVod {
	// ---------- 归并策略：严格照搬目录结构，不做任何跨目录聚合 ----------
	//
	// 规则很简单：**一个含视频的目录 = 一部影视**。
	//
	//   迅雷下载/凡人/01 风起天南/1.mp4  → 影视「01 风起天南」
	//   迅雷下载/凡人/02 燕家堡之战/1.mp4 → 影视「02 燕家堡之战」
	//   迅雷下载/电影/某片.mp4           → 影视「某片」
	//
	// 为什么不合并：此前几版尝试过「同一部剧的多个篇章文件夹合并成一部」，
	// 但判断依据（目录名带序号 / 集号连续 / 季名特征）在真实数据里都会误判，
	// 导致毫不相干的剧被并在一起，选集错乱且界面无法纠正。
	//
	// 现在的取舍：**完全信任用户的目录组织**。分得细一点，用户自己还能找；
	// 混错了却没法拆开。目录名保留序号（如「01 风起天南」），
	// 用户一眼就能看出这是第几个篇章，排序也自然正确。

	var vods []model.WebdavVod
	idx := map[string]int{}

	// mountPath 是挂载根在服务端的 URL 路径（已编码，如 /dav/%E8%BF%85%E9%9B%B7%E4%B8%8B%E8%BD%BD）。
	// 用来把每个条目的绝对目录裁成「相对挂载根」的多级路径，
	// 供前端按原来的目录层级分组展示，而不是一路平铺。
	//
	// 注意用 URLPath(src.URL) 而不是 URLPath(MountHref(src))：
	// MountHref 返回值已经过 safePath 编码，再喂给 URLPath 会二次编码，前缀对不上。
	mountPath := strings.TrimRight(safePath(URLPath(src.URL)), "/")

	for i, f := range files {
		p := parse(f.Name)
		// 文件所在目录。
		//
		// dirOf 取「最后一个 / 之前的部分」。但对被网盘误标成目录的视频文件
		//（极空间实测：.mp4 返回 isDir=true 且 href 带尾斜杠 ".../某片.mp4/"），
		// 直接 dirOf 得到的是文件自身（".../电影/某片.mp4"），必须再退一级。
		dir := dirOf(f.Href)
		if f.IsDir && hasVideoExt(f.Name) {
			dir = dirOf(strings.TrimRight(dir, "/"))
		}

		// 归属键：文件所在目录。同一目录下的所有视频归为一部。
		// 电影这类「一个文件就是一部」的，各自独立成条目。
		var key string
		if p.isSeries {
			key = "D|" + dir
		} else {
			key = "#" + strconv.Itoa(i)
		}

		pos, ok := idx[key]
		if !ok {
			// 片名优先用文件解析出的标题，取不到就用**目录名**（保留序号前缀）。
			title := p.title
			if title == "" || !p.isSeries {
				title = stripExt(f.Name)
			}
			kind := "movie"
			if p.isSeries {
				kind = "series"
				// 剧集用目录名做片名：目录结构是用户自己的组织方式，照搬最直观。
				// dirName 内部会做 URL 解码（中文目录名不能直接显示 %E5%87%A1%）。
				if dn := dirName(dir); dn != "" && dn != "/" {
					title = dn
				}
			}
			vods = append(vods, model.WebdavVod{
				Name:   title,
				Kind:   kind,
				Year:   p.year,
				Dir:    dir, // 所在目录（早期版本电影这里误存了文件路径，已修正）
				RelDir: relPathOf(dir, mountPath),
			})
			pos = len(vods) - 1
			idx[key] = pos
		}

		vod := &vods[pos]
		vod.Eps = append(vod.Eps, model.WebdavEp{
			Name: f.Name, URL: AbsURL(src, f.Href), Href: f.Href,
			Season: 0, Episode: p.episode,
		})
	}

	// 目录内的集按集号排序；集号解析不出来时保持原始顺序（SliceStable）
	for i := range vods {
		if vods[i].Kind != "series" {
			continue
		}
		sort.SliceStable(vods[i].Eps, func(a, b int) bool {
			ea, eb := vods[i].Eps[a], vods[i].Eps[b]
			// 无法解析集号的排在后面，但彼此保持原序
			if ea.Episode == 0 || eb.Episode == 0 {
				return ea.Episode != 0 && eb.Episode == 0
			}
			return ea.Episode < eb.Episode
		})
	}

	// 兜底：如果挂载根填的是域名根（mount="/"），relDir 会带上服务端的完整路径
	// （如 "dav/迅雷下载/凡人/01 风起天南"），前端展示就很丑。
	// 这种情况把所有 relDir 统一砍掉第一段（服务端的 mount 前缀），
	// 让层级从「用户实际看到的目录」开始。
	if mountPath == "" || mountPath == "/" {
		// 找出所有 relDir 的最长公共前缀段数
		common := commonPrefixSegs(vods)
		if common > 0 {
			for i := range vods {
				segs := strings.Split(vods[i].RelDir, "/")
				if len(segs) > common {
					vods[i].RelDir = strings.Join(segs[common:], "/")
				} else {
					vods[i].RelDir = ""
				}
			}
		}
	}

	// 整体按片名排序，让目录结构在列表里也保持可预期的顺序
	sort.SliceStable(vods, func(a, b int) bool {
		return naturalLess(vods[a].Name, vods[b].Name)
	})
	return vods
}

// commonPrefixSegs 返回所有条目 relDir 的公共前缀目录段数。
// 用于挂载根为 "/" 时剥掉服务端路径前缀（如 "dav/迅雷下载"）。
func commonPrefixSegs(vods []model.WebdavVod) int {
	if len(vods) == 0 {
		return 0
	}
	first := strings.Split(vods[0].RelDir, "/")
	n := len(first)
	for _, v := range vods[1:] {
		segs := strings.Split(v.RelDir, "/")
		i := 0
		for i < n && i < len(segs) && segs[i] == first[i] {
			i++
		}
		n = i
	}
	// 全相同时（所有条目在同一目录），保留最后一段作为目录名，不要全砍掉
	if n >= len(first) {
		n = len(first) - 1
	}
	if n < 0 {
		n = 0
	}
	return n
}

// relPathOf 把绝对目录裁成「相对挂载根」的路径，并解码成可展示的中文。
//
//	mount       = /dav/迅雷下载
//	dir         = /dav/迅雷下载/凡人/01 风起天南
//	→ 返回      凡人/01 风起天南
//
// 位于挂载根本身的条目（dir == mount）返回空串。
// 若 dir 不在 mount 之下（理论上不该发生），返回空串而不是乱码路径。
func relPathOf(dir, mount string) string {
	d := strings.TrimRight(dir, "/")
	m := strings.TrimRight(mount, "/")
	if m != "" {
		if d == m {
			return ""
		}
		if !strings.HasPrefix(d, m+"/") {
			return ""
		}
		d = strings.TrimPrefix(d, m+"/")
	}
	d = strings.TrimPrefix(d, "/")
	// 逐段解码：整串 PathUnescape 遇到 %2F 会解码成 /，破坏层级，所以按段处理
	segs := strings.Split(d, "/")
	for i, seg := range segs {
		if dec, err := url.PathUnescape(seg); err == nil {
			segs[i] = dec
		}
	}
	return strings.Join(segs, "/")
}

// naturalLess 按「自然顺序」比较字符串：数字段按数值比，
// 让「01 xxx」「2 yyy」「10 zzz」排成 01 < 2 < 10 而不是字典序 01 < 10 < 2。
func naturalLess(a, b string) bool {
	ai, bi := 0, 0
	for ai < len(a) && bi < len(b) {
		ca, cb := a[ai], b[bi]
		if isDigit(ca) && isDigit(cb) {
			// 各取一段连续数字
			as, ae := ai, ai
			for ae < len(a) && isDigit(a[ae]) {
				ae++
			}
			bs, be := bi, bi
			for be < len(b) && isDigit(b[be]) {
				be++
			}
			na := strings.TrimLeft(a[as:ae], "0")
			nb := strings.TrimLeft(b[bs:be], "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			ai, bi = ae, be
			continue
		}
		if ca != cb {
			return ca < cb
		}
		ai++
		bi++
	}
	return len(a)-ai < len(b)-bi
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

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
