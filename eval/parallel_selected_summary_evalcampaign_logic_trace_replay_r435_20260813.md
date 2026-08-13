# Selected parallel eval sweep

- date: 2026-08-13T12:36:45Z
- sweep_start_ts: 20260813-053644
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | - | 164s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h7_self_seat_full_spectrum-20260813-053645 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 210s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260813-053645 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
