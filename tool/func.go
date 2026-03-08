package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

type funcTool struct {
	name string
	desc string

	fn        reflect.Value
	inputType reflect.Type
}

func Func[I any, O any](
	name string,
	description string,
	fn func(context.Context, I) (O, error),
) (Tool, error) {

	if name == "" {
		return nil, errors.New("tool name is empty")
	}

	if fn == nil {
		return nil, errors.New("tool function is nil")
	}

	t := &funcTool{
		name: name,
		desc: description,
		fn:   reflect.ValueOf(fn),
	}

	var in I
	t.inputType = reflect.TypeOf(in)

	// input must be struct or map
	if t.inputType.Kind() != reflect.Struct &&
		t.inputType.Kind() != reflect.Map {

		return nil, fmt.Errorf(
			"tool %s: input type must be struct or map, got %s",
			name,
			t.inputType.Kind(),
		)
	}

	if t.inputType.Kind() == reflect.Map && t.inputType.Key().Kind() != reflect.String {
		return nil, fmt.Errorf(
			"tool %s: map input key type must be string, got %s",
			name,
			t.inputType.Key().Kind(),
		)
	}

	return t, nil
}

func (t *funcTool) Name() string {
	return t.name
}

func (t *funcTool) Description() string {
	return t.desc
}

func (t *funcTool) Spec() Spec {
	return Spec{
		Name:        t.name,
		Description: t.desc,
		Parameters:  structSchema(t.inputType),
	}
}

func (t *funcTool) Call(
	ctx context.Context,
	input json.RawMessage,
) (any, error) {

	in := reflect.New(t.inputType)

	if err := json.Unmarshal(input, in.Interface()); err != nil {
		return nil, fmt.Errorf(
			"tool %s: invalid arguments: %w",
			t.name,
			err,
		)
	}

	out := t.fn.Call([]reflect.Value{
		reflect.ValueOf(ctx),
		in.Elem(),
	})

	if !out[1].IsNil() {
		return nil, fmt.Errorf(
			"tool %s failed: %w",
			t.name,
			out[1].Interface().(error),
		)
	}

	return out[0].Interface(), nil
}
