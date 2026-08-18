# Selected parallel eval sweep

- date: 2026-08-18T08:34:27Z
- sweep_start_ts: 20260818-013426
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_c_platform_fork | PASS | - | 156s | 1 | 1 | 0 | 1 | 0 | 3 | 2 | 0 | 0 | 0 | none | eval/results/sr_c_platform_fork-20260818-013427 |
| 2 | github_issue_chrono_duration_min_symptom | FAIL | worktree_discarded_or_missing write_report_missing write_final_run_status:in_progress write_final_verdict:missing:missin | 388s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_chrono_duration_min_symptom-20260818-013427 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
