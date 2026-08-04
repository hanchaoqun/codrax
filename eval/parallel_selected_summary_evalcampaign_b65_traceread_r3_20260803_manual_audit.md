# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T06:47:34Z
- sweep_start_ts: 20260803-234732
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_command_current_source_explanation | PASS | eval/results/read_combo_command_current_source_explanation-20260803-234734 | answer_regex | none | 130s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Required route+operation axes said hybrid/investigate/current-source-required, but optional operation_kind=computer_operation still diverted the turn to operation. The first unquoted find failed; broad grep then guessed a wrong/incomplete evidence path and the four-stage source pipeline never ran. |
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260803-234734 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 140s | 33 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Model retained the 20.000ms sleep, 11.000ms fscache IO root, wakeup chain, dual dimensions and next steps. However both model queries widened the typed user window 2.000..2.020 to 2.000..2.021; family-presence suppression then skipped the exact-window root/rank supplement, so the system projection and principal_state published a 21ms window. The removed model-aggregate compact supplement did not recur. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
