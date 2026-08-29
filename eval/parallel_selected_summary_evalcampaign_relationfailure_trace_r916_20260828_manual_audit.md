# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T01:20:06Z
- sweep_start_ts: 20260828-182004
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260828-182006 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 207s | 39 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 窗、threadpool-400→network-300→cookie-200→app-100 完整链、11.000ms 链上 iowait 主席、三个 1.000ms 调度供给验证候选、实际占时/现规则可消双账户、链外背景隔离、业务下钻方向、完整 Trace 因果投影和自动补采均在；无成文拒绝、无固定 4ms/4m 或活动流降级。模型将 3.000cpu·ms 供给压力与系统补充的 9.000cpu·ms 进程内总 CPU 占用分别引用，两者来自不同 typed lane 并不矛盾，但摘要没有显式说明差别，记 B1429/P2 可读性观察，不扫描正文硬门。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260828-182006 | answer_regex,answer_contains,mermaid_edge_count,typed_diagram_participant_coverage | none | 433s | 40 | read=26,repo_map=4,list=0,trace=0,source_lens=1 | midloop=12,inv=7/0,fin_reject=3,unavail=0,prune=0 | partial | B1427 生产正证：同代 typed relation failures 均获得 live `failure_ref`，模型第 3 次修补能原子删除两条身份冲突边和一条 body-only 无锚边，从 r915 的 16 次拒绝/旧稿降级收敛到 3 次拒绝/正常出稿。但人工仍只判 partial：生产者给出两条 local-only typed `addition_ref`，并明确允许 BusContext/Mutable 作显示端点；原子执行器接受后，后续精确身份门却因 local-only 显示权限未进 authority roster，把同一边判为 `edge_anchor_node_identity_conflict`。模型最后只能删掉这两条已证局部关系，图中 Mutable/BusContext 数据流仍缺席。确认 B1428 合同自冲突，不是证据缺失或模型波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
