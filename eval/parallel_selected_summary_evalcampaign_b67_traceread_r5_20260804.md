# Selected parallel eval sweep

- date: 2026-08-04T07:31:38Z
- sweep_start_ts: 20260804-003137
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | read_combo_command_current_source_explanation | FAIL | no_regex_match:(internal/tool|非测试|Go 文件|统计|数量) no_regex_match:(命令|exec|command|measurement|统计 | 29s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_command_current_source_explanation-20260804-003138 |
| 1 | trace_query_wakeup_causal_io_chain | ABORTED | manual fail-fast after 20 completion calls repeated the same missing relation_claim rejection; evidence preserved | 572s | 3 | 22 | 0 | 0 | 0 | 22 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260804-003138 |

**Completed by runner: 0 / 2 — FAIL: 1 — manually aborted deterministic retry storm: 1**
