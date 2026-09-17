# Selected parallel eval sweep

- date: 2026-09-17T01:41:26Z
- sweep_start_ts: 20260916-184126
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_jank_field_inventory | FAIL | no_primary_text_regex_match:9007199255740993(.{0,200}){0,3}9007199256740993(.{0,200}){0,3}9007199254740993 no_primary_te | 191s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_jank_field_inventory-20260916-184126 |
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 221s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260916-184126 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
