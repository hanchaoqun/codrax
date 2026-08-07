# Selected parallel eval sweep

- date: 2026-08-07T08:42:11Z
- sweep_start_ts: 20260807-014209
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_cpp_virtual_chain | FAIL | no_regex_match:(virtual|虚函数|虚调用|动态分发|多态|dispatch) | 139s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_cpp_virtual_chain-20260807-014211 |
| 2 | sr_py_registry_dispatch | PASS | - | 161s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/sr_py_registry_dispatch-20260807-014211 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
