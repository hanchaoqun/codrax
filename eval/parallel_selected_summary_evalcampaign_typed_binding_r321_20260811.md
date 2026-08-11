# Selected parallel eval sweep

- date: 2026-08-11T15:59:25Z
- sweep_start_ts: 20260811-085924
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_cpp_virtual_chain | PASS | - | 130s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_cpp_virtual_chain-20260811-085925 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 440s | 1 | 1 | 0 | 1 | 0 | 5 | 5 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260811-085925 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
