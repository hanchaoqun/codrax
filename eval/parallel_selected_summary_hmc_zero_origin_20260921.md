# Selected parallel eval sweep

- date: 2026-09-21T13:59:15Z
- sweep_start_ts: 20260921-065914
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_zero_origin_20260921

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_zero_origin_wait_account | PASS | - | 154s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_zero_origin_20260921/trace_query_zero_origin_wait_account-20260921-065916 |
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 258s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_zero_origin_20260921/trace_query_wakeup_causal_io_chain-20260921-065916 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
