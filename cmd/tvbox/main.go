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
	"strings"

	tvbox "github.com/xxl6097/TVBoxWeb"
	"github.com/xxl6097/TVBoxWeb/githubupgrade"
)

func main() {
	addr := flag.String("addr", ":8090", "监听地址")
	dataDir := flag.String("data", "data", "数据目录")
	token := flag.String("token", "", "访问口令（公网部署强烈建议设置；留空则不鉴权）")
	user := flag.String("user", "", "登录用户名（与 -pass 同时设置才会启用登录页）")
	pass := flag.String("pass", "", "登录密码（与 -user 同时设置才会启用登录页）")
	secureCookie := flag.Bool("secure-cookie", false, "会话 Cookie 加 Secure 标记（HTTPS 部署时建议开启；HTTP 访问下开启会导致登录失效）")
	origins := flag.String("origins", "", "额外允许的 CORS 来源，逗号分隔")
	allowPrivate := flag.Bool("allow-private", false, "允许请求内网上游（家用 NAS / 内网订阅源）")
	hosts := flag.String("hosts", "", "上游域名白名单，逗号分隔（留空不限制）")
	flag.Parse()

	// 环境变量优先于命令行之外的默认值：容器部署时把口令放 env 更常见
	envOr := func(key, def string) string {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
		return def
	}

	split := func(v string) []string {
		var out []string
		for _, x := range strings.Split(v, ",") {
			if x = strings.TrimSpace(x); x != "" {
				out = append(out, x)
			}
		}
		return out
	}
	opts := []tvbox.Option{
		tvbox.WithDataDir(*dataDir),
		tvbox.WithAuthToken(*token),
		tvbox.WithLogin(envOr("YZ_USER", *user), envOr("YZ_PASS", *pass)),
		tvbox.WithCookieSecure(*secureCookie),
		tvbox.WithAllowedOrigins(split(*origins)...),
		tvbox.WithAllowPrivateUpstreams(*allowPrivate),
		tvbox.WithAllowedHosts(split(*hosts)...),
	}

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
