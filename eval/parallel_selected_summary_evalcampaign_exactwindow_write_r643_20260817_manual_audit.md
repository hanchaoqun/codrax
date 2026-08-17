# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T21:37:20Z
- sweep_start_ts: 20260817-143719
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260817-143720 | write_apply,write_patch_oracle,answer_contains | none | 103s | 25 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 计划、应用、验证三段闭环。applied-tree 相对 seed 只把 main.go 一行 `retrun` 改为 `return`；post_apply_verify 执行 `go test -json ./...` exit=0，changed_path_coverage=main.go，最终状态 verified。无重规划、成文拒绝或跨批验证域丢失。 |
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260817-143720 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 268s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B1013 生产回放转正。模型的 window_stats/wakeup_chain/root_cause_rank 均使用用户精确窗 2.000..2.020，系统仅在同窗补 critical_blocking_calls；头行、◎、树、明细和证据索引共同发布 threadpool-400 io_wait 11.000ms 为链上 #1，r642 的“头行加冕但榜面 context-only”矛盾消失。邻近/背景未升格，Trace 因果投影与自动补齐保留；活跃流 268s 未固定 4ms 降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
