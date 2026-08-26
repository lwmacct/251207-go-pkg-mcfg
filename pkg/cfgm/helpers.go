package cfgm

import (
	"encoding"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	yamlv3 "go.yaml.in/yaml/v3"
)

var (
	durationType = reflect.TypeFor[time.Duration]()
	timeType     = reflect.TypeFor[time.Time]()
)

type configField struct {
	field reflect.StructField
	index []int
	key   string
}

func configFields(typ reflect.Type) ([]configField, []reflect.Type) {
	var fields []configField
	var embeddedTypes []reflect.Type
	active := make(map[reflect.Type]bool)
	var collect func(reflect.Type, []int)
	collect = func(current reflect.Type, parentIndex []int) {
		current = normalizeStructType(current)
		if current.Kind() != reflect.Struct {
			panic(fmt.Errorf("cfgm: embedded config type %s must be a struct", current))
		}
		if active[current] {
			panic(fmt.Errorf("cfgm: recursive config type %s is not supported", current))
		}
		active[current] = true
		defer delete(active, current)
		for field := range current.Fields() {
			if field.PkgPath != "" {
				if field.Anonymous {
					panic(fmt.Errorf("cfgm: unexported anonymous field %s is unsupported", field.Name))
				}
				continue
			}
			key, embedded := configFieldTag(field)
			index := append(append([]int(nil), parentIndex...), field.Index...)
			if embedded {
				embeddedTypes = append(embeddedTypes, field.Type, normalizeStructType(field.Type))
				collect(normalizeStructType(field.Type), index)
				continue
			}
			if key == "" {
				continue
			}
			fields = append(fields, configField{field: field, index: index, key: key})
		}
	}
	collect(typ, nil)
	return fields, embeddedTypes
}

func configFieldTag(field reflect.StructField) (string, bool) {
	if tag, ok := field.Tag.Lookup("cfgm"); ok {
		panic(fmt.Errorf("cfgm: field %s uses removed cfgm tag %q; use json tags", field.Name, tag))
	}

	tag, _ := field.Tag.Lookup("json")
	if tag == "-" {
		return "", false
	}
	if !field.IsExported() {
		return "", false
	}

	parts := strings.Split(tag, ",")
	name := field.Name
	hasName := false
	if len(parts) > 0 && parts[0] != "" {
		name = parts[0]
		hasName = true
	}
	if hasName && strings.HasPrefix(name, "'") {
		panic(fmt.Errorf("cfgm: field %s uses an unsupported quoted json name", field.Name))
	}
	if !utf8.ValidString(name) {
		panic(fmt.Errorf("cfgm: field %s has a json name with invalid UTF-8", field.Name))
	}
	if strings.ContainsAny(name, "\\\"`") {
		panic(fmt.Errorf("cfgm: field %s has unsupported characters in json name %q", field.Name, name))
	}

	embed := false
	var unsupportedOption string
	for _, option := range parts[1:] {
		if option == "" {
			panic(fmt.Errorf("cfgm: field %s has malformed json tag %q", field.Name, tag))
		}
		switch option {
		case "embed":
			if embed {
				panic(fmt.Errorf("cfgm: field %s has duplicate json option %q", field.Name, option))
			}
			embed = true
		default:
			if unsupportedOption == "" {
				unsupportedOption = option
			}
		}
	}

	underlying := normalizeStructType(field.Type)
	implicitEmbed := field.Anonymous && !hasName && underlying.Kind() == reflect.Struct
	if field.Anonymous && !hasName && underlying.Kind() != reflect.Struct && !embed {
		panic(fmt.Errorf("cfgm: anonymous field %s must be a struct or have an explicit json name", field.Name))
	}
	if embed || implicitEmbed {
		if hasName || len(parts) > 2 || (len(parts) == 2 && parts[1] != "embed") {
			panic(fmt.Errorf("cfgm: embedded field %s may only use json:,embed", field.Name))
		}
		if underlying.Kind() != reflect.Struct || underlying == durationType || underlying == timeType {
			panic(fmt.Errorf("cfgm: embedded field %s must be a struct or pointer to struct; map fallbacks are unsupported", field.Name))
		}
		if implementsJSONMethods(underlying) {
			panic(fmt.Errorf("cfgm: embedded config type %s must not implement JSON or text methods", underlying))
		}
		return "", true
	}
	if unsupportedOption != "" {
		panic(fmt.Errorf("cfgm: field %s uses unsupported json option %q", field.Name, unsupportedOption))
	}
	return name, false
}

func implementsJSONMethods(typ reflect.Type) bool {
	interfaces := []reflect.Type{
		reflect.TypeFor[json.Marshaler](),
		reflect.TypeFor[json.Unmarshaler](),
		reflect.TypeFor[encoding.TextMarshaler](),
		reflect.TypeFor[encoding.TextUnmarshaler](),
	}
	for _, iface := range interfaces {
		if typ.Implements(iface) || reflect.PointerTo(typ).Implements(iface) {
			return true
		}
	}
	return false
}

func isStructType(typ reflect.Type) bool {
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	return typ.Kind() == reflect.Struct && typ != durationType && typ != timeType
}

func normalizeStructType(typ reflect.Type) reflect.Type {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ
}

func isMapType(typ reflect.Type) bool {
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	return typ.Kind() == reflect.Map
}

func structToMapWithCodecs(
	cfg any,
	codecs map[reflect.Type]valueCodec,
	fieldsFor func(reflect.Type) []configField,
) map[string]any {
	val := reflect.ValueOf(cfg)
	return structValueToMap(val, val.Type(), codecs, fieldsFor)
}

func structValueToMap(
	val reflect.Value,
	typ reflect.Type,
	codecs map[reflect.Type]valueCodec,
	fieldsFor func(reflect.Type) []configField,
) map[string]any {
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return map[string]any{}
		}
		val = val.Elem()
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return map[string]any{}
	}

	out := make(map[string]any)
	fields := fieldsFor(typ)
	for _, configured := range fields {
		field := configured.field
		fieldVal, ok := valueAtPath(val, configured.index)
		if !ok {
			continue
		}
		out[configured.key] = valueToAny(fieldVal, field.Type, codecs, fieldsFor)
	}

	return out
}

func valueToAny(
	val reflect.Value,
	typ reflect.Type,
	codecs map[reflect.Type]valueCodec,
	fieldsFor func(reflect.Type) []configField,
) any {
	if codec, ok := codecs[typ]; ok {
		return codec.format(val.Interface())
	}
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return nil
		}
		val = val.Elem()
		typ = typ.Elem()
	}

	if isStructType(typ) {
		return structValueToMap(val, typ, codecs, fieldsFor)
	}

	switch val.Kind() {
	case reflect.Slice:
		if val.IsNil() {
			return nil
		}
		out := make([]any, val.Len())
		for i := range val.Len() {
			elem := val.Index(i)
			out[i] = valueToAny(elem, elem.Type(), codecs, fieldsFor)
		}

		return out
	case reflect.Map:
		if val.IsNil() {
			return nil
		}
		out := make(map[string]any, val.Len())
		iter := val.MapRange()
		for iter.Next() {
			key := fmt.Sprintf("%v", iter.Key().Interface())
			out[key] = valueToAny(iter.Value(), iter.Value().Type(), codecs, fieldsFor)
		}

		return out
	default:
		return val.Interface()
	}
}

func parseConfigBytes(path string, content []byte) (map[string]any, error) {
	var raw any
	var err error
	if isJSONPath(path) {
		err = decodeJSONDocument(content, &raw)
	} else {
		err = yamlv3.Unmarshal(content, &raw)
	}
	if err != nil {
		return nil, err
	}

	normalized := normalizeMapKeys(raw)
	if normalized == nil {
		return map[string]any{}, nil
	}
	configMap, ok := normalized.(map[string]any)
	if !ok {
		return nil, errors.New("config root must be object")
	}

	return configMap, nil
}

func decodeJSONDocument(data []byte, out any) error {
	numbers := json.UnmarshalFromFunc(func(decoder *jsontext.Decoder, value *any) error {
		if decoder.PeekKind() == '0' {
			*value = jsontext.Value(nil)
		}
		return errors.ErrUnsupported
	})
	return json.Unmarshal(data, out, json.WithUnmarshalers(numbers))
}

func isJSONPath(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".json")
}

func normalizeMapKeys(val any) any {
	switch typed := val.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			out[key] = normalizeMapKeys(value)
		}

		return out
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			out[fmt.Sprintf("%v", key)] = normalizeMapKeys(value)
		}

		return out
	case []any:
		for i := range typed {
			typed[i] = normalizeMapKeys(typed[i])
		}
		return typed
	default:
		return val
	}
}

func mergeMaps(dst, src map[string]any) {
	for key, value := range src {
		if valueMap, ok := value.(map[string]any); ok {
			if dstMap, ok := dst[key].(map[string]any); ok {
				mergeMaps(dstMap, valueMap)
				continue
			}
		}

		dst[key] = value
	}
}

func setByPath(dst map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	current := dst
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value

			return
		}

		next, ok := current[part].(map[string]any)
		if !ok {
			next = make(map[string]any)
			current[part] = next
		}
		current = next
	}
}
