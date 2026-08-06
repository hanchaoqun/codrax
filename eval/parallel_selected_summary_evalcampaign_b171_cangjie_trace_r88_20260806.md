# Selected parallel eval sweep

- date: 2026-08-06T08:34:12Z
- sweep_start_ts: 20260806-013410
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | cangjie_repomap | PASS | - | 128s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260806-013412 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | - | 201s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260806-013412 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
