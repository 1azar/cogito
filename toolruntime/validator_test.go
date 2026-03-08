package toolruntime

import (
	"testing"

	"github.com/1azar/cogito/tool"
)

func TestValidateArguments_StrictObject(t *testing.T) {
	schema := tool.JSONSchema{
		Type: "object",
		Properties: map[string]tool.JSONSchema{
			"city": {Type: "string"},
		},
		Required: []string{"city"},
	}

	if err := ValidateArguments(schema, []byte(`{"city":"Rome"}`)); err != nil {
		t.Fatalf("expected valid arguments, got error: %v", err)
	}

	if err := ValidateArguments(schema, []byte(`{}`)); err == nil {
		t.Fatalf("expected missing required field error")
	}

	if err := ValidateArguments(schema, []byte(`{"city":"Rome","country":"IT"}`)); err == nil {
		t.Fatalf("expected unknown field validation error")
	}
}
