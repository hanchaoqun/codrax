# Selected parallel eval sweep

- date: 2026-08-16T15:18:41Z
- sweep_start_ts: 20260816-081839
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_zod_prefault | FAIL | write_final_verdict:unverified:production_verification_source_static_only | 160s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_zod_prefault-20260816-081841 |
| 1 | read_combo_answer_document_tools | FAIL | missing:retry | 980s | 1 | 3 | 0 | 2 | 1 | 15 | 16 | 0 | 0 | 0 | none | eval/results/read_combo_answer_document_tools-20260816-081841 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
