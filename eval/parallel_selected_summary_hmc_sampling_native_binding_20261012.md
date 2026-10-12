# Selected parallel eval sweep

- date: 2026-10-12T12:49:27Z
- sweep_start_ts: 20261012-054925
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_sampling_native_binding_20261012

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_lossless_sampling_tree | PASS | - | 174s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_sampling_native_binding_20261012/trace_lossless_sampling_tree-20261012-054927 |
| 2 | trace_business_io_sequence | PASS | - | 708s | 1 | 1 | 0 | 1 | 0 | 16 | 16 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_sampling_native_binding_20261012/trace_business_io_sequence-20261012-054927 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
