// SPDX-License-Identifier: Apache-2.0

// sarif.go — SARIF 2.1.0 JSON output formatter.
//
//   - Rules and artifacts are sorted deterministically (by ID and path) so output is diffable.
//   - Findings referencing files not in the original input list are added to artifacts on the fly
//     (handles whole-package analysis discovering package-mate files).
//   - Severity "info" maps to SARIF level "note"; unknown severities default to "warning".
//   - Only the SARIF subset actually emitted is modeled as Go structs; unused SARIF fields are omitted.
package output

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/adamgilman/idiomatic/internal/engine"
	"github.com/adamgilman/idiomatic/manifest"
)

// SARIF 2.1.0 typed structs. Only the subset we emit is defined here.

type SARIFLog struct {
	Version string    `json:"version"`
	Schema  string    `json:"$schema"`
	Runs    []SARIFRun `json:"runs"`
}

type SARIFRun struct {
	Tool        SARIFTool          `json:"tool"`
	Invocations []SARIFInvocation  `json:"invocations"`
	Artifacts   []SARIFArtifact    `json:"artifacts"`
	Results     []SARIFResult      `json:"results"`
}

type SARIFTool struct {
	Driver SARIFDriver `json:"driver"`
}

type SARIFDriver struct {
	Name           string                   `json:"name"`
	Version        string                   `json:"version"`
	InformationURI string                   `json:"informationUri,omitempty"`
	Rules          []SARIFReportingDescriptor `json:"rules"`
}

type SARIFReportingDescriptor struct {
	ID                   string                    `json:"id"`
	Name                 string                    `json:"name"`
	ShortDescription     SARIFMessage              `json:"shortDescription"`
	FullDescription      SARIFMessage              `json:"fullDescription"`
	Help                 SARIFMessage              `json:"help"`
	DefaultConfiguration SARIFDefaultConfiguration `json:"defaultConfiguration"`
	Properties           *SARIFRuleProperties      `json:"properties,omitempty"`
}

type SARIFDefaultConfiguration struct {
	Level string `json:"level"`
}

type SARIFRuleProperties struct {
	Tags       []string `json:"tags,omitempty"`
	References []string `json:"references,omitempty"`
}

type SARIFInvocation struct {
	ExecutionSuccessful        bool                    `json:"executionSuccessful"`
	EndTimeUTC                 string                  `json:"endTimeUtc"`
	WorkingDirectory           *SARIFArtifactLocation  `json:"workingDirectory,omitempty"`
	ToolExecutionNotifications []SARIFNotification     `json:"toolExecutionNotifications,omitempty"`
}

type SARIFNotification struct {
	Level   string       `json:"level"`
	Message SARIFMessage `json:"message"`
}

type SARIFArtifact struct {
	Location SARIFArtifactLocation `json:"location"`
}

type SARIFArtifactLocation struct {
	URI   string `json:"uri"`
	Index *int   `json:"index,omitempty"`
}

type SARIFResult struct {
	RuleID    string          `json:"ruleId"`
	RuleIndex int             `json:"ruleIndex"`
	Level     string          `json:"level"`
	Message   SARIFMessage    `json:"message"`
	Locations []SARIFLocation `json:"locations"`
	Fixes     []SARIFFix      `json:"fixes,omitempty"`
}

type SARIFLocation struct {
	PhysicalLocation SARIFPhysicalLocation `json:"physicalLocation"`
}

type SARIFPhysicalLocation struct {
	ArtifactLocation SARIFArtifactLocation `json:"artifactLocation"`
	Region           SARIFRegion           `json:"region"`
}

type SARIFRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn"`
	EndLine     int `json:"endLine"`
	EndColumn   int `json:"endColumn"`
}

type SARIFFix struct {
	Description SARIFMessage `json:"description"`
}

type SARIFMessage struct {
	Text string `json:"text"`
}

// SARIFFormatter produces SARIF 2.1.0 JSON output.
type SARIFFormatter struct{}

func (f *SARIFFormatter) Format(findings []engine.Finding, rules []manifest.Rule, files []string, meta InvocationMetadata) ([]byte, error) {
	// Build rule index sorted by rule ID.
	sortedRules := make([]manifest.Rule, len(rules))
	copy(sortedRules, rules)
	sort.Slice(sortedRules, func(i, j int) bool {
		return sortedRules[i].ID < sortedRules[j].ID
	})

	ruleIndex := make(map[string]int)
	sarifRules := make([]SARIFReportingDescriptor, 0)
	for i, r := range sortedRules {
		ruleIndex[r.ID] = i
		sarifRules = append(sarifRules, buildReportingDescriptor(r))
	}

	// Build artifact index. Sort files for deterministic order.
	sortedFiles := make([]string, len(files))
	copy(sortedFiles, files)
	sort.Strings(sortedFiles)

	artifactIndex := make(map[string]int)
	artifacts := make([]SARIFArtifact, 0)
	for i, file := range sortedFiles {
		relURI := toRelativeURI(file, meta.WorkingDir)
		artifactIndex[file] = i
		artifacts = append(artifacts, SARIFArtifact{
			Location: SARIFArtifactLocation{URI: relURI},
		})
	}

	// Build results. Use empty slice so JSON emits [] not null.
	results := make([]SARIFResult, 0)
	for _, finding := range findings {
		ri, ok := ruleIndex[finding.RuleID]
		if !ok {
			return nil, fmt.Errorf("finding references unknown rule %q", finding.RuleID)
		}

		ai, ok := artifactIndex[finding.File]
		if !ok {
			// Finding references a file not in the original input list (e.g. a
			// package-mate discovered during whole-package analysis). Add it now.
			ai = len(artifacts)
			artifactIndex[finding.File] = ai
			artifacts = append(artifacts, SARIFArtifact{
				Location: SARIFArtifactLocation{URI: toRelativeURI(finding.File, meta.WorkingDir)},
			})
		}

		result := SARIFResult{
			RuleID:    finding.RuleID,
			RuleIndex: ri,
			Level:     mapSeverityToLevel(finding.Severity),
			Message:   SARIFMessage{Text: finding.Message},
			Locations: []SARIFLocation{
				{
					PhysicalLocation: SARIFPhysicalLocation{
						ArtifactLocation: SARIFArtifactLocation{
							URI:   toRelativeURI(finding.File, meta.WorkingDir),
							Index: intPtr(ai),
						},
						Region: SARIFRegion{
							StartLine:   finding.StartLine,
							StartColumn: finding.StartCol,
							EndLine:     finding.EndLine,
							EndColumn:   finding.EndCol,
						},
					},
				},
			},
		}

		if finding.FixMessage != "" {
			fixText := finding.FixMessage
			if finding.FixExample != "" {
				fixText += "\n\nExample:\n" + finding.FixExample
			}
			result.Fixes = []SARIFFix{
				{Description: SARIFMessage{Text: fixText}},
			}
		}

		results = append(results, result)
	}

	// Build invocation.
	invocation := SARIFInvocation{
		ExecutionSuccessful: meta.ExecError == "",
		EndTimeUTC:          time.Now().UTC().Format(time.RFC3339),
	}
	if meta.WorkingDir != "" {
		invocation.WorkingDirectory = &SARIFArtifactLocation{
			URI: "file://" + filepath.ToSlash(meta.WorkingDir),
		}
	}
	if meta.ExecError != "" {
		invocation.ToolExecutionNotifications = []SARIFNotification{
			{Level: "error", Message: SARIFMessage{Text: meta.ExecError}},
		}
	}

	log := SARIFLog{
		Version: "2.1.0",
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Runs: []SARIFRun{
			{
				Tool: SARIFTool{
					Driver: SARIFDriver{
						Name:           meta.ToolName,
						Version:        meta.ToolVersion,
						InformationURI: "https://github.com/adamgilman/idiomatic",
						Rules:          sarifRules,
					},
				},
				Invocations: []SARIFInvocation{invocation},
				Artifacts:   artifacts,
				Results:     results,
			},
		},
	}

	// Emit pretty-printed JSON with trailing newline.
	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling SARIF: %w", err)
	}
	data = append(data, '\n')
	return data, nil
}

func buildReportingDescriptor(r manifest.Rule) SARIFReportingDescriptor {
	desc := SARIFReportingDescriptor{
		ID:               r.ID,
		Name:             r.Name,
		ShortDescription: SARIFMessage{Text: r.Name},
		FullDescription:  SARIFMessage{Text: strings.TrimSpace(r.Description)},
		Help:             SARIFMessage{Text: strings.TrimSpace(r.Rationale)},
		DefaultConfiguration: SARIFDefaultConfiguration{
			Level: mapSeverityToLevel(string(r.Severity)),
		},
	}

	if len(r.Tags) > 0 || len(r.References) > 0 {
		desc.Properties = &SARIFRuleProperties{
			Tags:       r.Tags,
			References: r.References,
		}
	}

	return desc
}

func mapSeverityToLevel(severity string) string {
	switch severity {
	case "error":
		return "error"
	case "warning":
		return "warning"
	case "info":
		return "note"
	default:
		return "warning"
	}
}

func toRelativeURI(filePath, workingDir string) string {
	if workingDir != "" {
		rel, err := filepath.Rel(workingDir, filePath)
		if err == nil {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(filePath)
}

func intPtr(i int) *int {
	return &i
}
