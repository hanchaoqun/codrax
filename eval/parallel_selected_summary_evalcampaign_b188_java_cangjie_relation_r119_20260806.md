# Selected parallel eval sweep

- date: 2026-08-06T19:22:26Z
- sweep_start_ts: 20260806-122225
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | cangjie_repomap_fixture | PASS | - | 66s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap_fixture-20260806-122226 |
| 1 | sr_java_handler_impls | PASS | - | 130s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_java_handler_impls-20260806-122226 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
