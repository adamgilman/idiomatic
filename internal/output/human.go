// SPDX-License-Identifier: Apache-2.0

// human.go — Human-readable output formatter (file:line:col: severity: message [rule-id]).
//
//   - Returns nil (not empty bytes) when there are no findings, so callers can distinguish
//     "no output needed" from "empty string output."
//   - The format intentionally matches gcc/golangci-lint diagnostic style for editor gutter integration.
package output

import (
	"fmt"
	"strings"

	"github.com/adamgilman/idiomatic/internal/engine"
	"github.com/adamgilman/idiomatic/manifest"
)

// HumanFormatter outputs findings in the human-readable format:
// file:line:col: severity: message [rule-id]
type HumanFormatter struct{}

func (f *HumanFormatter) Format(findings []engine.Finding, rules []manifest.Rule, files []string, meta InvocationMetadata) ([]byte, error) {
	if len(findings) == 0 {
		return nil, nil
	}

	var b strings.Builder
	for _, finding := range findings {
		fmt.Fprintf(&b, "%s:%d:%d: %s: %s [%s]\n",
			finding.File, finding.StartLine, finding.StartCol,
			finding.Severity, finding.Message, finding.RuleID)
	}
	return []byte(b.String()), nil
}
