// SPDX-License-Identifier: Apache-2.0

// adapter.go — Bridges a Capability to the legacy Backend interface.
//
//   - CapabilityBackendAdapter wraps any Capability so it can be used wherever a Backend is expected.
//   - The adapter's Name() returns the name passed at construction, not cap.Name(); this lets the
//     routing layer key backends by capability name independently of the underlying tool identity.
package engine

import (
	"context"

	"github.com/adamgilman/idiomatic/manifest"
)

type CapabilityBackendAdapter struct {
	cap  Capability
	name string
}

func NewCapabilityBackendAdapter(cap Capability, name string) *CapabilityBackendAdapter {
	return &CapabilityBackendAdapter{cap: cap, name: name}
}

func (a *CapabilityBackendAdapter) Name() string { return a.name }

func (a *CapabilityBackendAdapter) Analyze(ctx context.Context, files []string, rules []manifest.Rule) ([]Finding, error) {
	return a.cap.Analyze(ctx, AnalysisRequest{
		Files: files,
		Rules: rules,
	})
}
