# Selected parallel eval sweep

- date: 2026-08-05T18:26:31Z
- sweep_start_ts: 20260805-112630
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_py_registry_dispatch | PASS | - | 134s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/sr_py_registry_dispatch-20260805-112631 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | - | 163s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260805-112631 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
