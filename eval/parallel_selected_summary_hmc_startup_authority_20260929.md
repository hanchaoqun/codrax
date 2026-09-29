# Selected parallel eval sweep

- date: 2026-09-29T09:13:06Z
- sweep_start_ts: 20260929-021259
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_sqlite_startup_subject | FAIL | no_primary_regex_match:(^|[^0-9])8([.]0+)?[[:space:]*]*(ms|毫秒) no_primary_regex_match:(^|[^0-9])5([.]0+)?[[:space:]* | 160s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_sqlite_startup_subject-20260929-021306 |
| 2 | empty_python_module_apply | PASS | - | 285s | 1 | 2 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/empty_python_module_apply-20260929-021306 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

人工完整答案1/2，Trace FAIL不改，Python本次实际命中只读登记完整收尾；见[人工审计](parallel_selected_summary_hmc_startup_authority_20260929_manual_audit.md)。冻结构建revision7320af656b27，binary SHA-256=`8cb47cea55802a2d5199a600f7c306610bcdef8b3abf3e06834e6711672cab13`；独立全仓48334正式exit0（87测试包、13无测试、零FAIL）。未追加第三例。
