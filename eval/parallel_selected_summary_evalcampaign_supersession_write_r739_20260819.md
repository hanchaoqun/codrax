# Selected parallel eval sweep

- date: 2026-08-19T19:03:09Z
- sweep_start_ts: 20260819-120308
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_logic_view_read_pipeline | FAIL | read_exit:143 no_regex_match:(```mermaid|flowchart|graph[[:space:]]+(TD|LR)) mermaid_edges:0<1 | 581s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260819-120309 |
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | write_report_failed write_final_run_status:in_progress write_final_verdict:missing:missing | 1078s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-120309 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
