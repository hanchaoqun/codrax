# Selected parallel eval sweep

- date: 2026-08-10T13:50:27Z
- sweep_start_ts: 20260810-065025
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | data_multifile_reference_projection | PASS | - | 357s | 0 | 0 | 0 | 0 | 4 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260810-065027 |
| 2 | qf_logic_view_read_pipeline | PASS | - | 619s | 1 | 1 | 0 | 1 | 0 | 7 | 7 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260810-065027 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
