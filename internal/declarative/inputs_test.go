// SPDX-License-Identifier: Apache-2.0

// inputs_test.go — Tests for validateInputs: required field enforcement, type
// checking (string/list/map/any), and rejection of undeclared input names.
package declarative

import (
	"strings"
	"testing"
)

func TestValidateInputs(t *testing.T) {
	schema := map[string]InputField{
		"language": {Type: "string", Required: true},
		"pattern":  {Type: "any"},
		"items":    {Type: "list"},
		"meta":     {Type: "map"},
	}

	cases := []struct {
		name      string
		inputs    map[string]any
		wantError string // substring; "" means no error expected
	}{
		{
			name:   "ok scalar",
			inputs: map[string]any{"language": "go", "pattern": "fmt.Println(...)"},
		},
		{
			name:   "ok list",
			inputs: map[string]any{"language": "go", "items": []any{"a", "b"}},
		},
		{
			name:   "ok map",
			inputs: map[string]any{"language": "go", "meta": map[string]any{"k": "v"}},
		},
		{
			name:      "missing required",
			inputs:    map[string]any{"pattern": "fmt.Println(...)"},
			wantError: `required input "language" is missing`,
		},
		{
			name:      "wrong type for string",
			inputs:    map[string]any{"language": 42},
			wantError: `expected string`,
		},
		{
			name:      "wrong type for list",
			inputs:    map[string]any{"language": "go", "items": "not a list"},
			wantError: `expected list`,
		},
		{
			name:      "unknown input",
			inputs:    map[string]any{"language": "go", "bogus": 1},
			wantError: `unknown input "bogus"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateInputs(schema, tc.inputs)
			if tc.wantError == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantError)
			}
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantError)
			}
		})
	}
}
