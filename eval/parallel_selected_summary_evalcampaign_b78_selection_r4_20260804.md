# Selected parallel eval sweep

- date: 2026-08-04T13:38:17Z
- sweep_start_ts: 20260804-063816
- total cases: 2
- parallel: 2
- timeout: 900s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | data_text_filter_count | PASS | - | 33s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_text_filter_count-20260804-063817 |
| 2 | data_jsonl_filter_count | PASS | - | 96s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_jsonl_filter_count-20260804-063817 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
