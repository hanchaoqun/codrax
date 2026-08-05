# Selected parallel eval sweep

- date: 2026-08-05T03:20:16Z
- sweep_start_ts: 20260804-202015
- total cases: 2
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_multi_member_set_count_caveat | FAIL | dynamic_scalar_binding_missing:exported_functions:5 missing:RegisteredKinds no_regex_match:(函数.*(^|[^0-9])5([^0-9]|$ | 415s | 1 | 1 | 0 | 2 | 1 | 1 | 2 | 0 | 0 | 0 | none | eval/results/qf_multi_member_set_count_caveat-20260804-202016 |
| 1 | qf_sequence_analyzer_gate | FAIL | degraded_answer_checks_skipped:1 | 681s | 1 | 4 | 0 | 1 | 0 | 6 | 5 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260804-202016 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
