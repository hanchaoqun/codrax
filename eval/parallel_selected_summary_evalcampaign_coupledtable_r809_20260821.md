# Selected parallel eval sweep

- date: 2026-08-21T11:34:17Z
- sweep_start_ts: 20260821-043417
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 217s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260821-043417 |
| 1 | read_combo_pipeline_sequence_table | PASS | - | 384s | 1 | 2 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260821-043417 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
