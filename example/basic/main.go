// 示例：最简单的用法 —— 一行启动夜帧影视 Web 服务。
//
// 运行：
//
//	go run ./example/basic
//
// 然后打开 http://localhost:8090
package main

import (
	"log"

	tvbox "github.com/xxl6097/TVBoxWeb"
)

func main() {
	// 默认监听 :8090，数据存到 ./data 目录
	if err := tvbox.ListenAndServe(":8090", tvbox.WithDataDir("./data")); err != nil {
		log.Fatal(err)
	}
}
