# Selected parallel eval sweep

- date: 2026-09-11T02:00:08Z
- sweep_start_ts: 20260910-190007
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_dayjs_duration_nan_symptom | FAIL | write_report_failed write_final_verdict:unverified:runner_missing | 120s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_dayjs_duration_nan_symptom-20260910-190008 |
| 1 | read_combo_pipeline_sequence_table | PASS | - | 625s | 1 | 2 | 0 | 1 | 0 | 10 | 10 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260910-190008 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
