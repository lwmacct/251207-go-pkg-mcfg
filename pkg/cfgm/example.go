package cfgm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
	yamlv3 "go.yaml.in/yaml/v3"
)

// ExampleYAML 将配置结构体序列化为带注释的 YAML。
//
// 通过 desc tag 自动生成注释，适用于生成 config.example.yaml。
//
// 使用示例：
//
//	yaml := cfgm.ExampleYAML(DefaultConfig())
//	os.WriteFile("config/config.example.yaml", yaml, 0644)
func ExampleYAML[T any](cfg T) []byte {
	node := structToNode(reflect.ValueOf(cfg), reflect.TypeOf(cfg))
	node.HeadComment = "配置示例文件, 复制此文件为 config.yaml 并根据需要修改"

	var buf bytes.Buffer
	enc := yamlv3.NewEncoder(&buf)
	enc.SetIndent(2)
	_ = enc.Encode(node)
	_ = enc.Close()

	return buf.Bytes()
}

// MarshalYAML 将配置结构体序列化为 YAML（无注释）。
//
// 使用 koanf 原生 Marshal，输出简洁。
//
// 使用示例：
//
//	yaml := cfgm.MarshalYAML(cfg)
//	os.WriteFile("config/config.yaml", yaml, 0644)
func MarshalYAML[T any](cfg T) []byte {
	k := koanf.New(".")
	_ = k.Load(structs.Provider(cfg, "koanf"), nil)
	data, _ := k.Marshal(yaml.Parser())

	return data
}

// MarshalJSON 将配置结构体序列化为 JSON。
//
// 使用示例：
//
//	jsonBytes := cfgm.MarshalJSON(cfg)
//	os.WriteFile("config/config.json", jsonBytes, 0644)
func MarshalJSON[T any](cfg T) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	_ = enc.Encode(cfg) //nolint:errchkjson // T is a config struct, safe to encode

	return buf.Bytes()
}

// structToNode 将结构体转换为带注释的 yamlv3.Node。
func structToNode(val reflect.Value, typ reflect.Type) *yamlv3.Node {
	// 处理指针类型
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return &yamlv3.Node{Kind: yamlv3.ScalarNode, Tag: "!!null"}
		}
		val = val.Elem()
		typ = typ.Elem()
	}

	node := &yamlv3.Node{Kind: yamlv3.MappingNode}

	for i := range typ.NumField() {
		field := typ.Field(i)
		fieldVal := val.Field(i)

		key := field.Tag.Get("koanf")
		if key == "" {
			continue
		}
		comment := field.Tag.Get("desc")

		// Key node
		keyNode := &yamlv3.Node{Kind: yamlv3.ScalarNode, Value: key}

		// Value node
		var valNode *yamlv3.Node

		// 判断是否为复杂类型（结构体或数组）
		isStruct := field.Type.Kind() == reflect.Struct &&
			field.Type != reflect.TypeFor[time.Duration]() &&
			field.Type != reflect.TypeFor[time.Time]()
		isSlice := field.Type.Kind() == reflect.Slice

		switch {
		case isStruct:
			valNode = structToNode(fieldVal, field.Type)
			keyNode.HeadComment = "\n" + comment // 复杂类型注释放在 key 上方，前面加空行
		case isSlice:
			valNode = valueToNode(fieldVal, field.Type)
			keyNode.HeadComment = "\n" + comment // 复杂类型注释放在 key 上方，前面加空行
		default:
			valNode = valueToNode(fieldVal, field.Type)
			// 多行注释放在 key 上方（HeadComment），单行注释放在行尾（LineComment）
			setSimpleFieldComment(keyNode, valNode, comment)
		}

		node.Content = append(node.Content, keyNode, valNode)
	}

	return node
}

// setSimpleFieldComment 设置简单字段的注释。
// 多行注释放在 key 上方（HeadComment），单行注释放在行尾（LineComment）。
func setSimpleFieldComment(keyNode, valNode *yamlv3.Node, comment string) {
	if strings.Contains(comment, "\n") {
		keyNode.HeadComment = "\n" + comment
	} else {
		valNode.LineComment = comment
	}
}

// valueToNode 将值转换为 yamlv3.Node。
func valueToNode(val reflect.Value, typ reflect.Type) *yamlv3.Node {
	// 特殊类型处理
	switch typ {
	case reflect.TypeFor[time.Duration]():
		if d, ok := val.Interface().(time.Duration); ok {
			return &yamlv3.Node{
				Kind:  yamlv3.ScalarNode,
				Value: d.String(),
			}
		}
	case reflect.TypeFor[time.Time]():
		if t, ok := val.Interface().(time.Time); ok {
			return &yamlv3.Node{
				Kind:  yamlv3.ScalarNode,
				Value: t.Format(time.RFC3339),
			}
		}
	}

	switch val.Kind() {
	case reflect.String:
		return &yamlv3.Node{
			Kind:  yamlv3.ScalarNode,
			Value: val.String(),
			Style: yamlv3.DoubleQuotedStyle,
		}

	case reflect.Bool:
		return &yamlv3.Node{
			Kind:  yamlv3.ScalarNode,
			Value: strconv.FormatBool(val.Bool()),
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &yamlv3.Node{
			Kind:  yamlv3.ScalarNode,
			Value: strconv.FormatInt(val.Int(), 10),
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &yamlv3.Node{
			Kind:  yamlv3.ScalarNode,
			Value: strconv.FormatUint(val.Uint(), 10),
		}

	case reflect.Float32, reflect.Float64:
		return &yamlv3.Node{
			Kind:  yamlv3.ScalarNode,
			Value: fmt.Sprintf("%v", val.Float()),
		}

	case reflect.Slice:
		node := &yamlv3.Node{Kind: yamlv3.SequenceNode}
		if val.Len() == 0 {
			node.Style = yamlv3.FlowStyle // [] 形式
		} else {
			for j := range val.Len() {
				elem := val.Index(j)
				elemNode := valueToNode(elem, elem.Type())
				// slice 元素不使用引号样式，保持简洁
				elemNode.Style = 0
				node.Content = append(node.Content, elemNode)
			}
		}

		return node

	case reflect.Map:
		node := &yamlv3.Node{Kind: yamlv3.MappingNode}
		if val.Len() == 0 {
			node.Style = yamlv3.FlowStyle // {} 形式
		} else {
			iter := val.MapRange()
			for iter.Next() {
				k, v := iter.Key(), iter.Value()
				node.Content = append(node.Content,
					&yamlv3.Node{Kind: yamlv3.ScalarNode, Value: fmt.Sprintf("%v", k.Interface())},
					valueToNode(v, v.Type()),
				)
			}
		}

		return node

	default:
		return &yamlv3.Node{
			Kind:  yamlv3.ScalarNode,
			Value: fmt.Sprintf("%v", val.Interface()),
		}
	}
}

// ConfigTestHelper 配置测试辅助工具
//
// 使用示例：
//
//	var helper = cfgm.ConfigTestHelper[Config]{
//	    ExamplePath: "config/config.example.yaml",
//	    ConfigPath:  "config/config.yaml",
//	}
//
//	func TestWriteExample(t *testing.T) { helper.WriteExampleFile(t, DefaultConfig()) }
//	func TestConfigKeysValid(t *testing.T) { helper.ValidateKeys(t) }
type ConfigTestHelper[T any] struct {
	ExamplePath string // 示例文件相对路径（相对于 go.mod 所在目录）
	ConfigPath  string // 配置文件相对路径（相对于 go.mod 所在目录）
}

// WriteExampleFile 将示例配置写入文件
func (h *ConfigTestHelper[T]) WriteExampleFile(t *testing.T, defaultConfig T) {
	t.Helper()

	projectRoot, err := FindProjectRoot(1)
	if err != nil {
		t.Fatalf("无法找到项目根目录: %v", err)
	}

	yamlBytes := ExampleYAML(defaultConfig)

	outputPath := filepath.Join(projectRoot, h.ExamplePath)
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0750); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	if err := os.WriteFile(outputPath, yamlBytes, 0600); err != nil {
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

	if _, statErr := os.Stat(configPath); os.IsNotExist(statErr) {
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
		return "", errors.New("无法获取当前文件路径")
	}

	dir := filepath.Dir(filename)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("未找到 go.mod")
		}
		dir = parent
	}
}

// loadConfigKeys 加载配置文件并返回所有配置键（支持 YAML 和 JSON）。
func loadConfigKeys(path string) ([]string, error) {
	k := koanf.New(".")
	err := k.Load(file.Provider(path), parserForPath(path))
	if err != nil {
		return nil, fmt.Errorf("加载文件失败: %w", err)
	}

	return k.Keys(), nil
}
