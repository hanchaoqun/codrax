# Selected parallel eval sweep

- date: 2026-08-15T16:04:19Z
- sweep_start_ts: 20260815-090418
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | data_json_strict_ids | PASS | - | 250s | 0 | 0 | 0 | 0 | 2 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260815-090419 |
| 1 | qf_sequence_analyzer_gate | FAIL | no_text_regex_match:gate\.Run([^A-Za-z0-9_]|$).*RunWith.*gate\.go:[0-9]+ | 298s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260815-090419 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
