# Selected parallel eval sweep

- date: 2026-08-22T02:15:25Z
- sweep_start_ts: 20260821-191525
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | patch_c_typo | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 147s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_c_typo-20260821-191526 |
| 2 | sr_rust_cross_module_chain | FAIL | degraded_answer_checks_skipped:1 | 270s | 1 | 1 | 0 | 1 | 0 | 8 | 7 | 0 | 0 | 0 | none | eval/results/sr_rust_cross_module_chain-20260821-191526 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
