package cfgm

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	yamlv3 "go.yaml.in/yaml/v3"
)

var (
	durationType = reflect.TypeFor[time.Duration]()
	timeType     = reflect.TypeFor[time.Time]()
)

func configTagName(field reflect.StructField) string {
	return parseTagName(field.Tag.Get("json"))
}

type configField struct {
	field reflect.StructField
	index []int
}

func configFields(typ reflect.Type) ([]configField, []reflect.Type) {
	var fields []configField
	var inlinedTypes []reflect.Type
	var collect func(reflect.Type, []int)
	collect = func(current reflect.Type, parentIndex []int) {
		for field := range current.Fields() {
			if field.PkgPath != "" {
				continue
			}
			key, inline := configFieldTag(field)
			index := append(append([]int(nil), parentIndex...), field.Index...)
			if inline {
				inlinedTypes = append(inlinedTypes, field.Type)
				collect(field.Type, index)
				continue
			}
			if key == "" {
				continue
			}
			fields = append(fields, configField{field: field, index: index})
		}
	}
	collect(typ, nil)
	return fields, inlinedTypes
}

func configFieldTag(field reflect.StructField) (string, bool) {
	key := configTagName(field)
	tag := field.Tag.Get("cfgm")
	if tag == "" {
		return key, false
	}
	parts := strings.Split(tag, ",")
	if len(parts) != 2 || parts[0] != "" || parts[1] != "inline" {
		panic(fmt.Errorf("cfgm: config field %s has invalid cfgm tag %q", field.Name, tag))
	}
	if key != "" {
		panic(fmt.Errorf("cfgm: inline config field %s must not have a name", field.Name))
	}
	if field.Tag.Get("json") != "" {
		panic(fmt.Errorf("cfgm: inline config field %s must not have a json tag", field.Name))
	}
	if !field.Anonymous || field.Type.Kind() != reflect.Struct || field.Type == durationType || field.Type == timeType {
		panic(fmt.Errorf("cfgm: inline config field %s must be an anonymous non-pointer struct", field.Name))
	}
	return "", true
}

func parseTagName(tag string) string {
	if tag == "" {
		return ""
	}
	parts := strings.Split(tag, ",")
	if len(parts) == 0 || parts[0] == "" || parts[0] == "-" {
		return ""
	}

	return parts[0]
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

func structToMap(cfg any) map[string]any {
	return structToMapWithCodecs(cfg, nil)
}

func structToMapWithCodecs(cfg any, codecs map[reflect.Type]valueCodec) map[string]any {
	val := reflect.ValueOf(cfg)
	return structValueToMap(val, val.Type(), codecs)
}

func structValueToMap(val reflect.Value, typ reflect.Type, codecs map[reflect.Type]valueCodec) map[string]any {
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
	fields, _ := configFields(typ)
	for _, configured := range fields {
		field := configured.field
		key := configTagName(field)
		fieldVal := val.FieldByIndex(configured.index)
		out[key] = valueToAny(fieldVal, field.Type, codecs)
	}

	return out
}

func valueToAny(val reflect.Value, typ reflect.Type, codecs map[reflect.Type]valueCodec) any {
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
		return structValueToMap(val, typ, codecs)
	}

	switch val.Kind() {
	case reflect.Slice:
		if val.IsNil() {
			return nil
		}
		out := make([]any, val.Len())
		for i := range val.Len() {
			elem := val.Index(i)
			out[i] = valueToAny(elem, elem.Type(), codecs)
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
			out[key] = valueToAny(iter.Value(), iter.Value().Type(), codecs)
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
