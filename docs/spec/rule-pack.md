# Rule Pack Specification

> Reference for the YAML format used to describe an idiomatic rule pack.
> Audience: anyone authoring rules — for the official packs, a community
> registry, or a private one.

The rule pack YAML format is a public specification. Anyone can author packs
— community contributors, companies writing internal rules, individuals
enforcing conventions for their own projects. All packs use the same schema,
the same validation, and the same scanner regardless of source.

This document is the **reference** for the format. For a step-by-step
walkthrough that builds a new pack from scratch, see
[`authoring/rule-pack.md`](../authoring/rule-pack.md).

## Status and stability

| Property | Value |
|---|---|
| Schema name | `rules.idiomatic.dev/v1alpha1` |
| JSON Schema | [`schemas/rule-pack.schema.json`](../../schemas/rule-pack.schema.json) |
| Stability | Alpha. Breaking changes bump the schema name (`v1alpha2`, `v1beta1`, ...). Backwards-compatible additions do not. |
| Versioning | Each pack YAML declares its **own** semver in `pack.version`. |

## Editor support

Add this comment as the first line of any rule pack YAML and your editor
will validate the document and offer autocomplete (requires
[`yaml-language-server`](https://github.com/redhat-developer/yaml-language-server)):

```yaml
# yaml-language-server: $schema=../../schemas/rule-pack.schema.json
```

For local development inside this repo, point at the relative path instead:

```yaml
# yaml-language-server: $schema=../schemas/rule-pack.schema.json
```

## Validating outside an editor

```bash
# Python — pip install jsonschema pyyaml
python3 -c '
import json, yaml, sys
from jsonschema import Draft202012Validator
schema = json.load(open("schemas/rule-pack.schema.json"))
doc = yaml.safe_load(open(sys.argv[1]))
errs = list(Draft202012Validator(schema).iter_errors(doc))
for e in errs: print(f"{list(e.absolute_path)}: {e.message}")
sys.exit(1 if errs else 0)' packs/go-starter.yaml

# JavaScript — npm install -g ajv-cli ajv-formats
ajv validate \
  -s schemas/rule-pack.schema.json \
  -d packs/go-starter.yaml \
  --spec=draft2020 --strict=false
```

CI for community pack repos can use either.

---

## Document structure

A rule pack is a collection of rules that share metadata and a release
lifecycle. The minimum viable pack is:

```yaml
apiVersion: rules.idiomatic.dev/v1alpha1
kind: RulePack

pack:
  id: my-pack
  name: My Pack
  version: 0.1.0
  description: What this pack enforces.
  maintainer: my-team

rules:
  - id: my-rule
    name: Short title
    description: What this rule checks.
    rationale: Why it matters.
    severity: error
    detector:
      capability: semgrep
      version: "^1"
      config:
        language: go
        pattern: fmt.Println(...)
    applies_to: ["**/*.go"]
    fix:
      message: Use a structured logger instead.
```

---

## Top-level fields

| Field | Type | Required | Description |
|---|---|---|---|
| `apiVersion` | string | yes | Always `rules.idiomatic.dev/v1alpha1`. |
| `kind` | string | yes | Always `RulePack`. |
| `pack` | object | yes | Pack-level metadata. See [Pack metadata](#pack-metadata). |
| `rules` | object[] | yes | At least one rule. See [Rules](#rules). |

---

## Pack metadata

```yaml
pack:
  id: go-starter
  name: Go Starter
  version: 0.1.0
  description: Foundational Go conventions every project should enforce.
  maintainer: idiomatic
  license: MIT
  homepage: https://github.com/adamgilman/idiomatic/blob/main/packs/go-starter.yaml
  tags:
    language: go
    category: starter
```

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | string | yes | Unique identifier within the registry. Lowercase, hyphenated, alphanumeric. **Permanent once published — never rename.** |
| `name` | string | yes | Human-readable name. |
| `version` | string | yes | Semver version of this pack release. Bump on every change. |
| `description` | string | yes | What this pack enforces. |
| `maintainer` | string | yes | Who maintains this pack (org, team, or individual). |
| `license` | string | no | SPDX license identifier or `Proprietary`. |
| `homepage` | string | no | URL to documentation or source. |
| `tags` | object | no | Free-form key-value metadata for discoverability. Conventional keys: `language`, `category`, `vendor`, `architecture`. |

The `pack` object accepts additional unspecified fields (e.g. `descriptions`
with `business`/`developer` variants used by official packs). The schema
validates only the documented fields and lets the rest pass through.

---

## Rules

Each rule is one entry in the `rules` array.

```yaml
rules:
  - id: go-no-panic
    name: No panic() outside main and init
    description: Flags any call to panic() inside a function that is not main() or init().
    rationale: |
      Go's idiomatic error handling uses explicit error returns rather than panics.
      Production code should propagate errors so callers can decide how to recover.
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
    applies_to: ["**/*.go"]
    excludes: ["**/*_test.go", "**/vendor/**"]
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
    references:
      - https://go.dev/doc/effective_go#errors
    tests:
      - name: panic in regular function fires
        should: fire
        code: |
          package foo
          func Connect() { panic("nope") }
      - name: panic in main passes
        should: pass
        code: |
          package main
          func main() { panic("ok at startup") }
```

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | string | yes | Unique within the pack. Lowercase, hyphenated, alphanumeric. **Permanent once published.** |
| `name` | string | yes | Short title (max 80 chars). |
| `description` | string | yes | What the rule checks. One or two sentences. |
| `rationale` | string | yes | Why this rule exists — the reasoning a senior engineer would give. |
| `severity` | string | yes | `error` (must fix), `warning` (should fix), or `info` (awareness). |
| `tags` | string[] | no | Free-form categorization tags. |
| `detector` | object | yes | Routes the rule to a capability and supplies inputs. See [Detector](#detector). |
| `applies_to` | string[] | yes | File glob patterns this rule checks. Non-empty. |
| `excludes` | string[] | no | File glob patterns to skip. |
| `fix` | object | yes | Fix guidance. See [Fix](#fix). |
| `references` | string[] | no | URLs to documentation or rationale. |
| `tests` | object[] | no | Inline test fixtures. **Strongly recommended.** See [Tests](#tests). |

The rule object accepts additional unspecified fields (e.g. `descriptions`
with `business`/`developer` variants).

### `id` rules

Pack and rule IDs must match `^[a-z0-9][a-z0-9-]*[a-z0-9]$`:

- Lowercase letters, digits, and hyphens only
- No leading or trailing hyphens
- Single-character IDs are allowed

IDs are **permanent once published**. Never rename or reuse a published ID — community consumers may have pinned to it.

### Glob syntax

`applies_to` and `excludes` use standard `**`-aware globs. Examples:

```yaml
applies_to:
  - "**/*.go"          # every .go file in the tree
  - "src/**/*.ts"      # every .ts file under src/
excludes:
  - "**/vendor/**"     # everything under any vendor/ dir
  - "**/*_test.go"     # any test file
```

---

## Detector

Routes the rule to a capability and supplies its inputs.

```yaml
detector:
  capability: semgrep
  version: "^1"
  config:
    language: go
    pattern: fmt.Println(...)
```

| Field | Type | Required | Description |
|---|---|---|---|
| `capability` | string | yes | Name of a loaded capability (the capability spec's `metadata.name`). |
| `version` | string | no | Semver constraint on the capability's `metadata.version`. Empty matches anything. **Strongly recommended.** See [Versioning](#versioning) below. |
| `config` | object | yes | Per-rule inputs. The shape is defined by the named capability's `spec.inputs`. |

### Versioning

Authors are encouraged to declare a version constraint on every rule so a
future capability bump can't silently change the rule's behavior. Accepted
forms are everything [Masterminds/semver/v3](https://github.com/Masterminds/semver)
supports:

| Constraint | Matches |
|---|---|
| `1.0.0` | Exactly this version |
| `^1` | `>=1.0.0, <2.0.0` (any 1.x) |
| `^1.2` | `>=1.2.0, <2.0.0` |
| `~1.2` | `>=1.2.0, <1.3.0` |
| `>=1.2, <2` | Explicit range |
| `1.x` | Wildcard |
| (empty / omitted) | Any loaded version |

Mismatches fail loudly **before any tool runs**:

```
error: rule "X" targets capability "Y" with version constraint "^2",
       but the loaded capability is version 1.0.0
```

The constraint syntax is validated at manifest load time, so authoring typos
fail immediately rather than at runtime.

### Capability `config` shape

The `config` block's shape is defined by the capability's `spec.inputs`. The
manifest validator checks that `config` is non-empty; the **content** is
validated by the capability itself (`Capability.ValidateConfig`) at registry
load time.

For the canonical capabilities in this repo, see their reference:

| Capability | Required inputs | Optional inputs |
|---|---|---|
| `semgrep` | `language` | `pattern`, `patterns`, `pattern-either`, `pattern-not`, `pattern-inside`, `pattern-not-inside`, `metavariable-regex`, `metavariable-pattern` |
| `eslint` | `rule` | `plugin`, `type_aware` |
| `golangci-lint` | `linter` | `rule`, `settings` |
| `gosec` | `rule_id` | — |
| `gitleaks` | `rule_id` | — |
| `git` | `argv`, `fire_when` | — |
| `file-exists` | `path`, `fire_when` | — |
| `file-contains` | `path`, `pattern`, `fire_when` | — |

For capabilities outside this repo, see the capability's own documentation
or its YAML's `inputs:` block.

### `fire_when` expressions

Scalar-mode capabilities (`git`, `file-exists`, `file-contains`) require
each rule to provide a `fire_when` expression in `config`. The expression
queries the capability's signal and decides whether the rule fires.

Grammar:

```
expr      ::= or_expr
or_expr   ::= and_expr ( "or" and_expr )*
and_expr  ::= unary    ( "and" unary )*
unary     ::= "not" unary | "(" expr ")" | comparison
comparison ::= "signal." IDENT op value
op        ::= "==" | "!=" | "matches" | "contains"
value     ::= "\"" ... "\"" | "'" ... "'" | INTEGER
```

The RHS string literal is rendered as a Go template **first** (with sprig
helpers and a `.Inputs` map), so rules can interpolate their own input
values:

```yaml
config:
  pattern: "release-.*"
  fire_when: 'signal.stdout matches "{{ .Inputs.pattern }}"'
```

Operators:

| Operator | Behavior |
|---|---|
| `==` | String equality (or numeric for `exit_code`). |
| `!=` | String inequality. |
| `matches` | RHS is a regex; checks if the LHS matches anywhere. |
| `contains` | Substring containment. |

Examples:

```
signal.exit_code != 0
signal.stdout == ""
signal.stdout matches "^(main|master)$"
signal.stdout contains "WARNING"
not signal.stdout matches "release"
signal.stdout matches "x" and signal.exit_code == 0
(signal.stdout matches "x" or signal.stdout matches "y") and signal.exit_code == 0
```

The fields available depend on the capability's `signal.scalar_fields`.
Standard names (`stdout`, `stderr`, `exit_code`) are always present.

---

## Fix

```yaml
fix:
  message: Replace fmt.Println with a structured logger call.
  example: |
    // Before
    fmt.Println("request received")

    // After
    logger.Info("request received", "method", r.Method)
```

| Field | Type | Required | Description |
|---|---|---|---|
| `message` | string | yes | Actionable fix instruction. Max 200 characters. |
| `example` | string | no | Optional before/after code example. |

---

## Tests

Inline test fixtures. Each test asserts that the rule fires (or doesn't)
against a code snippet.

```yaml
tests:
  - name: panic outside main fires
    should: fire
    code: |
      package foo
      func Connect() { panic("nope") }
  - name: panic in main passes
    should: pass
    code: |
      package main
      func main() { panic("ok at startup") }
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Test case name. |
| `should` | string | yes | `fire` (expect a finding) or `pass` (expect no finding). |
| `code` | string | yes | Code snippet to run the rule against. |

Inline tests are **strongly recommended** for every rule. They double as
documentation and as a regression suite when the underlying tool changes.

---

## Complete example

```yaml
# yaml-language-server: $schema=../../schemas/rule-pack.schema.json
apiVersion: rules.idiomatic.dev/v1alpha1
kind: RulePack

pack:
  id: ai-workflow
  name: AI Workflow Standards
  version: 1.0.0
  description: Repository hygiene rules for AI-driven coding workflows.
  maintainer: idiomatic
  license: Proprietary
  tags:
    category: workflow

rules:
  - id: require-git-worktree
    name: Require git worktree for isolated work
    description: Checks that work is happening inside a linked git worktree.
    rationale: |
      Git worktrees provide isolated working copies that prevent accidental
      pollution of the main checkout.
    severity: warning
    tags: [git, workflow, isolation]
    detector:
      capability: git
      version: "^1"
      config:
        argv: [rev-parse, --absolute-git-dir]
        fire_when: 'not signal.stdout matches "/\\.git/worktrees/"'
    applies_to: ["**/*"]
    excludes: ["**/vendor/**", "**/node_modules/**", "**/.git/**"]
    fix:
      message: Create a git worktree with 'git worktree add ../worktree-name -b branch-name'.
      example: |
        git worktree add ../feature-work -b feature/my-feature
        cd ../feature-work
    references:
      - https://git-scm.com/docs/git-worktree
```
