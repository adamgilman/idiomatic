// SPDX-License-Identifier: Apache-2.0

// builtin_test.go — Smoke tests that load every YAML in the repo's canonical
// capabilities/ directory, confirming they parse, pass structural validation,
// and accept minimal rule inputs. This is the schema regression guard for the
// official capability set.
package declarative

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoCapabilitiesDir returns the absolute path to the top-level
// `capabilities/` directory in this repository, regardless of where `go test`
// is invoked from. Tests use this to load the canonical YAML set the same
// way the CLI does at runtime — there are no embedded copies.
func repoCapabilitiesDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile = ".../internal/declarative/builtin_test.go"
	// repo root is two levels up.
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	dir := filepath.Join(repoRoot, "capabilities")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("repo capabilities dir not found at %s: %v", dir, err)
	}
	return dir
}

// TestAllRepoCapabilitiesLoad confirms every YAML in the repo's
// `capabilities/` directory parses through LoadPath and produces a
// structurally valid Capability. This is the smoke test that protects the
// canonical set against schema regressions.
func TestAllRepoCapabilitiesLoad(t *testing.T) {
	caps, err := LoadPath(repoCapabilitiesDir(t))
	if err != nil {
		t.Fatalf("LoadPath: %v", err)
	}
	if len(caps) == 0 {
		t.Fatal("expected at least one capability")
	}

	got := make(map[string]*Capability, len(caps))
	for _, c := range caps {
		got[c.Name()] = c
	}

	expected := []string{
		"semgrep", "gosec", "gitleaks", "eslint",
		"git", "file-exists", "file-contains",
		"revive", "gocritic",
		"errcheck", "nakedret", "interfacebloat", "contextcheck",
		"errorlint", "recvcheck", "testpackage", "godot", "nolintlint",
		"tparallel", "gocognit", "cyclop", "nestif", "funlen", "dupl",
	}
	for _, name := range expected {
		if _, ok := got[name]; !ok {
			t.Errorf("missing capability %q", name)
		}
	}

	// Spot-check the new capabilities.

	if c := got["gosec"]; c != nil {
		if c.spec.Spec.Run.Per != "batch" {
			t.Errorf("gosec.run.per = %q", c.spec.Spec.Run.Per)
		}
		if !c.AppliesToFile("main.go") {
			t.Error("gosec should apply to .go files")
		}
		if c.AppliesToFile("main.ts") {
			t.Error("gosec should not apply to .ts files")
		}
	}

	if c := got["gitleaks"]; c != nil {
		if c.spec.Spec.Run.Per != "file" {
			t.Errorf("gitleaks.run.per = %q", c.spec.Spec.Run.Per)
		}
		if c.spec.Spec.Run.Output != "report_file" {
			t.Errorf("gitleaks.run.output = %q", c.spec.Spec.Run.Output)
		}
		if !c.AppliesToFile("anything.txt") {
			t.Error("gitleaks should apply to every file")
		}
	}

	// Per-linter golangci-lint-backed capabilities. Each must:
	// - Declare requires.binary == "golangci-lint"
	// - Set files_as_dirs (golangci-lint expects directories or "./...")
	// - Pass --path-mode=abs in argv (so paths are absolute, not relative to the tmp config dir)
	// - Use either by_input (multi-rule) or by_capability (single-rule) resolver strategy
	perLinterCaps := []string{
		"revive", "gocritic",
		"errcheck", "nakedret", "interfacebloat", "contextcheck",
		"errorlint", "recvcheck", "testpackage", "godot", "nolintlint",
		"tparallel", "gocognit", "cyclop", "nestif", "funlen", "dupl",
	}
	for _, name := range perLinterCaps {
		c := got[name]
		if c == nil {
			t.Errorf("missing per-linter capability %q", name)
			continue
		}
		if c.spec.Spec.Requires.Binary != "golangci-lint" {
			t.Errorf("%s.requires.binary = %q, want golangci-lint", name, c.spec.Spec.Requires.Binary)
		}
		if !c.spec.Spec.Run.FilesAsDirs {
			t.Errorf("%s should set files_as_dirs", name)
		}
		hasPathMode := false
		for _, arg := range c.spec.Spec.Run.Argv {
			if arg == "--path-mode=abs" {
				hasPathMode = true
				break
			}
		}
		if !hasPathMode {
			t.Errorf("%s argv missing --path-mode=abs", name)
		}
		switch c.spec.Spec.Signal.MatchRuleBy.Strategy {
		case "by_input", "by_capability":
			// ok
		default:
			t.Errorf("%s match_rule_by.strategy = %q, want by_input or by_capability", name, c.spec.Spec.Signal.MatchRuleBy.Strategy)
		}
	}

	if c := got["git"]; c != nil {
		if c.spec.Spec.Signal.Shape != "scalar" {
			t.Errorf("git.signal.shape = %q", c.spec.Spec.Signal.Shape)
		}
		if c.spec.Spec.Run.Per != "rule" {
			t.Errorf("git.run.per = %q", c.spec.Spec.Run.Per)
		}
		if c.spec.Spec.Run.RuleArgvField != "argv" {
			t.Errorf("git.run.rule_argv_field = %q", c.spec.Spec.Run.RuleArgvField)
		}
	}

	if c := got["file-exists"]; c != nil {
		if c.spec.Spec.Signal.Shape != "scalar" {
			t.Errorf("file-exists.signal.shape = %q", c.spec.Spec.Signal.Shape)
		}
		if !strings.Contains(c.spec.Spec.Signal.FixedFile, "{project}") {
			t.Errorf("file-exists.fixed_file = %q (expected {project} placeholder)", c.spec.Spec.Signal.FixedFile)
		}
	}

	if c := got["file-contains"]; c != nil {
		if c.spec.Spec.Run.Per != "rule" {
			t.Errorf("file-contains.run.per = %q", c.spec.Spec.Run.Per)
		}
	}
}

// TestAllRepoCapabilitiesValidateRule confirms each capability's input
// schema accepts a minimal rule of the right shape. This catches schema/typo
// bugs in the inputs declarations.
func TestAllRepoCapabilitiesValidateRule(t *testing.T) {
	caps, err := LoadPath(repoCapabilitiesDir(t))
	if err != nil {
		t.Fatalf("LoadPath: %v", err)
	}
	byName := make(map[string]*Capability, len(caps))
	for _, c := range caps {
		byName[c.Name()] = c
	}

	cases := []struct {
		cap    string
		inputs map[string]any
	}{
		{"semgrep", map[string]any{"language": "go", "pattern": "panic(...)"}},
		{"gosec", map[string]any{"rule_id": "G101"}},
		{"gitleaks", map[string]any{"rule_id": "aws-access-token"}},
		{"git", map[string]any{"argv": []any{"rev-parse", "HEAD"}, "fire_when": "signal.exit_code != 0"}},
		{"file-exists", map[string]any{"path": ".gitignore", "fire_when": "signal.exit_code != 0"}},
		{"file-contains", map[string]any{"path": ".gitignore", "pattern": "\\.env", "fire_when": "signal.exit_code != 0"}},
	}

	for _, tc := range cases {
		t.Run(tc.cap, func(t *testing.T) {
			c, ok := byName[tc.cap]
			if !ok {
				t.Skipf("capability %q not loaded", tc.cap)
			}
			if err := c.ValidateConfig(tc.inputs); err != nil {
				t.Errorf("ValidateConfig: %v", err)
			}
		})
	}
}

