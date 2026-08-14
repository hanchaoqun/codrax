# Selected parallel eval sweep

- date: 2026-08-14T10:24:27Z
- sweep_start_ts: 20260814-032425
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_sequence_analyzer_gate | FAIL | read_exit:2 banned:runTaskGraph no_regex_match:(```mermaid|sequenceDiagram) | 225s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260814-032427 |
| 2 | github_issue_commons_lang_random_ascii | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 346s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_commons_lang_random_ascii-20260814-032427 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
