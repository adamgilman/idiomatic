# manifest -- Rule Manifest Loader

> For the public-facing pack format and authoring guide, see `docs/authoring/rule-pack.md`.

Parses, validates, and loads v1alpha1 rule manifests. This is the first component in the pipeline and the contract that packs, capabilities, and the CLI all depend on. This package has **zero dependencies on the analysis engine** and must stay that way -- it is reused by CLI commands, validation tools, and future tooling. Never import `internal/engine`.

## Schema

Manifests are YAML files declaring a rule pack with one or more rules:

```yaml
apiVersion: rules.idiomatic.dev/v1alpha1
kind: RulePack

pack:
  id: my-pack
  name: My Pack
  version: 0.1.0
  description: What this pack does.
  maintainer: my-team

rules:
  - id: my-rule
    name: Short name
    description: What the rule checks.
    rationale: Why this rule exists.
    severity: error | warning | info
    detector:
      capability: eslint
      version: "^1"              # optional semver constraint on the capability
      config:
        rule: "@typescript-eslint/no-explicit-any"
        plugin: "@typescript-eslint/eslint-plugin"
        type_aware: false
    applies_to: ["**/*.ts"]
    excludes: ["**/vendor/**"]
    fix:
      message: How to fix it.
      example: |
        // before/after code
    tests:
      - name: test case
        should: fire | pass
        code: |
          // test fixture code
```

### Detector Format

The `Detector` struct has three fields:

- `Capability` -- the routing key, must match a loaded capability's `metadata.name`
- `Version` -- optional semver constraint on the capability's version (`^1`, `~1.2`, `>=1, <2`, exact, etc.). Empty matches anything. Syntax-checked at validation time using `Masterminds/semver/v3`; the actual constraint match against the loaded capability happens at registry load time in `internal/declarative`.
- `Config` -- per-rule inputs whose shape is defined by the capability's input schema

There is no hardcoded capability allowlist. Capabilities are discovered dynamically from the entries declared in `.idiomatic.yaml`.

```yaml
# ESLint capability
detector:
  capability: eslint
  version: "^1"
  config:
    rule: "@typescript-eslint/no-explicit-any"
    plugin: "@typescript-eslint/eslint-plugin"
    type_aware: true

# Semgrep capability
detector:
  capability: semgrep
  config:
    language: go
    pattern: fmt.Println(...)
```

## Loading Pipeline

`loadFromBytes` -> unmarshal -> validate -> build rule index -> collect warnings

The loader supports single files, directories (recursive `.yaml`/`.yml`), and auto-detection. It merges rule indices across files with cross-file duplicate detection and emits warnings for rules without inline tests.

## Validation

`validate.go` uses **batch error collection** -- all errors found in one pass, returned as `ValidationErrors`. Follow this pattern: never fail fast, always collect and return all errors.

Detector validation accepts only the capability format:
- `capability` required
- `config` must be non-empty
- `version` (if present) must be valid semver constraint syntax

## ID Rules

`isValidID` regex: `^[a-z0-9][a-z0-9-]*[a-z0-9]$` -- lowercase, digits, hyphens only, no leading/trailing hyphens. IDs are permanent once published.

## Files

- **types.go** -- Go structs for `Manifest`, `Pack`, `Rule`, `Detector`, `Fix`, `Test`, `Severity`, `TestExpect`. The `Detector` struct carries `Capability`, `Version`, and `Config` fields.
- **validate.go** -- Batch validator covering all spec requirements: apiVersion/kind, required fields, ID format, severity enum, detector fields, applies_to non-empty, fix.message length cap, test fixture validation, duplicate ID detection, version constraint syntax.
- **load.go** -- Loader supporting single files, directories (recursive `.yaml`/`.yml`), and auto-detection. Merges rule indices across files with cross-file duplicate detection. Emits warnings for rules without inline tests.
- **manifest_test.go** -- Tests covering valid manifests, multi-rule packs, missing fields, bad IDs, bad severity, duplicate IDs, bad apiVersion, bad tests, malformed YAML, directory loading, error formatting, ID validation, summary output.
- **testdata/** -- YAML fixtures exercising valid and invalid manifests.

## Modifying the Schema

1. Add field to struct in `types.go` with `yaml` tag
2. Add validation in `validateRule` or `validatePack` in `validate.go`
3. Add/update test fixtures in `testdata/`
4. Update tests in `manifest_test.go`

## Key Design Decisions

- **Batch error reporting**: All validation errors collected and reported at once. Users fix everything in one pass.
- **Typed structs only**: YAML parses directly into Go types via `gopkg.in/yaml.v3`.
- **Rule ID stability**: IDs are validated as `[a-z0-9-]` only. Once published, a rule ID is permanent.
- **Standalone package**: Zero dependencies on the analysis engine. Reusable by CLI commands, tests, and future tooling.
