# Selected parallel eval sweep

- date: 2026-09-02T01:45:17Z
- sweep_start_ts: 20260901-184515
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_py_registry_dispatch | PASS | - | 140s | 1 | 1 | 0 | 1 | 0 | 3 | 4 | 0 | 0 | 0 | none | eval/results/sr_py_registry_dispatch-20260901-184517 |
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | - | 220s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h7_self_seat_full_spectrum-20260901-184517 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
