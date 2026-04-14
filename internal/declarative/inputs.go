// SPDX-License-Identifier: Apache-2.0

// inputs.go — Input validation against the capability's declared input schema.
//
//   - Called at manifest load time (ValidateConfig) to catch rule authoring
//     errors before any tool runs. Checks required fields, type correctness
//     (string/list/map/any), and rejects undeclared inputs.
//   - Errors are aggregated in sorted order for deterministic output across
//     runs, then joined into a single error string.
//   - Capabilities with no declared inputs (empty schema) skip the unknown-
//     input check, allowing pass-through of arbitrary config.
package declarative

import (
	"fmt"
	"sort"
	"strings"
)

// validateInputs checks that the rule's inputs map satisfies the capability's
// declared input schema. Returns a single error that aggregates all problems
// in stable order so callers see the same message across runs.
func validateInputs(schema map[string]InputField, inputs map[string]any) error {
	var problems []string

	// Required + type checks (iterate schema in sorted order for stability).
	keys := make([]string, 0, len(schema))
	for k := range schema {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, name := range keys {
		field := schema[name]
		v, present := inputs[name]
		if !present {
			if field.Required {
				problems = append(problems, fmt.Sprintf("required input %q is missing", name))
			}
			continue
		}
		if err := checkType(name, field.Type, v); err != nil {
			problems = append(problems, err.Error())
		}
	}

	// Unknown inputs.
	if len(schema) > 0 {
		extras := make([]string, 0)
		for name := range inputs {
			if _, ok := schema[name]; !ok {
				extras = append(extras, name)
			}
		}
		sort.Strings(extras)
		for _, name := range extras {
			problems = append(problems, fmt.Sprintf("unknown input %q (not declared in capability schema)", name))
		}
	}

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(problems, "; "))
}

func checkType(name, want string, v any) error {
	switch want {
	case "", "any":
		return nil
	case "string":
		if _, ok := v.(string); !ok {
			return fmt.Errorf("input %q: expected string, got %T", name, v)
		}
	case "list":
		if _, ok := v.([]any); !ok {
			return fmt.Errorf("input %q: expected list, got %T", name, v)
		}
	case "map":
		if _, ok := v.(map[string]any); !ok {
			return fmt.Errorf("input %q: expected map, got %T", name, v)
		}
	default:
		return fmt.Errorf("input %q: capability declares unknown type %q", name, want)
	}
	return nil
}
