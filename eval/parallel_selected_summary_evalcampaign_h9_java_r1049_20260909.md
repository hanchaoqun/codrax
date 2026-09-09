# Selected parallel eval sweep

- date: 2026-09-09T12:45:17Z
- sweep_start_ts: 20260909-054517
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_java_call_chain | PASS | - | 148s | 1 | 2 | 0 | 1 | 0 | 1 | 2 | 0 | 0 | 0 | none | eval/results/sr_java_call_chain-20260909-054517 |
| 1 | real_trace_h9_conversion_single_basis | PASS | - | 361s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h9_conversion_single_basis-20260909-054517 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
