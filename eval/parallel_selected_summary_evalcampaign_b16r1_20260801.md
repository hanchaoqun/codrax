# Selected parallel eval sweep

- date: 2026-08-01T02:56:32Z
- sweep_start_ts: 20260731-195630
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_called_by_typed_relation_query | PASS | - | 70s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_called_by_typed_relation_query-20260731-195632 |
| 1 | trace_query_perf_quality_simpleperf_proto_offcpu | PASS | - | 111s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_perf_quality_simpleperf_proto_offcpu-20260731-195632 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
