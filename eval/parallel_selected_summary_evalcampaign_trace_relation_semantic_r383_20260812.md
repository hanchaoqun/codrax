# Selected parallel eval sweep

- date: 2026-08-12T11:29:24Z
- sweep_start_ts: 20260812-042923
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h10_spantop_member_subrows | PASS | - | 95s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h10_spantop_member_subrows-20260812-042925 |
| 1 | real_trace_h11_cross_direction_overlap | PASS | - | 156s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h11_cross_direction_overlap-20260812-042925 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
