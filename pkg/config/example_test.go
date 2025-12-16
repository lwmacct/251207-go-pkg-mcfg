// Author: lwmacct (https://github.com/lwmacct)
package config_test

import (
	"fmt"
	"os"
	"time"

	"github.com/lwmacct/251207-go-pkg-mcfg/pkg/config"
)

// Example_defaultPaths 演示如何获取默认配置文件搜索路径
func Example_defaultPaths() {
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

// Example_generateExampleYAML 演示如何根据配置结构体生成 YAML 示例
func Example_generateExampleYAML() {
	// 定义配置结构体，使用 koanf 和 desc 标签
	type ServerConfig struct {
		Host string `koanf:"host" desc:"服务器主机地址"`
		Port int    `koanf:"port" desc:"服务器端口"`
	}
	type AppConfig struct {
		Name    string        `koanf:"name"    desc:"应用名称"`
		Debug   bool          `koanf:"debug"   desc:"是否启用调试模式"`
		Timeout time.Duration `koanf:"timeout" desc:"超时时间"`
		Server  ServerConfig  `koanf:"server"  desc:"服务器配置"`
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

// Example_load 演示如何加载配置
//
// Load 函数按以下优先级合并配置:
//  1. 默认值 (最低优先级)
//  2. 配置文件
//  3. 环境变量
//  4. CLI flags (最高优先级)
func Example_load() {
	type Config struct {
		Name  string `koanf:"name"`
		Debug bool   `koanf:"debug"`
	}

	defaultCfg := Config{
		Name:  "default-app",
		Debug: false,
	}

	// 使用函数选项模式加载配置
	// 配置文件不存在时，使用默认值
	cfg, err := config.Load(defaultCfg,
		config.WithConfigPaths("nonexistent.yaml"),
	)
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

// Example_load_withEnvPrefix 演示如何通过环境变量加载配置
//
// 环境变量命名规则：
//   - 前缀 + 大写的 koanf key
//   - 点号 (.) 转为下划线 (_)
func Example_load_withEnvPrefix() {
	type Config struct {
		Name  string `koanf:"name"`
		Debug bool   `koanf:"debug"`
	}

	defaultCfg := Config{
		Name:  "default-app",
		Debug: false,
	}

	// 使用环境变量前缀 "MYAPP_"
	// 支持的环境变量：MYAPP_NAME, MYAPP_DEBUG
	cfg, err := config.Load(defaultCfg,
		config.WithEnvPrefix("MYAPP_"),
	)
	if err != nil {
		fmt.Println("加载失败:", err)
		return
	}

	// 如果设置了 MYAPP_NAME=prod-app，则 cfg.Name 为 "prod-app"
	// 如果没有设置环境变量，则使用默认值
	fmt.Println("Name:", cfg.Name)
	fmt.Println("Debug:", cfg.Debug)

	// Output:
	// Name: default-app
	// Debug: false
}

// Example_load_withEnvBindings 演示如何绑定第三方工具的环境变量
//
// 直接绑定环境变量到配置路径，无需遵循命名转换规则。
// 适用于复用 Redis、etcd、MySQL 等工具的标准环境变量。
func Example_load_withEnvBindings() {
	type RedisConfig struct {
		URL string `koanf:"url"`
	}
	type Config struct {
		Name  string      `koanf:"name"`
		Redis RedisConfig `koanf:"redis"`
	}

	defaultCfg := Config{
		Name: "default-app",
		Redis: RedisConfig{
			URL: "redis://localhost:6379",
		},
	}

	// 绑定 REDIS_URL 环境变量到 redis.url 配置路径
	// 如果设置了 REDIS_URL=redis://prod:6379，则 cfg.Redis.URL 为该值
	cfg, err := config.Load(defaultCfg,
		config.WithEnvBindings(map[string]string{
			"REDIS_URL": "redis.url",
		}),
	)
	if err != nil {
		fmt.Println("加载失败:", err)
		return
	}

	// 没有设置 REDIS_URL 环境变量时，使用默认值
	fmt.Println("Name:", cfg.Name)
	fmt.Println("Redis URL:", cfg.Redis.URL)

	// Output:
	// Name: default-app
	// Redis URL: redis://localhost:6379
}

// Example_load_withEnvBindKey 演示如何从配置文件中读取环境变量绑定
//
// 在配置文件中定义 envbind 节点，无需修改代码即可配置环境变量映射。
// 配置文件中的绑定优先级低于代码中的 WithEnvBindings。
func Example_load_withEnvBindKey() {
	type RedisConfig struct {
		URL string `koanf:"url"`
	}
	type Config struct {
		Name  string      `koanf:"name"`
		Redis RedisConfig `koanf:"redis"`
	}

	// 创建临时配置文件
	configContent := `
envbind:
  REDIS_URL: redis.url

name: "from-config"
redis:
  url: "redis://localhost:6379"
`
	tmpFile := "/tmp/example_envbindkey_test.yaml"
	if err := os.WriteFile(tmpFile, []byte(configContent), 0644); err != nil {
		fmt.Println("创建临时文件失败:", err)
		return
	}
	defer func() { _ = os.Remove(tmpFile) }()

	defaultCfg := Config{
		Name: "default-app",
		Redis: RedisConfig{
			URL: "redis://default:6379",
		},
	}

	// 从配置文件的 envbind 节点读取绑定关系
	// 如果设置了 REDIS_URL 环境变量，则会覆盖 redis.url
	cfg, err := config.Load(defaultCfg,
		config.WithConfigPaths(tmpFile),
		config.WithEnvBindKey("envbind"),
	)
	if err != nil {
		fmt.Println("加载失败:", err)
		return
	}

	// envbind 节点不会出现在最终配置中
	fmt.Println("Name:", cfg.Name)
	fmt.Println("Redis URL:", cfg.Redis.URL)

	// Output:
	// Name: from-config
	// Redis URL: redis://localhost:6379
}

// Example_generateExampleJSON 演示如何根据配置结构体生成 JSON 示例
func Example_generateExampleJSON() {
	type ServerConfig struct {
		Host string `koanf:"host" json:"host"`
		Port int    `koanf:"port" json:"port"`
	}
	type AppConfig struct {
		Name   string       `koanf:"name" json:"name"`
		Debug  bool         `koanf:"debug" json:"debug"`
		Server ServerConfig `koanf:"server" json:"server"`
	}

	defaultCfg := AppConfig{
		Name:  "example-app",
		Debug: false,
		Server: ServerConfig{
			Host: "localhost",
			Port: 8080,
		},
	}

	// 生成 JSON 示例 (注意：JSON 不支持注释)
	jsonBytes := config.GenerateExampleJSON(defaultCfg)
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

// Example_load_withJSONConfig 演示如何加载 JSON 格式的配置文件
//
// Load 函数会根据文件扩展名自动选择解析器：
//   - .yaml, .yml → YAML 解析器
//   - .json → JSON 解析器
func Example_load_withJSONConfig() {
	type Config struct {
		Name  string `koanf:"name"`
		Debug bool   `koanf:"debug"`
	}

	// 创建临时 JSON 配置文件
	configContent := `{
  "name": "json-app",
  "debug": true
}`
	tmpFile := "/tmp/example_json_test.json"
	if err := os.WriteFile(tmpFile, []byte(configContent), 0644); err != nil {
		fmt.Println("创建临时文件失败:", err)
		return
	}
	defer func() { _ = os.Remove(tmpFile) }()

	defaultCfg := Config{
		Name:  "default-app",
		Debug: false,
	}

	// 根据 .json 扩展名自动使用 JSON 解析器
	cfg, err := config.Load(defaultCfg,
		config.WithConfigPaths(tmpFile),
	)
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
