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

// ============ 核心语义：严格按目录结构，不做任何跨目录聚合 ============
//
// 需求（v1.10.0 起）：一个含视频的目录 = 一部影视。
// 即使目录结构是「剧名/01 篇章/1.mp4」「剧名/02 篇章/2.mp4」，
// 也要拆成「01 篇章」「02 篇章」两条，而不是合并成《剧名》。
//
// 背景：此前几版尝试自动合并篇章目录，但判定依据（目录名带序号 /
// 集号连续 / 季名特征）在真实数据里都会误判，导致不相关的剧被并在一起，
// 选集错乱且界面无法纠正。改为完全信任用户的目录组织。

// 篇章目录必须各自独立，片名照搬目录名（保留序号前缀）。
func TestAggregateKeepsChapterDirsSeparate(t *testing.T) {
	root := "/dav/迅雷下载/凡人"
	var files []model.WebdavEntry
	for _, ep := range []string{"1.mp4", "2.mp4", "3.mp4"} {
		files = append(files, mkFile(root+"/01 风起天南/"+ep))
	}
	for _, ep := range []string{"18.mp4", "19.mp4"} {
		files = append(files, mkFile(root+"/02 燕家堡之战/"+ep))
	}
	vods := aggregate(testSrc, files)

	if len(vods) != 2 {
		t.Fatalf("两个篇章目录应各自独立（2 部），实际 %d 部：%+v", len(vods), nameList(vods))
	}
	got := map[string]int{}
	for _, v := range vods {
		got[v.Name] = len(v.Eps)
	}
	// 片名保留序号前缀，与目录名完全一致
	if got["01 风起天南"] != 3 || got["02 燕家堡之战"] != 2 {
		t.Fatalf("片名应照搬目录名（含序号），实际 %v", got)
	}
	// 绝不能出现以父目录命名的聚合结果
	for _, v := range vods {
		if v.Name == "凡人" {
			t.Fatalf("不应聚合出父目录名《凡人》：%d 集", len(v.Eps))
		}
	}
}

// 平铺的多部剧：各自独立。
func TestAggregateFlatDirsStaySeparate(t *testing.T) {
	root := "/dav/剧集"
	var files []model.WebdavEntry
	for _, ep := range []string{"1.mp4", "2.mp4"} {
		files = append(files, mkFile(root+"/庆余年/"+ep))
	}
	for _, ep := range []string{"1.mp4", "2.mp4", "3.mp4"} {
		files = append(files, mkFile(root+"/狂飙/"+ep))
	}
	vods := aggregate(testSrc, files)

	if len(vods) != 2 {
		t.Fatalf("平铺的两部剧应各自独立，实际 %d 部：%+v", len(vods), nameList(vods))
	}
	got := map[string]int{}
	for _, v := range vods {
		got[v.Name] = len(v.Eps)
	}
	if got["庆余年"] != 2 || got["狂飙"] != 3 {
		t.Fatalf("应识别「庆余年」2 集、「狂飙」3 集，实际 %v", got)
	}
}

// 每部一个文件（电影）：一个文件 = 一条，绝不能合并。
func TestAggregateMoviesSeparate(t *testing.T) {
	root := "/dav/迅雷下载/电影"
	files := []model.WebdavEntry{
		mkFile(root + "/异星战境 Atlas.mkv"),
		mkFile(root + "/不可饶恕 BD国英双语中字.mp4"),
		mkFile(root + "/侏罗纪世界：重生-2025.mp4"),
	}
	vods := aggregate(testSrc, files)

	if len(vods) != 3 {
		t.Fatalf("三部电影应各自独立，实际 %d 部：%+v", len(vods), nameList(vods))
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

// 同一目录内的多集要归到一条（这是唯一允许的聚合：目录内部）。
func TestAggregateSameDirIsOneSeries(t *testing.T) {
	root := "/dav/某剧"
	files := []model.WebdavEntry{
		mkFile(root + "/第01集.mp4"),
		mkFile(root + "/第02集.mp4"),
		mkFile(root + "/第03集.mp4"),
	}
	vods := aggregate(testSrc, files)

	if len(vods) != 1 {
		t.Fatalf("同一目录的多集应归为 1 部，实际 %d 部：%+v", len(vods), nameList(vods))
	}
	if vods[0].Kind != "series" || len(vods[0].Eps) != 3 {
		t.Fatalf("应为 3 集剧集，实际 kind=%s eps=%d", vods[0].Kind, len(vods[0].Eps))
	}
	if vods[0].Name != "某剧" {
		t.Errorf("片名应为目录名「某剧」，实际 %q", vods[0].Name)
	}
}

// 目录内的集必须按集号排序，不能依赖 PROPFIND 返回顺序。
func TestAggregateSortsEpisodesInDir(t *testing.T) {
	root := "/dav/某剧"
	// 故意乱序
	files := []model.WebdavEntry{
		mkFile(root + "/10.mp4"),
		mkFile(root + "/2.mp4"),
		mkFile(root + "/1.mp4"),
	}
	vods := aggregate(testSrc, files)
	if len(vods) != 1 {
		t.Fatalf("应为 1 部，实际 %d", len(vods))
	}
	eps := vods[0].Eps
	if len(eps) != 3 {
		t.Fatalf("应为 3 集，实际 %d", len(eps))
	}
	if eps[0].Episode != 1 || eps[1].Episode != 2 || eps[2].Episode != 10 {
		t.Fatalf("应按集号 1,2,10 排序，实际 %d,%d,%d", eps[0].Episode, eps[1].Episode, eps[2].Episode)
	}
}

// 列表整体按自然顺序排列：1 < 2 < 10（字典序会变成 1 < 10 < 2）。
func TestAggregateSortsNaturally(t *testing.T) {
	root := "/dav/合集"
	var files []model.WebdavEntry
	for _, d := range []string{"2 第二部", "10 第十部", "1 第一部"} {
		files = append(files, mkFile(root+"/"+d+"/1.mp4"))
	}
	vods := aggregate(testSrc, files)
	if len(vods) != 3 {
		t.Fatalf("应为 3 部，实际 %d", len(vods))
	}
	want := []string{"1 第一部", "2 第二部", "10 第十部"}
	for i, w := range want {
		if vods[i].Name != w {
			t.Fatalf("自然排序应得到 %v，实际 %v", want, nameList(vods))
		}
	}
}

// 集号解析：纯数字 / 第xx集 / SxxExx 都要能取到。
func TestParseEpisodeForms(t *testing.T) {
	cases := []struct {
		name   string
		wantEp int
		wantS  int
	}{
		{"1.mp4", 1, 0},
		{"第12集.mp4", 12, 0},
		{"Show.S02E07.mkv", 7, 2},
		{"电影名 2024.mp4", 0, 0},
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

// dirOrder：目录名前缀序号
func TestDirOrder(t *testing.T) {
	cases := map[string]int{
		"01 风起天南": 1,
		"9 慕兰之战":  9,
		"10-外海风云": 10,
		"凡人":      0,
		"2024":    0, // 四位数不当作序号
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
