# Selected parallel eval sweep

- date: 2026-09-21T08:48:13Z
- sweep_start_ts: 20260921-014813
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_dependency_role_crossmode_20260921

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_dayjs_duration_nan_symptom | FAIL | write_report_failed write_final_verdict:unverified:runner_missing | 136s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_dependency_role_crossmode_20260921/github_issue_dayjs_duration_nan_symptom-20260921-014813 |
| 1 | trace_query_wakeup_background_demotion | FAIL | no_regex_match:(D-state|D 状态|iowait|io_wait|fscache_page_wait_on_page_bit).*(threadpool-400) | 234s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_dependency_role_crossmode_20260921/trace_query_wakeup_background_demotion-20260921-014813 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
