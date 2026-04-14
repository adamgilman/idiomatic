# testing — Integration Test Environment

Containerized integration tests for the Claude Code plugin. Validates the hook contract end-to-end without requiring a real Claude Code session or API key.

## Test Layers

| Layer | Where | Deterministic | Cost | What it validates |
|---|---|---|---|---|
| Unit tests | `internal/claudehook/` | Yes | Free | Budget decision, compact projection, silent no-ops, error envelopes, stdin parsing |
| **Integration tests** | **This directory** | **Yes** | **Free** | Installation, plugin registration, hook contract flow from simulated input through idio to valid response |
| Manual smoke tests | Real Claude Code | No | API tokens | LLM convergence, fix hint quality, iteration limit behavior |

## Structure

```
testing/
├── Containerfile                     # Podman/Docker image: Go + jq + idio
├── run-integration-tests.sh          # Test harness (10 test cases)
├── fixtures/                         # Go module fixtures
│   ├── clean-module/                 # Zero findings
│   ├── few-violations/               # Exactly 3 findings
│   ├── many-violations/              # 35+ findings (budget/truncation test)
│   ├── no-manifest/                  # Silent no-op (no .idiomatic.yaml)
│   └── broken-manifest/              # Malformed .idiomatic.yaml
└── hook-inputs/                      # JSON templates
    ├── edit.json                     # PostToolUse for Edit tool
    ├── write.json                    # PostToolUse for Write tool
    └── multiedit.json                # PostToolUse for MultiEdit tool
```

## Test Cases

1. **clean-module: zero findings** — SARIF with empty results
2. **few-violations: 3 findings** — SARIF with exactly 3 results
3. **many-violations: 35+ findings** — Budget path (SARIF or compact)
4. **non-Go file: silent no-op** — Exit 0, no output
5. **no manifest: silent no-op** — Exit 0, no output
6. **broken manifest: error envelope** — Error in `additionalContext`
7. **malformed stdin: error envelope** — Error in `additionalContext`
8. **Edit tool variant** — Works correctly
9. **Write tool variant** — Works correctly
10. **MultiEdit tool variant** — Works correctly

## Running

### Locally
```bash
go build -o /tmp/idio-test ./cmd/idio
bash testing/run-integration-tests.sh /tmp/idio-test
```

### Container
```bash
podman build -t idiomatic-test testing/
podman run --rm -v $PWD:/src idiomatic-test
```

## Hook Input Templates

Templates use `__FILE__` and `__CWD__` placeholders that the test harness replaces at runtime with actual fixture paths. The JSON shape matches what Claude Code sends to PostToolUse hooks.
