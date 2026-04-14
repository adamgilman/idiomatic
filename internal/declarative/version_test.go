// SPDX-License-Identifier: Apache-2.0

// version_test.go — Tests for semver constraint parsing, matching (caret,
// tilde, range, exact), empty-constraint passthrough, invalid-constraint
// errors, and the Registry.CheckRuleVersions integration path.
package declarative

import (
	"errors"
	"strings"
	"testing"
)

func TestParseConstraint_Empty(t *testing.T) {
	vc, err := ParseConstraint("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !vc.IsEmpty() {
		t.Error("empty string should produce IsEmpty constraint")
	}
	got, err := vc.Matches("1.0.0")
	if err != nil || !got {
		t.Errorf("empty constraint should match anything; got=%v err=%v", got, err)
	}
	got, err = vc.Matches("99.0.0-rc1+build.7")
	if err != nil || !got {
		t.Errorf("empty constraint should match prerelease; got=%v err=%v", got, err)
	}
}

func TestParseConstraint_Invalid(t *testing.T) {
	_, err := ParseConstraint("not a constraint")
	if err == nil {
		t.Fatal("expected error for invalid constraint")
	}
	if !strings.Contains(err.Error(), "invalid version constraint") {
		t.Errorf("error should mention 'invalid version constraint': %v", err)
	}
}

func TestParseConstraint_Matches(t *testing.T) {
	cases := []struct {
		name       string
		constraint string
		version    string
		want       bool
	}{
		{"caret major matches patch", "^1.0.0", "1.2.3", true},
		{"caret major rejects next major", "^1.0.0", "2.0.0", false},
		{"tilde matches patch", "~1.2.0", "1.2.99", true},
		{"tilde rejects next minor", "~1.2.0", "1.3.0", false},
		{"explicit range lower bound", ">=1.2.0, <2.0.0", "1.2.0", true},
		{"explicit range upper exclusive", ">=1.2.0, <2.0.0", "2.0.0", false},
		{"exact match", "1.0.0", "1.0.0", true},
		{"exact rejects different patch", "1.0.0", "1.0.1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vc, err := ParseConstraint(tc.constraint)
			if err != nil {
				t.Fatalf("ParseConstraint(%q): %v", tc.constraint, err)
			}
			got, err := vc.Matches(tc.version)
			if err != nil {
				t.Fatalf("Matches(%q): %v", tc.version, err)
			}
			if got != tc.want {
				t.Errorf("constraint=%q version=%q: got %v, want %v", tc.constraint, tc.version, got, tc.want)
			}
		})
	}
}

func TestCheckRuleVersions_Match(t *testing.T) {
	r := NewRegistry()
	if err := r.Add(&Capability{spec: CapabilitySpec{
		APIVersion: CapabilityAPIVersion,
		Kind:       CapabilityKind,
		Metadata:   CapabilityMetadata{Name: "semgrep", Version: "1.4.2"},
		Spec: CapabilitySpecBody{
			Requires: RequiresSpec{Binary: "semgrep"},
			Run:      RunSpec{Argv: []string{"x"}},
			Signal:   SignalSpec{Shape: "list", MatchRuleBy: &MatchRuleBy{From: "id"}},
		},
	}}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name      string
		req       RuleVersionRequest
		wantError bool
	}{
		{
			name: "no constraint",
			req:  RuleVersionRequest{RuleID: "r1", Capability: "semgrep", Constraint: ""},
		},
		{
			name: "caret major matches",
			req:  RuleVersionRequest{RuleID: "r2", Capability: "semgrep", Constraint: "^1"},
		},
		{
			name:      "caret major mismatch",
			req:       RuleVersionRequest{RuleID: "r3", Capability: "semgrep", Constraint: "^2"},
			wantError: true,
		},
		{
			name: "exact match",
			req:  RuleVersionRequest{RuleID: "r4", Capability: "semgrep", Constraint: "1.4.2"},
		},
		{
			name:      "exact mismatch",
			req:       RuleVersionRequest{RuleID: "r5", Capability: "semgrep", Constraint: "1.5.0"},
			wantError: true,
		},
		{
			name: "missing capability is ignored",
			req:  RuleVersionRequest{RuleID: "r6", Capability: "ghost", Constraint: "^1"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := r.CheckRuleVersions([]RuleVersionRequest{tc.req})
			if tc.wantError {
				if err == nil {
					t.Fatal("expected mismatch error")
				}
				var vm VersionMismatch
				if !errors.As(err, &vm) {
					t.Errorf("expected VersionMismatch, got %T: %v", err, err)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestCheckRuleVersions_BadConstraintError(t *testing.T) {
	r := NewRegistry()
	if err := r.Add(&Capability{spec: CapabilitySpec{
		APIVersion: CapabilityAPIVersion,
		Kind:       CapabilityKind,
		Metadata:   CapabilityMetadata{Name: "semgrep", Version: "1.0.0"},
		Spec: CapabilitySpecBody{
			Requires: RequiresSpec{Binary: "semgrep"},
			Run:      RunSpec{Argv: []string{"x"}},
			Signal:   SignalSpec{Shape: "list", MatchRuleBy: &MatchRuleBy{From: "id"}},
		},
	}}); err != nil {
		t.Fatal(err)
	}

	err := r.CheckRuleVersions([]RuleVersionRequest{{
		RuleID: "r1", Capability: "semgrep", Constraint: "not a real constraint",
	}})
	if err == nil {
		t.Fatal("expected error for malformed constraint")
	}
	if !strings.Contains(err.Error(), "invalid version constraint") {
		t.Errorf("error should mention 'invalid version constraint': %v", err)
	}
}
