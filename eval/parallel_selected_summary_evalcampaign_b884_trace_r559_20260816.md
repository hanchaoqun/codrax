# Selected parallel eval sweep

- date: 2026-08-16T09:29:09Z
- sweep_start_ts: 20260816-022908
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | no_principal_text_regex_match:((limit row|policy limit|策略上限|限制记录|限频记录).{0,160}(不能|不足以| | 100s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260816-022909 |
| 1 | read_combo_answer_document_tools | PASS | - | 501s | 1 | 1 | 0 | 1 | 0 | 9 | 9 | 0 | 0 | 0 | none | eval/results/read_combo_answer_document_tools-20260816-022909 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
