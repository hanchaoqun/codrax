# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T11:41:35Z
- sweep_start_ts: 20260820-044133
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260820-044135 | answer_regex,answer_contains | none | 204s | 33 | read=10,repo_map=2,list=0,trace=0,source_lens=0 | midloop=8,inv=2/0,fin_reject=2,unavail=0,prune=0 | partial | B1234 production-closed: the accepted analyzer carrier retains only concrete participant `LoopController`; the final diagram keeps all 12 exact model-authored `implements` edges and no longer emits the contradictory unproven boundary for `主要实现类型` or `LoopController`. No raw relation enum leaks. New B1235: the user-required file-location dimension has a table headed `文件位置`, but every value cell contains a responsibility description instead of the typed source path. Paths survive only in the diagram/citations. Current dimension/facet validation accepts `member_set` presence without checking that its typed requested field is rendered, so runner is a false positive on that surface. |
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-044135 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 345s | 40 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=2,inv=3/0,fin_reject=0,unavail=0,prune=0 | partial | Core Trace result is correct and complete: explicit 2.000..2.020s window, proved `threadpool-400 -> network-300 -> cookie-200 -> app-100` wake chain, 11.000ms chain iowait root, three 1.000ms scheduling-supply seats, actual-occupancy/rule-eliminable dual axes, causal projection, deterministic supplement, and adjacent/background demotion all survive. No active-stream timeout degradation occurred. One non-principal prose overclaim remains: the model calls background IO score 16.000 `偏高` even though the same typed legend says this composite score has no absolute pressure grade. Treat as model compliance variance/soft-context follow-up; do not scan or rewrite answer prose and do not make score wording a hard gate. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
