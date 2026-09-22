# Selected parallel eval sweep

- date: 2026-09-22T01:59:47Z
- sweep_start_ts: 20260921-185945
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_wait_raw_state_20260921

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_g1_english_dstate | PASS | - | 84s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 1 | perf_triage+trace_query | eval/results/hmc_wait_raw_state_20260921/real_trace_g1_english_dstate-20260921-185947 |
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 208s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_wait_raw_state_20260921/trace_query_wakeup_causal_io_chain-20260921-185947 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
