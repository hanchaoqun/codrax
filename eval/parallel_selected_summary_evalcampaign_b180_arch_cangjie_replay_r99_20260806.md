# Selected parallel eval sweep

- date: 2026-08-06T13:58:53Z
- sweep_start_ts: 20260806-065851
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | cangjie_repomap | PASS | - | 144s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260806-065853 |
| 1 | qf_architecture | PASS | - | 569s | 2 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_architecture-20260806-065853 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
