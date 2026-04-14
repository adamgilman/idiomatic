# Idiomatic

Deterministic enforcement for stochastic development.

[![CI](https://github.com/adamgilman/idiomatic/actions/workflows/ci.yml/badge.svg)](https://github.com/adamgilman/idiomatic/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg)

Idiomatic runs alongside AI coding agents like Claude Code. After every file edit, it checks the code against YAML rule packs using external analysis tools (ESLint, Semgrep, golangci-lint, gosec, gitleaks) and feeds findings back into the LLM's context. The LLM reads the findings and fixes violations before the developer ever sees them — converging to code that meets your standards on every commit.

## Install

**Homebrew (macOS and Linux):**

```bash
brew install adamgilman/tap/idio
```

**From source:**

```bash
go install github.com/adamgilman/idiomatic/cmd/idio@latest
```

**From GitHub Releases:**

Download the latest binary from [Releases](https://github.com/adamgilman/idiomatic/releases).

**Build locally:**

```bash
git clone https://github.com/adamgilman/idiomatic.git
cd idiomatic
make install
```

### External Tools

Idiomatic wraps external analysis tools. Install the ones you need:

```bash
# Go analysis
uv tool install semgrep
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

# TypeScript/React analysis
npm install eslint --save-dev

# Security scanning
go install github.com/securego/gosec/v2/cmd/gosec@latest
go install github.com/zricethezav/gitleaks/v8@latest
```

## Quick Start

**1. Create `.idiomatic.yaml` in your project root:**

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

**2. Run a scan:**

```bash
# Scan from the project root (discovers .idiomatic.yaml automatically)
idio scan ./src/

# Validate a pack
idio manifest validate packs/go-starter.yaml

# List rules in a pack
idio manifest list packs/go-starter.yaml
```

## Claude Code Integration

Idiomatic is designed to run as a [Claude Code hook](https://docs.anthropic.com/en/docs/claude-code/hooks). After every file edit, the hook runs analysis and feeds findings back into Claude's context for immediate correction.

**Install the plugin:**

Copy the plugin directory to your project, or reference it from your Claude Code configuration:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit|Write|MultiEdit",
        "hooks": [
          {
            "type": "command",
            "command": "idio scan --format=claude-hook",
            "timeout": 30
          }
        ]
      }
    ]
  }
}
```

**How it works:**

1. Claude Code edits a file
2. The PostToolUse hook fires, running `idio scan --format=claude-hook`
3. Idiomatic discovers the `.idiomatic.yaml` manifest by walking up from the edited file
4. Rules are routed to the appropriate external tools (Semgrep for Go, ESLint for TypeScript, etc.)
5. Findings are returned as `additionalContext` in the hook response
6. Claude reads the findings and fixes the violations
7. The loop repeats until the code is clean

## Rule Packs

All packs are open source and included in this repository. See [`packs/`](packs/) for details.

### Language

| Pack | Language | Description |
|---|---|---|
| `go-starter` | Go | Error handling, logging, context, testing |
| `typescript-starter` | TypeScript | Type safety, async patterns, modern idioms |
| `python-starter` | Python | Error handling, type safety, common pitfalls |
| `react-16-starter` | TypeScript | React 16/17 hooks, class safety, JSX, performance |
| `react-18-starter` | TypeScript | React 18+ hooks, no-class enforcement, deprecated APIs |

### Security

| Pack | Language | Description |
|---|---|---|
| `go-security` | Go | OWASP Top 10: injection, auth, crypto |
| `ts-security` | TypeScript | OWASP Top 10: XSS, injection, transport |
| `go-gosec` | Go | Deep security via gosec: taint tracking, crypto, permissions |
| `secrets-detection` | Any | Credential detection via gitleaks |
| `docker-security` | Dockerfile | Container security and layer hygiene |
| `terraform-aws` | HCL | AWS infrastructure security |
| `terraform-gcp` | HCL | GCP infrastructure security |
| `k8s-manifests` | YAML | Kubernetes resource limits, security contexts, probes |

### Domain

| Pack | Language | Description |
|---|---|---|
| `go-otel` / `ts-otel` | Go, TS | OpenTelemetry instrumentation patterns |
| `go-postgres` | Go | Database best practices, connection pools, SQL safety |
| `go-kafka` / `ts-kafka` | Go, TS | Kafka consumer/producer patterns |
| `go-redis` | Go | Cache patterns, TTL, distributed locks |
| `go-resilience` | Go | Circuit breakers, retries, graceful shutdown |
| `go-concurrency` | Go | Goroutine safety, channels, sync primitives |
| `go-httpclient` | Go | HTTP client timeouts, connection reuse |
| `go-grpc` | Go | gRPC status codes, interceptors, streaming |
| `go-aws` | Go | AWS SDK best practices |
| `go-pipeline` | Go | Data pipeline idempotency, backpressure |
| `go-worker` | Go | Background job patterns, graceful shutdown |
| `go-structured-logging` | Go | Structured logging discipline |
| `ts-nextjs` | TypeScript | Next.js server/client components, data fetching |
| `py-fastapi` | Python | FastAPI validation, dependency injection |
| `ai-workflow` | Any | AI-assisted development workflow standards |
| `claude-code-standards` | Any | Claude Code session standards |

### Quality

| Pack | Language | Description |
|---|---|---|
| `go-testing` / `ts-testing` | Go, TS | Test quality: flaky patterns, assertions |
| `go-comment-hygiene` | Go | Doc comments on exports, stale TODOs |
| `ts-comment-hygiene` | TypeScript | JSDoc discipline, suppression comments |
| `py-comment-hygiene` | Python | Docstrings on public APIs |

## Capabilities

Capabilities are YAML wrappers around external analysis tools. See [`capabilities/`](capabilities/) for all definitions.

| Capability | External Tool | Description |
|---|---|---|
| `eslint` | ESLint 9 | TypeScript, React, JSX analysis |
| `semgrep` | Semgrep | Pattern matching across languages |
| `golangci-lint` | golangci-lint | Go linter aggregator |
| `gosec` | gosec | Go security scanner |
| `gitleaks` | gitleaks | Secret and credential detection |
| `git` | git | Git repository checks |
| `file-exists` | stat | File presence checks |
| `file-contains` | grep | File content checks |

## Writing Custom Rules and Capabilities

- [Rule Pack Authoring Tutorial](docs/authoring/rule-pack.md) — build a rule pack from scratch
- [Capability Authoring Tutorial](docs/authoring/capability.md) — wrap a new CLI tool as a capability
- [Rule Pack Format Reference](docs/spec/rule-pack.md)
- [Capability Format Reference](docs/spec/capability.md)

## Architecture

Packs are data (YAML). Capabilities are data (YAML wrappers around CLI tools). The engine is a generic orchestrator. Both kinds of data load via git clone from HTTPS URLs declared in `.idiomatic.yaml` — there are no embedded defaults.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the full architecture overview.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for how to build, test, add rules, and submit pull requests.

## License

Apache License 2.0. See [LICENSE](LICENSE).
