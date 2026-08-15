# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T16:04:19Z
- sweep_start_ts: 20260815-090418
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260815-090419 | log_regex,answer_regex | none | 250s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Exact `{"ids":["u1","u3"]}`; typed rule/contribution/reconcile/final projection all completed. Two planner repairs were disclosed. The first removed a script from `extract_records`; the emitted tool schema already says script is required only for `custom_transform` and conditionally forbids it on every typed action, so this was a model violation correctly repaired rather than a contradictory system contract. |
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260815-090419 | answer_regex,answer_contains | none | 298s | 33 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | Final answer showed `buildAnalysisIR` fan-out and `buildAnalysisIR -> gate.RunWith`, but omitted the explicitly required `gate.Run` participant and the real convergence `buildAnalysisIR -> gate.RunWith <- gate.Run`. Analyzer retained the typed participant after repairing an invalid discover-path endpoint shape, yet participant evidence/final closure was disabled for source-call-chain `IntentTrace + AxisCall`. B841 fixes this enum-lane collision without creating edges or touching runtime Trace causal authority. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
