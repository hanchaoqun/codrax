# Selected parallel eval sweep

- date: 2026-08-01T01:10:50Z
- sweep_start_ts: 20260731-181049
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_h3_iofam_one_seat | PASS | - | 156s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h3_iofam_one_seat-20260731-181050 |
| 2 | real_trace_c2_dstate_iowait | PASS | - | 193s | 1 | 1 | 0 | 1 | 0 | 8 | 3 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c2_dstate_iowait-20260731-181050 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
