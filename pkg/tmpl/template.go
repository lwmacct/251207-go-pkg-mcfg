// Package tmpl provides Agent configuration template expansion.
//
// Template system aligns with Taskfile design, supporting flexible environment variable access.
//
// # Design References
//
//   - Taskfile template syntax: https://taskfile.dev/docs/reference/templating
//   - Taskfile environment variables: https://taskfile.dev/docs/reference/environment
//   - Sprig template functions: https://github.com/Masterminds/sprig
//
// # Core Design Principles
//
//  1. Environment variables are automatically available via {{.VAR}} (Taskfile style)
//  2. env function is optional, use when variable name conflicts occur
//  3. Pipeline friendly: {{.VAR | default "fallback"}}
//  4. Multi-level fallback: coalesce function supports graceful fallback chains
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

// ExpandTemplate expands template with environment variables
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
