package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"go.yaml.in/yaml/v3"
)

// GenerateExampleYAML 根据配置结构体生成带注释的 YAML 示例。
//
// 通过反射读取 koanf 和 desc tag 自动生成，使用 yaml.v3 Node API 确保正确的序列化。
//
// 使用示例：
//
//	yaml := config.GenerateExampleYAML(DefaultConfig())
//	os.WriteFile("config/config.example.yaml", yaml, 0644)
func GenerateExampleYAML[T any](cfg T) []byte {
	node := structToNode(reflect.ValueOf(cfg), reflect.TypeOf(cfg))
	node.HeadComment = "配置示例文件, 复制此文件为 config.yaml 并根据需要修改"

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	_ = enc.Encode(node)
	_ = enc.Close()
	return buf.Bytes()
}

// GenerateExampleJSON 根据配置结构体生成 JSON 示例。
//
// 注意：JSON 不支持注释，desc tag 将被忽略。如需注释说明，请参考 YAML 示例。
//
// 使用示例：
//
//	jsonBytes := config.GenerateExampleJSON(DefaultConfig())
//	os.WriteFile("config/config.example.json", jsonBytes, 0644)
func GenerateExampleJSON[T any](cfg T) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	_ = enc.Encode(cfg)
	return buf.Bytes()
}

// structToNode 将结构体转换为带注释的 yaml.Node。
func structToNode(val reflect.Value, typ reflect.Type) *yaml.Node {
	// 处理指针类型
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null"}
		}
		val = val.Elem()
		typ = typ.Elem()
	}

	node := &yaml.Node{Kind: yaml.MappingNode}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)

		key := field.Tag.Get("koanf")
		if key == "" {
			continue
		}
		comment := field.Tag.Get("desc")

		// Key node
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}

		// Value node
		var valNode *yaml.Node

		// 嵌套结构体（排除 time.Duration 和 time.Time）
		if field.Type.Kind() == reflect.Struct &&
			field.Type != reflect.TypeOf(time.Duration(0)) &&
			field.Type != reflect.TypeOf(time.Time{}) {
			valNode = structToNode(fieldVal, field.Type)
			keyNode.HeadComment = "\n" + comment // 结构体注释放在 key 上方，前面加空行
		} else {
			valNode = valueToNode(fieldVal, field.Type)
			valNode.LineComment = comment // 标量注释放在行尾
		}

		node.Content = append(node.Content, keyNode, valNode)
	}

	return node
}

// valueToNode 将值转换为 yaml.Node。
func valueToNode(val reflect.Value, typ reflect.Type) *yaml.Node {
	// 特殊类型处理
	switch typ {
	case reflect.TypeOf(time.Duration(0)):
		return &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: val.Interface().(time.Duration).String(),
		}
	case reflect.TypeOf(time.Time{}):
		return &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: val.Interface().(time.Time).Format(time.RFC3339),
		}
	}

	switch val.Kind() {
	case reflect.String:
		return &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: val.String(),
			Style: yaml.DoubleQuotedStyle,
		}

	case reflect.Bool:
		return &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: fmt.Sprintf("%t", val.Bool()),
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: fmt.Sprintf("%d", val.Int()),
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: fmt.Sprintf("%d", val.Uint()),
		}

	case reflect.Float32, reflect.Float64:
		return &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: fmt.Sprintf("%v", val.Float()),
		}

	case reflect.Slice:
		node := &yaml.Node{Kind: yaml.SequenceNode}
		if val.Len() == 0 {
			node.Style = yaml.FlowStyle // [] 形式
		} else {
			for j := 0; j < val.Len(); j++ {
				elem := val.Index(j)
				elemNode := valueToNode(elem, elem.Type())
				// slice 元素不使用引号样式，保持简洁
				elemNode.Style = 0
				node.Content = append(node.Content, elemNode)
			}
		}
		return node

	case reflect.Map:
		node := &yaml.Node{Kind: yaml.MappingNode}
		if val.Len() == 0 {
			node.Style = yaml.FlowStyle // {} 形式
		} else {
			iter := val.MapRange()
			for iter.Next() {
				k, v := iter.Key(), iter.Value()
				node.Content = append(node.Content,
					&yaml.Node{Kind: yaml.ScalarNode, Value: fmt.Sprintf("%v", k.Interface())},
					valueToNode(v, v.Type()),
				)
			}
		}
		return node

	default:
		return &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: fmt.Sprintf("%v", val.Interface()),
		}
	}
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

	exampleKeys, err := loadConfigKeys(examplePath)
	if err != nil {
		t.Fatalf("无法加载 %s: %v", h.ExamplePath, err)
	}

	configKeys, err := loadConfigKeys(configPath)
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

// loadConfigKeys 加载配置文件并返回所有配置键（支持 YAML 和 JSON）。
func loadConfigKeys(path string) ([]string, error) {
	k := koanf.New(".")
	if err := k.Load(file.Provider(path), parserForPath(path)); err != nil {
		return nil, fmt.Errorf("加载文件失败: %w", err)
	}
	return k.Keys(), nil
}
