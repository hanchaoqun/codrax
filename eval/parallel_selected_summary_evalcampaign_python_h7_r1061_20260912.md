# Selected parallel eval sweep

- date: 2026-09-12T07:00:30Z
- sweep_start_ts: 20260912-000029
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | github_issue_tokenizers_newline_run_multirepo_py | PASS | - | 242s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260912-000030 |
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | - | 348s | 1 | 1 | 0 | 1 | 0 | 3 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h7_self_seat_full_spectrum-20260912-000030 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
