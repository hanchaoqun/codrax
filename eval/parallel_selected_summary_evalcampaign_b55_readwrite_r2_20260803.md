# Selected parallel eval sweep

- date: 2026-08-04T01:35:04Z
- sweep_start_ts: 20260803-183503
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_sequence_analyzer_gate | PASS | - | 183s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260803-183504 |
| 2 | github_issue_commons_lang_random_ascii_symptom | FAIL | write_report_failed write_final_verdict:unverified:verification_incomplete | 393s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_commons_lang_random_ascii_symptom-20260803-183504 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
