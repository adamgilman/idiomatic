# Authoring a Rule Pack

> A step-by-step walkthrough that builds a working rule pack from scratch.
> Audience: anyone who wants to enforce a coding convention — for an
> internal team, a community library, or a personal project.

This guide builds a small Go pack with two rules. By the end you'll have a
YAML file you can run with `idio scan` and ship to other developers.

For the formal field reference, see [`spec/rule-pack.md`](../spec/rule-pack.md).

## Prerequisites

- The rule pack YAML format is [documented in the spec](../spec/rule-pack.md). Skim it first.
- An editor with [`yaml-language-server`](https://github.com/redhat-developer/yaml-language-server) is highly recommended — autocomplete and inline validation make rule writing much easier.
- The capability your rules will target must be declared in your `.idiomatic.yaml` config. The official capabilities (semgrep, eslint, gosec, gitleaks, git, file-exists, file-contains, and per-linter Go capabilities like revive, errcheck, gocritic, ...) ship with the official idiomatic repo at `https://github.com/adamgilman/idiomatic`. Custom or community capabilities are referenced the same way — add their repo and path to your config.
- You'll be testing against real source files, so install the underlying tool too. For this guide we'll use semgrep — install it with `uv tool install semgrep` (or `brew install semgrep`).

## Step 1: Pick a convention to enforce

A rule should encode a conviction: "in this codebase, X is wrong / Y is required, and here's why." Skip rules that are matters of taste — those belong in a formatter or a code review checklist, not in idiomatic.

For this walkthrough we'll enforce two Go conventions:

1. **No `fmt.Println` in production code** — production code should use a structured logger.
2. **No `panic()` outside `main()` and `init()`** — Go's idiomatic error handling uses explicit error returns.

Both are pattern matches, so we'll use the `semgrep` capability for both.

## Step 2: Pick the right capability

Decide which capability runs each rule. The match is usually obvious from the kind of analysis:

| If the rule is... | Use |
|---|---|
| Semantic pattern matching in source code | `semgrep` |
| ECMAScript/TypeScript/React linting | `eslint` |
| Go linting (multi-rule: revive, gocritic) | `revive`, `gocritic` (per-linter capability) |
| Go linting (single-rule: errcheck, nakedret, ...) | `errcheck`, `nakedret`, ... (per-linter capability) |
| Go security analysis | `gosec` |
| Secret scanning | `gitleaks` |
| Git repository state (current branch, worktree, ...) | `git` |
| Required file presence | `file-exists` |
| Required file content | `file-contains` |

Each capability has its own [input contract](../spec/rule-pack.md#capability-config-shape). Look at an existing rule that uses the same capability and copy its `detector.config` shape.

## Step 3: Sketch the pack file

Create `my-go-pack.yaml`:

```yaml
# yaml-language-server: $schema=../../schemas/rule-pack.schema.json
apiVersion: rules.idiomatic.dev/v1alpha1
kind: RulePack

pack:
  id: my-go-pack
  name: My Go Pack
  version: 0.1.0
  description: Internal Go conventions for the my-team org.
  maintainer: my-team
  license: MIT

rules:
  # Rules go here.
```

`pack.id` is the routing key for this pack — lowercase, hyphenated, **permanent**. `pack.version` is independent of any rule version; bump it on every release of the pack.

The `# yaml-language-server` comment at the top wires up your editor for autocomplete and inline validation.

## Step 4: Write the first rule

Open the [semgrep input reference](../spec/rule-pack.md#capability-config-shape) and copy the shape:

```yaml
detector:
  capability: semgrep
  version: "^1"
  config:
    language: go
    pattern: fmt.Println(...)
```

Now wrap it in a full rule with all the required fields:

```yaml
rules:
  - id: go-no-fmt-println
    name: No fmt.Println outside main packages
    description: Flags any call to fmt.Println in non-main packages.
    rationale: |
      Production code should use a structured logger, not fmt.Println.
      Logger calls carry context (level, fields, sampler) that fmt.Println loses.
    severity: error
    tags: [logging, go]
    detector:
      capability: semgrep
      version: "^1"
      config:
        language: go
        pattern: fmt.Println(...)
    applies_to:
      - "**/*.go"
    excludes:
      - "**/*_test.go"
      - "**/testdata/**"
      - "**/vendor/**"
    fix:
      message: Replace fmt.Println with a structured logger call.
      example: |
        // Before
        fmt.Println("request received")

        // After
        logger.Info("request received", "method", r.Method)
    references:
      - https://go.dev/blog/slog
    tests:
      - name: fmt.Println in handler fires
        should: fire
        code: |
          package handler
          import "fmt"
          func Handle() { fmt.Println("hello") }
      - name: logger call passes
        should: pass
        code: |
          package handler
          func Handle(logger Logger) { logger.Info("hello") }
```

A few things to notice:

- **`id`** — lowercase, hyphenated, prefixed with `go-`. Once published, never rename.
- **`severity: error`** — pick `error` for must-fix, `warning` for should-fix, `info` for awareness.
- **`detector.version: "^1"`** — pin to the major version of the capability. If semgrep capability ever bumps to `2.0.0` with a breaking change, this rule fails loudly instead of silently producing wrong findings.
- **`applies_to`** + **`excludes`** — file globs. `**/*` is recursive; `**/*.go` matches every Go file.
- **`fix.message`** — actionable instruction, max 200 chars. The user gets this when the rule fires.
- **`tests`** — strongly recommended. They double as documentation and as a regression suite.

## Step 5: Write the second rule

The panic rule needs a compound semgrep pattern. Look at the [semgrep capability inputs](../spec/rule-pack.md#capability-config-shape) and use `patterns` (plural) instead of `pattern`:

```yaml
- id: go-no-panic
  name: No panic() outside main and init
  description: Flags panic() in any function that is not main() or init().
  rationale: |
    Go's idiomatic error handling uses explicit error returns rather than
    panics. Production code should propagate errors so callers can decide
    how to recover.
  severity: error
  tags: [errors, go]
  detector:
    capability: semgrep
    version: "^1"
    config:
      language: go
      patterns:
        - pattern: panic(...)
        - pattern-not-inside: |
            func main() { ... }
        - pattern-not-inside: |
            func init() { ... }
  applies_to:
    - "**/*.go"
  excludes:
    - "**/*_test.go"
    - "**/vendor/**"
  fix:
    message: Return an error instead of panicking.
    example: |
      // Before
      func Connect(addr string) *Conn {
          c, err := dial(addr)
          if err != nil { panic(err) }
          return c
      }

      // After
      func Connect(addr string) (*Conn, error) {
          return dial(addr)
      }
  tests:
    - name: panic in handler fires
      should: fire
      code: |
        package handler
        func Handle() { panic("nope") }
    - name: panic in main passes
      should: pass
      code: |
        package main
        func main() { panic("ok at startup") }
```

## Step 6: Validate the YAML

If your editor uses `yaml-language-server`, the schema check is already
running in the background (red underlines on errors, autocomplete on
known fields).

For a one-shot validation:

```bash
python3 -c '
import json, yaml, sys
from jsonschema import Draft202012Validator
schema = json.load(open("schemas/rule-pack.schema.json"))
doc = yaml.safe_load(open(sys.argv[1]))
errs = list(Draft202012Validator(schema).iter_errors(doc))
for e in errs: print(f"{list(e.absolute_path)}: {e.message}")
sys.exit(1 if errs else 0)' my-go-pack.yaml
```

## Step 7: Run the pack against real code

First, create a `.idiomatic.yaml` that references the semgrep capability and your new pack. For local development, use `--config` to point at a test config:

```yaml
# test-config.yaml
apiVersion: rules.idiomatic.dev/v1alpha1
kind: ProjectConfig

capabilities:
  - repo: https://github.com/adamgilman/idiomatic
    path:
      - capabilities/semgrep.yaml

packs:
  - repo: https://github.com/adamgilman/idiomatic
    path:
      - packs/go-starter.yaml
```

Then create a test file and scan:

```bash
cat > /tmp/handler.go <<'EOF'
package handler

import "fmt"

func Handle() {
    fmt.Println("hello")
    if err := connect(); err != nil {
        panic(err)
    }
}

func connect() error { return nil }
EOF

idio scan --config test-config.yaml /tmp/handler.go
```

Expected output:

```
/tmp/handler.go:6:5: error: Replace fmt.Println with a structured logger call. [go-no-fmt-println]
/tmp/handler.go:8:9: error: Return an error instead of panicking. [go-no-panic]
```

Two findings. Both the right rule, both the right line, both the right
fix message.

If you get `capability "semgrep" binary is not installed`, install semgrep
and retry. The pack itself loaded fine — only the underlying tool is missing.

If you get a `version constraint` error, the loaded semgrep capability is
on a different major than `^1`. Either update the constraint or use the
matching capability version.

## Step 8: Run the pack against your inline tests

The inline `tests:` blocks are not (yet) automatically executed by `idio
scan`, but they document expected behavior and serve as the basis for a
regression test suite. Treat them as living documentation: every rule
should have at least one `fire` test and one `pass` test that exercise
the core distinction the rule encodes.

## Step 9: Iterate on rationale and fix messages

The most common gap in early packs is weak rationale and weak fix messages.
A rule that fires without an actionable next step is a worse experience
than no rule at all. Two rules of thumb:

- **Rationale** should answer "why does this rule exist?" in one paragraph. Imagine you're explaining to a new hire why your team made this call. Include the failure mode you're protecting against.
- **Fix message** should answer "what do I do right now?" in one sentence (max 200 chars). If the answer is more than one sentence, put the rest in `fix.example`.

Bad:

```yaml
rationale: We don't allow this.
fix:
  message: Don't do that.
```

Good:

```yaml
rationale: |
  Logger calls carry context (level, structured fields, sampler) that
  fmt.Println loses. Production code consuming logs needs that context to
  filter, route, and aggregate effectively.
fix:
  message: Replace fmt.Println with a structured logger (slog, zap, zerolog).
```

## Step 10: Commit the pack

A rule pack is data. Commit it to your repo alongside the code it governs:

```bash
git add my-go-pack.yaml
git commit -m "Add internal Go conventions pack"
```

Other developers add the pack to their `.idiomatic.yaml` and run `idio scan`.

## Publishing

To publish a pack for community use, push it to a public git repo:

```
my-go-pack/
├── packs/
│   └── go-conventions.yaml
└── README.md
```

Consumers add it to their `.idiomatic.yaml`:

```yaml
packs:
  - repo: https://github.com/me/my-go-pack
    path:
      - packs/go-conventions.yaml
```

For private packs, push to a private git repo and rely on git credential helpers.

## Common patterns

### A rule that uses a custom capability

The `detector.capability` field accepts any name, not just the canonical 8.
If your rule targets a community or private capability, add it to the
`capabilities:` section of `.idiomatic.yaml`:

```yaml
capabilities:
  - repo: https://github.com/me/my-capabilities
    path:
      - capabilities/my-tool.yaml

packs:
  - repo: https://github.com/me/my-pack
    path:
      - packs/my-pack.yaml
```

The pack and the capability are loaded by the same mechanism — there's no
distinction at runtime between official, community, and private.

### A rule that fires on git state (no file pattern)

For repository-level checks, use `git`/`file-exists`/`file-contains`:

```yaml
- id: no-work-on-main
  name: Don't commit directly to main
  description: Fires when the current branch is main or master.
  rationale: |
    Direct commits to the default branch bypass code review. Always create
    a feature branch.
  severity: warning
  detector:
    capability: git
    version: "^1"
    config:
      argv: [branch, --show-current]
      fire_when: 'signal.stdout matches "^(main|master)$"'
  applies_to: ["**/*"]
  fix:
    message: Create a feature branch (git switch -c feat/your-feature).
```

The `fire_when` expression is documented in the [rule pack spec](../spec/rule-pack.md#fire_when-expressions).

### A rule with a compound pattern

Use `patterns` (plural) and combine `pattern`, `pattern-not-inside`,
`metavariable-regex`, etc. See the [semgrep docs](https://semgrep.dev/docs/writing-rules/rule-syntax/)
for the full pattern grammar.

```yaml
detector:
  capability: semgrep
  version: "^1"
  config:
    language: go
    patterns:
      - pattern: log.$METHOD(...)
      - pattern-not-inside: func main() { ... }
      - metavariable-regex:
          metavariable: "$METHOD"
          regex: "^(Print|Fatal|Panic)"
```

### A rule that's specific to one file path

`applies_to` accepts arbitrary glob patterns. To target a single file, list
it explicitly:

```yaml
applies_to:
  - "Dockerfile"
  - "docker/Dockerfile.*"
```

## Versioning your pack

Pick a semver in `pack.version` and bump it on every release. For most
packs, simple decisions:

| Bump | When |
|---|---|
| Patch | Tweak a fix message, fix a typo, tighten a regex |
| Minor | Add a new rule, deprecate a rule (without removing it) |
| Major | Remove a rule, rename a rule (avoid this — see below), change a rule's semantics |

Rule IDs are **permanent**. If you need to retire a rule, leave it in the
pack with `severity: info` and a message that points at the replacement.
Renaming a rule is a breaking change that orphans every consumer that
silenced or pinned to the old id.

## Pinning capability versions

Every rule should pin its capability version. The pin protects against
silent breakage when the capability bumps to a new major:

```yaml
detector:
  capability: semgrep
  version: "^1"      # this rule was authored against semgrep capability 1.x
  config: { ... }
```

If you don't pin, the rule matches any loaded version — fine for
prototyping, dangerous for production. The `^MAJOR` form is the most common
choice: it lets the capability evolve within a major (new optional fields,
bug fixes) while protecting against renames and removals.

## Debugging tips

- **Schema errors at load**: run the YAML through the JSON Schema validator (Step 6 above). The error path tells you exactly which field is wrong.
- **Wrong rule fires**: check `match_rule_by` on the capability. For most capabilities the rule id and the tool's emitted id must match exactly. For capabilities using `by_input`, the rule's `config.<field>` must match the tool's emitted id.
- **No findings on code that should fail**: your `applies_to` glob might be filtering out the file. Try `applies_to: ["**/*"]` temporarily to confirm the rule itself works.
- **Fire-when expression doesn't match**: the LHS field name (`signal.stdout`, `signal.exit_code`) must come from the capability's `signal.scalar_fields` declaration. The RHS string is rendered as a Go template against `.Inputs` first, then matched against the field.
- **Unexpected findings on test files**: add `**/*_test.go` (or your project's test pattern) to `excludes`.

## Where to learn more

- [`spec/rule-pack.md`](../spec/rule-pack.md) — formal field reference for every field in the rule pack format.
- [`spec/capability.md`](../spec/capability.md) — formal field reference for the capability format. Read this if you're authoring a new capability.
- [`schemas/rule-pack.schema.json`](../../schemas/rule-pack.schema.json) — JSON Schema for IDE validation and CI gating.
- The canonical packs in [`packs/`](../../packs/) are real, in-production packs. Reading them is the fastest way to learn idiomatic patterns. Reference them in your `.idiomatic.yaml` via the official repo URL.
