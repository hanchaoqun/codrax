# Selected parallel eval sweep

- date: 2026-10-09T01:35:21Z
- sweep_start_ts: 20261008-183520
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_rendering_native_proof_20261008

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | native_registration_commandless | FAIL | write_final_verdict:unverified:impact_targets_unverified | 98s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_rendering_native_proof_20261008/native_registration_commandless-20261008-183521 |
| 1 | trace_rendering_candidates | PASS | - | 240s | 1 | 2 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_rendering_native_proof_20261008/trace_rendering_candidates-20261008-183521 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
