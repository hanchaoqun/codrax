# Selected parallel eval sweep

- date: 2026-08-02T10:12:07Z
- sweep_start_ts: 20260802-031206
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | read_combo_analyze_retry_anchor | PASS | - | 160s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_analyze_retry_anchor-20260802-031207 |
| 1 | github_issue_dayjs_duration_nan_symptom | PASS | - | 190s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_dayjs_duration_nan_symptom-20260802-031207 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
