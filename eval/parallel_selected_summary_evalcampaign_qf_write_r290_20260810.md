# Selected parallel eval sweep

- date: 2026-08-11T03:25:11Z
- sweep_start_ts: 20260810-202509
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_python_typo | PASS | - | 66s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_python_typo-20260810-202511 |
| 1 | qf_logic_view_read_pipeline | FAIL | missing:Mutable | 699s | 1 | 1 | 0 | 1 | 0 | 6 | 6 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260810-202511 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
