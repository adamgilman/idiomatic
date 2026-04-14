// SPDX-License-Identifier: Apache-2.0

// loader.go — Loads capability YAML from files, directories, and fs.FS sources.
//
//   - Directories are walked non-recursively (top-level *.yaml only), symmetric
//     with how rule packs are loaded elsewhere in the codebase.
//   - validateSpec runs at load time, rejecting unknown apiVersions, missing
//     required fields, and filling in defaults so downstream code never sees
//     partially-initialized specs.
//   - LoadFS exists for test convenience (fstest.MapFS); production code uses
//     LoadPath / LoadPaths with real filesystem paths.
package declarative

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// NewFromBytes parses a CapabilitySpec from YAML bytes and returns a Capability.
func NewFromBytes(data []byte) (*Capability, error) {
	var spec CapabilitySpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse capability YAML: %w", err)
	}
	if err := validateSpec(&spec); err != nil {
		return nil, err
	}
	return &Capability{spec: spec}, nil
}

// NewFromFile reads a YAML capability spec from disk.
func NewFromFile(path string) (*Capability, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read capability spec %s: %w", path, err)
	}
	return NewFromBytes(data)
}

// LoadPath loads capability specs from one filesystem path. The path can be a
// single YAML file or a directory of YAML files. Directories are walked
// non-recursively (only top-level *.yaml entries are loaded). Symmetrically
// to how the manifest loader treats rule packs.
func LoadPath(path string) ([]*Capability, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.IsDir() {
		c, err := NewFromFile(path)
		if err != nil {
			return nil, err
		}
		return []*Capability{c}, nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", path, err)
	}
	var caps []*Capability
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		full := filepath.Join(path, e.Name())
		c, err := NewFromFile(full)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", full, err)
		}
		caps = append(caps, c)
	}
	return caps, nil
}

// LoadPaths loads capabilities from multiple paths and concatenates the
// results.
func LoadPaths(paths []string) ([]*Capability, error) {
	var all []*Capability
	for _, p := range paths {
		caps, err := LoadPath(p)
		if err != nil {
			return nil, err
		}
		all = append(all, caps...)
	}
	return all, nil
}

// LoadFS loads every *.yaml at the top level of the given filesystem.
// Useful for tests that wrap an inline directory in fstest.MapFS.
func LoadFS(fsys fs.FS) ([]*Capability, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read fs root: %w", err)
	}
	var caps []*Capability
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		c, err := NewFromBytes(data)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", e.Name(), err)
		}
		caps = append(caps, c)
	}
	return caps, nil
}

// validateSpec performs structural validation that catches obviously broken
// specs at load time so Analyze never sees them.
func validateSpec(s *CapabilitySpec) error {
	// apiVersion + kind. Reject anything we don't recognize so a v1beta1
	// capability can't accidentally load against a v1alpha1 reader.
	if s.APIVersion != CapabilityAPIVersion {
		return fmt.Errorf("capability spec: apiVersion %q not supported (want %q)",
			s.APIVersion, CapabilityAPIVersion)
	}
	if s.Kind != CapabilityKind {
		return fmt.Errorf("capability spec: kind %q not supported (want %q)",
			s.Kind, CapabilityKind)
	}

	// Identity
	if strings.TrimSpace(s.Metadata.Name) == "" {
		return fmt.Errorf("capability spec: metadata.name is required")
	}
	if strings.TrimSpace(s.Metadata.Version) == "" {
		return fmt.Errorf("capability %q: metadata.version is required (semver, e.g. 1.0.0)", s.Metadata.Name)
	}
	if _, err := parseSemver(s.Metadata.Version); err != nil {
		return fmt.Errorf("capability %q: metadata.version %q is not a valid semver: %w",
			s.Metadata.Name, s.Metadata.Version, err)
	}

	// Behavior
	body := &s.Spec
	name := s.Metadata.Name

	if strings.TrimSpace(body.Requires.Binary) == "" {
		return fmt.Errorf("capability %q: spec.requires.binary is required", name)
	}
	if len(body.Run.Argv) == 0 && body.Run.RuleArgvField == "" {
		return fmt.Errorf("capability %q: spec.run.argv must not be empty (or set rule_argv_field for per:rule capabilities)", name)
	}
	if body.Run.Per == "" {
		body.Run.Per = "batch"
	}
	switch body.Run.Per {
	case "batch", "rule", "file":
	default:
		return fmt.Errorf("capability %q: spec.run.per must be batch|rule|file (got %q)", name, body.Run.Per)
	}
	if body.Run.Output == "" {
		body.Run.Output = "stdout"
	}
	switch body.Run.Output {
	case "stdout", "report_file":
	default:
		return fmt.Errorf("capability %q: spec.run.output must be stdout|report_file (got %q)", name, body.Run.Output)
	}
	if body.Run.Output == "report_file" && strings.TrimSpace(body.Run.ReportPathPattern) == "" {
		return fmt.Errorf("capability %q: spec.run.report_path_pattern is required when output=report_file", name)
	}
	switch body.Signal.Shape {
	case "list":
		if body.Signal.Format == "" {
			body.Signal.Format = "json"
		}
		if body.Signal.MatchRuleBy == nil || strings.TrimSpace(body.Signal.MatchRuleBy.From) == "" {
			return fmt.Errorf("capability %q: spec.signal.match_rule_by.from is required for list signals", name)
		}
		if body.Signal.MatchRuleBy.Strategy == "" {
			body.Signal.MatchRuleBy.Strategy = "by_id"
		}
	case "scalar":
		// Scalar capabilities don't enumerate match_rule_by; rules use
		// fire_when expressions to query the signal.
	default:
		return fmt.Errorf("capability %q: spec.signal.shape must be list|scalar (got %q)", name, body.Signal.Shape)
	}
	return nil
}
