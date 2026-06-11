# Eval 批 4 — data-lane reconcile 过触发(结构 gap)+ 残余 planner 行为

## 1. 批 4(6 案)结论

m2a / u1a / cond_resolve_first_byte_timeout / logtri_degraded **4 案 PASS**;两案 FAIL,性质不同:

- **trace_query_converted_inode_io_pressure**:答案**实质完全正确**(ino=0xb9b8e / dev=260:136 / entry_name=foo.db / file_io / page-cache churn / f2fs+scsi latency / io_pressure 全中)。FAIL 是 case-spec 的 inode 正则**强制字段顺序**(要 ino→dev,答案是 dev→ino),且多条严格正则与 LLM 措辞抖动冲突。**修法**:去掉 inode 行的顺序约束(三事实改各自独立必含,`EXPECT_CONTAINS` 本就要求全部三者)。trace lane 的输出稳定性(绝对路径/邻接措辞)是模型抖动,非系统结构 gap。

- **data_json_strict_ids**:暴露**真结构 gap**(下文)+ 残余 planner 行为。

## 2. 结构 gap:reconcile 由"答案组装"误触发(P1,已修)

**问题类**:纯提取任务(读 users.json,按 instructions.md 输出 active 用户 id 列表,**无任何数值聚合**)被强制走数值对账并必然失败。两层结构根因:

1. **契约过触发**:`assemble_answer`(几乎每个 data plan 的收尾动作)产 `LedgerFinalProjection`,而 `normalizeDataTaskPlanContractFromActions` 见到 final_projection 即强制 `reconcile_required=true`——把每个有答案组装步骤的 plan(即几乎全部)拖进数值对账。**修法**:reconcile 改由 `LedgerContributions`(真数值聚合,结构事实)驱动,而非 `LedgerFinalProjection`(答案组装)。精确信号:**对账你数值聚合过的东西**。

2. **多组假交叉校验**:`runReconcileArtifacts` 把多组对账的 `ExpectedAnswer` 设为组摘要 join 串(`u1/_source_index=1; u3/...`),validator 又拿它与最终答案交叉校验——但多组组摘要是内部产物,从不等于最终答案(列表/对象/表格)。**修法**:仅单组(标量答案)设 ExpectedAnswer;多组留空,per-group 校验照常、答案级交叉校验正确跳过。精确信号:组基数。

3. **答案级回退缺失**(配套):skill 承诺 final-output 任务用 `scope="answer"` 对账,validator 也已支持(无参与贡献时空组报告 pass),但 `runReconcileArtifacts` 提前在无数值组时报错,从未走答案级路径。**修法**:结构非聚合(无 compute_contributions)且无参与数值对账的贡献时,产出答案级 pass 报告而非报错;空贡献仍报畸形依赖错误,真聚合产零组仍 fail-loud。

三项均 typed/结构信号驱动,零关键字匹配。单元测试双向钉死(非聚合答案级 pass / 聚合零组仍 fail);67 包全量回归绿;`data_multifile_reference_projection`(聚合案)始终 PASS——**稳定场景零回归**。

## 3. 残余(已记录,不在本轮追入不稳定)

- **data-lane LLM planner 过度工程化**:即便移除上述结构强制,planner 仍反复把纯提取自建为数值聚合管线(造 compute_contributions + entity_resolution + reconcile,用 `_source_index` 当 metric)。这是 planner 行为/prompt 质量的更深层问题,含模型抖动成分;结构 gap 已移除架构陷阱,但无法强制 LLM 选简单 plan。**需独立专项**(planner 软引导强化),不在本轮以免踩稳定场景。
- **data_basic_sum_with_rules** 同批附带核查:FAIL 原因为 `rule_id "RULE_1"` 大小写引用不匹配 + 末阶段 custom_transform 禁用,**与 reconcile 改动无关**(reconcile_sum 成功执行),属既有 data-lane 脆弱点。
- **trace lane 输出稳定性**:绝对路径/邻接措辞的模型抖动,非结构 gap。

## 4. 任务列表

- [x] reconcile 改由 LedgerContributions 驱动(移除 final_projection 过触发)。
- [x] 多组对账不设 ExpectedAnswer(移除假答案交叉校验)。
- [x] runReconcileArtifacts 答案级回退(交付 skill 承诺的 scope="answer")。
- [x] inode case-spec 去顺序约束。
- [ ] 残余:data-lane planner 过度工程化软引导(独立专项)。
