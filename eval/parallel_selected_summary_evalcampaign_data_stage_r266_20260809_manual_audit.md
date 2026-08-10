# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T06:18:19Z
- sweep_start_ts: 20260809-231817
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260809-231819 | answer_regex,answer_contains,mermaid_edge_count | none | 376s | 39 | read=12,repo_map=2,list=0,trace=0,source_lens=0 | midloop=8,inv=4/0,fin_reject=5,unavail=0,prune=0 | fail | Explorer 在 operation 补证轮读到 `dispatchStage -> ctxbuilder.BuildAgentContext`，但发射的 relationship item 缺 `line_start`，`emit_evidence` 将其 SKIPPED，同时报告 `Current actionable repair targets: none`，没有 typed ToolRepair。completion 随后以 `flow_operation_carrier` low-delta 收敛，B452 participant lane 因前置 operation 仍缺失而未触发。Finalizer 5 次拒绝后删除全部边，required Mermaid 为零边；系统补写阶段表已退役，但 current-run stage authority 未触发，正文仍含混地把 TaskGraph/EvidencePlan 等确定性编译产物归到 Analyzer 输出。不是单次模型波动。 |
| 1 | data_multifile_reference_projection | PASS | eval/results/data_multifile_reference_projection-20260809-231819 | log_regex,answer_regex | none | 477s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 精确输出 `17,0,5`；4 份材料、9 条规则、22 条 decisions、4 条 contributions、reconcile=pass、complete reference projection 与 terminal artifact 闭合。r265 的中间 prefix `validate data workflow result: ... contributions is empty` 本轮为 0，证明 B455 的 deferred terminal intent 修复生效；最终 terminal rank 仍执行完整校验。仍有 11 batches/4 repairs/5 action failures，主要来自模型生成的陈腐 artifact path、错误字段和 alias 冲突，不以 PASS 掩盖效率债。B454 的 generated-status carrier 本轮未触发，继续等待定向 witness。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- Runner/human：`1 PASS / 1 FAIL`，与 runner 一致，但 QF 的失败机制必须按系统 GAP 记账，不能归为模型随机波动。
- `B455=production-closed`：中间态完整终验误触发归零，最终终态合同保持。
- `B452=production-replay-failed/prerequisite-repair-gap`：participant coverage 本身未被执行到，不能以 bounded caveat 宣称关闭。
- `B453=partial`：系统阶段补表已退役；producer provenance 的模型上下文触发仍不充分。
- 新立 `EVAL-B456-EVIDENCESKIPREPAIR1/P0`：结构化证据批次中 load-bearing relationship row 被 schema 校验跳过时，必须形成精确、局部、可执行的 typed repair debt；不能一面称“无 actionable target”，一面靠 prose 建议模型自行判断是否重发。
- 新立 `EVAL-B457-STAGEAUTHORITYREACH1/P1`：checkout 已验证的 current-run stage authority 不能只依赖本次未生成的 `stage_or_workflow` dimension；应从既有 typed diagram/flow participant 与 current-source authority 达成结构化触发，仍不得向任意客户仓注入 Codrax 内部架构。
