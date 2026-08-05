# Selected parallel eval sweep

- date: 2026-08-05T00:59:58Z
- sweep_start_ts: 20260804-175957
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_sequence_analyzer_gate | FAIL | degraded_answer_checks_skipped:1 | 256s | 1 | 1 | 0 | 1 | 0 | 6 | 5 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260804-175958 |
| 2 | qf_multi_member_set_count_caveat | PASS | - | 667s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_multi_member_set_count_caveat-20260804-175958 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
