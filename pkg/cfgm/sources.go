package cfgm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

type fileSource struct {
	paths    []string
	optional bool
}

func File(path string, opts ...FileOption) Source {
	return Files([]string{path}, opts...)
}

// Files loads the first existing file from paths.
func Files(paths []string, opts ...FileOption) Source {
	source := &fileSource{
		paths: append([]string{}, paths...),
	}
	for _, opt := range opts {
		opt(source)
	}

	return source
}

// FileOption configures file sources.
type FileOption func(*fileSource)

// Optional allows a file source to be absent.
func Optional() FileOption {
	return func(s *fileSource) {
		s.optional = true
	}
}

// Required requires a file source to exist. This is the default.
func Required() FileOption {
	return func(s *fileSource) {
		s.optional = false
	}
}

func (s *fileSource) Name() string {
	if len(s.paths) == 1 {
		return "file:" + s.paths[0]
	}

	return "files"
}

func (s *fileSource) Load(ctx context.Context, schema Schema) (map[string]any, error) {
	if len(s.paths) == 0 {
		if s.optional {
			return map[string]any{}, nil
		}

		return nil, errors.New("no config paths configured")
	}

	for _, path := range s.paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		content, err := os.ReadFile(path) //nolint:gosec // path is provided by the caller
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", path, err)
		}

		configMap, err := parseConfigBytes(path, content)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}

		return configMap, nil
	}

	if s.optional {
		return map[string]any{}, nil
	}

	return nil, fmt.Errorf("none of the config files exist: %s", strings.Join(s.paths, ", "))
}

type envSource struct {
	prefix string
}

// Env loads environment variables for schema fields using the given prefix.
func Env(prefix string) Source {
	return &envSource{prefix: prefix}
}

func (s *envSource) Name() string {
	return "env:" + s.prefix
}

func (s *envSource) Load(ctx context.Context, schema Schema) (map[string]any, error) {
	if s.prefix == "" {
		return map[string]any{}, nil
	}

	out := map[string]any{}
	for _, field := range schema.Fields() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		envKey := s.prefix + envName(field.Path)
		lookup := schema.lookup
		if lookup == nil {
			lookup = os.LookupEnv
		}
		value, exists := lookup(envKey)
		if !exists {
			continue
		}
		parsed, err := schema.parseEnvValue(field, value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", envKey, err)
		}
		setByPath(out, field.Path, parsed)
	}

	return out, nil
}

func (s Schema) parseEnvValue(field Field, raw string) (any, error) {
	if _, ok := s.codecs[field.Type]; ok {
		return raw, nil
	}
	typ := field.Type
	if typ.Kind() != reflect.Slice && typ.Kind() != reflect.Map {
		return parseEnvScalar(field.Path, typ, raw)
	}
	var value any
	if err := decodeJSONDocument([]byte(raw), &value); err != nil {
		return nil, fmt.Errorf("parse %s as JSON %s: %w", field.Path, typ, err)
	}
	if typ.Kind() == reflect.Slice {
		if _, ok := value.([]any); !ok {
			return nil, fmt.Errorf("%s must be a JSON array", field.Path)
		}
	} else if _, ok := value.(map[string]any); !ok {
		return nil, fmt.Errorf("%s must be a JSON object", field.Path)
	}
	return value, nil
}

func parseEnvScalar(path string, typ reflect.Type, raw string) (any, error) {
	switch typ.Kind() { //nolint:exhaustive // unsupported scalar kinds remain strings
	case reflect.Bool:
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("parse %s as %s: %w", path, typ, err)
		}
		return value, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if typ == durationType {
			return raw, nil
		}
		value, err := strconv.ParseInt(raw, 10, typ.Bits())
		if err != nil {
			return nil, fmt.Errorf("parse %s as %s: %w", path, typ, err)
		}
		out := reflect.New(typ).Elem()
		out.SetInt(value)
		return out.Interface(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		value, err := strconv.ParseUint(raw, 10, typ.Bits())
		if err != nil {
			return nil, fmt.Errorf("parse %s as %s: %w", path, typ, err)
		}
		out := reflect.New(typ).Elem()
		out.SetUint(value)
		return out.Interface(), nil
	case reflect.Float32, reflect.Float64:
		value, err := strconv.ParseFloat(raw, typ.Bits())
		if err != nil {
			return nil, fmt.Errorf("parse %s as %s: %w", path, typ, err)
		}
		out := reflect.New(typ).Elem()
		out.SetFloat(value)
		return out.Interface(), nil
	default:
		return raw, nil
	}
}

func envName(path string) string {
	return strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(path))
}
