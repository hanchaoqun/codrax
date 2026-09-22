# Selected parallel eval sweep

- date: 2026-09-22T06:32:36Z
- sweep_start_ts: 20260921-233235
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_relation_context_20260921

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_g1_english_dstate | PASS | - | 166s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_relation_context_20260921/real_trace_g1_english_dstate-20260921-233236 |
| 2 | sr_java_call_chain | FAIL | no_primary_regex_match:(System\.out\.println|控制台|标准输出).*(不|未|only|not).*(落库|持久|数据库|durab | 247s | 1 | 2 | 0 | 1 | 0 | 3 | 4 | 0 | 0 | 0 | none | eval/results/hmc_relation_context_20260921/sr_java_call_chain-20260921-233236 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
