# Selected parallel eval sweep

- date: 2026-08-11T21:12:49Z
- sweep_start_ts: 20260811-141248
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_type_relation_loop_controller | PASS | - | 271s | 1 | 1 | 0 | 2 | 1 | 1 | 2 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260811-141249 |
| 2 | qf_sequence_analyzer_gate | PASS | - | 827s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260811-141249 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
