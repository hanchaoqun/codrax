# Selected parallel eval sweep

- date: 2026-08-05T23:43:18Z
- sweep_start_ts: 20260805-164315
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | cangjie_repomap | PASS | - | 99s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260805-164318 |
| 2 | arkts_repomap | PASS | - | 139s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260805-164318 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
