package tvbox

import (
	"github.com/xxl6097/TVBoxWeb/internal/api"
)

// 升级相关类型与常量：从内部 api 包 re-export，供第三方开发者直接使用。
// 复刻 gofs 库的公开升级接口设计。

// Release 描述一个可用的新版本。
type Release = api.Release

// UpgradeProgress 是一次升级过程中的进度。
type UpgradeProgress = api.UpgradeProgress

// Upgrader 由调用方实现（下载、校验、替换、是否重启由实现方决定）。
type Upgrader = api.Upgrader

// 升级阶段常量。
const (
	StageChecking    = api.StageChecking
	StageDownloading = api.StageDownloading
	StageVerifying   = api.StageVerifying
	StageReplacing   = api.StageReplacing
	StageDone        = api.StageDone
)
