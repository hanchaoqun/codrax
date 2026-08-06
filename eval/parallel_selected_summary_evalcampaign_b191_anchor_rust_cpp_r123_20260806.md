# Selected parallel eval sweep

- date: 2026-08-06T20:18:19Z
- sweep_start_ts: 20260806-131817
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_cpp_sink_impls | PASS | - | 81s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_cpp_sink_impls-20260806-131819 |
| 1 | sr_rust_trait_impls | PASS | - | 84s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_rust_trait_impls-20260806-131819 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
