# Selected parallel eval sweep

- date: 2026-09-29T06:52:17Z
- sweep_start_ts: 20260928-235216
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_existing_sqlite_dictionary | FAIL | no_primary_regex_match:(^|[^0-9])8([.]0+)?[[:space:]*]*(ms|毫秒) no_primary_regex_match:(^|[^0-9])5([.]0+)?[[:space:]* | 218s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_existing_sqlite_dictionary-20260928-235217 |
| 1 | trace_resource_identity_inventory | FAIL | no_primary_text_regex_match:((不能|不足|无法|不支持|不代表|不等于|未证|未能).{0,120}(执行|函数|� | 233s | 1 | 2 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_resource_identity_inventory-20260928-235217 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
