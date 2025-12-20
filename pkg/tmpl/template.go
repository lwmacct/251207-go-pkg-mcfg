package tmpl

import (
	"bytes"
	"os"
	"strings"
	"text/template"
)

// ═══════════════════════════════════════════════════════════════════════════
// Template Functions (Reference: Taskfile and Sprig)
// ═══════════════════════════════════════════════════════════════════════════

// templateFuncs template function map
var templateFuncs = template.FuncMap{
	"env":      envFunc,
	"default":  defaultFunc,
	"coalesce": coalesceFunc,
}

// envFunc gets environment variable with optional default value
//
// Usage:
//   - {{env "VAR"}}           get env var, returns empty string if not set
//   - {{env "VAR" "default"}} get env var, returns default if not set
//   - {{env "VAR" | default "fallback"}} pipeline syntax
func envFunc(key string, defaultVal ...string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	if len(defaultVal) > 0 {
		return defaultVal[0]
	}

	return ""
}

// defaultFunc provides default value (pipeline friendly)
//
// Reference: Sprig implementation, parameter order: default(defaultValue, value)
//
// Usage:
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

// coalesceFunc returns first non-empty value (like Taskfile/Sprig)
//
// Usage:
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
// Template Data Object (Aligns with Taskfile design)
// ═══════════════════════════════════════════════════════════════════════════

// newTemplateData creates template data object
//
// Returns map[string]string, supporting Taskfile style {{.VAR}} syntax
// All environment variables are automatically loaded into top-level namespace
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
// Template Rendering
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
