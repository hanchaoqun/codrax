# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T15:30:13Z
- sweep_start_ts: 20260815-083011
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260815-083013 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 337s | 29 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | Typed state ledger, CPU1 placement, 0.800ms runnable seat, 5.000ms VerifyClass occupancy/business clue, dual axes and Trace causal projection survived. However analyzer reused the exact typed time-window quote as enumeration_boundary count=7, forcing four irrelevant emit_answer_symbol rounds and a false final “enumeration incomplete” supplement; B839 filed and fixed after this run. No fixed-age stream degradation. |
| 1 | github_issue_dateutil_relativedelta_float_symptom | FAIL | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260815-083013 | write_apply,write_patch_oracle | none | 543s | 24 | read=5,repo_map=1,list=1,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=1,prune=0 | fail | First patch was correctly rejected by 4/4 project-test failures and replan moved normalization into __init__. The retained second applied tree passes python3 -m unittest discover -v 4/4 manually, but five model probes called the module as a class; their typed probe-authoring parser errors suppressed the available project suite and final delivery stayed unverified. B838-A filed and fixed after this run. No evidence B837 relaxed a real conflict. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
