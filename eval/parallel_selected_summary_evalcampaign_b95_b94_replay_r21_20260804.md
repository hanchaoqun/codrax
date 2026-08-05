# Selected parallel eval sweep

- date: 2026-08-05T05:39:42Z
- sweep_start_ts: 20260804-223941
- total cases: 2
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_sequence_analyzer_gate | PASS | - | 496s | 1 | 3 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260804-223942 |
| 1 | qf_multi_member_set_count_caveat | TIMEOUT | exceeded 1500s wall-time | 1500s | 1 | 4 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_multi_member_set_count_caveat-20260804-223942 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
