# Selected parallel eval sweep

- date: 2026-09-20T11:20:31Z
- sweep_start_ts: 20260920-042031
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_witness_routing_io_plan_20260920

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_cpp_typo | PASS | - | 52s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_witness_routing_io_plan_20260920/patch_cpp_typo-20260920-042031 |
| 1 | trace_query_io_request_latency_distribution | FAIL | no_primary_regex_match:(^|[^0-9])11([^0-9]|$) no_primary_regex_match:(^|[^0-9])10[.]50*([^0-9]|$) no_primary_regex_match | 382s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_witness_routing_io_plan_20260920/trace_query_io_request_latency_distribution-20260920-042031 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
