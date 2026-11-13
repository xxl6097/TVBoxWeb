package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/xxl6097/TVBoxWeb/internal/model"
)

// initTemp 把 store 指向一个临时文件，避免测试互相污染真实数据。
func initTemp(t *testing.T) {
	t.Helper()
	if err := Init(filepath.Join(t.TempDir(), "store.json")); err != nil {
		t.Fatal(err)
	}
}

func TestAtomicWriteAndBackup(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "store.json")
	if err := atomicWrite(dst, []byte(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dst)
	if err != nil || string(b) != `{"a":1}` {
		t.Fatalf("首次写入内容不符: %q, err=%v", b, err)
	}
	if err := atomicWrite(dst, []byte(`{"a":2}`)); err != nil {
		t.Fatal(err)
	}
	bak, err := os.ReadFile(dst + ".bak")
	if err != nil || string(bak) != `{"a":1}` {
		t.Fatalf("覆盖前应保留上一版备份: %q, err=%v", bak, err)
	}
	// 不应残留临时文件
	files, _ := filepath.Glob(filepath.Join(dir, ".store-*.tmp"))
	if len(files) != 0 {
		t.Fatalf("原子写后残留临时文件: %v", files)
	}
	if st, err := os.Stat(dst); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("数据文件权限应为 0600, got %v, err=%v", st, err)
	}
}

func TestInitRecoversFromCorruptFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "store.json")
	if err := os.WriteFile(p, []byte(`{"broken":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p+".bak", []byte(`{"epgUrl":"https://backup.example.com/e.xml"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Init(p); err != nil {
		t.Fatalf("损坏的主文件应能从 .bak 自动恢复: %v", err)
	}
	if got := EpgURL(); got != "https://backup.example.com/e.xml" {
		t.Fatalf("恢复后数据不符: %q", got)
	}
}

func TestSnapshotRestoreReset(t *testing.T) {
	dir := t.TempDir()
	if err := Init(filepath.Join(dir, "store.json")); err != nil {
		t.Fatal(err)
	}
	SetEpgURL("https://epg.example.com/a.xml")
	snap, err := Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(snap), "epg.example.com") {
		t.Fatal("备份应包含刚写入的 EPG 地址")
	}

	Reset()
	after, _ := Snapshot()
	if strings.Contains(string(after), "epg.example.com") {
		t.Fatal("恢复出厂后旧数据应被清空")
	}
	if err := Restore(snap); err != nil {
		t.Fatal(err)
	}
	if got := EpgURL(); got != "https://epg.example.com/a.xml" {
		t.Fatalf("导入备份后数据不符: %q", got)
	}
	if err := Restore([]byte("not json")); err == nil {
		t.Fatal("非法备份文件应被拒绝")
	}
}

func TestConcurrentFlushDoesNotCorrupt(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "store.json")
	if err := Init(p); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			SetEpgURL(fmt.Sprintf("https://e%d.example.com/e.xml", n))
		}(i)
	}
	wg.Wait()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var d Data
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatalf("并发落盘后 store.json 损坏: %v", err)
	}
}

// ---------- 历史索引 ----------

// verifyIndexes 校验索引与切片完全一致（测试内部使用，调用方须持锁）。
func verifyIndexes(t *testing.T) {
	t.Helper()
	mu.RLock()
	defer mu.RUnlock()
	if len(histIdx) != len(data.Hist) {
		t.Fatalf("点播历史索引条目 %d != 切片长度 %d", len(histIdx), len(data.Hist))
	}
	for k, i := range histIdx {
		if i < 0 || i >= len(data.Hist) {
			t.Fatalf("点播历史索引 %q 下标越界: %d", k, i)
		}
		if histKey(data.Hist[i].SiteKey, data.Hist[i].VodID) != k {
			t.Fatalf("点播历史索引 %q 指向了别的记录 %q", k, histKey(data.Hist[i].SiteKey, data.Hist[i].VodID))
		}
	}
	if len(wdIdx) != len(data.WdHist) {
		t.Fatalf("网盘历史索引条目 %d != 切片长度 %d", len(wdIdx), len(data.WdHist))
	}
	for k, i := range wdIdx {
		if i < 0 || i >= len(data.WdHist) {
			t.Fatalf("网盘历史索引 %q 下标越界: %d", k, i)
		}
		if wdKey(data.WdHist[i].SrcID, data.WdHist[i].VodName) != k {
			t.Fatalf("网盘历史索引 %q 指向了别的记录 %q", k, wdKey(data.WdHist[i].SrcID, data.WdHist[i].VodName))
		}
	}
}

func TestHistoryIndexStaysConsistent(t *testing.T) {
	initTemp(t)
	ClearHistory()

	// 反复覆盖同一部片：索引必须始终只有一条
	for i := 0; i < 6; i++ {
		UpdateHistoryPos(HistItem{SiteKey: "s1", VodID: "v1", Name: "片名", EpIndex: i, Pos: int64(i)})
		verifyIndexes(t)
	}
	if got := len(History()); got != 1 {
		t.Fatalf("同一部片应只有 1 条历史，实际 %d 条", got)
	}

	// 多部片：索引命中的位置必须能取到正确记录
	for _, id := range []string{"v2", "v3", "v4"} {
		UpdateHistoryPos(HistItem{SiteKey: "s1", VodID: id, Name: id})
	}
	verifyIndexes(t)
	for _, id := range []string{"v1", "v2", "v3", "v4"} {
		h, ok := FindHistory("s1", id)
		if !ok {
			t.Fatalf("FindHistory 没找到 %s", id)
		}
		if h.VodID != id {
			t.Fatalf("FindHistory(%s) 返回了 %s", id, h.VodID)
		}
	}
	if _, ok := FindHistory("s1", "不存在"); ok {
		t.Fatal("不存在的影片不应命中")
	}

	// 删除中间一条后索引不能错位
	RemoveHistory(HistItem{SiteKey: "s1", VodID: "v2"})
	verifyIndexes(t)
	if _, ok := FindHistory("s1", "v2"); ok {
		t.Fatal("已删除的记录仍能命中")
	}
	if h, ok := FindHistory("s1", "v3"); !ok || h.VodID != "v3" {
		t.Fatal("删除后索引错位，v3 查不到了")
	}

	// 历史上限 40 条
	for i := 0; i < 60; i++ {
		PushHistory(HistItem{SiteKey: "s2", VodID: "p" + itoa(i), Name: "批量"})
	}
	if n := len(History()); n > 40 {
		t.Fatalf("历史应被限制在 40 条，实际 %d", n)
	}
	verifyIndexes(t)
}

func TestWdHistoryIndexStaysConsistent(t *testing.T) {
	initTemp(t)
	ClearWdHistory()

	for i := 0; i < 30; i++ {
		UpdateWdHistory(model.WdHistory{SrcID: "nas", VodName: "片" + itoa(i), Pos: int64(i)})
	}
	verifyIndexes(t)

	// 同一部片更新 5 次，不应产生重复记录
	for i := 0; i < 5; i++ {
		UpdateWdHistory(model.WdHistory{SrcID: "nas", VodName: "片7", Pos: int64(100 + i)})
		verifyIndexes(t)
	}
	if n := len(WdHistory()); n != 30 {
		t.Fatalf("更新不应新增记录，实际 %d 条", n)
	}
	h, ok := FindWdHistory("nas", "片7")
	if !ok || h.Pos != 104 {
		t.Fatalf("FindWdHistory 没拿到最新进度：%+v ok=%v", h, ok)
	}

	RemoveWdHistory(model.WdHistory{SrcID: "nas", VodName: "片7"})
	verifyIndexes(t)
	if _, ok := FindWdHistory("nas", "片7"); ok {
		t.Fatal("删除后仍能命中")
	}

	// 并发写入后索引仍需一致
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			UpdateWdHistory(model.WdHistory{SrcID: "nas", VodName: "并发" + itoa(n%4), Pos: int64(n)})
			UpdateHistoryPos(HistItem{SiteKey: "s", VodID: "c" + itoa(n%4), Pos: int64(n)})
		}(i)
	}
	wg.Wait()
	verifyIndexes(t)
}

// 片头片尾跳过必须按剧隔离：这是用户明确提的需求 ——
// 不同剧的片头长度差很多，一套全局值套所有剧必然错位。
func TestSkipCfgIsolatedPerVod(t *testing.T) {
	initTemp(t)

	// 全局默认设成 60/90，作为新剧的初始值
	SetIntroSkipSec(60)
	SetOutroSkipSec(90)

	// 还没单独设置过：拿到全局默认，且 own=false
	cfg, own := FindSkipCfg("siteA", "vod1")
	if own || cfg.Intro != 60 || cfg.Outro != 90 {
		t.Fatalf("未单独设置时应回落到全局默认，got %+v own=%v", cfg, own)
	}

	// 给剧 A 设成 15/30
	SetSkipCfg("siteA", "vod1", 15, 30)
	cfg, own = FindSkipCfg("siteA", "vod1")
	if !own || cfg.Intro != 15 || cfg.Outro != 30 {
		t.Fatalf("剧 A 应拿到自己的设置，got %+v own=%v", cfg, own)
	}

	// 剧 B 不受影响，仍是全局默认
	cfgB, ownB := FindSkipCfg("siteA", "vod2")
	if ownB || cfgB.Intro != 60 || cfgB.Outro != 90 {
		t.Fatalf("剧 B 不该被剧 A 的设置影响，got %+v own=%v", cfgB, ownB)
	}

	// 全局值本身没被按剧写入改掉
	if IntroSkipSec() != 60 || OutroSkipSec() != 90 {
		t.Fatalf("按剧写入不应改全局值，got %d/%d", IntroSkipSec(), OutroSkipSec())
	}

	// 同 siteKey 不同 vodId、同 vodId 不同 siteKey 都算不同剧
	SetSkipCfg("siteB", "vod1", 5, 8)
	c1, _ := FindSkipCfg("siteA", "vod1")
	c2, _ := FindSkipCfg("siteB", "vod1")
	if c1.Intro != 15 || c2.Intro != 5 {
		t.Fatalf("key 必须包含 siteKey 与 vodId，got A=%d B=%d", c1.Intro, c2.Intro)
	}
}

func TestSkipCfgFallbackOnZero(t *testing.T) {
	initTemp(t)
	SetIntroSkipSec(40)
	SetOutroSkipSec(70)
	SetSkipCfg("s", "v", 20, 30)
	if SkipCfgCount() != 1 {
		t.Fatalf("应记录 1 部剧，got %d", SkipCfgCount())
	}
	// 两个都置 0：删记录、回落全局默认
	SetSkipCfg("s", "v", 0, 0)
	if SkipCfgCount() != 0 {
		t.Fatalf("全 0 时应删掉记录，got %d", SkipCfgCount())
	}
	cfg, own := FindSkipCfg("s", "v")
	if own || cfg.Intro != 40 || cfg.Outro != 70 {
		t.Fatalf("应回落到全局默认 40/70，got %+v own=%v", cfg, own)
	}
}

func TestSkipCfgEmptyKeyIgnored(t *testing.T) {
	initTemp(t)
	SetIntroSkipSec(10)
	// 任一维为空都不该写入，避免产生 "|" 这种脏 key
	SetSkipCfg("", "v", 5, 5)
	SetSkipCfg("s", "", 5, 5)
	SetSkipCfg("", "", 5, 5)
	if SkipCfgCount() != 0 {
		t.Fatalf("空 key 不应写入，got %d", SkipCfgCount())
	}
	cfg, own := FindSkipCfg("", "")
	if own || cfg.Intro != 10 {
		t.Fatalf("空 key 查询应回落到全局，got %+v own=%v", cfg, own)
	}
}

func TestSkipCfgPersisted(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "store.json")
	if err := Init(file); err != nil {
		t.Fatal(err)
	}
	SetSkipCfg("s", "v", 12, 34)
	// 重新 Init 模拟重启
	if err := Init(file); err != nil {
		t.Fatal(err)
	}
	cfg, own := FindSkipCfg("s", "v")
	if !own || cfg.Intro != 12 || cfg.Outro != 34 {
		t.Fatalf("重启后应仍在，got %+v own=%v", cfg, own)
	}
}
