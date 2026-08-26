package cfgm

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

type EmbeddedReloadConfig struct {
	Certificates       []bindingTLSCertificate `json:"certificates"        desc:"TLS certificates"`
	DefaultCertificate string                  `json:"default-certificate" desc:"Default TLS certificate"`
	PollInterval       time.Duration           `json:"poll-interval"        desc:"TLS poll interval"`
}

type EmbeddedConfig struct {
	EmbeddedReloadConfig `json:",embed"`

	Enabled bool `json:"enabled" desc:"TLS enabled"`
}

type EmbeddedJSONMethodConfig struct {
	Name string `json:"name"`
}

func (EmbeddedJSONMethodConfig) MarshalJSON() ([]byte, error) { return []byte(`{}`), nil }

func embeddedDefaults() EmbeddedConfig {
	return EmbeddedConfig{
		Enabled:            true,
		Certificates:       []bindingTLSCertificate{{ID: "default", Certificate: "cert.pem", PrivateKey: "key.pem"}},
		DefaultCertificate: "default",
		PollInterval:       time.Minute,
	}
}

func TestManagerLoadsEmbeddedFieldsFromAllSources(t *testing.T) {
	path := writeTempConfig(t, "default-certificate: file\n")
	t.Setenv("APP_POLL_INTERVAL", "2m")
	manager := MustNew(embeddedDefaults(), WithoutDefaultPaths())

	cfg, err := manager.Load(t.Context(), File(path), Env("APP_"))
	require.NoError(t, err)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, "file", cfg.DefaultCertificate)
	assert.Equal(t, 2*time.Minute, cfg.PollInterval)
	require.Len(t, cfg.Certificates, 1)
	assert.Equal(t, "default", cfg.Certificates[0].ID)

	var loaded *EmbeddedConfig
	root := configuredRoot(t, manager, func(_ context.Context, _ *cli.Command, cfg *EmbeddedConfig) error {
		loaded = cfg
		return nil
	})
	requireFlagType[*cli.StringFlag](t, root.Flags, "default-certificate")
	requireFlagType[*cli.DurationFlag](t, root.Flags, "poll-interval")
	require.NoError(t, root.Run(t.Context(), []string{"app", "--default-certificate=cli", "--poll-interval=3m"}))
	require.NotNil(t, loaded)
	assert.Equal(t, "cli", loaded.DefaultCertificate)
	assert.Equal(t, 3*time.Minute, loaded.PollInterval)
}

func TestManagerUsesJSONDefaultFieldNames(t *testing.T) {
	type Config struct {
		Name string
	}
	manager := MustNew(Config{}, WithoutDefaultPaths())
	assert.True(t, manager.schema.isFieldPath("Name"))

	path := writeTempConfig(t, "Name: file\n")
	cfg, err := manager.Load(t.Context(), File(path))
	require.NoError(t, err)
	assert.Equal(t, "file", cfg.Name)

	t.Setenv("APP_NAME", "env")
	cfg, err = manager.Load(t.Context(), Env("APP_"))
	require.NoError(t, err)
	assert.Equal(t, "env", cfg.Name)

	yamlData, err := MarshalYAML(Config{Name: "yaml"})
	require.NoError(t, err)
	assert.Contains(t, string(yamlData), "Name: yaml")
	jsonData, err := MarshalJSON(Config{Name: "json"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"Name":"json"}`, string(jsonData))
}

func TestEmbeddedFieldsAppearInSchemaAndMarshaledConfig(t *testing.T) {
	cfg := embeddedDefaults()
	manager := MustNew(cfg, WithoutDefaultPaths())
	paths := make([]string, len(manager.schema.fields))
	for index, field := range manager.schema.fields {
		paths[index] = field.path
	}
	assert.Equal(t, []string{"certificates", "default-certificate", "poll-interval", "enabled"}, paths)
	assert.Equal(t, "Default TLS certificate", manager.schema.fields[1].desc)

	yamlData, err := MarshalYAML(cfg)
	require.NoError(t, err)
	yamlText := string(yamlData)
	assert.Contains(t, yamlText, "default-certificate: default")
	assert.NotContains(t, strings.ToLower(yamlText), "embeddedreloadconfig")

	jsonData, err := MarshalJSON(cfg)
	require.NoError(t, err)
	jsonText := string(jsonData)
	assert.Contains(t, jsonText, `"default-certificate": "default"`)
	assert.NotContains(t, jsonText, "EmbeddedReloadConfig")

	exampleData, err := ExampleYAML(cfg)
	require.NoError(t, err)
	example := string(exampleData)
	assert.Contains(t, example, "default-certificate: \"default\" # Default TLS certificate")
}

func TestManagerPreservesParentPathForEmbeddedFields(t *testing.T) {
	type Config struct {
		TLS EmbeddedConfig `json:"tls"`
	}
	manager := MustNew(Config{TLS: embeddedDefaults()}, WithoutDefaultPaths())
	paths := make([]string, len(manager.schema.fields))
	for index, field := range manager.schema.fields {
		paths[index] = field.path
	}
	assert.Equal(t, []string{
		"tls.certificates", "tls.default-certificate", "tls.poll-interval", "tls.enabled",
	}, paths)

	path := writeTempConfig(t, "tls:\n  default-certificate: nested\n")
	cfg, err := manager.Load(t.Context(), File(path))
	require.NoError(t, err)
	assert.Equal(t, "nested", cfg.TLS.DefaultCertificate)
	assert.Equal(t, time.Minute, cfg.TLS.PollInterval)
}

func TestManagerValidatesEmbeddedFieldsInsideStructSlices(t *testing.T) {
	type ItemBase struct {
		Name string `json:"name"`
	}
	type Item struct {
		ItemBase `json:",embed"`

		Enabled bool `json:"enabled"`
	}
	type Config struct {
		Items []Item `json:"items"`
	}

	manager := MustNew(Config{}, WithoutDefaultPaths())
	path := writeTempConfig(t, "items:\n  - enabled: true\n    name: file\n")
	cfg, err := manager.Load(t.Context(), File(path))
	require.NoError(t, err)
	require.Len(t, cfg.Items, 1)
	assert.Equal(t, "file", cfg.Items[0].Name)

	path = writeTempConfig(t, "items:\n  - enabled: true\n    typo: invalid\n")
	_, err = manager.Load(t.Context(), File(path))
	require.ErrorContains(t, err, `unknown object member name "typo"`)
	require.ErrorContains(t, err, `/items/0`)

	var loaded *Config
	root := configuredRoot(t, manager, func(_ context.Context, _ *cli.Command, cfg *Config) error {
		loaded = cfg
		return nil
	})
	require.NoError(t, root.Run(t.Context(), []string{"app", `--items={"enabled":true,"name":"cli"}`}))
	require.Len(t, loaded.Items, 1)
	assert.Equal(t, "cli", loaded.Items[0].Name)
}

func TestManagerEmbeddedFieldRules(t *testing.T) {
	t.Run("anonymous struct is implicitly embedded", func(t *testing.T) {
		type Base struct {
			Name string `json:"name"`
		}
		type Config struct {
			Base
		}
		manager := MustNew(Config{}, WithoutDefaultPaths())
		assert.True(t, manager.schema.isFieldPath("name"))
	})

	t.Run("named field is supported", func(t *testing.T) {
		type Base struct {
			Name string `json:"name"`
		}
		type Config struct {
			Base Base `json:",embed"`
		}
		manager := MustNew(Config{}, WithoutDefaultPaths())
		assert.True(t, manager.schema.isFieldPath("name"))
		jsonData, err := MarshalJSON(Config{Base: Base{Name: "json"}})
		require.NoError(t, err)
		assert.JSONEq(t, `{"name":"json"}`, string(jsonData))
	})

	t.Run("pointer field is supported", func(t *testing.T) {
		type Base struct {
			Name string `json:"name"`
		}
		type Config struct {
			*Base `json:",embed"`
		}
		manager := MustNew(Config{}, WithoutDefaultPaths())
		assert.True(t, manager.schema.isFieldPath("name"))
		path := writeTempConfig(t, "name: loaded\n")
		cfg, err := manager.Load(t.Context(), File(path))
		require.NoError(t, err)
		require.NotNil(t, cfg.Base)
		assert.Equal(t, "loaded", cfg.Name)
	})

	t.Run("pointer field receives environment and CLI values", func(t *testing.T) {
		type Base struct {
			Name string `json:"name"`
		}
		type Config struct {
			*Base `json:",embed"`
		}
		manager := MustNew(Config{}, WithoutDefaultPaths())
		t.Setenv("APP_NAME", "env")
		cfg, err := manager.Load(t.Context(), Env("APP_"))
		require.NoError(t, err)
		require.NotNil(t, cfg.Base)
		assert.Equal(t, "env", cfg.Name)

		var loaded *Config
		root := configuredRoot(t, manager, func(_ context.Context, _ *cli.Command, cfg *Config) error {
			loaded = cfg
			return nil
		})
		require.NoError(t, root.Run(t.Context(), []string{"app", "--name=cli"}))
		require.NotNil(t, loaded)
		require.NotNil(t, loaded.Base)
		assert.Equal(t, "cli", loaded.Name)
	})

	t.Run("named json option", func(t *testing.T) {
		type Base struct {
			Name string `json:"name"`
		}
		type Config struct {
			Base `json:"base,embed"`
		}
		_, err := New(Config{})
		require.EqualError(t, err, "cfgm: embedded field Base may only use json:,embed")
	})

	t.Run("extra json option", func(t *testing.T) {
		type Base struct {
			Name string `json:"name"`
		}
		type Config struct {
			Base `json:",embed,omitzero"`
		}
		_, err := New(Config{})
		require.EqualError(t, err, "cfgm: embedded field Base may only use json:,embed")
	})

	t.Run("map fallback", func(t *testing.T) {
		type Base map[string]string
		type Config struct {
			Base `json:",embed"`
		}
		_, err := New(Config{})
		require.EqualError(t, err, "cfgm: embedded field Base must be a struct or pointer to struct; map fallbacks are unsupported")
	})

	t.Run("json methods", func(t *testing.T) {
		type Config struct {
			EmbeddedJSONMethodConfig `json:",embed"`
		}
		_, err := New(Config{})
		require.EqualError(t, err, "cfgm: embedded config type cfgm.EmbeddedJSONMethodConfig must not implement JSON or text methods")
	})

	t.Run("removed cfgm tag", func(t *testing.T) {
		type Base struct {
			Name string `json:"name"`
		}
		type Config struct {
			Base `cfgm:",inline"`
		}
		_, err := New(Config{})
		require.EqualError(t, err, `cfgm: field Base uses removed cfgm tag ",inline"; use json tags`)
	})

	t.Run("codec", func(t *testing.T) {
		type Base struct {
			Name string `json:"name"`
		}
		type Config struct {
			Base `json:",embed"`
		}
		_, err := New(Config{}, WithCodec(Codec[Base]{
			Parse:  func(string) (Base, error) { return Base{}, nil },
			Format: func(Base) string { return "" },
		}))
		require.EqualError(t, err, "cfgm: embedded config type cfgm.Base cannot use a codec")
	})

	t.Run("duplicate key", func(t *testing.T) {
		type Base struct {
			Name string `json:"name"`
		}
		type Config struct {
			Base `json:",embed"`

			Name string `json:"name"`
		}
		_, err := New(Config{})
		require.EqualError(t, err, `cfgm: duplicate config path "name"`)
	})
}
