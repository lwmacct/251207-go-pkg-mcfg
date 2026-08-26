package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/lwmacct/251207-go-pkg-cfgm/pkg/cfgm"
)

type config struct {
	Name    string        `json:"name"    desc:"应用名称"`
	Timeout time.Duration `json:"timeout" desc:"请求超时"`
	Service struct {
		URL   string `json:"url"   desc:"服务地址"`
		Token string `json:"token" desc:"访问令牌"`
	} `json:"service" desc:"服务配置"`
}

func defaultConfig() config {
	var defaults config
	defaults.Name = "example"
	defaults.Timeout = 30 * time.Second
	defaults.Service.URL = "https://api.example.com"
	return defaults
}

var manager = cfgm.MustNew(defaultConfig(), cfgm.WithoutDefaultPaths())

func main() {
	loaded, err := manager.Load(
		context.Background(),
		cfgm.File("examples/config/config.yaml"),
		cfgm.Env("APP_"),
	)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(
		os.Stdout,
		"name=%s timeout=%s service=%s token_set=%t\n",
		loaded.Name,
		loaded.Timeout,
		loaded.Service.URL,
		loaded.Service.Token != "",
	)
}
