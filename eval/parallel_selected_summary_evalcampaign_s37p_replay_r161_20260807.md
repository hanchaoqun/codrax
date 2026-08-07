# Selected parallel eval sweep

- date: 2026-08-07T11:39:24Z
- sweep_start_ts: 20260807-043923
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_sequence_analyzer_gate | FAIL | no_text_regex_match:buildAnalysisIR.*analyzerGraphForNormalize.*analyzer\.go:[0-9]+ | 218s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260807-043925 |
| 2 | sr_rust_cross_module_chain | FAIL | degraded_answer_checks_skipped:1 | 254s | 1 | 1 | 0 | 1 | 0 | 7 | 6 | 0 | 0 | 0 | none | eval/results/sr_rust_cross_module_chain-20260807-043925 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
