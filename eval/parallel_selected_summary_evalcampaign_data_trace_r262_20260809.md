# Selected parallel eval sweep

- date: 2026-08-10T03:59:25Z
- sweep_start_ts: 20260809-205923
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | data_multifile_reference_projection | FAIL | read_exit:1 data_terminal_status:failed no_log_regex:\[cli/data\] data task result.*contributions=[1-9][0-9]*.*reconcile | 115s | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260809-205925 |
| 2 | real_trace_h7_self_seat_full_spectrum | FAIL | missing:49.638 missing:0.105 no_regex_match:同源二分:全窗49\.656ms=锚定0\.018ms\(⛓链上席\)\+本行其余49\ | 143s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h7_self_seat_full_spectrum-20260809-205925 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
