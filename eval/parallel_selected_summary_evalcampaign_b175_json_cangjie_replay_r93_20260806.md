# Selected parallel eval sweep

- date: 2026-08-06T11:24:15Z
- sweep_start_ts: 20260806-042414
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | data_json_strict_ids | PASS | - | 50s | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260806-042415 |
| 2 | cangjie_repomap | FAIL | inventory_count_mismatch:extend:got3:want2 | 214s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260806-042415 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
