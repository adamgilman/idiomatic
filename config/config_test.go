// SPDX-License-Identifier: Apache-2.0

// config_test.go — Tests for ProjectConfig YAML parsing and validation.
//
//   - Covers happy-path loading plus each validation branch (wrong kind, missing fields)
//   - Uses temp files to avoid depending on real .idiomatic.yaml fixtures
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, ".idiomatic.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad_Valid(t *testing.T) {
	p := writeTemp(t, `
apiVersion: config.idiomatic.dev/v1alpha1
kind: ProjectConfig
capabilities:
  - repo: https://github.com/adamgilman/idiomatic.git
    path:
      - capabilities/semgrep.yaml
packs:
  - repo: https://github.com/adamgilman/idiomatic.git
    path:
      - packs/python.yaml
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Kind != "ProjectConfig" {
		t.Fatalf("expected kind ProjectConfig, got %q", cfg.Kind)
	}
	if len(cfg.Capabilities) != 1 {
		t.Fatalf("expected 1 capability source, got %d", len(cfg.Capabilities))
	}
	if cfg.Capabilities[0].Repo != "https://github.com/adamgilman/idiomatic.git" {
		t.Fatalf("unexpected repo: %s", cfg.Capabilities[0].Repo)
	}
	if len(cfg.Capabilities[0].Path) != 1 || cfg.Capabilities[0].Path[0] != "capabilities/semgrep.yaml" {
		t.Fatalf("unexpected path: %v", cfg.Capabilities[0].Path)
	}
	if len(cfg.Packs) != 1 {
		t.Fatalf("expected 1 pack source, got %d", len(cfg.Packs))
	}
}

func TestLoad_WrongKind(t *testing.T) {
	p := writeTemp(t, `
apiVersion: config.idiomatic.dev/v1alpha1
kind: WrongKind
capabilities:
  - repo: https://example.com/repo.git
    path: [caps/foo.yaml]
packs:
  - repo: https://example.com/repo.git
    path: [packs/bar.yaml]
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error for wrong kind")
	}
	if !strings.Contains(err.Error(), "expected kind ProjectConfig") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_MissingCapabilities(t *testing.T) {
	p := writeTemp(t, `
apiVersion: config.idiomatic.dev/v1alpha1
kind: ProjectConfig
packs:
  - repo: https://example.com/repo.git
    path: [packs/bar.yaml]
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error for missing capabilities")
	}
	if !strings.Contains(err.Error(), "no capabilities declared") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_MissingPacks(t *testing.T) {
	p := writeTemp(t, `
apiVersion: config.idiomatic.dev/v1alpha1
kind: ProjectConfig
capabilities:
  - repo: https://example.com/repo.git
    path: [caps/foo.yaml]
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error for missing packs")
	}
	if !strings.Contains(err.Error(), "no packs declared") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_MissingRepo(t *testing.T) {
	p := writeTemp(t, `
apiVersion: config.idiomatic.dev/v1alpha1
kind: ProjectConfig
capabilities:
  - path: [caps/foo.yaml]
packs:
  - repo: https://example.com/repo.git
    path: [packs/bar.yaml]
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error for missing repo")
	}
	if !strings.Contains(err.Error(), "capabilities[0].repo is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_MissingPath(t *testing.T) {
	p := writeTemp(t, `
apiVersion: config.idiomatic.dev/v1alpha1
kind: ProjectConfig
capabilities:
  - repo: https://example.com/repo.git
packs:
  - repo: https://example.com/repo.git
    path: [packs/bar.yaml]
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error for missing path")
	}
	if !strings.Contains(err.Error(), "capabilities[0].path is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_MultipleRepos(t *testing.T) {
	p := writeTemp(t, `
apiVersion: config.idiomatic.dev/v1alpha1
kind: ProjectConfig
capabilities:
  - repo: https://github.com/adamgilman/idiomatic.git
    path:
      - capabilities/semgrep.yaml
  - repo: https://github.com/community/caps.git
    path:
      - capabilities/custom.yaml
      - capabilities/another.yaml
packs:
  - repo: https://github.com/adamgilman/idiomatic.git
    path:
      - packs/python.yaml
  - repo: https://github.com/community/packs.git
    path:
      - packs/go.yaml
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Capabilities) != 2 {
		t.Fatalf("expected 2 capability sources, got %d", len(cfg.Capabilities))
	}
	if len(cfg.Capabilities[1].Path) != 2 {
		t.Fatalf("expected 2 paths in second capability, got %d", len(cfg.Capabilities[1].Path))
	}
	if len(cfg.Packs) != 2 {
		t.Fatalf("expected 2 pack sources, got %d", len(cfg.Packs))
	}
}
