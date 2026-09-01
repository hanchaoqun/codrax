# Selected Eval Manual Audit Scaffold

- date: 2026-09-01T12:00:56Z
- sweep_start_ts: 20260901-050054
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260901-050056 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 198s | 43 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=2/2,fin_reject=0,unavail=0,prune=0 | pass | 显式 34579.490–34579.500s 仍精确计为 10.000ms；主因只从已证链选出 NetworkService-60595 优先级反转候选，有效归因 5.951ms，并单独披露帧因果未证。实际状态占时与规则可消除量分开，链上业务 span 保留排查方向，邻近 IO/压力仍为背景；Trace 因果投影与确定性补采均发布，未见固定 4ms/4m 或活跃流年龄降级。独立旁路 gap：模型已选择 5 个 typed candidate，却连续生产形把固定 schema_version 放到文档外层；系统删除该未知字段后按 version=0 忽略报告，因此本轮默认 `.root-causes.json` 仍缺失，记 B1544，不影响长答案正确性。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260901-050056 | answer_regex,answer_contains | none | 473s | 43 | read=13,repo_map=2,list=0,trace=0,source_lens=0 | midloop=8,inv=2/0,fin_reject=3,unavail=0,prune=4 | fail | B1542 获生产正证：finalizer reject 从 r1008 的 10 降到 3，空 block_id、缺 addition_ref/edge 字段和静态 repair surface 误教全部消失；一次整块替换被精确拒绝后，模型成功用当前 failure_ref/addition_ref 原子修复并完成 orphan disposition。最终正文和阶段表正确，但图仍只剩 Analyze→Explore→Extract→Finalize；Orchestrator 调度和 BusContext 状态交接仍只在正文/表里，未进入图的 typed relation inventory，B1541 继续确认。系统不能为补图自动造边或保留节点，需从已证调度/读写关系发布角色覆盖候选。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
