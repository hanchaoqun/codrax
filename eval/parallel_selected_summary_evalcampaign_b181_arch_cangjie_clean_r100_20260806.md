# Selected parallel eval sweep

- date: 2026-08-06T14:18:23Z
- sweep_start_ts: 20260806-071822
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | cangjie_repomap | PASS | - | 130s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260806-071823 |
| 1 | qf_architecture | PASS | - | 517s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_architecture-20260806-071823 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
