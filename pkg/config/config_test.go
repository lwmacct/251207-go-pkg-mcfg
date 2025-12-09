package config

import (
	"os"
	"testing"
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
			if result != tt.expected {
				t.Errorf("envKeyDecoder(%q)(%q) = %q, want %q", tt.prefix, tt.input, result, tt.expected)
			}
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
	os.Setenv("TEST_DEBUG", "true")
	os.Setenv("TEST_SERVER_URL", "http://test:8080")
	defer os.Unsetenv("TEST_DEBUG")
	defer os.Unsetenv("TEST_SERVER_URL")

	defaultCfg := Config{
		Debug: false,
		Server: ServerConfig{
			URL: "http://default:8080",
		},
	}

	cfg, err := Load(defaultCfg, WithEnvPrefix("TEST_"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if !cfg.Debug {
		t.Errorf("Debug = %v, want true", cfg.Debug)
	}
	if cfg.Server.URL != "http://test:8080" {
		t.Errorf("Server.URL = %q, want %q", cfg.Server.URL, "http://test:8080")
	}
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
	os.Setenv("REDIS_URL", "redis://test:6379")
	defer os.Unsetenv("REDIS_URL")

	defaultCfg := Config{
		Name: "default",
		Redis: RedisConfig{
			URL: "redis://default:6379",
		},
	}

	cfg, err := Load(defaultCfg,
		WithEnvBinding("REDIS_URL", "redis.url"),
	)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Redis.URL != "redis://test:6379" {
		t.Errorf("Redis.URL = %q, want %q", cfg.Redis.URL, "redis://test:6379")
	}
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
	os.Setenv("CLIENT_SERVER_PASSWORD", "secret123")
	os.Setenv("MY_SERVER_HOST", "test-host")
	defer os.Unsetenv("CLIENT_SERVER_PASSWORD")
	defer os.Unsetenv("MY_SERVER_HOST")

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
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Client.ServerPassword != "secret123" {
		t.Errorf("Client.ServerPassword = %q, want %q", cfg.Client.ServerPassword, "secret123")
	}
	if cfg.Client.ServerHost != "test-host" {
		t.Errorf("Client.ServerHost = %q, want %q", cfg.Client.ServerHost, "test-host")
	}
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
	if err := os.WriteFile(tmpFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile)

	// 设置环境变量
	os.Setenv("CLIENT_PWD", "env-password")
	defer os.Unsetenv("CLIENT_PWD")

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
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 环境变量应该覆盖配置文件中的值
	if cfg.Client.ServerPassword != "env-password" {
		t.Errorf("Client.ServerPassword = %q, want %q", cfg.Client.ServerPassword, "env-password")
	}
	if cfg.Name != "from-config" {
		t.Errorf("Name = %q, want %q", cfg.Name, "from-config")
	}
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
	if err := os.WriteFile(tmpFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile)

	// 设置两个环境变量
	os.Setenv("FILE_PWD", "from-file-binding")
	os.Setenv("CODE_PWD", "from-code-binding")
	defer os.Unsetenv("FILE_PWD")
	defer os.Unsetenv("CODE_PWD")

	defaultCfg := Config{
		Password: "default",
	}

	cfg, err := Load(defaultCfg,
		WithConfigPaths(tmpFile),
		WithEnvBindKey("envbind"),
		// 代码绑定优先级更高，应该覆盖配置文件绑定
		WithEnvBinding("CODE_PWD", "password"),
	)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 代码绑定优先级更高
	if cfg.Password != "from-code-binding" {
		t.Errorf("Password = %q, want %q", cfg.Password, "from-code-binding")
	}
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
	os.Setenv("TEST_NAME", "from-env")
	os.Setenv("TEST_CLIENT_SERVER_PASSWORD", "secret123")
	os.Setenv("TEST_CLIENT_SERVER_HOST", "test-host")
	os.Setenv("TEST_CLIENT_TIMEOUT", "30")
	defer os.Unsetenv("TEST_NAME")
	defer os.Unsetenv("TEST_CLIENT_SERVER_PASSWORD")
	defer os.Unsetenv("TEST_CLIENT_SERVER_HOST")
	defer os.Unsetenv("TEST_CLIENT_TIMEOUT")

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
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 验证普通字段
	if cfg.Name != "from-env" {
		t.Errorf("Name = %q, want %q", cfg.Name, "from-env")
	}

	// 验证带连字符的字段（这是自动绑定的核心价值）
	if cfg.Client.ServerPassword != "secret123" {
		t.Errorf("Client.ServerPassword = %q, want %q", cfg.Client.ServerPassword, "secret123")
	}
	if cfg.Client.ServerHost != "test-host" {
		t.Errorf("Client.ServerHost = %q, want %q", cfg.Client.ServerHost, "test-host")
	}
	if cfg.Client.Timeout != 30 {
		t.Errorf("Client.Timeout = %d, want %d", cfg.Client.Timeout, 30)
	}
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
	if err := os.WriteFile(tmpFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile)

	// 设置环境变量
	os.Setenv("TEST_VALUE2", "from-env-prefix")
	os.Setenv("TEST_VALUE3", "from-env-prefix")
	os.Setenv("BOUND_VALUE3", "from-env-binding")
	defer os.Unsetenv("TEST_VALUE2")
	defer os.Unsetenv("TEST_VALUE3")
	defer os.Unsetenv("BOUND_VALUE3")

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
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// value1: 仅配置文件覆盖默认值
	if cfg.Value1 != "from-file" {
		t.Errorf("Value1 = %q, want %q", cfg.Value1, "from-file")
	}

	// value2: 环境变量前缀覆盖配置文件
	if cfg.Value2 != "from-env-prefix" {
		t.Errorf("Value2 = %q, want %q", cfg.Value2, "from-env-prefix")
	}

	// value3: 环境变量绑定覆盖环境变量前缀
	if cfg.Value3 != "from-env-binding" {
		t.Errorf("Value3 = %q, want %q", cfg.Value3, "from-env-binding")
	}
}
