package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
	"github.com/urfave/cli/v3"
)

// loadOptions 配置加载选项。
type loadOptions struct {
	cmd         *cli.Command
	configPaths []string
	baseDir     string // 路径基准目录，用于将相对路径转换为绝对路径
	baseDirSet  bool   // 是否显式设置了 baseDir（区分空字符串和未设置）
	envPrefix   string
	envBindings map[string]string
	envBindKey  string
}

// Option 配置加载选项函数。
type Option func(*loadOptions)

// WithCommand 设置 CLI 命令，用于从 CLI flags 加载配置。
//
// CLI flags 具有最高优先级，仅当用户明确指定时才覆盖其他配置源。
func WithCommand(cmd *cli.Command) Option {
	return func(o *loadOptions) {
		o.cmd = cmd
	}
}

// WithConfigPaths 设置配置文件搜索路径。
//
// 按顺序搜索，找到第一个即停止。可使用 [DefaultPaths] 获取默认路径。
func WithConfigPaths(paths ...string) Option {
	return func(o *loadOptions) {
		o.configPaths = paths
	}
}

// WithBaseDir 设置相对路径的基准目录。
//
// 默认情况下，[Load] 使用项目根目录（go.mod 所在目录）作为基准。
// 使用此选项可覆盖默认行为：
//   - 传入空字符串：使用当前工作目录
//   - 传入自定义路径：使用指定目录
//
// 注意：绝对路径不受影响。
func WithBaseDir(path string) Option {
	return func(o *loadOptions) {
		o.baseDir = path
		o.baseDirSet = true
	}
}

// WithEnvPrefix 设置环境变量前缀。
//
// 启用后，会从环境变量加载配置，优先级高于配置文件，低于 CLI flags。
//
// 环境变量命名规则：
//   - 前缀 + 大写的 koanf key
//   - 点号 (.) 和连字符 (-) 都转为下划线 (_)
//
// 示例 (前缀为 "MYAPP_")：
//   - MYAPP_DEBUG → debug
//   - MYAPP_SERVER_URL → server.url
//   - MYAPP_CLIENT_REV_AUTH_USER → client.rev-auth-user (支持连字符)
//
// 注意：通过反射自动生成所有 koanf key 的绑定，因此支持任意命名的 koanf key。
func WithEnvPrefix(prefix string) Option {
	return func(o *loadOptions) {
		o.envPrefix = prefix
	}
}

// WithEnvBinding 绑定单个环境变量到配置路径。
//
// 用于复用第三方工具的标准环境变量，优先级高于 WithEnvPrefix。
//
// 示例：
//
//	config.WithEnvBinding("REDIS_URL", "redis.url")
//	config.WithEnvBinding("ETCDCTL_ENDPOINTS", "etcd.endpoints")
func WithEnvBinding(envKey, configPath string) Option {
	return func(o *loadOptions) {
		if o.envBindings == nil {
			o.envBindings = make(map[string]string)
		}
		o.envBindings[envKey] = configPath
	}
}

// WithEnvBindings 批量绑定环境变量到配置路径。
//
// 用于复用第三方工具的标准环境变量，优先级高于 WithEnvPrefix。
//
// 示例：
//
//	config.WithEnvBindings(map[string]string{
//	    "REDIS_URL":         "redis.url",
//	    "ETCDCTL_ENDPOINTS": "etcd.endpoints",
//	    "MYSQL_PWD":         "database.password",
//	})
func WithEnvBindings(bindings map[string]string) Option {
	return func(o *loadOptions) {
		if o.envBindings == nil {
			o.envBindings = make(map[string]string)
		}
		for k, v := range bindings {
			o.envBindings[k] = v
		}
	}
}

// WithEnvBindKey 设置配置文件中的环境变量绑定节点名称。
//
// 启用后，会从配置文件的指定节点读取环境变量绑定关系，无需修改代码即可配置映射。
// 配置文件中的绑定优先级低于代码中的 [WithEnvBindings]（代码显式指定更优先）。
//
// 配置文件示例：
//
//	envbind:
//	  REDIS_URL: redis.url
//	  ETCDCTL_ENDPOINTS: etcd.endpoints
//
//	redis:
//	  url: "redis://localhost:6379"
func WithEnvBindKey(key string) Option {
	return func(o *loadOptions) {
		o.envBindKey = key
	}
}

// DefaultPaths 返回默认配置文件搜索路径。
//
// appName 可选，若提供则包含应用专属配置路径。
//
// 搜索优先级 (从高到低)：
//  1. ./.appname.yaml - 当前目录应用配置 (项目级别)
//  2. ~/.appname.yaml - 用户主目录配置
//  3. /etc/appname/config.yaml - 系统级别配置
//  4. config.yaml - 当前目录通用配置
//  5. config/config.yaml - 子目录配置
func DefaultPaths(appName ...string) []string {
	var paths []string

	if len(appName) > 0 && appName[0] != "" {
		name := appName[0]
		// 当前目录应用配置 (最高优先级)
		paths = append(paths, "."+name+".yaml")
		// 用户主目录
		if home, err := os.UserHomeDir(); err == nil {
			paths = append(paths, filepath.Join(home, "."+name+".yaml"))
		}
		// 系统配置目录
		paths = append(paths, "/etc/"+name+"/config.yaml")
	}

	// 当前目录通用配置 (最低优先级)
	paths = append(paths, "config.yaml", "config/config.yaml")

	return paths
}

// Load 加载配置，按优先级合并。
//
// 优先级 (从低到高)：
//  1. 默认值 - 通过 defaultConfig 参数传入
//  2. 配置文件 - 通过 WithConfigPaths 选项设置
//  3. 环境变量(前缀) - 通过 WithEnvPrefix 选项启用
//  4. 环境变量(绑定) - 通过 WithEnvBindKey(配置文件) 或 WithEnvBindings(代码) 设置
//  5. CLI flags - 通过 WithCommand 选项设置，最高优先级
//
// 环境变量绑定优先级：代码中的 WithEnvBindings > 配置文件中的 envBindKey 节点。
//
// 泛型参数 T 为配置结构体类型，必须使用 koanf tag 标记字段。
func Load[T any](defaultConfig T, opts ...Option) (*T, error) {
	// 解析选项
	options := &loadOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// 默认使用项目根目录作为相对路径基准
	if !options.baseDirSet {
		if root, err := FindProjectRoot(1); err == nil {
			options.baseDir = root
		}
	}

	// 默认使用 DefaultPaths 作为配置文件搜索路径
	if len(options.configPaths) == 0 {
		options.configPaths = DefaultPaths()
	}

	k := koanf.New(".")

	// 1️⃣ 加载默认配置 (最低优先级)
	if err := k.Load(structs.Provider(defaultConfig, "koanf"), nil); err != nil {
		return nil, fmt.Errorf("failed to load default config: %w", err)
	}

	// 2️⃣ 加载配置文件 (按顺序搜索，找到第一个即停止)
	configLoaded := false
	paths := options.configPaths
	if options.baseDir != "" {
		paths = make([]string, len(options.configPaths))
		for i, p := range options.configPaths {
			if !filepath.IsAbs(p) {
				paths[i] = filepath.Join(options.baseDir, p)
			} else {
				paths[i] = p
			}
		}
	}
	for _, path := range paths {
		if err := k.Load(file.Provider(path), yaml.Parser()); err == nil {
			slog.Debug("Loaded config from file", "path", path)
			configLoaded = true
			break
		}
	}

	if len(options.configPaths) > 0 && !configLoaded {
		slog.Debug("No config file found, using defaults")
	}

	// 2.5️⃣ 从配置文件读取环境变量绑定 (在加载配置文件后)
	if options.envBindKey != "" {
		if bindings := k.StringMap(options.envBindKey); len(bindings) > 0 {
			// 构建已绑定配置路径的集合（代码中的绑定优先）
			boundPaths := make(map[string]bool)
			for _, configPath := range options.envBindings {
				boundPaths[configPath] = true
			}

			// 合并配置文件绑定（仅当配置路径未被绑定时）
			for envKey, configPath := range bindings {
				if !boundPaths[configPath] {
					if options.envBindings == nil {
						options.envBindings = make(map[string]string)
					}
					options.envBindings[envKey] = configPath
					boundPaths[configPath] = true
				}
			}
			// 删除绑定节点，不污染用户配置
			k.Delete(options.envBindKey)
			slog.Debug("Loaded env bindings from config", "key", options.envBindKey, "count", len(bindings))
		}
	}

	// 3️⃣ 自动生成环境变量绑定 (基于配置结构体的 koanf key)
	// 这解决了 koanf key 包含连字符（如 rev-auth-user）时无法通过前缀匹配的问题
	if options.envPrefix != "" {
		// 构建已绑定配置路径的集合（用户显式绑定优先）
		boundPaths := make(map[string]bool)
		for _, configPath := range options.envBindings {
			boundPaths[configPath] = true
		}

		autoBindings := generateEnvBindings(options.envPrefix, collectKoanfKeys(defaultConfig))
		// 合并自动绑定（仅当配置路径未被绑定时）
		for envKey, configPath := range autoBindings {
			if !boundPaths[configPath] {
				if options.envBindings == nil {
					options.envBindings = make(map[string]string)
				}
				options.envBindings[envKey] = configPath
				boundPaths[configPath] = true
			}
		}
		slog.Debug("Generated auto env bindings", "prefix", options.envPrefix, "count", len(autoBindings))
	}

	// 4️⃣ 加载环境变量绑定 (高于配置文件，低于 CLI flags)
	for envKey, configPath := range options.envBindings {
		if val := os.Getenv(envKey); val != "" {
			_ = k.Set(configPath, val)
			slog.Debug("Loaded env binding", "env", envKey, "path", configPath)
		}
	}

	// 5️⃣ 加载 CLI flags (最高优先级，仅当用户明确指定时)
	if options.cmd != nil {
		applyCLIFlagsGeneric(options.cmd, k, defaultConfig)
	}

	// 解析到结构体
	var cfg T
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

// envKeyDecoder 返回环境变量 key 解码器。
//
// 转换规则：
//  1. 移除前缀
//  2. 转为小写
//  3. 下划线 (_) 转为点号 (.)
//
// 示例：MYAPP_SERVER_URL → server.url
func envKeyDecoder(prefix string) func(string) string {
	return func(key string) string {
		key = strings.TrimPrefix(key, prefix)
		key = strings.ToLower(key)
		key = strings.ReplaceAll(key, "_", ".")
		return key
	}
}

// collectKoanfKeys 通过反射收集配置结构体的所有 koanf key。
//
// 递归遍历结构体字段，返回所有叶子节点的完整 koanf key。
// 例如对于 client.rev-auth-user 这样的嵌套结构，会返回完整路径。
func collectKoanfKeys[T any](defaultConfig T) []string {
	var keys []string
	collectKoanfKeysRecursive(reflect.TypeOf(defaultConfig), "", &keys)
	return keys
}

// collectKoanfKeysRecursive 递归收集 koanf key。
func collectKoanfKeysRecursive(typ reflect.Type, prefix string, keys *[]string) {
	// 处理指针类型
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	if typ.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		koanfKey := field.Tag.Get("koanf")
		if koanfKey == "" {
			continue
		}

		fullKey := koanfKey
		if prefix != "" {
			fullKey = prefix + "." + koanfKey
		}

		// 如果是嵌套结构体（非特殊类型），递归处理
		if field.Type.Kind() == reflect.Struct &&
			field.Type != reflect.TypeOf(time.Duration(0)) &&
			field.Type != reflect.TypeOf(time.Time{}) {
			collectKoanfKeysRecursive(field.Type, fullKey, keys)
			continue
		}

		*keys = append(*keys, fullKey)
	}
}

// generateEnvBindings 根据 koanf key 生成环境变量绑定。
//
// 转换规则：
//   - koanf key 中的 "." 和 "-" 都转为 "_"
//   - 转为大写
//   - 添加前缀
//
// 示例 (前缀 "APP_")：
//   - client.rev-auth-user → APP_CLIENT_REV_AUTH_USER
//   - server.idle-timeout → APP_SERVER_IDLE_TIMEOUT
func generateEnvBindings(prefix string, koanfKeys []string) map[string]string {
	bindings := make(map[string]string, len(koanfKeys))
	for _, key := range koanfKeys {
		// 将 "." 和 "-" 都转为 "_"，然后大写
		envKey := strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(key))
		bindings[prefix+envKey] = key
	}
	return bindings
}

// applyCLIFlagsGeneric 通过反射将用户明确指定的 CLI flags 应用到 koanf 实例。
//
// 自动根据配置结构体的 koanf 标签映射 CLI flag 名称。
//
// 支持两种 CLI flag 格式 (优先使用 kebab-case)：
//   - kebab-case: --server-skip_verify (仅 . 转为 -)
//   - dot notation: --server.skip_verify (保持原样)
//
// 映射示例 (koanf tag → CLI flags)：
//   - server.url → --server-url 或 --server.url
//   - tls.skip_verify → --tls-skip_verify 或 --tls.skip_verify
//
// 支持的类型：
//   - 基本类型: string, bool
//   - 整数类型: int, int8, int16, int32, int64
//   - 无符号整数: uint, uint8, uint16, uint32, uint64
//   - 浮点数: float32, float64
//   - 时间类型: time.Duration, time.Time
//   - 切片类型: []string, []int, []int64, []float64 等
//   - Map 类型: map[string]string
func applyCLIFlagsGeneric[T any](cmd *cli.Command, k *koanf.Koanf, defaultConfig T) {
	applyCLIFlagsRecursive(cmd, k, reflect.TypeOf(defaultConfig), "")
}

// applyCLIFlagsRecursive 递归遍历结构体字段应用 CLI flags。
func applyCLIFlagsRecursive(cmd *cli.Command, k *koanf.Koanf, typ reflect.Type, prefix string) {
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		// 获取 koanf 标签作为配置 key
		koanfKey := field.Tag.Get("koanf")
		if koanfKey == "" {
			continue
		}

		// 构建完整的 koanf key
		fullKoanfKey := koanfKey
		if prefix != "" {
			fullKoanfKey = prefix + "." + koanfKey
		}

		// 如果是嵌套结构体，递归处理
		if field.Type.Kind() == reflect.Struct &&
			field.Type != reflect.TypeOf(time.Duration(0)) &&
			field.Type != reflect.TypeOf(time.Time{}) {
			applyCLIFlagsRecursive(cmd, k, field.Type, fullKoanfKey)
			continue
		}

		// 检测用户设置的 flag 格式 (kebab-case 或 dot notation)
		cliFlag, isSet := detectCLIFlag(cmd, fullKoanfKey)
		if !isSet {
			continue
		}

		// 根据字段类型获取值并设置
		setCLIFlagValue(cmd, k, fullKoanfKey, cliFlag, field.Type)
	}
}

// detectCLIFlag 检测用户设置的 CLI flag 格式。
//
// 支持两种格式：kebab-case (server-skip_verify) 和 dot notation (server.skip_verify)。
// 返回实际设置的 flag 名称和是否被设置。
func detectCLIFlag(cmd *cli.Command, koanfKey string) (string, bool) {
	// 生成 kebab-case 格式: server.skip_verify -> server-skip_verify
	kebabFlag := strings.ReplaceAll(koanfKey, ".", "-")

	// dot notation 格式即为原始 koanf key: server.skip_verify
	dotFlag := koanfKey

	// 优先检查 kebab-case 格式
	if cmd.IsSet(kebabFlag) {
		return kebabFlag, true
	}

	// 再检查 dot notation 格式
	if cmd.IsSet(dotFlag) {
		return dotFlag, true
	}

	return "", false
}

// setCLIFlagValue 根据字段类型从 CLI 获取值并设置到 koanf。
func setCLIFlagValue(cmd *cli.Command, k *koanf.Koanf, koanfKey, cliFlag string, fieldType reflect.Type) {
	// 先检查特殊类型 (time.Duration, time.Time)
	switch fieldType {
	case reflect.TypeOf(time.Duration(0)):
		_ = k.Set(koanfKey, cmd.Duration(cliFlag))
		return
	case reflect.TypeOf(time.Time{}):
		_ = k.Set(koanfKey, cmd.Timestamp(cliFlag))
		return
	}

	// 处理基本类型和切片
	switch fieldType.Kind() {
	// 字符串
	case reflect.String:
		_ = k.Set(koanfKey, cmd.String(cliFlag))

	// 布尔
	case reflect.Bool:
		_ = k.Set(koanfKey, cmd.Bool(cliFlag))

	// 有符号整数
	case reflect.Int:
		_ = k.Set(koanfKey, cmd.Int(cliFlag))
	case reflect.Int8:
		_ = k.Set(koanfKey, cmd.Int8(cliFlag))
	case reflect.Int16:
		_ = k.Set(koanfKey, cmd.Int16(cliFlag))
	case reflect.Int32:
		_ = k.Set(koanfKey, cmd.Int32(cliFlag))
	case reflect.Int64:
		_ = k.Set(koanfKey, cmd.Int64(cliFlag))

	// 无符号整数
	case reflect.Uint:
		_ = k.Set(koanfKey, cmd.Uint(cliFlag))
	case reflect.Uint8:
		_ = k.Set(koanfKey, uint8(cmd.Uint(cliFlag)))
	case reflect.Uint16:
		_ = k.Set(koanfKey, cmd.Uint16(cliFlag))
	case reflect.Uint32:
		_ = k.Set(koanfKey, cmd.Uint32(cliFlag))
	case reflect.Uint64:
		_ = k.Set(koanfKey, cmd.Uint64(cliFlag))

	// 浮点数
	case reflect.Float32:
		_ = k.Set(koanfKey, cmd.Float32(cliFlag))
	case reflect.Float64:
		_ = k.Set(koanfKey, cmd.Float64(cliFlag))

	// 切片类型
	case reflect.Slice:
		setSliceFlagValue(cmd, k, koanfKey, cliFlag, fieldType)

	// Map 类型
	case reflect.Map:
		if fieldType.Key().Kind() == reflect.String && fieldType.Elem().Kind() == reflect.String {
			_ = k.Set(koanfKey, cmd.StringMap(cliFlag))
		}
	}
}

// setSliceFlagValue 处理切片类型的 CLI flag。
func setSliceFlagValue(cmd *cli.Command, k *koanf.Koanf, koanfKey, cliFlag string, fieldType reflect.Type) {
	elemType := fieldType.Elem()

	// 先检查特殊元素类型
	if elemType == reflect.TypeOf(time.Time{}) {
		_ = k.Set(koanfKey, cmd.TimestampArgs(cliFlag))
		return
	}

	switch elemType.Kind() {
	case reflect.String:
		_ = k.Set(koanfKey, cmd.StringSlice(cliFlag))
	case reflect.Int:
		_ = k.Set(koanfKey, cmd.IntSlice(cliFlag))
	case reflect.Int8:
		_ = k.Set(koanfKey, cmd.Int8Slice(cliFlag))
	case reflect.Int16:
		_ = k.Set(koanfKey, cmd.Int16Slice(cliFlag))
	case reflect.Int32:
		_ = k.Set(koanfKey, cmd.Int32Slice(cliFlag))
	case reflect.Int64:
		_ = k.Set(koanfKey, cmd.Int64Slice(cliFlag))
	case reflect.Uint16:
		_ = k.Set(koanfKey, cmd.Uint16Slice(cliFlag))
	case reflect.Uint32:
		_ = k.Set(koanfKey, cmd.Uint32Slice(cliFlag))
	case reflect.Float32:
		_ = k.Set(koanfKey, cmd.Float32Slice(cliFlag))
	case reflect.Float64:
		_ = k.Set(koanfKey, cmd.Float64Slice(cliFlag))
	}
}
