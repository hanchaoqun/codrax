# Selected parallel eval sweep

- date: 2026-10-08T09:31:25Z
- sweep_start_ts: 20261008-023123
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_catalog_registration_20261008

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_catalog_objects | PASS | - | 248s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | none | eval/results/hmc_catalog_registration_20261008/trace_catalog_objects-20261008-023126 |
| 2 | native_registration_commandless | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 283s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_catalog_registration_20261008/native_registration_commandless-20261008-023126 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
