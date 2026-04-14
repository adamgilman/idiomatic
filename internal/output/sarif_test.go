// SPDX-License-Identifier: Apache-2.0

// sarif_test.go — Tests for SARIF 2.1.0 output and the HumanFormatter.
//
//   - testRules and testFindings are shared fixtures; tests verify structural correctness
//     (valid JSON, sorted rules, cross-referenced ruleIndex/artifactIndex) not exact byte output.
//   - TestHuman_BackwardCompat pins the human format string to prevent accidental regressions
//     in editor integration.
//   - Severity mapping and execution-error embedding are tested as isolated concerns.
package output

import (
	"encoding/json"
	"testing"

	"github.com/adamgilman/idiomatic/internal/engine"
	"github.com/adamgilman/idiomatic/manifest"
)

var testRules = []manifest.Rule{
	{
		ID:          "go-no-panic-outside-main-and-init",
		Name:        "No panic() outside main and init",
		Description: "Flags panic in non-exempt functions.",
		Rationale:   "Use error returns.",
		Severity:    manifest.SeverityError,
		Tags:        []string{"errors", "go"},
		References:  []string{"https://example.com/panic"},
		Detector:    manifest.Detector{Capability: "semgrep", Config: map[string]any{"language": "go", "pattern": "panic(...)"}},
		AppliesTo:   []string{"**/*.go"},
		Fix:         manifest.Fix{Message: "Return an error.", Example: "return fmt.Errorf(...)"},
	},
	{
		ID:          "go-tdd-no-fmt-println",
		Name:        "No fmt.Println outside main",
		Description: "Flags fmt.Println in non-main packages.",
		Rationale:   "Use a structured logger.",
		Severity:    manifest.SeverityError,
		Tags:        []string{"logging"},
		Detector:    manifest.Detector{Capability: "semgrep", Config: map[string]any{"language": "go", "pattern": "fmt.Println(...)"}},
		AppliesTo:   []string{"**/*.go"},
		Fix:         manifest.Fix{Message: "Replace with logger."},
	},
}

var testFindings = []engine.Finding{
	{
		RuleID: "go-tdd-no-fmt-println", Severity: "error",
		File: "testdata/bad/bad.go", StartLine: 6, StartCol: 2, EndLine: 6, EndCol: 50,
		Message: "fmt.Println not allowed", FixMessage: "Replace with logger.", FixExample: "",
	},
	{
		RuleID: "go-no-panic-outside-main-and-init", Severity: "error",
		File: "testdata/panic_bad/bad.go", StartLine: 4, StartCol: 2, EndLine: 4, EndCol: 40,
		Message: "panic not allowed", FixMessage: "Return an error.", FixExample: "return fmt.Errorf(...)",
	},
}

var testFiles = []string{
	"testdata/bad/bad.go",
	"testdata/good/good.go",
	"testdata/panic_bad/bad.go",
}

var testMeta = InvocationMetadata{
	ToolName:    "idio",
	ToolVersion: "dev",
	WorkingDir:  "/home/user/project",
}

func TestSARIF_ValidJSON(t *testing.T) {
	f := &SARIFFormatter{}
	data, err := f.Format(testFindings, testRules, testFiles, testMeta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var log SARIFLog
	if err := json.Unmarshal(data, &log); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

func TestSARIF_Version(t *testing.T) {
	f := &SARIFFormatter{}
	data, _ := f.Format(testFindings, testRules, testFiles, testMeta)

	var log SARIFLog
	json.Unmarshal(data, &log)

	if log.Version != "2.1.0" {
		t.Errorf("version = %q, want %q", log.Version, "2.1.0")
	}
	if log.Schema != "https://json.schemastore.org/sarif-2.1.0.json" {
		t.Errorf("$schema = %q", log.Schema)
	}
}

func TestSARIF_SingleRun(t *testing.T) {
	f := &SARIFFormatter{}
	data, _ := f.Format(testFindings, testRules, testFiles, testMeta)

	var log SARIFLog
	json.Unmarshal(data, &log)

	if len(log.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(log.Runs))
	}
}

func TestSARIF_RulesSortedByID(t *testing.T) {
	f := &SARIFFormatter{}
	data, _ := f.Format(testFindings, testRules, testFiles, testMeta)

	var log SARIFLog
	json.Unmarshal(data, &log)

	rules := log.Runs[0].Tool.Driver.Rules
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	// Should be sorted by ID.
	if rules[0].ID != "go-no-panic-outside-main-and-init" {
		t.Errorf("rules[0].id = %q, want go-no-panic-outside-main-and-init", rules[0].ID)
	}
	if rules[1].ID != "go-tdd-no-fmt-println" {
		t.Errorf("rules[1].id = %q, want go-tdd-no-fmt-println", rules[1].ID)
	}
}

func TestSARIF_RuleLevelMapping(t *testing.T) {
	f := &SARIFFormatter{}
	data, _ := f.Format(testFindings, testRules, testFiles, testMeta)

	var log SARIFLog
	json.Unmarshal(data, &log)

	for _, rule := range log.Runs[0].Tool.Driver.Rules {
		if rule.DefaultConfiguration.Level != "error" {
			t.Errorf("rule %s level = %q, want error", rule.ID, rule.DefaultConfiguration.Level)
		}
	}
}

func TestSARIF_RuleProperties(t *testing.T) {
	f := &SARIFFormatter{}
	data, _ := f.Format(testFindings, testRules, testFiles, testMeta)

	var log SARIFLog
	json.Unmarshal(data, &log)

	// First rule (panic) has tags and references.
	panicRule := log.Runs[0].Tool.Driver.Rules[0]
	if panicRule.Properties == nil {
		t.Fatal("expected properties on panic rule")
	}
	if len(panicRule.Properties.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(panicRule.Properties.Tags))
	}
	if len(panicRule.Properties.References) != 1 {
		t.Errorf("expected 1 reference, got %d", len(panicRule.Properties.References))
	}
}

func TestSARIF_Results(t *testing.T) {
	f := &SARIFFormatter{}
	data, _ := f.Format(testFindings, testRules, testFiles, testMeta)

	var log SARIFLog
	json.Unmarshal(data, &log)

	results := log.Runs[0].Results
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// First result.
	if results[0].RuleID != "go-tdd-no-fmt-println" {
		t.Errorf("results[0].ruleId = %q", results[0].RuleID)
	}
	if results[0].Level != "error" {
		t.Errorf("results[0].level = %q", results[0].Level)
	}
	loc := results[0].Locations[0].PhysicalLocation
	if loc.Region.StartLine != 6 {
		t.Errorf("results[0] startLine = %d, want 6", loc.Region.StartLine)
	}

	// Second result.
	if results[1].RuleID != "go-no-panic-outside-main-and-init" {
		t.Errorf("results[1].ruleId = %q", results[1].RuleID)
	}
}

func TestSARIF_ResultRuleIndex(t *testing.T) {
	f := &SARIFFormatter{}
	data, _ := f.Format(testFindings, testRules, testFiles, testMeta)

	var log SARIFLog
	json.Unmarshal(data, &log)

	rules := log.Runs[0].Tool.Driver.Rules
	for _, r := range log.Runs[0].Results {
		if r.RuleIndex < 0 || r.RuleIndex >= len(rules) {
			t.Errorf("result ruleIndex %d out of range", r.RuleIndex)
			continue
		}
		if rules[r.RuleIndex].ID != r.RuleID {
			t.Errorf("ruleIndex %d points to %q, but ruleId is %q",
				r.RuleIndex, rules[r.RuleIndex].ID, r.RuleID)
		}
	}
}

func TestSARIF_ResultArtifactIndex(t *testing.T) {
	f := &SARIFFormatter{}
	data, _ := f.Format(testFindings, testRules, testFiles, testMeta)

	var log SARIFLog
	json.Unmarshal(data, &log)

	artifacts := log.Runs[0].Artifacts
	for _, r := range log.Runs[0].Results {
		loc := r.Locations[0].PhysicalLocation.ArtifactLocation
		if loc.Index == nil {
			t.Errorf("result %s has nil artifact index", r.RuleID)
			continue
		}
		if *loc.Index < 0 || *loc.Index >= len(artifacts) {
			t.Errorf("result %s artifact index %d out of range", r.RuleID, *loc.Index)
		}
	}
}

func TestSARIF_Artifacts(t *testing.T) {
	f := &SARIFFormatter{}
	data, _ := f.Format(testFindings, testRules, testFiles, testMeta)

	var log SARIFLog
	json.Unmarshal(data, &log)

	// Should contain all analyzed files, not just those with findings.
	if len(log.Runs[0].Artifacts) != 3 {
		t.Errorf("expected 3 artifacts, got %d", len(log.Runs[0].Artifacts))
	}
}

func TestSARIF_Fixes(t *testing.T) {
	f := &SARIFFormatter{}
	data, _ := f.Format(testFindings, testRules, testFiles, testMeta)

	var log SARIFLog
	json.Unmarshal(data, &log)

	for _, r := range log.Runs[0].Results {
		if len(r.Fixes) == 0 {
			t.Errorf("result %s has no fixes", r.RuleID)
		}
	}

	// The panic result should include the example.
	panicResult := log.Runs[0].Results[1]
	if len(panicResult.Fixes) > 0 {
		fixText := panicResult.Fixes[0].Description.Text
		if fixText == "" {
			t.Error("panic result fix description is empty")
		}
	}
}

func TestSARIF_EmptyFindings(t *testing.T) {
	f := &SARIFFormatter{}
	data, err := f.Format(nil, testRules, testFiles, testMeta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected SARIF output even with no findings")
	}

	var log SARIFLog
	if err := json.Unmarshal(data, &log); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(log.Runs[0].Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(log.Runs[0].Results))
	}
	// Rules should still be present.
	if len(log.Runs[0].Tool.Driver.Rules) != 2 {
		t.Errorf("expected 2 rules even with no findings, got %d", len(log.Runs[0].Tool.Driver.Rules))
	}
}

func TestSARIF_ExecutionError(t *testing.T) {
	errMeta := testMeta
	errMeta.ExecError = "backend failed: parse error"

	f := &SARIFFormatter{}
	data, err := f.Format(nil, nil, nil, errMeta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var log SARIFLog
	json.Unmarshal(data, &log)

	inv := log.Runs[0].Invocations[0]
	if inv.ExecutionSuccessful {
		t.Error("expected executionSuccessful=false")
	}
	if len(inv.ToolExecutionNotifications) == 0 {
		t.Error("expected toolExecutionNotifications")
	}
}

func TestSARIF_Invocation(t *testing.T) {
	f := &SARIFFormatter{}
	data, _ := f.Format(testFindings, testRules, testFiles, testMeta)

	var log SARIFLog
	json.Unmarshal(data, &log)

	inv := log.Runs[0].Invocations[0]
	if !inv.ExecutionSuccessful {
		t.Error("expected executionSuccessful=true")
	}
	if inv.EndTimeUTC == "" {
		t.Error("expected endTimeUtc to be set")
	}
	if inv.WorkingDirectory == nil {
		t.Error("expected workingDirectory to be set")
	}
}

func TestHuman_BackwardCompat(t *testing.T) {
	f := &HumanFormatter{}
	data, err := f.Format(testFindings, testRules, testFiles, testMeta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "testdata/bad/bad.go:6:2: error: fmt.Println not allowed [go-tdd-no-fmt-println]\n" +
		"testdata/panic_bad/bad.go:4:2: error: panic not allowed [go-no-panic-outside-main-and-init]\n"
	if string(data) != expected {
		t.Errorf("human output mismatch:\ngot:  %q\nwant: %q", string(data), expected)
	}
}

func TestHuman_EmptyFindings(t *testing.T) {
	f := &HumanFormatter{}
	data, err := f.Format(nil, testRules, testFiles, testMeta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty output for no findings, got %q", string(data))
	}
}

func TestSeverityMapping(t *testing.T) {
	tests := []struct {
		severity string
		level    string
	}{
		{"error", "error"},
		{"warning", "warning"},
		{"info", "note"},
		{"unknown", "warning"},
	}
	for _, tt := range tests {
		got := mapSeverityToLevel(tt.severity)
		if got != tt.level {
			t.Errorf("mapSeverityToLevel(%q) = %q, want %q", tt.severity, got, tt.level)
		}
	}
}
