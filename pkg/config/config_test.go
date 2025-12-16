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

// =============================================================================
// 测试辅助函数
// =============================================================================

// writeTempConfig 创建临时配置文件，返回文件路径和清理函数
func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "config_test_*.yaml")
	require.NoError(t, err, "Failed to create temp file")
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err, "Failed to write temp file")
	_ = tmpFile.Close()
	t.Cleanup(func() { _ = os.Remove(tmpFile.Name()) })
	return tmpFile.Name()
}

// runCLITest 运行 CLI 测试，返回加载的配置
func runCLITest[T any](t *testing.T, defaultCfg T, flags []cli.Flag, args []string, opts ...Option) *T {
	t.Helper()
	var loadedCfg *T
	cmd := &cli.Command{
		Name:  "test",
		Flags: flags,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			allOpts := append(opts, WithCommand(cmd))
			cfg, err := Load(defaultCfg, allOpts...)
			if err != nil {
				return err
			}
			loadedCfg = cfg
			return nil
		},
	}
	err := cmd.Run(context.Background(), args)
	require.NoError(t, err, "Command should run successfully")
	require.NotNil(t, loadedCfg, "Config should be loaded")
	return loadedCfg
}

// =============================================================================
// Load 函数测试
// =============================================================================

func TestLoadWithEnvPrefix(t *testing.T) {
	type ServerConfig struct {
		URL string `koanf:"url"`
	}
	type Config struct {
		Debug  bool         `koanf:"debug"`
		Server ServerConfig `koanf:"server"`
	}

	t.Setenv("TEST_DEBUG", "true")
	t.Setenv("TEST_SERVER_URL", "http://test:8080")

	cfg, err := Load(Config{Debug: false, Server: ServerConfig{URL: "http://default:8080"}}, WithEnvPrefix("TEST_"))
	require.NoError(t, err)

	assert.True(t, cfg.Debug)
	assert.Equal(t, "http://test:8080", cfg.Server.URL)
}

func TestLoadWithEnvBinding(t *testing.T) {
	type RedisConfig struct {
		URL string `koanf:"url"`
	}
	type Config struct {
		Name  string      `koanf:"name"`
		Redis RedisConfig `koanf:"redis"`
	}

	t.Setenv("REDIS_URL", "redis://test:6379")

	cfg, err := Load(
		Config{Name: "default", Redis: RedisConfig{URL: "redis://default:6379"}},
		WithEnvBinding("REDIS_URL", "redis.url"),
	)
	require.NoError(t, err)
	assert.Equal(t, "redis://test:6379", cfg.Redis.URL)
}

func TestLoadWithHyphenInKoanfKey(t *testing.T) {
	type ClientConfig struct {
		ServerPassword string `koanf:"server-password"`
		ServerHost     string `koanf:"server-host"`
	}
	type Config struct {
		Client ClientConfig `koanf:"client"`
	}

	t.Setenv("CLIENT_SERVER_PASSWORD", "secret123")
	t.Setenv("MY_SERVER_HOST", "test-host")

	cfg, err := Load(
		Config{Client: ClientConfig{ServerPassword: "default-password", ServerHost: "default-host"}},
		WithEnvBinding("CLIENT_SERVER_PASSWORD", "client.server-password"),
		WithEnvBinding("MY_SERVER_HOST", "client.server-host"),
	)
	require.NoError(t, err)

	assert.Equal(t, "secret123", cfg.Client.ServerPassword)
	assert.Equal(t, "test-host", cfg.Client.ServerHost)
}

func TestLoadWithEnvBindKey(t *testing.T) {
	type ClientConfig struct {
		ServerPassword string `koanf:"server-password"`
	}
	type Config struct {
		Name   string       `koanf:"name"`
		Client ClientConfig `koanf:"client"`
	}

	tmpFile := writeTempConfig(t, `
envbind:
  CLIENT_PWD: client.server-password
name: "from-config"
client:
  server-password: "file-password"
`)

	t.Setenv("CLIENT_PWD", "env-password")

	cfg, err := Load(
		Config{Name: "default", Client: ClientConfig{ServerPassword: "default-password"}},
		WithConfigPaths(tmpFile),
		WithEnvBindKey("envbind"),
	)
	require.NoError(t, err)

	assert.Equal(t, "env-password", cfg.Client.ServerPassword, "env should override config file")
	assert.Equal(t, "from-config", cfg.Name)
}

func TestEnvBindingPriority(t *testing.T) {
	type Config struct {
		Password string `koanf:"password"`
	}

	tmpFile := writeTempConfig(t, `
envbind:
  FILE_PWD: password
password: "file-value"
`)

	t.Setenv("FILE_PWD", "from-file-binding")
	t.Setenv("CODE_PWD", "from-code-binding")

	cfg, err := Load(
		Config{Password: "default"},
		WithConfigPaths(tmpFile),
		WithEnvBindKey("envbind"),
		WithEnvBinding("CODE_PWD", "password"), // 代码绑定优先级更高
	)
	require.NoError(t, err)
	assert.Equal(t, "from-code-binding", cfg.Password)
}

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

	t.Setenv("TEST_NAME", "from-env")
	t.Setenv("TEST_CLIENT_SERVER_PASSWORD", "secret123")
	t.Setenv("TEST_CLIENT_SERVER_HOST", "test-host")
	t.Setenv("TEST_CLIENT_TIMEOUT", "30")

	cfg, err := Load(
		Config{Name: "default", Client: ClientConfig{ServerPassword: "default", ServerHost: "default", Timeout: 10}},
		WithEnvPrefix("TEST_"),
	)
	require.NoError(t, err)

	a := assert.New(t)
	a.Equal("from-env", cfg.Name)
	a.Equal("secret123", cfg.Client.ServerPassword, "hyphenated key should work")
	a.Equal("test-host", cfg.Client.ServerHost, "hyphenated key should work")
	a.Equal(30, cfg.Client.Timeout)
}

func TestLoadPriority(t *testing.T) {
	type Config struct {
		Value1 string `koanf:"value1"`
		Value2 string `koanf:"value2"`
		Value3 string `koanf:"value3"`
	}

	tmpFile := writeTempConfig(t, `
value1: "from-file"
value2: "from-file"
value3: "from-file"
`)

	t.Setenv("TEST_VALUE2", "from-env-prefix")
	t.Setenv("TEST_VALUE3", "from-env-prefix")
	t.Setenv("BOUND_VALUE3", "from-env-binding")

	cfg, err := Load(
		Config{Value1: "default", Value2: "default", Value3: "default"},
		WithConfigPaths(tmpFile),
		WithEnvPrefix("TEST_"),
		WithEnvBinding("BOUND_VALUE3", "value3"),
	)
	require.NoError(t, err)

	a := assert.New(t)
	a.Equal("from-file", cfg.Value1, "config file > default")
	a.Equal("from-env-prefix", cfg.Value2, "env prefix > config file")
	a.Equal("from-env-binding", cfg.Value3, "env binding > env prefix")
}

func TestLoadWithDefaultsOnly(t *testing.T) {
	type Config struct {
		Name  string `koanf:"name"`
		Debug bool   `koanf:"debug"`
		Port  int    `koanf:"port"`
	}

	cfg, err := Load(Config{Name: "my-app", Debug: true, Port: 8080})
	require.NoError(t, err)

	a := assert.New(t)
	a.Equal("my-app", cfg.Name)
	a.True(cfg.Debug)
	a.Equal(8080, cfg.Port)
}

func TestLoadWithNonExistentConfigFile(t *testing.T) {
	type Config struct {
		Name string `koanf:"name"`
	}

	cfg, err := Load(Config{Name: "fallback-app"}, WithConfigPaths("/nonexistent/path/config.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "fallback-app", cfg.Name)
}

// TestLoadWithConfigFileOnly 测试纯配置文件读取 (cmd=nil, 无环境变量)
// 验证当只使用配置文件时，Load 函数能正确解析并覆盖默认值
func TestLoadWithConfigFileOnly(t *testing.T) {
	type ServerConfig struct {
		Host    string        `koanf:"host"`
		Port    int           `koanf:"port"`
		Timeout time.Duration `koanf:"timeout"`
	}
	type Config struct {
		Name   string       `koanf:"name"`
		Debug  bool         `koanf:"debug"`
		Server ServerConfig `koanf:"server"`
	}

	tmpFile := writeTempConfig(t, `
name: "from-file"
debug: true
server:
  host: "0.0.0.0"
  port: 9090
  timeout: 60s
`)

	// cmd=nil, 只有配置文件，没有 WithCommand/WithEnvPrefix/WithEnvBinding
	cfg, err := Load(
		Config{
			Name:  "default-app",
			Debug: false,
			Server: ServerConfig{
				Host:    "localhost",
				Port:    8080,
				Timeout: 30 * time.Second,
			},
		},
		WithConfigPaths(tmpFile),
	)
	require.NoError(t, err)

	a := assert.New(t)
	a.Equal("from-file", cfg.Name, "config file should override default")
	a.True(cfg.Debug, "config file should override default")
	a.Equal("0.0.0.0", cfg.Server.Host, "nested config should be loaded")
	a.Equal(9090, cfg.Server.Port, "nested config should be loaded")
	a.Equal(60*time.Second, cfg.Server.Timeout, "duration should be parsed correctly")
}

// TestLoadWithConfigFilePartialOverride 测试配置文件部分覆盖
// 验证配置文件只覆盖指定字段，未指定字段保持默认值
func TestLoadWithConfigFilePartialOverride(t *testing.T) {
	type Config struct {
		Name    string `koanf:"name"`
		Debug   bool   `koanf:"debug"`
		Port    int    `koanf:"port"`
		Timeout int    `koanf:"timeout"`
	}

	tmpFile := writeTempConfig(t, `
name: "partial-override"
port: 9000
`)

	cfg, err := Load(
		Config{Name: "default", Debug: true, Port: 8080, Timeout: 30},
		WithConfigPaths(tmpFile),
	)
	require.NoError(t, err)

	a := assert.New(t)
	a.Equal("partial-override", cfg.Name, "specified field should be overridden")
	a.True(cfg.Debug, "unspecified field should keep default (bool)")
	a.Equal(9000, cfg.Port, "specified field should be overridden")
	a.Equal(30, cfg.Timeout, "unspecified field should keep default (int)")
}

// TestLoadWithBaseDir 测试路径基准目录功能
func TestLoadWithBaseDir(t *testing.T) {
	type ServerConfig struct {
		Addr string `koanf:"addr"`
	}
	type Config struct {
		Server ServerConfig `koanf:"server"`
	}

	t.Run("default uses project root", func(t *testing.T) {
		// 默认行为：相对路径基于项目根目录
		cfg, err := Load(
			Config{Server: ServerConfig{Addr: "default"}},
			WithConfigPaths("config/config.example.yaml"),
		)
		require.NoError(t, err)
		assert.Equal(t, ":8080", cfg.Server.Addr)
	})

	t.Run("WithBaseDir empty uses cwd", func(t *testing.T) {
		// WithBaseDir("") 使用当前工作目录
		cfg, err := Load(
			Config{Server: ServerConfig{Addr: "fallback"}},
			WithBaseDir(""),
			WithConfigPaths("nonexistent.yaml"),
		)
		require.NoError(t, err)
		assert.Equal(t, "fallback", cfg.Server.Addr)
	})

	t.Run("absolute path unchanged", func(t *testing.T) {
		tmpFile := writeTempConfig(t, `server: {addr: ":9090"}`)
		cfg, err := Load(
			Config{Server: ServerConfig{Addr: "default"}},
			WithConfigPaths(tmpFile),
		)
		require.NoError(t, err)
		assert.Equal(t, ":9090", cfg.Server.Addr)
	})
}

// =============================================================================
// CLI Flags 测试 (github.com/urfave/cli/v3)
// =============================================================================

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

	defaultCfg := Config{Name: "default-app", Debug: false, Server: ServerConfig{Addr: ":8080", Timeout: 30 * time.Second}}
	flags := []cli.Flag{
		&cli.StringFlag{Name: "name", Value: defaultCfg.Name},
		&cli.BoolFlag{Name: "debug", Value: defaultCfg.Debug},
		&cli.StringFlag{Name: "server-addr", Value: defaultCfg.Server.Addr},
		&cli.DurationFlag{Name: "server-timeout", Value: defaultCfg.Server.Timeout},
	}

	cfg := runCLITest(t, defaultCfg, flags, []string{"test", "--name", "cli-app", "--debug"})

	a := assert.New(t)
	a.Equal("cli-app", cfg.Name, "CLI flag should override")
	a.True(cfg.Debug, "CLI flag should override")
	a.Equal(":8080", cfg.Server.Addr, "unset flag keeps default")
	a.Equal(30*time.Second, cfg.Server.Timeout, "unset flag keeps default")
}

func TestLoadWithCommand_NestedFlags(t *testing.T) {
	type ServerConfig struct {
		Addr    string        `koanf:"addr"`
		Timeout time.Duration `koanf:"timeout"`
	}
	type Config struct {
		Server ServerConfig `koanf:"server"`
	}

	defaultCfg := Config{Server: ServerConfig{Addr: ":8080", Timeout: 30 * time.Second}}
	flags := []cli.Flag{
		&cli.StringFlag{Name: "server-addr", Value: defaultCfg.Server.Addr},
		&cli.DurationFlag{Name: "server-timeout", Value: defaultCfg.Server.Timeout},
	}

	cfg := runCLITest(t, defaultCfg, flags, []string{"test", "--server-addr", ":9090", "--server-timeout", "60s"})

	assert.Equal(t, ":9090", cfg.Server.Addr)
	assert.Equal(t, 60*time.Second, cfg.Server.Timeout)
}

func TestLoadWithCommand_SubCommands(t *testing.T) {
	type Config struct {
		URL     string `koanf:"url"`
		Timeout int    `koanf:"timeout"`
	}

	defaultCfg := Config{URL: "http://localhost:8080", Timeout: 30}
	var loadedCfg *Config
	var executed bool

	subCmd := &cli.Command{
		Name: "health",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cfg, err := Load(defaultCfg, WithCommand(cmd))
			if err != nil {
				return err
			}
			loadedCfg = cfg
			executed = true
			return nil
		},
	}

	mainCmd := &cli.Command{
		Name:     "client",
		Commands: []*cli.Command{subCmd},
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "url", Value: defaultCfg.URL},
			&cli.IntFlag{Name: "timeout", Value: defaultCfg.Timeout},
		},
	}

	err := mainCmd.Run(context.Background(), []string{"client", "--url", "http://prod:8080", "health"})
	require.NoError(t, err)
	require.True(t, executed)

	assert.Equal(t, "http://prod:8080", loadedCfg.URL, "parent flag inherited")
	assert.Equal(t, 30, loadedCfg.Timeout, "unset keeps default")
}

func TestLoadWithCommand_Priority(t *testing.T) {
	type Config struct {
		Value string `koanf:"value"`
	}

	t.Setenv("TEST_VALUE", "from-env")

	defaultCfg := Config{Value: "default"}
	flags := []cli.Flag{&cli.StringFlag{Name: "value", Value: defaultCfg.Value}}

	cfg := runCLITest(t, defaultCfg, flags, []string{"test", "--value", "from-cli"}, WithEnvPrefix("TEST_"))

	assert.Equal(t, "from-cli", cfg.Value, "CLI > env")
}

func TestLoadWithCommand_OnlySetFlags(t *testing.T) {
	type Config struct {
		Name  string `koanf:"name"`
		Debug bool   `koanf:"debug"`
	}

	tmpFile := writeTempConfig(t, `
name: "from-file"
debug: true
`)

	defaultCfg := Config{Name: "default", Debug: false}
	flags := []cli.Flag{
		&cli.StringFlag{Name: "name", Value: defaultCfg.Name},
		&cli.BoolFlag{Name: "debug", Value: defaultCfg.Debug},
	}

	cfg := runCLITest(t, defaultCfg, flags, []string{"test", "--name", "from-cli"}, WithConfigPaths(tmpFile))

	assert.Equal(t, "from-cli", cfg.Name, "set flag uses CLI value")
	assert.True(t, cfg.Debug, "unset flag keeps config file value")
}

func TestLoadWithCommand_NumericTypes(t *testing.T) {
	type Config struct {
		Port    int     `koanf:"port"`
		Rate    float64 `koanf:"rate"`
		Retries uint    `koanf:"retries"`
	}

	defaultCfg := Config{Port: 8080, Rate: 1.0, Retries: 3}
	flags := []cli.Flag{
		&cli.IntFlag{Name: "port", Value: defaultCfg.Port},
		&cli.Float64Flag{Name: "rate", Value: defaultCfg.Rate},
		&cli.UintFlag{Name: "retries", Value: uint(defaultCfg.Retries)},
	}

	cfg := runCLITest(t, defaultCfg, flags, []string{"test", "--port", "9090", "--rate", "2.5", "--retries", "5"})

	a := assert.New(t)
	a.Equal(9090, cfg.Port)
	a.Equal(2.5, cfg.Rate)
	a.Equal(uint(5), cfg.Retries)
}

func TestLoadWithCommand_StringSlice(t *testing.T) {
	type Config struct {
		Hosts []string `koanf:"hosts"`
	}

	defaultCfg := Config{Hosts: []string{"localhost"}}
	flags := []cli.Flag{&cli.StringSliceFlag{Name: "hosts", Value: defaultCfg.Hosts}}

	cfg := runCLITest(t, defaultCfg, flags, []string{"test", "--hosts", "host1", "--hosts", "host2", "--hosts", "host3"})

	assert.Equal(t, []string{"host1", "host2", "host3"}, cfg.Hosts)
}

// =============================================================================
// GenerateExampleYAML 测试
// =============================================================================

func TestGenerateExampleYAML(t *testing.T) {
	tests := []struct {
		name     string
		cfg      any
		contains []string
		excludes []string
	}{
		{
			name: "basic types",
			cfg: struct {
				Name    string  `koanf:"name" desc:"应用名称"`
				Debug   bool    `koanf:"debug" desc:"调试模式"`
				Port    int     `koanf:"port" desc:"端口号"`
				Rate    float64 `koanf:"rate" desc:"速率"`
				Retries uint    `koanf:"retries" desc:"重试次数"`
			}{Name: "test-app", Debug: true, Port: 8080, Rate: 1.5, Retries: 3},
			contains: []string{
				"# 配置示例文件",
				`name: "test-app"`, "# 应用名称",
				"debug: true", "# 调试模式",
				"port: 8080",
				"rate: 1.5",
				"retries: 3",
			},
		},
		{
			name: "nested struct",
			cfg: struct {
				Name   string `koanf:"name" desc:"应用名称"`
				Server struct {
					Host string `koanf:"host" desc:"服务器地址"`
					Port int    `koanf:"port" desc:"服务器端口"`
				} `koanf:"server" desc:"服务器配置"`
			}{
				Name: "nested-app",
				Server: struct {
					Host string `koanf:"host" desc:"服务器地址"`
					Port int    `koanf:"port" desc:"服务器端口"`
				}{Host: "localhost", Port: 9090},
			},
			contains: []string{"server:", `host: "localhost"`, "port: 9090", "# 服务器配置"},
		},
		{
			name: "duration",
			cfg: struct {
				Timeout time.Duration `koanf:"timeout" desc:"超时时间"`
			}{Timeout: 30 * time.Second},
			contains: []string{"timeout: 30s", "# 超时时间"},
		},
		{
			name: "slice",
			cfg: struct {
				Hosts []string `koanf:"hosts" desc:"主机列表"`
				Empty []string `koanf:"empty" desc:"空列表"`
			}{Hosts: []string{"host1", "host2"}, Empty: []string{}},
			contains: []string{"hosts:", "- host1", "- host2", "empty: []"},
		},
		{
			name: "map",
			cfg: struct {
				Labels map[string]string `koanf:"labels" desc:"标签"`
				Empty  map[string]string `koanf:"empty" desc:"空映射"`
			}{Labels: map[string]string{"env": "prod"}, Empty: map[string]string{}},
			contains: []string{"labels:", "empty: {}"},
		},
		{
			name: "skip untagged",
			cfg: struct {
				Name     string `koanf:"name" desc:"应用名称"`
				Internal string // 无 koanf 标签
			}{Name: "test", Internal: "should-not-appear"},
			contains: []string{"name:"},
			excludes: []string{"should-not-appear", "Internal"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := string(GenerateExampleYAML(tt.cfg))
			a := assert.New(t)
			for _, s := range tt.contains {
				a.Contains(yaml, s)
			}
			for _, s := range tt.excludes {
				a.NotContains(yaml, s)
			}
		})
	}
}

// =============================================================================
// DefaultPaths 测试
// =============================================================================

func TestDefaultPaths(t *testing.T) {
	tests := []struct {
		name        string
		appName     string
		minLen      int
		mustContain []string
	}{
		{
			name:        "no app name",
			appName:     "",
			minLen:      2,
			mustContain: []string{"config.yaml", "config/config.yaml"},
		},
		{
			name:        "with app name",
			appName:     "myapp",
			minLen:      4,
			mustContain: []string{".myapp.yaml", "/etc/myapp/config.yaml", "config.yaml", "config/config.yaml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var paths []string
			if tt.appName == "" {
				paths = DefaultPaths()
			} else {
				paths = DefaultPaths(tt.appName)
			}

			a := assert.New(t)
			a.GreaterOrEqual(len(paths), tt.minLen)
			for _, p := range tt.mustContain {
				a.Contains(paths, p)
			}
		})
	}
}

// =============================================================================
// FindProjectRoot 测试
// =============================================================================

func TestFindProjectRoot(t *testing.T) {
	root, err := FindProjectRoot(0)
	require.NoError(t, err)
	assert.NotEmpty(t, root)

	_, err = os.Stat(root + "/go.mod")
	assert.NoError(t, err, "should contain go.mod")
}

// =============================================================================
// collectKoanfKeys 测试 (内部函数)
// =============================================================================

func TestCollectKoanfKeys(t *testing.T) {
	tests := []struct {
		name     string
		cfg      any
		expected []string
	}{
		{
			name: "flat",
			cfg: struct {
				Name  string `koanf:"name"`
				Debug bool   `koanf:"debug"`
				Port  int    `koanf:"port"`
			}{},
			expected: []string{"name", "debug", "port"},
		},
		{
			name: "nested",
			cfg: struct {
				Name   string `koanf:"name"`
				Server struct {
					Host string `koanf:"host"`
					Port int    `koanf:"port"`
				} `koanf:"server"`
			}{},
			expected: []string{"name", "server.host", "server.port"},
		},
		{
			name: "hyphenated keys",
			cfg: struct {
				Client struct {
					ServerPassword string `koanf:"server-password"`
					RevAuthUser    string `koanf:"rev-auth-user"`
				} `koanf:"client"`
			}{},
			expected: []string{"client.server-password", "client.rev-auth-user"},
		},
		{
			name: "duration not recursed",
			cfg: struct {
				Timeout  time.Duration `koanf:"timeout"`
				Interval time.Duration `koanf:"interval"`
			}{},
			expected: []string{"timeout", "interval"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := collectKoanfKeys(tt.cfg)
			a := assert.New(t)
			a.Len(keys, len(tt.expected))
			for _, k := range tt.expected {
				a.Contains(keys, k)
			}
		})
	}
}

// =============================================================================
// generateEnvBindings 测试 (内部函数)
// =============================================================================

func TestGenerateEnvBindings(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		keys     []string
		expected map[string]string
	}{
		{
			name:   "basic",
			prefix: "APP_",
			keys:   []string{"name", "debug", "port"},
			expected: map[string]string{
				"APP_NAME":  "name",
				"APP_DEBUG": "debug",
				"APP_PORT":  "port",
			},
		},
		{
			name:   "nested",
			prefix: "MYAPP_",
			keys:   []string{"server.host", "server.port", "client.url"},
			expected: map[string]string{
				"MYAPP_SERVER_HOST": "server.host",
				"MYAPP_SERVER_PORT": "server.port",
				"MYAPP_CLIENT_URL":  "client.url",
			},
		},
		{
			name:   "hyphenated",
			prefix: "APP_",
			keys:   []string{"client.server-password", "client.rev-auth-user"},
			expected: map[string]string{
				"APP_CLIENT_SERVER_PASSWORD": "client.server-password",
				"APP_CLIENT_REV_AUTH_USER":   "client.rev-auth-user",
			},
		},
		{
			name:   "empty prefix",
			prefix: "",
			keys:   []string{"name", "server.port"},
			expected: map[string]string{
				"NAME":        "name",
				"SERVER_PORT": "server.port",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bindings := generateEnvBindings(tt.prefix, tt.keys)
			assert.Equal(t, tt.expected, bindings)
		})
	}
}

// =============================================================================
// 模板展开测试 (默认启用)
// =============================================================================

func TestTemplateExpansion(t *testing.T) {
	type Config struct {
		APIKey  string `koanf:"api_key"`
		Model   string `koanf:"model"`
		BaseURL string `koanf:"base_url"`
	}

	t.Run("env function", func(t *testing.T) {
		t.Setenv("TEST_API_KEY", "sk-test-12345")

		configContent := `
api_key: '{{env "TEST_API_KEY"}}'
model: "gpt-4"
base_url: "https://api.openai.com"
`
		configPath := writeTempConfig(t, configContent)
		cfg, err := Load(Config{}, WithConfigPaths(configPath)) // 默认启用模板展开
		require.NoError(t, err)

		assert.Equal(t, "sk-test-12345", cfg.APIKey)
		assert.Equal(t, "gpt-4", cfg.Model)
	})

	t.Run("env with default value", func(t *testing.T) {
		configContent := `
api_key: '{{env "NONEXISTENT_KEY" "default-key"}}'
model: "gpt-3.5-turbo"
`
		configPath := writeTempConfig(t, configContent)
		cfg, err := Load(Config{}, WithConfigPaths(configPath))
		require.NoError(t, err)

		assert.Equal(t, "default-key", cfg.APIKey)
	})

	t.Run("default function pipeline", func(t *testing.T) {
		configContent := `
api_key: '{{env "NONEXISTENT_KEY" | default "fallback-key"}}'
model: "claude-3"
`
		configPath := writeTempConfig(t, configContent)
		cfg, err := Load(Config{}, WithConfigPaths(configPath))
		require.NoError(t, err)

		assert.Equal(t, "fallback-key", cfg.APIKey)
	})

	t.Run("taskfile style direct access", func(t *testing.T) {
		t.Setenv("MY_MODEL", "claude-haiku")
		t.Setenv("MY_BASE_URL", "https://api.anthropic.com")

		configContent := `
api_key: "test-key"
model: '{{.MY_MODEL}}'
base_url: '{{.MY_BASE_URL | default "https://default.com"}}'
`
		configPath := writeTempConfig(t, configContent)
		cfg, err := Load(Config{}, WithConfigPaths(configPath))
		require.NoError(t, err)

		assert.Equal(t, "claude-haiku", cfg.Model)
		assert.Equal(t, "https://api.anthropic.com", cfg.BaseURL)
	})

	t.Run("coalesce function", func(t *testing.T) {
		t.Setenv("SECONDARY_KEY", "secondary-value")

		configContent := `
api_key: '{{coalesce .PRIMARY_KEY .SECONDARY_KEY "final-default"}}'
model: "test"
`
		configPath := writeTempConfig(t, configContent)
		cfg, err := Load(Config{}, WithConfigPaths(configPath))
		require.NoError(t, err)

		assert.Equal(t, "secondary-value", cfg.APIKey)
	})

	t.Run("coalesce all empty returns default", func(t *testing.T) {
		configContent := `
api_key: '{{coalesce .MISSING1 .MISSING2 "final-default"}}'
model: "test"
`
		configPath := writeTempConfig(t, configContent)
		cfg, err := Load(Config{}, WithConfigPaths(configPath))
		require.NoError(t, err)

		assert.Equal(t, "final-default", cfg.APIKey)
	})

	t.Run("WithoutTemplateExpansion disables expansion", func(t *testing.T) {
		configContent := `
api_key: '{{env "TEST_KEY"}}'
model: "test"
`
		configPath := writeTempConfig(t, configContent)
		cfg, err := Load(Config{}, WithConfigPaths(configPath), WithoutTemplateExpansion()) // 显式禁用
		require.NoError(t, err)

		// 未展开，保持原样
		assert.Equal(t, "{{env \"TEST_KEY\"}}", cfg.APIKey)
	})

	t.Run("template syntax error", func(t *testing.T) {
		configContent := `
api_key: '{{env "TEST_KEY"'
model: "test"
`
		configPath := writeTempConfig(t, configContent)
		_, err := Load(Config{}, WithConfigPaths(configPath)) // 默认启用，语法错误会报错
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expand template")
	})

	t.Run("complex real-world example", func(t *testing.T) {
		t.Setenv("OPENROUTER_API_KEY", "or-key-12345")
		t.Setenv("LLM_MODEL", "anthropic/claude-haiku-4.5")

		configContent := `
api_key: '{{coalesce .OPENROUTER_API_KEY .ANTHROPIC_API_KEY "sk-default"}}'
model: '{{.LLM_MODEL | default "gpt-4"}}'
base_url: '{{env "LLM_BASE_URL" "https://openrouter.ai/api/v1"}}'
`
		configPath := writeTempConfig(t, configContent)
		cfg, err := Load(Config{}, WithConfigPaths(configPath))
		require.NoError(t, err)

		assert.Equal(t, "or-key-12345", cfg.APIKey)
		assert.Equal(t, "anthropic/claude-haiku-4.5", cfg.Model)
		assert.Equal(t, "https://openrouter.ai/api/v1", cfg.BaseURL)
	})
}

// =============================================================================
// JSON 格式支持测试
// =============================================================================

// writeTempJSONConfig 创建临时 JSON 配置文件
func writeTempJSONConfig(t *testing.T, content string) string {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "config_test_*.json")
	require.NoError(t, err, "Failed to create temp file")
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err, "Failed to write temp file")
	_ = tmpFile.Close()
	t.Cleanup(func() { _ = os.Remove(tmpFile.Name()) })
	return tmpFile.Name()
}

func TestLoadWithJSONConfig(t *testing.T) {
	type ServerConfig struct {
		Host    string        `koanf:"host"`
		Port    int           `koanf:"port"`
		Timeout time.Duration `koanf:"timeout"`
	}
	type Config struct {
		Name   string       `koanf:"name"`
		Debug  bool         `koanf:"debug"`
		Server ServerConfig `koanf:"server"`
	}

	jsonContent := `{
  "name": "json-app",
  "debug": true,
  "server": {
    "host": "0.0.0.0",
    "port": 9090,
    "timeout": "60s"
  }
}`

	tmpFile := writeTempJSONConfig(t, jsonContent)

	cfg, err := Load(
		Config{Name: "default", Server: ServerConfig{Port: 8080}},
		WithConfigPaths(tmpFile),
	)
	require.NoError(t, err)

	a := assert.New(t)
	a.Equal("json-app", cfg.Name)
	a.True(cfg.Debug)
	a.Equal("0.0.0.0", cfg.Server.Host)
	a.Equal(9090, cfg.Server.Port)
	a.Equal(60*time.Second, cfg.Server.Timeout)
}

func TestParserForPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		isJSON   bool
	}{
		{"yaml extension", "config.yaml", false},
		{"yml extension", "config.yml", false},
		{"json extension", "config.json", true},
		{"uppercase YAML", "CONFIG.YAML", false},
		{"uppercase JSON", "CONFIG.JSON", true},
		{"no extension", "config", false},
		{"unknown extension", "config.conf", false},
		{"json in path", "/path/to/config.json", true},
		{"yaml in path", "/etc/app/config.yaml", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := parserForPath(tt.path)
			// 通过解析简单内容来验证 parser 类型
			if tt.isJSON {
				// JSON parser 应该能解析 JSON
				result, err := parser.Unmarshal([]byte(`{"key": "value"}`))
				require.NoError(t, err)
				assert.Equal(t, "value", result["key"])
			} else {
				// YAML parser 应该能解析 YAML
				result, err := parser.Unmarshal([]byte("key: value"))
				require.NoError(t, err)
				assert.Equal(t, "value", result["key"])
			}
		})
	}
}

func TestJSONTemplateExpansion(t *testing.T) {
	type Config struct {
		APIKey string `koanf:"api_key"`
		Model  string `koanf:"model"`
	}

	t.Run("env function in JSON", func(t *testing.T) {
		t.Setenv("JSON_TEST_KEY", "sk-json-12345")

		// 使用反引号包裹模板参数，避免 JSON 转义引号与模板语法冲突
		jsonContent := "{\"api_key\": \"{{env `JSON_TEST_KEY`}}\", \"model\": \"gpt-4\"}"
		tmpFile := writeTempJSONConfig(t, jsonContent)
		cfg, err := Load(Config{}, WithConfigPaths(tmpFile))
		require.NoError(t, err)

		assert.Equal(t, "sk-json-12345", cfg.APIKey)
		assert.Equal(t, "gpt-4", cfg.Model)
	})

	t.Run("default value in JSON", func(t *testing.T) {
		// 使用反引号包裹模板参数
		jsonContent := "{\"api_key\": \"{{env `NONEXISTENT` `default-json-key`}}\", \"model\": \"claude-3\"}"
		tmpFile := writeTempJSONConfig(t, jsonContent)
		cfg, err := Load(Config{}, WithConfigPaths(tmpFile))
		require.NoError(t, err)

		assert.Equal(t, "default-json-key", cfg.APIKey)
	})

	t.Run("disable template in JSON", func(t *testing.T) {
		jsonContent := "{\"api_key\": \"{{env `TEST`}}\", \"model\": \"test\"}"
		tmpFile := writeTempJSONConfig(t, jsonContent)
		cfg, err := Load(Config{}, WithConfigPaths(tmpFile), WithoutTemplateExpansion())
		require.NoError(t, err)

		assert.Equal(t, "{{env `TEST`}}", cfg.APIKey)
	})
}

func TestJSONPartialOverride(t *testing.T) {
	type Config struct {
		Name    string `koanf:"name"`
		Debug   bool   `koanf:"debug"`
		Port    int    `koanf:"port"`
		Timeout int    `koanf:"timeout"`
	}

	jsonContent := `{
  "name": "json-override",
  "port": 9000
}`

	tmpFile := writeTempJSONConfig(t, jsonContent)
	cfg, err := Load(
		Config{Name: "default", Debug: true, Port: 8080, Timeout: 30},
		WithConfigPaths(tmpFile),
	)
	require.NoError(t, err)

	a := assert.New(t)
	a.Equal("json-override", cfg.Name, "specified field should be overridden")
	a.True(cfg.Debug, "unspecified field should keep default")
	a.Equal(9000, cfg.Port, "specified field should be overridden")
	a.Equal(30, cfg.Timeout, "unspecified field should keep default")
}

func TestJSONWithEnvPrefix(t *testing.T) {
	type Config struct {
		Name  string `koanf:"name"`
		Debug bool   `koanf:"debug"`
	}

	jsonContent := `{
  "name": "json-app",
  "debug": false
}`

	tmpFile := writeTempJSONConfig(t, jsonContent)
	t.Setenv("JSONTEST_DEBUG", "true")

	cfg, err := Load(
		Config{Name: "default", Debug: false},
		WithConfigPaths(tmpFile),
		WithEnvPrefix("JSONTEST_"),
	)
	require.NoError(t, err)

	a := assert.New(t)
	a.Equal("json-app", cfg.Name, "from JSON file")
	a.True(cfg.Debug, "env should override JSON file")
}
