# Selected parallel eval sweep

- date: 2026-09-21T04:41:42Z
- sweep_start_ts: 20260920-214141
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_marker_navigation_envelope_20260920

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_c_typo | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 124s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_marker_navigation_envelope_20260920/patch_c_typo-20260920-214142 |
| 1 | trace_query_business_marker_io_chain | FAIL | trace_final_projection_blocks:0_want_1 | 287s | 1 | 2 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_marker_navigation_envelope_20260920/trace_query_business_marker_io_chain-20260920-214142 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
