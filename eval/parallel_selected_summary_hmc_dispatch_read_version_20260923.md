# Selected parallel eval sweep

- date: 2026-09-23T08:59:30Z
- sweep_start_ts: 20260923-015930
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_dispatch_read_version_20260923

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | empty_python_module_apply | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 298s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_dispatch_read_version_20260923/empty_python_module_apply-20260923-015930 |
| 2 | trace_capability_discovery | PASS | - | 313s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | none | eval/results/hmc_dispatch_read_version_20260923/trace_capability_discovery-20260923-015930 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
