# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T01:46:26Z
- sweep_start_ts: 20260818-184624
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_tokenizers_newline_run_multirepo_py | PASS | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260818-184626 | log_regex,write_apply,answer_regex,answer_contains | none | 389s | 25 | read=6,repo_map=2,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | B1121 已生产生效：analyzer 发出 exact-path `preserve_regression_test`，五换行测试未改，旧 self-fulfilling 绿消失。但新增实现把单个 `\n` 也直接替换成 rank；`count % 2` 两分支完全相同。既有 suite 只有普通 `hi` 与五换行正例，未验证 feature 已启用时的 below-threshold/no-op 边界，runner PASS 不能证明“普通 merge 行为保持”。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260818-184626 | answer_regex,answer_contains,mermaid_edge_count | none | 685s | 48 | read=27,repo_map=3,list=0,trace=0,source_lens=0 | midloop=23,inv=18/6,fin_reject=2,unavail=1,prune=0 | fail | B1123 已让模型拿到并尝试画 `bus.Mutable -> AgentContext.Mutable` 真 data-flow；但 dotted node 经 Mermaid shim 改成内部 ID 后，anchor 的另一端仍是技术 identity，关系门把同一条边拆成 missing-anchor 与 orphan-anchor。修补回合删掉真边才通过，终图仍缺共享状态主数据流；正文还把 getter `EmittedAnswerSymbols()` 误述为 Extractor 写入。探索阶段另有耗尽 completion 工具继续暴露的合同矛盾，造成 18 次完成调用/6 次拒绝。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual findings

### Read / diagram

- B1123 获生产正证：第一稿及第一轮 patch 均携带 exact
  `from_identity=bus.Mutable, to_identity=AgentContext.Mutable, relation_kind=data_flow`，证明成员 initializer 的
  parser-owned container 已进入 Finalizer；旧裸 `Mutable` typed endpoint 不再是主阻塞。
- B1124 本轮没有以 participant candidate 截断形触发：首稿已从 typed recipe 主动选择 data-flow，第一次 participant
  candidate map 只需修复 extractor precedence。因此只能记 no-regression，不能虚记生产闭环。
- 新确认 B1125：第一轮 patch 在 body 使用 `Mutable -> AgentContext.Mutable`。Mermaid compatibility shim 把 dotted
  target 改成 `codraxNode1`，anchor target 同步成功；但 source anchor 仍为 `bus.Mutable`，而 body 的 source node id 是
  `Mutable`。关系门遂同时报 `missing_relation_anchor Mutable->codraxNode1` 与
  `typed_anchor_without_visible_edge bus.Mutable->codraxNode1`。二者实际是同一 model-authored relation 的单侧 node-ref
  漂移；系统没有给出或执行唯一的 metadata-only 对齐，模型第三轮删除该真边才通过。
- 最终图保留四阶段 precedence、NewMutableState→Mutable、dispatchStage→BuildAgentContext 等局部真边，但题目所求的
  BusContext/Mutable→AgentContext 共享数据流仍缺。正文却继续声称 `AgentContext.Mutable` 指向同一实例，并把
  `ctx.Mutable.EmittedAnswerSymbols()` getter 写成 Extractor“写入答案符号”，答案图文不一致且读/写语义越证据。
- 探索阶段累计 500 条证据、27 reads、23 midloop、18 次 investigation_complete、6 次拒绝、95k Finalizer context。
  日志明确出现“只允许 emit_investigation_complete”同时该工具已 budget exhausted，随后模型尝试 schema 外
  emit_evidence。立案 B1127：可调用 schema 与 typed runtime budget 不一致；耗尽动作必须撤权或提供明确 stop/close
  出口，不能让模型在不可能合同上继续重试。

### Write / verification

- B1121 获生产正证：`emit_write_analysis` 明确发
  `preserve_regression_test target=tests/test_tokenizer.py`，controller/context pack 均携带该 P0，最终 plan 只改
  `fastlex/tokenizer.py`，五换行输入和 `[...300]` baseline oracle 字节保持。
- B1122 本轮无 Make 内层失败，故只记单测已实现、生产未触发；不能以 suite 一次通过虚记正证。
- 新实现对任意 newline run 都追加一个 rank；单个 newline 也从 `10` 变成 `300`。注释声称 odd count 留一个 10，
  但 if/else 两臂完全相同。这个反例违反 pair-rule 的最小激活边界和“现有普通行为保持”，不是五换行 oracle 问题。
- 新确认 B1126：行为合同已写“任意长度（包括奇数）”且用户允许缺覆盖时加测试，Planner 仍只复用既有 suite，既没有
  below-threshold/no-op，也没有 minimum-trigger/mixed-neighbor probe。最优方案是通用软教学：对 cardinality/range/
  sequence transformation，检查边界分区而不是从一个 nominal regression 外推全域；不以 raw task 关键词作硬门。

### Priority

1. **P0/P1 B1125**：只在一个 orphan typed anchor 与一个未拥有 visible edge 共享同向一端且匹配唯一时，机械同步另一
   `from_node/to_node`；保持 body、identity、relation、direction、label 全字节不动，歧义 fail-closed。
2. **P1 B1127**：typed budget/tool-schema 同源；耗尽 completion 工具不得继续作为唯一 callable action，提供 stop/close
   出口并 pin 不可调用工具不再诱导重试。
3. **P1 B1126**：ChangePlan 软教学补通用边界分区验证；不增加答案/请求字符串硬门，也不让系统选择实现或期望值。

两案均无畸形 JSON 恢复、旧答案降级、空答案或 active-stream 4ms 固定年龄降级。本批不涉及 Trace 查询、显式窗、
因果投影、自动补齐、链上-only 主因、邻近/背景 support-only 或实际占时/规则可消双轴。
