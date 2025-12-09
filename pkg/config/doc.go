// Package config 提供通用的配置加载功能，可被外部项目复用。
//
// # 特性
//
// 使用泛型支持任意配置结构体类型，配置加载优先级 (从低到高)：
//  1. 默认值 - 通过 defaultConfig 参数传入
//  2. 配置文件 - 通过 WithConfigPaths 选项设置
//  3. 环境变量 - 通过 WithEnvPrefix 选项启用
//  4. CLI flags - 通过 WithCommand 选项设置，最高优先级
//
// # 快速开始
//
// 定义配置结构体，使用 koanf 和 comment 标签：
//
//	type Config struct {
//	    Name    string        `koanf:"name"    comment:"应用名称"`
//	    Debug   bool          `koanf:"debug"   comment:"调试模式"`
//	    Timeout time.Duration `koanf:"timeout" comment:"超时时间"`
//	}
//
// 加载配置（使用函数选项模式）：
//
//	cfg, err := config.Load(Config{
//	    Name:    "default",
//	    Debug:   false,
//	    Timeout: 30 * time.Second,
//	},
//	    config.WithConfigPaths(config.DefaultPaths("myapp")...),
//	    config.WithEnvPrefix("MYAPP_"),
//	    config.WithCommand(cmd),
//	)
//
// # 环境变量
//
// 通过 [WithEnvPrefix] 启用环境变量支持，命名规则：
//   - 前缀 + 大写的 koanf key
//   - 点号 (.) 转为下划线 (_)
//
// 示例 (前缀为 "MYAPP_")：
//   - MYAPP_DEBUG → debug
//   - MYAPP_SERVER_URL → server.url
//   - MYAPP_CLIENT_TIMEOUT → client.timeout
//
// # CLI Flag 映射
//
// 支持两种 CLI flag 格式 (优先使用 kebab-case)：
//   - kebab-case: --server-skip_verify (仅 . 转为 -)
//   - dot notation: --server.skip_verify (保持原样)
//
// 映射示例 (koanf tag → CLI flags)：
//   - server.url → --server-url 或 --server.url
//   - tls.skip_verify → --tls-skip_verify 或 --tls.skip_verify
//
// # 支持的类型
//
// 基本类型：string, bool, int*, uint*, float*
// 时间类型：time.Duration, time.Time
// 复合类型：[]string, []int, map[string]string 等
//
// # 生成配置示例
//
// 使用 [GenerateExampleYAML] 根据配置结构体生成带注释的 YAML 示例文件：
//
//	yaml := config.GenerateExampleYAML(defaultConfig)
//	os.WriteFile("config.example.yaml", yaml, 0644)
//
// # 测试辅助
//
// 提供测试入口函数，方便外部项目集成：
//   - [RunGenerateExampleTest] - 生成配置示例文件
//   - [RunConfigKeysValidTest] - 校验配置文件不包含无效键
package config
