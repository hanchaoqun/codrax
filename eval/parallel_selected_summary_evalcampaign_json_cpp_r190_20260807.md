# Selected parallel eval sweep

- date: 2026-08-07T22:23:53Z
- sweep_start_ts: 20260807-152351
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | data_json_strict_ids | PASS | - | 38s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260807-152353 |
| 2 | sr_cpp_virtual_chain | PASS | - | 136s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_cpp_virtual_chain-20260807-152353 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
