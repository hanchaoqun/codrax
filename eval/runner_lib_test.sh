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

printf 'analyzer_dispatches=2\nnul_metric=7\000\nfinalizer_dispatches=5\n' >"$tmp/metrics.txt"
assert_eq "$(eval_metric_field "$tmp/metrics.txt" analyzer_dispatches)" "2" "metric field parse"
assert_eq "$(eval_metric_field "$tmp/metrics.txt" nul_metric)" "7" "metric field parse with NUL"
assert_eq "$(eval_metric_field "$tmp/metrics.txt" missing_key)" "-" "missing metric field"

printf 'one\nreject\000\nreject\n' >"$tmp/log.txt"
assert_eq "$(eval_count_pattern 'reject' "$tmp/log.txt")" "2" "pattern count"
assert_eq "$(eval_count_pattern 'reject$' "$tmp/log.txt")" "2" "pattern count with NUL line"
assert_eq "$(eval_count_pattern 'reject' "$tmp/missing.log")" "0" "missing log pattern count"

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
assert_eq "$(eval_sum_answer_contract_violations_for_section "$tmp/finalizer-control.log" lane_block_kind)" "1" "answer contract section violation sum"
assert_eq "$(eval_count_agent_iterations "$tmp/finalizer-control.log" explorer)" "1" "agent iteration control count"
assert_eq "$(eval_count_agent_dispatches "$tmp/finalizer-control.log" explorer)" "1" "agent dispatch control count"
assert_eq "$(eval_count_semantic_quality_dispatches "$tmp/finalizer-control.log")" "1" "semantic dispatch control count"
assert_eq "$(eval_count_semantic_quality_concerns "$tmp/finalizer-control.log")" "2" "semantic concern control count"
assert_eq "$(eval_count_self_consistency_concerns "$tmp/finalizer-control.log")" "2" "self consistency control count"

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
assert_eq "$(eval_sum_answer_contract_violations_for_section "$tmp/finalizer-content-only.log" lane_block_kind)" "0" "answer contract section content contamination"
assert_eq "$(eval_count_agent_iterations "$tmp/finalizer-content-only.log" explorer)" "0" "agent iteration content contamination"
assert_eq "$(eval_count_agent_dispatches "$tmp/finalizer-content-only.log" explorer)" "0" "agent dispatch content contamination"
assert_eq "$(eval_count_semantic_quality_dispatches "$tmp/finalizer-content-only.log")" "0" "semantic dispatch content contamination"
assert_eq "$(eval_count_semantic_quality_concerns "$tmp/finalizer-content-only.log")" "0" "semantic concern content contamination"
assert_eq "$(eval_count_self_consistency_concerns "$tmp/finalizer-content-only.log")" "0" "self consistency content contamination"

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

true_bin="$(command -v true || true)"
if [[ -n "$true_bin" && -x "$true_bin" ]]; then
  case_file="$tmp/runner_contract.case"
  cat >"$case_file" <<'CASE'
ID=runner_contract
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

echo "ok eval runner contracts"
