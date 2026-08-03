# Selected parallel eval sweep

- date: 2026-08-03T03:04:34Z
- sweep_start_ts: 20260802-200432
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | data_basic_sum_with_rules | PASS | - | 102s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_basic_sum_with_rules-20260802-200434 |
| 1 | sr_java_call_chain | PASS | - | 356s | 1 | 4 | 0 | 1 | 0 | 8 | 4 | 0 | 0 | 0 | none | eval/results/sr_java_call_chain-20260802-200434 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
