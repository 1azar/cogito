package structured

import "testing"

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "pure object",
			input: `{"name":"Ana","age":7}`,
			want:  `{"name":"Ana","age":7}`,
		},
		{
			name:  "fenced json",
			input: "Here you go:\n```json\n{\"name\":\"Ana\",\"age\":7}\n```",
			want:  `{"name":"Ana","age":7}`,
		},
		{
			name:  "json in mixed text",
			input: "result: {\"name\":\"Ana\",\"age\":7} thanks",
			want:  `{"name":"Ana","age":7}`,
		},
		{
			name:    "invalid response",
			input:   "not json",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExtractJSON(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tc.want {
				t.Fatalf("unexpected json. want=%q got=%q", tc.want, got)
			}
		})
	}
}
