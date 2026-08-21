# Selected parallel eval sweep

- date: 2026-08-21T00:58:53Z
- sweep_start_ts: 20260820-175853
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | read_combo_pipeline_sequence_table | INCOMPLETE | selected eval host session terminated while an active model request was in flight; no product verdict | 317s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260820-175853 |
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 212s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 1 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260820-175853 |

**Pass: 1 / 2 — Incomplete: 1 — Fail/Timeout/LaunchFail: 0**
