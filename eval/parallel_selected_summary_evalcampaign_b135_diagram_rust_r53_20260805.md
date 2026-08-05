# Selected parallel eval sweep

- date: 2026-08-05T21:51:11Z
- sweep_start_ts: 20260805-145109
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_rust_trait_impls | FAIL | no_regex_match:(--fixed|fixed) | 87s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_rust_trait_impls-20260805-145111 |
| 1 | qf_diagram_pipeline | PASS | - | 133s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_diagram_pipeline-20260805-145111 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
