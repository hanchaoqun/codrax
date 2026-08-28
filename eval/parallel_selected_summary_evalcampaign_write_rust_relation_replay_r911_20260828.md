# Selected parallel eval sweep

- date: 2026-08-28T23:15:55Z
- sweep_start_ts: 20260828-161554
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | patch_python_typo | PASS | - | 47s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_python_typo-20260828-161555 |
| 2 | sr_rust_cross_module_chain | PASS | - | 178s | 1 | 2 | 0 | 1 | 0 | 3 | 2 | 0 | 0 | 0 | none | eval/results/sr_rust_cross_module_chain-20260828-161555 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
