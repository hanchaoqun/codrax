# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T09:11:24Z
- sweep_start_ts: 20260819-021123
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260819-021124 | answer_regex | none | 125s | 27 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=2/0,fin_reject=1,unavail=0,prune=0 | pass | 五条跨模块关系与五个可见列表引用逐条同位；一次拒绝只因 ordered relation list 未携带 edge_anchors，修补后图、列表、引用一致。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-021124 | answer_regex,answer_contains,mermaid_edge_count | none | 303s | 41 | read=13,repo_map=3,list=0,trace=0,source_lens=0 | midloop=6,inv=4/0,fin_reject=1,unavail=0,prune=1 | partial | B1154 合同分叉未复现；但两次 participant repair 都选择局部 Mutable 读取，未选择 BuildAgentContext→consumer 值路径，终稿仍为三块断图。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工结论

### Rust：B1153 生产闭环

- 终稿关系为 `main→run`、`run→collect_files`、`collect_files→walk`、
  `run→index_file`、`index_file→Matcher.is_match`；对应可见列表分别引用
  `src/main.rs:10`、`:20`、`src/walker.rs:6`、`src/main.rs:23`、`:30`。
- 模型首稿有错行/错 ordinal，typed repair 只把引用绑定到既有边，没有生成关系或替写答案。
- 无 JSON 尾片、重复恢复图、Mermaid 降级、活跃流年龄降级。

### QF：B1154 正证，B1155 未触发，新增 B1156

- pre-emit 与 post-finalizer 使用同一 participant evidence provider；r724 的
  `available_typed_incident_edge_not_rendered` 前后矛盾未复现，B1154 可收账。
- `flow_value_consumer_coverage` 次数为 0：模型没有先选择 participant-relevant 调用结果赋值，
  因而 B1155 的窄门按设计未触发，不能把本次 partial 误记为 B1155 逻辑失败。
- participant repair 第一次精确导向 `orchestrator.go:4775-4799` 的
  `env.WriteClosure = o.busCtx.Mutable.WriteClosure()`，第二次导向
  `tier1_floor.go:62-86` 的 `EmittedEvidence/StableInvestigationResultKind`；这些均是真实局部调用，
  但没有把 BusContext/Mutable 接到 analyzer/explorer/extractor/finalizer 数据流。
- 三次低增量后诚实收敛；终稿图仍分成阶段顺序、Mutable 构造、WriteClosure 三个孤立组件。
  文中还声称“用虚线表示 unproven 边界”，图里并无该虚线，属于 authoring partial，不以答案扫描硬修。

## B1156 根因与施工

1. 同一请求的 r724 能选中 `extract_work.go:15`，r725 在 Mutable 获得精确 symbol 身份后却退回局部调用。
   深审确认 `flowRepairDeclaredBindingSites` 对同一参与者的静态绑定先按文件名排序，再以 6 个文件截断；
   `BusContext` 这类高频类型的早期参数/字段占满预算后，真正拥有跨组件 handoff 的 binding 根本不进入候选评分。
2. B1156 以 parser-only 通用形修复：先在全部精确静态 binding 上用解析器 callable/call endpoint 计算
   “连接到另一 requested participant”的潜力和外部 receiver handoff 潜力，再应用原有文件/别名上限；
   源码文本仍只读取有界幸存者。
3. 最终候选再增加 `connectionGainRank`：连接“仍缺参与者→已覆盖参与者”的坐标优先于只在多个仍缺
   状态对象之间打转的局部坐标。它是软导航排序，不关闭 coverage、不生成 evidence/edge/diagram/answer。
4. 红转绿覆盖全部 `SupportedReadLanguages()`，并保留同调用结果整值消费者、跨参与者预算、已有高质量
   already-read join 等回归。

## 红线核对

- 本批不扫描用户原文、模型思考或答案原文作硬门；只读 analyzer typed participants、parser graph、
  静态 binding、call endpoint 与 read closure。
- 不改 Trace 路径：显式窗、因果投影、自动补齐、链上-only 主因、实际占用/业务线索与规则可消除量双轴保持。
- 流式输出不按 4ms 或累计年龄降级；本批两个 case 均未出现流式恢复/降级。
- 系统没有代写模型结论或图；无法证明的关系仍可诚实保持 unproven。
