# Selected parallel eval sweep

- date: 2026-08-17T23:38:40Z
- sweep_start_ts: 20260817-163838
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | - | 251s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_runnable-20260817-163840 |
| 1 | read_combo_config_two_knobs_precedence | FAIL | degraded_answer_checks_skipped:1 no_regex_match:(代码默认值[^0-9]{0,24}3([^0-9]|$)|code default[^0-9]{0,24}3([^0-9] | 987s | 1 | 1 | 0 | 1 | 0 | 14 | 13 | 0 | 0 | 0 | none | eval/results/read_combo_config_two_knobs_precedence-20260817-163840 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
