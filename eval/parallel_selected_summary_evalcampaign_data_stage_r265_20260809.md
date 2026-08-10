# Selected parallel eval sweep

- date: 2026-08-10T05:30:26Z
- sweep_start_ts: 20260809-223025
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_logic_view_read_pipeline | PASS | - | 343s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260809-223026 |
| 1 | data_multifile_reference_projection | PASS | - | 502s | 0 | 0 | 0 | 0 | 5 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260809-223026 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
