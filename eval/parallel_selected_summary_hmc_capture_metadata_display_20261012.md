# Selected parallel eval sweep

- date: 2026-10-12T09:33:42Z
- sweep_start_ts: 20261012-023342
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_capture_metadata_display_20261012

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_dual_capture_metadata | PASS | - | 197s | 1 | 1 | 0 | 1 | 0 | 2 | 3 | 0 | 0 | 0 | none | eval/results/hmc_capture_metadata_display_20261012/trace_dual_capture_metadata-20261012-023342 |
| 2 | trace_business_io_sequence | PASS | - | 321s | 1 | 2 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_capture_metadata_display_20261012/trace_business_io_sequence-20261012-023342 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
