# Selected parallel eval sweep

- date: 2026-09-02T08:11:05Z
- sweep_start_ts: 20260902-011104
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_fmt_tm_year_overflow_symptom | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 174s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_fmt_tm_year_overflow_symptom-20260902-011105 |
| 1 | real_trace_h11_cross_direction_overlap | PASS | - | 206s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h11_cross_direction_overlap-20260902-011105 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
