# Selected parallel eval sweep

- date: 2026-08-12T14:28:32Z
- sweep_start_ts: 20260812-072831
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_java_call_chain | PASS | - | 116s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_java_call_chain-20260812-072833 |
| 1 | qf_sequence_analyzer_gate | PASS | - | 267s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260812-072833 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
