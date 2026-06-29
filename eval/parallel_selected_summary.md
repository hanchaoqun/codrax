# Selected parallel eval sweep

- date: 2026-06-29T01:52:00Z
- sweep_start_ts: 20260629-095200
- total cases: 6
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------------|
| 2 | arkts_repomap | PASS | - | 81s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | eval/results/arkts_repomap-20260629-095200 |
| 1 | cangjie_repomap | PASS | - | 114s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | eval/results/cangjie_repomap-20260629-095200 |
| 3 | qf_relation_subagent_registry | PASS | - | 75s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | eval/results/qf_relation_subagent_registry-20260629-095321 |
| 4 | read_combo_trace_current_source_explanation | PASS | - | 208s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | eval/results/read_combo_trace_current_source_explanation-20260629-095354 |
| 5 | read_combo_log_current_source_explanation | PASS | - | 175s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | eval/results/read_combo_log_current_source_explanation-20260629-095436 |
| 6 | sr_ts_workspace_impls | PASS | - | 77s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | eval/results/sr_ts_workspace_impls-20260629-095722 |

**Pass: 6 / 6 — Fail/Timeout/LaunchFail: 0**
