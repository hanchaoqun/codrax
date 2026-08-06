# Selected parallel eval sweep

- date: 2026-08-06T00:42:01Z
- sweep_start_ts: 20260805-174200
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_wakeup_causal_io_chain | PASS | - | 166s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260805-174202 |
| 2 | data_jsonl_filter_count | FAIL | no_regex_match:^2[[:space:]]*$ | 222s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_jsonl_filter_count-20260805-174202 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
