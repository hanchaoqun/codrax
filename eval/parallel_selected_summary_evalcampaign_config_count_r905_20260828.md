# Selected parallel eval sweep

- date: 2026-08-28T21:18:28Z
- sweep_start_ts: 20260828-141826
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_config_precedence | PASS | - | 152s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_config_precedence-20260828-141828 |
| 2 | qf_multi_member_set_count_caveat | FAIL | dynamic_scalar_binding_missing:exported_functions:5 no_regex_match:(函数.*(^|[^0-9])5([^0-9]|$)|(^|[^0-9])5([^0-9]|$). | 375s | 1 | 3 | 0 | 1 | 0 | 1 | 2 | 0 | 0 | 0 | none | eval/results/qf_multi_member_set_count_caveat-20260828-141828 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
