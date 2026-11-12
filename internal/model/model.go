// Package model 定义数据模型与界面常量，与鸿蒙影视 Model.ets 一一对应。
package model

import (
	"strconv"
	"strings"
)

// FlexInt 兼容「数字 / 字符串数字」两种 JSON 取值（采集站常把 limit 等写成字符串）。
type FlexInt int

func (f *FlexInt) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	s = strings.Trim(s, `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	if n, err := strconv.Atoi(s); err == nil {
		*f = FlexInt(n)
		return nil
	}
	if fl, err := strconv.ParseFloat(s, 64); err == nil {
		*f = FlexInt(fl)
		return nil
	}
	return nil
}

func (f FlexInt) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Itoa(int(f))), nil
}

// ============ TVBox 配置 JSON 原始结构 ============

type RawSite struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Type        int    `json:"type"`
	API         string `json:"api"`
	Searchable  int    `json:"searchable"`
	QuickSearch int    `json:"quickSearch"`
	Filterable  int    `json:"filterable"`
	Ext         any    `json:"ext"` // 可能是字符串或对象
	Jar         string `json:"jar"`
}

type RawParse struct {
	Name string      `json:"name"`
	Type int         `json:"type"`
	URL  string      `json:"url"`
	Ext  RawParseExt `json:"ext"`
}

type RawParseExt struct {
	Flag []string `json:"flag"`
}

type RawLive struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type RawConfig struct {
	Sites   []RawSite  `json:"sites"`
	Parses  []RawParse `json:"parses"`
	Lives   []RawLive  `json:"lives"`
	Name    string     `json:"name"`
	Updated string     `json:"updated"`
	Spider  string     `json:"spider"`
	Flags   []string   `json:"flags"`
	Ads     []string   `json:"ads"`
}

// ============ 接口返回 JSON 结构（已把 "class" 预处理成 catList） ============

type JVod struct {
	VodID       FlexInt `json:"vod_id"`
	VodName     string  `json:"vod_name"`
	VodPic      string  `json:"vod_pic"`
	VodRemarks  string  `json:"vod_remarks"`
	VodYear     string  `json:"vod_year"`
	VodArea     string  `json:"vod_area"`
	VodActor    string  `json:"vod_actor"`
	VodDirector string  `json:"vod_director"`
	VodContent  string  `json:"vod_content"`
	VodBlurb    string  `json:"vod_blurb"`
	VodPlayFrom string  `json:"vod_play_from"`
	VodPlayURL  string  `json:"vod_play_url"`
	VodTime     string  `json:"vod_time"`
	TypeName    string  `json:"type_name"`
	Tid         FlexInt `json:"tid"`
}

type JClass struct {
	TypeID   FlexInt `json:"type_id"`
	TypePid  FlexInt `json:"type_pid"`
	TypeName string  `json:"type_name"`
}

type JResp struct {
	Code      FlexInt  `json:"code"`
	Msg       string   `json:"msg"`
	Page      FlexInt  `json:"page"`
	PageCount FlexInt  `json:"pagecount"`
	Total     FlexInt  `json:"total"`
	Limit     FlexInt  `json:"limit"`
	List      []JVod   `json:"list"`
	CatList   []JClass `json:"catList"`
	Data      *JResp   `json:"data"`
}

// ============ 应用模型 ============

type Site struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Type        int    `json:"type"`
	API         string `json:"api"`
	Searchable  bool   `json:"searchable"`
	QuickSearch bool   `json:"quickSearch"`
	Ext         string `json:"ext"`
	Usable      bool   `json:"usable"`
	Reason      string `json:"reason"`
	Enabled     bool   `json:"enabled"`
}

type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	PID  string `json:"pid"`
}

type Vod struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Pic      string `json:"pic"`
	Remark   string `json:"remark"`
	Year     string `json:"year"`
	Area     string `json:"area"`
	Actor    string `json:"actor"`
	Director string `json:"director"`
	Content  string `json:"content"`
	TypeName string `json:"typeName"`
	From     string `json:"from"`
	PlayURL  string `json:"playUrl"`
	SiteKey  string `json:"siteKey"`
	SiteName string `json:"siteName"`
}

type Episode struct {
	Name string   `json:"name"`
	URL  string   `json:"url"`
	URLs []string `json:"urls"` // 直播专用：同一频道的备用线路
}

type Line struct {
	Name   string    `json:"name"`
	Direct bool      `json:"direct"`
	Eps    []Episode `json:"eps"`
}

type VodPage struct {
	Items     []Vod `json:"items"`
	Page      int   `json:"page"`
	PageCount int   `json:"pageCount"`
	Total     int   `json:"total"`
}

type ParseRule struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// ============ 筛选 ============

type FilterValue struct {
	N string `json:"n"`
	V string `json:"v"`
}

type FilterGroup struct {
	Key    string        `json:"key"`
	Name   string        `json:"name"`
	Values []FilterValue `json:"values"`
}

// ============ 直播 ============

type Channel struct {
	Name  string   `json:"name"`
	URL   string   `json:"url"`
	Logo  string   `json:"logo"`
	Group string   `json:"group"`
	URLs  []string `json:"urls"`
}

type ChannelGroup struct {
	Name     string    `json:"name"`
	Channels []Channel `json:"channels"`
}

type LiveDir struct {
	Name     string    `json:"name"`
	Count    int       `json:"count"`
	Children []LiveDir `json:"children"`
	Channels []Channel `json:"channels"`
}

// ============ WebDAV ============

type WebdavSource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
	User string `json:"user"`
	Pass string `json:"pass"`
	Root string `json:"root"`
}

type WebdavEntry struct {
	Href  string `json:"href"`
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
	Mtime string `json:"mtime"`
}

type WebdavVod struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // movie / series
	Year string `json:"year"`
	// Dir 是条目所在的绝对目录（URL 路径，已保留原有编码）。
	Dir string `json:"dir"`
	// RelDir 是相对「网盘挂载根」的目录路径（已 URL 解码），如「凡人/01 风起天南」。
	// 前端用它还原目录层级分组，避免把子目录里的内容一路平铺。
	// 直接放在挂载根下的条目为空串。
	RelDir string     `json:"relDir"`
	Eps    []WebdavEp `json:"eps"`
}

type WebdavEp struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Href    string `json:"href"`
	Season  int    `json:"season"`
	Episode int    `json:"episode"`
}

type WdHistory struct {
	SrcID   string `json:"srcId"`
	SrcName string `json:"srcName"`
	VodName string `json:"vodName"`
	Dir     string `json:"dir"`
	EpIndex int    `json:"epIndex"`
	EpName  string `json:"epName"`
	URL     string `json:"url"`
	Pos     int64  `json:"pos"`
	Dur     int64  `json:"dur"`
	Time    int64  `json:"time"`
}

// ============ 界面常量（来自 FongMi/TV leanback 资源） ============

var T = struct {
	BG        string
	SURFACE   string
	BLACK20   string
	BLACK40   string
	BLACK50   string
	BLACK70   string
	BLACK80   string
	BLACK90   string
	WHITE15   string
	WHITE30   string
	WHITE70   string
	WHITE80   string
	PRIMARY   string
	SELECTED  string
	CARD      string
	CARDHI    string
	ACCENT    string
	ACCENTDIM string
	TXT       string
	SUB       string
	DIM       string
	WARN      string
	ERR       string
	LIVE      string
	ANIM      int
	BORDER    int
	RADIUS    int
	RADIUSSM  int
	RADIUSPIL int
	HOVERSCAL float64
}{
	BG:        "#141218",
	SURFACE:   "#1C1B1F",
	BLACK20:   "#33000000",
	BLACK40:   "#66000000",
	BLACK50:   "#80000000",
	BLACK70:   "#B3000000",
	BLACK80:   "#CC000000",
	BLACK90:   "#E6000000",
	WHITE15:   "#26FFFFFF",
	WHITE30:   "#4DFFFFFF",
	WHITE70:   "#B3FFFFFF",
	WHITE80:   "#CCFFFFFF",
	PRIMARY:   "#FFFFFF",
	SELECTED:  "#FFEB3B",
	CARD:      "#33000000",
	CARDHI:    "#66000000",
	ACCENT:    "#FFFFFF",
	ACCENTDIM: "#4DFFFFFF",
	TXT:       "#FFFFFF",
	SUB:       "#B3FFFFFF",
	DIM:       "#80FFFFFF",
	WARN:      "#FFB4AB",
	ERR:       "#FFB4AB",
	LIVE:      "#F44336",
	ANIM:      140,
	BORDER:    2,
	RADIUS:    8,
	RADIUSSM:  4,
	RADIUSPIL: 28,
	HOVERSCAL: 1.1,
}
