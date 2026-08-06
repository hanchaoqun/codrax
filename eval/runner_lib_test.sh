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

cat >"$tmp/repair-metrics.txt" <<'METRICS'
repair_plan_lines=2
data_repair_rounds=3
METRICS
assert_eq "$(eval_total_repair_rounds "$tmp/repair-metrics.txt")" "5" "repair summary combines read and data typed counters"
printf 'data_repair_rounds=4\n' >"$tmp/data-only-repair-metrics.txt"
assert_eq "$(eval_total_repair_rounds "$tmp/data-only-repair-metrics.txt")" "4" "data-only repair must not render as zero"

cat >"$tmp/data-terminal.json" <<'JSON'
{
  "status": "complete",
  "result_summary": "answer_len=2 decisions=5 reconcile=\"pass\"",
  "action_events": [
    {"Status": "failed"},
    {"status": "failed"},
    {"Status": "executed"}
  ],
  "action_graph": {}
}
JSON
if command -v python3 >/dev/null 2>&1; then
  assert_eq "$(eval_json_top_string_field "$tmp/data-terminal.json" result_summary)" 'answer_len=2 decisions=5 reconcile="pass"' "json escaped string field parse"
fi
assert_eq "$(eval_data_terminal_action_failed_count "$tmp/data-terminal.json")" "2" "data terminal failed action count accepts serialized field casing"

cat >"$tmp/convergence-lossless-repair.metrics" <<'METRICS'
tool_read_file=9
tool_repo_map=0
tool_list_files=0
explorer_iters=7
explorer_dispatches=1
semantic_quality_concerns=0
finalizer_iters=1
finalizer_rejects=0
finalizer_rewrites=0
mermaid_source_repair_applied=2
answer_contract_violations=0
answer_contract_strict_violations=0
tool_history_prunes=0
mixed_origin_autocomplete_blocks=0
METRICS
assert_eq "$(eval_convergence_flags "$tmp/convergence-lossless-repair.metrics" PASS)" "—" "lossless deterministic repair should not flag convergence"

cat >"$tmp/convergence-repair-churn.metrics" <<'METRICS'
tool_read_file=9
tool_repo_map=0
tool_list_files=0
explorer_iters=7
explorer_dispatches=1
semantic_quality_concerns=0
finalizer_iters=2
finalizer_rejects=1
finalizer_rewrites=0
mermaid_source_repair_applied=2
answer_contract_violations=0
answer_contract_strict_violations=0
tool_history_prunes=0
mixed_origin_autocomplete_blocks=0
METRICS
assert_eq "$(eval_convergence_flags "$tmp/convergence-repair-churn.metrics" PASS)" "finalizer repair_churn" "repair with finalizer churn should be flagged precisely"

cat >"$tmp/convergence-adaptive-loop.metrics" <<'METRICS'
tool_read_file=1
tool_repo_map=1
tool_list_files=0
explorer_iters=6
explorer_dispatches=1
semantic_quality_concerns=0
finalizer_iters=1
finalizer_rejects=0
finalizer_rewrites=0
mermaid_source_repair_applied=0
answer_contract_violations=0
answer_contract_strict_violations=0
tool_history_prunes=0
mixed_origin_autocomplete_blocks=0
analyze_refine_dispatches=2
read_loop_add_proof_consumed=0
METRICS
assert_eq "$(eval_convergence_flags "$tmp/convergence-adaptive-loop.metrics" PASS)" "adaptive_loop" "repeated adaptive read loops should be flagged"

cat >"$tmp/convergence-contract-repaired.metrics" <<'METRICS'
tool_read_file=1
tool_repo_map=1
tool_list_files=0
explorer_iters=6
explorer_dispatches=1
semantic_quality_concerns=0
finalizer_iters=1
finalizer_rejects=0
finalizer_rewrites=0
mermaid_source_repair_applied=0
answer_contract_violations=2
answer_contract_strict_violations=2
answer_contract_first_pass_strict_violations=2
answer_contract_final_strict_violations=0
answer_contract_auto_repaired_strict_violations=2
tool_history_prunes=0
mixed_origin_autocomplete_blocks=0
analyze_refine_dispatches=0
read_loop_add_proof_consumed=0
METRICS
assert_eq "$(eval_convergence_flags "$tmp/convergence-contract-repaired.metrics" PASS)" "—" "auto-repaired contract strict findings should not flag final convergence"

cat >"$tmp/convergence-contract-final.metrics" <<'METRICS'
tool_read_file=1
tool_repo_map=1
tool_list_files=0
explorer_iters=6
explorer_dispatches=1
semantic_quality_concerns=0
finalizer_iters=1
finalizer_rejects=0
finalizer_rewrites=0
mermaid_source_repair_applied=0
answer_contract_violations=2
answer_contract_strict_violations=2
answer_contract_first_pass_strict_violations=2
answer_contract_final_strict_violations=1
answer_contract_auto_repaired_strict_violations=1
tool_history_prunes=0
mixed_origin_autocomplete_blocks=0
analyze_refine_dispatches=0
read_loop_add_proof_consumed=0
METRICS
assert_eq "$(eval_convergence_flags "$tmp/convergence-contract-final.metrics" PASS)" "contract_warning" "remaining final contract strict findings should still flag convergence"

metric_row="$(eval_print_efficiency_advisory_row "$tmp/metrics.txt" 1 high_analyzer_dispatches analyzer_dispatches 1 || true)"
assert_eq "$metric_row" "| 1 | high_analyzer_dispatches | analyzer_dispatches=2 limit=1 |" "metric advisory row"
assert_eq "$(eval_print_efficiency_advisory_row "$tmp/metrics.txt" 1 high_analyzer_dispatches analyzer_dispatches 2 || true)" "" "metric advisory row under limit"
assert_eq "$(eval_metric_budget_reasons "$tmp/metrics.txt" analyzer_dispatches 1 finalizer_dispatches 10)" "perf_budget:analyzer_dispatches:2>1" "metric budget reason"

cat >"$tmp/oracle-surface.case" <<'CASE'
ID="oracle_surface"
MODE="apply"
QUESTION="oracle surface smoke"
HTRACE="# tracer"
EXPECT_CONTAINS="answer"
EXPECT_DIMENSIONS="package"
EXPECT_INVENTORY_ROWSETS="package"
EXPECT_LOG_MATCHES_REGEX="phase=toolcall .*tool=trace_query"
MAX_TOOL_READ_FILE=4
ADVISORY_MAX_MIDLOOP_INJECT=2
POST_APPLY_FILE="src/main.py"
CASE
assert_eq "$(eval_case_oracle_surface "$tmp/oracle-surface.case")" "typed_inventory_rowset,dimension_substring,log_regex,trace_attachment,write_apply,write_patch_oracle,answer_contains,metric_hard_budget,metric_advisory_budget" "case oracle surface classification"

cat >"$tmp/oracle-read-regex.case" <<'CASE'
ID="oracle_read_regex"
QUESTION="read regex smoke"
EXPECT_MATCHES_REGEX="foo"
CASE
assert_eq "$(eval_case_oracle_surface "$tmp/oracle-read-regex.case")" "answer_regex" "read EXPECT_MATCHES_REGEX should be answer regex oracle"

cat >"$tmp/oracle-principal.case" <<'CASE'
ID="oracle_principal"
QUESTION="principal answer smoke"
EXPECT_PRINCIPAL_MATCHES_REGEX="foo"
CASE
assert_eq "$(eval_case_oracle_surface "$tmp/oracle-principal.case")" "principal_answer" "principal answer oracle surface classification"

cat >"$tmp/oracle-primary.case" <<'CASE'
ID="oracle_primary"
QUESTION="primary oracle smoke"
EXPECT_PRIMARY_MATCHES_REGEX="foo"
CASE
assert_eq "$(eval_case_oracle_surface "$tmp/oracle-primary.case")" "primary_answer" "primary answer oracle surface classification"

cat >"$tmp/oracle-write-post-apply-regex.case" <<'CASE'
ID="oracle_write_post_apply_regex"
MODE="apply"
QUESTION="write post apply regex smoke"
POST_APPLY_FILE="src/main.py"
EXPECT_MATCHES_REGEX="return foo"
CASE
assert_eq "$(eval_case_oracle_surface "$tmp/oracle-write-post-apply-regex.case")" "write_apply,write_patch_oracle" "post-apply EXPECT_MATCHES_REGEX should stay write patch oracle only"

cat >"$tmp/oracle-basic.case" <<'CASE'
ID="oracle_basic"
QUESTION="basic smoke"
CASE
assert_eq "$(eval_case_oracle_surface "$tmp/oracle-basic.case")" "basic_output" "case oracle surface basic fallback"
assert_eq "$(eval_case_oracle_surface "$tmp/missing.case")" "unknown" "case oracle surface missing file"

# EVAL-B1-R11/E1: full-answer oracles may inspect deterministic supplements,
# while principal-answer oracles must stop before the system-authored trace
# projection. A correct footer cannot mask a wrong principal answer.
cat >"$tmp/fake-codrax-principal-scope" <<'SH'
#!/usr/bin/env bash
echo 'thinking stream contains correct-footer but is before the separator'
echo '━━━'
echo 'required-principal first row'
echo 'second principal row'
echo '## Trace 因果投影'
echo 'correct-footer forbidden-footer'
SH
chmod +x "$tmp/fake-codrax-principal-scope"
cat >"$tmp/principal-scope-pass.case" <<'CASE'
ID="principal_scope_pass"
NAME="principal scope pass"
QUESTION="principal scope test"
MIN_OUTPUT_CHARS=1
EXPECT_CONTAINS="correct-footer"
EXPECT_PRINCIPAL_CONTAINS="required-principal"
EXPECT_PRINCIPAL_NOT_CONTAINS="forbidden-footer"
EXPECT_PRINCIPAL_MATCHES_REGEX="^required-principal"
EXPECT_PRINCIPAL_MATCHES_TEXT_REGEX="required-principal +first row +second principal row"
CASE
CODRAX_BIN="$tmp/fake-codrax-principal-scope" EVAL_RESULTS_ROOT="$tmp/principal-results" CODRAX_PROVIDER_ARGS_RAW="" \
  eval/run.sh "$tmp/principal-scope-pass.case" 1 >/dev/null || fail "principal scope pass eval failed to run"
principal_pass_dir="$(eval_latest_result_dir "$tmp/principal-results" principal_scope_pass 00000000-000000 || true)"
[[ -n "$principal_pass_dir" ]] || fail "principal scope pass result dir missing"
assert_eq "$(cat "$principal_pass_dir/run-1.verdict")" "PASS" "principal scope should exclude deterministic footer"

cat >"$tmp/principal-scope-fail.case" <<'CASE'
ID="principal_scope_fail"
NAME="principal scope fail"
QUESTION="principal scope test"
MIN_OUTPUT_CHARS=1
EXPECT_CONTAINS="correct-footer"
EXPECT_PRINCIPAL_MATCHES_REGEX="correct-footer"
CASE
CODRAX_BIN="$tmp/fake-codrax-principal-scope" EVAL_RESULTS_ROOT="$tmp/principal-results" CODRAX_PROVIDER_ARGS_RAW="" \
  eval/run.sh "$tmp/principal-scope-fail.case" 1 >/dev/null || fail "principal scope fail eval failed to run"
principal_fail_dir="$(eval_latest_result_dir "$tmp/principal-results" principal_scope_fail 00000000-000000 || true)"
[[ -n "$principal_fail_dir" ]] || fail "principal scope fail result dir missing"
case "$(cat "$principal_fail_dir/run-1.verdict")" in
  "FAIL no_principal_regex_match:correct-footer")
    ;;
  *)
    fail "footer-only witness must not satisfy principal oracle, got: $(cat "$principal_fail_dir/run-1.verdict")"
    ;;
esac

# EVAL-B51-ORACLE1: terminal primary assertions must not be satisfied by
# pre-separator progress, renderer-owned citations, or raw fallback reasoning.
cat >"$tmp/fake-codrax-primary-scope" <<'SH'
#!/usr/bin/env bash
echo 'draft-only-symbol before terminal separator'
echo '━━━'
echo 'primary-hop-A is the terminal conclusion'
echo '**引用**：'
echo 'citation-only-symbol'
echo '**模型最后一轮原文：**'
echo 'raw-fallback-symbol'
SH
chmod +x "$tmp/fake-codrax-primary-scope"
cat >"$tmp/primary-scope-pass.case" <<'CASE'
ID="primary_scope_pass"
NAME="primary scope pass"
QUESTION="primary scope test"
MIN_OUTPUT_CHARS=1
EXPECT_CONTAINS="citation-only-symbol"
EXPECT_PRIMARY_CONTAINS="primary-hop-A"
EXPECT_PRIMARY_NOT_CONTAINS="draft-only-symbol citation-only-symbol raw-fallback-symbol"
EXPECT_PRIMARY_MATCHES_REGEX="^primary-hop-A"
CASE
CODRAX_BIN="$tmp/fake-codrax-primary-scope" EVAL_RESULTS_ROOT="$tmp/primary-results" CODRAX_PROVIDER_ARGS_RAW="" \
  eval/run.sh "$tmp/primary-scope-pass.case" 1 >/dev/null || fail "primary scope pass eval failed to run"
primary_pass_dir="$(eval_latest_result_dir "$tmp/primary-results" primary_scope_pass 00000000-000000 || true)"
[[ -n "$primary_pass_dir" ]] || fail "primary scope pass result dir missing"
assert_eq "$(cat "$primary_pass_dir/run-1.verdict")" "PASS" "primary scope should exclude non-conclusion surfaces"

cat >"$tmp/primary-scope-fail.case" <<'CASE'
ID="primary_scope_fail"
NAME="primary scope fail"
QUESTION="primary scope test"
MIN_OUTPUT_CHARS=1
EXPECT_PRIMARY_MATCHES_REGEX="citation-only-symbol"
CASE
CODRAX_BIN="$tmp/fake-codrax-primary-scope" EVAL_RESULTS_ROOT="$tmp/primary-results" CODRAX_PROVIDER_ARGS_RAW="" \
  eval/run.sh "$tmp/primary-scope-fail.case" 1 >/dev/null || fail "primary scope fail eval failed to run"
primary_fail_dir="$(eval_latest_result_dir "$tmp/primary-results" primary_scope_fail 00000000-000000 || true)"
[[ -n "$primary_fail_dir" ]] || fail "primary scope fail result dir missing"
case "$(cat "$primary_fail_dir/run-1.verdict")" in
  "FAIL no_primary_regex_match:citation-only-symbol")
    ;;
  *)
    fail "citation-only witness must not satisfy primary oracle, got: $(cat "$primary_fail_dir/run-1.verdict")"
    ;;
esac

# Snippets and recovery panels are renderer-owned tail domains in their own
# right. They must stay excluded even when no citation footer precedes them.
cat >"$tmp/fake-codrax-primary-tail-domains" <<'SH'
#!/usr/bin/env bash
echo '━━━'
echo 'primary-only-symbol is the conclusion'
echo '**关键代码**：'
echo 'snippet-only-symbol'
echo '> **系统保留内容**'
echo 'recovery-only-symbol'
SH
chmod +x "$tmp/fake-codrax-primary-tail-domains"
cat >"$tmp/primary-tail-domains.case" <<'CASE'
ID="primary_tail_domains"
NAME="primary tail domains"
QUESTION="primary tail scope test"
MIN_OUTPUT_CHARS=1
EXPECT_PRIMARY_CONTAINS="primary-only-symbol"
EXPECT_PRIMARY_NOT_CONTAINS="snippet-only-symbol recovery-only-symbol"
CASE
CODRAX_BIN="$tmp/fake-codrax-primary-tail-domains" EVAL_RESULTS_ROOT="$tmp/primary-results" CODRAX_PROVIDER_ARGS_RAW="" \
  eval/run.sh "$tmp/primary-tail-domains.case" 1 >/dev/null || fail "primary tail domains eval failed to run"
primary_tail_dir="$(eval_latest_result_dir "$tmp/primary-results" primary_tail_domains 00000000-000000 || true)"
[[ -n "$primary_tail_dir" ]] || fail "primary tail domains result dir missing"
assert_eq "$(cat "$primary_tail_dir/run-1.verdict")" "PASS" "primary scope should independently exclude snippet and recovery tails"

# EVAL-B1-E2: a Markdown table may carry a unit once in its column header.
# The oracle composition keeps the header and every exact row mandatory while
# avoiding a false failure merely because cells do not repeat the unit.
cat >"$tmp/fake-codrax-principal-table" <<'SH'
#!/usr/bin/env bash
echo '━━━'
echo '| start | duration（ms） |'
echo '| 1.001 | 0.138 |'
echo '| 1.002 | 0.147 |'
echo '2 rows total 0.285 ms'
SH
chmod +x "$tmp/fake-codrax-principal-table"
cat >"$tmp/principal-table-pass.case" <<'CASE'
ID="principal_table_pass"
NAME="principal table unit inheritance"
QUESTION="principal table test"
MIN_OUTPUT_CHARS=1
EXPECT_PRINCIPAL_CONTAINS="duration（ms）"
EXPECT_PRINCIPAL_MATCHES_REGEX="1\\.001.*0\\.138
1\\.002.*0\\.147"
EXPECT_PRINCIPAL_MATCHES_TEXT_REGEX="duration（ms）.*1\\.001.*0\\.138.*1\\.002.*0\\.147.*2 rows.*0\\.285 *ms"
CASE
CODRAX_BIN="$tmp/fake-codrax-principal-table" EVAL_RESULTS_ROOT="$tmp/principal-results" CODRAX_PROVIDER_ARGS_RAW="" \
  eval/run.sh "$tmp/principal-table-pass.case" 1 >/dev/null || fail "principal table unit inheritance eval failed to run"
principal_table_dir="$(eval_latest_result_dir "$tmp/principal-results" principal_table_pass 00000000-000000 || true)"
[[ -n "$principal_table_dir" ]] || fail "principal table result dir missing"
assert_eq "$(cat "$principal_table_dir/run-1.verdict")" "PASS" "table header unit should authorize unitless cells"

# EVAL-B1-E2 follow-up: a list is equivalently authoritative when every row
# carries its own unit. The same closed oracle must still reject an entirely
# unitless list; this is presentation equivalence, not a weaker fact bar.
cat >"$tmp/principal-unit-shapes.case" <<'CASE'
ID="principal_unit_shapes"
NAME="principal unit presentation shapes"
QUESTION="principal unit shapes test"
MIN_OUTPUT_CHARS=1
EXPECT_PRINCIPAL_MATCHES_REGEX="1\\.001.*0\\.138
1\\.002.*0\\.147"
EXPECT_PRINCIPAL_MATCHES_TEXT_REGEX="((duration（ms）.*1\\.001.*0\\.138.*1\\.002.*0\\.147)|(1\\.001.*0\\.138 ?(ms|milliseconds).*1\\.002.*0\\.147 ?(ms|milliseconds))).*2 rows.*0\\.285 *ms"
CASE

cat >"$tmp/fake-codrax-principal-unit-list" <<'SH'
#!/usr/bin/env bash
echo '━━━'
echo '- 1.001: 0.138 ms'
echo '- 1.002: 0.147 milliseconds'
echo '2 rows total 0.285 ms'
SH
chmod +x "$tmp/fake-codrax-principal-unit-list"
CODRAX_BIN="$tmp/fake-codrax-principal-unit-list" EVAL_RESULTS_ROOT="$tmp/principal-results" CODRAX_PROVIDER_ARGS_RAW="" \
  eval/run.sh "$tmp/principal-unit-shapes.case" 1 >/dev/null || fail "principal per-row unit eval failed to run"
principal_unit_list_dir="$(eval_latest_result_dir "$tmp/principal-results" principal_unit_shapes 00000000-000000 || true)"
[[ -n "$principal_unit_list_dir" ]] || fail "principal per-row unit result dir missing"
assert_eq "$(cat "$principal_unit_list_dir/run-1.verdict")" "PASS" "per-row units should authorize list values"

cat >"$tmp/fake-codrax-principal-unitless-list" <<'SH'
#!/usr/bin/env bash
echo '━━━'
echo '- 1.001: 0.138'
echo '- 1.002: 0.147'
echo '2 rows total 0.285 ms'
SH
chmod +x "$tmp/fake-codrax-principal-unitless-list"
CODRAX_BIN="$tmp/fake-codrax-principal-unitless-list" EVAL_RESULTS_ROOT="$tmp/principal-results" CODRAX_PROVIDER_ARGS_RAW="" \
  eval/run.sh "$tmp/principal-unit-shapes.case" 1 >/dev/null || fail "principal unitless list eval failed to run"
principal_unitless_list_dir="$(eval_latest_result_dir "$tmp/principal-results" principal_unit_shapes 00000000-000000 || true)"
[[ -n "$principal_unitless_list_dir" ]] || fail "principal unitless list result dir missing"
case "$(cat "$principal_unitless_list_dir/run-1.verdict")" in
  "FAIL no_principal_text_regex_match:"*)
    ;;
  *)
    fail "unitless list must not satisfy the closed unit oracle, got: $(cat "$principal_unitless_list_dir/run-1.verdict")"
    ;;
esac

printf 'one\nreject\000\nreject\n' >"$tmp/log.txt"
assert_eq "$(eval_count_pattern 'reject' "$tmp/log.txt")" "2" "pattern count"
assert_eq "$(eval_count_pattern 'reject$' "$tmp/log.txt")" "2" "pattern count with NUL line"
assert_eq "$(eval_count_pattern 'reject' "$tmp/missing.log")" "0" "missing log pattern count"

cat >"$tmp/mermaid-repair-dedupe.log" <<'LOG'
2026-05-24T00:00:00.001 DEBUG [mermaidcompat] source repair applied repair_hash=aaa before_bytes=100 after_bytes=120
2026-05-24T00:00:00.002 DEBUG [mermaidcompat] source repair applied repair_hash=aaa before_bytes=100 after_bytes=120
2026-05-24T00:00:00.003 DEBUG [mermaidcompat] source repair applied repair_hash=bbb before_bytes=90 after_bytes=110
2026-05-24T00:00:00.004 DEBUG [mermaidcompat] source repair applied before_bytes=10 after_bytes=20
LOG
assert_eq "$(eval_count_mermaid_source_repairs "$tmp/mermaid-repair-dedupe.log")" "3" "mermaid repair hash dedupe count"

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
cat >"$tmp/write-eval/plan-write-eval-pass.final.json" <<'JSON'
{
  "kind": "final_report",
  "run_status": "complete",
  "completion": {"verdict": "verified", "reason_code": "all_batches_verified"},
  "plan": {"id": "plan-write-eval-pass"}
}
JSON
eval_write_apply_result_record "$tmp/write-eval/pass.json" "$tmp/write-eval/run-1.plan.json" "$tmp/write-eval" "$tmp/write-scratch" 1 1 0
assert_eq "$(eval_json_top_string_field "$tmp/write-eval/pass.json" plan_id)" "plan-write-eval-pass" "write apply result plan id"
assert_eq "$(eval_json_top_bool_field "$tmp/write-eval/pass.json" report_exists)" "true" "write apply result report exists"
assert_eq "$(eval_json_top_bool_field "$tmp/write-eval/pass.json" report_passed)" "true" "write apply result report passed"
assert_eq "$(eval_json_top_bool_field "$tmp/write-eval/pass.json" final_report_exists)" "true" "write apply final report exists"
assert_eq "$(eval_json_top_string_field "$tmp/write-eval/pass.json" final_verdict)" "verified" "write apply final verdict"
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

cat >"$tmp/write-eval/run-5.plan.json" <<JSON
{
  "id": "plan-write-eval-unverified",
  "status": "applied",
  "worktree_path": "$tmp/write-worktree"
}
JSON
cat >"$tmp/write-eval/plan-write-eval-unverified.report.json" <<'JSON'
{
  "plan_id": "plan-write-eval-unverified",
  "channel": "post_apply_verify",
  "passed": true
}
JSON
cat >"$tmp/write-eval/plan-write-eval-unverified.final.json" <<'JSON'
{
  "kind": "final_report",
  "run_status": "complete",
  "completion": {"verdict": "unverified", "reason_code": "verification_proof_incomplete"},
  "plan": {"id": "plan-write-eval-unverified"}
}
JSON
eval_write_apply_result_record "$tmp/write-eval/unverified.json" "$tmp/write-eval/run-5.plan.json" "$tmp/write-eval" "$tmp/write-scratch" 1 1 0
assert_eq "$(eval_json_top_bool_field "$tmp/write-eval/unverified.json" report_passed)" "true" "write apply unverified keeps local report pass"
assert_eq "$(eval_json_top_string_field "$tmp/write-eval/unverified.json" final_verdict)" "unverified" "write apply records final unverified verdict"
assert_eq "$(eval_json_top_string_field "$tmp/write-eval/unverified.json" final_reason_code)" "verification_proof_incomplete" "write apply records final reason"
assert_eq "$(eval_json_top_bool_field "$tmp/write-eval/unverified.json" verify_authoritative)" "false" "write apply final unverified is not authoritative"

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
2026-05-24T00:00:00.000 DEBUG [diag finalizer] iter=0 ASSISTANT content: quoted WARN [orchestrator] finalizer returned degraded answer; skipping structured answer checks reason=answer_document_retry_state_recovered
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
2026-05-24T00:00:00.068 DEBUG [diag orchestrator] phase=read_dag_dispatch stage=explore reason=read_dag_explore_window key="n_analyze_refine" kind=probe surface=default node_ids=n_analyze_refine criteria=progress_replan_required analyze_refine=true
2026-05-24T00:00:00.068 DEBUG [diag orchestrator] phase=read_loop_next_action_selected action=add_proof reason=proof_weak proof_state=weak truth_action=add_proof route_surface=verify route_reason=loop_tool_route_verification policy_active=true policy_tools=run_tests,repo_map,read_file trigger=retry
2026-05-24T00:00:00.068 DEBUG [diag orchestrator] phase=read_loop_next_action_consumed action=add_proof reason=proof_weak proof_state=weak truth_action=add_proof route_surface=verify route_reason=loop_tool_route_verification policy_active=true policy_tools=run_tests,repo_map,read_file
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
2026-05-24T00:00:00.110 WARN [orchestrator] finalizer returned degraded answer; skipping structured answer checks reason=answer_document_retry_state_recovered
LOG
assert_eq "$(eval_count_finalizer_rejects "$tmp/finalizer-control.log")" "1" "finalizer reject mirror dedupe count"
assert_eq "$(eval_count_degraded_read_answer_check_skips "$tmp/finalizer-control.log")" "1" "degraded answer check-skip count excludes quoted model prose"

cat >"$tmp/fake-codrax-degraded-read" <<'SH'
#!/usr/bin/env bash
logdir=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --log-dir)
      logdir="${2:-}"
      shift 2
      ;;
    --log-dir=*)
      logdir="${1#*=}"
      shift
      ;;
    *)
      shift
      ;;
  esac
done
if [[ -n "$logdir" ]]; then
  mkdir -p "$logdir"
  echo '2026-05-24T00:00:00.110 WARN [orchestrator] finalizer returned degraded answer; skipping structured answer checks reason=answer_document_retry_state_recovered' >"$logdir/codrax-fake.log"
fi
echo '━━━'
echo 'answer contains expected-symbol but did not pass structured checks'
SH
chmod +x "$tmp/fake-codrax-degraded-read"
cat >"$tmp/degraded-read.case" <<'CASE'
ID="degraded_read"
NAME="degraded read verdict"
QUESTION="degraded read test"
MIN_OUTPUT_CHARS=1
EXPECT_CONTAINS="expected-symbol"
CASE
CODRAX_BIN="$tmp/fake-codrax-degraded-read" EVAL_RESULTS_ROOT="$tmp/degraded-results" CODRAX_PROVIDER_ARGS_RAW="" \
  eval/run.sh "$tmp/degraded-read.case" 1 >/dev/null || fail "degraded read eval failed to run"
degraded_read_dir="$(eval_latest_result_dir "$tmp/degraded-results" degraded_read 00000000-000000 || true)"
[[ -n "$degraded_read_dir" ]] || fail "degraded read result dir missing"
assert_eq "$(cat "$degraded_read_dir/run-1.verdict")" "FAIL degraded_answer_checks_skipped:1" "degraded read must not pass through answer regex"

ALLOW_DEGRADED_READ_ANSWER=1 CODRAX_BIN="$tmp/fake-codrax-degraded-read" EVAL_RESULTS_ROOT="$tmp/degraded-optout-results" CODRAX_PROVIDER_ARGS_RAW="" \
  eval/run.sh "$tmp/degraded-read.case" 1 >/dev/null || fail "degraded read opt-out eval failed to run"
degraded_optout_dir="$(eval_latest_result_dir "$tmp/degraded-optout-results" degraded_read 00000000-000000 || true)"
[[ -n "$degraded_optout_dir" ]] || fail "degraded read opt-out result dir missing"
assert_eq "$(cat "$degraded_optout_dir/run-1.verdict")" "PASS" "explicit degraded-lane case may opt out"

cat >"$tmp/finalizer-reject-asymmetric.log" <<'LOG'
2026-05-24T00:00:00.010 DEBUG [diag finalizer] iter=0 phase=toolresult TOOLRESULT emit_answer_document ok=false len=12:
2026-05-24T00:00:00.020 DEBUG [diag finalizer] iter=1 phase=toolresult TOOLRESULT emit_answer_document_patch ok=false len=12:
2026-05-24T00:00:00.030 INFO [render]   • 成文校验未通过
LOG
assert_eq "$(eval_count_finalizer_rejects "$tmp/finalizer-reject-asymmetric.log")" "2" "finalizer reject asymmetric mirror count"
assert_eq "$(eval_count_finalizer_rewrites "$tmp/finalizer-control.log")" "1" "finalizer rewrite control count"
assert_eq "$(eval_count_answer_document_patch_calls "$tmp/finalizer-control.log")" "1" "answer patch control count"
assert_eq "$(eval_count_midloop_injects "$tmp/finalizer-control.log")" "1" "midloop inject control count"
assert_eq "$(eval_count_tool_calls "$tmp/finalizer-control.log" repo_map)" "1" "tool-call control count"
assert_eq "$(eval_count_source_inventory_tool_calls "$tmp/finalizer-control.log")" "1" "source inventory tool-call count"
assert_eq "$(eval_count_repo_lens_discovery_hints "$tmp/finalizer-control.log")" "1" "repo lens discovery hint control count"
assert_eq "$(eval_count_analyze_refine_dispatches "$tmp/finalizer-control.log")" "1" "analyze-refine dispatch count"
assert_eq "$(eval_count_read_loop_add_proof_selected "$tmp/finalizer-control.log")" "1" "read loop add-proof selected count"
assert_eq "$(eval_count_read_loop_add_proof_consumed "$tmp/finalizer-control.log")" "1" "read loop add-proof consumed count"
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

inventory_answer=$'| category | symbol | path | package |\n| extend | extend Cart | eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj | demo.cart |\n| foreign func | native_add | eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj | demo.bridge |'
EXPECT_INVENTORY_ROWSETS="extend foreign_func"
EXPECT_INVENTORY_ROW_SCOPE_EXTEND="line"
EXPECT_INVENTORY_ROWS_EXTEND=$'extend Cart|Cart.cj|demo.cart'
EXPECT_INVENTORY_COUNT_EXTEND=1
EXPECT_INVENTORY_ROW_SCOPE_FOREIGN_FUNC="line"
EXPECT_INVENTORY_ROWS_FOREIGN_FUNC=$'native_add|Bridge.cj|demo.bridge'
EXPECT_INVENTORY_COUNT_FOREIGN_FUNC=1
EXPECT_INVENTORY_BANNED_ROWS_FOREIGN_FUNC=$'foreign func|runOnMainThread|Bridge.cj'
assert_eq "$(eval_inventory_rowset_reasons "$inventory_answer")" "" "inventory row oracle pass"

inventory_missing=$'| category | symbol | path | package |\n| extend | extend Cart | eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj | demo.cart |'
assert_eq "$(eval_inventory_rowset_reasons "$inventory_missing")" "missing_inventory_row:foreign_func:native_add_Bridge.cj_demo.bridge
inventory_count_mismatch:foreign_func:got0:want1" "inventory row oracle missing row"

inventory_banned="${inventory_answer}"$'\n| foreign func | runOnMainThread | eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj | demo.bridge |'
assert_eq "$(eval_inventory_rowset_reasons "$inventory_banned")" "banned_inventory_row:foreign_func:foreign_func_runOnMainThread_Bridge.cj" "inventory row oracle banned row"
unset EXPECT_INVENTORY_ROWSETS EXPECT_INVENTORY_ROW_SCOPE_EXTEND EXPECT_INVENTORY_ROWS_EXTEND EXPECT_INVENTORY_COUNT_EXTEND
unset EXPECT_INVENTORY_ROW_SCOPE_FOREIGN_FUNC EXPECT_INVENTORY_ROWS_FOREIGN_FUNC EXPECT_INVENTORY_COUNT_FOREIGN_FUNC EXPECT_INVENTORY_BANNED_ROWS_FOREIGN_FUNC

EXPECT_INVENTORY_ROWSETS="extend public_class"
EXPECT_INVENTORY_ROW_SCOPE_EXTEND="line"
EXPECT_INVENTORY_ROWS_EXTEND=$'Cart|Cart.cj|demo.cart'
EXPECT_INVENTORY_COUNT_EXTEND=1
EXPECT_INVENTORY_ROW_SCOPE_PUBLIC_CLASS="line"
EXPECT_INVENTORY_ROWS_PUBLIC_CLASS=$'Cart|Cart.cj|demo.cart'
EXPECT_INVENTORY_COUNT_PUBLIC_CLASS=1
inventory_cross_bucket=$'### extend\n- Cart — package: demo.cart (`Cart.cj:30` — extend Cart {)\n\n### public class\n- Bridge — package: demo.bridge (`Bridge.cj:15` — public class Bridge {)'
assert_eq "$(eval_inventory_rowset_reasons "$inventory_cross_bucket")" "missing_inventory_row:public_class:Cart_Cart.cj_demo.cart" "inventory row oracle should not satisfy a row from a sibling section while preserving the independent visible-row count"
inventory_cross_bucket_ok=$'### extend\n- Cart — package: demo.cart (`Cart.cj:30` — extend Cart {)\n\n### public class\n- Cart — package: demo.cart (`Cart.cj:14` — public class Cart {)'
assert_eq "$(eval_inventory_rowset_reasons "$inventory_cross_bucket_ok")" "" "inventory row oracle should pass when the row appears in the matching section"
unset EXPECT_INVENTORY_ROWSETS EXPECT_INVENTORY_ROW_SCOPE_EXTEND EXPECT_INVENTORY_ROWS_EXTEND EXPECT_INVENTORY_COUNT_EXTEND
unset EXPECT_INVENTORY_ROW_SCOPE_PUBLIC_CLASS EXPECT_INVENTORY_ROWS_PUBLIC_CLASS EXPECT_INVENTORY_COUNT_PUBLIC_CLASS

EXPECT_INVENTORY_ROWSETS="extend foreign_func public_class"
EXPECT_INVENTORY_ROW_SCOPE_EXTEND="line"
EXPECT_INVENTORY_ROW_MARKER_EXTEND="| extend "
EXPECT_INVENTORY_ROWS_EXTEND=$'String|String.cj|demo.stringext\nCart|Cart.cj|demo.cart'
EXPECT_INVENTORY_COUNT_EXTEND=2
EXPECT_INVENTORY_ROW_SCOPE_FOREIGN_FUNC="line"
EXPECT_INVENTORY_ROW_MARKER_FOREIGN_FUNC="| foreign func"
EXPECT_INVENTORY_ROWS_FOREIGN_FUNC=$'native_add|FFI.cj|demo.ffi\nnative_add|Bridge.cj|demo.bridge'
EXPECT_INVENTORY_COUNT_FOREIGN_FUNC=2
EXPECT_INVENTORY_ROW_SCOPE_PUBLIC_CLASS="line"
EXPECT_INVENTORY_ROW_MARKER_PUBLIC_CLASS="| public "
EXPECT_INVENTORY_ROWS_PUBLIC_CLASS=$'Bridge|Bridge.cj|demo.bridge\nAnimal|Animal.cj|demo.modifiers'
EXPECT_INVENTORY_COUNT_PUBLIC_CLASS=2
inventory_mixed_table=$'### extend blocks\n\nTwo extensions were found.\n\n### foreign func declarations\n\nTwo foreign functions were found.\n\n### public classes\n\nTwo public classes were found.\n\n| kind | symbol | path | package |\n|---|---|---|---|\n| extend String | String | src/String.cj:6 | demo.stringext |\n| extend Cart | Cart | src/Cart.cj:30 | demo.cart |\n| foreign func native_add | native_add | src/FFI.cj:6 | demo.ffi |\n| foreign func native_add | native_add | src/Bridge.cj:6 | demo.bridge |\n| public class Bridge | Bridge | src/Bridge.cj:15 | demo.bridge |\n| public sealed class Animal | Animal | src/Animal.cj:6 | demo.modifiers; extended by extend Cart |'
assert_eq "$(eval_inventory_rowset_reasons "$inventory_mixed_table")" "" "typed row markers must partition a combined inventory table even when prose headings appear first"
inventory_mixed_table_extra="${inventory_mixed_table}"$'\n| extend Extra | Extra | src/Extra.cj:9 | demo.extra |'
assert_eq "$(eval_inventory_rowset_reasons "$inventory_mixed_table_extra")" "inventory_count_mismatch:extend:got3:want2" "typed row markers must still reject an unexpected member in a combined table"
unset EXPECT_INVENTORY_ROWSETS EXPECT_INVENTORY_ROW_SCOPE_EXTEND EXPECT_INVENTORY_ROW_MARKER_EXTEND EXPECT_INVENTORY_ROWS_EXTEND EXPECT_INVENTORY_COUNT_EXTEND
unset EXPECT_INVENTORY_ROW_SCOPE_FOREIGN_FUNC EXPECT_INVENTORY_ROW_MARKER_FOREIGN_FUNC EXPECT_INVENTORY_ROWS_FOREIGN_FUNC EXPECT_INVENTORY_COUNT_FOREIGN_FUNC
unset EXPECT_INVENTORY_ROW_SCOPE_PUBLIC_CLASS EXPECT_INVENTORY_ROW_MARKER_PUBLIC_CLASS EXPECT_INVENTORY_ROWS_PUBLIC_CLASS EXPECT_INVENTORY_COUNT_PUBLIC_CLASS

EXPECT_INVENTORY_ROWSETS="extend"
EXPECT_INVENTORY_ROW_SCOPE_EXTEND="line"
EXPECT_INVENTORY_ROWS_EXTEND=$'Cart|Cart.cj|demo.cart'
EXPECT_INVENTORY_COUNT_EXTEND=1
inventory_extra_table_row=$'**extend 块**\n\n| symbol | path | package |\n|---|---|---|\n| Cart | Cart.cj:30 | demo.cart |\n| Highlight | ArkTS.ets:8 | demo.marker |\n\n**public class**\n\n| symbol | path | package |\n| Cart | Cart.cj:14 | demo.cart |'
assert_eq "$(eval_inventory_rowset_reasons "$inventory_extra_table_row")" "inventory_count_mismatch:extend:got2:want1" "inventory exact count must reject an unexpected visible table row"
inventory_exact_bold_table=$'**extend 块**\n\n| symbol | path | package |\n|---|---|---|\n| Cart | Cart.cj:30 | demo.cart |\n\n**public class**\n\n| symbol | path | package |\n| Cart | Cart.cj:14 | demo.cart |\n\n> **范围说明** only the checked scope\n\n**引用**：\n\n- `Cart.cj:30`'
assert_eq "$(eval_inventory_rowset_reasons "$inventory_exact_bold_table")" "" "inventory exact count accepts the actual rows under a bold section heading"
inventory_list_with_note=$'### extend\n\n1. **Cart** — package demo.cart (`Cart.cj:30` — extend Cart {)\n\n**说明**：\n\n- 该清单按 package 分组展示。'
assert_eq "$(eval_inventory_rowset_reasons "$inventory_list_with_note")" "" "inventory exact count must not count a trailing prose note as another list member"
unset EXPECT_INVENTORY_ROWSETS EXPECT_INVENTORY_ROW_SCOPE_EXTEND EXPECT_INVENTORY_ROWS_EXTEND EXPECT_INVENTORY_COUNT_EXTEND

EXPECT_INVENTORY_ROWSETS="entry_page"
EXPECT_INVENTORY_ROW_SCOPE_ENTRY_PAGE="document"
EXPECT_INVENTORY_SECTION_LABEL_ENTRY_PAGE="@Entry pages"
EXPECT_INVENTORY_ROWS_ENTRY_PAGE=$'Index|Index.ets\nParent|Parent.ets'
EXPECT_INVENTORY_COUNT_ENTRY_PAGE=2
inventory_nested_extra=$'### @Entry pages\n\nRequested entries:\n\n**@Entry table**\n\n| symbol | path |\n|---|---|\n| Index | Index.ets:5 |\n| Parent | Parent.ets:9 |\n| Undecorated | Ability.ets:12 |\n\n### @Builder fragments\n\n| symbol | path |\n|---|---|\n| Card | Card.ets:3 |'
assert_eq "$(eval_inventory_rowset_reasons "$inventory_nested_extra")" "inventory_count_mismatch:entry_page:got3:want2" "explicit inventory section label must count unexpected rows through nested bold headings"
EXPECT_INVENTORY_SECTION_LABEL_ENTRY_PAGE="missing section"
assert_eq "$(eval_inventory_rowset_reasons "$inventory_nested_extra")" "missing_inventory_section:entry_page:missing_section" "declared inventory section label must fail loudly when absent"
EXPECT_INVENTORY_ROW_MARKER_ENTRY_PAGE="@Entry"
EXPECT_INVENTORY_SECTION_LABEL_ENTRY_PAGE="@Entry 页面入口"
inventory_section_with_uneven_marker_rows=$'### @Entry 页面入口\n\n- **Index** — @Entry 页面，位于 `src/Index.ets:5`\n- **Parent** — 页面入口，位于 `src/Parent.ets:9`\n\n### @Builder 复用片段\n\n- **Card** — 位于 `src/Card.ets:3`'
assert_eq "$(eval_inventory_rowset_reasons "$inventory_section_with_uneven_marker_rows")" "" "explicit inventory section must outrank a partial marker-row subset"
EXPECT_INVENTORY_SECTION_LABEL_ENTRY_PAGE="missing section"
inventory_marker_heading=$'### @Entry 页面入口\n\n| symbol | path |\n|---|---|\n| Index | Index.ets:5 |\n| Parent | Parent.ets:9 |\n\n### @Builder 复用片段\n\n| symbol | path |\n|---|---|\n| Card | Card.ets:3 |'
assert_eq "$(eval_inventory_rowset_reasons "$inventory_marker_heading")" "" "typed inventory marker scopes a semantically identical heading without requiring exact presentation copy"
inventory_marker_summary_heading=$'## ArkTS @Entry 和 @Builder 装饰器成员清单\n\n总计 3 项。\n\n### @Entry 页面入口\n\n| symbol | path |\n|---|---|\n| Index | Index.ets:5 |\n| Parent | Parent.ets:9 |\n\n### @Builder 复用片段\n\n| symbol | path |\n|---|---|\n| Card | Card.ets:3 |'
assert_eq "$(eval_inventory_rowset_reasons "$inventory_marker_summary_heading")" "" "typed inventory marker selects the specific nested group instead of a broad matching report title"
inventory_marker_summary_with_citations=$'## ArkTS @Entry 和 @Builder 装饰器成员清单\n\n### @Entry 页面入口\n\n| symbol | path |\n|---|---|\n| Index | Index.ets:5 |\n| Parent | Parent.ets:9 |\n\n### @Builder 复用片段\n\n| symbol | path |\n|---|---|\n| Card | Card.ets:3 |\n\n**引用**：\n\n- `Index.ets:5` — @Entry\n- `Parent.ets:9` — @Entry\n- `Card.ets:3` — @Builder'
assert_eq "$(eval_inventory_rowset_reasons "$inventory_marker_summary_with_citations")" "" "inventory section counts stop before renderer citation appendix rows"
inventory_marker_heading_extra=$'### @Entry 页面入口\n\n| symbol | path |\n|---|---|\n| Index | Index.ets:5 |\n| Parent | Parent.ets:9 |\n| Ability | Ability.ets:12 |\n\n### @Builder 复用片段\n\n| symbol | path |\n|---|---|\n| Card | Card.ets:3 |'
assert_eq "$(eval_inventory_rowset_reasons "$inventory_marker_heading_extra")" "inventory_count_mismatch:entry_page:got3:want2" "typed marker heading still rejects an unexpected member inside the group"
inventory_inline_exact=$'仓库中有两个入口。\n\n1. **Index** — @Entry 页面，位于 `src/Index.ets:5`\n2. **Parent** — @Entry 页面，位于 `src/Parent.ets:9`\n\n**引用**：\n\n- `src/Index.ets:5` — @Entry\n- `src/Parent.ets:9` — @Entry'
assert_eq "$(eval_inventory_rowset_reasons "$inventory_inline_exact")" "" "typed inventory marker accepts heading-free ordered rows without double-counting citations"
inventory_inline_extra=$'仓库中有三个入口。\n\n1. **Index** — @Entry 页面，位于 `src/Index.ets:5`\n2. **Parent** — @Entry 页面，位于 `src/Parent.ets:9`\n3. **Ability** — @Entry 页面，位于 `src/Ability.ets:12`'
assert_eq "$(eval_inventory_rowset_reasons "$inventory_inline_extra")" "inventory_count_mismatch:entry_page:got3:want2" "typed inventory marker rejects an extra heading-free inventory row"
EXPECT_INVENTORY_ROW_MARKER_ENTRY_PAGE="@Missing"
assert_eq "$(eval_inventory_rowset_reasons "$inventory_inline_exact")" "missing_inventory_group:entry_page:@Missing" "declared inventory group fails loudly when neither section nor typed marker rows exist"
unset EXPECT_INVENTORY_ROWSETS EXPECT_INVENTORY_ROW_SCOPE_ENTRY_PAGE EXPECT_INVENTORY_SECTION_LABEL_ENTRY_PAGE EXPECT_INVENTORY_ROW_MARKER_ENTRY_PAGE EXPECT_INVENTORY_ROWS_ENTRY_PAGE EXPECT_INVENTORY_COUNT_ENTRY_PAGE

cat >"$tmp/fake-codrax-inventory-rowset" <<'SH'
#!/usr/bin/env bash
echo 'thinking stream'
echo '━━━'
echo '| category | symbol | path | package |'
echo '| extend | extend Cart | eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj | demo.cart |'
SH
chmod +x "$tmp/fake-codrax-inventory-rowset"
cat >"$tmp/inventory-rowset.case" <<'CASE'
ID="inventory_rowset"
NAME="inventory rowset"
QUESTION="inventory test"
MIN_OUTPUT_CHARS=1
EXPECT_INVENTORY_ROWSETS="extend foreign_func"
EXPECT_INVENTORY_ROW_SCOPE_EXTEND="line"
EXPECT_INVENTORY_ROWS_EXTEND=$'extend Cart|Cart.cj|demo.cart'
EXPECT_INVENTORY_COUNT_EXTEND=1
EXPECT_INVENTORY_ROW_SCOPE_FOREIGN_FUNC="line"
EXPECT_INVENTORY_ROWS_FOREIGN_FUNC=$'native_add|Bridge.cj|demo.bridge'
EXPECT_INVENTORY_COUNT_FOREIGN_FUNC=1
CASE
CODRAX_BIN="$tmp/fake-codrax-inventory-rowset" EVAL_RESULTS_ROOT="$tmp/inventory-results" CODRAX_PROVIDER_ARGS_RAW="" eval/run.sh "$tmp/inventory-rowset.case" 1 >/dev/null || fail "inventory rowset eval failed to run"
inventory_rowset_dir="$(eval_latest_result_dir "$tmp/inventory-results" inventory_rowset 00000000-000000 || true)"
[[ -n "$inventory_rowset_dir" ]] || fail "inventory rowset result dir missing"
case "$(cat "$inventory_rowset_dir/run-1.verdict")" in
  "FAIL missing_inventory_row:foreign_func:native_add_Bridge.cj_demo.bridge inventory_count_mismatch:foreign_func:got0:want1")
    ;;
  *)
    fail "inventory rowset verdict should report missing typed row, got: $(cat "$inventory_rowset_dir/run-1.verdict")"
    ;;
esac

cat >"$tmp/fake-codrax-inventory-marker" <<'SH'
#!/usr/bin/env bash
echo 'thinking stream'
echo '━━━'
echo '1. **Index** — @Entry page at `src/Index.ets:5`'
echo '2. **Parent** — @Entry page at `src/Parent.ets:9`'
echo ''
echo '**引用**：'
echo '- `src/Index.ets:5` — @Entry'
echo '- `src/Parent.ets:9` — @Entry'
SH
chmod +x "$tmp/fake-codrax-inventory-marker"
cat >"$tmp/inventory-marker.case" <<'CASE'
ID="inventory_marker"
NAME="inventory marker"
QUESTION="inventory marker test"
MIN_OUTPUT_CHARS=1
EXPECT_INVENTORY_ROWSETS="entry_page"
EXPECT_INVENTORY_SECTION_LABEL_ENTRY_PAGE="@Entry pages"
EXPECT_INVENTORY_ROW_MARKER_ENTRY_PAGE="@Entry"
EXPECT_INVENTORY_ROW_SCOPE_ENTRY_PAGE="document"
EXPECT_INVENTORY_ROWS_ENTRY_PAGE=$'Index|Index.ets\nParent|Parent.ets'
EXPECT_INVENTORY_COUNT_ENTRY_PAGE=2
CASE
CODRAX_BIN="$tmp/fake-codrax-inventory-marker" EVAL_RESULTS_ROOT="$tmp/inventory-results" CODRAX_PROVIDER_ARGS_RAW="" eval/run.sh "$tmp/inventory-marker.case" 1 >/dev/null || fail "inventory marker eval failed to run"
inventory_marker_dir="$(eval_latest_result_dir "$tmp/inventory-results" inventory_marker 00000000-000000 || true)"
[[ -n "$inventory_marker_dir" ]] || fail "inventory marker result dir missing"
assert_eq "$(cat "$inventory_marker_dir/run-1.verdict")" "PASS" "run.sh inventory marker wiring must accept heading-free primary rows and ignore renderer citations"

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

cat >"$tmp/finalizer-contract-phases.log" <<'LOG'
2026-06-08T00:00:01.000 DEBUG [diag finalizer] phase=answer_contract_check section=v2_block_oracles done elapsed=1ms violations=3 strict_violations=2 soft_violations=1
2026-06-08T00:00:02.000 DEBUG [diag finalizer] phase=answer_contract_check section=lane_block_kind done elapsed=1ms violations=1 strict_violations=1 soft_violations=0
2026-06-08T00:00:03.000 DEBUG [diag finalizer] phase=answer_contract_check section=v2_block_oracles done elapsed=1ms violations=1 strict_violations=0 soft_violations=1
2026-06-08T00:00:04.000 DEBUG [diag finalizer] phase=answer_contract_check section=lane_block_kind done elapsed=1ms violations=0 strict_violations=0 soft_violations=0
LOG
assert_eq "$(eval_first_pass_answer_contract_strict_violations "$tmp/finalizer-contract-phases.log")" "3" "answer contract first-pass strict sum"
assert_eq "$(eval_final_answer_contract_strict_violations "$tmp/finalizer-contract-phases.log")" "0" "answer contract final strict sum"
assert_eq "$(eval_auto_repaired_answer_contract_strict_violations "$tmp/finalizer-contract-phases.log")" "3" "answer contract auto-repaired strict sum"
assert_eq "$(eval_first_pass_answer_contract_strict_violations_for_section "$tmp/finalizer-contract-phases.log" lane_block_kind)" "1" "answer contract section first-pass strict"
assert_eq "$(eval_final_answer_contract_strict_violations_for_section "$tmp/finalizer-contract-phases.log" lane_block_kind)" "0" "answer contract section final strict"
assert_eq "$(eval_auto_repaired_answer_contract_strict_violations_for_section "$tmp/finalizer-contract-phases.log" lane_block_kind)" "1" "answer contract section auto-repaired strict"
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
2026-06-22T10:13:12.000 WARN [emit_answer_document] blocks[] arrived as JSON-encoded string; re-parsed via flat-mode tolerance path
2026-06-22T10:13:13.000 DEBUG [diag explorer] iter=1 ASSISTANT content: quoted "[emit_answer_document] blocks[] arrived as JSON-encoded string; re-parsed via flat-mode tolerance path"
2026-06-22T10:13:14.000 WARN [emit_investigation_complete] aggregate_facts arrived as JSON-encoded string; re-parsed losslessly
2026-06-22T10:13:15.000 DEBUG [diag explorer] iter=1 ASSISTANT content: quoted "[emit_investigation_complete] aggregate_facts arrived as JSON-encoded string; re-parsed losslessly"
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
assert_eq "$(eval_metric_field "$partial_dir/run-1.metrics.txt" answer_document_blocks_string_recovery_events)" "1" "partial timeout blocks-string recovery metric excludes model quotation"
assert_eq "$(eval_metric_field "$partial_dir/run-1.metrics.txt" investigation_aggregate_facts_string_recovery_events)" "1" "partial timeout aggregate-facts string recovery metric excludes model quotation"

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
2026-06-08T00:00:03.000 WARN [emit_answer_document] blocks[] arrived as JSON-encoded string; re-parsed via flat-mode tolerance path
2026-06-08T00:00:04.000 DEBUG [diag finalizer] iter=1 ASSISTANT content: quoted "[emit_answer_document] blocks[] arrived as JSON-encoded string; re-parsed via flat-mode tolerance path"
2026-06-08T00:00:05.000 WARN [emit_investigation_complete] aggregate_facts arrived as JSON-encoded string; re-parsed losslessly
2026-06-08T00:00:06.000 DEBUG [diag finalizer] iter=1 ASSISTANT content: quoted "[emit_investigation_complete] aggregate_facts arrived as JSON-encoded string; re-parsed losslessly"
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
assert_eq "$(eval_metric_field "$multilog_dir/run-1.metrics.txt" answer_document_blocks_string_recovery_events)" "1" "aggregate log blocks-string recovery metric excludes model quotation"
assert_eq "$(eval_metric_field "$multilog_dir/run-1.metrics.txt" investigation_aggregate_facts_string_recovery_events)" "1" "aggregate log aggregate-facts string recovery metric excludes model quotation"

runtime_log="$tmp/runtime-authority.log"
cat >"$runtime_log" <<'LOG'
2026-06-08T00:00:03.000 DEBUG [diag perf_triager] DISPATCH stage=perf_triage attempt=0
2026-06-08T00:00:03.010 DEBUG [diag perf_triager] iter=0 phase=toolcall call[0] tool=emit_perf_trace params={}
2026-06-08T00:00:03.020 DEBUG [diag explorer] iter=0 phase=toolcall call[0] tool=trace_query params={"view":"window_stats"}
2026-06-08T00:00:03.030 DEBUG [diag explorer] iter=0 phase=toolcall call[1] tool=trace_query params={"view":"root_cause_rank","pid":42591,"time_start":1.0,"time_end":1.1}
2026-06-08T00:00:03.040 DEBUG [diag explorer] iter=0 phase=toolcall call[2] tool=trace_query params={"view":"wakeup_chain","thread":"main"}
2026-06-08T00:00:03.050 DEBUG [diag explorer] iter=0 phase=toolresult TOOLRESULT trace_query ok=true summary="trace_query_target_inherited=true"
LOG
assert_eq "$(eval_count_stage_dispatches "$runtime_log" perf_triage)" "1" "runtime perf pre-stage dispatch metric"
assert_eq "$(eval_runtime_attachment_kind_from_log "$runtime_log")" "trace" "runtime attachment inference"
assert_eq "$(eval_runtime_authority_path trace "$runtime_log")" "perf_triage+trace_query" "runtime authority path helper"
assert_eq "$(eval_count_trace_query_dimension_families "$runtime_log")" "4" "trace_query dimension family metric"
assert_eq "$(eval_count_trace_query_view_family "$runtime_log" 'root_cause_rank|frame_root_cause_bundle|frame_bundle')" "1" "trace_query root view metric"
assert_eq "$(eval_count_trace_query_view_family "$runtime_log" 'wakeup_chain|causal_impact|frame_root_cause_bundle|frame_bundle')" "1" "trace_query wakeup view metric"
assert_eq "$(eval_count_trace_query_windowed_calls "$runtime_log")" "1" "trace_query windowed metric"
assert_eq "$(eval_count_trace_query_pid_filtered_calls "$runtime_log")" "1" "trace_query pid filter metric"
assert_eq "$(eval_count_trace_query_thread_filtered_calls "$runtime_log")" "1" "trace_query thread filter metric"
assert_eq "$(eval_count_trace_query_target_inherited "$runtime_log")" "1" "trace_query inherited target metric"

fake_runtime="$tmp/fake-codrax-runtime-authority"
cat >"$fake_runtime" <<'FAKE'
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
cat >"$logdir/codrax-20260608-000003-000-1.log" <<'LOG'
2026-06-08T00:00:03.000 DEBUG [diag perf_triager] DISPATCH stage=perf_triage attempt=0
2026-06-08T00:00:03.010 DEBUG [diag perf_triager] iter=0 phase=toolcall call[0] tool=emit_perf_trace params={}
2026-06-08T00:00:03.020 DEBUG [diag explorer] iter=0 phase=toolcall call[0] tool=trace_query params={"view":"window_stats"}
2026-06-08T00:00:03.030 DEBUG [diag explorer] iter=0 phase=toolcall call[1] tool=trace_query params={"view":"root_cause_rank","pid":42591,"time_start":1.0,"time_end":1.1}
2026-06-08T00:00:03.040 DEBUG [diag explorer] iter=0 phase=toolcall call[2] tool=trace_query params={"view":"wakeup_chain","thread":"main"}
2026-06-08T00:00:03.050 DEBUG [diag explorer] iter=0 phase=toolresult TOOLRESULT trace_query ok=true summary="trace_query_target_inherited=true"
LOG
printf 'runtime authority answer\n## Trace Causal Projection\n'
FAKE
chmod +x "$fake_runtime"
runtime_case="$tmp/runner_runtime_authority.case"
cat >"$runtime_case" <<'CASE'
ID=runner_runtime_authority
NAME="runner runtime authority"
QUESTION="runner runtime authority smoke"
HTRACE="# tracer: nop"
MIN_OUTPUT_CHARS=1
EXPECT_CONTAINS="runtime authority answer"
CASE
CODRAX_BIN="$fake_runtime" EVAL_RESULTS_ROOT="$tmp/eval-results" bash eval/run.sh "$runtime_case" 1 >/dev/null 2>"$tmp/runner-runtime-authority.err"
runtime_dir="$(find "$tmp/eval-results" -maxdepth 1 -type d -name 'runner_runtime_authority-*' | sort | tail -1)"
if [[ -z "$runtime_dir" ]]; then
  fail "eval/run.sh did not write runtime-authority result dir"
fi
assert_eq "$(eval_metric_field "$runtime_dir/run-1.metrics.txt" runtime_artifact_attached)" "trace" "runtime artifact attached metric"
assert_eq "$(eval_metric_field "$runtime_dir/run-1.metrics.txt" runtime_authority_path)" "perf_triage+trace_query" "runtime authority path metric"
assert_eq "$(eval_metric_field "$runtime_dir/run-1.metrics.txt" perf_triage_dispatches)" "1" "runtime perf dispatch metric"
assert_eq "$(eval_metric_field "$runtime_dir/run-1.metrics.txt" emit_perf_trace_calls)" "1" "runtime emit perf metric"
assert_eq "$(eval_metric_field "$runtime_dir/run-1.metrics.txt" trace_query_dimension_families)" "4" "runtime trace dimension metric"
assert_eq "$(eval_metric_field "$runtime_dir/run-1.metrics.txt" trace_query_target_inherited)" "1" "runtime trace inherited target metric"
assert_eq "$(eval_metric_field "$runtime_dir/run-1.metrics.txt" trace_query_final_projection_blocks)" "1" "runtime trace final projection metric"
if ! grep -q '| 1 | trace | perf_triage+trace_query | 0 | 1 | 3 | 0 | 1 |' "$runtime_dir/summary.md"; then
  fail "runtime authority path audit summary missing expected row"
fi
if ! grep -q '| 1 | 4 | 1 | 1 | 0 | 1 | 1 | 1 | 1 | 1 | 1 | 1 |' "$runtime_dir/summary.md"; then
  fail "trace query coverage audit summary missing expected row"
fi

projection_metric_fixture="$tmp/projection-metric-fixture.out"
cat >"$projection_metric_fixture" <<'OUT'
model progress mentions Trace 因果投影
<title>Trace 因果投影 · manual</title>
Trace Causal Projection
## Trace 因果投影解读
## Trace 因果投影
> **Trace 因果投影覆盖边界** not a projection heading
## Trace Causal Projection — target window
OUT
assert_eq "$(eval_count_trace_query_final_projection_blocks "$projection_metric_fixture")" "2" "trace projection metric counts only exact answer headings"

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
2026-06-08T00:00:04.000 DEBUG [diag orchestrator] phase=transient_retry_checkpoint stage=explore installed=true
2026-06-08T00:00:04.000 DEBUG [diag explorer] DISPATCH stage=explore attempt=1
2026-06-08T00:00:04.001 DEBUG [diag explorer] iter=0 ASSISTANT content_len=12
2026-06-08T00:00:04.002 DEBUG [diag explorer] phase=toolcall tool=read_file params={"path":"internal/foo.go"}
2026-06-08T00:00:04.003 DEBUG [diag explorer] phase=midloop_inject key="explorer.mid-loop.read-without-emit"
2026-06-08T00:00:04.004 DEBUG [mermaidcompat] source repair applied before_bytes=1 after_bytes=2
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
ADVISORY_MAX_TRANSIENT_RETRY_CHECKPOINTS=0
ADVISORY_MAX_MERMAID_SOURCE_REPAIR_APPLIED=0
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
if ! grep -q '| 1 | transient_retry_checkpoint | transient_retry_checkpoints=1 limit=0 |' "$efficiency_dir/summary.md"; then
  fail "efficiency advisory summary missing transient retry row"
fi
if ! grep -q '| 1 | mermaid_source_repair_churn | mermaid_source_repair_applied=1 limit=0 |' "$efficiency_dir/summary.md"; then
  fail "efficiency advisory summary missing mermaid repair row"
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
    if [[ "${FAKE_WRITE_MULTIREF:-0}" == "1" ]]; then
      # Cold-discard multi-plan session (rework P2-1): an EARLIER plan's
      # checkpoint pinned as a SIBLING commit (branched from base, NOT
      # chained under the final plan's checkpoint). The complete delivery
      # is the in-order union of both refs; materializing only the newest
      # ref loses early.py.
      printf 'early plan marker\n' >"$repo/early.py"
      git -C "$repo" -c user.email=eval@codrax -c user.name=eval add early.py
      GIT_COMMITTER_DATE='2026-01-01T00:00:00 +0000' git -C "$repo" -c user.email=eval@codrax -c user.name=eval commit -q -m "fake applied early plan"
      git -C "$repo" update-ref refs/codrax/applied/plan-fake-write-early "$(git -C "$repo" rev-parse HEAD)"
      git -C "$repo" reset --hard -q HEAD~1
    fi
    sed 's/retrun/return/' "$repo/main.py" >"$repo/main.py.tmp"
    mv "$repo/main.py.tmp" "$repo/main.py"
    git -C "$repo" -c user.email=eval@codrax -c user.name=eval add main.py
    git -C "$repo" -c user.email=eval@codrax -c user.name=eval commit -q -m "fake applied"
    applied_sha="$(git -C "$repo" rev-parse HEAD)"
    git -C "$repo" update-ref refs/codrax/applied/plan-fake-write-apply "$applied_sha"
    git -C "$repo" reset --hard -q HEAD~1
  else
    # Production parity: a successful apply always pins the recovery
    # ref; a preserved worktree is orthogonal. FAKE_WRITE_REF=none
    # simulates a broken delivery chain (worktree only, no ref);
    # FAKE_WRITE_REF=stale simulates the zod run-1 mask (worktree has
    # the fix, the durable ref does NOT).
    worktree="$plan_dir/fake-worktree"
    mkdir -p "$worktree"
    sed 's/retrun/return/' "$repo/main.py" >"$worktree/main.py"
    applied_sha=""
    case "${FAKE_WRITE_REF:-real}" in
      none)
        ;;
      stale)
        git -C "$repo" -c user.email=eval@codrax -c user.name=eval commit -q --allow-empty -m "fake applied (stale: no fix)"
        applied_sha="$(git -C "$repo" rev-parse HEAD)"
        git -C "$repo" update-ref refs/codrax/applied/plan-fake-write-apply "$applied_sha"
        git -C "$repo" reset --hard -q HEAD~1
        ;;
      *)
        sed 's/retrun/return/' "$repo/main.py" >"$repo/main.py.tmp"
        mv "$repo/main.py.tmp" "$repo/main.py"
        git -C "$repo" -c user.email=eval@codrax -c user.name=eval add main.py
        git -C "$repo" -c user.email=eval@codrax -c user.name=eval commit -q -m "fake applied"
        applied_sha="$(git -C "$repo" rev-parse HEAD)"
        git -C "$repo" update-ref refs/codrax/applied/plan-fake-write-apply "$applied_sha"
        git -C "$repo" reset --hard -q HEAD~1
        ;;
    esac
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
  if [[ "${FAKE_WRITE_FINAL:-1}" == "1" ]]; then
    final_verdict="${FAKE_WRITE_FINAL_VERDICT:-verified}"
    final_reason="all_batches_verified"
    if [[ "$final_verdict" != "verified" ]]; then
      final_reason="verification_proof_incomplete"
    fi
    cat >"$plan_dir/plan-fake-write-apply.final.json" <<JSON
{
  "kind": "final_report",
  "run_status": "complete",
  "completion": {"verdict": "$final_verdict", "reason_code": "$final_reason"},
  "plan": {"id": "plan-fake-write-apply"}
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

write_unverified_case="$tmp/runner_write_apply_final_unverified.case"
cat >"$write_unverified_case" <<'CASE'
ID=runner_write_apply_final_unverified
NAME="runner write apply final unverified"
MODE=apply
FIXTURE="eval/fixtures/testdata/patch_typo_python"
QUESTION="fix typo"
POST_APPLY_FILE="main.py"
EXPECT_MATCHES_REGEX="return f"
CASE
FAKE_WRITE_REPORT=1 FAKE_WRITE_FINAL_VERDICT=unverified CODRAX_BIN="$fake_write_apply" EVAL_RESULTS_ROOT="$tmp/eval-results" bash eval/run.sh "$write_unverified_case" 1 >/dev/null 2>"$tmp/runner-write-unverified.err"
write_unverified_dir="$(find "$tmp/eval-results" -maxdepth 1 -type d -name 'runner_write_apply_final_unverified-*' | sort | tail -1)"
if [[ -z "$write_unverified_dir" ]]; then
  fail "eval/run.sh did not write final-unverified result dir"
fi
write_unverified_verdict="$(cat "$write_unverified_dir/run-1.verdict")"
case "$write_unverified_verdict" in
  FAIL*write_final_verdict:unverified:verification_proof_incomplete*)
    ;;
  *)
    fail "write apply final unverified must fail despite local report pass: $write_unverified_verdict"
    ;;
esac
assert_eq "$(eval_json_top_bool_field "$write_unverified_dir/run-1.write-apply.json" report_passed)" "true" "write apply final-unverified local report remains passed"
assert_eq "$(eval_json_top_bool_field "$write_unverified_dir/run-1.write-apply.json" verify_authoritative)" "false" "write apply final-unverified result not authoritative"

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

# Cold-discard multi-plan union (rework P2-1): the session pinned TWO
# sibling (non-chained) applied refs and the worktree is gone. EXPECT
# needs bytes from BOTH plans — the materializer must union the refs in
# apply order; newest-ref-only materialization loses the early plan's
# bytes and would flunk a complete delivery.
write_multiref_case="$tmp/runner_write_apply_multi_ref_union.case"
cat >"$write_multiref_case" <<'CASE'
ID=runner_write_apply_multi_ref_union
NAME="runner write apply cold-discard multi-plan union delivers every plan's bytes"
MODE=apply
FIXTURE="eval/fixtures/testdata/patch_typo_python"
QUESTION="fix typo"
EXPECT_MATCHES_REGEX="return f
early plan marker"
CASE
FAKE_WRITE_REPORT=1 FAKE_WRITE_CLEANUP=1 FAKE_WRITE_MULTIREF=1 CODRAX_BIN="$fake_write_apply" EVAL_RESULTS_ROOT="$tmp/eval-results" bash eval/run.sh "$write_multiref_case" 1 >/dev/null 2>"$tmp/runner-write-multiref.err"
write_multiref_dir="$(find "$tmp/eval-results" -maxdepth 1 -type d -name 'runner_write_apply_multi_ref_union-*' | sort | tail -1)"
if [[ -z "$write_multiref_dir" ]]; then
  fail "eval/run.sh did not write multi-ref union result dir"
fi
assert_eq "$(cat "$write_multiref_dir/run-1.verdict")" "PASS" "cold-discard multi-plan union tree must satisfy EXPECT drawn from both plans"
if [[ ! -f "$write_multiref_dir/run-1.applied-tree/early.py" ]]; then
  fail "union applied-tree must carry the earlier sibling ref's bytes"
fi
if ! grep -q "return f" "$write_multiref_dir/run-1.applied-tree/main.py"; then
  fail "union applied-tree must keep the final plan's fix bytes"
fi

# Durable-delivery-first pins (eval-audit 20260719 GAP-2 eval infra):
# EXPECT must judge the recovery-ref bytes, not the live worktree.

# 1. Worktree present but NO durable ref → fail loud with
#    durable_apply_ref_missing instead of silently passing off the
#    worktree bytes.
write_noref_case="$tmp/runner_write_apply_no_durable_ref.case"
cat >"$write_noref_case" <<'CASE'
ID=runner_write_apply_no_durable_ref
NAME="runner write apply worktree without durable ref fails loud"
MODE=apply
FIXTURE="eval/fixtures/testdata/patch_typo_python"
QUESTION="fix typo"
POST_APPLY_FILE="main.py"
EXPECT_MATCHES_REGEX="return f"
CASE
FAKE_WRITE_REPORT=1 FAKE_WRITE_REF=none CODRAX_BIN="$fake_write_apply" EVAL_RESULTS_ROOT="$tmp/eval-results" bash eval/run.sh "$write_noref_case" 1 >/dev/null 2>"$tmp/runner-write-noref.err"
write_noref_dir="$(find "$tmp/eval-results" -maxdepth 1 -type d -name 'runner_write_apply_no_durable_ref-*' | sort | tail -1)"
if [[ -z "$write_noref_dir" ]]; then
  fail "eval/run.sh did not write no-durable-ref result dir"
fi
write_noref_verdict="$(cat "$write_noref_dir/run-1.verdict")"
case "$write_noref_verdict" in
  FAIL*durable_apply_ref_missing*) ;;
  *) fail "worktree-only apply must fail loud with durable_apply_ref_missing; got: $write_noref_verdict" ;;
esac

# 2. Zod run-1 mask shape: worktree carries the fix, the durable ref does
#    NOT. EXPECT must read the ref bytes and go red — the live worktree
#    must not mask a broken durable delivery chain.
write_stale_case="$tmp/runner_write_apply_stale_ref.case"
cat >"$write_stale_case" <<'CASE'
ID=runner_write_apply_stale_ref
NAME="runner write apply stale durable ref is not masked by worktree"
MODE=apply
FIXTURE="eval/fixtures/testdata/patch_typo_python"
QUESTION="fix typo"
POST_APPLY_FILE="main.py"
EXPECT_MATCHES_REGEX="return f"
CASE
FAKE_WRITE_REPORT=1 FAKE_WRITE_REF=stale CODRAX_BIN="$fake_write_apply" EVAL_RESULTS_ROOT="$tmp/eval-results" bash eval/run.sh "$write_stale_case" 1 >/dev/null 2>"$tmp/runner-write-stale.err"
write_stale_dir="$(find "$tmp/eval-results" -maxdepth 1 -type d -name 'runner_write_apply_stale_ref-*' | sort | tail -1)"
if [[ -z "$write_stale_dir" ]]; then
  fail "eval/run.sh did not write stale-ref result dir"
fi
write_stale_verdict="$(cat "$write_stale_dir/run-1.verdict")"
case "$write_stale_verdict" in
  FAIL*) ;;
  *) fail "stale durable ref must fail EXPECT even when the worktree has the fix (mask shape); got: $write_stale_verdict" ;;
esac
if [[ ! -d "$write_stale_dir/run-1.applied-tree" ]]; then
  fail "stale-ref shape should have materialized the durable ref tree for EXPECT"
fi

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

# --- eval_archive_output_artifacts (ARTIFACT-KEEP, PIN-1 B7) ---------------
fake_root="$tmp/artifact-keep-root"
mkdir -p "$fake_root/.codrax/output"
printf 'witness md\n' >"$fake_root/.codrax/output/20260713-000001.000-11.md"
printf 'witness html\n' >"$fake_root/.codrax/output/20260713-000001.000-11.html"
eval_archive_output_artifacts "$fake_root" || fail "archive helper must not fail"
[[ -f "$fake_root/.codrax/output_archive/20260713-000001.000-11.md" ]] ||
  fail "md witness was not archived"
[[ -f "$fake_root/.codrax/output_archive/20260713-000001.000-11.html" ]] ||
  fail "html witness was not archived"
[[ -f "$fake_root/.codrax/output/20260713-000001.000-11.md" ]] ||
  fail "archive must copy, never move, the original dump"
# Idempotent no-clobber: a re-run keeps the FIRST archived copy even if the
# live dump was rewritten in between.
printf 'rewritten\n' >"$fake_root/.codrax/output/20260713-000001.000-11.md"
eval_archive_output_artifacts "$fake_root" || fail "archive helper must not fail on rerun"
assert_eq "$(cat "$fake_root/.codrax/output_archive/20260713-000001.000-11.md")" "witness md" \
  "archive must be no-clobber (first copy wins)"
# Absent output dir: silent no-op.
eval_archive_output_artifacts "$tmp/artifact-keep-missing" || fail "missing output dir must be a no-op"
# run.sh must actually invoke the guard before any run (wiring pin; the Go
# side mirrors this in internal/outputdump).
grep -q 'eval_archive_output_artifacts "\$ROOT"' eval/run.sh ||
  fail "eval/run.sh lost the ARTIFACT-KEEP archive call"

# --- eval_expect_token_regex / eval_expect_token_present (EVALFIX h5) ------
# §29.64/§29.67 wound: banned `×3` substring-bit `×39` (and ×2/×4 bit
# ×28/×40) in real_trace_h5 — digit-edged tokens need digit-boundary guards;
# \b is unusable in CJK/symbol context, so boundaries are explicit classes.
assert_eq "$(eval_expect_token_regex '×3')" '×3([^0-9]|$)' \
  "digit-suffix token gets a trailing digit guard"
assert_eq "$(eval_expect_token_regex '132.041')" '(^|[^0-9])132\.041([^0-9]|$)' \
  "digit-both-edges token gets both guards and dot escaping"
assert_eq "$(eval_expect_token_regex 'still_present')" 'still_present' \
  "non-digit-edged token keeps plain literal semantics"
assert_eq "$(eval_expect_token_regex '块设备IO(inode)')" '块设备IO\(inode\)' \
  "ERE metacharacters inside the token are escaped"

# The h5 false-bite form: ×39 in the answer must NOT trigger banned ×3.
if eval_expect_token_present '×3' 'fscache_page_get_an ×39 次合计 14.756ms'; then
  fail "banned ×3 must not bite ×39 (§29.64 substring wound)"
fi
if eval_expect_token_present '×2' '（×28 次）说明目标线程'; then
  fail "banned ×2 must not bite ×28"
fi
# The constructed true-occurrence forms MUST still bite: mid-line,
# punctuation-adjacent, line-end, and own-line.
eval_expect_token_present '×3' 'sync_buffer_read_wait ×3（Σ1.354ms）' ||
  fail "true ×3 occurrence must bite (mid-line, CJK paren follower)"
eval_expect_token_present '×3' '合并计数 ×3' ||
  fail "true ×3 occurrence must bite (end of text)"
eval_expect_token_present '×3' "$(printf 'first\n×3\nlast')" ||
  fail "true ×3 occurrence must bite (own line)"
# Digit-prefix family: a longer number must not satisfy/trip the token.
if eval_expect_token_present '132.041' 'sum 5132.041 ms'; then
  fail "digit-prefixed superstring must not bite"
fi
if eval_expect_token_present '132.041' 'total 132.0415 ms'; then
  fail "digit-suffixed superstring must not bite"
fi
eval_expect_token_present '132.041' 'total 132.041ms' ||
  fail "digit token followed by a unit must bite"
# Positive channel precision: 4次( must not be satisfied by 14次(.
if eval_expect_token_present '4次(3.774~16.064ms)' '折叠 14次(3.774~16.064ms)'; then
  fail "4次(...) must not be satisfied by 14次(...)"
fi
eval_expect_token_present '4次(3.774~16.064ms)' '折叠 4次(3.774~16.064ms)' ||
  fail "paren-carrying fold token must bite its exact form"
# Historical semantics preserved: ASCII case-insensitivity and trailing-digit
# identifiers followed by non-digit context.
eval_expect_token_present 'rust' 'Rust 运行时崩溃' ||
  fail "case-insensitive concept matching must be preserved"
eval_expect_token_present 'binder:496_9' '对端 binder:496_9-10961 (proc 9743)' ||
  fail "trailing-digit identifier followed by dash must bite"
# Hash-prefix carve-out: git short hashes are identity PREFIXES — the next
# char being a hex digit is the same object (archived u7g PASS witness
# carries aa27be488e9e030afc88; a digit guard there is a false miss).
assert_eq "$(eval_expect_token_regex 'aa27be48')" 'aa27be48' \
  "hex short-hash token keeps plain substring semantics"
eval_expect_token_present 'aa27be48' '合入 aa27be488e9e030afc88 引入了' ||
  fail "hash prefix must still match inside the full hash"
# But short pure-digit values are NOT hashes — the guard applies.
if eval_expect_token_present '8330' 'period 18330us'; then
  fail "pure-digit value must not bite inside a longer number"
fi

# EVAL-B71-CASESCOPE1: checkout-dependent scalar values must be recomputed
# from a declared command, bound to the requested answer surface, and leave a
# provenance receipt. A stale hard-coded value must not green-light the case.
mkdir -p "$tmp/dynamic-repo/nested"
touch "$tmp/dynamic-repo/a.go" "$tmp/dynamic-repo/nested/b.go" "$tmp/dynamic-repo/nested/c_test.go"
EXPECT_DYNAMIC_SCALARS="go_recursive"
EXPECT_DYNAMIC_SCALAR_COMMAND_GO_RECURSIVE="find . -type f -name '*.go' ! -name '*_test.go' -print | LC_ALL=C wc -l | tr -d '[:space:]'"
EXPECT_DYNAMIC_SCALAR_DATA_SCOPE_GO_RECURSIVE="repo_checkout:.;recursion=recursive;include=*.go;exclude=*_test.go"
EXPECT_DYNAMIC_SCALAR_SURFACE_GO_RECURSIVE="primary_text"
EXPECT_DYNAMIC_SCALAR_BINDING_REGEX_GO_RECURSIVE='files[^0-9]{0,20}(^|[^0-9]){{VALUE}}([^0-9]|$)'
dynamic_reasons="$(eval_dynamic_scalar_reasons \
  'unused full answer' 'unused principal' $'recursive files\n2' \
  "$tmp/dynamic-repo" "$tmp/dynamic-pass.tsv" "")"
assert_eq "$dynamic_reasons" "" "dynamic scalar matching checkout value"
if ! grep -q $'go_recursive\t2\tprimary_text\trepo_checkout:.;recursion=recursive' "$tmp/dynamic-pass.tsv"; then
  fail "dynamic scalar receipt lost value/surface/data-scope provenance"
fi
if ! grep -q "find . -type f" "$tmp/dynamic-pass.tsv"; then
  fail "dynamic scalar receipt lost command provenance"
fi
dynamic_reasons="$(eval_dynamic_scalar_reasons \
  'unused full answer' 'unused principal' $'recursive files\n1' \
  "$tmp/dynamic-repo" "$tmp/dynamic-stale.tsv" "")"
assert_eq "$dynamic_reasons" "dynamic_scalar_binding_missing:go_recursive:2" \
  "stale hard-coded scalar must fail"
unset EXPECT_DYNAMIC_SCALARS EXPECT_DYNAMIC_SCALAR_COMMAND_GO_RECURSIVE
unset EXPECT_DYNAMIC_SCALAR_DATA_SCOPE_GO_RECURSIVE EXPECT_DYNAMIC_SCALAR_SURFACE_GO_RECURSIVE
unset EXPECT_DYNAMIC_SCALAR_BINDING_REGEX_GO_RECURSIVE

cat >"$tmp/fake-codrax-dynamic-scalar" <<'SH'
#!/usr/bin/env bash
echo '━━━'
echo 'recursive files: 1'
SH
chmod +x "$tmp/fake-codrax-dynamic-scalar"
cat >"$tmp/dynamic-scalar.case" <<'CASE'
ID="dynamic_scalar_wire"
NAME="dynamic scalar wire"
QUESTION="dynamic scalar test"
MIN_OUTPUT_CHARS=1
EXPECT_DYNAMIC_SCALARS="current"
EXPECT_DYNAMIC_SCALAR_COMMAND_CURRENT="printf 2"
EXPECT_DYNAMIC_SCALAR_DATA_SCOPE_CURRENT="repo_checkout:.;synthetic=true"
EXPECT_DYNAMIC_SCALAR_SURFACE_CURRENT="primary_text"
EXPECT_DYNAMIC_SCALAR_BINDING_REGEX_CURRENT='files[^0-9]{0,20}(^|[^0-9]){{VALUE}}([^0-9]|$)'
CASE
CODRAX_BIN="$tmp/fake-codrax-dynamic-scalar" EVAL_RESULTS_ROOT="$tmp/dynamic-results" CODRAX_PROVIDER_ARGS_RAW="" \
  eval/run.sh "$tmp/dynamic-scalar.case" 1 >/dev/null || fail "dynamic scalar wire eval failed to run"
dynamic_wire_dir="$(eval_latest_result_dir "$tmp/dynamic-results" dynamic_scalar_wire 00000000-000000 || true)"
[[ -n "$dynamic_wire_dir" ]] || fail "dynamic scalar wire result dir missing"
assert_eq "$(cat "$dynamic_wire_dir/run-1.verdict")" \
  "FAIL dynamic_scalar_binding_missing:current:2" \
  "run.sh must consume dynamic scalar verdict reasons"
if ! grep -q $'current\t2\tprimary_text\trepo_checkout:.;synthetic=true' "$dynamic_wire_dir/run-1.dynamic-scalars.tsv"; then
  fail "run.sh dynamic scalar receipt missing"
fi

# EVAL-B73-OPEVAL1 / B74-OPREPAUTH1: the last typed operation evaluation,
# rather than matching answer prose or an earlier round, owns the eval verdict.
cat >"$tmp/operation-terminal.log" <<'LOG'
2026-08-04T05:03:28.387 INFO [cli/operation] command evaluation status=complete material_coverage_status=complete coverage_material_refs="material-coverage:v1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa:html_text" coverage_source_refs="/tmp/user-guide.html" coverage_source_identities="sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa:bytes:42" coverage_source_locators="argv:curl -L https://example.test/user_guide.html" confidence="high" reason="complete" materials=1 rounds=2
2026-08-04T05:03:38.697 INFO [cli/operation] command evaluation status=partial_answer_possible material_coverage_status=partial coverage_material_refs="" coverage_source_refs="" coverage_source_identities="" coverage_source_locators="" confidence="medium" reason="partial" materials=1 rounds=2
LOG
EXPECT_OPERATION_TERMINAL_STATUS="complete"
EXPECT_OPERATION_MATERIAL_COVERAGE_STATUS="complete"
EXPECT_OPERATION_COVERAGE_REF_REGEX='^material-coverage:v1:[0-9a-f]{64}:html_text$'
EXPECT_OPERATION_COVERAGE_SOURCE_REGEX='user_guide\.html'
operation_reasons="$(eval_operation_terminal_reasons "$tmp/operation-terminal.log" "$tmp/operation-terminal.tsv")"
for want in \
  "operation_terminal_status:partial_answer_possible:expected:complete" \
  "operation_material_coverage_status:partial:expected:complete" \
  "operation_coverage_ref_missing:"; do
  if ! grep -qF "$want" <<<"$operation_reasons"; then
    fail "last typed operation terminal did not report $want: $operation_reasons"
  fi
done

cat >"$tmp/operation-terminal-multi.log" <<'LOG'
2026-08-04T05:04:28.387 INFO [cli/operation] command evaluation status=complete material_coverage_status=complete coverage_material_refs="material-coverage:v1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa:html_text | material-coverage:v1:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb:html_text" coverage_source_refs="/tmp/home.html | /tmp/user-guide.html" coverage_source_identities="sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa:bytes:42 | sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb:bytes:84" coverage_source_locators="argv:curl https://example.test/home.html | argv:curl https://example.test/user_guide.html" confidence="high" reason="complete" materials=2 rounds=2
LOG
EXPECT_OPERATION_TERMINAL_STATUS="complete"
EXPECT_OPERATION_MATERIAL_COVERAGE_STATUS="complete"
EXPECT_OPERATION_COVERAGE_REF_REGEX='^material-coverage:v1:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb:html_text$'
EXPECT_OPERATION_COVERAGE_SOURCE_REGEX='^argv:curl https://example\.test/user_guide\.html$'
operation_multi_reasons="$(eval_operation_terminal_reasons "$tmp/operation-terminal-multi.log" "$tmp/operation-terminal-multi.tsv")"
assert_eq "$operation_multi_reasons" "" "typed operation list members must be matched independently"

EXPECT_OPERATION_COVERAGE_REF_REGEX='^material-coverage:v1:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc:html_text$'
operation_multi_missing="$(eval_operation_terminal_reasons "$tmp/operation-terminal-multi.log" "$tmp/operation-terminal-multi-missing.tsv")"
if ! grep -q '^operation_coverage_ref_missing:' <<<"$operation_multi_missing"; then
  fail "typed operation member matching must retain the missing-ref negative arm: $operation_multi_missing"
fi
EXPECT_OPERATION_COVERAGE_REF_REGEX='^material-coverage:v1:[0-9a-f]{64}:html_text$'
EXPECT_OPERATION_COVERAGE_SOURCE_REGEX='user_guide\.html'
if ! grep -q $'complete\tpartial_answer_possible\tcomplete\tpartial\t' "$tmp/operation-terminal.tsv"; then
  fail "typed operation terminal receipt lost expected/actual authority"
fi
head -n 1 "$tmp/operation-terminal.log" >"$tmp/operation-complete.log"
assert_eq "$(eval_operation_terminal_reasons "$tmp/operation-complete.log" "$tmp/operation-complete.tsv")" "" \
  "complete final operation terminal with receipt"
sed 's#user_guide\.html#index.html#' "$tmp/operation-complete.log" >"$tmp/operation-wrong-source.log"
if ! eval_operation_terminal_reasons "$tmp/operation-wrong-source.log" "$tmp/operation-wrong-source.tsv" | \
  grep -qF "operation_coverage_source_missing:"; then
  fail "complete coverage for the wrong source locator must not satisfy the goal-material oracle"
fi
cat >"$tmp/operation-terminal.case" <<'CASE'
ID="operation_terminal_oracle"
NAME="operation terminal oracle"
QUESTION="operation terminal oracle"
EXPECT_OPERATION_TERMINAL_STATUS="complete"
EXPECT_OPERATION_COVERAGE_SOURCE_REGEX='user_guide\.html'
CASE
assert_eq "$(eval_case_oracle_surface "$tmp/operation-terminal.case")" "typed_operation_terminal" \
  "operation terminal case oracle surface"

cat >"$tmp/fake-codrax-operation-terminal" <<'SH'
#!/usr/bin/env bash
logdir=""
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "--log-dir" && $# -gt 1 ]]; then
    logdir="$2"
    shift 2
    continue
  fi
  shift
done
mkdir -p "$logdir"
printf '%s\n' '2026-08-04T05:03:38.697 INFO [cli/operation] command evaluation status=partial_answer_possible material_coverage_status=partial coverage_material_refs="" coverage_source_refs="" coverage_source_identities="" coverage_source_locators="" confidence="medium" reason="partial" materials=1 rounds=2' >"$logdir/codrax-fake.log"
echo 'operation terminal oracle answer with enough content'
SH
chmod +x "$tmp/fake-codrax-operation-terminal"
cat >"$tmp/operation-terminal-wire.case" <<'CASE'
ID="operation_terminal_wire"
NAME="operation terminal wire"
QUESTION="operation terminal wire"
EXPECT_CONTAINS="oracle"
EXPECT_OPERATION_TERMINAL_STATUS="complete"
EXPECT_OPERATION_MATERIAL_COVERAGE_STATUS="complete"
CASE
CODRAX_BIN="$tmp/fake-codrax-operation-terminal" EVAL_RESULTS_ROOT="$tmp/operation-terminal-results" CODRAX_PROVIDER_ARGS_RAW="" \
  eval/run.sh "$tmp/operation-terminal-wire.case" 1 >/dev/null || fail "operation terminal wire eval failed to run"
operation_wire_dir="$(eval_latest_result_dir "$tmp/operation-terminal-results" operation_terminal_wire 00000000-000000 || true)"
[[ -n "$operation_wire_dir" ]] || fail "operation terminal wire result dir missing"
if ! grep -qF "operation_terminal_status:partial_answer_possible:expected:complete" "$operation_wire_dir/run-1.verdict"; then
  fail "run.sh did not consume typed operation terminal verdict reasons"
fi
if [[ ! -f "$operation_wire_dir/run-1.operation-terminal.tsv" ]]; then
  fail "run.sh typed operation terminal receipt missing"
fi
unset EXPECT_OPERATION_TERMINAL_STATUS EXPECT_OPERATION_MATERIAL_COVERAGE_STATUS EXPECT_OPERATION_COVERAGE_REF_REGEX EXPECT_OPERATION_COVERAGE_SOURCE_REGEX

# --- run.sh OUTDIR exclusivity (same-case same-second collision guard) -----
# Two run.sh processes launched for the SAME case in the SAME second used to
# share ID-TS and race each other's run files (witnessed 2026-07-14: parallel
# same-case arms cross-contaminated verdicts / setup_fail). run.sh must claim
# an unused directory atomically; when the second-granular name is taken it
# suffixes .2, .3, … — simulate the collision by pre-claiming every name the
# invocation could pick in the next seconds.
cat >"$tmp/fake-codrax-outdir" <<'SH'
#!/usr/bin/env bash
echo '━━━'
echo 'outdir exclusivity probe answer with enough characters to pass'
SH
chmod +x "$tmp/fake-codrax-outdir"
cat >"$tmp/outdir-claim.case" <<'CASE'
ID="outdir_claim"
NAME="outdir claim"
QUESTION="outdir exclusivity test"
EXPECT_CONTAINS="probe"
CASE
mkdir -p "$tmp/outdir-results"
for pre_ts in $(python3 -c 'import time
for i in range(0, 20):
    print(time.strftime("%Y%m%d-%H%M%S", time.localtime(time.time() + i)))'); do
  mkdir -p "$tmp/outdir-results/outdir_claim-$pre_ts"
done
CODRAX_BIN="$tmp/fake-codrax-outdir" EVAL_RESULTS_ROOT="$tmp/outdir-results" CODRAX_PROVIDER_ARGS_RAW="" \
  eval/run.sh "$tmp/outdir-claim.case" 1 >/dev/null 2>&1 || fail "outdir claim eval failed to run"
outdir_suffixed="$(ls -dt "$tmp/outdir-results"/outdir_claim-*.2 2>/dev/null | head -1)"
[[ -n "$outdir_suffixed" ]] || fail "collided OUTDIR was not re-claimed with a .2 suffix"
assert_eq "$(cat "$outdir_suffixed/run-1.verdict")" "PASS" "suffixed OUTDIR run must complete normally"

# EVALGUARD §29.143: the run-count three-state contract (CLI > case
# N_DEFAULT > builtin 3) — a one-line resolver in run.sh that gained a
# case-level default; pin all three states without invoking the harness.
n_resolve() { local N_CLI="$1" N_DEFAULT="$2"; echo "${N_CLI:-${N_DEFAULT:-3}}"; }
assert_eq "$(n_resolve 5 2)" "5" "CLI N must win over case N_DEFAULT"
assert_eq "$(n_resolve "" 2)" "2" "case N_DEFAULT must win when CLI N absent"
assert_eq "$(n_resolve "" "")" "3" "builtin 3 must be the final fallback"

echo "ok eval runner contracts"
