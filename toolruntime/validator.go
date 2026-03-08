package toolruntime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/1azar/cogito/tool"
)

func ValidateArguments(schema tool.JSONSchema, raw json.RawMessage) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte("{}")
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	var value any
	if err := dec.Decode(&value); err != nil {
		return fmt.Errorf("arguments must be valid JSON: %w", err)
	}

	if err := validateValue(schema, value, "$"); err != nil {
		return err
	}

	return nil
}

func validateValue(schema tool.JSONSchema, value any, path string) error {
	if schema.Type == "" {
		return nil
	}

	switch schema.Type {
	case "object":
		obj, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be object", path)
		}

		for _, field := range schema.Required {
			if _, exists := obj[field]; !exists {
				return fmt.Errorf("%s.%s is required", path, field)
			}
		}

		if len(schema.Properties) == 0 {
			return nil
		}

		for key := range obj {
			childSchema, exists := schema.Properties[key]
			if !exists {
				return fmt.Errorf("%s.%s is not allowed", path, key)
			}
			if err := validateValue(childSchema, obj[key], path+"."+key); err != nil {
				return err
			}
		}

		return nil

	case "array":
		arr, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be array", path)
		}
		if schema.Items == nil {
			return nil
		}
		for i, item := range arr {
			if err := validateValue(*schema.Items, item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		return nil

	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s must be string", path)
		}
		return nil

	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be boolean", path)
		}
		return nil

	case "number":
		if !isNumber(value) {
			return fmt.Errorf("%s must be number", path)
		}
		return nil

	case "integer":
		if !isInteger(value) {
			return fmt.Errorf("%s must be integer", path)
		}
		return nil

	default:
		return fmt.Errorf("%s has unsupported schema type %q", path, schema.Type)
	}
}

func isNumber(v any) bool {
	switch v.(type) {
	case float64, json.Number:
		return true
	default:
		return false
	}
}

func isInteger(v any) bool {
	switch n := v.(type) {
	case json.Number:
		s := n.String()
		return !strings.ContainsAny(s, ".eE")
	case float64:
		return n == float64(int64(n))
	default:
		return false
	}
}
