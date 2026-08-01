# Selected parallel eval sweep

- date: 2026-08-01T02:18:29Z
- sweep_start_ts: 20260731-191827
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | read_combo_config_absent_present_mix | PASS | - | 150s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_config_absent_present_mix-20260731-191829 |
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | FAIL | no_regex_match:最晚相关边 34579\.496810s,凭证=直接裸边 | 177s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260731-191829 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
