// SPDX-License-Identifier: Apache-2.0

// spec_test.go — Tests for CapabilitySpec parsing and validation, covering
// both the semgrep (list-mode) and git (scalar-mode) YAML shapes, plus
// negative cases that verify structural validation rejects broken specs.
package declarative

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// semgrepYAML reads the canonical semgrep.yaml from the repo's top-level
// capabilities/ directory using a runtime.Caller-resolved path so the test
// works regardless of where `go test` is invoked from. There are no
// embedded copies of the capability files.
var semgrepYAML = func() []byte {
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	data, err := os.ReadFile(filepath.Join(repoRoot, "capabilities", "semgrep.yaml"))
	if err != nil {
		panic("read capabilities/semgrep.yaml: " + err.Error())
	}
	return data
}()

// TestNewFromBytes_Semgrep verifies the bundled semgrep.yaml round-trips
// through the loader and exposes the expected fields. This is the simpler of
// the two flexibility checks.
func TestNewFromBytes_Semgrep(t *testing.T) {
	cap, err := NewFromBytes(semgrepYAML)
	if err != nil {
		t.Fatalf("NewFromBytes: %v", err)
	}
	if cap.Name() != "semgrep" {
		t.Errorf("Name() = %q, want semgrep", cap.Name())
	}
	if cap.spec.Spec.Run.Per != "batch" {
		t.Errorf("Run.Per = %q, want batch", cap.spec.Spec.Run.Per)
	}
	if cap.spec.Spec.Signal.Shape != "list" {
		t.Errorf("Signal.Shape = %q, want list", cap.spec.Spec.Signal.Shape)
	}
	if cap.spec.Spec.Signal.MatchRuleBy == nil || cap.spec.Spec.Signal.MatchRuleBy.Transform != "tail_after_dot" {
		t.Errorf("MatchRuleBy.Transform missing or wrong: %+v", cap.spec.Spec.Signal.MatchRuleBy)
	}
	if _, ok := cap.spec.Spec.Inputs["language"]; !ok {
		t.Error("expected 'language' input declared")
	}
}

// TestNewFromBytes_GitScalar verifies the schema is flexible enough to express
// a scalar-mode capability (per-rule invocation, fire_when binding) even
// though scalar mode is not yet implemented at runtime. This is the
// "schema-not-implementation" flexibility check from the plan.
func TestNewFromBytes_GitScalar(t *testing.T) {
	cap, err := NewFromBytes([]byte(gitYAML))
	if err != nil {
		t.Fatalf("NewFromBytes: %v", err)
	}
	if cap.Name() != "git" {
		t.Errorf("Name() = %q, want git", cap.Name())
	}
	if cap.spec.Spec.Run.Per != "rule" {
		t.Errorf("Run.Per = %q, want rule", cap.spec.Spec.Run.Per)
	}
	if cap.spec.Spec.Signal.Shape != "scalar" {
		t.Errorf("Signal.Shape = %q, want scalar", cap.spec.Spec.Signal.Shape)
	}
	// Confirm the per-input schema parsed.
	if got := cap.spec.Spec.Inputs["argv"].Type; got != "list" {
		t.Errorf("inputs.argv.type = %q, want list", got)
	}
	if !cap.spec.Spec.Inputs["argv"].Required {
		t.Error("inputs.argv.required should be true")
	}
}

// TestValidateSpec_RejectsBroken catches the obvious structural mistakes.
// Each YAML below is missing exactly one required piece of the schema.
func TestValidateSpec_RejectsBroken(t *testing.T) {
	header := `apiVersion: capabilities.idiomatic.dev/v1alpha1
kind: Capability
metadata: { name: foo, version: 1.0.0 }
spec:
`

	cases := []struct {
		name string
		yaml string
	}{
		{
			name: "missing apiVersion",
			yaml: `
kind: Capability
metadata: { name: foo, version: 1.0.0 }
spec:
  requires: { binary: bar }
  run: { argv: [baz] }
  signal: { shape: list, match_rule_by: { from: id } }
`,
		},
		{
			name: "wrong kind",
			yaml: `
apiVersion: capabilities.idiomatic.dev/v1alpha1
kind: WrongThing
metadata: { name: foo, version: 1.0.0 }
spec:
  requires: { binary: bar }
  run: { argv: [baz] }
  signal: { shape: list, match_rule_by: { from: id } }
`,
		},
		{
			name: "missing metadata.name",
			yaml: `
apiVersion: capabilities.idiomatic.dev/v1alpha1
kind: Capability
metadata: { version: 1.0.0 }
spec:
  requires: { binary: bar }
  run: { argv: [baz] }
  signal: { shape: list, match_rule_by: { from: id } }
`,
		},
		{
			name: "missing metadata.version",
			yaml: `
apiVersion: capabilities.idiomatic.dev/v1alpha1
kind: Capability
metadata: { name: foo }
spec:
  requires: { binary: bar }
  run: { argv: [baz] }
  signal: { shape: list, match_rule_by: { from: id } }
`,
		},
		{
			name: "non-semver metadata.version",
			yaml: `
apiVersion: capabilities.idiomatic.dev/v1alpha1
kind: Capability
metadata: { name: foo, version: "not-a-version" }
spec:
  requires: { binary: bar }
  run: { argv: [baz] }
  signal: { shape: list, match_rule_by: { from: id } }
`,
		},
		{
			name: "missing binary",
			yaml: header + `
  run: { argv: [bar] }
  signal: { shape: list, match_rule_by: { from: id } }
`,
		},
		{
			name: "empty argv",
			yaml: header + `
  requires: { binary: bar }
  run: { argv: [] }
  signal: { shape: list, match_rule_by: { from: id } }
`,
		},
		{
			name: "list signal without match_rule_by",
			yaml: header + `
  requires: { binary: bar }
  run: { argv: [baz] }
  signal: { shape: list }
`,
		},
		{
			name: "unknown signal shape",
			yaml: header + `
  requires: { binary: bar }
  run: { argv: [baz] }
  signal: { shape: martian }
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewFromBytes([]byte(tc.yaml)); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

// gitYAML is an inline stand-in spec used by parser tests. It exercises
// every scalar-mode schema feature so the loader is forced to handle them.
const gitYAML = `
apiVersion: capabilities.idiomatic.dev/v1alpha1
kind: Capability
metadata:
  name: git
  version: 1.0.0
  description: Wrapper around the git CLI

spec:
  requires:
    binary: git
    install: "your system package manager"

  inputs:
    argv:
      type: list
      required: true
      description: argv passed to git

  run:
    per: rule
    argv: ["{{ .Inputs.argv }}"]
    timeout: 5s
    ok_exit: [0, 1, 128]

  signal:
    shape: scalar
    scalar_fields:
      stdout:    { from: stdout, transform: trim }
      stderr:    { from: stderr, transform: trim }
      exit_code: { from: exit }
`

// TestValidateSpec_ByCapability_NoFromAccepted confirms a list-mode capability
// with strategy=by_capability is accepted even when match_rule_by.from is
// empty — by_capability ignores the JSON item entirely, so there's no path
// to extract.
func TestValidateSpec_ByCapability_NoFromAccepted(t *testing.T) {
	yaml := `apiVersion: capabilities.idiomatic.dev/v1alpha1
kind: Capability
metadata: { name: foo, version: 1.0.0 }
spec:
  requires: { binary: bar }
  run: { argv: [baz] }
  signal:
    shape: list
    format: json
    list: Issues
    match_rule_by:
      strategy: by_capability
    fields:
      file: Pos.Filename
      line: Pos.Line
`
	if _, err := NewFromBytes([]byte(yaml)); err != nil {
		t.Fatalf("expected by_capability without from to be accepted, got: %v", err)
	}
}

// TestValidateSpec_NonByCapability_RequiresFrom confirms that a list-mode
// capability with any non-by_capability strategy still requires a non-empty
// match_rule_by.from. This locks in the existing rejection so a future
// loosening doesn't accidentally let by_id capabilities ship without `from`.
func TestValidateSpec_NonByCapability_RequiresFrom(t *testing.T) {
	yaml := `apiVersion: capabilities.idiomatic.dev/v1alpha1
kind: Capability
metadata: { name: foo, version: 1.0.0 }
spec:
  requires: { binary: bar }
  run: { argv: [baz] }
  signal:
    shape: list
    format: json
    list: Issues
    match_rule_by:
      strategy: by_id
    fields:
      file: Pos.Filename
`
	if _, err := NewFromBytes([]byte(yaml)); err == nil {
		t.Error("expected error for by_id without from, got nil")
	}
}
