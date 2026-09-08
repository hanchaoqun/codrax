# r1038 Trace IO 链与仓颉声明清单人工审计

- date: 2026-09-08T07:14:39Z
- sweep_start_ts: 20260908-001438
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

基线：main `6d77732c4cb5`，已推送；clean make，版本 0.1.20260908。两题共用冻结二进制快照 `.codrax/tmp/codrax-selected-20260908-001438`，恰两路并发，没有第三路、改 oracle 或单题追绿。本次机器均 PASS，但人工均为部分正确，不能收完整正确。下面的 partial 表示有已核实正确内容，也有明确错误，不是尚未读答案。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | cangjie_repomap_fixture | PASS | eval/results/cangjie_repomap_fixture-20260908-001439 | dimension_substring,answer_contains | none | 87s | 28 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=2,inv=3/0,fin_reject=1,unavail=0,prune=0 | partial | 五项源码/包名/引用完整；extend 被误解释成继承；模型备注混入已验证清单上下文 |
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260908-001439 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 203s | 43 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | 链上 IO 11ms/四节点唤醒/投影在；用户 20ms 窗被 21ms 查询替代，另有模型优先级与时序误述 |

## 1. Trace：正确链路与窗口缺口分别收账

工件：`.codrax/output/20260908-001759.856-45585.md`、同名 HTML 与 `.root-causes.json`。本节日志行号均指 `eval/results/trace_query_wakeup_causal_io_chain-20260908-001439/run-1.logs.all.log`；源真值是原 case 内 HTRACE，未改写历史答案。

### 源码/原始 trace 核对

- 用户只请求 app-100 的 `2.000..2.020`，即 20ms。
- app 在 2.000 切 S，cookie 在 2.001 切 S，network 在 2.002 切 S，threadpool 在 2.003 切 D；2.004 的 blocked_reason 明确 `pid=400 iowait=1 caller=fscache_page_wait_on_page_bit`。2.014 唤醒 threadpool，所以该等待 11ms。
- 唤醒顺序 threadpool-400→network-300→cookie-200→app-100，时刻为 2.016/2.018/2.020；目标 CPU 依次 3/2/1，发起 CPU 4/3/2。三条跨 CPU 唤醒关系完整，不能从中单独推断竞争或迁核开销。
- app 的 0.020ms runnable 是 `2.020000..2.020020`，正时长全部在用户窗末端之外。它可以保留为窗后调度证据，不能充当用户窗内等待。cookie/network 的 17/14ms 是嵌套路径等待，不能相加成总体阻塞。
- 本轮是 Harmony 优先级语义；app 的 prio=52 为 RT，其余三个线程 prio=20 为 CFS。不得错误套用 Linux 数字方向否定现有低优先级依赖候选，也不得把 app 的 52 移给 threadpool。

### B1619 / TRACE-REQUESTED-SCOPE-PARITY1（P1，确认，待施工）

1. 接受的分析结果（793）正确保存显式 `2..2.020`，但同时把 100/400/200 三个 PID 都标成 `user_explicit`。后两者其实是调查对象；这是模型分类错误，不是工具后来追加游标。
2. 五次模型 trace_query（1195–1197、1284–1285）均主动传 `time_end=2.021`；叙述仍称 2..2.020，未解释扩窗用途。不是引擎自动扩窗。
3. 补采（1447–1448）因 `no_typed_target` 没有 engine 调用。`traceSupplementDeriveTarget` 对多用户 PID 不擅自选首项/游标是正确保护；不能据此放宽主目标资格。
4. `projectionAnchorWindow` 在缺精确用户窗载体时保留合法探索投影，这是已存在且需保护的正向行为。但最终提示 2347 说只有 20ms 用户窗能作总量依据，2450–2452 又把 21ms 投影标为 `requested_or_elected_window`。状态、决策与投影入口未统一范围角色，绕过普通 observation 提示已有的用户窗过滤。
5. 最终模型导语、系统状态表、因果投影和根因 JSON 全沿用 21ms，未明确区分为补充查询账户。模型越窗在前，系统范围权威混杂在后，不能归为纯模型波动。

最小方案：复用 RuntimeArtifactScopeProfile、原始查询窗及 TraceCausalProjectionPrincipalValueSameWindow 单点分类，区分用户范围/实测范围/精确一致性，贯通 principal-value/decision/state/prompt/发布及旁路语义。缺精确账户时保留探索图、原值和链，明确“补充查询范围；用户指定范围尚无独立同窗账户”。禁止给 21ms 账户改标签成 20ms、把窗外 .020 裁零或删除投影。成员级用户目标来源绑定另列第二阶段，不扫描原始输入词、按 PID 顺序或 confidence 替模型挑目标。

需保留 `TestTraceProjectionExplicitUserWindowRequiresExactTraceCoverage` 的探索图正针，并补：20ms 请求/仅21ms查询；补得20ms后主账户恢复且21ms可追溯；窗后值不得冒充用户窗总量；无显式窗旧选举；多目标仍不自动挑选；不同捕获与不同窗不混域。

### 其余人工结论、JSON 与重试

- 主要链上 IO 11ms、三项各1ms低优先级依赖候选、四节点唤醒、实际占时和规则可消两轴、背景 IO 活动指数不入根因排序均保留。没有 Mermaid 是本题可接受形，Trace 因果投影确实存在；不能记成“没图”。
- 模型首稿已错称“阻塞完全由上游传导”，又说未证明直接阻塞者；误将 2.016/2.018 唤醒点称睡眠区间起点；把 RT/prio52 赋给 threadpool；把广义依赖方供给候选过窄地解释为必须补锁/PI-mutex。正确区间/优先级提示已在输入，不能再靠具体词硬门代写答案。“唯一突破性席位”等内部术语泄漏记软表达观察。
- 系统占时表 cookie/network 两行分别重复；记录为 P2 待核物理来源的展示观察，不按文字相等盲去重，不能从重复表行推断实际发生两次。
- analyzer 三次提交、两次拒绝分别是场景/事实族不同字段不一致；未确认同一声明必带又必拒。上下文峰值43%，无不可用工具和剪枝，不以预算不足归因。
- 初次 full emit 在2574–2575已接受。2578是 post-emit 展示归属提示，不是首稿拒绝；随后2617–2618的 metadata patch **实际被拒绝**，因为 runtime_work_relation 把受 schema 限制的 conclusion 填成中文长句。动态 schema 本有逐行 `oneOf/const`；不能把此轮说成“零拒绝”或“patch 成功”。未知 schema_version 被隔离但不构成该次失败。事务未 staged，已接受首稿保留；没有删除模型答案或新增无证根因。
- JSON 旁路6487字节、schema2、status=available、5项为模型明确选择；其中 .020ms 也继承21ms账户的范围问题。文件正常生成不等于内容范围已正确。

## 2. 仓颉：完整声明清单不等于行为解释已证

工件：`.codrax/output/20260908-001604.409-45596.md`。本节日志行号指 `eval/results/cangjie_repomap_fixture-20260908-001439/run-1.logs.all.log`。实际评测目录三个源文件与 fixture 字节一致。

五项真值完整保留：

| 类别 | 精确声明 | 定位 | package |
|---|---|---|---|
| extend | Cart | cart/Cart.cj:30 | demo.cart |
| foreign func | native_add | bridge/Bridge.cj:6 | demo.bridge |
| public class | Bridge | bridge/Bridge.cj:15 | demo.bridge |
| public class | Cart | cart/Cart.cj:14 | demo.cart |
| public class | App | main.cj:11 | demo.app |

未将 Item struct 当 class、ohSum wrapper 当 foreign，也未合并 extend Cart 与 class Cart；五引用均对应。题目不要求图，无图不是缺失。B1586 的 emit-only 两次交接（1123–1127、1450–1454）和 finalizer（2039–2043、2276–2294）保留三类别及各自计数，无旧 type=4→public class=4 串类。

**B1620 / AGGREGATE-FIELD-AUTHORITY1（P1 候选范围确认、实现待设计）**：答案 MD:9/13 将 extend Cart 误说为继承。该块仅增加 isEmpty，无父类声明。错误始于模型 member_notes（883、1539），首稿2417和patch2488沿用，不是 renderer 改写。系统却把含 note 的 aggregate 整行标为 `claim_authority=independently_proven`（2157）；2273虽说声明清单不证明继承，2281紧接着无局部权限标识输出模型的错误 note。源码 `answer_aggregate_fact.go:2930` 可按精确坐标提升 aggregate，而 evaluator:11434直接输出附注。已确认字段权限混杂，不把模型错句全部归因系统。

方案应隔离“已证声明成员/坐标/计数”和“模型候选说明”的支持上限，producer/consumer共用来源，不撤销正确声明的引用资格、不隐藏业务说明、不扫描继承关键词、不替模型改正文。需要异构声明/行为说明正负例，不做仓颉单语言特判。

过程87s；2次repo_map、2次source_lens，3次completion无拒绝。一次成文拒绝（2428–2437）是漏填已明确教学的精确label/cells；patch补五label，unchanged=1/replace=3，正文/引用不丢。字符串包裹replace_blocks成功恢复（2490）；漏claim_uses仅软提示（2495），无再次重试。峰值28%，不加预算或新成文硬门求绿。无活跃流被提前截断见证。

## 3. 优先级与交付边界

1. B1619：首先打通明确用户窗与探索窗的范围角色；不妨碍边界追链、自动补采或正常因果投影。用户目标成员来源绑定另案，不能以放宽歧义保护代替。
2. B1618-P2a：完整/预览target wait清单、state匹配、reader-shadow按同capture/target/query-window来源隔离；保护已有精确墙钟数值。P1本轮已推不等于P2关闭。
3. B1620：声明字段与模型备注权限隔离；同源schema的有界错误提示可另做减负，不复制第二份JSON教学合同。
4. B1616b：三载体同计划履约、profile一致性；跨计划需完整定义和真实执行/交付树绑定；B1561原生逐断言与十个异构评测域继续保留。

本轮不修改case、oracle、历史答案、模型结论或排序；原机器表保留。B1618-P1最终全仓86包通过、clean构建并推送，与本轮两题人工部分正确是不同的验收结论。活跃SSE正文/推理/tool call的旧时间阈值回归已在B1618-P1独立通过；本轮没有4分钟长流，不冒称新生产长流验收，更不引入4ms无答案降级。
