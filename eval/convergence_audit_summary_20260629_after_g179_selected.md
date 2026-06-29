# Selected parallel eval sweep

- date: 2026-06-29T11:19:22Z
- sweep_start_ts: 20260629-191922
- total cases: 6
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 2 | arkts_repomap | PASS | - | 98s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260629-191922 |
| 1 | cangjie_repomap | PASS | - | 121s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260629-191922 |
| 3 | qf_relation_subagent_registry | PASS | - | 118s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_relation_subagent_registry-20260629-192100 |
| 4 | trace_query_openharmony_bytrace_thread | PASS | - | 171s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_openharmony_bytrace_thread-20260629-192124 |
| 5 | read_combo_trace_current_source_explanation | PASS | - | 153s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage | eval/results/read_combo_trace_current_source_explanation-20260629-192259 |
| 6 | patch_go_typo | PASS | - | 118s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_go_typo-20260629-192415 |

**Pass: 6 / 6 — Fail/Timeout/LaunchFail: 0**
