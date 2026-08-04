# Selected parallel eval sweep

- date: 2026-08-04T16:00:56Z
- sweep_start_ts: 20260804-090054
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | cangjie_repomap_fixture | PASS | - | 64s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap_fixture-20260804-090056 |
| 2 | arkts_repomap | PASS | - | 142s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260804-090056 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
