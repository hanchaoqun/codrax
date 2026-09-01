# Selected Eval Manual Audit Scaffold

- date: 2026-09-01T11:01:26Z
- sweep_start_ts: 20260901-040125
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260901-040126 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 186s | 43 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 用户指定的 34579.490–34579.500s 窗保持 10.000ms；主因只从已证链选出 NetworkService-60595 优先级反转候选，有效归因 5.951ms，并明确帧因果未证。实际状态占时与规则可消除量分列，业务 span 保留为链上线索，邻近 IO/压力只作背景；Trace 因果投影与确定性补采均在，未见固定 4ms/4m 或活跃流年龄降级。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260901-040126 | answer_regex,answer_contains | none | 663s | 54 | read=29,repo_map=2,list=0,trace=0,source_lens=0 | midloop=24,inv=6/1,fin_reject=10,unavail=0,prune=3 | fail | B1540 已生效：既有 `BusCtx as BusContext` 未再被判非 typed carrier，也未生成第二个 BusContext。B1538 也获生产正证：关系阶段结束后精确发布 Extract/Orchestrator 两项孤立节点处置；模型自行删除二者，完整主链门随后正确拒绝并要求恢复。最终正文/表基本正确且图收敛为 Analyze→Explore→Extract→Finalizer，但正文称 Orchestrator 是唯一中枢，图却无 Orchestrator；BusContext 仅声明未连边，未呈现各阶段共享状态流。10 次 finalizer reject 中还含禁用整块替换、两次空 block_id、缺少 addition edge 字段等 JSON 分支误用，答案关系完整性与模型心智负担仍不合格。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
