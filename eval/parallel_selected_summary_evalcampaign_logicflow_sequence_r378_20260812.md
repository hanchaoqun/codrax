# Selected parallel eval sweep

- date: 2026-08-12T09:30:34Z
- sweep_start_ts: 20260812-023032
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_sequence_analyzer_gate | PASS | - | 183s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260812-023034 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 526s | 1 | 2 | 0 | 1 | 0 | 4 | 4 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260812-023034 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
