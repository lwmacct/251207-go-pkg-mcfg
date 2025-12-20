package tmpl_test

import (
	"testing"

	"github.com/lwmacct/251207-go-pkg-cfgm/pkg/tmpl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateFunction_env(t *testing.T) {
	t.Setenv("TEST_VAR", "test-value")

	tests := []struct {
		name     string
		template string
		want     string
		wantErr  bool
	}{
		{
			name:     "env function with existing var",
			template: `{{env "TEST_VAR"}}`,
			want:     "test-value",
		},
		{
			name:     "env function with missing var",
			template: `{{env "MISSING_VAR"}}`,
			want:     "",
		},
		{
			name:     "env function with default",
			template: `{{env "MISSING_VAR" "default-value"}}`,
			want:     "default-value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tmpl.ExpandTemplate(tt.template)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTemplateFunction_default(t *testing.T) {
	tests := []struct {
		name     string
		template string
		want     string
		wantErr  bool
	}{
		{
			name:     "default with empty string",
			template: `{{env "MISSING_VAR" | default "fallback"}}`,
			want:     "fallback",
		},
		{
			name:     "default with non-empty value",
			template: `{{"value" | default "fallback"}}`,
			want:     "value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tmpl.ExpandTemplate(tt.template)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTemplateFunction_coalesce(t *testing.T) {
	t.Setenv("VAR1", "value1")

	tests := []struct {
		name     string
		template string
		want     string
		wantErr  bool
	}{
		{
			name:     "coalesce returns first non-empty",
			template: `{{coalesce "" "second" "third"}}`,
			want:     "second",
		},
		{
			name:     "coalesce with all empty",
			template: `{{coalesce "" "" ""}}`,
			want:     "<no value>",
		},
		{
			name:     "coalesce with env vars",
			template: `{{coalesce (env "MISSING1") (env "VAR1") "default"}}`,
			want:     "value1",
		},
		{
			name:     "coalesce with direct var access",
			template: `{{coalesce .MISSING .VAR1 "default"}}`,
			want:     "value1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tmpl.ExpandTemplate(tt.template)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTemplateData_DirectVarAccess(t *testing.T) {
	t.Setenv("MY_VAR", "my-value")

	tests := []struct {
		name     string
		template string
		want     string
		wantErr  bool
	}{
		{
			name:     "access env via .VAR (Taskfile style)",
			template: `{{.MY_VAR}}`,
			want:     "my-value",
		},
		{
			name:     "access missing var via .VAR",
			template: `{{.MISSING_VAR}}`,
			want:     "<no value>",
		},
		{
			name:     ".VAR with default",
			template: `{{.MISSING_VAR | default "fallback"}}`,
			want:     "fallback",
		},
		{
			name:     "coalesce with direct var access",
			template: `{{coalesce .MISSING .MY_VAR "default"}}`,
			want:     "my-value",
		},
		{
			name:     "mix env function and direct access",
			template: `{{coalesce (env "MISSING") .MY_VAR "default"}}`,
			want:     "my-value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tmpl.ExpandTemplate(tt.template)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExpandTemplate_JSONConfig(t *testing.T) {
	t.Setenv("API_KEY", "sk-test-123")
	t.Setenv("MODEL", "gpt-4")

	jsonConfig := "{\"name\": \"{{coalesce .AGENT_NAME `test-agent`}}\", \"model\": \"{{.MODEL | default `gpt-3.5-turbo`}}\", \"api_key\": \"{{.API_KEY}}\", \"max_tokens\": 2048}"

	expanded, err := tmpl.ExpandTemplate(jsonConfig)
	require.NoError(t, err, "tmpl.ExpandTemplate() should succeed")
	assert.NotEmpty(t, expanded, "tmpl.ExpandTemplate() should return non-empty string")
	assert.Contains(t, expanded, "gpt-4", "MODEL should be expanded to gpt-4")
	assert.Contains(t, expanded, "sk-test-123", "API_KEY should be expanded")
}

// =============================================================================
// 错误场景测试
// =============================================================================

func TestExpandTemplate_Errors(t *testing.T) {
	tests := []struct {
		name     string
		template string
		errMsg   string
	}{
		{
			name:     "invalid template syntax - unclosed action",
			template: `{{env "VAR"`,
			errMsg:   "unclosed action",
		},
		{
			name:     "invalid template syntax - undefined function",
			template: `{{undefined_func "arg"}}`,
			errMsg:   "undefined",
		},
		{
			name:     "invalid template syntax - missing argument",
			template: `{{env}}`,
			errMsg:   "wrong number of args",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tmpl.ExpandTemplate(tt.template)
			require.Error(t, err, "tmpl.ExpandTemplate() should return error for invalid template")
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}
