# Idiomatic

## Public Specifications

The capability and rule pack YAML formats are public. Anyone can author capabilities and rule packs — for the official set, a community registry, or a private one. The authoritative reference and tutorials live under `docs/`:

- [`docs/spec/capability.md`](docs/spec/capability.md) — capability format reference
- [`docs/spec/rule-pack.md`](docs/spec/rule-pack.md) — rule pack format reference
- [`docs/authoring/capability.md`](docs/authoring/capability.md) — capability authoring tutorial (builds a Ruff capability from scratch)
- [`docs/authoring/rule-pack.md`](docs/authoring/rule-pack.md) — rule pack authoring tutorial
- [`schemas/capability.schema.json`](schemas/capability.schema.json) and [`schemas/rule-pack.schema.json`](schemas/rule-pack.schema.json) — JSON Schema (draft 2020-12) for editor validation via `yaml-language-server` and CI gating via `ajv` / `jsonschema`

When you change the schema, update both the JSON Schema and the Markdown reference. They should always agree.

## Core Principle

**Don't reinvent wheels.** If an existing CLI tool (ESLint, Semgrep, golangci-lint, Ruff, Clippy) already does the analysis, wrap it as a capability. Never rewrite analysis logic. Our value is orchestration, not detection.

## Architecture in One Sentence

Packs are data (YAML), capabilities are data (YAML wrappers around CLI tools), the engine is a generic orchestrator, and **both kinds of data load via git clone from HTTPS URLs declared in `.idiomatic.yaml`** — there are no embedded defaults.

## How to Add Things

**New rule:** Edit a pack YAML in `packs/`. Each rule names a `capability` and supplies a `config` block whose shape is defined by that capability's `inputs:` schema. No code changes — adding a rule is dropping YAML.

**New capability (new external tool):** Drop a YAML file into a `capabilities/` directory in a git repo. The shape is Kubernetes-style:

```yaml
apiVersion: capabilities.idiomatic.dev/v1alpha1
kind: Capability
metadata:
  name: <unique routing key, also referenced by rules via detector.capability>
  version: <semver, e.g. 1.0.0 — bump on every behavioral change>
  description: <one-liner>
spec:
  requires: { binary, install, discover, discover_files, precheck }
  inputs:   { <name>: { type, required, description } }
  applies_to_files: { extensions }   # omit to receive all files
  run:      { per, argv, timeout, ok_exit, output, config_file, ... }
  signal:   { shape: list|scalar, ... }
```

Then reference the capability file in your project's `.idiomatic.yaml` under the `capabilities:` section. No Go code changes are needed.

**New language:** Same as new capability. Pick the best external CLI tool for that language and write the YAML wrapper.

**No hand-coded capabilities, no embedded defaults.** Every tool integration is YAML, loaded via git clone from URLs declared in `.idiomatic.yaml`. There is no `//go:embed`, no `builtin/` package, no special-case code for any capability.

## Versioning

Every capability declares a `metadata.version` (semver). Rules can pin to a version constraint via `detector.version`:

```yaml
- id: my-rule
  detector:
    capability: semgrep
    version: "^1"        # caret: matches >=1.0.0, <2.0.0
    config: { ... }
```

Accepted constraint forms are everything `Masterminds/semver/v3` supports: exact (`1.0.0`), caret (`^1`, `^1.2`), tilde (`~1.2`), range (`>=1.2,<2`), wildcard (`1.x`). An empty constraint (or no `version:` field) matches any loaded version.

The CLI enforces constraints **after** the registry loads but **before** any tool runs. A stale rule pinned to an older capability fails with a clear `rule "X" targets capability "Y" with version constraint "^1", but the loaded capability is version 2.0.0` error — never a silent semantic drift. The constraint syntax itself is validated at manifest load time, so authoring typos surface as a validation error rather than a runtime surprise.

**When to bump capability version:**
- Patch: bug fix that doesn't change rule behavior
- Minor: backwards-compatible additions (new optional inputs, new optional spec fields)
- Major: breaking changes (renamed inputs, changed rule_id strategy, changed output projection)

Authors are encouraged to declare a `version: "^MAJOR"` constraint on every rule so a future major bump can't silently change behavior.

## Loading Capabilities and Packs

All loading is driven by `.idiomatic.yaml` (discovered by walking up from CWD or the edited file). The file has `kind: ProjectConfig` and declares explicit `capabilities:` and `packs:` sections:

```yaml
apiVersion: rules.idiomatic.dev/v1alpha1
kind: ProjectConfig

capabilities:
  - repo: https://github.com/adamgilman/idiomatic
    path:
      - capabilities/semgrep.yaml
      - capabilities/eslint.yaml

packs:
  - repo: https://github.com/adamgilman/idiomatic
    path:
      - packs/go-starter.yaml
```

Each entry has a `repo:` (plain HTTPS URL) and `path:` (array of file paths within that repo). The CLI clones each repo once and caches it locally. There are no filesystem path sources, no shortcut prefixes, and no environment variable overrides.

## Patterns to Follow

- **Two binding modes:** list-mode capabilities (semgrep, gosec, gitleaks, golangci-lint, eslint) batch all rules into one tool invocation and walk the JSON output as an array (or a nested array for tools like eslint that have a two-level shape); scalar-mode capabilities (git, file-exists, file-contains) run once per rule and decide via a `fire_when` expression on stdout/stderr/exit_code.
- **Exit code 1 = findings:** External tools usually exit 1 when they find issues. Declare this with `run.ok_exit: [1]` so the runtime treats it as success.
- **Temp file lifecycle:** The runtime creates and removes config files (`{tmp}/...` patterns become real tempfiles, `{project_root}/...` paths get written to the project and cleaned up the same way). Capability YAMLs only describe the shape.
- **No hardcoded capability lists.** Capabilities are discovered dynamically from the entries declared in `.idiomatic.yaml`. There is no allowlist to maintain.

## What Not to Do

- Don't embed analysis logic in Go. Write a YAML capability that wraps an external tool.
- Don't hardcode rule names anywhere. Rule data lives in pack YAML.
- Don't add new Go capability packages. There are none — every tool wrapper is YAML.
- Don't bundle capability YAMLs into the binary. They live on the filesystem like rule packs.
- Don't break the hook contract: always exit 0, errors go in `additionalContext`.

## Project Structure

Single Go module. `make` orchestrates builds/tests. `cmd/` for binaries, `manifest/` for the public manifest package, `internal/` for private packages, `packs/` for rule data, `capabilities/` for capability data. Configuration is driven by `.idiomatic.yaml` (`kind: ProjectConfig`), which declares git repos and file paths for both capabilities and packs.

## External Tool Requirements

- ESLint: must be in `node_modules/.bin/` (auto-discovered) or PATH
- Semgrep: must be on PATH (`uv tool install semgrep`)
- golangci-lint: must be on PATH (`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`)
- gosec, gitleaks: must be on PATH (`go install github.com/...`)
- git, stat, grep: standard POSIX tools (used by the git/file-exists/file-contains capabilities)

## Testing

`go test ./...` runs everything (or `make test`). Tests load capabilities from the repo's `capabilities/` directory using a `runtime.Caller`-based helper, so they work regardless of where `go test` is invoked from. Rebuild the installed binary after engine changes: `make install`
