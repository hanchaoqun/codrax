# Selected parallel eval sweep

- date: 2026-06-29T06:43:36Z
- sweep_start_ts: 20260629-144336
- total cases: 6
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 2 | arkts_repomap | PASS | - | 111s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260629-144336 |
| 3 | qf_relation_subagent_registry | PASS | - | 96s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_relation_subagent_registry-20260629-144527 |
| 1 | cangjie_repomap | PASS | - | 250s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260629-144336 |
| 4 | read_combo_trace_current_source_explanation | PASS | - | 194s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage | eval/results/read_combo_trace_current_source_explanation-20260629-144704 |
| 5 | read_combo_log_current_source_explanation | PASS | - | 154s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/read_combo_log_current_source_explanation-20260629-144747 |
| 6 | trace_query_openharmony_bytrace_thread | PASS | - | 156s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_openharmony_bytrace_thread-20260629-145018 |

**Pass: 6 / 6 — Fail/Timeout/LaunchFail: 0**
