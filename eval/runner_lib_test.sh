#!/usr/bin/env bash

set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT" || exit 1

# shellcheck source=eval/runner_lib.sh
source eval/runner_lib.sh

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_eq() {
  local got="$1"
  local want="$2"
  local label="$3"
  if [[ "$got" != "$want" ]]; then
    fail "$label: got '$got', want '$want'"
  fi
}

tmp="$(mktemp -d "${TMPDIR:-/tmp}/codrax-eval-runner-test.XXXXXX")" || exit 1
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$tmp/results/sample-20260511-000000"
mkdir -p "$tmp/results/sample-20260511-010000"
latest="$(eval_latest_result_dir "$tmp/results" sample 20260511-005959)" || fail "latest result dir not found"
assert_eq "$latest" "$tmp/results/sample-20260511-010000" "fresh result selection"

printf 'analyzer_dispatches=2\nstring_metric=complete\nnul_metric=7\000\nfinalizer_dispatches=5\n' >"$tmp/metrics.txt"
assert_eq "$(eval_metric_field "$tmp/metrics.txt" analyzer_dispatches)" "2" "metric field parse"
assert_eq "$(eval_metric_field "$tmp/metrics.txt" string_metric)" "complete" "string metric field parse"
assert_eq "$(eval_metric_field "$tmp/metrics.txt" nul_metric)" "7" "metric field parse with NUL"
assert_eq "$(eval_metric_field "$tmp/metrics.txt" missing_key)" "-" "missing metric field"
assert_eq "$(eval_metric_int_field "$tmp/metrics.txt" analyzer_dispatches)" "2" "metric int field parse"
assert_eq "$(eval_metric_int_field "$tmp/metrics.txt" string_metric)" "0" "metric int non-numeric fallback"
assert_eq "$(eval_metric_int_field "$tmp/metrics.txt" missing_key)" "0" "missing metric int fallback"
metric_row="$(eval_print_efficiency_advisory_row "$tmp/metrics.txt" 1 high_analyzer_dispatches analyzer_dispatches 1 || true)"
assert_eq "$metric_row" "| 1 | high_analyzer_dispatches | analyzer_dispatches=2 limit=1 |" "metric advisory row"
assert_eq "$(eval_print_efficiency_advisory_row "$tmp/metrics.txt" 1 high_analyzer_dispatches analyzer_dispatches 2 || true)" "" "metric advisory row under limit"
assert_eq "$(eval_metric_budget_reasons "$tmp/metrics.txt" analyzer_dispatches 1 finalizer_dispatches 10)" "perf_budget:analyzer_dispatches:2>1" "metric budget reason"

printf 'one\nreject\000\nreject\n' >"$tmp/log.txt"
assert_eq "$(eval_count_pattern 'reject' "$tmp/log.txt")" "2" "pattern count"
assert_eq "$(eval_count_pattern 'reject$' "$tmp/log.txt")" "2" "pattern count with NUL line"
assert_eq "$(eval_count_pattern 'reject' "$tmp/missing.log")" "0" "missing log pattern count"

mkdir -p "$tmp/write-eval" "$tmp/write-scratch" "$tmp/write-worktree/.codrax/plans"
cat >"$tmp/write-eval/run-1.plan.json" <<JSON
{
  "id": "plan-write-eval-pass",
  "status": "applied",
  "worktree_path": "$tmp/write-worktree"
}
JSON
cat >"$tmp/write-eval/plan-write-eval-pass.report.json" <<'JSON'
{
  "plan_id": "plan-write-eval-pass",
  "channel": "post_apply_verify",
  "test_results": [
    {
      "passed": true
    }
  ],
  "passed": true
}
JSON
eval_write_apply_result_record "$tmp/write-eval/pass.json" "$tmp/write-eval/run-1.plan.json" "$tmp/write-eval" "$tmp/write-scratch" 1 1 0
assert_eq "$(eval_json_top_string_field "$tmp/write-eval/pass.json" plan_id)" "plan-write-eval-pass" "write apply result plan id"
assert_eq "$(eval_json_top_bool_field "$tmp/write-eval/pass.json" report_exists)" "true" "write apply result report exists"
assert_eq "$(eval_json_top_bool_field "$tmp/write-eval/pass.json" report_passed)" "true" "write apply result report passed"
assert_eq "$(eval_json_top_bool_field "$tmp/write-eval/pass.json" verify_authoritative)" "true" "write apply result authoritative"

cat >"$tmp/write-eval/run-2.plan.json" <<JSON
{
  "id": "plan-write-eval-missing",
  "status": "applied",
  "worktree_path": "$tmp/write-worktree"
}
JSON
eval_write_apply_result_record "$tmp/write-eval/missing.json" "$tmp/write-eval/run-2.plan.json" "$tmp/write-eval" "$tmp/write-scratch" 1 1 0
assert_eq "$(eval_json_top_bool_field "$tmp/write-eval/missing.json" report_exists)" "false" "write apply result missing report"
assert_eq "$(eval_json_top_bool_field "$tmp/write-eval/missing.json" verify_authoritative)" "false" "write apply missing report not authoritative"

cat >"$tmp/write-eval/run-3.plan.json" <<JSON
{
  "id": "plan-write-eval-fail",
  "status": "applied",
  "worktree_path": "$tmp/write-worktree"
}
JSON
cat >"$tmp/write-eval/plan-write-eval-fail.report.json" <<'JSON'
{
  "plan_id": "plan-write-eval-fail",
  "channel": "post_apply_verify",
  "passed": false
}
JSON
eval_write_apply_result_record "$tmp/write-eval/fail.json" "$tmp/write-eval/run-3.plan.json" "$tmp/write-eval" "$tmp/write-scratch" 1 1 0
assert_eq "$(eval_json_top_bool_field "$tmp/write-eval/fail.json" report_exists)" "true" "write apply failed report exists"
assert_eq "$(eval_json_top_bool_field "$tmp/write-eval/fail.json" report_passed)" "false" "write apply failed report parsed"
assert_eq "$(eval_json_top_bool_field "$tmp/write-eval/fail.json" verify_authoritative)" "false" "write apply failed report not authoritative"

cat >"$tmp/write-eval/run-4.plan.json" <<JSON
{
  "id": "plan-write-eval-mismatch",
  "status": "applied",
  "worktree_path": "$tmp/write-worktree"
}
JSON
cat >"$tmp/write-eval/plan-write-eval-mismatch.report.json" <<'JSON'
{
  "plan_id": "other-plan",
  "channel": "post_apply_verify",
  "passed": true
}
JSON
eval_write_apply_result_record "$tmp/write-eval/mismatch.json" "$tmp/write-eval/run-4.plan.json" "$tmp/write-eval" "$tmp/write-scratch" 1 1 0
assert_eq "$(eval_json_top_string_field "$tmp/write-eval/mismatch.json" report_plan_id)" "other-plan" "write apply mismatched report plan"
assert_eq "$(eval_json_top_bool_field "$tmp/write-eval/mismatch.json" verify_authoritative)" "false" "write apply mismatched report not authoritative"

mkdir -p "$tmp/source-parent/archive-tree" "$tmp/source-root"
git -C "$tmp/source-parent" init -q || fail "source-parent git init failed"
printf 'parent tracked\n' >"$tmp/source-parent/tracked.txt"
git -C "$tmp/source-parent" add tracked.txt || fail "source-parent git add failed"
printf 'archive applied source\n' >"$tmp/source-parent/archive-tree/main.py"
archive_collected="$(eval_collect_apply_source_text "$tmp/source-parent/archive-tree")"
case "$archive_collected" in
  *"archive applied source"*)
    ;;
  *)
    fail "non-git materialized tree under a parent git repo should be collected by file traversal"
    ;;
esac

git -C "$tmp/source-root" init -q || fail "source-root git init failed"
printf 'root tracked\n' >"$tmp/source-root/tracked.txt"
printf 'root untracked\n' >"$tmp/source-root/untracked.txt"
git -C "$tmp/source-root" add tracked.txt || fail "source-root git add failed"
root_collected="$(eval_collect_apply_source_text "$tmp/source-root")"
case "$root_collected" in
  *"root tracked"*)
    ;;
  *)
    fail "git-root source should collect tracked files"
    ;;
esac
case "$root_collected" in
  *"root untracked"*)
    fail "git-root source should not collect untracked files"
    ;;
esac

cat >"$tmp/finalizer-control.log" <<'LOG'
2026-05-24T00:00:00.000 DEBUG [diag finalizer] iter=0 ASSISTANT content: source mentions finalizer_rejects=7 and 成文校验未通过 but this is answer text
2026-05-24T00:00:00.001 DEBUG [diag explorer] iter=0 ASSISTANT content: ⟳ 4/4 答案待完善，正在重写 is quoted customer text
2026-05-24T00:00:00.002 DEBUG [diag finalizer] iter=0 ASSISTANT content: quoted full line 2026-05-24T00:00:00.010 DEBUG [diag finalizer] iter=0 phase=toolresult TOOLRESULT emit_answer_document ok=false len=12:
2026-05-24T00:00:00.010 DEBUG [diag finalizer] iter=0 phase=toolresult TOOLRESULT emit_answer_document ok=false len=12:
2026-05-24T00:00:00.015 DEBUG [diag finalizer] iter=0 phase=toolcall call[0] tool=emit_answer_document_patch params={"replace_blocks":[]}
2026-05-24T00:00:00.020 INFO [render]   • 成文校验未通过
2026-05-24T00:00:00.030 INFO [render]   ⟳ 4/4 答案待完善，正在重写
2026-05-24T00:00:00.040 DEBUG [diag explorer] iter=1 phase=midloop_inject MIDLOOP inject len=123:
2026-05-24T00:00:00.050 DEBUG [diag explorer] iter=1 ASSISTANT content_len=5
2026-05-24T00:00:00.060 DEBUG [diag explorer] DISPATCH stage=explore attempt=1
2026-05-24T00:00:00.065 DEBUG [diag explorer] iter=1 phase=toolcall call[0] tool=repo_map params={"view":"source_inventory"}
2026-05-24T00:00:00.066 DEBUG [diag explorer] iter=1 phase=toolcall call[1] tool=read_file params={"path":"a.go"}
2026-05-24T00:00:00.067 DEBUG [diag explorer] iter=1 phase=prune TOOL HISTORY PRUNED (budget=153600 bytes)
2026-05-24T00:00:00.068 DEBUG [diag explorer] iter=1 phase=llm_request model=test context_tokens_est=512 context_window=4096 messages=3 tools=2
2026-05-24T00:00:00.068 DEBUG [diag orchestrator] phase=transient_retry_checkpoint stage=explore installed=true len=512
2026-05-24T00:00:00.068 DEBUG [orchestrator] window hint applied key="orchestrator.dag-window" len=400 body="Checkpoint summary (non-authoritative counts): structured evidence rows=2. DAG-scheduled investigation window. Cover every node objective below in this dispatch:"
2026-05-24T00:00:00.068 DEBUG [repo_lens] discovery_hint stage=explore agent=explorer tool=list_files len=512
2026-05-24T00:00:00.068 DEBUG [diag explorer] iter=2 phase=midloop_signal hint=true progress=true stop=false key="explorer.mid-loop.read-without-emit-closure-only.2" → inject_hint ()
2026-05-24T00:00:00.068 WARN [agent] tool "grep" rejected before execution: not in current tool schema
2026-05-24T00:00:00.068 WARN [explorer] tool "read_file" rejected: tool "read_file" is not available in the current explorer repair state; available tools here: emit_evidence
2026-05-24T00:00:00.068 DEBUG [mermaidcompat] source repair applied before_bytes=120 after_bytes=119
2026-05-24T00:00:00.068 DEBUG [repair_debt] checkpoint principal_blocking=2 surgical_grounding=3 advisory=4 rows=9
2026-05-24T00:00:00.068 DEBUG [repair_debt] close_ready filtered_advisory=1 remaining=2
2026-05-24T00:00:00.069 DEBUG [diag finalizer] iter=0 phase=llm_request model=test context_tokens_est=2049 context_window=4096 messages=5 tools=1
2026-05-24T00:00:00.069 DEBUG [diag finalizer] phase=answer_contract_check section=lane_block_kind violations=1 elapsed=1ms
2026-05-24T00:00:00.069 DEBUG [diag finalizer] phase=answer_contract_check section=member_set_coverage violations=3 elapsed=2ms
2026-05-24T00:00:00.070 INFO [semantic_quality_reviewer] verdict sufficient=false confidence=0.9
2026-05-24T00:00:00.080 INFO [semantic_quality_reviewer] emitted 1 concern(s)
2026-05-24T00:00:00.090 INFO [self_consistency_reviewer] V2 emitted 1 contradiction(s)
2026-05-24T00:00:00.100 DEBUG [orchestrator] violation kind=self_contradiction detail=x
LOG
assert_eq "$(eval_count_finalizer_rejects "$tmp/finalizer-control.log")" "2" "finalizer reject control count"
assert_eq "$(eval_count_finalizer_rewrites "$tmp/finalizer-control.log")" "1" "finalizer rewrite control count"
assert_eq "$(eval_count_answer_document_patch_calls "$tmp/finalizer-control.log")" "1" "answer patch control count"
assert_eq "$(eval_count_midloop_injects "$tmp/finalizer-control.log")" "1" "midloop inject control count"
assert_eq "$(eval_count_tool_calls "$tmp/finalizer-control.log" repo_map)" "1" "tool-call control count"
assert_eq "$(eval_count_source_inventory_tool_calls "$tmp/finalizer-control.log")" "1" "source inventory tool-call count"
assert_eq "$(eval_count_repo_lens_discovery_hints "$tmp/finalizer-control.log")" "1" "repo lens discovery hint control count"
assert_eq "$(eval_count_tool_calls "$tmp/finalizer-control.log" read_file)" "1" "read-file tool-call control count"
assert_eq "$(eval_count_transient_retry_checkpoints "$tmp/finalizer-control.log")" "1" "transient retry checkpoint control count"
assert_eq "$(eval_count_unavailable_tool_attempts "$tmp/finalizer-control.log")" "2" "unavailable tool attempt control count"
assert_eq "$(eval_count_checkpoint_continuation_broad_hint "$tmp/finalizer-control.log")" "1" "checkpoint broad hint control count"
assert_eq "$(eval_count_closure_only_repeated "$tmp/finalizer-control.log")" "1" "closure-only repeated control count"
assert_eq "$(eval_count_mermaid_source_repairs "$tmp/finalizer-control.log")" "1" "mermaid repair control count"
assert_eq "$(eval_count_repair_debt_checkpoints "$tmp/finalizer-control.log")" "1" "repair debt checkpoint count"
assert_eq "$(eval_count_repair_debt_close_ready_filters "$tmp/finalizer-control.log")" "1" "repair debt close-ready filter count"
assert_eq "$(eval_max_repair_debt_checkpoint_class "$tmp/finalizer-control.log" principal_blocking)" "2" "repair debt principal max"
assert_eq "$(eval_max_repair_debt_checkpoint_class "$tmp/finalizer-control.log" surgical_grounding)" "3" "repair debt surgical max"
assert_eq "$(eval_max_repair_debt_checkpoint_class "$tmp/finalizer-control.log" advisory)" "4" "repair debt advisory max"
assert_eq "$(eval_count_control_pattern 'DEBUG \[diag [^]]+\][^:]*phase=prune TOOL HISTORY PRUNED' "$tmp/finalizer-control.log")" "1" "tool history prune control count"
assert_eq "$(eval_max_context_tokens_estimate "$tmp/finalizer-control.log")" "2049" "max context token estimate"
assert_eq "$(eval_max_context_window_tokens "$tmp/finalizer-control.log")" "4096" "max context window"
assert_eq "$(eval_max_context_window_pct "$tmp/finalizer-control.log")" "50" "max context window pct"
assert_eq "$(eval_sum_answer_contract_violations "$tmp/finalizer-control.log")" "4" "answer contract violation sum"
assert_eq "$(eval_sum_answer_contract_strict_violations "$tmp/finalizer-control.log")" "4" "legacy answer contract strict violation fallback"
assert_eq "$(eval_sum_answer_contract_violations_for_section "$tmp/finalizer-control.log" lane_block_kind)" "1" "answer contract section violation sum"
assert_eq "$(eval_sum_answer_contract_strict_violations_for_section "$tmp/finalizer-control.log" lane_block_kind)" "1" "legacy answer contract section strict fallback"
assert_eq "$(eval_count_agent_iterations "$tmp/finalizer-control.log" explorer)" "1" "agent iteration control count"
assert_eq "$(eval_count_agent_dispatches "$tmp/finalizer-control.log" explorer)" "1" "agent dispatch control count"
assert_eq "$(eval_count_semantic_quality_dispatches "$tmp/finalizer-control.log")" "1" "semantic dispatch control count"
assert_eq "$(eval_count_semantic_quality_concerns "$tmp/finalizer-control.log")" "2" "semantic concern control count"
assert_eq "$(eval_count_self_consistency_concerns "$tmp/finalizer-control.log")" "2" "self consistency control count"

cat >"$tmp/mcp-repeat-control.log" <<'LOG'
2026-05-24T00:00:00.001 DEBUG [diag explorer] iter=0 phase=toolcall call[0] tool=mcp_read_resource params={"uri":"mcp://fixture/a"}
2026-05-24T00:00:00.002 DEBUG [diag explorer] iter=1 phase=toolcall call[0] tool=mcp_read_resource params={"uri":"mcp://fixture/a"}
2026-05-24T00:00:00.003 DEBUG [diag explorer] iter=2 phase=toolcall call[0] tool=mcp_read_resource params={"uri":"mcp://fixture/b"}
2026-05-24T00:00:00.004 DEBUG [diag explorer] iter=3 ASSISTANT content: quoted 2026-05-24T00:00:00.005 DEBUG [diag explorer] iter=4 phase=toolcall call[0] tool=mcp_read_resource params={"uri":"mcp://fixture/a"}
LOG
assert_eq "$(eval_count_repeated_mcp_resource_reads "$tmp/mcp-repeat-control.log")" "1" "repeated MCP resource read control count"

cat >"$tmp/fake-codrax-dimension-pass" <<'SH'
#!/usr/bin/env bash
echo 'thinking stream'
echo '━━━'
echo 'extend Cart — 包路径 demo.cart'
echo 'foreign func native_add — 包路径 demo.bridge'
SH
chmod +x "$tmp/fake-codrax-dimension-pass"

cat >"$tmp/dimension-pass.case" <<'CASE'
ID="dimension_pass"
NAME="dimension pass"
QUESTION="dimension test"
MIN_OUTPUT_CHARS=1
EXPECT_DIMENSIONS="package"
EXPECT_DIMENSION_VALUES_PACKAGE="demo.cart demo.bridge"
CASE
CODRAX_BIN="$tmp/fake-codrax-dimension-pass" EVAL_RESULTS_ROOT="$tmp/dimension-results" CODRAX_PROVIDER_ARGS_RAW="" eval/run.sh "$tmp/dimension-pass.case" 1 >/dev/null || fail "dimension pass eval failed to run"
dimension_pass_dir="$(eval_latest_result_dir "$tmp/dimension-results" dimension_pass 00000000-000000 || true)"
[[ -n "$dimension_pass_dir" ]] || fail "dimension pass result dir missing"
assert_eq "$(cat "$dimension_pass_dir/run-1.verdict")" "PASS" "dimension semantic values should pass without literal section label"

cat >"$tmp/dimension-missing.case" <<'CASE'
ID="dimension_missing"
NAME="dimension missing"
QUESTION="dimension test"
MIN_OUTPUT_CHARS=1
EXPECT_DIMENSIONS="package"
EXPECT_DIMENSION_VALUES_PACKAGE="demo.cart demo.missing"
CASE
CODRAX_BIN="$tmp/fake-codrax-dimension-pass" EVAL_RESULTS_ROOT="$tmp/dimension-results" CODRAX_PROVIDER_ARGS_RAW="" eval/run.sh "$tmp/dimension-missing.case" 1 >/dev/null || fail "dimension missing eval failed to run"
dimension_missing_dir="$(eval_latest_result_dir "$tmp/dimension-results" dimension_missing 00000000-000000 || true)"
[[ -n "$dimension_missing_dir" ]] || fail "dimension missing result dir missing"
case "$(cat "$dimension_missing_dir/run-1.verdict")" in
  "FAIL missing_dimension:package:demo.missing")
    ;;
  *)
    fail "dimension missing should report missing semantic value, got: $(cat "$dimension_missing_dir/run-1.verdict")"
    ;;
esac

cat >"$tmp/finalizer-content-only.log" <<'LOG'
2026-05-24T00:00:00.000 DEBUG [diag finalizer] iter=0 ASSISTANT content: the source code contains TOOLRESULT emit_answer_document ok=false and finalizer_rewrites strings
2026-05-24T00:00:00.001 DEBUG [diag explorer] iter=0 ASSISTANT content: 客户日志片段里有 成文校验未通过 和 ⟳ 4/4 答案待完善，正在重写
2026-05-24T00:00:00.002 DEBUG [diag explorer] iter=0 ASSISTANT content: quoted 2026-05-24T00:00:00.040 DEBUG [diag explorer] iter=1 phase=midloop_inject MIDLOOP inject len=123:
2026-05-24T00:00:00.003 DEBUG [diag finalizer] iter=0 ASSISTANT content: quoted 2026-05-24T00:00:00.015 DEBUG [diag finalizer] iter=0 phase=toolcall call[0] tool=emit_answer_document_patch params={}
2026-05-24T00:00:00.004 DEBUG [diag explorer] iter=0 ASSISTANT content: source says 2026-05-24T00:00:00.050 DEBUG [diag explorer] iter=1 ASSISTANT content_len=5
2026-05-24T00:00:00.005 DEBUG [diag explorer] iter=0 ASSISTANT content: source says 2026-05-24T00:00:00.060 DEBUG [diag explorer] DISPATCH stage=explore attempt=1
2026-05-24T00:00:00.005 DEBUG [diag explorer] iter=0 ASSISTANT content: source says 2026-05-24T00:00:00.065 DEBUG [diag explorer] iter=1 phase=toolcall call[0] tool=repo_map params={"view":"source_inventory"}
2026-05-24T00:00:00.005 DEBUG [diag explorer] iter=0 ASSISTANT content: source says 2026-05-24T00:00:00.067 DEBUG [diag explorer] iter=1 phase=prune TOOL HISTORY PRUNED
2026-05-24T00:00:00.005 DEBUG [diag explorer] iter=0 ASSISTANT content: source says 2026-05-24T00:00:00.068 DEBUG [diag explorer] iter=1 phase=llm_request model=test context_tokens_est=999 context_window=1000
2026-05-24T00:00:00.005 DEBUG [diag explorer] iter=0 ASSISTANT content: source says 2026-05-24T00:00:00.068 DEBUG [diag orchestrator] phase=transient_retry_checkpoint stage=explore installed=true len=512
2026-05-24T00:00:00.005 DEBUG [diag explorer] iter=0 ASSISTANT content: source says 2026-05-24T00:00:00.068 DEBUG [orchestrator] window hint applied key="orchestrator.dag-window" body="Checkpoint summary ... DAG-scheduled investigation window"
2026-05-24T00:00:00.005 DEBUG [diag explorer] iter=0 ASSISTANT content: source says 2026-05-24T00:00:00.068 WARN [agent] tool "grep" rejected before execution: not in current tool schema
2026-05-24T00:00:00.005 DEBUG [diag explorer] iter=0 ASSISTANT content: source says 2026-05-24T00:00:00.068 DEBUG [diag explorer] iter=2 phase=midloop_signal hint=true progress=true key="explorer.mid-loop.read-without-emit-closure-only.2"
2026-05-24T00:00:00.005 DEBUG [diag explorer] iter=0 ASSISTANT content: source says 2026-05-24T00:00:00.068 DEBUG [mermaidcompat] source repair applied before_bytes=120 after_bytes=119
2026-05-24T00:00:00.005 DEBUG [diag explorer] iter=0 ASSISTANT content: source says 2026-05-24T00:00:00.068 DEBUG [repair_debt] checkpoint principal_blocking=9 surgical_grounding=9 advisory=9 rows=27
2026-05-24T00:00:00.005 DEBUG [diag explorer] iter=0 ASSISTANT content: source says 2026-05-24T00:00:00.068 DEBUG [repair_debt] close_ready filtered_advisory=9 remaining=9
2026-05-24T00:00:00.005 DEBUG [diag explorer] iter=0 ASSISTANT content: source says 2026-05-24T00:00:00.069 DEBUG [diag finalizer] phase=answer_contract_check section=lane_block_kind violations=9 elapsed=1ms
2026-05-24T00:00:00.006 DEBUG [diag explorer] iter=0 ASSISTANT content: source mentions 2026-05-24T00:00:00.070 INFO [semantic_quality_reviewer] verdict sufficient=false confidence=0.9
2026-05-24T00:00:00.007 DEBUG [diag explorer] iter=0 ASSISTANT content: source mentions 2026-05-24T00:00:00.090 INFO [self_consistency_reviewer] V2 emitted 1 contradiction(s)
2026-05-24T00:00:00.008 DEBUG [diag explorer] iter=0 ASSISTANT content: Repo Lens: Source Inventory suggests view="source_inventory"
2026-05-24T00:00:00.009 DEBUG [diag explorer] iter=0 ASSISTANT content: quoted 2026-05-24T00:00:00.068 DEBUG [repo_lens] discovery_hint stage=explore agent=explorer tool=list_files len=512
LOG
assert_eq "$(eval_count_finalizer_rejects "$tmp/finalizer-content-only.log")" "0" "finalizer reject content contamination"
assert_eq "$(eval_count_finalizer_rewrites "$tmp/finalizer-content-only.log")" "0" "finalizer rewrite content contamination"
assert_eq "$(eval_count_answer_document_patch_calls "$tmp/finalizer-content-only.log")" "0" "answer patch content contamination"
assert_eq "$(eval_count_midloop_injects "$tmp/finalizer-content-only.log")" "0" "midloop inject content contamination"
assert_eq "$(eval_count_tool_calls "$tmp/finalizer-content-only.log" repo_map)" "0" "tool-call content contamination"
assert_eq "$(eval_count_source_inventory_tool_calls "$tmp/finalizer-content-only.log")" "0" "source inventory content contamination"
assert_eq "$(eval_count_repo_lens_discovery_hints "$tmp/finalizer-content-only.log")" "0" "repo lens discovery content contamination"
assert_eq "$(eval_count_transient_retry_checkpoints "$tmp/finalizer-content-only.log")" "0" "transient retry checkpoint content contamination"
assert_eq "$(eval_count_unavailable_tool_attempts "$tmp/finalizer-content-only.log")" "0" "unavailable tool attempt content contamination"
assert_eq "$(eval_count_checkpoint_continuation_broad_hint "$tmp/finalizer-content-only.log")" "0" "checkpoint broad hint content contamination"
assert_eq "$(eval_count_closure_only_repeated "$tmp/finalizer-content-only.log")" "0" "closure-only repeated content contamination"
assert_eq "$(eval_count_mermaid_source_repairs "$tmp/finalizer-content-only.log")" "0" "mermaid repair content contamination"
assert_eq "$(eval_count_repair_debt_checkpoints "$tmp/finalizer-content-only.log")" "0" "repair debt checkpoint content contamination"
assert_eq "$(eval_count_repair_debt_close_ready_filters "$tmp/finalizer-content-only.log")" "0" "repair debt close-ready filter content contamination"
assert_eq "$(eval_max_repair_debt_checkpoint_class "$tmp/finalizer-content-only.log" principal_blocking)" "0" "repair debt principal content contamination"
assert_eq "$(eval_count_control_pattern 'DEBUG \[diag [^]]+\][^:]*phase=prune TOOL HISTORY PRUNED' "$tmp/finalizer-content-only.log")" "0" "tool history prune content contamination"
assert_eq "$(eval_max_context_tokens_estimate "$tmp/finalizer-content-only.log")" "0" "max context token estimate content contamination"
assert_eq "$(eval_max_context_window_pct "$tmp/finalizer-content-only.log")" "0" "max context pct content contamination"
assert_eq "$(eval_sum_answer_contract_violations "$tmp/finalizer-content-only.log")" "0" "answer contract violation content contamination"
assert_eq "$(eval_sum_answer_contract_strict_violations "$tmp/finalizer-content-only.log")" "0" "answer contract strict content contamination"
assert_eq "$(eval_sum_answer_contract_violations_for_section "$tmp/finalizer-content-only.log" lane_block_kind)" "0" "answer contract section content contamination"
assert_eq "$(eval_sum_answer_contract_strict_violations_for_section "$tmp/finalizer-content-only.log" lane_block_kind)" "0" "answer contract strict section content contamination"

cat >"$tmp/finalizer-contract-severity.log" <<'LOG'
2026-06-08T00:00:01.000 DEBUG [diag finalizer] phase=answer_contract_check section=v2_block_oracles done elapsed=1ms violations=4 strict_violations=1 soft_violations=3
2026-06-08T00:00:02.000 DEBUG [diag finalizer] phase=answer_contract_check section=lane_block_kind done elapsed=1ms violations=2 strict_violations=0 soft_violations=2
LOG
assert_eq "$(eval_sum_answer_contract_violations "$tmp/finalizer-contract-severity.log")" "1" "answer contract violations excludes soft advisory"
assert_eq "$(eval_sum_answer_contract_strict_violations "$tmp/finalizer-contract-severity.log")" "1" "answer contract strict excludes soft advisory"
assert_eq "$(eval_sum_answer_contract_advisories "$tmp/finalizer-contract-severity.log")" "5" "answer contract advisory metric preserves soft audit"
assert_eq "$(eval_sum_answer_contract_violations_for_section "$tmp/finalizer-contract-severity.log" lane_block_kind)" "0" "answer contract section violations excludes soft advisory"
assert_eq "$(eval_sum_answer_contract_strict_violations_for_section "$tmp/finalizer-contract-severity.log" lane_block_kind)" "0" "answer contract section strict excludes soft advisory"
assert_eq "$(eval_sum_answer_contract_advisories_for_section "$tmp/finalizer-contract-severity.log" lane_block_kind)" "2" "answer contract section advisory metric preserves soft audit"
assert_eq "$(eval_count_agent_iterations "$tmp/finalizer-content-only.log" explorer)" "0" "agent iteration content contamination"
assert_eq "$(eval_count_agent_dispatches "$tmp/finalizer-content-only.log" explorer)" "0" "agent dispatch content contamination"
assert_eq "$(eval_count_semantic_quality_dispatches "$tmp/finalizer-content-only.log")" "0" "semantic dispatch content contamination"
assert_eq "$(eval_count_semantic_quality_concerns "$tmp/finalizer-content-only.log")" "0" "semantic concern content contamination"
assert_eq "$(eval_count_self_consistency_concerns "$tmp/finalizer-content-only.log")" "0" "self consistency content contamination"

cat >"$tmp/provider-blocked-control.log" <<'LOG'
2026-06-04T15:11:34.552 WARN [agent/analyzer] LLM call failed at iter=0 (LLM API error (status 402): {"type":"error","error":{"type":"insufficient_balance_error","message":"insufficient balance (1008)","http_code":"402"}})
2026-06-04T15:11:35.000 ERROR [llm] default adapter: LLM provider is not configured, so Codrax cannot start
2026-06-04T15:11:36.000 ERROR [llm] default adapter: providers.yaml: llm.default.provider is required
LOG
assert_eq "$(eval_detect_provider_blocked "$tmp/provider-blocked-control.log")" "insufficient_balance,provider_unconfigured" "provider blocked control classification"

cat >"$tmp/provider-blocked-content-only.log" <<'LOG'
2026-06-04T15:11:34.552 DEBUG [diag explorer] iter=0 ASSISTANT content: customer pasted "LLM API error (status 402): insufficient_balance_error"
LOG
assert_eq "$(eval_detect_provider_blocked "$tmp/provider-blocked-content-only.log")" "" "provider blocked content contamination"

if command -v timeout >/dev/null 2>&1 ||
  command -v gtimeout >/dev/null 2>&1 ||
  command -v python3 >/dev/null 2>&1; then
  start="$(date +%s)"
  eval_run_with_timeout 1 bash -c 'sleep 5' >/dev/null 2>&1
  rc=$?
  elapsed=$(($(date +%s) - start))
  assert_eq "$rc" "124" "timeout return code"
  if [[ "$elapsed" -gt 12 ]]; then
    fail "timeout fallback took too long: ${elapsed}s"
  fi
fi

if command -v python3 >/dev/null 2>&1; then
  leak="$tmp/timeout-grandchild-leak"
  eval_run_with_timeout 1 bash -c '(sleep 3; echo leaked > "$1") & wait' _ "$leak" >/dev/null 2>&1
  rc=$?
  assert_eq "$rc" "124" "process-group timeout return code"
  sleep 4
  if [[ -e "$leak" ]]; then
    fail "timeout left a grandchild running after parent exit"
  fi
fi

partial_dir="$tmp/partial-results/arkts_repomap-20260622-101305"
mkdir -p "$partial_dir/run-1.logs"
cat >"$partial_dir/run-1.logs/codrax-20260622-101306-000-1.log" <<'LOG'
2026-06-22T10:13:06.000 DEBUG [diag analyzer] DISPATCH stage=analyze attempt=1
2026-06-22T10:13:07.000 DEBUG [diag explorer] DISPATCH stage=explore attempt=1
2026-06-22T10:13:08.000 DEBUG [diag explorer] iter=0 ASSISTANT content_len=12
2026-06-22T10:13:09.000 DEBUG [diag explorer] phase=toolcall tool=repo_map params={"view":"source_inventory","scope":"internal/thirdparty/tree-sitter-arkts/corpus/sources"}
2026-06-22T10:13:10.000 DEBUG [diag explorer] phase=toolcall tool=read_file params={"path":"internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets"}
2026-06-22T10:13:11.000 DEBUG [diag explorer] phase=midloop_inject key="explorer.mid-loop.source-inventory"
LOG
eval_materialize_partial_run_result "$partial_dir" 1 124 901 "eval_worker_incomplete"
assert_eq "$(head -1 "$partial_dir/run-1.verdict")" "TIMEOUT eval_worker_incomplete" "partial timeout verdict"
assert_eq "$(cat "$partial_dir/run-1.wall")" "901" "partial timeout wall"
if [[ ! -s "$partial_dir/run-1.logs.all.log" ]]; then
  fail "partial timeout did not aggregate logs"
fi
assert_eq "$(eval_metric_field "$partial_dir/run-1.metrics.txt" partial_result)" "1" "partial timeout metric marker"
assert_eq "$(eval_metric_field "$partial_dir/run-1.metrics.txt" exit_code)" "124" "partial timeout exit metric"
assert_eq "$(eval_metric_field "$partial_dir/run-1.metrics.txt" tool_repo_map)" "1" "partial timeout repo_map metric"
assert_eq "$(eval_metric_field "$partial_dir/run-1.metrics.txt" source_inventory_lens)" "1" "partial timeout source lens metric"
assert_eq "$(eval_metric_field "$partial_dir/run-1.metrics.txt" tool_read_file)" "1" "partial timeout read metric"
assert_eq "$(eval_metric_field "$partial_dir/run-1.metrics.txt" analyzer_dispatches)" "1" "partial timeout analyzer dispatch metric"
assert_eq "$(eval_metric_field "$partial_dir/run-1.metrics.txt" explorer_iters)" "1" "partial timeout explorer iter metric"
assert_eq "$(eval_metric_field "$partial_dir/run-1.metrics.txt" midloop_inject)" "1" "partial timeout midloop metric"

true_bin="$(command -v true || true)"
if [[ -n "$true_bin" && -x "$true_bin" ]]; then
  case_file="$tmp/runner_contract.case"
cat >"$case_file" <<'CASE'
ID=runner_contract
NAME="runner contract"
QUESTION="runner harness smoke"
EXPECT_CONTAINS="this-will-not-appear"
CASE
  out="$(CODRAX_BIN="$true_bin" EVAL_RESULTS_ROOT="$tmp/eval-results" bash eval/run.sh "$case_file" 1 2>&1)"
  if [[ "$out" != *"using codrax snapshot: $true_bin"* ]]; then
    fail "eval/run.sh did not honor CODRAX_BIN snapshot; output: $out"
  fi
  result_count="$(find "$tmp/eval-results" -maxdepth 1 -type d -name 'runner_contract-*' | wc -l | tr -d ' ')"
  if [[ "$result_count" -lt 1 ]]; then
    fail "eval/run.sh did not write under EVAL_RESULTS_ROOT"
  fi
fi

fake_multilog="$tmp/fake-codrax-multilog"
cat >"$fake_multilog" <<'FAKE'
#!/usr/bin/env bash
set -euo pipefail
logdir=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --log-dir)
      logdir="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
mkdir -p "$logdir"
cat >"$logdir/codrax-20260608-000001-000-1.log" <<'LOG'
2026-06-08T00:00:01.000 INFO [cmd/route] single-shot turn policy raw_route=data route=data source=repo
LOG
cat >"$logdir/codrax-20260608-000002-000-1.log" <<'LOG'
2026-06-08T00:00:02.000 DEBUG [diag finalizer] phase=answer_contract_check section=lane_block_kind violations=2 strict_violations=0 soft_violations=2 elapsed=1ms
LOG
printf 'aggregated-answer\n'
FAKE
chmod +x "$fake_multilog"
case_file="$tmp/runner_multilog.case"
cat >"$case_file" <<'CASE'
ID=runner_multilog
NAME="runner multilog"
QUESTION="runner harness multilog smoke"
MIN_OUTPUT_CHARS=1
EXPECT_CONTAINS="aggregated-answer"
EXPECT_LOG_MATCHES_REGEX="route=data
answer_contract_check"
CASE
CODRAX_BIN="$fake_multilog" EVAL_RESULTS_ROOT="$tmp/eval-results" bash eval/run.sh "$case_file" 1 >/dev/null 2>"$tmp/runner-multilog.err"
multilog_dir="$(find "$tmp/eval-results" -maxdepth 1 -type d -name 'runner_multilog-*' | sort | tail -1)"
if [[ -z "$multilog_dir" ]]; then
  fail "eval/run.sh did not write multilog result dir"
fi
assert_eq "$(cat "$multilog_dir/run-1.verdict")" "PASS" "multilog verdict should pass across aggregate logs"
if ! grep -q '^log_file=.*run-1.logs.all.log$' "$multilog_dir/run-1.metrics.txt"; then
  fail "metrics did not point at aggregate log"
fi
assert_eq "$(grep '^answer_contract_violations=' "$multilog_dir/run-1.metrics.txt" | cut -d= -f2)" "0" "aggregate log answer contract metric excludes advisory"
assert_eq "$(grep '^answer_contract_strict_violations=' "$multilog_dir/run-1.metrics.txt" | cut -d= -f2)" "0" "aggregate log strict answer contract metric"
assert_eq "$(grep '^answer_contract_advisories=' "$multilog_dir/run-1.metrics.txt" | cut -d= -f2)" "2" "aggregate log advisory answer contract metric"

fake_efficiency="$tmp/fake-codrax-efficiency-budget"
cat >"$fake_efficiency" <<'FAKE'
#!/usr/bin/env bash
set -euo pipefail
logdir=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --log-dir)
      logdir="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
mkdir -p "$logdir"
cat >"$logdir/codrax-20260608-000004-000-1.log" <<'LOG'
2026-06-08T00:00:04.000 DEBUG [diag explorer] DISPATCH stage=explore attempt=1
2026-06-08T00:00:04.001 DEBUG [diag explorer] iter=0 ASSISTANT content_len=12
2026-06-08T00:00:04.002 DEBUG [diag explorer] phase=toolcall tool=read_file params={"path":"internal/foo.go"}
2026-06-08T00:00:04.003 DEBUG [diag explorer] phase=midloop_inject key="explorer.mid-loop.read-without-emit"
LOG
printf 'working\n━━━\nanswer has enough content for the budget test\n'
FAKE
chmod +x "$fake_efficiency"
efficiency_case="$tmp/runner_efficiency_budget.case"
cat >"$efficiency_case" <<'CASE'
ID=runner_efficiency_budget
NAME="runner efficiency budget"
QUESTION="runner efficiency budget smoke"
MIN_OUTPUT_CHARS=1
MAX_TOOL_READ_FILE=0
ADVISORY_MAX_TOOL_READ_FILE=0
EXPECT_CONTAINS="answer has enough content"
CASE
CODRAX_BIN="$fake_efficiency" EVAL_RESULTS_ROOT="$tmp/eval-results" bash eval/run.sh "$efficiency_case" 1 >/dev/null 2>"$tmp/runner-efficiency.err"
efficiency_dir="$(find "$tmp/eval-results" -maxdepth 1 -type d -name 'runner_efficiency_budget-*' | sort | tail -1)"
if [[ -z "$efficiency_dir" ]]; then
  fail "eval/run.sh did not write efficiency-budget result dir"
fi
efficiency_verdict="$(cat "$efficiency_dir/run-1.verdict")"
case "$efficiency_verdict" in
  FAIL*perf_budget:tool_read_file:1\>0*)
    ;;
  *)
    fail "efficiency hard budget should fail from typed metric: $efficiency_verdict"
    ;;
esac
if ! grep -q '| 1 | high_source_reads | tool_read_file=1 limit=0 |' "$efficiency_dir/summary.md"; then
  fail "efficiency advisory summary missing source-read row"
fi

fake_write_apply="$tmp/fake-codrax-write-apply"
cat >"$fake_write_apply" <<'FAKE'
#!/usr/bin/env bash
set -euo pipefail
repo=""
plan_out=""
plan_file=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo)
      repo="$2"
      shift 2
      ;;
    --plan-out)
      plan_out="$2"
      shift 2
      ;;
    --plan-file)
      plan_file="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
if [[ -n "$plan_out" ]]; then
  cat >"$plan_out" <<'JSON'
{
  "id": "plan-fake-write-apply",
  "status": "ready",
  "changes": [
    {
      "path": "main.py",
      "kind": "patch"
    }
  ]
}
JSON
  printf 'planned\n'
  exit 0
fi
if [[ -n "$plan_file" ]]; then
  plan_dir="$(dirname "$plan_file")"
  if [[ "${FAKE_WRITE_CLEANUP:-0}" == "1" ]]; then
    worktree="$plan_dir/fake-worktree-discarded"
    sed 's/retrun/return/' "$repo/main.py" >"$repo/main.py.tmp"
    mv "$repo/main.py.tmp" "$repo/main.py"
    git -C "$repo" -c user.email=eval@codrax -c user.name=eval add main.py
    git -C "$repo" -c user.email=eval@codrax -c user.name=eval commit -q -m "fake applied"
    applied_sha="$(git -C "$repo" rev-parse HEAD)"
    git -C "$repo" update-ref refs/codrax/applied/plan-fake-write-apply "$applied_sha"
    git -C "$repo" reset --hard -q HEAD~1
  else
    worktree="$plan_dir/fake-worktree"
    mkdir -p "$worktree"
    sed 's/retrun/return/' "$repo/main.py" >"$worktree/main.py"
    applied_sha=""
  fi
  cat >"$plan_file" <<JSON
{
  "id": "plan-fake-write-apply",
  "status": "applied",
  "applied_commit_sha": "$applied_sha",
  "worktree_path": "$worktree",
  "changes": [
    {
      "path": "main.py",
      "kind": "patch"
    }
  ]
}
JSON
  if [[ "${FAKE_WRITE_REPORT:-1}" == "1" ]]; then
    cat >"$plan_dir/plan-fake-write-apply.report.json" <<'JSON'
{
  "plan_id": "plan-fake-write-apply",
  "channel": "post_apply_verify",
  "passed": true,
  "executed_commands": [
    {
      "runner": "make",
      "working_dir": ".",
      "command": "make check",
      "exit_code": 0,
      "source": "test_surface",
      "outcome": "executed"
    }
  ]
}
JSON
  fi
  printf 'applied\n'
  exit 0
fi
printf 'unsupported fake invocation\n' >&2
exit 2
FAKE
chmod +x "$fake_write_apply"

write_pass_case="$tmp/runner_write_apply_report_pass.case"
cat >"$write_pass_case" <<'CASE'
ID=runner_write_apply_report_pass
NAME="runner write apply report pass"
MODE=apply
FIXTURE="eval/fixtures/testdata/patch_typo_python"
QUESTION="fix typo"
POST_APPLY_FILE="main.py"
EXPECT_MATCHES_REGEX="return f"
CASE
FAKE_WRITE_REPORT=1 CODRAX_BIN="$fake_write_apply" EVAL_RESULTS_ROOT="$tmp/eval-results" bash eval/run.sh "$write_pass_case" 1 >/dev/null 2>"$tmp/runner-write-pass.err"
write_pass_dir="$(find "$tmp/eval-results" -maxdepth 1 -type d -name 'runner_write_apply_report_pass-*' | sort | tail -1)"
if [[ -z "$write_pass_dir" ]]; then
  fail "eval/run.sh did not write write-mode pass result dir"
fi
assert_eq "$(cat "$write_pass_dir/run-1.verdict")" "PASS" "write apply report pass verdict"
assert_eq "$(eval_json_top_bool_field "$write_pass_dir/run-1.write-apply.json" verify_authoritative)" "true" "write apply result authoritative in run.sh"

write_ref_case="$tmp/runner_write_apply_report_ref.case"
cat >"$write_ref_case" <<'CASE'
ID=runner_write_apply_report_ref
NAME="runner write apply report recovery ref"
MODE=apply
FIXTURE="eval/fixtures/testdata/patch_typo_python"
QUESTION="fix typo"
POST_APPLY_FILE="main.py"
EXPECT_MATCHES_REGEX="return f"
CASE
FAKE_WRITE_REPORT=1 FAKE_WRITE_CLEANUP=1 CODRAX_BIN="$fake_write_apply" EVAL_RESULTS_ROOT="$tmp/eval-results" bash eval/run.sh "$write_ref_case" 1 >/dev/null 2>"$tmp/runner-write-ref.err"
write_ref_dir="$(find "$tmp/eval-results" -maxdepth 1 -type d -name 'runner_write_apply_report_ref-*' | sort | tail -1)"
if [[ -z "$write_ref_dir" ]]; then
  fail "eval/run.sh did not write write-mode recovery-ref result dir"
fi
assert_eq "$(cat "$write_ref_dir/run-1.verdict")" "PASS" "write apply report pass should use recovery ref when worktree is gone"
assert_eq "$(eval_json_top_bool_field "$write_ref_dir/run-1.write-apply.json" worktree_exists)" "false" "write apply result records discarded worktree"
assert_eq "$(eval_json_top_bool_field "$write_ref_dir/run-1.write-apply.json" verify_authoritative)" "true" "write apply recovery-ref result authoritative"
if ! grep -q "Matched oracle lines" "$write_ref_dir/summary.md"; then
  fail "write apply summary should include matched oracle lines"
fi
if ! grep -q "return f" "$write_ref_dir/summary.md"; then
  fail "write apply summary should include matched post-apply source line"
fi
if ! grep -q "Applied diff hunk" "$write_ref_dir/summary.md"; then
  fail "write apply summary should include applied diff hunk from recovery ref"
fi
if ! grep -q "Post-apply verify commands" "$write_ref_dir/summary.md"; then
  fail "write apply summary should include post-apply verify command provenance"
fi
if ! grep -q "runner=make cwd=. exit=0 outcome=executed source=test_surface cmd=make check" "$write_ref_dir/summary.md"; then
  fail "write apply summary should include normalized executed command row"
fi

write_ref_tree_case="$tmp/runner_write_apply_report_ref_tree.case"
cat >"$write_ref_tree_case" <<'CASE'
ID=runner_write_apply_report_ref_tree
NAME="runner write apply report recovery ref tree"
MODE=apply
FIXTURE="eval/fixtures/testdata/patch_typo_python"
QUESTION="fix typo"
EXPECT_MATCHES_REGEX="return f"
CASE
FAKE_WRITE_REPORT=1 FAKE_WRITE_CLEANUP=1 CODRAX_BIN="$fake_write_apply" EVAL_RESULTS_ROOT="$tmp/eval-results" bash eval/run.sh "$write_ref_tree_case" 1 >/dev/null 2>"$tmp/runner-write-ref-tree.err"
write_ref_tree_dir="$(find "$tmp/eval-results" -maxdepth 1 -type d -name 'runner_write_apply_report_ref_tree-*' | sort | tail -1)"
if [[ -z "$write_ref_tree_dir" ]]; then
  fail "eval/run.sh did not write write-mode recovery-ref tree result dir"
fi
assert_eq "$(cat "$write_ref_tree_dir/run-1.verdict")" "PASS" "write apply report pass should aggregate materialized recovery ref tree when no POST_APPLY_FILE is set"

write_missing_case="$tmp/runner_write_apply_report_missing.case"
cat >"$write_missing_case" <<'CASE'
ID=runner_write_apply_report_missing
NAME="runner write apply report missing"
MODE=apply
FIXTURE="eval/fixtures/testdata/patch_typo_python"
QUESTION="fix typo"
POST_APPLY_FILE="main.py"
EXPECT_MATCHES_REGEX="return f"
CASE
FAKE_WRITE_REPORT=0 CODRAX_BIN="$fake_write_apply" EVAL_RESULTS_ROOT="$tmp/eval-results" bash eval/run.sh "$write_missing_case" 1 >/dev/null 2>"$tmp/runner-write-missing.err"
write_missing_dir="$(find "$tmp/eval-results" -maxdepth 1 -type d -name 'runner_write_apply_report_missing-*' | sort | tail -1)"
if [[ -z "$write_missing_dir" ]]; then
  fail "eval/run.sh did not write write-mode missing-report result dir"
fi
write_missing_verdict="$(cat "$write_missing_dir/run-1.verdict")"
case "$write_missing_verdict" in
  FAIL*write_report_missing*)
    ;;
  *)
    fail "write apply missing report should fail: $write_missing_verdict"
    ;;
esac
assert_eq "$(eval_json_top_bool_field "$write_missing_dir/run-1.write-apply.json" report_exists)" "false" "write apply missing report recorded"

fake_blocked_data="$tmp/fake-codrax-blocked-data"
cat >"$fake_blocked_data" <<'FAKE'
#!/usr/bin/env bash
set -euo pipefail
logdir=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --log-dir)
      logdir="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
mkdir -p "$logdir" "$PWD/.codrax/data-audit"
terminal="$PWD/.codrax/data-audit/fake-blocked-terminal.json"
cat >"$terminal" <<'JSON'
{
  "status": "blocked",
  "reason": "synthetic blocked data workflow",
  "data_rounds": 7,
  "repair_rounds": 3,
  "record_count": 5,
  "result_summary": "answer_len=8 decisions=5 rules=9 contributions=5 resolutions=23 consumed=15 warnings=0 reconcile=\"pass\"",
  "action_events": [
    {"status": "executed"},
    {"status": "failed"}
  ],
  "action_graph": {}
}
JSON
cat >"$logdir/codrax-20260608-000003-000-1.log" <<LOG
2026-06-08T00:00:03.000 INFO [cmd/route] single-shot turn policy raw_route=data route=data source=repo
2026-06-08T00:00:03.001 INFO [cli/data] data task result contributions=2 reconcile=pass
2026-06-08T00:00:03.002 INFO [cli/data] terminal full path=$terminal
LOG
printf 'working\n━━━\n数据处理已阻止。正确值应该是 17,0,5\n'
FAKE
chmod +x "$fake_blocked_data"
blocked_case="$tmp/runner_blocked_data.case"
cat >"$blocked_case" <<'CASE'
ID=runner_blocked_data
NAME="runner blocked data"
QUESTION="runner blocked data smoke"
DATA_FIXTURE="data-multifile-reference"
MIN_OUTPUT_CHARS=1
EXPECT_MATCHES_REGEX="(^|[^0-9])17[[:space:]]*,[[:space:]]*0[[:space:]]*,[[:space:]]*5([^0-9]|$)"
EXPECT_LOG_MATCHES_REGEX="route=data
\[cli/data\] data task result.*contributions=[1-9].*reconcile=pass
\[cli/data\] terminal full path=.*terminal\.json"
CASE
CODRAX_BIN="$fake_blocked_data" EVAL_RESULTS_ROOT="$tmp/eval-results" bash eval/run.sh "$blocked_case" 1 >/dev/null 2>"$tmp/runner-blocked-data.err"
blocked_dir="$(find "$tmp/eval-results" -maxdepth 1 -type d -name 'runner_blocked_data-*' | sort | tail -1)"
if [[ -z "$blocked_dir" ]]; then
  fail "eval/run.sh did not write blocked data result dir"
fi
blocked_verdict="$(cat "$blocked_dir/run-1.verdict")"
case "$blocked_verdict" in
  FAIL*data_terminal_status:blocked*)
    ;;
  *)
    fail "blocked terminal should fail even when stdout contains expected regex: $blocked_verdict"
    ;;
esac
assert_eq "$(grep '^data_terminal_status=' "$blocked_dir/run-1.metrics.txt" | cut -d= -f2)" "blocked" "data terminal status metric"
assert_eq "$(grep '^data_rounds=' "$blocked_dir/run-1.metrics.txt" | cut -d= -f2)" "7" "data rounds metric"
assert_eq "$(grep '^data_repair_rounds=' "$blocked_dir/run-1.metrics.txt" | cut -d= -f2)" "3" "data repair rounds metric"
assert_eq "$(grep '^data_record_count=' "$blocked_dir/run-1.metrics.txt" | cut -d= -f2)" "5" "data record count metric"
assert_eq "$(grep '^data_action_failed=' "$blocked_dir/run-1.metrics.txt" | cut -d= -f2)" "1" "data action failed metric"
assert_eq "$(grep '^data_answer_len=' "$blocked_dir/run-1.metrics.txt" | cut -d= -f2)" "8" "data answer length metric"

scenario_dir="$tmp/data-real-scenario"
mkdir -p "$scenario_dir"
fake_codrax="$tmp/fake-codrax-data-gate"
cat >"$fake_codrax" <<'FAKE'
#!/usr/bin/env bash
set -euo pipefail
mkdir -p "$PWD/.codrax/data-audit"
terminal="$PWD/.codrax/data-audit/fake-terminal.json"
cat >"$terminal" <<'JSON'
{
  "status": "completed",
  "action_graph": {},
  "artifact_graph": {},
  "progress": {},
  "decision": {},
  "process_events": [],
  "resume": {"records": []}
}
JSON
echo "[cli/data] data task result contributions=2 reconcile=pass" >&2
echo "[cli/data] terminal full path=$terminal" >&2
printf '42\n'
FAKE
chmod +x "$fake_codrax"
gate_out="$(
  DATA_REAL_SCENARIO_DIR="$scenario_dir" \
    DATA_REAL_SCENARIO_REQUEST="gate smoke" \
    DATA_REAL_SCENARIO_EXPECTED="42" \
    DATA_REAL_SCENARIO_RUNS=2 \
    CODRAX_BIN="$fake_codrax" \
    bash eval/data_real_scenario_gate.sh 2>"$tmp/data-real-gate.err"
)"
assert_eq "$gate_out" "42" "data real scenario gate stable stdout"
if ! grep -q 'PASS runs=2' "$tmp/data-real-gate.err"; then
  fail "data real scenario gate did not report multi-run pass"
fi

volatile_count="$tmp/fake-codrax-volatile-count"
fake_volatile="$tmp/fake-codrax-data-gate-volatile"
cat >"$fake_volatile" <<FAKE
#!/usr/bin/env bash
set -euo pipefail
count_file="$volatile_count"
count=0
if [[ -f "\$count_file" ]]; then
  count="\$(cat "\$count_file")"
fi
count=\$((count + 1))
printf '%s' "\$count" >"\$count_file"
mkdir -p "\$PWD/.codrax/data-audit"
terminal="\$PWD/.codrax/data-audit/fake-terminal-\$count.json"
cat >"\$terminal" <<'JSON'
{
  "status": "completed",
  "action_graph": {},
  "artifact_graph": {},
  "progress": {},
  "decision": {},
  "process_events": [],
  "resume": {"records": []}
}
JSON
echo "[cli/data] data task result contributions=2 reconcile=pass" >&2
echo "[cli/data] terminal full path=\$terminal" >&2
printf '%s\n' "\$count"
FAKE
chmod +x "$fake_volatile"
DATA_REAL_SCENARIO_DIR="$scenario_dir" \
  DATA_REAL_SCENARIO_REQUEST="gate volatility smoke" \
  DATA_REAL_SCENARIO_RUNS=2 \
  CODRAX_BIN="$fake_volatile" \
  bash eval/data_real_scenario_gate.sh >"$tmp/data-real-gate-volatile.out" 2>"$tmp/data-real-gate-volatile.err"
rc=$?
if [[ "$rc" -eq 0 ]]; then
  fail "data real scenario gate should fail volatile answers"
fi
if ! grep -q 'answer volatility detected' "$tmp/data-real-gate-volatile.err"; then
  fail "data real scenario gate volatility failure was not explained"
fi

echo "ok eval runner contracts"
