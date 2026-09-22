# Selected parallel eval sweep

- date: 2026-09-22T09:35:21Z
- sweep_start_ts: 20260922-023520
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_closure_origin_potential_20260922

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | nested_python_increment | PASS | - | 146s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_closure_origin_potential_20260922/nested_python_increment-20260922-023521 |
| 1 | trace_query_frame_semantic_span_optimization | PASS | - | 195s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_closure_origin_potential_20260922/trace_query_frame_semantic_span_optimization-20260922-023521 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**

全文人工审计：**0/2 PASS，2/2 FAIL**。Trace正文完成/唤醒顺序错误；Python补丁正确但跳过用户要求的已有测试。机器结果不改，详见[完整人工审计](parallel_selected_summary_hmc_closure_origin_potential_20260922_manual_audit.md)。Python外层146秒与案例自身`run-1.wall`143秒分别保留。
