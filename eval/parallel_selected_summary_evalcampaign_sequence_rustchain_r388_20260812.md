# Selected parallel eval sweep

- date: 2026-08-12T13:25:21Z
- sweep_start_ts: 20260812-062520
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_rust_cross_module_chain | PASS | - | 189s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_rust_cross_module_chain-20260812-062521 |
| 1 | qf_sequence_analyzer_gate | PASS | - | 223s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260812-062521 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
