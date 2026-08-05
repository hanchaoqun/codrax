# Selected parallel eval sweep

- date: 2026-08-05T04:11:38Z
- sweep_start_ts: 20260804-211135
- total cases: 2
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_multi_member_set_count_caveat | PASS | - | 405s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_multi_member_set_count_caveat-20260804-211138 |
| 1 | qf_sequence_analyzer_gate | FAIL | degraded_answer_checks_skipped:1 | 542s | 2 | 3 | 0 | 1 | 0 | 6 | 5 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260804-211138 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
