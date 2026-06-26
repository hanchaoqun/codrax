#!/usr/bin/env bash
#
# Focused T11 convergence audit.
#
# Runs a small, high-signal eval batch and summarizes convergence metrics that
# indicate explorer/finalizer instability. This is an advisory report: it does
# not change product runtime behavior and must not be used as a hard gate over
# model-authored answers.
#
# Usage:
#   bash eval/convergence_audit.sh
#   CASES="eval/cases/u7k.case eval/cases/s5b.case" bash eval/convergence_audit.sh
#   PARALLEL=2 RUNS=1 TIMEOUT=1200 bash eval/convergence_audit.sh

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT"

# shellcheck source=eval/runner_lib.sh
source "$SCRIPT_DIR/runner_lib.sh"

DEFAULT_CASES="
eval/cases/qf_architecture.case
eval/cases/qf_diagram_pipeline.case
eval/cases/qf_type_relation_loop_controller.case
eval/cases/s5b.case
eval/cases/harmony/arkts_repomap.case
eval/cases/harmony/cangjie_repomap.case
eval/cases/u7k.case
eval/cases/read_combo_git_two_diffs_current_code.case
eval/cases/read_combo_log_current_source_explanation.case
eval/cases/read_combo_trace_current_source_explanation.case
eval/cases/read_combo_command_current_source_explanation.case
"

CASES_TEXT="${CASES:-$DEFAULT_CASES}"
# shellcheck disable=SC2206
CASES_LIST=($CASES_TEXT)
PARALLEL="${PARALLEL:-2}"
RUNS="${RUNS:-1}"
TIMEOUT="${TIMEOUT:-1200}"
RESULTS_ROOT="${EVAL_RESULTS_ROOT:-eval/results}"
SWEEP_START="$(date +%Y%m%d-%H%M%S)"
SUMMARY="${SUMMARY:-eval/convergence_audit_summary.md}"

metric_field() {
  eval_metric_field "$1" "$2"
}

metric_number() {
  local v
  v="$(metric_field "$1" "$2")"
  case "$v" in
    ''|'-') echo 0 ;;
    *) echo "$v" ;;
  esac
}

case_id_for_file() {
  local file="$1"
  (
    # shellcheck source=/dev/null
    source "$file"
    printf '%s\n' "${ID:-$(basename "$file" .case)}"
  )
}

run_case() {
  local case_file="$1"
  local case_id start_ts end_ts elapsed rc dir
  case_id="$(case_id_for_file "$case_file")"
  start_ts="$(date +%s)"
  eval_run_with_timeout "$TIMEOUT" env \
    EVAL_RESULTS_ROOT="$RESULTS_ROOT" \
    CODRAX_BIN="$CODRAX_BIN" \
    CODRAX_PROVIDER_ARGS_RAW="${CODRAX_PROVIDER_ARGS_RAW:-}" \
    eval/run.sh "$case_file" "$RUNS"
  rc=$?
  end_ts="$(date +%s)"
  elapsed=$((end_ts - start_ts))
  dir="$(eval_latest_result_dir "$RESULTS_ROOT" "$case_id" "$SWEEP_START" 2>/dev/null || true)"
  if [[ -n "$dir" && ( "$rc" -ne 0 || ! -f "$dir/run-1.metrics.txt" || ! -f "$dir/run-1.verdict" ) ]]; then
    eval_materialize_partial_run_result "$dir" 1 "$rc" "$elapsed" "eval_worker_incomplete"
  fi
  return "$rc"
}

if [[ ! -x "${CODRAX_BIN:-}" ]]; then
  if [[ ! -x ./codrax ]]; then
    echo "building codrax..." >&2
    make build >/dev/null || { echo "build failed" >&2; exit 1; }
  fi
  SWEEP_BIN="$ROOT/.codrax-convergence-${SWEEP_START}"
  cp ./codrax "$SWEEP_BIN" || exit 1
  chmod +x "$SWEEP_BIN" 2>/dev/null || true
  export CODRAX_BIN="$SWEEP_BIN"
  trap 'rm -f "$SWEEP_BIN"' EXIT
fi

echo "[$(date +%H:%M:%S)] convergence audit start: cases=${#CASES_LIST[@]} parallel=$PARALLEL runs=$RUNS" >&2
for case_file in "${CASES_LIST[@]}"; do
  if [[ ! -f "$case_file" ]]; then
    echo "case file not found: $case_file" >&2
    continue
  fi
  eval_wait_for_slot "$PARALLEL"
  (
    trap - EXIT
    echo "[$(date +%H:%M:%S)] running $case_file" >&2
    run_case "$case_file" >/dev/null
  ) &
done
wait

{
  echo "# T11 Explorer Convergence Audit"
  echo
  echo "- date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "- sweep_start_ts: $SWEEP_START"
  echo "- cases: ${#CASES_LIST[@]}"
  echo "- runs_per_case: $RUNS"
  echo "- parallel: $PARALLEL"
  echo "- results_root: $RESULTS_ROOT"
  echo
  echo "This report is advisory. It helps decide whether a typed runtime fix is justified; it must not be interpreted as permission to override a model answer that is fully supported by evidence."
  echo
  echo "| case | verdict | data_status | data_r | data_rep | data_ans | read | repo_map | list_files | source_lens | ana_it | exp_it | exp_disp | midloop | sibling_skip | origin_block | fin_it | fin_reject | fin_rewrite | sem | flags |"
  echo "|------|---------|-------------|-------:|---------:|---------:|-----:|---------:|-----------:|------------:|-------:|-------:|---------:|--------:|-------------:|-------------:|-------:|-----------:|------------:|----:|-------|"

  total=0
  flagged=0
  for case_file in "${CASES_LIST[@]}"; do
    [[ -f "$case_file" ]] || continue
    case_id="$(case_id_for_file "$case_file")"
    dir="$(eval_latest_result_dir "$RESULTS_ROOT" "$case_id" "$SWEEP_START" || true)"
    if [[ -z "$dir" ]]; then
      printf '| %s | MISSING | - | - | - | - | - | - | - | - | - | - | no_result_dir |\n' "$case_id"
      flagged=$((flagged + 1))
      continue
    fi
    metrics="$dir/run-1.metrics.txt"
    verdict_file="$dir/run-1.verdict"
    verdict="$(head -1 "$verdict_file" 2>/dev/null | awk '{print $1}')"
    verdict="${verdict:-UNKNOWN}"
    data_status="$(metric_field "$metrics" data_terminal_status)"
    data_status="${data_status:-—}"
    [[ "$data_status" == "-" ]] && data_status="—"
    data_rounds="$(metric_number "$metrics" data_rounds)"
    data_repairs="$(metric_number "$metrics" data_repair_rounds)"
    data_answer_len="$(metric_number "$metrics" data_answer_len)"
    read_calls="$(metric_number "$metrics" tool_read_file)"
    repo_map_calls="$(metric_number "$metrics" tool_repo_map)"
    list_files_calls="$(metric_number "$metrics" tool_list_files)"
    source_lens="$(metric_number "$metrics" source_inventory_lens)"
    ana="$(metric_number "$metrics" analyzer_iters)"
    exp="$(metric_number "$metrics" explorer_iters)"
    exp_disp="$(metric_number "$metrics" explorer_dispatches)"
    midloop="$(metric_number "$metrics" midloop_inject)"
    sibling="$(metric_number "$metrics" parallel_sibling_skips)"
    origin_block="$(metric_number "$metrics" mixed_origin_autocomplete_blocks)"
    fin="$(metric_number "$metrics" finalizer_iters)"
    fin_reject="$(metric_number "$metrics" finalizer_rejects)"
    fin_rewrite="$(metric_number "$metrics" finalizer_rewrites)"
    sem="$(metric_number "$metrics" semantic_quality_concerns)"
    mermaid_repair="$(metric_number "$metrics" mermaid_source_repair_applied)"
    answer_contract="$(metric_number "$metrics" answer_contract_violations)"
    answer_contract_strict_raw="$(metric_field "$metrics" answer_contract_strict_violations)"
    if [[ -n "$answer_contract_strict_raw" && "$answer_contract_strict_raw" != "-" ]]; then
      answer_contract_strict="$(metric_number "$metrics" answer_contract_strict_violations)"
    else
      answer_contract_strict="$answer_contract"
    fi
    history_prunes="$(metric_number "$metrics" tool_history_prunes)"
    flags=""
    if [[ "$verdict" != "PASS" ]]; then
      flags="${flags} verdict"
    fi
    if [[ "$fin" -gt 1 || "$fin_reject" -gt 0 || "$fin_rewrite" -gt 0 ]]; then
      flags="${flags} finalizer"
    fi
    if [[ "$exp_disp" -gt 1 || "$exp" -gt 25 ]]; then
      flags="${flags} explorer_long"
    fi
    if [[ "$read_calls" -gt 30 || "$repo_map_calls" -gt 8 || "$list_files_calls" -gt 12 ]]; then
      flags="${flags} wide_search"
    fi
    if [[ "$sem" -gt 0 ]]; then
      flags="${flags} semantic"
    fi
    if [[ "$answer_contract_strict" -gt 0 ]]; then
      flags="${flags} contract_warning"
    fi
    if [[ "$mermaid_repair" -gt 0 ]]; then
      flags="${flags} auto_repair"
    fi
    if [[ "$history_prunes" -gt 0 ]]; then
      flags="${flags} context_prune"
    fi
    if [[ "$origin_block" -gt 0 ]]; then
      flags="${flags} lane_wait"
    fi
    flags="${flags# }"
    flags="${flags:-—}"
    if [[ "$flags" != "—" ]]; then
      flagged=$((flagged + 1))
    fi
    total=$((total + 1))
    printf '| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n' \
      "$case_id" "$verdict" "$data_status" "$data_rounds" "$data_repairs" "$data_answer_len" "$read_calls" "$repo_map_calls" "$list_files_calls" "$source_lens" \
      "$ana" "$exp" "$exp_disp" "$midloop" "$sibling" "$origin_block" \
      "$fin" "$fin_reject" "$fin_rewrite" "$sem" "$flags"
  done
  echo
  echo "**flagged: $flagged / $total**"
  echo
  echo "Flag meanings: \`finalizer\` = finalizer took multiple turns or had document/patch rejects/rewrite renders; \`explorer_long\` = multiple explorer dispatches or very high explorer iterations; \`wide_search\` = high read_file/repo_map/list_files cost; \`lane_wait\` = typed mixed-origin closure correctly waited for missing lanes; \`semantic\` = semantic reviewer emitted concerns; \`contract_warning\` = answer contract check logged strict contract violations; soft advisory violations remain in metrics for audit but do not set this flag; \`auto_repair\` = renderer/compat auto-repair was applied; \`context_prune\` = tool history pruning occurred."
} >"$SUMMARY"

echo "summary written: $SUMMARY" >&2
cat "$SUMMARY"
