# Selected parallel eval sweep

- date: 2026-09-23T05:08:48Z
- sweep_start_ts: 20260922-220848
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_documentation_citation_20260923

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | empty_python_module_apply | FAIL | write_report_failed write_final_verdict:unverified:verification_incomplete | 287s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_documentation_citation_20260923/empty_python_module_apply-20260922-220848 |
| 1 | trace_capability_discovery | PASS | - | 398s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_documentation_citation_20260923/trace_capability_discovery-20260922-220848 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
