// SPDX-License-Identifier: Apache-2.0

// validate.go — Batch validation with error collection for rule pack manifests.
//
//   - Never fail fast: collect all errors in one pass so authors can fix everything at once.
//   - IDs must match ^[a-z0-9][a-z0-9-]*[a-z0-9]$ (no underscores, uppercase, or leading/trailing hyphens).
//   - Detector.Version constraints are syntax-checked here via Masterminds/semver/v3,
//     but the actual constraint-vs-capability match happens at registry load time in the engine.
//   - Only the capability detector format is supported; the legacy backend/id/params form was retired.

package manifest

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// idPattern matches valid identifiers: lowercase letters, digits, hyphens only.
var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$`)

// ValidationError represents a single validation problem.
type ValidationError struct {
	File       string // file path where the error occurred
	RuleID     string // rule id or "pack-level" for pack metadata errors
	Field      string // dotted field path (e.g., "rules[0].severity")
	Message    string // what is wrong
	Suggestion string // optional suggestion for how to fix it
}

func (e ValidationError) Error() string {
	var b strings.Builder
	if e.File != "" {
		b.WriteString(e.File)
		b.WriteString(": ")
	}
	if e.RuleID != "" {
		fmt.Fprintf(&b, "[%s] ", e.RuleID)
	}
	if e.Field != "" {
		fmt.Fprintf(&b, "%s: ", e.Field)
	}
	b.WriteString(e.Message)
	if e.Suggestion != "" {
		fmt.Fprintf(&b, " (%s)", e.Suggestion)
	}
	return b.String()
}

// ValidationErrors is a collection of validation errors.
type ValidationErrors []ValidationError

func (errs ValidationErrors) Error() string {
	var b strings.Builder
	for i, e := range errs {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(e.Error())
	}
	return b.String()
}

// Validate checks a parsed manifest for structural correctness.
// It returns all errors at once so users can fix everything in a single pass.
// The filePath parameter is used for error messages only.
func Validate(m *Manifest, filePath string) ValidationErrors {
	var errs ValidationErrors

	// Top-level fields.
	if m.APIVersion == "" {
		errs = append(errs, ValidationError{
			File:    filePath,
			RuleID:  "pack-level",
			Field:   "apiVersion",
			Message: "apiVersion is required",
		})
	} else if m.APIVersion != APIVersionV1Alpha1 {
		errs = append(errs, ValidationError{
			File:       filePath,
			RuleID:     "pack-level",
			Field:      "apiVersion",
			Message:    fmt.Sprintf("unrecognized apiVersion %q", m.APIVersion),
			Suggestion: fmt.Sprintf("use %q", APIVersionV1Alpha1),
		})
	}

	if m.Kind == "" {
		errs = append(errs, ValidationError{
			File:    filePath,
			RuleID:  "pack-level",
			Field:   "kind",
			Message: "kind is required",
		})
	} else if m.Kind != KindRulePack {
		errs = append(errs, ValidationError{
			File:       filePath,
			RuleID:     "pack-level",
			Field:      "kind",
			Message:    fmt.Sprintf("unrecognized kind %q", m.Kind),
			Suggestion: fmt.Sprintf("use %q", KindRulePack),
		})
	}

	// Pack metadata.
	errs = append(errs, validatePack(&m.Pack, filePath)...)

	// Rules.
	if len(m.Rules) == 0 {
		errs = append(errs, ValidationError{
			File:    filePath,
			RuleID:  "pack-level",
			Field:   "rules",
			Message: "at least one rule is required",
		})
	}

	seenIDs := make(map[string]bool)
	for i := range m.Rules {
		r := &m.Rules[i]
		prefix := fmt.Sprintf("rules[%d]", i)
		errs = append(errs, validateRule(r, prefix, filePath)...)

		if r.ID != "" {
			if seenIDs[r.ID] {
				errs = append(errs, ValidationError{
					File:    filePath,
					RuleID:  r.ID,
					Field:   prefix + ".id",
					Message: fmt.Sprintf("duplicate rule id %q", r.ID),
				})
			}
			seenIDs[r.ID] = true
		}
	}

	return errs
}

func validatePack(p *Pack, filePath string) ValidationErrors {
	var errs ValidationErrors

	requireStr := func(field, value string) {
		if strings.TrimSpace(value) == "" {
			errs = append(errs, ValidationError{
				File:    filePath,
				RuleID:  "pack-level",
				Field:   "pack." + field,
				Message: field + " is required",
			})
		}
	}

	requireStr("id", p.ID)
	requireStr("name", p.Name)
	requireStr("version", p.Version)
	requireStr("description", p.Description)
	requireStr("maintainer", p.Maintainer)

	if p.ID != "" && !isValidID(p.ID) {
		errs = append(errs, ValidationError{
			File:       filePath,
			RuleID:     "pack-level",
			Field:      "pack.id",
			Message:    fmt.Sprintf("invalid pack id %q: must be lowercase, hyphenated, alphanumeric", p.ID),
			Suggestion: "use only [a-z0-9-]",
		})
	}

	return errs
}

func validateRule(r *Rule, prefix, filePath string) ValidationErrors {
	var errs ValidationErrors
	ruleID := r.ID
	if ruleID == "" {
		ruleID = prefix
	}

	// Required string fields.
	requiredFields := map[string]string{
		"id":          r.ID,
		"name":        r.Name,
		"description": r.Description,
		"rationale":   r.Rationale,
	}
	for field, value := range requiredFields {
		if strings.TrimSpace(value) == "" {
			errs = append(errs, ValidationError{
				File:    filePath,
				RuleID:  ruleID,
				Field:   prefix + "." + field,
				Message: field + " is required",
			})
		}
	}

	// ID format.
	if r.ID != "" && !isValidID(r.ID) {
		errs = append(errs, ValidationError{
			File:       filePath,
			RuleID:     ruleID,
			Field:      prefix + ".id",
			Message:    fmt.Sprintf("invalid rule id %q: must be lowercase, hyphenated, alphanumeric", r.ID),
			Suggestion: "use only [a-z0-9-]",
		})
	}

	// Name length.
	if len(r.Name) > 80 {
		errs = append(errs, ValidationError{
			File:    filePath,
			RuleID:  ruleID,
			Field:   prefix + ".name",
			Message: fmt.Sprintf("name exceeds 80 characters (got %d)", len(r.Name)),
		})
	}

	// Severity.
	if r.Severity == "" {
		errs = append(errs, ValidationError{
			File:    filePath,
			RuleID:  ruleID,
			Field:   prefix + ".severity",
			Message: "severity is required",
		})
	} else if !isValidSeverity(r.Severity) {
		errs = append(errs, ValidationError{
			File:       filePath,
			RuleID:     ruleID,
			Field:      prefix + ".severity",
			Message:    fmt.Sprintf("invalid severity %q", r.Severity),
			Suggestion: "use one of: error, warning, info",
		})
	}

	// Detector: capability + config is the only supported format. The legacy
	// backend+id form was retired pre-launch.
	if strings.TrimSpace(r.Detector.Capability) == "" {
		errs = append(errs, ValidationError{
			File:       filePath,
			RuleID:     ruleID,
			Field:      prefix + ".detector.capability",
			Message:    "detector.capability is required",
			Suggestion: "add detector.capability",
		})
	}
	// detector.config is optional. Single-rule per-linter capabilities (errcheck,
	// nakedret, etc.) need no config inputs from the pack rule, so requiring a
	// non-empty config here would force authors to write `config: {}` placeholder
	// blocks. The capability layer validates that required inputs (per the
	// capability YAML's inputs schema) are actually present at invocation time.

	// Optional version constraint on the capability. Validated for syntax
	// here; the actual capability lookup happens at registry load time so
	// manifest can stay free of engine dependencies.
	if v := strings.TrimSpace(r.Detector.Version); v != "" {
		if _, err := semver.NewConstraint(v); err != nil {
			errs = append(errs, ValidationError{
				File:       filePath,
				RuleID:     ruleID,
				Field:      prefix + ".detector.version",
				Message:    fmt.Sprintf("invalid semver constraint %q: %v", v, err),
				Suggestion: "use forms like \"1.0.0\", \"^1\", \"~1.2\", \">=1.2,<2\", or omit the field",
			})
		}
	}

	// applies_to.
	if len(r.AppliesTo) == 0 {
		errs = append(errs, ValidationError{
			File:    filePath,
			RuleID:  ruleID,
			Field:   prefix + ".applies_to",
			Message: "applies_to is required and must not be empty",
		})
	}

	// fix.message.
	if strings.TrimSpace(r.Fix.Message) == "" {
		errs = append(errs, ValidationError{
			File:    filePath,
			RuleID:  ruleID,
			Field:   prefix + ".fix.message",
			Message: "fix.message is required",
		})
	} else if len(r.Fix.Message) > 200 {
		errs = append(errs, ValidationError{
			File:    filePath,
			RuleID:  ruleID,
			Field:   prefix + ".fix.message",
			Message: fmt.Sprintf("fix.message exceeds 200 characters (got %d)", len(r.Fix.Message)),
		})
	}

	// Tests.
	for j, test := range r.Tests {
		testPrefix := fmt.Sprintf("%s.tests[%d]", prefix, j)
		if strings.TrimSpace(test.Name) == "" {
			errs = append(errs, ValidationError{
				File:    filePath,
				RuleID:  ruleID,
				Field:   testPrefix + ".name",
				Message: "test name is required",
			})
		}
		if test.Should != TestExpectFire && test.Should != TestExpectPass {
			errs = append(errs, ValidationError{
				File:       filePath,
				RuleID:     ruleID,
				Field:      testPrefix + ".should",
				Message:    fmt.Sprintf("invalid test expectation %q", test.Should),
				Suggestion: "use one of: fire, pass",
			})
		}
		if strings.TrimSpace(test.Code) == "" {
			errs = append(errs, ValidationError{
				File:    filePath,
				RuleID:  ruleID,
				Field:   testPrefix + ".code",
				Message: "test code is required",
			})
		}
	}

	return errs
}

func isValidID(id string) bool {
	return idPattern.MatchString(id)
}

func isValidSeverity(s Severity) bool {
	switch s {
	case SeverityError, SeverityWarning, SeverityInfo:
		return true
	}
	return false
}
