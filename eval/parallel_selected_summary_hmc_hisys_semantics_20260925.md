# Selected parallel eval sweep

- date: 2026-09-26T04:16:53Z
- sweep_start_ts: 20260925-211651
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_existing_sqlite_hisys_semantics | PASS | - | 154s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_existing_sqlite_hisys_semantics-20260925-211653 |
| 1 | trace_existing_sqlite_dictionary | FAIL | no_primary_regex_match:(^|[^0-9])8([.]0+)?[[:space:]*]*(ms|毫秒) no_primary_regex_match:(^|[^0-9])5([.]0+)?[[:space:]* | 182s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_existing_sqlite_dictionary-20260925-211653 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
