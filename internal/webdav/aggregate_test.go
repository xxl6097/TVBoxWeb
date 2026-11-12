package webdav

import (
	"net/url"
	"strings"
	"testing"

	"github.com/xxl6097/TVBoxWeb/internal/model"
)

// mkFile 构造一个 WebDAV 文件条目，href 用真实目录结构（含中文，需 URL 编码）。
func mkFile(path string) model.WebdavEntry {
	return model.WebdavEntry{Name: baseName(path), Href: encodePath(path)}
}

func baseName(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}

// encodePath 逐段 URL 编码，保持与真实 WebDAV href 一致（中文被百分号编码）。
// 按 "/" 切段后整段 PathEscape —— 不能逐字节拼接，否则会把 UTF-8 字节切碎。
func encodePath(p string) string {
	parts := strings.Split(p, "/")
	for i, seg := range parts {
		parts[i] = url.PathEscape(seg)
	}
	return strings.Join(parts, "/")
}

var testSrc = model.WebdavSource{ID: "t", Name: "t", URL: "https://dav.example.com", User: "u", Pass: "p"}

// 核心回归：「剧名/01 篇章/1.mp4」这种结构必须合成一部剧，
// 而不是按目录拆成十几部。集号在整部剧内连续，篇章目录体现为季号。
func TestAggregateMergesSeasons(t *testing.T) {
	root := "/dav/迅雷下载"
	var files []model.WebdavEntry
	// 两个篇章：01 风起天南（1~3 集）、03 魔道争锋（4~5 集）
	for _, ep := range []string{"1.mp4", "2.mp4", "3.mp4"} {
		files = append(files, mkFile(root+"/凡人/01 风起天南/"+ep))
	}
	for _, ep := range []string{"4.mp4", "5.mp4"} {
		files = append(files, mkFile(root+"/凡人/03 魔道争锋/"+ep))
	}
	vods := aggregate(testSrc, files)

	if len(vods) != 1 {
		t.Fatalf("两个篇章应合成 1 部剧，实际 %d 部：%+v", len(vods), nameList(vods))
	}
	v := vods[0]
	if v.Kind != "series" {
		t.Fatalf("应为剧集，实际 %s", v.Kind)
	}
	if v.Name != "凡人" {
		t.Fatalf("片名应取剧名目录「凡人」，实际 %q", v.Name)
	}
	if len(v.Eps) != 5 {
		t.Fatalf("应聚齐 5 集，实际 %d", len(v.Eps))
	}
	// 季号来自子目录序号：01→1，03→3
	if v.Eps[0].Season != 1 || v.Eps[0].Episode != 1 {
		t.Errorf("首集应为 S1E1，实际 S%dE%d", v.Eps[0].Season, v.Eps[0].Episode)
	}
	if v.Eps[4].Season != 3 || v.Eps[4].Episode != 5 {
		t.Errorf("末集应为 S3E5，实际 S%dE%d", v.Eps[4].Season, v.Eps[4].Episode)
	}
	// 排序：季优先，其次集
	if !(v.Eps[2].Season == 1 && v.Eps[3].Season == 3) {
		t.Errorf("应按季排序，实际顺序 %+v", seasonEpisodePairs(v))
	}
}

// 电影不能因为「同一目录下多个文件」被并成一部：每部保持独立。
func TestAggregateKeepsMoviesSeparate(t *testing.T) {
	root := "/dav/迅雷下载/电影"
	files := []model.WebdavEntry{
		mkFile(root + "/异星战境 Atlas.mkv"),
		mkFile(root + "/不可饶恕 BD国英双语中字.mp4"),
	}
	vods := aggregate(testSrc, files)
	if len(vods) != 2 {
		t.Fatalf("两部电影应各自独立，实际 %d 部：%+v", len(vods), nameList(vods))
	}
	for _, v := range vods {
		if v.Kind != "movie" {
			t.Errorf("%q 应为 movie，实际 %s", v.Name, v.Kind)
		}
		if len(v.Eps) != 1 {
			t.Errorf("%q 应只有 1 集，实际 %d", v.Name, len(v.Eps))
		}
	}
}

// 单目录剧集（无篇章子目录）仍要能识别成 series。
func TestAggregateSingleDirSeries(t *testing.T) {
	root := "/dav/某剧"
	files := []model.WebdavEntry{
		mkFile(root + "/第01集.mp4"),
		mkFile(root + "/第02集.mp4"),
	}
	vods := aggregate(testSrc, files)
	if len(vods) != 1 {
		t.Fatalf("应识别为 1 部，实际 %d：%+v", len(vods), nameList(vods))
	}
	if vods[0].Kind != "series" || len(vods[0].Eps) != 2 {
		t.Fatalf("应为 2 集剧集，实际 kind=%s eps=%d", vods[0].Kind, len(vods[0].Eps))
	}
}

// 集号解析：纯数字文件名 / 第xx集 / SxxExx 都要能取到。
func TestParseEpisodeForms(t *testing.T) {
	cases := []struct {
		name     string
		wantEp   int
		wantS    int
		wantKind string
	}{
		{"1.mp4", 1, 0, "series"},
		{"第12集.mp4", 12, 0, "series"},
		{"Show.S02E07.mkv", 7, 2, "series"},
		{"电影名 2024.mp4", 0, 0, "movie"},
	}
	for _, c := range cases {
		p := parse(c.name)
		if p.episode != c.wantEp {
			t.Errorf("parse(%q).episode = %d，期望 %d", c.name, p.episode, c.wantEp)
		}
		if p.season != c.wantS {
			t.Errorf("parse(%q).season = %d，期望 %d", c.name, p.season, c.wantS)
		}
	}
}

// 关键回归：不同影视放在同一个父目录下，且子目录名**没有序号前缀**时，
// 必须各自独立成一部。
//
// 典型场景：网盘里「影视/」下面平铺着好几部剧，像
//
//	影视/庆余年/1.mp4
//	影视/狂飙/1.mp4
//
// 旧策略只看「父目录下有几个含视频的子目录」，≥2 就往上合并，
// 结果这两部毫不相干的剧被并成一部《影视》，选集里混在一起。
func TestAggregateKeepsUnnumberedDirsSeparate(t *testing.T) {
	root := "/dav/影视"
	var files []model.WebdavEntry
	for _, ep := range []string{"1.mp4", "2.mp4"} {
		files = append(files, mkFile(root+"/庆余年/"+ep))
	}
	for _, ep := range []string{"1.mp4", "2.mp4"} {
		files = append(files, mkFile(root+"/狂飙/"+ep))
	}
	vods := aggregate(testSrc, files)

	if len(vods) != 2 {
		t.Fatalf("两个无序号目录应各自独立（共 2 部），实际 %d 部：%+v", len(vods), nameList(vods))
	}
	names := map[string]int{}
	for _, v := range vods {
		names[v.Name] = len(v.Eps)
	}
	if names["庆余年"] != 2 || names["狂飙"] != 2 {
		t.Fatalf("应识别出「庆余年」和「狂飙」各 2 集，实际 %v", names)
	}
	// 绝不能出现以父目录命名的合并结果
	for _, v := range vods {
		if v.Name == "影视" {
			t.Fatalf("不应把整个父目录合并成一部，实际出现 %q（%d 集）", v.Name, len(v.Eps))
		}
	}
}

// 对比：子目录名**带序号**时才认定为同一部剧的分季结构。
// 这保证「01 篇章 / 02 篇章」仍能正确合并（见 TestAggregateMergesSeasons）。
func TestAggregateMergeRequiresNumberedDirs(t *testing.T) {
	root := "/dav/剧集"
	// 混合场景：有序号的分季剧 + 无序号的独立剧同处一个父目录
	var files []model.WebdavEntry
	files = append(files, mkFile(root+"/某长篇/01 第一季/1.mp4"))
	files = append(files, mkFile(root+"/某长篇/02 第二季/1.mp4"))
	files = append(files, mkFile(root+"/另一部剧/1.mp4"))

	vods := aggregate(testSrc, files)
	if len(vods) != 2 {
		t.Fatalf("分季剧应合成 1 部、独立剧保持 1 部，共 2 部；实际 %d 部：%+v", len(vods), nameList(vods))
	}
	for _, v := range vods {
		switch v.Name {
		case "某长篇":
			if len(v.Eps) != 2 {
				t.Errorf("「某长篇」应有 2 集，实际 %d", len(v.Eps))
			}
		case "另一部剧":
			if len(v.Eps) != 1 {
				t.Errorf("「另一部剧」应有 1 集，实际 %d", len(v.Eps))
			}
		default:
			t.Errorf("出现了意料之外的条目：%q", v.Name)
		}
	}
}

// dirOrder：目录名前缀序号 → 季号
func TestDirOrder(t *testing.T) {
	cases := map[string]int{
		"01 风起天南": 1,
		"9 慕兰之战":  9,
		"10-外海风云": 10,
		"凡人":      0,
		"2024":    0, // 四位数不当作季号
	}
	for in, want := range cases {
		if got := dirOrder(in); got != want {
			t.Errorf("dirOrder(%q) = %d，期望 %d", in, got, want)
		}
	}
}

func nameList(vods []model.WebdavVod) []string {
	out := make([]string, len(vods))
	for i, v := range vods {
		out[i] = v.Name
	}
	return out
}

func seasonEpisodePairs(v model.WebdavVod) []string {
	out := make([]string, len(v.Eps))
	for i, e := range v.Eps {
		out[i] = "S" + itoa(e.Season) + "E" + itoa(e.Episode)
	}
	return out
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

// 用户实际场景：某个文件夹下平铺着几部不同的剧，每部一个子文件夹。
//
// 关键难点：这些子目录**恰好带序号前缀**（01 庆余年 / 02 狂飙），
// 光看目录名和「01 风起天南 / 02 燕家堡之战」一模一样，
// 但前者是不同的剧、后者是同一部剧的篇章。
//
// 区分靠集号：同一部剧的篇章集号跨目录连续（1..17, 18..31），
// 不同的剧各自从 1 重新开始。用 map[min]max 会因键冲突漏判（踩过）。
func TestAggregateFlatNumberedDifferentShows(t *testing.T) {
	root := "/dav/剧集"
	files := []model.WebdavEntry{
		mkFile(root + "/01 庆余年/1.mp4"),
		mkFile(root + "/01 庆余年/2.mp4"),
		mkFile(root + "/01 庆余年/3.mp4"),
		mkFile(root + "/02 狂飙/1.mp4"),
		mkFile(root + "/02 狂飙/2.mp4"),
	}
	vods := aggregate(testSrc, files)

	if len(vods) != 2 {
		t.Fatalf("平铺的两部不同剧应各自独立（2 部），实际 %d 部：%+v", len(vods), nameList(vods))
	}
	got := map[string]int{}
	for _, v := range vods {
		got[v.Name] = len(v.Eps)
	}
	if got["庆余年"] != 3 || got["狂飙"] != 2 {
		t.Fatalf("应识别「庆余年」3 集、「狂飙」2 集，实际 %v", got)
	}
	for _, v := range vods {
		if v.Name == "剧集" {
			t.Fatalf("不应合并成父目录名《剧集》：%d 集", len(v.Eps))
		}
	}
}

// 对照组：同样是带序号的子目录，但集号跨目录连续 → 是同一部剧的篇章，应合并。
func TestAggregateNumberedContiguousMerges(t *testing.T) {
	root := "/dav/凡人"
	files := []model.WebdavEntry{
		mkFile(root + "/01 风起天南/1.mp4"),
		mkFile(root + "/01 风起天南/17.mp4"),
		mkFile(root + "/02 燕家堡之战/18.mp4"),
		mkFile(root + "/02 燕家堡之战/31.mp4"),
		mkFile(root + "/03 魔道争锋/32.mp4"),
	}
	vods := aggregate(testSrc, files)

	if len(vods) != 1 {
		t.Fatalf("集号连续应合并成 1 部，实际 %d 部：%+v", len(vods), nameList(vods))
	}
	if vods[0].Name != "凡人" {
		t.Errorf("片名应为「凡人」，实际 %q", vods[0].Name)
	}
	if len(vods[0].Eps) != 5 {
		t.Errorf("应有 5 集，实际 %d", len(vods[0].Eps))
	}
}

// 「第一季 / 第二季」这种结构：集号各自从 1 开始，但目录名带季特征，应合并。
func TestAggregateSeasonNamedDirsMerge(t *testing.T) {
	root := "/dav/某剧"
	files := []model.WebdavEntry{
		mkFile(root + "/第一季/1.mp4"),
		mkFile(root + "/第一季/2.mp4"),
		mkFile(root + "/第二季/1.mp4"),
		mkFile(root + "/第二季/2.mp4"),
	}
	vods := aggregate(testSrc, files)
	if len(vods) != 1 {
		t.Fatalf("带季名的子目录应合并成 1 部，实际 %d 部：%+v", len(vods), nameList(vods))
	}
	if vods[0].Name != "某剧" {
		t.Errorf("片名应为「某剧」，实际 %q", vods[0].Name)
	}
}

// 各种「集号范围」组合下，平铺的多部剧必须各自独立。
// 用户反馈「每部的集号范围」差异很大，这里穷举常见组合。
func TestAggregateFlatVariousEpisodeRanges(t *testing.T) {
	cases := []struct {
		name  string
		dirs  map[string][2]int // 子目录名 → [起, 止]
		wantN int
	}{
		{
			name:  "都从1开始",
			dirs:  map[string][2]int{"01 庆余年": {1, 20}, "02 狂飙": {1, 30}},
			wantN: 2,
		},
		{
			name:  "都从1开始且集数相同",
			dirs:  map[string][2]int{"01 A剧": {1, 10}, "02 B剧": {1, 10}},
			wantN: 2,
		},
		{
			name:  "区间重叠（A 1~20, B 15~40）",
			dirs:  map[string][2]int{"01 A剧": {1, 20}, "02 B剧": {15, 40}},
			wantN: 2,
		},
		{
			name:  "区间相接但不对齐（A 1~20, B 21~50）",
			dirs:  map[string][2]int{"01 A剧": {1, 20}, "02 B剧": {21, 50}},
			wantN: 2, // 名字不同 → 仍是两部；不能因为集号接上就合并
		},
		{
			name:  "完全不相接（A 1~10, B 100~120）",
			dirs:  map[string][2]int{"01 A剧": {1, 10}, "02 B剧": {100, 120}},
			wantN: 2,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := "/dav/剧集"
			var files []model.WebdavEntry
			for dir, rng := range c.dirs {
				for ep := rng[0]; ep <= rng[1] && ep <= rng[0]+3; ep++ {
					files = append(files, mkFile(root+"/"+dir+"/"+itoa(ep)+".mp4"))
				}
			}
			vods := aggregate(testSrc, files)
			if len(vods) != c.wantN {
				t.Fatalf("%s：期望 %d 部，实际 %d 部：%+v", c.name, c.wantN, len(vods), nameList(vods))
			}
			for _, v := range vods {
				if v.Name == "剧集" {
					t.Fatalf("%s：不应合并成父目录名《剧集》：%d 集", c.name, len(v.Eps))
				}
			}
		})
	}
}
