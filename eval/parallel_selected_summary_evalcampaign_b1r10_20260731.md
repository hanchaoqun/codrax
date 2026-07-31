# Selected parallel eval sweep

- date: 2026-07-31T08:11:49Z
- sweep_start_ts: 20260731-011149
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_c2_dstate_iowait | FAIL | missing_principal:时长（ms） no_principal_text_regex_match:时长（ms）.*34579\.451701.*0\.138.*34579\.452934.*0\. | 155s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c2_dstate_iowait-20260731-011149 |
| 2 | github_issue_zod_prefault_symptom | PASS | - | 218s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_zod_prefault_symptom-20260731-011149 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
