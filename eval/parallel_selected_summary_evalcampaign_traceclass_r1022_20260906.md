# Selected parallel eval sweep

- date: 2026-09-06T07:20:53Z
- sweep_start_ts: 20260906-002042
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_type_relation_loop_controller | PASS | - | 182s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260906-002053 |
| 1 | real_trace_h11_cross_direction_overlap | PASS | - | 451s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h11_cross_direction_overlap-20260906-002053 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
