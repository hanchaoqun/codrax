# Selected parallel eval sweep

- date: 2026-09-14T02:37:54Z
- sweep_start_ts: 20260913-193754
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_h6_channel_mixed_display | FAIL | missing:根因排序#1 missing:❶ | 189s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h6_channel_mixed_display-20260913-193754 |
| 2 | github_issue_fmt_tm_year_overflow_symptom | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 263s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_fmt_tm_year_overflow_symptom-20260913-193754 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
