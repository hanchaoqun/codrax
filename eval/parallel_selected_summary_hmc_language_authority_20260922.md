# Selected parallel eval sweep

- date: 2026-09-22T07:15:10Z
- sweep_start_ts: 20260922-001507
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_language_authority_20260922

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_g1_english_dstate | PASS | - | 148s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 1 | perf_triage+trace_query | eval/results/hmc_language_authority_20260922/real_trace_g1_english_dstate-20260922-001510 |
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 389s | 1 | 3 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_language_authority_20260922/trace_query_wakeup_causal_io_chain-20260922-001510 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
