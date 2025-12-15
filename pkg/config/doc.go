// Package config 提供通用的配置加载功能，可被外部项目复用。
//
// # 特性
//
// 使用泛型支持任意配置结构体类型，支持 YAML 和 JSON 格式（根据文件扩展名自动检测）。
//
// 配置加载优先级 (从低到高)：
//  1. 默认值 - 通过 defaultConfig 参数传入
//  2. 配置文件 - 通过 WithConfigPaths 选项设置
//  3. 环境变量(前缀) - 通过 WithEnvPrefix 选项启用
//  4. 环境变量(绑定) - 通过 WithEnvBindKey(配置文件) 或 WithEnvBindings(代码) 设置
//  5. CLI flags - 通过 WithCommand 选项设置，最高优先级
//
// # 快速开始
//
// 定义配置结构体，使用 koanf 和 desc 标签：
//
//	type Config struct {
//	    Name    string        `koanf:"name"    desc:"应用名称"`
//	    Debug   bool          `koanf:"debug"   desc:"调试模式"`
//	    Timeout time.Duration `koanf:"timeout" desc:"超时时间"`
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
//	    config.WithEnvBindKey("envbind"),
//	    config.WithCommand(cmd),
//	)
//
// # 环境变量(前缀)
//
// 通过 [WithEnvPrefix] 启用环境变量支持，命名规则：
//   - 前缀 + 大写的 koanf key
//   - 点号 (.) 和连字符 (-) 都转为下划线 (_)
//
// 示例 (前缀为 "MYAPP_")：
//   - MYAPP_DEBUG → debug
//   - MYAPP_SERVER_URL → server.url
//   - MYAPP_CLIENT_REV_AUTH_USER → client.rev-auth-user (支持连字符)
//
// 注意：通过反射自动生成所有 koanf key 的绑定，因此支持任意命名的 koanf key。
//
// # 环境变量(绑定)
//
// 方式一：通过代码绑定 [WithEnvBindings]：
//
//	config.WithEnvBindings(map[string]string{
//	    "REDIS_URL":         "redis.url",
//	    "ETCDCTL_ENDPOINTS": "etcd.endpoints",
//	})
//
// 方式二：通过配置文件绑定 [WithEnvBindKey]：
//
//	# config.yaml
//	envbind:
//	  REDIS_URL: redis.url
//	  ETCDCTL_ENDPOINTS: etcd.endpoints
//
//	redis:
//	  url: "redis://localhost:6379"
//
// 代码中的绑定优先级高于配置文件中的绑定。
//
// # 模板展开
//
// 配置文件默认启用模板展开功能，在解析前处理模板语法（YAML 和 JSON 均支持）。
// 使用 [WithoutTemplateExpansion] 可禁用此功能。
//
// 支持的模板函数：
//   - env: 获取环境变量 {{env "VAR"}} 或 {{env "VAR" "default"}}
//   - default: 管道式默认值 {{.VAR | default "fallback"}}
//   - coalesce: 返回第一个非空值 {{coalesce .VAR1 .VAR2 "default"}}
//
// Taskfile 风格直接访问环境变量：
//
//	api_key: "{{.OPENAI_API_KEY}}"
//	model: "{{.MODEL | default \"gpt-4\"}}"
//
// 配置文件示例：
//
//	# config.yaml
//	api_key: "{{env `OPENAI_API_KEY`}}"
//	model: "{{.LLM_MODEL | default `gpt-4`}}"
//	base_url: "{{coalesce .PROD_URL .DEV_URL `http://localhost:8080`}}"
//
// 禁用模板展开：
//
//	cfg, err := config.Load(Config{},
//	    config.WithConfigPaths("config.yaml"),
//	    config.WithoutTemplateExpansion(), // 禁用模板展开
//	)
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
// 使用 [GenerateExampleJSON] 生成 JSON 示例文件（JSON 不支持注释）：
//
//	jsonBytes := config.GenerateExampleJSON(defaultConfig)
//	os.WriteFile("config.example.json", jsonBytes, 0644)
//
// # 测试辅助
//
// 使用 [ConfigTestHelper] 提供测试辅助功能：
//
//	var helper = config.ConfigTestHelper[Config]{
//	    ExamplePath: "config/config.example.yaml",
//	    ConfigPath:  "config/config.yaml",
//	}
//
//	func TestGenerateExample(t *testing.T) { helper.GenerateExample(t, DefaultConfig()) }
//	func TestConfigKeysValid(t *testing.T) { helper.ValidateKeys(t) }
package config
