# Selected parallel eval sweep

- date: 2026-08-04T02:28:14Z
- sweep_start_ts: 20260803-192813
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_sequence_analyzer_gate | PASS | - | 385s | 1 | 1 | 0 | 1 | 0 | 6 | 5 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260803-192814 |
| 2 | github_issue_commons_lang_random_ascii_symptom | FAIL | write_report_failed write_final_verdict:unverified:verification_incomplete | 535s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_commons_lang_random_ascii_symptom-20260803-192814 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
