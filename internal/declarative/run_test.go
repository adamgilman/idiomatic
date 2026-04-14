// SPDX-License-Identifier: Apache-2.0

// run_test.go — Tests for tool execution mechanics: config file rendering
// (semgrep template round-trip), argv expansion ({config_file}, {files}
// tokens), and filenamePattern temp-file naming.
package declarative

import (
	"os"
	"testing"

	"github.com/adamgilman/idiomatic/manifest"
	"gopkg.in/yaml.v3"
)

// TestRenderConfigFile_Semgrep verifies the bundled semgrep template produces
// a YAML rules file that semgrep can consume. We don't byte-diff against the
// legacy generator (different field ordering, different list styles) — we
// instead unmarshal the result and assert structural equivalence.
func TestRenderConfigFile_Semgrep(t *testing.T) {
	cap, err := NewFromBytes(semgrepYAML)
	if err != nil {
		t.Fatalf("NewFromBytes: %v", err)
	}

	rules := []manifest.Rule{
		{
			ID:       "go-no-panic",
			Severity: manifest.SeverityError,
			Fix:      manifest.Fix{Message: "Don't panic in production code"},
			Detector: manifest.Detector{
				Config: map[string]any{
					"language": "go",
					"pattern":  "panic(...)",
				},
			},
		},
		{
			ID:       "go-test-sleep",
			Severity: manifest.SeverityWarning,
			Fix:      manifest.Fix{Message: "Use channels instead of sleep"},
			Detector: manifest.Detector{
				Config: map[string]any{
					"language": "go",
					"patterns": []any{
						map[string]any{"pattern": "time.Sleep(...)"},
						map[string]any{"pattern-inside": "func Test$NAME($T *testing.T) { ... }"},
					},
				},
			},
		},
	}

	path, err := cap.renderConfigFile(rules, &runtimeCtx{ProjectRoot: "/tmp", Discovered: map[string]string{}})
	if err != nil {
		t.Fatalf("renderConfigFile: %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered file: %v", err)
	}

	var got struct {
		Rules []map[string]any `yaml:"rules"`
	}
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("rendered YAML did not parse: %v\n---\n%s", err, string(data))
	}
	if len(got.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d\n%s", len(got.Rules), string(data))
	}

	// Rule 0: scalar pattern.
	r0 := got.Rules[0]
	if r0["id"] != "go-no-panic" {
		t.Errorf("rules[0].id = %v, want go-no-panic", r0["id"])
	}
	if r0["severity"] != "ERROR" {
		t.Errorf("rules[0].severity = %v, want ERROR", r0["severity"])
	}
	if r0["pattern"] != "panic(...)" {
		t.Errorf("rules[0].pattern = %v, want panic(...)", r0["pattern"])
	}
	langs0, ok := r0["languages"].([]any)
	if !ok || len(langs0) != 1 || langs0[0] != "go" {
		t.Errorf("rules[0].languages = %v, want [go]", r0["languages"])
	}

	// Rule 1: list of pattern fragments must round-trip.
	r1 := got.Rules[1]
	if r1["id"] != "go-test-sleep" {
		t.Errorf("rules[1].id = %v, want go-test-sleep", r1["id"])
	}
	if r1["severity"] != "WARNING" {
		t.Errorf("rules[1].severity = %v, want WARNING", r1["severity"])
	}
	patterns, ok := r1["patterns"].([]any)
	if !ok || len(patterns) != 2 {
		t.Fatalf("rules[1].patterns missing or wrong shape: %v", r1["patterns"])
	}
	first, _ := patterns[0].(map[string]any)
	if first["pattern"] != "time.Sleep(...)" {
		t.Errorf("rules[1].patterns[0].pattern = %v", first["pattern"])
	}
}

func TestExpandArgv(t *testing.T) {
	cases := []struct {
		name       string
		argv       []string
		configFile string
		files      []string
		want       []string
	}{
		{
			name:       "config_file standalone arg",
			argv:       []string{"scan", "--config", "{config_file}", "--json"},
			configFile: "/tmp/cfg.yaml",
			want:       []string{"scan", "--config", "/tmp/cfg.yaml", "--json"},
		},
		{
			name:  "files expand inline",
			argv:  []string{"scan", "{files}"},
			files: []string{"a.go", "b.go"},
			want:  []string{"scan", "a.go", "b.go"},
		},
		{
			name:       "config_file embedded in arg",
			argv:       []string{"--config={config_file}"},
			configFile: "/tmp/cfg.yaml",
			want:       []string{"--config=/tmp/cfg.yaml"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cap := &Capability{spec: CapabilitySpec{Spec: CapabilitySpecBody{Run: RunSpec{Argv: tc.argv}}}}
			got, err := cap.expandArgv(invocationCtx{
				ConfigFile: tc.configFile,
				Files:      tc.files,
			}, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !equalStrings(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFilenamePattern(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"{tmp}/idio-semgrep-rules.yaml", "idio-semgrep-rules-*.yaml"},
		{"foo.json", "foo-*.json"},
		{"noext", "noext-*"},
	}
	for _, tc := range cases {
		if got := filenamePattern(tc.in); got != tc.want {
			t.Errorf("filenamePattern(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
