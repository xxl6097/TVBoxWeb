// Package metrics 实现运行时性能指标收集，供监控仪表板展示。
package metrics

import (
	"runtime"
	"sort"
	"sync"
	"time"
)

// Metrics 收集运行时指标。
type Metrics struct {
	mu sync.RWMutex

	startTime time.Time

	// 请求统计
	totalRequests int64
	requestsByPath map[string]int64

	// 响应时间
	totalLatencyMs int64   // 累计延迟（毫秒）
	recentLatency  []int64 // 最近 100 次延迟（环形缓冲近似）

	// 缓存
	cacheHits int64

	// 错误
	totalErrors int64
}

var global = &Metrics{
	startTime:      time.Now(),
	requestsByPath: make(map[string]int64),
	recentLatency:  make([]int64, 0, 100),
}

// Record 记录一次请求。
func Record(path string, latencyMs int64) {
	global.mu.Lock()
	defer global.mu.Unlock()

	global.totalRequests++
	global.requestsByPath[path]++
	global.totalLatencyMs += latencyMs

	// 保留最近 100 次延迟
	global.recentLatency = append(global.recentLatency, latencyMs)
	if len(global.recentLatency) > 100 {
		global.recentLatency = global.recentLatency[len(global.recentLatency)-100:]
	}
}

// RecordCacheHit 记录一次缓存命中（ETag 304）。
func RecordCacheHit() {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.cacheHits++
}

// RecordError 记录一次错误。
func RecordError() {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.totalErrors++
}

// SetWSConnections 由 main/ws 设置 WebSocket 连接数（避免循环依赖，用回调注入）。
var wsConnections func() int

// SetWSConnectionGetter 注入 WebSocket 连接数获取器。
func SetWSConnectionGetter(getter func() int) {
	wsConnections = getter
}

// Snapshot 返回当前指标快照。
func Snapshot() map[string]any {
	global.mu.RLock()
	defer global.mu.RUnlock()

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	// 计算平均响应时间
	avgMs := int64(0)
	if global.totalRequests > 0 {
		avgMs = global.totalLatencyMs / global.totalRequests
	}

	// 计算最近延迟（p50/p95）
	p50, p95 := percentile(global.recentLatency)

	// 缓存命中率
	cacheRate := 0.0
	if global.totalRequests > 0 {
		cacheRate = float64(global.cacheHits) / float64(global.totalRequests)
	}

	// 请求路径 Top 10
	paths := topPaths(global.requestsByPath, 10)

	// WebSocket 连接数
	wsCount := 0
	if wsConnections != nil {
		wsCount = wsConnections()
	}

	return map[string]any{
		"uptime":    int64(time.Since(global.startTime).Seconds()),
		"requests": map[string]any{
			"total":   global.totalRequests,
			"errors":  global.totalErrors,
			"avgMs":   avgMs,
			"p50Ms":   p50,
			"p95Ms":   p95,
		},
		"cache": map[string]any{
			"hits":     global.cacheHits,
			"hitRate":  cacheRate,
		},
		"memory": map[string]any{
			"allocMb":       int64(mem.Alloc / 1024 / 1024),
			"totalAllocMb":  int64(mem.TotalAlloc / 1024 / 1024),
			"sysMb":         int64(mem.Sys / 1024 / 1024),
			"heapObjects":   int64(mem.HeapObjects),
			"gcCycles":      int64(mem.NumGC),
		},
		"runtime": map[string]any{
			"goroutines": runtime.NumGoroutine(),
			"cpus":       runtime.NumCPU(),
		},
		"websocket": map[string]any{
			"connections": wsCount,
		},
		"topPaths": paths,
	}
}

// percentile 计算分位数（p50/p95）。
func percentile(latencies []int64) (int64, int64) {
	if len(latencies) == 0 {
		return 0, 0
	}
	sorted := make([]int64, len(latencies))
	copy(sorted, latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	p50 := sorted[len(sorted)/2]
	idx := int(float64(len(sorted)) * 0.95)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	p95 := sorted[idx]
	return p50, p95
}

// topPaths 返回请求次数最多的路径。
func topPaths(m map[string]int64, n int) []map[string]any {
	type kv struct {
		path  string
		count int64
	}
	pairs := make([]kv, 0, len(m))
	for p, c := range m {
		pairs = append(pairs, kv{p, c})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].count > pairs[j].count })

	out := make([]map[string]any, 0, n)
	for i := 0; i < len(pairs) && i < n; i++ {
		out = append(out, map[string]any{
			"path":  pairs[i].path,
			"count": pairs[i].count,
		})
	}
	return out
}
