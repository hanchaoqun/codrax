# Selected parallel eval sweep

- date: 2026-08-10T09:28:25Z
- sweep_start_ts: 20260810-022824
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | data_multifile_reference_projection | FAIL | no_regex_match:^[[:space:]]*17[[:space:]]*,[[:space:]]*0[[:space:]]*,[[:space:]]*5[[:space:]]*$ | 605s | 0 | 0 | 0 | 0 | 3 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260810-022825 |
| 2 | qf_logic_view_read_pipeline | PASS | - | 758s | 1 | 1 | 0 | 2 | 1 | 14 | 15 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260810-022825 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
