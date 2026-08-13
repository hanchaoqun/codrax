# Selected parallel eval sweep

- date: 2026-08-13T05:56:11Z
- sweep_start_ts: 20260812-225609
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | data_jsonl_filter_count | PASS | - | 171s | 0 | 0 | 0 | 0 | 2 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_jsonl_filter_count-20260812-225611 |
| 1 | data_json_strict_ids | PASS | - | 198s | 0 | 0 | 0 | 0 | 2 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260812-225611 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
