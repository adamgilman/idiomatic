// SPDX-License-Identifier: Apache-2.0

// adapter_test.go — Tests for CapabilityBackendAdapter.
//
//   - stubCapability is a minimal no-op Capability; recordingCapability extends it to capture the
//     last AnalysisRequest, verifying that the adapter faithfully passes files and rules through.
package engine

import (
	"context"
	"testing"

	"github.com/adamgilman/idiomatic/manifest"
)

func TestCapabilityBackendAdapter_Name(t *testing.T) {
	adapter := NewCapabilityBackendAdapter(&stubCapability{name: "eslint"}, "typescript")
	if adapter.Name() != "typescript" {
		t.Errorf("Name() = %q, want %q", adapter.Name(), "typescript")
	}
}

func TestCapabilityBackendAdapter_Analyze(t *testing.T) {
	cap := &recordingCapability{}
	adapter := NewCapabilityBackendAdapter(cap, "typescript")

	rules := []manifest.Rule{{ID: "test-rule"}}
	files := []string{"/tmp/test.ts"}

	_, err := adapter.Analyze(context.Background(), files, rules)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cap.lastReq.Files) != 1 || cap.lastReq.Files[0] != "/tmp/test.ts" {
		t.Errorf("Files not passed through: %v", cap.lastReq.Files)
	}
	if len(cap.lastReq.Rules) != 1 || cap.lastReq.Rules[0].ID != "test-rule" {
		t.Errorf("Rules not passed through")
	}
}

type stubCapability struct {
	name string
}

func (s *stubCapability) Name() string                                            { return s.name }
func (s *stubCapability) Version() string                                         { return "1.0.0" }
func (s *stubCapability) ValidateConfig(map[string]any) error                     { return nil }
func (s *stubCapability) Detect() (*ToolInfo, error)                              { return &ToolInfo{Name: s.name}, nil }
func (s *stubCapability) Analyze(_ context.Context, _ AnalysisRequest) ([]Finding, error) {
	return nil, nil
}

type recordingCapability struct {
	stubCapability
	lastReq AnalysisRequest
}

func (r *recordingCapability) Analyze(ctx context.Context, req AnalysisRequest) ([]Finding, error) {
	r.lastReq = req
	return nil, nil
}
