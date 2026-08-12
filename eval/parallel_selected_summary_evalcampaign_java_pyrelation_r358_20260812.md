# Selected parallel eval sweep

- date: 2026-08-12T03:54:39Z
- sweep_start_ts: 20260811-205436
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_java_annotation_route | PASS | - | 103s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_java_annotation_route-20260811-205439 |
| 2 | sr_py_registry_dispatch | PASS | - | 108s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_py_registry_dispatch-20260811-205439 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
