# Selected parallel eval sweep

- date: 2026-08-12T23:53:42Z
- sweep_start_ts: 20260812-165340
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | - | 133s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h7_self_seat_full_spectrum-20260812-165342 |
| 1 | github_issue_zod_prefault | FAIL | write_final_verdict:unverified:production_verification_source_static_only | 198s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_zod_prefault-20260812-165342 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
