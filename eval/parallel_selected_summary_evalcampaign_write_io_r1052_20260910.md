# Selected parallel eval sweep

- date: 2026-09-10T09:42:38Z
- sweep_start_ts: 20260910-024237
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | github_issue_tokenizers_newline_run_multirepo_py | PASS | - | 249s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260910-024238 |
| 2 | real_trace_h3_iofam_one_seat | FAIL | no_regex_match:41\.329.*(非墙钟|不可相加|non.wall.clock|non.additive) | 319s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h3_iofam_one_seat-20260910-024238 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
