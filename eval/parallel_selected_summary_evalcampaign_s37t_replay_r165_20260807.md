# Selected parallel eval sweep

- date: 2026-08-07T13:03:59Z
- sweep_start_ts: 20260807-060358
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | read_combo_answer_document_tools | PASS | - | 126s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_answer_document_tools-20260807-060359 |
| 1 | qf_sequence_analyzer_gate | FAIL | no_text_regex_match:buildAnalysisIR.*analyzerGraphForNormalize.*analyzer\.go:[0-9]+ | 322s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260807-060359 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
