# Selected parallel eval sweep

- date: 2026-08-06T22:14:16Z
- sweep_start_ts: 20260806-151414
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | logtri_rust | PASS | - | 80s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/logtri_rust-20260806-151416 |
| 1 | sr_rust_cross_module_chain | PASS | - | 214s | 1 | 1 | 0 | 1 | 0 | 4 | 4 | 0 | 0 | 0 | none | eval/results/sr_rust_cross_module_chain-20260806-151416 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
