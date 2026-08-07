# Selected parallel eval sweep

- date: 2026-08-07T16:17:54Z
- sweep_start_ts: 20260807-091752
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_cpp_typo | PASS | - | 93s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_cpp_typo-20260807-091754 |
| 1 | qf_sequence_analyzer_gate | PASS | - | 597s | 1 | 2 | 0 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260807-091754 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
