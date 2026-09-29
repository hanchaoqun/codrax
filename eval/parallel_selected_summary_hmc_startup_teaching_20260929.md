# Selected parallel eval sweep

- date: 2026-09-29T07:43:49Z
- sweep_start_ts: 20260929-004348
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_existing_sqlite_dictionary | FAIL | missing_primary:LoadPreferences no_primary_regex_match:(^|[^0-9])8([.]0+)?[[:space:]*]*(ms|毫秒) no_primary_regex_matc | 170s | 1 | 1 | 0 | 1 | 0 | 2 | 3 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_existing_sqlite_dictionary-20260929-004349 |
| 2 | trace_resource_identity_inventory | FAIL | no_primary_regex_match:(空字符串|空名称|空文本|空串) no_primary_text_regex_match:((不能|不足|无法|不� | 188s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_resource_identity_inventory-20260929-004349 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
