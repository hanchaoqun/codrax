# Selected parallel eval sweep

- date: 2026-08-02T08:54:55Z
- sweep_start_ts: 20260802-015454
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_h11_cross_direction_overlap | PASS | - | 211s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h11_cross_direction_overlap-20260802-015455 |
| 2 | read_combo_member_set_closure_scope | PASS | - | 503s | 1 | 3 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_member_set_closure_scope-20260802-015455 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
