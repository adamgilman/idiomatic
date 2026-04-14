// SPDX-License-Identifier: Apache-2.0

// types.go — Core data types and the Backend interface for the analysis pipeline.
//
//   - Finding is the universal output type: every capability produces these, every formatter consumes them.
//   - Backend is the legacy routing interface; new tool integrations use Capability (see capability.go)
//     and are bridged via CapabilityBackendAdapter (see adapter.go).
//   - Line/column numbers are 1-indexed; EndCol is exclusive.
package engine

import (
	"context"

	"github.com/adamgilman/idiomatic/manifest"
)

// Finding represents a single rule violation found during analysis.
type Finding struct {
	RuleID   string // The id of the rule that fired, from the manifest.
	Severity string // "error", "warning", or "info" (mirrored from the rule).

	// Location
	File     string // Absolute or repo-relative path to the file containing the violation.
	StartLine int   // 1-indexed line number.
	StartCol  int   // 1-indexed column number.
	EndLine   int   // 1-indexed; equal to StartLine if single-line.
	EndCol    int   // 1-indexed; exclusive end column.

	// Human-facing content
	Message    string // Short description of the specific violation.
	FixMessage string // From rule.fix.message in the manifest.
	FixExample string // From rule.fix.example in the manifest (may be empty).
}

// Backend analyzes files and returns findings. Only one implementation exists
// at this stage (the in-process Go backend), but the interface exists to
// establish the boundary.
type Backend interface {
	// Name returns the backend identifier, e.g., "go-analysis".
	// This must match the detector.backend field in rule manifests.
	Name() string

	// Analyze runs all applicable rules against the given files and returns findings.
	// The rules argument contains only rules whose detector.backend matches this backend.
	Analyze(ctx context.Context, files []string, rules []manifest.Rule) ([]Finding, error)
}
