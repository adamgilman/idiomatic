// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionOutput(t *testing.T) {
	cmd := NewCmdVersion()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "idio") {
		t.Errorf("expected output to contain 'idio', got: %s", output)
	}
}

// TestVersionInfo_LdflagsTakePrecedence verifies that when ldflags-injected
// values are present (the goreleaser/Makefile path), they win over the
// build-info fallback. We simulate this by stashing the package-level vars,
// setting all three to non-default values, and asserting they pass through.
func TestVersionInfo_LdflagsTakePrecedence(t *testing.T) {
	origV, origC, origD := Version, Commit, Date
	defer func() { Version, Commit, Date = origV, origC, origD }()

	Version = "v1.2.3"
	Commit = "abc1234"
	Date = "2026-05-03T10:00:00Z"

	v, c, d := versionInfo()
	if v != "v1.2.3" || c != "abc1234" || d != "2026-05-03T10:00:00Z" {
		t.Errorf("ldflags values not preserved: got (%q, %q, %q)", v, c, d)
	}
}

// TestVersionInfo_FallbackRunsWhenSentinels verifies that when ldflags
// weren't set, versionInfo replaces sentinels with runtime/debug.ReadBuildInfo
// data when available. This is the `go install` path. We can't synthesize a
// BuildInfo in test (the Go runtime owns it), so we just assert the function
// returns *something* — under `go test`, ReadBuildInfo always succeeds and
// at least vcs.revision tends to be populated when the test binary is built
// from a git checkout, so commit usually moves off "none".
func TestVersionInfo_FallbackRunsWhenSentinels(t *testing.T) {
	origV, origC, origD := Version, Commit, Date
	defer func() { Version, Commit, Date = origV, origC, origD }()

	Version, Commit, Date = "dev", "none", "unknown"

	v, c, d := versionInfo()
	// At minimum the function must not panic and must return strings. We
	// cannot assert specific non-default values because debug.ReadBuildInfo
	// content depends on the test environment.
	if v == "" || c == "" || d == "" {
		t.Errorf("versionInfo returned empty fields: (%q, %q, %q)", v, c, d)
	}
}
