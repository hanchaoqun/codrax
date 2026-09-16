# Selected parallel eval sweep

- date: 2026-09-16T06:32:55Z
- sweep_start_ts: 20260915-233254
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | - | 210s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h7_self_seat_full_spectrum-20260915-233255 |
| 2 | sr_cpp_virtual_chain | PASS | - | 460s | 1 | 2 | 0 | 1 | 0 | 13 | 13 | 0 | 0 | 0 | none | eval/results/sr_cpp_virtual_chain-20260915-233255 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
