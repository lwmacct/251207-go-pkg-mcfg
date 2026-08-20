package cfgm

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
)

// Option configures a Manager.
type Option interface {
	applyManager(options *managerOptions)
}

type managerOptionFunc func(*managerOptions)

func (f managerOptionFunc) applyManager(options *managerOptions) {
	f(options)
}

type managerOptions struct {
	appName          string
	codecs           map[reflect.Type]valueCodec
	defaultPaths     bool
	expandTemplates  bool
	allowUnknownKeys bool
	logger           *slog.Logger
	aliases          map[string][]string
	noCLI            map[string]bool
}

func AppName(name string) Option {
	return managerOptionFunc(func(options *managerOptions) {
		options.appName = strings.TrimSpace(name)
	})
}

func WithoutDefaultPaths() Option {
	return managerOptionFunc(func(options *managerOptions) {
		options.defaultPaths = false
	})
}

func WithoutTemplateExpansion() Option {
	return managerOptionFunc(func(options *managerOptions) {
		options.expandTemplates = false
	})
}

func AllowUnknownKeys() Option {
	return managerOptionFunc(func(options *managerOptions) {
		options.allowUnknownKeys = true
	})
}

func Logger(logger *slog.Logger) Option {
	if logger == nil {
		panic("cfgm: logger must not be nil")
	}
	return managerOptionFunc(func(options *managerOptions) {
		options.logger = logger
	})
}

type Codec[T any] struct {
	Parse  func(string) (T, error)
	Format func(T) string
}

func WithCodec[T any](codec Codec[T]) Option {
	if codec.Parse == nil {
		panic(fmt.Sprintf("cfgm: codec for %s requires Parse", reflect.TypeFor[T]()))
	}
	if codec.Format == nil {
		panic(fmt.Sprintf("cfgm: codec for %s requires Format", reflect.TypeFor[T]()))
	}
	return managerOptionFunc(func(options *managerOptions) {
		if options.codecs == nil {
			options.codecs = make(map[reflect.Type]valueCodec)
		}
		typ := reflect.TypeFor[T]()
		options.codecs[typ] = valueCodec{
			parse: func(value string) (any, error) { return codec.Parse(value) },
			format: func(value any) string {
				typed, ok := value.(T)
				if !ok {
					panic(fmt.Errorf("cfgm: codec value has type %T, want %s", value, typ))
				}
				return codec.Format(typed)
			},
		}
	})
}

// HideCLI excludes canonical config field or struct paths from generated CLI
// flags. Files and environment variables can still configure these paths.
func HideCLI(paths ...string) Option {
	return managerOptionFunc(func(options *managerOptions) {
		if options.noCLI == nil {
			options.noCLI = make(map[string]bool)
		}
		for _, path := range paths {
			if path = cleanConfigPath(path); path != "" {
				options.noCLI[path] = true
			}
		}
	})
}

// CLIAlias adds aliases to one canonical config field path.
func CLIAlias(path string, aliases ...string) Option {
	return managerOptionFunc(func(options *managerOptions) {
		if options.aliases == nil {
			options.aliases = make(map[string][]string)
		}
		path = cleanConfigPath(path)
		for _, alias := range aliases {
			if alias = strings.TrimSpace(alias); alias != "" {
				options.aliases[path] = append(options.aliases[path], alias)
			}
		}
	})
}

type valueCodec struct {
	parse  func(string) (any, error)
	format func(any) string
}

// Manager compiles one config schema and owns all file, environment, CLI, and
// example behavior for that config type.
type Manager[T any] struct {
	defaults          T
	appName           string
	schema            *schemaModel
	codecs            map[reflect.Type]valueCodec
	defaultPaths      bool
	expandTemplates   bool
	strictUnknownKeys bool
	logger            *slog.Logger
	aliases           map[string][]string
	noCLI             map[string]bool
	bindings          map[string]*commandBinding[T]
	commands          map[*cli.Command]*commandBinding[T]
	configured        bool
}

// New creates a Manager from non-pointer struct defaults.
func New[T any](defaults T, opts ...Option) *Manager[T] {
	options := managerOptions{defaultPaths: true, expandTemplates: true, logger: slog.Default()}
	for _, opt := range opts {
		if opt != nil {
			opt.applyManager(&options)
		}
	}
	manager := &Manager[T]{
		defaults:          defaults,
		appName:           options.appName,
		schema:            buildSchemaModel(reflect.TypeFor[T](), options.codecs),
		codecs:            maps.Clone(options.codecs),
		defaultPaths:      options.defaultPaths,
		expandTemplates:   options.expandTemplates,
		strictUnknownKeys: !options.allowUnknownKeys,
		logger:            options.logger,
		aliases:           mapsCloneSlices(options.aliases),
		noCLI:             maps.Clone(options.noCLI),
		bindings:          make(map[string]*commandBinding[T]),
		commands:          make(map[*cli.Command]*commandBinding[T]),
	}
	manager.validateCLIOptions()
	return manager
}

// Load applies non-CLI sources after defaults and optional default paths.
func (m *Manager[T]) Load(ctx context.Context, sources ...Source) (*T, error) {
	config, _, err := m.LoadReport(ctx, sources...)
	return config, err
}

// MustLoad is Load with panic-on-error startup semantics.
func (m *Manager[T]) MustLoad(ctx context.Context, sources ...Source) *T {
	config, err := m.Load(ctx, sources...)
	if err != nil {
		panic(fmt.Sprintf("cfgm: failed to load config: %v", err))
	}
	return config
}

// LoadReport loads config and reports the keys contributed by each source.
func (m *Manager[T]) LoadReport(ctx context.Context, sources ...Source) (*T, *Report, error) {
	loader := m.loader()
	if m.defaultPaths {
		loader.sources = append(loader.sources, Files(DefaultPaths(m.appName), Optional()))
	}
	loader.sources = append(loader.sources, sources...)
	return loader.load(ctx)
}

func (m *Manager[T]) loader() *configLoader[T] {
	return &configLoader[T]{
		defaults:          m.defaults,
		schema:            m.schema,
		logger:            m.logger,
		expandTemplates:   m.expandTemplates,
		strictUnknownKeys: m.strictUnknownKeys,
		codecs:            m.codecs,
	}
}

// ActionFunc receives config loaded for the current CLI command lineage.
type ActionFunc[T any] func(context.Context, *cli.Command, *T) error

// ReportActionFunc also receives the CLI load report.
type ReportActionFunc[T any] func(context.Context, *cli.Command, *T, *Report) error

type commandBinding[T any] struct {
	manager     *Manager[T]
	commandPath string
	fields      []boundField
}

type commandConfiguration[T any] struct {
	command *cli.Command
	binding *commandBinding[T]
	flags   []cli.Flag
}

type boundField struct {
	field schemaField
	name  string
}

func (m *Manager[T]) validateCLIOptions() {
	for path := range m.noCLI {
		if !m.schema.isFieldPath(path) && !m.schema.isStructPath(path) {
			panic(fmt.Errorf("cfgm: hidden CLI path %q does not select config fields", path))
		}
	}
	for path, aliases := range m.aliases {
		if !m.schema.isFieldPath(path) {
			panic(fmt.Errorf("cfgm: CLI alias path %q is not a config field", path))
		}
		if bindingExcluded(path, m.noCLI) {
			panic(fmt.Errorf("cfgm: CLI alias path %q is hidden", path))
		}
		seen := make(map[string]bool, len(aliases))
		for _, alias := range aliases {
			if isReservedFlagName(alias) {
				panic(fmt.Errorf("cfgm: alias --%s is reserved", alias))
			}
			if seen[alias] {
				panic(fmt.Errorf("cfgm: duplicate alias --%s for %s", alias, path))
			}
			seen[alias] = true
		}
	}
}

func (m *Manager[T]) newCommandBinding(commandPath string, rootOnly bool) (*commandBinding[T], error) {
	if commandPath != "" && !m.schema.isStructPath(commandPath) {
		return nil, fmt.Errorf("cfgm: command path %q is not a config struct path", commandPath)
	}
	fields := make([]boundField, 0, len(m.schema.fields))
	seenNames := make(map[string]string)
	for _, field := range m.schema.fields {
		if rootOnly && strings.Contains(field.path, ".") {
			continue
		}
		if commandPath != "" && !pathWithin(field.path, commandPath) {
			continue
		}
		if bindingExcluded(field.path, m.noCLI) {
			continue
		}
		name := bindingFlagName(field.path, commandPath)
		if isReservedFlagName(name) {
			return nil, fmt.Errorf("cfgm: generated CLI flag --%s is reserved", name)
		}
		field.aliases = append([]string(nil), m.aliases[field.path]...)
		for _, flagName := range append([]string{name}, field.aliases...) {
			if previous, exists := seenNames[flagName]; exists {
				return nil, fmt.Errorf("cfgm: CLI flag --%s is ambiguous: matches %s and %s", flagName, previous, field.path)
			}
			seenNames[flagName] = field.path
		}
		fields = append(fields, boundField{field: field, name: name})
	}
	slices.SortFunc(fields, func(a, b boundField) int { return strings.Compare(a.name, b.name) })
	return &commandBinding[T]{manager: m, commandPath: commandPath, fields: fields}, nil
}

func isReservedFlagName(name string) bool {
	return name == configFlagName || name == envPrefixFlagName ||
		name == "c" || name == "e" || name == "help" || name == "h"
}

func (b *commandBinding[T]) flags() ([]cli.Flag, error) {
	flags := make([]cli.Flag, 0, len(b.fields))
	for _, field := range b.fields {
		flag, err := b.newFlag(field)
		if err != nil {
			return nil, err
		}
		flags = append(flags, flag)
	}
	return flags, nil
}

// Configure adds root control flags and command-local config flags to a
// completed urfave command tree. Call it before Command.Run.
func (m *Manager[T]) Configure(root *cli.Command) error {
	if root == nil {
		return errors.New("cfgm: root command is nil")
	}
	if _, exists := m.commands[root]; exists {
		return errors.New("cfgm: command tree is already configured")
	}

	newBindings := make(map[string]*commandBinding[T])
	rootBinding, exists := m.bindings[""]
	if !exists {
		var err error
		rootBinding, err = m.newCommandBinding("", true)
		if err != nil {
			return err
		}
		newBindings[""] = rootBinding
	}
	rootConfigFlags, err := rootBinding.flags()
	if err != nil {
		return err
	}
	mergedRootFlags, err := mergeCLIFlags(root.Flags, append(rootFlags(), rootConfigFlags...))
	if err != nil {
		return fmt.Errorf("cfgm: configure root command: %w", err)
	}
	configurations := []commandConfiguration[T]{{command: root, binding: rootBinding, flags: mergedRootFlags}}
	seenPaths := make(map[string]bool)
	for _, command := range root.Commands {
		if err := m.compileCommand(command, "", seenPaths, newBindings, &configurations); err != nil {
			return err
		}
	}
	maps.Copy(m.bindings, newBindings)
	for _, configuration := range configurations {
		configuration.command.Flags = configuration.flags
		m.commands[configuration.command] = configuration.binding
	}
	m.configured = true
	return nil
}

func (m *Manager[T]) compileCommand(
	command *cli.Command,
	parentPath string,
	seenPaths map[string]bool,
	newBindings map[string]*commandBinding[T],
	configurations *[]commandConfiguration[T],
) error {
	if command == nil {
		return errors.New("cfgm: command tree contains nil command")
	}
	name := strings.TrimSpace(command.Name)
	if name == "" || strings.Contains(name, ".") {
		return fmt.Errorf("cfgm: invalid command name %q", name)
	}
	commandPath := joinSchemaPath(parentPath, name)
	if seenPaths[commandPath] {
		return fmt.Errorf("cfgm: duplicate command path %q", commandPath)
	}
	seenPaths[commandPath] = true
	if command.Action != nil && m.schema.isStructPath(commandPath) {
		configuration, err := m.compileActionCommand(command, commandPath, newBindings)
		if err != nil {
			return err
		}
		*configurations = append(*configurations, configuration)
	}
	for _, child := range command.Commands {
		if err := m.compileCommand(child, commandPath, seenPaths, newBindings, configurations); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager[T]) compileActionCommand(
	command *cli.Command,
	commandPath string,
	newBindings map[string]*commandBinding[T],
) (commandConfiguration[T], error) {
	binding, exists := m.bindings[commandPath]
	if !exists {
		binding, exists = newBindings[commandPath]
	}
	if !exists {
		var err error
		binding, err = m.newCommandBinding(commandPath, false)
		if err != nil {
			return commandConfiguration[T]{}, err
		}
		newBindings[commandPath] = binding
	}
	flags, err := binding.flags()
	if err != nil {
		return commandConfiguration[T]{}, err
	}
	mergedFlags, err := mergeCLIFlags(command.Flags, flags)
	if err != nil {
		return commandConfiguration[T]{}, fmt.Errorf("cfgm: configure command %q: %w", commandPath, err)
	}
	return commandConfiguration[T]{command: command, binding: binding, flags: mergedFlags}, nil
}

// MustConfigure is Configure with panic-on-error startup semantics.
func (m *Manager[T]) MustConfigure(root *cli.Command) {
	if err := m.Configure(root); err != nil {
		panic(err)
	}
}

// Action wraps a typed config callback as an urfave action.
func (m *Manager[T]) Action(run ActionFunc[T]) cli.ActionFunc {
	if run == nil {
		panic("cfgm: action must not be nil")
	}
	return func(ctx context.Context, cmd *cli.Command) error {
		config, err := m.loadCommand(ctx, cmd)
		if err != nil {
			return err
		}
		return run(ctx, cmd, config)
	}
}

// ActionReport wraps a typed config and load-report callback as an urfave action.
func (m *Manager[T]) ActionReport(run ReportActionFunc[T]) cli.ActionFunc {
	if run == nil {
		panic("cfgm: report action must not be nil")
	}
	return func(ctx context.Context, cmd *cli.Command) error {
		config, report, err := m.loadCommandReport(ctx, cmd)
		if err != nil {
			return err
		}
		return run(ctx, cmd, config, report)
	}
}

func (m *Manager[T]) loadCommand(ctx context.Context, cmd *cli.Command) (*T, error) {
	config, _, err := m.loadCommandReport(ctx, cmd)
	return config, err
}

func (m *Manager[T]) loadCommandReport(ctx context.Context, cmd *cli.Command) (*T, *Report, error) {
	if ctx == nil {
		return nil, nil, errors.New("cfgm: nil context")
	}
	if !m.configured {
		return nil, nil, errors.New("cfgm: manager must be configured before running CLI actions")
	}
	commandPath := commandLineagePath(cmd)
	binding, exists := m.commands[cmd]
	if !exists {
		return nil, nil, fmt.Errorf("cfgm: command path %q was not configured by this manager", commandPath)
	}
	loader := m.loader()

	appName := m.appName
	if appName == "" {
		appName = commandRootName(cmd)
	}
	if m.defaultPaths {
		loader.sources = append(loader.sources, Files(DefaultPaths(appName), Optional()))
	}
	if configPath := commandConfigPath(cmd); configPath != "" {
		loader.sources = append(loader.sources, File(configPath))
	}
	if prefix, ok := commandEnvPrefix(cmd); ok {
		if prefix != "" {
			loader.sources = append(loader.sources, Env(prefix))
		}
	} else if appName != "" {
		loader.sources = append(loader.sources, Env(strings.ToUpper(strings.ReplaceAll(appName, "-", "_"))+"_"))
	}
	loader.sources = append(loader.sources, &bindingCLISource[T]{binding: binding, cmd: cmd})
	return loader.load(ctx)
}

func (b *commandBinding[T]) newFlag(bound boundField) (cli.Flag, error) {
	field := bound.field
	aliases := append([]string(nil), field.aliases...)
	usage := field.desc
	defaultValue, ok := valueAtPath(reflect.ValueOf(b.manager.defaults), field.index)
	if !ok {
		defaultValue = reflect.Zero(field.typ)
	}

	if codec, ok := b.manager.codecs[field.typ]; ok {
		value := ""
		if defaultValue.IsValid() {
			value = codec.format(defaultValue.Interface())
		}
		return &cli.StringFlag{Name: bound.name, Aliases: aliases, Usage: usage, Value: value}, nil
	}
	if field.kind == schemaFieldStructSlice {
		return b.newStructSliceFlag(bound, defaultValue), nil
	}

	value := defaultValue.Interface()
	switch field.typ {
	case durationType:
		return &cli.DurationFlag{
			Name: bound.name, Aliases: aliases, Usage: usage, Value: reflectAs[time.Duration](defaultValue),
		}, nil
	case timeType:
		return &cli.TimestampFlag{
			Name: bound.name, Aliases: aliases, Usage: usage, Value: reflectAs[time.Time](defaultValue),
			Config: cli.TimestampConfig{Layouts: []string{time.RFC3339Nano, time.RFC3339}},
		}, nil
	}
	switch field.typ.Kind() { //nolint:exhaustive // unsupported config fields fail binding construction
	case reflect.String:
		return &cli.StringFlag{Name: bound.name, Aliases: aliases, Usage: usage, Value: defaultValue.String()}, nil
	case reflect.Bool:
		return &cli.BoolFlag{Name: bound.name, Aliases: aliases, Usage: usage, Value: defaultValue.Bool()}, nil
	case reflect.Int:
		return &cli.IntFlag{Name: bound.name, Aliases: aliases, Usage: usage, Value: int(defaultValue.Int())}, nil
	case reflect.Int8:
		return &cli.Int8Flag{Name: bound.name, Aliases: aliases, Usage: usage, Value: reflectAs[int8](defaultValue)}, nil
	case reflect.Int16:
		return &cli.Int16Flag{Name: bound.name, Aliases: aliases, Usage: usage, Value: reflectAs[int16](defaultValue)}, nil
	case reflect.Int32:
		return &cli.Int32Flag{Name: bound.name, Aliases: aliases, Usage: usage, Value: reflectAs[int32](defaultValue)}, nil
	case reflect.Int64:
		return &cli.Int64Flag{Name: bound.name, Aliases: aliases, Usage: usage, Value: defaultValue.Int()}, nil
	case reflect.Uint:
		return &cli.UintFlag{Name: bound.name, Aliases: aliases, Usage: usage, Value: uint(defaultValue.Uint())}, nil
	case reflect.Uint8:
		return &cli.Uint8Flag{Name: bound.name, Aliases: aliases, Usage: usage, Value: reflectAs[uint8](defaultValue)}, nil
	case reflect.Uint16:
		return &cli.Uint16Flag{Name: bound.name, Aliases: aliases, Usage: usage, Value: reflectAs[uint16](defaultValue)}, nil
	case reflect.Uint32:
		return &cli.Uint32Flag{Name: bound.name, Aliases: aliases, Usage: usage, Value: reflectAs[uint32](defaultValue)}, nil
	case reflect.Uint64:
		return &cli.Uint64Flag{Name: bound.name, Aliases: aliases, Usage: usage, Value: defaultValue.Uint()}, nil
	case reflect.Float32:
		return &cli.Float32Flag{Name: bound.name, Aliases: aliases, Usage: usage, Value: float32(defaultValue.Float())}, nil
	case reflect.Float64:
		return &cli.Float64Flag{Name: bound.name, Aliases: aliases, Usage: usage, Value: defaultValue.Float()}, nil
	case reflect.Slice:
		return newScalarSliceFlag(bound.name, aliases, usage, field.typ, value)
	case reflect.Map:
		if field.typ.Key().Kind() == reflect.String && field.typ.Elem().Kind() == reflect.String {
			return &cli.StringMapFlag{
				Name: bound.name, Aliases: aliases, Usage: usage, Value: stringMapAs(value),
			}, nil
		}
	}
	return nil, fmt.Errorf("cfgm: config field %s has unsupported CLI type %s", field.path, field.typ)
}

func (b *commandBinding[T]) newStructSliceFlag(bound boundField, defaultValue reflect.Value) cli.Flag {
	defaultText := "[]"
	if defaultValue.IsValid() {
		encoded, err := marshalConfigJSON(defaultValue.Interface())
		if err == nil {
			defaultText = string(encoded)
		}
	}
	return &cli.GenericFlag{
		Name:        bound.name,
		Aliases:     append([]string(nil), bound.field.aliases...),
		Usage:       bound.field.desc,
		DefaultText: defaultText,
		Value:       newStructSliceValue(bound.field.typ, b.manager.codecs),
	}
}

func reflectAs[T any](value reflect.Value) T {
	converted := value.Convert(reflect.TypeFor[T]()).Interface()
	typed, ok := converted.(T)
	if !ok {
		panic(fmt.Errorf("cfgm: cannot convert %s to %s", value.Type(), reflect.TypeFor[T]()))
	}
	return typed
}

func newScalarSliceFlag(name string, aliases []string, usage string, typ reflect.Type, value any) (cli.Flag, error) {
	switch typ.Elem().Kind() { //nolint:exhaustive // unsupported slice elements return an error
	case reflect.String:
		return &cli.StringSliceFlag{Name: name, Aliases: aliases, Usage: usage, Value: sliceAs[string](value)}, nil
	case reflect.Int:
		return &cli.IntSliceFlag{Name: name, Aliases: aliases, Usage: usage, Value: sliceAs[int](value)}, nil
	case reflect.Int8:
		return &cli.Int8SliceFlag{Name: name, Aliases: aliases, Usage: usage, Value: sliceAs[int8](value)}, nil
	case reflect.Int16:
		return &cli.Int16SliceFlag{Name: name, Aliases: aliases, Usage: usage, Value: sliceAs[int16](value)}, nil
	case reflect.Int32:
		return &cli.Int32SliceFlag{Name: name, Aliases: aliases, Usage: usage, Value: sliceAs[int32](value)}, nil
	case reflect.Int64:
		return &cli.Int64SliceFlag{Name: name, Aliases: aliases, Usage: usage, Value: sliceAs[int64](value)}, nil
	case reflect.Uint16:
		return &cli.Uint16SliceFlag{Name: name, Aliases: aliases, Usage: usage, Value: sliceAs[uint16](value)}, nil
	case reflect.Uint32:
		return &cli.Uint32SliceFlag{Name: name, Aliases: aliases, Usage: usage, Value: sliceAs[uint32](value)}, nil
	case reflect.Uint:
		return &cli.UintSliceFlag{Name: name, Aliases: aliases, Usage: usage, Value: sliceAs[uint](value)}, nil
	case reflect.Uint8:
		return &cli.Uint8SliceFlag{Name: name, Aliases: aliases, Usage: usage, Value: sliceAs[uint8](value)}, nil
	case reflect.Uint64:
		return &cli.Uint64SliceFlag{Name: name, Aliases: aliases, Usage: usage, Value: sliceAs[uint64](value)}, nil
	case reflect.Float32:
		return &cli.Float32SliceFlag{Name: name, Aliases: aliases, Usage: usage, Value: sliceAs[float32](value)}, nil
	case reflect.Float64:
		return &cli.Float64SliceFlag{Name: name, Aliases: aliases, Usage: usage, Value: sliceAs[float64](value)}, nil
	}
	return nil, fmt.Errorf("cfgm: config field --%s has unsupported slice type %s", name, typ)
}

func sliceAs[T any](value any) []T {
	input := reflect.ValueOf(value)
	out := make([]T, input.Len())
	for index := range input.Len() {
		out[index] = reflectAs[T](input.Index(index))
	}
	return out
}

func stringMapAs(value any) map[string]string {
	input := reflect.ValueOf(value)
	out := make(map[string]string, input.Len())
	iterator := input.MapRange()
	for iterator.Next() {
		out[iterator.Key().Convert(reflect.TypeFor[string]()).String()] =
			iterator.Value().Convert(reflect.TypeFor[string]()).String()
	}
	return out
}

type bindingCLISource[T any] struct {
	binding *commandBinding[T]
	cmd     *cli.Command
}

func (s *bindingCLISource[T]) Name() string { return "cli" }

func (s *bindingCLISource[T]) Load(ctx context.Context, _ Schema) (map[string]any, error) {
	if s.cmd == nil {
		return map[string]any{}, nil
	}
	out := map[string]any{}
	for _, bound := range s.binding.fields {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !s.cmd.IsSet(bound.name) {
			continue
		}
		value, err := s.binding.flagValue(s.cmd, bound)
		if err != nil {
			return nil, fmt.Errorf("--%s: %w", bound.name, err)
		}
		setByPath(out, bound.field.path, value)
	}
	return out, nil
}

func (b *commandBinding[T]) flagValue(cmd *cli.Command, bound boundField) (any, error) {
	if bound.field.kind == schemaFieldStructSlice {
		value, ok := cmd.Value(bound.name).(*structSliceValue)
		if !ok || value == nil {
			return nil, errors.New("invalid structured flag value")
		}
		return value.configValue(), nil
	}
	if _, ok := b.manager.codecs[bound.field.typ]; ok {
		return cmd.String(bound.name), nil
	}
	if bound.field.typ.Kind() == reflect.Slice {
		return scalarSliceFlagValue(cmd, bound.name, bound.field.typ)
	}
	if bound.field.typ.Kind() == reflect.Map &&
		bound.field.typ.Key().Kind() == reflect.String && bound.field.typ.Elem().Kind() == reflect.String {
		return stringMapToAny(cmd.StringMap(bound.name)), nil
	}
	return cmd.Value(bound.name), nil
}

func scalarSliceFlagValue(cmd *cli.Command, name string, typ reflect.Type) (any, error) {
	switch typ.Elem().Kind() { //nolint:exhaustive // unsupported slices fail during flag construction
	case reflect.String:
		return sliceToAny(cmd.StringSlice(name)), nil
	case reflect.Int:
		return sliceToAny(cmd.IntSlice(name)), nil
	case reflect.Int8:
		return sliceToAny(cmd.Int8Slice(name)), nil
	case reflect.Int16:
		return sliceToAny(cmd.Int16Slice(name)), nil
	case reflect.Int32:
		return sliceToAny(cmd.Int32Slice(name)), nil
	case reflect.Int64:
		return sliceToAny(cmd.Int64Slice(name)), nil
	case reflect.Uint16:
		return sliceToAny(cmd.Uint16Slice(name)), nil
	case reflect.Uint32:
		return sliceToAny(cmd.Uint32Slice(name)), nil
	case reflect.Uint:
		return sliceToAny(cmd.UintSlice(name)), nil
	case reflect.Uint8:
		return sliceToAny(cmd.Uint8Slice(name)), nil
	case reflect.Uint64:
		return sliceToAny(cmd.Uint64Slice(name)), nil
	case reflect.Float32:
		return sliceToAny(cmd.Float32Slice(name)), nil
	case reflect.Float64:
		return sliceToAny(cmd.Float64Slice(name)), nil
	}
	return nil, fmt.Errorf("unsupported slice type %s", typ)
}

func sliceToAny[T any](values []T) []any {
	out := make([]any, len(values))
	for index, value := range values {
		out[index] = value
	}
	return out
}

func stringMapToAny(values map[string]string) map[string]any {
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

type schemaFieldKind uint8

const (
	schemaFieldScalar schemaFieldKind = iota
	schemaFieldStructSlice
)

type schemaField struct {
	path    string
	typ     reflect.Type
	desc    string
	index   []int
	kind    schemaFieldKind
	aliases []string
}

type schemaModel struct {
	rootType reflect.Type
	fields   []schemaField
	paths    map[string]reflect.Type
	structs  map[string]bool
	fieldSet map[string]bool
	codecs   map[reflect.Type]valueCodec
	active   map[reflect.Type]bool
}

func buildSchemaModel(typ reflect.Type, codecs map[reflect.Type]valueCodec) *schemaModel {
	if typ == nil {
		panic("cfgm: config root must be a struct, got nil")
	}
	if typ.Kind() != reflect.Struct {
		panic(fmt.Errorf("cfgm: config root must be a struct, got %s", typ))
	}
	model := &schemaModel{
		rootType: typ,
		paths:    make(map[string]reflect.Type),
		structs:  make(map[string]bool),
		fieldSet: make(map[string]bool),
		codecs:   codecs,
		active:   make(map[reflect.Type]bool),
	}
	model.collect(typ, "", nil)
	model.validateEnvironmentNames()
	return model
}

func (m *schemaModel) collect(typ reflect.Type, prefix string, parentIndex []int) {
	typ = normalizeStructType(typ)
	m.enterType(typ)
	defer m.leaveType(typ)
	for _, configured := range m.configFields(typ) {
		field := configured.field
		key := configTagName(field)
		m.validateNewPath(prefix, key)
		path := joinSchemaPath(prefix, key)
		index := append(append([]int(nil), parentIndex...), configured.index...)
		m.paths[path] = field.Type
		_, hasCodec := m.codecs[field.Type]
		if isStructType(field.Type) && !hasCodec {
			m.structs[path] = true
			m.collect(field.Type, path, index)
			continue
		}
		kind := schemaFieldScalar
		if isStructSlice(field.Type) {
			kind = schemaFieldStructSlice
			m.collectCompositePaths(field.Type, path)
		}
		m.fields = append(m.fields, schemaField{path: path, typ: field.Type, desc: field.Tag.Get("desc"), index: index, kind: kind})
		m.fieldSet[path] = true
	}
}

func (m *schemaModel) validateNewPath(prefix, key string) {
	if strings.Contains(key, ".") {
		panic(fmt.Errorf("cfgm: config key %q must not contain dots", key))
	}
	path := joinSchemaPath(prefix, key)
	if _, exists := m.paths[path]; exists {
		panic(fmt.Errorf("cfgm: duplicate config path %q", path))
	}
}

func (m *schemaModel) collectCompositePaths(typ reflect.Type, prefix string) {
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
		typ = typ.Elem()
	}
	if typ.Kind() == reflect.Map {
		typ = typ.Elem()
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
	}
	if typ.Kind() != reflect.Struct || typ == timeType {
		return
	}
	if _, hasCodec := m.codecs[typ]; hasCodec {
		return
	}
	m.enterType(typ)
	defer m.leaveType(typ)
	for _, configured := range m.configFields(typ) {
		field := configured.field
		key := configTagName(field)
		m.validateNewPath(prefix, key)
		path := joinSchemaPath(prefix, key)
		m.paths[path] = field.Type
		m.collectCompositePaths(field.Type, path)
	}
}

func (m *schemaModel) configFields(typ reflect.Type) []configField {
	fields, inlinedTypes := configFields(typ)
	for _, inlinedType := range inlinedTypes {
		if _, hasCodec := m.codecs[inlinedType]; hasCodec {
			panic(fmt.Errorf("cfgm: inline config type %s cannot use a codec", inlinedType))
		}
	}
	return fields
}

func (m *schemaModel) enterType(typ reflect.Type) {
	if m.active[typ] {
		panic(fmt.Errorf("cfgm: recursive config type %s is not supported", typ))
	}
	m.active[typ] = true
}

func (m *schemaModel) leaveType(typ reflect.Type) { delete(m.active, typ) }

func (m *schemaModel) validateEnvironmentNames() {
	seen := make(map[string]string, len(m.fields))
	for _, field := range m.fields {
		name := envName(field.path)
		if previous, ok := seen[name]; ok {
			panic(fmt.Errorf("cfgm: config paths %q and %q map to the same environment name %s", previous, field.path, name))
		}
		seen[name] = field.path
	}
}

func (m *schemaModel) validateData(
	data map[string]any,
	codecs map[reflect.Type]valueCodec,
	allowUnknownKeys bool,
) error {
	encoded, err := marshalConfigJSON(data)
	if err != nil {
		return err
	}
	output := reflect.New(m.rootType).Interface()
	return decodeConfigJSON(encoded, output, codecs, !allowUnknownKeys)
}

func (m *schemaModel) hasPath(path string) bool {
	if _, ok := m.paths[path]; ok {
		return true
	}
	for fieldPath, typ := range m.paths {
		if typ.Kind() == reflect.Map && strings.HasPrefix(path, fieldPath+".") {
			return true
		}
	}
	return false
}

func (m *schemaModel) isStructPath(path string) bool { return m.structs[path] }

func (m *schemaModel) isFieldPath(path string) bool { return m.fieldSet[path] }

func isStructSlice(typ reflect.Type) bool {
	if typ.Kind() != reflect.Slice {
		return false
	}
	elem := typ.Elem()
	if elem.Kind() == reflect.Pointer {
		elem = elem.Elem()
	}
	return elem.Kind() == reflect.Struct && elem != timeType && elem != durationType
}

type structSliceValue struct {
	typ     reflect.Type
	codecs  map[reflect.Type]valueCodec
	items   []map[string]any
	cleared bool
}

func newStructSliceValue(typ reflect.Type, codecs map[reflect.Type]valueCodec) *structSliceValue {
	return &structSliceValue{typ: typ, codecs: codecs}
}

func (v *structSliceValue) Set(raw string) error {
	if strings.TrimSpace(raw) == "[]" {
		if len(v.items) > 0 {
			return errors.New("clear value [] cannot be combined with structured values")
		}
		v.cleared = true
		return nil
	}
	if v.cleared {
		return errors.New("structured values cannot be combined with clear value []")
	}
	var item map[string]any
	targetType := v.typ.Elem()
	if targetType.Kind() == reflect.Pointer {
		targetType = targetType.Elem()
	}
	if err := decodeJSONDocument([]byte(raw), &item); err != nil {
		return fmt.Errorf("parse JSON object: %w", err)
	}
	if item == nil {
		return errors.New("value must be a JSON object")
	}
	if err := validateJSONObject(item, targetType, v.codecs); err != nil {
		return err
	}
	v.items = append(v.items, item)
	return nil
}

func (v *structSliceValue) String() string {
	if v.cleared {
		return "[]"
	}
	encoded, err := json.Marshal(v.items)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func (v *structSliceValue) Get() any { return v }

func (v *structSliceValue) configValue() any {
	if v.cleared {
		return []any{}
	}
	items := make([]any, len(v.items))
	for index := range v.items {
		items[index] = v.items[index]
	}
	return items
}

type configLoader[T any] struct {
	defaults          T
	schema            *schemaModel
	sources           []Source
	logger            *slog.Logger
	expandTemplates   bool
	strictUnknownKeys bool
	codecs            map[reflect.Type]valueCodec
}

func (l *configLoader[T]) load(ctx context.Context) (*T, *Report, error) {
	if ctx == nil {
		return nil, nil, errors.New("cfgm: nil context")
	}
	configMap := structToMapWithCodecs(l.defaults, l.codecs)
	lookup := environmentSnapshot()
	report := &Report{}
	for _, source := range l.sources {
		if err := ctx.Err(); err != nil {
			return nil, report, err
		}
		if source == nil {
			continue
		}
		data, err := source.Load(ctx, Schema{model: l.schema, codecs: l.codecs, lookup: lookup})
		if err != nil {
			return nil, report, fmt.Errorf("%s: %w", source.Name(), err)
		}
		keys := flattenSchemaKeys(data)
		slices.Sort(keys)
		if err := l.schema.validateData(data, l.codecs, !l.strictUnknownKeys); err != nil {
			return nil, report, fmt.Errorf("%s: %w", source.Name(), err)
		}
		mergeMaps(configMap, data)
		report.Sources = append(report.Sources, SourceReport{Name: source.Name(), Keys: keys})
		l.logger.DebugContext(ctx, "Loaded config source", "source", source.Name(), "keys", keys)
	}
	if l.expandTemplates {
		if _, err := expandTemplateValues(configMap, "root", lookup); err != nil {
			return nil, report, fmt.Errorf("expand template in effective config: %w", err)
		}
	}
	var config T
	if err := decodeConfigMapWithCodecs(configMap, &config, l.codecs); err != nil {
		return nil, report, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return &config, report, nil
}

func decodeConfigMapWithCodecs(data map[string]any, out any, codecs map[reflect.Type]valueCodec) error {
	encoded, err := marshalConfigJSON(data)
	if err != nil {
		return err
	}
	return decodeConfigJSON(encoded, out, codecs, false)
}

func decodeConfigJSON(data []byte, out any, codecs map[reflect.Type]valueCodec, rejectUnknown bool) error {
	opts := []json.Options{json.MatchCaseInsensitiveNames(false)}
	if rejectUnknown {
		opts = append(opts, json.RejectUnknownMembers(true))
	}
	opts = append(opts, json.WithUnmarshalers(configUnmarshalers(codecs)))
	return json.Unmarshal(data, out, opts...)
}

// configUnmarshalers bridges the runtime-configured codec registry to the
// generic json/v2 unmarshaler. The interface target is intentionally broad:
// json/v2 supplies a pointer to each concrete value, allowing one function to
// dispatch only the configured leaf types and defer all others to defaults.
func configUnmarshalers(codecs map[reflect.Type]valueCodec) *json.Unmarshalers {
	return json.UnmarshalFromFunc(func(decoder *jsontext.Decoder, value any) error {
		pointer := reflect.ValueOf(value)
		if !pointer.IsValid() || pointer.Kind() != reflect.Pointer || pointer.IsNil() {
			return errors.ErrUnsupported
		}
		typ := pointer.Type().Elem()
		if decoder.PeekKind() == 'n' && typ.Kind() != reflect.Pointer && typ.Kind() != reflect.Slice &&
			typ.Kind() != reflect.Map && typ.Kind() != reflect.Interface {
			return fmt.Errorf("config value for %s cannot be null", typ)
		}
		codec, ok := codecs[typ]
		if !ok && typ != durationType {
			return errors.ErrUnsupported
		}
		if decoder.PeekKind() != '"' {
			if typ == durationType {
				return errors.ErrUnsupported
			}
			return fmt.Errorf("config value for %s must be a string", typ)
		}
		raw, err := decoder.ReadValue()
		if err != nil {
			return err
		}
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return fmt.Errorf("config value for %s must be a string: %w", typ, err)
		}
		var parsed any
		if typ == durationType {
			parsed, err = time.ParseDuration(text)
		} else {
			parsed, err = codec.parse(text)
		}
		if err != nil {
			return err
		}
		converted := reflect.ValueOf(parsed)
		if !converted.IsValid() {
			pointer.Elem().Set(reflect.Zero(typ))
			return nil
		}
		if converted.Type() != typ {
			if !converted.Type().ConvertibleTo(typ) {
				return fmt.Errorf("codec returned %s, want %s", converted.Type(), typ)
			}
			converted = converted.Convert(typ)
		}
		pointer.Elem().Set(converted)
		return nil
	})
}

func marshalConfigJSON(value any) ([]byte, error) {
	return json.Marshal(value, json.WithMarshalers(durationMarshaler()))
}

func durationMarshaler() *json.Marshalers {
	return json.MarshalFunc(func(value time.Duration) ([]byte, error) {
		return json.Marshal(value.String())
	})
}

func cleanConfigPath(path string) string {
	return strings.Trim(strings.TrimSpace(path), ".")
}

func bindingExcluded(path string, exclusions map[string]bool) bool {
	for exclusion := range exclusions {
		if pathWithin(path, exclusion) {
			return true
		}
	}
	return false
}

func pathWithin(path, prefix string) bool {
	return prefix != "" && (path == prefix || strings.HasPrefix(path, prefix+"."))
}

func bindingFlagName(path, commandPath string) string {
	if commandPath != "" {
		if name, ok := strings.CutPrefix(path, commandPath+"."); ok {
			return name
		}
	}
	return path
}

func joinSchemaPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

func valueAtPath(root reflect.Value, index []int) (reflect.Value, bool) {
	if root.Kind() == reflect.Pointer {
		if root.IsNil() {
			return reflect.Value{}, false
		}
		root = root.Elem()
	}
	for _, fieldIndex := range index {
		if root.Kind() == reflect.Pointer {
			if root.IsNil() {
				return reflect.Value{}, false
			}
			root = root.Elem()
		}
		root = root.Field(fieldIndex)
	}
	return root, true
}

func validateJSONObject(
	item map[string]any,
	typ reflect.Type,
	codecs map[reflect.Type]valueCodec,
) error {
	encoded, err := marshalConfigJSON(item)
	if err != nil {
		return err
	}
	return decodeConfigJSON(encoded, reflect.New(typ).Interface(), codecs, true)
}

func flattenSchemaKeys(data map[string]any) []string {
	var keys []string
	flattenSchemaValue(data, "", &keys)
	slices.Sort(keys)
	return slices.Compact(keys)
}

func flattenSchemaValue(value any, prefix string, keys *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		if len(typed) == 0 && prefix != "" {
			*keys = append(*keys, prefix)
			return
		}
		for key, child := range typed {
			path := joinSchemaPath(prefix, key)
			flattenSchemaValue(child, path, keys)
		}
	case []any:
		if len(typed) == 0 {
			*keys = append(*keys, prefix)
			return
		}
		for _, child := range typed {
			flattenSchemaValue(child, prefix, keys)
		}
	default:
		*keys = append(*keys, prefix)
	}
}

func mapsCloneSlices[K comparable, V any](source map[K][]V) map[K][]V {
	if source == nil {
		return nil
	}
	out := make(map[K][]V, len(source))
	for key, values := range source {
		out[key] = append([]V(nil), values...)
	}
	return out
}
