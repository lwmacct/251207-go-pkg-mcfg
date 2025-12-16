package tmpl_test

import (
	"fmt"
	"os"

	"github.com/lwmacct/251207-go-pkg-mcfg/pkg/tmpl"
)

// Example_envFunction demonstrates using the env template function
// to access environment variables with optional default values.
func Example_envFunction() {
	_ = os.Setenv("API_KEY", "sk-12345")
	defer func() { _ = os.Unsetenv("API_KEY") }()

	// Access existing environment variable
	result1, _ := tmpl.ExpandTemplate(`{{env "API_KEY"}}`)
	fmt.Println(result1)

	// Access missing variable with default value
	result2, _ := tmpl.ExpandTemplate(`{{env "MISSING_VAR" "default-value"}}`)
	fmt.Println(result2)

	// Output:
	// sk-12345
	// default-value
}

// Example_defaultFunction demonstrates using the default function
// as a pipeline filter to provide fallback values.
func Example_defaultFunction() {
	// default function provides fallback for empty values
	result1, _ := tmpl.ExpandTemplate(`{{env "MISSING" | default "fallback"}}`)
	fmt.Println(result1)

	// Non-empty values pass through unchanged
	result2, _ := tmpl.ExpandTemplate(`{{"actual-value" | default "fallback"}}`)
	fmt.Println(result2)

	// Output:
	// fallback
	// actual-value
}

// Example_coalesceFunction demonstrates using coalesce for multi-level fallback.
// coalesce returns the first non-empty value from its arguments.
func Example_coalesceFunction() {
	_ = os.Setenv("BACKUP_URL", "https://backup.example.com")
	defer func() { _ = os.Unsetenv("BACKUP_URL") }()

	// coalesce returns first non-empty value
	result, _ := tmpl.ExpandTemplate(`{{coalesce (env "PRIMARY_URL") (env "BACKUP_URL") "https://default.example.com"}}`)
	fmt.Println(result)

	// Output:
	// https://backup.example.com
}

// Example_directVarAccess demonstrates Taskfile-style direct variable access.
// Environment variables can be accessed directly via {{.VAR_NAME}} syntax.
func Example_directVarAccess() {
	_ = os.Setenv("DATABASE_HOST", "localhost")
	_ = os.Setenv("DATABASE_PORT", "5432")
	defer func() {
		_ = os.Unsetenv("DATABASE_HOST")
		_ = os.Unsetenv("DATABASE_PORT")
	}()

	// Direct access to environment variables (Taskfile style)
	result, _ := tmpl.ExpandTemplate(`{{.DATABASE_HOST}}:{{.DATABASE_PORT}}`)
	fmt.Println(result)

	// Combine with default for missing variables
	result2, _ := tmpl.ExpandTemplate(`{{.MISSING_VAR | default "fallback"}}`)
	fmt.Println(result2)

	// Output:
	// localhost:5432
	// fallback
}
