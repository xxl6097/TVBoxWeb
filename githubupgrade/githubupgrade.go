// Package githubupgrade 提供基于 GitHub Release 的升级功能（兼容 go-fs 的 API）
package githubupgrade

import (
	"github.com/xxl6097/TVBoxWeb/internal/api"
	"github.com/xxl6097/TVBoxWeb/internal/upgrade"
)

// Options 升级器选项
type Options struct {
	Owner string // GitHub 仓库所有者
	Repo  string // GitHub 仓库名称
}

// Upgrader 升级器（暴露给外部）
type Upgrader = upgrade.Upgrader

// New 创建一个 GitHub 升级器
func New(opts Options) (Upgrader, error) {
	u, err := upgrade.NewGitHubUpgrader(opts.Owner, opts.Repo, api.AppVersion)
	if err != nil {
		return nil, err
	}

	// 设置到 API 包中，供 HTTP 处理器使用
	api.SetUpgrader(u)

	return u, nil
}
