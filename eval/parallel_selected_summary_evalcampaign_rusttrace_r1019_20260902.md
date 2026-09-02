# Selected parallel eval sweep

- date: 2026-09-02T07:51:24Z
- sweep_start_ts: 20260902-005122
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h11_cross_direction_overlap | FAIL | no_log_regex:cross_direction_de_minimis pair running x io_latency overlap 0\.114ms | 220s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h11_cross_direction_overlap-20260902-005124 |
| 1 | sr_rust_cross_module_chain | PASS | - | 301s | 1 | 1 | 0 | 1 | 0 | 5 | 6 | 0 | 0 | 0 | none | eval/results/sr_rust_cross_module_chain-20260902-005124 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
