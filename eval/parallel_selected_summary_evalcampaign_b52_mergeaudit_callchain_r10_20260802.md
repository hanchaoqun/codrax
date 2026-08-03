# Selected parallel eval sweep

- date: 2026-08-03T10:00:59Z
- sweep_start_ts: 20260803-030058
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_called_by_typed_relation_query | PASS | - | 107s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_called_by_typed_relation_query-20260803-030059 |
| 1 | sr_java_call_chain | PASS | - | 220s | 1 | 1 | 0 | 1 | 0 | 4 | 2 | 0 | 0 | 0 | none | eval/results/sr_java_call_chain-20260803-030059 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
