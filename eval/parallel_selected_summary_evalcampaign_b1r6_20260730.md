# Selected parallel eval sweep

- date: 2026-07-31T06:44:04Z
- sweep_start_ts: 20260730-234404
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_c2_dstate_iowait | FAIL | no_principal_regex_match:34579\.471372.*0\.350 ?(ms|毫秒) no_principal_text_regex_match:34579\.451701.*0\.138 ?(ms|毫 | 128s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c2_dstate_iowait-20260730-234404 |
| 2 | github_issue_zod_prefault_symptom | PASS | - | 231s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_zod_prefault_symptom-20260730-234404 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
