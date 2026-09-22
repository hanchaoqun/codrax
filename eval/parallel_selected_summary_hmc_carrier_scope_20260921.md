# Selected parallel eval sweep

- date: 2026-09-22T03:51:30Z
- sweep_start_ts: 20260921-205128
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_carrier_scope_20260921

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_g1_english_dstate | PASS | - | 78s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_carrier_scope_20260921/real_trace_g1_english_dstate-20260921-205130 |
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 168s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_carrier_scope_20260921/trace_query_wakeup_causal_io_chain-20260921-205130 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
