// SPDX-License-Identifier: Apache-2.0

// load.go — YAML loading pipeline: single file, recursive directory, or auto-detect.
//
//   - Pipeline: loadFromBytes -> YAML unmarshal -> Validate -> build rule index -> collect warnings.
//   - LoadDir walks recursively for .yaml/.yml files; parse failures become ValidationErrors,
//     not fatal errors, so one bad file does not abort the entire directory load.
//   - mergeResults detects cross-file duplicate rule IDs and appends them as errors.
//   - Rules without inline tests produce warnings, not errors.

package manifest

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadResult contains the outcome of loading one or more manifest files.
type LoadResult struct {
	// Manifests maps file path → parsed manifest.
	Manifests map[string]*Manifest

	// Rules is the merged index of all rules keyed by rule ID.
	Rules map[string]*Rule

	// Errors contains all validation errors across all files.
	Errors ValidationErrors

	// Warnings contains non-fatal issues (e.g., rules without tests).
	Warnings []string
}

// Summary returns a human-readable summary of the load result.
func (r *LoadResult) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Loaded %d manifest(s), %d rule(s)", len(r.Manifests), len(r.Rules))
	if len(r.Warnings) > 0 {
		fmt.Fprintf(&b, ", %d warning(s)", len(r.Warnings))
	}
	if len(r.Errors) > 0 {
		fmt.Fprintf(&b, ", %d error(s)", len(r.Errors))
	}
	return b.String()
}

// LoadFile parses and validates a single manifest file.
func LoadFile(path string) (*LoadResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	return loadFromBytes(data, path)
}

// LoadDir recursively loads all .yaml and .yml files from a directory.
func LoadDir(dir string) (*LoadResult, error) {
	result := &LoadResult{
		Manifests: make(map[string]*Manifest),
		Rules:     make(map[string]*Rule),
	}

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		fileResult, loadErr := LoadFile(path)
		if loadErr != nil {
			result.Errors = append(result.Errors, ValidationError{
				File:    path,
				RuleID:  "pack-level",
				Message: loadErr.Error(),
			})
			return nil
		}

		mergeResults(result, fileResult)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking directory %s: %w", dir, err)
	}

	return result, nil
}

// Load detects whether the path is a file or directory and loads accordingly.
func Load(path string) (*LoadResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	if info.IsDir() {
		return LoadDir(path)
	}
	return LoadFile(path)
}

func loadFromBytes(data []byte, filePath string) (*LoadResult, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing YAML in %s: %w", filePath, err)
	}

	result := &LoadResult{
		Manifests: map[string]*Manifest{filePath: &m},
		Rules:     make(map[string]*Rule),
	}

	// Validate.
	if errs := Validate(&m, filePath); len(errs) > 0 {
		result.Errors = errs
		return result, nil
	}

	// Build rule index and collect warnings.
	for i := range m.Rules {
		r := &m.Rules[i]

		result.Rules[r.ID] = r

		if len(r.Tests) == 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"%s: rule %q has no inline tests (strongly recommended)", filePath, r.ID,
			))
		}
	}

	return result, nil
}

func mergeResults(dst, src *LoadResult) {
	for k, v := range src.Manifests {
		dst.Manifests[k] = v
	}

	for id, rule := range src.Rules {
		if _, exists := dst.Rules[id]; exists {
			dst.Errors = append(dst.Errors, ValidationError{
				RuleID:  id,
				Message: fmt.Sprintf("duplicate rule id %q across manifest files", id),
			})
			continue
		}
		dst.Rules[id] = rule
	}

	dst.Errors = append(dst.Errors, src.Errors...)
	dst.Warnings = append(dst.Warnings, src.Warnings...)
}
