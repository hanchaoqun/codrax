# Selected parallel eval sweep

- date: 2026-08-12T16:28:02Z
- sweep_start_ts: 20260812-092801
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | cond_resolve_stall_timeout | PASS | - | 112s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cond_resolve_stall_timeout-20260812-092802 |
| 2 | qf_type_relation_loop_controller | FAIL | no_regex_match:(```mermaid|classDiagram|flowchart|graph[[:space:]]+(TD|LR)) | 132s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260812-092802 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
