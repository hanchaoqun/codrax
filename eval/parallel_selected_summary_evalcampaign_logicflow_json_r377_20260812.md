# Selected parallel eval sweep

- date: 2026-08-12T09:12:05Z
- sweep_start_ts: 20260812-021203
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | data_json_strict_ids | PASS | - | 39s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260812-021205 |
| 1 | qf_logic_view_read_pipeline | FAIL | degraded_answer_checks_skipped:1 | 728s | 1 | 1 | 0 | 1 | 0 | 13 | 12 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260812-021205 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
