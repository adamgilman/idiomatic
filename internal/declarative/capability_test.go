// SPDX-License-Identifier: Apache-2.0

// capability_test.go — Tests for the Capability type: name/version accessors,
// input validation delegation, binary detection with install hints, and an
// end-to-end Analyze test using a fake semgrep binary (testdata/fake-semgrep).
package declarative

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adamgilman/idiomatic/internal/engine"
	"github.com/adamgilman/idiomatic/manifest"
)

// TestCapability_Name confirms the Capability satisfies engine.Capability and
// reports the spec name.
func TestCapability_Name(t *testing.T) {
	cap, err := NewFromBytes(semgrepYAML)
	if err != nil {
		t.Fatalf("NewFromBytes: %v", err)
	}
	if cap.Name() != "semgrep" {
		t.Errorf("Name() = %q", cap.Name())
	}
	if cap.Version() != "1.0.0" {
		t.Errorf("Version() = %q", cap.Version())
	}
}

// TestCapability_ValidateConfig delegates to validateInputs and surfaces
// schema errors per-rule.
func TestCapability_ValidateConfig(t *testing.T) {
	cap, _ := NewFromBytes(semgrepYAML)
	if err := cap.ValidateConfig(map[string]any{"language": "go", "pattern": "panic(...)"}); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}
	if err := cap.ValidateConfig(map[string]any{"pattern": "panic(...)"}); err == nil {
		t.Error("missing language should fail")
	}
}

// TestCapability_Detect_NotFound forces an empty PATH so the binary lookup
// fails and the install hint is surfaced.
func TestCapability_Detect_NotFound(t *testing.T) {
	cap, _ := NewFromBytes(semgrepYAML)
	t.Setenv("PATH", t.TempDir())
	_, err := cap.Detect()
	if err == nil {
		t.Fatal("expected error when binary not on PATH")
	}
	if !strings.Contains(err.Error(), "uv tool install semgrep") {
		t.Errorf("error should include install hint: %v", err)
	}
}

// TestCapability_Analyze_E2E_Semgrep is the end-to-end proof that the
// declarative pipeline works against a fake semgrep binary. The fake script
// emits canned JSON; we assert the resulting findings match expectations.
func TestCapability_Analyze_E2E_Semgrep(t *testing.T) {
	// Make the fake-semgrep script discoverable as "semgrep" on PATH.
	tmpBinDir := t.TempDir()
	fakePath, err := filepath.Abs("testdata/fake-semgrep")
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	if _, err := os.Stat(fakePath); err != nil {
		t.Fatalf("fake-semgrep missing: %v", err)
	}
	link := filepath.Join(tmpBinDir, "semgrep")
	if err := os.Symlink(fakePath, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	t.Setenv("PATH", tmpBinDir)

	cap, err := NewFromBytes(semgrepYAML)
	if err != nil {
		t.Fatalf("NewFromBytes: %v", err)
	}

	rules := []manifest.Rule{
		{
			ID:       "go-no-panic",
			Severity: manifest.SeverityError,
			Fix:      manifest.Fix{Message: "Don't panic"},
			Detector: manifest.Detector{
				Config: map[string]any{
					"language": "go",
					"pattern":  "panic(...)",
				},
			},
		},
	}

	findings, err := cap.Analyze(context.Background(), engine.AnalysisRequest{
		Files: []string{"main.go"}, // fake script ignores args
		Rules: rules,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d: %+v", len(findings), findings)
	}
	f := findings[0]
	if f.RuleID != "go-no-panic" {
		t.Errorf("RuleID = %q", f.RuleID)
	}
	if f.File != "main.go" {
		t.Errorf("File = %q", f.File)
	}
	if f.StartLine != 5 {
		t.Errorf("StartLine = %d", f.StartLine)
	}
	if f.Severity != "error" {
		t.Errorf("Severity = %q", f.Severity)
	}
	if f.Message != "panic detected" {
		t.Errorf("Message = %q", f.Message)
	}
	if f.FixMessage != "Don't panic" {
		t.Errorf("FixMessage = %q", f.FixMessage)
	}
}

// TestCapability_Analyze_ScalarRequiresFireWhen confirms that scalar-mode
// capabilities require each rule to provide a fire_when expression. This
// is the contract that lets a single capability serve many rules.
func TestCapability_Analyze_ScalarRequiresFireWhen(t *testing.T) {
	cap, err := NewFromBytes([]byte(gitYAML))
	if err != nil {
		t.Fatalf("NewFromBytes(git): %v", err)
	}
	// Force the scalar branch to actually run by giving it a discoverable
	// binary (we use 'sh' which exists everywhere) and a rule with no
	// fire_when. The rule supplies argv: [-c, true] so the inner exec succeeds.
	_, err = cap.Analyze(context.Background(), engine.AnalysisRequest{
		Files: []string{"x"},
		Rules: []manifest.Rule{{
			ID: "no-fire-when",
			Detector: manifest.Detector{Config: map[string]any{
				"argv": []any{"--version"},
			}},
		}},
	})
	if err == nil {
		t.Fatal("expected error about missing fire_when")
	}
	if !strings.Contains(err.Error(), "fire_when") {
		t.Errorf("error should mention 'fire_when': %v", err)
	}
}

// Confirm the package's exported Capability type satisfies engine.Capability.
var _ engine.Capability = (*Capability)(nil)

// Avoid an "imported and not used" if errors lands unused on a refactor.
var _ = errors.New
