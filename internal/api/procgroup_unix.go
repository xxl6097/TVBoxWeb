//go:build !windows

package api

import "syscall"

// sysProcAttrGroup 让 ffmpeg 在新进程组里运行。
// 这样取消 context 时可以整组杀掉，避免 ffmpeg 留下孤儿子进程继续占用带宽。
func sysProcAttrGroup() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
