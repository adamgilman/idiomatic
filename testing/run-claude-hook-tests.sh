#!/usr/bin/env bash
# ---------------------------------------------------------------------------
# Claude Code subprocess hook integration tests.
#
# These tests spawn `claude -p` in an ephemeral fixture directory and verify
# the FULL validate-as-you-write loop:
#
#   Edit/Write tool fires
#     -> Claude Code invokes the configured PostToolUse hook
#     -> idio scan --format=claude-hook returns SARIF/compact JSON
#     -> findings reach the model context as additionalContext
#     -> the model can read & act on them
#
# This is the layer above run-integration-tests.sh, which only verifies that
# idio responds correctly to synthetic stdin; this script verifies that
# Claude Code actually fires the hook and surfaces findings.
#
# Cost: each test makes 1-3 LLM calls capped at $0.10 each via
# --max-budget-usd. Full suite worst case ~$0.30.
#
# Skipped if `claude` or `idio` is not on PATH (CI runners typically lack
# Claude auth and may lack idio).
#
# Run from anywhere:
#   ./testing/run-claude-hook-tests.sh
#   make claude-hook-test
# ---------------------------------------------------------------------------
set -euo pipefail

# ── Colour helpers ─────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

PASS=0
FAIL=0
TESTS=()
RESULTS=()

pass() {
  PASS=$((PASS + 1))
  TESTS+=("$1")
  RESULTS+=("PASS")
  echo -e "  ${GREEN}PASS${NC}  $1"
}

fail() {
  FAIL=$((FAIL + 1))
  TESTS+=("$1")
  RESULTS+=("FAIL")
  echo -e "  ${RED}FAIL${NC}  $1"
  if [[ -n "${2:-}" ]]; then
    echo -e "        ${YELLOW}detail:${NC} $2"
  fi
}

skip_all() {
  echo -e "${YELLOW}SKIP${NC}: $1"
  echo -e "      These tests require both \`claude\` and \`idio\` on PATH."
  exit 0
}

# ── Paths ──────────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# ── Preflight ──────────────────────────────────────────────────────────────
command -v claude &>/dev/null || skip_all "'claude' CLI not on PATH"
command -v idio &>/dev/null   || skip_all "'idio' CLI not on PATH"
command -v semgrep &>/dev/null || echo -e "${YELLOW}WARN${NC}: semgrep not on PATH; semgrep-based rules will fail to fire"

echo ""
echo "==> Running Claude Code hook integration tests"
echo "    repo root: $REPO_ROOT"
echo "    claude:    $(claude --version 2>/dev/null | head -1)"
echo "    idio:      $(idio version 2>/dev/null | head -1)"
echo ""

# ── write_hook_settings <dir> ──────────────────────────────────────────────
# Write a .claude/settings.json that wires the PostToolUse hook to
# `idio scan --format=claude-hook` for Edit/Write/MultiEdit. Claude Code's
# settings watcher only loads this from the directory where the session
# starts, so cross-cwd tests must call this on the session dir too.
write_hook_settings() {
  local dir="$1"
  mkdir -p "$dir/.claude"
  cat > "$dir/.claude/settings.json" <<'EOF'
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit|Write|MultiEdit",
        "hooks": [{
          "type": "command",
          "command": "idio scan --format=claude-hook",
          "timeout": 30
        }]
      }
    ]
  }
}
EOF
}

# ── make_workdir <slug> ────────────────────────────────────────────────────
# Create an ephemeral Go module wired up with a hook config and an
# .idiomatic.yaml that points at this repo's local capabilities/packs (so
# tests don't depend on the public repo or network).
#
# Echoes the absolute path to the created dir.
make_workdir() {
  local slug="$1"
  local dir
  dir="$(mktemp -d "/tmp/claude-hook-e2e-${slug}-XXXXXX")"

  cat > "$dir/.idiomatic.yaml" <<EOF
apiVersion: rules.idiomatic.dev/v1alpha1
kind: ProjectConfig
capabilities:
  - path:
      # Listing every per-linter capability go-starter + go-comment-hygiene
      # actually reference. The first missing one would short-circuit the
      # scan with "capability X is not loaded", which makes the test fail
      # not because the hook is broken but because the fixture is incomplete.
      - $REPO_ROOT/capabilities/semgrep.yaml
      - $REPO_ROOT/capabilities/revive.yaml
      - $REPO_ROOT/capabilities/errcheck.yaml
      - $REPO_ROOT/capabilities/nakedret.yaml
      - $REPO_ROOT/capabilities/interfacebloat.yaml
      - $REPO_ROOT/capabilities/contextcheck.yaml
      - $REPO_ROOT/capabilities/errorlint.yaml
      - $REPO_ROOT/capabilities/recvcheck.yaml
      - $REPO_ROOT/capabilities/testpackage.yaml
      - $REPO_ROOT/capabilities/godot.yaml
      - $REPO_ROOT/capabilities/nolintlint.yaml
packs:
  - path:
      - $REPO_ROOT/packs/go-starter.yaml
      - $REPO_ROOT/packs/go-comment-hygiene.yaml
EOF

  write_hook_settings "$dir"

  cat > "$dir/go.mod" <<'EOF'
module testfixture

go 1.25
EOF

  echo "$dir"
}

# ── claude_p <workdir> <prompt> ────────────────────────────────────────────
# Run claude -p with consistent flags; capture stdout. Stderr is discarded
# unless CLAUDE_HOOK_DEBUG=1 is set.
claude_p() {
  local workdir="$1"
  local prompt="$2"
  local stderr_target=/dev/null
  if [[ "${CLAUDE_HOOK_DEBUG:-0}" == "1" ]]; then
    stderr_target="$workdir/claude.stderr.log"
  fi
  ( cd "$workdir" && \
    claude -p "$prompt" \
      --permission-mode acceptEdits \
      --model haiku \
      --max-budget-usd 0.10 \
      --no-session-persistence \
      2>"$stderr_target" )
}

# =========================================================================
# TEST 1: Hook fires after Edit, finding reaches the model
# =========================================================================
TEST_NAME="hook fires + finding reaches model"
echo -e "${BLUE}--- $TEST_NAME ---${NC}"
WORKDIR="$(make_workdir "fires")"
cat > "$WORKDIR/auth.go" <<'EOF'
package authsvc

// Session represents an authenticated user's session.
type Session struct {
	token string
}

// Token returns the session's bearer token.
func (s *Session) Token() string {
	return s.token
}
EOF

PROMPT="Add a method named GetUser() string that returns the literal 'alice' to the Session struct in auth.go. Just add the method, no other changes."
RESPONSE="$(claude_p "$WORKDIR" "$PROMPT")"

# After the Edit, the hook should have fired and idio should have flagged
# the GetUser method (go-no-get-prefix) AND the missing doc comment
# (go-require-doc-comment). The model should mention at least one of
# these in its response — proving the additionalContext reached it.
if echo "$RESPONSE" | grep -qiE "linter|warning|finding|idiom|convention|prefix|doc comment"; then
  pass "$TEST_NAME"
else
  fail "$TEST_NAME" "response did not mention any expected finding. Got: $(echo "$RESPONSE" | head -c 400)"
fi
rm -rf "$WORKDIR"

# =========================================================================
# TEST 2: Convergence — model fixes findings on a broad prompt
# =========================================================================
TEST_NAME="convergence: post-edit scan returns 0 findings"
echo -e "${BLUE}--- $TEST_NAME ---${NC}"
WORKDIR="$(make_workdir "converge")"
cat > "$WORKDIR/auth.go" <<'EOF'
package authsvc

import "errors"

// Session is a user session.
type Session struct {
	token string
}

// GetToken returns the bearer token.
func (self *Session) GetToken() string {
	return self.token
}

// DoThing does a thing.
func DoThing(force bool) error {
	if force {
		return errors.New("Operation failed.")
	}
	return nil
}
EOF

PROMPT="Look at auth.go and bring it up to Go conventions. Make whatever edits the linter recommends — if it flags a convention, fix it."
RESPONSE="$(claude_p "$WORKDIR" "$PROMPT")"

# Assert the post-state: idio scan exits 0.
set +e
( cd "$WORKDIR" && idio scan auth.go ) > "$WORKDIR/post-scan.txt" 2>&1
SCAN_RC=$?
set -e
if [[ $SCAN_RC -eq 0 ]]; then
  pass "$TEST_NAME"
else
  fail "$TEST_NAME" "scan still has findings after claude's edits. Remaining:
$(head -10 "$WORKDIR/post-scan.txt")"
fi
rm -rf "$WORKDIR"

# =========================================================================
# TEST 3: Cross-cwd hook discovery (regression for U1)
# =========================================================================
# Claude runs from a directory that has NO .idiomatic.yaml; the file it
# edits lives in a separate project that DOES. The hook must walk up from
# the edited file to find the right config — not from claude's cwd.
TEST_NAME="cross-cwd: hook discovers config from edited-file path (U1)"
echo -e "${BLUE}--- $TEST_NAME ---${NC}"
WORKDIR="$(make_workdir "crosscwd")"
SESSION_DIR="$(mktemp -d /tmp/claude-hook-e2e-session-XXXXXX)"
# Claude's session dir needs the hook settings (the watcher loads them
# from the session's cwd). The session dir has no .idiomatic.yaml, so
# the hook will have to discover the project config by walking up from
# the edited file's path — exactly the U1 regression we're testing.
write_hook_settings "$SESSION_DIR"

cat > "$WORKDIR/auth.go" <<'EOF'
package authsvc

// Session is a user session.
type Session struct {
	token string
}

// Token returns the bearer token.
func (s *Session) Token() string {
	return s.token
}
EOF

PROMPT="Add a method named GetID() string returning 'session-1' to the Session struct at $WORKDIR/auth.go. Just add the method."

set +e
RESPONSE="$( cd "$SESSION_DIR" && \
  claude -p "$PROMPT" \
    --permission-mode acceptEdits \
    --model haiku \
    --max-budget-usd 0.10 \
    --no-session-persistence \
    --add-dir "$WORKDIR" \
    2>/dev/null )"
set -e

# Same assertion as TEST 1: response mentions the finding. Proves the
# hook fired AND findings reached the model context AND the project
# config was discovered from the file's path (not from the empty
# session dir).
if echo "$RESPONSE" | grep -qiE "linter|warning|finding|idiom|convention|prefix|doc comment"; then
  pass "$TEST_NAME"
else
  fail "$TEST_NAME" "response did not surface a finding for the foreign-cwd edit. Got: $(echo "$RESPONSE" | head -c 400)"
fi
rm -rf "$WORKDIR" "$SESSION_DIR"

# =========================================================================
# Summary
# =========================================================================
echo ""
echo "==========================================="
TOTAL=$((PASS + FAIL))
echo -e "  Total: ${TOTAL}   ${GREEN}Passed: ${PASS}${NC}   ${RED}Failed: ${FAIL}${NC}"
echo "==========================================="
echo ""

for i in "${!TESTS[@]}"; do
  if [[ "${RESULTS[$i]}" == "PASS" ]]; then
    echo -e "  ${GREEN}PASS${NC}  ${TESTS[$i]}"
  else
    echo -e "  ${RED}FAIL${NC}  ${TESTS[$i]}"
  fi
done
echo ""

if [[ $FAIL -gt 0 ]]; then
  echo -e "${RED}Some tests failed.${NC}"
  exit 1
fi
echo -e "${GREEN}All tests passed.${NC}"
