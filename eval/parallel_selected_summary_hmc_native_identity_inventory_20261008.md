# Selected parallel eval sweep

- date: 2026-10-08T13:00:30Z
- sweep_start_ts: 20261008-060028
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_native_identity_inventory_20261008

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_static_initialize | PASS | - | 134s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_native_identity_inventory_20261008/trace_static_initialize-20261008-060030 |
| 2 | native_registration_commandless | FAIL | write_final_verdict:unverified:impact_targets_unverified | 159s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_native_identity_inventory_20261008/native_registration_commandless-20261008-060030 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
