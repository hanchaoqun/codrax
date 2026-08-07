# Selected parallel eval sweep

- date: 2026-08-07T12:46:31Z
- sweep_start_ts: 20260807-054630
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_sequence_analyzer_gate | FAIL | no_text_regex_match:buildAnalysisIR.*analyzerGraphForNormalize.*analyzer\.go:[0-9]+ no_text_regex_match:gate\.Run([^A-Za | 153s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260807-054632 |
| 1 | sr_rust_cross_module_chain | PASS | - | 160s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_rust_cross_module_chain-20260807-054632 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
