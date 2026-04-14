// SPDX-License-Identifier: Apache-2.0

// source.go — Git clone with local caching for remote capability sources.
//
//   - CloneRepo is idempotent: it hashes the URL to derive a stable cache path
//     under os.UserCacheDir()/idiomatic/repos/ and skips re-cloning if present.
//   - Clones are always depth-1 (shallow) to minimize bandwidth.
//   - The cache is never invalidated automatically; a stale clone persists
//     until the user removes it manually or the cache directory is cleared.
package declarative

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// CloneRepo shallow-clones a git repo URL into the user's cache directory.
// Returns the local cache path. Idempotent: if already cached, returns
// the existing path without re-cloning.
func CloneRepo(url string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	target := filepath.Join(cacheDir, "idiomatic", "repos", urlHash(url))

	if info, err := os.Stat(target); err == nil && info.IsDir() {
		return target, nil
	}

	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}

	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("git not found on PATH; required to clone %s", url)
	}

	cmd := exec.Command("git", "clone", "--depth", "1", url, target)
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(target)
		return "", fmt.Errorf("git clone %s: %w\n%s", url, err, string(out))
	}
	return target, nil
}

func urlHash(url string) string {
	sum := sha256.Sum256([]byte(url))
	return hex.EncodeToString(sum[:8])
}
