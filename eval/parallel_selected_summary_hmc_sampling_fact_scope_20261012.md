# Selected parallel eval sweep

- date: 2026-10-12T11:34:32Z
- sweep_start_ts: 20261012-043430
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_sampling_fact_scope_20261012

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_lossless_sampling_tree | PASS | - | 182s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_sampling_fact_scope_20261012/trace_lossless_sampling_tree-20261012-043432 |
| 2 | trace_business_io_sequence | PASS | - | 978s | 2 | 1 | 0 | 1 | 0 | 12 | 12 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_sampling_fact_scope_20261012/trace_business_io_sequence-20261012-043432 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
