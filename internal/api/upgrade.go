package api

import "context"

// Release 描述一个可用的新版本（升级检测结果）。
type Release struct {
	Version     string `json:"version"`
	Notes       string `json:"notes,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
	PageURL     string `json:"page_url,omitempty"`
	Size        int64  `json:"size,omitempty"`
	FullURL     string `json:"full_url,omitempty"`
	PatchURL    string `json:"patch_url,omitempty"`
}

// UpgradeProgress 是一次升级过程中的进度。
type UpgradeProgress struct {
	Stage  string `json:"stage"`
	Loaded int64  `json:"loaded"`
	Total  int64  `json:"total"`
	Message string `json:"message,omitempty"`
}

// 升级阶段常量。
const (
	StageChecking    = "checking"
	StageDownloading = "downloading"
	StageVerifying   = "verifying"
	StageReplacing   = "replacing"
	StageDone        = "done"
)

// Upgrader 由调用方实现（复刻 gofs.Upgrader）。
type Upgrader interface {
	Check(ctx context.Context) (*Release, error)
	Upgrade(ctx context.Context, rel *Release, onProgress func(UpgradeProgress)) error
}

// upgrader 全局升级实现，由 tvbox 包通过 SetUpgrader 注入。
var upgrader Upgrader

// SetUpgrader 注入升级实现。
func SetUpgrader(u Upgrader) {
	upgrader = u
}
