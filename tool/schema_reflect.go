package tool

import (
	"reflect"
	"strings"
)

func structSchema(t reflect.Type) JSONSchema {
	t = unwrapType(t)

	if t.Kind() != reflect.Struct {
		return JSONSchema{
			Type: "object",
		}
	}

	props := map[string]JSONSchema{}
	var required []string

	for i := 0; i < t.NumField(); i++ {

		f := t.Field(i)

		if !f.IsExported() {
			continue
		}

		name, omitEmpty, skip := parseJSONFieldTag(f)
		if skip {
			continue
		}

		props[name] = fieldSchema(f.Type)
		if !omitEmpty {
			required = append(required, name)
		}
	}

	return JSONSchema{
		Type:       "object",
		Properties: props,
		Required:   required,
	}
}

func fieldSchema(t reflect.Type) JSONSchema {
	t = unwrapType(t)

	switch t.Kind() {

	case reflect.String:
		return JSONSchema{Type: "string"}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return JSONSchema{Type: "integer"}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return JSONSchema{Type: "integer"}

	case reflect.Float32, reflect.Float64:
		return JSONSchema{Type: "number"}

	case reflect.Bool:
		return JSONSchema{Type: "boolean"}

	case reflect.Struct:
		return structSchema(t)

	case reflect.Slice, reflect.Array:
		items := fieldSchema(t.Elem())
		return JSONSchema{
			Type:  "array",
			Items: &items,
		}

	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return JSONSchema{Type: "object"}
		}
		return JSONSchema{Type: "object"}

	default:
		return JSONSchema{Type: "string"}
	}
}

func parseJSONFieldTag(f reflect.StructField) (name string, omitEmpty bool, skip bool) {
	tag := f.Tag.Get("json")
	if tag == "-" {
		return "", false, true
	}
	if tag == "" {
		return f.Name, false, false
	}

	parts := strings.Split(tag, ",")
	if parts[0] == "" {
		name = f.Name
	} else {
		name = parts[0]
	}

	for _, opt := range parts[1:] {
		if opt == "omitempty" {
			omitEmpty = true
			break
		}
	}

	return name, omitEmpty, false
}

func unwrapType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}
