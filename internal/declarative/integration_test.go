// SPDX-License-Identifier: Apache-2.0

// integration_test.go — Integration tests for cross-file features: walk-up
// binary discovery, discover_files variable resolution, precheck enforcement,
// nested-list signal parsing (eslint two-level JSON shape), and a full
// end-to-end eslint Analyze using a fake binary with canned output.
package declarative

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adamgilman/idiomatic/internal/engine"

	"github.com/adamgilman/idiomatic/manifest"
)

// TestDiscoverBinary_WalkUpFromCwd verifies the platform feature that locates
// a binary by walking parent directories looking for a relative path. This
// is the cornerstone of the eslint port.
func TestDiscoverBinary_WalkUpFromCwd(t *testing.T) {
	// Build a fake project layout: tmp/proj/sub/ + tmp/proj/node_modules/.bin/myTool
	root := t.TempDir()
	binDir := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(binDir, "mytool")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(root, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Run the discovery from the sub directory.
	prevWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(prevWd) }()
	if err := os.Chdir(subDir); err != nil {
		t.Fatal(err)
	}

	cap := &Capability{spec: CapabilitySpec{
		Metadata: CapabilityMetadata{Name: "mytool", Version: "1.0.0"},
		Spec: CapabilitySpecBody{
			Requires: RequiresSpec{
				Binary: "mytool",
				Discover: &DiscoverBinarySpec{
					Strategy: "walk_up_from_cwd",
					Relative: "node_modules/.bin/mytool",
				},
			},
		},
	}}

	bin, projectRoot, err := cap.discoverBinary(subDir)
	if err != nil {
		t.Fatalf("discoverBinary: %v", err)
	}
	if bin != binPath {
		t.Errorf("bin = %q, want %q", bin, binPath)
	}
	if projectRoot != root {
		t.Errorf("projectRoot = %q, want %q", projectRoot, root)
	}
}

// TestDiscoverFiles_WalkUpAndOptional verifies discover_files exposes
// found files as runtime variables, and that optional missing files
// resolve to "" without erroring.
func TestDiscoverFiles_WalkUpAndOptional(t *testing.T) {
	root := t.TempDir()
	tsconfig := filepath.Join(root, "tsconfig.json")
	if err := os.WriteFile(tsconfig, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(root, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	prevWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(prevWd) }()
	if err := os.Chdir(subDir); err != nil {
		t.Fatal(err)
	}

	cap := &Capability{spec: CapabilitySpec{
		Metadata: CapabilityMetadata{Name: "mytool", Version: "1.0.0"},
		Spec: CapabilitySpecBody{
			Requires: RequiresSpec{
				Binary: "sh", // any binary on PATH so binary discovery succeeds
				DiscoverFiles: []DiscoverFileSpec{
					{Var: "tsconfig", Name: "tsconfig.json", WalkUpFrom: "cwd", Optional: true},
					{Var: "missing", Name: "absolutely-not-here.json", WalkUpFrom: "cwd", Optional: true},
				},
			},
		},
	}}

	rt, err := cap.resolveRuntime(engineFiles{})
	if err != nil {
		t.Fatalf("resolveRuntime: %v", err)
	}
	if rt.Discovered["tsconfig"] != tsconfig {
		t.Errorf("Discovered.tsconfig = %q, want %q", rt.Discovered["tsconfig"], tsconfig)
	}
	if rt.Discovered["missing"] != "" {
		t.Errorf("Discovered.missing = %q, want empty", rt.Discovered["missing"])
	}
}

// TestPrecheck_FileExistsForEachUniqueInput verifies the precheck loop
// over rule inputs aborts when a file is missing and passes when it exists.
func TestPrecheck_FileExistsForEachUniqueInput(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "node_modules", "good-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}

	cap := &Capability{spec: CapabilitySpec{
		Metadata: CapabilityMetadata{Name: "mytool", Version: "1.0.0"},
		Spec: CapabilitySpecBody{
			Requires: RequiresSpec{
				Binary: "sh",
				Precheck: []PrecheckSpec{{
					Kind:               "file_exists",
					Path:               "{project_root}/node_modules/{{ .Value }}",
					ForEachUniqueInput: "plugin",
					SkipValues:         []string{"baked-in-plugin"},
					ErrorMessage:       "plugin {{ .Value }} not installed",
				}},
			},
		},
	}}

	rules := []manifest.Rule{
		{ID: "r1", Detector: manifest.Detector{Capability: "mytool", Config: map[string]any{"plugin": "good-plugin"}}},
		{ID: "r2", Detector: manifest.Detector{Capability: "mytool", Config: map[string]any{"plugin": "baked-in-plugin"}}},
	}

	rt := &runtimeCtx{ProjectRoot: root, Discovered: map[string]string{}}

	// All declared plugins are present (or skipped).
	if err := cap.runPrechecks(rt, rules); err != nil {
		t.Errorf("unexpected precheck failure: %v", err)
	}

	// Add a rule referencing a missing plugin.
	rules = append(rules, manifest.Rule{
		ID:       "r3",
		Detector: manifest.Detector{Capability: "mytool", Config: map[string]any{"plugin": "missing-plugin"}},
	})
	err := cap.runPrechecks(rt, rules)
	if err == nil {
		t.Fatal("expected precheck failure for missing plugin")
	}
	if !strings.Contains(err.Error(), "missing-plugin not installed") {
		t.Errorf("error should mention 'missing-plugin not installed': %v", err)
	}
}

// TestNestedListSignal_EslintShape exercises the nested_list + parent_fields
// extraction against the actual ESLint JSON shape. This is the canonical
// test for the platform feature.
func TestNestedListSignal_EslintShape(t *testing.T) {
	caps, err := LoadPath(repoCapabilitiesDir(t))
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	var es *Capability
	for _, c := range caps {
		if c.Name() == "eslint" {
			es = c
			break
		}
	}
	if es == nil {
		t.Fatal("eslint capability missing")
	}

	stdout := []byte(`[
		{
			"filePath": "/repo/src/foo.ts",
			"messages": [
				{
					"ruleId": "@typescript-eslint/no-explicit-any",
					"severity": 2,
					"message": "Unexpected any.",
					"line": 4,
					"column": 12,
					"endLine": 4,
					"endColumn": 15
				},
				{
					"ruleId": "@typescript-eslint/no-unused-vars",
					"severity": 1,
					"message": "Unused variable.",
					"line": 7,
					"column": 5,
					"endLine": 7,
					"endColumn": 8
				}
			]
		},
		{
			"filePath": "/repo/src/bar.ts",
			"messages": [
				{
					"ruleId": "@typescript-eslint/no-explicit-any",
					"severity": 2,
					"message": "Unexpected any.",
					"line": 1,
					"column": 1,
					"endLine": 1,
					"endColumn": 4
				}
			]
		}
	]`)

	rules := []manifest.Rule{
		{
			ID:       "ts-no-any",
			Severity: manifest.SeverityError,
			Detector: manifest.Detector{
				Capability: "eslint",
				Config:     map[string]any{"rule": "@typescript-eslint/no-explicit-any"},
			},
		},
		{
			ID:       "ts-no-unused",
			Severity: manifest.SeverityWarning,
			Detector: manifest.Detector{
				Capability: "eslint",
				Config:     map[string]any{"rule": "@typescript-eslint/no-unused-vars"},
			},
		},
	}

	findings, err := es.parseListSignal(&runOutput{Stdout: stdout}, rules)
	if err != nil {
		t.Fatalf("parseListSignal: %v", err)
	}
	if len(findings) != 3 {
		t.Fatalf("expected 3 findings, got %d", len(findings))
	}

	// finding[0]: @typescript-eslint/no-explicit-any in foo.ts
	if findings[0].File != "/repo/src/foo.ts" {
		t.Errorf("findings[0].File = %q (parent.file projection broken)", findings[0].File)
	}
	if findings[0].RuleID != "ts-no-any" {
		t.Errorf("findings[0].RuleID = %q", findings[0].RuleID)
	}
	if findings[0].Severity != "error" {
		t.Errorf("findings[0].Severity = %q (severity_map broken for numeric 2)", findings[0].Severity)
	}
	if findings[0].StartLine != 4 || findings[0].StartCol != 12 {
		t.Errorf("findings[0] location wrong: %+v", findings[0])
	}

	// finding[1]: @typescript-eslint/no-unused-vars in foo.ts
	if findings[1].RuleID != "ts-no-unused" {
		t.Errorf("findings[1].RuleID = %q", findings[1].RuleID)
	}
	if findings[1].Severity != "warning" {
		t.Errorf("findings[1].Severity = %q (numeric 1 should map to warning)", findings[1].Severity)
	}

	// finding[2]: @typescript-eslint/no-explicit-any in bar.ts (different parent)
	if findings[2].File != "/repo/src/bar.ts" {
		t.Errorf("findings[2].File = %q (parent.file should change between outer items)", findings[2].File)
	}
}

// TestEslintBuiltin_LoadAndValidate confirms the eslint YAML loads and the
// declared input schema accepts a typical rule.
func TestEslintBuiltin_LoadAndValidate(t *testing.T) {
	caps, _ := LoadPath(repoCapabilitiesDir(t))
	var es *Capability
	for _, c := range caps {
		if c.Name() == "eslint" {
			es = c
		}
	}
	if es == nil {
		t.Fatal("eslint not loaded")
	}
	if !es.AppliesToFile("foo.ts") {
		t.Error("should apply to .ts")
	}
	if !es.AppliesToFile("foo.tsx") {
		t.Error("should apply to .tsx")
	}
	if es.AppliesToFile("foo.go") {
		t.Error("should not apply to .go")
	}
	if err := es.ValidateConfig(map[string]any{
		"rule":   "@typescript-eslint/no-explicit-any",
		"plugin": "@typescript-eslint/eslint-plugin",
	}); err != nil {
		t.Errorf("ValidateConfig: %v", err)
	}
}

// TestEslint_Analyze_E2E_FakeBinary is the full end-to-end test: a fake
// node_modules layout with a stub eslint, a tsconfig, and a single rule.
// Exercises every platform feature in one go.
func TestEslint_Analyze_E2E_FakeBinary(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	stub := filepath.Join(binDir, "eslint")
	if err := os.WriteFile(stub, []byte(fakeEslintScript), 0755); err != nil {
		t.Fatal(err)
	}
	tsconfig := filepath.Join(root, "tsconfig.json")
	if err := os.WriteFile(tsconfig, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	subDir := filepath.Join(root, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(subDir, "main.ts")
	if err := os.WriteFile(target, []byte("const x: any = 1;\n"), 0644); err != nil {
		t.Fatal(err)
	}

	prevWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(prevWd) }()
	if err := os.Chdir(subDir); err != nil {
		t.Fatal(err)
	}

	caps, _ := LoadPath(repoCapabilitiesDir(t))
	var es *Capability
	for _, c := range caps {
		if c.Name() == "eslint" {
			es = c
		}
	}

	rules := []manifest.Rule{{
		ID:       "ts-no-any",
		Severity: manifest.SeverityError,
		Fix:      manifest.Fix{Message: "use a real type"},
		Detector: manifest.Detector{
			Capability: "eslint",
			Config: map[string]any{
				"rule":   "@typescript-eslint/no-explicit-any",
				"plugin": "@typescript-eslint/eslint-plugin",
			},
		},
	}}

	findings, err := es.Analyze(context.Background(), engine.AnalysisRequest{
		Files: []string{target},
		Rules: rules,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d: %+v", len(findings), findings)
	}
	f := findings[0]
	if f.RuleID != "ts-no-any" {
		t.Errorf("RuleID = %q", f.RuleID)
	}
	if f.File != target {
		t.Errorf("File = %q, want %q", f.File, target)
	}
	if f.Severity != "error" {
		t.Errorf("Severity = %q", f.Severity)
	}
	if f.StartLine != 4 {
		t.Errorf("StartLine = %d", f.StartLine)
	}
}

// TestPerLinter_Errcheck verifies the single-rule per-linter capability shape:
// errcheck capability runs golangci-lint -E errcheck, finds the unchecked
// error in the fixture, and attributes the finding to the pack rule that
// uses the errcheck capability. Path must be relative (no `..` prefix) —
// regression guard for Bug #1.
func TestPerLinter_Errcheck(t *testing.T) {
	if _, err := exec.LookPath("golangci-lint"); err != nil {
		t.Skip("golangci-lint not available on PATH")
	}

	fixture := filepath.Join("testdata", "per-linter-errcheck")
	abs, err := filepath.Abs(fixture)
	if err != nil {
		t.Fatal(err)
	}

	prevWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(prevWd) }()
	if err := os.Chdir(abs); err != nil {
		t.Fatal(err)
	}

	caps, err := LoadPath(repoCapabilitiesDir(t))
	if err != nil {
		t.Fatalf("LoadPath: %v", err)
	}
	var errcheckCap *Capability
	for _, c := range caps {
		if c.Name() == "errcheck" {
			errcheckCap = c
			break
		}
	}
	if errcheckCap == nil {
		t.Fatal("errcheck capability not found")
	}

	rules := []manifest.Rule{
		{
			ID: "go-check-error-return",
			Detector: manifest.Detector{
				Capability: "errcheck",
				Config:     map[string]any{},
			},
			Severity: manifest.SeverityError,
		},
	}

	findings, err := errcheckCap.Analyze(context.Background(), engine.AnalysisRequest{
		Files: []string{filepath.Join(abs, "main.go")},
		Rules: rules,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(findings), findings)
	}
	f := findings[0]
	if f.RuleID != "go-check-error-return" {
		t.Errorf("RuleID = %q, want go-check-error-return", f.RuleID)
	}
	if strings.HasPrefix(f.File, "..") {
		t.Errorf("path has unexpected `..` prefix: %q (Bug #1 regression)", f.File)
	}
}

// TestPerLinter_Revive_AtSourceRestriction verifies that the revive
// capability configures revive to run ONLY the rules pack rules ask for.
// The fixture contains code that would trigger several revive default rules
// (package-comments, unused-parameter, etc.), but the pack only enables
// `exported`, so we expect findings to be ONLY from that rule. This is the
// regression test for Bug #2 (misattribution): noise rules cannot fire
// because they are not enabled at source.
func TestPerLinter_Revive_AtSourceRestriction(t *testing.T) {
	if _, err := exec.LookPath("golangci-lint"); err != nil {
		t.Skip("golangci-lint not available on PATH")
	}

	fixture := filepath.Join("testdata", "per-linter-revive")
	abs, err := filepath.Abs(fixture)
	if err != nil {
		t.Fatal(err)
	}

	prevWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(prevWd) }()
	if err := os.Chdir(abs); err != nil {
		t.Fatal(err)
	}

	caps, err := LoadPath(repoCapabilitiesDir(t))
	if err != nil {
		t.Fatalf("LoadPath: %v", err)
	}
	var reviveCap *Capability
	for _, c := range caps {
		if c.Name() == "revive" {
			reviveCap = c
			break
		}
	}
	if reviveCap == nil {
		t.Fatal("revive capability not found")
	}

	rules := []manifest.Rule{
		{
			ID: "go-require-doc-comment",
			Detector: manifest.Detector{
				Capability: "revive",
				Config:     map[string]any{"rule": "exported"},
			},
			Severity: manifest.SeverityWarning,
		},
	}

	findings, err := reviveCap.Analyze(context.Background(), engine.AnalysisRequest{
		Files: []string{filepath.Join(abs, "main.go")},
		Rules: rules,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	// At-source restriction means only `exported` fires. Other revive
	// default rules (package-comments, etc.) are NOT enabled.
	for _, f := range findings {
		if f.RuleID != "go-require-doc-comment" {
			t.Errorf("unexpected rule ID %q in finding (Bug #2 regression — at-source restriction failed): %+v", f.RuleID, f)
		}
		if strings.HasPrefix(f.File, "..") {
			t.Errorf("path has unexpected `..` prefix: %q (Bug #1 regression)", f.File)
		}
	}
	if len(findings) == 0 {
		t.Error("expected at least one finding from revive's exported rule")
	}
}

// fakeEslintScript is a tiny POSIX shell stub used by the e2e test. It
// emits canned ESLint JSON to stdout and exits 1 (findings found).
const fakeEslintScript = `#!/bin/sh
TARGET=""
for arg in "$@"; do
  case "$arg" in
    *.ts|*.tsx) TARGET="$arg" ;;
  esac
done
echo '['
echo '  {'
echo "    \"filePath\": \"$TARGET\","
echo '    "messages": ['
echo '      {'
echo '        "ruleId": "@typescript-eslint/no-explicit-any",'
echo '        "severity": 2,'
echo '        "message": "Unexpected any.",'
echo '        "line": 4,'
echo '        "column": 12,'
echo '        "endLine": 4,'
echo '        "endColumn": 15'
echo '      }'
echo '    ]'
echo '  }'
echo ']'
exit 1
`
