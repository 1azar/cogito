package tool

type Spec struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  JSONSchema `json:"parameters"`
}

type JSONSchema struct {
	Type        string                `json:"type,omitempty"`
	Description string                `json:"description,omitempty"`
	Properties  map[string]JSONSchema `json:"properties,omitempty"`
	Required    []string              `json:"required,omitempty"`
	Items       *JSONSchema           `json:"items,omitempty"`
}
