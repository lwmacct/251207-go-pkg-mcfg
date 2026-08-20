package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/lwmacct/251207-go-pkg-cfgm/pkg/cfgm"
)

type endpoint struct {
	URL *url.URL
}

func parseEndpoint(value string) (endpoint, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return endpoint{}, fmt.Errorf("parse endpoint: %w", err)
	}
	if parsed.Scheme != "svc" || parsed.Host == "" {
		return endpoint{}, errors.New("endpoint must use svc:// with a host")
	}
	return endpoint{URL: parsed}, nil
}

func (e endpoint) String() string {
	if e.URL == nil {
		return ""
	}
	return e.URL.String()
}

type certificate struct {
	ID          string `json:"id"          desc:"证书标识"`
	Certificate string `json:"certificate" desc:"证书 URI"`
	PrivateKey  string `json:"private-key" desc:"私钥 URI"`
}

type config struct {
	Server struct {
		Addr         string        `json:"addr"         desc:"监听地址"`
		Timeout      time.Duration `json:"timeout"      desc:"请求超时"`
		Tags         []string      `json:"tags"         desc:"服务标签"`
		Certificates []certificate `json:"certificates" desc:"TLS 证书"`
		Upstream     endpoint      `json:"upstream"     desc:"上游服务"`
		Redis        struct {
			URL      string `json:"url"      desc:"Redis URL"`
			Password string `json:"password" desc:"Redis 密码"`
		} `json:"redis" desc:"Redis 配置"`
	} `json:"server" desc:"服务端配置"`
}

func defaultConfig() config {
	var defaults config
	defaults.Server.Addr = ":8080"
	defaults.Server.Timeout = 30 * time.Second
	defaults.Server.Tags = []string{"default"}
	defaults.Server.Certificates = []certificate{{
		ID:          "default",
		Certificate: "file:///default.crt",
		PrivateKey:  "file:///default.key",
	}}
	defaults.Server.Upstream = endpoint{URL: &url.URL{Scheme: "svc", Host: "default"}}
	defaults.Server.Redis.URL = "redis://localhost:6379/0"
	return defaults
}

func main() {
	manager := cfgm.New(
		defaultConfig(),
		cfgm.AppName("cfgm-example"),
		cfgm.CLIAlias("server.addr", "a"),
		cfgm.HideCLI("server.redis.password"),
		cfgm.WithoutDefaultPaths(),
		cfgm.WithCodec(cfgm.Codec[endpoint]{Parse: parseEndpoint, Format: endpoint.String}),
	)

	server := &cli.Command{
		Name:  "server",
		Usage: "加载并打印完整服务配置",
		Action: manager.ActionReport(func(_ context.Context, _ *cli.Command, loaded *config, report *cfgm.Report) error {
			_, _ = fmt.Fprintf(
				os.Stdout,
				"addr=%s timeout=%s tags=%v certificates=%d upstream=%s redis=%s password_set=%t\n",
				loaded.Server.Addr,
				loaded.Server.Timeout,
				loaded.Server.Tags,
				len(loaded.Server.Certificates),
				loaded.Server.Upstream.String(),
				loaded.Server.Redis.URL,
				loaded.Server.Redis.Password != "",
			)
			for _, source := range report.Sources {
				_, _ = fmt.Fprintf(os.Stdout, "source=%s keys=%v\n", source.Name, source.Keys)
			}
			return nil
		}),
	}
	app := &cli.Command{
		Name:     "cfgm-example",
		Usage:    "完整的 cfgm CLI 使用场景",
		Commands: []*cli.Command{server},
	}
	manager.MustConfigure(app)

	if err := app.Run(context.Background(), os.Args); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
