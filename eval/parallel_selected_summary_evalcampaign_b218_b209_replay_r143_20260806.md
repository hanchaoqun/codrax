# Selected parallel eval sweep

- date: 2026-08-07T02:38:46Z
- sweep_start_ts: 20260806-193844
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_cpp_virtual_chain | PASS | - | 108s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/sr_cpp_virtual_chain-20260806-193846 |
| 2 | sr_py_registry_dispatch | PASS | - | 166s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/sr_py_registry_dispatch-20260806-193846 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
