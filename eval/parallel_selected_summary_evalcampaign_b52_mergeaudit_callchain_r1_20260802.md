# Selected parallel eval sweep

- date: 2026-08-03T06:51:32Z
- sweep_start_ts: 20260802-235131
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_called_by_typed_relation_query | PASS | - | 108s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_called_by_typed_relation_query-20260802-235132 |
| 1 | sr_java_call_chain | PASS | - | 300s | 2 | 1 | 0 | 1 | 0 | 12 | 5 | 0 | 0 | 0 | none | eval/results/sr_java_call_chain-20260802-235132 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
