# Selected parallel eval sweep

- date: 2026-08-05T22:17:44Z
- sweep_start_ts: 20260805-151742
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_rust_trait_impls | PASS | - | 78s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_rust_trait_impls-20260805-151744 |
| 2 | sr_java_handler_impls | PASS | - | 104s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_java_handler_impls-20260805-151744 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
