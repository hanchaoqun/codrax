# Selected parallel eval sweep

- date: 2026-08-04T03:48:31Z
- sweep_start_ts: 20260803-204830
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | arkts_repomap | PASS | - | 126s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260803-204831 |
| 1 | qf_sequence_analyzer_gate | FAIL | degraded_answer_checks_skipped:1 | 418s | 1 | 4 | 0 | 1 | 0 | 6 | 5 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260803-204831 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
