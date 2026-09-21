# Selected parallel eval sweep

- date: 2026-09-21T11:48:32Z
- sweep_start_ts: 20260921-044832
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_native_identity_crossmode_20260921

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | nested_python_increment | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 237s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_native_identity_crossmode_20260921/nested_python_increment-20260921-044832 |
| 2 | trace_query_business_marker_io_chain | PASS | - | 253s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_native_identity_crossmode_20260921/trace_query_business_marker_io_chain-20260921-044832 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
