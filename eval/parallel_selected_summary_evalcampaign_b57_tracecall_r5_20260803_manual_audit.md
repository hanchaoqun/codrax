# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T03:15:44Z
- sweep_start_ts: 20260803-201542
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260803-201544 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 142s | 33 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | uncertain | Core typed surfaces pass: explicit 1.000–1.010s window, 3 target/window-filtered trace queries, worker-200→app-100 wakeup, 10.000ms target partition, 9.000ms chain cumulative, 8.300ms effective elimination, two-axis principal table, deterministic projection and supplement. Model prose nevertheless says the target blocked 10.020ms (artifact span, not selected window) and was “completely” caused by the 8.300ms runnable slice. The injected context contains the correct distinct values and non-addition boundary, so file as model-watch; do not add prose keyword gates. |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260803-201544 | answer_regex,answer_contains | none | 315s | 27 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=8,inv=1/0,fin_reject=2,unavail=0,prune=0 | fail | Analyzer emitted schema-valid `call_chain + AxisCall + exactly two symbol endpoints`, but `IsRelationalLookup=false/IsCrossComponent=true`; B57's fallback unnecessarily required the redundant relational boolean and skipped reachability. Final answer invents an external caller flowing `AnalysisIR` into `gate.Run`; source proves `buildAnalysisIR -> gate.RunWith` and the reverse wrapper `gate.Run -> gate.RunWith`. Implemented equivalent typed-encoding coverage as `EVAL-B58-CALLENC1`. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
