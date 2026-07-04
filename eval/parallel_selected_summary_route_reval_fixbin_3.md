# Selected parallel eval sweep

- date: 2026-07-04T20:03:52Z
- sweep_start_ts: 20260705-040352
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 1 | data_json_strict_ids | PASS | - | 62s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260705-040352 |
| 2 | data_text_filter_count | FAIL | no_regex_match:^2[[:space:]]*$ | 222s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_text_filter_count-20260705-040352 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
