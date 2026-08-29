# Selected parallel eval sweep

- date: 2026-08-29T09:20:38Z
- sweep_start_ts: 20260829-022038
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 190s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260829-022038 |
| 2 | sr_rust_cross_module_chain | PASS | - | 455s | 1 | 1 | 0 | 1 | 0 | 6 | 6 | 0 | 0 | 0 | none | eval/results/sr_rust_cross_module_chain-20260829-022038 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
