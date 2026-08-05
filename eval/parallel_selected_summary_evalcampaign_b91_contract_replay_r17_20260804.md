# Selected parallel eval sweep

- date: 2026-08-05T02:00:24Z
- sweep_start_ts: 20260804-190023
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_sequence_analyzer_gate | FAIL | degraded_answer_checks_skipped:1 | 563s | 1 | 4 | 0 | 1 | 0 | 6 | 5 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260804-190025 |
| 2 | qf_multi_member_set_count_caveat | TIMEOUT | exceeded 1200s wall-time | 1201s | 1 | 2 | 0 | 1 | 0 | 2 | 1 | 0 | 0 | 0 | none | eval/results/qf_multi_member_set_count_caveat-20260804-190025 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
