// SPDX-License-Identifier: Apache-2.0

// claudehook_test.go — Tests for the Claude Code PostToolUse hook handler.
//
//   - Tests run without a declarative.Registry (nil), so file routing always returns no matches;
//     this exercises the silent no-op and error paths without needing real capabilities.
//   - BuildResponse and BuildCompactSummary are tested independently of Run to isolate
//     SARIF formatting, budget-based fallback, and systemMessage generation.
package claudehook

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/adamgilman/idiomatic/internal/engine"
	"github.com/adamgilman/idiomatic/internal/output"
	"github.com/adamgilman/idiomatic/manifest"
)

var testMeta = output.InvocationMetadata{
	ToolName:    "idio",
	ToolVersion: "test",
	WorkingDir:  "/tmp",
}

func makeHookInput(toolName, filePath, cwd string) []byte {
	input := map[string]interface{}{
		"session_id":      "test-session",
		"cwd":             cwd,
		"hook_event_name": "PostToolUse",
		"tool_name":       toolName,
		"tool_input":      map[string]string{"file_path": filePath},
	}
	data, _ := json.Marshal(input)
	return data
}

func TestRun_MalformedStdin(t *testing.T) {
	resp, err := Run(context.Background(), []byte("not json"), nil, nil, nil, testMeta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected error response, got nil")
	}

	var hookResp HookResponse
	if err := json.Unmarshal(resp, &hookResp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	var errPayload ErrorPayload
	if err := json.Unmarshal([]byte(hookResp.HookSpecificOutput.AdditionalContext), &errPayload); err != nil {
		t.Fatalf("invalid error payload: %v", err)
	}
	if errPayload.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestRun_UnsupportedExtension(t *testing.T) {
	// Unsupported extensions (.md, etc.) produce silent no-op.
	input := makeHookInput("Edit", "/tmp/test.md", "/tmp")
	resp, err := Run(context.Background(), input, nil, nil, nil, testMeta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response for unsupported extension, got %s", string(resp))
	}
}

func TestRun_TypeScriptFile_NoRegistry(t *testing.T) {
	// A .ts file with nil registry → silent no-op (no routes).
	input := makeHookInput("Edit", "/tmp/app.ts", "/tmp")
	resp, err := Run(context.Background(), input, map[string]engine.Backend{}, nil, nil, testMeta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response for nil registry, got %s", string(resp))
	}
}

func TestRun_NoRules(t *testing.T) {
	// A Go file with nil registry and nil rules → silent no-op (no routes).
	input := makeHookInput("Edit", "/tmp/test.go", "/tmp")
	backends := map[string]engine.Backend{}
	resp, err := Run(context.Background(), input, backends, nil, nil, testMeta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response for nil registry, got %s", string(resp))
	}
}

func TestRun_EmptyToolInput(t *testing.T) {
	input := []byte(`{"session_id":"test","cwd":"/tmp","hook_event_name":"PostToolUse","tool_name":"Edit","tool_input":{}}`)
	resp, err := Run(context.Background(), input, nil, nil, nil, testMeta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No file_path in tool_input → silent no-op.
	if resp != nil {
		t.Errorf("expected nil response for empty tool_input, got %s", string(resp))
	}
}

func TestRun_WriteToolVariant(t *testing.T) {
	// Write tool has the same tool_input shape as Edit.
	input := makeHookInput("Write", "/tmp/test.md", "/tmp")
	resp, _ := Run(context.Background(), input, nil, nil, nil, testMeta)
	if resp != nil {
		t.Errorf("expected nil for non-Go file via Write tool, got %s", string(resp))
	}
}

func TestBuildCompactSummary_Basic(t *testing.T) {
	findings := []engine.Finding{
		{RuleID: "r1", Severity: "error", File: "a.go", StartLine: 1, StartCol: 1, Message: "msg1", FixMessage: "fix1"},
		{RuleID: "r2", Severity: "warning", File: "b.go", StartLine: 2, StartCol: 2, Message: "msg2", FixMessage: "fix2"},
	}

	summary := BuildCompactSummary(findings, []string{"a.go", "b.go"}, "/tmp/full.sarif")

	if summary.Summary.FilesAnalyzed != 2 {
		t.Errorf("files_analyzed = %d, want 2", summary.Summary.FilesAnalyzed)
	}
	if summary.Summary.FindingsTotal != 2 {
		t.Errorf("findings_total = %d, want 2", summary.Summary.FindingsTotal)
	}
	if summary.Summary.FindingsBySeverity["error"] != 1 {
		t.Errorf("error count = %d, want 1", summary.Summary.FindingsBySeverity["error"])
	}
	if summary.Summary.Truncated != 0 {
		t.Errorf("truncated = %d, want 0", summary.Summary.Truncated)
	}
	if len(summary.Findings) != 2 {
		t.Errorf("findings len = %d, want 2", len(summary.Findings))
	}
}

func TestBuildCompactSummary_Truncation(t *testing.T) {
	var findings []engine.Finding
	for i := 0; i < 50; i++ {
		findings = append(findings, engine.Finding{
			RuleID: "r1", Severity: "error", File: "a.go",
			StartLine: i, StartCol: 1, Message: "msg", FixMessage: "fix",
		})
	}

	summary := BuildCompactSummary(findings, []string{"a.go"}, "/tmp/full.sarif")

	if len(summary.Findings) != MaxCompactFindings {
		t.Errorf("findings len = %d, want %d", len(summary.Findings), MaxCompactFindings)
	}
	if summary.Summary.Truncated != 20 {
		t.Errorf("truncated = %d, want 20", summary.Summary.Truncated)
	}
	if summary.Summary.FindingsTotal != 50 {
		t.Errorf("findings_total = %d, want 50", summary.Summary.FindingsTotal)
	}
}

func TestBuildResponse_UnderBudget(t *testing.T) {
	findings := []engine.Finding{
		{RuleID: "r1", Severity: "error", File: "a.go", StartLine: 1, StartCol: 1,
			EndLine: 1, EndCol: 10, Message: "msg", FixMessage: "fix"},
	}
	rules := []manifest.Rule{
		{ID: "r1", Severity: "error", Detector: manifest.Detector{Capability: "semgrep", Config: map[string]any{"language":"go","pattern":"x"}}},
	}

	resp, err := BuildResponse(findings, rules, []string{"a.go"}, testMeta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var hookResp HookResponse
	if err := json.Unmarshal(resp, &hookResp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// Should be SARIF (has "version" field).
	var sarif map[string]interface{}
	if err := json.Unmarshal([]byte(hookResp.HookSpecificOutput.AdditionalContext), &sarif); err != nil {
		t.Fatalf("failed to parse additionalContext: %v", err)
	}
	if sarif["version"] != "2.1.0" {
		t.Errorf("expected SARIF format, got: %v", sarif)
	}
}

func TestBuildResponse_ZeroFindings(t *testing.T) {
	resp, err := BuildResponse(nil, nil, []string{"a.go"}, testMeta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var hookResp HookResponse
	if err := json.Unmarshal(resp, &hookResp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	var sarif map[string]interface{}
	if err := json.Unmarshal([]byte(hookResp.HookSpecificOutput.AdditionalContext), &sarif); err != nil {
		t.Fatalf("failed to unmarshal additionalContext: %v", err)
	}
	if sarif["version"] != "2.1.0" {
		t.Error("expected SARIF even with zero findings")
	}
}

func TestEmitError_Format(t *testing.T) {
	resp, _ := emitError("test error", "detail here", "do this")

	var hookResp HookResponse
	_ = json.Unmarshal(resp, &hookResp)

	var errPayload ErrorPayload
	_ = json.Unmarshal([]byte(hookResp.HookSpecificOutput.AdditionalContext), &errPayload)

	if errPayload.Error != "test error" {
		t.Errorf("error = %q", errPayload.Error)
	}
	if errPayload.Detail != "detail here" {
		t.Errorf("detail = %q", errPayload.Detail)
	}
	if errPayload.SuggestedAction != "do this" {
		t.Errorf("suggested_action = %q", errPayload.SuggestedAction)
	}
	if hookResp.SystemMessage != "idiomatic: analysis failed" {
		t.Errorf("systemMessage = %q, want %q", hookResp.SystemMessage, "idiomatic: analysis failed")
	}
}

// --- systemMessage tests ---

func TestFormatSystemMessage_ZeroFindings(t *testing.T) {
	if got := formatSystemMessage(nil); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestFormatSystemMessage_OneError(t *testing.T) {
	findings := []engine.Finding{{Severity: "error"}}
	got := formatSystemMessage(findings)
	want := "idiomatic: 1 issue found (error)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatSystemMessage_MultipleErrors(t *testing.T) {
	findings := []engine.Finding{{Severity: "error"}, {Severity: "error"}, {Severity: "error"}}
	got := formatSystemMessage(findings)
	want := "idiomatic: 3 issues found (errors)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatSystemMessage_MixedSeverities(t *testing.T) {
	findings := []engine.Finding{
		{Severity: "error"}, {Severity: "error"},
		{Severity: "warning"},
	}
	got := formatSystemMessage(findings)
	want := "idiomatic: 3 issues found (2 errors, 1 warning)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatSystemMessage_AllThreeSeverities(t *testing.T) {
	findings := []engine.Finding{
		{Severity: "error"},
		{Severity: "warning"}, {Severity: "warning"},
		{Severity: "info"},
	}
	got := formatSystemMessage(findings)
	want := "idiomatic: 4 issues found (1 error, 2 warnings, 1 info)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatSystemMessage_OnlyWarnings(t *testing.T) {
	findings := []engine.Finding{{Severity: "warning"}, {Severity: "warning"}}
	got := formatSystemMessage(findings)
	want := "idiomatic: 2 issues found (warnings)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildResponse_SystemMessage_Present(t *testing.T) {
	findings := []engine.Finding{
		{RuleID: "r1", Severity: "error", File: "a.go", StartLine: 1, StartCol: 1,
			EndLine: 1, EndCol: 10, Message: "msg", FixMessage: "fix"},
	}
	rules := []manifest.Rule{
		{ID: "r1", Severity: "error", Detector: manifest.Detector{Capability: "semgrep", Config: map[string]any{"language":"go","pattern":"x"}}},
	}

	resp, _ := BuildResponse(findings, rules, []string{"a.go"}, testMeta)

	var hookResp HookResponse
	_ = json.Unmarshal(resp, &hookResp)

	if hookResp.SystemMessage != "idiomatic: 1 issue found (error)" {
		t.Errorf("systemMessage = %q", hookResp.SystemMessage)
	}
}

func TestBuildResponse_SystemMessage_Absent_ZeroFindings(t *testing.T) {
	resp, _ := BuildResponse(nil, nil, []string{"a.go"}, testMeta)

	var hookResp HookResponse
	_ = json.Unmarshal(resp, &hookResp)

	if hookResp.SystemMessage != "" {
		t.Errorf("expected empty systemMessage for zero findings, got %q", hookResp.SystemMessage)
	}

	// Also verify the field is actually absent from JSON (omitempty).
	var raw map[string]interface{}
	_ = json.Unmarshal(resp, &raw)
	if _, exists := raw["systemMessage"]; exists {
		t.Error("systemMessage field should be absent from JSON when empty")
	}
}
