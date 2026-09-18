# Selected parallel eval sweep

- date: 2026-09-18T04:05:14Z
- sweep_start_ts: 20260917-210512
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_native_resource_metadata | FAIL | no_primary_text_regex_match:((共|总计|合计|共有|总数).{0,60}(3|三)[[:space:]*]*(次|条|个)|(3|三)[[:space:] | 186s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_native_resource_metadata-20260917-210514 |
| 2 | trace_query_business_marker_io_chain | FAIL | missing_primary:LoadDocumentIndex no_primary_regex_match:(^|[^0-9])50([.]0+)?[[:space:]*]*(ms|毫秒) no_primary_regex_m | 273s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_business_marker_io_chain-20260917-210514 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
