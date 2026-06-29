#!/usr/bin/env bash
#
# Run an explicit eval case list with bounded concurrency.
#
# Usage:
#   PARALLEL=2 TIMEOUT=1200 bash eval/parallel_selected.sh \
#     eval/cases/harmony/cangjie_repomap.case \
#     eval/cases/qf_relation_subagent_registry.case
#
# This is the operator-safe path for representative batches. It reuses
# runner_lib.sh timeout/process-group cleanup and snapshots ./codrax once,
# avoiding host-specific timeout(1) commands and ad-hoc shell job control.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=eval/runner_lib.sh
source "$SCRIPT_DIR/runner_lib.sh"

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <case-file> [<case-file> ...]" >&2
  exit 2
fi

PARALLEL="${PARALLEL:-2}"
TIMEOUT="${TIMEOUT:-1200}"
RESULTS_ROOT="${EVAL_RESULTS_ROOT:-eval/results}"
SWEEP_START="$(date +%Y%m%d-%H%M%S)"
SUMMARY="${EVAL_SELECTED_SUMMARY:-eval/parallel_selected_summary.md}"
case "$SUMMARY" in
  *.md)
    DEFAULT_MANUAL_AUDIT="${SUMMARY%.md}_manual_audit.md"
    ;;
  *)
    DEFAULT_MANUAL_AUDIT="${SUMMARY}.manual_audit.md"
    ;;
esac
MANUAL_AUDIT="${EVAL_SELECTED_MANUAL_AUDIT:-$DEFAULT_MANUAL_AUDIT}"
TOTAL="$#"

case "$PARALLEL" in
  ""|*[!0-9]*)
    echo "PARALLEL must be a positive integer" >&2
    exit 2
    ;;
esac
if [[ "$PARALLEL" -lt 1 ]]; then
  echo "PARALLEL must be >= 1" >&2
  exit 2
fi

case "$TIMEOUT" in
  ""|*[!0-9]*)
    echo "TIMEOUT must be a positive integer" >&2
    exit 2
    ;;
esac
if [[ "$TIMEOUT" -lt 1 ]]; then
  echo "TIMEOUT must be >= 1" >&2
  exit 2
fi

for case_file in "$@"; do
  if [[ ! -f "$case_file" ]]; then
    echo "case file not found: $case_file" >&2
    exit 2
  fi
done

SWEEP_BIN="./.codrax-selected-${SWEEP_START}"
cleanup() {
  local pids
  pids="$(jobs -pr || true)"
  if [[ -n "$pids" ]]; then
    # shellcheck disable=SC2086
    kill $pids 2>/dev/null || true
    sleep 1
    pids="$(jobs -pr || true)"
    if [[ -n "$pids" ]]; then
      # shellcheck disable=SC2086
      kill -9 $pids 2>/dev/null || true
    fi
  fi
  rm -f "$SWEEP_BIN"
}
on_signal() {
  local rc="$1"
  cleanup
  exit "$rc"
}
trap cleanup EXIT
trap 'on_signal 130' INT
trap 'on_signal 143' TERM

if [[ -x "./codrax" ]]; then
  if cp ./codrax "$SWEEP_BIN" 2>/dev/null; then
    chmod +x "$SWEEP_BIN" 2>/dev/null || true
    export CODRAX_BIN="$SWEEP_BIN"
    echo "[$(date +%H:%M:%S)] selected eval binary snapshot: $SWEEP_BIN" >&2
  fi
fi

echo "# Selected parallel eval sweep" >"$SUMMARY"
echo "" >>"$SUMMARY"
echo "- date: $(date -u +%Y-%m-%dT%H:%M:%SZ)" >>"$SUMMARY"
echo "- sweep_start_ts: $SWEEP_START" >>"$SUMMARY"
echo "- total cases: $TOTAL" >>"$SUMMARY"
echo "- parallel: $PARALLEL" >>"$SUMMARY"
echo "- timeout: ${TIMEOUT}s per case" >>"$SUMMARY"
echo "- results_root: $RESULTS_ROOT" >>"$SUMMARY"
echo "" >>"$SUMMARY"
echo "| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |" >>"$SUMMARY"
echo "|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|" >>"$SUMMARY"

echo "# Selected Eval Manual Audit Scaffold" >"$MANUAL_AUDIT"
echo "" >>"$MANUAL_AUDIT"
echo "- date: $(date -u +%Y-%m-%dT%H:%M:%SZ)" >>"$MANUAL_AUDIT"
echo "- sweep_start_ts: $SWEEP_START" >>"$MANUAL_AUDIT"
echo "- total cases: $TOTAL" >>"$MANUAL_AUDIT"
echo "- parallel: $PARALLEL" >>"$MANUAL_AUDIT"
echo "- timeout: ${TIMEOUT}s per case" >>"$MANUAL_AUDIT"
echo "- results_root: $RESULTS_ROOT" >>"$MANUAL_AUDIT"
echo "" >>"$MANUAL_AUDIT"
echo "This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement." >>"$MANUAL_AUDIT"
echo "" >>"$MANUAL_AUDIT"
echo "| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |" >>"$MANUAL_AUDIT"
echo "|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|" >>"$MANUAL_AUDIT"

eval_selected_wait_for_slot() {
  while [[ "$(jobs -pr | wc -l | tr -d ' ')" -ge "$PARALLEL" ]]; do
    sleep 1
  done
}

eval_selected_run_one() {
  local idx="$1"
  local case_file="$2"
  local case_id start_ts end_ts elapsed rc dir verdict reason metrics log analyzer explorer extractor finalizer repair rejects patches sem self runtime_auth
  local oracle_surface ctx_pct read_calls repo_map_calls list_calls trace_calls source_lens unavailable prunes midloop inv_calls inv_rejects tool_summary churn_summary
  case_id="$(basename "$case_file" .case)"
  start_ts="$(date +%s)"
  echo "[$(date +%H:%M:%S)] [$idx/$TOTAL] start $case_id" >&2
  eval_run_with_timeout "$TIMEOUT" bash eval/run.sh "$case_file" 1 >/dev/null 2>&1
  rc=$?
  end_ts="$(date +%s)"
  elapsed=$((end_ts - start_ts))
  dir="$(eval_latest_result_dir "$RESULTS_ROOT" "$case_id" "$SWEEP_START" 2>/dev/null || true)"
  if [[ -n "$dir" && ( "$rc" -ne 0 || ! -f "$dir/run-1.metrics.txt" || ! -f "$dir/run-1.verdict" ) ]]; then
    eval_materialize_partial_run_result "$dir" 1 "$rc" "$elapsed" "selected_eval_worker_incomplete"
  fi
  verdict="UNKNOWN"
  reason="-"
  if [[ -n "$dir" && -f "$dir/run-1.verdict" ]]; then
    local first_line
    first_line="$(head -1 "$dir/run-1.verdict")"
    verdict="$(echo "$first_line" | awk '{print $1}')"
    reason="$(echo "$first_line" | cut -d' ' -f2- | head -c 120)"
    if [[ -z "$reason" || "$reason" == "$verdict" ]]; then
      reason="-"
    fi
  fi
  if [[ "$rc" -eq 124 ]]; then
    verdict="TIMEOUT"
    reason="exceeded ${TIMEOUT}s wall-time"
  elif [[ -z "$dir" ]]; then
    verdict="LAUNCH_FAIL"
    reason="no fresh result dir produced"
  fi
  metrics="$dir/run-1.metrics.txt"
  log=""
  if [[ -n "$dir" ]]; then
    log="$(ls -t "$dir"/run-1.logs/codrax-*.log 2>/dev/null | head -1)"
  fi
  analyzer="$(eval_metric_field "$metrics" analyzer_dispatches)"
  explorer="$(eval_metric_field "$metrics" explorer_dispatches)"
  extractor="$(eval_metric_field "$metrics" extractor_dispatches)"
  finalizer="$(eval_metric_field "$metrics" finalizer_dispatches)"
  repair="$(eval_metric_field "$metrics" repair_plan_lines)"
  rejects="$(eval_count_finalizer_rejects "$log")"
  patches="$(eval_count_answer_document_patch_calls "$log")"
  sem="$(eval_count_semantic_quality_concerns "$log")"
  self="$(eval_count_self_consistency_concerns "$log")"
  runtime_auth="$(eval_metric_field "$metrics" runtime_authority_path)"
  printf "| %d | %s | %s | %s | %ds | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n" \
    "$idx" "$case_id" "$verdict" "$reason" "$elapsed" \
    "$analyzer" "$explorer" "$extractor" "$finalizer" "$repair" "$rejects" "$patches" "$sem" "$self" "$runtime_auth" "$dir" >>"$SUMMARY"

  oracle_surface="$(eval_case_oracle_surface "$case_file")"
  ctx_pct="$(eval_metric_field "$metrics" max_context_window_pct)"
  read_calls="$(eval_metric_field "$metrics" tool_read_file)"
  repo_map_calls="$(eval_metric_field "$metrics" tool_repo_map)"
  list_calls="$(eval_metric_field "$metrics" tool_list_files)"
  trace_calls="$(eval_metric_field "$metrics" tool_trace_query)"
  source_lens="$(eval_metric_field "$metrics" source_inventory_lens)"
  unavailable="$(eval_metric_field "$metrics" unavailable_tool_attempts)"
  prunes="$(eval_metric_field "$metrics" tool_history_prunes)"
  midloop="$(eval_metric_field "$metrics" midloop_inject)"
  inv_calls="$(eval_metric_field "$metrics" investigation_complete_calls)"
  inv_rejects="$(eval_metric_field "$metrics" investigation_complete_rejects)"
  tool_summary="read=${read_calls},repo_map=${repo_map_calls},list=${list_calls},trace=${trace_calls},source_lens=${source_lens}"
  churn_summary="midloop=${midloop},inv=${inv_calls}/${inv_rejects},fin_reject=${rejects},unavail=${unavailable},prune=${prunes}"
  printf "| %d | %s | %s | %s | %s | %s | %ds | %s | %s | %s | TODO | TODO |\n" \
    "$idx" "$case_id" "$verdict" "$dir" "$oracle_surface" "$runtime_auth" "$elapsed" "$ctx_pct" "$tool_summary" "$churn_summary" >>"$MANUAL_AUDIT"
  echo "[$(date +%H:%M:%S)] [$idx/$TOTAL] done $case_id -> $verdict (${elapsed}s)" >&2
}

idx=0
for case_file in "$@"; do
  idx=$((idx + 1))
  eval_selected_wait_for_slot
  eval_selected_run_one "$idx" "$case_file" &
done
wait

pass_count="$(grep -c '| PASS |' "$SUMMARY" || true)"
fail_count="$(grep -cE '\| FAIL |\| TIMEOUT |\| LAUNCH_FAIL ' "$SUMMARY" || true)"
echo "" >>"$SUMMARY"
echo "**Pass: $pass_count / $TOTAL — Fail/Timeout/LaunchFail: $fail_count**" >>"$SUMMARY"
echo "" >>"$MANUAL_AUDIT"
echo "## Human Audit Checklist" >>"$MANUAL_AUDIT"
echo "" >>"$MANUAL_AUDIT"
echo "- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff." >>"$MANUAL_AUDIT"
echo "- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles." >>"$MANUAL_AUDIT"
echo "- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative." >>"$MANUAL_AUDIT"
echo "- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion." >>"$MANUAL_AUDIT"
echo "[$(date +%H:%M:%S)] selected sweep complete — pass=$pass_count fail=$fail_count of $TOTAL" >&2
