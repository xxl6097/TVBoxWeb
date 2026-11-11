package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

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
