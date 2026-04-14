// SPDX-License-Identifier: Apache-2.0

// output.go — Formatter interface and InvocationMetadata shared by all output formats.
//
//   - Formatter is the sole output abstraction; implementations (HumanFormatter, SARIFFormatter)
//     receive the complete analysis result set and produce serialized bytes.
//   - InvocationMetadata travels from the CLI entry point through to formatters; WorkingDir is
//     used to relativize file paths in SARIF output.
//   - ExecError in metadata signals a failed analysis; formatters embed it in their output
//     rather than returning an error (SARIF uses toolExecutionNotifications).
package output

import (
	"github.com/adamgilman/idiomatic/internal/engine"
	"github.com/adamgilman/idiomatic/manifest"
)

// InvocationMetadata carries context about the CLI invocation.
type InvocationMetadata struct {
	ToolName    string
	ToolVersion string
	WorkingDir  string // Absolute path to working directory.
	ExecError   string // Non-empty if analysis failed.
}

// Formatter produces formatted output from analysis results.
type Formatter interface {
	Format(findings []engine.Finding, rules []manifest.Rule, files []string, meta InvocationMetadata) ([]byte, error)
}
