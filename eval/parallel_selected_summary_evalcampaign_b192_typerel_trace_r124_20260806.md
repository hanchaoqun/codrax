# Selected parallel eval sweep

- date: 2026-08-06T20:38:01Z
- sweep_start_ts: 20260806-133759
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_type_relation_loop_controller | PASS | - | 178s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260806-133801 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | - | 201s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260806-133801 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
