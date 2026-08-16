# r568 人工审计：关系补采生产出口与写判定域

- 基线：`main@b20e272dc208`
- 并发：严格 `PARALLEL=2`，案例为 `read_combo_answer_document_tools` 与
  `github_issue_libgit2_foreach_worktree`
- runner：`2 PASS / 0 FAIL`
- 人工：Combo `partial`；write `pass`

## 1. Combo：B905 已止住死循环，但关系本身仍未回答

1. Explorer 逐行确认两个 `Name()` literal 与首次完整输出/retry patch 职责，但没有形成任何
   citable directed operation；completion 首次正确进入 `flow_operation_carrier_evidence` 补采。
2. Analyzer 的 required flow diagram 合法携带空 `diagram_hint.participants`。现有补采器只从显式
   participant slate 选 parser-owned operation target，因此没有把已经 resolver-confirmed 的
   `emit_answer_document` / `emit_answer_document_patch` 实体升级为软导航种子，也就没有定向到
   `cmd/root.go:4315/4319` 的两条真实注册操作。
3. 模型随后只在 `internal/agent/answer_document_evaluator.go:1-100` 做了一次无命中 grep，未读
   `buildToolSchemas`、`skillToolSuggestionBlocked`、`FilterToolSchemas` 或注册点；第二次 completion
   立即按 `flow_operation_carrier` 两次同 blocker 收敛，发布
   `relation_surface_exit=typed_unproven_node_only`。
4. Finalizer 第一稿画了五条未证关系，validator 正确拒绝；第二稿只是把同一组边改名为
   `register/data_flow/precedence`，仍没有 source owner，再次正确拒绝。B905-C 随即给出短的可执行
   node-only 修复，第三稿收敛，未再发生 r567 的 25 次风暴。
5. 最终 literal 和表格正确，Mermaid 语法有效，但图只有四个节点、零箭头，且正文没有明确写出
   “关系未证”。所以 runner PASS 不能签人工 pass：用户要求的 finalizer 关系仍未得到解释。

结论：`B905-A/B/C` 获得生产正证（2 次拒绝后有界诚实退出），但发现新的通用补采断层
`B907-FLOWEMPTYSLATENAVIGATION1`。最优方案不是放松关系证据门，也不是系统代画图，而是：当必选
source-flow 图没有显式 participant 时，仅把 typed resolver 已确认、`UseForSearch/UseForShape` 的
symbol 实体用于软导航；最终关系仍必须由模型读取 operation site 后通过 `emit_evidence` 发证。与此同时
`flow_operation_carrier` 需像 participant coverage 一样允许“定位→读取/发证”两个修复回合，第三次同
blocker 才可诚实收敛。

## 2. Write：B906 获生产闭环

1. 产物只修改 `repository.c`；没有测试文件改动、额外批次或 durable goal 漂移。
2. callback 分支修为 `(error = cb_result) != 0`，lookup 分支修为
   `(error = lookup_result) < 0`，没有再次把任意非零 callback 语义缩成负值-only。
3. `make check` 实际执行并通过四个分区：callback `-42`、`17`、`0` 以及 lookup `-7`；新增正非零
   witness 确定性拦住 r567 的错误修向。
4. Planner 的计划明确保留四个行为结果，最终只有一个 plan、一个 patch、一次 verify，终态
   `verified`。这证明通用 typed behavior-domain 软引导加真实正/负/零测试面能够改善模型决策，无需扫描
   request/patch/final prose，也无需新增运算符正则硬门。

## 3. 不变量复核

- 本批没有运行 Trace 案例，也没有修改 Trace query、显式窗、因果投影或自动补齐；后续 B907 必须继续
  明确排除 `QFRootCauseTrace`，主因仍只来自 typed on-chain，邻近/背景仅作 support。
- 系统未生成模型关系、Mermaid 或结论；B905 node-only 出口只改变可满足性，实际节点和披露仍由模型写。
- 两案均未出现畸形 JSON、旧稿恢复、空答案或 active-stream 固定年龄降级；持续字节流下 4ms 不授权降级。

状态：

`B905-REQUIREDDIAGRAMZERORELATIONCONTRACT1=production-positive/bounded-node-only-exit-r568`；
`B906-WRITEORIGINALCONDITIONDOMAIN1=production-closed-r568`；
`B907-FLOWEMPTYSLATENAVIGATION1=implemented/resolver-symbol-soft-navigation+3-attempt-bound/pending-replay`；
`raw-request/model/final-prose-hard-gate=none`；
`system-answer/diagram/relation/conclusion-authorship=none`；
`active-stream-fixed-age-degrade=forbidden/not-observed`。
