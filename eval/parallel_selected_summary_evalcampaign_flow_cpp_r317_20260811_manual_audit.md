# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T14:20:29Z
- sweep_start_ts: 20260811-072027
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260811-072029 | answer_regex,answer_contains | none | 179s | 24 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | 正文正确列出工厂选择、guard、返回 `ConsoleSink`、`Logger.log -> Sink.write`、虚分派和 `ConsoleSink.write -> fputs`；但 required call-DAG 首稿把 guard/return/dispatch 画成无 typed anchor 的箭头，被 validator 正确拒绝。copy-ready patch 随后只保留三条静态 call，最终图成为三个互不连接的片段，与“完整调用路径”正文不一致。关系权威没有造边，但系统提供的 call-DAG 修补骨架缺少表达 typed return/type-relation/dynamic-dispatch boundary 的注释层，自动 PASS 未覆盖图的连通语义。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260811-072029 | answer_regex,answer_contains,mermaid_edge_count | none | 326s | 40 | read=10,repo_map=6,list=0,trace=0,source_lens=0 | midloop=12,inv=3/0,fin_reject=5,unavail=0,prune=0 | fail | B534/B535 生产生效：第一次 completion 同轮发布 stems/files，第二次未强制收敛；模型执行 grep、读取 `ctx.Mutable.ResetPrescanSummary/SetPrescanRoundLimit` 并发出两条精确 call row，第三次才 caveated close。残余 gap 是 participant `Mutable` 无法与精确 endpoint `ctx.Mutable.<method>` 的 receiver/owner 对齐，故 Mutable 与 BusContext 仍同时被判 no-incident；Finalizer 经 5 次拒绝后把二者保留为孤立 unproven 节点，未交付用户要求的共享状态数据流。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
