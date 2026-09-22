# Selected parallel eval sweep

- date: 2026-09-22T13:00:43Z
- sweep_start_ts: 20260922-060043
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_pytest_invocation_20260922

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_dateutil_relativedelta_float | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 133s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_pytest_invocation_20260922/github_issue_dateutil_relativedelta_float-20260922-060044 |
| 1 | trace_query_jank_field_inventory | PASS | - | 337s | 2 | 1 | 0 | 1 | 0 | 1 | 2 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_pytest_invocation_20260922/trace_query_jank_field_inventory-20260922-060044 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

完整答案、上下文及交付审计见[人工审计](parallel_selected_summary_hmc_pytest_invocation_20260922_manual_audit.md)：完整人工0/2，不覆盖或重签上述机器结果。
