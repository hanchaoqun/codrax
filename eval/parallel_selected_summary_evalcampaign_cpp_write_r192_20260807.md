# Selected parallel eval sweep

- date: 2026-08-07T23:23:38Z
- sweep_start_ts: 20260807-162336
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_go_typo | PASS | - | 116s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_go_typo-20260807-162338 |
| 1 | sr_cpp_virtual_chain | PASS | - | 134s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_cpp_virtual_chain-20260807-162338 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
