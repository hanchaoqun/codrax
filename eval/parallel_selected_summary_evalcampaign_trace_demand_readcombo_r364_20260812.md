# Selected parallel eval sweep

- date: 2026-08-12T05:23:15Z
- sweep_start_ts: 20260811-222313
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | read_combo_trace_current_source_explanation | FAIL | no_regex_match:(HiTrace|trace|RenderService|DoFrame|(^|[^0-9])86\.111|耗时).*(当前源码|源码|internal/)|(当前� | 144s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/read_combo_trace_current_source_explanation-20260811-222315 |
| 1 | real_trace_d4_demand_vs_supply | PASS | - | 174s | 1 | 1 | 0 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_d4_demand_vs_supply-20260811-222315 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
