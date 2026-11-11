package quality

import (
	"strings"
	"testing"
)

// ---------- LabelOf / LabelOfLevel ----------

func TestLabelOfByHeight(t *testing.T) {
	cases := []struct {
		h, bps int
		want   string
	}{
		{2160, 0, "4K"},
		{1440, 0, "2K"},
		{1080, 0, "1080P"},
		{720, 0, "720P"},
		{540, 0, "540P"},
		{480, 0, "480P"},
		{360, 0, "360P"},
		// 无分辨率时按码率分级
		{0, 4_000_000, "高清"},
		{0, 1_500_000, "标清"},
		{0, 500_000, "流畅"},
		{0, 0, "原画"},
		// 边界：分辨率够高时优先于码率
		{720, 100, "720P"},
	}
	for _, c := range cases {
		if got := LabelOf(c.h, c.bps); got != c.want {
			t.Errorf("LabelOf(%d,%d) = %q，期望 %q", c.h, c.bps, got, c.want)
		}
	}
}

func TestLabelOfLevelFallbacks(t *testing.T) {
	// 完全没有信息 → 原画，但有 NAME 时用 NAME
	if got := LabelOfLevel(QLevel{Name: "杜比视界"}); got != "杜比视界" {
		t.Errorf("无名分辨率时应用 NAME，实际 %q", got)
	}
	if got := LabelOfLevel(QLevel{}); got != "原画" {
		t.Errorf("完全空时应为原画，实际 %q", got)
	}
	// 「模糊档位」但给了分辨率 → 用分辨率兜底（576 落在 >=540 档，显示 540P）
	if got := LabelOfLevel(QLevel{Height: 576, Bps: 1_600_000}); got != "540P" {
		t.Errorf("标清但有分辨率时应显示 540P，实际 %q", got)
	}
	// 模糊档位、无分辨率、有码率 → 「标清 1500K」
	if got := LabelOfLevel(QLevel{Bps: 1_800_000}); got != "标清 1800K" {
		t.Errorf("应显示码率，实际 %q", got)
	}
}

// ---------- ParseMaster ----------

const masterPlaylist = `#EXTM3U
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=800000,BANDWIDTH=1200000,RESOLUTION=1280x720,NAME="720P"
720/index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2500000,RESOLUTION=1920x1080
1080/index.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=6000000,RESOLUTION=3840x2160,VIDEO-RANGE=HDR
https://cdn.example.com/4k/index.m3u8
`

func TestParseMaster(t *testing.T) {
	lv := ParseMaster(masterPlaylist, "https://origin.example.com/vod/")
	if len(lv) != 3 {
		t.Fatalf("应解析出 3 档，实际 %d：%+v", len(lv), lv)
	}
	// 按高度升序
	if lv[0].Height != 720 || lv[1].Height != 1080 || lv[2].Height != 2160 {
		t.Fatalf("应按高度升序，实际 %+v", heights(lv))
	}
	if lv[0].Width != 1280 {
		t.Errorf("720P 宽应为 1280，实际 %d", lv[0].Width)
	}
	// BANDWIDTH 覆盖 AVERAGE-BANDWIDTH
	if lv[0].Bps != 1200000 {
		t.Errorf("Bps 应取 BANDWIDTH=1200000，实际 %d", lv[0].Bps)
	}
	// NAME 优先
	if lv[0].Name != "720P" {
		t.Errorf("Name 应为 720P，实际 %q", lv[0].Name)
	}
	// 没有 NAME 时回落到 VIDEO-RANGE
	if lv[2].Name != "HDR" {
		t.Errorf("无 NAME 时应用 VIDEO-RANGE，实际 %q", lv[2].Name)
	}
	// 相对地址要拼成绝对地址；绝对地址保持原样
	if !strings.HasPrefix(lv[0].URL, "https://origin.example.com/vod/") {
		t.Errorf("相对地址未拼接：%q", lv[0].URL)
	}
	if lv[2].URL != "https://cdn.example.com/4k/index.m3u8" {
		t.Errorf("绝对地址不应被改写，实际 %q", lv[2].URL)
	}
}

func TestParseMasterNotMaster(t *testing.T) {
	// 媒体列表（无 EXT-X-STREAM-INF）应返回空，表示「只有一档」
	media := "#EXTM3U\n#EXTINF:10,\nseg1.ts\n#EXTINF:10,\nseg2.ts\n"
	if lv := ParseMaster(media, ""); len(lv) != 0 {
		t.Fatalf("媒体列表不应解析出档位，实际 %+v", lv)
	}
	if lv := ParseMaster("", ""); len(lv) != 0 {
		t.Fatalf("空文本应返回空，实际 %+v", lv)
	}
}

func TestParseMasterCRLF(t *testing.T) {
	// Windows 换行与多余空行不能影响解析
	crlf := strings.ReplaceAll(masterPlaylist, "\n", "\r\n") + "\r\n\r\n"
	lv := ParseMaster(crlf, "https://o.example.com/")
	if len(lv) != 3 {
		t.Fatalf("CRLF 应解析出 3 档，实际 %d", len(lv))
	}
}

// ---------- PickBySpeed ----------

func TestPickBySpeed(t *testing.T) {
	levels := []QLevel{
		{Height: 360, Bps: 500_000},
		{Height: 720, Bps: 1_500_000},
		{Height: 1080, Bps: 3_000_000},
	}
	// 0 或负值：无法实测，取最低档（0 号）
	if got := PickBySpeed(0, levels); got != 0 {
		t.Errorf("实测为 0 应取最低档，实际 %d", got)
	}
	// 空列表 → -1
	if got := PickBySpeed(1000, nil); got != -1 {
		t.Errorf("空列表应返回 -1，实际 %d", got)
	}
	// 留 30% 余量：10Mbps 预算 7000kbps，1080P 需 3000kbps → 可上 1080P
	if got := PickBySpeed(10_000, levels); got != 2 {
		t.Errorf("10Mbps 应选 1080P(idx2)，实际 %d", got)
	}
	// 2Mbps 预算 1400kbps → 720P(1500) 超了，只能 360P
	if got := PickBySpeed(2_000, levels); got != 0 {
		t.Errorf("2Mbps 应选 360P(idx0)，实际 %d", got)
	}
	// 极低速：一档都不够，仍返回 0（不能返回 -1，否则调用方会乱）
	if got := PickBySpeed(100, levels); got != 0 {
		t.Errorf("极低速应兜底取 0，实际 %d", got)
	}
	// 无 Bps 但有分辨率：按 height*4000 估算需求
	noBps := []QLevel{{Height: 480}, {Height: 1080}}
	// 1080*4000 = 4.32Mbps 需要；10Mbps*0.7=7Mbps 够
	if got := PickBySpeed(10_000, noBps); got != 1 {
		t.Errorf("按分辨率估算应选 1080P，实际 %d", got)
	}
}

// ---------- PickByCap ----------

func TestPickByCap(t *testing.T) {
	levels := []QLevel{
		{Height: 360},
		{Height: 720},
		{Height: 1080},
	}
	if got := PickByCap(TOP, levels); got != 2 {
		t.Errorf("TOP 应取最高档，实际 %d", got)
	}
	if got := PickByCap(720, levels); got != 1 {
		t.Errorf("cap=720 应选 720P，实际 %d", got)
	}
	// cap 低于最低档：仍返回 0（保证有得播）
	if got := PickByCap(240, levels); got != 0 {
		t.Errorf("cap 过低应兜底 0，实际 %d", got)
	}
	if got := PickByCap(1000, levels); got != 1 {
		t.Errorf("cap=1000 应选 720P(idx1)，实际 %d", got)
	}
	if got := PickByCap(1080, nil); got != -1 {
		t.Errorf("空列表应返回 -1，实际 %d", got)
	}
	// Height 未知（0）的档位视为「可接受」，取最后一个
	withUnknown := []QLevel{{Height: 720}, {Height: 0}}
	if got := PickByCap(720, withUnknown); got != 1 {
		t.Errorf("未知分辨率应被接受，实际 %d", got)
	}
}

// ---------- FirstSegment ----------

func TestFirstSegment(t *testing.T) {
	pl := "#EXTM3U\n#EXT-X-TARGETDURATION:10\n#EXTINF:10.0,\nseg0.ts\n#EXTINF:10.0,\nseg1.ts\n"
	got := FirstSegment(pl, "https://cdn.example.com/a/b/index.m3u8")
	if !strings.HasSuffix(got, "seg0.ts") {
		t.Fatalf("应取第一个分片，实际 %q", got)
	}
	if !strings.HasPrefix(got, "https://") {
		t.Fatalf("应为绝对地址，实际 %q", got)
	}

	// m4s 分片
	if got := FirstSegment("#EXTINF:5,\ninit.m4s\n", "https://c.com/x/"); !strings.HasSuffix(got, "init.m4s") {
		t.Errorf("应识别 m4s，实际 %q", got)
	}
	// 没有分片 → 空
	if got := FirstSegment("#EXTM3U\n", "https://c.com/"); got != "" {
		t.Errorf("无分片应返回空，实际 %q", got)
	}
}

// ---------- FmtKbps ----------

// FmtKbps 的入参是 kbps，但展示单位是 KB/s（÷8 的换算不在这层做），
// 这里按现有行为锁定，避免以后有人「顺手改成 Mbps」破坏前端显示。
func TestFmtKbps(t *testing.T) {
	cases := map[int]string{
		0:     "", // 无数据时不显示
		-1:    "",
		500:   "500 KB/s",
		999:   "999 KB/s",
		1000:  "10.0 MB/s", // kbps/100 + "." + 余数两位
		1500:  "15.0 MB/s",
		10000: "100.0 MB/s",
	}
	for in, want := range cases {
		if got := FmtKbps(in); got != want {
			t.Errorf("FmtKbps(%d) = %q，期望 %q", in, got, want)
		}
	}
}

func heights(lv []QLevel) []int {
	out := make([]int, len(lv))
	for i, l := range lv {
		out[i] = l.Height
	}
	return out
}
