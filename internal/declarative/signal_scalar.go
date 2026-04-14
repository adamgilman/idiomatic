// SPDX-License-Identifier: Apache-2.0

// signal_scalar.go — Scalar-mode signal parsing: per-rule tool invocation,
// fire_when evaluation, and finding construction.
//
//   - Each rule is run independently (per:rule), and its output is projected
//     into a scalarSignal with named fields (stdout, stderr, exit_code, plus
//     any declared in scalar_fields). The fire_when expression decides firing.
//   - Per-rule exec failures are silently skipped (not fatal), matching legacy
//     repocheck behavior. Only fire_when parse errors are loud.
//   - Scalar findings use fixed_file (templated against rule inputs and project
//     root) instead of file-level context, since these capabilities (git,
//     file-exists) don't produce per-line locations.
package declarative

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/adamgilman/idiomatic/internal/engine"
	"github.com/adamgilman/idiomatic/manifest"
)

// scalarSignal is the projected per-rule view fed to fire_when expressions.
type scalarSignal struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Fields   map[string]any
}

// analyzeScalar handles per:rule scalar-mode capabilities (git, file-exists,
// file-contains). For each rule it runs the tool, projects the output into
// a scalarSignal, evaluates the rule's fire_when expression, and emits one
// finding when the predicate matches.
func (c *Capability) analyzeScalar(ctx context.Context, req engine.AnalysisRequest) ([]engine.Finding, error) {
	results, err := c.runPerRule(ctx, req)
	if err != nil {
		return nil, err
	}

	var findings []engine.Finding
	for _, r := range results {
		if r.Err != nil {
			// Treat per-rule exec failures as silent skips, matching legacy
			// repocheck behavior. The capability's ok_exit policy already
			// catches everything we care about; remaining errors are e.g.
			// "not in a git repo".
			continue
		}
		if r.Out == nil {
			continue
		}
		signal := buildScalarSignal(r.Out, c.spec.Spec.Signal.ScalarFields)
		fired, err := evalFireWhen(r.Rule.Detector.Config, signal)
		if err != nil {
			// fire_when parse errors are loud — they signal an authoring bug.
			return nil, err
		}
		if !fired {
			continue
		}
		findings = append(findings, c.scalarFinding(r.Rule, signal, req.Files))
	}
	return findings, nil
}

// buildScalarSignal projects raw runOutput into named fields per the spec.
func buildScalarSignal(out *runOutput, fields map[string]ScalarField) scalarSignal {
	s := scalarSignal{
		Stdout:   string(out.Stdout),
		Stderr:   string(out.Stderr),
		ExitCode: out.ExitCode,
		Fields:   make(map[string]any, len(fields)),
	}
	for name, f := range fields {
		var raw string
		switch f.From {
		case "stdout":
			raw = string(out.Stdout)
		case "stderr":
			raw = string(out.Stderr)
		case "exit":
			s.Fields[name] = out.ExitCode
			continue
		default:
			raw = ""
		}
		switch f.Transform {
		case "trim":
			raw = strings.TrimSpace(raw)
		}
		s.Fields[name] = raw
	}
	// Always also expose the standard names so fire_when can use them
	// without enumerating in scalar_fields.
	if _, ok := s.Fields["stdout"]; !ok {
		s.Fields["stdout"] = strings.TrimSpace(s.Stdout)
	}
	if _, ok := s.Fields["stderr"]; !ok {
		s.Fields["stderr"] = strings.TrimSpace(s.Stderr)
	}
	if _, ok := s.Fields["exit_code"]; !ok {
		s.Fields["exit_code"] = s.ExitCode
	}
	return s
}

// scalarFinding builds a finding for a scalar-mode rule that fired. Scalar
// capabilities don't have file context per se — we use either the spec's
// fixed_file template (resolved against rule + project) or the first scanned
// file as the location.
func (c *Capability) scalarFinding(rule manifest.Rule, signal scalarSignal, files []string) engine.Finding {
	file := c.spec.Spec.Signal.FixedFile
	if file == "" && len(files) > 0 {
		file = files[0]
	}
	if file != "" {
		root := projectRoot(files)
		file = resolveScalarPath(file, root, rule)
	}

	msg := rule.Fix.Message
	if signal.Stdout != "" {
		msg = "(" + strings.TrimSpace(signal.Stdout) + ") " + msg
	}

	return engine.Finding{
		RuleID:     rule.ID,
		Severity:   string(rule.Severity),
		File:       file,
		StartLine:  1,
		Message:    msg,
		FixMessage: rule.Fix.Message,
		FixExample: rule.Fix.Example,
	}
}

// resolveScalarPath substitutes {project} and {{ .Inputs.<field> }} tokens in
// the spec's fixed_file path against the active rule and project root.
func resolveScalarPath(path, project string, rule manifest.Rule) string {
	// Substitute literal {project} first.
	path = strings.ReplaceAll(path, "{project}", project)
	// Then run a Go template if there are template markers.
	if strings.Contains(path, "{{") {
		t, err := template.New("path").Parse(path)
		if err != nil {
			return filepath.Clean(path)
		}
		var buf bytes.Buffer
		view := struct{ Inputs map[string]any }{Inputs: rule.Detector.Config}
		if err := t.Execute(&buf, view); err == nil {
			path = buf.String()
		}
	}
	return filepath.Clean(path)
}
