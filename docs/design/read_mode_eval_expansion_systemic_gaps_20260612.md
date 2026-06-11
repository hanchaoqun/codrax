# 读模式 eval 扫批取证 + 系统性 gap 修复 + 多语言 fixture 扩充(2026-06-12)

Baseline:`ceb3a57c`。方法:trace_query 余 6 案 + read_combo 21 案真实运行(2 并行 × 6/批)→ 27 案逐日志机械挖掘(工具分布 / elapsed / retry / remap / WARN 类)→ 真 FAIL 亲手根因到行级 → 系统级泛化方案。红线全程:精确信号才进硬门 / 禁关键字匹配散文 / 禁 case 拟合补丁 / 不破坏稳定场景 / prompt CN+EN 且零内部术语泄漏 / R2'。

## §1 eval 总账

- trace_query 6 案:5 首跑 PASS;`inode_event_search` 答案实质正确("无法直接…对应…路径"),spec 否定词表第 7 例,拓宽后对既有输出验证 PASS(`366b82a8`)。
- read_combo 21 案:17 首跑 PASS;3 FAIL 为 spec 侧(词表 ×2 + 目录白名单 stale ×1,均对既有输出验证后修 spec:`bbbdcb10` `ceb3a57c`);**1 真 FAIL = `pipeline_sequence_table` "missing:Mutable"**,根因见 §2。
- 健康面:全部 27 案 `strict_decode_remap_events=0`,LLM transport 零重试,ctx 峰值 16-41%,tool_history_prunes 仅在 41% 两案触发且工作正常。

## §2 真 FAIL 根因链(read_combo_pipeline_sequence_table)

用户要求 Mermaid sequenceDiagram **加一张表**(并点名状态载体 例如 AnalysisIR、EvidenceItems、AnswerDocument、Mutable/BusContext)。日志取证(run `20260612-011524`):

1. **模型发出了完整正确的表格**:blocks[] 第二对象 `id=t1, kind="table", title="Read Mode 各阶段输入、输出与状态载体", items=[r1..r5]`,5 行 cells 全部含 MutableState/BusContext——但同一对象**融合**携带 `diagram:{...}` payload + `edge_anchors[]`。
2. **F-A 静默重分类丢表**:`NormalizeEmitAnswerBlock`(internal/tool/answer_block_normalize.go:133-142)discriminator repair:`raw.Diagram != nil → blk.Kind = BlockDiagram`,声明的 kind="table" 被覆盖且不可恢复;items 虽拷入 typed block(:116-132),但渲染按 Kind 单分发(render/answerdoc.go:150-171),`renderV2BlockDiagram`(:846)从不读 Items/Columns → **表格内容静默蒸发**。acceptance 日志 `(kinds=summary,diagram)` 读的是覆盖后 Kind(emit_answer_document_v2.go:3572)。`validateMergedV2Doc`(:227)的硬拒("diagram payload 只合法于 kind=diagram")存在但**不可达**——两层 repair 先行。schema 侧也不设防:`projectKindPayloadConditionals` 只加 required 不禁字段;table 故意无 payload-field 约束;`projectDiagramField` 只在无 diagram 契约时删字段——本案有 diagram 契约,diagram 在任意 kind 上均合法。
3. **F-B advisory 三层失效**:pre-emit `preCheckRequiredMechanismAnchors`(answer_document_pre_emit_check.go:7195)确实检出 anchors 缺失,但 (a) `ViolPrincipalSupportMemberOmitted` SoftByDefault → 永远 advisory;(b) 检查面只看 block.Title / item.Label / edge endpoints(answer_required_anchor.go:159-191),**typed 的 item.Cells 被排除**——原始 emit 里 anchors 全在 cells;(c) anchor 自动注入 `normalizeRequiredMechanismAnchorCarriers` 只在 `RequiresAnchorSkeleton → Family==QFGeneric` 物化,而 anchor lane 启用于 QFArchitecture/QFCallChain/QFGeneric——**拮抗门对**:arch/callchain 永远只有软提示;(d) persist 后无任何 answer 侧复查(MissingRequiredMechanismAnchors 零 post-emit 调用方)。
4. **"Mutable" 半边丢失定位**:anchor 池 = analyzer LLM 发出的 Entities ∩ RawRequest 子串(MentionedEntities,analyzer.go:1753)——analyzer 只发了可解析的 `BusContext`,从未发 `Mutable`;编译器从不切 `/`(requiredAnchorQualifiedParts:255 显式 bail)。丢失在 analyzer 实体发射层,非编译层。`maxRequiredMechanismAnchors=6` 截断无日志(次级)。

## §3 系统性 gap 清单(27 案挖掘 + 根因取证)

| # | gap | 证据 | 级别 |
|---|-----|------|------|
| G1 | 融合 block 静默重分类丢用户必需 surface(F-A) | §2;唯一真 FAIL | P0 |
| G2 | repo_map 采用率:6 案 zero repo_map 全靠 grep/read_file(config_absent 25 grep、two_knobs 16+14、bucketed 11+13…);反例 criterion_rich_functions repo_map-first 9 calls/102s vs 同规模 grep-first 28 calls/159s;3 真伪 FAIL 中 2 个在 zero-repo_map 集 | P1 |
| G3 | extractor 首发 emit_hypothesis_verdict artifact 引用被拒(`runtime_artifact` / `log:2` / 裸 ev-id),6/26 案各烧 1 轮;契约在拒绝后才到模型 | P1 |
| G4 | emit_investigation_complete 弹回 2-5 次 × 7 案,无 WARN 痕迹、无 typed 计数,每弹一次 = 1 explorer 轮 | P1 |
| G5 | F-B anchor lane 三层失效(soft-only + label-only 面 + QFGeneric-only 注入 + 无 answer 侧复查 + cap 静默截断) | §2 | P1 |
| G6 | emit_answer_document_patch 静默 detach 3 个无可替换 citation_ref(trace_current_code_dimensions)——引用静默丢失非修复 | P2 |
| G7 | `facet_softened observed_artifact_fact` 硬→软降级 6 案 ×5 在 log/trace 案;bucketed FAIL 中被软化的 facet 恰是 verdict 所测的 log↔code 连接面——降级是答案面丢失的前导指标但目前静默 | P2 |
| G8 | parallel explore:windows=5-7 @ parallelism=2,sibling-cancel WARN 三连 ×6 案;binder_aux_wait 取消窗 iter=0 零工作(空转 dispatch) | P2 |
| G9 | explorer 在 emit-only 窗口批发 grep+read_file 被拒 "not in current tool schema" ×2——限制已 enforced 但 prompt 未传达 | P2 |
| G10 | 观测盲区:wall time 不入 summary.md(全靠日志时间戳推算);operation-route 运行 metrics 全零;G4 弹回无计数 | P2 |
| G11 | 性能结论:**纯 LLM-latency-bound**(>2s 的 elapsed 100% 是 llm_request 8-44s;repomap build 峰值 439ms / rank 516ms / prompt 装配 0-1ms / tracequery 无热点)。性能优化的真实杠杆 = 减少 LLM 轮次:G2(工具效率)+G3(省 1 轮/23% artifact 案)+G4(省 1-4 轮/案)+G8(空转窗) | 结论 |
| G12 | analyzer `source_quotes` 单案 9 条 fabricated 被 typed ignore 层正确丢弃;quote-copy 指令弱(soft,防御层健康) | P3 |

## §4 修复方案(系统级泛化,逐项红线对照)

### B1(P0)融合 block 确定性拆分器 —— 修 G1
**原则**:不重写模型内容(answer_document_table_compile.go:17 no-op 先例:系统不得改写模型表格 surface),**拆分保全两个 surface**。
- 新共享 helper `splitFusedDiagramBlocks(raw []emitAnswerBlockV2) []emitAnswerBlockV2`,**raw 侧**、在 `NormalizeEmitAnswerBlock` 循环之前(声明 kind 在 :141 覆盖后不可恢复)。
- **精确触发**(§1.5 合规,单布尔组合):`raw.Diagram != nil && raw.Kind != "diagram" && (len(raw.Items)>0 || len(raw.Columns)>0)`,且两半独立有效(diagram.body 非空——否则保持原样让既有 :149 硬拒走 retryable 路径)。
- 拆分:原 kind 半保留 id/title/text/columns/items/claim_uses/facet_ids/surface_role;diagram 半携带 diagram + edge_anchors,派生唯一 id(原 id + 后缀),**紧邻插入原块之后**(canonicalizeSummaryLeadBlock 只动 summary,叙事序保持)。
- **三调用方同步**(G2 normalize 红线):full-emit(emit_answer_document_v2.go:182 前)/ patch(emit_answer_document_patch.go:596 convertEmitBlocksToTyped)/ text-recovery(answer_document_text_recovery.go:323)。
- 守卫:maxBlocksPerDoc=64;WARN 一行(系统侧 telemetry,非 LLM-facing);ToolResult Summary 的 kinds= 自然反映拆分后真实 kind。
- 测试:fused 全字段分区 pin(镜像 TestNormalizeEmitAnswerBlock_AllFieldsPropagate)+ 触发条件负例(空 diagram.body 不拆 / kind=diagram 不拆 / 无 items 不拆)+ patch 路径 merge 语义(synthetic id 不混淆 remove/unchanged_block_ids)。
- persist 层 `normalizeMergedDiagramPayloadKinds`(mutation_runtime.go:237)保留为纵深(系统侧直接构造 fused typed block 的路径,如 text-recovery promote)。

### B2(P1)anchor lane 三层收口 —— 修 G5(配合 B1 修 §2 全链)
1. **检查面扩到 typed Cells**:`MissingRequiredMechanismAnchors` 的 recordAnchorSurface 加 `item.Cells`(typed []string,精确)。block.Text/prose 仍排除(散文面,守住"不扫 raw 文本"红线)。
2. **answer 侧复查**:post-emit 在 contract_check 的确定性 V2 oracle 链加 `validateRequiredMechanismAnchorsRendered`:对最终 typed doc 重跑 MissingRequiredMechanismAnchors(含 Cells 面),缺失 → 产 `ViolPrincipalSupportMemberOmitted`(已注册 kind,SoftByDefault+Promotable 不变;闭掉"persist 后无人复查"洞,软→caveat+telemetry,org 可 promote)。must_include oracle(checker.go:272 查 FinalAnswer 全文)是先例;本检查停留 typed 面更精确。
3. **analyzer skill 软引导一句**(CN+EN):用户在问题里枚举示例标识符(含 `A/B` 斜杠并列形态)时,每个标识符独立进 entities,不合并不省略。修 "Mutable" 半边丢失的根因层。
4. **cap 截断可见**:>6 截断时 DEBUG 一行(系统日志,非 LLM-facing)。

### B3(P1)extractor artifact 引用契约前置 + 完成门弹回可观测 —— 修 G3 G4
- G3:extractor skill 的 hypothesis-verdict 段加**条件 section**(gate 在 typed `BusContext.AttachedLog/AttachedHitrace`,既有 pre-stage 条件渲染先例):artifact 行锚点的合法 citation 形态(现行拒绝文案已 typed 化,把同一契约**首发前**给到模型,CN+EN)。预期消除 ~23% artifact 案的 1 轮浪费。
- G4:emit_investigation_complete 拒绝路径加 typed 计数(metrics `investigation_complete_bounces`)+ 每次弹回 DEBUG 一行带 typed reason(floor-gate / coverage / 其他既有 typed 拒因);模型面 retry hint 已存在不动。先观测后优化,不预设弹回都是坏事(floor 重定向是设计内)。

### B4(P1)repo_map 采用率软引导 —— 修 G2
- **不做硬门**(工具选择是模型自由,强拦截踩"系统不可代替 LLM"红线)。两层软引导:
  1. explorer skill:既有 repo_map 视图清单处加一句**何时优先**(结构/枚举/配置类问题先 repo_map 后定向 read_file;implementers/source_inventory 视图点名,CN+EN)。批 2 implementers 视图先例。
  2. analyzer→explorer typed 通道:EvidencePlan 已有 ToolSuggestions 机制——按 question family 确定性补 repo_map 建议(QFEnumeration/QFArchitecture/QFConfigPrecedence → 对应 view)。typed 信号驱动软建议,零散文匹配。
- 验收:重跑 zero-repo_map 6 案,观测 repo_map 调用 >0 且 LLM 轮次下降(非硬性,记录数据)。

### B5(P2)修复层可观测性三件 —— 修 G6 G7 G9
- G6:patch detach 无可替换 citation_ref 时,除现有 WARN 外物化一条系统 caveat(appendSystemCaveatString 既有汇点,双语)+ typed 计数。引用静默丢失变可见降级。
- G7:`facet_softened` 时在 finalizer 下一轮(若有)retry-hint 附软提示(已 softened 的 facet 名 + "若证据可承载请补该面"),并加 typed 计数;不改判定本身(降级判定是噪声信号,只做软引导)。
- G9:emit-only 窗口的 prompt 已声明工具集——把拒绝文案与窗口约束对齐一句(模型面已有拒绝信息,补窗口开头声明,CN+EN)。

### B6-B8(P1)多语言/跨语言/多仓 fixture 扩充(GitHub 蓝本,coverage_gaps 实证后选型)
现有 22 fixture 实测空白(4-agent 盘点交叉验证):TS monorepo+路径别名、C++ 虚分发层次/模板、Python MRO/装饰器注册表、Java 注解 DI、IDL 契约漂移、C 宏/平台分叉、非 Go async。新 6 fixture × 2 case = 12 案,全部走既有 FIXTURE=/MULTIREPO= 通道(零 runner 改动):
- **B6**:`ts-monorepo-ws`(turborepo/pnpm-workspace 蓝本:packages/{core,client,cli} + tsconfig paths 别名 + barrel 再导出 + interface 双实现 + async 链)→ cases: 跨包调用链 / implementers;`cpp-sink-hierarchy`(spdlog/fmt 蓝本:抽象 Sink + 3 实现 + 工厂注册 + CRTP + 模板特化)→ cases: 实现者枚举(repo_map implementers 视图实战)/ 虚调用链。
- **B7**:`python-plugin-mro`(pluggy/flask 蓝本:ABC + @register 装饰器注册表 + mixin 菱形 MRO + asyncio 管道)→ cases: 动态分发链 / MRO 顺序;`java-annotation-router`(spring 形态自研注解:@Route/@Inject 反射路由 + Handler 三实现)→ cases: 注解路由解析 / 实现者枚举。
- **B8**:`proto-contract-pair`(grpc 蓝本,MULTIREPO 3 子仓:proto-defs + java-server(新 stub)+ py-client(旧 stub 漂移:RPC 改名 + 字段缺失))→ cases: 跨仓契约漂移 / 哪侧 stale;`c-macro-platform`(redis/libgit2 形态:X-macro 命令表 + #ifdef POSIX/WIN32 分叉)→ cases: X-macro 展开成员 / 平台分叉边界。
- spec 纪律(本批 8 例词表教训):EXPECT 用 token CONTAINS + 结构正则,不钉动词词表、不钉目录布局、不钉行号;fixture 双向校验(答案可从 fixture 唯一推出)。
- 运行观察面:repomap 对新语言形态的 parse tier 回退 WARN(`repomap: <file> X→Y (tier N→M)`)与构图耗时 = 性能/韧性的真实压力测试。

### B9(P2)观测补全 —— 修 G10
- run.sh summary.md 加 wall 列(日志时间戳差,runner 侧);operation-route 运行在 metrics.txt 标注 route(避免全零误读);G4 计数入 metrics 提取。

## §5 性能与内存结论(G11)
今日 27 案实测:repomap/tracequery 经前几日修复(BuildGraph 19.6s→78ms、Event 窗口 165MB→224B 等)后已无 >600ms 热点;prompt 装配 0-1ms;内存无新增长类。**管线为纯 LLM 延迟约束**(单请求 8-44s,墙钟≈轮次×~10s)。因此本方案的性能交付物 = B3/B4/B8 的轮次缩减与空转消除,不再做 CPU/内存微优化(避免无基准的伪优化;基准先行纪律保持)。

## §6 交付批次与状态
- [x] B0 spec 修复 4 例(已 push:`366b82a8` `bbbdcb10` `ceb3a57c`)
- [ ] B1 融合 block 拆分器(P0)
- [ ] B2 anchor lane 三层收口(P1)
- [ ] B3 extractor 契约前置 + 完成门观测(P1)
- [ ] B4 repo_map 软引导双层(P1)
- [ ] B5 修复层可观测三件(P2)
- [ ] B6 fixtures: ts-monorepo-ws + cpp-sink-hierarchy + 4 cases + 实测
- [ ] B7 fixtures: python-plugin-mro + java-annotation-router + 4 cases + 实测
- [ ] B8 fixtures: proto-contract-pair + c-macro-platform + 4 cases + 实测
- [ ] B9 观测补全(P2)
- [ ] B10 验收:pipeline_sequence_table + zero-repo_map 案重跑;新 12 案首跑;回归抽样(qf_arch / s1a / logtri / trace 各 1)

每批独立 commit+push;ledger(本文件 §6)逐批勾选。
