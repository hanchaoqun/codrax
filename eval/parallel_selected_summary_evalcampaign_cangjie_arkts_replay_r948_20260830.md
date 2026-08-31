# Selected parallel eval sweep

- date: 2026-08-31T02:53:00Z
- sweep_start_ts: 20260830-195258
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | arkts_repomap | PASS | - | 190s | 1 | 3 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260830-195300 |
| 1 | cangjie_repomap | FAIL | inventory_count_mismatch:extend:got12:want2 inventory_count_mismatch:foreign_func:got12:want2 inventory_count_mismatch:p | 262s | 1 | 3 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260830-195300 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
