# Selected parallel eval sweep

- date: 2026-09-07T06:34:11Z
- sweep_start_ts: 20260906-233409
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_c_xmacro_table | PASS | - | 128s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_c_xmacro_table-20260906-233412 |
| 1 | arkts_repomap | FAIL | inventory_count_mismatch:builder_fragment:got6:want2 | 299s | 2 | 3 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260906-233412 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
