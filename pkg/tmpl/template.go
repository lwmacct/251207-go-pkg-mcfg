package tmpl

import (
	"bytes"
	"os"
	"strings"
	"text/template"
)

// ═══════════════════════════════════════════════════════════════════════════
// 模板函数 (参考: Taskfile 和 Sprig)
// ═══════════════════════════════════════════════════════════════════════════

// templateFuncs 模板函数映射表
var templateFuncs = template.FuncMap{
	"env":      envFunc,
	"default":  defaultFunc,
	"coalesce": coalesceFunc,
}

// envFunc 获取环境变量，支持可选的默认值。
//
// 使用方式：
//   - {{env "VAR"}}           获取环境变量，未设置时返回空字符串
//   - {{env "VAR" "default"}} 获取环境变量，未设置时返回默认值
//   - {{env "VAR" | default "fallback"}} 管道语法
func envFunc(key string, defaultVal ...string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	if len(defaultVal) > 0 {
		return defaultVal[0]
	}

	return ""
}

// defaultFunc 提供默认值（管道友好）。
//
// 参考 Sprig 实现，参数顺序：default(默认值, 实际值)
//
// 使用方式：
//   - {{env "VAR" | default "fallback"}}
//   - {{.Env.VAR | default .Env.OTHER | default "final"}}
func defaultFunc(defaultVal, value any) any {
	if value == nil {
		return defaultVal
	}
	if str, ok := value.(string); ok && str == "" {
		return defaultVal
	}

	return value
}

// coalesceFunc 返回第一个非空值（类似 Taskfile/Sprig）。
//
// 使用方式：
//   - {{coalesce .Env.VAR1 .Env.VAR2 "default"}}
//   - {{coalesce .APIKey .Env.OPENAI_API_KEY .Env.ANTHROPIC_API_KEY "sk-xxx"}}
func coalesceFunc(values ...any) any {
	for _, v := range values {
		if v == nil {
			continue
		}
		if str, ok := v.(string); ok && str == "" {
			continue
		}

		return v
	}

	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// 模板数据对象 (与 Taskfile 设计对齐)
// ═══════════════════════════════════════════════════════════════════════════

// newTemplateData 创建模板数据对象。
//
// 返回 map[string]string，支持 Taskfile 风格的 {{.VAR}} 语法。
// 所有环境变量自动加载到顶级命名空间。
func newTemplateData() map[string]string {
	vars := make(map[string]string)
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			vars[parts[0]] = parts[1]
		}
	}

	return vars
}

// ═══════════════════════════════════════════════════════════════════════════
// 模板渲染
// ═══════════════════════════════════════════════════════════════════════════

// ExpandTemplate 展开模板字符串中的环境变量引用。
//
// 使用 Go text/template 引擎处理模板，所有环境变量自动加载到顶级命名空间。
//
// 支持的语法：
//   - {{.VAR}} - 直接访问环境变量（Taskfile 风格）
//   - {{env "VAR"}} - env 函数方式
//   - {{env "VAR" "default"}} - 带默认值
//   - {{.VAR | default "fallback"}} - 管道式默认值
//   - {{coalesce .VAR1 .VAR2 "default"}} - 多级 fallback
//
// 返回展开后的字符串。如果模板语法错误或执行失败，返回 error。
func ExpandTemplate(text string) (string, error) {
	tmpl, err := template.New("config").Funcs(templateFuncs).Parse(text)
	if err != nil {
		return "", err
	}

	data := newTemplateData()

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}
