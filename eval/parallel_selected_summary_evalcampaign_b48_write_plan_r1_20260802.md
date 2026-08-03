# Selected parallel eval sweep

- date: 2026-08-03T00:41:40Z
- sweep_start_ts: 20260802-174138
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_cpp_typo | PASS | - | 56s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_cpp_typo-20260802-174140 |
| 1 | github_issue_commons_lang_random_ascii_symptom | FAIL | write_report_failed write_final_run_status:blocked write_final_verdict:missing:missing | 319s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_commons_lang_random_ascii_symptom-20260802-174140 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
