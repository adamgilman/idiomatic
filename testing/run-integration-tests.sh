#!/usr/bin/env bash
# ---------------------------------------------------------------------------
# Integration-test harness for the idio CLI (claude-hook mode).
#
# Expects:
#   - Source tree mounted/copied at /src  (used by the Containerfile build)
#   - idio binary already on PATH         (Containerfile builds it)
#   - jq available
#   - Fixture dirs under /testing/fixtures/
#   - Hook input templates under /testing/hook-inputs/
# ---------------------------------------------------------------------------
set -euo pipefail

# ── Colour helpers ─────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No colour

# ── Counters ───────────────────────────────────────────────────────────────
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

# ── Paths ──────────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FIXTURES="${FIXTURES:-/testing/fixtures}"
HOOKS="${HOOKS:-/testing/hook-inputs}"
# Allow local override: if default paths don't exist, fall back to repo-relative paths
if [[ ! -d "$FIXTURES" ]]; then
  FIXTURES="$SCRIPT_DIR/fixtures"
fi
if [[ ! -d "$HOOKS" ]]; then
  HOOKS="$SCRIPT_DIR/hook-inputs"
fi

# ── Step 0: Build idio if not already on PATH ─────────────────────────────
if ! command -v idio &>/dev/null; then
  echo "==> Building idio from /src ..."
  cd /src
  go build -o /usr/local/bin/idio \
      -ldflags "-X github.com/adamgilman/idiomatic/cmd/idio/cmd.Version=integration-test" \
      ./cmd/idio
  echo "    idio built OK"
fi


echo ""
echo "==> Running integration tests"
echo ""

# ── Helper: build hook JSON from a template ────────────────────────────────
# Usage: build_hook <template> <file_path> <cwd>
build_hook() {
  local tpl="$1" file="$2" cwd="$3"
  sed -e "s|__FILE__|${file}|g" -e "s|__CWD__|${cwd}|g" "$tpl"
}

# ── Helper: run idio claude-hook and capture output + exit code ────────────
# Usage: run_hook <hook_json>
# Sets: HOOK_OUT, HOOK_RC
run_hook() {
  local input="$1"
  set +e
  HOOK_OUT=$(echo "$input" | idio scan --format=claude-hook 2>&1)
  HOOK_RC=$?
  set -e
}

# =========================================================================
# TEST 1: Happy path — clean module, zero findings
# =========================================================================
TEST_NAME="clean-module: zero findings"
CWD="$FIXTURES/clean-module"
FILE="$CWD/clean.go"
HOOK_JSON=$(build_hook "$HOOKS/edit.json" "$FILE" "$CWD")
run_hook "$HOOK_JSON"

if [[ $HOOK_RC -ne 0 ]]; then
  fail "$TEST_NAME" "exit code $HOOK_RC, expected 0"
elif [[ -z "$HOOK_OUT" ]]; then
  fail "$TEST_NAME" "expected SARIF output, got empty stdout"
elif ! echo "$HOOK_OUT" | jq -e '.hookSpecificOutput.additionalContext' &>/dev/null; then
  fail "$TEST_NAME" "output is not valid hook JSON: $HOOK_OUT"
else
  # additionalContext should contain SARIF with zero results.
  RESULTS_COUNT=$(echo "$HOOK_OUT" | jq -r '.hookSpecificOutput.additionalContext' | jq '.runs[0].results | length')
  if [[ "$RESULTS_COUNT" == "0" ]]; then
    pass "$TEST_NAME"
  else
    fail "$TEST_NAME" "expected 0 findings, got $RESULTS_COUNT"
  fi
fi

# =========================================================================
# TEST 2: Happy path — few violations, exactly 3 findings
# =========================================================================
TEST_NAME="few-violations: exactly 3 findings"
CWD="$FIXTURES/few-violations"
FILE="$CWD/violations.go"
HOOK_JSON=$(build_hook "$HOOKS/edit.json" "$FILE" "$CWD")
run_hook "$HOOK_JSON"

if [[ $HOOK_RC -ne 0 ]]; then
  fail "$TEST_NAME" "exit code $HOOK_RC, expected 0"
elif [[ -z "$HOOK_OUT" ]]; then
  fail "$TEST_NAME" "expected SARIF output, got empty stdout"
elif ! echo "$HOOK_OUT" | jq -e '.hookSpecificOutput.additionalContext' &>/dev/null; then
  fail "$TEST_NAME" "output is not valid hook JSON: $HOOK_OUT"
else
  SARIF_CONTENT=$(echo "$HOOK_OUT" | jq -r '.hookSpecificOutput.additionalContext')
  RESULTS_COUNT=$(echo "$SARIF_CONTENT" | jq '.runs[0].results | length')
  if [[ "$RESULTS_COUNT" == "3" ]]; then
    pass "$TEST_NAME"
  else
    fail "$TEST_NAME" "expected 3 findings, got $RESULTS_COUNT"
  fi
fi

# =========================================================================
# TEST 3: Many violations — 35+ findings, budget/truncation
# =========================================================================
TEST_NAME="many-violations: 35+ findings"
CWD="$FIXTURES/many-violations"
FILE="$CWD/violations.go"
HOOK_JSON=$(build_hook "$HOOKS/edit.json" "$FILE" "$CWD")
run_hook "$HOOK_JSON"

if [[ $HOOK_RC -ne 0 ]]; then
  fail "$TEST_NAME" "exit code $HOOK_RC, expected 0"
elif [[ -z "$HOOK_OUT" ]]; then
  fail "$TEST_NAME" "expected output, got empty stdout"
elif ! echo "$HOOK_OUT" | jq -e '.hookSpecificOutput.additionalContext' &>/dev/null; then
  fail "$TEST_NAME" "output is not valid hook JSON: $HOOK_OUT"
else
  ADDITIONAL=$(echo "$HOOK_OUT" | jq -r '.hookSpecificOutput.additionalContext')
  # Could be SARIF or compact format depending on budget.
  # Check total findings >= 35 in either format.
  if echo "$ADDITIONAL" | jq -e '.runs[0].results' &>/dev/null; then
    # SARIF format
    RESULTS_COUNT=$(echo "$ADDITIONAL" | jq '.runs[0].results | length')
  elif echo "$ADDITIONAL" | jq -e '.summary.findings_total' &>/dev/null; then
    # Compact format
    RESULTS_COUNT=$(echo "$ADDITIONAL" | jq '.summary.findings_total')
  else
    RESULTS_COUNT=0
  fi

  if [[ "$RESULTS_COUNT" -ge 35 ]]; then
    pass "$TEST_NAME"
  else
    fail "$TEST_NAME" "expected >= 35 findings, got $RESULTS_COUNT"
  fi
fi

# =========================================================================
# TEST 4: Silent no-op for non-Go file
# =========================================================================
TEST_NAME="non-go-file: silent no-op"
CWD="$FIXTURES/clean-module"
FILE="$CWD/README.md"
HOOK_JSON=$(build_hook "$HOOKS/edit.json" "$FILE" "$CWD")
run_hook "$HOOK_JSON"

if [[ $HOOK_RC -ne 0 ]]; then
  fail "$TEST_NAME" "exit code $HOOK_RC, expected 0"
elif [[ -n "$HOOK_OUT" ]]; then
  fail "$TEST_NAME" "expected empty output for non-Go file, got: $HOOK_OUT"
else
  pass "$TEST_NAME"
fi

# =========================================================================
# TEST 5: Silent no-op for no manifest
# =========================================================================
TEST_NAME="no-manifest: silent no-op"
CWD="$FIXTURES/no-manifest"
FILE="$CWD/clean.go"
HOOK_JSON=$(build_hook "$HOOKS/edit.json" "$FILE" "$CWD")
run_hook "$HOOK_JSON"

if [[ $HOOK_RC -ne 0 ]]; then
  fail "$TEST_NAME" "exit code $HOOK_RC, expected 0"
elif [[ -n "$HOOK_OUT" ]]; then
  fail "$TEST_NAME" "expected empty output for no manifest, got: $HOOK_OUT"
else
  pass "$TEST_NAME"
fi

# =========================================================================
# TEST 6: Error for broken manifest
# =========================================================================
TEST_NAME="broken-manifest: error reported"
CWD="$FIXTURES/broken-manifest"
FILE="$CWD/clean.go"
HOOK_JSON=$(build_hook "$HOOKS/edit.json" "$FILE" "$CWD")
run_hook "$HOOK_JSON"

if [[ $HOOK_RC -ne 0 ]]; then
  fail "$TEST_NAME" "exit code $HOOK_RC, expected 0 (errors go in additionalContext)"
elif [[ -z "$HOOK_OUT" ]]; then
  fail "$TEST_NAME" "expected error output, got empty stdout"
elif ! echo "$HOOK_OUT" | jq -e '.hookSpecificOutput.additionalContext' &>/dev/null; then
  fail "$TEST_NAME" "output is not valid hook JSON: $HOOK_OUT"
else
  ADDITIONAL=$(echo "$HOOK_OUT" | jq -r '.hookSpecificOutput.additionalContext')
  if echo "$ADDITIONAL" | jq -e '.error' &>/dev/null; then
    pass "$TEST_NAME"
  else
    fail "$TEST_NAME" "expected error in additionalContext, got: $ADDITIONAL"
  fi
fi

# =========================================================================
# TEST 7: Error for malformed stdin
# =========================================================================
TEST_NAME="malformed-stdin: error reported"
set +e
HOOK_OUT=$(echo '{"this is not valid json' | idio scan --format=claude-hook 2>&1)
HOOK_RC=$?
set -e

if [[ $HOOK_RC -ne 0 ]]; then
  fail "$TEST_NAME" "exit code $HOOK_RC, expected 0"
elif [[ -z "$HOOK_OUT" ]]; then
  fail "$TEST_NAME" "expected error output, got empty stdout"
elif ! echo "$HOOK_OUT" | jq -e '.hookSpecificOutput.additionalContext' &>/dev/null; then
  fail "$TEST_NAME" "output is not valid hook JSON: $HOOK_OUT"
else
  ADDITIONAL=$(echo "$HOOK_OUT" | jq -r '.hookSpecificOutput.additionalContext')
  if echo "$ADDITIONAL" | jq -e '.error' &>/dev/null; then
    pass "$TEST_NAME"
  else
    fail "$TEST_NAME" "expected error in additionalContext, got: $ADDITIONAL"
  fi
fi

# =========================================================================
# TEST 8: Edit tool variant works
# =========================================================================
TEST_NAME="tool-variant: Edit"
CWD="$FIXTURES/few-violations"
FILE="$CWD/violations.go"
HOOK_JSON=$(build_hook "$HOOKS/edit.json" "$FILE" "$CWD")
run_hook "$HOOK_JSON"

if [[ $HOOK_RC -ne 0 ]]; then
  fail "$TEST_NAME" "exit code $HOOK_RC"
elif [[ -z "$HOOK_OUT" ]]; then
  fail "$TEST_NAME" "expected output"
elif echo "$HOOK_OUT" | jq -e '.hookSpecificOutput.additionalContext' &>/dev/null; then
  pass "$TEST_NAME"
else
  fail "$TEST_NAME" "no additionalContext in output"
fi

# =========================================================================
# TEST 9: Write tool variant works
# =========================================================================
TEST_NAME="tool-variant: Write"
CWD="$FIXTURES/few-violations"
FILE="$CWD/violations.go"
HOOK_JSON=$(build_hook "$HOOKS/write.json" "$FILE" "$CWD")
run_hook "$HOOK_JSON"

if [[ $HOOK_RC -ne 0 ]]; then
  fail "$TEST_NAME" "exit code $HOOK_RC"
elif [[ -z "$HOOK_OUT" ]]; then
  fail "$TEST_NAME" "expected output"
elif echo "$HOOK_OUT" | jq -e '.hookSpecificOutput.additionalContext' &>/dev/null; then
  pass "$TEST_NAME"
else
  fail "$TEST_NAME" "no additionalContext in output"
fi

# =========================================================================
# TEST 10: MultiEdit tool variant works
# =========================================================================
TEST_NAME="tool-variant: MultiEdit"
CWD="$FIXTURES/few-violations"
FILE="$CWD/violations.go"
HOOK_JSON=$(build_hook "$HOOKS/multiedit.json" "$FILE" "$CWD")
run_hook "$HOOK_JSON"

if [[ $HOOK_RC -ne 0 ]]; then
  fail "$TEST_NAME" "exit code $HOOK_RC"
elif [[ -z "$HOOK_OUT" ]]; then
  fail "$TEST_NAME" "expected output"
elif echo "$HOOK_OUT" | jq -e '.hookSpecificOutput.additionalContext' &>/dev/null; then
  pass "$TEST_NAME"
else
  fail "$TEST_NAME" "no additionalContext in output"
fi

# =========================================================================
# TEST 11: React few-violations — TSX file with findings
# =========================================================================
TEST_NAME="react-few-violations: TSX findings reported"
CWD="$FIXTURES/react-few-violations"
FILE="$CWD/App.tsx"
HOOK_JSON=$(build_hook "$HOOKS/tsx-edit.json" "$FILE" "$CWD")
run_hook "$HOOK_JSON"

if [[ $HOOK_RC -ne 0 ]]; then
  fail "$TEST_NAME" "exit code $HOOK_RC, expected 0"
elif [[ -z "$HOOK_OUT" ]]; then
  fail "$TEST_NAME" "expected output, got empty stdout"
elif ! echo "$HOOK_OUT" | jq -e '.hookSpecificOutput.additionalContext' &>/dev/null; then
  fail "$TEST_NAME" "output is not valid hook JSON: $HOOK_OUT"
else
  SARIF_CONTENT=$(echo "$HOOK_OUT" | jq -r '.hookSpecificOutput.additionalContext')
  RESULTS_COUNT=$(echo "$SARIF_CONTENT" | jq '.runs[0].results | length')
  if [[ "$RESULTS_COUNT" -ge 1 ]]; then
    pass "$TEST_NAME"
  else
    fail "$TEST_NAME" "expected >= 1 finding, got $RESULTS_COUNT"
  fi
fi

# =========================================================================
# TEST 12: React clean — TSX file with zero findings
# =========================================================================
TEST_NAME="react-clean: TSX zero findings"
CWD="$FIXTURES/react-clean"
FILE="$CWD/App.tsx"
HOOK_JSON=$(build_hook "$HOOKS/tsx-clean-edit.json" "$FILE" "$CWD")
run_hook "$HOOK_JSON"

if [[ $HOOK_RC -ne 0 ]]; then
  fail "$TEST_NAME" "exit code $HOOK_RC, expected 0"
elif [[ -z "$HOOK_OUT" ]]; then
  fail "$TEST_NAME" "expected SARIF output, got empty stdout"
elif ! echo "$HOOK_OUT" | jq -e '.hookSpecificOutput.additionalContext' &>/dev/null; then
  fail "$TEST_NAME" "output is not valid hook JSON: $HOOK_OUT"
else
  SARIF_CONTENT=$(echo "$HOOK_OUT" | jq -r '.hookSpecificOutput.additionalContext')
  RESULTS_COUNT=$(echo "$SARIF_CONTENT" | jq '.runs[0].results | length')
  if [[ "$RESULTS_COUNT" == "0" ]]; then
    pass "$TEST_NAME"
  else
    fail "$TEST_NAME" "expected 0 findings, got $RESULTS_COUNT"
  fi
fi

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
exit 0
