# Selected parallel eval sweep

- date: 2026-08-03T00:08:41Z
- sweep_start_ts: 20260802-170840
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | read_combo_log_current_code_boundary | PASS | - | 160s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/read_combo_log_current_code_boundary-20260802-170841 |
| 1 | github_issue_commons_lang_random_ascii_symptom | FAIL | write_report_missing write_final_run_status:blocked write_final_verdict:missing:missing | 313s | 1 | 2 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_commons_lang_random_ascii_symptom-20260802-170841 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
