# Selected parallel eval sweep

- date: 2026-08-06T16:37:27Z
- sweep_start_ts: 20260806-093726
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | arkts_repomap | PASS | - | 93s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260806-093727 |
| 1 | cangjie_repomap | PASS | - | 155s | 1 | 1 | 0 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260806-093727 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
