# Selected parallel eval sweep

- date: 2026-08-07T06:41:05Z
- sweep_start_ts: 20260806-234103
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_py_registry_dispatch | PASS | - | 116s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_py_registry_dispatch-20260806-234105 |
| 1 | sr_cpp_virtual_chain | PASS | - | 142s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_cpp_virtual_chain-20260806-234105 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
