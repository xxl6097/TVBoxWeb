// Package quality 实现清晰度解析与网速实测，与鸿蒙影视 Quality.ets 一致。
package quality

import (
	"context"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/xxl6097/TVBoxWeb/internal/httpc"
)

const (
	AUTO = 0
	TOP  = 9999
)

type QLevel struct {
	Height int    `json:"height"`
	Width  int    `json:"width"`
	Bps    int    `json:"bps"`
	URL    string `json:"url"`
	Name   string `json:"name"`
}

type QSpeed struct {
	Kbps  int `json:"kbps"`
	Bytes int `json:"bytes"`
	MS    int `json:"ms"`
}

// ParseMaster 解析 HLS master playlist，只有带 EXT-X-STREAM-INF 才算多档。
func ParseMaster(text, base string) []QLevel {
	var out []QLevel
	if !strings.Contains(text, "#EXT-X-STREAM-INF") {
		return out
	}
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"), "\n")
	var pending *QLevel
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF") {
			lv := QLevel{}
			if v := numAttr(line, "AVERAGE-BANDWIDTH"); v > 0 {
				lv.Bps = v
			}
			if peak := numAttr(line, "BANDWIDTH"); peak > 0 {
				lv.Bps = peak
			}
			if res := strAttr(line, "RESOLUTION"); res != "" {
				if x := strings.Index(strings.ToLower(res), "x"); x > 0 {
					lv.Width = toNum(res[:x])
					lv.Height = toNum(res[x+1:])
				}
			}
			lv.Name = strAttr(line, "NAME")
			if lv.Name == "" {
				lv.Name = strAttr(line, "VIDEO-RANGE")
			}
			pending = &lv
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if pending != nil {
			pending.URL = abs(base, line)
			out = append(out, *pending)
			pending = nil
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ha := out[i].Height
		if ha <= 0 {
			ha = out[i].Bps / 20000
		}
		hb := out[j].Height
		if hb <= 0 {
			hb = out[j].Bps / 20000
		}
		if ha != hb {
			return ha < hb
		}
		return out[i].Bps < out[j].Bps
	})
	return out
}

// LabelOf 显示名，优先用分辨率。
func LabelOf(h, bps int) string {
	switch {
	case h >= 2160:
		return "4K"
	case h >= 1440:
		return "2K"
	case h >= 1080:
		return "1080P"
	case h >= 720:
		return "720P"
	case h >= 540:
		return "540P"
	case h >= 480:
		return "480P"
	case h >= 360:
		return "360P"
	case bps >= 4000000:
		return "高清"
	case bps >= 1500000:
		return "标清"
	case bps > 0:
		return "流畅"
	default:
		return "原画"
	}
}

func LabelOfLevel(l QLevel) string {
	s := LabelOf(l.Height, l.Bps)
	if s == "原画" && l.Name != "" {
		return l.Name
	}
	vague := s == "流畅" || s == "标清" || s == "高清"
	if vague && l.Height > 0 {
		return strconv.Itoa(l.Height) + "P"
	}
	if vague && l.Bps > 0 {
		return s + " " + strconv.Itoa(l.Bps/1000) + "K"
	}
	return s
}

// PickBySpeed 按实测速率挑档：留 30% 余量。
func PickBySpeed(kbps int, levels []QLevel) int {
	if len(levels) == 0 {
		return -1
	}
	if kbps <= 0 {
		return 0
	}
	budget := float64(kbps) * 0.7 * 1000
	best := -1
	for i, l := range levels {
		need := float64(l.Bps)
		if l.Bps <= 0 && l.Height > 0 {
			need = float64(l.Height * 4000)
		}
		if need <= budget {
			best = i
		}
	}
	if best < 0 {
		return 0
	}
	return best
}

// PickByCap 选「不高于 cap 的最高档」；cap 为 TOP 时直接取最高档。
func PickByCap(cap int, levels []QLevel) int {
	if len(levels) == 0 {
		return -1
	}
	if cap >= TOP {
		return len(levels) - 1
	}
	best := -1
	for i, l := range levels {
		if l.Height <= 0 || l.Height <= cap {
			best = i
		}
	}
	if best < 0 {
		return 0
	}
	return best
}

// FirstSegment 从媒体列表里找第一个分片。
func FirstSegment(playlist, base string) string {
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(playlist, "\r\n", "\n"), "\r", "\n"), "\n")
	nextIsSeg := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#EXTINF") {
			nextIsSeg = true
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if nextIsSeg || strings.Contains(line, ".ts") || strings.Contains(line, ".m4s") ||
			strings.Contains(line, ".mp4") || strings.Contains(line, ".flv") {
			return abs(base, line)
		}
	}
	return ""
}

// Measure 实测下行速率 kbps。
func Measure(url string, ms time.Duration, limit int) QSpeed {
	st := QSpeed{Kbps: -1}
	if url == "" {
		return st
	}
	ctx, cancel := context.WithTimeout(context.Background(), ms)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return st
	}
	req.Header.Set("User-Agent", httpc.UA)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Range", "bytes=0-"+strconv.Itoa(limit-1))
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	t0 := time.Now()
	res, err := client.Do(req)
	if err != nil {
		return st
	}
	defer res.Body.Close()
	buf := make([]byte, 32*1024)
	bytes := 0
	for bytes < limit {
		n, err := res.Body.Read(buf)
		bytes += n
		if err != nil {
			break
		}
	}
	st.Bytes = bytes
	st.MS = int(time.Since(t0).Milliseconds())
	if st.MS > 150 && bytes > 16384 {
		st.Kbps = bytes * 8 / st.MS
	}
	return st
}

func FmtKbps(kbps int) string {
	if kbps <= 0 {
		return ""
	}
	if kbps >= 1000 {
		return strconv.Itoa(kbps/100) + "." + strconv.Itoa(kbps%100/10) + " MB/s"
	}
	return strconv.Itoa(kbps) + " KB/s"
}

// ---------- 内部 ----------

func toNum(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

func numAttr(line, key string) int {
	return toNum(strAttr(line, key))
}

func strAttr(line, key string) string {
	i := strings.Index(line, key+"=")
	if i < 0 {
		return ""
	}
	s := i + len(key) + 1
	e := len(line)
	if s < len(line) && line[s] == '"' {
		s++
		if q := strings.Index(line[s:], `"`); q >= 0 {
			return line[s : s+q]
		}
		return line[s:]
	}
	if comma := strings.Index(line[s:], ","); comma >= 0 {
		e = s + comma
	}
	return strings.TrimSpace(line[s:e])
}

func abs(base, ref string) string {
	r := strings.TrimSpace(ref)
	if strings.HasPrefix(r, "http://") || strings.HasPrefix(r, "https://") {
		return r
	}
	scheme := strings.Index(base, "://")
	if scheme < 0 {
		return r
	}
	hostEnd := strings.Index(base[scheme+3:], "/")
	origin := base
	if hostEnd >= 0 {
		origin = base[:scheme+3+hostEnd]
	}
	if strings.HasPrefix(r, "//") {
		return base[:scheme] + ":" + r
	}
	if strings.HasPrefix(r, "/") {
		return origin + r
	}
	cut := strings.Index(base, "?")
	clean := base
	if cut >= 0 {
		clean = base[:cut]
	}
	dir := clean[:strings.LastIndex(clean, "/")+1]
	path := dir + r
	rest := path
	if len(path) > len(origin) {
		rest = path[len(origin):]
	}
	parts := strings.Split(rest, "/")
	var stack []string
	for _, p := range parts {
		if p == ".." {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		} else if p != "." && p != "" {
			stack = append(stack, p)
		}
	}
	return origin + "/" + strings.Join(stack, "/")
}

// 供播放器用：从 m3u8 文本里取第一个媒体/分片地址做测速。
func SegOfPlaylist(pl, base string) string { return FirstSegment(pl, base) }

var _ = io.EOF
