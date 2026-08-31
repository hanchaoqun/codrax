# Selected parallel eval sweep

- date: 2026-08-31T14:27:01Z
- sweep_start_ts: 20260831-072659
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_java_typo | PASS | - | 50s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_java_typo-20260831-072701 |
| 1 | qf_sequence_analyzer_gate | FAIL | degraded_answer_checks_skipped:1 | 970s | 1 | 1 | 0 | 1 | 0 | 20 | 19 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260831-072701 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
