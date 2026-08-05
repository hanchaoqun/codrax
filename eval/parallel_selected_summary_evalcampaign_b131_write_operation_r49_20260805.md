# Selected parallel eval sweep

- date: 2026-08-05T21:05:41Z
- sweep_start_ts: 20260805-140540
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | operation_system_inventory | PASS | - | 41s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/operation_system_inventory-20260805-140541 |
| 1 | patch_cpp_typo | PASS | - | 89s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_cpp_typo-20260805-140541 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
