# Selected parallel eval sweep

- date: 2026-09-08T14:48:21Z
- sweep_start_ts: 20260908-074819
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_d4_demand_vs_supply | FAIL | no_regex_match:(空闲|idle|余量|充足|不是.{0,12}(算力|CPU|供给)|并非.{0,12}(算力|CPU|供给)|(算力|CPU| | 152s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_d4_demand_vs_supply-20260908-074821 |
| 1 | sr_py_registry_dispatch | PASS | - | 284s | 1 | 2 | 0 | 1 | 0 | 4 | 4 | 0 | 0 | 0 | none | eval/results/sr_py_registry_dispatch-20260908-074821 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
