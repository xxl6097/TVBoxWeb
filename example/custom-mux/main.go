// 示例：将夜帧影视接入自定义 http.ServeMux / 任意 http.Handler 框架。
//
// 通过 srv.Handler() 拿到完整的 http.Handler，可以挂到任意路由、
// 套中间件、或嵌入 Gin / Echo 等框架。
//
// 运行：
//
//	go run ./example/custom-mux
//
// 然后打开 http://localhost:8090
package main

import (
	"log"
	"net/http"

	tvbox "github.com/xxl6097/TVBoxWeb"
)

func main() {
	// 1. 创建实例（自定义数据目录）
	srv, err := tvbox.New(tvbox.WithDataDir("./mydata"))
	if err != nil {
		log.Fatal(err)
	}
	defer srv.Close()

	// 2. 拿到完整 Handler（含全部 API + 内嵌前端）
	handler := srv.Handler()

	// 3. 自由组合：加日志中间件、限流、鉴权等
	wrapped := withLogging(handler)

	// 4. 挂到自己的 mux
	mux := http.NewServeMux()
	mux.Handle("/", wrapped)

	log.Println("服务已启动：http://localhost:8090")
	if err := http.ListenAndServe(":8090", mux); err != nil {
		log.Fatal(err)
	}
}

// withLogging 是一个简单的日志中间件示例。
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[%s] %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
