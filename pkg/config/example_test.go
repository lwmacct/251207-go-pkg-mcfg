// Author: lwmacct (https://github.com/lwmacct)
package config_test

import (
	"fmt"
	"time"

	"github.com/lwmacct/251207-go-pkg-config/pkg/config"
)

// ExampleDefaultPaths 演示如何获取默认配置文件搜索路径
func ExampleDefaultPaths() {
	// 不指定应用名称时，返回基础路径
	paths := config.DefaultPaths()
	fmt.Println("基础路径数量:", len(paths))

	// 指定应用名称时，会包含应用专属配置路径
	paths = config.DefaultPaths("myapp")
	fmt.Println("带应用名路径数量:", len(paths))

	// Output:
	// 基础路径数量: 2
	// 带应用名路径数量: 5
}

// ExampleGenerateExampleYAML 演示如何根据配置结构体生成 YAML 示例
func ExampleGenerateExampleYAML() {
	// 定义配置结构体，使用 koanf 和 comment 标签
	type ServerConfig struct {
		Host string `koanf:"host" comment:"服务器主机地址"`
		Port int    `koanf:"port" comment:"服务器端口"`
	}
	type AppConfig struct {
		Name    string        `koanf:"name"    comment:"应用名称"`
		Debug   bool          `koanf:"debug"   comment:"是否启用调试模式"`
		Timeout time.Duration `koanf:"timeout" comment:"超时时间"`
		Server  ServerConfig  `koanf:"server"  comment:"服务器配置"`
	}

	// 创建默认配置
	defaultCfg := AppConfig{
		Name:    "example-app",
		Debug:   false,
		Timeout: 30 * time.Second,
		Server: ServerConfig{
			Host: "localhost",
			Port: 8080,
		},
	}

	// 生成 YAML 示例
	yaml := config.GenerateExampleYAML(defaultCfg)
	fmt.Println(string(yaml))

	// Output:
	// # 配置示例文件, 复制此文件为 config.yaml 并根据需要修改
	// name: "example-app" # 应用名称
	// debug: false # 是否启用调试模式
	// timeout: 30s # 超时时间
	//
	// # 服务器配置
	// server:
	//   host: "localhost" # 服务器主机地址
	//   port: 8080 # 服务器端口
}

// ExampleLoad 演示如何加载配置
//
// Load 函数按以下优先级合并配置:
//  1. 默认值 (最低优先级)
//  2. 配置文件
//  3. CLI flags (最高优先级)
func ExampleLoad() {
	type Config struct {
		Name  string `koanf:"name"`
		Debug bool   `koanf:"debug"`
	}

	defaultCfg := Config{
		Name:  "default-app",
		Debug: false,
	}

	// 不使用 CLI 命令时，cmd 参数传 nil
	// 配置文件不存在时，使用默认值
	cfg, err := config.Load(nil, []string{"nonexistent.yaml"}, defaultCfg)
	if err != nil {
		fmt.Println("加载失败:", err)
		return
	}

	fmt.Println("Name:", cfg.Name)
	fmt.Println("Debug:", cfg.Debug)

	// Output:
	// Name: default-app
	// Debug: false
}
