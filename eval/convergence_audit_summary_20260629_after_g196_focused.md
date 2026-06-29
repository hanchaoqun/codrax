# Selected parallel eval sweep

- date: 2026-06-29T14:28:57Z
- sweep_start_ts: 20260629-222856
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 2 | read_combo_trace_current_source_explanation | FAIL | no_regex_match:(HiTrace|trace|RenderService|DoFrame|86\.111|耗时).*(当前源码|源码|internal/)|(当前源码|源� | 122s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/read_combo_trace_current_source_explanation-20260629-222857 |
| 1 | cangjie_repomap | PASS | - | 176s | 1 | 1 | 0 | 1 | 0 | 2 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260629-222857 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
