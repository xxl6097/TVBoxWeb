package webdav

import "testing"

func TestCleanDirNameCompat(t *testing.T) {
	cases := map[string]string{
		"01 风起天南":   "风起天南",
		"9 慕兰之战":    "慕兰之战",
		"10-外海风云":   "外海风云",
		"02_再别天南":   "再别天南",
		"03.初入星海":   "初入星海",
		"凡人":       "凡人",
		"2024 某电影": "2024 某电影", // 四位年份保留（不当序号）
		"7":        "7",          // 纯数字无分隔符：保留（可能就是剧名）
	}
	for in, want := range cases {
		if got := cleanDirName(in); got != want {
			t.Errorf("cleanDirName(%q) = %q，期望 %q", in, got, want)
		}
	}
}
