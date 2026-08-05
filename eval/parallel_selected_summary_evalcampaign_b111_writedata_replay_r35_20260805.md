# Selected parallel eval sweep

- date: 2026-08-05T13:31:31Z
- sweep_start_ts: 20260805-063130
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | github_issue_memoclaw_text_search_multirepo_ts | PASS | - | 179s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_memoclaw_text_search_multirepo_ts-20260805-063131 |
| 2 | data_multifile_reference_projection | PASS | - | 220s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260805-063131 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
