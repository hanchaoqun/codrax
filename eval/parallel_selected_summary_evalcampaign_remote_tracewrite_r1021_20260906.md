# Selected parallel eval sweep

- date: 2026-09-06T07:01:05Z
- sweep_start_ts: 20260906-000101
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_fmt_tm_year_overflow_symptom | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 146s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_fmt_tm_year_overflow_symptom-20260906-000106 |
| 1 | real_trace_h11_cross_direction_overlap | FAIL | read_exit:2 trace_final_projection_blocks:0_want_1 no_regex_match:12\.658ms.*IO阻塞.*共47段 | 199s | 1 | 1 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h11_cross_direction_overlap-20260906-000106 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
