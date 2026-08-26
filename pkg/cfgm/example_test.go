// Author: lwmacct (https://github.com/lwmacct)
package cfgm_test

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/lwmacct/251207-go-pkg-cfgm/pkg/cfgm"
)

func Example_defaultPaths() {
	paths := cfgm.DefaultPaths()
	fmt.Println("基础路径数量:", len(paths))

	paths = cfgm.DefaultPaths("app")
	fmt.Println("带应用名路径数量:", len(paths))

	// Output:
	// 基础路径数量: 6
	// 带应用名路径数量: 15
}

func Example_exampleYAML() {
	type ServerConfig struct {
		Host string `json:"host" desc:"服务器主机地址"`
		Port int    `json:"port" desc:"服务器端口"`
	}
	type AppConfig struct {
		Name    string        `json:"name"    desc:"应用名称"`
		Debug   bool          `json:"debug"   desc:"是否启用调试模式"`
		Timeout time.Duration `json:"timeout" desc:"超时时间"`
		Server  ServerConfig  `json:"server"  desc:"服务器配置"`
	}

	defaultCfg := AppConfig{
		Name:    "example-app",
		Debug:   false,
		Timeout: 30 * time.Second,
		Server: ServerConfig{
			Host: "localhost",
			Port: 8080,
		},
	}

	yaml, _ := cfgm.ExampleYAML(defaultCfg)
	fmt.Println(string(yaml))

	// Output:
	// # 默认配置示例文件, 此文件由单元测试生成, 请勿直接修改
	// # 复制此文件为 config.yaml 并根据需要修改
	// name: "example-app" # 应用名称
	// debug: false # 是否启用调试模式
	// timeout: 30s # 超时时间
	//
	// # 服务器配置
	// server:
	//   host: "localhost" # 服务器主机地址
	//   port: 8080 # 服务器端口
}

func ExampleManager_Load() {
	type Config struct {
		Name  string `json:"name"`
		Debug bool   `json:"debug"`
	}

	manager := cfgm.MustNew(Config{
		Name:  "default-app",
		Debug: false,
	}, cfgm.WithoutDefaultPaths())
	cfg, err := manager.Load(context.Background())
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

func ExampleManager_Load_withEnv() {
	type Config struct {
		Name  string `json:"name"`
		Debug bool   `json:"debug"`
	}

	manager := cfgm.MustNew(Config{Name: "default-app"}, cfgm.WithoutDefaultPaths())
	cfg, err := manager.Load(context.Background(), cfgm.Env("APP_"))
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

func Example_marshalJSON() {
	type ServerConfig struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	type AppConfig struct {
		Name   string       `json:"name"`
		Debug  bool         `json:"debug"`
		Server ServerConfig `json:"server"`
	}

	defaultCfg := AppConfig{
		Name:  "example-app",
		Debug: false,
		Server: ServerConfig{
			Host: "localhost",
			Port: 8080,
		},
	}

	jsonBytes, _ := cfgm.MarshalJSON(defaultCfg)
	fmt.Println(string(jsonBytes))

	// Output:
	// {
	//   "name": "example-app",
	//   "debug": false,
	//   "server": {
	//     "host": "localhost",
	//     "port": 8080
	//   }
	// }
}

func ExampleManager_Load_withJSONConfig() {
	type Config struct {
		Name  string `json:"name"`
		Debug bool   `json:"debug"`
	}

	configContent := `{
  "name": "json-app",
  "debug": true
}`
	tmpFile := "/tmp/example_json_test.json"
	if err := os.WriteFile(tmpFile, []byte(configContent), 0600); err != nil {
		fmt.Println("创建临时文件失败:", err)

		return
	}
	defer func() { _ = os.Remove(tmpFile) }()

	manager := cfgm.MustNew(Config{Name: "default-app"}, cfgm.WithoutDefaultPaths())
	cfg, err := manager.Load(context.Background(), cfgm.File(tmpFile))
	if err != nil {
		fmt.Println("加载失败:", err)

		return
	}

	fmt.Println("Name:", cfg.Name)
	fmt.Println("Debug:", cfg.Debug)

	// Output:
	// Name: json-app
	// Debug: true
}
