//go:build windows

package api

import "syscall"

// sysProcAttrGroup Windows 下没有进程组语义，返回空即可；
// CommandContext 取消时仍会 TerminateProcess 杀掉 ffmpeg 本身。
func sysProcAttrGroup() *syscall.SysProcAttr {
	return nil
}
