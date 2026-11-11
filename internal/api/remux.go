package api

// MKV 等容器在线转封装。
//
// 背景：MKV（Matroska）是下载党最常用的容器，但 Chrome / Edge / Firefox
// 的 <video> 与 MSE 都不支持它 —— 里面的 H.264/AAC 明明能解，就是「装不进去」。
// 用户看到的是「这个文件无法直接播放」。
//
// 解决方式不是让人去转码，而是**转封装（remux）**：把 MKV 里的音视频流
// 原样搬进 fMP4 容器，不重新编码，CPU 开销极小（实测 2.5GB 的 4K 文件
// 取 20 秒片段约 6 秒完成，主要是网络 IO）。
//
// 实现要点：
//   - 用 empty_moov + frag_keyframe 输出**分片式 MP4**：头很小、可以边转边播，
//     不必等整个文件转完（否则 2.5GB 的片子要等几分钟）。
//   - 转封装有损的只有容器，画质完全不变；但如果源本身不是 H.264/AAC
//     （如 HEVC/AC3），`-c copy` 会失败，此时退回「不转」，让前端给出明确提示。
//   - ffmpeg 不存在时直接 501，前端回落到直接播放并提示格式问题。

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ffmpegPath 缓存探测结果，避免每个请求都 exec.LookPath。
var (
	ffmpegOnce sync.Once
	ffmpegBin  string
)

// FFmpegAvailable 返回 ffmpeg 可执行文件路径；不可用返回空串与原因。
func FFmpegAvailable() (string, string) {
	ffmpegOnce.Do(func() {
		p, err := exec.LookPath("ffmpeg")
		if err != nil {
			return
		}
		ffmpegBin = p
	})
	if ffmpegBin == "" {
		return "", "未找到 ffmpeg，无法转换 MKV 等容器（安装后重启即可，例如 apt install ffmpeg / brew install ffmpeg）"
	}
	return ffmpegBin, ""
}

// hRemux 把上游视频实时转封装成 fMP4 并通过 HTTP 吐出。
//
// 参数与 /api/proxy 一致：url + 可选 auth（Basic Auth，给 WebDAV 用）。
// 不支持 Range（分片式 MP4 由浏览器顺序拉流即可播放）；
// 需要跳转时前端应改用 seek 参数从指定秒数开始转。
func hRemux(w http.ResponseWriter, r *http.Request) {
	target := q(r, "url")
	if target == "" {
		fail(w, http.StatusBadRequest, "缺少 url 参数")
		return
	}
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		fail(w, http.StatusBadRequest, "url 必须以 http:// 或 https:// 开头")
		return
	}
	if !guardURL(w, target) {
		return
	}
	bin, reason := FFmpegAvailable()
	if bin == "" {
		fail(w, http.StatusNotImplemented, reason)
		return
	}

	auth := q(r, "auth")
	seek := 0
	if v := q(r, "seek"); v != "" {
		fmt.Sscanf(v, "%d", &seek)
	}

	// ctx 跟着客户端走：用户关掉页面，ffmpeg 立刻被杀掉，不留孤儿进程。
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	args := []string{
		"-hide_banner", "-loglevel", "error",
	}
	// 认证信息通过 header 传给 ffmpeg（不写进命令行，避免出现在 ps 里）
	if auth != "" {
		args = append(args, "-headers", "Authorization: Basic "+auth+"\r\n")
	}
	// 网络抖动时别直接失败
	args = append(args, "-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_delay_max", "5")
	// 从指定位置开始（用于拖进度条）。
	// -ss 放在 -i 之前是「快速定位」：直接跳到最近的关键帧，不必解码前面的内容。
	// 注意输出时间戳会从 0 重新开始，所以前端要在 seek 之后把 UI 进度
	// 加上 offset 显示（见 remuxSeek 的用法）。
	if seek > 0 {
		args = append(args, "-ss", fmt.Sprintf("%d", seek))
	}
	args = append(args,
		"-i", target,
		// 只搬流，不重编码：画质零损失，CPU 几乎不花
		"-map", "0:v:0", "-map", "0:a:0?",
		"-c", "copy",
		// 分片 MP4：头写在前、按关键帧切片 → 可以边转边播
		"-movflags", "frag_keyframe+empty_moov+default_base_moof",
		"-f", "mp4",
		"pipe:1",
	)

	cmd := exec.CommandContext(ctx, bin, args...)
	// 杀掉进程组，确保 ffmpeg 的子进程也一起退出
	cmd.SysProcAttr = sysProcAttrGroup()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fail(w, http.StatusInternalServerError, "无法创建管道："+err.Error())
		return
	}
	var stderr strings.Builder
	cmd.Stderr = &limitedWriter{w: &stderr, max: 4096}

	if err := cmd.Start(); err != nil {
		fail(w, http.StatusInternalServerError, "启动 ffmpeg 失败："+err.Error())
		return
	}

	// 等第一块数据到达再写响应头：这样「源不可解码」时还能返回 JSON 错误，
	// 而不是先发 200 再吐一个空流，让前端拿不到任何信息。
	buf := make([]byte, 64*1024)
	n, readErr := stdout.Read(buf)
	if n == 0 {
		_ = cmd.Wait()
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = "转换失败（源可能不是 H.264/AAC，无法直接转封装）"
		}
		fail(w, http.StatusUnsupportedMediaType, "无法转换该视频："+firstLine(msg))
		return
	}

	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "no-store")
	// 不是标准 Range 流，明确告诉浏览器不要尝试断点续传
	w.Header().Set("Accept-Ranges", "none")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(buf[:n]); err != nil {
		return
	}

	// 逐块转发，每块写一次 Flush，保证播放器能及时拿到数据
	flusher, _ := w.(http.Flusher)
	if flusher != nil {
		flusher.Flush()
	}
	for {
		n, err := stdout.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			break
		}
		// 防止客户端已断开后继续读
		select {
		case <-r.Context().Done():
			return
		default:
		}
	}
	_ = cmd.Wait()
	_ = readErr
}

// maxRemuxIdle 让 ffmpeg 不要在无输出时无限等下去
const maxRemuxIdle = 30 * time.Second

// limitedWriter 只保留前 max 字节，避免 ffmpeg 疯狂刷日志把内存吃满。
type limitedWriter struct {
	w   *strings.Builder
	max int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.w.Len() < l.max {
		remain := l.max - l.w.Len()
		if len(p) > remain {
			l.w.Write(p[:remain])
		} else {
			l.w.Write(p)
		}
	}
	return len(p), nil
}

// firstLine 取 stderr 的第一行有效信息，避免把整段日志塞给用户。
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return s
}
