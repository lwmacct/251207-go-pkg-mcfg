// Package cfgm provides Schema-driven configuration for Go applications.
//
// A Manager owns defaults, validation, codecs, file and environment loading,
// generated urfave/cli flags, typed actions, and config examples:
//
//	manager := cfgm.MustNew(DefaultConfig(), cfgm.AppName("app"))
//	config, err := manager.Load(ctx, cfgm.Env("APP_"))
//
// Later sources replace earlier values. Manager.Load searches optional
// DefaultPaths before caller-provided sources unless WithoutDefaultPaths is
// set. Unknown keys are rejected by default.
//
// # CLI Integration
//
// Manager.Configure walks a completed urfave command tree. It adds root
// --config/-c and --env-prefix/-e flags and projects each actionable command's
// matching config subtree into typed local flags:
//
//	manager := cfgm.MustNew(DefaultConfig(),
//	    cfgm.CLIAlias("server.addr", "a"),
//	    cfgm.HideCLI("server.redis.password"),
//	)
//	command := &cli.Command{
//	    Name: "server",
//	    Action: manager.Action(func(ctx context.Context, cmd *cli.Command, config *Config) error {
//	        return run(ctx, config)
//	    }),
//	}
//	root := &cli.Command{Name: "app", Commands: []*cli.Command{command}}
//	manager.MustConfigure(root)
//
// Command paths map directly to json-tagged config structs. The example maps
// Config.Server to the server command, so server.addr becomes --addr.
// Manager.Action applies defaults, default paths, an explicit config file, the
// selected environment prefix, and explicitly set CLI flags in that order.
// JSON v2 embedded structs (using json:",embed" or an anonymous struct with
// no explicit JSON name) contribute their fields at the containing config path
// across every source and generated output. Embedded types cannot use codecs,
// JSON/text methods, or map fallbacks; duplicate paths are rejected.
// To keep every source equivalent, the supported JSON tag subset is unquoted
// member names, "-", and "embed"; representation-specific options are rejected.
//
// # Composite Values
//
// Environment slices and maps use complete JSON values. Scalar slices use
// urfave's repeatable typed flags. Struct slices use repeatable strict JSON
// objects; [] explicitly clears the collection and cannot be mixed with object
// occurrences. A CLI collection replaces lower-priority sources as a whole.
//
// WithCodec registers parsing for custom leaf types across files, environment
// variables, and CLI flags. ConfigFiles validates runtime files with the same
// Manager Schema used by loading.
package cfgm
