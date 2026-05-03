# testing — Integration Test Environment

Validation for the Claude Code hook integration. Two suites — synthetic-stdin tests that pipe canned hook payloads into idio (fast, free, deterministic) and subprocess E2E tests that actually spawn `claude -p` and verify the full validate-as-you-write loop end to end.

## Test Layers

| Layer | Where | Deterministic | Cost | What it validates |
|---|---|---|---|---|
| Unit tests | `internal/claudehook/` | Yes | Free | Budget decision, compact projection, silent no-ops, error envelopes, stdin parsing |
| **Synthetic-stdin integration** | **`run-integration-tests.sh`** | **Yes** | **Free** | Installation, plugin registration, hook contract flow from simulated input through idio to valid response |
| **Subprocess E2E** | **`run-claude-hook-tests.sh`** | **No (real LLM)** | **API tokens (~$0.30/run)** | Claude Code actually fires the configured hook; findings reach the model context; convergence; cross-cwd config discovery |
| Manual smoke | Real Claude Code session | No | API tokens | Long-running flows, multi-file fixes, iteration-limit behavior |

## Structure

```
testing/
├── Containerfile                     # Podman/Docker image: Go + jq + idio
├── run-integration-tests.sh          # Synthetic-stdin tests (12 cases)
├── run-claude-hook-tests.sh          # Subprocess E2E with `claude -p` (3 cases)
├── fixtures/                         # Go module fixtures (synthetic suite)
│   ├── clean-module/                 # Zero findings
│   ├── few-violations/               # Exactly 3 findings
│   ├── many-violations/              # 35+ findings (budget/truncation test)
│   ├── no-manifest/                  # Silent no-op (no .idiomatic.yaml)
│   ├── broken-manifest/              # Malformed .idiomatic.yaml
│   ├── react-clean/                  # Clean TypeScript fixture
│   └── react-few-violations/         # TSX fixture with findings
└── hook-inputs/                      # JSON templates (synthetic suite)
    ├── edit.json                     # PostToolUse for Edit tool
    ├── write.json                    # PostToolUse for Write tool
    └── multiedit.json                # PostToolUse for MultiEdit tool
```

The subprocess E2E suite generates ephemeral fixtures under `/tmp/claude-hook-e2e-*/` so it doesn't depend on network or pre-baked fixture files. It points each fixture's `.idiomatic.yaml` at this repo's local `capabilities/` and `packs/` via absolute paths, so the test always exercises the current branch's code, not whatever's published.

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

### Synthetic-stdin suite (free, ~5s)

```bash
make integration-test
# or: bash testing/run-integration-tests.sh
```

### Container

```bash
podman build -t idiomatic-test testing/
podman run --rm -v $PWD:/src idiomatic-test
```

### Subprocess E2E suite

Requires both `claude` (with valid auth) and `idio` on PATH. Costs LLM tokens; capped at `--max-budget-usd 0.10` per test (~$0.30 worst case for the full suite). Skipped silently when prerequisites are missing — safe to wire into CI without breaking on runners that lack auth.

```bash
make claude-hook-test
# or: bash testing/run-claude-hook-tests.sh
```

Set `CLAUDE_HOOK_DEBUG=1` to capture each subprocess's stderr in the fixture's working directory for post-mortem inspection (the directories are otherwise removed at the end of each test).

## Hook Input Templates

Templates use `__FILE__` and `__CWD__` placeholders that the synthetic harness replaces at runtime with actual fixture paths. The JSON shape matches what Claude Code sends to PostToolUse hooks.
