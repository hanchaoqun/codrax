# Selected parallel eval sweep

- date: 2026-09-22T15:47:03Z
- sweep_start_ts: 20260922-084703
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_aux_scope_20260922

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_zero_origin_wait_account | PASS | - | 144s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_aux_scope_20260922/trace_query_zero_origin_wait_account-20260922-084703 |
| 2 | read_combo_pipeline_sequence_table | PASS | - | 632s | 1 | 2 | 0 | 1 | 0 | 7 | 7 | 0 | 0 | 0 | none | eval/results/hmc_aux_scope_20260922/read_combo_pipeline_sequence_table-20260922-084703 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
