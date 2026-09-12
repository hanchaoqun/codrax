# Selected parallel eval sweep

- date: 2026-09-12T11:07:35Z
- sweep_start_ts: 20260912-040735
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_e1_dual_window_normalized | PASS | - | 109s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_e1_dual_window_normalized-20260912-040736 |
| 2 | sr_py_registry_dispatch | FAIL | degraded_answer_checks_skipped:1 | 424s | 1 | 2 | 0 | 1 | 0 | 14 | 13 | 0 | 0 | 0 | none | eval/results/sr_py_registry_dispatch-20260912-040736 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
