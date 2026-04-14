---
name: idiomatic
description: |
  Deterministic rule enforcement for this project's code. Activate when
  writing or editing code, when a hook has returned findings from
  idiomatic, when the user asks to check code against project rules,
  or when the user asks to set up idiomatic / code quality enforcement.
  If no .idiomatic.yaml exists, guide setup per plugin/CLAUDE.md.
---

# idiomatic: deterministic rule enforcement

This project uses `idiomatic` to enforce code quality rules. After every
file edit, a hook runs the analyzer and returns findings in your next
turn as `additionalContext`.

## Reading findings

Findings arrive in one of two shapes:

**SARIF 2.1.0** when the output fits the context budget. Findings live
in `runs[0].results`, each with `ruleId`, `level`, `message.text`,
`locations[0].physicalLocation` (file, line, column), and `fixes`.

**Compact summary** when SARIF exceeds the budget. A JSON object with
`summary` and a `findings` array. Each finding has `rule`, `severity`,
`file`, `line`, `column`, `message`, `fix`, and optional `fix_example`.
The full SARIF is at `summary.full_report_path` if you need more detail.

Both shapes contain the same information. The compact shape only omits
metadata that is not needed to fix code.

## Responding to findings

For each **error**-level finding:

1. Read `fix` (or `fixes[0].description.text` in SARIF) for the instruction.
2. Use `fix_example` as a reference if present.
3. Edit the file to apply the fix.
4. The hook runs again automatically and returns new findings or none.

**Warning**-level findings are advisory. Mention them; do not block on them.
**Info**-level findings are informational only.

## Iteration limit

If you have applied three rounds of fixes to the same file and error-level
findings remain, **stop** and explain the situation to the user. Persistent
findings after three rounds usually mean a rule conflicts with the user's
request, a fix instruction is ambiguous, or a design change is needed.
Surface the problem rather than thrashing.

## Empty findings

Zero findings means the code conforms. Proceed with the task.

## Errors

If the `additionalContext` contains an `error` field, surface it to the
user along with the `suggested_action` and stop. Do not try to work
around enforcement errors.
