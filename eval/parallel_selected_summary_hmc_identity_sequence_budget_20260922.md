# Selected parallel eval sweep

- date: 2026-09-23T01:24:57Z
- sweep_start_ts: 20260922-182445
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_identity_sequence_budget_20260922

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_e2_cross_trace_asymmetry | FAIL | no_text_regex_match:(时基|时间基准|时间轴|基准|时钟|clock|timebase).{0,60}(不同|不一致|不能直接|� | 150s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_identity_sequence_budget_20260922/real_trace_e2_cross_trace_asymmetry-20260922-182457 |
| 2 | qf_sequence_analyzer_gate | PASS | - | 362s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/hmc_identity_sequence_budget_20260922/qf_sequence_analyzer_gate-20260922-182457 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
