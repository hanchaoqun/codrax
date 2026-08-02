# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T12:43:21Z
- sweep_start_ts: 20260802-054321
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_text_filter_count | PASS | eval/results/data_text_filter_count-20260802-054321 | log_regex,answer_regex | none | 44s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | route=data；AST guard 在执行前拒绝漏读 instructions.md 的 terminal script，repair 后 data_rounds=1、repair_rounds=1、action_failed=0，最终严格只有 `2`。r7 的多轮 DAG 未复现，判模型效率波动。 |
| 2 | trace_query_binder_ipc_peer | PASS | eval/results/trace_query_binder_ipc_peer-20260802-054321 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 118s | 28 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=2,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | FACTFAMILY1 验收通过：无状态卡/状态 typed 附注/full supplement/floor/因果投影，三项 principal facts 正确。但模型额外把 3.015→3.030 的 15ms 总 non-running 错称“睡眠到被唤醒”；真实 t_sleep→t_wake=5ms，t_wake→t_run=10ms runnable。另有 runtime citation 被转为 artifact selector 后仍显示“已移除引用”的 P3 可读性 watch。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
