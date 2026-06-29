# Selected parallel eval sweep

- date: 2026-06-29T13:05:52Z
- sweep_start_ts: 20260629-210552
- total cases: 6
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 1 | qf_relation_subagent_registry | PASS | - | 90s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_relation_subagent_registry-20260629-210552 |
| 2 | arkts_repomap | PASS | - | 194s | 1 | 1 | 0 | 1 | 0 | 2 | 1 | 0 | 0 | none | eval/results/arkts_repomap-20260629-210552 |
| 3 | cangjie_repomap | FAIL | missing_inventory_row:foreign_func:native_add_07_foreign_ffi.cj_demo.ffi inventory_count_mismatch:foreign_func:got1:want | 139s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260629-210723 |
| 4 | trace_query_openharmony_bytrace_thread | PASS | - | 121s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_openharmony_bytrace_thread-20260629-210907 |
| 5 | read_combo_log_current_source_explanation | PASS | - | 183s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/read_combo_log_current_source_explanation-20260629-210943 |
| 6 | read_combo_trace_current_source_explanation | PASS | - | 137s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage | eval/results/read_combo_trace_current_source_explanation-20260629-211109 |

**Pass: 5 / 6 — Fail/Timeout/LaunchFail: 1**
