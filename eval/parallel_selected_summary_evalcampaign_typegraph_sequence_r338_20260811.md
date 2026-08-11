# Selected parallel eval sweep

- date: 2026-08-11T21:51:02Z
- sweep_start_ts: 20260811-145101
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_sequence_analyzer_gate | PASS | - | 160s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260811-145102 |
| 1 | qf_type_relation_loop_controller | PASS | - | 266s | 1 | 1 | 0 | 1 | 0 | 7 | 7 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260811-145102 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
