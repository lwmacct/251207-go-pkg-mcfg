package cfgm

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := t.TempDir() + "/config.yaml"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

type environmentMutationSource map[string]string

func (environmentMutationSource) Name() string { return "environment-mutation" }

func (s environmentMutationSource) Load(ctx context.Context, _ Schema) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for name, value := range s {
		if err := os.Setenv(name, value); err != nil {
			return nil, err
		}
	}

	return map[string]any{}, nil
}

func TestManagerLoadsExplicitSourcesInOrder(t *testing.T) {
	type Config struct {
		Name  string `json:"name"`
		Debug bool   `json:"debug"`
	}
	path := writeTempConfig(t, "name: from-file\ndebug: true\n")
	t.Setenv("APP_NAME", "from-env")

	cfg, err := MustNew(Config{Name: "default"}, WithoutDefaultPaths()).Load(t.Context(), File(path), Env("APP_"))
	require.NoError(t, err)
	assert.Equal(t, "from-env", cfg.Name)
	assert.True(t, cfg.Debug)
}

func TestManagerMustLoadPanicsOnError(t *testing.T) {
	type Config struct {
		Name string `json:"name"`
	}
	manager := MustNew(Config{}, WithoutDefaultPaths())
	assert.PanicsWithValue(t,
		"cfgm: failed to load config: file:/missing/config.yaml: none of the config files exist: /missing/config.yaml",
		func() { manager.MustLoad(t.Context(), File("/missing/config.yaml")) },
	)
}

func TestManagerLoadReportAndLogger(t *testing.T) {
	type Config struct {
		Name string `json:"name"`
	}
	path := writeTempConfig(t, "name: from-file\n")
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg, report, err := MustNew(Config{}, WithoutDefaultPaths(), Logger(logger)).LoadReport(t.Context(), File(path))
	require.NoError(t, err)
	assert.Equal(t, "from-file", cfg.Name)
	require.Len(t, report.Sources, 1)
	assert.Equal(t, "file:"+path, report.Sources[0].Name)
	assert.Equal(t, []string{"name"}, report.Sources[0].Keys)
	assert.Contains(t, logs.String(), "Loaded config source")
}

func TestManagerSearchesDefaultPaths(t *testing.T) {
	type Config struct {
		Name string `json:"name"`
	}
	tests := []struct {
		name    string
		file    string
		content string
	}{
		{name: "yaml", file: "config.yaml", content: "name: from-yaml\n"},
		{name: "yml", file: "config.yml", content: "name: from-yml\n"},
		{name: "json", file: "config.json", content: `{"name":"from-json"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			require.NoError(t, os.WriteFile(test.file, []byte(test.content), 0o600))
			cfg, err := MustNew(Config{Name: "default"}).Load(t.Context())
			require.NoError(t, err)
			assert.NotEqual(t, "default", cfg.Name)
		})
	}
}

func TestJSONFilesUseStrictGo127Semantics(t *testing.T) {
	type Config struct {
		Name  string `json:"name"`
		Count uint64 `json:"count"`
	}
	manager := MustNew(Config{}, WithoutDefaultPaths())

	writeJSON := func(t *testing.T, content []byte) string {
		t.Helper()
		path := t.TempDir() + "/config.json"
		require.NoError(t, os.WriteFile(path, content, 0o600))
		return path
	}

	t.Run("preserves large integers", func(t *testing.T) {
		path := writeJSON(t, []byte(`{"name":"app","count":18446744073709551615}`))
		cfg, err := manager.Load(t.Context(), File(path))
		require.NoError(t, err)
		assert.Equal(t, uint64(18446744073709551615), cfg.Count)
	})

	t.Run("rejects duplicate names", func(t *testing.T) {
		path := writeJSON(t, []byte(`{"name":"first","name":"second"}`))
		_, err := manager.Load(t.Context(), File(path))
		require.ErrorContains(t, err, `duplicate object member name "name"`)
	})

	t.Run("matches field names exactly", func(t *testing.T) {
		path := writeJSON(t, []byte(`{"Name":"wrong-case"}`))
		_, err := manager.Load(t.Context(), File(path))
		require.ErrorContains(t, err, `unknown object member name "Name"`)
	})

	t.Run("rejects invalid UTF-8", func(t *testing.T) {
		path := writeJSON(t, []byte{'{', '"', 'n', 'a', 'm', 'e', '"', ':', '"', 0xff, '"', '}'})
		_, err := manager.Load(t.Context(), File(path))
		require.ErrorContains(t, err, "invalid UTF-8")
	})
}

func TestDurationJSONUsesStringRepresentation(t *testing.T) {
	type Config struct {
		Timeout time.Duration `json:"timeout"`
	}
	manager := MustNew(Config{}, WithoutDefaultPaths())

	path := t.TempDir() + "/config.json"
	require.NoError(t, os.WriteFile(path, []byte(`{"timeout":"30s"}`), 0o600))
	cfg, err := manager.Load(t.Context(), File(path))
	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, cfg.Timeout)

	require.NoError(t, os.WriteFile(path, []byte(`{"timeout":30000000000}`), 0o600))
	_, err = manager.Load(t.Context(), File(path))
	require.ErrorContains(t, err, "time.Duration")

	jsonData, err := MarshalJSON(Config{Timeout: 30 * time.Second})
	require.NoError(t, err)
	assert.JSONEq(t, `{"timeout":"30s"}`, string(jsonData))
}

func TestCompositeEnvironmentJSONUsesStrictGo127Semantics(t *testing.T) {
	type Config struct {
		Labels map[string]string `json:"labels"`
	}
	t.Setenv("APP_LABELS", `{"region":"cn","region":"us"}`)
	_, err := MustNew(Config{}, WithoutDefaultPaths()).Load(t.Context(), Env("APP_"))
	require.ErrorContains(t, err, `duplicate object member name "region"`)
}

func TestManagerRejectsWeakScalarConversions(t *testing.T) {
	type Config struct {
		Name string `json:"name"`
	}
	manager := MustNew(Config{}, WithoutDefaultPaths())
	path := writeTempConfig(t, "name: 42\n")
	_, err := manager.Load(t.Context(), File(path))
	require.ErrorContains(t, err, `Go string`)
}

func TestManagerRejectsNullForNonNullableFields(t *testing.T) {
	type Config struct {
		Name string `json:"name"`
	}
	manager := MustNew(Config{}, WithoutDefaultPaths())
	path := writeTempConfig(t, "name: null\n")
	_, err := manager.Load(t.Context(), File(path))
	require.ErrorContains(t, err, "cannot be null")
}

func TestDefaultPathOrderAndOptions(t *testing.T) {
	type Config struct {
		Name string `json:"name"`
	}
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("config.yaml", []byte("name: from-yaml\n"), 0o600))
	require.NoError(t, os.WriteFile("config.json", []byte(`{"name":"from-json"}`), 0o600))

	cfg, err := MustNew(Config{Name: "default"}).Load(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "from-yaml", cfg.Name)

	cfg, err = MustNew(Config{Name: "default"}, WithoutDefaultPaths()).Load(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "default", cfg.Name)

	explicit := writeTempConfig(t, "name: explicit\n")
	cfg, err = MustNew(Config{Name: "default"}).Load(t.Context(), File(explicit))
	require.NoError(t, err)
	assert.Equal(t, "explicit", cfg.Name)
}

func TestManagerDefaultPathsAreOptional(t *testing.T) {
	type Config struct {
		Name string `json:"name"`
	}
	t.Chdir(t.TempDir())
	cfg, err := MustNew(Config{Name: "default"}).Load(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "default", cfg.Name)
}

func TestManagerUnknownKeyPolicy(t *testing.T) {
	type Config struct {
		Name string `json:"name"`
	}
	path := writeTempConfig(t, "name: app\ntypo: true\n")

	_, err := MustNew(Config{}, WithoutDefaultPaths()).Load(t.Context(), File(path))
	require.ErrorContains(t, err, "unknown object member")
	require.ErrorContains(t, err, "typo")

	cfg, err := MustNew(Config{}, WithoutDefaultPaths(), AllowUnknownKeys()).Load(t.Context(), File(path))
	require.NoError(t, err)
	assert.Equal(t, "app", cfg.Name)
}

func TestAllowUnknownKeysStillValidatesKnownFieldShapes(t *testing.T) {
	type Config struct {
		Names []string `json:"names"`
	}
	path := writeTempConfig(t, "names: wrong\nextra: true\n")
	_, err := MustNew(Config{}, WithoutDefaultPaths(), AllowUnknownKeys()).Load(t.Context(), File(path))
	require.ErrorContains(t, err, `Go []string`)
	require.ErrorContains(t, err, `/names`)
}

func TestManagerNullableStructs(t *testing.T) {
	type Provider struct {
		Issuer string `json:"issuer"`
	}
	type Config struct {
		Provider *Provider `json:"provider"`
	}
	manager := MustNew(Config{}, WithoutDefaultPaths())

	path := writeTempConfig(t, "provider:\n  issuer: https://auth.example.com\n")
	cfg, err := manager.Load(t.Context(), File(path))
	require.NoError(t, err)
	require.NotNil(t, cfg.Provider)
	assert.Equal(t, "https://auth.example.com", cfg.Provider.Issuer)

	path = writeTempConfig(t, "provider: {}\n")
	cfg, err = manager.Load(t.Context(), File(path))
	require.NoError(t, err)
	require.NotNil(t, cfg.Provider)

	path = writeTempConfig(t, "provider: null\n")
	cfg, err = MustNew(Config{Provider: &Provider{Issuer: "default"}}, WithoutDefaultPaths()).Load(t.Context(), File(path))
	require.NoError(t, err)
	assert.Nil(t, cfg.Provider)
}

func TestManagerRejectsUnknownSiblingOfNullableStruct(t *testing.T) {
	type Provider struct {
		Issuer string `json:"issuer"`
	}
	type Config struct {
		Provider *Provider `json:"provider"`
	}
	path := writeTempConfig(t, "provider: null\nprovider-typo: null\n")
	_, err := MustNew(Config{}, WithoutDefaultPaths()).Load(t.Context(), File(path))
	require.ErrorContains(t, err, "provider-typo")
}

func TestFileOptionalAndRequired(t *testing.T) {
	type Config struct {
		Name string `json:"name"`
	}
	manager := MustNew(Config{Name: "default"}, WithoutDefaultPaths())
	cfg, err := manager.Load(t.Context(), File("/path/does/not/exist.yaml", Optional()))
	require.NoError(t, err)
	assert.Equal(t, "default", cfg.Name)

	_, err = manager.Load(t.Context(), File("/path/does/not/exist.yaml"))
	require.ErrorContains(t, err, "none of the config files exist")
}

func TestManagerTemplateExpansion(t *testing.T) {
	type Config struct {
		Name     string `json:"name"`
		Fallback string `json:"fallback"`
		Price    string `json:"price"`
	}
	t.Setenv("CFG_NAME", "from-template")
	t.Setenv("CFG_DEFAULT", "from-default-template")
	path := writeTempConfig(t, "name: ${CFG_NAME}\nprice: $$10\n")

	cfg, err := MustNew(Config{Fallback: "${CFG_DEFAULT}"}, WithoutDefaultPaths()).Load(t.Context(), File(path))
	require.NoError(t, err)
	assert.Equal(t, "from-template", cfg.Name)
	assert.Equal(t, "from-default-template", cfg.Fallback)
	assert.Equal(t, "$10", cfg.Price)

	cfg, err = MustNew(Config{Fallback: "${CFG_DEFAULT}"}, WithoutDefaultPaths(), WithoutTemplateExpansion()).Load(t.Context(), File(path))
	require.NoError(t, err)
	assert.Equal(t, "${CFG_NAME}", cfg.Name)
	assert.Equal(t, "${CFG_DEFAULT}", cfg.Fallback)
	assert.Equal(t, "$$10", cfg.Price)
}

func TestManagerExpandsOnlyEffectiveTemplateValues(t *testing.T) {
	type Config struct {
		TokenSecret string `json:"token-secret"`
	}
	manager := MustNew(Config{
		TokenSecret: `${DIRECTIVE_TOKEN_SECRET:?DIRECTIVE_TOKEN_SECRET is required}`,
	}, WithoutDefaultPaths())

	t.Run("file overrides default template", func(t *testing.T) {
		path := writeTempConfig(t, "token-secret: from-file\n")
		cfg, err := manager.Load(t.Context(), File(path))
		require.NoError(t, err)
		assert.Equal(t, "from-file", cfg.TokenSecret)
	})

	t.Run("env source overrides file template", func(t *testing.T) {
		path := writeTempConfig(t, "token-secret: ${FILE_TOKEN_SECRET:?FILE_TOKEN_SECRET is required}\n")
		t.Setenv("APP_TOKEN_SECRET", "from-env-source")
		cfg, err := manager.Load(t.Context(), File(path), Env("APP_"))
		require.NoError(t, err)
		assert.Equal(t, "from-env-source", cfg.TokenSecret)
	})

	t.Run("effective template still fails", func(t *testing.T) {
		_, err := manager.Load(t.Context())
		require.Error(t, err)
		require.ErrorContains(t, err, "root.token-secret")
		assert.ErrorContains(t, err, "DIRECTIVE_TOKEN_SECRET is required")
	})
}

func TestManagerEscapesLiteralTemplate(t *testing.T) {
	type Config struct {
		Value string `json:"value"`
	}
	cfg, err := MustNew(Config{Value: `$${LITERAL}`}, WithoutDefaultPaths()).Load(t.Context())
	require.NoError(t, err)
	assert.Equal(t, `${LITERAL}`, cfg.Value)
}

func TestManagerUsesOneEnvironmentSnapshotPerLoad(t *testing.T) {
	type Config struct {
		FromEnv      string `json:"from-env"`
		Interpolated string `json:"interpolated"`
	}
	t.Setenv("APP_FROM_ENV", "before")
	t.Setenv("TEMPLATE_VALUE", "before")
	manager := MustNew(Config{Interpolated: `${TEMPLATE_VALUE}`}, WithoutDefaultPaths())

	cfg, err := manager.Load(t.Context(), environmentMutationSource{
		"APP_FROM_ENV":   "after",
		"TEMPLATE_VALUE": "after",
	}, Env("APP_"))
	require.NoError(t, err)
	assert.Equal(t, "before", cfg.FromEnv)
	assert.Equal(t, "before", cfg.Interpolated)
}

func TestManagerTemplateExpansionPreservesParsedStructure(t *testing.T) {
	type Config struct {
		Injected bool   `json:"injected"`
		Name     string `json:"name"`
	}
	t.Setenv("CFG_NAME", "safe\ninjected: true")
	path := writeTempConfig(t, "name: ${CFG_NAME}\n")

	cfg, err := MustNew(Config{}, WithoutDefaultPaths()).Load(t.Context(), File(path))
	require.NoError(t, err)
	assert.Equal(t, "safe\ninjected: true", cfg.Name)
	assert.False(t, cfg.Injected)
}

func TestManagerTemplateExpansionReportsValuePath(t *testing.T) {
	type Redis struct {
		Password string `json:"password"`
	}
	type Config struct {
		Redis Redis `json:"redis"`
	}
	path := writeTempConfig(t, "redis:\n  password: ${REDISCLI_AUTH:?Redis password is required}\n")

	_, err := MustNew(Config{}, WithoutDefaultPaths()).Load(t.Context(), File(path))
	require.Error(t, err)
	require.ErrorContains(t, err, "root.redis.password")
	assert.ErrorContains(t, err, "Redis password is required")
}

func TestManagerRejectsNilContext(t *testing.T) {
	type Config struct {
		Name string `json:"name"`
	}
	var ctx context.Context
	_, err := MustNew(Config{}, WithoutDefaultPaths()).Load(ctx)
	require.ErrorContains(t, err, "nil context")
}

func TestExampleYAMLKeepsMapCommentOnMap(t *testing.T) {
	type Provider struct {
		Issuer string `json:"issuer"`
	}
	type Config struct {
		Credentials map[string]string `json:"credentials" desc:"Credential map"`
		Provider    *Provider         `json:"provider"    desc:"Optional provider"`
	}
	yamlData, err := ExampleYAML(Config{Credentials: map[string]string{"admin": "digest"}})
	require.NoError(t, err)
	yaml := string(yamlData)
	assert.Contains(t, yaml, "# Credential map\ncredentials:")
	assert.Contains(t, yaml, "# Optional provider\nprovider: null")
	assert.NotContains(t, yaml, "provider: null # Credential map")
}

func TestRenderersReturnDefinitionErrors(t *testing.T) {
	type Base struct {
		Name string `json:"name"`
	}
	type Config struct {
		Base `json:",embed,omitzero"`
	}

	_, err := MarshalJSON(Config{})
	require.ErrorContains(t, err, "may only use json:,embed")
	_, err = MarshalYAML(Config{})
	require.ErrorContains(t, err, "may only use json:,embed")
	_, err = ExampleYAML(Config{})
	require.ErrorContains(t, err, "may only use json:,embed")
}

func TestDefaultPaths(t *testing.T) {
	assert.Equal(t, []string{
		"config.yaml", "config.yml", "config.json",
		"config/config.yaml", "config/config.yml", "config/config.json",
	}, DefaultPaths())
	assert.Len(t, DefaultPaths("app"), 15)
}

func TestManagerHonorsCanceledContext(t *testing.T) {
	type Config struct {
		Name string `json:"name"`
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := MustNew(Config{}, WithoutDefaultPaths()).Load(ctx, Env("APP_"))
	require.ErrorIs(t, err, context.Canceled)
}
