package structured

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/1azar/cogito/controller"
)

const (
	defaultMaxAttempts = 3
)

const defaultBasePrompt = `Return ONLY valid JSON that strictly matches the schema.
Rules:
1) Output must be a single JSON value (object or array), no markdown, no prose.
2) Use exact field names from the schema.
3) Do not add unknown fields.
4) Keep value types strict (numbers are numbers, booleans are booleans).

Target JSON schema:
{{SCHEMA}}

User task:
{{USER_INPUT}}`

const defaultRepairPrompt = `Your previous output is invalid for the target schema.
Fix it and return ONLY corrected JSON.

Target JSON schema:
{{SCHEMA}}

Validation/parse error:
{{ERROR}}

Previous output:
{{LAST_RESPONSE}}`

type Config struct {
	// Output must be a non-nil pointer to target value (struct, slice, map, etc).
	Output any

	// MaxAttempts is total attempts (first generation + repairs).
	// If <= 0, default is 3.
	MaxAttempts int

	// BasePrompt allows overriding the initial prompt template.
	// Supported placeholders: {{SCHEMA}}, {{USER_INPUT}}.
	BasePrompt string

	// RepairPrompt allows overriding the self-repair prompt template.
	// Supported placeholders: {{SCHEMA}}, {{ERROR}}, {{LAST_RESPONSE}}, {{USER_INPUT}}.
	RepairPrompt string

	// AllowUnknownFields disables strict unknown-field checks during decode.
	AllowUnknownFields bool
}

type Controller[T any] struct {
	cfg Config
}

func New[T any](cfg Config) *Controller[T] {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = defaultMaxAttempts
	}
	if cfg.BasePrompt == "" {
		cfg.BasePrompt = defaultBasePrompt
	}
	if cfg.RepairPrompt == "" {
		cfg.RepairPrompt = defaultRepairPrompt
	}

	return &Controller[T]{cfg: cfg}
}

func (c *Controller[T]) Run(ctx context.Context, agent controller.AgentLike[T], input string) (string, error) {
	if err := validateOutputTarget(c.cfg.Output); err != nil {
		return "", err
	}

	schema, err := buildSchemaJSON(c.cfg.Output)
	if err != nil {
		return "", fmt.Errorf("build output schema: %w", err)
	}

	prompt := applyTemplate(c.cfg.BasePrompt, templateValues{
		Schema:    schema,
		UserInput: input,
	})

	var lastErr error
	var lastResp string

	for attempt := 1; attempt <= c.cfg.MaxAttempts; attempt++ {
		resp, err := agent.CallLLM(ctx, prompt)
		if err != nil {
			return "", fmt.Errorf("llm call attempt %d failed: %w", attempt, err)
		}
		lastResp = resp

		extracted, err := ExtractJSON(resp)
		if err != nil {
			lastErr = fmt.Errorf("extract JSON: %w", err)
		} else {
			canonical, err := decodeIntoTarget(c.cfg.Output, []byte(extracted), !c.cfg.AllowUnknownFields)
			if err == nil {
				return canonical, nil
			}
			lastErr = fmt.Errorf("decode JSON: %w", err)
		}

		if attempt == c.cfg.MaxAttempts {
			break
		}

		prompt = applyTemplate(c.cfg.RepairPrompt, templateValues{
			Schema:       schema,
			UserInput:    input,
			ErrorText:    lastErr.Error(),
			LastResponse: lastResp,
		})
	}

	return "", fmt.Errorf("failed to produce valid structured JSON in %d attempts: %w", c.cfg.MaxAttempts, lastErr)
}

func validateOutputTarget(target any) error {
	if target == nil {
		return errors.New("structured controller: config.Output is nil")
	}

	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return errors.New("structured controller: config.Output must be a non-nil pointer")
	}

	return nil
}

func decodeIntoTarget(target any, payload []byte, strict bool) (string, error) {
	ptrValue := reflect.ValueOf(target)
	elemType := ptrValue.Elem().Type()

	tmp := reflect.New(elemType)
	dec := json.NewDecoder(bytes.NewReader(payload))
	if strict {
		dec.DisallowUnknownFields()
	}
	dec.UseNumber()

	if err := dec.Decode(tmp.Interface()); err != nil {
		return "", err
	}

	if err := ensureEOF(dec); err != nil {
		return "", err
	}

	ptrValue.Elem().Set(tmp.Elem())

	canonical, err := json.Marshal(tmp.Interface())
	if err != nil {
		return "", err
	}

	return string(canonical), nil
}

func ensureEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("unexpected trailing JSON tokens")
		}
		return err
	}
	return nil
}

func isValidJSON(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	return json.Valid([]byte(s))
}

type templateValues struct {
	Schema       string
	UserInput    string
	ErrorText    string
	LastResponse string
}

func applyTemplate(tpl string, values templateValues) string {
	replacer := strings.NewReplacer(
		"{{SCHEMA}}", values.Schema,
		"{{USER_INPUT}}", values.UserInput,
		"{{ERROR}}", values.ErrorText,
		"{{LAST_RESPONSE}}", values.LastResponse,
	)

	return replacer.Replace(tpl)
}

func buildSchemaJSON(target any) (string, error) {
	t := reflect.TypeOf(target)
	if t == nil {
		return "", errors.New("nil target type")
	}
	if t.Kind() != reflect.Ptr {
		return "", errors.New("target must be pointer")
	}

	schema := schemaForType(t.Elem(), make(map[reflect.Type]bool))
	b, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return "", err
	}

	return string(b), nil
}

func schemaForType(t reflect.Type, seen map[reflect.Type]bool) map[string]any {
	t = unwrapType(t)

	if seen[t] {
		return map[string]any{"type": "object"}
	}

	seen[t] = true
	defer delete(seen, t)

	switch t.Kind() {
	case reflect.Struct:
		props := make(map[string]any)
		required := make([]string, 0)
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}

			name, omitEmpty, skip := parseJSONTag(f)
			if skip {
				continue
			}

			props[name] = schemaForType(f.Type, seen)
			if !omitEmpty {
				required = append(required, name)
			}
		}

		out := map[string]any{
			"type":       "object",
			"properties": props,
		}
		if len(required) > 0 {
			out["required"] = required
		}
		return out

	case reflect.Slice, reflect.Array:
		return map[string]any{
			"type":  "array",
			"items": schemaForType(t.Elem(), seen),
		}

	case reflect.Map:
		if t.Key().Kind() == reflect.String {
			return map[string]any{
				"type":                 "object",
				"additionalProperties": schemaForType(t.Elem(), seen),
			}
		}
		return map[string]any{"type": "object"}

	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Interface:
		return map[string]any{}
	default:
		return map[string]any{"type": "string"}
	}
}

func unwrapType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

func parseJSONTag(f reflect.StructField) (name string, omitEmpty bool, skip bool) {
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

	for _, p := range parts[1:] {
		if p == "omitempty" {
			omitEmpty = true
			break
		}
	}

	return name, omitEmpty, false
}

var _ controller.Controller[any] = (*Controller[any])(nil)
