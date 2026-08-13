# Selected parallel eval sweep

- date: 2026-08-13T06:20:03Z
- sweep_start_ts: 20260812-232001
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | data_json_strict_ids | PASS | - | 46s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260812-232003 |
| 2 | data_jsonl_filter_count | FAIL | no_regex_match:^2[[:space:]]*$ | 166s | 0 | 0 | 0 | 0 | 3 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_jsonl_filter_count-20260812-232003 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
