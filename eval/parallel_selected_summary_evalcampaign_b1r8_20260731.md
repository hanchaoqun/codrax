# Selected parallel eval sweep

- date: 2026-07-31T07:44:04Z
- sweep_start_ts: 20260731-004404
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_c2_dstate_iowait | FAIL | no_principal_regex_match:34579\.451701.*0\.138 ?(ms|毫秒) no_principal_regex_match:34579\.452934.*0\.147 ?(ms|毫秒)  | 132s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c2_dstate_iowait-20260731-004404 |
| 2 | github_issue_zod_prefault_symptom | PASS | - | 213s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_zod_prefault_symptom-20260731-004404 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
