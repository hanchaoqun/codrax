# Selected parallel eval sweep

- date: 2026-08-01T05:57:49Z
- sweep_start_ts: 20260731-225748
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | read_combo_git_two_diffs_current_code | PASS | - | 128s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_git_two_diffs_current_code-20260731-225749 |
| 1 | trace_query_donghu_real_frame_multicausal | FAIL | no_text_regex_match:(代表|典型|representative).{0,200}34579\.(4[7-9]|5[0-8])|34579\.(4[7-9]|5[0-8])[0-9]*.{0,200}(� | 172s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260731-225749 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
