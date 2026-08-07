# Selected parallel eval sweep

- date: 2026-08-07T13:37:19Z
- sweep_start_ts: 20260807-063717
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_java_call_chain | PASS | - | 244s | 2 | 1 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/sr_java_call_chain-20260807-063719 |
| 1 | qf_sequence_analyzer_gate | FAIL | no_text_regex_match:buildAnalysisIR.*analyzerGraphForNormalize.*analyzer\.go:[0-9]+ | 265s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260807-063719 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
