// 夜帧影视 · 可执行入口
//
// 基于 tvbox 库构建的独立二进制，内嵌前端，开箱即用。
//
// 用法：
//
//	tvbox -addr :8090 -data data
package main

import (
	"flag"
	"fmt"
	"os"

	tvbox "github.com/xxl6097/TVBoxWeb"
)

func main() {
	addr := flag.String("addr", ":8090", "监听地址")
	dataDir := flag.String("data", "data", "数据目录")
	flag.Parse()

	if err := tvbox.ListenAndServe(*addr, tvbox.WithDataDir(*dataDir)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
