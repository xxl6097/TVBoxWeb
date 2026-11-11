// Package live 的 EPG 节目单支持：拉取 XMLTV（xmltv.dtd 常见格式）、按频道名匹配、内存缓存。
package live

import (
	"context"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/xxl6097/TVBoxWeb/internal/httpc"
)

// EpgProgram 一条节目单记录，时间统一为毫秒时间戳。
type EpgProgram struct {
	Start int64  `json:"start"`
	Stop  int64  `json:"stop"`
	Title string `json:"title"`
	Desc  string `json:"desc"`
}

// EpgResult 某频道的节目单（含正在播出与下一档）。
type EpgResult struct {
	Channel  string       `json:"channel"`
	Programs []EpgProgram `json:"programs"`
	Now      *EpgProgram  `json:"now"`
	Next     *EpgProgram  `json:"next"`
}

// xmltv 只取需要的字段，忽略其余扩展属性。
type xmltv struct {
	Channels []struct {
		ID    string   `xml:"id,attr"`
		Names []string `xml:"display-name"`
	} `xml:"channel"`
	Programmes []struct {
		Start   string `xml:"start,attr"`
		Stop    string `xml:"stop,attr"`
		Channel string `xml:"channel,attr"`
		Title   string `xml:"title"`
		Desc    string `xml:"desc"`
	} `xml:"programme"`
}

type epgCacheItem struct {
	at   time.Time
	body string
}

var (
	epgMu    sync.Mutex
	epgCache = map[string]epgCacheItem{}
)

const epgTTL = 30 * time.Minute

// FetchEPG 拉取并解析节目单，按频道名做宽松匹配（忽略大小写、空格、常见清晰度后缀）。
func FetchEPG(epgURL, channel string) (EpgResult, error) {
	return FetchEPGCtx(context.Background(), epgURL, channel)
}

// FetchEPGCtx 与 FetchEPG 相同，ctx 可取消（命中缓存时不发请求）。
func FetchEPGCtx(ctx context.Context, epgURL, channel string) (EpgResult, error) {
	out := EpgResult{Channel: channel, Programs: []EpgProgram{}}
	if strings.TrimSpace(epgURL) == "" {
		return out, fmt.Errorf("尚未配置 EPG 节目单地址")
	}
	if strings.TrimSpace(channel) == "" {
		return out, fmt.Errorf("缺少频道名称")
	}

	body, err := fetchEPGBody(ctx, epgURL)
	if err != nil {
		return out, err
	}
	var tv xmltv
	if err := xml.Unmarshal([]byte(strings.TrimSpace(body)), &tv); err != nil {
		return out, fmt.Errorf("EPG 格式无法解析（请使用 XMLTV 格式）：%v", err)
	}

	// 频道名 → channel id：优先用 display-name 匹配，其次用 id 本身匹配。
	key := normalizeChannel(channel)
	ids := map[string]bool{}
	for _, c := range tv.Channels {
		if channelMatch(normalizeChannel(c.ID), key) {
			ids[c.ID] = true
		}
		for _, n := range c.Names {
			if channelMatch(normalizeChannel(n), key) {
				ids[c.ID] = true
			}
		}
	}

	now := time.Now().UnixMilli()
	for _, p := range tv.Programmes {
		if len(ids) > 0 {
			if !ids[p.Channel] {
				continue
			}
		} else if !channelMatch(normalizeChannel(p.Channel), key) {
			continue
		}
		st := parseEPGTime(p.Start)
		if st == 0 {
			continue
		}
		out.Programs = append(out.Programs, EpgProgram{
			Start: st,
			Stop:  parseEPGTime(p.Stop),
			Title: strings.TrimSpace(p.Title),
			Desc:  strings.TrimSpace(p.Desc),
		})
	}
	sort.Slice(out.Programs, func(i, j int) bool { return out.Programs[i].Start < out.Programs[j].Start })

	for i := range out.Programs {
		p := &out.Programs[i]
		if out.Now == nil && p.Start <= now && (p.Stop == 0 || p.Stop > now) {
			out.Now = p
		}
		if out.Next == nil && p.Start > now {
			out.Next = p
		}
	}
	return out, nil
}

// fetchEPGBody 同一地址 30 分钟内复用，避免每次切台都重新下载整份 EPG。
func fetchEPGBody(ctx context.Context, u string) (string, error) {
	epgMu.Lock()
	if c, ok := epgCache[u]; ok && time.Since(c.at) < epgTTL {
		body := c.body
		epgMu.Unlock()
		return body, nil
	}
	epgMu.Unlock()

	body, err := httpc.TextCtx(ctx, u, nil, 20*time.Second)
	if err != nil {
		return "", err
	}
	epgMu.Lock()
	epgCache[u] = epgCacheItem{at: time.Now(), body: body}
	epgMu.Unlock()
	return body, nil
}

// parseEPGTime 兼容 XMLTV 常见的几种时间写法。
func parseEPGTime(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	layouts := []string{
		"20060102150405 -0700",
		"20060102150405",
		"2006-01-02T15:04:05-07:00",
		"2006-01-02 15:04:05",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UnixMilli()
		}
	}
	return 0
}

func normalizeChannel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer(" ", "", "-", "", "_", "", "：", "", ":", "").Replace(s)
	for _, suf := range []string{"超高清", "高清", "标清", "fhd", "1080p", "720p", "4k", "hd"} {
		s = strings.TrimSuffix(s, suf)
	}
	return s
}

func channelMatch(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return a == b || strings.Contains(a, b) || strings.Contains(b, a)
}
