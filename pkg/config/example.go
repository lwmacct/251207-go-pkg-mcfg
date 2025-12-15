package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// GenerateExampleYAML 根据配置结构体生成带注释的 YAML 示例。
//
// 通过反射读取 koanf 和 comment tag 自动生成。
//
// 使用示例：
//
//	yaml := config.GenerateExampleYAML(DefaultConfig())
//	os.WriteFile("config/config.example.yaml", yaml, 0644)
func GenerateExampleYAML[T any](cfg T) []byte {
	var buf bytes.Buffer
	buf.WriteString("# 配置示例文件, 复制此文件为 config.yaml 并根据需要修改\n")
	writeStructYAML(&buf, reflect.ValueOf(cfg), reflect.TypeOf(cfg), 0)
	return buf.Bytes()
}

// ConfigTestHelper 配置测试辅助工具
//
// 使用示例：
//
//	var helper = config.ConfigTestHelper[Config]{
//	    ExamplePath: "config/config.example.yaml",
//	    ConfigPath:  "config/config.yaml",
//	}
//
//	func TestGenerateExample(t *testing.T) { helper.GenerateExample(t, DefaultConfig()) }
//	func TestConfigKeysValid(t *testing.T) { helper.ValidateKeys(t) }
type ConfigTestHelper[T any] struct {
	ExamplePath string // 示例文件相对路径（相对于 go.mod 所在目录）
	ConfigPath  string // 配置文件相对路径（相对于 go.mod 所在目录）
}

// GenerateExample 根据默认配置生成示例文件
func (h *ConfigTestHelper[T]) GenerateExample(t *testing.T, defaultConfig T) {
	t.Helper()

	projectRoot, err := FindProjectRoot(1)
	if err != nil {
		t.Fatalf("无法找到项目根目录: %v", err)
	}

	yamlBytes := GenerateExampleYAML(defaultConfig)

	outputPath := filepath.Join(projectRoot, h.ExamplePath)
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	if err := os.WriteFile(outputPath, yamlBytes, 0644); err != nil {
		t.Fatalf("写入配置文件失败: %v", err)
	}

	t.Logf("✅ 已生成配置示例文件: %s", outputPath)
}

// ValidateKeys 校验配置文件中的键名是否都在示例文件中定义
func (h *ConfigTestHelper[T]) ValidateKeys(t *testing.T) {
	t.Helper()

	projectRoot, err := FindProjectRoot(1)
	if err != nil {
		t.Fatalf("无法找到项目根目录: %v", err)
	}

	configPath := filepath.Join(projectRoot, h.ConfigPath)
	examplePath := filepath.Join(projectRoot, h.ExamplePath)

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skipf("%s 不存在，跳过验证", h.ConfigPath)
	}

	exampleKeys, err := loadYAMLKeys(examplePath)
	if err != nil {
		t.Fatalf("无法加载 %s: %v", h.ExamplePath, err)
	}

	configKeys, err := loadYAMLKeys(configPath)
	if err != nil {
		t.Fatalf("无法加载 %s: %v", h.ConfigPath, err)
	}

	validKeyMap := make(map[string]bool, len(exampleKeys))
	for _, key := range exampleKeys {
		validKeyMap[key] = true
	}

	var invalidKeys []string
	for _, key := range configKeys {
		if !validKeyMap[key] {
			invalidKeys = append(invalidKeys, key)
		}
	}

	if len(invalidKeys) > 0 {
		t.Errorf("%s 包含以下无效配置项:\n", h.ConfigPath)
		for _, key := range invalidKeys {
			t.Errorf("  - %s", key)
		}
	}
}

// FindProjectRoot 通过查找 go.mod 文件定位项目根目录。
//
// skip 指定跳过的调用栈层数，0 表示调用者，1 表示调用者的调用者，以此类推。
func FindProjectRoot(skip int) (string, error) {
	_, filename, _, ok := runtime.Caller(skip + 1)
	if !ok {
		return "", fmt.Errorf("无法获取当前文件路径")
	}

	dir := filepath.Dir(filename)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("未找到 go.mod")
		}
		dir = parent
	}
}

// loadYAMLKeys 加载 YAML 文件并返回所有配置键。
func loadYAMLKeys(path string) ([]string, error) {
	k := koanf.New(".")
	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		return nil, fmt.Errorf("加载文件失败: %w", err)
	}
	return k.Keys(), nil
}

// writeStructYAML 递归写入结构体的 YAML 格式。
func writeStructYAML(buf *bytes.Buffer, val reflect.Value, typ reflect.Type, indent int) {
	// 处理指针类型
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return
		}
		val = val.Elem()
		typ = typ.Elem()
	}

	prefix := ""
	for i := 0; i < indent; i++ {
		prefix += "  "
	}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)

		koanfKey := field.Tag.Get("koanf")
		comment := field.Tag.Get("comment")
		if koanfKey == "" {
			continue
		}

		// 处理嵌套结构体
		if field.Type.Kind() == reflect.Struct && field.Type.String() != "time.Duration" && field.Type.String() != "time.Time" {
			fmt.Fprintf(buf, "\n%s# %s\n", prefix, comment)
			fmt.Fprintf(buf, "%s%s:\n", prefix, koanfKey)
			writeStructYAML(buf, fieldVal, field.Type, indent+1)
			continue
		}

		// 根据字段类型输出不同格式
		switch fieldVal.Kind() {
		case reflect.String:
			fmt.Fprintf(buf, "%s%s: %q # %s\n", prefix, koanfKey, fieldVal.String(), comment)
		case reflect.Bool:
			fmt.Fprintf(buf, "%s%s: %t # %s\n", prefix, koanfKey, fieldVal.Bool(), comment)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if field.Type.String() == "time.Duration" {
				fmt.Fprintf(buf, "%s%s: %s # %s\n", prefix, koanfKey, fieldVal.Interface(), comment)
			} else {
				fmt.Fprintf(buf, "%s%s: %d # %s\n", prefix, koanfKey, fieldVal.Int(), comment)
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			fmt.Fprintf(buf, "%s%s: %d # %s\n", prefix, koanfKey, fieldVal.Uint(), comment)
		case reflect.Float32, reflect.Float64:
			fmt.Fprintf(buf, "%s%s: %v # %s\n", prefix, koanfKey, fieldVal.Float(), comment)
		case reflect.Slice:
			if fieldVal.Len() == 0 {
				fmt.Fprintf(buf, "%s%s: [] # %s\n", prefix, koanfKey, comment)
			} else {
				fmt.Fprintf(buf, "%s%s: # %s\n", prefix, koanfKey, comment)
				for j := 0; j < fieldVal.Len(); j++ {
					fmt.Fprintf(buf, "%s  - %v\n", prefix, fieldVal.Index(j).Interface())
				}
			}
		case reflect.Map:
			if fieldVal.Len() == 0 {
				fmt.Fprintf(buf, "%s%s: {} # %s\n", prefix, koanfKey, comment)
			} else {
				fmt.Fprintf(buf, "%s%s: # %s\n", prefix, koanfKey, comment)
				iter := fieldVal.MapRange()
				for iter.Next() {
					fmt.Fprintf(buf, "%s  %v: %v\n", prefix, iter.Key().Interface(), iter.Value().Interface())
				}
			}
		default:
			fmt.Fprintf(buf, "%s%s: %v # %s\n", prefix, koanfKey, fieldVal.Interface(), comment)
		}
	}
}
