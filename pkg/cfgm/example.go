package cfgm

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	yamlv3 "go.yaml.in/yaml/v3"
)

// ExampleYAML 将配置结构体序列化为带注释的 YAML 示例。
//
// 使用 json tag 作为 key，desc tag 作为注释，适用于生成 config.example.yaml。
//
// 使用示例：
//
//	yaml := cfgm.ExampleYAML(DefaultConfig())
//	os.WriteFile("config/config.example.yaml", yaml, 0644)
func ExampleYAML[T any](cfg T) []byte {
	node := structToNode(reflect.ValueOf(cfg), reflect.TypeOf(cfg))
	node.HeadComment = "默认配置示例文件, 此文件由单元测试生成, 请勿直接修改\n复制此文件为 config.yaml 并根据需要修改"

	var buf bytes.Buffer
	enc := yamlv3.NewEncoder(&buf)
	enc.SetIndent(2)
	_ = enc.Encode(node)
	_ = enc.Close()

	return normalizeBlankLines(buf.Bytes())
}

// normalizeBlankLines removes indentation from whitespace-only lines while
// preserving line count and surrounding content.
func normalizeBlankLines(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ""
		}
	}

	return []byte(strings.Join(lines, "\n"))
}

// MarshalYAML 将配置结构体序列化为 YAML（无注释）。
//
// 使用示例：
//
//	yaml := cfgm.MarshalYAML(cfg)
//	os.WriteFile("config/config.yaml", yaml, 0644)
func MarshalYAML[T any](cfg T) []byte {
	data, _ := yamlv3.Marshal(structToMap(cfg))

	return data
}

// MarshalJSON 将配置结构体序列化为 JSON（带缩进）。
//
// 使用示例：
//
//	jsonBytes := cfgm.MarshalJSON(cfg)
//	os.WriteFile("config/config.json", jsonBytes, 0644)
func MarshalJSON[T any](cfg T) []byte {
	data, err := json.Marshal(cfg, json.WithMarshalers(durationMarshaler()))
	if err != nil {
		return nil
	}
	value := jsontext.Value(data)
	if err := value.Indent(jsontext.WithIndent("  ")); err != nil {
		return nil
	}
	return append(value, '\n')
}

// InitConfigFile 将默认配置写入运行配置文件。
//
// 该函数用于显式初始化本地配置文件（如 config/config.yaml）。如果目标文件已存在，
// 函数会返回错误并拒绝覆盖。
func InitConfigFile[T any](defaultConfig T, configPath string) error {
	if configPath == "" {
		return errors.New("config path is empty")
	}

	outputPath, err := resolveProjectPath(configPath, 1)
	if err != nil {
		return err
	}

	if _, statErr := os.Stat(outputPath); statErr == nil {
		return fmt.Errorf("config file already exists: %s", outputPath)
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("stat config file %s: %w", outputPath, statErr)
	}

	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0750); err != nil {
		return fmt.Errorf("create config directory %s: %w", outputDir, err)
	}

	if err := os.WriteFile(outputPath, MarshalYAML(defaultConfig), 0600); err != nil {
		return fmt.Errorf("write config file %s: %w", outputPath, err)
	}

	return nil
}

func resolveProjectPath(path string, callerSkip int) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}

	projectRoot, err := FindProjectRoot(callerSkip + 1)
	if err != nil {
		return "", fmt.Errorf("find project root: %w", err)
	}

	return filepath.Join(projectRoot, path), nil
}

// structToNode 将结构体转换为带注释的 yamlv3.Node。
func structToNode(val reflect.Value, typ reflect.Type) *yamlv3.Node {
	// 处理指针类型
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return &yamlv3.Node{Kind: yamlv3.ScalarNode, Tag: "!!null", Value: "null"}
		}
		val = val.Elem()
		typ = typ.Elem()
	}

	node := &yamlv3.Node{Kind: yamlv3.MappingNode}

	fields, _ := configFields(typ)
	for _, configured := range fields {
		field := configured.field
		fieldVal := val.FieldByIndex(configured.index)
		key := configTagName(field)
		comment := field.Tag.Get("desc")

		// Key node
		keyNode := &yamlv3.Node{Kind: yamlv3.ScalarNode, Value: key}

		// Value node
		var valNode *yamlv3.Node

		// 判断是否为复杂类型（结构体或数组）
		isStruct := isStructType(field.Type)
		isSlice := field.Type.Kind() == reflect.Slice
		isMap := isMapType(field.Type)

		switch {
		case isStruct:
			valNode = structToNode(fieldVal, field.Type)
			setComplexFieldComment(keyNode, comment)
		case isSlice || isMap:
			valNode = valueToNode(fieldVal, field.Type)
			setComplexFieldComment(keyNode, comment)
		default:
			valNode = valueToNode(fieldVal, field.Type)
			// 多行注释放在 key 上方（HeadComment），单行注释放在行尾（LineComment）
			setSimpleFieldComment(keyNode, valNode, comment)
		}

		node.Content = append(node.Content, keyNode, valNode)
	}

	return node
}

// setComplexFieldComment 设置复杂字段的注释。
// 复杂类型注释放在 key 上方，前面加空行。
func setComplexFieldComment(keyNode *yamlv3.Node, comment string) {
	if comment != "" {
		keyNode.HeadComment = "\n" + comment
	}
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
	if !val.IsValid() {
		return &yamlv3.Node{Kind: yamlv3.ScalarNode, Tag: "!!null", Value: "null"}
	}

	if val.Kind() == reflect.Interface {
		if val.IsNil() {
			return &yamlv3.Node{Kind: yamlv3.ScalarNode, Tag: "!!null", Value: "null"}
		}
		inner := val.Elem()
		return valueToNode(inner, inner.Type())
	}

	if typ.Kind() == reflect.Pointer {
		if val.IsNil() {
			return &yamlv3.Node{Kind: yamlv3.ScalarNode, Tag: "!!null", Value: "null"}
		}
		return valueToNode(val.Elem(), typ.Elem())
	}

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

	if typ.Kind() == reflect.Struct {
		return structToNode(val, typ)
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
			entries := make([]mapNodeEntry, 0, val.Len())
			iter := val.MapRange()
			for iter.Next() {
				k, v := iter.Key(), iter.Value()
				entries = append(entries, mapNodeEntry{
					key:   fmt.Sprintf("%v", k.Interface()),
					value: v,
				})
			}
			sort.Slice(entries, func(i, j int) bool {
				return entries[i].key < entries[j].key
			})

			for _, entry := range entries {
				node.Content = append(node.Content,
					&yamlv3.Node{Kind: yamlv3.ScalarNode, Value: entry.key},
					valueToNode(entry.value, entry.value.Type()),
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

type mapNodeEntry struct {
	key   string
	value reflect.Value
}

// ConfigFiles 声明一组由配置管理器驱动的配置文件。
//
// 使用示例：
//
//	var files = cfgm.ConfigFiles[Config]{
//	    Manager:     cfgm.New(DefaultConfig()),
//	    ExampleFile: "config/config.example.yaml",
//	    RuntimeFile: "config/config.yaml",
//	}
//
//	func TestWriteConfigExample(t *testing.T) { files.WriteExample(t) }
//	func TestRuntimeConfigKeysValid(t *testing.T) { files.ValidateRuntimeConfig(t) }
type ConfigFiles[T any] struct {
	Manager     *Manager[T] // 配置管理器
	ExampleFile string      // 示例文件相对路径（相对于 go.mod 所在目录）
	RuntimeFile string      // 运行配置文件相对路径（相对于 go.mod 所在目录）
}

// WriteExample 将示例配置写入 ExampleFile。
func (f ConfigFiles[T]) WriteExample(t *testing.T) {
	t.Helper()

	if f.Manager == nil {
		t.Fatal("Manager is nil")
	}

	outputPath, err := resolveProjectPath(f.ExampleFile, 1)
	if err != nil {
		t.Fatalf("无法找到项目根目录: %v", err)
	}

	yamlBytes := ExampleYAML(f.Manager.defaults)

	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0750); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	if err := os.WriteFile(outputPath, yamlBytes, 0600); err != nil {
		t.Fatalf("写入配置文件失败: %v", err)
	}

	t.Logf("✅ 已生成配置示例文件: %s", outputPath)
}

// ValidateRuntimeConfig 使用同一配置定义校验 RuntimeFile。
func (f ConfigFiles[T]) ValidateRuntimeConfig(t *testing.T) {
	t.Helper()
	if f.Manager == nil {
		t.Fatal("Manager is nil")
	}

	configPath, err := resolveProjectPath(f.RuntimeFile, 1)
	if err != nil {
		t.Fatalf("无法找到项目根目录: %v", err)
	}
	if _, statErr := os.Stat(configPath); os.IsNotExist(statErr) {
		t.Skipf("%s 不存在，跳过验证", f.RuntimeFile)
	}

	loader := f.Manager.loader()
	loader.sources = []Source{File(configPath)}
	if _, _, err := loader.load(t.Context()); err != nil {
		t.Fatalf("%s 配置无效: %v", f.RuntimeFile, err)
	}

	t.Logf("✅ 配置文件 %s 的所有配置项都有效", f.RuntimeFile)
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
