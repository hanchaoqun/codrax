# Selected parallel eval sweep

- date: 2026-08-22T14:51:52Z
- sweep_start_ts: 20260822-075150
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_py_registry_dispatch | PASS | - | 285s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/sr_py_registry_dispatch-20260822-075152 |
| 2 | sr_cpp_virtual_chain | FAIL | degraded_answer_checks_skipped:1 | 341s | 1 | 1 | 0 | 1 | 0 | 11 | 10 | 0 | 0 | 0 | none | eval/results/sr_cpp_virtual_chain-20260822-075152 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
