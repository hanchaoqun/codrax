# Selected parallel eval sweep

- date: 2026-08-13T03:04:00Z
- sweep_start_ts: 20260812-200359
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_type_relation_loop_controller | FAIL | no_regex_match:(```mermaid|classDiagram|flowchart|graph[[:space:]]+(TD|LR)) | 181s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260812-200400 |
| 1 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | - | 229s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260812-200400 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
