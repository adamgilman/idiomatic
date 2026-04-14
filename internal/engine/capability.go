// SPDX-License-Identifier: Apache-2.0

// capability.go — Capability interface and supporting request/response types.
//
//   - Capability is implemented solely by declarative.Capability (YAML-driven); there are no hand-coded implementations.
//   - Detect discovers the external binary and returns ToolInfo; it must succeed before Analyze is called.
//   - ValidateConfig checks a rule's config map against the capability's declared inputs schema.
//   - To plug a Capability into the Backend-based routing layer, wrap it with CapabilityBackendAdapter.
package engine

import (
	"context"

	"github.com/adamgilman/idiomatic/manifest"
)

// Capability defines the contract for an external analysis tool integration.
type Capability interface {
	Name() string
	Version() string
	ValidateConfig(config map[string]any) error
	Detect() (*ToolInfo, error)
	Analyze(ctx context.Context, req AnalysisRequest) ([]Finding, error)
}

type ToolInfo struct {
	Name    string
	Path    string
	Version string
}

type AnalysisRequest struct {
	Files []string
	Rules []manifest.Rule
}
