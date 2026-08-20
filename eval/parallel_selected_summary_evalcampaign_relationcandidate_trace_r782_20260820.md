# Selected parallel eval sweep

- date: 2026-08-20T17:49:52Z
- sweep_start_ts: 20260820-104951
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 221s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260820-104952 |
| 1 | read_combo_pipeline_sequence_table | FAIL | no_regex_match:(extractor|Extract) | 1303s | 1 | 3 | 0 | 1 | 0 | 12 | 11 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260820-104953 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
