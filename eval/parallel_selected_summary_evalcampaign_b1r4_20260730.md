# Selected parallel eval sweep

- date: 2026-07-31T05:26:02Z
- sweep_start_ts: 20260730-222602
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_c2_dstate_iowait | PASS | - | 134s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c2_dstate_iowait-20260730-222602 |
| 2 | github_issue_zod_prefault_symptom | PASS | - | 169s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_zod_prefault_symptom-20260730-222602 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
