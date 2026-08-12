# Selected parallel eval sweep

- date: 2026-08-12T08:55:09Z
- sweep_start_ts: 20260812-015507
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | - | 286s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h7_self_seat_full_spectrum-20260812-015509 |
| 2 | qf_logic_view_read_pipeline | FAIL | degraded_answer_checks_skipped:1 | 421s | 1 | 1 | 0 | 1 | 0 | 9 | 8 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260812-015509 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
