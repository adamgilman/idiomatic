// SPDX-License-Identifier: Apache-2.0

// source_test.go — Tests for the git clone caching layer: URL hash
// determinism, collision resistance, and the error path when git is not
// on PATH.
package declarative

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUrlHash verifies deterministic output for a known URL.
func TestUrlHash(t *testing.T) {
	url := "https://github.com/adamgilman/idiomatic.git"
	h1 := urlHash(url)
	h2 := urlHash(url)
	if h1 != h2 {
		t.Fatalf("urlHash not deterministic: %q != %q", h1, h2)
	}
	// SHA-256 of the URL, first 8 bytes hex-encoded = 16 hex chars.
	if len(h1) != 16 {
		t.Fatalf("expected 16 hex chars, got %d: %q", len(h1), h1)
	}
}

// TestUrlHash_DifferentUrls verifies different URLs produce different hashes.
func TestUrlHash_DifferentUrls(t *testing.T) {
	a := urlHash("https://github.com/adamgilman/idiomatic.git")
	b := urlHash("https://github.com/other/repo.git")
	if a == b {
		t.Fatalf("different URLs produced the same hash: %q", a)
	}
}

// TestCloneRepo_NoGit sets PATH to an empty temp dir so git is not found,
// and verifies the error message mentions "git not found".
func TestCloneRepo_NoGit(t *testing.T) {
	// Create an empty directory to use as PATH so git cannot be found.
	emptyDir := t.TempDir()
	t.Setenv("PATH", emptyDir)

	// Also ensure the cache target does not already exist by using a
	// unique URL that won't match any prior cache entry.
	uniqueURL := "https://example.com/no-git-test-" + filepath.Base(emptyDir)

	// Remove any pre-existing cache entry (unlikely but safe).
	cacheDir, _ := os.UserCacheDir()
	target := filepath.Join(cacheDir, "idiomatic", "repos", urlHash(uniqueURL))
	os.RemoveAll(target)

	_, err := CloneRepo(uniqueURL)
	if err == nil {
		t.Fatal("expected error when git is not on PATH")
	}
	if !strings.Contains(err.Error(), "git not found") {
		t.Errorf("error should mention 'git not found', got: %v", err)
	}
}
