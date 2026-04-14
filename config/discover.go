// SPDX-License-Identifier: Apache-2.0

// discover.go — Locates .idiomatic.yaml in a project root directory.
//
//   - Returns the full path if the file exists, empty string if not.
//   - The file is expected at the project root, like other dotfiles.
package config

import (
	"os"
	"path/filepath"
)

const Filename = ".idiomatic.yaml"

func Find(dir string) string {
	candidate := filepath.Join(dir, Filename)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}
