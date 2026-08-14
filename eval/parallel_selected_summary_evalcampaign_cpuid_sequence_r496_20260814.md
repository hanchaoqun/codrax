# Selected parallel eval sweep

- date: 2026-08-14T15:52:41Z
- sweep_start_ts: 20260814-085239
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_sequence_analyzer_gate | PASS | - | 190s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260814-085241 |
| 1 | trace_query_perf_quality_raw_fallback | PASS | - | 211s | 1 | 3 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_perf_quality_raw_fallback-20260814-085241 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
