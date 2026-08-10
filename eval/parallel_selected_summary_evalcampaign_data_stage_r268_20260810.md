# Selected parallel eval sweep

- date: 2026-08-10T11:07:01Z
- sweep_start_ts: 20260810-040700
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_logic_view_read_pipeline | PASS | - | 206s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260810-040702 |
| 1 | data_multifile_reference_projection | FAIL | no_regex_match:^[[:space:]]*17[[:space:]]*,[[:space:]]*0[[:space:]]*,[[:space:]]*5[[:space:]]*$ | 571s | 0 | 0 | 0 | 0 | 4 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260810-040702 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
