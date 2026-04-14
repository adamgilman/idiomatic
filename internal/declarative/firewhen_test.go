// SPDX-License-Identifier: Apache-2.0

// firewhen_test.go — Tests for the fire_when expression parser and evaluator,
// covering grammar acceptance, evaluation against synthetic signals, template
// interpolation from rule inputs, and the missing-expression error contract.
package declarative

import (
	"strings"
	"testing"
)

// TestParseFireWhen_Grammar exercises the recursive-descent parser against
// the cases the bundled capabilities will actually use.
func TestParseFireWhen_Grammar(t *testing.T) {
	cases := []string{
		`signal.exit_code != 0`,
		`signal.exit_code == 0`,
		`signal.stdout matches "^(main|master)$"`,
		`signal.stdout contains "WARNING"`,
		`signal.stdout != ""`,
		`signal.stdout matches "main" and signal.exit_code == 0`,
		`not signal.stdout matches "release"`,
		`(signal.stdout matches "x" or signal.stdout matches "y") and signal.exit_code == 0`,
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			if _, err := parseFireWhen(in); err != nil {
				t.Errorf("parseFireWhen(%q): %v", in, err)
			}
		})
	}
}

// TestEvalFireWhen evaluates parsed expressions against synthetic signals.
func TestEvalFireWhen(t *testing.T) {
	cases := []struct {
		name      string
		ruleInput map[string]any
		signal    scalarSignal
		want      bool
	}{
		{
			name:      "exit_code != 0 fires",
			ruleInput: map[string]any{"fire_when": "signal.exit_code != 0"},
			signal:    scalarSignal{ExitCode: 1, Fields: map[string]any{"exit_code": 1}},
			want:      true,
		},
		{
			name:      "exit_code != 0 not fires",
			ruleInput: map[string]any{"fire_when": "signal.exit_code != 0"},
			signal:    scalarSignal{ExitCode: 0, Fields: map[string]any{"exit_code": 0}},
			want:      false,
		},
		{
			name:      "stdout matches main fires",
			ruleInput: map[string]any{"fire_when": `signal.stdout matches "^(main|master)$"`},
			signal:    scalarSignal{Stdout: "main", Fields: map[string]any{"stdout": "main"}},
			want:      true,
		},
		{
			name:      "stdout matches main does not fire on feature branch",
			ruleInput: map[string]any{"fire_when": `signal.stdout matches "^(main|master)$"`},
			signal:    scalarSignal{Stdout: "feature/x", Fields: map[string]any{"stdout": "feature/x"}},
			want:      false,
		},
		{
			name:      "templated pattern from inputs",
			ruleInput: map[string]any{"fire_when": `signal.stdout matches "{{ .Inputs.pattern }}"`, "pattern": "release-.*"},
			signal:    scalarSignal{Stdout: "release-1.0", Fields: map[string]any{"stdout": "release-1.0"}},
			want:      true,
		},
		{
			name:      "stdout contains WARNING",
			ruleInput: map[string]any{"fire_when": `signal.stdout contains "WARNING"`},
			signal:    scalarSignal{Stdout: "build WARNING line", Fields: map[string]any{"stdout": "build WARNING line"}},
			want:      true,
		},
		{
			name:      "and: both required",
			ruleInput: map[string]any{"fire_when": `signal.stdout != "" and signal.exit_code == 0`},
			signal:    scalarSignal{Stdout: "x", ExitCode: 0, Fields: map[string]any{"stdout": "x", "exit_code": 0}},
			want:      true,
		},
		{
			name:      "not negates",
			ruleInput: map[string]any{"fire_when": `not signal.stdout matches "main"`},
			signal:    scalarSignal{Stdout: "feature", Fields: map[string]any{"stdout": "feature"}},
			want:      true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := evalFireWhen(tc.ruleInput, tc.signal)
			if err != nil {
				t.Fatalf("evalFireWhen: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestEvalFireWhen_MissingExpression confirms scalar rules without
// fire_when produce an authoring error.
func TestEvalFireWhen_MissingExpression(t *testing.T) {
	_, err := evalFireWhen(map[string]any{}, scalarSignal{})
	if err == nil {
		t.Fatal("expected error when fire_when missing")
	}
	if !strings.Contains(err.Error(), "fire_when") {
		t.Errorf("error should mention fire_when: %v", err)
	}
}
