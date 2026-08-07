# Selected parallel eval sweep

- date: 2026-08-07T14:20:33Z
- sweep_start_ts: 20260807-072032
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | data_json_strict_ids | PASS | - | 46s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260807-072033 |
| 1 | qf_sequence_analyzer_gate | FAIL | no_text_regex_match:buildAnalysisIR.*analyzerGraphForNormalize.*analyzer\.go:[0-9]+ | 196s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260807-072033 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
