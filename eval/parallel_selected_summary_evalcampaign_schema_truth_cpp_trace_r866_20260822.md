# Selected parallel eval sweep

- date: 2026-08-22T15:40:30Z
- sweep_start_ts: 20260822-084029
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_cpp_virtual_chain | PASS | - | 173s | 1 | 2 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/sr_cpp_virtual_chain-20260822-084031 |
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 176s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260822-084031 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
