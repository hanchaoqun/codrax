# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T04:26:49Z
- sweep_start_ts: 20260801-212648
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | logtri_goroutine_dump | PASS | eval/results/logtri_goroutine_dump-20260801-212649 | log_attachment,answer_regex | log_triage | 87s | 18 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Exact error-message gate reduced `errors[]` to one, but a failure-severity runtime observation and downstream synthesis still promoted goroutines 87/120 from snapshots to crash causes. |
| 2 | logtri_java | PASS | eval/results/logtri_java-20260801-212650 | log_attachment,answer_regex,answer_contains | log_triage | 522s | 18 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | uncertain | Three explicit `Caused by` messages/types survived correctly, but the answer inferred return-null/wrapping mechanics beyond the stack. `emit_log_triage` itself blocked ~444s because each Java basename repeatedly walked the artifact-heavy repository. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
