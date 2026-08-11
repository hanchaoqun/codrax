# Selected parallel eval sweep

- date: 2026-08-11T13:40:02Z
- sweep_start_ts: 20260811-064001
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_ts_workspace_chain | PASS | - | 147s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_ts_workspace_chain-20260811-064003 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 279s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260811-064003 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
