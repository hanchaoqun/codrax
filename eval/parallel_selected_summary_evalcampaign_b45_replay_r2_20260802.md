# Selected parallel eval sweep

- date: 2026-08-02T18:29:52Z
- sweep_start_ts: 20260802-112952
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | read_combo_answer_document_tools | PASS | - | 111s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_answer_document_tools-20260802-112952 |
| 1 | operation_web_manual_summary | PASS | - | 206s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/operation_web_manual_summary-20260802-112952 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
