# packs — Rule Packs

Declarative YAML rule packs that define what `idio` checks for. This directory contains the **official** idiomatic packs.

## Pack Format

The pack format is an open YAML spec. Anyone can write packs — for a language idiomatic doesn't cover yet, for a framework's conventions, or for company-internal standards. Community and private packs live in external git repositories and are referenced by URL. For the full authoring guide, see `docs/authoring/rule-pack.md`.

## Naming Conventions (Official Packs)

Official packs follow a consistent naming convention:
- **Language starter packs** (`{language}-starter`) — foundational rules, no team discussion required to adopt
- **Security packs** (`{language}-security`, `secrets-detection`, etc.) — vulnerability detection and infrastructure security
- **Testing packs** (`{language}-testing`) — test quality enforcement
- **Comment hygiene packs** (`{language}-comment-hygiene`) — documentation and comment quality
- **Domain packs** (`{language}-{topic}`) — opinionated domain-specific rules

Community and private packs may follow their own naming conventions.

## Available Packs

### Language Starter Packs

| Pack | Language | Target | Description |
|---|---|---|---|
| `go-starter` | Go | Go projects | Foundational Go rules: error handling, logging, context, testing |
| `typescript-starter` | TypeScript | TypeScript projects | Foundational TypeScript rules: type safety, async patterns, modern idioms |
| `python-starter` | Python | Python projects | Foundational Python rules: error handling, type safety, common pitfalls |
| `react-16-starter` | TypeScript | React 16/17 (class + functional) | 20 rules covering hooks, class safety, JSX correctness, performance, and safety |
| `react-18-starter` | TypeScript | React 18+ (functional only) | 20 rules covering hooks, no-class enforcement, deprecated API migration, JSX correctness, performance, and safety |

### Security Packs

| Pack | Language | Target | Description |
|---|---|---|---|
| `go-security` | Go | Go services | OWASP Top 10 security hardening: injection, auth bypasses, crypto misuse |
| `ts-security` | TypeScript | TypeScript services | OWASP Top 10 security hardening: XSS, injection, auth, transport security |
| `go-gosec` | Go | Go services | Deep security analysis powered by gosec: taint tracking, crypto, file permissions |
| `secrets-detection` | Any | All files | Secret and credential detection powered by gitleaks: API keys, passwords, tokens |
| `docker-security` | Dockerfile | Dockerfiles | Container security: root user, unpinned images, secret leakage, layer hygiene |
| `terraform-aws` | HCL | Terraform (AWS) | Infrastructure security: permissive IAM, public databases, missing encryption |
| `terraform-gcp` | HCL | Terraform (GCP) | Infrastructure security: firewall rules, public databases, IAM misconfigurations |
| `k8s-manifests` | YAML | Kubernetes manifests | Kubernetes security: resource limits, security contexts, health probes, network policies |

### Testing Packs

| Pack | Language | Target | Description |
|---|---|---|---|
| `go-testing` | Go | Go test files | Test quality: flaky patterns, missing cleanup, assertion discipline |
| `ts-testing` | TypeScript | TypeScript test files | Test quality: missing assertions, focused tests, flaky patterns |

### Comment Hygiene Packs

| Pack | Language | Target | Description |
|---|---|---|---|
| `go-comment-hygiene` | Go | Go files | Comment quality: doc comments on exports, Go conventions, stale TODOs |
| `ts-comment-hygiene` | TypeScript | TypeScript files | Comment quality: JSDoc discipline, suppression comments, stale TODOs |
| `py-comment-hygiene` | Python | Python files | Comment quality: docstrings on public APIs, suppression comments, stale TODOs |

## Usage

```bash
# Validate a pack
idio manifest validate packs/go-starter.yaml

# List rules
idio manifest list packs/go-starter.yaml

# Scan code (discovers .idiomatic.yaml automatically)
idio scan ./src/
```

## Versioning

Capabilities declare a `metadata.version` (semver). Rules pin via `detector.version`:

```yaml
detector:
  capability: semgrep
  version: "^1"           # caret: matches >=1.0.0, <2.0.0
  config: { ... }
```

Mismatched constraints fail loudly **before** any tool runs:

```
error: rule "X" targets capability "Y" with version constraint "^2",
       but the loaded capability is version 1.0.0
```

The constraint syntax itself is validated at manifest load time, so authoring typos surface immediately.

## Config Block Patterns

Each capability has a specific config shape. Look at existing rules in the same capability for the pattern:

- **eslint rules**: `config.rule` is the full ESLint rule name, `config.plugin` is the npm package, `config.type_aware` if it needs tsconfig
- **semgrep rules**: `config.language` + `config.pattern` (or `config.patterns` for compound rules with `pattern-not-inside`, `metavariable-regex`, etc.)
- **golangci-lint rules**: `config.linter` is the linter name, `config.rule` for sub-rule matching, `config.settings` for linter config

## Writing Packs

See `docs/authoring/rule-pack.md` for the full authoring tutorial. Key points:
- One pack per YAML file with `apiVersion: rules.idiomatic.dev/v1alpha1`
- Each rule needs: id, name, description, rationale, severity, detector, applies_to, fix.message
- Rule IDs are permanent once published — never reuse them
- Inline tests (`tests[].should: fire|pass`) are strongly recommended

## When Adding Rules

1. Verify the capability actually supports the check you want (test the external tool manually first)
2. Copy an existing rule in the same capability — don't write from scratch
3. Run `go test ./manifest/...` to validate YAML structure
4. Test end-to-end: create a `.idiomatic.yaml` referencing the pack and run `idio scan <test-file>`
