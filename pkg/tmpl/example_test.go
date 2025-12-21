package tmpl_test

import (
	"fmt"
	"os"

	"github.com/lwmacct/251207-go-pkg-cfgm/pkg/tmpl"
)

// Example_envFunction 演示如何使用 env 模板函数访问环境变量，支持可选的默认值。
func Example_envFunction() {
	_ = os.Setenv("API_KEY", "sk-12345")
	defer func() { _ = os.Unsetenv("API_KEY") }()

	// 访问已存在的环境变量
	result1, _ := tmpl.ExpandTemplate(`{{env "API_KEY"}}`)
	fmt.Println(result1)

	// 访问不存在的环境变量时使用默认值
	result2, _ := tmpl.ExpandTemplate(`{{env "MISSING_VAR" "default-value"}}`)
	fmt.Println(result2)

	// Output:
	// sk-12345
	// default-value
}

// Example_defaultFunction 演示如何使用 default 函数作为管道过滤器提供回退值。
func Example_defaultFunction() {
	// default 函数为空值提供回退
	result1, _ := tmpl.ExpandTemplate(`{{env "MISSING" | default "fallback"}}`)
	fmt.Println(result1)

	// 非空值保持不变
	result2, _ := tmpl.ExpandTemplate(`{{"actual-value" | default "fallback"}}`)
	fmt.Println(result2)

	// Output:
	// fallback
	// actual-value
}

// Example_coalesceFunction 演示如何使用 coalesce 实现多级回退。
// coalesce 返回参数中第一个非空值。
func Example_coalesceFunction() {
	_ = os.Setenv("BACKUP_URL", "https://backup.example.com")
	defer func() { _ = os.Unsetenv("BACKUP_URL") }()

	// coalesce 返回第一个非空值
	result, _ := tmpl.ExpandTemplate(`{{coalesce (env "PRIMARY_URL") (env "BACKUP_URL") "https://default.example.com"}}`)
	fmt.Println(result)

	// Output:
	// https://backup.example.com
}

// Example_directVarAccess 演示 Taskfile 风格的直接变量访问。
// 环境变量可通过 {{.VAR_NAME}} 语法直接访问。
func Example_directVarAccess() {
	_ = os.Setenv("DATABASE_HOST", "localhost")
	_ = os.Setenv("DATABASE_PORT", "5432")
	defer func() {
		_ = os.Unsetenv("DATABASE_HOST")
		_ = os.Unsetenv("DATABASE_PORT")
	}()

	// 直接访问环境变量 (Taskfile 风格)
	result, _ := tmpl.ExpandTemplate(`{{.DATABASE_HOST}}:{{.DATABASE_PORT}}`)
	fmt.Println(result)

	// 配合 default 处理缺失的变量
	result2, _ := tmpl.ExpandTemplate(`{{.MISSING_VAR | default "fallback"}}`)
	fmt.Println(result2)

	// Output:
	// localhost:5432
	// fallback
}
