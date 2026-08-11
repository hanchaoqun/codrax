# Selected parallel eval sweep

- date: 2026-08-11T18:22:54Z
- sweep_start_ts: 20260811-112253
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_py_registry_dispatch | PASS | - | 149s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/sr_py_registry_dispatch-20260811-112255 |
| 1 | sr_cpp_virtual_chain | PASS | - | 319s | 1 | 1 | 0 | 1 | 0 | 5 | 5 | 0 | 0 | 0 | none | eval/results/sr_cpp_virtual_chain-20260811-112254 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
