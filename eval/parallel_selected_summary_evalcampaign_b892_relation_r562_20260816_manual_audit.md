# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T10:41:31Z
- sweep_start_ts: 20260816-034129
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | PASS | eval/results/real_trace_h4_supply_thermal_witness-20260816-034131 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 141s | 36 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B892 same-key join 已进入 prompt，CPU12 不再借用 CPU4 policy，最终“直接影响无法确认”边界正确；但正文反而把已有 `CPU4 target_running=35.960ms + same_cpu_target_running_frequency=558000kHz + same_cpu_policy=present` 写成“CPU4 无同核目标运行记录”，并把最大贡献 CPU12 无权称为“主核”。四态表还生成 5 cells/4 columns，且 `coverage_status=complete` 等内部 enum 继续泄漏。自动 PASS 未覆盖这些事实/展示错误。 |
| 1 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260816-034131 | answer_regex,answer_contains | none | 540s | 42 | read=5,repo_map=1,list=0,trace=0,source_lens=1 | midloop=13,inv=3/0,fin_reject=8,unavail=0,prune=0 | fail | Analyzer 连续 6 次被 snake_case/CamelCase 同身份及短标识符前缀误判成“遗漏共同参与者”；最终 accepted payload 又完全遗漏 `call_chain_endpoints`。探索因此未建立首次/full 与 retry/patch 选择证据。正文错误宣称 `lastEmitFromPatch` 决定工具选择；真实选择由每轮 schema refresh、patch-base availability 与 evaluator preference/force-full 状态共同完成。图只有 ParametersFor/Name 局部边，没有用户要求的选择关系。8 次 finalizer reject 是缺失主证据后在 participant/return 图合同间反复修补，runner PASS 为假阳性。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. Trace 的 typed join 已把 CPU4 行精确并置为
   `target_running=35.960ms + target_running_frequency=558000kHz + policy=558000..2100000kHz`，并明确
   “没有 target-slice overlap/binding carrier”。模型保留了正确的 unproven 总结，却在表中把该行写成
   “无同核目标运行记录”；这是模型没有一致消费已提供上下文，不授权系统扫描/改写正文。本轮先记生产
   fail，继续异构回放；若重复，再从 typed 行的紧凑度/优先级而非关键词硬门处理。
2. 组合题不是模型波动。分析拒绝日志确定性显示：`emit_answer_document` 被误认成另一个
   `EmitAnswerDocument` 实体，`emit_answer_document_patch` 又因标点剥离的 substring 比较同时命中短实体；
   这使合法 participant slate 形成重试风暴。
3. 更深合同断层是 `runtime_selection_required` 文档宣称独立，但代码要求 endpoint profile `Active()`，并且
   非 `call_chain+call` 结果会整对象丢弃；mechanism/flow 无法把“首次/重试选择”义务传给 Explorer 和
   Finalizer。`lastEmitFromPatch` 只是结果谱系观测，不是选择器，最终答案的核心关系因此错误。
4. 修复方向冻结为：完整 typed 标识符边界（含 snake/Camel 等价、禁止前缀命中）；runtime selection 与
   endpoint authority 解耦；非 call-chain 也发布同一 selection evidence handoff；允许与已证 call/
   registration endpoint 相连的 guard 成为 availability 选择事实，孤立 guard 仍 fail-closed。所有判断只读
   schema-valid typed 字段，不扫描请求/模型/答案原文，不由系统生成关系或结论。
