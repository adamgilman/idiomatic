// SPDX-License-Identifier: Apache-2.0

// signal_list_test.go — Tests for list-mode signal parsing against canned
// semgrep JSON output, verifying gjson extraction, tail_after_dot rule-ID
// transform, severity mapping, location fields, and unmatched-rule skipping.
package declarative

import (
	"testing"

	"github.com/adamgilman/idiomatic/manifest"
)

// TestParseListSignal_Semgrep verifies that canned semgrep JSON output is
// correctly walked, the rule id is extracted via tail_after_dot, and each
// finding is populated from the right gjson paths.
func TestParseListSignal_Semgrep(t *testing.T) {
	cap, err := NewFromBytes(semgrepYAML)
	if err != nil {
		t.Fatalf("NewFromBytes: %v", err)
	}

	// A real semgrep JSON output, simplified to two results that match the
	// rules in the request. Note the prefixed check_id ("idio-rules.go-no-panic")
	// — tail_after_dot must strip the prefix to find the pack rule.
	stdout := []byte(`{
		"results": [
			{
				"check_id": "idio-rules.go-no-panic",
				"path": "main.go",
				"start": {"line": 10, "col": 1},
				"end": {"line": 10, "col": 14},
				"extra": {"message": "Don't panic", "severity": "ERROR"}
			},
			{
				"check_id": "idio-rules.go-test-sleep",
				"path": "main_test.go",
				"start": {"line": 22, "col": 4},
				"end": {"line": 22, "col": 20},
				"extra": {"message": "No sleeps in tests", "severity": "WARNING"}
			},
			{
				"check_id": "idio-rules.unknown-rule",
				"path": "x.go",
				"start": {"line": 1, "col": 1},
				"end": {"line": 1, "col": 1},
				"extra": {"message": "ignored", "severity": "INFO"}
			}
		]
	}`)

	rules := []manifest.Rule{
		{
			ID:       "go-no-panic",
			Severity: manifest.SeverityError,
			Fix:      manifest.Fix{Message: "Don't panic in production", Example: "return err"},
		},
		{
			ID:       "go-test-sleep",
			Severity: manifest.SeverityWarning,
			Fix:      manifest.Fix{Message: "Use channels"},
		},
	}

	findings, err := cap.parseListSignal(&runOutput{Stdout: stdout}, rules)
	if err != nil {
		t.Fatalf("parseListSignal: %v", err)
	}

	if len(findings) != 2 {
		t.Fatalf("expected 2 findings (unknown-rule must be skipped), got %d", len(findings))
	}

	f0 := findings[0]
	if f0.RuleID != "go-no-panic" {
		t.Errorf("findings[0].RuleID = %q", f0.RuleID)
	}
	if f0.Severity != "error" {
		t.Errorf("findings[0].Severity = %q, want error", f0.Severity)
	}
	if f0.File != "main.go" {
		t.Errorf("findings[0].File = %q", f0.File)
	}
	if f0.StartLine != 10 || f0.StartCol != 1 || f0.EndLine != 10 || f0.EndCol != 14 {
		t.Errorf("findings[0] location wrong: %+v", f0)
	}
	if f0.Message != "Don't panic" {
		t.Errorf("findings[0].Message = %q", f0.Message)
	}
	if f0.FixMessage != "Don't panic in production" {
		t.Errorf("findings[0].FixMessage = %q", f0.FixMessage)
	}
	if f0.FixExample != "return err" {
		t.Errorf("findings[0].FixExample = %q", f0.FixExample)
	}

	f1 := findings[1]
	if f1.RuleID != "go-test-sleep" {
		t.Errorf("findings[1].RuleID = %q", f1.RuleID)
	}
	if f1.Severity != "warning" {
		t.Errorf("findings[1].Severity = %q, want warning", f1.Severity)
	}
}

func TestExtractRuleID_TailAfterDot(t *testing.T) {
	// Test extractRuleID directly via a synthetic input.
	cases := []struct {
		input string
		want  string
	}{
		{"tmp.go-no-panic", "go-no-panic"},
		{"a.b.c.go-rule", "go-rule"},
		{"no-prefix", "no-prefix"},
	}
	for _, tc := range cases {
		// We can't easily build a gjson.Result for a single string, so we
		// instead embed it in a fake JSON object and parse via the same
		// path the real code uses.
		stdout := []byte(`{"results":[{"check_id":"` + tc.input + `"}]}`)
		cap, err := NewFromBytes(semgrepYAML)
		if err != nil {
			t.Fatal(err)
		}
		// Provide a rule with the expected stripped ID so we can confirm
		// the extraction succeeds and matches.
		rules := []manifest.Rule{{ID: tc.want, Fix: manifest.Fix{}}}
		findings, err := cap.parseListSignal(&runOutput{Stdout: stdout}, rules)
		if err != nil {
			t.Fatalf("input %q: %v", tc.input, err)
		}
		if len(findings) != 1 {
			t.Errorf("input %q: expected 1 finding, got %d", tc.input, len(findings))
			continue
		}
		if findings[0].RuleID != tc.want {
			t.Errorf("input %q: RuleID = %q, want %q", tc.input, findings[0].RuleID, tc.want)
		}
	}
}

func TestTransformRuleID_PrefixBeforeColon(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "revive single-word rule", raw: "exported: should have a comment", want: "exported"},
		{name: "revive multi-word rule", raw: "package-comments: should have a package comment", want: "package-comments"},
		{name: "no colon — passthrough", raw: "no rule prefix here", want: "no rule prefix here"},
		{name: "leading whitespace stripped", raw: " exported : msg", want: "exported"},
		{name: "empty input — passthrough", raw: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := transformRuleID(tt.raw, "prefix_before_colon")
			if got != tt.want {
				t.Errorf("transformRuleID(%q, prefix_before_colon) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
