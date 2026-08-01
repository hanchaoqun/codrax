# Selected parallel eval sweep

- date: 2026-08-01T08:43:28Z
- sweep_start_ts: 20260801-014327
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_c2_dstate_iowait | FAIL | no_principal_text_regex_match:((时长（ms）.*34579\.451701.*0\.138.*34579\.452934.*0\.147.*34579\.471372.*0\.350)|(34 | 161s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c2_dstate_iowait-20260801-014328 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 164s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260801-014328 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
