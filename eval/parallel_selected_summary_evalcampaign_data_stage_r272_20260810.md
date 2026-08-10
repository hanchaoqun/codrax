# Selected parallel eval sweep

- date: 2026-08-10T17:23:18Z
- sweep_start_ts: 20260810-102317
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | data_multifile_reference_projection | PASS | - | 430s | 0 | 0 | 0 | 0 | 3 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260810-102318 |
| 2 | qf_logic_view_read_pipeline | FAIL | degraded_answer_checks_skipped:1 | 571s | 1 | 2 | 0 | 1 | 0 | 10 | 9 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260810-102318 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
