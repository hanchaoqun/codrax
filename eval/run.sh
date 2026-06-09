#!/usr/bin/env bash
#
# Multi-sample evaluation runner.
#
# Usage:
#   eval/run.sh <case-file> [N]
#   eval/run.sh eval/cases/df1.case 5
#
# Runs the case N times (default 3), captures stdout/log per run, checks
# EXPECT_CONTAINS / EXPECT_NOT_CONTAINS substrings, and optionally
# EXPECT_MATCHES_REGEX (ERE, ALL must match — useful for numeric
# scalar answers like "at least 4 digits somewhere in the answer"),
# EXPECT_SECTIONS (space-sep tokens, ALL must appear as literal
# substrings — useful for comparison questions that require both
# sides of "A vs B" to be mentioned), and EXPECT_LOG_MATCHES_REGEX /
# EXPECT_LOG_NOT_MATCHES_REGEX (newline-separated ERE patterns over
# the control-plane log, useful for hidden subsystem-execution guards).
# Extracts mechanism trace metrics from each run's debug log, and
# prints a markdown summary.
#
# Output layout:
#   eval/results/<case-id>-<timestamp>/
#     run-1.out          # codrax stdout
#     run-1.metrics.txt  # mechanism trace counters
#     run-1.verdict      # PASS/FAIL + reason
#     summary.md         # aggregated table

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=eval/runner_lib.sh
source "$SCRIPT_DIR/runner_lib.sh"

# 2026-05-10 — honor sweep-private binary snapshot to avoid
# concurrent-rebuild races. parallel_all.sh sets CODRAX_BIN to a
# stable copy; standalone usage falls back to ./codrax.
CODRAX_BIN_FROM_ENV="${CODRAX_BIN:-}"
CODRAX_BIN="${CODRAX_BIN_FROM_ENV:-./codrax}"

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <case-file> [N]" >&2
  exit 2
fi

CASE_FILE="$1"
N="${2:-3}"

if [[ ! -f "$CASE_FILE" ]]; then
  echo "case file not found: $CASE_FILE" >&2
  exit 2
fi

# shellcheck source=/dev/null
source "$CASE_FILE"

: "${ID:?case must define ID}"
: "${QUESTION:?case must define QUESTION}"
EXPECT_CONTAINS="${EXPECT_CONTAINS:-}"
EXPECT_NOT_CONTAINS="${EXPECT_NOT_CONTAINS:-}"
EXPECT_MATCHES_REGEX="${EXPECT_MATCHES_REGEX:-}"
EXPECT_SECTIONS="${EXPECT_SECTIONS:-}"
# Log-triage eval cases may set LOG=<inline panic> to attach a runtime log
# excerpt via --log-text. Perf-trace eval cases should set HTRACE=<inline
# trace> so the binary exercises the dedicated --htrace-text / perf_triage
# channel instead of the generic log_triager.
LOG="${LOG:-}"
HTRACE="${HTRACE:-}"
if [[ -n "$LOG" && -n "$HTRACE" ]]; then
  echo "case must not set both LOG and HTRACE" >&2
  exit 2
fi
# Write-mode eval vars (session 35). MODE=plan|apply switches the
# runner from the default read-mode dispatch to the write pipeline;
# FIXTURE points at a source tree under eval/fixtures/ that gets
# cloned into a scratch git repo per-run. PLAN_EXPECT_REGEX runs
# against the emitted ChangePlan JSON (always checked when MODE is
# non-empty). POST_APPLY_FILE scopes the EXPECT_* verdict checks to
# a single file's post-apply content for MODE=apply; when unset, the
# verdict reads the concatenation of all tracked fixture files.
MODE="${MODE:-}"
FIXTURE="${FIXTURE:-}"
PLAN_EXPECT_REGEX="${PLAN_EXPECT_REGEX:-}"
POST_APPLY_FILE="${POST_APPLY_FILE:-}"
# Multi-repo eval (2026-05-08): when MULTIREPO=<seed-name> is set, the
# runner copies eval/fixtures/<seed-name>/ to a scratch parent dir and
# `git init`-s each immediate child sub-repo (no .git/ checked into the
# codrax repo itself). codrax is then dispatched with --repo <scratch>
# so the topology layer (LoadOrDiscover BFS depth=4) auto-detects the
# sub-repos. Read-mode only — write-mode + multi-repo is a future
# combination once write fan-out lands.
MULTIREPO="${MULTIREPO:-}"
# FOCUS is the comma-separated --focus value forwarded to the binary
# when MULTIREPO is set. Each token is a slug-or-RootRel resolved by
# topology.Resolve. Empty (default) = no pin, the routing fold's A
# channel is empty. Used by mr_pin_isolation / future cases that test
# operator pin behaviour without REPL interaction.
FOCUS="${FOCUS:-}"
# CAP overrides multi_repo_max_active for the eval-specific yaml
# (multirepo_settings.yaml). Empty (default) keeps the file's own
# value; setting CAP="2" makes one of the 3 multirepo-basic sub-repos
# fall outside the active set so mr_inactive_path can test the L1
# refusal + L0 advisory recovery path.
CAP="${CAP:-}"
# Optional read-mode settings override for one eval case. This is intentionally
# single-repo/read-only so normal read cases keep the operator's environment
# unchanged and write/multirepo fixtures continue to use their dedicated yaml.
SETTINGS="${SETTINGS:-}"
DATA_FIXTURE="${DATA_FIXTURE:-}"
# Optional sanity floor for cases whose correct final answer is intentionally
# very short, such as strict scalar/data-only outputs. Keep the default floor
# for ordinary explanatory answers; individual cases may lower it when their
# output contract explicitly requires a tiny payload.
MIN_OUTPUT_CHARS="${MIN_OUTPUT_CHARS:-20}"

case "$MODE" in
  "" | read | plan | apply) ;;
  *)
    echo "case MODE=$MODE invalid (allowed: plan, apply, or empty for read)" >&2
    exit 2
    ;;
esac
if [[ -n "$MODE" && "$MODE" != "read" && -z "$FIXTURE" ]]; then
  echo "case MODE=$MODE requires FIXTURE=<path under eval/fixtures>" >&2
  exit 2
fi
if [[ -n "$FIXTURE" && ! -d "$FIXTURE" ]]; then
  # Resolve after we cd into $ROOT below; defer the dir-exists check.
  :
fi
# MULTIREPO and MODE/FIXTURE are mutually exclusive — write-mode +
# multi-repo is a future combination, not a current eval scenario.
if [[ -n "$MULTIREPO" && -n "$MODE" && "$MODE" != "read" ]]; then
  echo "case MULTIREPO=$MULTIREPO is incompatible with MODE=$MODE (multi-repo write-mode not yet supported)" >&2
  exit 2
fi
if [[ -n "$MULTIREPO" && -n "$FIXTURE" ]]; then
  echo "case MULTIREPO=$MULTIREPO and FIXTURE=$FIXTURE are mutually exclusive" >&2
  exit 2
fi
if [[ -n "$SETTINGS" && -n "$MODE" && "$MODE" != "read" ]]; then
  echo "case SETTINGS=$SETTINGS is read-mode only and is incompatible with MODE=$MODE" >&2
  exit 2
fi
if [[ -n "$SETTINGS" && -n "$MULTIREPO" ]]; then
  echo "case SETTINGS=$SETTINGS is incompatible with MULTIREPO=$MULTIREPO; use the multirepo settings fixture instead" >&2
  exit 2
fi
if [[ -n "$DATA_FIXTURE" && ( -n "$MULTIREPO" || ( -n "$MODE" && "$MODE" != "read" ) ) ]]; then
  echo "case DATA_FIXTURE=$DATA_FIXTURE is read-mode only and is incompatible with MULTIREPO or write MODE" >&2
  exit 2
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
CODRAX_PROVIDER_ARGS=()
if [[ -f "$ROOT/providers.yaml" ]]; then
  CODRAX_PROVIDER_ARGS=(--providers "$ROOT/providers.yaml")
fi
if [[ -n "$SETTINGS" && ! -f "$SETTINGS" ]]; then
  echo "case SETTINGS file not found: $SETTINGS" >&2
  exit 2
fi

# Standalone eval runs rebuild so metrics never silently use a stale
# binary. Parallel sweeps pass CODRAX_BIN as a private snapshot; in
# that mode rebuilding per case defeats the snapshot and dirties the
# working tree under concurrent runs.
if [[ -n "$CODRAX_BIN_FROM_ENV" && ! -x "$CODRAX_BIN" ]]; then
  echo "codrax snapshot missing or not executable: $CODRAX_BIN; falling back to ./codrax" >&2
  CODRAX_BIN_FROM_ENV=""
  CODRAX_BIN="./codrax"
fi
if [[ -n "$CODRAX_BIN_FROM_ENV" && -x "$CODRAX_BIN" ]]; then
  echo "using codrax snapshot: $CODRAX_BIN" >&2
else
  echo "building codrax..." >&2
  make build >/dev/null || { echo "build failed" >&2; exit 1; }
  CODRAX_BIN="./codrax"
fi

TS="$(date +%Y%m%d-%H%M%S)"
RESULTS_ROOT="${EVAL_RESULTS_ROOT:-eval/results}"
OUTDIR="${RESULTS_ROOT}/${ID}-${TS}"
mkdir -p "$OUTDIR"

echo "case: $ID  ($NAME)" >&2
echo "question: $QUESTION" >&2
echo "runs: $N" >&2
echo "outdir: $OUTDIR" >&2
echo >&2

count_pattern() {
  # Robust grep -c that returns "0" cleanly on no-match (grep exits 1).
  local pat="$1" file="$2" n
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  # Treat metric patterns as ERE. Several existing counters use alternation
  # and character classes; ERE keeps the run summary aligned with the
  # parallel-sweep helper in eval/runner_lib.sh.
  n=$(LC_ALL=C grep -aE -c "$pat" "$file" 2>/dev/null) || n=0
  echo "${n:-0}"
}

# scope_stdout <out-file> → prints ANSI-stripped, post-'━━━'-only body.
# Read-mode only: cmd/root.go prints the separator on stderr between
# thinking trace and final answer. Write-mode outputs (plan.json,
# post-apply files) are raw bytes and MUST NOT be scoped this way.
scope_stdout() {
  local out="$1" cleaned
  # The progress stream can contain multilingual text plus UI-truncated
  # previews. A preview may cut through a UTF-8 code point before adding
  # an ellipsis, which makes locale-aware sed/grep abort with
  # "illegal byte sequence" on macOS. Eval matching is byte-substring
  # matching, so force byte mode end-to-end here.
  cleaned="$(LC_ALL=C sed -r 's/\x1B\[[0-9;]*[A-Za-z]//g' "$out")"
  if LC_ALL=C grep -aqF '━━━' <<<"$cleaned"; then
    cleaned="$(LC_ALL=C awk 'found{print; next} /━━━/{found=1}' <<<"$cleaned")"
  fi
  printf '%s' "$cleaned"
}

json_string_field() {
  local file="$1" field="$2"
  [[ -f "$file" ]] || return 1
  LC_ALL=C sed -nE 's/^[[:space:]]*"'"$field"'"[[:space:]]*:[[:space:]]*"([^"]*)".*/\1/p' "$file" | head -1
}

json_number_field() {
  local file="$1" field="$2"
  [[ -f "$file" ]] || return 1
  LC_ALL=C sed -nE 's/^[[:space:]]*"'"$field"'"[[:space:]]*:[[:space:]]*([0-9]+).*/\1/p' "$file" | head -1
}

data_terminal_action_failed_count() {
  local file="$1"
  [[ -f "$file" ]] || { echo 0; return; }
  LC_ALL=C awk '
    /"action_events"[[:space:]]*:[[:space:]]*\[/ { in_actions=1; next }
    in_actions && /"action_graph"[[:space:]]*:/ { in_actions=0 }
    in_actions && /"status"[[:space:]]*:[[:space:]]*"failed"/ { count++ }
    END { print count + 0 }
  ' "$file"
}

data_terminal_answer_len() {
  local summary="$1"
  LC_ALL=C sed -nE 's/.*answer_len=([0-9]+).*/\1/p' <<<"$summary" | head -1
}

latest_data_terminal_path() {
  local log="$1" path=""
  [[ -n "$log" && -f "$log" ]] || return 1
  path="$(LC_ALL=C sed -nE 's/.*\[cli\/data\] terminal full path=([^[:space:]]+).*/\1/p' "$log" | tail -1)"
  [[ -n "$path" ]] || return 1
  if [[ "$path" = /* ]]; then
    printf '%s' "$path"
  else
    printf '%s/%s' "$ROOT" "$path"
  fi
}

# setup_scratch <scratch-dir> — copies $ROOT/$FIXTURE into scratch and
# initialises it as a git repo with a single seed commit (write mode
# requires a git worktree). Wipes any prior contents. Uses a detached
# committer identity so no global git config is needed on CI hosts.
setup_scratch() {
  local scratch="$1"
  rm -rf "$scratch"
  mkdir -p "$scratch"
  cp -r "$ROOT/$FIXTURE/." "$scratch/"
  (
    cd "$scratch"
    git init -q -b main
    git -c user.email=eval@codrax -c user.name=eval add -A
    git -c user.email=eval@codrax -c user.name=eval commit -q -m "seed"
  ) || return $?
}

# setup_multirepo_scratch <scratch-parent-dir> — copies the seed under
# eval/fixtures/$MULTIREPO/ to <scratch-parent>, then git-init's every
# immediate subdirectory as its own sub-repo (the parent itself stays
# a plain dir, no .git/ — that is exactly what the topology BFS layer
# expects). README.md / shared assets at the parent level are NOT
# committed (they would not belong to any sub-repo). Each sub-repo
# gets a single seed commit so cgit-friendly tooling treats it as a
# real repo.
setup_multirepo_scratch() {
  local scratch="$1"
  rm -rf "$scratch"
  mkdir -p "$scratch"
  cp -r "$ROOT/eval/fixtures/$MULTIREPO/." "$scratch/"
  # Init each immediate subdir of $scratch as its own repo.
  local sub
  for sub in "$scratch"/*/; do
    [[ -d "$sub" ]] || continue
    (
      cd "$sub"
      git init -q -b main
      git -c user.email=eval@codrax -c user.name=eval add -A
      git -c user.email=eval@codrax -c user.name=eval commit -q -m "seed" 2>/dev/null || true
    ) || return $?
  done
}

# setup_data_scratch <scratch-dir> — copies eval/fixtures/<seed-name>/ to a
# scratch read-mode repo root for data-lane evals. It intentionally does not
# create source files or git history; the data lane should not require either.
setup_data_scratch() {
  local scratch="$1"
  rm -rf "$scratch"
  mkdir -p "$scratch"
  cp -r "$ROOT/eval/fixtures/$DATA_FIXTURE/." "$scratch/"
}

# run_read_step / run_plan_step / run_apply_step — the three pipeline
# invocations. All accept (run-i, out-file, log-dir) as positional args
# plus any mode-specific extras. Each appends to $out (read mode uses
# '>' to truncate on first call; write-mode apply appends after plan).
run_read_step() {
  local i="$1" out="$2" logdir="$3"
  # Multi-repo eval (2026-05-08): when $4 is set, --repo points at the
  # multi-repo parent scratch (where each immediate child is its own
  # git repo), exercising the topology BFS + MultiGraph carrier.
  # Empty $4 falls back to the historical --repo . (codrax self-repo)
  # so all existing single-repo cases stay byte-identical.
  local repo_arg="."
  if [[ -n "${4:-}" ]]; then
    repo_arg="${4}"
  fi
  # FOCUS env (multi-repo only): forwarded as --focus per token.
  # Single-repo / no-MULTIREPO cases ignore FOCUS — the binary's
  # topology has no sub-repo to match the value, so the flag is a
  # no-op. Building the args dynamically keeps single-repo runs
  # byte-identical (no extra flag passed).
  local attach_args=()
  if [[ -n "$LOG" ]]; then
    attach_args=(--log-text "$LOG")
  elif [[ -n "$HTRACE" ]]; then
    attach_args=(--htrace-text "$HTRACE")
  fi
  if [[ ${#attach_args[@]} -gt 0 ]]; then
    if [[ -n "$FOCUS" ]]; then
      "$CODRAX_BIN" "${CODRAX_PROVIDER_ARGS[@]}" --repo "$repo_arg" --branch main --pipeline-max-steps 15 \
        --log-level debug \
        --log-dir "$logdir" \
        "${attach_args[@]}" \
        --focus "$FOCUS" \
        --request "$QUESTION" \
        >"$out" 2>&1
    else
      "$CODRAX_BIN" "${CODRAX_PROVIDER_ARGS[@]}" --repo "$repo_arg" --branch main --pipeline-max-steps 15 \
        --log-level debug \
        --log-dir "$logdir" \
        "${attach_args[@]}" \
        --request "$QUESTION" \
        >"$out" 2>&1
    fi
  else
    if [[ -n "$FOCUS" ]]; then
      "$CODRAX_BIN" "${CODRAX_PROVIDER_ARGS[@]}" --repo "$repo_arg" --branch main --pipeline-max-steps 15 \
        --log-level debug \
        --log-dir "$logdir" \
        --focus "$FOCUS" \
        --request "$QUESTION" \
        >"$out" 2>&1
    else
      "$CODRAX_BIN" "${CODRAX_PROVIDER_ARGS[@]}" --repo "$repo_arg" --branch main --pipeline-max-steps 15 \
        --log-level debug \
        --log-dir "$logdir" \
        --request "$QUESTION" \
        >"$out" 2>&1
    fi
  fi
}

run_plan_step() {
  local i="$1" out="$2" logdir="$3" scratch="$4" plan="$5"
  echo "=== plan step (run $i) ===" >"$out"
  "$CODRAX_BIN" "${CODRAX_PROVIDER_ARGS[@]}" --repo "$scratch" --branch main --pipeline-max-steps 15 \
    --mode=plan --plan-out "$plan" \
    --log-level debug \
    --log-dir "$logdir" \
    --request "$QUESTION" \
    >>"$out" 2>&1
}

run_apply_step() {
  local i="$1" out="$2" logdir="$3" scratch="$4" plan="$5"
  echo "" >>"$out"
  echo "=== apply step (run $i) ===" >>"$out"
  "$CODRAX_BIN" "${CODRAX_PROVIDER_ARGS[@]}" --repo "$scratch" --branch main --pipeline-max-steps 15 \
    --mode=apply --plan-file "$plan" --auto-apply \
    --log-level debug \
    --log-dir "$logdir" \
    --request "$QUESTION" \
    >>"$out" 2>&1
}

# write_metrics <run-i> <exit-code> <log-file> — writes the mechanism
# trace counter file. Read-mode focused; write-mode cases see zeroes
# for analyzer/explorer counters (expected — write stages don't emit
# those log lines).
write_metrics() {
  local i="$1" rc="$2" log="$3"
  local metrics="$OUTDIR/run-$i.metrics.txt"
  local data_terminal_path="" data_terminal_status="" data_rounds="0" data_repair_rounds="0" data_record_count="0" data_result_summary="" data_answer_len="0" data_action_failed="0"
  data_terminal_path="$(latest_data_terminal_path "$log" 2>/dev/null || true)"
  if [[ -n "$data_terminal_path" && -f "$data_terminal_path" ]]; then
    data_terminal_status="$(json_string_field "$data_terminal_path" "status" || true)"
    data_rounds="$(json_number_field "$data_terminal_path" "data_rounds" || true)"
    data_repair_rounds="$(json_number_field "$data_terminal_path" "repair_rounds" || true)"
    data_record_count="$(json_number_field "$data_terminal_path" "record_count" || true)"
    data_result_summary="$(json_string_field "$data_terminal_path" "result_summary" || true)"
    data_answer_len="$(data_terminal_answer_len "$data_result_summary" || true)"
    data_action_failed="$(data_terminal_action_failed_count "$data_terminal_path" || true)"
  fi
  data_rounds="${data_rounds:-0}"
  data_repair_rounds="${data_repair_rounds:-0}"
  data_record_count="${data_record_count:-0}"
  data_answer_len="${data_answer_len:-0}"
  data_action_failed="${data_action_failed:-0}"
  {
    echo "exit_code=$rc"
    echo "log_file=$log"
    echo "data_terminal_path=$data_terminal_path"
    echo "data_terminal_status=$data_terminal_status"
    echo "data_rounds=$data_rounds"
    echo "data_repair_rounds=$data_repair_rounds"
    echo "data_record_count=$data_record_count"
    echo "data_action_failed=$data_action_failed"
    echo "data_answer_len=$data_answer_len"
    echo "data_result_summary=$data_result_summary"
    echo "tool_read_file=$(eval_count_tool_calls "$log" read_file)"
    echo "tool_repo_map=$(eval_count_tool_calls "$log" repo_map)"
    echo "tool_list_files=$(eval_count_tool_calls "$log" list_files)"
    echo "tool_trace_query=$(eval_count_tool_calls "$log" trace_query)"
    echo "tool_mcp_read_resource=$(eval_count_tool_calls "$log" mcp_read_resource)"
    echo "repeated_mcp_resource_reads=$(eval_count_repeated_mcp_resource_reads "$log")"
    echo "mcp_tool_calls=$(eval_count_control_pattern 'DEBUG \[diag [^]]+\][^:]*phase=toolcall [^:]*tool=[A-Za-z0-9_-]+__[A-Za-z0-9_-]+' "$log")"
    echo "source_inventory_lens=$(eval_count_source_inventory_tool_calls "$log")"
    echo "repo_lens_discovery_hints=$(eval_count_repo_lens_discovery_hints "$log")"
    echo "transient_retry_checkpoints=$(eval_count_transient_retry_checkpoints "$log")"
    echo "unavailable_tool_attempts=$(eval_count_unavailable_tool_attempts "$log")"
    echo "checkpoint_continuation_broad_hint=$(eval_count_checkpoint_continuation_broad_hint "$log")"
    echo "closure_only_repeated=$(eval_count_closure_only_repeated "$log")"
    echo "mermaid_source_repair_applied=$(eval_count_mermaid_source_repairs "$log")"
    echo "repair_debt_checkpoints=$(eval_count_repair_debt_checkpoints "$log")"
    echo "repair_debt_close_ready_filters=$(eval_count_repair_debt_close_ready_filters "$log")"
    echo "repair_debt_principal_blocking_max=$(eval_max_repair_debt_checkpoint_class "$log" principal_blocking)"
    echo "repair_debt_surgical_grounding_max=$(eval_max_repair_debt_checkpoint_class "$log" surgical_grounding)"
    echo "repair_debt_advisory_max=$(eval_max_repair_debt_checkpoint_class "$log" advisory)"
    echo "tool_history_prunes=$(eval_count_control_pattern 'DEBUG \[diag [^]]+\][^:]*phase=prune TOOL HISTORY PRUNED' "$log")"
    echo "max_context_tokens_est=$(eval_max_context_tokens_estimate "$log")"
    echo "max_context_window=$(eval_max_context_window_tokens "$log")"
    echo "max_context_window_pct=$(eval_max_context_window_pct "$log")"
    echo "answer_contract_violations=$(eval_sum_answer_contract_violations "$log")"
    echo "answer_contract_lane_block_kind_violations=$(eval_sum_answer_contract_violations_for_section "$log" lane_block_kind)"
    echo "concrete_values=$(count_pattern 'concrete values' "$log")"
    echo "synthesis_runs=$(count_pattern 'SYNTHESIS prompt' "$log")"
    echo "function_boundary_push=$(count_pattern 'CRITICAL.*Incomplete' "$log")"
    echo "enumeration_push=$(count_pattern 'explorer\\.mid-loop\\.enumeration|Principal Enumeration Rows|系统按已验证证据补充成员|系统按已验证证据补充缺失成员' "$log")"
    echo "focus_warning=$(count_pattern 'Potential Focus' "$log")"
    echo "t11_gate_skip=$(count_pattern 'T1.1 gate.*skipping' "$log")"
    echo "t11_gate_run=$(count_pattern 'T1.1 gate.*running' "$log")"
    echo "dataflow_intent_lookup=$(count_pattern 'dataflowIntent=lookup' "$log")"
    echo "dataflow_intent_propagate=$(count_pattern 'dataflowIntent=propagate' "$log")"
    echo "midloop_inject=$(eval_count_midloop_injects "$log")"
    echo "parallel_sibling_skips=$(count_pattern 'skipping non-winning parallel explore sibling' "$log")"
    echo "mixed_origin_autocomplete_blocks=$(count_pattern 'accepted investigation closure cannot auto-complete mixed-origin explore window' "$log")"
    echo "finalizer_rejects=$(eval_count_finalizer_rejects "$log")"
    echo "finalizer_rewrites=$(eval_count_finalizer_rewrites "$log")"
    echo "answer_chain_lines=$(count_pattern 'answer_chain' "$log")"
    # B6-F5 (post-shape consolidated audit, 2026-05-04): per-agent
    # LLM-turn counters. Each ReAct iteration logs exactly one
    # "[diag <agent>] iter=N ASSISTANT content_len=…" line, so
    # counting that suffix gives the actual model-turn count
    # uncontaminated by INIT msg / TOOLRESULT / MIDLOOP siblings.
    echo "analyzer_iters=$(eval_count_agent_iterations "$log" analyzer)"
    echo "explorer_iters=$(eval_count_agent_iterations "$log" explorer)"
    echo "extractor_iters=$(eval_count_agent_iterations "$log" extractor)"
    echo "finalizer_iters=$(eval_count_agent_iterations "$log" finalizer)"
    # R12 (post-shape 残留 audit, 2026-05-04): per-agent dispatch
    # counters distinct from per-iteration "ASSISTANT content_len="
    # lines. Operators previously misread `explorer_iters=40` as a
    # single long ReAct loop when it was 4 dispatches × ~10 iters
    # each. The orchestrator emits exactly one `[diag <agent>]
    # DISPATCH stage=… attempt=…` line per dispatch, so iter median
    # per dispatch = explorer_iters / explorer_dispatches.
    echo "analyzer_dispatches=$(eval_count_agent_dispatches "$log" analyzer)"
    echo "explorer_dispatches=$(eval_count_agent_dispatches "$log" explorer)"
    echo "extractor_dispatches=$(eval_count_agent_dispatches "$log" extractor)"
    echo "finalizer_dispatches=$(eval_count_agent_dispatches "$log" finalizer)"
    # G1 (post_v2_runtime_gap_remediation, 2026-05-04) repair-plan +
    # repair-exec telemetry. The orchestrator's retry-decision site
    # emits one repair_plan= line + one repair_exec= line per failed
    # validator pass.
    #
    #   repair_plan_lines      — total repair-plan trace lines
    #   repair_exec_lines      — total repair-exec trace lines (≤ repair_plan_lines)
    #   repair_exec_promote    — repair-exec lines whose remaining > 0
    #                            AND budget_downgrade=true (queue-state
    #                            advance signal; rebuild branch always
    #                            shows budget_downgrade=true on first
    #                            cluster, so this is a coarse upper bound)
    #   repair_exec_failloud   — repair-exec lines that surfaced fail_loud=true
    echo "repair_plan_lines=$(count_pattern 'repair_plan: ' "$log")"
    echo "repair_exec_lines=$(count_pattern 'repair_exec: ' "$log")"
    echo "repair_exec_promote=$(count_pattern 'repair_exec: .*remaining=[1-9].*budget_downgrade=true' "$log")"
    echo "repair_exec_failloud=$(count_pattern 'repair_exec: .*fail_loud=true' "$log")"
    # G5 (post_v2_runtime_gap_remediation, 2026-05-04) — semantic
    # quality reviewer dispatch + verdict telemetry.
    #   semantic_quality_dispatches  — reviewer dispatched this run
    #   semantic_quality_concerns    — reviewer-emitted concern count
    echo "semantic_quality_dispatches=$(eval_count_semantic_quality_dispatches "$log")"
    echo "semantic_quality_concerns=$(eval_count_semantic_quality_concerns "$log")"
    # G7 trigger-data observability (post_v2_runtime_gap_remediation,
    # 2026-05-04). strict_decode_remap fires when an LLM emit hits a
    # known misplaced-field pattern. Counts the per-Run frequency so
    # the G7 path-sensitive prescan ROI can be assessed from real
    # data — high counts here justify implementing G7, low counts
    # justify keeping it deferred.
    echo "strict_decode_remap_events=$(count_pattern 'strict_decode_remap.*misplaced field' "$log")"
  } >"$metrics"
}

# write_verdict <verdict-file> <cleaned-string> [extra-reasons...]
#
# Shared EXPECT_CONTAINS / EXPECT_NOT_CONTAINS / EXPECT_MATCHES_REGEX /
# EXPECT_SECTIONS matcher. cleaned is the bytes the verdict checks
# against — mode-specific upstream (scope_stdout output for read, plan
# JSON for plan, post-apply fixture bytes for apply). extra-reasons
# are pre-seeded failure tokens from mode-specific pre-checks (e.g.
# "no_plan_regex:...", "apply_exit:1", "no_log_regex:...");
# any non-empty value forces FAIL even if EXPECT_* all match.
write_verdict() {
  local verdict_file="$1" cleaned="$2"
  shift 2
  local extra_reasons=("$@")
  local pass=1
  local reasons=()
  if (( ${#extra_reasons[@]} > 0 )); then
    reasons=("${extra_reasons[@]}")
    pass=0
  fi

  # Global min-length sanity filter (2026-04-14 deferred #10): by default any
  # answer body shorter than 20 non-whitespace characters is a
  # fragment — "type" (Go keyword picked by bug #14), "3" (df1 count
  # hallucination), "• **type" (round-4 truncation), etc. The
  # threshold is low enough to allow a legitimate short single-symbol
  # answer like "explorer (foo.go:12)" (~25 chars) to pass. For
  # write-mode plan.json / post-apply source files the 20-char floor
  # is also safe: an empty plan JSON is 2 bytes ('{}') and a
  # post-apply file erased to zero bytes would obviously fail here.
  # Cases with a typed output contract that intentionally returns a
  # short scalar can set MIN_OUTPUT_CHARS=1 instead of padding the
  # product answer just to satisfy the harness.
  local stripped
  stripped="$(LC_ALL=C tr -d '[:space:]' <<<"$cleaned")"
  local min_output_chars="$MIN_OUTPUT_CHARS"
  if ! [[ "$min_output_chars" =~ ^[0-9]+$ ]]; then
    min_output_chars=20
  fi
  if (( min_output_chars > 0 && ${#stripped} < min_output_chars )); then
    pass=0
    reasons+=("too_short:${#stripped}chars")
  fi

  # Case-insensitive substring matching across all three lists.
  # Case-sensitive matching (grep -F without -i) was historically
  # chosen but produces false negatives when case conventions
  # diverge between case-file authors (often lowercase tokens) and
  # LLM answers (proper-noun / class-name capitalisation). The
  # 2026-04-21 logtri_rust failure is the canonical trace:
  # EXPECT_CONTAINS='rust' vs. answer 'Rust 运行时' — 25 hits
  # case-insensitive, 0 case-sensitive. Using -iF uniformly treats
  # EXPECT_CONTAINS / EXPECT_NOT_CONTAINS / EXPECT_SECTIONS as
  # "this concept must appear" rather than "this exact byte
  # sequence must appear"; if a case ever legitimately needs the
  # latter, EXPECT_MATCHES_REGEX is the explicit case-sensitive
  # channel.
  if [[ -n "$EXPECT_CONTAINS" ]]; then
    for needle in $EXPECT_CONTAINS; do
      if ! LC_ALL=C grep -aqiF -- "$needle" <<<"$cleaned"; then
        pass=0
        reasons+=("missing:$needle")
      fi
    done
  fi
  if [[ -n "$EXPECT_NOT_CONTAINS" ]]; then
    for needle in $EXPECT_NOT_CONTAINS; do
      if LC_ALL=C grep -aqiF -- "$needle" <<<"$cleaned"; then
        pass=0
        reasons+=("banned:$needle")
      fi
    done
  fi

  # EXPECT_MATCHES_REGEX: newline-separated ERE patterns (IFS hack so
  # whitespace inside a pattern is preserved — regex alternation
  # typically has no spaces but user patterns might). ALL must match.
  if [[ -n "$EXPECT_MATCHES_REGEX" ]]; then
    local old_ifs="$IFS"
    IFS=$'\n'
    for rx in $EXPECT_MATCHES_REGEX; do
      [[ -z "$rx" ]] && continue
      if ! LC_ALL=C grep -aEq -- "$rx" <<<"$cleaned"; then
        pass=0
        reasons+=("no_regex_match:${rx}")
      fi
    done
    IFS="$old_ifs"
  fi

  # EXPECT_SECTIONS: space-separated literal tokens; ALL must appear.
  # Intended for comparison-shape questions where the answer must
  # symmetrically mention both sides (A AND B). Semantically identical
  # to EXPECT_CONTAINS but spelled separately so summary reasons
  # distinguish "missing symmetric section" from "missing load-bearing
  # identifier".
  if [[ -n "$EXPECT_SECTIONS" ]]; then
    for needle in $EXPECT_SECTIONS; do
      if ! LC_ALL=C grep -aqiF -- "$needle" <<<"$cleaned"; then
        pass=0
        reasons+=("missing_section:$needle")
      fi
    done
  fi

  if [[ $pass -eq 1 ]]; then
    echo "PASS" >"$verdict_file"
  else
    printf 'FAIL %s\n' "${reasons[*]}" >"$verdict_file"
  fi
}

run_one() {
  local i="$1"
  local out="$OUTDIR/run-$i.out"
  local verdict="$OUTDIR/run-$i.verdict"
  # Per-run log dir so we don't accidentally pick up an unrelated log.
  local logdir="$OUTDIR/run-$i.logs"
  mkdir -p "$logdir"

  local rc=0
  local scratch=""
  local plan=""

  case "$MODE" in
    plan|apply)
      scratch="$OUTDIR/run-$i.repo"
      plan="$OUTDIR/run-$i.plan.json"
      if ! setup_scratch "$scratch"; then
        echo "FAIL setup_fail" >"$verdict"
        echo "run $i: FAIL setup_fail" >&2
        return
      fi
      # write_enabled is yaml-only (no CLI flag). Export the fixture
      # yaml only for the write steps; unset on exit so a subsequent
      # read-mode run in the same process would not inherit the gate.
      export CODRAX_SETTINGS="$ROOT/eval/fixtures/write_enabled.yaml"
      run_plan_step "$i" "$out" "$logdir" "$scratch" "$plan"
      rc=$?
      if [[ "$MODE" == "apply" && $rc -eq 0 && -f "$plan" ]]; then
        run_apply_step "$i" "$out" "$logdir" "$scratch" "$plan"
        rc=$?
      fi
      unset CODRAX_SETTINGS
      ;;
    *)
      if [[ -n "$DATA_FIXTURE" ]]; then
        scratch="$OUTDIR/run-$i.data"
        if ! setup_data_scratch "$scratch"; then
          echo "FAIL data_setup_fail" >"$verdict"
          echo "run $i: FAIL data_setup_fail" >&2
          return
        fi
        run_read_step "$i" "$out" "$logdir" "$scratch"
        rc=$?
      elif [[ -n "$MULTIREPO" ]]; then
        scratch="$OUTDIR/run-$i.parent"
        if ! setup_multirepo_scratch "$scratch"; then
          echo "FAIL multirepo_setup_fail" >"$verdict"
          echo "run $i: FAIL multirepo_setup_fail" >&2
          return
        fi
        # The multirepo-basic fixture has three sub-repos and most
        # mr_* eval cases assume every one is active. Phase 0
        # (2026-05-08) lowered the default cap from 3 → 2 for
        # production; export the eval-specific override so the
        # routing fold keeps all three resident. CAP env overrides
        # the yaml at run time for cases (e.g. mr_inactive_path)
        # that need a smaller cap to make a sub-repo fall outside
        # the active set. Single-repo cases bypass this branch and
        # continue to read the operator's default config.
        local settings_yaml="$ROOT/eval/fixtures/multirepo_settings.yaml"
        if [[ -n "$CAP" ]]; then
          local capped_yaml="$OUTDIR/run-$i.settings.yaml"
          sed -E "s/^(multi_repo_max_active:).*/\1 ${CAP}/" "$settings_yaml" >"$capped_yaml"
          settings_yaml="$capped_yaml"
        fi
        export CODRAX_SETTINGS="$settings_yaml"
        run_read_step "$i" "$out" "$logdir" "$scratch"
        rc=$?
        unset CODRAX_SETTINGS
      else
        local had_settings_env=0
        local prior_settings_env=""
        if [[ ${CODRAX_SETTINGS+x} ]]; then
          had_settings_env=1
          prior_settings_env="$CODRAX_SETTINGS"
        fi
        if [[ -n "$SETTINGS" ]]; then
          export CODRAX_SETTINGS="$ROOT/$SETTINGS"
        fi
        run_read_step "$i" "$out" "$logdir"
        rc=$?
        if [[ -n "$SETTINGS" ]]; then
          if [[ $had_settings_env -eq 1 ]]; then
            export CODRAX_SETTINGS="$prior_settings_env"
          else
            unset CODRAX_SETTINGS
          fi
        fi
      fi
      ;;
  esac

  # Aggregate every log file in this run's dedicated logdir. Data-route
  # executions can create multiple codrax-*.log files for one logical run; using
  # only the latest log loses route/result control lines and creates false
  # no_log_regex failures. MODE=apply also benefits by keeping plan/apply
  # control logs visible to metrics.
  local log
  log="$(ls -t "$logdir"/codrax-*.log 2>/dev/null | head -1)"
  if [[ -n "$log" ]]; then
    local all_log="$OUTDIR/run-$i.logs.all.log"
    : >"$all_log"
    local lf
    for lf in $(ls -tr "$logdir"/codrax-*.log 2>/dev/null); do
      {
        echo
        echo "===== $lf ====="
        cat "$lf"
      } >>"$all_log"
    done
    log="$all_log"
  fi
  write_metrics "$i" "$rc" "$log"

  # Verdict source bytes selection by MODE.
  local cleaned=""
  local extra_reasons=()
  case "$MODE" in
    plan)
      if [[ -f "$plan" ]]; then
        cleaned="$(cat "$plan")"
      else
        extra_reasons+=("plan_not_written")
      fi
      ;;
    apply)
      # Apply happens inside a worktree (L5 red line: worktree is the
      # write sandbox; the scratch/ main repo HEAD bytes never change
      # automatically). Post-apply content therefore lives under the
      # worktree path, NOT the scratch repo root. ChangePlan records
      # the worktree path in WorktreePath once apply fires, provided
      # pipeline_keep_worktree_on_success: true in the yaml (which
      # eval/fixtures/write_enabled.yaml sets).
      #
      # Strategy:
      #   1. Read worktree_path from plan.json.
      #   2. If present + dir exists, that's the apply source of truth.
      #   3. Fall back to scratch if worktree missing (surfaces the
      #      keep-on-success misconfiguration as post_apply_file_missing,
      #      not a silent false-pass against pre-apply fixture bytes).
      apply_source="$scratch"
      if [[ -f "$plan" ]]; then
        wt=$(grep -oE '"worktree_path":[[:space:]]*"[^"]+"' "$plan" 2>/dev/null | sed -E 's/.*"([^"]+)"$/\1/')
        if [[ -n "$wt" && -d "$wt" ]]; then
          apply_source="$wt"
        else
          extra_reasons+=("worktree_discarded_or_missing")
        fi
      fi
      if [[ -n "$POST_APPLY_FILE" ]]; then
        if [[ -f "$apply_source/$POST_APPLY_FILE" ]]; then
          cleaned="$(cat "$apply_source/$POST_APPLY_FILE")"
        else
          extra_reasons+=("post_apply_file_missing:$POST_APPLY_FILE")
        fi
      else
        if [[ -d "$apply_source/.git" ]]; then
          cleaned="$(cd "$apply_source" && git ls-files -z 2>/dev/null | xargs -0 cat 2>/dev/null)"
        fi
      fi
      ;;
    *)
      cleaned="$(scope_stdout "$out")"
      if (( rc != 0 )); then
        extra_reasons+=("read_exit:$rc")
      fi
      if LC_ALL=C grep -aqF '(no result)' <<<"$cleaned"; then
        extra_reasons+=("no_result")
      fi
      if [[ -n "$DATA_FIXTURE" ]]; then
        local terminal_path terminal_status
        terminal_path="$(latest_data_terminal_path "$log" || true)"
        if [[ -z "$terminal_path" ]]; then
          extra_reasons+=("data_terminal_missing")
        elif [[ ! -f "$terminal_path" ]]; then
          extra_reasons+=("data_terminal_file_missing:$terminal_path")
        else
          terminal_status="$(json_string_field "$terminal_path" "status" || true)"
          case "$terminal_status" in
            complete|completed)
              ;;
            "")
              extra_reasons+=("data_terminal_status_missing")
              ;;
            *)
              extra_reasons+=("data_terminal_status:$terminal_status")
              ;;
          esac
        fi
      fi
      ;;
  esac

  # PLAN_EXPECT_REGEX always runs against plan.json when MODE is
  # plan|apply. Newline-separated ERE patterns; ALL must match.
  if [[ -n "$MODE" && "$MODE" != "read" && -n "$PLAN_EXPECT_REGEX" ]]; then
    if [[ ! -f "$plan" ]]; then
      extra_reasons+=("no_plan_file")
    else
      local old_ifs="$IFS"
      IFS=$'\n'
      for rx in $PLAN_EXPECT_REGEX; do
        [[ -z "$rx" ]] && continue
        if ! grep -Eq -- "$rx" "$plan"; then
          extra_reasons+=("no_plan_regex:${rx}")
        fi
      done
      IFS="$old_ifs"
    fi
  fi

  # Apply exit code must be 0 for verdict to pass.
  if [[ "$MODE" == "apply" && $rc -ne 0 ]]; then
    extra_reasons+=("apply_exit:$rc")
  fi

  local provider_blocked
  provider_blocked="$(eval_detect_provider_blocked "$log")"
  if [[ -n "$provider_blocked" ]]; then
    printf 'BLOCKED_PROVIDER %s\n' "$provider_blocked" >"$verdict"
    echo "run $i: $(cat "$verdict")" >&2
    return
  fi

  # Optional hidden quality assertions over control-plane logs. These are for
  # eval harness integrity, not product routing: case authors can require that
  # a scenario exercised a typed subsystem (for example the operation runner)
  # instead of merely producing answer text that happens to match.
  if [[ -n "${EXPECT_LOG_MATCHES_REGEX:-}" ]]; then
    if [[ -z "$log" || ! -f "$log" ]]; then
      extra_reasons+=("log_missing")
    else
      old_ifs="$IFS"
      IFS=$'\n'
      for rx in $EXPECT_LOG_MATCHES_REGEX; do
        [[ -z "$rx" ]] && continue
        if ! LC_ALL=C grep -aEq -- "$rx" "$log"; then
          extra_reasons+=("no_log_regex:${rx}")
        fi
      done
      IFS="$old_ifs"
    fi
  fi
  if [[ -n "${EXPECT_LOG_NOT_MATCHES_REGEX:-}" && -n "$log" && -f "$log" ]]; then
    old_ifs="$IFS"
    IFS=$'\n'
    for rx in $EXPECT_LOG_NOT_MATCHES_REGEX; do
      [[ -z "$rx" ]] && continue
      if LC_ALL=C grep -aEq -- "$rx" "$log"; then
        extra_reasons+=("banned_log_regex:${rx}")
      fi
    done
    IFS="$old_ifs"
  fi

  write_verdict "$verdict" "$cleaned" "${extra_reasons[@]:+${extra_reasons[@]}}"
  echo "run $i: $(cat "$verdict")" >&2
}

for i in $(seq 1 "$N"); do
  run_one "$i"
done

# Aggregate.
SUMMARY="$OUTDIR/summary.md"
{
  echo "# Eval results — $ID"
  echo
  echo "- case: \`$NAME\`"
  echo "- question: \`$QUESTION\`"
  echo "- runs: $N"
  echo "- timestamp: $TS"
  echo
  echo "## Verdicts"
  echo
  echo "| run | result | reasons |"
  echo "|----:|--------|---------|"
  pass_count=0
  blocked_count=0
  for i in $(seq 1 "$N"); do
    v="$(cat "$OUTDIR/run-$i.verdict")"
    if [[ "$v" == "PASS" ]]; then
      pass_count=$((pass_count + 1))
      echo "| $i | PASS | — |"
    elif [[ "$v" == BLOCKED_PROVIDER* ]]; then
      blocked_count=$((blocked_count + 1))
      reason="${v#BLOCKED_PROVIDER }"
      echo "| $i | BLOCKED_PROVIDER | $reason |"
    else
      reason="${v#FAIL }"
      echo "| $i | FAIL | $reason |"
    fi
  done
  echo
  echo "**pass rate: $pass_count / $N**"
  if [[ "$blocked_count" -gt 0 ]]; then
    echo
    echo "**provider-blocked: $blocked_count / $N**"
  fi
  echo

  # Write-mode plan artifact summary. Extracts Kind / Path / Patch-len
  # per ChangeUnit from plan.json — the primary diagnostic for
  # answering "did the LLM emit kind=patch end-to-end?". Omitted for
  # read-mode cases (no plan files produced).
  if [[ -n "$MODE" && "$MODE" != "read" ]]; then
    echo "## Plan artifacts"
    echo
    echo "| run | plan written | changes | kinds | paths |"
    echo "|----:|:-------------|--------:|:------|:------|"
    for i in $(seq 1 "$N"); do
      plan_path="$OUTDIR/run-$i.plan.json"
      if [[ ! -f "$plan_path" ]]; then
        echo "| $i | no | — | — | — |"
        continue
      fi
      # "$N changes" where N = count of '"kind":' occurrences.
      change_count=$(grep -c '"kind":' "$plan_path" 2>/dev/null || echo 0)
      kinds="$(grep -oE '"kind":[[:space:]]*"[^"]+"' "$plan_path" 2>/dev/null | sed -E 's/.*"([a-z]+)"$/\1/' | paste -sd, -)"
      kinds="${kinds:-—}"
      paths="$(grep -oE '"path":[[:space:]]*"[^"]+"' "$plan_path" 2>/dev/null | sed -E 's/.*"([^"]+)"$/\1/' | paste -sd, -)"
      paths="${paths:-—}"
      # Collapse overly-long path lists (table readability).
      if [[ ${#paths} -gt 60 ]]; then paths="${paths:0:57}…"; fi
      echo "| $i | yes | $change_count | $kinds | $paths |"
    done
    echo
    if [[ "$MODE" == "apply" ]]; then
      # Post-apply file snapshot for the primary file (when
      # POST_APPLY_FILE is set). Reads from the worktree (via
      # plan.json worktree_path) because apply is sandboxed there;
      # scratch-repo bytes are unchanged by construction (L5 red line).
      # Truncated to 20 lines per run so the summary stays scannable.
      if [[ -n "$POST_APPLY_FILE" ]]; then
        echo "## Post-apply file — \`$POST_APPLY_FILE\` (first 20 lines, from worktree)"
        echo
        for i in $(seq 1 "$N"); do
          echo "### run $i"
          echo
          plan_path="$OUTDIR/run-$i.plan.json"
          src=""
          if [[ -f "$plan_path" ]]; then
            wt=$(grep -oE '"worktree_path":[[:space:]]*"[^"]+"' "$plan_path" 2>/dev/null | sed -E 's/.*"([^"]+)"$/\1/')
            if [[ -n "$wt" && -f "$wt/$POST_APPLY_FILE" ]]; then
              src="$wt/$POST_APPLY_FILE"
            fi
          fi
          if [[ -z "$src" && -f "$OUTDIR/run-$i.repo/$POST_APPLY_FILE" ]]; then
            src="$OUTDIR/run-$i.repo/$POST_APPLY_FILE"
          fi
          if [[ -n "$src" ]]; then
            echo '```'
            head -20 "$src"
            echo '```'
          else
            echo "_(file missing — apply likely did not land or worktree was discarded)_"
          fi
          echo
        done
      fi
    fi
  fi

  echo "## Mechanism trace metrics"
  echo
  echo "| metric | $(seq 1 "$N" | sed 's|^|run |' | tr '\n' '|' | sed 's/|$//') | median |"
  echo "|--------|$(printf '%.0s---|' $(seq 1 "$N"))------|"
  # B6-F5 added per-agent LLM-turn counters (analyzer / explorer /
  # extractor / finalizer iters). R5.1 (post_shape_residual_audit
  # 2026-05-04): write_metrics writes them to run-N.metrics.txt;
  # aggregate them into the summary table so they show up next to
  # the legacy 12 mechanism counters with median.
  metric_keys="data_rounds data_repair_rounds data_record_count data_action_failed data_answer_len tool_read_file tool_repo_map tool_list_files tool_trace_query tool_mcp_read_resource repeated_mcp_resource_reads mcp_tool_calls source_inventory_lens repo_lens_discovery_hints transient_retry_checkpoints unavailable_tool_attempts checkpoint_continuation_broad_hint closure_only_repeated mermaid_source_repair_applied answer_contract_violations answer_contract_lane_block_kind_violations repair_debt_checkpoints repair_debt_close_ready_filters repair_debt_principal_blocking_max repair_debt_surgical_grounding_max repair_debt_advisory_max tool_history_prunes max_context_tokens_est max_context_window max_context_window_pct concrete_values synthesis_runs function_boundary_push enumeration_push focus_warning t11_gate_skip t11_gate_run dataflow_intent_lookup dataflow_intent_propagate midloop_inject parallel_sibling_skips mixed_origin_autocomplete_blocks finalizer_rejects finalizer_rewrites answer_chain_lines analyzer_iters explorer_iters extractor_iters finalizer_iters analyzer_dispatches explorer_dispatches extractor_dispatches finalizer_dispatches repair_plan_lines repair_exec_lines repair_exec_promote repair_exec_failloud semantic_quality_dispatches semantic_quality_concerns strict_decode_remap_events"
  for key in $metric_keys; do
    row="| $key |"
    vals=()
    for i in $(seq 1 "$N"); do
      v=$(grep "^$key=" "$OUTDIR/run-$i.metrics.txt" 2>/dev/null | cut -d= -f2 || echo "")
      v="${v:-0}"
      vals+=("$v")
      row+=" $v |"
    done
    # Median (sorted middle).
    median=$(printf '%s\n' "${vals[@]}" | sort -n | awk -v n="$N" 'NR==int((n+1)/2){print; exit}')
    row+=" $median |"
    echo "$row"
  done
  echo

  echo "## Efficiency advisories"
  echo
  echo "| run | advisory | detail |"
  echo "|----:|----------|--------|"
  advisory_count=0
  for i in $(seq 1 "$N"); do
    repeated_mcp=$(grep "^repeated_mcp_resource_reads=" "$OUTDIR/run-$i.metrics.txt" 2>/dev/null | cut -d= -f2 || echo 0)
    mcp_tools=$(grep "^mcp_tool_calls=" "$OUTDIR/run-$i.metrics.txt" 2>/dev/null | cut -d= -f2 || echo 0)
    mcp_resources=$(grep "^tool_mcp_read_resource=" "$OUTDIR/run-$i.metrics.txt" 2>/dev/null | cut -d= -f2 || echo 0)
    source_reads=$(grep "^tool_read_file=" "$OUTDIR/run-$i.metrics.txt" 2>/dev/null | cut -d= -f2 || echo 0)
    repeated_mcp="${repeated_mcp:-0}"
    mcp_tools="${mcp_tools:-0}"
    mcp_resources="${mcp_resources:-0}"
    source_reads="${source_reads:-0}"
    if [[ "$repeated_mcp" -gt 0 ]]; then
      advisory_count=$((advisory_count + 1))
      echo "| $i | repeated_mcp_resource_reads | repeated=$repeated_mcp |"
    fi
    if [[ "$mcp_tools" -gt 2 ]]; then
      advisory_count=$((advisory_count + 1))
      echo "| $i | high_mcp_tool_calls | calls=$mcp_tools |"
    fi
    if [[ "$mcp_resources" -gt 0 && "$source_reads" -gt 0 ]]; then
      advisory_count=$((advisory_count + 1))
      echo "| $i | mixed_mcp_and_source_reads | mcp_resources=$mcp_resources source_reads=$source_reads |"
    fi
  done
  if [[ "$advisory_count" -eq 0 ]]; then
    echo "| — | none | — |"
  fi
  echo
} >"$SUMMARY"

echo >&2
echo "summary written: $SUMMARY" >&2
cat "$SUMMARY"
