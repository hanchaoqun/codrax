# Selected parallel eval sweep

- date: 2026-09-10T08:59:17Z
- sweep_start_ts: 20260910-015917
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_java_config_precedence | PASS | - | 126s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_java_config_precedence-20260910-015917 |
| 2 | sr_cpp_virtual_chain | PASS | - | 389s | 1 | 2 | 0 | 1 | 0 | 7 | 7 | 0 | 0 | 0 | none | eval/results/sr_cpp_virtual_chain-20260910-015917 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
