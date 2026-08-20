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

type InlineReloadConfig struct {
	Certificates       []bindingTLSCertificate `json:"certificates"        desc:"TLS certificates"`
	DefaultCertificate string                  `json:"default-certificate" desc:"Default TLS certificate"`
	PollInterval       time.Duration           `json:"poll-interval"        desc:"TLS poll interval"`
}

type InlineConfig struct {
	InlineReloadConfig `cfgm:",inline"`

	Enabled bool `json:"enabled" desc:"TLS enabled"`
}

func inlineDefaults() InlineConfig {
	return InlineConfig{
		Enabled:            true,
		Certificates:       []bindingTLSCertificate{{ID: "default", Certificate: "cert.pem", PrivateKey: "key.pem"}},
		DefaultCertificate: "default",
		PollInterval:       time.Minute,
	}
}

func TestManagerLoadsInlineFieldsFromAllSources(t *testing.T) {
	path := writeTempConfig(t, "default-certificate: file\n")
	t.Setenv("APP_POLL_INTERVAL", "2m")
	manager := New(inlineDefaults(), WithoutDefaultPaths())

	cfg, err := manager.Load(t.Context(), File(path), Env("APP_"))
	require.NoError(t, err)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, "file", cfg.DefaultCertificate)
	assert.Equal(t, 2*time.Minute, cfg.PollInterval)
	require.Len(t, cfg.Certificates, 1)
	assert.Equal(t, "default", cfg.Certificates[0].ID)

	var loaded *InlineConfig
	root := configuredRoot(t, manager, func(_ context.Context, _ *cli.Command, cfg *InlineConfig) error {
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

func TestInlineFieldsAppearInSchemaAndMarshaledConfig(t *testing.T) {
	cfg := inlineDefaults()
	manager := New(cfg, WithoutDefaultPaths())
	paths := make([]string, len(manager.schema.fields))
	for index, field := range manager.schema.fields {
		paths[index] = field.path
	}
	assert.Equal(t, []string{"certificates", "default-certificate", "poll-interval", "enabled"}, paths)
	assert.Equal(t, "Default TLS certificate", manager.schema.fields[1].desc)

	yamlText := string(MarshalYAML(cfg))
	assert.Contains(t, yamlText, "default-certificate: default")
	assert.NotContains(t, strings.ToLower(yamlText), "inlinereloadconfig")

	jsonText := string(MarshalJSON(cfg))
	assert.Contains(t, jsonText, `"default-certificate": "default"`)
	assert.NotContains(t, jsonText, "InlineReloadConfig")

	example := string(ExampleYAML(cfg))
	assert.Contains(t, example, "default-certificate: \"default\" # Default TLS certificate")
}

func TestManagerPreservesParentPathForInlineFields(t *testing.T) {
	type Config struct {
		TLS InlineConfig `json:"tls"`
	}
	manager := New(Config{TLS: inlineDefaults()}, WithoutDefaultPaths())
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

func TestManagerValidatesInlineFieldsInsideStructSlices(t *testing.T) {
	type ItemBase struct {
		Name string `json:"name"`
	}
	type Item struct {
		ItemBase `cfgm:",inline"`

		Enabled bool `json:"enabled"`
	}
	type Config struct {
		Items []Item `json:"items"`
	}

	manager := New(Config{}, WithoutDefaultPaths())
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

func TestManagerRejectsInvalidInlineFields(t *testing.T) {
	t.Run("named field", func(t *testing.T) {
		type Base struct {
			Name string `json:"name"`
		}
		type Config struct {
			Base Base `cfgm:",inline"`
		}
		assert.PanicsWithError(t, "cfgm: inline config field Base must be an anonymous non-pointer struct", func() {
			New(Config{})
		})
	})

	t.Run("pointer field", func(t *testing.T) {
		type Base struct {
			Name string `json:"name"`
		}
		type Config struct {
			*Base `cfgm:",inline"`
		}
		assert.PanicsWithError(t, "cfgm: inline config field Base must be an anonymous non-pointer struct", func() {
			New(Config{})
		})
	})

	t.Run("named tag", func(t *testing.T) {
		type Base struct {
			Name string `json:"name"`
		}
		type Config struct {
			Base `json:"base" cfgm:",inline"`
		}
		assert.PanicsWithError(t, "cfgm: inline config field Base must not have a name", func() {
			New(Config{})
		})
	})

	t.Run("json options", func(t *testing.T) {
		type Base struct {
			Name string `json:"name"`
		}
		type Config struct {
			Base `json:"-" cfgm:",inline"`
		}
		assert.PanicsWithError(t, "cfgm: inline config field Base must not have a json tag", func() {
			New(Config{})
		})
	})

	t.Run("invalid cfgm tag", func(t *testing.T) {
		type Base struct {
			Name string `json:"name"`
		}
		type Config struct {
			Base `cfgm:"inline"`
		}
		assert.PanicsWithError(t, `cfgm: config field Base has invalid cfgm tag "inline"`, func() {
			New(Config{})
		})
	})

	t.Run("codec", func(t *testing.T) {
		type Base struct {
			Name string `json:"name"`
		}
		type Config struct {
			Base `cfgm:",inline"`
		}
		assert.PanicsWithError(t, "cfgm: inline config type cfgm.Base cannot use a codec", func() {
			New(Config{}, WithCodec(Codec[Base]{
				Parse:  func(string) (Base, error) { return Base{}, nil },
				Format: func(Base) string { return "" },
			}))
		})
	})

	t.Run("duplicate key", func(t *testing.T) {
		type Base struct {
			Name string `json:"name"`
		}
		type Config struct {
			Base `cfgm:",inline"`

			Name string `json:"name"`
		}
		assert.PanicsWithError(t, `cfgm: duplicate config path "name"`, func() {
			New(Config{})
		})
	})
}
