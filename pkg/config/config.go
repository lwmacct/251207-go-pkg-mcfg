// Author: lwmacct (https://github.com/lwmacct)
// Package config 提供通用的配置加载功能，可被外部项目复用。
//
// 使用泛型支持任意配置结构体类型，配置加载优先级 (从低到高)：
//  1. 默认值 - 通过 defaultConfig 参数传入
//  2. 配置文件 - 按 configPaths 顺序搜索
//  3. CLI flags - 最高优先级
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

// DefaultPaths 返回默认配置文件搜索路径
// appName 可选，若提供则包含用户主目录和系统配置目录
func DefaultPaths(appName ...string) []string {
	paths := []string{
		"config.yaml",
		"config/config.yaml",
	}

	if len(appName) > 0 && appName[0] != "" {
		name := appName[0]
		// 添加用户主目录
		if home, err := os.UserHomeDir(); err == nil {
			paths = append(paths, filepath.Join(home, "."+name+".yaml"))
		}
		// 添加系统配置目录
		paths = append(paths, "/etc/"+name+"/config.yaml")
	}

	return paths
}

// Load 加载配置，按优先级合并：
// 1. 默认值 (最低优先级) - 通过 defaultConfig 参数传入
// 2. 配置文件 (按 configPaths 顺序搜索，找到第一个即停止)
// 3. CLI flags (最高优先级)
//
// 泛型参数 T 为配置结构体类型，必须使用 koanf tag 标记字段。
func Load[T any](cmd *cli.Command, configPaths []string, defaultConfig T) (*T, error) {
	k := koanf.New(".")

	// 1️⃣ 加载默认配置 (最低优先级)
	if err := k.Load(structs.Provider(defaultConfig, "koanf"), nil); err != nil {
		return nil, fmt.Errorf("failed to load default config: %w", err)
	}

	// 2️⃣ 加载配置文件 (按顺序搜索，找到第一个即停止)
	configLoaded := false
	for _, path := range configPaths {
		if err := k.Load(file.Provider(path), yaml.Parser()); err == nil {
			slog.Debug("Loaded config from file", "path", path)
			configLoaded = true
			break
		}
	}

	if !configLoaded {
		slog.Debug("No config file found, using defaults")
	}

	// 3️⃣ 加载 CLI flags (最高优先级，仅当用户明确指定时)
	if cmd != nil {
		applyCLIFlagsGeneric(cmd, k, defaultConfig)
	}

	// 解析到结构体
	var cfg T
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

// applyCLIFlagsGeneric 通过反射将用户明确指定的 CLI flags 应用到 koanf 实例
// 自动根据配置结构体的 koanf 标签映射 CLI flag 名称
// koanf 标签使用 snake_case，CLI flag 使用 kebab-case
//
// 支持嵌套结构体，例如：
//   - server.url → --server-url
//   - tls.skip_verify → --tls-skip-verify
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

// applyCLIFlagsRecursive 递归遍历结构体字段应用 CLI flags
func applyCLIFlagsRecursive(cmd *cli.Command, k *koanf.Koanf, typ reflect.Type, prefix string) {
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		// 获取 koanf 标签作为配置 key (snake_case)
		koanfKey := field.Tag.Get("koanf")
		if koanfKey == "" {
			continue
		}

		// 构建完整的 koanf key 和 CLI flag 名称
		fullKoanfKey := koanfKey
		if prefix != "" {
			fullKoanfKey = prefix + "." + koanfKey
		}

		// 转换为 CLI flag 名称 (kebab-case)
		cliFlag := strings.ReplaceAll(fullKoanfKey, ".", "-")
		cliFlag = strings.ReplaceAll(cliFlag, "_", "-")

		// 如果是嵌套结构体，递归处理
		if field.Type.Kind() == reflect.Struct &&
			field.Type != reflect.TypeOf(time.Duration(0)) &&
			field.Type != reflect.TypeOf(time.Time{}) {
			applyCLIFlagsRecursive(cmd, k, field.Type, fullKoanfKey)
			continue
		}

		// 只有用户明确指定时才覆盖
		if !cmd.IsSet(cliFlag) {
			continue
		}

		// 根据字段类型获取值并设置
		setCLIFlagValue(cmd, k, fullKoanfKey, cliFlag, field.Type)
	}
}

// setCLIFlagValue 根据字段类型从 CLI 获取值并设置到 koanf
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

// setSliceFlagValue 处理切片类型的 CLI flag
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
