# Selected parallel eval sweep

- date: 2026-08-12T17:19:01Z
- sweep_start_ts: 20260812-101859
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | mr_poly_binding_chain | PASS | - | 125s | 1 | 1 | 0 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260812-101901 |
| 1 | qf_sequence_analyzer_gate | PASS | - | 244s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260812-101901 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
