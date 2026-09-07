# Selected parallel eval sweep

- date: 2026-09-07T11:26:46Z
- sweep_start_ts: 20260907-042644
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_cpp_sink_impls | PASS | - | 96s | 1 | 1 | 0 | 1 | 0 | 1 | 2 | 0 | 0 | 0 | none | eval/results/sr_cpp_sink_impls-20260907-042646 |
| 1 | real_trace_h1_binder_true_false_attribution | PASS | - | 204s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h1_binder_true_false_attribution-20260907-042646 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
