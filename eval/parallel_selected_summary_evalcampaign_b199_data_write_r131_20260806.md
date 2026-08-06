# Selected parallel eval sweep

- date: 2026-08-06T22:10:32Z
- sweep_start_ts: 20260806-151030
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | data_jsonl_filter_count | PASS | - | 66s | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_jsonl_filter_count-20260806-151032 |
| 2 | patch_c_typo | PASS | - | 123s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_c_typo-20260806-151032 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
