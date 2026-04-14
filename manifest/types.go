// SPDX-License-Identifier: Apache-2.0

// types.go — Kubernetes-style type definitions for v1alpha1 rule pack manifests.
//
//   - Manifest/Pack/Rule structs mirror the YAML schema 1:1; add a yaml tag for every new field.
//   - Detector routes rules to capabilities by name, not by Go type — there are no Go capability packages.
//   - Detector.Version is an optional semver constraint string validated at load time (validate.go),
//     but the actual match against the loaded capability happens in the engine, not here.
//   - This package must never import the engine; it is shared by CLI commands, validators, and tooling.

// Package manifest provides types, parsing, validation, and loading for
// v1alpha1 rule manifests. It is intentionally free of dependencies on the
// analysis engine so it can be reused by validate, test, and future tooling.
package manifest

const (
	APIVersionV1Alpha1 = "rules.idiomatic.dev/v1alpha1"
	KindRulePack       = "RulePack"
)

// Manifest is the top-level structure of a rule manifest file.
type Manifest struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Pack       Pack     `yaml:"pack"`
	Rules      []Rule   `yaml:"rules"`
}

// Pack contains metadata about a rule pack.
type Pack struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
	Maintainer  string `yaml:"maintainer"`
	License     string `yaml:"license,omitempty"`
	Homepage    string `yaml:"homepage,omitempty"`
}

// Rule defines a single rule within a pack.
type Rule struct {
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Rationale   string   `yaml:"rationale"`
	Severity    Severity `yaml:"severity"`
	Tags        []string `yaml:"tags,omitempty"`
	Detector    Detector `yaml:"detector"`
	AppliesTo   []string `yaml:"applies_to"`
	Excludes    []string `yaml:"excludes,omitempty"`
	Fix         Fix      `yaml:"fix"`
	References  []string `yaml:"references,omitempty"`
	Tests       []Test   `yaml:"tests,omitempty"`
}

// Severity represents the severity level of a rule.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Detector routes a rule to a capability.
//
//	Capability — name of a loaded capability (semgrep, gosec, eslint, ...)
//	Version    — optional semver constraint on the capability's metadata.version
//	             ("^1", ">=1.2,<2", "1.x"). Empty = no constraint (matches any).
//	Config     — per-rule inputs whose shape is defined by the capability's
//	             input schema.
//
// Authors are encouraged to declare a Version constraint on every rule so a
// future capability bump can't silently change the rule's behavior. The CLI
// validates the constraint after loading the registry but before any tool
// runs, so mismatches fail loudly with a clear error.
type Detector struct {
	Capability string         `yaml:"capability"`
	Version    string         `yaml:"version,omitempty"`
	Config     map[string]any `yaml:"config"`
}

// Fix contains structured guidance for resolving a finding.
type Fix struct {
	Message string `yaml:"message"`
	Example string `yaml:"example,omitempty"`
}

// Test is an inline test fixture for validating a rule.
type Test struct {
	Name   string     `yaml:"name"`
	Should TestExpect `yaml:"should"`
	Code   string     `yaml:"code"`
}

// TestExpect represents the expected outcome of a test.
type TestExpect string

const (
	TestExpectFire TestExpect = "fire"
	TestExpectPass TestExpect = "pass"
)
