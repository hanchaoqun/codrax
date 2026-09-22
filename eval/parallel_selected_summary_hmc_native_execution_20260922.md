# Selected parallel eval sweep

- date: 2026-09-22T10:59:28Z
- sweep_start_ts: 20260922-035927
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_native_execution_20260922

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | nested_python_increment | PASS | - | 127s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_native_execution_20260922/nested_python_increment-20260922-035928 |
| 1 | trace_query_frame_semantic_span_optimization | PASS | - | 151s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_native_execution_20260922/trace_query_frame_semantic_span_optimization-20260922-035928 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**

完整人工按本次用户任务正确性判 **2/2 PASS**；[完整审计](parallel_selected_summary_hmc_native_execution_20260922_manual_audit.md)分别记录命中范围和剩余债。Trace的worker语义墙钟图例命中，正文保未证帧因果；Python真实3个原测试通过，但走既有PTO路径，新run_existing_test/强收据与分析补读未live命中，不能据此代签它们或B2–B6。Python案例自身125秒、外层127秒分开保留；原历史FAIL不改。
