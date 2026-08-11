# Selected parallel eval sweep

- date: 2026-08-11T19:06:14Z
- sweep_start_ts: 20260811-120613
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | github_issue_gson_lazy_number_symptom | FAIL | write_report_failed write_final_verdict:unverified:runner_missing | 124s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_gson_lazy_number_symptom-20260811-120614 |
| 2 | qf_logic_view_read_pipeline | FAIL | degraded_answer_checks_skipped:1 | 743s | 1 | 1 | 0 | 1 | 0 | 19 | 18 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260811-120614 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
