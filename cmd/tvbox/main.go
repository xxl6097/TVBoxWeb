// 夜帧影视 · 可执行入口
//
// 基于 tvbox 库构建的独立二进制，内嵌前端，开箱即用。
// 通过 githubupgrade 注入 GitHub Releases 在线升级能力。
//
// 用法：
//
//	tvbox -addr :8090 -data data
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	tvbox "github.com/xxl6097/TVBoxWeb"
	"github.com/xxl6097/TVBoxWeb/githubupgrade"
)

func main() {
	addr := flag.String("addr", ":8090", "监听地址")
	dataDir := flag.String("data", "data", "数据目录")
	flag.Parse()

	opts := []tvbox.Option{tvbox.WithDataDir(*dataDir)}

	// 注入 GitHub Releases 在线升级（复刻 go-fs 的实现方式）。
	// 失败不阻塞启动：升级是可选项。
	if up, err := githubupgrade.New(githubupgrade.Options{
		Owner: "xxl6097",
		Repo:  "TVBoxWeb",
	}); err == nil {
		opts = append(opts, tvbox.WithUpgrader(up))
	} else {
		log.Printf("在线升级未启用: %v", err)
	}

	if err := tvbox.ListenAndServe(*addr, opts...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
