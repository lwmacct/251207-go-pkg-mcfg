package config

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// TestEnvKeyDecoder 测试环境变量 key 解码器
func TestEnvKeyDecoder(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		input    string
		expected string
	}{
		{
			name:     "simple key",
			prefix:   "MYAPP_",
			input:    "MYAPP_DEBUG",
			expected: "debug",
		},
		{
			name:     "nested key",
			prefix:   "MYAPP_",
			input:    "MYAPP_SERVER_URL",
			expected: "server.url",
		},
		{
			name:     "deeply nested key",
			prefix:   "MYAPP_",
			input:    "MYAPP_CLIENT_SERVER_PASSWORD",
			expected: "client.server.password",
		},
		{
			name:     "empty prefix",
			prefix:   "",
			input:    "SERVER_URL",
			expected: "server.url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := envKeyDecoder(tt.prefix)
			result := decoder(tt.input)
			assert.Equal(t, tt.expected, result, "envKeyDecoder(%q)(%q)", tt.prefix, tt.input)
		})
	}
}

// TestLoadWithEnvPrefix 测试环境变量前缀加载
func TestLoadWithEnvPrefix(t *testing.T) {
	type ServerConfig struct {
		URL string `koanf:"url"`
	}
	type Config struct {
		Debug  bool         `koanf:"debug"`
		Server ServerConfig `koanf:"server"`
	}

	// 设置环境变量
	t.Setenv("TEST_DEBUG", "true")
	t.Setenv("TEST_SERVER_URL", "http://test:8080")

	defaultCfg := Config{
		Debug: false,
		Server: ServerConfig{
			URL: "http://default:8080",
		},
	}

	cfg, err := Load(defaultCfg, WithEnvPrefix("TEST_"))
	require.NoError(t, err, "Load should not fail")

	assert.True(t, cfg.Debug, "Debug should be true from env")
	assert.Equal(t, "http://test:8080", cfg.Server.URL, "Server.URL should be from env")
}

// TestLoadWithEnvBinding 测试直接绑定环境变量
func TestLoadWithEnvBinding(t *testing.T) {
	type RedisConfig struct {
		URL string `koanf:"url"`
	}
	type Config struct {
		Name  string      `koanf:"name"`
		Redis RedisConfig `koanf:"redis"`
	}

	// 设置环境变量
	t.Setenv("REDIS_URL", "redis://test:6379")

	defaultCfg := Config{
		Name: "default",
		Redis: RedisConfig{
			URL: "redis://default:6379",
		},
	}

	cfg, err := Load(defaultCfg,
		WithEnvBinding("REDIS_URL", "redis.url"),
	)
	require.NoError(t, err, "Load should not fail")

	assert.Equal(t, "redis://test:6379", cfg.Redis.URL, "Redis.URL should be from env binding")
}

// TestLoadWithHyphenInKoanfKey 测试 koanf key 包含连字符的情况
//
// 当 koanf key 包含 "-" 时（如 server-password），无法通过 WithEnvPrefix 映射，
// 因为环境变量名不允许包含 "-"。此时应使用 WithEnvBinding 直接绑定。
func TestLoadWithHyphenInKoanfKey(t *testing.T) {
	type ClientConfig struct {
		ServerPassword string `koanf:"server-password"`
		ServerHost     string `koanf:"server-host"`
	}
	type Config struct {
		Client ClientConfig `koanf:"client"`
	}

	// 设置环境变量
	t.Setenv("CLIENT_SERVER_PASSWORD", "secret123")
	t.Setenv("MY_SERVER_HOST", "test-host")

	defaultCfg := Config{
		Client: ClientConfig{
			ServerPassword: "default-password",
			ServerHost:     "default-host",
		},
	}

	cfg, err := Load(defaultCfg,
		// 使用直接绑定，因为 koanf key 包含 "-"
		WithEnvBinding("CLIENT_SERVER_PASSWORD", "client.server-password"),
		WithEnvBinding("MY_SERVER_HOST", "client.server-host"),
	)
	require.NoError(t, err, "Load should not fail")

	assert.Equal(t, "secret123", cfg.Client.ServerPassword, "ServerPassword should be from env binding")
	assert.Equal(t, "test-host", cfg.Client.ServerHost, "ServerHost should be from env binding")
}

// TestLoadWithEnvBindKey 测试从配置文件读取绑定
func TestLoadWithEnvBindKey(t *testing.T) {
	type ClientConfig struct {
		ServerPassword string `koanf:"server-password"`
	}
	type Config struct {
		Name   string       `koanf:"name"`
		Client ClientConfig `koanf:"client"`
	}

	// 创建临时配置文件
	configContent := `
envbind:
  CLIENT_PWD: client.server-password

name: "from-config"
client:
  server-password: "file-password"
`
	tmpFile := "/tmp/test_envbindkey.yaml"
	err := os.WriteFile(tmpFile, []byte(configContent), 0644)
	require.NoError(t, err, "Failed to create temp file")
	defer os.Remove(tmpFile)

	// 设置环境变量
	t.Setenv("CLIENT_PWD", "env-password")

	defaultCfg := Config{
		Name: "default",
		Client: ClientConfig{
			ServerPassword: "default-password",
		},
	}

	cfg, err := Load(defaultCfg,
		WithConfigPaths(tmpFile),
		WithEnvBindKey("envbind"),
	)
	require.NoError(t, err, "Load should not fail")

	// 环境变量应该覆盖配置文件中的值
	assert.Equal(t, "env-password", cfg.Client.ServerPassword, "ServerPassword should be from env (higher priority)")
	assert.Equal(t, "from-config", cfg.Name, "Name should be from config file")
}

// TestEnvBindingPriority 测试绑定优先级：代码 > 配置文件
func TestEnvBindingPriority(t *testing.T) {
	type Config struct {
		Password string `koanf:"password"`
	}

	// 创建临时配置文件
	configContent := `
envbind:
  FILE_PWD: password

password: "file-value"
`
	tmpFile := "/tmp/test_priority.yaml"
	err := os.WriteFile(tmpFile, []byte(configContent), 0644)
	require.NoError(t, err, "Failed to create temp file")
	defer os.Remove(tmpFile)

	// 设置两个环境变量
	t.Setenv("FILE_PWD", "from-file-binding")
	t.Setenv("CODE_PWD", "from-code-binding")

	defaultCfg := Config{
		Password: "default",
	}

	cfg, err := Load(defaultCfg,
		WithConfigPaths(tmpFile),
		WithEnvBindKey("envbind"),
		// 代码绑定优先级更高，应该覆盖配置文件绑定
		WithEnvBinding("CODE_PWD", "password"),
	)
	require.NoError(t, err, "Load should not fail")

	// 代码绑定优先级更高
	assert.Equal(t, "from-code-binding", cfg.Password, "Password should use code binding (higher priority)")
}

// TestAutoEnvBinding 测试自动环境变量绑定功能
//
// 当使用 WithEnvPrefix 时，会自动生成所有 koanf key 的环境变量绑定，
// 这解决了 koanf key 包含连字符（如 rev-auth-user）时无法通过前缀匹配的问题。
func TestAutoEnvBinding(t *testing.T) {
	type ClientConfig struct {
		ServerPassword string `koanf:"server-password"`
		ServerHost     string `koanf:"server-host"`
		Timeout        int    `koanf:"timeout"`
	}
	type Config struct {
		Name   string       `koanf:"name"`
		Client ClientConfig `koanf:"client"`
	}

	// 设置环境变量（使用自动绑定规则：. 和 - 都转为 _）
	t.Setenv("TEST_NAME", "from-env")
	t.Setenv("TEST_CLIENT_SERVER_PASSWORD", "secret123")
	t.Setenv("TEST_CLIENT_SERVER_HOST", "test-host")
	t.Setenv("TEST_CLIENT_TIMEOUT", "30")

	defaultCfg := Config{
		Name: "default",
		Client: ClientConfig{
			ServerPassword: "default-password",
			ServerHost:     "default-host",
			Timeout:        10,
		},
	}

	// 仅使用 WithEnvPrefix，自动绑定应该处理所有字段（包括带连字符的）
	cfg, err := Load(defaultCfg,
		WithEnvPrefix("TEST_"),
	)
	require.NoError(t, err, "Load should not fail")

	// 验证普通字段
	assert.Equal(t, "from-env", cfg.Name, "Name should be from env")

	// 验证带连字符的字段（这是自动绑定的核心价值）
	assert.Equal(t, "secret123", cfg.Client.ServerPassword, "ServerPassword should work with hyphenated koanf key")
	assert.Equal(t, "test-host", cfg.Client.ServerHost, "ServerHost should work with hyphenated koanf key")
	assert.Equal(t, 30, cfg.Client.Timeout, "Timeout should be from env")
}

// TestLoadPriority 测试完整的加载优先级
//
// 优先级（从低到高）：
// 1. 默认值
// 2. 配置文件
// 3. 环境变量(前缀)
// 4. 环境变量(绑定)
// 5. CLI flags
func TestLoadPriority(t *testing.T) {
	type Config struct {
		Value1 string `koanf:"value1"`
		Value2 string `koanf:"value2"`
		Value3 string `koanf:"value3"`
	}

	// 创建临时配置文件
	configContent := `
value1: "from-file"
value2: "from-file"
value3: "from-file"
`
	tmpFile := "/tmp/test_load_priority.yaml"
	err := os.WriteFile(tmpFile, []byte(configContent), 0644)
	require.NoError(t, err, "Failed to create temp file")
	defer os.Remove(tmpFile)

	// 设置环境变量
	t.Setenv("TEST_VALUE2", "from-env-prefix")
	t.Setenv("TEST_VALUE3", "from-env-prefix")
	t.Setenv("BOUND_VALUE3", "from-env-binding")

	defaultCfg := Config{
		Value1: "default",
		Value2: "default",
		Value3: "default",
	}

	cfg, err := Load(defaultCfg,
		WithConfigPaths(tmpFile),
		WithEnvPrefix("TEST_"),
		WithEnvBinding("BOUND_VALUE3", "value3"),
	)
	require.NoError(t, err, "Load should not fail")

	// value1: 仅配置文件覆盖默认值
	assert.Equal(t, "from-file", cfg.Value1, "Value1: config file > default")

	// value2: 环境变量前缀覆盖配置文件
	assert.Equal(t, "from-env-prefix", cfg.Value2, "Value2: env prefix > config file")

	// value3: 环境变量绑定覆盖环境变量前缀
	assert.Equal(t, "from-env-binding", cfg.Value3, "Value3: env binding > env prefix")
}

// TestLoadWithDefaultsOnly 测试仅使用默认配置
func TestLoadWithDefaultsOnly(t *testing.T) {
	type Config struct {
		Name  string `koanf:"name"`
		Debug bool   `koanf:"debug"`
		Port  int    `koanf:"port"`
	}

	defaultCfg := Config{
		Name:  "my-app",
		Debug: true,
		Port:  8080,
	}

	cfg, err := Load(defaultCfg)
	require.NoError(t, err, "Load should not fail with defaults only")

	assert.Equal(t, "my-app", cfg.Name)
	assert.True(t, cfg.Debug)
	assert.Equal(t, 8080, cfg.Port)
}

// TestLoadWithNonExistentConfigFile 测试配置文件不存在时使用默认值
func TestLoadWithNonExistentConfigFile(t *testing.T) {
	type Config struct {
		Name string `koanf:"name"`
	}

	defaultCfg := Config{
		Name: "fallback-app",
	}

	cfg, err := Load(defaultCfg,
		WithConfigPaths("/nonexistent/path/config.yaml"),
	)
	require.NoError(t, err, "Load should not fail when config file not found")

	assert.Equal(t, "fallback-app", cfg.Name, "Should use default when config file not found")
}

// =============================================================================
// CLI Flags 测试 (github.com/urfave/cli/v3)
// =============================================================================

// TestLoadWithCommand 测试 CLI flags 覆盖配置
func TestLoadWithCommand(t *testing.T) {
	type ServerConfig struct {
		Addr    string        `koanf:"addr"`
		Timeout time.Duration `koanf:"timeout"`
	}
	type Config struct {
		Name   string       `koanf:"name"`
		Debug  bool         `koanf:"debug"`
		Server ServerConfig `koanf:"server"`
	}

	defaultCfg := Config{
		Name:  "default-app",
		Debug: false,
		Server: ServerConfig{
			Addr:    ":8080",
			Timeout: 30 * time.Second,
		},
	}

	// 创建测试命令
	var loadedCfg *Config
	cmd := &cli.Command{
		Name: "test",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "name", Value: defaultCfg.Name},
			&cli.BoolFlag{Name: "debug", Value: defaultCfg.Debug},
			&cli.StringFlag{Name: "server-addr", Value: defaultCfg.Server.Addr},
			&cli.DurationFlag{Name: "server-timeout", Value: defaultCfg.Server.Timeout},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cfg, err := Load(defaultCfg, WithCommand(cmd))
			if err != nil {
				return err
			}
			loadedCfg = cfg
			return nil
		},
	}

	// 模拟 CLI 参数：覆盖 name 和 debug
	args := []string{"test", "--name", "cli-app", "--debug"}
	err := cmd.Run(context.Background(), args)
	require.NoError(t, err, "Command should run successfully")
	require.NotNil(t, loadedCfg, "Config should be loaded")

	// CLI flags 应该覆盖默认值
	assert.Equal(t, "cli-app", loadedCfg.Name, "Name should be from CLI flag")
	assert.True(t, loadedCfg.Debug, "Debug should be true from CLI flag")

	// 未设置的 flag 应该保持默认值
	assert.Equal(t, ":8080", loadedCfg.Server.Addr, "Server.Addr should keep default")
	assert.Equal(t, 30*time.Second, loadedCfg.Server.Timeout, "Server.Timeout should keep default")
}

// TestLoadWithCommand_NestedFlags 测试嵌套结构的 CLI flags (使用 kebab-case)
func TestLoadWithCommand_NestedFlags(t *testing.T) {
	type ServerConfig struct {
		Addr    string        `koanf:"addr"`
		Timeout time.Duration `koanf:"timeout"`
	}
	type Config struct {
		Server ServerConfig `koanf:"server"`
	}

	defaultCfg := Config{
		Server: ServerConfig{
			Addr:    ":8080",
			Timeout: 30 * time.Second,
		},
	}

	var loadedCfg *Config
	cmd := &cli.Command{
		Name: "test",
		Flags: []cli.Flag{
			// kebab-case: server.addr → --server-addr
			&cli.StringFlag{Name: "server-addr", Value: defaultCfg.Server.Addr},
			&cli.DurationFlag{Name: "server-timeout", Value: defaultCfg.Server.Timeout},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cfg, err := Load(defaultCfg, WithCommand(cmd))
			if err != nil {
				return err
			}
			loadedCfg = cfg
			return nil
		},
	}

	// 通过 CLI 覆盖嵌套配置
	args := []string{"test", "--server-addr", ":9090", "--server-timeout", "60s"}
	err := cmd.Run(context.Background(), args)
	require.NoError(t, err, "Command should run successfully")
	require.NotNil(t, loadedCfg, "Config should be loaded")

	assert.Equal(t, ":9090", loadedCfg.Server.Addr, "Server.Addr should be from CLI flag")
	assert.Equal(t, 60*time.Second, loadedCfg.Server.Timeout, "Server.Timeout should be from CLI flag")
}

// TestLoadWithCommand_SubCommands 测试子命令继承父命令的 flags
func TestLoadWithCommand_SubCommands(t *testing.T) {
	type Config struct {
		URL     string `koanf:"url"`
		Timeout int    `koanf:"timeout"`
	}

	defaultCfg := Config{
		URL:     "http://localhost:8080",
		Timeout: 30,
	}

	var loadedCfg *Config
	var subCommandExecuted bool

	// 子命令
	subCmd := &cli.Command{
		Name:  "health",
		Usage: "Check health",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cfg, err := Load(defaultCfg, WithCommand(cmd))
			if err != nil {
				return err
			}
			loadedCfg = cfg
			subCommandExecuted = true
			return nil
		},
	}

	// 主命令
	mainCmd := &cli.Command{
		Name:     "client",
		Commands: []*cli.Command{subCmd},
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "url", Value: defaultCfg.URL},
			&cli.IntFlag{Name: "timeout", Value: defaultCfg.Timeout},
		},
	}

	// 执行子命令，父命令的 flags 应该被继承
	args := []string{"client", "--url", "http://prod:8080", "health"}
	err := mainCmd.Run(context.Background(), args)
	require.NoError(t, err, "Command should run successfully")
	require.True(t, subCommandExecuted, "Subcommand should be executed")
	require.NotNil(t, loadedCfg, "Config should be loaded")

	assert.Equal(t, "http://prod:8080", loadedCfg.URL, "URL should be from parent CLI flag")
	assert.Equal(t, 30, loadedCfg.Timeout, "Timeout should keep default (not set)")
}

// TestLoadWithCommand_Priority 测试 CLI flags 优先级高于环境变量
func TestLoadWithCommand_Priority(t *testing.T) {
	type Config struct {
		Value string `koanf:"value"`
	}

	defaultCfg := Config{
		Value: "default",
	}

	// 设置环境变量
	t.Setenv("TEST_VALUE", "from-env")

	var loadedCfg *Config
	cmd := &cli.Command{
		Name: "test",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "value", Value: defaultCfg.Value},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cfg, err := Load(defaultCfg,
				WithEnvPrefix("TEST_"),
				WithCommand(cmd),
			)
			if err != nil {
				return err
			}
			loadedCfg = cfg
			return nil
		},
	}

	// CLI flag 应该覆盖环境变量
	args := []string{"test", "--value", "from-cli"}
	err := cmd.Run(context.Background(), args)
	require.NoError(t, err, "Command should run successfully")
	require.NotNil(t, loadedCfg, "Config should be loaded")

	// CLI flags 具有最高优先级
	assert.Equal(t, "from-cli", loadedCfg.Value, "CLI flag should override env var")
}

// TestLoadWithCommand_OnlySetFlags 测试只有明确设置的 flags 才会覆盖
func TestLoadWithCommand_OnlySetFlags(t *testing.T) {
	type Config struct {
		Name  string `koanf:"name"`
		Debug bool   `koanf:"debug"`
	}

	// 配置文件中的值
	configContent := `
name: "from-file"
debug: true
`
	tmpFile := "/tmp/test_cli_only_set.yaml"
	err := os.WriteFile(tmpFile, []byte(configContent), 0644)
	require.NoError(t, err, "Failed to create temp file")
	defer os.Remove(tmpFile)

	defaultCfg := Config{
		Name:  "default",
		Debug: false,
	}

	var loadedCfg *Config
	cmd := &cli.Command{
		Name: "test",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "name", Value: defaultCfg.Name},
			&cli.BoolFlag{Name: "debug", Value: defaultCfg.Debug},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cfg, err := Load(defaultCfg,
				WithConfigPaths(tmpFile),
				WithCommand(cmd),
			)
			if err != nil {
				return err
			}
			loadedCfg = cfg
			return nil
		},
	}

	// 只设置 --name，不设置 --debug
	args := []string{"test", "--name", "from-cli"}
	err = cmd.Run(context.Background(), args)
	require.NoError(t, err, "Command should run successfully")
	require.NotNil(t, loadedCfg, "Config should be loaded")

	// --name 被设置，应该使用 CLI 值
	assert.Equal(t, "from-cli", loadedCfg.Name, "Name should be from CLI flag")

	// --debug 未设置，应该使用配置文件值
	assert.True(t, loadedCfg.Debug, "Debug should keep config file value (not overridden)")
}

// TestLoadWithCommand_NumericTypes 测试各种数值类型的 CLI flags
func TestLoadWithCommand_NumericTypes(t *testing.T) {
	type Config struct {
		Port    int     `koanf:"port"`
		Rate    float64 `koanf:"rate"`
		Retries uint    `koanf:"retries"`
	}

	defaultCfg := Config{
		Port:    8080,
		Rate:    1.0,
		Retries: 3,
	}

	var loadedCfg *Config
	cmd := &cli.Command{
		Name: "test",
		Flags: []cli.Flag{
			&cli.IntFlag{Name: "port", Value: defaultCfg.Port},
			&cli.Float64Flag{Name: "rate", Value: defaultCfg.Rate},
			&cli.UintFlag{Name: "retries", Value: uint(defaultCfg.Retries)},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cfg, err := Load(defaultCfg, WithCommand(cmd))
			if err != nil {
				return err
			}
			loadedCfg = cfg
			return nil
		},
	}

	args := []string{"test", "--port", "9090", "--rate", "2.5", "--retries", "5"}
	err := cmd.Run(context.Background(), args)
	require.NoError(t, err, "Command should run successfully")
	require.NotNil(t, loadedCfg, "Config should be loaded")

	assert.Equal(t, 9090, loadedCfg.Port, "Port should be from CLI flag")
	assert.Equal(t, 2.5, loadedCfg.Rate, "Rate should be from CLI flag")
	assert.Equal(t, uint(5), loadedCfg.Retries, "Retries should be from CLI flag")
}

// TestLoadWithCommand_StringSlice 测试字符串切片类型的 CLI flags
func TestLoadWithCommand_StringSlice(t *testing.T) {
	type Config struct {
		Hosts []string `koanf:"hosts"`
	}

	defaultCfg := Config{
		Hosts: []string{"localhost"},
	}

	var loadedCfg *Config
	cmd := &cli.Command{
		Name: "test",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{Name: "hosts", Value: defaultCfg.Hosts},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cfg, err := Load(defaultCfg, WithCommand(cmd))
			if err != nil {
				return err
			}
			loadedCfg = cfg
			return nil
		},
	}

	// 多次使用同一个 flag 来传递多个值
	args := []string{"test", "--hosts", "host1", "--hosts", "host2", "--hosts", "host3"}
	err := cmd.Run(context.Background(), args)
	require.NoError(t, err, "Command should run successfully")
	require.NotNil(t, loadedCfg, "Config should be loaded")

	assert.Equal(t, []string{"host1", "host2", "host3"}, loadedCfg.Hosts, "Hosts should be from CLI flags")
}

// =============================================================================
// GenerateExampleYAML 测试
// =============================================================================

// TestGenerateExampleYAML_BasicTypes 测试基本类型的 YAML 生成
func TestGenerateExampleYAML_BasicTypes(t *testing.T) {
	type Config struct {
		Name    string  `koanf:"name" comment:"应用名称"`
		Debug   bool    `koanf:"debug" comment:"调试模式"`
		Port    int     `koanf:"port" comment:"端口号"`
		Rate    float64 `koanf:"rate" comment:"速率"`
		Retries uint    `koanf:"retries" comment:"重试次数"`
	}

	cfg := Config{
		Name:    "test-app",
		Debug:   true,
		Port:    8080,
		Rate:    1.5,
		Retries: 3,
	}

	yaml := GenerateExampleYAML(cfg)
	yamlStr := string(yaml)

	// 验证包含文件头注释
	assert.Contains(t, yamlStr, "# 配置示例文件", "Should contain header comment")

	// 验证各字段
	assert.Contains(t, yamlStr, `name: "test-app"`, "Should contain name field")
	assert.Contains(t, yamlStr, "debug: true", "Should contain debug field")
	assert.Contains(t, yamlStr, "port: 8080", "Should contain port field")
	assert.Contains(t, yamlStr, "rate: 1.5", "Should contain rate field")
	assert.Contains(t, yamlStr, "retries: 3", "Should contain retries field")

	// 验证注释
	assert.Contains(t, yamlStr, "# 应用名称", "Should contain name comment")
	assert.Contains(t, yamlStr, "# 调试模式", "Should contain debug comment")
}

// TestGenerateExampleYAML_NestedStruct 测试嵌套结构体的 YAML 生成
func TestGenerateExampleYAML_NestedStruct(t *testing.T) {
	type ServerConfig struct {
		Host string `koanf:"host" comment:"服务器地址"`
		Port int    `koanf:"port" comment:"服务器端口"`
	}
	type Config struct {
		Name   string       `koanf:"name" comment:"应用名称"`
		Server ServerConfig `koanf:"server" comment:"服务器配置"`
	}

	cfg := Config{
		Name: "nested-app",
		Server: ServerConfig{
			Host: "localhost",
			Port: 9090,
		},
	}

	yaml := GenerateExampleYAML(cfg)
	yamlStr := string(yaml)

	// 验证嵌套结构
	assert.Contains(t, yamlStr, "server:", "Should contain server section")
	assert.Contains(t, yamlStr, `host: "localhost"`, "Should contain nested host field")
	assert.Contains(t, yamlStr, "port: 9090", "Should contain nested port field")
	assert.Contains(t, yamlStr, "# 服务器配置", "Should contain server section comment")
}

// TestGenerateExampleYAML_Duration 测试 time.Duration 类型
func TestGenerateExampleYAML_Duration(t *testing.T) {
	type Config struct {
		Timeout time.Duration `koanf:"timeout" comment:"超时时间"`
	}

	cfg := Config{
		Timeout: 30 * time.Second,
	}

	yaml := GenerateExampleYAML(cfg)
	yamlStr := string(yaml)

	assert.Contains(t, yamlStr, "timeout: 30s", "Should contain duration in human-readable format")
	assert.Contains(t, yamlStr, "# 超时时间", "Should contain timeout comment")
}

// TestGenerateExampleYAML_Slice 测试切片类型
func TestGenerateExampleYAML_Slice(t *testing.T) {
	type Config struct {
		Hosts  []string `koanf:"hosts" comment:"主机列表"`
		Empty  []string `koanf:"empty" comment:"空列表"`
		Ports  []int    `koanf:"ports" comment:"端口列表"`
	}

	cfg := Config{
		Hosts:  []string{"host1", "host2"},
		Empty:  []string{},
		Ports:  []int{8080, 9090},
	}

	yaml := GenerateExampleYAML(cfg)
	yamlStr := string(yaml)

	assert.Contains(t, yamlStr, "hosts:", "Should contain hosts field")
	assert.Contains(t, yamlStr, "- host1", "Should contain first host")
	assert.Contains(t, yamlStr, "- host2", "Should contain second host")
	assert.Contains(t, yamlStr, "empty: []", "Should contain empty slice")
	assert.Contains(t, yamlStr, "- 8080", "Should contain first port")
}

// TestGenerateExampleYAML_Map 测试 map 类型
func TestGenerateExampleYAML_Map(t *testing.T) {
	type Config struct {
		Labels map[string]string `koanf:"labels" comment:"标签"`
		Empty  map[string]string `koanf:"empty" comment:"空映射"`
	}

	cfg := Config{
		Labels: map[string]string{"env": "prod", "app": "test"},
		Empty:  map[string]string{},
	}

	yaml := GenerateExampleYAML(cfg)
	yamlStr := string(yaml)

	assert.Contains(t, yamlStr, "labels:", "Should contain labels field")
	assert.Contains(t, yamlStr, "empty: {}", "Should contain empty map")
}

// TestGenerateExampleYAML_SkipUntagged 测试跳过无 koanf 标签的字段
func TestGenerateExampleYAML_SkipUntagged(t *testing.T) {
	type Config struct {
		Name     string `koanf:"name" comment:"应用名称"`
		Internal string // 无 koanf 标签，应该跳过
	}

	cfg := Config{
		Name:     "test",
		Internal: "should-not-appear",
	}

	yaml := GenerateExampleYAML(cfg)
	yamlStr := string(yaml)

	assert.Contains(t, yamlStr, "name:", "Should contain tagged field")
	assert.NotContains(t, yamlStr, "should-not-appear", "Should not contain untagged field value")
	assert.NotContains(t, yamlStr, "Internal", "Should not contain untagged field name")
}

// =============================================================================
// DefaultPaths 测试
// =============================================================================

// TestDefaultPaths_NoAppName 测试无应用名称时的默认路径
func TestDefaultPaths_NoAppName(t *testing.T) {
	paths := DefaultPaths()

	assert.Len(t, paths, 2, "Should return 2 paths without app name")
	assert.Contains(t, paths, "config.yaml", "Should contain config.yaml")
	assert.Contains(t, paths, "config/config.yaml", "Should contain config/config.yaml")
}

// TestDefaultPaths_WithAppName 测试有应用名称时的默认路径
func TestDefaultPaths_WithAppName(t *testing.T) {
	paths := DefaultPaths("myapp")

	assert.GreaterOrEqual(t, len(paths), 4, "Should return at least 4 paths with app name")

	// 验证包含应用专属路径
	assert.Contains(t, paths, ".myapp.yaml", "Should contain current dir app config")
	assert.Contains(t, paths, "/etc/myapp/config.yaml", "Should contain system config")

	// 验证包含通用路径
	assert.Contains(t, paths, "config.yaml", "Should contain config.yaml")
	assert.Contains(t, paths, "config/config.yaml", "Should contain config/config.yaml")
}

// TestDefaultPaths_EmptyAppName 测试空应用名称
func TestDefaultPaths_EmptyAppName(t *testing.T) {
	paths := DefaultPaths("")

	// 空字符串应该等同于不传参数
	assert.Len(t, paths, 2, "Empty app name should return base paths only")
}

// =============================================================================
// FindProjectRoot 测试
// =============================================================================

// TestFindProjectRoot 测试查找项目根目录
func TestFindProjectRoot(t *testing.T) {
	// skip=0 表示从当前函数开始查找
	root, err := FindProjectRoot(0)

	require.NoError(t, err, "Should find project root")
	assert.NotEmpty(t, root, "Root path should not be empty")

	// 验证找到的目录包含 go.mod
	_, err = os.Stat(root + "/go.mod")
	assert.NoError(t, err, "Project root should contain go.mod")
}

// =============================================================================
// collectKoanfKeys 测试 (内部函数)
// =============================================================================

// TestCollectKoanfKeys_Flat 测试扁平结构的 key 收集
func TestCollectKoanfKeys_Flat(t *testing.T) {
	type Config struct {
		Name  string `koanf:"name"`
		Debug bool   `koanf:"debug"`
		Port  int    `koanf:"port"`
	}

	cfg := Config{}
	keys := collectKoanfKeys(cfg)

	assert.Len(t, keys, 3, "Should collect 3 keys")
	assert.Contains(t, keys, "name", "Should contain name key")
	assert.Contains(t, keys, "debug", "Should contain debug key")
	assert.Contains(t, keys, "port", "Should contain port key")
}

// TestCollectKoanfKeys_Nested 测试嵌套结构的 key 收集
func TestCollectKoanfKeys_Nested(t *testing.T) {
	type ServerConfig struct {
		Host string `koanf:"host"`
		Port int    `koanf:"port"`
	}
	type Config struct {
		Name   string       `koanf:"name"`
		Server ServerConfig `koanf:"server"`
	}

	cfg := Config{}
	keys := collectKoanfKeys(cfg)

	assert.Len(t, keys, 3, "Should collect 3 keys (1 top-level + 2 nested)")
	assert.Contains(t, keys, "name", "Should contain name key")
	assert.Contains(t, keys, "server.host", "Should contain nested server.host key")
	assert.Contains(t, keys, "server.port", "Should contain nested server.port key")
}

// TestCollectKoanfKeys_WithHyphen 测试包含连字符的 key
func TestCollectKoanfKeys_WithHyphen(t *testing.T) {
	type ClientConfig struct {
		ServerPassword string `koanf:"server-password"`
		RevAuthUser    string `koanf:"rev-auth-user"`
	}
	type Config struct {
		Client ClientConfig `koanf:"client"`
	}

	cfg := Config{}
	keys := collectKoanfKeys(cfg)

	assert.Len(t, keys, 2, "Should collect 2 keys")
	assert.Contains(t, keys, "client.server-password", "Should contain hyphenated key")
	assert.Contains(t, keys, "client.rev-auth-user", "Should contain hyphenated key")
}

// TestCollectKoanfKeys_Duration 测试 time.Duration 不被当作嵌套结构体
func TestCollectKoanfKeys_Duration(t *testing.T) {
	type Config struct {
		Timeout  time.Duration `koanf:"timeout"`
		Interval time.Duration `koanf:"interval"`
	}

	cfg := Config{}
	keys := collectKoanfKeys(cfg)

	assert.Len(t, keys, 2, "Should collect 2 keys (Duration should not be recursed)")
	assert.Contains(t, keys, "timeout", "Should contain timeout key")
	assert.Contains(t, keys, "interval", "Should contain interval key")
}

// =============================================================================
// generateEnvBindings 测试 (内部函数)
// =============================================================================

// TestGenerateEnvBindings_Basic 测试基本的环境变量绑定生成
func TestGenerateEnvBindings_Basic(t *testing.T) {
	koanfKeys := []string{"name", "debug", "port"}
	bindings := generateEnvBindings("APP_", koanfKeys)

	assert.Len(t, bindings, 3, "Should generate 3 bindings")
	assert.Equal(t, "name", bindings["APP_NAME"], "APP_NAME should map to name")
	assert.Equal(t, "debug", bindings["APP_DEBUG"], "APP_DEBUG should map to debug")
	assert.Equal(t, "port", bindings["APP_PORT"], "APP_PORT should map to port")
}

// TestGenerateEnvBindings_Nested 测试嵌套 key 的环境变量绑定
func TestGenerateEnvBindings_Nested(t *testing.T) {
	koanfKeys := []string{"server.host", "server.port", "client.url"}
	bindings := generateEnvBindings("MYAPP_", koanfKeys)

	assert.Equal(t, "server.host", bindings["MYAPP_SERVER_HOST"], "Should convert . to _")
	assert.Equal(t, "server.port", bindings["MYAPP_SERVER_PORT"], "Should convert . to _")
	assert.Equal(t, "client.url", bindings["MYAPP_CLIENT_URL"], "Should convert . to _")
}

// TestGenerateEnvBindings_Hyphen 测试包含连字符的 key
func TestGenerateEnvBindings_Hyphen(t *testing.T) {
	koanfKeys := []string{"client.server-password", "client.rev-auth-user"}
	bindings := generateEnvBindings("APP_", koanfKeys)

	assert.Equal(t, "client.server-password", bindings["APP_CLIENT_SERVER_PASSWORD"], "Should convert - to _")
	assert.Equal(t, "client.rev-auth-user", bindings["APP_CLIENT_REV_AUTH_USER"], "Should convert - to _")
}

// TestGenerateEnvBindings_EmptyPrefix 测试空前缀
func TestGenerateEnvBindings_EmptyPrefix(t *testing.T) {
	koanfKeys := []string{"name", "server.port"}
	bindings := generateEnvBindings("", koanfKeys)

	assert.Equal(t, "name", bindings["NAME"], "Should work without prefix")
	assert.Equal(t, "server.port", bindings["SERVER_PORT"], "Should work without prefix")
}
