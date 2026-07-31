# Eval 多维优先级与泛化审计战役（2026-07-30）

## 1. 基线与目标

- 代码基线：`main@fcbdccdee`，已执行 `git fetch origin && git rebase origin/main`，与 `origin/main` 一致。
- case 总量：243；默认 read 215，apply 25，plan 3；其中 trace 62、log 24、data 6、带 fixture 67。
- runner：`eval/parallel_selected.sh`；每批固定 `PARALLEL=2`、每 case 运行 1 次。运行前必须重建 `./codrax`，runner 只快照现有二进制，不替操作者 build。
- 本战役不把 runner PASS 当作人工正确。每批必须同时审：
  1. `run-1.out` 的最终答案和推理过程；
  2. `run-1.logs.all.log` / `run-1.logs/` 的控制面过程；
  3. case 的精确 oracle、禁止项和真实问题；
  4. 工具调用、重试、上下文、降级、引用与运行时 authority。

## 2. 排序维度

总分只用于排执行先后，不作为质量门。五项均按 0–5 评估：

| 维度 | 权重 | 含义 |
|---|---:|---|
| 客户/安全影响 | 4 | 错答、错误修改、证据丢失的外部损害 |
| 机制覆盖面 | 3 | 一个失败是否代表同类请求的共同机制 |
| 现有回归证据 | 3 | 最近 FAIL、人工审计 GAP、未完成验证 |
| oracle 判决力 | 2 | 是否有可复算事实、负向约束、行为校验 |
| 过程诊断价值 | 2 | 能否暴露路由、工具、终局门和性能问题 |

另记运行成本 0–5，但只用于同优先级内调度，不因昂贵而降低正确性优先级。批内尽量跨模式配对，既保持并行 2，又减少同一资源面相互放大。

## 3. 当前优先级

| 顺位 | case | 主要维度 | 选择理由 | 批次 |
|---:|---|---|---|---|
| 1 | `real_trace_c2_dstate_iowait` | trace 精确事实/因果 | 最近自动 FAIL；计数 3、Σ0.635ms、caller、时间与“非主因”必须同时成立，可杀数值与因果漂移 | B1 |
| 2 | `github_issue_zod_prefault_symptom` | write 症状定位/行为闭环 | 非直给修点；检验探索、计划、修改、false/0/空串回归与防改测试绕过 | B1 |
| 3 | `real_trace_h4_supply_thermal_witness` | trace 供给/四态/性能 | 真 trace、高复杂度；上轮 338s/19 次 trace_query，兼具事实、权限和重复查询风险 | B2 |
| 4 | `data_multifile_reference_projection` | data 谱系/对账 | 曾暴露幽灵组键；可验证全局组键语义、贡献账和 reconcile 是否真正闭合 | B2 |
| 5 | `read_combo_log_current_code_boundary` | runtime+源码双权威 | 历史 PASS→FAIL→PASS；显式要求当前源码，适合检验零源码见证的 bounded 修复门 | B3 |
| 6 | `logtri_oversized` | log 长尾/分段 | 约 40KB 两段式路径；检验分段成本、尾段退化与证据合并，不只看是否提到 panic | B3 |
| 7 | `trace_query_state_churn_root_cause_rank` | trace 排序/缓存 | 曾从 7 次查询、309s 降到 1 次、131s；验证 memo 的泛化与根因排序稳定性 | B4 |
| 8 | `github_issue_gson_lazy_number_symptom` | write Java 泛化 | 与直给版本分离，检验跨语言的症状定位和行为 oracle，而非 TypeScript 单例拟合 | B4 |
| 9 | `real_trace_e2_cross_trace_asymmetry` | 多 trace 边界 | 最近自动 FAIL；需要区分“未采样”与设备事实，并守住不可直接对齐的时基边界 | B5 |
| 10 | `harmony/cangjie_repomap` | 源码清单/稀有语言 | 历史上有 source-class、分页、principal row/citation 覆盖债，能检验跨语言泛化 | B5 |
| 11 | `qf_config_precedence` | 配置权威/源码引用 | 复杂优先级问答，适合验证单次分析、精确引用和 degradation 披露 | B6 |
| 12 | `patch_java_typo` | plan 隔离/最小变更 | 低复杂度判别件，用于区分平台机制故障与模型深推理失败 | B6 |

## 4. 反过拟合施工纪律

发现 GAP 后，先归入机制类，再决定是否改代码：

- 禁止以 case ID、fixture 路径、题面关键词、某个数值或某种语言为生产触发条件。
- 硬行为只能消费 schema 校验后的 typed/精确信号；词面和相似度只能用于软引导。
- 修复必须至少有：原 witness、同类变体、相邻负例、接线/枚举 tripwire。
- 优先修同源谓词、权威载体、typed 逃逸、统一投影或缓存键；不为单个输出句追加 prompt 补丁。
- 若问题只是 case oracle 词面窄，修 eval，不降低产品答案 bar；若产品答案事实错误，即使自动 PASS 也按产品 GAP 处理。

## 5. 执行记录

### B1：真实 Trace 精确事实 × 症状驱动写模式

- cases：`real_trace_c2_dstate_iowait`、`github_issue_zod_prefault_symptom`
- 状态：已运行；自动 2/2 PASS，人工 1 PASS / 1 FAIL。
- 选择目的：跨 read/trace 与 apply 两个最高风险面，验证最新主线的终局正确性和泛化定位能力。
- 结果目录：
  - `eval/results/real_trace_c2_dstate_iowait-20260730-203955`
  - `eval/results/github_issue_zod_prefault_symptom-20260730-203955`
- 人工裁定：
  - Trace：FAIL。正文未逐项给出三次发生时间，`D-state=0` 没有声明其为“非 IO D-state”；系统补充又输出大段非请求因果树，并同时声称“没有因果行”、误报 `0.635ms > 0`。
  - Write：PASS。最终修复和测试正确，但 source/test 被拆片，导致一次可避免的失败验证与重规划，耗时 344s。

第一次修复后回放（`main@6e398ce08`）：

- 结果目录：
  - `eval/results/real_trace_c2_dstate_iowait-20260730-210716`
  - `eval/results/github_issue_zod_prefault_symptom-20260730-210716`
- runner：2/2 PASS；人工仍为 Trace FAIL、Write PASS（存在新的控制面 GAP）。
- Trace 已确认修复：五态使用“非 IO D-state”，IO 数值 comparator 不再误报，query-local “无因果行”不再覆盖最终投影。
- Trace 未确认修复：analyzer 把该题标成 `intent=root_cause + question_kind=conditional + predicate_axis=condition + runtime_target=59566`；family resolver 仍让宽泛 intent 抢在精确条件事实之前，补采继续输出约 800 行根因投影。
- Trace 新 witness：模型第一次把 blob 年份从 `2026` 写成 `2025`。这个不存在的别名被当成 action-required 物理 admission，并污染整轮；随后正确的 attached-trace 查询和其他探索工具全被终止闩锁拒绝。系统补采却绕过同一闩锁，对健康附件执行无窗 `root_cause_rank`，形成“模型不能查、系统能查”的权限矛盾。
- Trace 正文仍漏三次逐项发生时间，且把三条 `sched_blocked_reason` 错述成没有记录；聚合树仍将两个直接成员组写成“共 2 段”，而原始 occurrence 为 3。
- Write 的普通 source+test 已在同一 slice 应用，W1 生效；操作型 `tests/check_prefault_schema.py` 保持隔离符合安全负例。新 GAP 是 controller 追加 `batch-1-cumulative-review` 后，planner 为“仅验证”制造无功能注释 patch，`verify_batch` 又被规范化成 `apply_plan`，无谓产生第二个修改批。

第二次修复后回放（`main@5fb0b594b`）：

- 结果目录：
  - `eval/results/real_trace_c2_dstate_iowait-20260730-212419`
  - `eval/results/github_issue_zod_prefault_symptom-20260730-212419`
- runner：Trace FAIL、Write PASS；人工两项均 FAIL。R1 已确认生效：该题进入 `QFGeneric`，系统只补 `window_stats`，不再执行 `root_cause_rank`；R2/R3 本轮未触发，既有单测继续作为接线证据。
- Trace 的物理引擎其实已在 `blocked_reason_census` 保存目标 59566 的 `count=3` 和 caller×3，但目标自身 `io_wait=0.635ms` 低于 `IOWaitTop` 的固定 Top-8，未进入该结果面；最终 prompt 的 32 条投影又没有目标 PID 优先权，目标 census 被大量后台线程记录挤出。模型因此按三个离散 D 起点自行相减，把两段中间的运行区间错误并入等待，输出 2 次/19.671ms，而真实值为 3 次/0.635ms。
- 同一运行还暴露了 occurrence 载体错误：`emit_investigation_complete` 的 `member_set.value=2` 同时携带 3 个成员/支持引用，归一器再按去重后的 label 把值改成 1。集合语义不能表达“同 caller、不同时间的三次发生”，发生次数被错误折叠。
- 虽然问题已路由 Generic，成文仍无条件插入背景 `VerifyClass` 因果投影、确定性优化、无关 state_churn 指标、频率/VSync 权限和 214 条系统观测。状态事实问答被全量因果/报告合同污染的问题仍存在于答案 materializer，而不在 family resolver。
- Write 自动 PASS 但人工 FAIL：分析阶段把 `result.schema.default ??= result.schema._prefault` 错读为“右值 falsy 时不赋值”，首个 ChangePlan 又错误建议改成 `||=`；验证失败后第二个 plan 才改成直接赋值并通过。最终 patch 正确不代表首轮机制理解正确，需后续以行为契约校验 plan 语义，而不能只以最终测试绿判定全程健康。

第三次修复后回放（runner 快照 `main@c82ba0ead`）：

- 结果目录：
  - `eval/results/real_trace_c2_dstate_iowait-20260730-214656`
  - `eval/results/github_issue_zod_prefault_symptom-20260730-214656`
- runner：2/2 PASS；人工 Trace FAIL、Write PASS（但 Write 控制面 FAIL）。
- Trace 的 cap-immune 目标账和 target-first prompt 已生效：首段能稳定拿到 `blocked_reason count=3` 与 `io_wait=0.635ms`。但 analyzer 本轮随机发出 `intent=trace + question_kind=call_chain`，并把同一个运行时身份拆成 `com.baidu.tieba`、`59566` 两个 entities；无 `predicate_axis=call`、无 relational predicate、只有一个 `runtime_target`。这使 R7 的窄事实答案形状守卫被绕过，因果树/优化/背景指标再次全部插入。
- 主答案又暴露了独立的逐次证据缺口：它把 capped `event_search` 的两个样本当成全量，按错误的 sched-in 边界算出 `0.168+0.183=0.351ms`，随后为与 typed 总账 `3次/0.635ms` 的冲突编造“预览窗外或其他内核计量方式”。真实三段是 `0.138+0.147+0.350=0.635ms`；精确发生时刻/区间不能靠 sampled event rows 或模型自行配对。
- 自动 oracle 仍 PASS，因为它只分别看到 “3次” 与 “0.635ms”，没有检查正文是否同时列满三次、逐项和是否等于总量、是否出现无证据的差额解释。
- Write 首个计划已正确修改 truthy check 并补全 false/0/空串测试；但计划内 Python probe 调用缺失的 `npx`，基础设施 unavailable 被写成代码 verify failure，触发多轮 replan/explore/verify。第二计划甚至无谓地把已经正确的 `"_prefault" in schema` 改成 `!== undefined`；最终 `run_tests` 的 TestSurface 才正确执行 `npm missing → make check`，42ms 即通过。最终产品正确，不代表 537s 的控制面健康。

第四次修复后回放（runner 快照 `main@22623c89f`）：

- 结果目录：
  - `eval/results/real_trace_c2_dstate_iowait-20260730-222602`
  - `eval/results/github_issue_zod_prefault_symptom-20260730-222602`
- runner：2/2 PASS；人工 Trace FAIL、Write PASS。Trace analyzer 已稳定输出 `intent=return_value`，R9 的 call-chain 收紧已被真实回放覆盖。
- R10 的引擎值通道和 ledger 均正确：完整集合是三段 `0.138+0.147+0.350=0.635ms`。但集合记录把 roster 塞进共用 `Summary`，在 prompt 投影时受 180 字符上限截断为只含第一段的前缀；逐段 leaf 又因 observation 数量预算只进入 closure、没有进入成文上下文。模型因此仍自行重配对 event rows，正文错误写成 2 段、`0.168+0.183=0.351ms`，并错误解释第三条与第一条重叠。
- 自动 oracle 再次假 PASS：它能从系统 footer 的不同位置分别匹配到 `3` 和 `0.635`，但没有约束 principal answer 必须列满三段且逐项和闭合。由此确认 R11/E1 不是测试词面问题，而是 eval 缺少 principal 作用域与关系约束。
- Write 首个计划、实现和 false/0/空串回归均正确，无重规划，`make check` 通过。本轮没有生成缺少 child executable 的 verification probe，故 W4 状态为“专项测试已覆盖、真实回放未触发”，不能虚报为回放覆盖。

第五次修复后回放（runner 快照 `main@0dbfe2350`）：

- 结果目录：
  - `eval/results/real_trace_c2_dstate_iowait-20260730-224539`
  - `eval/results/github_issue_zod_prefault_symptom-20260730-224539`
- runner：Trace FAIL、Write PASS；人工两项均 FAIL。R11/E1 已生效：Trace 主答案缺三组精确 occurrence 与闭合关系时，系统 footer 不再能替它满足 oracle。
- Trace 新根因一：用户没有给时间边界，语义范围应是整份 trace；模型查询却自行指定 `34579.450627..34579.470000`，第三段从 `34579.471372` 开始，窄窗引擎因此诚实发布 2 段/0.285ms。探索窗被误升级成用户请求范围，最终又声称“全程仅 2 次”。这是范围 authority GAP，不是第三段的数值特例。
- Trace 新根因二：R12 的 occurrence set 与 leaf 已进入 `Typed Repair And Evidence Handoff`，但这里只渲染 observation ref；top-10 Observation Ledger 又被 root-cause rows 占满，未携带 set 的 notes/value。成文看到了“有一份权威集合”的指针，却看不到完整性、成员和 Σ，继续按 sched rows 自行配对成 2 段/0.351ms。
- Write 首个 plan 已正确修复属性存在性判断并补 false/0/空串测试；但 workflow 又创建 `batch-1-cumulative-review` 修改批，把正确的 `??=` 改成无条件赋值，重复追加 4 个同义测试。最终要求顺序 cherry-pick 两个 plan。测试未覆盖“已有 default 优先”负例，故错误覆盖语义仍通过。这把 W2 从性能/控制流债升级成会交付错误补丁的 correctness GAP。

## 6. 统一 GAP 台账与施工状态

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B1-T1 | P0 | 问题类型 | `IntentTrace` 被当作调用链答案形状 | 明确的非 call `question_kind/axis` 进入 generic/runtime fact；显式 call 与旧无信号记录保留调用链兼容 | 部分修复；RootCause 条件事实残余转 R1 |
| EVAL-B1-T2 | P0 | 补采权限 | 补采以全量核心因果族齐全为目标，不看请求族 | D/blocked_reason 事实只要求 `WindowStates + BlockedReasonCensus`，缺失时仅补 `window_stats` | 部分修复；RootCause 条件事实残余转 R1 |
| EVAL-B1-T3 | P0 | 覆盖权限 | query-local refinement 被升级成 report-global absence | 最终 typed 投影已有因果行时，不发布“没有因果行”的局部 refinement 总结 | 回放已覆盖 |
| EVAL-B1-T4 | P0 | 状态语义 | 互斥五态与含子类四态共用裸 `D-state` 标签 | 五态显式标“非 IO D-state”；四态保留 D 族并披露其中 io_wait | 回放已覆盖 |
| EVAL-B1-T5 | P0 | 数值校验 | IO comparator 漏独立 `io_wait` lane | comparator 使用 `io_wait + sleep_io_wait`，不混入非 IO D | 回放已覆盖 |
| EVAL-B1-W1 | P1 | 写调度 | 小型 source+test 计划按角色先拆再验 | ≤在线上限且无敏感隔离路径时保持一个原子 slice | 回放已覆盖；敏感文件仍隔离 |
| EVAL-B1-R1 | P0 | 类型优先级 | 宽泛 `intent=root_cause` 抢在精确 `conditional + condition + runtime_target` 之前 | 无 call 信号的 typed runtime conditional fact 先路由 Generic；无 target 的 genuine root cause 与显式 call 保持原族 | 第二次回放已覆盖 |
| EVAL-B1-R2 | P0 | 输入权限 | 一个不存在的模型别名被升级为整轮物理 admission terminal | 仅在“路径确实不存在且 canonical attached source 在场”时返回 typed、非终止 source-selection repair；禁止静默换源；无附件/二进制仍 terminal | 已施工，待回放 |
| EVAL-B1-R3 | P0 | 补采权限 | 系统补采绕过模型侧 run-wide admission terminal | supplement 先读取同一个 typed terminal latch，命中即零 engine 执行并发布 closed-set skip reason | 已施工，待回放 |
| EVAL-B1-R4 | P0 | 值通道 | 目标线程的低时长状态账被固定 Top-N 挤掉，完整 census 仅留在引擎私有面 | 目标锚定的 `window_stats` 在完整 artifact 窗也发布 `target_window_states`；目标五态账是 cap-immune 的单一值源，禁止从相邻状态起点做 hull 相减 | 第八次回放已覆盖 |
| EVAL-B1-R5 | P0 | Prompt 权限 | 32 条 observation 投影只按来源/角色排序，不保 requested runtime target | 仅用 typed `RuntimeTargets` 的精确 PID/thread 匹配给目标记录软排序优先权；不解析题面、不改变事实硬门 | 第八次回放已覆盖 |
| EVAL-B1-R6 | P1 | Occurrence 语义 | 运行时 `support_ref` 不进入源码位置解析，`member_set` 又剥掉成员括号中的 `line/ts`，把“同 caller、不同时间”的重复发生折叠为一个成员 | 归一身份同时消费语义标签和 schema 化 occurrence 坐标（line/ts/start_ts/end_ts/ordinal）；同坐标幂等去重、不同坐标保留，纯说明性 decorator 仍按集合语义去重；次数的长期权威仍由专用 occurrence/leaf carrier 承担 | 已施工；当前聚合逃逸覆盖，专用 carrier 归 T6/R8 |
| EVAL-B1-R7 | P0 | 答案形状 | Generic 状态事实仍无条件追加因果树、优化点、背景指标与全量观测 | 复用同一 typed runtime-conditional-fact 谓词，禁止这些非请求报告块 materialize；真实 root cause/call 负例保持原行为 | 主 materializer 第八次回放已覆盖；last-mile raw dump 漏网转 R19 |
| EVAL-B1-R8 | P0 | 终局一致性 | principal answer 可与目标 typed census/五态账/完整 occurrence roster 冲突，系统 footer 不能修正文 | 对精确目标事实建立 typed principal-value consistency check；确定性事实冲突重写/重试，缺值仅 advisory | occurrence roster 臂第八次回放已覆盖；其他精确值族后续扩展 |
| EVAL-B1-T6 | P1 | 聚合口径 | 直接成员组数被渲染为原始发生段数 | 新增/贯通 leaf occurrence count，未知时显示“成员组”而非“段” | 待施工 |
| EVAL-B1-E1 | P1 | Eval oracle | footer/supplement 可替主答案满足 anchor | 增加 principal-answer 作用域的 contains/banned/line-regex/folded-text-regex oracle，保留全答 oracle 用于系统面 | 第六次回放已覆盖 |
| EVAL-B1-W2 | P0 | 工作流状态机/正确性 | 已正确应用的批次被派生为修改型“累计验证”计划；第二 plan 可覆盖正确语义并重复测试，测试绿仍交付错误双 commit | typed verify-only/cumulative-review 不允许携带 ChangePlan；已完成目标直接复用前批 diff/report 或执行 verification probe。若确需新修改必须有新的未满足行为合同和失败证据，不能以累计审查为理由改代码 | 第六次回放已覆盖 |
| EVAL-B1-W3 | P1 | Write 语义校验 | 最终测试绿掩盖首个 plan 对 `??=`/`||=` 的错误机制理解和无效修法 | 从 typed 行为契约验证 planned operator/赋值语义；失败必须在 apply 前 replan，禁止把“后续修对”当作首轮健康 | 待施工 |
| EVAL-B1-A1 | P2 | Analyzer 语义 | 普通线程状态/等待粒度被误标 `error_granularity_profile=true` | 修 analyzer schema/example 与 typed consistency 校验；当前运行时路由不得依赖该 noisy 错标 | 已立案 |
| EVAL-B1-R9 | P0 | Analyzer/答案形状权威 | `call_chain` 可在没有 call axis、relational predicate 或真正端点时持久化；同一 runtime label+PID 被误当成两个 endpoints，随机绕过窄事实守卫 | `call_chain` 必须携带精确关系信号；通用面要求 call axis/relational/two named entities，runtime 面进一步按 distinct `runtime_targets` 判定，单 focus identity 必须有 call/relational 信号；同步补 analyzer guard，不解析题面关键词 | 第四次回放已覆盖 |
| EVAL-B1-R10 | P0 | 逐次状态证据 | capped `event_search` 只提供样本，目标账只提供总量；模型自行拿 sched-in 配对，漏第三段并制造 0.351ms 与 0.635ms 冲突 | 在目标窗状态权威中增加 bounded、带 emitted/total/complete 的逐次区间 carrier；区间边界、state、iowait、caller、duration 由引擎配对，成文禁止从相邻事件自行 hull 相减 | partial：引擎与 ledger 已覆盖；prompt 末公里转 R12 |
| EVAL-B1-R11 | P1 | Eval/终局一致性 | oracle 分别匹配 count 与总量，正文列少 occurrence、逐项和不等于总量仍 PASS | 增加 principal 作用域 occurrence rowset 与 folded relation oracle；先验证三条精确 start/duration，再要求三条与 count/Σ 同处主答案，禁止 footer 单独满足 | 第六次回放已覆盖；产品硬一致性门 R8 仍独立开放 |
| EVAL-B1-R12 | P0 | Prompt 集合权限 | 完整 typed rowset 被拼成一个共享 Summary；字符上限把集合截成看似完整的前缀，逐行 leaf 又受记录数预算挤出 | 集合 Summary 只声明 authority；用独立 typed notes 携带 prompt 层 `complete/incomplete + emitted/total + prefix_sum` 和一行一个 occurrence。prompt cap 与 note budget 显式对齐；截断时必须标 incomplete，禁止把前缀当全量 | partial：carrier 正确，通用成文投影仍可丢失，转 R15 |
| EVAL-B1-R13 | P0 | 请求范围权威 | 用户未给时间窗时，全 trace 语义被模型自行选择的探索窄窗替代；窗内 complete 被误述成 artifact complete | 区分 `requested_scope` 与 `query_window`；未给用户窗的 exhaustive/count/when 请求必须至少有 artifact-scope census，窄窗结果只能是局部 witness，不能发布全局 complete/total | partial：无界查询已覆盖全 artifact，成文 coverage 识别转 R16 |
| EVAL-B1-R14 | P0 | Repair handoff 值逃逸 | typed repair handoff 只渲染 observation ref，完整 rowset 的 notes/value 依赖普通 ledger Top-N，可能被无关高分行挤掉 | 对 repair refs 中的 exact bounded rowset 做值级投影：携带 completeness/meta/member notes；仍服从显式 note/member cap，超限标 incomplete，禁止只给 opaque ref | partial：repair 精确引用场景已覆盖；非 repair-ref carrier 转 R15 |
| EVAL-B1-R15 | P0 | 成文值权限 | 权威 bounded rowset 只有普通 ledger 或“恰好命中 repair ref”两条机会；前者会截断 notes，后者依赖偶然引用关系 | 为完整、目标匹配的 typed bounded rowset 建立独立答案 authority lane，按 meta/sum/member 行有界投影；不参与普通 Top-N，也不依赖 repair 发生 | 第八次回放已覆盖 |
| EVAL-B1-R16 | P1 | 范围覆盖识别 | 用户请求 full artifact、无界 `trace_query` 已覆盖 artifact，但 finalizer 只认 system supplement metadata，误报 whole-artifact unavailable | 从 typed query bounds/source/result coverage 铸造统一 `runtime_artifact_scope_coverage`；无界 canonical query 与系统补采共用消费面，模型窄窗不得铸 full | 第八次回放已覆盖 |
| EVAL-B1-R17 | P0 | Analyzer typed carrier 完整性 | analyzer 已提供 `runtime_targets`，但 process target 缺 pid 等身份结构错误只触发 WARN，随后该项被丢弃；target-first、最小补采、scope 与 occurrence authority 同时失权 | 对已提供但身份结构非法的 runtime target fail-loud，要求 analyzer 当轮修正；非法 source 可继续清空并警告，因为不改变身份；不得从自由文本或 thread 后缀静默猜 pid | 第八次回放已覆盖 |
| EVAL-B1-R18 | P0 | Runtime fact 形状 | 单目标、非 call、非 diagnostic 的状态/时间/内核原因事实若被分类为 `intent=explain + kind=mechanism`，不进入既有 conditional-fact 谓词；补采与 materializer 又扩回根因报告 | 建立共享 typed `focused runtime fact` 谓词，覆盖 explain/mechanism 的非 call 非 diagnostic 形；family、supplement、report materializer 同源消费，真实 root-cause/call 保持负例 | 第八次回放已覆盖 |
| EVAL-B1-R19 | P1 | Last-mile 发布权限 | 主 materializer 已按 focused-fact gate 禁止 raw observation dump，但 `renderTraceQueryObservationSupplement` 是独立最后一公里通道，未消费同一答案形状权威，仍把 220 条后台观测按 cap 发布 44 条 | last-mile renderer 复用 `IsFocusedRuntimeFactQuestion`；聚焦事实保留主答案、typed target authority、范围/算术等必要附注，但不发布通用因果/背景 raw dump；diagnostic/root-cause/call 负例保持 | 第十次回放已覆盖 |
| EVAL-B1-R20 | P2 | 算术关系绑定 | duration→percentage 正则允许跨 96 字符自由连接，把前一指标 `sleep=85.915ms` 与后一指标 `io_wait<0.5%` 拼成一条关系，生成 59.433% 假警报 | 关系发现先按句内 clause/显式关系连接器绑定，同一片段出现新的 metric subject 时拒绝跨主体配对；保持 advisory，不提高为硬门 | 第十次回放已覆盖 |
| EVAL-B1-E2 | P2 | Eval 单位呈现等价 | principal oracle 先后只接受“每个数值重复 `ms`”或只接受“表头统一声明 `时长（ms）`”，两种都合法，单边限制会随模型选择列表/表格产生假 FAIL | 不降低产品事实 bar；精确 rowset/count/Σ 均保留，并允许“表头一次声明单位”或“每行显式声明单位”两种闭集形态；不用产品代码读取答案词面 | 已施工；双正例与无单位负例覆盖 |
| EVAL-B1-R21 | P0 | 终局一致性接线 | typed occurrence 硬门只扫描 `BlockSummary` 或显式 `SurfacePrincipal`；模型可把同一主答案拆成普通 `section/table/list` 且省略 surface_role，错误可见事实因此绕过完整 roster | 一致性检查消费所有模型生成的可见 block，排除带不可伪造 `SystemGeneratedKind` 的系统块；仍只在 complete target roster + 已开始发布 occurrence 关系时激活，不解析题面/PID/数值 | 第十次回放已覆盖 |
| EVAL-B1-W5 | P2 | Write 计划效率 | 正确微修在 write analysis schema、probe changed-symbol/path 和 insert anchor 上反复修形，23 行 patch 耗时 323s；最终 complete batch 还再次请求 verify | 先按失败类型审计 schema/examples 与 probe contract，统一表达可执行 changed-symbol 引用和 EOF 插入；controller 对 complete batch 保持 finish（已有确定性兜底） | 第七次回放立案，B1 correctness 收口后处理 |
| EVAL-B1-W4 | P0 | 验证可用性 | ChangePlan probe 的子进程缺失（`npx`）被判为产品代码失败，触发 replan 并改写已经正确的实现 | probe wrapper 以结构化 `subprocess._execute_child + ENOENT + filename` 识别 child executable 缺失并发布 `verification_probe_dependency_missing`；unavailable 自动落到确定性 TestSurface 下一候选，只有真实断言/测试失败才进入 needs_replan | 已施工；专项测试覆盖，第四次回放未触发该分支 |

施工批次：

1. `B1-T/P0`：T1–T5；问题形状、补采需求、覆盖权限与 D/IO 口径一次闭环。
2. `B1-W/P1`：W1；小型计划原子 slice，敏感路径负例保持隔离。
3. `B1-R`：重建后仍以并行 2 同时回放两个 B1 case；人工审查不得只看自动 PASS。
4. `B1-F`：T6/E1 独立小批，避免把聚合 carrier 与 runner oracle 混进运行时修复。
5. `B1-R/P0`：R1–R3；精确条件事实优先级、source-selection/physical-admission 分层、补采共享 terminal authority。
6. `B1-W2/P1`：禁止 verify-only 批次制造 patch；与 trace carrier/Eval runner 分批。
7. `B1-V/P0`：R4+R5；先打通目标五态单一值源和 prompt 目标优先权，再重放判断是否还需要 R8 硬一致性门。
8. `B1-S/P0`：R7；窄事实答案禁插非请求报告块，真实因果问答作为负例。
9. `B1-O/P1`：R6+T6；occurrence/set 分型和 leaf count 独立施工，避免把展示措辞与值通道混批。
10. `B1-W/P1`：W2+W3；write controller 状态机与 plan 语义校验独立施工。
11. `B1-RC/P0`：R9；收紧 call-chain typed 一致性，禁止一个运行时 focus identity 伪装 source→sink。
12. `B1-IV/P0`：R10+R8+R11；逐次区间 carrier、principal consistency 和 eval 关系 oracle 同源闭环。
13. `B1-VU/P0`：W4；验证 probe unavailable 分型并自动 fall through，不把环境缺工具升级成代码 replan。
14. `B1-PL/P0`：R12；修复 bounded rowset 的 prompt 末公里，集合完整性与成员逐行分载体；与 eval runner 的 principal oracle 分批提交。
15. `B1-EV/P1`：R11+E1；runner 增加 principal-answer 可选作用域与 exact-row/folded-relation 合同，完整 footer oracle 保持兼容。
16. `B1-RA/P0`：R13+R14；先守请求范围，再让 repair ref 携带 bounded exact values；不得通过扩大普通 ledger 全局预算解决。
17. `B1-WF/P0`：W2；禁止已经正确且已验证的批次派生新的修改型 cumulative-review；验证汇总必须复用已应用 diff/report，新增行为负例固定“已有 default 不被覆盖”。
18. `B1-AP/P0`：R15+R8；先建立独立的权威 rowset 成文载体，再用同一载体做 principal 一致性校验，禁止重复解析自由文本或按具体 PID/时间特判。
19. `B1-SC/P1`：R16；统一无界模型查询与系统补采的 artifact-scope coverage 载体，范围权威与值权威分离施工。
20. `B1-TI/P0`：R17；先让 runtime target 身份结构 fail-loud，再验证 R15/R16 的真实接线，禁止以 entity/thread 文本猜补 PID。
21. `B1-RF/P0`：R18；focused runtime fact 形状扩到非 diagnostic explain/mechanism，三处消费者共用同一 typed 判定。
22. `B1-WE/P2`：W5；B1 correctness 收口后处理 write schema/probe 规划摩擦，不与 trace 权限批混交。
23. `B1-LM/P1`：R19；修 last-mile raw observation 发布权限，复用既有 focused-fact typed 判定。
24. `B1-AR/P2`：R20；算术关系从“同句任意相邻数值”收窄为同主体显式关系，保持 advisory。
25. `B1-EV2/P2`：E2；为 principal 表格补单位继承的通用 oracle，不改产品答案。
26. `B1-RG/P0`：R21；complete typed roster 的一致性范围覆盖所有模型可见 block，系统生成块保持隔离。

`B1-T/P0` 验证：新增原 witness、显式 causal 邻接正例、旧无 typed 形兼容负例、IO comparator 正/负例和 projection-local refinement 接线 pin；`go test ./internal/types ./internal/orchestrator ./internal/tool -count=1` 三包通过（tool 全包 168.166s）。

`B1-W/P1` 验证：两文件“生产实现+直接回归测试”保持单 slice；同样两文件但含 `package.json` 的负例仍拆成两个 slice；6 项跨 owner/role/CI 既有大计划仍按原策略拆分。安全审批、风险门与工作树隔离均未改变。

`B1-R/P0` 局部验证：RootCause 标记的 `conditional + condition + runtime target` 进入 Generic，target-less RootCause 与显式 call 负例保持原族；D-state 补采只需 `window_stats`；不存在别名+健康附件只返回 `trace_source_selection/action_recommended` 且不 materialize/不 arm terminal，无附件缺失路径仍是 action-required admission；terminal latch 命中时系统补采零结果并返回注册 reason `trace_input_admission_terminal`。

`B1-R/P0` 第二次回放：R1 已覆盖；R2/R3 未在该 witness 触发。新的失败已收敛为 R4/R5/R7 三个通用机制，而不是为 `59566` 或 `0.635` 添加题面/数值特判。

`B1-V/S` 验证：真实 tieba fixture 的无显式时间窗 `window_stats(pid=59566)` 直接发布 `target_window_states.io_wait=0.635ms`、`d_state=0`，不再依赖 Top-8；40 条后台记录挤压下，typed PID/thread 精确匹配的目标五态账和 blocked_reason census 均保留在 4 条 prompt 预算内，exploration cursor 负例无优先权；同一个窄事实谓词同时接入 family resolver 与 8 个系统 materializer，target-less root cause 和显式 call 负例保留全报告权限。半窗负例继续 fail-closed。`go test ./internal/types ./internal/tracequery ./internal/tool -count=1` 三包全量通过（types 23.394s、tracequery 69.753s、tool 161.358s）。

`B1-O/P1`（R6 第一段）验证：用第二次回放中三条同 caller、不同 `line/ts` 的 blocked-reason 成员原样构造回归，归一后保持 `value=3 / members=3`；相同坐标但字段次序不同的重复记录仍归一为 1，`(background)/(supporting)` 等无 occurrence 坐标的说明性 decorator 仍保持原集合去重。该修复不把 `.txt` 运行时附件伪装成当前源码引用，也不改变源码同名成员按真实 source location 区分的既有规则。`go test ./internal/types` 全量通过（24.658s）。

`B1-RC/P0` 局部验证：`call_chain` 在无 call axis、无 relational predicate、少于两个 named entities 时 fail-loud；源码单 target caller/wakeup（AxisCall）、relational lookup、双端点 source→sink 均保留。运行时附加收紧：一个 `runtime_target` 即使被 analyzer 拆成“标签 + PID”两个 entities，仍必须提供 call/relational 信号，否则拒绝并触发 analyzer 重试；两个 distinct runtime targets 保留关系形状。该门只读 schema 化 typed 字段，不读中英文题面、不匹配 case/PID。

`B1-IV/P0`（R10 值通道）验证：`Interval` 由原有单次 blocked-reason enrichment 直接携带 caller/reason-line/iowait authority，目标窗状态账不二次扫事件、不按 sched-in 重配对；`TargetWindowStateAccount` 发布按时间排序的 D/io_wait 与 Harmony `S+iowait` occurrence roster，硬上限 32，显式携带 `status/emitted/total`。真实 tieba fixture 精确固定三段：`34579.451701 / 0.138ms`、`34579.452934 / 0.147ms`、`34579.471372 / 0.350ms`，三段均为 `sync_buffer_read_wi`、`iowait=1`，Σ 与目标 `IOWaitMs=0.635` 相等。ledger 同时发布一个完整 roster set 记录和逐段 typed span；tight prompt budget 优先保 target account、blocked census、occurrence set，再选 leaf，探索 cursor 负例不获得用户目标优先权。零等待发布 `complete count=0`，不是 absent。

`B1-WV/P0`（W4 验证可用性）验证：Python probe wrapper 不解析 stderr 文案，而是从异常对象与 traceback 精确确认 `FileNotFoundError(errno=ENOENT)` 来自 `subprocess.py::_execute_child`，并携带 `missing_executable` 字段；该类记录为 `runner_missing / verification_probe_dependency_missing / unavailable`，继续执行 typed TestSurface。端到端 fixture 固定“探针内部缺少 child executable → `make check` 通过 → 最终 verification=passed”；产品代码主动抛出缺文件异常的既有负例仍为 `verification_probe_exception/tests_failed`，不被环境分型吞掉。

`B1-PL/P0`（R12 prompt 末公里）验证：集合 Summary 不再携带可被字符 clamp 截断的 roster；prompt 记录改为独立携带 `target_wait_occurrence_prompt=status,emitted,total`、`target_wait_occurrence_prompt_sum_ms` 和逐行 `target_wait_occurrence`。prompt 硬上限为 8 条 occurrence，与默认 runtime-origin note 预算的“meta + sum + 8 行”闭合；引擎集合超过该上限或本身不完整时，prompt status 必为 `incomplete`。原三段 fixture 在一次 prompt projection 后仍精确保留三组 start/end/duration/state/iowait/caller，且成文规则禁止重建、合并或丢弃已列 occurrence。

`B1-EV/P1`（R11/E1 eval 终局判定）验证：runner 新增可选 `EXPECT_PRINCIPAL_CONTAINS`、`EXPECT_PRINCIPAL_NOT_CONTAINS`、`EXPECT_PRINCIPAL_MATCHES_REGEX`、`EXPECT_PRINCIPAL_MATCHES_TEXT_REGEX`，只检查分隔线后的模型可见答案，并在确定性 `Trace 因果投影 / Trace Causal Projection` 标题前停止；旧全答案 oracle 语义不变。假 runner 固定“principal 正确、footer 含 banned 词”仍 PASS，以及“所需事实只在 footer”必须 FAIL。B1 Trace case 现要求 principal 精确列出三组 start/duration，并以 folded relation 同时携带三组、count 与 `0.635ms`；第四次旧错误答案在新合同下稳定缺 4 项，不再假 PASS。`bash -n` 与 `eval/runner_lib_test.sh` 全部通过。

`B1-RA/P0`（R14 repair handoff 值逃逸）验证：通用 `ToolObservationRef` 继续只携带 identity/source/producer，不扩大跨阶段 schema；finalizer 只对 carrier 精确引用、且具有 typed `target_wait_occurrence_prompt` 的 observation 按 ID 回连完整 ledger，把 `status/emitted/total`、prefix sum 和最多 8 条 engine-paired occurrence 逐值投影在 ref 旁。该专用投影不参与普通 Observation Ledger 的 Top-N，也不提高其预算；10-note 上限与 R12 的 `meta + sum + 8 occurrence` 完全闭合。相邻负例固定无关 advisory 即使位于同一 record 也不得逃逸。三类 wire key 已从 `display_only` 升为注册的 `soft_consumer`，producer fixture 与注册表覆盖门同步钉住。

`B1-RA/P0`（R13 请求范围权威）验证：`AnalysisIR v15` 新增必填 `runtime_artifact_scope_profile`，闭集区分 `not_applicable / full_artifact / explicit_time_window / bounded_selector / unspecified`；`full_artifact`、`explicit_time_window` 只有在 `source_quote` 与当前请求逐字相符（且显式窗上下界合法）时才成为确定性 authority，模型/工具文本不能铸权。对于窄 D/IO 状态事实，quote 锚定的全 artifact 或用户显式窗即使已有另一个模型窄窗的完整 family，仍强制一次同目标的最小 `window_stats`：全 artifact 以无 time bounds 的 engine 默认全窗执行，显式窗使用用户上下界，均不继承探索窗。真实 C2 fixture 复刻第五次回放的 `34579.450627..34579.470000` 窄窗，补采后恢复三段 `34579.451701/0.138ms + 34579.452934/0.147ms + 34579.471372/0.350ms = 0.635ms`。系统 metadata、zh/en disclosure 与 finalizer guidance 均区分“用户请求全 trace”和“未能推导窗口”；若全 trace 结果缺席，成文明示禁止用局部 witness 声称 artifact-wide all/only/count/total。`go test ./internal/types ./internal/skill ./internal/agent ./internal/tool ./internal/orchestrator ./internal/analysis/... -count=1` 全部通过（tool 168.462s）。

`B1-WF/P0`（W2 累计复审权限）验证：durable batch 新增仅由确定性控制器铸造的 `execution_mode=verify_only`，该字段不进入模型 schema；累计 actual-diff/proof follow-up 追加后直接进入 `verifying`，复用已应用的 ChangePlan 与当前 worktree，主循环明确跳过 StagePlan/StageApply；无论缺口是否带 dependency/graph 目标，非失败复审都不得借源码探索绕回 planner。后续模型回显不得覆盖控制器写入的 goal/purpose/expected_paths/success_criteria；`verification_probe` 来源的 target 保持 proof 分类，不再与同一缺口重复合成为 impact repair。若 verify-only 真的产生 typed verify-failure handoff，才创建独立普通 repair batch，并携带原未满足 criteria 与 failure-handoff 要求。端到端测试原样输入第五次回放的恶性动作（要求把 `??=` 改成 `=`），固定只执行一次 StageVerify、零 StagePlan、零 StageApply、零第二 ChangePlan。Zod eval fixture 同时新增“已有 `default: "existing"` 不得被 `_prefault: "replacement"` 覆盖”的负例，并要求保留 nullish assignment，防止测试只覆盖 falsy 正例却放过覆盖语义。`go test ./internal/types ./internal/writeflow ./internal/agent ./internal/orchestrator -count=1` 全部通过。

第六次修复后回放（runner 快照 `main@150347e0d96c`）：

- 结果目录：
  - `eval/results/real_trace_c2_dstate_iowait-20260730-234404`
  - `eval/results/github_issue_zod_prefault_symptom-20260730-234404`
- runner：Trace FAIL、Write PASS；人工结论相同。R11/E1 的 principal oracle 正确拦截了第 3 段错误，未再被系统 footer 假满足。
- Trace engine 的 typed occurrence roster 是完整且正确的：第 3 段为 `34579.471372..34579.471722 / 0.350ms`，总账仍为三段 `0.635ms`。Explorer 却把 `sched_blocked_reason@34579.471723` 当成区间起点，并自造 `34579.471876` 终点；该错误写入 `emit_investigation_complete.aggregate_facts` 后被 finalizer 原样发布。
- R12/R14 在真实链路中只覆盖了局部：普通 Observation Ledger 的整行 clamp 只露出 roster 第 1 段；R14 又只对“恰好被 repair ref 精确引用”的 carrier 投影值，本次完整 roster 不在 repair ref 集合，因此 finalizer 看不到 engine 权威的三行成员。这确认需要 R15 独立 authority lane，而不能继续扩大 Top-N 或依赖 repair 偶然性。
- 用户范围已由 analyzer 逐字锚定为 `full_artifact`，首个无界 `thread_timeline(pid=59566)` 也实际覆盖完整 artifact；但 finalizer 只检查 system-supplement metadata，仍错误发布“no typed whole-artifact supplement”。这是 R16 的统一 coverage 铸造缺口，不应回退到读取模型措辞或比较某个 fixture 时间值。
- Write 只有一个 ChangePlan、一次 apply、一次 verify；实现使用 `_prefault !== undefined` 且保留 `default ??=`，false/0/空串和已有 default 保留负例均通过，未生成 cumulative-review 修改批。W2 由真实回放覆盖；W3 仍是跨 case 的 plan 语义防线，不能因本轮模型恰好选对实现而关闭。

`B1-AP/P0`（R15 + R8 occurrence 臂）验证：新增唯一共享的 `TargetWaitOccurrenceAuthority` 编译器，只接收 deterministic trace-query producer、runtime-artifact origin、hard grounding、精确 predicate、typed 用户 runtime target、`status=complete`、`emitted=total`、`ResultCount=total`、最多 8 行且逐行 grammar 可解码的 roster；run-suffix producer 复用统一分类器。相同 roster 幂等去重，同一目标出现冲突 complete roster 时整体 fail-open，不让歧义驱动硬门。Finalizer 新增独立 authority section，直接按 `count/sum/row*` 投影，不参与普通 Observation Ledger Top-N，也不依赖 repair ref。全量 roster 硬一致性只在 principal 已开始发布 occurrence 关系时激活：每行必须在同一 principal segment 保留精确 start/end/duration；带权威 duration 但配错 interval 的行同轮拒绝，并携带完整 typed repair roster。重复 duration 的不同合法区间不会互相误判。硬门使用独立 `complete_target_wait_roster` closed signal 与 policy row；同轮硬策略仍由结构测试固定为仅三类。端到端 `emit_answer_document` 测试固定错误值落地前拒绝、Mutable 不留错误文档、精确修订后成功持久化；full emit 与 patch 共用原有 pre-emit chokepoint。`go test ./internal/types ./internal/agent ./internal/tool -count=1` 全部通过（types 19.162s、agent 2.958s、tool 162.475s），随后相等 duration 加固与接线专项测试继续通过。

`B1-SC/P1`（R16 统一范围覆盖）验证：新增 `RuntimeArtifactScopeCoverage` 单一消费面，同时接收 deterministic `trace_query` 的 typed coverage observation 与 `SystemTraceSupplementMeta`；普通查询只有在纯 core 成功、`IndexWindowed=false`、没有显式 time/line bounds、没有 pattern/span/recipe 派生窗时才能铸造 `full_artifact`。PID/thread、事件族与 limit 只约束关系/输出，不篡改物理时间范围；因此 coverage 明确与 `EnumerationAuthority` 正交，不能单独授权 `all/only/count/total`。Finalizer 不再把“系统补采”当唯一 full-artifact 证明：无界 canonical query 与系统补采共用 guidance；窄窗、行窗、pattern、span、recipe、windowed-index 六类负例均不能铸 full。专项测试覆盖统一编译器、run-suffix producer、系统补采合流、无界目标查询正例及六类负例。

第七次修复后回放（runner 快照 `main@394deb3320db`）：

- 结果目录：
  - `eval/results/real_trace_c2_dstate_iowait-20260731-002428`
  - `eval/results/github_issue_zod_prefault_symptom-20260731-002428`
- runner 与人工均为 Trace FAIL、Write PASS。
- Trace 的直接根因不是 R15/R16 算法再次算错，而是更上游的 typed target 被丢失：analyzer 两轮都发出 `kind=process, thread=com.baidu.tieba-59566` 但遗漏 `pid`；`emit_analysis` 两轮仅记录 `process target requires pid` WARN 并仍接受，规范化后的 `RuntimeTargets` 为空。目标 authority、target-first、用户 full-artifact 最小 `window_stats` 因此均没有授权输入；系统补采退回模型的 `34579.450000..34579.470000` 探索窗并执行无关 `root_cause_rank + critical_blocking_calls`，正文诚实但局部地只写 2 次/0.285ms。R15/R16 本轮未真正获得有效 carrier，状态仍是“待有效目标回放”。
- 同轮还确认形状谓词残余：`intent=explain + question_kind=mechanism + single runtime target + non-call + non-diagnostic` 没进入 conditional fact gate；这类状态/发生时间/内核等待原因问答被补采和答案 materializer 扩成根因报告。该修复必须基于 typed intent/kind/diagnostic/call/target 组合，不读 D-state 词面。
- Write 只有一份 ChangePlan、一次 apply、一次 verify，补丁为 `_prefault !== undefined` 并保留 `default ??=`；false、0、空串和已有 default 不被覆盖的回归均通过，W2 保持 covered。效率仍有 5 轮 write-analysis schema 修形、8 轮 planner probe/insert-anchor 修形及一次不可用 grep，形成 W5 P2，不与 Trace P0 混批。

`B1-TI/P0`（R17 typed target 完整性）验证：`parseRuntimeTargets` 不再把结构非法的目标降为 warning 后丢弃；任何已提供 target 若缺少 kind-specific identity、confidence 越界、PID 越界或 kind 非法，整组 fail-loud，`emit_analysis` 不持久化部分 RequestModel，要求 analyzer 在同轮修正。一个合法 target 加一个非法 target 的集合也整体拒绝，避免下游在不完整目标集合上铸 scope/selection/occurrence authority。相邻的非法 `source` 仍只清空来源并警告，因为 PID/thread 身份本身完整；重复项和超过 8 项的有界 cap 行为不变。实现没有从 entity、description、thread 后缀或用户自由文本猜补 PID。

`B1-RF/P0`（R18 focused runtime fact）验证：新增 types 层 `IsFocusedRuntimeFactQuestion` 单一判定源，保留既有 trace/root-cause + conditional/condition 形，并增加 `intent=explain + kind=mechanism + typed runtime target + non-call + non-diagnostic`。`ResolveQuestionFamily`、D-state 最小 supplement 与全部 runtime report materializer 共用该函数；旧 `IsRuntimeConditionalFactQuestion` 仅作为兼容 wrapper。正例固定第七轮 analyzer 形状只补 `window_stats` 且禁止因果树、优化、背景 metric、next-step、perf-quality 和 observation dump；负例固定 `Predicates.IsDiagnosticQuestion=true`、`DiagnosticProfile.IsDiagnostic=true`、root-cause mechanism、显式 call 和无 target 均保留原报告权限。硬判定只读 typed intent/kind/diagnostic/call/target，不读 D-state 词面、PID 或具体 case。

第八次修复后回放（runner 快照 `main@b81c6439931d`）：

- 结果目录：
  - `eval/results/real_trace_c2_dstate_iowait-20260731-004404`
  - `eval/results/github_issue_zod_prefault_symptom-20260731-004404`
- runner：Trace FAIL、Write PASS；人工两项均 PASS，但 Trace 有两个末公里质量 gap。Trace runner 的 FAIL 是 E2 假阴性：主答案表格已精确列出三行 `34579.451701..34579.451839 / 0.138`、`34579.452934..34579.453081 / 0.147`、`34579.471372..34579.471722 / 0.350`，列头统一声明 `时长（ms）`，并给出 3 次、统一 `sync_buffer_read_wi` 与 `0.635ms`；现有 regex 却要求每个单元格后重复单位。
- R17/R18 已真实覆盖：analyzer 在唯一成功的 emit 中给出合法 `kind=thread,pid=59566,thread=com.baidu.tieba`，范围逐字锚定 `full_artifact`；系统补采只执行一次无窗 `window_stats`，没有执行 root-cause/critical-blocking。R15 的三行完整 authority 和 R16 的 typed whole-artifact coverage 都进入 finalizer，模型主答案与权威 roster 一致，R8 没有误拒绝。
- R19：八个主 materializer 的 gate 生效，答案中没有因果树、优化点、metric/next-step/perf-quality 等系统块；但 agent 最后一公里仍无条件调用 raw observation supplement，220 条记录中按类别配额发布 44 条，绝大多数为无关后台线程。这是同一 typed gate 的漏接线，不是再增加一种问题分类。
- R20：正文只说 `io_wait 占比极小（<0.5%）`，算术正则却把该百分比向前绑定到另一个主体的 `sleep=85.915ms`，生成“重算 59.433%”假警报。该 arm 本来就是 soft advisory，应优先收窄关系绑定而非制造更多警告。
- Write：一份 ChangePlan、一次 apply、一次 verify；补丁为属性存在性检查并保留 `??=`，false/0/空串与已有 default 不覆盖均通过。Node/npm 不可用后按 W4 落到 `make check` 成功。仍有两轮 write-analysis（首轮精确合同缺 evidence_ref 被拒）和一次 planner 不可用 read_file，W5 保持开放，但未影响本轮正确性。

`B1-LM/P1`（R19 last-mile 发布权限）验证：`renderTraceQueryObservationSupplement` 现在与 family、supplement selector 和八个 runtime report materializer 共用 `IsFocusedRuntimeFactQuestion`；聚焦事实返回空 raw dump，但 observation ledger、专用 target-wait occurrence authority、范围覆盖和其他必要校验通道均未改动。typed diagnostic 正例、显式 call 正例及 `AnalysisIR=nil` 兼容路径继续发布原始观测补充。专项与 `go test ./internal/agent -count=1` 全包通过（2.774s）。

`B1-AR/P2`（R20 算术关系绑定）验证：duration/percentage 正则现在单独捕获二者之间的 bridge；跨逗号/分号时，只有空 bridge 或闭集的显式比例连接器才保留关系，新的指标主体默认跳过，符合 soft advisory 的高精度取舍。中英文 `sleep=85.915ms → io_wait<0.5%` 跨主体负例均不再生成 caveat；“占总窗口”“representing”正例和既有客户算术复算保持。实现不读取题面、PID、case 或具体数值，不改变任何硬门。专项与 `go test ./internal/tool -count=1` 全包通过（161.712s）。

R19 显式时间窗防回归加固：`explicit_time_window` 本身从不触发 focused-fact 抑制。带用户时间窗的 diagnostic explain/mechanism 与 root-cause mechanism 继续允许完整 Trace 因果投影；自动补采仍选择 `root_cause_rank + critical_blocking_calls`；agent last-mile raw observation supplement 同样保留。只有“typed target + 非 call + 非 diagnostic”的聚焦事实继续走窄形，时间窗不是取消因果能力的判据。

`B1-EV2/P2`（E2 表格单位继承）验证：B1 principal oracle 仍要求 `sync_buffer_read_wi`、三组精确 start/duration、三行与 count/`0.635ms` 的 folded relation；唯一变化是把单位权威显式放到 `时长（ms）` 表头，不再要求每个 cell 重复 `ms`。runner 自测新增通用 Markdown 表格形，表头携带单位、cell 只携带数值时 PASS；因此是 oracle 结构修正，不是放宽产品事实或读取特定 PID。

第九次修复后回放（runner 快照 `main@80f54e0ad55d`）：

- 结果目录：
  - `eval/results/real_trace_c2_dstate_iowait-20260731-010149`
  - `eval/results/github_issue_zod_prefault_symptom-20260731-010149`
- runner 与人工均为 Trace FAIL、Write PASS。R19 已覆盖：最后一公里不再发布 44 条后台 raw observation，focused fact 的系统输出只保留必要的五态/范围/补采披露。R20 已覆盖：`sleep=85.915ms → io_wait<0.5%` 假关系消失；本轮只复算正文明确发布的 `0.635ms / 0.44%`。E2 也生效，FAIL 来自真实第三段错误而非表格单位。
- Trace typed authority 仍完整正确，但正文第三段写成 `34579.471372..34579.471743 / 0.371ms`，同时在括号中又写 `0.350ms`，并声称“无 io_wait reason/blocked_reason 未记录”；权威行是 `34579.471372..34579.471722 / 0.350ms / iowait=1 / sync_buffer_read_wi`。R8 未拒绝的机制是结构性的：三段分别在 `kind=section, surface_role=""`，`targetWaitOccurrencePrincipalSegments` 只扫描 summary/显式 principal，因此连 activation 都未覆盖。这不是某个时间值或模型措辞特例，归 R21。
- Write 仍是一份 ChangePlan、一次 apply、一次 verify；`!== undefined` 保留 `??=`，false/0/空串与已有 default 不覆盖均通过。Node probe/npm unavailable 后确定性落到 `make check`，没有 replan。两轮 write-analysis 和一次不可用工具继续记 W5 P2，不阻塞 B1 correctness 收口。

`B1-RG/P0`（R21 可见模型块一致性）验证：occurrence consistency 的 segment census 不再用可选 `SurfaceRole` 作为事实权限边界，而是覆盖全部模型生成的可见 block（title/text/items/cells），并以 json 不可伪造的 `SystemGeneratedKind` 精确排除确定性系统块。第九轮的三个无 surface-role section 原样进入测试，错误第三段在持久化前收到 exact roster hard repair；同样内容标为 system-generated 时不激活模型一致性门。硬门的 complete roster、typed target、deterministic producer、已开始 occurrence relation 等既有激活条件均未放宽。专项及 `go test ./internal/types ./internal/tool -count=1` 全包通过（types 21.623s、tool 166.557s）。

第十次修复后回放（runner 快照 `main@7d1d894078eb`）：

- 结果目录：
  - `eval/results/real_trace_c2_dstate_iowait-20260731-011149`
  - `eval/results/github_issue_zod_prefault_symptom-20260731-011149`
- runner 为 Trace FAIL、Write PASS；人工两项均 PASS。Trace 主答案逐行精确发布 `34579.451701..34579.451839 / 0.138ms`、`34579.452934..34579.453081 / 0.147ms`、`34579.471372..34579.471722 / 0.350ms`，并给出三次、`sync_buffer_read_wi×3` 和 `0.635ms`。R21 已真实覆盖；R19 的后台 raw dump 和 R20 的跨主体算术假警报均没有复现。
- Trace 自动 FAIL 不是产品错误：本轮用列表并在每行显式写 `ms`，E2 却反向强制必须出现 `时长（ms）` 表头。合法的表格单位继承与列表逐行单位是两种等价呈现，评测应以闭集二选一表达，同时继续要求三行精确值、次数、Σ 和 caller。该修复只属于 eval oracle，不进入产品 hard gate。
- Write 仍是一份 ChangePlan、一次 apply、一次 verify；属性存在性判断覆盖 false/0/空串，`default ??=` 保留已有 default，probe 与 `make check` 全部通过，没有修改型 cumulative-review 或 replan。W5 的分析/探索效率债继续开放，但不再阻塞更高优先级 B2。
- B1 产品 correctness 至此收口。未触发的 R2/R3、T6/W3/A1/W5 继续留在统一台账，不能因单个 witness 未出现而虚报关闭；后续不再围绕该 case 增加题面、PID、数值或模型措辞硬门，转入 B2。

`B1-EV2/P2`（E2 单位呈现等价收口）验证：principal oracle 保留三条精确 start/duration、次数、Σ 与 caller 的全部事实约束，只把单位证明收敛成闭集二选一：表格由 `时长（ms）` 列头统一授权，列表则要求三条 occurrence 各自携带 `ms/毫秒`。runner 自测分别固定表格正例、逐行单位正例和完全无单位负例，前两者 PASS、后者 FAIL。该变化只在 eval case/runner test，不修改产品答案，不增加任何用户输入或模型原文驱动的生产硬门。

### B2：显式窗口供给/热限制 × 多文件数据引用投影

- cases：`real_trace_h4_supply_thermal_witness`、`data_multifile_reference_projection`
- 首轮快照：`main@f9da0419058c`
- 结果目录：
  - `eval/results/real_trace_h4_supply_thermal_witness-20260731-012003`
  - `eval/results/data_multifile_reference_projection-20260731-012003`
- runner：Trace PASS、Data FAIL；人工两项均 FAIL。
- Trace 正确面：用户显式 `13762.791708..13763.024898` 窗没有被 focused-fact gate 抑制，自动补齐与完整 Trace 因果投影均存在；四态账 `157.248+5.604+70.338+0=233.190ms` 闭合。
- Trace 错误面：模型把 CPU4 的实际/观测峰值比和全局 transition 次数升级成“明显受限”，违反同轮 typed `transition_authority=background_only`。窗内直接限制记录其实是 `cpu0 max=1530000,count=16,line=8048` 与 `cpu4 max=2100000,count=28,line=17113`；这些值在 verbose `window_stats` 尾部，head-safe frequency authority 只携带 evidence token，不携带 witness。模型随后做 4 个无结果 event_search，仍没恢复准确证据。系统因果 footer 的其他链节点碰巧含 `受热限压至 1.53GHz`，旧全答案 `EXPECT_CONTAINS` 因而假 PASS。
- Data 正确面：typed DAG 已计算 4 条贡献，reconcile 三组为 `17/4/5`，targets reference projection 与 actual answer 均为 `17,0,5`。
- Data 失败面：累计规则清册 ID 是 `rule_1..rule_6`，后续 qualify decisions 使用等价 ordinal `R2`。`NormalizeResult` 对任意未知 ref 的旧 fallback 只在规则有 `evidence_refs` 时启用；本轮规则由 typed action 生成、只有 notes，因此 `R2` 没有按唯一 ordinal 归一为 `rule_2`，完成门 fail-closed。之后 `next_stage=complete` 上的 custom_transform 尝试是 validation failure 的级联，不是第二个计算缺口。

新增 GAP：

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B2-T1 | P0 | Trace 频率证据权限 | `TraceEvidenceAuthority` 只给 typed evidence 类别 token，不给窗内 limit 的 CPU/min/max/count/line/ts；verbose 明细可能落到预览尾部，模型只能看 transition count/驻留比自行推断 | 从 `WindowStats.CPUFrequencyLimits` 铸造有界 typed witness roster，同时进入 ToolResult authority、head-safe summary 与 finalizer guidance；明确实际驻留/transition count 不等于 policy cap。只读 typed 结果，不扫描题面或答案原文 | partial：值载体已真实覆盖；ceiling presence/binding 语义转 T2 |
| EVAL-B2-E1 | P1 | Eval principal 作用域 | `1.53GHz` 与“全窗四态”只要在系统因果 footer 出现即可满足旧全答案 contains，错误主答案仍 PASS | 把目标运行量、四态和直接限频 witness 迁到 principal oracle；系统投影继续独立验存在，不取消显式时间窗因果能力 | 已真实覆盖；大小写等价漏斗转 E2 |
| EVAL-B2-D1 | P0 | Data 规则身份生命周期 | 多批 typed DAG 中，规则定义使用 `rule_N`，后续 ledger 使用 `RN`；两者是唯一可判的 ordinal alias，却因规则只有 notes、无 source-backed evidence_refs 而不归一，正确答案/reconcile 被 unknown_rule_ref 拦截 | 对闭集 ordinal 形 `R<N>/rule_<N>/rule-<N>/rule<N>` 建唯一 alias 索引；唯一时规范到实际 ID，歧义或任意未知值继续 fail-closed；保留旧 source-backed fallback | 第二次回放已覆盖 |
| EVAL-B2-P1 | P2 | 查询效率 | limit 事实藏在 verbose window_stats 尾部，模型连续发起带错误 pattern/selector 的 4 个 event_search | T1 的 head-safe witness 同时作为查询终止信息；若回放仍重复，再处理工具参数 affordance，不先为本例加 prompt 特判 | 观察 T1 回放 |

施工顺序：

1. `B2-D/P0`：先修规则 ordinal alias 的跨批规范化；独立单测覆盖唯一映射、歧义、任意未知 fail-closed，并提交推送。
2. `B2-T/P0`：再建频率 limit typed witness roster，接通 summary/finalizer；显式时间窗的因果投影与自动补采作为负回归固定，不增加用户/模型词面硬门。
3. `B2-E/P1`：eval principal oracle 只约束准确事实；系统 footer 另验，不能替主答案作答。
4. 重建后仍严格并行 2 回放两个 B2 case，人工审计日志和答案；纯模型措辞波动若不形成稳定机制，不追加硬约束。

`B2-D/P0`（D1 规则身份生命周期）验证：`NormalizeResult` 现在从实际 `RuleCoverage.RuleID` 建立闭集 ordinal alias 索引，仅识别 `R<N>/rule_<N>/rule-<N>/rule<N>` 四种结构形；只有一个 ordinal 对应唯一实际 ID 时，才把 rows、contributions、entity resolutions 的后续 typed `rule_refs` 规范到该 ID，并逐 ledger family 留存 `ResultPatches` 审计记录。同 ordinal 同时存在两个实际 ID 时不映射，任意未知 ID 也不映射，二者继续由 contract validation 以 `unknown rule_id` fail-closed。原有 source-backed fallback 保留；整个判定不读规则正文、notes、用户输入或模型答案词面。专项测试覆盖 notes-only 唯一映射、三类 ledger、跨批 seed、歧义和任意未知负例；`go test ./internal/dataquery ./internal/repl -count=1` 全包通过。

`B2-T/P0`（T1 频率证据权限）验证：`TraceEvidenceAuthority` 新增 `FrequencyLimitWitnesses`，唯一来源是 trace 引擎已经 pair-atomic 严格校验的窗内 `CPUFrequencyLimits`；每行携带 CPU、min/max kHz、有效 limit 行数、最严格 max 所在 line/ts、查询窗和闭集 `authority=direct_in_window_policy_limit`。max/count/line 任一无效的显示零值不能铸权；auto-window 合并只做精确 tuple 去重并保持 8 行有界。相同 witness 同时进入 trace summary 头部、finalizer 软指导与系统频率权限 caveat；transition count、实际/平均/驻留频率继续与 policy limit 分离。实现没有新增正文一致性 hard gate，也不读取用户输入或模型输出词面；显式时间窗、因果投影、自动补采选择和既有 causal hard gate 均未改动。专项与 `go test ./internal/types ./internal/agent ./internal/tool -count=1` 全包通过（types 18.716s、agent 2.846s、tool 167.188s）。

`B2-E/P1`（E1 principal 作用域）验证：H4 case 不再允许系统 footer 中的频率/四态词替模型主答案过关。principal 必须发布 `running=157.248ms / runnable=5.604ms / sleep=70.338ms / D-state=0`，并把目标确有运行证据的 CPU4 与直接窗内 `max=2.10GHz` policy-limit 行建立关系；`558MHz` 实际/驻留值或 830 次 transition 不能替代 cap witness。完整答案仍独立要求 `Trace 因果投影` 存在，因此显式时间窗的因果能力与系统自动补齐属于正向回归，不因 principal 收紧而被删除。该变更只在 eval oracle，不进入产品生产 hard gate。

第二次修复后回放（runner 快照 `main@87aa1cfbc3be`）：

- 结果目录：
  - `eval/results/real_trace_h4_supply_thermal_witness-20260731-014707`
  - `eval/results/data_multifile_reference_projection-20260731-014707`
- runner：Trace FAIL、Data PASS；人工：Trace FAIL、Data partial。
- Trace 正确面：四态 `157.248+5.604+70.338+0=233.190ms` 在 principal 内完整闭合；用户显式窗的完整 `Trace 因果投影` 和系统自动补齐均在，证明 T1/E1 没有伤害因果能力。T1 也真实接线：CPU0/CPU4 的 min/max/count/line/ts/window exact witness 同时进入 explorer、finalizer 和系统 caveat；trace_query 从首轮 9 次降为 6 次。
- Trace 失败面：模型已逐字看到 `cpu4 max=2100000kHz limit_rows=28 authority=direct_in_window_policy_limit`，却把 `558000 < 2100000` 推成“低负载自然降频，因此无策略限制”。这是“policy ceiling 是否存在”和“该 ceiling 是否命中/形成性能影响”两个结论轴未分型；direct limit 行证明前者 present，但后者必须另取 overlap/compute-supply。答案还写出 `2.075GHz 高于 2.34GHz` 的反向比较。T2 应增加 typed 两轴状态并在软指导中要求统一单位复算，不扫描模型正文做 hard gate。
- Trace runner 的首个 FAIL reason 是 E2 假阴性：principal 用 `Running/Runnable/S_Sleep` 合法大小写，oracle 只接受全小写。该处改成闭集大小写等价；同时新增“双轴说明” principal oracle，避免只出现 2.10GHz 数值却继续下错误总判。
- Trace 效率 P2：本轮 typed 值已足够，explorer 仍因 source-evidence floor 追加 1 次 raw `read_file`、3 次 `event_search`/trace_query、2 次 `emit_evidence`、共 3 次 `emit_investigation_complete`，wall 从 138s 波动到 314s。修向是让 runtime-artifact deterministic query observation 成为外部证据闭包的一等 typed source，不是给本 case/PID 加工具禁令；先排在 correctness 后。
- Data 正确面：D1 真实覆盖，`R2` 不再触发 unknown_rule_ref；一次 reference grounding repair 后 terminal 为 complete，最终 `17,0,5`，targets `T1/T2/T3`、zero-fill 和 reconcile pass 全正确，wall 从 240s 降到 154s。
- Data 审计 gap D2：有效且已映射的源行是 r1/r2/r4/r5；本轮 DAG 先把 `active_observations` 与 targets 做内连接，再在 `active_with_targets` 上 compute contributions，导致非目标组 GroupB 的 r4=4 在贡献账前被删。最终投影本来就只应输出 T1/T2/T3，但 contribution ledger 应先覆盖全部 included source records；reference projection 不能反向改写计算种群。

第二轮新增 GAP：

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B2-T2 | P0 | Trace 频率语义权限 | direct policy-limit 行只携带数值，没有把 `ceiling present` 与 `binding impact` 拆成两个 typed 结论；模型用实际频率低于 ceiling 反推“无策略限制” | authority 增加 `policy_limit_status=present` 与 `binding_caliber=...requires_separate_overlap_or_supply_evidence`；summary/finalizer/caveat 同源强调低于 ceiling 既不能否定 policy limit，也不能单独证明 binding；频率比较统一单位复算。只做 typed 软指导，不扫答案 hard gate | 已施工；待回放 |
| EVAL-B2-E2 | P2 | Eval 表达等价 | principal 四态 oracle 对 `Running`/`running` 大小写敏感，正确答案假 FAIL | 闭集接受英文状态首字母大小写；精确数值、状态关系、Σ 与 principal 作用域不放宽 | 已施工；待回放 |
| EVAL-B2-D2 | P1 | Data 计算/投影边界 | reference targets 内连接被提前到 compute contributions 之前，非目标但有效源行从审计 ledger 消失 | planner 把 contribution population 与 final reference projection 明确为两个 typed DAG stage：先按业务 qualification/filter 的 included source population 在原生 group-key domain 计算和对账，reference 只由 assemble_answer 做顺序/零填充投影；若“不在 reference”确为业务排除规则，必须先用 qualify_records 发布 typed exclude。由于是否贡献属于业务语义、无法从 join 结构精确推断，本批不加 fail-closed completion 硬门；eval 对本 fixture 精确要求 `contributions=4` | 已施工；待回放 |
| EVAL-B2-P2 | P2 | Runtime 证据闭包/效率 | deterministic trace_query 已有 runtime typed authority，explorer 仍绕到 raw file/current-source evidence floor，产生重复 completion 与查询 | runtime-artifact query observation 成为外部证据闭包的一等 typed source；不得伪造成 current-source citation；保留显式窗因果补采与必要 trace_query | correctness 后施工 |

第三轮施工顺序：

1. `B2-TS/P0`：T2 typed 双轴语义与统一单位软指导；独立提交推送。
2. `B2-DC/P1`：D2 贡献种群和 final reference projection 解耦；独立提交推送。
3. `B2-E2/P2` 与 T2 case 契约同批，仅改 eval 等价面；runner 契约后提交。
4. 重建并仍严格并行 2 回放 B2；T2/D2 correctness 收口后再判断 P2 是否稳定施工，避免把单轮 latency 波动硬编码进产品。

第三次修复后回放（runner 快照 `main@6c679a764530`）：

- 结果目录：
  - `eval/results/real_trace_h4_supply_thermal_witness-20260731-020828`
  - `eval/results/data_multifile_reference_projection-20260731-020828`
- 严格 `parallel=2`；runner 为 Trace FAIL、Data PASS；人工同判。
- Data D2 已真实覆盖：贡献账精确包含 r1/r2/r4/r5 四条，reconcile 在最终投影前保留 `GroupA=17,GroupB=4,GroupC=5`；targets reference 只负责最后顺序与 GroupX 零填充，答案为 `17,0,5`。D1/D2 correctness 收口。8 个 data rounds、一次过早跨 DAG rank 的 compute staging rejection 和两次 answer projection 形态收敛使 wall=278s，登记效率 P2，不为该 fixture 增加动作特判。
- Trace 四态与 direct limit 值正确，trace_query 从上轮 6 次降到 4 次、wall=148s；T1 值载体与 E2 大小写均已覆盖。T2 软权限已进入 finalizer prompt，但模型仍把 ceiling presence 越权写成 thermal binding impact，属于连续两轮可见的模型服从性问题；在禁止扫描答案原文做生产硬门的约束下，不能用关键词否定器处理。
- 本轮新 P0 并非 T2 措辞本身：system supplement 明确补跑 root_cause_rank 318 条、critical_blocking_calls 38 条，却没有发布 `Trace 因果投影`，频率权限 caveat 也同时缺席。代码根因是共享 `IsFocusedRuntimeFactQuestion` gate：本轮 analyzer 把同一显式窗请求标为 non-diagnostic explain/mechanism，于是所有 full-report materializer 被抑制；上轮 diagnostic 标签不同则投影存在。显式窗能力不应依赖这个软标签波动。

第三轮新增/更新 GAP：

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B2-T3 | P0 | 显式窗报告权限 | 精确显式窗同时被 typed focused-fact materialization gate 归为轻量状态事实；同一请求仅因 analyzer diagnostic 标签波动，就随机抑制 causal projection、frequency authority caveat 等全部 full-report surface | `RuntimeArtifactScopeProfile.ExplicitTimeWindow()` 是已校验的精确信号（起止值 + 当前请求 exact source quote）。只在共享 answer-materializer gate 恢复已有证据的系统输出；family 与 supplement 选择不变，避免轻量状态窗额外触发根因查询。非法/无锚窗口 fail-closed，无窗状态查询继续轻量。不得扫描用户关键词或模型答案 | 已施工；待回放 |
| EVAL-B2-T2 | P1 | Trace 频率语义权限 | typed 双轴软指导已送达但模型连续两轮选择相反强结论 | 保留 typed authority 与确定性频率 caveat；T3 恢复 caveat 出厂后，以系统权限块明确覆盖证据口径。除非建立新的 typed claim schema，不以正文关键词硬拒绝；后续若仅模型波动则降级遗留 | partial；待 T3 回放 |
| EVAL-B2-D2 | P1 | Data 计算/投影边界 | reference 提前缩窄 contribution population | contribution/reconcile 使用全部 typed included source，assemble_answer 才做 complete-reference projection；fixture 精确要求 4 条贡献 | covered |
| EVAL-B2-P3 | P2 | Data DAG 效率 | correctness 已闭合，但 8 rounds 中仍有跨 rank staging rejection 与重复 answer projection 收敛 | 从 typed workflow state/action scaffold 提升合法 next-rank 一次命中率；不按 GroupA/targets 或 case 类型写动作捷径 | filed；B2 correctness 后与 P2 一并排序 |

`B2-T3/P0` 施工说明：共享 `runtimeTraceFullReportMaterializationAllowed` 现在先读取 `RuntimeArtifactScopeProfile.ExplicitTimeWindow()`；只有该方法验证出正窗口、双时间值和非空 current-request source quote 时，才允许全部 runtime system materializer 发布已经收集的 bounded evidence。`IsFocusedRuntimeFactQuestion`、question family 与 deterministic supplement view 选择保持原样，因此轻量窗口状态事实仍只做最小 census，不会因本修复新增 root-cause/blocking 查询。无窗和无锚显式窗负例仍被抑制；实现未读取 raw request、keywords、case ID 或 answer prose，也没有修改 trace query、因果构造、自动补采或正文 hard gate。

第四次回放（runner 快照 `main@79e5d0215827`）：

- 结果目录：
  - `eval/results/real_trace_h4_supply_thermal_witness-20260731-022537`
  - `eval/results/data_multifile_reference_projection-20260731-022537`
- 严格 `parallel=2`；runner 为 Trace FAIL、Data PASS，人工同判。
- T3 已真实覆盖：Trace 最终答案重新发布完整 `Trace 因果投影`，runner 计数 `trace_query_final_projection_blocks=2`；同一个 shared materializer gate 也恢复 frequency authority caveat。自动补采没有因修复膨胀，模型 trace_query 从 4 降到 3，包含一次 windowed root_cause_rank；wall=131s。故 T3 从“待回放”转 covered。
- T2 正文仍失败：模型把 policy ceiling presence、实际频率低于 ceiling、compute-supply 折算与 thermal binding 混成一个强结论，并把 CPU0/CPU4/CPU12 的 limit count/身份写乱。deterministic caveat 则正确发布 CPU0 `1530000/16/line8048`、CPU4 `2100000/28/line17113` 和 `binding_impact_requires_separate_overlap_or_supply_evidence`。这证明值链与系统权限链已正确，剩余是模型没有遵守软权限；在禁止扫描 answer prose 做 hard gate 的前提下，本战役不再为该 case 增加关键词否定器。未来只在建立通用 typed claim schema/系统拥有结论席位时处理。
- Data D1/D2 连续覆盖：`contributions=4`、reconcile pass、final=`17,0,5`。本轮 12 rounds/3 repair，首次 assemble_answer 未携 complete-reference 参数，且出现同批 producer→consumer 跨 rank；typed completion 最终修正。属于 workflow efficiency P2，不影响 correctness。

B2 收口状态：

| ID | 状态 | 收口证据/后续 |
|---|---|---|
| EVAL-B2-T1/E1/E2/T3 | covered | direct limit roster、principal 作用域、大小写等价、显式窗投影与 caveat 全部真实回放；自动补齐无回退 |
| EVAL-B2-D1/D2 | covered | ordinal alias 与 4 条 included contribution 连续回放，reference projection 正确 |
| EVAL-B2-T2 | deferred-model/schema | typed 系统权限正确，模型正文仍越权；禁止用用户/答案关键词 hard gate，等待泛化 typed claim schema |
| EVAL-B2-P2/P3 | filed P2 | Trace 查询已降至 3；Data DAG 仍有 8~12 rounds 和 repair 波动，后续与跨 case workflow efficiency 一并处理 |

因此 B2 correctness 战役停止继续拟合该单例，转入优先级表的下一组；T2/P2/P3 保留在统一台账，不虚报消失。

### B3：日志观察 × 当前源码双权威 / 超大日志长尾

- cases：`read_combo_log_current_code_boundary`、`logtri_oversized`
- 首轮快照：`main@fe1ae698c`
- 结果目录：
  - `eval/results/read_combo_log_current_code_boundary-20260731-023047`
  - `eval/results/logtri_oversized-20260731-023047`
- 严格 `parallel=2`；runner 为 1 PASS / 1 FAIL；人工为一项 FAIL、一项 pass-with-efficiency-gap。
- `read_combo_log_current_code_boundary`：turn router 已正确给出 `route=repo / source=artifact / needs_repo=true`，并在 rationale 中明确需要当前源码。analyzer 随后把“不要把日志行当成当前源码引用”和“请结合当前源码说明……”两段同时装入 `external_observation_policy.source_quotes[]`，再发出 `current_source_mode=exclude / exclusion_kind=explicit_user_exclusion`；`current_source_explanation_profile` 同时缺席。当前实现只检查 quote 是否逐字来自请求，不区分 quote 的 typed 作用域，因此错误 exclusion 合法化并静默压掉源码 lane。最终 `read_file=0 / repo_map=0`，正文只复述日志的 timeout 与 rewrite，无法用当前实现区分 LLM transport timeout 和 answer validation failure。
- 该根因不是“没匹配到结合当前源码”这种词面问题。wire schema 把两种正交权限共用 `source_quotes[]`：一类证明“禁止读取/分析当前源码”，另一类只证明“附件行号仍是 external-observation citation”。最优修复是在 analyzer emit schema 分离 `current_source_exclusion_quote` 与 `artifact_citation_quotes[]`；只有前者可铸 active exclusion，后者永远不能关闭源码 lane。旧 `source_quotes[]` 仅作兼容/审计，不能再在 analyzer mint 路径单独证明 exclusion。下游仍只读 typed policy，不扫描 raw request、rationale 或 answer prose。
- `logtri_oversized`：两段式预处理正确找到 `main.crashy()` / `internal/agent/analyzer.go:100`，最终也披露该 frame 与当前 checkout 不一致，人工 correctness 可接受。但预处理为 byte segment 估算反复读取，主探索又用 8 轮、10 次源码读取验证合成 fixture，wall=286s。它代表“运行时定位已足够”和“是否还需当前源码核验”没有按 typed obligation 分层，以及 byte/line segment 仍依赖模型估算的 P2 效率债；当前不为 40KB、panic、Go 或该 fixture 加次数硬门。

B3 新增 GAP：

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B3-S1 | P0 | Analyzer 权限证明 | source exclusion 与 artifact citation identity 共用 `source_quotes[]`；引用边界可被模型错铸成“禁止当前源码”，使 mixed runtime+source 请求退化为 observation-only | wire schema 拆分 exclusion proof 与 citation proof；active exclude 必须有单一、独立、current-request anchored 的 `current_source_exclusion_quote`。`artifact_citation_quotes[]` 只证明外部引用身份；legacy combined quotes 不再能在 mint 路径独立关闭源码 lane。保留真正“只分析日志/trace，不读代码”的负例 | 已施工；待回放 |
| EVAL-B3-S2 | P1 | Mixed-lane 完整性 | analyzer 可同时遗漏 `current_source_explanation_profile` 并错发 exclude；现有 current-source normalizer 只有 profile 已在场时才会纠冲突 | S1 先阻止错误 exclusion；typed route `needs_repo=true` 现有 allow synthesis 随后恢复 required source lane。若未来仍有 profile 遗漏，再以独立 typed carrier/schema 处理，不从原文补铸 | 待 S1 回放 |
| EVAL-B3-P1 | P2 | 日志分段效率 | 两段式 log triage 让模型估算 byte boundaries，出现多轮读取/中段确认；确定性附件索引已知 size/line，却没有直接提供 segment 坐标 | 由附件读取层提供确定性 byte↔line 索引和候选关键区间，模型只做区域语义分类；保持全量 coverage 与 unknown-chunk fail-loud | filed |
| EVAL-B3-P2 | P2 | Runtime/source 探索成本 | artifact 栈顶已回答定位后，仍无条件进入宽 current-source 验证；合成/旧构建 mismatch 导致 8 轮、10 reads | 以 typed current-source obligation 和 runtime/source alignment status 决定 required/optional lane；optional 核验有 bounded closure，精确 mixed 请求不得被削弱 | filed |

B3 施工顺序：

1. `B3-S/P0`：先做 S1 schema 权限分载体与 mint-time 归一化；原 witness、真实源码排除、citation-only、unanchored quote、current-source profile 冲突五类回归同时固定。
2. 重建后仍严格并行 2 回放 B3；第一例必须真实产生源码工具证据，第二例只观察成本波动，不为其添加工具禁令。
3. S2 只在 S1 回放仍出现 source lane 遗失时独立施工。P1/P2 与跨 case 的 runtime 证据闭包/效率债合并排序，避免按 oversized 单例过拟合。

`B3-S/P0`（S1 权限证明分载体）验证：`emit_analysis` schema、analysis skill 和显式 runtime-path shortcut 统一使用 `current_source_exclusion_quote` 与 `artifact_citation_quotes[]`。新铸造的 active exclude 必须同时具备 `current_source_mode=exclude + exclusion_kind=explicit_user_exclusion +` 一个逐字锚定的 exclusion 专用 quote；citation quote 和旧 role-less `source_quotes[]` 仍可进入审计引用集合，但不能单独关闭源码 lane。B3 原 witness 的两个混合 legacy quotes 在 typed route `needs_repo=true` 下被确定性降为 `allow`，artifact citation identity 保留，current-source lane 恢复 required。真正的中文 `不分析代码`、显式 route 冲突时用户 exclusion 优先、unanchored/paraphrase fail-open、current-source profile 对冲、missing exclusion kind repair 均有负/正回归。实现不扫描 raw request 的关键词或模型答案，只对 LLM 已选择并逐字引用的 schema 字段做 provenance 验证。`go test ./internal/tool ./internal/agent ./internal/skill -count=1` 全包通过（tool 157.761s、agent 3.116s、skill 1.606s）。

B3 第二次回放（runner 快照 `main@ef8d8b33d`）：

- 结果目录：
  - `eval/results/read_combo_log_current_code_boundary-20260731-025051`
  - `eval/results/logtri_oversized-20260731-025051`
- 严格 `parallel=2`；runner 为 2/2 PASS，人工为 1 PASS / 1 FAIL。第二例是自动 false PASS。
- `read_combo_log_current_code_boundary` 真实覆盖 S1：analyzer 不再关闭源码 lane，探索读取当前实现，最终引用 `internal/render/status_messages.go:233`、`internal/orchestrator/write_retry_helpers.go:134`、`internal/agent/finalizer.go:1`，并把运行时 `first_byte_timeout / stream timeout` 与 contract/semantic validation 分开。S1 covered；S2 在该 witness 不再需要额外从原文补铸。
- `logtri_oversized` 的预处理已明确给出 runtime stack top `main.crashy()` at external source line 100，caller 为 `main.main()` line 200；最终答案却因当前 checkout 没有同名生产函数而声称“panic 来自外部程序，无法定位具体发出位置”。checkout mismatch 只能限制“无法映射到当前源码”，不能抹掉运行时已经直接观察到的发出函数。6 次 read、1 次 repo_map、7 explorer rounds 也说明错误的 current-source obligation 同时制造 correctness 和效率损失。
- 根因是两种 typed 语义共用 `NeedsRepoAccess`：它在 dispatcher 中表示“必须进入 repo/read pipeline”，所以附件独立 log/trace 也必须为 true；runtime/source authority 却把它解释为“当前 checkout 证据必须参与答案”。同一个 bool 无法表达“走 pipeline 但源码可选”，导致 artifact-only observation 被强制源码核验并反向否定运行时事实。

B3 第二轮新增/更新 GAP：

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B3-S1 | P0 | Analyzer 权限证明 | citation proof 可错铸 source exclusion | exclusion/citation 分载体，legacy role-less quote 不得独立 mint exclude | covered |
| EVAL-B3-S2 | P1 | Mixed-lane 完整性 | mixed 请求可能遗漏源码 profile | S1 后 route-required allow synthesis 已恢复本 witness；若未来 typed profile 仍遗漏再独立立案，不从原文扫描补铸 | covered-by-existing-route synthesis |
| EVAL-B3-S3 | P0 | 路由执行权限 / 证据义务 | `NeedsRepoAccess` 同时承担 pipeline admission 与 current-source obligation；artifact-only runtime fact 被 checkout mismatch 覆盖 | 新增闭集 `current_source_evidence_mode=required|optional`，与 `needs_repo_access` 正交；route classifier 必填，TurnRouteHint 显式携带，analyzer prescan、emit-analysis source policy、runtime/source authority 只消费该 enum。legacy unspecified 保留旧 fallback。不得扫描 request/answer prose | 已施工；待回放 |
| EVAL-B3-E1 | P2 | Eval principal oracle | `logtri_oversized` 仅检查附件处理，错误主结论仍 PASS | 增加 principal runtime-origin oracle：保留直接观察的 stack top `main.crashy()`；源码 mismatch 只作 mapping caveat。只改 eval，不作产品正文关键词 hard gate | filed |
| EVAL-B3-P1 | P2 | 日志分段效率 | byte boundary 仍由模型估算 | 确定性 byte↔line 索引与候选区间 | filed |

`B3-S3/P0` 施工约束：

1. `needs_repo_access` 继续严格表示 route/pipeline admission，不改 dispatcher、不把 artifact-only 退回 local/operation/data。
2. `current_source_evidence_mode=required` 保留普通源码问题、显式 runtime+current-source correlation；`optional` 只解除当前 checkout 的答案义务，不削弱 runtime artifact、显式时间窗 Trace 因果投影或自动补采。
3. 所有生产决策只读 schema enum 和既有 typed RequestModel/ledger；不匹配中英文用户关键词，不扫描模型最终答案。
4. 旧 persisted hint、测试 stub、第三方 adapter 未携字段时继续 fallback 到 `NeedsRepoAccess`，避免兼容性静默倒退。
5. 回归必须同时覆盖 optional artifact、required mixed/source、explicit source exclusion、legacy fallback，以及 analyzer prescan/emit-analysis/runtime authority 三个消费层。

`B3-S3/P0` 施工验证：route classifier schema 新增必填闭集 `current_source_evidence_mode=required|optional`，并在 prompt 中明确它与 `needs_repo_access` 正交；`TurnPolicy → TurnRouteHint` 原样投影。新生产 hint 下，analyzer prescan、`emit_analysis` 的 source allow/exclude 归一化、`RuntimeSourceAnswerAuthority` 统一消费 `RequiresCurrentSourceEvidence()`；artifact-only 仍为 `route=repo / needs_repo_access=true`，但不会 synthesize current-source allow，也不会让 checkout mismatch 夺走 runtime authority。mixed/current-source 为 required，继续保留源码预扫描与证据义务；explicit exclusion 仍可覆盖 required route。legacy unspecified hint 继续回退 `NeedsRepoAccess`，不破坏持久化和 adapter 兼容。路由日志新增 `current_source=<required|optional>` 便于回放核查。定向测试与 `go test ./internal/types ./internal/repl ./internal/agent ./internal/tool ./cmd -count=1` 全部通过（types 20.865s、repl 39.199s、agent 2.770s、tool 166.118s、cmd 9.837s）。实现未触及 trace query、显式时间窗 full-report gate、因果投影构造、自动补采选择或 answer prose hard gate。

B3 第三次回放（runner 快照 `main@5bfa3a04a`）：

- 结果目录：
  - `eval/results/read_combo_log_current_code_boundary-20260731-030602`
  - `eval/results/logtri_oversized-20260731-030602`
- 严格 `parallel=2`；runner 1 PASS / 1 FAIL；人工为 mixed FAIL、oversized pass-with-efficiency-gap。
- 两例路由日志均真实显示 `needs_repo=true current_source=optional source=artifact`，证明新字段从 classifier 到单次 CLI route hint 的生产接线生效。
- mixed case 的 classifier reason 与 analyzer thinking 都明确说需要“日志 + 源码”，但 classifier enum 仍为 optional，analyzer 也没有发 `current_source_explanation_profile`；最终 0 source reads，并明确声明“未读取当前代码仓库源码”。这是两个 typed producer 同轮遗漏后的模型波动，不能从 reason/thinking 或用户词面回填 hard authority。
- oversized case 的 principal 已恢复 runtime stack top `main.crashy`、caller `main.main`，checkout mismatch 只限制当前源码映射，S3 的 correctness 方向覆盖。但 analyzer 把 artifact phrase “这个大日志里的 panic”写进 repo path `source_scope_profile(all)`；`RequestModel.CurrentSourceLaneDecision()` 又通过 `sourceScopeHasCurrentRequestAnchor()` 将 lane 升为 precise required，完全绕过 route optional。随后 12 reads、1 repo_map、2 list_files、4 completion、296s，并引入与 panic 无关的 `sealedTraceStreamerDBOutputs.Size`。

B3 第三轮新增/更新 GAP：

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B3-S3A | P0 | Authority 单一入口 | analyzer prescan/allow synthesis 已消费新 enum，但 `AssessExternalObservationSufficiency`、`CurrentSourceLaneDecision` 仍可由 RM allow/source-scope 旁路重新铸 required | 在统一 runtime/source authority 编译器内合并 route+RM：显式 route optional 下，只有 independent typed current-source proof 可升级，再由既有 precision 编译器区分 hard/soft；generic allow/default、source-scope quote、diagnostic prose flag不能单独越权。route required、legacy unspecified、explicit exclude 保持语义 | 已施工；待回放 |
| EVAL-B3-S3B | P1 | Source-scope 权限 | repo path `source_scope_profile` 只凭任意 exact request quote 就被当 current-source anchor；artifact scope quote 可错铸源码义务 | route optional 时 source-scope 不得独立升级；未来若需单独 hard authority，新增专用 current-source scope proof，而非扫描 quote 关键词。schema prompt 明确 artifact scope 必须放 `runtime_artifact_scope_profile` | filed-with-S3A |
| EVAL-B3-S4 | P1 | Typed producer 稳定性 | mixed case 的 route reason/analyzer thinking 知道需要源码，但两个正式 carrier 同时遗漏 | 强化 route schema：mixed/current-source correlation 必须 required；强化 analyzer：不能以 source_scope/current_risk/allow 代替 `current_source_explanation_profile`。只做结构化 prompt/typed enum，不消费 reason、thinking、用户/答案关键词。若复放继续单次波动则保留，不上正文 hard gate | 施工中（软） |
| EVAL-B3-P1 | P2 | 日志分段/收敛效率 | oversized 仍 296s，12 reads、4 completion，且 byte segment 由模型估算 | S3A 先去掉错误 source lane；剩余成本再归入确定性 byte↔line 索引与 typed closure，不按 panic/Go/40KB 特判 | filed |

`B3-S3A/P0` 施工不变量：

1. route optional 只对 runtime/external-observation current-source obligation 生效；普通当前源码问题不走该 carve。
2. dedicated `current_source_explanation_profile` 精确 quote、resolved current files、精确 source path/line target、required current-code dimension仍可独立恢复 required，避免 classifier 一次误判破坏 mixed 能力。
3. route required 与 legacy unspecified 保留现有 required/fallback；explicit source exclusion 最高优先。
4. `BuildRuntimeSourceAnswerAuthoritySnapshot`、request-only precision、external sufficiency必须共用同一 request-side helper，禁止再出现不同消费者各自解释。
5. 不改 Trace 显式窗、因果投影、自动补采，也不新增 request/answer prose 扫描 hard gate。

`B3-S3A/P0` 施工验证：`runtimeSourceAuthorityRequestCurrentSourceRequired` 成为 request-side 单一编译入口，`AssessExternalObservationSufficiency`、ledger-backed snapshot 与 request-only precision 共用该入口。显式 route optional 的 external-observation turn 只允许下列独立 typed proof 恢复 source obligation：专用 current-source explanation profile 的 verbatim request quote、mint-certified dropped obligation、带精确 code/path anchor 的 required dimension/target/file hint、或 source-scope 中的精确 path/line quote；generic `allow/default`、diagnostic flags、任意非空 source-scope quote不能越权。软 obligation 仍允许 runtime sufficiency + caveat，只有 precise obligation 阻断 external sufficiency。route required、legacy unspecified 与 explicit exclude 保持原语义。新增真实形状回归固定 `optional + allow + source_scope(all, quote="这个大日志里的 panic")` 不重开 source，以及 `optional + current_source_explanation_profile(source_quote="结合当前源码")` 恢复 soft mixed lane。结构化 prompt 同步明确 artifact scope 只能进 `runtime_artifact_scope_profile`，mixed request 必须使用 dedicated profile；没有读取 route reason/thinking、用户关键词或答案正文。`go test ./internal/types ./internal/repl ./internal/skill ./internal/agent ./internal/tool ./cmd -count=1` 全部通过（types 20.048s、repl 33.182s、skill 1.845s、agent 3.539s、tool 159.622s、cmd 7.573s）。

B3 第四次回放（runner 快照 `main@cfa7feb70`）：

- 结果目录：
  - `eval/results/read_combo_log_current_code_boundary-20260731-032215`
  - `eval/results/logtri_oversized-20260731-032215`
- 严格 `parallel=2`；runner 2/2 PASS，人工为 mixed FAIL、oversized partial。自动 PASS 均没有覆盖本轮暴露的生产语义/来源边界。
- mixed 的 typed route 已变为 `needs_repo=true current_source=required source=mixed`，analyzer 发出两个 current-source explanation，三次源码读取完成，证明 S3A/S4 的结构化恢复真实接线。但答案把 dock 的 `4/4` 解释成 finalizer 的“第 4 次相同错误重试/上限”。代码生产链相反：`renderer_dock.go::stageProgressForFocus` 明确规定 read 四阶段 `finalize=4/4`；`finalizerIdenticalErrorStreak=4` 只是同错误类循环 breaker；`status_messages.go` 只提供重试状态短语。相同数字不能证明同一语义。答案还把 `first_byte_timeout` 直接归因网络层，超出了“上游在期限内没有产生 SSE 字节”的可观察边界。
- oversized 的 principal 已正确保留 `main.crashy → main.main`，route 为 `current_source=optional`，wall 从上一轮 296s 降至 161s；S3/S3A correctness 方向覆盖。但 preflight 在运行入口已持有 `kind=log source=eval/fixtures/oversized_log.txt`，后续对同一路径的 `read_file`、`grep` 和 `emit_evidence` 仍全部被 ledger 铸为 `origin=current_source`。因为附件恰好位于 repo 内，artifact 行被洗成源码 citation，并把 source record count 从 0 变成 7。这是 provenance 边界错误，不是答案措辞波动。

B3 第四轮新增/更新 GAP：

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B3-S5 | P1 | 组合状态语义 | 模型把同一可见状态行的 `K/N` 进度前缀与本轮 retry payload 当作同一生产者，再用仓库中碰巧相同的数值常量建立错误因果关系 | 用 producer-chain 软指导约束机械解释：装饰层 ordinal、payload lookup、agent retry policy 必须分别追踪；数值相等不能替代调用/赋值关系。若需要机器消费，后续增 typed `progress_ordinal` / `retry_attempt` 载体；不扫描答案正文做硬门 | covered；r5 发现 intent 适用面过窄，已改由 typed mechanism gate |
| EVAL-B3-S6 | P0 | Evidence origin 洗白 | observation ledger 默认把 repo-relative `read_file/grep/emit_evidence` 路径视为 current source，没有与 run-entry typed runtime artifact identity 做精确对照 | 将 `RuntimeArtifactPreflight + RepoRoot` 投影进统一 ledger input；在单一 add 铸点用规范化后的精确 path identity 把同一附件的 current-source/negative-search 记录重资格为 runtime artifact，并保留 artifact kind/id。覆盖所有 producer；不按 `.txt`、目录、题面或答案关键词猜测 | covered |
| EVAL-B3-P1 | P2 | 日志分段/收敛效率 | typed route 已降成本，但 40KiB 日志仍有 7 reads、10 explorer rounds、3 completion | S6 先恢复正确来源/闭包；之后建立 deterministic byte↔line index 和候选区间，按 artifact 通用能力优化，不按 panic/Go fixture 特判 | filed |

`B3-S6/P0` 施工不变量：

1. 匹配只使用 run-entry preflight 的 typed artifact source 与 repo-root canonical path，不读取 raw request、model thinking、evidence summary 或 final answer。
2. 只有精确同一 artifact identity 才重资格；同目录、同扩展名、相似 basename 均不能命中。
3. runtime artifact 位于 repo 内不改变其来源；current source 文件位于 artifact 附近也不被降级。
4. 单一 ledger add 铸点覆盖 tool carrier、model evidence、aggregate binding 和未来 producer，避免逐工具补丁。
5. 显式时间窗 Trace query、因果投影集合、系统自动补采及其 typed runtime records保持不变。

`B3-S6/P0` 施工验证：`ObservationLedgerInput` 新增 run-entry `RuntimeArtifactPreflight + RepoRoot`，Agent/Bus 两个标准投影入口均接线；单一 `add` 铸点在任何 producer 记录入账前，用 repo-relative canonical path 与 preflight source 做精确 identity 对照。同一附件经 `read_file`、`grep`、`emit_evidence` 或 aggregate current-source binding 回流时统一重资格为 `origin/kind=runtime_artifact`，补齐稳定 artifact id/kind，同时保留生产者原始 grounding policy、span、raw/payload ref 和负查结果。邻近 `.txt`、同目录/同扩展文件、无 preflight legacy 调用继续保持 current source。专项测试覆盖绝对/相对同一身份、三类 producer、邻居负例与无 preflight 兼容；`go test ./internal/types ./internal/tool -count=1` 全包通过（types 20.696s、tool 154.392s），agent/orchestrator 全包亦通过。

防回归说明：初版曾在重资格时重算 grounding policy，跨层测试立即捕获 `trace_query` hard pair-atomic 行被软化、因果投影消失。最终实现只改变 origin/source identity，绝不覆盖 producer 的证据强度；多 trace 单边/双边投影专项全部通过。因此 S6 不影响显式时间窗因果投影、自动补采或多工件 partition。

同轮发现但不并入 S6 的基线测试债：

| ID | 优先级 | 类别 | 事实 | 处置 |
|---|---:|---|---|---|
| EVAL-INFRA-1 | P2 | 测试清册漂移 | `answer-document-skill` 当前已有 31 条 Tier-B workflow，`TestSkillTierAwareWorkflow_AnswerDocumentSkill_TierBCount` 仍硬编码 30，导致 `go test ./internal/context` 在本批改动外失败 | covered：确认第 31 条为 `TARGET WAIT OCCURRENCE AUTHORITY` 后补回显式清册；S5 加入后固定 32 条身份，不删除生产 skill、不放宽为仅下界 |

`B3-S5/P1` 初次施工验证：探索与成文共用单一 `MECHANICAL PRODUCER-CHAIN SEPARATION` Tier-B directive，初版仅在 typed `IntentExplain` 下发布。规则把 composed output 拆为 visible-fragment producer：外层 prefix/progress ordinal、localized/status payload、retry/loop policy分别追踪，只有 direct call/assignment/parameter flow/return value 才能建立控制关系；同数值、同 token、邻近常量和同一渲染行明确只作候选线索。它要求从实际 formatter/composer 向后追输入，缺链则披露边界。没有 hard gate、没有 request/answer keyword scan，也没有为 `4/4`、某文件或某常量写 case 特判。

`EVAL-INFRA-1` 同批收口：第 31 条实际是已落地但旧计数清册漏记的 `TARGET WAIT OCCURRENCE AUTHORITY`，不是无设计来源的漂移。测试清册补回该身份，再加入 S5 后显式固定为 32 条；因此没有删除生产 skill 或用放宽断言掩盖漂移，context 全包恢复通过。

B3 第五次回放（runner 快照 `main@324af9706`）：

- 结果目录：
  - `eval/results/read_combo_log_current_code_boundary-20260731-034425`
  - `eval/results/logtri_oversized-20260731-034425`
- 严格 `parallel=2`；runner 为 mixed FAIL、oversized PASS；人工为 mixed FAIL、oversized partial。
- mixed 的 route 与 analyzer 均正确：`current_source=required`、`question_kind=mechanism`、`current_source_explanation_profile={explain_current_mechanism,trace_current_flow}`。但 explorer 只发出三个 `grep(files_only)`，没有 `read_file`；这些正向 path-discovery 行把 runtime/source authority 从 `source=0,satisfied=false` 错改为 `source=3,satisfied=true`，于是系统放行无源码结案。最终错误地把 render 的 pipeline `4/4` 解释为 render 自己耗尽四次尝试；实际 formatter 的四阶段来自 `renderer_dock.go`，与 finalizer 的 retry budget 是两条生产链。这个失败同时证明 S5 初版的 `IntentExplain` gate 过窄：同一 typed mechanism 问题被 analyzer 合理归为 `IntentRootCause` 时，规则没有发布。
- oversized 已验证 S6：对 repo 内附件 `eval/fixtures/oversized_log.txt` 的三段 `read_file` 全部保持 runtime-artifact observation，final ledger 的 principal log row 也是 `origin=runtime_artifact`，没有再次被洗成 current source。主结论 `main.crashy → main.main` 与 checkout-mismatch 边界正确。残余是 11 reads/13 rounds、无关源码 absence 证据，以及 `main.main` 条目复用 line 643 的 `main.crashy` citation；列为通用 citation binding/效率债，不为 panic、Go 或该文件建立特判。

B3 第五轮新增/更新 GAP：

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B3-S5A | P1 | Typed 指令适用面 | `Intent` 是问题目的，`question_kind` 是答案证据形态；mechanism/root-cause 合法组合被 intent-only gate 漏掉 | Tier-B applicability 新增 `RequiresMechanism/HasMechanism`，唯一读取 `NormalizeRequirementKind(AnalyzerHints.Kind)==ReqMechanism`；explore/finalize 共用，不扫 raw request/answer | 已施工；待回放 |
| EVAL-B3-S7 | P0 | Current-source 证明权限 | 所有 `origin=current_source` ledger row 都直接满足 source lane，导致正向 grep/list/repo-map 导航等价于已读源码 | `CurrentSourceRecordCount` 继续保留导航遥测，但 `CurrentSourceSatisfied` 只接受带 source line/support ref 的 exact witness 或既有 deterministic negative-search valve。正向 path discovery 不再能完成 required lane | 已施工；待回放 |
| EVAL-B3-C1 | P2 | 外部 observation citation binding | ordered item `main.main(line 645)` 复用 `main.crashy(line 643)` citation，现有绑定门对 external artifact row 的逐项坐标不足 | 把 artifact-local frame/item identity 与 line/span 编成 typed support ref，逐 item 校验引用身份；不能用正文数字/函数名正则补门 | filed |
| EVAL-B3-P1 | P2 | 日志分段/收敛效率 | S6 已恢复来源，仍有 11 reads/13 rounds/3 completion | 建确定性 byte↔line index、typed candidate interval 与 runtime-only bounded closure；跨 case 证明 ROI 后施工 | filed |

`B3-S5A/P1 + S7/P0` 施工验证：Tier-B filter/context 新增 mechanism 布尔轴，值只来自已规范化的 analyzer `question_kind`，因此 `IntentRootCause + ReqMechanism` 会收到 producer-chain 指令，单纯 `IntentExplain` 不再被误当机制证明。runtime/source authority 则把“记录存在”和“证明可用”分开：positive path discovery 仍计入 source telemetry，但不再置 `CurrentSourceSatisfied`；addressable line/support-ref 证据与 canonical negative repo search valve保持原语义。专项覆盖 root-cause mechanism、非 mechanism、positive path discovery、真实 line witness 与 negative search；`go test ./internal/skill ./internal/context ./internal/types ./internal/agent -count=1` 全包通过（skill 0.722s、context 0.711s、types 22.384s、agent 2.642s）。未修改 trace materializer、显式窗口判定、因果投影或自动补采代码。

B3 第六次回放（runner 快照 `main@7608f9fe0`）：

- 结果目录：
  - `eval/results/read_combo_log_current_code_boundary-20260731-035428`
  - `eval/results/logtri_oversized-20260731-035428`
- 严格 `parallel=2`；runner 2/2 PASS，人工 mixed FAIL、oversized partial。S7 已真实覆盖：mixed 在 repo-map/正向 path discovery 后仍为 `current_source_satisfied=false`，直到 `read_file + emit_evidence` 产出源码行才变 true。S5A 也在 explore/finalize 两面真实出现，不是接线未生效。
- mixed 仍错误的更深根因在 pre-triage：`emit_log_triage.observations[2].evidence` 只有第 4 行原文，`summary` 却自行声称“4/4 表示第 4 轮答案生成（iter 从 0 开始）”以及“finalizer 失败传给 render”。该 summary 被 context 称为 structured fact、ledger 直接放进 principal `Summary + observed_direct_cause`，analyzer/explorer 在读取源码前已继承错误机制。最终答案虽然引用了 status payload 定义，却仍重复错误 ordinal，并把源码注释明确包含的 `gate-driven rejection` 说成“不触发重写”。这不是 S5A 适用面问题，而是 LLM pre-triage interpretation 被系统升级成 runtime fact。
- mixed 还暴露独立 P0：成文把 `orchestrator.go:4346` 的函数/quote 填到 `internal/render/status_messages.go:4346`；后者仅 984 行，仍被 quote repair 和文档 gate 接受并呈现。当前 citation 修复只看 quote token/已有 evidence 邻域，没有 fail-closed 核验 file total/range identity。
- oversized runtime 核心定位正确，S6 保持；read 从 11 降为 4，wall 154s。但两个外部 frame hop 都引用当前 `internal/agent/analyzer.go:1`，与正文“无法映射 checkout”矛盾，C1 仍开放。

B3 第六轮新增/更新 GAP：

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B3-S5A | P1 | Typed 指令适用面 | mechanism/root-cause 漏发 producer-chain 指令 | typed `RequiresMechanism` 已接线且 r6 实际发布；错误仍在说明该批已覆盖，不继续加软文案 | covered |
| EVAL-B3-S7 | P0 | Current-source 证明权限 | 正向导航伪装源码证明 | r6 在无 line witness 时保持 unsatisfied，真实行证据后才 satisfied | covered |
| EVAL-B3-S8 | P0 | Artifact 观察/推断权限 | LLM triager 的 `summary` 与 artifact `evidence` 没有权威分层，summary 中代码机制/计数语义被 ledger 当直接观察与 principal claim | 保留两者但反转权威：evidence+line 是 observed fact；summary 是 `triager_interpretation_advisory`，只作搜索假设。context、stage report、artifact profile 与 ledger 共用该边界；profile 首选 literal evidence，meta/summary 仅 fallback。triager prompt 禁止从视觉邻接推断 composed-output producer。不得扫描 summary 内容做关键词否定 | 已施工；待回放 |
| EVAL-B3-S9 | P0 | Citation file/range 身份 | quote repair 可把另一文件的真实 token 填到超出目标文件总行数的 citation，产生物理不存在的 `file:line` | citation 接受/repair 单一入口必须验证 canonical file 在已读/grounded范围且 line 落在文件总行数/读覆盖内；不允许跨文件 quote 搬运。失败时删除/拒绝该 citation，而不是补写邻近 package 行 | covered；待真实回放 |
| EVAL-B3-C1 | P1 | External artifact item binding | runtime frame item 被当前同名/同路径 checkout line citation 覆盖，尽管 frame drift 已 typed unverified | item 使用 artifact-local support ref/uncited external observation；只有 frame-current-source alignment proven 才可绑定 repo citation | filed；与 S9 分批 |

`B3-S8/P0` 施工验证：log-triage workflow/schema 明确 `evidence` 是 observed artifact text，`summary` 只是受该行约束的 triager interpretation；numeric prefix、progress ordinal、attempt counter、status payload 与邻行不再可由视觉布局推断生产者。下游 structured log context 把两者分别渲染为 `observed_evidence` 与 `triager_interpretation (advisory)`；stage report 同样显式标记。`ArtifactObservationProfile.SymptomSummary` 优先 error message/observation evidence，meta synopsis 只在无 literal carrier 时 fallback。ledger principal `Summary/RawExcerpt` 使用 evidence，原 summary 无损放入 `RichNotes: triager_interpretation_advisory=...`，因此不再占 observed fact 席位。实现不按 `4/4`、finalizer、语言或某条日志匹配，不扫描用户输入或成文；专项及 `go test ./internal/skill ./internal/context ./internal/types ./internal/agent ./internal/tool -count=1` 全包通过（skill 0.875s、context 0.783s、types 19.459s、agent 2.979s、tool 156.886s）。Trace 因果投影、显式窗口和自动补采未改。

`B3-S9/P0` 施工验证：新增结构化 citation 物理行界归一化，复用仓库根、敏感配置拒读、超大文件限界与 runtime-artifact identity 的既有单一读取边界。只有文件在仓库内、可安全完整读取且 `line..line_end` 明确超出实际行数时，才删除 pool entry；共享 drop helper 同步重映射存活引用并把指向无效 entry 的 item 置为无引用。附件引用、负向搜索、敏感/超大/不可读/仓库外文件保持原验证路径，不由本批猜测；尾部换行产生的 `strings.Split` 空哨兵不计作额外物理源码行。校验在初次 quote repair、pre-emit 修复链尾、pre-persist 和 degraded export 四个可能铸造 citation 的面统一接线，因此无法再把其他文件的真实 quote 挂到物理不存在的目标行。专项固定“有效引用重映射 + 两类越界引用删除 + artifact/negative/unreadable/outside 保持”以及 pre-emit 接线；`go test ./internal/tool -count=1`（156.297s）和 `go test ./internal/types ./internal/agent ./internal/orchestrator ./cmd -count=1`（18.304s/2.982s/12.374s/7.058s）通过。实现只消费 typed citation/file metadata，不扫描用户输入或模型答案正文；未改 Trace 窗口、因果投影、自动补采。

B3 第七次回放（runner 快照 `main@d3eae9980`）：

- 结果目录：
  - `eval/results/read_combo_log_current_code_boundary-20260731-041322`
  - `eval/results/logtri_oversized-20260731-041322`
- 严格 `parallel=2`；runner 2/2 PASS，人工 0/2 PASS。mixed 214s/5 reads/2 repo-map，oversized 237s/17 reads/1 list；自动 oracle 再次没有覆盖主结论的机制关系和 current-status 权限。
- S8 已真实接线：两例 prompt 均把 `observed_evidence` 与 `triager_interpretation (advisory)` 分开，ledger principal 不再直接消费 triager summary。S9 也覆盖：mixed 不再出现超过目标文件总行数的 citation；所有当前源码引用行物理存在。两项从“待回放”转为 covered。
- mixed 仍把 `finalizerIdenticalErrorStreak=4` 解释成 status 行 `4/4` 的来源，尽管 S5 producer-chain directive 在 explore/finalize 两面均明确禁止用相同数字、同一渲染行或邻近常量建边。答案拥有 phrase 定义、retry notice 与 streak 常量三组真实行，却没有 `renderer_dock.go` 中 ordinal 的 formatter/input 生产链；现有 mechanism floor 只要求若干 `definition_fact/call_edge` 候选，模型可用无 direct-flow 的真实行拼出错误关系。鉴于软指令已生效且继续加词面约束只会过拟合，本轮不再扩写 prompt；该项保留为通用 typed relation-proof 设计债。
- oversized 的 route 明确为 `needs_repo=true / current_source=optional / source=artifact`；analyzer 却把 artifact-only 请求原文「这个大日志里的 panic 从哪里发出」放入 `current_source_explanation_profile`，并自行发 `verify_current_status/locate_current_code`。当前 authority 把任何 verbatim profile quote 视为 independent source requirement，`zeroWitnessExplicitSourceDemandFatalClass` 又不检查 route optional，故 artifact-only 请求被升级成 precise current-source/current-status合同，触发 17 reads、146 source telemetry rows与 forced completion。最终 decision 仅引用 runtime artifact `main.crashy()`，却发 `current_status_verdict=fixed`，把“当前 checkout 找不到一个未验证历史构建的函数”扩成“已修复/Comparable risk 已消除”。这是结构化 authority GAP，不是单次措辞波动。
- C1 仍在：frame items 的 label 已带 artifact line 643/645，但 citation 分别指向 644/646；pre-emit 只发 soft advisory，且候选修复误列当前 checkout 的同名 `main`。该问题独立于 S9 的物理行界，继续 filed。

B3 第七轮新增/更新 GAP：

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B3-S8 | P0 | Artifact 观察/推断权限 | triager interpretation 被当 runtime fact | r7 prompt/context/ledger 均已分权；后续错误由模型重新拼接真实源码行产生，不再归因 S8 | covered |
| EVAL-B3-S9 | P0 | Citation file/range 身份 | 物理不存在 `file:line` 被接受 | r7 无越界引用；四面归一化真实生效 | covered |
| EVAL-B3-S10 | P0 | Route/analysis source authority | route optional 时，任意 verbatim current-source profile quote仍能独立重开 hard source/current-status lane；artifact scope 原文被 analyzer 错铸即可越权 | 显式 external-observation route optional 下，profile 仍保留为软探索指导，但不能独立 mint hard requirement；只有 route required 或 precise current-source target/path/line/required-dimension/obligation signal可升级。zero-witness fatal class必须同时要求统一 authority 已判 current-source required。route required mixed、legacy precise target、explicit exclusion保持 | covered；待真实回放 |
| EVAL-B3-R1 | P1 | Mechanism relation proof | 真实但无直接 flow 的 source lines 可满足 mechanism floor，模型借相同常量拼错 producer chain | 后续设计 typed relation witness：每个 principal call/control edge绑定 from/to + direct call/assignment/parameter/return evidence；缺链只能降为候选/边界。不得扫描成文关键词，也不为 `4/4` 特判 | filed；不继续软 prompt 拟合 |
| EVAL-B3-C1 | P1 | Artifact item/citation identity | artifact frame label 与相邻 stack line错配，修复候选跨到 checkout 同名 symbol | artifact-local support ref/line identity优先；unverified external frame禁止候选切换到 current-source同名 symbol，除非 alignment proven | filed |

`B3-S10/P0` 施工不变量：

1. route `current_source=required` 的 mixed current-source 请求仍可建立 source obligation；显式 source exclusion 继续最高优先。
2. route optional 只阻止 analyzer 的 generic/verbatim prose profile独立铸 hard authority；profile 仍保留在 IR/skill 中作软探索提示，不删除当前源码能力。
3. 精确 code/config path、`file:line`、required file/target/dimension、mint-certified obligation signal仍可在 route optional 下独立升级。
4. zero-witness completion门必须消费统一 runtime/source authority 的 `CurrentSourceRequired`，不能再旁路重解释同一 profile。
5. 不读取用户或答案原文做关键词 hard gate；不改 Trace 显式时间窗、因果投影、自动补采、trace query 或多工件 partition。

`B3-S10/P0` 施工验证：explicit external-observation route optional 的独立升级函数不再消费“任意 verbatim profile quote”，只接受既有精确 code/config path 或 `file:line`，以及原有 required target/file/dimension/obligation signal。`CurrentSourceExplanationProfile` 本身继续原样保存在 IR 和 skill 中作软探索指导；legacy 未携 route mode 的调用保持旧 fallback，route required mixed 与 explicit exclusion 不变。zero-witness fatal class新增统一 authority `CurrentSourceRequired` 前置条件，消除完成门对 profile 的第二套旁路解释。专项覆盖 optional+generic 不重开、optional+file:line 仍 precise hard-block、route-required generic fatal class保持、source witness打破 fatal，以及从 BusContext 到 AnswerSurfacePlan/AnswerSemanticView 的生产接线：artifact-only optional 形状不再发布 current-status contract/decision block。`go test ./internal/types ./internal/agent ./internal/orchestrator ./cmd -count=1`（18.576s/2.716s/11.917s/7.503s）与 `go test ./internal/tool -count=1`（156.989s）通过；显式用户窗、full-artifact覆盖窄模型窗、因果投影 materialization、frame supplement process scope 和 system-supplement authority 五项专项通过。实现只消费 route enum、profile enum/路径形状和 authority bool，不扫描用户输入或答案成文；Trace 查询、显式窗口、因果投影与自动补采未改。

B3 第八次回放（runner 快照 `main@2e877b061`）：

- 结果目录：
  - `eval/results/read_combo_log_current_code_boundary-20260731-042850`
  - `eval/results/logtri_oversized-20260731-042850`
- 严格 `parallel=2`；runner 2/2 PASS，人工 0/2 PASS。mixed 162s/4 reads/2 repo-map，oversized 202s/8 reads；相比 r7 有成本下降，但仍不能据此判 correctness。
- mixed 本轮正确定位 `4/4` 为 read pipeline 的 finalize 阶段 ordinal，不再借 `finalizerIdenticalErrorStreak=4` 解释该数字，证明这部分确有模型波动。但它又把“本次 finalizer retry notice”直接连到“重试耗尽/no-visible-output fallback → SkipAnswerChecks”，源码只证明 fallback 函数存在及其内部行为，没有证明日志第 4 行已经走到该 fallback；系统校验失败的固定文案区分也无 direct producer chain。R1 是同一通用关系证明缺口，不再追加 prompt。
- oversized 主结论从 r7 的错误 `fixed` 改为 `not_enough_evidence`，明确当前 checkout absence 不能证明外部历史路径已移除，这是 safer output。但本轮 route classifier 自身从此前 optional 漂为 `current_source=required`，reason 还自行加入“关联当前仓库源码”，所以 post-fix replay 没有命中 S10 的 explicit optional 分支；该分支只有确定性跨层测试覆盖，不能虚报本轮实证。由于 route enum 是正式 typed producer，系统不能在下游扫描用户原文反驳它；列为模型 producer 波动，暂不为单例增加 hard gate。
- oversized 的 C1 仍严重：artifact frame label `main.crashy` 被绑定到当前测试 fixture 的 `/src/main.go:42`，正文又称“位于外部 `internal/releaseartifact/cmd/verify/main.go:42`”，把 artifact-local frame、fixture 字符串和 checkout 同名 main 混为一体。主问“从哪里发出”虽答到 crashy，但具体 source identity 错，人工仍 fail。

B3 r8 收口裁定：

| 项目 | 裁定 |
|---|---|
| S8/S9 | covered，r7/r8 均无回归 |
| S10 | 代码与跨层 wiring covered；r8 因 route producer 漂为 required，真实 optional replay inconclusive，诚实保留该说明 |
| mixed producer chain | R1 filed；连续错误形态不同且本轮 ordinal 已自行恢复，禁止继续围绕单例加词面/常量硬门 |
| artifact frame citation | C1 保持 P1，归入后续“external artifact item identity”通用批，不按 Go/panic/crashy 特判 |
| B3 后续 | 停止继续重放同两例，进入更高覆盖面的下一组 eval；R1/C1 留统一台账，不宣称消失 |

### B4：Trace 碎片状态根因排序 × Write 值语义修复

- cases：`trace_query_state_churn_root_cause_rank`、`github_issue_gson_lazy_number_symptom`
- 首轮快照：`main@1cb238628`
- 结果目录：
  - `eval/results/trace_query_state_churn_root_cause_rank-20260731-043407`
  - `eval/results/github_issue_gson_lazy_number_symptom-20260731-043407`
- 严格 `parallel=2`；runner 2/2 PASS，人工为 Trace FAIL、Write PASS-with-control-gap，故 correctness 仅 1/2。

Trace 人工审计：

1. 显式 `11.000~11.008s` 窗完整保留；模型并行调用 `root_cause_rank + window_stats` 两个有界 view，最终系统仍发布完整 `Trace 因果投影`。本轮没有破坏显式时间窗、投影构造或已有自动补齐能力。
2. 引擎真值不是“state_churn 永不参与”。`window_stats.state_churn` 先铸出 `fragmented_runnable_wait=5.000ms`，进入 formal root-rank population；因为它与同线程正式 `runnable_wait=5.000ms` 是同一物理账，cross-type reconciler 将前者放入 `absorbed_items`，后者成为唯一 rank=1 席，携带 `rank_family_key=runnable_wait>fragmented_runnable_wait` 与 `absorbed_rank_rows=1`。这表示“碎片累计状态参与竞争，但同账只占一席”，不是 context-only。
3. `internal/tool/trace_query.go` 的工具合同连续存在相反规则：一条说 state_churn 让 dominant cumulative state 竞争主因，下一条又说 context-only、never takes a rank-board seat；后续 alias 说明也一正一反。模型读取源码后选择错误规则，把正确的 absorbed-family 语义写成“state_churn 是表因、永不入榜”。这是生产提示合同自相矛盾，不是模型随机措辞。
4. route 日志为 `source=artifact / needs_repo=true / current_source=optional`，Agent prompt 的 typed navigation 也显示 `current_source_requirement=none`；第一次 `emit_investigation_complete` 却声称 source lane required 并强迫 8 次源码读取。代码根因是 `types.ToolBusContext` 的 AgentContext→BusContext 投影没有复制 `TurnRouteHint`，也未把它加入 `projectionTypedSignalFields`。因此 S10 在类型层测试正确，但 completion 工具生产接线收到零值 hint，legacy fallback 又把 `NeedsRepoAccess` 当 source required；最终izer 用完整 AgentContext 再次显示 optional，形成同一轮 authority 前后跳变。
5. `Trace 指标快照` 同时发布同线程同状态账的 `19 次切换/20 段` 与 `20 次切换/21 段`。两个原始 trace_query payload 的 `WindowStats.StateChurn` 都只有 19/20，说明第二行来自另一个 derived deterministic observation face，而 snapshot selector 没有在同 subject/window/state-account 上优先采用 canonical `predicate=state_churn`。系统附注因此制造了不存在于原始查询的矛盾。
6. deterministic supplement 日志为 `no_typed_target`，但模型查询已按 `thread=app` 覆盖精确窗口并形成投影。本轮 correctness 未因此缺证；仍登记 typed target handoff P2，后续用跨 case 证据判断，不能为 `app-20` 单例加名称解析特判。

Write 人工审计：

1. applied tree 正确新增基于内部 `String value` 的 `equals` 与一致的 `hashCode`；`intValue/longValue/floatValue/doubleValue/toString` 均未改变，fixture source oracle 通过。补丁 correctness 判 PASS。
2. 第一次 verify 已真实调用 `run_tests {}` 并生成 green `ChangeReport`。系统随后创建 `batch-1-cumulative-review` verify-only 批，但没有清除旧 report；`verifierEvaluator.FilterToolSchemas` 见 report 非空后删除 `run_tests`，而 verifier prompt 仍要求再次调用它。模型的第二次调用于是变成 unavailable tool。
3. `ParseOutput` 随后直接读取旧 report，再次宣告 “all tests passed”，输出也重复两段“测试通过”。这会让未执行的复核伪装成新验证，属于验证真值/代际隔离 GAP；本轮恰好补丁正确不能降低其严重性。

B4 新增 GAP 与施工冻结：

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B4-A1 | P0 | Typed authority 生产接线 | `TurnRouteHint` 在 AgentContext→ToolBusContext 投影丢失；工具 hard gate 回退到 legacy `NeedsRepoAccess`，与 agent/finalizer authority 分叉 | 把 `TurnRouteHint` 纳入 canonical typed-signal projection，双向复制并用反射契约 + completion e2e pin 看护 optional/required；只消费 route enum，不扫描请求/答案 | covered，批 A |
| EVAL-B4-T1 | P0 | Trace 排序合同 | 同一工具描述同时宣告 state_churn 可竞争与永不入榜，覆盖了引擎的 absorbed-family 真语义 | 删除矛盾句，单一表述为“按 dominant typed caliber 参与；与正式状态同账时吸收为一席且诊断载体保留；running 无 supply deficit 仍 context-only”。同步 schema/description tests，不改引擎排序 | covered，批 B |
| EVAL-B4-T2 | P1 | Trace 系统快照 authority | canonical state_churn 与 derived record 同 subject/window/state totals 仍可各占快照槽并给出冲突 fragment/switch count | snapshot 候选按 typed predicate/scope 建 authority：whole-window canonical `state_churn` 优先；chain-episode 保留独立 scope；同账 derived row 不再并列。不得按最终文案去重 | covered，批 B |
| EVAL-B4-T3 | P2 | Trace typed target handoff | runtime_targets 在场但 supplement 报 `no_typed_target` | 审计 thread label→typed target carrier 的既有正规化；只有跨 case 复现才施工，禁止按 app/thread 名称特判 | filed |
| EVAL-B4-W1 | P0 | Write verification truth | verify-only follow-up继承旧 ChangeReport，动态工具面隐藏 run_tests，未执行复核却复用旧 green verdict | 进入 verify-only batch 的 StageVerify 前原子清除上一代 ChangeReport，保留 plan/context pack；新 verifier 必须产生本代 report，否则 fail-loud。e2e 预置旧 green report并断言 stage入口为空、run_tests 可调用 | covered，批 C |

B4 不变量：

1. 批 A 只修 typed route authority 传递；不得修改 Trace 查询、窗口、投影、自动补采或正文。
2. 批 B 只统一已有引擎真值的工具合同与 typed snapshot 选择；不为该 fixture 改排序值，不允许 state_churn 与同一正式状态双计。
3. 批 C 只做验证报告代际隔离；不得重新规划/应用第二份补丁，也不得把旧通过报告复制成新 report。
4. 三批均禁止读取 raw user wording、case ID 或 model answer prose 作 production hard gate；所有决定来自 route enum、trace typed predicate/scope、workflow execution mode 与 report generation。
5. 每批独立测试、提交、推送；批 A/B 后重跑 Trace 显式窗/因果投影/自动补齐专项，批 C 后重跑 write controller/verifier 专项。

`B4-A/P0` 施工验证：`TurnRouteHint` 已加入 `projectionTypedSignalFields` 单一清册，并在 `ToolBusContext` 与 `SubAgentContext` 双向投影；未来新增/删漏该字段会由反射契约测试直接失败。agent→tool 生产边界测试除核对 enum 字段相等外，还用 optional artifact route + generic current-source profile 重新编译 tool-side runtime/source authority，确认不会再从 `NeedsRepoAccess` legacy fallback 重开 current-source requirement。`go test ./internal/types ./internal/agent -count=1` 全包通过（21.292s/2.646s），completion/source authority 与显式窗因果投影、用户窗覆盖模型窄窗、full-artifact scope、frame process scope、system-supplement authority 五组专项通过。实现只传递既有 typed route metadata，未读取请求或答案原文，未改 trace query、显式窗口、因果投影或自动补采。

`B4-B/P0+P1` 施工验证：`trace_query` 的 Description/Parameters 已统一为同一引擎真值：`state_churn` 的 dominant typed state 走既有 closed matrix，fragmented runnable 与互斥 typed D/IO 可以按授权值参加候选，fragmented running 仍须有正的 CAP/compute-supply deficit，sleep/unknown 保持 context-only；同线程同物理状态账由既有 cross-type reconciler 吸收为一席，churn 诊断仍保留。没有修改任何排序值或根因引擎代码。指标快照新增 typed authority 选择：仅当 artifact projection、canonical subject、两个 `selected_window` 端点（共享 ±1ms tolerance）及五项状态时长完全一致时，canonical `predicate=state_churn` 才压掉同账 derived whole-window face；缺窗、坏值、未归属 projection 均 fail-open，`wakeup_causal_*` 或 `actual_window` 证明的 chain-episode face始终独立保留。旧 raw metric dedupe 移到 authority 选择之后，防止 ledger 中更早的派生行反向遮住 canonical 行；全过程不读取用户输入或答案正文。dispatch-sensitive Description golden 已按 ritual 重签并写入 evolution record。专项合同、canonical-first/episode-negative、窗口分组测试通过，`go test ./internal/tool -count=1` 全包通过（154.127s）；显式用户窗、full-artifact 覆盖、因果投影 materialization 与 system supplement authority 八项防回归专项通过（1.245s）。

`B4-C/P0` 施工验证：controller 只在 typed `ExecutionMode=verify_only` 的 `verify_batch` 动作进入本代 verifier 前清除 `Mutable.ChangeReport`；上一代报告仍保存在既有 workflow attempt/context ledger，不清 plan、baseline、context pack，不进入 planner/apply。reset 位于 verify action 内且在 infra-retry loop 外：一代只清一次，本代 `run_tests` 一旦生成报告不会被重试循环擦掉；若本代没有生成报告，既有 missing-report/infra retry 路径继续 fail-loud。e2e 预置 `stale-first-generation` green report，断言 `StageVerify` 入口为空、最终只保留新 `existing-default-retained` assertion，planner/apply 零调用且进度账出现 `verification_only_report_generation_reset`；verifier 动态 schema pin 进一步证明 reset 后 `run_tests` 重新可见。`go test ./internal/types ./internal/agent ./internal/orchestrator -count=1` 全包通过（22.345s/3.704s/9.977s）。实现只读 workflow execution enum 与 report slot，没有读取请求、case ID 或模型输出原文。
