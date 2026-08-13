# Selected parallel eval sweep

- date: 2026-08-13T04:41:57Z
- sweep_start_ts: 20260812-214156
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | github_issue_zod_prefault | FAIL | write_final_verdict:unverified:production_verification_source_static_only | 129s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_zod_prefault-20260812-214157 |
| 2 | read_combo_pipeline_sequence_table | FAIL | no_regex_match:(```mermaid|sequenceDiagram) | 304s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260812-214157 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
