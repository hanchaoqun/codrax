#!/usr/bin/env bash
#
# Parallel runner — fires N eval cases at once, waits for the batch
# before starting the next.
#
# Usage: bash eval/parallel_all.sh
#   Optional env vars:
#     PARALLEL=4       (default 4) — concurrent case workers
#     TIMEOUT=1200     per-case wall-time cap (seconds)
#     CASES_GLOB="eval/cases/*.case"  — restrict the sweep to a subset
#   Output:
#     eval/parallel_all_summary.md   — PASS/FAIL per case + rollup
#
# 2026-05-08 v2: TIMEOUT=1200s (was 600 — too short for big repo +
# complex questions). Stale-result-dir guard: only consider dirs whose
# basename timestamp is >= sweep_start, preventing the previous bug
# where a never-launched case (LLM API blip) read a stale 05-02 PASS.
#
# 2026-05-10 v3 (P5-B follow-up): default PARALLEL=4 (was 3),
# env-var-tunable. Post-sweep digest now extracts per-stage dispatch
# counts + repair-loop activation per case so the operator can judge
# answer quality + pipeline efficiency without per-case ls'ing.

set -uo pipefail

CASES_GLOB="${CASES_GLOB:-eval/cases/*.case}"
# shellcheck disable=SC2206
CASES=($CASES_GLOB)
TOTAL=${#CASES[@]}
PARALLEL="${PARALLEL:-4}"
TIMEOUT="${TIMEOUT:-1200}"
SWEEP_START=$(date +%Y%m%d-%H%M%S)
SUMMARY="eval/parallel_all_summary.md"

echo "# Parallel eval sweep — TypedDenials + BugClass regression check" >"$SUMMARY"
echo "" >>"$SUMMARY"
echo "- date: $(date -u +%Y-%m-%dT%H:%M:%SZ)" >>"$SUMMARY"
echo "- sweep_start_ts: $SWEEP_START (stale-dir filter)" >>"$SUMMARY"
echo "- baseline: b10fd9f (post-TypedDenials + BugClass + multi-repo)" >>"$SUMMARY"
echo "- total cases: $TOTAL" >>"$SUMMARY"
echo "- parallel: $PARALLEL" >>"$SUMMARY"
echo "- timeout: ${TIMEOUT}s per case" >>"$SUMMARY"
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
  # Find the latest result dir whose timestamp suffix is >= sweep
  # start (don't read stale dirs from previous sweeps).
  local dir=""
  for d in $(ls -dt eval/results/"${case_id}"-* 2>/dev/null); do
    local ts
    ts=$(basename "$d" | sed "s/${case_id}-//")
    # Lexicographic compare works because format is YYYYMMDD-HHMMSS
    if [[ "$ts" > "$SWEEP_START" ]] || [[ "$ts" == "$SWEEP_START"* ]]; then
      dir="$d"
      break
    fi
  done
  if [[ -n "$dir" && -f "$dir/run-1.verdict" ]]; then
    local first_line
    first_line=$(head -1 "$dir/run-1.verdict")
    verdict=$(echo "$first_line" | awk '{print $1}')
    reason=$(echo "$first_line" | cut -d' ' -f2- | head -c 120)
    if [[ -z "$reason" || "$reason" == "$verdict" ]]; then
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

i=0
while [[ $i -lt $TOTAL ]]; do
  for ((j = 0; j < PARALLEL && i < TOTAL; j++)); do
    case_file="${CASES[$i]}"
    idx=$((i + 1))
    run_one "$idx" "$case_file" &
    i=$((i + 1))
  done
  wait
done

# Tail rollup.
echo "" >>"$SUMMARY"
total_pass=$(grep -c '| PASS |' "$SUMMARY" || true)
total_fail=$(grep -cE '\| FAIL |\| TIMEOUT |\| LAUNCH_FAIL ' "$SUMMARY" || true)
echo "**Pass: $total_pass / $TOTAL — Fail/Timeout/LaunchFail: $total_fail**" >>"$SUMMARY"

# Pipeline-efficiency digest — per case extracts the dispatch counts
# and repair-loop activation. Helps the operator audit answer quality
# at a glance: a PASS with high finalizer/repair counts still warrants
# scrutiny, even if the verdict is green.
echo "" >>"$SUMMARY"
echo "## Pipeline efficiency digest" >>"$SUMMARY"
echo "" >>"$SUMMARY"
echo "| case | analyze | explore | extract | finalize | repair_plan | repair_exec | sem_qa |" >>"$SUMMARY"
echo "|------|--------:|--------:|--------:|---------:|------------:|------------:|-------:|" >>"$SUMMARY"
metric_field() {
  # $1 = metrics file path, $2 = key name; emits the value or '-'.
  local v
  v=$(grep -oE "^${2}=[0-9]+" "$1" 2>/dev/null | head -1 | cut -d= -f2)
  echo "${v:--}"
}
for case_file in "${CASES[@]}"; do
  case_id=$(basename "$case_file" .case)
  digest_dir=""
  for d in $(ls -dt eval/results/"${case_id}"-* 2>/dev/null); do
    ts=$(basename "$d" | sed "s/${case_id}-//")
    if [[ "$ts" > "$SWEEP_START" ]] || [[ "$ts" == "$SWEEP_START"* ]]; then
      digest_dir="$d"
      break
    fi
  done
  m="${digest_dir}/run-1.metrics.txt"
  if [[ -z "$digest_dir" || ! -f "$m" ]]; then
    printf "| %s | - | - | - | - | - | - | - |\n" "$case_id" >>"$SUMMARY"
    continue
  fi
  printf "| %s | %s | %s | %s | %s | %s | %s | %s |\n" \
    "$case_id" \
    "$(metric_field "$m" analyzer_dispatches)" \
    "$(metric_field "$m" explorer_dispatches)" \
    "$(metric_field "$m" extractor_dispatches)" \
    "$(metric_field "$m" finalizer_dispatches)" \
    "$(metric_field "$m" repair_plan_lines)" \
    "$(metric_field "$m" repair_exec_lines)" \
    "$(metric_field "$m" semantic_quality_dispatches)" >>"$SUMMARY"
done

echo "[$(date +%H:%M:%S)] sweep complete — pass=$total_pass fail=$total_fail of $TOTAL" >&2
