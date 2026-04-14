// SPDX-License-Identifier: Apache-2.0

// claudehook.go — Claude Code PostToolUse hook handler: stdin JSON in, analysis, hook response JSON out.
//
//   - The hook contract is inviolable: always exit 0, always emit valid JSON or nothing.
//     Errors are reported inside additionalContext, never via exit codes or stderr.
//   - SARIF output is preferred, but if it exceeds BudgetThreshold (9KB) the response
//     falls back to a compact summary with a temp-file path to the full SARIF report.
//   - File routing is driven by the declarative Registry (each YAML capability's applies_to_files);
//     files that match no capability produce a silent nil response, not an error.
package claudehook

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/adamgilman/idiomatic/internal/declarative"
	"github.com/adamgilman/idiomatic/internal/engine"
	"github.com/adamgilman/idiomatic/internal/output"
	"github.com/adamgilman/idiomatic/manifest"
)

const (
	// BudgetThreshold is the character limit for the additionalContext payload.
	BudgetThreshold = 9000

	// MaxCompactFindings caps findings in the compact summary.
	MaxCompactFindings = 30
)

// HookInput represents the JSON Claude Code sends to PostToolUse hooks on stdin.
type HookInput struct {
	SessionID     string          `json:"session_id"`
	CWD           string          `json:"cwd"`
	HookEventName string          `json:"hook_event_name"`
	ToolName      string          `json:"tool_name"`
	ToolInput     json.RawMessage `json:"tool_input"`
	ToolResponse  json.RawMessage `json:"tool_response"`
}

// EditToolInput is the tool_input shape for Edit/Write/MultiEdit tools.
type EditToolInput struct {
	FilePath string `json:"file_path"`
}

// HookSpecificOutput is the nested structure Claude Code expects for PostToolUse.
type HookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// HookResponse is the JSON written to stdout for Claude Code to consume.
type HookResponse struct {
	SystemMessage      string             `json:"systemMessage,omitempty"`
	HookSpecificOutput HookSpecificOutput `json:"hookSpecificOutput"`
}

// CompactSummary is the budget-friendly format when SARIF is too large.
type CompactSummary struct {
	Summary  CompactSummaryMeta `json:"summary"`
	Findings []CompactFinding   `json:"findings"`
}

// CompactSummaryMeta holds aggregate information about the analysis.
type CompactSummaryMeta struct {
	FilesAnalyzed      int            `json:"files_analyzed"`
	FindingsTotal      int            `json:"findings_total"`
	FindingsBySeverity map[string]int `json:"findings_by_severity"`
	FullReportPath     string         `json:"full_report_path,omitempty"`
	Truncated          int            `json:"truncated"`
}

// CompactFinding is a single finding in the compact format.
type CompactFinding struct {
	Rule       string `json:"rule"`
	Severity   string `json:"severity"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Column     int    `json:"column"`
	Message    string `json:"message"`
	Fix        string `json:"fix"`
	FixExample string `json:"fix_example,omitempty"`
}

// ErrorPayload is emitted in additionalContext when something goes wrong.
type ErrorPayload struct {
	Error           string `json:"error"`
	Detail          string `json:"detail,omitempty"`
	SuggestedAction string `json:"suggested_action,omitempty"`
}

// Run executes the full hook flow: parse stdin, route files, analyze, respond.
// Always returns valid JSON or nil. Errors are reported inside the response.
// The backends map and rules are injected by the caller (CLI command); the
// registry drives file-to-backend routing via each capability's AppliesToFile
// method.
func Run(ctx context.Context, stdinData []byte, backends map[string]engine.Backend, registry *declarative.Registry, rules []manifest.Rule, meta output.InvocationMetadata) ([]byte, error) {
	// Parse hook input.
	var input HookInput
	if err := json.Unmarshal(stdinData, &input); err != nil {
		return emitError("Failed to parse hook input", err.Error(),
			"Check that Claude Code is sending valid JSON to the hook.")
	}

	// Extract edited file path(s).
	files := extractFilePaths(input)
	if len(files) == 0 {
		return nil, nil
	}

	// Route files to backends. The declarative registry knows which
	// capabilities want which files (via AppliesToFile).
	filesByBackend := routeFiles(files, registry)
	if len(filesByBackend) == 0 {
		// Silent no-op: unsupported file extension.
		return nil, nil
	}

	// Use CWD from hook input if available.
	cwd := input.CWD
	if cwd == "" {
		cwd = meta.WorkingDir
	}

	// Run analysis. Routing key is the capability name (rule.Detector.Capability)
	// — there are no separate "backend names" anymore.
	rulesByCapability := make(map[string][]manifest.Rule)
	for _, rule := range rules {
		rulesByCapability[rule.Detector.Capability] = append(rulesByCapability[rule.Detector.Capability], rule)
	}

	var allFindings []engine.Finding
	allFiles := files

	for capName, capRules := range rulesByCapability {
		capFiles := filesByBackend[capName]
		if len(capFiles) == 0 {
			// No files matching this capability's filter — skip without
			// requiring the binary.
			continue
		}
		backend, ok := backends[capName]
		if !ok {
			return emitError("Capability not registered",
				fmt.Sprintf("capability %q required by manifest but its binary is missing", capName),
				"Install the underlying tool or remove the rule from the manifest.")
		}
		findings, err := backend.Analyze(ctx, capFiles, capRules)
		if err != nil {
			return emitError("Analysis failed", err.Error(),
				"Check that the source files are syntactically valid.")
		}
		allFindings = append(allFindings, findings...)
	}

	// Build the response payload based on budget.
	meta.WorkingDir = cwd
	return BuildResponse(allFindings, rules, allFiles, meta)
}

// routeFiles maps each input file to every capability that wants to receive
// it. Routing is driven by the declarative registry — each YAML capability
// declares its applies_to_files and the registry tells us which ones match
// each file path.
func routeFiles(files []string, registry *declarative.Registry) map[string][]string {
	filesByCapability := make(map[string][]string)
	if registry == nil {
		return filesByCapability
	}
	for _, f := range files {
		for _, capName := range registry.RouteFile(f) {
			filesByCapability[capName] = append(filesByCapability[capName], f)
		}
	}
	return filesByCapability
}

// extractFilePaths gets the edited file path(s) from the hook input.
func extractFilePaths(input HookInput) []string {
	var ti EditToolInput
	if err := json.Unmarshal(input.ToolInput, &ti); err == nil && ti.FilePath != "" {
		return []string{ti.FilePath}
	}
	return nil
}

// BuildResponse decides SARIF vs compact based on the budget threshold.
func BuildResponse(findings []engine.Finding, rules []manifest.Rule, files []string, meta output.InvocationMetadata) ([]byte, error) {
	// Try SARIF first.
	sarifFormatter := &output.SARIFFormatter{}
	sarifBytes, err := sarifFormatter.Format(findings, rules, files, meta)
	if err != nil {
		return emitError("Failed to format SARIF", err.Error(), "")
	}

	sysMsg := formatSystemMessage(findings)

	sarifStr := string(sarifBytes)
	if len(sarifStr) <= BudgetThreshold {
		return wrapResponse(sarifStr, sysMsg)
	}

	// SARIF too large. Write full report to temp file, emit compact summary.
	tmpFile, err := os.CreateTemp("", "idiomatic-*.sarif")
	if err != nil {
		return emitError("Failed to create temp file", err.Error(), "")
	}
	tmpFile.Write(sarifBytes)
	tmpFile.Close()

	compact := BuildCompactSummary(findings, files, tmpFile.Name())
	compactBytes, err := json.Marshal(compact)
	if err != nil {
		return emitError("Failed to format compact summary", err.Error(), "")
	}

	return wrapResponse(string(compactBytes), sysMsg)
}

// BuildCompactSummary creates the budget-friendly format.
func BuildCompactSummary(findings []engine.Finding, files []string, fullReportPath string) CompactSummary {
	bySeverity := map[string]int{"error": 0, "warning": 0, "info": 0}
	for _, f := range findings {
		bySeverity[f.Severity]++
	}

	truncated := 0
	compactFindings := make([]CompactFinding, 0)
	for i, f := range findings {
		if i >= MaxCompactFindings {
			truncated = len(findings) - MaxCompactFindings
			break
		}
		compactFindings = append(compactFindings, CompactFinding{
			Rule:       f.RuleID,
			Severity:   f.Severity,
			File:       f.File,
			Line:       f.StartLine,
			Column:     f.StartCol,
			Message:    f.Message,
			Fix:        f.FixMessage,
			FixExample: f.FixExample,
		})
	}

	return CompactSummary{
		Summary: CompactSummaryMeta{
			FilesAnalyzed:      len(files),
			FindingsTotal:      len(findings),
			FindingsBySeverity: bySeverity,
			FullReportPath:     fullReportPath,
			Truncated:          truncated,
		},
		Findings: compactFindings,
	}
}

func wrapResponse(payload, systemMessage string) ([]byte, error) {
	resp := HookResponse{
		SystemMessage: systemMessage,
		HookSpecificOutput: HookSpecificOutput{
			HookEventName:     "PostToolUse",
			AdditionalContext: payload,
		},
	}
	return json.Marshal(resp)
}

func emitError(errMsg, detail, suggestedAction string) ([]byte, error) {
	payload := ErrorPayload{
		Error:           errMsg,
		Detail:          detail,
		SuggestedAction: suggestedAction,
	}
	payloadBytes, _ := json.Marshal(payload)
	return wrapResponse(string(payloadBytes), "idiomatic: analysis failed")
}

// formatSystemMessage builds a one-line user-facing summary from findings.
// Returns "" when there are no findings (omits the field from JSON via omitempty).
func formatSystemMessage(findings []engine.Finding) string {
	if len(findings) == 0 {
		return ""
	}

	bySeverity := map[string]int{}
	for _, f := range findings {
		bySeverity[f.Severity]++
	}

	total := len(findings)
	noun := "issues"
	if total == 1 {
		noun = "issue"
	}

	// Build severity breakdown.
	var parts []string
	for _, sev := range []string{"error", "warning", "info"} {
		n := bySeverity[sev]
		if n == 0 {
			continue
		}
		label := sev + "s"
		if n == 1 {
			label = sev
		}
		parts = append(parts, fmt.Sprintf("%d %s", n, label))
	}

	// Single severity: simpler format.
	if len(parts) == 1 {
		sev := ""
		for s, n := range bySeverity {
			if n > 0 {
				sev = s
				if n > 1 {
					sev = s + "s"
				}
				break
			}
		}
		return fmt.Sprintf("idiomatic: %d %s found (%s)", total, noun, sev)
	}

	return fmt.Sprintf("idiomatic: %d %s found (%s)", total, noun, strings.Join(parts, ", "))
}
