// SPDX-License-Identifier: Apache-2.0

// discover_test.go — Tests for .idiomatic.yaml project root lookup.
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFind_Exists(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, Filename)
	if err := os.WriteFile(cfgPath, []byte("kind: ProjectConfig\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := Find(dir)
	if got != cfgPath {
		t.Fatalf("expected %s, got %s", cfgPath, got)
	}
}

func TestFind_NotFound(t *testing.T) {
	dir := t.TempDir()
	got := Find(dir)
	if got != "" {
		t.Fatalf("expected empty string, got %s", got)
	}
}
