// Package resolve 实现播放地址解析（parses type=1），与鸿蒙影视 Resolve.ets 一致。
package resolve

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/xxl6097/TVBoxWeb/internal/cms"
	"github.com/xxl6097/TVBoxWeb/internal/httpc"
	"github.com/xxl6097/TVBoxWeb/internal/model"
)

type Resolved struct {
	URL    string            `json:"url"`
	Header map[string]string `json:"header"`
	Via    string            `json:"via"`
	OK     bool              `json:"ok"`
}

type parseResp struct {
	Code   int               `json:"code"`
	URL    string            `json:"url"`
	Msg    string            `json:"msg"`
	Header map[string]string `json:"header"`
}

// Resolve 解析播放地址：直链直接返回，否则轮询前 8 个 type=1 解析接口。
func Resolve(raw string, rules []model.ParseRule) Resolved {
	return ResolveCtx(context.Background(), raw, rules)
}

// ResolveCtx 与 Resolve 相同，ctx 可取消（逐个尝试解析接口时会及时放弃）。
func ResolveCtx(ctx context.Context, raw string, rules []model.ParseRule) Resolved {
	r := Resolved{URL: raw}
	if raw == "" {
		r.Via = "空地址"
		return r
	}
	if cms.IsDirect(raw) {
		r.Via = "直链"
		r.OK = true
		return r
	}
	limit := len(rules)
	if limit > 8 {
		limit = 8
	}
	for i := 0; i < limit; i++ {
		if ctx.Err() != nil {
			r.Via = "已取消"
			return r
		}
		rule := rules[i]
		cands := []string{rule.URL + raw, rule.URL + urlEncode(raw)}
		for _, cand := range cands {
			txt, err := httpc.TextCtx(ctx, cand, nil, 6*time.Second)
			if err != nil {
				continue
			}
			var j parseResp
			if err := json.Unmarshal([]byte(cms.CleanJSON(txt)), &j); err != nil {
				continue
			}
			u := strings.TrimSpace(j.URL)
			if strings.HasPrefix(u, "http") {
				r.URL = u
				r.Via = rule.Name
				r.OK = true
				if j.Header != nil {
					r.Header = j.Header
				}
				return r
			}
		}
	}
	r.Via = "未解析（可能需要专用解析器）"
	return r
}

func urlEncode(s string) string {
	// 与 JS encodeURIComponent 一致：保留 -_.!~*'()
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '!' || c == '~' || c == '*' || c == '\'' || c == '(' || c == ')' {
			b.WriteByte(c)
		} else {
			b.WriteString(hexEncode(c))
		}
	}
	return b.String()
}

func hexEncode(c byte) string {
	const hex = "0123456789ABCDEF"
	return "%" + string(hex[c>>4]) + string(hex[c&0xF])
}
