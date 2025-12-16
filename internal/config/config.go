// Package config 提供应用配置管理。
//
// 配置加载优先级 (从低到高)：
//  1. 默认值 - DefaultConfig() 函数中定义
//  2. 配置文件 - 通过 WithConfigPaths 选项设置
//  3. 环境变量 - 通过 WithEnvPrefix 选项启用
//  4. CLI flags - 通过 WithCommand 选项设置
package config

import (
	"time"

	"github.com/lwmacct/251207-go-pkg-mcfg/pkg/mcfg"
	"github.com/urfave/cli/v3"
)

// Config 应用配置
type Config struct {
	Server ServerConfig `koanf:"server" desc:"服务端配置"`
	Client ClientConfig `koanf:"client" desc:"客户端配置"`
}

// ServerConfig 服务端配置
type ServerConfig struct {
	Addr     string        `koanf:"addr" desc:"服务器监听地址"`
	Docs     string        `koanf:"docs" desc:"VitePress 文档目录路径"`
	Timeout  time.Duration `koanf:"timeout" desc:"HTTP 读写超时"`
	Idletime time.Duration `koanf:"idletime" desc:"HTTP 空闲超时"`
}

// ClientConfig 客户端配置
type ClientConfig struct {
	URL     string        `koanf:"url" desc:"服务器地址"`
	Timeout time.Duration `koanf:"timeout" desc:"请求超时时间"`
	Retries int           `koanf:"retries" desc:"重试次数"`
}

// DefaultConfig 返回默认配置
// 注意：internal/command/command.go 中的 Defaults 变量引用此函数以实现单一配置来源。
func DefaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Addr:     ":8080",
			Docs:     "docs/.vitepress/dist",
			Timeout:  15 * time.Second,
			Idletime: 60 * time.Second,
		},
		Client: ClientConfig{
			URL:     "http://localhost:8080",
			Timeout: 30 * time.Second,
			Retries: 3,
		},
	}
}

func Load(cmd *cli.Command, appName string, opts ...mcfg.Option) (*Config, error) {
	return mcfg.Load(
		DefaultConfig(),
		append([]mcfg.Option{
			mcfg.WithCommand(cmd),
			mcfg.WithConfigPaths(mcfg.DefaultPaths(appName)...),
			mcfg.WithEnvPrefix("APP_"),
		}, opts...)...,
	)
}
