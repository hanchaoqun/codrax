#!/usr/bin/env bash
#
# Parallel runner — keeps up to N eval cases running at once.
#
# Usage: bash eval/parallel_all.sh
#   Optional env vars:
#     PARALLEL=4       (default 4) — concurrent case workers
#     TIMEOUT=1200     per-case wall-time cap (seconds)
#     CASES_GLOB="eval/cases/*.case"  — restrict the sweep to a subset
#     RANDOMIZE=1      shuffle case order before launching workers
#     RANDOMIZE_SEED=N optional deterministic shuffle seed
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

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=eval/runner_lib.sh
source "$SCRIPT_DIR/runner_lib.sh"

CASES_GLOB="${CASES_GLOB:-eval/cases/*.case}"
# shellcheck disable=SC2206
CASES=($CASES_GLOB)
RANDOMIZE="${RANDOMIZE:-0}"
RANDOMIZE_SEED="${RANDOMIZE_SEED:-$(date +%s)}"
if [[ "$RANDOMIZE" == "1" || "$RANDOMIZE" == "true" ]]; then
  # shellcheck disable=SC2207
  CASES=($(
    printf '%s\n' "${CASES[@]}" |
      awk -v seed="$RANDOMIZE_SEED" 'BEGIN { srand(seed) } { printf "%.17f\t%s\n", rand(), $0 }' |
      sort -n |
      cut -f2-
  ))
fi
TOTAL=${#CASES[@]}
PARALLEL="${PARALLEL:-4}"
TIMEOUT="${TIMEOUT:-1200}"
RESULTS_ROOT="${EVAL_RESULTS_ROOT:-eval/results}"
SWEEP_START=$(date +%Y%m%d-%H%M%S)
SUMMARY="eval/parallel_all_summary.md"

# 2026-05-10 — defensive binary snapshot. Sweep hits a wave of
# LAUNCH_FAIL when a concurrent `go build` (developer iterating in
# the same tree) rewrites ./codrax mid-flight; running workers
# get ETXTBSY / "exec format error" / immediate exit. Snapshot
# the binary to a sweep-private name BUT keep it in the SAME
# directory as ./codrax — codrax's providers.yaml / codrax.yaml
# lookup is anchored to exeDir, so the snapshot must sit beside
# the original config files.
#
# CODRAX_BIN is read by eval/run.sh — when unset / empty, falls
# back to ./codrax (legacy behaviour).
SWEEP_BIN="./.codrax-sweep-${SWEEP_START}"
if [[ -x "./codrax" ]]; then
  if cp ./codrax "$SWEEP_BIN" 2>/dev/null; then
    chmod +x "$SWEEP_BIN" 2>/dev/null || true
    export CODRAX_BIN="$SWEEP_BIN"
    trap 'rm -f "$SWEEP_BIN"' EXIT
    echo "[$(date +%H:%M:%S)] sweep binary snapshot: $SWEEP_BIN" >&2
  fi
fi

echo "# Parallel eval sweep — TypedDenials + BugClass regression check" >"$SUMMARY"
echo "" >>"$SUMMARY"
echo "- date: $(date -u +%Y-%m-%dT%H:%M:%SZ)" >>"$SUMMARY"
echo "- sweep_start_ts: $SWEEP_START (stale-dir filter)" >>"$SUMMARY"
echo "- baseline: b10fd9f (post-TypedDenials + BugClass + multi-repo)" >>"$SUMMARY"
echo "- total cases: $TOTAL" >>"$SUMMARY"
echo "- parallel: $PARALLEL" >>"$SUMMARY"
echo "- timeout: ${TIMEOUT}s per case" >>"$SUMMARY"
echo "- results_root: $RESULTS_ROOT" >>"$SUMMARY"
if [[ "$RANDOMIZE" == "1" || "$RANDOMIZE" == "true" ]]; then
  echo "- randomized_order: true (seed=$RANDOMIZE_SEED)" >>"$SUMMARY"
else
  echo "- randomized_order: false" >>"$SUMMARY"
fi
echo "" >>"$SUMMARY"

if [[ "${EVAL_PROVIDER_PREFLIGHT:-1}" != "0" && "${EVAL_PROVIDER_PREFLIGHT:-1}" != "false" ]]; then
  PREFLIGHT_DIR="${RESULTS_ROOT}/provider-preflight-${SWEEP_START}"
  echo "[$(date +%H:%M:%S)] provider preflight start" >&2
  provider_blocked="$(eval_provider_preflight "$CODRAX_BIN" "$PWD" "$PREFLIGHT_DIR" "${EVAL_PROVIDER_PREFLIGHT_TIMEOUT:-120}")"
  if [[ -n "$provider_blocked" ]]; then
    echo "- provider_preflight: BLOCKED_PROVIDER $provider_blocked" >>"$SUMMARY"
    echo "- provider_preflight_dir: $PREFLIGHT_DIR" >>"$SUMMARY"
    echo "" >>"$SUMMARY"
    echo "**provider-blocked before launching cases: $provider_blocked**" >>"$SUMMARY"
    echo "[$(date +%H:%M:%S)] provider preflight blocked: $provider_blocked" >&2
    exit 0
  fi
  echo "- provider_preflight: ok" >>"$SUMMARY"
  echo "- provider_preflight_dir: $PREFLIGHT_DIR" >>"$SUMMARY"
  echo "" >>"$SUMMARY"
fi

echo "| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self |" >>"$SUMMARY"
echo "|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|" >>"$SUMMARY"

run_one() {
  local idx="$1"
  local case_file="$2"
  local case_id
  case_id=$(basename "$case_file" .case)
  local start_ts
  start_ts=$(date +%s)
  eval_run_with_timeout "$TIMEOUT" bash eval/run.sh "$case_file" 1 >/dev/null 2>&1
  local rc=$?
  local end_ts
  end_ts=$(date +%s)
  local elapsed=$((end_ts - start_ts))
  local verdict="UNKNOWN"
  local reason="-"
  # Find the latest result dir whose timestamp suffix is >= sweep
  # start (don't read stale dirs from previous sweeps).
  local dir=""
  dir=$(eval_latest_result_dir "$RESULTS_ROOT" "$case_id" "$SWEEP_START" 2>/dev/null || true)
  if [[ -n "$dir" && ( "$rc" -ne 0 || ! -f "$dir/run-1.metrics.txt" || ! -f "$dir/run-1.verdict" ) ]]; then
    eval_materialize_partial_run_result "$dir" 1 "$rc" "$elapsed" "eval_worker_incomplete"
  fi
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
  local metrics="$dir/run-1.metrics.txt"
  local log=""
  if [[ -n "$dir" ]]; then
    log=$(ls -t "$dir"/run-1.logs/codrax-*.log 2>/dev/null | head -1)
  fi
  local analyzer explorer extractor finalizer repair rejects patches sem self
  analyzer=$(eval_metric_field "$metrics" analyzer_dispatches)
  explorer=$(eval_metric_field "$metrics" explorer_dispatches)
  extractor=$(eval_metric_field "$metrics" extractor_dispatches)
  finalizer=$(eval_metric_field "$metrics" finalizer_dispatches)
  repair=$(eval_metric_field "$metrics" repair_plan_lines)
  rejects=$(eval_count_finalizer_rejects "$log")
  patches=$(eval_count_answer_document_patch_calls "$log")
  sem=$(eval_count_semantic_quality_concerns "$log")
  self=$(eval_count_self_consistency_concerns "$log")
  printf "| %d | %s | %s | %s | %ds | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n" \
    "$idx" "$case_id" "$verdict" "$reason" "$elapsed" \
    "$analyzer" "$explorer" "$extractor" "$finalizer" "$repair" "$rejects" "$patches" "$sem" "$self" >>"$SUMMARY"
  echo "[$(date +%H:%M:%S)] [$idx/$TOTAL] $case_id → $verdict (${elapsed}s)" >&2
}

for ((i = 0; i < TOTAL; i++)); do
  eval_wait_for_slot "$PARALLEL"
  case_file="${CASES[$i]}"
  idx=$((i + 1))
  run_one "$idx" "$case_file" &
done
wait

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
for case_file in "${CASES[@]}"; do
  case_id=$(basename "$case_file" .case)
  digest_dir=$(eval_latest_result_dir "$RESULTS_ROOT" "$case_id" "$SWEEP_START" 2>/dev/null || true)
  m="${digest_dir}/run-1.metrics.txt"
  if [[ -z "$digest_dir" || ! -f "$m" ]]; then
    printf "| %s | - | - | - | - | - | - | - |\n" "$case_id" >>"$SUMMARY"
    continue
  fi
  printf "| %s | %s | %s | %s | %s | %s | %s | %s |\n" \
    "$case_id" \
    "$(eval_metric_field "$m" analyzer_dispatches)" \
    "$(eval_metric_field "$m" explorer_dispatches)" \
    "$(eval_metric_field "$m" extractor_dispatches)" \
    "$(eval_metric_field "$m" finalizer_dispatches)" \
    "$(eval_metric_field "$m" repair_plan_lines)" \
    "$(eval_metric_field "$m" repair_exec_lines)" \
    "$(eval_metric_field "$m" semantic_quality_dispatches)" >>"$SUMMARY"
done

echo "[$(date +%H:%M:%S)] sweep complete — pass=$total_pass fail=$total_fail of $TOTAL" >&2
