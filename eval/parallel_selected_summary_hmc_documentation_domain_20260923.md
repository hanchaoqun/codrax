# Selected parallel eval sweep

- date: 2026-09-23T11:50:15Z
- sweep_start_ts: 20260923-045012
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_documentation_domain_20260923

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_capability_and_window_analysis | FAIL | answer_surface_receipt_missing trace_final_projection_blocks:0_want_1 no_log_regex:phase=toolcall .*tool=trace_capabilit | 86s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | missing_runtime_authority | eval/results/hmc_documentation_domain_20260923/trace_capability_and_window_analysis-20260923-045015 |
| 1 | trace_capability_discovery | PASS | - | 139s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_documentation_domain_20260923/trace_capability_discovery-20260923-045015 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
