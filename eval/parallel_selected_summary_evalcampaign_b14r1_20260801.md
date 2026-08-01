# Selected parallel eval sweep

- date: 2026-08-01T00:59:04Z
- sweep_start_ts: 20260731-175902
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | mr_poly_binding_chain | PASS | - | 136s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260731-175904 |
| 1 | real_trace_h3_iofam_one_seat | FAIL | missing:IO延迟 missing:块设备层 missing:块设备IO(inode) missing:综合评分,非墙钟 missing:完成端到端� | 184s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h3_iofam_one_seat-20260731-175904 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
