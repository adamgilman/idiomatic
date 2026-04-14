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

	"github.com/adamgilman/idiomatic/manifest"
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
		"semgrep", "gosec", "gitleaks", "golangci-lint", "eslint",
		"git", "file-exists", "file-contains",
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

	if c := got["golangci-lint"]; c != nil {
		if c.spec.Spec.Signal.MatchRuleBy.Strategy != "linter_contains" {
			t.Errorf("golangci-lint match_rule_by.strategy = %q", c.spec.Spec.Signal.MatchRuleBy.Strategy)
		}
		if !c.spec.Spec.Run.FilesAsDirs {
			t.Error("golangci-lint should set files_as_dirs")
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
		{"golangci-lint", map[string]any{"linter": "govet"}},
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

// TestRepoGolangciLintRendersConfigFile is the smoke test for the most
// complex bundled template. It uses sprig + the custom toYaml helper to
// merge per-linter settings into the rendered config. If toYaml is missing
// from the template func map, parsing fails before render and this test
// catches it.
func TestRepoGolangciLintRendersConfigFile(t *testing.T) {
	caps, err := LoadPath(repoCapabilitiesDir(t))
	if err != nil {
		t.Fatalf("LoadPath: %v", err)
	}
	var gcl *Capability
	for _, c := range caps {
		if c.Name() == "golangci-lint" {
			gcl = c
			break
		}
	}
	if gcl == nil {
		t.Fatal("golangci-lint capability not found")
	}

	rules := []manifest.Rule{
		{
			ID:       "go-bare-return",
			Severity: manifest.SeverityWarning,
			Detector: manifest.Detector{
				Capability: "golangci-lint",
				Config:     map[string]any{"linter": "nakedret"},
			},
		},
		{
			ID:       "go-no-elseif",
			Severity: manifest.SeverityWarning,
			Detector: manifest.Detector{
				Capability: "golangci-lint",
				Config: map[string]any{
					"linter": "gocritic",
					"rule":   "elseif",
					"settings": map[string]any{
						"disabled-checks": []any{"elseif"},
					},
				},
			},
		},
	}

	path, err := gcl.renderConfigFile(rules, &runtimeCtx{ProjectRoot: "/tmp", Discovered: map[string]string{}})
	if err != nil {
		t.Fatalf("renderConfigFile (toYaml support broken?): %v", err)
	}
	defer func() { _ = os.Remove(path) }()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "nakedret") {
		t.Errorf("rendered config missing nakedret:\n%s", got)
	}
	if !strings.Contains(got, "gocritic") {
		t.Errorf("rendered config missing gocritic:\n%s", got)
	}
	if !strings.Contains(got, "disabled-checks") {
		t.Errorf("rendered config missing disabled-checks setting:\n%s", got)
	}
}
