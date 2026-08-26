package cfgm

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

type bindingTLSCertificate struct {
	ID          string        `json:"id"          desc:"证书标识"`
	Certificate string        `json:"certificate" desc:"证书 URI"`
	PrivateKey  string        `json:"private-key" desc:"私钥 URI"`
	Refresh     time.Duration `json:"refresh"     desc:"刷新间隔"`
}

type bindingRoute struct {
	Path     string `json:"path"`
	Backends []struct {
		URL string `json:"url"`
	} `json:"backends"`
}

type bindingTestConfig struct {
	Server struct {
		Addr         string                  `json:"addr"         desc:"监听地址"`
		Debug        bool                    `json:"debug"        desc:"调试模式"`
		Workers      int                     `json:"workers"      desc:"工作线程"`
		Tags         []string                `json:"tags"         desc:"标签"`
		Certificates []bindingTLSCertificate `json:"certificates" desc:"证书列表"`
		Routes       []bindingRoute          `json:"routes"       desc:"路由列表"`
		Redis        struct {
			URL      string `json:"url"      desc:"Redis URL"`
			Password string `json:"password" desc:"Redis 密码"`
		} `json:"redis" desc:"Redis 配置"`
	} `json:"server" desc:"服务端配置"`
}

func bindingDefaults() bindingTestConfig {
	var cfg bindingTestConfig
	cfg.Server.Addr = ":8080"
	cfg.Server.Workers = 2
	cfg.Server.Tags = []string{"default"}
	cfg.Server.Certificates = []bindingTLSCertificate{{
		ID:          "default",
		Certificate: "file:///default.crt",
		PrivateKey:  "file:///default.key",
		Refresh:     time.Minute,
	}}
	cfg.Server.Redis.URL = "redis://localhost:6379"
	return cfg
}

func BenchmarkManagerLoadDefaults(b *testing.B) {
	manager := MustNew(bindingDefaults(), WithoutDefaultPaths())
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := manager.Load(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func TestManagerGeneratesTypedFlags(t *testing.T) {
	manager := MustNew(
		bindingDefaults(),
		AppName("testapp"),
		CLIAlias("server.addr", "a"),
		HideCLI("server.redis.password"),
	)
	server := &cli.Command{Name: "server", Action: manager.Action(func(context.Context, *cli.Command, *bindingTestConfig) error { return nil })}
	root := &cli.Command{Name: "app", Commands: []*cli.Command{server}}
	manager.MustConfigure(root)

	configFlag := requireFlagType[*cli.StringFlag](t, root.Flags, "config")
	assert.Equal(t, []string{"c"}, configFlag.Aliases)
	envPrefixFlag := requireFlagType[*cli.StringFlag](t, root.Flags, "env-prefix")
	assert.Equal(t, []string{"e"}, envPrefixFlag.Aliases)
	assert.Nil(t, findFlag(root.Flags, "server.addr"))

	flags := server.Flags
	addr := requireFlagType[*cli.StringFlag](t, flags, "addr")
	assert.Contains(t, addr.Aliases, "a")
	requireFlagType[*cli.BoolFlag](t, flags, "debug")
	requireFlagType[*cli.IntFlag](t, flags, "workers")
	requireFlagType[*cli.StringSliceFlag](t, flags, "tags")
	requireFlagType[*cli.GenericFlag](t, flags, "certificates")
	requireFlagType[*cli.StringFlag](t, flags, "redis.url")
	assert.Nil(t, findFlag(flags, "redis.password"))
}

func TestManagerTrimsNestedCommandPath(t *testing.T) {
	manager := MustNew(bindingDefaults(), WithoutDefaultPaths(), HideCLI("server.redis.password"))
	var loaded *bindingTestConfig
	redis := &cli.Command{
		Name: "redis",
		Action: manager.Action(func(_ context.Context, _ *cli.Command, cfg *bindingTestConfig) error {
			loaded = cfg
			return nil
		}),
	}
	root := &cli.Command{
		Name: "app",
		Commands: []*cli.Command{{
			Name:     "server",
			Commands: []*cli.Command{redis},
		}},
	}
	manager.MustConfigure(root)
	requireFlagType[*cli.StringFlag](t, redis.Flags, "url")
	assert.Nil(t, findFlag(redis.Flags, "password"))

	err := root.Run(t.Context(), []string{"app", "server", "redis", "--url=redis://nested:6379"})
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "redis://nested:6379", loaded.Server.Redis.URL)
}

func TestManagerActionRequiresConfiguration(t *testing.T) {
	manager := MustNew(bindingDefaults(), WithoutDefaultPaths())
	cmd := &cli.Command{Name: "app", Action: manager.Action(func(context.Context, *cli.Command, *bindingTestConfig) error { return nil })}
	err := cmd.Run(t.Context(), []string{"app"})
	require.ErrorContains(t, err, "must be configured")
}

func TestManagerLoadsSourcesInPriorityOrder(t *testing.T) {
	manager := MustNew(bindingDefaults(), AppName("testapp"), HideCLI("server.redis.password"))
	configPath := writeTempConfig(t, `
server:
  addr: ":8000"
  workers: 3
  redis:
    url: "redis://file:6379"
`)
	t.Setenv("TESTAPP_SERVER_ADDR", ":8100")
	t.Setenv("TESTAPP_SERVER_REDIS_URL", "redis://env:6379")

	var loaded *bindingTestConfig
	server := &cli.Command{
		Name: "server",
		Action: manager.Action(func(_ context.Context, _ *cli.Command, cfg *bindingTestConfig) error {
			loaded = cfg
			return nil
		}),
	}
	root := &cli.Command{
		Name:     "testapp",
		Commands: []*cli.Command{server},
	}
	manager.MustConfigure(root)

	err := root.Run(t.Context(), []string{
		"testapp", "--config", configPath, "server",
		"--addr=:8200",
		"--workers=4",
	})
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, ":8200", loaded.Server.Addr)
	assert.Equal(t, 4, loaded.Server.Workers)
	assert.Equal(t, "redis://env:6379", loaded.Server.Redis.URL)
}

func TestManagerCLIOverridesRequiredDefaultTemplate(t *testing.T) {
	defaults := bindingDefaults()
	defaults.Server.Addr = `${SERVER_ADDR:?SERVER_ADDR is required}`
	manager := MustNew(defaults, WithoutDefaultPaths())

	loaded, err := runManager(t, manager, "--addr=:8200")
	require.NoError(t, err)
	assert.Equal(t, ":8200", loaded.Server.Addr)
}

func TestManagerLoadsRepeatedStructValues(t *testing.T) {
	manager := MustNew(bindingDefaults())
	loaded, err := runManager(t, manager,
		`--certificates={"id":"main","certificate":"op://cert/main","private-key":"op://key/main","refresh":"30s"}`,
		`--certificates={"id":"api","certificate":"op://cert/api","private-key":"op://key/api","refresh":"1m"}`,
	)
	require.NoError(t, err)
	require.Len(t, loaded.Server.Certificates, 2)
	assert.Equal(t, "main", loaded.Server.Certificates[0].ID)
	assert.Equal(t, 30*time.Second, loaded.Server.Certificates[0].Refresh)
	assert.Equal(t, "api", loaded.Server.Certificates[1].ID)
	assert.Equal(t, time.Minute, loaded.Server.Certificates[1].Refresh)
}

func TestManagerStructValuesReplaceLowerPrioritySources(t *testing.T) {
	manager := MustNew(bindingDefaults())
	loaded, err := runManager(t, manager,
		`--certificates={"id":"only","certificate":"file:///only.crt","private-key":"file:///only.key"}`,
	)
	require.NoError(t, err)
	require.Len(t, loaded.Server.Certificates, 1)
	assert.Equal(t, "only", loaded.Server.Certificates[0].ID)
}

func TestManagerStructValuesCanBeCleared(t *testing.T) {
	manager := MustNew(bindingDefaults())
	loaded, err := runManager(t, manager, `--certificates=[]`)
	require.NoError(t, err)
	assert.Empty(t, loaded.Server.Certificates)
}

func TestManagerScalarCollectionsReplaceDefaults(t *testing.T) {
	manager := MustNew(bindingDefaults())
	loaded, err := runManager(t, manager, `--tags=cli`)
	require.NoError(t, err)
	assert.Equal(t, []string{"cli"}, loaded.Server.Tags)
}

func TestManagerRejectsInvalidStructValues(t *testing.T) {
	manager := MustNew(bindingDefaults())
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "invalid json", args: []string{`--certificates={`}, want: "certificates"},
		{name: "unknown field", args: []string{`--certificates={"id":"main","unknown":true}`}, want: "unknown"},
		{name: "non object", args: []string{`--certificates="main"`}, want: "JSON object"},
		{name: "clear mixed with value", args: []string{`--certificates=[]`, `--certificates={"id":"main"}`}, want: "cannot be combined"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := runManager(t, manager, test.args...)
			require.ErrorContains(t, err, test.want)
		})
	}
}

type bindingEndpoint string

func TestManagerUsesRegisteredCodec(t *testing.T) {
	type codecConfig struct {
		Endpoint bindingEndpoint `json:"endpoint" desc:"服务端点"`
	}
	manager := MustNew(codecConfig{}, WithCodec(Codec[bindingEndpoint]{
		Parse: func(value string) (bindingEndpoint, error) {
			if !strings.HasPrefix(value, "svc://") {
				return "", errors.New("scheme must be svc")
			}
			return bindingEndpoint(value), nil
		},
		Format: func(value bindingEndpoint) string { return string(value) },
	}))
	var loaded *codecConfig
	cmd := configuredRoot(t, manager, func(_ context.Context, _ *cli.Command, cfg *codecConfig) error {
		loaded = cfg
		return nil
	})
	require.NoError(t, cmd.Run(t.Context(), []string{"app", "--endpoint=svc://main"}))
	assert.Equal(t, bindingEndpoint("svc://main"), loaded.Endpoint)
	require.ErrorContains(t, cmd.Run(t.Context(), []string{"app", "--endpoint=http://main"}), "scheme must be svc")
}

func TestManagerRejectsUnknownFileFieldsInsideStructSlices(t *testing.T) {
	manager := MustNew(bindingDefaults())
	configPath := writeTempConfig(t, `
server:
  certificates:
    - id: main
      certificate: file:///main.crt
      private-key: file:///main.key
      unknown: true
`)

	_, err := runManagerWithRootArgs(t, manager, []string{"--config", configPath})
	require.ErrorContains(t, err, "unknown")
}

func TestManagerLoadsCompositeEnvironmentValuesAsJSON(t *testing.T) {
	type Config struct {
		Tags         []string                `json:"tags"`
		Labels       map[string]string       `json:"labels"`
		Certificates []bindingTLSCertificate `json:"certificates"`
	}
	t.Setenv("APP_TAGS", `["api","edge"]`)
	t.Setenv("APP_LABELS", `{"region":"cn"}`)
	t.Setenv("APP_CERTIFICATES", `[{"id":"main","refresh":"15s"}]`)

	cfg, err := MustNew(Config{}, WithoutDefaultPaths()).Load(t.Context(), Env("APP_"))
	require.NoError(t, err)
	assert.Equal(t, []string{"api", "edge"}, cfg.Tags)
	assert.Equal(t, map[string]string{"region": "cn"}, cfg.Labels)
	require.Len(t, cfg.Certificates, 1)
	assert.Equal(t, "main", cfg.Certificates[0].ID)
	assert.Equal(t, 15*time.Second, cfg.Certificates[0].Refresh)
}

func TestManagerEnvironmentCanSetEmptyScalar(t *testing.T) {
	type Config struct {
		Name string `json:"name"`
	}
	t.Setenv("APP_NAME", "")
	cfg, err := MustNew(Config{Name: "default"}, WithoutDefaultPaths()).Load(t.Context(), Env("APP_"))
	require.NoError(t, err)
	assert.Empty(t, cfg.Name)
}

func TestManagerRejectsInvalidCompositeEnvironmentValues(t *testing.T) {
	type Config struct {
		Tags []string `json:"tags"`
	}
	t.Setenv("APP_TAGS", "api,edge")
	_, err := MustNew(Config{}, WithoutDefaultPaths()).Load(t.Context(), Env("APP_"))
	require.ErrorContains(t, err, "APP_TAGS")
	require.ErrorContains(t, err, "JSON")
}

func TestManagerRejectsDeepUnknownFields(t *testing.T) {
	type Config struct {
		Routes []bindingRoute `json:"routes"`
	}
	path := writeTempConfig(t, `
routes:
  - path: /api
    backends:
      - url: https://api.example.com
        typo: true
`)
	_, err := MustNew(Config{}, WithoutDefaultPaths()).Load(t.Context(), File(path))
	require.ErrorContains(t, err, `unknown object member name "typo"`)
	require.ErrorContains(t, err, `/routes/0/backends/0`)
}

func TestManagerRejectsDeepUnknownStructFlagFields(t *testing.T) {
	manager := MustNew(bindingDefaults())
	_, err := runManager(t, manager,
		`--routes={"path":"/api","backends":[{"url":"https://api.example.com","typo":true}]}`,
	)
	require.ErrorContains(t, err, `unknown object member name "typo"`)
	require.ErrorContains(t, err, `/backends/0`)
}

func TestManagerCodecAppliesToFileAndEnvironment(t *testing.T) {
	type Config struct {
		Endpoint bindingEndpoint `json:"endpoint"`
	}
	manager := MustNew(Config{}, WithoutDefaultPaths(), WithCodec(Codec[bindingEndpoint]{
		Parse: func(value string) (bindingEndpoint, error) {
			if !strings.HasPrefix(value, "svc://") {
				return "", errors.New("scheme must be svc")
			}
			return bindingEndpoint(value), nil
		},
		Format: func(value bindingEndpoint) string { return string(value) },
	}))
	path := writeTempConfig(t, "endpoint: svc://file\n")
	cfg, err := manager.Load(t.Context(), File(path))
	require.NoError(t, err)
	assert.Equal(t, bindingEndpoint("svc://file"), cfg.Endpoint)

	t.Setenv("APP_ENDPOINT", "svc://env")
	cfg, err = manager.Load(t.Context(), Env("APP_"))
	require.NoError(t, err)
	assert.Equal(t, bindingEndpoint("svc://env"), cfg.Endpoint)

	path = writeTempConfig(t, "endpoint: http://invalid\n")
	_, err = manager.Load(t.Context(), File(path))
	require.ErrorContains(t, err, "scheme must be svc")

	path = writeTempConfig(t, "endpoint: 42\n")
	_, err = manager.Load(t.Context(), File(path))
	require.ErrorContains(t, err, "must be a string")
	require.ErrorContains(t, err, "bindingEndpoint")
}

func TestManagerRejectsInvalidOptionsAndSchema(t *testing.T) {
	type Config struct {
		Name string `json:"name"`
	}
	_, err := New(Config{}, WithCodec(Codec[string]{}))
	require.EqualError(t, err, "cfgm: codec for string requires Parse")
	_, err = New(Config{}, WithCodec(Codec[string]{Parse: func(value string) (string, error) { return value, nil }}))
	require.EqualError(t, err, "cfgm: codec for string requires Format")
	_, err = New(Config{}, Logger(nil))
	require.EqualError(t, err, "cfgm: logger must not be nil")
	_, err = New("")
	require.ErrorContains(t, err, "config root must be a struct")
	_, err = New((*Config)(nil))
	require.ErrorContains(t, err, "config root must be a struct")
	type UnsupportedOption struct {
		Name string `json:"name,omitempty"`
	}
	_, err = New(UnsupportedOption{})
	require.EqualError(t, err, `cfgm: field Name uses unsupported json option "omitempty"`)
	type QuotedName struct {
		Name string `json:"'name'"`
	}
	_, err = New(QuotedName{})
	require.EqualError(t, err, "cfgm: field Name uses an unsupported quoted json name")
	type IgnoredField struct {
		Visible string `json:"visible"`
		Hidden  string `json:"-"`
	}
	manager, err := New(IgnoredField{})
	require.NoError(t, err)
	assert.True(t, manager.schema.isFieldPath("visible"))
	assert.False(t, manager.schema.hasPath("Hidden"))
}

func TestManagerTreatsCodecStructAsLeaf(t *testing.T) {
	type Endpoint struct {
		Scheme string
		Name   string
	}
	type Config struct {
		Endpoint Endpoint `json:"endpoint"`
	}
	manager := MustNew(Config{}, WithCodec(Codec[Endpoint]{
		Parse: func(value string) (Endpoint, error) {
			parts := strings.SplitN(value, "://", 2)
			if len(parts) != 2 {
				return Endpoint{}, errors.New("invalid endpoint")
			}
			return Endpoint{Scheme: parts[0], Name: parts[1]}, nil
		},
		Format: func(value Endpoint) string { return value.Scheme + "://" + value.Name },
	}))
	root := configuredRoot(t, manager, func(context.Context, *cli.Command, *Config) error { return nil })
	requireFlagType[*cli.StringFlag](t, root.Flags, "endpoint")
}

func TestManagerPreservesDefaultCodecValues(t *testing.T) {
	type Endpoint struct {
		Scheme string
		Name   string
	}
	type Config struct {
		Endpoint Endpoint `json:"endpoint"`
	}
	manager := MustNew(Config{Endpoint: Endpoint{Scheme: "svc", Name: "default"}},
		WithoutDefaultPaths(),
		WithCodec(Codec[Endpoint]{
			Parse: func(value string) (Endpoint, error) {
				parts := strings.SplitN(value, "://", 2)
				if len(parts) != 2 {
					return Endpoint{}, errors.New("invalid endpoint")
				}
				return Endpoint{Scheme: parts[0], Name: parts[1]}, nil
			},
			Format: func(value Endpoint) string { return value.Scheme + "://" + value.Name },
		}),
	)
	cfg, err := manager.Load(t.Context())
	require.NoError(t, err)
	assert.Equal(t, Endpoint{Scheme: "svc", Name: "default"}, cfg.Endpoint)
}

func TestManagerPreservesDefaultCodecsInsideComposites(t *testing.T) {
	type Endpoint struct {
		Name string
	}
	type Service struct {
		Endpoint Endpoint `json:"endpoint"`
	}
	type Config struct {
		Services []Service `json:"services"`
	}
	manager := MustNew(Config{Services: []Service{{Endpoint: Endpoint{Name: "default"}}}},
		WithoutDefaultPaths(),
		WithCodec(Codec[Endpoint]{
			Parse:  func(value string) (Endpoint, error) { return Endpoint{Name: value}, nil },
			Format: func(value Endpoint) string { return value.Name },
		}),
	)
	cfg, err := manager.Load(t.Context())
	require.NoError(t, err)
	require.Len(t, cfg.Services, 1)
	assert.Equal(t, "default", cfg.Services[0].Endpoint.Name)
}

func TestManagerUsesCodecInsideStructSlice(t *testing.T) {
	type Endpoint struct {
		Name string
	}
	type Service struct {
		Endpoint Endpoint `json:"endpoint"`
	}
	type Config struct {
		Services []Service `json:"services"`
	}
	manager := MustNew(Config{}, WithCodec(Codec[Endpoint]{
		Parse:  func(value string) (Endpoint, error) { return Endpoint{Name: value}, nil },
		Format: func(value Endpoint) string { return value.Name },
	}))
	var loaded *Config
	cmd := configuredRoot(t, manager, func(_ context.Context, _ *cli.Command, cfg *Config) error {
		loaded = cfg
		return nil
	})
	require.NoError(t, cmd.Run(t.Context(), []string{"app", `--services={"endpoint":"main"}`}))
	require.Len(t, loaded.Services, 1)
	assert.Equal(t, "main", loaded.Services[0].Endpoint.Name)
}

func TestManagerSupportsNamedScalarTypes(t *testing.T) {
	type Name string
	type Count int
	type Config struct {
		Name  Name  `json:"name"`
		Count Count `json:"count"`
	}
	manager := MustNew(Config{Name: "default", Count: 2})
	root := configuredRoot(t, manager, func(context.Context, *cli.Command, *Config) error { return nil })
	requireFlagType[*cli.StringFlag](t, root.Flags, "name")
	requireFlagType[*cli.IntFlag](t, root.Flags, "count")
}

func TestManagerSupportsNamedScalarSliceTypes(t *testing.T) {
	type Name string
	type Names []Name
	type Config struct {
		Names Names `json:"names"`
	}
	manager := MustNew(Config{Names: Names{"default"}})
	root := configuredRoot(t, manager, func(context.Context, *cli.Command, *Config) error { return nil })
	requireFlagType[*cli.StringSliceFlag](t, root.Flags, "names")
}

func TestManagerSupportsNamedStringMapTypes(t *testing.T) {
	type Labels map[string]string
	type Config struct {
		Labels Labels `json:"labels"`
	}
	manager := MustNew(Config{Labels: Labels{"default": "yes"}})
	root := configuredRoot(t, manager, func(context.Context, *cli.Command, *Config) error { return nil })
	requireFlagType[*cli.StringMapFlag](t, root.Flags, "labels")
}

func TestManagerRejectsAmbiguousSchemaKeys(t *testing.T) {
	assert.PanicsWithError(t, `cfgm: duplicate config path "name"`, func() {
		typ := reflect.StructOf([]reflect.StructField{
			{Name: "First", Type: reflect.TypeFor[string](), Tag: `json:"name"`},
			{Name: "Second", Type: reflect.TypeFor[string](), Tag: `json:"name"`},
		})
		buildSchemaModel(typ, nil)
	})
	_, err := New(struct {
		Addr string `json:"server.addr"`
	}{})
	require.EqualError(t, err, `cfgm: config key "server.addr" must not contain dots`)
	type Node struct {
		Children []Node `json:"children"`
	}
	_, err = New(Node{})
	require.ErrorContains(t, err, "recursive config type")
}

func TestManagerRejectsInvalidCLIOptions(t *testing.T) {
	tests := []struct {
		name  string
		build func() error
		want  string
	}{
		{name: "hidden path", build: func() error { _, err := New(bindingDefaults(), HideCLI("missing")); return err }, want: "hidden CLI path"},
		{name: "alias path", build: func() error { _, err := New(bindingDefaults(), CLIAlias("missing", "m")); return err }, want: "alias path"},
		{name: "hidden alias", build: func() error {
			_, err := New(bindingDefaults(), HideCLI("server.redis"), CLIAlias("server.redis.url", "r"))
			return err
		}, want: "hidden"},
		{name: "reserved alias", build: func() error { _, err := New(bindingDefaults(), CLIAlias("server.addr", "c")); return err }, want: "reserved"},
		{name: "help alias", build: func() error { _, err := New(bindingDefaults(), CLIAlias("server.addr", "h")); return err }, want: "reserved"},
		{name: "duplicate alias", build: func() error { _, err := New(bindingDefaults(), CLIAlias("server.addr", "x", "x")); return err }, want: "duplicate"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorContains(t, test.build(), test.want)
		})
	}
}

func TestManagerConfigureRejectsInvalidCommandTrees(t *testing.T) {
	manager := MustNew(bindingDefaults(), CLIAlias("server.addr", "x"), CLIAlias("server.debug", "x"))
	server := &cli.Command{Name: "server", Action: manager.Action(func(context.Context, *cli.Command, *bindingTestConfig) error { return nil })}
	require.ErrorContains(t, manager.Configure(&cli.Command{Name: "app", Commands: []*cli.Command{server}}), "ambiguous")

	require.ErrorContains(t, MustNew(bindingDefaults()).Configure(nil), "nil")
	invalid := MustNew(bindingDefaults())
	require.ErrorContains(t, invalid.Configure(&cli.Command{Name: "app", Commands: []*cli.Command{{Name: "bad.name"}}}), "invalid command")

	collision := MustNew(bindingDefaults())
	server = &cli.Command{
		Name:   "server",
		Flags:  []cli.Flag{&cli.StringFlag{Name: "addr"}},
		Action: collision.Action(func(context.Context, *cli.Command, *bindingTestConfig) error { return nil }),
	}
	require.ErrorContains(t, collision.Configure(&cli.Command{Name: "app", Commands: []*cli.Command{server}}), "ambiguous")

	missing := MustNew(bindingDefaults())
	other := &cli.Command{Name: "other", Action: missing.Action(func(context.Context, *cli.Command, *bindingTestConfig) error { return nil })}
	root := &cli.Command{Name: "app", Commands: []*cli.Command{other}}
	missing.MustConfigure(root)
	require.ErrorContains(t, root.Run(t.Context(), []string{"app", "other"}), "was not configured")
}

func TestManagerActionRejectsAnUnconfiguredCommandTree(t *testing.T) {
	manager := MustNew(bindingDefaults())
	configured := &cli.Command{
		Name: "app",
		Commands: []*cli.Command{{
			Name:   "server",
			Action: manager.Action(func(context.Context, *cli.Command, *bindingTestConfig) error { return nil }),
		}},
	}
	manager.MustConfigure(configured)

	unconfigured := &cli.Command{
		Name: "app",
		Commands: []*cli.Command{{
			Name:   "server",
			Action: manager.Action(func(context.Context, *cli.Command, *bindingTestConfig) error { return nil }),
		}},
	}
	require.ErrorContains(
		t,
		unconfigured.Run(t.Context(), []string{"app", "server"}),
		"was not configured by this manager",
	)
}

func TestManagerConfigureIsTransactional(t *testing.T) {
	manager := MustNew(bindingDefaults())
	root := &cli.Command{
		Name: "app",
		Commands: []*cli.Command{
			{
				Name:   "server",
				Flags:  []cli.Flag{&cli.StringFlag{Name: "addr"}},
				Action: manager.Action(func(context.Context, *cli.Command, *bindingTestConfig) error { return nil }),
			},
		},
	}
	require.ErrorContains(t, manager.Configure(root), "ambiguous")
	assert.Empty(t, root.Flags)
	assert.Len(t, root.Commands[0].Flags, 1)
}

func TestManagerRejectsGeneratedReservedNames(t *testing.T) {
	type Config struct {
		Config string `json:"config"`
	}
	manager := MustNew(Config{})
	require.ErrorContains(t, manager.Configure(&cli.Command{Name: "app"}), "reserved")
}

func TestManagerAllowsNullCollections(t *testing.T) {
	type Config struct {
		Names  []string          `json:"names"`
		Labels map[string]string `json:"labels"`
	}
	path := writeTempConfig(t, "names: null\nlabels: null\n")
	cfg, err := MustNew(Config{}, WithoutDefaultPaths()).Load(t.Context(), File(path))
	require.NoError(t, err)
	assert.Nil(t, cfg.Names)
	assert.Nil(t, cfg.Labels)
}

func TestManagerSupportsNilCompositeDefaultsAndTimestamps(t *testing.T) {
	type Config struct {
		Tags   []string          `json:"tags"`
		Labels map[string]string `json:"labels"`
		At     time.Time         `json:"at"`
	}
	manager := MustNew(Config{})
	var loaded *Config
	cmd := configuredRoot(t, manager, func(_ context.Context, _ *cli.Command, cfg *Config) error {
		loaded = cfg
		return nil
	})
	requireFlagType[*cli.StringSliceFlag](t, cmd.Flags, "tags")
	requireFlagType[*cli.StringMapFlag](t, cmd.Flags, "labels")
	requireFlagType[*cli.TimestampFlag](t, cmd.Flags, "at")
	require.NoError(t, cmd.Run(t.Context(), []string{"app", "--at=2026-07-15T10:20:30Z"}))
	assert.Equal(t, time.Date(2026, 7, 15, 10, 20, 30, 0, time.UTC), loaded.At)
}

func runManager(
	t *testing.T,
	manager *Manager[bindingTestConfig],
	args ...string,
) (*bindingTestConfig, error) {
	t.Helper()
	return runManagerWithRootArgs(t, manager, nil, args...)
}

func runManagerWithRootArgs(
	t *testing.T,
	manager *Manager[bindingTestConfig],
	rootArgs []string,
	args ...string,
) (*bindingTestConfig, error) {
	t.Helper()
	var loaded *bindingTestConfig
	root := &cli.Command{
		Name: "app",
		Commands: []*cli.Command{{
			Name: "server",
			Action: manager.Action(func(_ context.Context, _ *cli.Command, cfg *bindingTestConfig) error {
				loaded = cfg
				return nil
			}),
		}},
	}
	manager.MustConfigure(root)
	commandArgs := append([]string{"app"}, rootArgs...)
	commandArgs = append(commandArgs, "server")
	commandArgs = append(commandArgs, args...)
	err := root.Run(t.Context(), commandArgs)
	return loaded, err
}

func requireFlagType[T cli.Flag](t *testing.T, flags []cli.Flag, name string) T {
	t.Helper()
	flag := findFlag(flags, name)
	require.NotNil(t, flag, "flag %q not found", name)
	typed, ok := flag.(T)
	require.True(t, ok, "flag %q has type %T, want %s", name, flag, reflect.TypeFor[T]())
	return typed
}

func findFlag(flags []cli.Flag, name string) cli.Flag {
	for _, flag := range flags {
		if flag.Names()[0] == name {
			return flag
		}
	}
	return nil
}

func configuredRoot[T any](t *testing.T, manager *Manager[T], run ActionFunc[T]) *cli.Command {
	t.Helper()
	root := &cli.Command{Name: "app", Action: manager.Action(run)}
	manager.MustConfigure(root)
	return root
}
