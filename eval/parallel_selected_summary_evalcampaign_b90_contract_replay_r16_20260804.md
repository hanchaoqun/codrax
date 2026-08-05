# Selected parallel eval sweep

- date: 2026-08-05T01:30:58Z
- sweep_start_ts: 20260804-183056
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_multi_member_set_count_caveat | FAIL | dynamic_scalar_binding_missing:exported_functions:5 banned:private no_regex_match:(函数.*(^|[^0-9])5([^0-9]|$)|(^|[^0- | 329s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_multi_member_set_count_caveat-20260804-183058 |
| 1 | qf_sequence_analyzer_gate | PASS | - | 497s | 1 | 4 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260804-183058 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
