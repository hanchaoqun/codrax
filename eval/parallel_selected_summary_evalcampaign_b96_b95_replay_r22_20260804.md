# Selected parallel eval sweep

- date: 2026-08-05T06:26:27Z
- sweep_start_ts: 20260804-232625
- total cases: 2
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_sequence_analyzer_gate | PASS | - | 239s | 1 | 1 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260804-232627 |
| 1 | qf_multi_member_set_count_caveat | FAIL | dynamic_scalar_binding_missing:exported_functions:5 no_regex_match:(类型.*(^|[^0-9])3([^0-9]|$)|(^|[^0-9])3([^0-9]|$). | 953s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_multi_member_set_count_caveat-20260804-232627 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
