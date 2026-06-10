#!/usr/bin/env bash
# Polyglot eval harness with optional retry budget.
# Args: <src> <task> [retry_budget]
set -uo pipefail

CODRAX="/home/chatpp/codrax/codrax"
# Prepend rustup-installed cargo (1.95+) ahead of system /usr/bin/cargo
# (1.75) so polyglot rust tasks that need edition2024 / newer features
# work. No-op if ~/.cargo/bin doesn't exist.
export PATH="$HOME/.cargo/bin:$PATH"
SRC="$1"
TASK="$2"
RETRY="${3:-0}"   # pipeline_write_retry_budget, 0 = no retry, max 5
WORKDIR="/tmp/codrax-eval/runs/$TASK-r$RETRY"
PLAN_OUT="/tmp/codrax-eval/runs/$TASK-r$RETRY.plan.json"
SETTINGS_FILE="/tmp/codrax-eval/runs/$TASK-r$RETRY.codrax.yaml"

rm -rf "$WORKDIR" "$PLAN_OUT"
mkdir -p "$WORKDIR"
if [[ ! -d "$SRC" ]]; then
  echo "===== TASK: $TASK ====="
  echo "VERDICT: $TASK SRC_MISSING (path '$SRC' does not exist)" >&2
  echo "VERDICT: $TASK SRC_MISSING (path '$SRC' does not exist)"
  exit 2
fi
cp -r "$SRC"/. "$WORKDIR"/

cd "$WORKDIR"

# Per-language pre-setup before git init.
# JS: install jest + babel deps via package.json (~30s first time, fast on cache).
if [[ -f "$WORKDIR/package.json" ]]; then
  npm install --silent --no-fund --no-audit --no-progress 2>&1 | tail -3 >/dev/null
fi

git init -q -b main
git add -A
git -c user.email=eval@x -c user.name=eval commit -q -m "baseline (stub)" >/dev/null 2>&1

INSTRUCTIONS=""
if [[ -f .docs/instructions.md ]]; then
  INSTRUCTIONS=$(cat .docs/instructions.md)
fi

# Detect "design-a-test-suite" exercises (Exercism's deprecated `counter`
# is the only one in the polyglot subset, but the pattern generalises).
# These ship a stub source FILE plus an EMPTY test file, then ask the
# learner to write tests against multiple given implementations. The
# default "implement to pass tests" REQUEST is reversed for them — the
# task is to author tests, not implement code. Skip with a clear
# verdict so they don't drag the batch into UNKNOWN/FAIL noise.
if echo "$INSTRUCTIONS" | grep -qiE 'design a test suite|you get to define the test|Instead of creating code that works with an existing test'; then
  echo "===== TASK: $TASK (retry=$RETRY) ====="
  echo "VERDICT: $TASK retry=$RETRY SKIPPED  (fixture is design-a-test-suite type — REQUEST template inverted)"
  exit 0
fi

REQUEST="Implement the function(s) defined in this exercise so that the test file passes.

INSTRUCTIONS:
$INSTRUCTIONS

Read the existing stub file and the test file to understand the contract; emit a ChangePlan with kind=modify (or kind=patch) on the stub source file with the working implementation. Do NOT modify the test file."

# Custom settings: write_enabled + retry budget + keep worktree on success.
cat > "$SETTINGS_FILE" <<EOF
write_enabled: true
pipeline_write_retry_budget: $RETRY
pipeline_keep_worktree_on_success: true
EOF
export CODRAX_SETTINGS="$SETTINGS_FILE"

T0=$(date +%s)
echo "===== TASK: $TASK (retry=$RETRY) ====="
echo "[1/2] plan"
"$CODRAX" --repo "$WORKDIR" --mode=write --write-phase=plan --request "$REQUEST" --plan-out "$PLAN_OUT" >/tmp/codrax-eval/runs/$TASK-r$RETRY.plan.out 2>&1 || true
PLAN_DUR=$(($(date +%s) - T0))
if [[ ! -s "$PLAN_OUT" ]]; then
  echo "VERDICT: $TASK retry=$RETRY PLAN_FAIL ($PLAN_DUR s) -- plan tail:"
  tail -6 /tmp/codrax-eval/runs/$TASK-r$RETRY.plan.out
  exit 1
fi
echo "  plan ok ($(wc -c < $PLAN_OUT) bytes, ${PLAN_DUR}s)"

T1=$(date +%s)
echo "[2/2] apply (auto, retry=$RETRY)"
"$CODRAX" --repo "$WORKDIR" --mode=write --write-phase=apply --plan-file "$PLAN_OUT" --auto-apply --request "$REQUEST" \
  >/tmp/codrax-eval/runs/$TASK-r$RETRY.apply.out 2>&1 || true
APPLY_DUR=$(($(date +%s) - T1))

# Try report.json first (success path); fall back to stderr regex (failure path).
# In --mode=write --write-phase=apply --plan-file workflow, saveChangeReport writes to the
# plan file's directory (= /tmp/codrax-eval/runs/), not WORKDIR/.codrax/plans/.
#
# Lookup order matters when verify→plan retry fires:
#   1. Apply.out's terminal "Report saved to .../<plan-id>.report.json"
#      line names the FINAL plan ID (post-retry). We extract the plan-id
#      from the most-recent such line and join against runs/.
#   2. Original PLAN_OUT plan-id — only correct when no retry fired.
#   3. WORKDIR's .codrax/plans/ — last-resort scan.
APPLY_OUT="/tmp/codrax-eval/runs/$TASK-r$RETRY.apply.out"
PLAN_ID=""
if [[ -f "$APPLY_OUT" ]]; then
  # Extract last-mentioned plan-id from "Report saved to ...<plan-id>.report.json"
  PLAN_ID=$(grep -oE 'plan-[0-9]+-[0-9]+' "$APPLY_OUT" | tail -1)
fi
if [[ -z "$PLAN_ID" ]]; then
  PLAN_ID=$(python3 -c "import json,sys; print(json.load(open('$PLAN_OUT'))['id'])" 2>/dev/null)
fi
REPORT=""
if [[ -n "$PLAN_ID" ]] && [[ -f "/tmp/codrax-eval/runs/${PLAN_ID}.report.json" ]]; then
  REPORT="/tmp/codrax-eval/runs/${PLAN_ID}.report.json"
fi
if [[ -z "$REPORT" ]]; then
  REPORT=$(ls -t "$WORKDIR"/.codrax/plans/*.report.json 2>/dev/null | head -1)
fi
if [[ -n "$REPORT" ]]; then
  read PASSED TOTAL ALL_PASSED < <(python3 -c "
import json
r = json.load(open('$REPORT'))
tr = r.get('test_results') or []
total = sum(1 for t in tr if (t.get('kind') or 'unit') == 'unit')
passed = sum(1 for t in tr if (t.get('kind') or 'unit') == 'unit' and t.get('passed'))
print(passed, total, 'true' if r.get('passed') else 'false')
" 2>/dev/null)
elif grep -qE "verify failed: \[.*\] .* — [0-9]+ passed, [0-9]+ failed" /tmp/codrax-eval/runs/$TASK-r$RETRY.apply.out 2>/dev/null; then
  # parse "21 passed, 1 failed, 0 error, 0 skipped of 22 total" (pytest/jest/rspec shape)
  read PASSED TOTAL < <(grep -oE "[0-9]+ passed, [0-9]+ failed, [0-9]+ error, [0-9]+ skipped of [0-9]+ total" /tmp/codrax-eval/runs/$TASK-r$RETRY.apply.out | tail -1 | awk '{print $1, $11}')
  ALL_PASSED="false"
elif grep -qE "verify failed: \[(go|rust|cargo)\] [0-9]+ test\(s\) failed across [0-9]+ total" /tmp/codrax-eval/runs/$TASK-r$RETRY.apply.out 2>/dev/null; then
  # parse Go/Rust runner shape: "X test(s) failed across Y total"
  read FAILED TOTAL < <(grep -oE "[0-9]+ test\(s\) failed across [0-9]+ total" /tmp/codrax-eval/runs/$TASK-r$RETRY.apply.out | tail -1 | awk '{print $1, $5}')
  PASSED=$((TOTAL - FAILED))
  ALL_PASSED="false"
else
  PASSED="?"
  TOTAL="?"
  ALL_PASSED="?"
fi

if [[ "$ALL_PASSED" == "true" ]]; then
  echo "VERDICT: $TASK retry=$RETRY PASS  (tests=$PASSED/$TOTAL plan=${PLAN_DUR}s apply=${APPLY_DUR}s)"
elif [[ "$ALL_PASSED" == "false" ]]; then
  echo "VERDICT: $TASK retry=$RETRY FAIL  (tests=$PASSED/$TOTAL plan=${PLAN_DUR}s apply=${APPLY_DUR}s)"
else
  echo "VERDICT: $TASK retry=$RETRY UNKNOWN  (plan=${PLAN_DUR}s apply=${APPLY_DUR}s) — see apply.out"
fi
