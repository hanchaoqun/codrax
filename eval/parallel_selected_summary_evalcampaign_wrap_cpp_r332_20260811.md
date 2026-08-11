# Selected parallel eval sweep

- date: 2026-08-11T19:50:09Z
- sweep_start_ts: 20260811-125008
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_cpp_virtual_chain | PASS | - | 196s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_cpp_virtual_chain-20260811-125010 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 499s | 1 | 2 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260811-125010 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
