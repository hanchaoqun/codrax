# Selected parallel eval sweep

- date: 2026-08-29T07:07:27Z
- sweep_start_ts: 20260829-000726
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_java_call_chain | FAIL | no_primary_regex_match:(System\.out\.println|控制台|标准输出).*(不|未|only|not).*(落库|持久|数据库|durab | 175s | 1 | 2 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/sr_java_call_chain-20260829-000727 |
| 2 | sr_ts_workspace_chain | PASS | - | 393s | 1 | 2 | 0 | 1 | 0 | 8 | 8 | 0 | 0 | 0 | none | eval/results/sr_ts_workspace_chain-20260829-000727 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
