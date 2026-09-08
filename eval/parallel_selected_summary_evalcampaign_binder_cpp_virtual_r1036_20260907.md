# Selected parallel eval sweep

- date: 2026-09-08T01:54:19Z
- sweep_start_ts: 20260907-185419
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_cpp_virtual_chain | PASS | - | 131s | 1 | 1 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/sr_cpp_virtual_chain-20260907-185419 |
| 1 | real_trace_h1_binder_true_false_attribution | PASS | - | 249s | 1 | 1 | 0 | 1 | 0 | 2 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h1_binder_true_false_attribution-20260907-185419 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
