# Selected parallel eval sweep

- date: 2026-09-07T01:58:41Z
- sweep_start_ts: 20260906-185838
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_java_call_chain | FAIL | no_primary_regex_match:(System\.out\.println|控制台|标准输出).*(不|未|only|not).*(落库|持久|数据库|durab | 153s | 1 | 2 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/sr_java_call_chain-20260906-185841 |
| 1 | read_combo_log_current_code_boundary | PASS | - | 359s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | log_triage | eval/results/read_combo_log_current_code_boundary-20260906-185841 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
