# Selected parallel eval sweep

- date: 2026-08-09T04:36:48Z
- sweep_start_ts: 20260808-213646
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | data_jsonl_filter_count | PASS | - | 50s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_jsonl_filter_count-20260808-213648 |
| 1 | trace_query_frame_timeline_flow | PASS | - | 179s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_frame_timeline_flow-20260808-213648 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
