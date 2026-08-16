# Selected parallel eval sweep

- date: 2026-08-16T21:59:28Z
- sweep_start_ts: 20260816-145926
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | - | 226s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h7_self_seat_full_spectrum-20260816-145928 |
| 2 | cangjie_repomap | FAIL | degraded_answer_checks_skipped:1 | 238s | 1 | 1 | 0 | 1 | 0 | 4 | 3 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260816-145928 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
