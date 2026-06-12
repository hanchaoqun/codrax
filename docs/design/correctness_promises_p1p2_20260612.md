# 正确性承诺兑现(P1)+ 数据已指向小步项(P2)——设计与交付账本(2026-06-12)

Baseline:`fd5b9e93`。来源:本线(eval 扩充 + 硬面 ×4 取证)与并行线(TypedDenials 通道修通)两 session 残余合并。红线:精确信号才进硬门 / §1.6 硬门必配 model-declarable typed 逃生道 / 禁关键字匹配散文 / 不重复造轮子 / 不破坏稳定场景 / R2' / prompt 零内部术语泄漏。三路 recon(206 tool calls)行级锚定后定稿。

## §0 当前系统 gap 全量清单(两线合并,2026-06-12)

**第 1 优先级(正确性承诺兑现,本账本交付)**
- GAP-1 预算耗尽绕完成门:explorer 5 条出口可不经 emit_investigation_complete 直达 extract(SuccessCriteria-pass / ShouldStop 含 react-iters cap / hard stall / blocked DAG / **step-drain——该路径连 checkTier1Floor 都不跑**);所有挂在完成调用上的 typed 义务结构性可绕(×4 实测 run2 实锤)。
- GAP-2 TypedDenials 站点缺位:通道指针修通(`d98daf56`)但仅 2 站点;enum 已含 oracle_symbol_unverified / drift_frame_relocated / evidence_subject_unverified 等 6 类,文档承诺的 4 类站点不存在;另有 perf-stall symbol 跨类错位(path-shaped class 下 stamp symbol → L1 symbol 门永不触发)。

**第 2 优先级(数据已指向,本账本交付)**
- GAP-3 L4 BODY-vs-evidence:盲点精确化(recon §4)——(a) block.Text 散文按红线不查;(b) item.Text 自由断言无 evidence 对照;(c) **principal scalar 值除 count 问题外零 evidence 校验(V1 literal-grounding 已退役,只剩 reject-summary matcher)**;(d) decision verdict 只查 view enum,EmittedHypothesisVerdicts typed 通道答案侧零消费;(e) cells 零 typed 门。最高价值单点 = (c)。
- GAP-4 predicate 声明率 2/4(攒数据,禁关键词);GAP-5 B3 残余 1 例(观察);GAP-6 sequence_table spec agent 名词复审(case 层)。

**独立专项/设计待启动(不在本账本)**:data-planner 3 案 6 形态;写模式 W1/E6/E7;finalizer Phase A(先 qf_arch ×4 验 bar)/Phase C/MCP/Survey。
**明确不做**:split provenance(并行线裁定);FileReadCoverageStore;三条低害观察。
**recon 顺带发现(本账本小修)**:`types/answer_document.go:263` 注释宣称 GroundCitation QuoteMatched 防线仍在跑——实际 V2 答案路径零调用方(孤儿),文档失实需改;EdgeAnchors 自证回路(伪 edge anchor 可为伪 exact_resolution 作保)记 watch。

## §1 批 1:预算耗尽完成 lane(GAP-1)

**原则**:完成义务高于步频(write F1 原话 "completion outranks pacing");复用既有收口点与既有 emit-only surface,零新轮子。

设计(全 typed,F1 同构):
1. **义务计算** `pendingCompletionObligations(busCtx) []string`(orchestrator 侧纯函数):初版一条——`Predicates.HasPerMemberTable && 无 member_set ∈ StableInvestigationAggregateFacts && 无 absence justification`;返回 typed 义务名列表,结构可扩(未来 absence/aggregate 义务同函数)。零散文。
2. **lane 触发**(一次/Run,结构性防重入):`!Mutable.IsInvestigationComplete() && len(pending)>0 && !laneFired`。挂三处汇聚:checkTier1Floor 两个调用点(orchestrator.go:5202 / :5229)之前 + step-drain 的 contractFailureBreak 头(该路径今天零检查)。**统一为单 helper** `runCompletionObligationLane(...) bool`,三处调用。
3. **一次有界 dispatch**:explorer 再派发一次,`MaxIterOverride` 小值(4);**emit-only surface 复用**——新 typed `AgentContext.CompletionOnlySurface bool`,explorer.FilterToolSchemas 读它返回既有 `completionProgressToolNames`(emit_evidence + emit_investigation_complete;validateExplorerToolBoundary 双重 enforcement 自动生效);窗口 hint 用既有 window-hint 通道注入一段聚焦指令(命名 pending 义务,CN/EN 风格随既有 hint;含义务对应的逃生道句:absence_justification)。
4. **计费**:同 stepsUsed 指针穿越 ceiling(F1 同构);breadcrumb:logging.Info reason=completion_obligation_lane + metrics 计数(runner 侧 `completion_lane_fired`)。
5. **防重入结构性**:dispatch 后无论 IsInvestigationComplete 与否均继续原路径(lane 单发);laneFired 是 run-局部变量(scheduler 栈上),不入持久状态。
6. **不破坏稳定场景**:predicate=false 或义务已满足时字节不变;lane 只在"本来就要带着空手去 extract"的路径上加一跳。

任务:
- [x] 1.1 `pendingCompletionObligations` + 单测(义务命中/满足/逃生道三态)
- [x] 1.2 CompletionOnlySurface(BusContext+AgentContext+builder copy;ShallowClone 反射自动覆盖)+ FilterToolSchemas 分支 + 收窄/fail-open/flag-off 三向测试
- [x] 1.3 lane + 三汇聚点(forced :5202 前 / pre-extract :5229 前 / contractFailureBreak 头)+ 单发防重入三守卫测试 + 同 stepsUsed 计费
- [x] 1.4 hint(命名义务+双 typed 出口含 support_refs 契约)+ runner completion_lane_fired 计数
- [x] 1.5 守卫矩阵单测;67 包绿 + vet 干净
- [x] 1.6 实测达成:sequence_table ×4 = **4/4 PASS(历史首次)**;predicate 3/4;run2/3 完成门工具层各弹 1 次→当轮修正(首防闭环,lane 兜底 0 触发 = 分层正确)

## §2 批 2:TypedDenials 站点补全(GAP-2)

每站点 stamp 前置守卫(recon 风险清单逐条吸收):

| 站点 | 类(enum 已存在) | stamp 点 | 守卫 |
|---|---|---|---|
| (a) oracle 符号确证失败 | oracle_symbol_unverified | **三个 confirmed-hallucination append 点**(ViolEnumerationLabelHallucinated / ViolDiagramEdgeEndpointHallucinated / ViolInlineIdentifierHallucinated 的 violation 构造处),共享 helper | 已天然 post-escape-lane(七条桥全失败才到这);identifier-shaped + ≥10 字符 floor 保持;绝不在 oracle 实现层 stamp |
| (b) drift 帧迁移 | drift_frame_relocated | authority 路径 drift 判定处 | **只 stamp DriftStatusUnmappable**(FileMoved 的文件可能真实存在且相关——硬拒会伤合法 read);渲染器不 stamp(prompt 构建不得有副作用);BackfillEvidenceProjector 无 TypedDenials 的 gap 显式接受并记录 |
| (c) 证据 grounding 失败 | evidence_subject_unverified | emit_evidence.Execute(**ground 包不碰**——L3 红线) | 双条件:7 轮稳定化后最终 verdict 仍 Ungrounded **且** token 在 SymbolExists+SymbolExistsFlat 双 miss(防堵同轮修复回路);stamp 于 reports[i]=r 之后 |
| (d) MCP 形状失配 | (需先 groundwork) | **本批 descope** | 失配今天是静默 bool false,需先改 typed 三态 + mcp_read_resource 无 L1 门 + token 选型(URI)未定——半成品不发,落 groundwork 任务 |
| (e) perf-stall 跨类错位修复 | external_perf_stall_unresolved | classIsSymbolShaped 对齐 IsSymbolDenied 文档(:268-273 已承诺覆盖) | 行为变化 = symbol 门开始对该类生效——与文档一致即正确方向;负向 pin 防过杀 |

任务:
- [x] 2.1 stampOracleSymbolDenial + 三确证点(denials 参数穿链,69 测试点机械补 nil)+ 正负 pin
- [x] 2.2 (b) authority Unmappable-only stamp(FileMoved 显式不 stamp 注释)
- [x] 2.3 (c) finalize-entry stampUngroundedEvidenceDenials(最终 Ungrounded + 双 oracle miss + 形状/floor;vouching oracle 抑制 = 修复回路保护测试钉)
- [x] 2.4 (e) perf-stall 类双轴化(symbol 半边接通 L1 symbol 门;path 半边 exact-match 不受影响,双向 pin)
- [x] 2.5 (d) MCP descope 确认落账:需 parseTypedObservationEnvelope typed 三态(当前 not-envelope 与 activated-but-malformed 同返 false)+ executeMCPReadResource 无 L1 门 + class 选型(AttachedExtractedUnscoped 语义不合,新类需 7 处 in-repo sync 含 denied_token_answer_check.go:52 重复表)——独立 groundwork 批
- [x] 2.6 67 包绿;既有 TestTypedDenials_ToolStampsReachOrchestratorBus 端到端在卷,新站点同指针即时生效

## §3 批 3:validateScalarValueGrounding(GAP-3 单点)

s11b oracle 模式逐项复制(typed-vs-typed / Soft+Promotable / 空池 no-op / LocusFinalizer / Layer v2_oracle):
- **触发(全 typed)**:BlockScalar && SurfaceRole==principal && block 级 claim_uses 声明 form ∈ {literal_value_fact?(以实际 enum 为准:assignment_fact/definition_fact/return_fact)} && block.Text 可保守归约为单一 verbatim literal(整数/引号串/单 identifier;多 token 保守跳过)。**form 未声明 = 跳过**(§1.6:声明缺席不得入门)。
- **支持池(全 typed)**:item.citation_ref 经 answerEvidenceIndex.citedEvidenceItems 桥到 grounded EvidenceItem 的 Snippet/SurfaceTerms/Subject/Object/AnchorSymbol;StableInvestigationAggregateFacts 的 Value/Members(复用 aggregateFactExactCount);claim_form==external_observation → 改走 attached-artifact lane 不查 repo 池。
- **比较**:NormalizeCodeKey + 数字精确比对;**空池 no-op**。
- 新 `ViolScalarValueUngrounded`:registry(Medium/Soft/Promotable/LocusFinalizer/v2_oracle)+ legacy 三表 + cgec covered 表 + hint composer case(系统側 kind,R2' 2-5 n/a)。
- **不双发**:enumeration 答案的 member 值已由 member-set review 覆盖——scalar oracle 只看 BlockScalar。
- 顺带:`answer_document.go:263` 失实注释改写(GroundCitation 在 V2 路径无调用方,防线描述删除/改为历史注记)。

任务:
- [x] 3.1 validator + 六向测试(命中/聚合支持/未声明 form/external_observation/空池/多 token 跳过 + 数字边界负向 pin + 标识符 canonicalizer)
- [x] 3.2 ViolScalarValueUngrounded 七注册点 + composer case(contract test 强制)
- [x] 3.3 GroundCitation V2 孤儿事实写入 DefaultCitationMaxQuoteChars 注释(历史注记)
- [x] 3.4 67 包绿;scalar 族实测并入批 4 数据批

## §4 批 4:小步项(GAP-4/5/6)+ 验证收口

- [x] 4.1 spec 复审定案(`258c9358`):中间阶段必须出现、词形不钉(equiv 类):问题主语是 stage;**结论候选**=EXPECT_CONTAINS 的 agent 名词改为(agent 名|Stage 名)等价类,但仅当人工读问题文本确认 agent 名非核心实质——否则保持。单独小 commit 说理。
- [x] 4.2 12 案数据批 **12/12 PASS**:seq×4 全过 + artifact×3(B3 维持归零)+ 新 fixture 抽 4 零回归 + qf_config(scalar oracle 零误伤)
- [x] 4.3 完成。

## §5 交付纪律

每批:实现+gofmt+vet+全 67 包绿 → commit → push → 本账本勾选。对抗复审环节保留(并行线"修复需要再审"硬规律)。
