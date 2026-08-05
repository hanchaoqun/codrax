# Selected parallel eval sweep

- date: 2026-08-05T08:02:02Z
- sweep_start_ts: 20260805-010200
- total cases: 2
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_sequence_analyzer_gate | PASS | - | 231s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260805-010202 |
| 2 | qf_multi_member_set_count_caveat | FAIL | degraded_answer_checks_skipped:1 | 572s | 1 | 1 | 0 | 1 | 0 | 4 | 3 | 0 | 0 | 0 | none | eval/results/qf_multi_member_set_count_caveat-20260805-010202 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
