#!/usr/bin/env bash
#
# Priority-ordered parallel eval — Phase 2.C verification sweep.
# First batch: Phase 2.C-affected cases (analyzer expand path c/d,
# budget uplift, EXHAUSTIVE ENUMERATION rule, ScalarCount carve-out
# for semantic counts). Second batch: everything else.
#
# Both batches run with PARALLEL=4. Output: eval/parallel_all_summary.md

set -uo pipefail

# Phase 2.C priority set — directly affected by the 4 commits in this
# session (8a3c473 budget / 4e57a36 path c+d / eba8f31 EXHAUSTIVE /
# 1b1b728 syntactic vs semantic count split). Plus the originally-real-
# fail cases (Phase 3+1+2.B) for regression smoke.
PRIORITY_CASES=(
  # Phase 2.C path (c) / (d) targets — package handle / parent dir.
  eval/cases/s5b.case
  eval/cases/u8a.case
  eval/cases/u8b.case
  # Phase 2.B regression smoke (verify still PASS post-Phase-2.C).
  eval/cases/s7a.case
  eval/cases/s7b.case
  eval/cases/s8a.case
  # Phase 1 / Phase 2 originally-real-fail smoke.
  eval/cases/u4a.case
  eval/cases/u4b.case
  eval/cases/mr_pin_isolation.case
  # Case-spec broadens applied this session.
  eval/cases/s1b.case
  eval/cases/s11b.case
  # Jitter case (informational; expect ±50% PASS rate).
  eval/cases/qf_architecture.case
)

ALL_CASES=(eval/cases/*.case)
PARALLEL=4
TIMEOUT=1200
SWEEP_START=$(date +%Y%m%d-%H%M%S)
SUMMARY="eval/parallel_all_summary.md"

# Build "remaining" set — every case NOT in PRIORITY_CASES.
declare -A in_priority
for c in "${PRIORITY_CASES[@]}"; do
  in_priority["$c"]=1
done
REMAINING_CASES=()
for c in "${ALL_CASES[@]}"; do
  if [[ -z "${in_priority[$c]:-}" ]]; then
    REMAINING_CASES+=("$c")
  fi
done

PRIORITY_COUNT=${#PRIORITY_CASES[@]}
REMAINING_COUNT=${#REMAINING_CASES[@]}
TOTAL=$((PRIORITY_COUNT + REMAINING_COUNT))

echo "# Phase 2.C priority-ordered parallel eval sweep" >"$SUMMARY"
echo "" >>"$SUMMARY"
echo "- date: $(date -u +%Y-%m-%dT%H:%M:%SZ)" >>"$SUMMARY"
echo "- sweep_start_ts: $SWEEP_START (stale-dir filter)" >>"$SUMMARY"
echo "- baseline: 1b1b728 (Phase 2.C — budget uplift + path c/d + EXHAUSTIVE + semantic count split)" >>"$SUMMARY"
echo "- total cases: $TOTAL" >>"$SUMMARY"
echo "- parallel: $PARALLEL" >>"$SUMMARY"
echo "- timeout: ${TIMEOUT}s per case" >>"$SUMMARY"
echo "- batch 1 (priority): $PRIORITY_COUNT cases" >>"$SUMMARY"
echo "- batch 2 (remaining): $REMAINING_COUNT cases" >>"$SUMMARY"
echo "" >>"$SUMMARY"
echo "| # | case | verdict | reason | sec |" >>"$SUMMARY"
echo "|--:|------|---------|--------|----:|" >>"$SUMMARY"

run_one() {
  local idx="$1"
  local case_file="$2"
  local case_id
  case_id=$(basename "$case_file" .case)
  local start_ts
  start_ts=$(date +%s)
  timeout "$TIMEOUT" bash eval/run.sh "$case_file" 1 >/dev/null 2>&1
  local rc=$?
  local end_ts
  end_ts=$(date +%s)
  local elapsed=$((end_ts - start_ts))
  local verdict="UNKNOWN"
  local reason="-"
  local dir=""
  for d in $(ls -dt eval/results/"${case_id}"-* 2>/dev/null); do
    local ts
    ts=$(basename "$d" | sed "s/${case_id}-//")
    if [[ "$ts" > "$SWEEP_START" ]] || [[ "$ts" == "$SWEEP_START"* ]]; then
      dir="$d"
      break
    fi
  done
  if [[ -n "$dir" && -f "$dir/run-1.verdict" ]]; then
    verdict=$(head -1 "$dir/run-1.verdict")
    reason=$(awk '/reasons:/{flag=1;next}flag' "$dir/run-1.verdict" | head -1 | tr -d '\n' | head -c 120)
    if [[ -z "$reason" || "$reason" == "" ]]; then
      reason="-"
    fi
  fi
  if [[ $rc -eq 124 ]]; then
    verdict="TIMEOUT"
    reason="exceeded ${TIMEOUT}s wall-time"
  elif [[ -z "$dir" ]]; then
    verdict="LAUNCH_FAIL"
    reason="no fresh result dir produced (LLM API or script error)"
  fi
  printf "| %d | %s | %s | %s | %ds |\n" "$idx" "$case_id" "$verdict" "$reason" "$elapsed" >>"$SUMMARY"
  echo "[$(date +%H:%M:%S)] [$idx/$TOTAL] $case_id → $verdict (${elapsed}s)" >&2
}

run_batch() {
  local label="$1"
  shift
  local -a cases=("$@")
  local count=${#cases[@]}
  echo "[$(date +%H:%M:%S)] === batch $label ($count cases, parallel=$PARALLEL) ===" >&2
  local i=0
  while [[ $i -lt $count ]]; do
    for ((j = 0; j < PARALLEL && i < count; j++)); do
      case_file="${cases[$i]}"
      # Global idx = position across both batches
      idx=$((BASE_IDX + i + 1))
      run_one "$idx" "$case_file" &
      i=$((i + 1))
    done
    wait
  done
}

BASE_IDX=0
echo "" >>"$SUMMARY"
echo "## Batch 1 — Phase 2.C priority cases" >>"$SUMMARY"
echo "" >>"$SUMMARY"
run_batch "1 (priority)" "${PRIORITY_CASES[@]}"
BASE_IDX=$PRIORITY_COUNT

echo "" >>"$SUMMARY"
echo "## Batch 2 — remaining cases" >>"$SUMMARY"
echo "" >>"$SUMMARY"
if [[ $REMAINING_COUNT -gt 0 ]]; then
  run_batch "2 (remaining)" "${REMAINING_CASES[@]}"
fi

# Rollup.
echo "" >>"$SUMMARY"
total_pass=$(grep -c '| PASS |' "$SUMMARY" || true)
total_fail=$(grep -cE '\| FAIL |\| TIMEOUT |\| LAUNCH_FAIL ' "$SUMMARY" || true)
echo "**Pass: $total_pass / $TOTAL — Fail/Timeout/LaunchFail: $total_fail**" >>"$SUMMARY"
echo "[$(date +%H:%M:%S)] sweep complete — pass=$total_pass fail=$total_fail of $TOTAL" >&2
