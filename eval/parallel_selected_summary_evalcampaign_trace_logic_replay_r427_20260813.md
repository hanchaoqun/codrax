# Selected parallel eval sweep

- date: 2026-08-13T07:23:22Z
- sweep_start_ts: 20260813-002321
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | - | 259s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h7_self_seat_full_spectrum-20260813-002323 |
| 2 | qf_logic_view_read_pipeline | PASS | - | 474s | 1 | 2 | 0 | 1 | 0 | 5 | 4 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260813-002323 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
