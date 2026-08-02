# Selected parallel eval sweep

- date: 2026-08-02T22:12:21Z
- sweep_start_ts: 20260802-151219
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | read_combo_log_current_code_dimensions | PASS | - | 258s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/read_combo_log_current_code_dimensions-20260802-151221 |
| 2 | github_issue_commons_lang_random_ascii_symptom | FAIL | write_report_failed write_final_verdict:unverified:verification_incomplete | 426s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_commons_lang_random_ascii_symptom-20260802-151221 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
