# Selected parallel eval sweep

- date: 2026-08-09T10:10:40Z
- sweep_start_ts: 20260809-031039
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_logic_view_read_pipeline | PASS | - | 210s | 1 | 1 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260809-031040 |
| 2 | data_basic_sum_with_rules | PASS | - | 264s | 0 | 0 | 0 | 0 | 4 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_basic_sum_with_rules-20260809-031040 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
