# Selected parallel eval sweep

- date: 2026-08-07T20:28:52Z
- sweep_start_ts: 20260807-132851
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_diagram_pipeline | PASS | - | 120s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_diagram_pipeline-20260807-132852 |
| 1 | sr_cpp_virtual_chain | PASS | - | 136s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_cpp_virtual_chain-20260807-132852 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
