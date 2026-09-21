# Selected parallel eval sweep

- date: 2026-09-21T13:01:42Z
- sweep_start_ts: 20260921-060141
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_d_io_semantics_20260921

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_io_chain | FAIL | trace_final_projection_blocks:0_want_1 missing:iowait no_regex_match:(D/IO|D 状态|D-state|io_wait|iowait|不可中断| | 196s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_d_io_semantics_20260921/trace_query_wakeup_causal_io_chain-20260921-060142 |
| 1 | real_trace_c2_dstate_iowait | PASS | - | 197s | 1 | 2 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_d_io_semantics_20260921/real_trace_c2_dstate_iowait-20260921-060142 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
