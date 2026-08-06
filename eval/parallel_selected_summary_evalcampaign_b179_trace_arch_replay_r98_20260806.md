# Selected parallel eval sweep

- date: 2026-08-06T13:37:51Z
- sweep_start_ts: 20260806-063749
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_d4_demand_vs_supply | PASS | - | 168s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_d4_demand_vs_supply-20260806-063751 |
| 2 | qf_architecture | FAIL | no_regex_match:(classif|分类|hypothes|假设|意图|理解|推断|识别|解析|静态分析|[Tt]ask.?[Gg]raph|任务� | 603s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_architecture-20260806-063751 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
