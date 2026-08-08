# Selected parallel eval sweep

- date: 2026-08-08T20:09:08Z
- sweep_start_ts: 20260808-130907
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_logic_view_read_pipeline | FAIL | degraded_answer_checks_skipped:1 mermaid_edges:0<1 | 590s | 1 | 1 | 0 | 2 | 1 | 11 | 11 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260808-130908 |
| 2 | qf_diagram_pipeline | FAIL | mermaid_edges:0<1 | 835s | 1 | 1 | 0 | 2 | 1 | 20 | 20 | 0 | 0 | 0 | none | eval/results/qf_diagram_pipeline-20260808-130908 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
