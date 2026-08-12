# Selected parallel eval sweep

- date: 2026-08-12T04:09:48Z
- sweep_start_ts: 20260811-210945
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_java_call_chain | PASS | - | 117s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_java_call_chain-20260811-210948 |
| 1 | sr_py_registry_dispatch | PASS | - | 140s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_py_registry_dispatch-20260811-210948 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
