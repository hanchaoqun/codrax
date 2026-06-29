# Selected parallel eval sweep

- date: 2026-06-29T11:59:24Z
- sweep_start_ts: 20260629-195924
- total cases: 6
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 1 | cangjie_repomap | PASS | - | 143s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260629-195925 |
| 2 | arkts_repomap | PASS | - | 166s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260629-195925 |
| 3 | qf_relation_subagent_registry | PASS | - | 103s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_relation_subagent_registry-20260629-200149 |
| 4 | read_combo_trace_current_source_explanation | FAIL | no_regex_match:internal/[^[:space:]]+\.go:[0-9]+ | 173s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/read_combo_trace_current_source_explanation-20260629-200211 |
| 6 | patch_cpp_typo | PASS | - | 82s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_cpp_typo-20260629-200504 |
| 5 | read_combo_log_current_source_explanation | PASS | - | 215s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/read_combo_log_current_source_explanation-20260629-200332 |

**Pass: 5 / 6 — Fail/Timeout/LaunchFail: 1**
