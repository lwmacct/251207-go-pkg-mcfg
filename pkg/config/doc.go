// Package config 提供通用的配置加载功能，可被外部项目复用。
//
// # 特性
//
// 使用泛型支持任意配置结构体类型，配置加载优先级 (从低到高)：
//  1. 默认值 - 通过 defaultConfig 参数传入
//  2. 配置文件 - 按 configPaths 顺序搜索，找到第一个即停止
//  3. CLI flags - 最高优先级，仅当用户明确指定时覆盖
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
// 加载配置：
//
//	cfg, err := config.Load(cmd, config.DefaultPaths("myapp"), Config{
//	    Name:    "default",
//	    Debug:   false,
//	    Timeout: 30 * time.Second,
//	})
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
