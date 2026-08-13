# Selected parallel eval sweep

- date: 2026-08-13T06:49:41Z
- sweep_start_ts: 20260812-234940
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | data_jsonl_filter_count | PASS | - | 35s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_jsonl_filter_count-20260812-234941 |
| 2 | data_json_strict_ids | PASS | - | 39s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260812-234941 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
