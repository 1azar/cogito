package tool

import "reflect"

func structSchema(t reflect.Type) map[string]any {

	if t.Kind() != reflect.Struct {
		return map[string]any{
			"type": "object",
		}
	}

	props := map[string]any{}
	var required []string

	for i := 0; i < t.NumField(); i++ {

		f := t.Field(i)

		if !f.IsExported() {
			continue
		}

		name := f.Tag.Get("json")
		if name == "" {
			name = f.Name
		}

		props[name] = fieldSchema(f.Type)
		required = append(required, name)
	}

	return map[string]any{
		"type":       "object",
		"properties": props,
		"required":   required,
	}
}

func fieldSchema(t reflect.Type) map[string]any {

	switch t.Kind() {

	case reflect.String:
		return map[string]any{"type": "string"}

	case reflect.Int, reflect.Int64:
		return map[string]any{"type": "integer"}

	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}

	case reflect.Bool:
		return map[string]any{"type": "boolean"}

	case reflect.Struct:
		return structSchema(t)

	case reflect.Slice, reflect.Array:
		return map[string]any{
			"type":  "array",
			"items": fieldSchema(t.Elem()),
		}

	default:
		return map[string]any{"type": "string"}
	}
}
