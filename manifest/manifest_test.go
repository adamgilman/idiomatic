// SPDX-License-Identifier: Apache-2.0

// manifest_test.go — Tests for YAML loading, validation, and error reporting.
//
//   - Test fixtures live in testdata/; malformed.yaml is generated at runtime by TestMain.
//   - Tests cover both valid and invalid manifests to exercise the batch error collection pattern.
//   - TestMain creates the testdata directory and the malformed YAML fixture before any test runs.

package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFile_ValidManifest(t *testing.T) {
	result, err := LoadFile("testdata/valid.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected validation errors:\n%s", result.Errors)
	}

	m := result.Manifests["testdata/valid.yaml"]
	if m == nil {
		t.Fatal("manifest not found in result")
	}

	// Check top-level.
	if m.APIVersion != APIVersionV1Alpha1 {
		t.Errorf("apiVersion = %q, want %q", m.APIVersion, APIVersionV1Alpha1)
	}
	if m.Kind != KindRulePack {
		t.Errorf("kind = %q, want %q", m.Kind, KindRulePack)
	}

	// Check pack.
	if m.Pack.ID != "go-tdd-starter" {
		t.Errorf("pack.id = %q, want %q", m.Pack.ID, "go-tdd-starter")
	}
	if m.Pack.Version != "0.1.0" {
		t.Errorf("pack.version = %q, want %q", m.Pack.Version, "0.1.0")
	}
	if m.Pack.License != "Proprietary" {
		t.Errorf("pack.license = %q, want %q", m.Pack.License, "Proprietary")
	}

	// Check rules.
	if len(m.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(m.Rules))
	}

	r := m.Rules[0]
	if r.ID != "go-tdd-test-file-required" {
		t.Errorf("rule.id = %q", r.ID)
	}
	if r.Severity != SeverityError {
		t.Errorf("rule.severity = %q, want %q", r.Severity, SeverityError)
	}
	if r.Detector.Capability != "semgrep" {
		t.Errorf("detector.capability = %q", r.Detector.Capability)
	}
	if r.Detector.Config["language"] != "go" {
		t.Errorf("detector.config.language = %v", r.Detector.Config["language"])
	}
	if r.Detector.Config["pattern"] != "package $NAME" {
		t.Errorf("detector.config.pattern = %v", r.Detector.Config["pattern"])
	}
	if len(r.AppliesTo) != 1 || r.AppliesTo[0] != "**/*.go" {
		t.Errorf("applies_to = %v", r.AppliesTo)
	}
	if len(r.Excludes) != 3 {
		t.Errorf("excludes len = %d, want 3", len(r.Excludes))
	}
	if len(r.Tests) != 2 {
		t.Errorf("tests len = %d, want 2", len(r.Tests))
	}
	if len(r.References) != 2 {
		t.Errorf("references len = %d, want 2", len(r.References))
	}

	// Check rule index.
	if _, ok := result.Rules["go-tdd-test-file-required"]; !ok {
		t.Error("rule not found in index")
	}
}

func TestLoadFile_ValidCapabilityFormat(t *testing.T) {
	result, err := LoadFile("testdata/valid-capability.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected validation errors:\n%s", result.Errors)
	}

	m := result.Manifests["testdata/valid-capability.yaml"]
	if m == nil {
		t.Fatal("manifest not found in result")
	}

	r := m.Rules[0]
	if r.Detector.Capability != "semgrep" {
		t.Errorf("detector.capability = %q, want %q", r.Detector.Capability, "semgrep")
	}
	if r.Detector.Config["pattern"] != "require-test-file" {
		t.Errorf("detector.config.pattern = %v", r.Detector.Config["pattern"])
	}
}

func TestLoadFile_MultiRule(t *testing.T) {
	result, err := LoadFile("testdata/multi-rule.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected validation errors:\n%s", result.Errors)
	}

	if len(result.Rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(result.Rules))
	}

	for _, id := range []string{"rule-alpha", "rule-beta", "rule-gamma"} {
		if _, ok := result.Rules[id]; !ok {
			t.Errorf("rule %q not found in index", id)
		}
	}

	// alpha and beta have no inline tests.
	untestedCount := 0
	for _, w := range result.Warnings {
		if strings.Contains(w, "no inline tests") {
			untestedCount++
		}
	}
	if untestedCount != 2 {
		t.Errorf("expected 2 untested-rule warnings, got %d", untestedCount)
	}
}

func TestLoadFile_InvalidMissingFields(t *testing.T) {
	result, err := LoadFile("testdata/invalid-missing-fields.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation errors, got none")
	}

	// Should catch: pack.name, pack.version, pack.description, pack.maintainer,
	// rule.id, rule.name, rule.description, rule.rationale, rule.severity,
	// rule.detector (neither format), rule.applies_to, rule.fix.message
	expectedFields := []string{
		"pack.name", "pack.version", "pack.description", "pack.maintainer",
		"id", "name", "description", "rationale", "severity",
		"detector", "applies_to", "fix.message",
	}
	for _, field := range expectedFields {
		found := false
		for _, e := range result.Errors {
			if strings.Contains(e.Field, field) || strings.Contains(e.Message, field) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected error for field %q, not found in:\n%s", field, result.Errors)
		}
	}
}

func TestLoadFile_InvalidBadIDs(t *testing.T) {
	result, err := LoadFile("testdata/invalid-bad-ids.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation errors for bad IDs")
	}

	// Should have errors for both pack.id and rule.id.
	packIDError := false
	ruleIDError := false
	for _, e := range result.Errors {
		if strings.Contains(e.Field, "pack.id") {
			packIDError = true
		}
		if strings.Contains(e.Message, "UPPER_CASE_ID") {
			ruleIDError = true
		}
	}
	if !packIDError {
		t.Error("expected error for invalid pack.id")
	}
	if !ruleIDError {
		t.Error("expected error for invalid rule.id")
	}
}

func TestLoadFile_InvalidBadSeverity(t *testing.T) {
	result, err := LoadFile("testdata/invalid-bad-severity.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation errors for bad severity")
	}

	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "critical") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error mentioning invalid severity 'critical'")
	}
}

func TestLoadFile_InvalidDuplicateIDs(t *testing.T) {
	result, err := LoadFile("testdata/invalid-duplicate-ids.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation errors for duplicate IDs")
	}

	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "duplicate") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected duplicate id error, got:\n%s", result.Errors)
	}
}

func TestLoadFile_InvalidBadAPIVersion(t *testing.T) {
	result, err := LoadFile("testdata/invalid-bad-apiversion.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation errors for bad apiVersion")
	}

	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "v2beta1") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error mentioning unrecognized apiVersion")
	}
}

func TestLoadFile_InvalidBadTests(t *testing.T) {
	result, err := LoadFile("testdata/invalid-bad-tests.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation errors for bad tests")
	}

	// Should catch: test.name, test.should, test.code
	fields := []string{"name", "should", "code"}
	for _, field := range fields {
		found := false
		for _, e := range result.Errors {
			if strings.Contains(e.Field, "tests[0]."+field) || strings.Contains(e.Message, "test "+field) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected error for test field %q, got:\n%s", field, result.Errors)
		}
	}
}

func TestLoadFile_MalformedYAML(t *testing.T) {
	_, err := LoadFile("testdata/malformed.yaml")
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestLoadFile_NotFound(t *testing.T) {
	_, err := LoadFile("testdata/does-not-exist.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadDir(t *testing.T) {
	result, err := LoadDir("testdata")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should load multiple manifests.
	if len(result.Manifests) == 0 {
		t.Fatal("expected manifests to be loaded from directory")
	}

	// The valid and multi-rule manifests should contribute rules.
	// Invalid manifests should produce errors.
	if len(result.Errors) == 0 {
		t.Error("expected some validation errors from invalid test files")
	}
}

func TestLoad_File(t *testing.T) {
	result, err := Load("testdata/valid.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(result.Rules))
	}
}

func TestLoad_Dir(t *testing.T) {
	result, err := Load("testdata")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Manifests) == 0 {
		t.Error("expected manifests loaded from directory")
	}
}

func TestValidate_EmptyManifest(t *testing.T) {
	m := &Manifest{}
	errs := Validate(m, "test.yaml")
	if len(errs) == 0 {
		t.Fatal("expected errors for empty manifest")
	}
}

func TestValidationError_Format(t *testing.T) {
	e := ValidationError{
		File:       "rules/pack.yaml",
		RuleID:     "my-rule",
		Field:      "severity",
		Message:    "invalid severity",
		Suggestion: "use one of: error, warning, info",
	}
	got := e.Error()
	if !strings.Contains(got, "rules/pack.yaml") {
		t.Error("expected file path in error")
	}
	if !strings.Contains(got, "my-rule") {
		t.Error("expected rule ID in error")
	}
	if !strings.Contains(got, "severity") {
		t.Error("expected field in error")
	}
	if !strings.Contains(got, "invalid severity") {
		t.Error("expected message in error")
	}
	if !strings.Contains(got, "use one of") {
		t.Error("expected suggestion in error")
	}
}

func TestIsValidID(t *testing.T) {
	valid := []string{"a", "abc", "my-rule", "go-tdd-test-file-required", "a1", "1a", "a-b-c"}
	invalid := []string{"", "ABC", "my_rule", "My-Rule", "has space", "-leading", "trailing-"}

	for _, id := range valid {
		if !isValidID(id) {
			t.Errorf("isValidID(%q) = false, want true", id)
		}
	}
	for _, id := range invalid {
		if id == "" {
			continue // empty is caught by required check, not id format
		}
		if isValidID(id) {
			t.Errorf("isValidID(%q) = true, want false", id)
		}
	}
}

func TestSummary(t *testing.T) {
	result := &LoadResult{
		Manifests: map[string]*Manifest{"a.yaml": {}},
		Rules:     map[string]*Rule{"r1": {}, "r2": {}},
		Warnings:  []string{"warn1"},
	}
	s := result.Summary()
	if !strings.Contains(s, "1 manifest") {
		t.Errorf("summary missing manifest count: %s", s)
	}
	if !strings.Contains(s, "2 rule") {
		t.Errorf("summary missing rule count: %s", s)
	}
	if !strings.Contains(s, "1 warning") {
		t.Errorf("summary missing warning count: %s", s)
	}
}

func TestMain(m *testing.M) {
	// Create the malformed YAML test fixture.
	if err := os.MkdirAll("testdata", 0755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create testdata dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join("testdata", "malformed.yaml"), []byte("{{invalid yaml: ["), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write test fixture: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
