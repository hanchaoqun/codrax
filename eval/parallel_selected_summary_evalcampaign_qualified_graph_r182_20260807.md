# Selected parallel eval sweep

- date: 2026-08-07T19:05:27Z
- sweep_start_ts: 20260807-120525
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_rust_cross_module_chain | PASS | - | 149s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_rust_cross_module_chain-20260807-120527 |
| 2 | sr_cpp_virtual_chain | PASS | - | 300s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_cpp_virtual_chain-20260807-120527 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
