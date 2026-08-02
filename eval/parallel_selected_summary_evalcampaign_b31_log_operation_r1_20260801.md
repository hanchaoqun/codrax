# Selected parallel eval sweep

- date: 2026-08-02T01:24:08Z
- sweep_start_ts: 20260801-182407
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | operation_system_inventory | PASS | - | 27s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/operation_system_inventory-20260801-182408 |
| 1 | log_path_question_multi_runtime_files | FAIL | no_regex_match:(panic|nil pointer|空指针).*(\(\*Store\)\.Get|Store\.Get|store\.go:88)|(\(\*Store\)\.Get|Store\.Get|st | 83s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/log_path_question_multi_runtime_files-20260801-182408 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
