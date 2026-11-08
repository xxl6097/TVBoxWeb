// Package liveprobe 实现直播源测速排行 + 可用性检测，与鸿蒙影视 LiveProbe.ets 一致。
package liveprobe

import (
	"sort"
	"sync"
	"time"

	"tvbox/internal/live"
	"tvbox/internal/model"
)

const (
	Sample    = 3
	StreamMS  = 3000 * time.Millisecond
	ListMS    = 8000 * time.Millisecond
	GuardMS   = 10000 * time.Millisecond
)

type LiveScore struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	OK      bool   `json:"ok"`
	MS      int    `json:"ms"`
	Count   int    `json:"count"`
	Alive   int    `json:"alive"`
	Sampled int    `json:"sampled"`
	Score   int    `json:"score"`
	State   string `json:"state"`
}

// RankOne 测一个源，不抛异常。
func RankOne(name, url string) LiveScore {
	r := LiveScore{Name: name, URL: url, MS: -1}
	t0 := time.Now()
	gs, err := loadWithGuard(url)
	if err != nil {
		r.State = "拉取失败"
		return r
	}
	r.MS = int(time.Since(t0).Milliseconds())
	n := 0
	for _, g := range gs {
		n += len(g.Channels)
	}
	if n == 0 {
		r.State = "列表为空"
		return r
	}
	r.OK = true
	r.Count = n

	// 抽样：先从不同分组各取第一个，不够再补。
	var samples []string
	for i := 0; i < len(gs) && len(samples) < Sample; i++ {
		chs := gs[i].Channels
		if len(chs) > 0 {
			u := chs[0].URL
			if len(chs[0].URLs) > 0 {
				u = chs[0].URLs[0]
			}
			samples = append(samples, u)
		}
	}
	for k := 1; len(samples) < Sample; k++ {
		added := false
		for i := 0; i < len(gs) && len(samples) < Sample; i++ {
			chs := gs[i].Channels
			if k < len(chs) {
				u := chs[k].URL
				if len(chs[k].URLs) > 0 {
					u = chs[k].URLs[0]
				}
				if !containsStr(samples, u) {
					samples = append(samples, u)
					added = true
				}
			}
		}
		if !added {
			break
		}
	}
	r.Sampled = len(samples)

	var wg sync.WaitGroup
	alive := make([]bool, len(samples))
	for i, s := range samples {
		wg.Add(1)
		go func(i int, s string) {
			defer wg.Done()
			alive[i] = live.Probe(s, StreamMS)
		}(i, s)
	}
	wg.Wait()
	for _, a := range alive {
		if a {
			r.Alive++
		}
	}
	r.Score = score(&r)
	r.State = timeStr(r.MS) + " ms · " + itoa(r.Count) + " 台 · 可播 " + itoa(r.Alive) + "/" + itoa(r.Sampled)
	return r
}

func loadWithGuard(url string) ([]model.ChannelGroup, error) {
	type result struct {
		gs  []model.ChannelGroup
		err error
	}
	ch := make(chan result, 1)
	go func() {
		gs, err := live.Load(url, ListMS)
		ch <- result{gs, err}
	}()
	select {
	case r := <-ch:
		return r.gs, r.err
	case <-time.After(GuardMS):
		return nil, errTimeout
	}
}

var errTimeout = &timeoutError{}

type timeoutError struct{}

func (*timeoutError) Error() string { return "探测超时" }

// Score 评分：可播率 70% + 速度 30%。
func score(r *LiveScore) int {
	if !r.OK {
		return 0
	}
	availability := 0.0
	if r.Sampled > 0 {
		availability = float64(r.Alive) / float64(r.Sampled) * 100
	}
	speed := 100 - r.MS/50
	if speed < 0 {
		speed = 0
	}
	return int(availability*0.7 + float64(speed)*0.3 + 0.5)
}

// RankAll 批量测速，每批 3 个（保留兼容，推荐使用 RankAllConcurrent）。
func RankAll(items []LiveScore, onEach func(i int, r LiveScore)) []LiveScore {
	out := make([]LiveScore, len(items))
	for i := range items {
		out[i] = LiveScore{Name: items[i].Name, URL: items[i].URL, MS: -1}
	}
	for s := 0; s < len(out); s += 3 {
		var wg sync.WaitGroup
		end := s + 3
		if end > len(out) {
			end = len(out)
		}
		for i := s; i < end; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				r := RankOne(out[i].Name, out[i].URL)
				out[i] = r
				onEach(i, r)
			}(i)
		}
		wg.Wait()
	}
	return out
}

// RankAllConcurrent 并发批量测速，使用固定 worker pool。
// workers: 并发 worker 数量（建议 5-10），0 则自动设为 8。
// timeout: 整体超时（0 则不限制）。
func RankAllConcurrent(items []LiveScore, workers int, timeout time.Duration, onEach func(i int, r LiveScore)) []LiveScore {
	if workers <= 0 {
		workers = 8
	}
	out := make([]LiveScore, len(items))
	for i := range items {
		out[i] = LiveScore{Name: items[i].Name, URL: items[i].URL, MS: -1}
	}
	
	// 创建任务通道
	type task struct {
		idx  int
		item LiveScore
	}
	tasks := make(chan task, len(items))
	for i, it := range items {
		tasks <- task{idx: i, item: it}
	}
	close(tasks)
	
	// 启动 worker pool
	var wg sync.WaitGroup
	done := make(chan struct{})
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range tasks {
				select {
				case <-done:
					return
				default:
					r := RankOne(t.item.Name, t.item.URL)
					out[t.idx] = r
					if onEach != nil {
						onEach(t.idx, r)
					}
				}
			}
		}()
	}
	
	// 超时控制
	if timeout > 0 {
		go func() {
			time.Sleep(timeout)
			close(done)
		}()
	}
	
	wg.Wait()
	return out
}

// Sort 按评分排序；同分台多在前，再同按耗时。
func Sort(list []LiveScore) []LiveScore {
	out := append([]LiveScore(nil), list...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.OK != b.OK {
			return a.OK
		}
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		return a.MS < b.MS
	})
	return out
}

// Channels 逐个检测频道，返回失效数量。
func Channels(chs []model.Channel, onTick func(ch model.Channel, alive bool), parallel int) int {
	if parallel <= 0 {
		parallel = 4
	}
	dead := 0
	var mu sync.Mutex
	for s := 0; s < len(chs); s += parallel {
		var wg sync.WaitGroup
		end := s + parallel
		if end > len(chs) {
			end = len(chs)
		}
		for i := s; i < end; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				ch := chs[i]
				u := ch.URL
				if len(ch.URLs) > 0 {
					u = ch.URLs[0]
				}
				ok := live.Probe(u, StreamMS)
				if !ok {
					mu.Lock()
					dead++
					mu.Unlock()
				}
				onTick(ch, ok)
			}(i)
		}
		wg.Wait()
	}
	return dead
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}

func timeStr(ms int) string {
	if ms < 0 {
		return "-1"
	}
	return itoa(ms)
}
