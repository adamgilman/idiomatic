// SPDX-License-Identifier: Apache-2.0

// signal_list.go — List-mode signal parsing: gjson extraction, rule matching,
// and finding construction from JSON tool output.
//
//   - Supports flat lists (semgrep, gosec) and nested two-level lists (eslint)
//     via the NestedList + ParentFields spec. Parent fields are projected onto
//     inner items so findings can reference the outer object (e.g. filePath).
//   - Rule resolution uses four strategies: by_id (pack rule ID == extracted
//     value), by_input (match against a rule's input field), linter_contains
//     (primary match + substring disambiguation for tools like golangci-lint
//     where multiple rules share a linter name), and by_capability (every
//     finding maps to the first pack rule using this capability — used by
//     single-rule per-linter capabilities like errcheck).
//   - The tail_after_dot transform strips prefixes like "idio-rules.go-no-panic"
//     down to "go-no-panic" so semgrep check_ids map to pack rule IDs.
//   - The prefix_before_colon transform extracts the substring before the first
//     ":" (whitespace-trimmed), used by revive output like
//     "exported: should have a comment" to yield just "exported".
package declarative

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/adamgilman/idiomatic/internal/engine"
	"github.com/adamgilman/idiomatic/manifest"
	"github.com/tidwall/gjson"
)

// parseListSignal walks the captured tool output, extracts each result entry,
// resolves it back to a pack rule, and emits one engine.Finding per match.
func (c *Capability) parseListSignal(out *runOutput, rules []manifest.Rule) ([]engine.Finding, error) {
	if out == nil || len(out.Stdout) == 0 {
		return nil, nil
	}
	if c.spec.Spec.Signal.Format != "json" {
		return nil, fmt.Errorf("signal.format=%q not supported (only 'json')", c.spec.Spec.Signal.Format)
	}

	root := gjson.ParseBytes(out.Stdout)
	items := c.collectItems(root)
	resolver := newRuleResolver(c.spec.Spec.Signal.MatchRuleBy, rules)

	var findings []engine.Finding
	for _, item := range items {
		rule, ok := resolver.resolve(item.inner)
		if !ok {
			continue
		}
		findings = append(findings, c.signalItemToFinding(item, rule))
	}
	return findings, nil
}

// signalItem is one extracted entry from the tool's output. The inner field
// is the raw gjson result for the entry; parent (when set by NestedList)
// holds string-valued fields projected from the outer item. The resolver
// always reads rule-id paths from inner; itemToFinding can read either
// inner or parent (via "parent.<alias>" paths in spec.Signal.Fields).
type signalItem struct {
	inner  gjson.Result
	parent map[string]string
}

// collectItems walks the configured list and (optionally) nested_list paths,
// returning every item that should become a finding candidate. Without
// NestedList the result is one item per outer entry. With NestedList the
// result is one item per inner element, each tagged with its parent's
// projected fields.
func (c *Capability) collectItems(root gjson.Result) []signalItem {
	listPath := c.spec.Spec.Signal.List
	var outer []gjson.Result

	walkInto := func(parent gjson.Result) []gjson.Result {
		var out []gjson.Result
		parent.ForEach(func(_, v gjson.Result) bool {
			out = append(out, v)
			return true
		})
		return out
	}

	if listPath == "" {
		outer = walkInto(root)
	} else {
		path := strings.TrimSuffix(listPath, ".#")
		outer = walkInto(root.Get(path))
	}

	if c.spec.Spec.Signal.NestedList == "" {
		items := make([]signalItem, 0, len(outer))
		for _, o := range outer {
			items = append(items, signalItem{inner: o})
		}
		return items
	}

	// Two-level walk: project parent fields onto each inner item.
	nestedPath := strings.TrimSuffix(c.spec.Spec.Signal.NestedList, ".#")
	var items []signalItem
	for _, o := range outer {
		parent := map[string]string{}
		for alias, gpath := range c.spec.Spec.Signal.ParentFields {
			parent[alias] = o.Get(gpath).String()
		}
		o.Get(nestedPath).ForEach(func(_, v gjson.Result) bool {
			items = append(items, signalItem{inner: v, parent: parent})
			return true
		})
	}
	return items
}

// signalItemToFinding extracts the finding fields from a signalItem. When
// the item carries parent context, fields can be resolved against either
// the inner JSON (default) or the parent (via "parent.<alias>" paths in
// SignalSpec.Fields).
func (c *Capability) signalItemToFinding(item signalItem, rule manifest.Rule) engine.Finding {
	get := func(field string) gjson.Result {
		path, ok := c.spec.Spec.Signal.Fields[field]
		if !ok {
			return gjson.Result{}
		}
		// Parent-prefixed paths read from the projected parent map.
		if strings.HasPrefix(path, "parent.") && len(item.parent) > 0 {
			alias := strings.TrimPrefix(path, "parent.")
			if v, ok := item.parent[alias]; ok {
				return gjson.Parse(`"` + jsonEscape(v) + `"`)
			}
		}
		return item.inner.Get(path)
	}

	severity := string(rule.Severity)
	if sev := get("severity").String(); sev != "" {
		severity = mapSeverity(sev, c.spec.Spec.Signal.SeverityMap)
	}

	startLine := int(get("line").Int())
	startCol := int(get("col").Int())
	endLine := int(get("end_line").Int())
	endCol := int(get("end_col").Int())
	if endLine == 0 {
		endLine = startLine
	}
	if endCol == 0 {
		endCol = startCol
	}

	return engine.Finding{
		RuleID:     rule.ID,
		Severity:   severity,
		File:       get("file").String(),
		StartLine:  startLine,
		StartCol:   startCol,
		EndLine:    endLine,
		EndCol:     endCol,
		Message:    get("message").String(),
		FixMessage: rule.Fix.Message,
		FixExample: rule.Fix.Example,
	}
}

// jsonEscape escapes a string for embedding inside a JSON literal so we can
// build a gjson.Result for parent-projected string values.
func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	// json.Marshal returns the quoted form ("..."); strip the quotes.
	if len(b) >= 2 && b[0] == '"' && b[len(b)-1] == '"' {
		return string(b[1 : len(b)-1])
	}
	return s
}

// ruleResolver maps a tool-output item to a pack rule using one of the
// configured strategies. It is built once per Analyze call and reused across
// items.
type ruleResolver struct {
	spec  *MatchRuleBy
	rules []manifest.Rule

	// Strategy: by_id — pack rule id == extracted value (with optional transform).
	// Strategy: by_input — extracted value matches rule.Inputs[Field].
	// Strategy: linter_contains — first try by_input, then disambiguate by
	// substring-matching SubruleField against the item's MatchIn.
	// Strategy: by_capability — every finding maps to the first rule (no item
	// extraction; used by single-rule per-linter capabilities).
	indexByID    map[string]manifest.Rule
	indexByInput map[string][]manifest.Rule
}

func newRuleResolver(spec *MatchRuleBy, rules []manifest.Rule) *ruleResolver {
	r := &ruleResolver{spec: spec, rules: rules}
	r.indexByID = make(map[string]manifest.Rule, len(rules))
	r.indexByInput = make(map[string][]manifest.Rule)
	for _, rule := range rules {
		r.indexByID[rule.ID] = rule
		if spec != nil && spec.Field != "" {
			if v, ok := rule.Detector.Config[spec.Field].(string); ok && v != "" {
				r.indexByInput[v] = append(r.indexByInput[v], rule)
			}
		}
	}
	return r
}

func (r *ruleResolver) resolve(item gjson.Result) (manifest.Rule, bool) {
	if r.spec == nil {
		return manifest.Rule{}, false
	}

	// Strategies that don't need an extracted value.
	if r.spec.Strategy == "by_capability" {
		if len(r.rules) > 0 {
			return r.rules[0], true
		}
		return manifest.Rule{}, false
	}

	raw := item.Get(r.spec.From).String()
	if raw == "" {
		return manifest.Rule{}, false
	}
	val := transformRuleID(raw, r.spec.Transform)

	switch r.spec.Strategy {
	case "", "by_id":
		rule, ok := r.indexByID[val]
		return rule, ok
	case "by_input":
		matches := r.indexByInput[val]
		if len(matches) == 0 {
			return manifest.Rule{}, false
		}
		return matches[0], true
	case "linter_contains":
		matches := r.indexByInput[val]
		if len(matches) == 0 {
			return manifest.Rule{}, false
		}
		if len(matches) == 1 || r.spec.MatchIn == "" || r.spec.SubruleField == "" {
			return matches[0], true
		}
		text := strings.ToLower(item.Get(r.spec.MatchIn).String())
		for _, m := range matches {
			sub, _ := m.Detector.Config[r.spec.SubruleField].(string)
			if sub == "" {
				continue
			}
			if strings.Contains(text, strings.ToLower(sub)) {
				return m, true
			}
		}
		return matches[0], true
	default:
		return manifest.Rule{}, false
	}
}

func transformRuleID(raw, transform string) string {
	switch transform {
	case "", "identity":
		return raw
	case "tail_after_dot":
		if idx := strings.LastIndex(raw, "."); idx >= 0 {
			return raw[idx+1:]
		}
		return raw
	case "prefix_before_colon":
		if idx := strings.IndexByte(raw, ':'); idx >= 0 {
			return strings.TrimSpace(raw[:idx])
		}
		return raw
	default:
		return raw
	}
}

// mapSeverity translates an upstream severity using the spec's severity_map.
func mapSeverity(raw string, m map[string]string) string {
	if mapped, ok := m[raw]; ok {
		return mapped
	}
	switch strings.ToUpper(raw) {
	case "ERROR", "HIGH":
		return "error"
	case "WARNING", "WARN", "MEDIUM":
		return "warning"
	case "INFO", "LOW":
		return "info"
	}
	return strings.ToLower(raw)
}
