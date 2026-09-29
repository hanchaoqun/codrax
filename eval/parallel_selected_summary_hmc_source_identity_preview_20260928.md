# Selected parallel eval sweep

- date: 2026-09-29T04:00:06Z
- sweep_start_ts: 20260928-210004
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | empty_python_module_apply | PASS | - | 128s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/empty_python_module_apply-20260928-210006 |
| 2 | trace_existing_sqlite_dictionary | FAIL | no_primary_regex_match:(^|[^0-9])8([.]0+)?[[:space:]*]*(ms|毫秒) no_primary_regex_match:(^|[^0-9])5([.]0+)?[[:space:]* | 217s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_existing_sqlite_dictionary-20260928-210006 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
