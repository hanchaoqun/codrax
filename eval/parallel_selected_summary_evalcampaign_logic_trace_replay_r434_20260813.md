# Selected parallel eval sweep

- date: 2026-08-13T12:10:28Z
- sweep_start_ts: 20260813-051025
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | - | 335s | 1 | 1 | 0 | 1 | 0 | 2 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h7_self_seat_full_spectrum-20260813-051028 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 437s | 1 | 1 | 0 | 1 | 0 | 7 | 7 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260813-051028 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
