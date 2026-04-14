# idiomatic plugin

## Setup (first use)

When a user installs this plugin and no `.idiomatic.yaml` exists in their project, guide them through setup:

### 1. Check that `idio` is installed

Run `which idio` or `idio version`. If not found, tell the user:

```
idio is not installed. Install it with:

  brew install adamgilman/tap/idio

or:

  go install github.com/adamgilman/idiomatic/cmd/idio@latest
```

### 2. Generate `.idiomatic.yaml`

Analyze the project to determine which languages and frameworks are in use. Look at:
- File extensions (`.go`, `.ts`, `.tsx`, `.py`, `.js`, `.jsx`)
- Config files (`go.mod`, `package.json`, `tsconfig.json`, `Dockerfile`, `*.tf`)
- Directory structure

Then generate a `.idiomatic.yaml` at the project root with the appropriate capabilities and packs from the official repo (`https://github.com/adamgilman/idiomatic`):

| Project has | Capability | Packs to consider |
|---|---|---|
| `.go` files | `capabilities/semgrep.yaml` | `packs/go-starter.yaml`, `packs/go-security.yaml` |
| `.go` files | `capabilities/golangci-lint.yaml` | `packs/go-testing.yaml`, `packs/go-comment-hygiene.yaml` |
| `.ts`/`.tsx` files | `capabilities/eslint.yaml` | `packs/typescript-starter.yaml`, `packs/ts-security.yaml` |
| `.py` files | `capabilities/semgrep.yaml` | `packs/python-starter.yaml` |
| `Dockerfile` | `capabilities/semgrep.yaml` | `packs/docker-security.yaml` |
| `*.tf` files | `capabilities/semgrep.yaml` | `packs/terraform-aws.yaml` or `packs/terraform-gcp.yaml` |
| Any repo | `capabilities/gitleaks.yaml` | `packs/secrets-detection.yaml` |

Example output for a Go project:

```yaml
apiVersion: rules.idiomatic.dev/v1alpha1
kind: ProjectConfig

capabilities:
  - repo: https://github.com/adamgilman/idiomatic
    path:
      - capabilities/semgrep.yaml
      - capabilities/gitleaks.yaml

packs:
  - repo: https://github.com/adamgilman/idiomatic
    path:
      - packs/go-starter.yaml
      - packs/go-security.yaml
      - packs/secrets-detection.yaml
```

### 3. Check external tools

After generating the config, check that the required external tools are installed. Run `which <tool>` for each capability referenced:

| Capability | Binary | Install command |
|---|---|---|
| semgrep | `semgrep` | `uv tool install semgrep` or `brew install semgrep` |
| eslint | `eslint` (usually in `node_modules/.bin/`) | `npm install eslint --save-dev` |
| golangci-lint | `golangci-lint` | `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest` |
| gosec | `gosec` | `go install github.com/securego/gosec/v2/cmd/gosec@latest` |
| gitleaks | `gitleaks` | `go install github.com/zricethezav/gitleaks/v8@latest` |

Tell the user which tools are missing and how to install them.

## Operations (ongoing use)

### Hook contract

The hook always exits 0. Errors go in `additionalContext` as JSON envelopes, never as exit codes.

### Response shape

```json
{
  "systemMessage": "idiomatic: 3 issues found (2 errors, 1 warning)",
  "hookSpecificOutput": {
    "hookEventName": "PostToolUse",
    "additionalContext": "<SARIF or compact JSON>"
  }
}
```

- `systemMessage` — present only when findings found or on error. Omitted on clean runs.
- `additionalContext` — full analysis payload. SARIF if under 9000 chars, compact summary if larger.
- Silent no-op (nil response) — unsupported file type or no `.idiomatic.yaml` discoverable.

### File extension routing

Routing is driven by each capability's `applies_to_files.extensions` field in its YAML definition. There is no hardcoded routing in Go code. To add a new language, create a capability YAML with the appropriate extensions and reference it in `.idiomatic.yaml`.
