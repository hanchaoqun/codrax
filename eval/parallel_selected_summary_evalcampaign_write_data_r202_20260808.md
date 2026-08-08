# Selected parallel eval sweep

- date: 2026-08-08T09:08:21Z
- sweep_start_ts: 20260808-020820
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | data_text_filter_count | PASS | - | 36s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_text_filter_count-20260808-020821 |
| 1 | patch_python_typo | PASS | - | 59s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_python_typo-20260808-020821 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
