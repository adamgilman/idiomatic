// SPDX-License-Identifier: Apache-2.0

// loader_test.go — Tests for LoadProject repo cloning and capability/pack assembly.
//
//   - TestLoadProject_NonexistentRepo verifies clone failure for unreachable repos
//   - TestLoadedProject_Fields is a structural smoke test for the LoadedProject type
//   - No real git clones happen in the field test; it constructs LoadedProject directly
package config

import (
	"strings"
	"testing"

	"github.com/adamgilman/idiomatic/internal/declarative"
	"github.com/adamgilman/idiomatic/manifest"
)

func TestLoadProject_NonexistentRepo(t *testing.T) {
	cfg := &ProjectConfig{
		APIVersion: "config.idiomatic.dev/v1alpha1",
		Kind:       "ProjectConfig",
		Capabilities: []GitSource{
			{
				Repo: "https://example.com/nonexistent/repo-does-not-exist.git",
				Path: []string{"capabilities/"},
			},
		},
		Packs: []GitSource{
			{
				Repo: "https://example.com/nonexistent/repo-does-not-exist.git",
				Path: []string{"packs/"},
			},
		},
	}

	_, err := LoadProject(cfg)
	if err == nil {
		t.Fatal("expected error for nonexistent repo")
	}
	if !strings.Contains(err.Error(), "git") {
		t.Fatalf("expected error to mention git, got: %v", err)
	}
}

func TestLoadedProject_Fields(t *testing.T) {
	// Verify LoadedProject has the expected fields and types.
	lp := &LoadedProject{
		Registry: declarative.NewRegistry(),
		Rules:    []manifest.Rule{{ID: "test-rule"}},
		Warnings: []string{"some warning"},
	}

	if lp.Registry == nil {
		t.Fatal("expected non-nil registry")
	}
	if len(lp.Rules) != 1 || lp.Rules[0].ID != "test-rule" {
		t.Fatalf("unexpected rules: %v", lp.Rules)
	}
	if len(lp.Warnings) != 1 || lp.Warnings[0] != "some warning" {
		t.Fatalf("unexpected warnings: %v", lp.Warnings)
	}
}
