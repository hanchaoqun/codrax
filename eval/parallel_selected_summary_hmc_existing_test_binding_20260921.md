# Selected parallel eval sweep

- date: 2026-09-21T07:46:39Z
- sweep_start_ts: 20260921-004637
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_existing_test_binding_20260921

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 192s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_existing_test_binding_20260921/trace_query_wakeup_causal_io_chain-20260921-004639 |
| 1 | github_issue_dateutil_relativedelta_float_symptom | PASS | - | 235s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_existing_test_binding_20260921/github_issue_dateutil_relativedelta_float_symptom-20260921-004639 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
