# Selected parallel eval sweep

- date: 2026-08-05T22:34:47Z
- sweep_start_ts: 20260805-153445
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | operation_system_inventory | PASS | - | 37s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/operation_system_inventory-20260805-153447 |
| 1 | data_json_strict_ids | FAIL | no_regex_match:"ids" no_regex_match:"u1" no_regex_match:"u3" | 355s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260805-153447 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
