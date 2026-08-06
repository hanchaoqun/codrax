# Selected parallel eval sweep

- date: 2026-08-06T22:28:27Z
- sweep_start_ts: 20260806-152825
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_rust_cross_module_chain | PASS | - | 125s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/sr_rust_cross_module_chain-20260806-152827 |
| 2 | sr_cpp_virtual_chain | PASS | - | 172s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/sr_cpp_virtual_chain-20260806-152827 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
