# Selected parallel eval sweep

- date: 2026-08-02T02:09:28Z
- sweep_start_ts: 20260801-190927
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | data_basic_sum_with_rules | PASS | - | 155s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_basic_sum_with_rules-20260801-190928 |
| 1 | data_multifile_reference_projection | PASS | - | 657s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260801-190928 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
