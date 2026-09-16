# Selected parallel eval sweep

- date: 2026-09-16T00:43:44Z
- sweep_start_ts: 20260915-174344
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_h10_spantop_member_subrows | FAIL | missing:2.388 missing:1.781 missing:0.607 no_regex_match:单段1\.781ms no_regex_match:单段0\.607ms no_regex_match:行 | 117s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h10_spantop_member_subrows-20260915-174344 |
| 2 | read_combo_criterion_rich_functions | PASS | - | 294s | 1 | 3 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/read_combo_criterion_rich_functions-20260915-174344 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
