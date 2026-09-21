# Selected parallel eval sweep

- date: 2026-09-21T03:56:26Z
- sweep_start_ts: 20260920-205626
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_measurement_owner_20260920

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 140s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_measurement_owner_20260920/trace_query_wakeup_causal_io_chain-20260920-205626 |
| 1 | trace_query_business_marker_io_chain | FAIL | missing_primary:LoadDocumentIndex no_primary_text_regex_match:((不可|不能|不应|不要|不再|避免).{0,60}(相加 | 250s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_measurement_owner_20260920/trace_query_business_marker_io_chain-20260920-205626 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
