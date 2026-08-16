# Selected parallel eval sweep

- date: 2026-08-16T10:41:31Z
- sweep_start_ts: 20260816-034129
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h4_supply_thermal_witness | PASS | - | 141s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260816-034131 |
| 1 | read_combo_answer_document_tools | PASS | - | 540s | 1 | 1 | 0 | 1 | 0 | 8 | 8 | 0 | 0 | 0 | none | eval/results/read_combo_answer_document_tools-20260816-034131 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
