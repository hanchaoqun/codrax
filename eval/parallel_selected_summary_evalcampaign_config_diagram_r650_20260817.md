# Selected parallel eval sweep

- date: 2026-08-17T23:10:31Z
- sweep_start_ts: 20260817-161029
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | read_combo_config_two_knobs_precedence | FAIL | no_regex_match:(代码默认值[^0-9]{0,24}3([^0-9]|$)|code default[^0-9]{0,24}3([^0-9]|$)|MaxRetriesPerStage.{0,48}(初 | 184s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_config_two_knobs_precedence-20260817-161031 |
| 2 | qf_logic_view_read_pipeline | PASS | - | 431s | 1 | 2 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260817-161031 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
