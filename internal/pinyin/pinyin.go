// Package pinyin 提供汉字 → 拼音首字母缩写、T9 九宫格映射，
// 供搜索联想做「拼音首字母」猜测，逻辑与鸿蒙影视 Pinyin.ets 一致。
package pinyin

import "sync"

var (
	once sync.Once
	m    map[rune]string
)

func buildMap() map[rune]string {
	mm := make(map[rune]string)
	// 多音字修正优先
	for i := 0; i < len(extraTable); {
		eq := indexOf(extraTable, '=', i)
		if eq < 0 {
			break
		}
		ch := []rune(extraTable[i:eq])[0]
		semi := indexOf(extraTable, ';', eq)
		end := len(extraTable)
		if semi >= 0 {
			end = semi
		}
		mm[ch] = extraTable[eq+1 : end]
		if semi < 0 {
			break
		}
		i = semi + 1
	}
	// 主表
	for i := 0; i < len(mainTable); {
		col := indexOf(mainTable, ':', i)
		if col <= 0 {
			break
		}
		ini := mainTable[i:col]
		semi := indexOf(mainTable, ';', col)
		end := len(mainTable)
		if semi >= 0 {
			end = semi
		}
		for _, ch := range mainTable[col+1 : end] {
			if _, ok := mm[ch]; !ok {
				mm[ch] = ini
			}
		}
		if semi < 0 {
			break
		}
		i = semi + 1
	}
	return mm
}

func indexOf(s string, b byte, from int) int {
	for i := from; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// initialsOf 取单字的首字母候选（1~2 个字母；表外字返回空串）。
func initialsOf(ch rune) string {
	once.Do(func() { m = buildMap() })
	return m[ch]
}

// abbrsOf 生成一个词的所有「拼音首字母缩写」候选。
// 汉字→声母；字母/数字原样小写；空格标点跳过；多音字分叉。
func abbrsOf(word string) []string {
	out := []string{""}
	for _, c := range word {
		var arr []string
		switch {
		case c >= '\u4e00' && c <= '\u9fff':
			inis := initialsOf(c)
			if inis == "" {
				continue // 表外生僻字，跳过
			}
			for _, r := range inis {
				arr = append(arr, string(r))
			}
		case (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9'):
			arr = append(arr, string(c))
		case c >= 'A' && c <= 'Z':
			arr = append(arr, string(c-'A'+'a'))
		default:
			continue // 空格、标点跳过
		}
		var next []string
		for _, a := range out {
			for _, b := range arr {
				next = append(next, a+b)
			}
		}
		out = next
		if len(out) > 8 {
			break // 多音字分叉防膨胀
		}
	}
	return out
}

// AbbrsOf 导出版本：生成一个词的所有拼音首字母缩写候选。
func AbbrsOf(word string) []string {
	return abbrsOf(word)
}

// T9 九宫格：数字键对应的字母组
var t9 = []string{"", "", "abc", "def", "ghi", "jkl", "mno", "pqrs", "tuv", "wxyz"}

// abbrToT9 把拼音首字母缩写映射成 T9 九宫格数字串。
func abbrToT9(abbr string) string {
	out := ""
	for _, c := range abbr {
		switch {
		case c >= 'a' && c <= 'c':
			out += "2"
		case c >= 'd' && c <= 'f':
			out += "3"
		case c >= 'g' && c <= 'i':
			out += "4"
		case c >= 'j' && c <= 'l':
			out += "5"
		case c >= 'm' && c <= 'o':
			out += "6"
		case c >= 'p' && c <= 's':
			out += "7"
		case c >= 't' && c <= 'v':
			out += "8"
		case c >= 'w' && c <= 'z':
			out += "9"
		default:
			out += string(c) // 数字原样
		}
	}
	return out
}

// MatchAbbr 判断候选词 word 是否能被输入的缩写/关键词命中。
// 支持：中文原名、拼音首字母缩写、T9 数字串 三种匹配方式。
func MatchAbbr(word string, query string) bool {
	if word == "" || query == "" {
		return false
	}
	// 直接包含
	if contains(word, query) {
		return true
	}
	abbrs := abbrsOf(word)
	for _, a := range abbrs {
		if contains(a, query) {
			return true
		}
		// T9 数字串匹配
		if isDigit(query) {
			if contains(abbrToT9(a), query) {
				return true
			}
		}
	}
	return false
}

func contains(s, sub string) bool {
	if sub == "" {
		return true
	}
	return indexOfStr(s, sub) >= 0
}

func indexOfStr(s, sub string) int {
	r := []rune(s)
	sb := []rune(sub)
	for i := 0; i+len(sb) <= len(r); i++ {
		if string(r[i:i+len(sb)]) == string(sb) {
			return i
		}
	}
	return -1
}

func isDigit(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// T9Letters 数字键对应的字母组（UI 提示用），如「2」→「ABC」。
func T9Letters(digit string) string {
	if len(digit) == 1 && digit[0] >= '2' && digit[0] <= '9' {
		d := int(digit[0] - '0')
		return toUpper(t9[d])
	}
	return digit
}

func toUpper(s string) string {
	out := make([]rune, len(s))
	for i, c := range s {
		if c >= 'a' && c <= 'z' {
			out[i] = c - 'a' + 'A'
		} else {
			out[i] = c
		}
	}
	return string(out)
}
