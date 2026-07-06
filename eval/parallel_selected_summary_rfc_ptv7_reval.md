# Selected parallel eval sweep

- date: 2026-07-06T04:58:09Z
- sweep_start_ts: 20260706-125809
- total cases: 3
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 1 | real_trace_c4_freq_supply_evidence | PASS | - | 83s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c4_freq_supply_evidence-20260706-125809 |
| 2 | real_trace_e1_dual_window_normalized | PASS | - | 166s | 2 | 1 | 0 | 3 | 2 | 0 | 2 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_e1_dual_window_normalized-20260706-125809 |
| 3 | real_trace_f1_exclude_no_code | PASS | - | 124s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_f1_exclude_no_code-20260706-125933 |

**Pass: 3 / 3 — Fail/Timeout/LaunchFail: 0**
