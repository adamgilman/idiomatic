// SPDX-License-Identifier: Apache-2.0

// firewhen.go — Hand-rolled recursive-descent parser and evaluator for the
// fire_when expression language used by scalar-mode capabilities.
//
//   - The grammar is intentionally tiny: comparisons on signal.<field> with
//     ==, !=, matches (regex), contains, plus not/and/or/parens combinators.
//   - Template expansion runs first (sprig + rule inputs), so RHS literals
//     can interpolate rule config like {{ .Inputs.pattern }}.
//   - Every scalar rule MUST provide a fire_when expression; omitting it is
//     a hard error, not a default-to-fire. This prevents accidental findings.
//   - The parser was hand-rolled (no cel-go or expr dependency) because the
//     grammar is small enough that a library would be overkill.
package declarative

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/template"
)

// fire_when expression language (intentionally tiny):
//
//   signal.<field> == "literal"
//   signal.<field> != "literal"
//   signal.<field> matches "regex"
//   signal.<field> contains "literal"
//   signal.<field> == 0           (numeric for exit_code)
//   signal.<field> != 0
//   not <expr>
//   <expr> and <expr>
//   <expr> or  <expr>
//   ( <expr> )
//
// Templates run on the literal RHS first using rule context (so a rule can
// say `signal.stdout matches "{{ .Inputs.pattern }}"`).
//
// The grammar is parsed by hand because the ruleset is small enough that
// pulling in cel-go or expr would be overkill.

// evalFireWhen looks up the active rule's fire_when expression (in the
// rule's inputs as the "fire_when" field) and evaluates it against the
// scalar signal. If no fire_when is set, the rule never fires.
func evalFireWhen(ruleInputs map[string]any, signal scalarSignal) (bool, error) {
	expr, ok := ruleInputs["fire_when"].(string)
	if !ok || strings.TrimSpace(expr) == "" {
		// No fire_when on the rule means the rule never produces a finding.
		// Scalar capabilities require fire_when to be authored.
		return false, fmt.Errorf("scalar rule requires inputs.fire_when expression")
	}

	// Render the expression with sprig + rule input context first, so a
	// regex literal can interpolate {{ .Inputs.pattern }}.
	rendered, err := renderFireWhen(expr, ruleInputs)
	if err != nil {
		return false, fmt.Errorf("render fire_when: %w", err)
	}

	parsed, err := parseFireWhen(rendered)
	if err != nil {
		return false, fmt.Errorf("parse fire_when %q: %w", rendered, err)
	}
	return parsed.eval(signal)
}

func renderFireWhen(expr string, inputs map[string]any) (string, error) {
	if !strings.Contains(expr, "{{") {
		return expr, nil
	}
	t, err := template.New("fw").Funcs(templateFuncs()).Parse(expr)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	view := struct{ Inputs map[string]any }{Inputs: inputs}
	if err := t.Execute(&buf, view); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// node is the AST type for a parsed fire_when expression.
type node interface {
	eval(s scalarSignal) (bool, error)
}

type cmpNode struct {
	field string
	op    string // ==, !=, matches, contains
	value string
	num   bool   // true if value is numeric
	numV  int64
}

func (n cmpNode) eval(s scalarSignal) (bool, error) {
	raw, ok := s.Fields[n.field]
	if !ok {
		return false, nil
	}
	switch n.op {
	case "==":
		if n.num {
			i, err := toInt(raw)
			if err != nil {
				return false, nil
			}
			return i == n.numV, nil
		}
		return fmt.Sprint(raw) == n.value, nil
	case "!=":
		if n.num {
			i, err := toInt(raw)
			if err != nil {
				return true, nil
			}
			return i != n.numV, nil
		}
		return fmt.Sprint(raw) != n.value, nil
	case "matches":
		re, err := regexp.Compile(n.value)
		if err != nil {
			return false, fmt.Errorf("regex %q: %w", n.value, err)
		}
		return re.MatchString(fmt.Sprint(raw)), nil
	case "contains":
		return strings.Contains(fmt.Sprint(raw), n.value), nil
	default:
		return false, fmt.Errorf("unknown operator %q", n.op)
	}
}

type notNode struct{ inner node }

func (n notNode) eval(s scalarSignal) (bool, error) {
	v, err := n.inner.eval(s)
	if err != nil {
		return false, err
	}
	return !v, nil
}

type andNode struct{ left, right node }

func (n andNode) eval(s scalarSignal) (bool, error) {
	l, err := n.left.eval(s)
	if err != nil || !l {
		return false, err
	}
	return n.right.eval(s)
}

type orNode struct{ left, right node }

func (n orNode) eval(s scalarSignal) (bool, error) {
	l, err := n.left.eval(s)
	if err == nil && l {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return n.right.eval(s)
}

func toInt(v any) (int64, error) {
	switch x := v.(type) {
	case int:
		return int64(x), nil
	case int64:
		return x, nil
	case string:
		return strconv.ParseInt(strings.TrimSpace(x), 10, 64)
	default:
		return strconv.ParseInt(fmt.Sprint(x), 10, 64)
	}
}

// parseFireWhen is a hand-rolled recursive-descent parser for the tiny
// fire_when grammar. It is forgiving about whitespace and case-insensitive
// for keywords.
func parseFireWhen(input string) (node, error) {
	p := &fwParser{src: input}
	p.skipWS()
	n, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	p.skipWS()
	if p.pos != len(p.src) {
		return nil, fmt.Errorf("trailing input at position %d: %q", p.pos, p.src[p.pos:])
	}
	return n, nil
}

type fwParser struct {
	src string
	pos int
}

func (p *fwParser) parseOr() (node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for {
		p.skipWS()
		if !p.matchKeyword("or") {
			return left, nil
		}
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = orNode{left, right}
	}
}

func (p *fwParser) parseAnd() (node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		p.skipWS()
		if !p.matchKeyword("and") {
			return left, nil
		}
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = andNode{left, right}
	}
}

func (p *fwParser) parseUnary() (node, error) {
	p.skipWS()
	if p.matchKeyword("not") {
		inner, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return notNode{inner}, nil
	}
	if p.peek() == '(' {
		p.pos++
		n, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		p.skipWS()
		if p.peek() != ')' {
			return nil, fmt.Errorf("missing ')' at %d", p.pos)
		}
		p.pos++
		return n, nil
	}
	return p.parseCmp()
}

func (p *fwParser) parseCmp() (node, error) {
	p.skipWS()
	if !strings.HasPrefix(p.src[p.pos:], "signal.") {
		return nil, fmt.Errorf("expected 'signal.<field>' at %d, got %q", p.pos, p.src[p.pos:])
	}
	p.pos += len("signal.")
	field := p.consumeIdent()
	if field == "" {
		return nil, fmt.Errorf("expected field name at %d", p.pos)
	}
	p.skipWS()

	var op string
	switch {
	case strings.HasPrefix(p.src[p.pos:], "=="):
		op = "=="
		p.pos += 2
	case strings.HasPrefix(p.src[p.pos:], "!="):
		op = "!="
		p.pos += 2
	case p.matchKeyword("matches"):
		op = "matches"
	case p.matchKeyword("contains"):
		op = "contains"
	default:
		return nil, fmt.Errorf("expected operator at %d, got %q", p.pos, p.src[p.pos:])
	}
	p.skipWS()

	// Value: either a quoted string or a bare integer.
	if p.peek() == '"' || p.peek() == '\'' {
		val, err := p.consumeString()
		if err != nil {
			return nil, err
		}
		return cmpNode{field: field, op: op, value: val}, nil
	}
	if isDigit(p.peek()) || p.peek() == '-' {
		num := p.consumeNumber()
		i, err := strconv.ParseInt(num, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("bad number %q: %w", num, err)
		}
		return cmpNode{field: field, op: op, num: true, numV: i}, nil
	}
	return nil, fmt.Errorf("expected literal value at %d, got %q", p.pos, p.src[p.pos:])
}

func (p *fwParser) skipWS() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t' || p.src[p.pos] == '\n') {
		p.pos++
	}
}

func (p *fwParser) peek() byte {
	if p.pos >= len(p.src) {
		return 0
	}
	return p.src[p.pos]
}

func (p *fwParser) matchKeyword(kw string) bool {
	if p.pos+len(kw) > len(p.src) {
		return false
	}
	if !strings.EqualFold(p.src[p.pos:p.pos+len(kw)], kw) {
		return false
	}
	// Word boundary check.
	if p.pos+len(kw) < len(p.src) {
		next := p.src[p.pos+len(kw)]
		if isIdentChar(next) {
			return false
		}
	}
	p.pos += len(kw)
	return true
}

func (p *fwParser) consumeIdent() string {
	start := p.pos
	for p.pos < len(p.src) && isIdentChar(p.src[p.pos]) {
		p.pos++
	}
	return p.src[start:p.pos]
}

func (p *fwParser) consumeNumber() string {
	start := p.pos
	if p.peek() == '-' {
		p.pos++
	}
	for p.pos < len(p.src) && isDigit(p.src[p.pos]) {
		p.pos++
	}
	return p.src[start:p.pos]
}

func (p *fwParser) consumeString() (string, error) {
	quote := p.src[p.pos]
	p.pos++
	var buf bytes.Buffer
	for p.pos < len(p.src) {
		ch := p.src[p.pos]
		if ch == '\\' && p.pos+1 < len(p.src) {
			next := p.src[p.pos+1]
			buf.WriteByte(next)
			p.pos += 2
			continue
		}
		if ch == quote {
			p.pos++
			return buf.String(), nil
		}
		buf.WriteByte(ch)
		p.pos++
	}
	return "", fmt.Errorf("unterminated string starting at %d", p.pos)
}

func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}
