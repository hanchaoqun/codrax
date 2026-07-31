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

B4 修后回放（runner 快照 `main@8880c609d`）：

- 结果目录：
  - `eval/results/trace_query_state_churn_root_cause_rank-20260731-050314`
  - `eval/results/github_issue_gson_lazy_number_symptom-20260731-050314`
- 严格 `parallel=2`；runner 2/2 PASS，人工 0/2 完全通过：Trace 的确定性主链已正确但最终正文仍混入未授权外推；Write 的生产修复正确但变更闭包含语法损坏的测试文件，自动 PASS 属于 false pass。
- Trace 防回归确认：两次 `trace_query` 都严格使用用户窗 `11.000000~11.008000`；完整 `Trace 因果投影` 仍发布；0 次源码读取；snapshot 只剩 canonical 两行，`app-20` 的 `19 switches / 20 fragments` 不再与 derived face 冲突。`fragmented_runnable_wait` 正确吸收进 rank=1 `runnable_wait` family，没有再写成“永不参与”。A/T1/T2 均获生产回放覆盖；T3 的 supplement 仍为 `no_typed_target`，但本轮查询/投影证据完整，保持 P2 filed。
- Trace 新根因发生在 perf pre-stage：`perf_triager` 在没有 `trace_query` 工具的独立 dispatch 中，把假定的 `0.2ms/次切换开销`、`3ms损耗`、`16.67ms外推后净计算约11ms`、`prio=53 属于 CFS/time-slice exhaustion` 写入自由 `PerfObservation.summary`。后续确定性查询已给出相反/更精确的 running/runnable 与 Harmony RT 语义；observation ledger 也已将 `perf_trace` producer 降为 advisory，但 `context.BuildPromptContext` 直接绕过 ledger，把这些 summary 继续渲染成 `Perf Triage — Validated Extraction`。finalizer 因而复制 11ms 外推，并把 runnable wait 误写成 context-switch loss。
- Explorer 还把 `8ms-running(3ms)=5ms` 重新命名为“state_churn 损耗”，并以自由 provenance 字符串发成 aggregate fact；`pressure_density=0.96` 又被叫作“临界”而没有任何 calibration。前者说明模型派生 aggregate fact 仍缺“实测量/数学派生/机制归因”的 typed derivation caliber，后者说明无量纲评分/密度仍缺比较基线。它们不应通过扫描答案词面拦截。
- Write 的 W1 已真实覆盖：日志出现两次独立 `run_tests {}`，两次都在 Maven/JDK 缺失后执行 `make check` 并生成新报告；旧 report 未复用。`unavailable_tool_attempts=1` 是 planner 尝试未开放的 `read_file`，与 verifier 无关。
- Write 的生产类补丁按 `value` 实现 `equals/hashCode`，数值转换方法未改；但生成的 `LazilyParsedNumberTest.java` 末尾有两组额外 `}`。直接根因不是 Java 个例：structured edit 收到 `start_line=1,end_line=20`，而 `old_text` 只覆盖 1~18 行；`normalizeStructuredEdits` 发现 old_text 在附近唯一命中后，静默把替换范围收窄为 1~18，再把包含完整闭合括号的 replacement 接到原 19~20 行之前，制造重复尾部。stale relocation 本应只平移等长范围，不能改变用户声明的替换跨度。
- 验证随后再次失守：Java verification probe 和 Maven 都是 `runner_missing`，fallback `make check` 只运行 Python source oracle；oracle 只检查生产类含 `equals/hashCode/value`，没有编译或解析被改坏的 Java 测试文件。系统仍将 report 标为 authoritative green。缺工具可以降级，但“某个行为 oracle 通过”不能等价为“所有 changed paths 的语法/构建闭包已验证”。

B4 修后新增 GAP 与后续批次：

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B4-T4 | P0 | Pre-triage / deterministic authority | LLM 预分析自由 observation 绕过 ledger 的 producer precedence，继续以 validated extraction 进入 finalizer，与 trace_query 竞争 | `PerfObservation` 增 validator-owned authority enum；model emission 与 deterministic semantic normalization 分流。确定性 trace_query 在场后，prompt projection 删除 model-extracted summary，仅保留 validator/legacy 兼容行；查询前明确只作 navigation。只读 typed authority/producer，不扫 prose | covered，批 D |
| EVAL-B4-T5 | P1 | 派生量 caliber | explorer 可把 occupancy shortfall 重命名为开销/损耗，并把自由 provenance 伪装成 measured aggregate | 为 aggregate fact 增 closed derivation caliber（observed/measured arithmetic/inference）与 typed operand refs；机制归因不能只凭两个标量相减升级。先跨 case 收集，不按 “损耗/临界”词面硬门 | filed |
| EVAL-B4-T6 | P2 | 无量纲指标可解释性 | `pressure_density=0.96` 没有 typed scale/baseline/threshold，模型自行称“临界” | 指标定义携 scale、方向、比较基线和 calibrated band；缺 band 时只报值和组成，不硬门答案 | filed |
| EVAL-B4-T7 | P1 | Perf segmentation fidelity | `MergePerfBundles` 声称合并 Layer 1~3，却没有合并 `Observations`；大 trace 分段后 model observation 与 system-minted time/priority semantics 全部消失 | 按 authority+kind+subject+span 的 typed identity 合并去重，deterministic validator 行优先，稳定截断到 schema cap；补单段/多段 parity 与冲突顺序测试 | filed，独立批 G |
| EVAL-B4-W2 | P0 | Structured edit 原子范围 | old_text relocation 可把声明的 replace 范围从 20 行静默缩为18行，replacement 与遗留后缀拼接 | relocation 只允许等长平移；候选 span 长度与原声明跨度不同则 `old_text_mismatch` fail-loud。覆盖缩窄/扩张负例与等长行漂移正例 | covered，批 E |
| EVAL-B4-W3 | P0 | Changed-path verification closure | 首选语言 runner 缺失后，旁路 source oracle 通过即可把包含未解析 changed path 的报告升为 authoritative green | verification report 增加 typed changed-path coverage：每个 changed path 必须有 compile/parse/test/explicit source oracle caliber；行为 oracle 不能覆盖未读取的同语言文件。缺 runner 且无等价 parser 时返回 unverified/fail-loud，不伪装 PASS | covered，批 F |
| EVAL-B4-W4 | P2 | 验证代际呈现 | W1 修后两次真实验证都发布为同名“测试通过”，用户难区分初验与 cumulative review | rendering 消费 batch/report generation id，标明初验/复核；不合并不同代真值 | filed |

批 D 不变量：

1. authority 由 `emit_perf_trace` validator 铸造，不允许模型在工具 schema 中自报；旧持久化 bundle 的空 authority 保持兼容。
2. deterministic time/priority normalization 保留；model-extracted observation 在查询前仍可作定位提示，查询后不得与 trace_query 数值/机制/因果结论竞争。
3. prompt projection 只消费 authority enum 与 ledger 的 deterministic producer 事实；禁止匹配用户输入、observation summary、case 名或最终答案。
4. 不修改 `trace_query` 窗口选择、root rank、因果投影 materialization、system supplement 或自动补采。
5. W2/W3 独立提交，避免把 Trace evidence authority 与 Write edit/verify truth 混成一个不可审计批次。

`B4-D/P0` 施工验证：`PerfObservation.Authority` 新增 validator-owned 闭集，`emit_perf_trace` schema 明确不暴露该字段；模型发出的 observation 由 `toPerfBundle` 固定铸为 `pretriage_model_extraction`，系统生成的时间/优先级语义及对冲突优先级文案的 deterministic normalization 固定铸为 `deterministic_validator`，旧 bundle 的空值保留 legacy 兼容。prompt 在确定性 `trace_query` 未出现前仍渲染 model observation，但显式标为 navigation-only，并要求数值、调度类别、机制、因果先由确定性工具验证；任一 accepted deterministic query 出现后，projection 按 enum 删除 model observation，只保留 validator 与 legacy 行，同时 finalizer 既有 residue suppression 不变。perf-triage skill 同步禁止把 runnable/non-running、切换间隔或片段数自行换算为切换开销、丢失计算、帧预算外推、调度策略或根因。全部生产判断只读 authority enum 与 ledger producer，没有读取用户输入、observation summary、case id 或最终答案。`go test ./internal/types ./internal/skill ./internal/context ./internal/tool -count=1` 全包通过（21.116s/0.525s/1.756s/157.773s）；schema 非可写、model/validator/legacy 三臂、Harmony normalization、用户窗关系、full-artifact supplement、因果投影 materialization 与 system-supplement authority 专项通过（2.1s）。

`B4-E/P0` 施工验证：structured edit 的 replace/delete relocation 现在把原 `start_line..end_line` 长度作为原子 typed invariant；`old_text` 只可把该范围整体平移到唯一同长度候选，不能缩窄、扩张或改变替换跨度。当前行号越界时也只有等长唯一候选可恢复，其余继续走原有 invalid-range/mismatch fail-loud；insert_before/insert_after 的多行 anchor 语义不变，因为插入操作没有声明 replace/delete end range。回归覆盖一行 old_text 试图缩窄两行 replace、两行 old_text 试图扩张一行 delete、两行范围等长平移正例、局部重复 anchor 正例，以及客户形状“全文件 replace 声明覆盖闭合括号、old_text 只到正文前缀、replacement 自带完整括号”必须拒绝，不能再产出重复 suffix。`go test ./internal/tool -count=1` 全包通过（154.854s）。实现只比较结构化行范围与 old_text 定位结果，不读取请求、模型解释或目标语言关键词；适用于所有文本语言。

`B4-F/P0` 施工验证：`ChangeReport` 新增逐源码路径的 `changed_path_coverage` 权威账，`ExecutedCommand` 新增系统铸造的精确 `covered_paths`。唯一 `finishReport` 收口会对当前 active slice（无 slice 时为完整 plan）的 recognized source paths 逐项编译覆盖：成功 project runner 只能覆盖其 typed language family 且位于该 runner working directory 下的路径；syntax/source-check 只能覆盖执行器实际收到的精确文件；passing verification probe 只有在同语言且携 `path:` 绑定时覆盖多文件计划，单源码计划可沿用既有 changed-symbol 单目标绑定。config/workflow 文件不被伪装成源码编译义务。任何 nominal pass 若仍有源码路径未覆盖，会改铸为 `failure_kind=verification_incomplete`、`reason_code=changed_path_verification_uncovered`、`verification_status=unavailable`，ToolResult 同步 false；真实 red test/build 与已经 unavailable 的尝试不被改写。controller、retry suppression、online slice completion 和 resumed completion 全部把该状态当“已应用但本地未验证”，不误归因成代码失败，也不进入重复修代码循环。

客户同型 e2e 已固定：Java/Maven runner 缺失后 Make `check` 即使 exit 0，也不能替 `src/test/java/...java` 签字，报告必须保留 Java `uncovered` 行；同语言 Java runner、精确 Python syntax fallback、同语言 path-bound probe 仍为 covered/pass；Python runner 只覆盖混合 Java+Python 计划中的 Python 路径，跨语言伪造 `CoveredPaths` 也会 fail-closed。所有判断只消费 plan path、active slice、runner/framework enum、working directory、exit/outcome 和 typed probe refs；不解析命令正文，不扫描用户输入、case ID 或模型答案。

验证结果：changed-path focused/e2e、`types`/`writeflow` 全包、orchestrator unverified/controller 专项通过；`go test ./internal/tool -count=1` 全包通过（153.688s）。显式用户窗优先、full-artifact 自动补齐、system-supplement authority、因果投影窗口/分区/主因专项再次通过（tool 1.235s、types 0.658s），批 F 未触及 Trace 查询、时间窗、投影构造或自动补采。

### B5：多 Trace 范围/信号不对称 × Cangjie 构造清单

- cases：`real_trace_e2_cross_trace_asymmetry`、`harmony/cangjie_repomap`
- 结果目录：
  - `eval/results/real_trace_e2_cross_trace_asymmetry-20260731-054331`
  - `eval/results/cangjie_repomap-20260731-054331`
- 严格 `parallel=2`；runner 0/2 PASS，人工 0/2 PASS。
- Trace 原始事实复算：
  - `donghu_tieba_frame.systrace` 有 15623 个带时间戳行，物理范围 `34579.450627..34579.595184`，覆盖 `144.557ms`；
  - `donghu_short_excerpt.systrace` 有 92 个带时间戳行，物理范围 `2942.244845..2942.245401`，覆盖 `0.556ms`；
  - 两者时基分离，不能无校准直接对齐；短 trace 中 CPU frequency/VSync 应表述为“本工件未采样/未观察到”，不能升级成设备事实。
- Trace 答案正确保留了时基不可直对齐和短 trace 单边缺样方向，但把 `event_search` 返回/命中的首尾时间当成工件范围，将首份误写为 `138.4ms`；又把 `limit`/matched rows 写成“完整、总计 90”，与自身 footer 的 enumeration incomplete 冲突。无因果行的工件覆盖对比还被强行追加空的 `Trace 因果投影覆盖边界`、补采建议、频率/VSync authority 和 `60.4 Hzns` 摘录，显著稀释答案。
- Cangjie 答案正确列出 2 个 extend、2 个 foreign func，但 public class 只列 `Bridge/App/Cart` 3 个，遗漏 thirdparty corpus 中 `Greeter/Animal/Dog/Service/Version` 5 个。首个 source-inventory observation 已明示 `candidate_budget_truncated`，但 completion 最终仍发布 `accepted_requested_universe`。

B5 新 GAP：

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B5-T1 | P0 | Runtime scope carrier 接线 | run-entry `RuntimeArtifactPreflight` 已精确识别请求中的两个 trace，`emit_analysisHasRuntimeArtifactCarrier` 却只看 attached log/trace 和 mutable pre-stage，导致 analyzer 的 typed `full_artifact` 被降为 `not_applicable` | 将 normalized preflight `HasRuntimeArtifact()` 接入唯一 carrier 判定；覆盖 request-path、attached、无 artifact 三臂。只读 typed preflight，不重扫请求词面 | covered，批 A |
| EVAL-B5-T2 | P0 | Trace 范围/枚举 caliber | streaming `event_search` 在 raw-pattern 预筛后才形成 parsed-event index，`Result.TimeStart/End` 又取 emitted/matched rows 的首尾；调用方无法区分“工件/选择范围”和“命中范围”，模型遂把 138.4ms 误当 144.557ms | 增 typed search coverage：selected/artifact timestamp envelope、matched envelope、matched total、emitted、scope/enumeration complete 分席；stream scan 在 pattern gate 前复用已解析 raw timestamp 统计 envelope，indexed lane 用 accounting 查询。旧 `TimeStart/End` 保持兼容但明确为结果/查询范围 | covered，批 A |
| EVAL-B5-T3 | P1 | 因果报告 materialization authority | `runtimeTraceFullReportMaterializationAllowed` 只排除 focused fact，普通 artifact coverage/timebase/sampling comparison 因而获得全套因果/优化/指标/补采报告；没有 causal rows 时仍生成空 boundary cluster | 建 typed answer-shape/evidence gate：显式用户时间窗永远保留；诊断、root-cause/call-chain 或实际存在 causal rows 保留；非诊断 generic comparison 且无 causal rows 不 materialize 因果报告。自动补采选择不改，不扫请求/答案正文 | covered，批 B |
| EVAL-B5-T4 | P2 | 指标单位组合 | metric snapshot 把已经格式化为 Hz 的值与原单位 ns 再拼成 `Hzns` | 单位由 typed metric formatter 单点持有，换算后不得继承输入单位；补 ns→ms→Hz 组合回归 | covered，批 B |
| EVAL-B5-S1 | P0 | Source-inventory 前置过滤 | `SourceQuotes` 在 lens query 中被拼成普通 query 后 token 化；`foreign func` 产生宽泛 `func`，又用 contains 匹配几乎全部 function。候选预算在精确 surface family 过滤前消耗，Cangjie 行被 Go 候选挤掉 | 将 schema 化 `SourceQuotes` 编成 typed surface-family phrases，按候选 `SurfaceTerms` 在预算/截断前过滤；存在已识别 family 时不回退宽 token，未知 family 才用通用 query。语言中立，不按 Cangjie/文件名特判 | covered，批 C |
| EVAL-B5-S2 | P0 | Source-inventory closure authority | aggregate-family fallback 只按 language/source-class 判断“碰过该 census”，extend/foreign 的少量 Cangjie thirdparty 行即可替遗漏 public-class 行自证完整 | 当请求短语可解析为 typed surface families 时，每个 `role + surface family` 必须由 complete lens 或 exact universe 覆盖；partial aggregate 只能补充描述，不能铸 `accepted_requested_universe` | covered，批 C |

B5 施工不变量与批次：

1. 批 A（T1+T2）只补 typed artifact/range/accounting authority；不修改 root-cause 排序、显式时间窗边界、因果投影构造或自动补采选择。现有显式窗 `TimeStart/End` 消费保持兼容。
2. 批 B（T3+T4）只控制系统报告 materialization 和 typed 单位组合。显式用户时间窗无条件保留 Trace 因果投影；诊断/root-cause/call-chain 与实际 causal rows 保留；普通多工件覆盖对比不因 `is_cross_component` 自动升级为因果诊断。
3. 批 C（S1+S2）以 `SourceInventoryProfile.SourceQuotes → typed SurfaceTerms` 为唯一精确入口，在候选预算前过滤，并在同一 family 粒度闭包；不读取 case ID、语言名、文件名或最终答案。
4. 每批必须覆盖原 witness、相邻正例、相邻负例和接线 tripwire；每批独立提交推送。批 A/B 后重跑显式时间窗、full-artifact auto supplement、causal projection；批 C 后重跑既有 Cangjie/Java surface-family 与宽泛 unknown-query fallback。

`B5-A/P0` 施工验证：`emit_analysis` 的 runtime carrier 单一判定已接入 normalized run-entry `RuntimeArtifactPreflight`，request-path trace/log 与既有 attached/mutable carrier同权；空 profile 仍不能铸权。`Result.EventSearchCoverage` 将 `scope_kind/scope_time/scope_timestamp_rows/scope_complete` 与 `matched_time/matched_total/emitted/enumeration_complete` 分席：stream lane 在 raw-pattern gate 前累计所选物理时间域，pattern 不再把工件范围缩成命中范围；indexed lane在既有 bounded event/census 面完成后用 allocation-free accounting pass 得到完整命中账，取消发生时不反向污染已完成面。旧 `TimeStart/TimeEnd` 保持 query/result window 语义。summary 头部明确标注 `selected_window_caliber=query_or_matched_rows`，full-artifact observation 只有在 typed coverage 证明 `scope_kind=artifact && scope_complete=true` 时才允许 pattern event_search 铸造；局部窗、anchor 片段与 windowed index 不越权。

原 witness 的通用缩小版固定：物理时间域 `1..5s`、命中域 `2..3s`、limit 后 emitted=1，stream/indexed 两车道均发布 `matched_total=2`，而 legacy result window 仍为 `2..2s`。工具端完整扫描 `9.000..9.010s` 发布 `scope_duration_ms=10.000` 和 full-artifact observation。`go test ./internal/tracequery ./internal/types -count=1` 全包通过（68.410s/24.492s）；显式用户窗、full-artifact 覆盖模型窄窗、因果投影 materialization、stream summary、carrier 与 scope-observation 专项通过。批 A 未修改根因排序、显式窗边界、因果投影构造或自动补采选择。

`B5-B/P1+P2` 施工验证：因果投影在共享 full-report shape gate 之后新增独立 typed materialization authority。精确显式时间窗、编译出的真实 causal rows、`IntentRootCause`、diagnostic profile/predicate、`ScenarioPerformanceBottleneck` 以及 resolved `QFRootCauseTrace/QFCallChain` 均保留；非诊断 `QFGeneric/QFComparison` 且零 causal rows 时不再仅因 trace_query 发布诚实的 `causal_conclusion=unproven` 而生成空投影。该判定只读取规范化 RequestModel 与编译投影集合，不读取 raw request、关键词、case ID 或答案正文，也不改变 supplement 选择、查询视图、根因排序和自动补采。

结构化指标的单位组合改为只给可精确解析的 bare scalar 追加独立 `Unit`；已携完整换算表达式的 scalar value 原样保留，故 `16552213 ns ≈ 16.55 ms ≈ 60.4 Hz` 不再被损坏成 `Hzns`，普通 `0.5 + ms` 与 `state` 语义保持。专项覆盖 generic 空投影负例、显式窗正例、真实 causal row 正例、diagnostic 空边界正例、生产 materializer 接线和 end-to-end metric supplement；定向矩阵通过（tool 1.938s、agent 1.054s），`go test ./internal/tool ./internal/agent -count=1` 包级回归通过（tool 159.348s、agent 2.976s）。

`B5-C/P0` 施工验证：`SourceInventoryProfile.SourceQuotes` 不再只被压成宽 token。系统将已校验 source quote 与当前 parser graph 实际发布的 base+specific `SurfaceTerms` 做规范化短语交集，得到 `role × surface-family` 集合；没有内建 Cangjie/Java/ArkTS 词表。识别到 family 时，typed language 过滤保留、宽 token 停用，family 精确过滤先于扫描预算；无法识别 family 的 query 完全保留旧 token/language 和 query-miss budget 行为。通用缩小 witness 用 12 个前置 Go function 加 3 个后置 Cangjie `public class`，在 `MaxScanPerRole=4` 下仍完整返回三个 class，证明无关 symbol 不再烧掉 construct-family 预算。

每个 complete lens 新增规范化 `SurfaceFamilies`，身份键也包含该维度，避免同 role/language/source-class 的不同构造面相互代签。completion 在允许 language/source-class fallback 前，从同一 parser graph 和 typed source quotes 重建所请求的 role-family 集；每一席都必须有对应 complete lens，且该席的每个 exact row 都被 principal member_set 覆盖或 typed excluded。回归固定 `function × foreign func` 与 `type × public class`：只交付一个 class 和 foreign func 时不得铸 `accepted_requested_universe`，补齐第二个 class 后闭包成立。实现未读取 raw request、case ID、语言名、文件名或最终答案。

结构收敛同步完成：新增逻辑拆入 candidate-set、query surface-family、requested-family closure 与 complete-lens surface concern 文件，`source_inventory_reconcile.go` 从既有 3651 ceiling 下压到 3610 行，未抬高任何旧文件上限；每个新文件均设置独立 LOC ratchet。定向矩阵与 convergence tests 通过，`go test ./internal/tool ./internal/types -count=1` 全包通过（tool 160.840s、types 20.892s），`go test ./internal/agent ./internal/orchestrator -count=1` 通过（agent 3.925s、orchestrator 11.301s）。

B5 首次修复后回放（runner 快照 `main@11b8e2284`，sweep `20260731-063724`）：

- 结果目录：
  - `eval/results/real_trace_e2_cross_trace_asymmetry-20260731-063724`
  - `eval/results/cangjie_repomap-20260731-063724`
- 严格 `parallel=2`；runner Trace PASS/Cangjie FAIL，人工 0/2 PASS。自动 Trace oracle 只钉了范围/单位等局部表面，没有覆盖“是否应发布因果报告”、计数 caliber 和跨工件时钟关系。
- Trace 的 T1/T2/T4 已真实生效：物理范围为 `144.557ms` 与 `0.556ms`，`Hzns` 消失。但 generic coverage comparison 的投影集合中存在一条 `VerifyClass` 语义优化 span，现有 `runtimeTraceProjectionSetHasCausalRows` 将 `SemanticSpans` 无条件视为 causal row，绕过 T3 gate，系统追加约 110 行根因/状态/优化报告。该行自己又被渲染为“优化项，非根因”，权限与呈现自相矛盾。
- 同一 Trace 查询发布 `matched_total=90, emitted=40`，正文和 aggregate fact 却写“CPU 频率事件共40条”。工具 summary 的 `matched_events=40` 与 frequency authority 的 `transition_events=40` 实际都读取 emitted slice；footer 同时正确警告 total=90，答案内部直接矛盾。
- 每个 artifact 的 `alignment=identity` 只证明其自身 canonicalization 未换算，不能证明两个独立 artifact 有共同 clock origin；答案先说“理论上属于同一时钟域”，再说“没有跨文件校准锚点、不能对齐”。安全结论正确，前一关系主张越权。
- Cangjie 人工调查最终给出正确完整集合：2 extend、2 foreign func、8 public class、11 package。失败不是成员检索结果错误，而是用户要求的 `name/location/package` 没有同 row 呈现。`Principal Enumeration Rows` 每项有精确 location，但 package 只在 note；最终表只保留 label/text 两列，package 被拆到独立 section/citation，typed row oracle 因此正确报缺行。
- 更上游的真实 lens 没命中构造族：repomap 持久图只含 production/常规索引文件，root source-inventory 临时 auxiliary projection 以 256 文件为上限按 tracked path 顺序取前缀；在两千余辅助 Go/test 文件的仓库中，少数 Cangjie/ArkTS 文件到达预算前已经被截掉。`repo_lens:auxiliary_projection` 虽存在，family filter面对的 graph仍没有这些构造，合成测试小于预算故未暴露此缺口。之后 query-role 扩张又把已精确声明的 function/type/file 扩成 field/method/constant/variable，进一步稀释检查面。
- analyzer 还发出 `requested_scope=auxiliary`，唯一 scope quote 因不在当前请求中已被 validator 删除，但空 quote 的 scope enum仍存活。现有 construct-only softening只处理“存在且回声 construct quote”，遗漏“所有 quote 均被拒绝”一臂；该 enum虽未直接过滤本次 active inventory（另一路保护放开），但会误导 tool prompt/model并可污染其他消费者。

B5 回放新增 GAP 与施工顺序：

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B5-T5 | P0 | 因果发布权限 | `SemanticSpans` 中 off-chain/background 优化点也被 broad `HasCausalRows` 当作真实根因/链路，generic coverage 绕过 T3 | 保留 broad predicate 给 coverage/非空集合；新增 publication-grade predicate，只接受 primary root、on-chain/adjacent causal rows、wakeup path/supporting hops，或 semantic row 已同时进入 on-chain causal bucket。显式窗、diagnostic/root-cause/call-chain保持 | covered；批 G，待真实回放 |
| EVAL-B5-T6 | P0 | 计数 caliber | event_search emitted slice 被命名为 `matched_events`/`transition_events`，尽管 typed coverage 已有 MatchedTotal | summary/authority 分席 `matched_total` 与 `emitted`; compacted 时禁止 emitted 冒充总数。只有查询事件族已 typed 限定时，frequency authority 才可消费 exact total | confirmed；批 H |
| EVAL-B5-T7 | P1 | 跨工件时钟关系 | per-artifact `alignment=identity` 被模型组合成共同 clock domain | 新增 cross-artifact relation authority：未有共享 calibration/session anchor 时只发布 `relation=unproven`；不得从两个 local identity 推 common origin | filed；批 H 后评估 |
| EVAL-B5-S3 | P0 | Auxiliary projection universe | root projection 在解析前按路径前缀消耗 256 文件预算，少数语言/源类可永久缺席 | 对候选按 typed language × source-role 做确定性覆盖均衡，先轮转每个已支持语言/源类，再填剩余预算；不读取 query/user prose，不按语言名特判 | covered；批 G，待真实回放 |
| EVAL-B5-S4 | P0 | 请求字段 row-local 投影 | accepted principal row 可只有 location，requested package 留在 note/独立 section，finalizer仍通过 | 请求字段必须由 source observation attribute或同文件 grounded package/module/namespace declaration的 typed join进入 row.Attributes；table/list compiler统一消费，不能解析模型答案正文 | covered；批 G，待真实回放 |
| EVAL-B5-S5 | P1 | Source scope quote authority | scope quote全部被拒绝后，model-inferred scope enum仍保留 | 仅当输入 source quote 非空且验证后全部归零时软化 SourceScopeProfile；从 typed auxiliary paths 合成或显式空 quote 的 scope 不受影响 | covered；批 G，待真实回放 |
| EVAL-B5-S6 | P1 | Query role authority | query/source-quote/entity 可以补充 profile 未列出的 construct role；在大候选面上可能稀释预算，但完全冻结又会漏掉 typed entity 明确要求的角色 | 不能按“profile 已有 role”一刀切。先由 S3 的 language × source-role 公平投影消除文件宇宙饿死；若后续仍复现角色预算挤占，应给 typed entity/source-family role 与宽 query-token role 分级配额，而不是禁止补充 | filed；批 G 审计纠正，不施工 |

批 G 不变量：

1. Trace publication gate只改变“generic 无根因请求是否显示整块报告”，不改投影编译、根因排序、唤醒链、窗内量、自动补采或 trace_query。
2. 显式 typed 时间窗即使只有 semantic/background row也继续保留 Trace 因果投影；diagnostic/root-cause/call-chain 空边界继续保留。
3. auxiliary projection采用支持语言/源角色的 typed metadata做覆盖均衡，不扫描用户请求、query字符串、模型答案或 case 名；全体仍受同一文件数/大小预算。
4. package/module/namespace列只由结构化 observation 或 grounded same-file declaration join铸造；中文 note 中出现“包路径”不构成权限。
5. analyzer scope修正只撤销没有 validated source quote 的模型推断，不削弱用户明确命名路径、production/test/docs/auxiliary scope或显式排除；query-role 补充保持现状，避免损伤 typed entity/source-quote 自动补齐。

`B5-G` 施工中间审计纠正：最初曾尝试在 analyzer 已给 principal roles 时完全禁止 query-role 扩张；包级回归立即证明这会破坏现有 typed entity/source quote 自动补齐，例如 function profile 下由结构化 `Index` 实体补出 type role 的三组正规场景均漏行。该方案已在提交前撤销，S6 从“confirmed/批 G”改为 filed；不能为了单次 Cangjie 候选稀释关闭一类有效能力。批 G 保留更上游、更通用的 S3：在相同 256 文件与单文件大小预算内，使用 typed language × source-role 确定性轮转，避免大量单一语言测试文件在解析前饿死少数语言/源类；它不读取 query、用户原文、case 名或模型答案。

`B5-G` 施工验证：Trace publication authority 从 broad “有任何可渲染 context”谓词拆出 publication-grade 谓词；standalone `SemanticSpans` 不再给 generic coverage/comparison 铸整份因果报告，真实 primary/on-chain/adjacent/wakeup/supporting hop 仍可铸权。显式 typed 时间窗在 publication-grade 行为空时仍无条件保留因果投影，投影编译、根因排序、窗内量、唤醒链和 supplement 选择均未修改。

root auxiliary projection 保持 256 文件和原单文件大小界限，只把“按 tracked path 取前缀”改为 language × source-role 的确定性轮转；通用压力 fixture 以 336+ 个前置 Go test 文件和后置的 6 个 Cangjie fixture/thirdparty 文件验证，6 个少数语言文件及两个辅助 source role 都进入相同预算。请求的 package/module/namespace 维度新增精确 typed join：principal evidence 的 `Object` 必须与同文件、grounded/recovered package-like declaration 的 `AnchorSymbol` 完全相等，才进入 `EnumerationDisplayRow.Attributes`；不解析 member note、summary、用户原文、答案正文，也不从路径猜 package。scope 修正同样区分“未提供 quote”和“提供但全部验证失败”，只撤销后者。

定向矩阵覆盖 semantic-only generic 负例、真实 causal row 与显式窗正例、balanced auxiliary universe、grounded same-file declaration join、全 quote rejected scope，以及 query-role/typed entity 与 synthesized all-scope 邻接正例。`go test ./internal/types ./internal/tool/repomap ./internal/tool ./internal/agent -count=1` 全包通过（types 18.493s、repomap 2.309s、tool 154.257s、agent 2.830s）；`git diff --check` 通过。下一步先提交推送本批，再严格 `parallel=2` 重放 B5。

### B5 当前代码回放与批 I（2026-07-31）

第一次批 G 回放（sweep `20260731-071128`）作废：`parallel_selected.sh` 只复制工作区已有 `./codrax`，没有核对构建 revision。人工检查时 binary 仍为 `11b8e2284`，而计划验收 HEAD 已是 `4c740bc31`；Trace PASS/Cangjie FAIL 都只能作为旧基线，不能证明当前批次。显式执行 `make` 后，binary revision=`4c740bc31790-dirty`，再以严格 `parallel=2` 重跑：

- 结果目录：
  - `eval/results/real_trace_e2_cross_trace_asymmetry-20260731-072430`
  - `eval/results/cangjie_repomap-20260731-072430`
- runner 2/2 PASS；人工 Trace FAIL、Cangjie PASS。
- Trace 当前代码已经覆盖 T5：5 次 `trace_query` 后 `trace_query_final_projection_blocks=0`，最终没有 `Trace 因果投影`；物理范围 `144.557ms/0.556ms`、frequency exact total=`90`、VSync/frequency 单边采样和“不具备直接对齐权限”主方向正确。
- 这类请求不需要 `Trace 因果投影`：它是两个完整工件的覆盖/采样/时基比较，没有显式用户时间窗、诊断/root-cause/call-chain 形状，也没有 publication-grade causal row。反向保护不变：显式 typed 时间窗、诊断/root-cause/call-chain 或真实 causal row 任一成立时仍 materialize；自动补采、projection 编译、根因排序、唤醒链和窗内量均未改。
- Trace 新暴露的是上游合同偏移：analyzer 已发 `intent=explain, scenario=generic, full_artifact` 和明确 `current_source_mode=exclude`，`buildAnalysisIR` 却把 generic 当“未分类”，经 `InferScenario` 默认升级为 `architecture_explain`。最终要求 `current_code_path/component_relation`，模型三次 completion、两次 runtime evidence 被拒，最后用 `external_only_trace` waiver 绕过。此问题与因果投影是否发布正交。
- Trace 工具每轮都明确发布 Harmony 权威语义“larger numeric higher；1-40=CFS；41-159=RT”，最终模型仍反写成 `1-40=RT/41-159=normal`。当前单次 witness 先判模型波动，不以扫描答案正文或关键词 hard gate 修正；若跨 case/模型重复，再建设 typed priority observation 的可见字段投影。
- Cangjie 最终通过逐文件回退交付完整清单，但用了 17 次 read、3 次 list、18 explorer rounds、283s。两个 root source-inventory lens 都带 `repo_lens:auxiliary_projection`，持久图也已有 Cangjie path/language census，仍只返回 Go 行。代码根因是辅助投影把“路径已存在”误当“该文件已有可复用符号索引”；同路径 `ParseTier>=4`/零 symbols 的 path-only 空壳被跳过，且临时解析结果在 merge 时又被 same-path 去重丢弃。

新增/更新 GAP：

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-INFRA-2 | P0 | Eval 构建身份 | selected runner 不校验 binary revision/dirty Go inputs，提交后的旧二进制可伪装成当前代码验收 | 启动时 fail-loud 比对 `codrax version` revision 与 12-char HEAD，并拒绝 tracked/untracked Go build inputs；提交后必须 `make` 才能回放 | 已施工；批 I |
| EVAL-B5-T8 | P0 | Runtime scenario authority | `scenario=generic` 被统一视作缺省；纯 runtime scope + explicit source exclusion 被默认升级 architecture | `InferScenario` 增加 typed scoped-runtime observation lane：config/root-cause/performance 保持优先；其余 validated runtime scope + explicit source exclusion 留在 generic。只读 enum、validated carrier 和统一 source-lane decision，不解析 quote 内容 | 已施工；批 I |
| EVAL-B5-S7 | P0 | Auxiliary graph refresh | 已存在 path/language census 的空壳 FileInfo 阻断有界辅助重解析，临时同路径解析结果也无法替换 | 只有 language 一致、ParseTier<4 且有 symbols 的 existing row 才可复用；其余进入原有文件/字节预算，并在临时 projected graph 中 same-path replace，调用结束恢复原图 | 已施工；批 I |
| EVAL-B5-T6 | P0 | 计数 caliber | emitted 冒充 total | 当前回放已显示 exact total=90，正文不再写 40；标记 covered | covered |
| EVAL-B5-T7 | P1 | 跨工件时钟关系 | 两个 local `alignment=identity` 仍缺少显式 cross-artifact relation carrier | 安全结论已正确，typed relation 仍 filed；批 I 后再按更高优先级 eval 复现率排序 | filed |
| EVAL-B5-M1 | P2 | 模型语义波动 | 模型反写工具已明确的 priority mapping | 暂不加答案正文 hard gate；记录 witness，等待跨 case/模型复现再决定 typed projection | filed-model-variance |

批 I 不变量与验证要求：

1. runtime scenario 修正不读取 raw request、model thinking、source quote 内容或最终答案；只消费 `RuntimeArtifactScopeProfile.Active()`、`ExternalObservationPolicy.ExcludesCurrentSource()` 和统一 current-source lane。
2. `IntentRootCause`、config 与 typed performance 仍优先；显式时间窗即使 scenario=generic 仍由独立 causal materialization authority保留投影。
3. auxiliary refresh 不改变持久图、不提高 256 文件/单文件字节预算、不按语言名/case/query 特判；已有完整索引继续零重解析。
4. 新生产路径 e2e 用两份真实 trace fixture 执行与 eval 同形的六个查询，确认 background semantic row 不会重复铸成 publication-grade cause；已有显式窗/真实 cause 正臂共同回归。
5. selected runner 的 revision 检查是 eval fail-loud，不进入产品路由或答案门控。

批 I 定向验证已通过：`internal/types` 固定 full/explicit-window/bounded-selector scope authority；`internal/analysis/compiler` 固定 pure runtime comparison→generic 与 runtime root-cause→root-cause；`internal/tool/repomap` 固定同路径 ParseTier4/零 symbols 的 Cangjie 文件被临时刷新并恢复原图；`internal/tool` 真实 E2 trace e2e 确认 generic cross-artifact coverage 不 materialize 因果投影，显式窗/真实 cause 专项继续通过；`bash -n eval/parallel_selected.sh` 通过。提交前仍需跑相关包级回归，提交推送后 `make`，再以同一双 case 做最终验收。

### B5 r5 人工审计与批 J（2026-07-31）

批 I 提交 `ccd1b1d33` 后重建，runner 校验 binary revision 与 clean build inputs，再以严格 `parallel=2` 回放（sweep `20260731-074318`）：

- 结果目录：
  - `eval/results/real_trace_e2_cross_trace_asymmetry-20260731-074318`
  - `eval/results/cangjie_repomap-20260731-074318`
- runner 2/2 PASS；人工 Trace FAIL、Cangjie PASS。
- Trace 的 T8 已真实覆盖：全程 4 次 `trace_query`、零 repo/source read，最终没有 architecture/current-source 合同，也没有 `Trace 因果投影`。主比较给出 `144.557ms` 对 `0.556ms`、短 trace 当前窗内无 CPU frequency/VSync、两工件无共享 calibration 因而不能直接对齐，方向正确。
- 但因果块收窄后，其他 sibling materializer 仍由旧 `runtimeTraceFullReportMaterializationAllowed = !focused_fact` 放行。最终额外出现“确定性优化点”“Trace 指标快照”“trace_query 关键观测核对”和补采 caveat；普通完整工件覆盖/采样比较不是 focused fact，因此只修 causal block 留下了同根的全报告发布分叉。
- 同一 `window_stats` 的 `frequency_authority transition_events=413` 实际为 `EventCPUFrequency + EventClockSetRate`：真实 `cpu_frequency` 精确总数为 90，另有 323 条通用 `clock_set_rate`。工具把两个不同事件族相加并命名为调频 transition，模型据此写成“413 次 CPU 频率切换”。这是确定性计数 caliber 错，不是模型波动；此前 T6“emitted 40 冒充 total 90”已修复，但不能覆盖本次跨事件族合并。
- Cangjie 最终完整交付 2 extend、2 foreign func、8 public class 及 row-local location/package，正确性通过；耗时由 r4 的 283s 降为 167s。但两个 source_inventory lens 仍只返回 Go field/method，靠 8 次 `read_file` 回退。
- 冷读和临时诊断证明 S7 的 same-path refresh 已生效：临时图包含 11 个 Cangjie 文件及正确 `extend/foreign-func/class` symbols。剩余丢失发生在第二级 execution view：`sourceinventory.NewExecutionView` 按 `Graph.Files` 原顺序在过滤 family 前截取文件前缀；临时辅助文件追加在 3892 个 base 文件之后，精确 parser-derived family 根本进不了 candidate loop。此前“family 在 symbol scan budget 前过滤”的测试使用未截断 view，未覆盖这层文件成员资格。

B5 r5 新增 GAP 与批 J 状态：

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B5-T9 | P0 | Trace 完整报告发布权限 | causal block 使用窄 publication authority，语义优化/指标/next-step/perf-quality/观测/frequency/VSync sibling 仍使用 broad `!focused_fact` gate；generic coverage 无根因仍被注入整套报告 | 统一 typed report-shape authority：显式窗优先保留；focused fact 保持收窄；diagnostic/root-cause/performance/call-chain保留；generic 仅在 compiled ledger 有 publication-grade root/chain row 时放行。所有 materializer 继续消费一个 gate | 已施工；批 J，待 r6 回放 |
| EVAL-B5-S8 | P0 | 精确 family 二级预算 | auxiliary symbols 已入临时图，但 bounded execution-view 在 family filter 前按文件前缀截断，后追加的少数语言永远不可见 | 仅对 analyzer-validated source quote × parser-derived exact surface-family，绕过 view membership并在完整 symbol index 上先做精确 family/scope filter，再消费独立 query/materialization budget；token/no-query lane仍受 view约束，deadline/cancel fail-closed | 已施工；批 J，待 r6 回放 |
| EVAL-B5-T10 | P0 | Frequency 事件族 caliber | `cpu_frequency + clock_set_rate` 被合并为 `transition_events`，通用时钟活动冒充 CPU 调频次数 | WindowStats 和 TraceEvidenceAuthority 分席 `cpu_frequency_rows` 与 `clock_set_rate_events`；前者只消费共享 strict per-CPU sample predicate，后者单独统计有效 clock rows；summary/answer caveat明确两者都只是 background activity | 已施工；批 J，待 r6 回放 |
| EVAL-B5-T7 | P1 | 跨工件时钟关系 | 尚无正式 cross-artifact calibration relation carrier | r5 主结论安全，保持 filed，按后续复现率排序，不为单例增加文案门 | filed |

批 J 不变量：

1. 显式 typed 时间窗在所有 analyzer label 下都拥有最高报告权限；现有 root-cause、diagnostic、performance、QFRootCauseTrace/QFCallChain 与真实 publication-grade causal row均保留完整报告。
2. 不改变 trace query 选择、自动补采、因果投影编译、根因排序、唤醒链或窗内可消除量；只统一回答发布 authority 和拆分已有事件计数。
3. focused runtime fact 必须在 ledger 行判断前 fail-narrow，不能因 incidental causal-looking row重新膨胀。
4. exact surface-family 旁路只消费 validated typed source quote 与 parser symbol `SurfaceTerms`；显式 scopes/visibility/exclusion仍是 hard boundary，query scan、materialization、deadline/cancel仍可截断并披露。token/no-query 不得绕过 execution view。
5. 不扫描 raw user request、model thinking、case ID 或最终答案正文，不按 Cangjie/Go、具体类名、具体 trace 文件名特判。

批 J 施工验证：

- report authority 新增统一 shape 决策，所有八个 runtime system materializer 继续由既有 tripwire确认接入同一个 full-report gate；generic comparison 无 causal rows为负，真实 primary/on-chain row为正，explicit-window/root/call-chain邻接正臂保持。
- 真实 E2 两 trace 生产路径 e2e 执行同形六查询，确认 generic comparison 不生成任何 authenticated runtime system block；同一 fixture 同时确认 `cpu_frequency` 与 `clock_set_rate` 均进入不同的非空 typed lane。
- source-inventory 压力 fixture 的 late auxiliary `public class` 全部位于 `MaxScanPerRole=4` 的 execution-view 前缀之外；exact family仍完整返回三个成员且 complete，token-only `Bridge` 保持被 view 截断并披露 incomplete。
- frequency synthetic pin 同窗设置 12 个 CPU frequency rows 与 37 个 clock rows，authority 分别输出，不再相加；malformed clock row不进入 typed clock census。
- `go test ./internal/tracequery ./internal/types ./internal/analysis/compiler ./internal/tool/repomap -count=1` 通过（68.603s/23.357s/2.240s/2.622s）；`go test ./internal/tool -count=1` 通过（155.771s）；真实 E2 新增计数分席断言后专项再次通过。`git diff --check` 通过。

### B5 r6 人工审计与批 K（2026-07-31）

批 J 提交 `f834e6917`、重建并通过 runner revision/clean-input 校验后，以严格 `parallel=2` 回放（sweep `20260731-080749`）：

- 结果目录：
  - `eval/results/real_trace_e2_cross_trace_asymmetry-20260731-080749`
  - `eval/results/cangjie_repomap-20260731-080749`
- runner 2/2 PASS；人工 Trace FAIL、Cangjie PASS。
- Trace 的 T10 已真实覆盖：正文明确分开 `cpu_frequency_rows=90` 与 `clock_set_rate_events=323`，不再相加冒充 CPU 调频次数。结构化 AnswerDocument 的因果投影、优化、指标、next-step、frequency/VSync 等块均未发布。
- 但 finalizer 的 last-mile renderer 在 AnswerDocument 之后独立追加 `trace_query 关键观测核对`，只接了旧 focused-fact 特判，没有消费批 J 的共享 full-report authority。最终约 40 条 top sleep/state churn/I/O inode 背景行重新污染普通覆盖比较；这不是模型波动，而是第二发布栈的确定性接线遗漏。
- Trace 正文的覆盖/采样主结论正确，但仍有三项非阻断问题：local `alignment=identity` 被组合成“两份时间基准相同”（T7）；`15623 个事件`附近生成了 `156ba_frame`；两条 zero-match 的 pattern 标签互换。后两项当前只在单次模型输出出现，系统对未知 `ba_frame` 已发软校验附注；遵守“不扫描模型答案正文做 hard gate”，暂记 model variance。
- Cangjie 最终清单仍完整正确，但确定性 inventory 没有交付。analyzer 将构造词写成非法 `target_roles=["extend","foreign","public"]`；`parseSourceInventoryProfile` 因零合法角色返回 nil。稍后的 typed-enumeration synthesizer确实保留了整句、已逐字验证的 source quote，并恢复 function/method/type，说明批 J exact-family 下游不是根因。
- 真正丢失发生在恢复时序之前：profile 为 nil 时，`softenModelAuthoredRequiredFilesForSourceInventory` 未识别这仍是显式 inventory 请求，模型猜测的 `internal/tool/repomap/index/cangjie_parser.go` 被保留为 hard required file；同一 profile 中合法的 `requested_fields=name/location/package` 也被合成默认 `name/location/summary` 覆盖。后续 lens 因而只在 parser 文件范围列出 Go function/method/type，最终依赖 14 read、3 list 手工恢复。

B5 r6 新增/更新 GAP：

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B5-T9 | P0 | Trace 完整报告发布权限 | 结构化八个 materializer 已统一，但 finalizer last-mile raw observation supplement 属第二发布栈，仍绕过 authority | authority 下沉为 types 共享函数；tool materializer 与 agent last-mile 同时消费 `typed request shape + compiled publication-grade projection`。nil IR fail-open | partial；由 T11 收尾 |
| EVAL-B5-T11 | P0 | Last-mile 发布接线 | generic comparison 在结构化报告被抑制后仍追加 raw causal/background observation wall | `renderTraceQueryObservationSupplement` 接共享 full-report authority；显式窗、diagnostic/root-cause/performance/call-chain、真实 root/chain 正臂保持。只读 typed IR/ledger，不读 raw prompt/答案 | 已施工；批 K，待回放 |
| EVAL-B5-S9 | P0 | Optional profile 局部降级 | 一个可选 enum 数组全坏使整个 profile 变 nil，连带放大无关 guessed required_file 并丢失合法 display facets | `is_source_inventory=true` 作为独立精确信号先软化非用户点名 required_files；typed enumeration 合成时逐字段保留已独立验证的 requested_fields/source_quotes/type facets，只有角色降级为通用 structural carriers。禁止把非法词映射成 case/语言特判 | 已施工；批 K，待回放 |
| EVAL-B5-T7 | P1 | 跨工件时钟关系 | local identity 仍被模型组合成 shared timebase | 保持 filed；需要正式 cross-artifact calibration/session relation carrier，不加答案关键词门 | filed |
| EVAL-B5-M2 | P2 | 模型生成波动 | `156ba_frame` 及两个 zero-match label 互换 | validator 已软提示未知术语；等待跨 case/模型复现，不以原文扫描或 case-specific rewrite 施工 | filed-model-variance |

批 K 不变量：

1. last-mile gate 与结构化 report gate使用同一 types authority；不得再维护 agent/tool 两份判定。
2. 显式 typed 时间窗优先级最高；即使没有 root row，Trace 因果投影、根因边界、唤醒链、窗内可消除量与系统自动补齐仍可发布。
3. generic comparison 若 compiled ledger 出现 publication-grade primary/on-chain/adjacent/wakeup/supporting row，仍可升级完整报告；standalone background semantic row不能铸权。
4. source inventory 修复不把 `extend/foreign/public` 或任何语言构造词新增为 role alias；只保留逐字验证 quote 和 schema-valid fields，exact family继续由 parser `SurfaceTerms` 交集产生。
5. `is_source_inventory=false` 的普通机制请求不受 required-file softening影响；用户在当前请求中明确写出的 exact inventory path继续保留。

批 K 验证已固定：generic comparison last-mile负臂、explicit-window/root-cause/call-relation正臂；共享 types authority的 empty/background/root/window/call矩阵；invalid-role inventory保留 name/location/package 和逐字合法 quotes、丢弃未点名 guessed parser required-file；显式 non-inventory保留普通 required-file。回归过程中旧测试捕获 `intent=explain + question_kind=call_chain/axis=call` 不一定被 family resolver提升为 QFCallChain，已将这两个 schema enum信号直接纳入共享 authority，未用词面修补。`go test ./internal/types ./internal/agent ./internal/tool ./internal/tool/repomap -count=1` 全包通过（types 21.889s、agent 2.791s、tool 155.000s、repomap 2.522s）；`git diff --check` 通过。待提交推送与严格双 case r7。

### B5 r7 人工审计与批 L/M（2026-07-31）

批 K 提交 `0c484c8d3`、重建并通过 runner revision/clean-input 校验后，以严格 `parallel=2` 回放（sweep `20260731-082841`）：

- 结果目录：
  - `eval/results/real_trace_e2_cross_trace_asymmetry-20260731-082841`
  - `eval/results/cangjie_repomap-20260731-082841`
- runner 2/2 PASS；人工 Trace FAIL、Cangjie FAIL。
- Trace 的 T11 已真实覆盖：6 次 `trace_query`、零 repo/source read，结构化报告和 last-mile `trace_query 关键观测核对` 均未发布，也没有 `Trace 因果投影`。这类没有显式时间窗、诊断/root-cause/call-chain 或 publication-grade causal row 的完整工件覆盖比较，本来就不需要因果投影；显式 typed 时间窗正臂、自动补齐与完整报告权限未改。
- Trace 的数值主体正确：`144.557ms` 对 `0.556ms`、`cpu_frequency=90`、VSync 仅第一份存在、绝对时间戳偏移约 `31637s≈8.8h`。但答案无跨工件证据声称“两者来自同一台设备（com.baidu.tieba 进程）”，又把 artifact-local `alignment=identity` 组合成跨工件时基关系；第一份还被称作“完整帧渲染链路”。当前 ledger 只有每份工件各自的范围/identity，没有 shared-device、shared-session、shared-clock-origin 或 capture-completeness carrier。T7 已连续多轮复现，应由 P1 升为 P0。
- Cangjie 的 S9 已真实覆盖：analyzer 一次发出 schema-valid `target_roles=type/function/constant`、`requested_fields=name/location/package` 和逐字 source quotes，且没有 guessed parser required-file hard scope。确定性 source-inventory 已在第一轮完整返回 2 个 Cangjie `extend`、2 个 `foreign func`、8 个 `public class` 及 row-local package。
- 但同一 source-inventory 还把 ArkTS `@Extend(Text) function highlight` 发布为 `surface=@Extend(Text) extend extend highlight`。代码根因不是 Cangjie/ArkTS 语言路由，而是 `sourceInventoryConstructSurfaceTerms` 对任何 `kind=extend` 无条件铸裸 `extend`；parser 已提供的 `@` 语法标记没有参与 family identity，导致 annotation/decorator form 与 keyword-block form碰撞。之后完整性门把该错行当权威缺口，强制模型补读并将 extend 总数写成 3。
- runner 的 `EXPECT_INVENTORY_COUNT_EXTEND=2` 仍 PASS，是因为 `eval_inventory_rowset_reasons` 的 `matched` 只累计预先声明的 expected rows；`expected_count` 实际校验“预期行中命中了几条”，不校验回答可见 rowset 的真实基数，也不拒绝未声明 extra row。该 oracle 能发现漏行，不能发现多报。

B5 r7 新增/更新 GAP：

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B5-T11 | P0 | Last-mile 发布接线 | generic comparison 的第二发布栈绕过共享 authority | r7 中因果投影、结构化完整报告和 last-mile 观测墙均消失；显式窗正臂已有单测 | covered |
| EVAL-B5-S9 | P0 | Optional profile 局部降级 | invalid optional roles 连带丢失合法 fields/quotes并放大 guessed file | r7 analyzer 直接给出合法 profile，且无 guessed file 缩窄；合法 field/quote 全保留 | covered |
| EVAL-B5-S10 | P0 | Parser 语法表面身份 | parser `kind` 的裸 keyword family 覆盖了其已提供的 annotation/decorator sigil，令同名 keyword 与 `@Marker(...)` 跨语法形碰撞 | 从单 row 的 parser metadata 提取 syntax-form identity：当 marker base 与 construct kind 同源时，family 使用带 sigil 的 marker base+member，不再同时铸裸 kind；keyword form维持原 family。不得看语言、case、请求或答案 | 已施工；批 L，待回放 |
| EVAL-INFRA-3 | P1 | Inventory exact-count oracle | `EXPECT_INVENTORY_COUNT_*` 只比较 expected-row matches，extra row 不改变 matched，产生 false PASS | 为 typed inventory oracle增加可解析的确定性 rowset receipt/真实可见成员基数，再校验 exact expected count与 unexpected members；在 receipt 前不得靠 Markdown bullet 猜成员 | filed；批 L 后单独评估 |
| EVAL-B5-T7 | P0 | 跨工件关系权限 | per-artifact local identity/range 被模型组合成 shared device/session/clock relation | 在 runtime ledger 建立 per-artifact identity 与 pair relation 分席；只有共同 session/device/calibration anchor 可铸 `proven`，否则显式 `unproven`，并把该 typed relation交给成文。不得扫描答案纠错 | 已施工；批 M，待回放 |
| EVAL-B5-M3 | P2 | 模型语义波动 | “完整帧渲染链路”和短片段“仅有两类事件”等 completeness 绝对化措辞 | 先由 T7 pair relation carrier和既有 enumeration caliber软约束；若跨 case复现，再建设 artifact completeness typed field，不扫描答案正文 | filed-model-variance |

批 L/M 不变量：

1. 批 L 只使用 parser 单行已经发布的 `Kind/Doc/Name/SurfaceTerms` 区分 keyword、annotation/decorator 等 syntax form；不读取 raw request、source quote语义、语言名、case ID或答案正文。
2. Cangjie `extend Cart`/`extend String` 继续属于 `extend` family；ArkTS `@Extend(Text)` 属于带 sigil 的 marker family。显式请求 `@Extend` 的能力必须保留，不能靠删除 ArkTS row修复。
3. source-inventory 的 role、scope、visibility、预算、complete-lens 与 row-local package join均不变；只修 family identity。
4. 批 M 的 cross-artifact relation是新 typed 证据，不改变单工件 canonical time、显式时间窗、Trace 因果投影、根因排序、唤醒链、窗内可消除量、查询选择或自动补齐。
5. 没有共同锚点时必须表示 `unproven`，不能把“时间戳不同”猜成不同设备，也不能把“格式/单位相同”猜成同一时钟；存在校准/session anchor的将来正臂必须可扩展。

`B5-L/P0` 施工验证：construct family 在单一 parser row 内先比较规范化 `Kind` 与前导 marker base；同源 marker 使用带 `@` 的 base+member terms，keyword row继续使用裸 kind+member。与通用 query token 不同，已验证 source quote 的专用规范化保留 `@` syntax sigil，因此 `extend 块` 只选择裸 `extend` family，显式 `@Extend` 只选择 `@extend` family。端到端 candidate-set fixture 同时放置 keyword 与 marker 两行，分别只返回 `Cart` 与 `highlight`，证明不是简单删除 ArkTS 能力。

结构收敛门同批守住：没有抬高既有 `source_inventory_construct_surface.go` 或 `source_inventory_requested_surface_family.go` 的 LOC 上限；syntax-form identity抽入 70 行独立 concern文件并设置精确 70 行 ratchet，旧 construct 文件由 70 行进一步降到 64 行。`go test ./internal/types ./internal/tool/repomap ./internal/tool -count=1` 全包通过（types 18.066s、repomap 2.032s、tool 153.861s）；`git diff --check` 通过。批 L 提交推送后进入批 M，不用等待客户或额外 eval 数据。

`B5-M/P0` 施工验证：新增 `RuntimeArtifactPairRelationAuthority`，只消费 accepted runtime-artifact observation 中的 deterministic `trace_query` producer、typed `artifact_id/path` 与 endpoint-local clock metadata。canonical preflight ID优先；兼容入口仍携带占位 `artifact_id=trace_query` 时，按既有 typed path identity fail-safe 分开，不再把不同路径折成一个工件。两个独立 endpoint 即使同为 `time_domain=trace_seconds`、`canonical_domain=trace_seconds`、`alignment=identity`，pair仍明确发布 `shared_clock_origin/direct_time_alignment/shared_device/shared_capture_session=unproven`；相同 domain只保留为 `same_*_label=true`，不能升级关系。

finalizer 的 Observation Ledger 前新增共享 typed `Cross-Artifact Relation Authority` 段，明确 local identity不证明共同 clock origin，绝对时间戳相减只是一项数值 offset、不是校准后的采集间隔；该段由结构化 ledger生成，不读取 raw request、模型 thinking 或答案正文，也不做答案字符串拒绝/重写。单工件 ledger不发布 pair authority。真实 E2 两 trace 的六查询生产路径已固定：恰好生成两个 endpoint/一条 unproven pair，同时 generic comparison 仍无 publication-grade causal row、无 Trace 因果投影、无完整报告；显式窗正臂的既有 authority未变。`go test ./internal/types ./internal/agent ./internal/tool -count=1` 全包通过（types 18.089s、agent 2.533s、tool 160.768s）；`git diff --check` 通过。待提交推送并重建后严格 `parallel=2` 回放 B5 r8。

### B5 r8 人工审计与批 N（2026-07-31）

批 L `73eb2a71b`、批 M `bce716c6a` 推送并重建后，以严格 `parallel=2` 回放（sweep `20260731-085923`）：

- 结果目录：
  - `eval/results/real_trace_e2_cross_trace_asymmetry-20260731-085923`
  - `eval/results/cangjie_repomap-20260731-085923`
- runner 2/2 PASS；人工 Trace FAIL、Cangjie PASS。
- Cangjie 已真实覆盖批 L：第一轮 source inventory 和最终答案均为 `extend=2`、`foreign func=2`、`public class=8`；`@Extend(Text) highlight` 未再进入裸 `extend` family，keyword 与 marker syntax form 已分席。最终一句 `public class has 8 item(s)` 是不改变事实/成员/引用的双语模型波动，按纪律不加生产硬门。
- Trace 的 T11 继续正确：6 次 `trace_query`、零源码工具、零因果投影、零完整报告；覆盖时长、`cpu_frequency=90`、VSync 单边存在和 `31637s≈8.8h` 数值差均正确。
- 但批 M 的 typed pair authority 虽已进入 finalizer prompt，模型仍把 `unproven` 写成“时间基准不相同”“不共享校准锚点”，并称联合分析“必须以数值偏移修正为前提”。证据只能证明当前没有共同校准/session/device anchor，不能证明同或不同；裸时间戳差也不是校准变换。由此确认 T7 不是 prompt 缺字，而是 prompt-only authority 没有最终确定性发布席。

B5 r8 GAP 更新：

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B5-S10 | P0 | Parser 语法表面身份 | keyword 与 marker/decorator 共用裸 kind family | r8 精确 2/2/8，marker row 未污染 keyword 清单 | covered |
| EVAL-B5-T7 | P0 | 跨工件关系最终发布 | typed pair authority 只作为模型提示；模型可忽略 `unproven`，accepted document没有确定性 pair-level席位 | 从同一 typed pair authority在统一 document mutation choke point发布有界系统表；逐对显示 shared clock/direct alignment/device/session=`unproven`，解释 local identity与数值 offset边界；不扫描或改写题面/答案 | 批 N 已施工，待回放 |
| EVAL-INFRA-3 | P1 | Inventory exact-count oracle | runner仍可能在 extra row 时 false PASS | 批 L 产品回放已正确，但 eval 判决力缺口不因此消失；后续独立补 typed visible-rowset receipt | filed |

批 N 不变量：

1. 触发条件只读 accepted deterministic runtime observations 的 typed artifact identity 与 pair authority；禁止读取 raw request、case/path特例、模型 thinking或最终答案原文。
2. `unproven` 必须明确表示“未证明相同也未证明不同”，不能渲染为“不同”；相同 local domain label、两端 `alignment=identity` 和数值 timestamp offset均只作为证据边界。
3. pair relation block 与 causal report gate相互独立：generic comparison只增加关系边界，不获得 Trace 因果投影；显式时间窗或 publication-grade causal row仍可同时获得关系边界、因果投影、根因排序、唤醒链、窗内可消除量及自动补齐。
4. 单工件不发布 pair block；多工件按稳定顺序有界显示，模型伪造同名 block ID不能压制系统块；重复 mutation幂等。

`B5-N/P0` 施工验证：统一 `persistMergedAnswerDocument` choke point从既有 `RuntimeArtifactPairRelationAuthority` 直接构造系统 table，不读取 `RawRequest`、case、路径特例、模型思考或答案正文。表内每对分别发布 shared clock origin、direct time alignment、same device、same capture session四席状态，并将 `unproven` 确定性解释为“未证明相同也未证明不同”；相同 local time-domain label、两端 identity和数值 offset只进入证据边界。单工件负例、双工件中英文/幂等、模型保留 ID碰撞、真实 E2 六查询 generic comparison及多工件 publication-grade causal projection共存均已固定。generic E2只增加关系表，仍无因果投影/完整报告；因果正臂仍同时保留每工件 projection sections。`go test ./internal/types ./internal/agent ./internal/tool -count=1` 全包通过（types 18.937s、agent 3.313s、tool 167.200s）；`git diff --check` 通过。待提交推送、重建并严格 `parallel=2` 回放 B5 r9。

### B5 r9 人工审计与批 O（2026-07-31）

批 N `793f4157a` 推送并重建后，以严格 `parallel=2` 回放（sweep `20260731-091433`）：

- 结果目录：
  - `eval/results/real_trace_e2_cross_trace_asymmetry-20260731-091433`
  - `eval/results/cangjie_repomap-20260731-091433`
- runner Trace PASS、Cangjie FAIL；人工两项均 PASS。
- Trace 已真实覆盖批 N：最终可见确定性表恰有一个工件对，shared clock origin、direct time alignment、same device、same capture session四列均为“未证明”；边界明确 local `identity`、相同 `trace_seconds` 标签和数值 timestamp差都不能证明共享 clock。generic comparison仍无 Trace因果投影或完整报告，T11不回退。
- Cangjie 产品答案仍精确为 2/2/8，所有符号名、路径、package与引用正确。runner FAIL 的直接原因是 case把构造显示名 `extend String/Cart`当“符号名”，而确定性归一表按用户要求正确显示 `String/Cart`；更深层原因仍是 `EXPECT_INVENTORY_COUNT_*`用“expected rows matched”冒充 visible exact cardinality。
- Trace 同轮暴露 P1 次级 gap：系统校验附注把 `clock_alignment`写成“未在本报告证据面出现”，但 deterministic SourceRef 与系统 pair block均已携带该 typed 字段。cross-check lexicon board没有吸收 accepted source-reference clock metadata。

B5 r9 GAP 更新：

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B5-T7 | P0 | 跨工件关系最终发布 | prompt-only authority可被模型忽略 | r9 visible system pair table正确发布四席 unproven，且generic/causal权限分席 | covered |
| EVAL-INFRA-3 | P1 | Inventory exact-count oracle | matched expected rows不等于实际可见行数；section extractor又只识别`#`标题 | matching section同时识别 Markdown heading与单行 bold heading；若存在结构化 table/list，计数真实 data rows并与 expected exact count比较，missing/banned row仍独立；无scoped structured carrier才回退legacy matched count | 批 O 已施工，待提交 |
| EVAL-B5-T12 | P1 | 系统交叉校验证据词表 | accepted Observation SourceRef中的 time domain/alignment元数据未进入 lexicon board，系统把自身 typed术语误报成未知 | 把 accepted deterministic source-reference元数据投影到 cross-check evidence lexicon；只消除有 typed witness的误报，不扫描题面、不放宽未知术语 | 批 P 已施工，待回放 |
| EVAL-B5-P4 | P2 | Analyzer过程波动 | 同一完整清单 r8为1次 source lens，r9为3次 lens+1次 list，结果均正确 | 先观察后续高优先级case复现率；不为一次模型探索波动加硬门 | filed-model-variance |

`B5-O/P1` 施工验证：rowset section extractor新增单行 `**标题**`、blockquote bold与尾随中英文冒号边界；真实结构化 section内分别计算 Markdown table data rows或bullet rows，header/separator/citation尾段均不计。成员 token、banned row与visible exact count成为三个独立判据：同基数错成员只报 missing，expected全命中但多一个row会报真实 `got2:want1`，从而关闭旧 false PASS。Cangjie case的 extend成员改为用户所求的符号身份 `String/Cart`，category仍由section提供，不把展示构造词塞进 symbol列。`bash -n eval/runner_lib.sh eval/run.sh eval/runner_lib_test.sh`与`bash eval/runner_lib_test.sh`通过；对r9已保存答案重放 `eval_inventory_rowset_reasons`零错误。待提交推送。

`B5-P/P1` 施工验证：新增单一 SourceRef→lexicon投影器；对每条 accepted ledger record，仅当对应 typed字段实际非空/非nil时，才发布 `tool_call_id/raw_ref/payload_ref/row_set_ref/page_ref/artifact_id/artifact_kind/time_domain/canonical_time_domain/clock_alignment/clock_calibrated/clock_offset_sec/clock_slope/resource_uri/mime_type`等字段名及值。不存在的字段不进入词表，故邻接负例 `clock_slope` 仍产生 information finding；不存在全局白名单。该 lane原有 information-only、零 hard reject、零答案改写语义不变，也没有读取 case或模型答案来决定 typed字段是否存在。定向 present/absent臂与既有附件近似词负例通过，`go test ./internal/orchestrator -count=1` 全包通过（10.972s）；`git diff --check`通过。待提交推送并在下一次高优先级eval中观察，不单独为文案回放占用一批。

### B6 r1 人工审计与批 Q（2026-07-31）

在批 O `6420f185d`、批 P `2a742dc72` 已推送并重建后，以严格 `parallel=2` 执行：

- 结果目录：
  - `eval/results/qf_config_precedence-20260731-093120`
  - `eval/results/patch_java_typo-20260731-093120`
- runner 2/2 PASS；人工 Java PASS、config partial。
- Java ChangePlan 仅含 `Main.java` 第16行 `retrun→return`，一个 patch change、一个 replace edit、一个 slice；原始 patch 用 `jq -jr` 提取后通过 `git apply --check --recount`。两次 plan repair 是模型把 Python `subprocess` 代码标成 Java probe，repair pack正确拒绝，第三次改为 Python probe通过。该过程波动记 `EVAL-B6-W1/P2`，当前不加生产硬门。
- Config 答案的事实与五个源码位置一致：默认值50、YAML `*int` 映射、YAML merge、CLI Changed保护和 code<YAML<CLI 顺序均正确。`cmd/root.go:649` 未读证据和错误 `anchor_kind=call` 的拒绝属于正确 fail-closed；decorated member无 refs的 completion修复也正确。
- 真正缺口在 typed citation wiring：模型提交6条 citations，但只给 scalar item连接一条。accepted positional `support_refs` 含 `cmd/root.go:88`、`codrax.yaml.example:485`、`cmd/root.go:649`；解析器无法识别 `.yaml.example` 后压缩 bare-ref集合，使另外两个有效槽也失去 location。Principal Enumeration Rows 因而不携 location/citation_key；完整 Markdown 表的 typed 行又未计入 citation 使用集合，unused-prune 删除其余5条引用。最终事实正确，但机制证据覆盖不完整。

B6 r1 GAP 与处置：

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B6-C1 | P1 | positional support-ref 隔离 | 先丢不可解析 ref再按压缩后数量判断 positional binding，一个未知槽毒化全部兄弟槽 | 当 `len(support_refs)==len(members)` 时先按原索引独立解析该槽；合法 label/空 label/generic label按既有成员相容规则绑定，失败只影响本行 | 批 Q 已验证 |
| EVAL-B6-C2 | P1 | 配置模板路径语法 | source/config location只看最后扩展名，`codrax.yaml.example` 被当 `.example` 非配置文件 | 在统一 path suffix authority中支持声明式配置扩展+有界模板装饰后缀；源码 `*.go.example` 与任意 `.example` 保持拒绝 | 批 Q 已验证 |
| EVAL-B6-C3 | P1 | Markdown row citation接线 | 完整 model-authored Markdown table可满足成员可见性，却没有原生 row citation_ref，pool被清理 | unused-prune 把已accepted EnumerationDisplayRow且已在同表可见的行计为 citation 已使用；坐标只来自typed row，表格文字只确认现有carrier，不创建成员/坐标/结构 | 批 Q 已验证 |
| EVAL-B6-W1 | P2 | write probe过程波动 | 模型连续把 Python subprocess代码声明成Java inline probe | repair pack已安全收敛；先观察其他语言case，若复现再加强language/code-shape软指导，不加case/type硬门 | filed-model-variance |

批 Q 不变量：

1. 不读取 raw request、case ID、模型 thinking或自由文本来铸成员/引用；成员和坐标只来自 accepted aggregate facts、typed support refs和grounded evidence。
2. Markdown table text只用于确认 accepted row已有现成可见 carrier；不新增隐藏 item、不改写表格、不增加/删除成员，citation 使用判定重复执行幂等。
3. 模板后缀只放行声明式配置格式，不把任意 `.example`、runtime artifact或源码伪装成当前源码引用；最终 current-source quote/物理行校验继续 fail-closed。
4. 本批不修改 runtime request model、Trace query选择、显式时间窗、因果投影、根因排序、唤醒链、窗内可消除量、跨工件关系或自动补齐。

`B6-Q/P1` 施工验证：`aggregateMemberStructuredLocation` 在一成员一槽的 typed positional形上先解析原索引，未知兄弟槽不再改变有效槽坐标；后续 named/generic/bare fallback保持。统一 suffix authority新增声明式配置模板装饰的有界组合，`codrax.yaml.example`、`settings.toml.dist`、`schema.json.sample`可作为配置证据路径，`source.go.example`、`notes.txt.example`、`binary.example`仍拒绝。完整 Markdown table在 accepted row可见且row有typed source时，由 unused citation 清理器将对应精确坐标计为已使用；表格 `Items` 保持为空，既有多列布局和正文不变，重复执行幂等。无source的runtime mode成员不生成citation。专项 tests与 `go test ./internal/types ./internal/tool ./internal/agent ./internal/orchestrator -count=1` 均通过；`git diff --check`通过。待提交推送和后续高优先级双case观察。

### 第二阶段优先级（B7 起）

首阶段 12 个 case 已全部执行。下一阶段继续按客户影响、机制覆盖、回归证据、oracle 判决力和过程诊断价值排序；分数只排执行顺序，不作质量门：

| 顺位 | case | 主要维度 | 选择理由 | 批次 |
|---:|---|---|---|---|
| 13 | `trace_query_wakeup_causal_io_chain` | 显式窗/根因排序/唤醒链 | 同时验 D/IO 终端根因、四跳链、优先级语义和显式窗因果投影，是前轮收窄的最高风险正臂 | B7 |
| 14 | `github_issue_dayjs_duration_nan_symptom` | write JS/症状定位/验证权限 | 跨语言症状定位，边界缺字段语义明确，可审验证不可用与 proof authority | B7 |
| 15 | `real_trace_d2_chain_via_networkservice` | 真实 trace/多跳链 | 真实 Donghu 数据验证 NetworkService 中转链，避免只在合成四行链上成立 | B8 |
| 16 | `data_join_entity_reconcile` | data 谱系/贡献账 | 精确单值、贡献记录和 reconcile 三重 oracle，覆盖非 LLM 算术闭环 | B8 |
| 17 | `trace_query_frame_timeline_flow` | 帧链/跨线程 flow | Expected/UI/RenderService/GPU 四阶段，检验 span/帧能力与调度因果面互不污染 | B9 |
| 18 | `read_combo_trace_current_code_boundary` | trace+当前源码双权威 | 同时要求 artifact 时间/行和 current-source citation，能杀跨工件坐标串用 | B9 |
| 19 | `real_trace_d4_demand_vs_supply` | 真实 trace/供需归因 | 真实窗睡眠、系统 idle 与频率上限共同约束需求侧结论 | B10 |
| 20 | `github_issue_libgit2_foreach_worktree_symptom` | write C/错误码传播 | 检验跨语言调用链和负数错误码保真，不与 Java/JS 修复形状同类 | B10 |

### B7 r1 人工审计与批 R（2026-07-31）

- 严格 `parallel=2`，结果：
  - `eval/results/trace_query_wakeup_causal_io_chain-20260731-095616`
  - `eval/results/github_issue_dayjs_duration_nan_symptom-20260731-095616`
- runner 2/2 PASS；人工 Trace FAIL、write 产品修复 PASS / 控制面 partial。
- Trace 引擎真值正确：链 `threadpool-400 → network-300 → cookie-200 → app-100`，edge wakeup 为 2.016/2.018/2.020s，终端 D/IO 为 2.003–2.014s、11.000ms。显式窗因果投影、目标四态和 window_stats 自动补齐均发布，证明状态查询收窄未伤显式窗正臂。
- Trace 主答案却把前两条 wakeup 提前为 2.015/2.017s，并把不同起点的累计 latency 称为边传导延迟；查询窗 20.000ms 与窗外恢复点 20.020ms 也混写。自动 oracle 未约束 exact edge timeline。
- 确定性投影还将同一 threadpool IO 段发布两席：E4 `wakeup_causal_impact` 与 E5 `root_evidence:io_wait`；现有同线范围 R1 因 6–9 vs 6–7 未吸收。
- Write diff 仅一行 `Number(value) → Number(value ?? 0)`，Python oracle 基线失败/修后通过，产品修复正确。两个 JS 行为 probe 和 npm 均 unavailable；proof review 正确留下 3 个 contract 与 2 个 symbol 未覆盖义务，但 verify-only follow-up 重复同一弱验证后错误升级 `batch_verified/all_verified`。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B7-T1 | P1 | wakeup edge 时间权限 | typed edge 有精确 wakeup_ts，但最终主答案未稳定消费，模型混用 sched-in/累计 latency | typed edge 进入最终 handoff/确定性边展示；不扫正文、不按 case/数值特判 | filed |
| EVAL-B7-T2 | P1 | 因果投影物理段双席 | causal impact 与 root evidence 同物理 D/IO 段因 source span 一宽一窄逃过 exact-range R1 | typed subject/state/value/artifact/window 加 interval containment 的单一吸收点，证据 lossless、展示一席 | filed |
| EVAL-B7-T3 | P2 | 窗口/实际口径 | 模型把 20.000ms 窗内投影与窗外 20µs 恢复点混写 | 先跨 case 观察；确定性 projection 已分口径，不为一次波动加正文硬门 | filed-model-variance |
| EVAL-B7-W1 | P1 | proof follow-up authority | report-level source check pass 覆盖 typed proof ledger 的 unavailable/uncovered，重复弱验证被升级 all_verified | 仅对 typed verify-only proof purpose，以 proof ledger closed 为 verified 条件；未闭合完成为 unverified | 批 R 已施工 |
| EVAL-B7-W2 | P2 | proof follow-up 效率 | verify-only 批只能复跑旧 plan，稳定 runner-missing 时通常无 proof delta | W1 先诚实降级；观察后再按 typed capability/no-delta 决定是否跳过重复执行 | filed |

批 R 不变量：

1. stricter gate 只由 batch `execution_mode=verify_only`、typed `purpose` 与 `VerificationProofLedger` 启用；普通实现批的 report-level pass 语义不变。
2. 不读取用户输入、模型 reasoning、最终答案或测试输出关键词；runner/probe/contract/symbol 状态均来自 schema 化报告与 proof ledger。
3. 未闭合 proof 不等于代码失败，不触发错误 replan；完成 verdict 为 unverified，并保留已应用补丁与可用的 source-check 证据。
4. 不修改 read/trace 路由、显式时间窗、因果投影、根因排序、唤醒链、窗内可消除量和自动补齐。

`B7-R/P1` 施工验证：新增 proof-followup outcome reconciliation；只有 verify-only proof batch 且 ledger state=verified、uncovered/unavailable/failed 均为0时保留 `report_passed`，否则转 typed `verification_incomplete(reason=verification_proof_incomplete)`，attempt/slice/batch 记录为 unverified。相邻负例固定普通实现批不受影响，正例固定强 project runner proof 仍通过。专项 tests及 `go test ./internal/orchestrator -count=1` 通过；待提交推送。

### 批 S：精确唤醒边进入确定性因果投影（2026-07-31）

对 B7-T1 冷读后确认，问题不是 `trace_query` 缺数据：每条
`wakeup_chain_edge` 的 `ObservationSpan` 已携精确 `StartTs/LineStart`，
但 `traceCausalProjectionWakeupEdge` 只抄 waker、wakee、evidence ID 和
relation，编译时永久丢失唤醒点。因果树又以“上游子节点唤醒父节点”
组织，不能把子节点自己的下一跳时间直接拼到当前树边，否则会把
`network→cookie` 的时间贴成 `threadpool→network`。

批 S 的统一方案是在被唤醒节点的 typed drilldown relation 上保留：

- `DrilldownWakeupPointKnown`：独立 presence bit，保证 trace 合法
  `0.000000s` 不被零值误判成 unavailable；
- `DrilldownWakeupTs`：直接来自 accepted `wakeup_chain_edge` 的
  `ObservationSpan.StartTs`；
- `DrilldownWakeupLine`：直接来自同一记录的 `LineStart`。

逐节点 lossless 明细新增“直接上游唤醒点”，确定性输出
`waker → wakee @ 2.020000s（trace 行15）`。path-only fallback不生成
时间；多个不同 waker继续使上游关系 ambiguous；同一 waker若存在多个
不同 typed 时间点，则仍保留唯一上游身份，但清空精确时间/行号和单一
evidence ID，禁止按记录到达顺序任选一个。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B7-T1 | P1 | wakeup edge 时间权限 | typed edge 的点坐标在 projection compile 时丢失，模型只能从多种时长/切入点自由组合 | 在被唤醒节点的 typed direct-upstream relation 保留 presence+ts+line，并在 lossless 明细确定性发布；缺失/冲突不猜造 | 批 S 已施工，待 B8 真实 trace 观察 |

批 S 不变量：

1. 不改变 full-report authority、显式时间窗优先级、query 选择、自动补采、根因排序、链节点选择或窗内可消除量；只增加 accepted typed edge 已有坐标的无损携带和显示。
2. 不读取 raw request、case ID、模型 thinking、final answer 或非结构化说明来判断时间；唯一数值源是 `ObservationSpan`。仅在时间恰为零、span 零值无法表达 presence 时，用注册过的 typed `wakeup_ts=0` note 证明“零确实存在”，note 不替代数值源。
3. path-only wakeup chain只能提供拓扑，不能铸时间；点冲突 fail-closed，不以 epsilon、频次或到达顺序挑选。
4. 时间 presence 与数值分席，已知 `0s` 可发布，unknown不能渲染成 `0.000000s`。

`B7-S/P1` 施工验证：types 正例固定 app/worker 两跳分别保留
`2.020s/line15` 与 `2.016s/line11`；同一 waker 的冲突点固定只保留
上游身份、不发布时间；原多 waker ambiguous 负例保持。renderer
中英文精确格式与 unknown/known-zero 两臂均固定；line-only 负例不能
伪造 trace-zero，注册过的 typed zero witness 正例可保留 0s；info-contract census
登记三字段显示面，`wakeup_ts` carrier 由 display-only 同步升格为
hard-consumer，避免编译消费与 note-key 契约漂移。
`go test ./internal/types ./internal/tool -count=1`全包通过
（types 21.167s、tool 162.614s）；`git diff --check`通过。

### B8 r1 人工审计与批 T（2026-07-31）

批 S `8da6b7ba6` 推送、重建并通过 revision/clean-input 校验后，以
严格 `parallel=2` 执行：

- `eval/results/real_trace_d2_chain_via_networkservice-20260731-103137`
- `eval/results/data_join_entity_reconcile-20260731-103137`

runner 2/2 PASS；人工两项产品正确。

Trace 真值为
`ThreadPoolForeg-60555 → NetworkService-60595 → CookieMonsterCl-59843 → com.baidu.tieba-59566`，
NetworkService 明确是中转节点。该请求虽未显式给时间窗，但 typed
question kind 是 call-chain/diagnostic 且存在真实 wakeup path/causal rows，
所以需要并正确获得完整 `Trace 因果投影`；这不等于把普通状态查询重新
套进全量合同。查询3次、源码读取0次，投影继续保留目标状态账、链上归因
和边界。

模型主答案将4个节点称作“4跳”（实际3条边），并把 lower-priority-waker
候选写成较强的“优先级反转风险”；确定性树仍使用候选词且链方向正确。
当前按 model variance 记录，不扫描答案纠字。真实重复链同时验证批 S 的
fail-closed：同一 waker/wakee pair 在窗内有多个不同 edge point，单点
relation没有任选一个时间，因此明细未生成“直接上游唤醒点”。要完整覆盖
此类场景，后续应从 typed edge/census 建设有界 count+first/last 多点口径，
而不是放宽单点冲突门。

Data 最终严格只输出 `30`；terminal 为 complete，rule=1、
entity resolutions=3、contributions=2、decisions=5、reconcile=pass，
贡献和对账正确。过程用了9个执行批、171s，并保留5条 prior error：
前三条是模型先给 typed action 携 script/跨 rule prerequisite，后两条是
模型提交跨多个 DAG stage 的批次。系统后续已将合法 prefix/remainder
确定性拆批并顺序执行，没有结果污染；在仍需逐 rank 观察真实字段的安全
约束下，本轮先把它记为 P2 效率项，不为了一个小 fixture 合并依赖 stage。

人工审计发现 runner 自身有两个确定性假阴性：

1. `json_string_field` 用 `[^"]*` 解析 JSON string，遇到
   `result_summary` 内合法的 `reconcile=\"pass\"` 时只得到尾部反斜杠；
2. failed-action 计数只认小写 `"status"`，而 terminal 的
   `action_events` 实际序列化为 `"Status"`，所以一个 failed event 被记成0。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B7-T1 | P1 | 重复 wakeup edge 时间权限 | 单点 carrier 对同 pair 多次真实唤醒必须 fail-closed，因此重复链仍无有界 exact-time 摘要 | typed pair roster/census 建 count+first/last，并与单点 known 分席；不得任选一次 | partial；单点 covered，多点 filed |
| EVAL-B8-T4 | P2 | Trace 模型措辞 | 4节点被称4跳、候选被写成风险 | 确定性投影正确；等待跨 case 复现，不扫描正文硬改 | filed-model-variance |
| EVAL-B8-D1 | P2 | Data workflow 效率 | 模型反复提交跨 stage plan；系统安全拆成逐 rank，正确但9批/171s | 保留真实字段逐 rank gate；先观察后续 data case，若复现则增强 typed plan scaffold或对已验证同 rank做批量调度 | filed |
| EVAL-INFRA-4 | P1 | Data terminal JSON metrics | sed 不能解析 escaped JSON string，action status 字段大小写又与terminal真实形不一致 | 共享 JSON decoder读取顶层 string；failed counter同时消费既有 `Status/status` 两种序列化形 | 批 T 已施工 |

批 T 不变量：

1. 只修 eval 审计读取，不改变产品路由、data workflow、Trace authority、
   显式时间窗、因果投影、自动补齐或答案。
2. JSON string 必须由标准 decoder 解转义；没有 Python 时保留既有
   sed fallback，不把 parser 缺失变成 case 产品失败。
3. failed 计数仍只限 `action_events` 区间，不把 prior error、guard
   记录或 `action_graph` 中其他状态误计为执行失败。

`B8-T/P1` 施工验证：runner helper fixture 固定 escaped
`reconcile=\"pass\"` 解码成完整字符串，并在同一 terminal 中将一个
`Status=failed` 与一个 `status=failed` 精确计为2、executed不计。
`bash -n eval/runner_lib.sh eval/run.sh eval/runner_lib_test.sh`及
`bash eval/runner_lib_test.sh`通过；待提交推送。

### B9 r1 人工审计与批 U（2026-07-31）

批 T `05d6f4f90` 推送、重建并通过 revision/clean-input 校验后，以严格
`parallel=2` 执行：

- `eval/results/trace_query_frame_timeline_flow-20260731-104036`
- `eval/results/read_combo_trace_current_code_boundary-20260731-104036`

runner 2/2 PASS；人工 Frame FAIL、mixed trace/current-source partial。

Frame case 的四个 B/E span 时间线和角色分类正确，但输入没有 async S/F
cookie、scheduler/binder edge、官方 flow ID 或其他显式连接器。旧
`buildFrameTimelineFromPipeline` 仍把时间排序后的每对相邻 span 无条件写成
`FrameFlowEdge`，其 JSON 只有端点、phase、latency、line 和 summary，
没有 relation kind/source/causality。主答案遂错误宣称
UI→RenderService→GPU 的跨线程 flow 已确认且“无丢失跳”。这是 engine
权限泄漏，不是单次模型措辞波动。

Mixed case 正确算出 86.111ms，且源码机制和 trace/source 文件名空间总体
分开；但 principal scalar `86.111ms` 的 item 把
`internal/tracequery/query.go:20636` 作为 citation。该行只证明
`BuildFrameRootCauseBundle` 调用机制，不能证明客户 trace 的 86.111ms。
Analyzer 已正确发出 active `artifact_value_profile`，scalar block 也声明
`claim_form=external_observation`；缺口是 mixed-origin 模式只在整份答案
级保留 current-source lane，没有按 item 的 typed origin 对齐引用。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B9-T1 | P1 | 帧边因果权限 | 相邻 span 被命名为 flow，但边无 connector/causality authority，模型可把顺序升级为因果 | 边增加 typed relation kind/source/causal conclusion；B/E 相邻固定为 temporal_sequence/unproven；JSON、evidence、工具文本、系统覆盖块同源发布 | 批 U 已施工 |
| EVAL-B9-C1 | P1 | mixed-origin item 引用 | mixed 请求需要保留源码解释，旧 cleanup 仅整份答案判定，artifact scalar可借用兄弟机制项的 current-source citation | active artifact value profile + scalar block + external_observation claim_use 作为精确门，仅移除该 item 的 current-source ref，保留正文与源码解释引用 | 批 V 已施工 |
| EVAL-B9-W1 | P2 | 帧 oracle 判决力 | runner 只检查阶段词和 flow 文本，未约束因果权限，错误过度结论仍 PASS | 批 U 后让 fixture要求确定性 authority surface；eval oracle只验系统发布字段，不作为产品正文关键词硬门 | filed |

批 U 不变量：

1. 不读取 raw request、case ID、模型 thinking/final answer；边权限只由
   engine 构造时实际使用的 typed connector 决定。
2. 保留 `frame_flow` view/JSON 数组作为兼容入口，但现有相邻 span 边明确
   是 `relation_kind=temporal_sequence`、
   `relation_source=sorted_complete_span_adjacency`、
   `causal_conclusion=unproven`；后续显式 connector 可独立获得更高权限。
3. 不改变 span 发现、角色分类、目标选择、显式时间窗、Trace 因果投影、
   根因排序、唤醒链、窗内可消除量或自动补齐；只收回原本不存在的跨线程
   因果权限。
4. `frame_causality=unproven` 的原“帧证据 absent/unavailable”语义保留；
   新增的是“帧存在但边仅时间相邻”的分席权限，不能混为无帧。

`B9-U/P1` 施工验证：`FrameFlowEdge` 新增三项 typed authority；相邻边
summary 改为 `frame temporal sequence ... causal_link=unproven`，
EvidenceFact predicate 改为 `frame_temporal_sequence`，工具文本明确输出
relation/source/conclusion。`TraceEvidenceAuthority` 汇总 edge count 和
relation ceiling，系统覆盖块确定性发布中英文“不能升级为已确认的跨线程
因果 flow”。工具 schema 参数说明同步纠正；PIN-1 检查确认独立的
Description byte golden 未发生变化（本批没有占用 Description 槽）。
该变更是既有错误权限的撤回，不引入 prompt case 词或硬门。
专项 tracequery/tool/types tests 已通过；批 U 已由 `2beea8634` 提交推送。

### 批 V：运行时 scalar 与源码机制引用分席（2026-07-31）

批 V 在已有 document-level mixed-origin authority 之下增加 item-level
对齐。触发条件全部来自 typed carrier 的合取：

1. `RuntimeArtifactValueProfile.Active()`；
2. `AnswerBlock.Kind=scalar`；
3. block 的 `ClaimUses` 含 `ClaimExternalObservation`；
4. item 的 citation 是当前仓库内的正行号 source citation，且不是 typed
   attachment spelling 或 runtime-artifact path。

命中时只把该 item 的 `citation_ref` 置为 -1。citation pool entry 不删除，
兄弟 summary/section 对同一源码行的机制引用继续有效；scalar 文本也不删除。
新的 typed disclosure kind `evidence_origin_mismatch` 在最终 persist 后按同一
item identity 判断内容是否仍可见，并诚实说明“源码可解释机制但不能证明
该次运行时测量”。未命中时 byte behavior 保持。

批 V 不变量：

1. 不扫描 scalar 文本、数值、用户输入、模型输出、case ID 或文件内容来
   判断 origin；尤其不对 `86.111`、`DoFrame` 或 `50ms` 做特判。
2. active profile 但 source/literal claim 的 scalar 不受影响；artifact
   citation、仓外 external source、inactive profile 均不受影响。
3. 不清除 mixed request 的 current-source evidence lane，不影响源码机制
   citation supplement、current-status 检查或 source quote 校验。
4. 不修改 Trace query、显式时间窗、因果投影、自动补齐、根因排序、唤醒
   链和窗内可消除量。

`B9-V/P1` 施工验证：正例固定一个 artifact scalar 错借源码 ref，同时同一
ref 被机制 summary 正确使用、另一个 artifact provenance ref 正确保留；
三者结果分别为 detached/kept/kept。负例覆盖 inactive profile、
source-literal claim、artifact citation、仓外 external path。production
normalize chain pin 固定 pass 接线与 typed disclosure ferry，防止只留下
孤立 helper。`go test ./internal/types ./internal/tool -count=1` 全包通过
（types 20.670s、tool 164.148s）；批 V 已由 `e9936bf6f` 提交推送并完成
B9 r2。

### B9 r2 人工审计与批 W1（2026-07-31）

批 U/V 推送、重建并通过 revision/clean-input 校验后，以严格
`parallel=2` 重跑：

- `eval/results/trace_query_frame_timeline_flow-20260731-110319`
- `eval/results/read_combo_trace_current_code_boundary-20260731-110319`

runner 2/2 PASS。Frame 主问题已修复：四段仍完整，三条边明确为
`temporal_sequence`、`causal_conclusion=unproven`，最终答案也明确禁止
把它们升级为已证实的 UI→RS→GPU 因果依赖。但系统覆盖块显示
`edges=8`，而完整视图实际只有3条边。原因不是 trace 数据错，而是覆盖
聚合把 pid-filtered/full 两轮的 timeline/flow 四份 per-view authority
按 `1+1+3+3` 求和。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B9-W1 | P1 | 多查询 census 聚合 | per-view 数量是重复/嵌套视图的局部 census，跨 ToolResult 求和会把重复读取伪装成新增事实 | 跨结果取最完整单视图 `max`；auto/combined 结果若确有子视图聚合，已在自己的 authority 内给出总数 | 批 W1 已施工 |
| EVAL-B9-W2 | P1 | mixed-origin 引用 | 批 V 把来源门绑定 `BlockScalar`，模型换成 decision 形后同一 external observation 又借用源码引用 | 门提升为“block 的非空 claim-use 全部为 external_observation”，不依赖块类型 | 批 W2 已施工 |
| EVAL-B9-W3 | P1 | error-granularity 适用性 | diagnostic scalar 被误当作失败范围的专属形，阈值判断被渲染成 `per_item_rejection/逐条拒绝` | 修正 typed applicability：普通 scalar 不等于 item-vs-batch failure scope；保留真正 return-value failure-scope lane | 批 W3 已施工 |
| EVAL-B9-W4 | P2 | 数值关系模型波动 | 本轮模型声称 86.111ms 大于 100ms，和数值本身矛盾；r1 未复现 | 记录为 model variance；后续若跨 case 复现再建设 typed numeric relation carrier，不扫描正文硬改 | filed-model-variance |

批 W1 不变量：

1. 不改变任何 trace query、span/edge 构造、显式时间窗、Trace 因果投影、
   自动补齐、根因排序、唤醒链或可消除量；只修系统覆盖块的跨结果 census
   代数。
2. 不按 view 名、case、请求或答案正文去重；聚合只消费
   `TraceEvidenceAuthority.FrameFlowEdgeCount`。
3. 同一个 ToolResult 内已经由 auto/combined view 汇总的合法总数保持；
   多个 ToolResult 之间选择 typed 最大完备视图，避免重复观察扩张基数。

`B9-W1/P1` 单测固定四份 authority 的 `1/1/3/3` 输入，覆盖块必须发布
`edges=3` 且禁止回到 `edges=8`；批 W1 已由 `802abadb6` 提交推送。

批 W2 将批 V 的 item-level origin 对齐从 scalar 特例提升为 claim-use
不变量。`BlockScalar/Decision/Summary/List` 只要非空 `ClaimUses` 全部是
`external_observation`，其中引用当前仓库源码的 item 都会移除这条不兼容
引用；只要混有 `definition_fact/call_edge/literal_value_fact` 等源码机制
claim，则保持源码引用，禁止把混合块误当纯观测块。

批 W2 不变量：

1. block kind 不再参与来源权限；渲染形变化不能改变同一事实的证据 origin。
2. 空 claim-use fail-open，混合 claim-use 保留源码引用；不从正文猜 claim。
3. artifact citation、仓外 external source、inactive artifact profile 与兄弟
   源码机制块保持不变；仍只拆 item→citation edge，不删正文或 citation pool。
4. 不影响 Trace query、显式时间窗、因果投影、自动补齐及所有调度分析面。

批 W2 已由 `12ac1305d` 提交推送。

批 W3 修正 `errorGranularityHasDedicatedFailureScopeAnswerShape` 的 typed
语义：`is_scalar_answer` 只表示答案基数，耗时、阈值、布尔、状态和真正的
failure-scope verdict 都可能是 scalar，因此它不能单独证明用户问的是
单条/整批失败范围。diagnostic/mechanism 形中的误发 profile 现在会在
analysis 接线处软化；`IntentReturnValue` 或 `ReqReturnValue` 的真正
failure-scope 请求仍保留 canonical decision 合同。

批 W3 不变量：

1. 不检查 source quote、rationale、原始请求或最终答案词面；只调整既有
   typed family applicability。
2. 非 diagnostic 的 active failure-scope profile 仍直接生效；diagnostic
   下具有 return-value family 的请求也继续生效。
3. 软化只移除误发的 `ErrorGranularityProfile`，不改变 scalar value、
   artifact observation、current-source explanation 或 Trace 能力。

批 W3 已由 `26585b71f` 提交推送。三批合并后
`go test ./internal/types ./internal/tool -count=1` 全包通过
（types 21.482s、tool 159.153s）。

### B9 r3 人工审计与批 X（2026-07-31）

批 W1/W2/W3 推送、重建并通过 revision/clean-input 校验后，以严格
`parallel=2` 第三次重跑：

- `eval/results/trace_query_frame_timeline_flow-20260731-111803`
- `eval/results/read_combo_trace_current_code_boundary-20260731-111803`

runner 2/2 PASS；人工两项仍为 FAIL。这个差异再次证明 fixture 的
`answer_contains/answer_regex` 只能证明最低表面词存在，不能替代因果权限、
证据来源和引用真假审计。

Frame 的确定性 census 已正确收敛为 `edges=3`，说明批 W1 生效；四个
span、三个 `temporal_sequence` 边、40ms 总窗均正确。但模型正文再次把
它们写成“形成完整 UI→RenderService→GPU flow”“Flow 验证”“每一段切换
都有时间戳证据支撑”，与同答案确定性覆盖块
`frame_flow_causality=unproven` 冲突。r2 模型曾正确服从同一权限，r1/r3
未服从，属于软引导承载不足叠加模型波动，不能用扫描最终答案词面的硬门
修复。另有 pretriage/analysis 将真实 11ms Choreographer span 写成 12ms
的过程数值漂移，最终主列表为正确 11ms，但尾部 caveat 又出现 12ms。

Mixed case 的三项前修均生效：

1. principal 86.111ms 外部观测不再借源码 citation；
2. 普通 diagnostic scalar 不再触发 `per_item_rejection/逐条拒绝`；
3. 本轮正确写成 86.111ms > 50ms，未复现 r2 的 >100ms 算术错。

但 patch 追加了一条
`attached_trace-44d2a269.txt [absence: H:RenderService:DoFrame]`，同一
工件的 typed trace observation 明确证明该 span 位于 lines 5–6。根因有
两层：Codrax 自己的 `StoreBlobArtifact` 生成
`attached_trace-<8hex>.txt`，原 runtime-artifact path authority 只识别
固定 basename；同时 full emit 的未使用 citation 裁剪故意不应用于 patch，
所以这个未被 item 引用的错误 negative proof 留在引用附录。曾尝试把 full
裁剪直接移入 shared persist（`d2cc87c01`），完整回归发现它破坏4个已固定
的继承 citation pool/index 合同，已由批 X2 收窄纠正，不能以更新测试掩盖。

Mixed 正文还把“窗内没有 sched/binder/IO/lock 证据”升级成
`heavy-compute`、`pure computation` 和 `1/1 jank=100%`。日志中的
`pretriage_model_extraction` 已明确是 navigation-only，typed
`root_cause_rank` 也只有 `tier=context_only`，这是模型没有服从证据权限，
当前按 P2 跨 case 观察；不为一个 span 名、一个数值或一个 fixture 增加
正文关键词扫描/替换。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B9-X1 | P1 | artifact path authority | Codrax 内容寻址的 `attached_trace-<8hex>.txt` 未被识别为系统保留 runtime artifact，词法上落在 repo 内时可误入 current-source lane | 类型层按保留 stem + 固定8位 hex + `.txt` 识别 StoreBlobArtifact 结构名；current-source 仍须 stat 为真实普通文件 | covered：`acec852d8` + `12b3bfa3a` |
| EVAL-B9-X2 | P1 | patch negative citation 真值 | patch 保留 citation pool 是索引合同，但未使用的 runtime absence proof 即使被工件正内容直接反证仍可渲染 | 保留 patch pool/index；仅对未被结构化 item 使用、scope=negative、保留 runtime blob、pattern 在绑定工件内正匹配的条目剔除；64MiB 流式上限、不可读/非法 pattern fail-open | covered：宽修 `d2cc87c01` 已由 `12b3bfa3a` 收窄纠正 |
| EVAL-B9-X3 | P1 | frame authority 软承载 | typed unproven 已确定性展示，但 finalizer 通用指导不足，模型跨轮可把时间相邻升级为已证 flow | guidance view 携 typed unproven/relation/max-count，明确图边只能标 temporal adjacency (unproven)；保持软引导，不扫描正文硬拒绝 | covered：`c3637976f`，待 r4 |
| EVAL-B9-X4 | P2 | pretriage/model 权限 | navigation-only 预抽取和“缺少其他事件”被模型升级成 heavy-compute、pure computation、100% jank；11ms 又漂移为12ms | 保持 typed observation/root-cause ceiling；先跨高优先级 trace case 观察，复现后建设统一的 pretriage claim authority carrier，不做 span/type 词面补丁 | filed-model-variance |

批 X 不变量：

1. 不读取 raw request、case ID、模型 thinking 或最终答案正文作为判定信号；
   negative citation 核验只消费结构化 `scope/negative_pattern/file`、
   Codrax 保留 blob 结构名、stat 绑定路径与工件正匹配。
2. 不把 full emit 的全量未使用 citation 裁剪推广到 patch；既有 inherited
   pool 顺序和 citation_ref 索引继续保持。只有未被引用且自证矛盾的 runtime
   negative proof 可删除。
3. 工件扫描有64MiB上限；路径不可绑定、pattern非法、超过上限均
   fail-open，不猜造 absence/presence。
4. 不改变 trace query、显式时间窗、Trace 因果投影、自动补齐、span/edge
   构造、根因排序、唤醒链或窗内可消除量。
5. Frame 因果限制仍是 typed soft guidance + 确定性 authority block；
   不对“形成/验证/flow”等自然语言做硬门或自动替换。

批 X 回归：新增系统保留内容寻址名正负矩阵、未引用矛盾 negative proof
删除、真实 absent proof/source negative proof 保留、referenced inherited
entry 索引保持、shared-persist 生产接线 pin。此前失败的4个 patch 合并/
重绑/路径 canonicalization 用例重新通过；完整
`go test ./internal/types ./internal/agent ./internal/tool -count=1` 通过
（types 18.826s、agent 2.793s、tool 159.975s）。

### B9 r4 人工审计与批 Y 规划（2026-07-31）

批 X 推送、重建并通过 revision/clean-input 校验后，以严格
`parallel=2` 第四次重跑：

- `eval/results/trace_query_frame_timeline_flow-20260731-114754`
- `eval/results/read_combo_trace_current_code_boundary-20260731-114754`

runner 2/2 PASS；人工两项仍为 FAIL，但批 X 的目标修复已完全生效。
Mixed 最终引用中不再出现
`[absence: H:RenderService:DoFrame]`，内容寻址的 attached trace 没有再被
当作当前源码；B/E `artifact_spans` selector 与仓库源码引用分席正确。
patch 只追加两条真实 line-scope artifact quote，既有 pool/index 没有漂移。

Frame 的新 typed hint 也确认进入 finalizer：

`frame_flow_causality=unproven, relation=temporal_sequence, edges=3`，
并明确要求不得写成 formed/confirmed/validated flow。模型的边条目、diagram
和 caveat 都服从了该权限，但 summary 首段仍写“形成完整的跨线程 flow”。
这是同一答案内部的权限矛盾。四轮中 r1/r3/r4 复现、r2 正确，证明模型对
软提示服从不稳定；确定性 authority block 始终正确。按用户约束，本轮
不增加扫描“形成/flow/验证”等答案原文的硬拒绝或自动替换，先将其降为
P2 模型波动，继续更高优先级 case。若后续多类 relation 均复现，再建设
不依赖正文词面的 typed principal relation-verdict block。

本轮发现的更高 ROI 系统 gap 是 PerfJank cause authority。当前
`perf-triage-skill` 明说 `reason=best guess`，但 `PerfJank` 没有 authority
字段；`compilePerfBundleObservations` 只要 `Reason` 或 `TriggerSpan`
非空，就将其铸为 `ObservationProvenanceObservedDirectCause`，并把 reason
直接放进 principal runtime-artifact summary。`context/builder`、
`answer_claim_binding`、artifact profile 和 external observation seed 也
把该字符串当普通事实发布。因此“没有 IO/lock 等事件”经模型猜成
`heavy-compute` 后，会跨层升级为已证触发原因；这是系统权限泄漏，不是
最终一句措辞错误。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B9-Y1 | P1 | PerfJank cause authority | pretriage model 的 `reason=best guess` 无 typed authority，却进入 direct-cause provenance、principal observation、claim binding 和 finalizer prompt | 给 PerfJank cause 增加 validator-owned authority；新 bundle 的 trigger/reason/tags 标为 pretriage_model_extraction。时间窗/janky budget 保留观测席，cause 只作 candidate/navigation；deterministic trace_query 可独立升格 | code-complete，待回放 |
| EVAL-B9-Y2 | P2 | frame summary 模型波动 | typed hint、边、图和 caveat 正确，summary 仍把 unproven adjacency 称完整 flow | 不扫描正文硬门；跨后续 relation case 观察。若复现扩展，增加 typed principal relation verdict，而非对 frame/flow 单 type 拟合 | filed-model-variance |
| EVAL-B9-X2 | P1 | patch negative citation 真值 | r4 未再出现被工件反证的 absence citation，真实 artifact line 引用保留 | 无新增施工 | verified-closed |

批 Y1 不变量：

1. duration/start/end、`DurationMs > PerfFrameBudget60HzMs` 的确定性 janky
   位不因 cause 降权而丢失；只分离“发生了慢帧”和“为什么慢”。
2. authority 由 `toPerfBundle` validator 写入，不暴露给模型 schema；不得
   从 reason 词面、span 名、用户问题或最终答案推断。
3. legacy authority 空值保持兼容；新 pretriage cause 不进入
   `ObservedDirectCause`。deterministic trace_query 的 typed root-cause
   row 保持最高权限，不受影响。
4. 不改变显式时间窗、Trace 因果投影、自动补齐、root ranking、wakeup
   chain、窗内可消除量或 frame span/edge 构造。

批 Y1 施工结果：`PerfJank.CausalAuthority` 由 `toPerfBundle` validator
按 trigger/reason/tags 是否存在自动铸为
`pretriage_model_extraction`，tool schema 不含该字段。authority-aware
消费已覆盖 prompt structured bundle、perf stage report、observation
ledger、claim binding、artifact observation profile 和 external
observation seed：慢帧 interval/duration 仍是 runtime observation，
cause 只以 `trigger_candidate/reason_candidate/cause_authority` 保留用于
导航；不再进入 `ObservedDirectCause`，也不再作为普通 reason fact seed。
legacy authority 空值继续走原兼容臂。

测试固定 validator-owned schema、candidate prompt 词面、ledger provenance、
claim support、answer seed 降权，以及 legacy direct-cause 兼容臂。完整
`go test ./internal/types ./internal/tool ./internal/context ./internal/agent ./internal/skill -count=1`
通过（types 19.803s、tool 166.914s、context 0.609s、agent 4.094s、
skill 2.559s）。

### B10 r1 人工审计与批 Z 规划（2026-07-31）

批 Y1 推送、重建并通过 revision/clean-input 校验后，以严格
`parallel=2` 运行：

- `eval/results/real_trace_d4_demand_vs_supply-20260731-120121`
- `eval/results/read_combo_trace_current_code_boundary-20260731-120121`

runner 2/2 PASS，人工 0/2。Mixed case 证明 Y1 已生效：
`heavy-compute` 只作为 `reason_candidate` 携
`causal_authority=pretriage_model_extraction`，不再进入聚合事实或直接根因。
但 `root_evidence:trace_gap` 仍由 typed 观测铸造器赋予
`observed_direct_cause`；模型因此把“窗口没有 scheduler 区间”反向推出
“连续执行、没有抢占或睡眠、纯计算”。对应 `root_cause_data_gap` 主记录
其实已正确位于 `artifact_span`，说明这是同一 data-gap 在 reduced-shape
副本上的权限分叉。

D4 case 验证用户红线没有回退：显式 114.940ms 时间窗、四态闭合、根因
排序、唤醒链、窗内可消除量、完整 Trace 因果投影和系统自动补采全部在场，
主方向“需求侧占主导”正确。深审仍发现：

1. 覆盖摘要选中早先 120ms 宽窗的 target-state 账，覆盖了显式
   114.940ms 窗，导致一份答案内出现两套 running/runnable 数值；
2. 正文写“排除了算力不足”，typed 排名却有正值 10.331ms 供给折算缺口
   #4；应表述为需求主导、供给为次级有界候选，而非不存在；
3. 正文将可能重叠的前三席相加为 53.468ms，而投影合同明确跨线程/物理
   时间不可直接相加；排名可比较不等于可求和。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B10-Z1 | P1 | data-gap provenance | reduced-shape `root_evidence:trace_gap` 无视 typed `trace_gap_kind/tier`，统一铸成 direct cause | data-gap root-evidence 使用 coverage/artifact provenance；typed guidance 固定缺区间不证明连续执行、CPU 占用、未抢占或未睡眠 | covered |
| EVAL-B10-Z2 | P1 | 显式窗口权限 | 多轮查询的 target-state 账按遇到顺序/宽窗聚合，自动补采宽窗可夺取覆盖摘要 principal window | 从 typed request scope 取得显式窗，优先精确匹配；宽窗仅 supplemental，补采和因果投影本身不变 | covered |
| EVAL-B10-Z3 | P1 | demand/supply 混合结论与可加性 | 排名席的方向/正值 supply seat/折算口径未形成统一 verdict；模型把“非主因”写成不存在并直接求和重叠席 | typed 主次方向指导；有正值 supply 席时只能说次级/有界；rank 默认 non-additive，只有明确 disjoint/union caliber 才可相加 | covered |
| EVAL-B10-Z4 | P1 | 唤醒 census 消费语义 | 36/34 来自完整 typed census，数值正确；但模型把 `sleep_exit=34`（唤醒前目标退出 S）反写成“每次唤醒后立即睡眠”，把前态分类当后继转移 | 从 target-wakee cap-immune census 构造紧凑 typed authority，统一给出总量、per-waker 分解、前态 split 与后态证明边界；软提示同步，不扫描答案词面 | covered |

批 Z 不变量：

1. 不读取 raw request、模型 thinking 或最终答案正文作为硬门信号；全部修复
   只消费既有 typed tier、trace-gap kind、request time scope、rank seat、
   causal direction 与 fold caliber。
2. 不移除、缩减或跳过显式窗 Trace 因果投影、自动补采、根因排序、唤醒链、
   帧/调度证据或窗内可消除量。
3. 自动补采可使用扩展窗探索，但 principal coverage/accounting 必须服从
   用户显式窗；扩窗结果可明确标为 supplemental。
4. demand-dominant 不等于 supply-absent；ranked effective impact 默认
   只能比较，未获 typed disjoint/union 权限不得求和。

批 Z1 已完成 typed 与降级文本双通道修复。`root_evidence:trace_gap` 仅按
typed `Type` 降为 `artifact_span`，不从 summary 或答案词面猜测；普通
`binder_wait` 等正向 root evidence 继续保持 `observed_direct_cause`。
降级 summary 解析的 `tier=data_gap/context_only/caliber_side` 同步采用
supporting coverage 权限，避免 payload 缺失时重新升格。finalizer 的
trace-gated 软指导明确：`no_sched_data` 不能证明连续执行、CPU 占用、无
抢占、无睡眠或纯计算；`no_eligible_wait` 也不能证明短等待不存在。

完整 `go test ./internal/types ./internal/tool ./internal/skill -count=1`
通过（types 20.453s、tool 158.623s、skill 0.400s）。

批 Z2 将 `RuntimeArtifactScopeProfile.ExplicitTimeWindow()` 提升为单工件
coverage face 的 principal window authority。`target_window_states` 账户
现在先按 typed `selected_window` 精确匹配该窗口，不能再由“最宽窗优先”
让探索/自动补采的扩展窗夺席；若所有候选都明确声明了不同窗口，则可选状态
摘要 fail-closed。没有已知 principal window 的 legacy/non-windowed 运行
继续保留原“选最宽账户且不求和”兼容策略。

定向用例固定 120.000ms 探索账户先于 114.940ms 显式窗账户，最终覆盖文本
只能发布 114.940ms 及其 26.946/3.636ms 状态值；另有 declared-window
mismatch fail-closed 与 legacy 正例。完整
`go test ./internal/types ./internal/tool -count=1` 通过
（types 20.434s、tool 160.876s）。

批 Z3 复用既有 `rank/effective_impact_ms/fix_direction`，没有新增第二套
因果分类。finalizer 的 Observation Ledger 后新增紧凑 typed authority：

- 每个 trace 分区公布 top seat 与 seated count，并明确
  `cross_row_additivity=forbidden`；不同根因席只能比较 attribution/head-room，
  不能跨行、跨线程、跨方向求和。只有某一 merged row 自己公布的 total 才
  可在该行的 typed fold caliber 内解释为合计。
- 若存在正值 `fix_direction=frequency_thermal` 席，则发布
  `compute_delivery_positive=true` 及其 rank/subject/type/value；若它不是
  top，权限词固定为 `secondary_bounded_candidate`，因此“不是主因”不能被
  模型升级成“不存在/已排除”。

answer-document skill 与 exploration skill 同步增加相同 typed 软指导；
`NO CROSS-ROW DURATION SUMS` 明确把 top-N 根因席纳入禁加范围。整个实现不
扫描 raw request、thinking 或答案原文，不新增 hard reject，不改变因果投影、
自动补采、榜单、席值或排序。生产接线测试从真实
`renderAnswerDocObservationLedger` 入口固定 lock-priority #1 +
frequency-thermal #4 的混合形。完整
`go test ./internal/agent ./internal/skill -count=1` 通过
（agent 2.470s、skill 0.923s）。

批 Z4 对真实 fixture 的显式窗
`34579.472865..34579.587805` 直接复算：目标
`com.baidu.tieba-59566` 共36条 `sched_wakeup`，其中
CookieMonsterCl-59843 为34条，Binder:43397_19-23088 与
T7@ZeusThreadPo-61839 各1条。引擎
`WakeupEdgeCensus` 和 observation 的三条 target-wakee 记录与原始行完全
一致，且 `target_wakee=true` 使目标 pair 在 engine/tool 两层 row cap 下
免疫。因此“36/34”不是分支样本、重复视图或模型编数，原计划中的“缺单一
census”假设被证伪。

真正的 gap 是时序方向被消费层反转。现有三条记录均为
`sleep_exit=count`；该字段在代码合同中明确是 wakeup 发生时 wakee
“离开的状态”，即唤醒前的 S-sleep。客户答案却写成“每次唤醒后主线程都
立即进入睡眠”。真实调度序列也存在醒后先运行、被 `R+` 抢占、恢复后才
进入 S 的样例，故不能由 pre-wakeup split 证明 post-wakeup 的“立即/每次”。

修复不更改 census、链、榜单或投影：types 层仅从
`producer=trace_query + predicate=wakeup_edge_census +
target_wakee=true` 构造 `TraceTargetWakeupCensusAuthority`，按单个 query
结果给出完整总量、per-waker 分解、三类 pre-wakeup exit split，并对
split 恒等式或重复 pair 冲突 fail-closed。finalizer 在 Observation
Ledger 前置紧凑权威块中明确：

1. 方向固定为 waker → target；
2. `sleep_exit/d_exit/other_exit` 是唤醒前离开的状态；
3. wakeup 只证明目标进入 runnable，后续 switch-in、运行、抢占和
   switch-out 是独立事实；
4. “唤醒后每次立即睡眠”必须另有完整的后继调度转移配对 census。

answer skill 同步同一 typed 软指导，没有 raw request/模型 thinking/最终
答案关键词扫描，也没有 hard reject 或文本改写。测试固定 34+1+1=36 的
完整分解、pre-wakeup sleep=36、非目标 pair 不混入、split 冲突
fail-closed，以及真实 `renderAnswerDocObservationLedger` 生产接线。
完整 `go test ./internal/types ./internal/agent ./internal/skill -count=1`
通过（types 21.737s、agent 3.061s、skill 0.473s）。

### B10 r2 人工审计与批 AA 规划（2026-07-31）

批 Z4 推送、重建并通过 revision/clean-input 校验后，以严格
`parallel=2` 重跑：

- `eval/results/real_trace_d4_demand_vs_supply-20260731-123201`
- `eval/results/read_combo_trace_current_code_boundary-20260731-123201`

runner 2/2 PASS，人工 0/2。D4 对 Z2/Z4 的验收通过：principal coverage
只发布显式 114.940ms 窗；目标 wakeup census 为36，waker roster 为
CookieMonsterCl 34、Binder 1、T7 1，`sleep_exit` 不再被反写成
“唤醒后立即睡眠”。Z3 也保住了正值 frequency/thermal #4 席，并把它称为
次级有界候选；但模型仍在另一段把 `23.994 + 19.041` 求和为43.035ms，
与确定性 `cross_row_additivity=forbidden` 相冲突。按本战役红线，不扫描
答案数字或求和词面做硬拒绝；先作为 P2 模型波动观察跨 case 复现。

D4 还暴露了一个不同层级的通用语义边界：目标线程
running=26.946ms/23.4% 是该线程自己的状态分区，不能推出全 CPU
“利用率不饱和/远未饱和”。低 runnable 只能约束该目标线程的 scheduler
queueing；CPU-wide 饱和度必须由 per-CPU/core/system busy、idle、pressure
等 typed 账户证明。这不是某个线程名或数值的特例。

Mixed case 对 Y1/Z1 的验收通过：`heavy-compute` 没有升为根因，缺少
sched 区间只形成 data gap，86.111ms 大于用户给定50ms的标量关系正确，
trace 行与源码行也没有混淆。但 current-source 机制出现三类实质错误：

1. `EventTraceMark` 只是事件类型枚举常量，答案却称其负责解析 B/E；
2. 当前代码只有 `traceSpanCategory`，答案虚构了
   `classifyFrameCategory`，并把 `strings.Contains(...)` 的 case 行当作
   该虚构函数的 call site；
3. 50ms 来自用户比较条件，Codrax 确定性的60Hz pretriage budget 实为
   `PerfFrameBudget60HzMs=16.67`，答案却把50ms写成系统卡顿阈值。

第2项已定位到 grounder 的结构性失效：
`lineCorroboratesCallSite` 看到 `LineFeatureCallExpression` 就直接返回
true，没有验证该 AST 调用行是否包含 `AnchorSymbol` 指定的 callee；
`preferredCallTargetNames` 又同时混入 subject/object。因此“这一行存在某个
调用”被错误升级成“这一行调用了指定函数”。该证据随后以
`claim_form=call_edge` 进入 principal path，导致不存在的机制节点获得
factual authority。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B10-AA1 | P1 | target/CPU scope semantics | target_window_states 的 thread-local running/runnable 比例缺少消费权限边界，模型把目标线程 occupancy 推成 CPU-wide utilization/saturation | 从 typed target-state 与 CPU-accounting 账户构造紧凑 authority；只允许描述目标状态/排队，CPU-wide 结论必须另有 typed per-CPU/core/system 证据 | covered |
| EVAL-B10-AA2 | P1 | call-anchor integrity | AST 只证明“行内有调用”，未证明指定 callee；subject/object 还能替代显式 AnchorSymbol | 显式 AnchorSymbol 存在时只以该 symbol 为 call target；AST feature 仅可覆盖 definition-shape 误判，仍须 exact callee relation 或 line-local call syntax | covered |
| EVAL-B10-AA3 | P1 | mechanism path closure | 多条独立 definition/case facts 可被包装成 principal_path_edge，零 grounded relation 仍形成“完整机制链” | 由结构化 anchor kind、grounded relation 与 path-edge facet 构造 relation authority；无边时明确为 independent mechanism facts，不证明调用顺序 | covered |
| EVAL-B10-AA4 | P2 | threshold provenance | 用户比较阈值、artifact 观测和源码配置阈值没有 typed 分席，模型把提问中的50ms升级为系统规则 | 建立 validator-owned comparator/profile，分别发布 user comparator 与 code-configured threshold；没有源码证据不得称系统阈值 | covered |
| EVAL-B10-AA5 | P2 | rank additivity model variance | typed non-additivity 已发布，模型仍有一处直接求和 | 不增加答案词面/数字扫描硬门；跨不同 rank case 复现后再考虑 typed principal verdict，当前先观察 | filed-model-variance |

批 AA 不变量：

1. 不读取 raw request、模型 thinking 或最终答案正文来做硬门，也不按
   `classifyFrameCategory`、50ms、CPU 饱和等本 case 词面拟合。
2. 显式时间窗、Trace 因果投影、系统自动补采、根因排序、wakeup chain、
   frame span/edge 与窗内可消除量保持原样。
3. AST `LineFeatureCallExpression` 继续作为“存在调用表达式”的精确信号，
   但不再越权代表任意指定 callee；真实 `yield*`、`await`、动态限定调用的
   exact target 仍应 grounded。
4. 用户阈值可用于回答用户提出的标量比较，但不能在没有当前源码证据时被
   描述为产品内部判定阈值。

批 AA2 已修复 call-anchor 的“行形状代替 callee 身份”越权。
`lineCorroboratesCallSite` 现在先要求 exact graph call relation 或目标名字
在本行构成真实 call syntax；`LineFeatureCallExpression` 只允许覆盖
definition-shape 启发式的误判，不再单独返回成功。若存在显式
`AnchorSymbol`，首次 grounding 只消费该 symbol，subject/object 不能把
错误名字直接铸成 grounded。

既有自动恢复没有被简单删除：当 nearest-call 等 recovery tier 从 typed
relation 找到真实 callee 时，可使用 legacy object/subject 候选，但必须把
`AnchorSymbol` 改写成实际 callee，并以 `recovered` 发布。因此
caller-shaped 常见错误仍可修复，而 `strings.Contains(...)` 所在行不能再
为零 relation、零本地 syntax 的 `classifyFrameCategory` 提供权限。

回归覆盖无关 AST call 拒绝、错误显式锚点不能直接 grounded、typed relation
恢复后锚点改写、真实 `yield*`/普通 call 与既有 caller-shaped recovery。
`go test ./internal/tool/ground ./internal/tool -count=1` 通过
（ground 1.217s、tool 159.549s）。

批 AA1 复用投影编译器已经完成显式窗选举的
`TargetStateAccount`，没有从全部 raw state rows 另选一套账户。新增紧凑
`Trace Target-State Scope Authority`，逐工件发布
`scope=target_thread_only` 与
`cpu_wide_saturation_authority=not_provided_by_target_window_states`：
running/runnable/sleep/D-state 只描述目标线程自己的墙钟分区；低 runnable
只能约束该目标的 scheduler queueing。CPU-wide utilization、idle
head-room 或 saturation 必须另有 typed per-CPU/core-class/process-domain/
system occupancy、idle 或 pressure 账户。

answer skill 同步相同 soft guidance，trace-gated、零 OnViolation；不扫描
用户或答案词面，不改数值，也不阻断有独立 CPU 账户支撑的系统级结论。测试
固定26.946ms running、3.636ms runnable 的真实形只能取得 target-thread
scope，并验证生产 `renderAnswerDocObservationLedger` 接线。完整
`go test ./internal/types ./internal/agent ./internal/skill -count=1` 通过
（types 22.230s、agent 4.348s、skill 1.105s）。

批 AA3 新增 `Current-Source Mechanism Relation Authority`，仅在 typed
mechanism/call-chain 或 active current-source mechanism/flow 请求上出现。
它只消费 accepted `EvidenceItem` 字段和 supported `FlowFindingDigest`：

- grounded definition/constant/branch/assignment 等计为 independent local
  fact，不自动形成边；
- call site 只有同时具备规范化后的非空、不同 subject/object，才发布
  `caller -> callee`；
- `FlowFindingDigest.UnsupportedReason` 非空的路径不取得时序权限；
- 零边时明确 `ordered_path_authority=unproven`；有 call edge 但无 typed
  flow 时为 `listed_edges_only`；typed flow 存在时也只允许复述列出的路径。

该块明确 `principal_path_edge` 是答案版面义务，不是凭空铸造调用顺序的
证据。实现没有解析 raw request、thinking、closure prose 或最终答案，也
没有新增 emit/reviewer hard reject；真实 AnswerChain/FlowFinding 和 grounded
caller→callee edge 仍能完整表达。`go test ./internal/agent -count=1` 通过
（2.811s）。

批 AA4 没有扫描请求中的阈值词面，也没有相信 perf 模型写入的 summary 或
`janky=true`。finalizer 直接读取 validated `PerfBundle.Frames`，用与
`toPerfBundle` 相同的 validator-owned
`PerfFrameBudget60HzMs=16.67` 重新计算，仅对
`duration_ms > 16.67` 的帧发布 `validator_janky=true`。紧凑
`Perf Threshold Provenance Authority` 将该值标为
`deterministic_validator_constant`。

同一权限块明确：请求给出的其他阈值仍可正常用于回答标量比较，但若它只在
request comparator、model aggregate 或 summary 中出现，不能改称 Codrax
内部 jank rule；只有单独的 current-source evidence 才能证明另一条产品
阈值。低于16.67ms但由模型提交 `janky=true` 的负臂不会取得 deterministic
threshold authority。实现不读取答案文本、不新增硬门。
`go test ./internal/agent -count=1` 通过（2.873s）。

### B11 r1 人工审计与批 AB 规划（2026-07-31）

在 revision `78e7c9da89c3` 重建后，以严格 `parallel=2` 执行：

- `eval/results/github_issue_libgit2_foreach_worktree_symptom-20260731-125556`
- `eval/results/real_trace_h1_binder_true_false_attribution-20260731-125556`

runner 1/2，人工 0/2。两个失败来自不同通道，均可由 typed 状态和结构化
载体泛化修复，不需要读取用户原始问句或模型答案词面。

写模式 C 用例的产品修改正确：两个比较/赋值优先级缺陷均增加括号，现有
`test_repository.c` 覆盖 `-42`、`-7`、`0`。原生 C verification probe
因当前枚举只支持 Python/JavaScript/Ruby/Java/Go 被拒后，模型改为 Python
wrapper；wrapper 确实调用 gcc 编译并执行测试二进制且通过。问题发生在
`run_tests` 的控制顺序：

1. pre-suite probe 通过；
2. `verificationProbePassProjectSuiteContinuationReason` 在 changed-path
   coverage 尚未计算时返回空；
3. `make@.` / `make check` 虽已由 TestSurface 识别，仍被
   `probe_primary_suite_skipped` 跳过；
4. 随后 `applyChangedPathVerificationCoverage` 才发现 Python probe
   不能授权 `repository.c`，把整份报告降为
   `changed_path_verification_uncovered`。

因此这是所有“跨语言 wrapper probe 通过、但修改源文件未被该 probe
family 覆盖、同时存在匹配项目 suite”都会触发的接线 gap，不是 C 或
libgit2 单点。

H1 的 typed binder 事实正确：目标线程已呈现的同步等待为
transaction 12145859，peer `binder:496_9-10961`，发生于
13762.835861..13762.837270，耗时 1.409ms。显式窗口的完整 Trace 因果投影
仍然存在，15.758ms pacing idle 也被确定性投影排除出根因榜。AA1
`Trace Target-State Scope Authority` 已出现在 answer-writer 输入中，且
正文没有用目标线程 running/runnable 比例反推 CPU-wide 饱和度。

但 H1 暴露出两个高价值 typed 消费缺口：

- model-visible `root_cause_rank:13` 同一行发布 value=15.758ms，却携带
  span=13762.984951..13762.985960（约1.009ms）。15.758ms 是累计/投影
  pacing-idle 值，真实对应成员窗约为
  13762.992415..13763.008173。也就是说，聚合值与单次 occurrence span
  被错误拼成一条 observation，模型随后忠实写出了物理不可能的
  duration/window 对。
- `Trace Rank Arithmetic And Supply Authority` 只发布席位总数、top seat
  和 frequency seat，不发布完整有序 roster。模型手写表把 binder
  1.409ms 放在 #6，又把 3.429/3.309ms 放在 #7/#8；binder 在 typed ranked
  seats 中实际无该席。确定性 projection 和校验附注能事后指出冲突，却
  没在 answer-writer 的 typed authority 中给出可直接转录的完整顺序。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B11-AB1 | P1 | write verification continuation | probe-pass 决策早于 changed-path coverage，导致已识别的匹配 suite 被跳过后才发现覆盖不足 | 对 probe pass 的临时报表先执行 changed-path coverage；若有 changed source 未覆盖且存在可运行 TestSurface，则继续项目 suite，并由组合报告取得最终覆盖 | covered |
| EVAL-B11-AB2 | P1 | trace aggregate temporal identity | 累计/投影值与某个单次 occurrence span 共用 observation，duration 与 window 失配 | 让聚合记录只携带 aggregate/member-set 窗；单次 span 必须绑定同一 member 的 duration，无法一一对应则不发布单次精确窗 | covered |
| EVAL-B11-AB3 | P1 | rank roster authority | writer 仅获 top/supply 摘要，完整排序留在成文后的 deterministic projection | 从 compiled projection 发布紧凑、完整、按 rank 排序的 typed roster，区分 ranked seat 与 context-only/binder composition，供正文直接转录 | covered |
| EVAL-B11-AB4 | P2 | incomplete enumeration wording | observation coverage 已为 incomplete，正文仍写“窗口内全部/其余均为” | 将 per-view enumeration completeness 与可用 rowset 绑定成 typed wording authority；保持 soft guidance，不扫描答案做 hard gate | filed |
| EVAL-B11-AB5 | P2 | native probe capability | verification probe schema 无 C/C++/shell，模型需用另一语言 wrapper | 先由 AB1 自动续跑匹配项目 suite 保证正确性；原生 probe 扩展另案评估安全沙箱与命令边界，不阻塞本批 | filed |

批 AB 不变量：

1. 不扫描 raw request、thinking 或最终答案文字/数字做硬门。
2. 不改变显式窗选举、Trace 因果投影、根因排序、唤醒链、窗内可消除量和
   自动补采；只修 typed 载体身份与消费接线。
3. 通过的 bounded probe 仍可跳过无关项目 suite；只有 changed-path
   coverage 实际不足且存在匹配 TestSurface 时才续跑。
4. 聚合值可以保持完整累计语义，但不能借用不对应的单次精确时间窗。

批 AB1 已把 changed-path 权限前移到 probe-pass 的 suite 决策点。判定只消费
ChangePlan 的 active target paths、已通过 probe 的 typed language family /
`changed_symbol_refs`，以及 TestSurface runner 的 language family /
working directory。只有仍未覆盖的修改源文件确实存在匹配 runner 时，才以
`verification_probe_changed_path_uncovered` 继续项目 suite；Python probe
不能再替 C/C++ 路径跳过 make/cmake/meson，同语言且已覆盖目标的 bounded
probe 仍可跳过无关测试。

该 continuation reason 没有加入 infra-downgrade 白名单：若 matching suite
失败或超时，不能用未覆盖路径的 probe 把结果降级成 PASS。回归用真实
Python wrapper 调用 `cc` 编译并执行 C 测试，再确认系统仍续跑 `make check`
并由 make 的 C-family command 闭合 `repository.c`。同时固定完整 probe
继续跳过无关 Python suite、impact-related suite 优先级、修改测试文件时
suite 失败/超时保持 non-pass。完整
`go test ./internal/tool -count=1` 通过（155.450s）。

批 AB2 定位到错误并非模型重写时间，而是 typed coverage 的合法回退被错误
输入触发：`pacing_idle` 根因席位没有自己的 `StartTs/EndTs`，因此
`traceObservationWindow` 退到 `nearest_chain_window`。后者只是拓扑附着锚
（本例为目标的另一段 1.009ms 链节点窗
`13762.984951..13762.985960`），不能代表该席 15.758ms 值的发生区间。
真实同事实窗在 `PacingIdleSummary` 中完整存在：
`13762.992415..13763.008173`。

修复在 `RootEvidence` 增加权威 occurrence interval 载体，并从三个本来就
具有单次发生边界的生产通道贯通到 `RootCauseRankItem`：

1. pacing/periodic idle 使用自身 `WindowStartTs/WindowEndTs`；
2. binder wait 使用 sleep-start 到 wakeup；
3. missing-wakeup 使用未闭合 sleep interval 本身。

`traceQueryTypedObservations` 原有的 rank-item→ObservationSpan 直通因此取得
同一事实区间，coverage 始终优先使用该 span，`nearest_chain_window` 继续
只作 topology fallback。特意没有把普通 `WakeupCausalImpact.Window` 全量
灌入 RootEvidence 席位：完整回归证明那是聚合计算包络，会改变既有链相关性
和双尺榜位；回退后 DHM-A1a 的 wall/edge seats 保持原值。这一负臂守住了
“时间身份修复不迁榜”的边界。

回归固定三层：

- tracequery 真实 donghu 行片段：15.758ms pacing RootEvidence 与 rank row
  均携带 `13762.992415..13763.008173`，且区间宽度与值在 1µs 内一致；
- tool 投影：即使另有不同 `NearestChainWindow`，ObservationSpan 仍逐字
  保留 occurrence interval；
- types coverage：value-owning span 的优先级高于 topology anchor。

完整 `go test ./internal/tracequery -count=1` 通过（66.378s），完整
`go test ./internal/types ./internal/tool -count=1` 通过
（types 20.036s、tool 158.848s）。实现不改 root score、rank/tier、因果
投影、唤醒链、窗内可消除量、显式窗选举或自动补采，也不读取任何用户/模型
文本。

批 AB3 把完整榜单权限放回 typed projection，而不是继续要求模型从散落的
top observation 或探索摘要重建。`TraceCausalProjection.RankedSeats` 在
R1/R2 去重与聚合已经完成、相邻/背景展示 cap 尚未折叠时冻结：因此它继承
既有 seat 的 rank/tier/effective/fix-direction 语义，又不受树面展示容量
影响。该字段是只读 side channel，不参与排序、分值、准入、折叠或补采。

新的 `BuildTraceRankRosterAuthorities` 按完整 board identity 分组：

- trace artifact；
- rank board target；
- rank-supplying query window；
- board params fingerprint。

同一工件、同一时间窗下不同 target/knob 的 `#1` 不会混成一张榜。每板严格
检查 ordinal 从1连续、无重复，且每席存在正值 published effective：
满足时 `roster_status=complete`；缺席、撞号或值权限不足时发布 typed
incomplete 状态，保留已知席但禁止补猜。

answer-writer 现在逐席收到：
`#N / type / subject / effective / tier / channel / fix_direction`，并明确该
roster 是唯一 ordinal authority。任何未入 roster 的 measured component、
context-only、target symptom、data gap、caliber side rail 或 absorbed row
均不得仅凭时长、发现顺序、另一张表或叙事重要性获得 `#N`。这统一覆盖 H1
把 1.409ms binder component 自授 #6、又把更大席放 #7/#8 的问题，而不是
对 binder 类型做单点规则。

回归覆盖：

1. 同 artifact 双 board 的 rank #1 分域与每板连续性；
2. rank gap fail-closed 为 incomplete；
3. 生产 `renderAnswerDocObservationLedger` 接线输出 #1..#4 严格有序；
4. rank=0 的 binder context 不进入 roster；
5. finalizer soft skill 同步 roster-only ordinal 规则。

完整 `go test ./internal/types ./internal/agent ./internal/skill -count=1`
通过（types 21.068s、agent 3.827s、skill 1.486s）。实现不扫描 request /
thinking /答案原文，也不改显式窗、因果投影树、根因榜计算、wakeup chain、
窗内可消除量或自动补采。projection 新载体已登记信息契约为 answer-writer
displayed authority、非树面 gate；完整 `go test ./internal/tool -count=1`
通过（159.045s）。

### B11 r2 人工审计与批 AC 规划（2026-07-31）

在 revision `712db9f326bd` 重建后，以严格 `parallel=2` 回放：

- `eval/results/github_issue_libgit2_foreach_worktree_symptom-20260731-135209`
- `eval/results/real_trace_h1_binder_true_false_attribution-20260731-135209`

runner 1/2，人工 0/2。H1 自动 PASS 只证明既有 regex/oracle 命中，人工仍
为 FAIL；C 用例的产品补丁正确，但交付因验证控制面失败。

H1 证明 AB2 的 occurrence carrier 已在真实答案面生效：15.758ms
`pacing_idle` 现在绑定自身发生窗
`13762.992415..13763.008173`，不再借用 1.009ms topology anchor。完整
Trace 因果投影、用户显式 233.190ms 窗、系统补采、目标四态、唤醒链与
窗内可消除量均在，红线正臂没有回退。

但 AB3 首版遗漏了 rank board 的 channel 维度。相同 artifact/target/window/
params 下，`on_chain` 与 `adjacent` 各有独立的 `#1..#N`；首版把两张榜
合成 26 席，正确检测成 `duplicate_rank`，finalizer 因而没有 complete
roster 可转录，又自行给 RenderThread、DetectViewRect、Binder component
等非榜席编造 #1/#2/#4。最优修复不是改 H1 名字或数值，而是把 typed
`ChainRelevance` 归一成 board channel 并进入 roster identity；不同 channel
的序数禁止比较，同 channel 内才检查连续性。

H1 还暴露 AB2 的消费尾差。typed
`root_cause_target_self_state/binder_wait` 已携同事实等待窗
`13762.835861..13762.837270` 和 1.409ms；explorer aggregate 却把同一
transaction 的发送阶段窗 `13762.834345..13762.835754` 填入
`timestamp`，finalizer 将它写成“真正等待时间戳”。这是跨事件阶段的
值/窗拼接，不是数值误差。方案应从 typed observation 建通用
value-owner temporal authority：同 subject/type/value 的 occurrence 窗
是该值的唯一时间身份；command aggregate 可作叙事候选，但不能用另一阶段
的窗覆盖。冲突或多 occurrence 必须标 ambiguous，不按到达顺序任选。

C 用例的 `TestSurface` 明确只有并选中了 `make@.`（`make check`，有真实
test signal）。但 `preferredRunnerFromChangePlan` 将 `.c` 映射为 `cmake`；
`defaultRunnerPlansFromTestSurface` 在找不到 cmake candidate 时仍合成一个
未配置 cmake runner，并先占用工作目录 `.`，把真正的 make candidate
去重掉。验证因此以 `not_configured` 失败。这里精确信号与软偏好倒置：
plan-touched language 只能给已有 runnable candidate 排序；当任一
TestSurface candidate 有真实 test signal 时，不得凭偏好合成不存在的
runner 抢占同目录。只有没有真实 test signal 时，才保留 plan-touched
syntax/no-test fallback。

后续 no-change replan 被拒绝不是新产品 GAP：模型调用的是无参
`run_tests`，不是绑定原 applied plan 的
`dry_run=true + verification_probe`，系统没有把一次旁路检查升级为原计划
的 passing proof，按设计 fail-closed。修好首轮 runner 选举后该级联消失。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B11-AC1 | P1 | rank board channel identity | 同 target/window/params 的 on-chain 与 adjacent 独立榜被合并，产生重复序数并使完整 roster 失权 | board identity 增加 typed channel；逐 channel 连续校验和发布，禁止跨 channel 比较 | covered |
| EVAL-B11-AC2 | P1 | value-owner temporal authority | typed 等待值与自身 occurrence 窗在场，但 command aggregate 可携另一 transaction stage 的 timestamp 覆盖成文 | 从 typed observation 发布 subject/type/value/occurrence 的紧凑权限；冲突/多值 fail-closed，aggregate 不铸新窗 | covered |
| EVAL-B11-AC3 | P1 | runnable TestSurface signal precedence | plan-touched runner 偏好在无 candidate 时合成未配置 native runner，并以同目录去重挤掉真实 make test surface | 真实 HasTestSignal candidate 优先；仅在零真实 test signal 时允许 inferred preferred-runner fallback | covered |
| EVAL-B11-AB2 | P1 | trace aggregate temporal identity | pacing occurrence carrier 真实回放已正确 | carrier covered；成文消费由 AC2 收尾 | verified-partial |
| EVAL-B11-AB4 | P2 | incomplete enumeration wording | `enumeration_status=incomplete` 在场，正文仍写“其余所有睡眠段均…” | 保持既有 filed；不为当前 P1 批扫描答案词面 | reproduced-filed |

批 AC 不变量：

1. 不读取 raw request、case ID、模型 thinking 或最终答案词面做硬门。
2. 不修改显式窗选举、Trace 因果投影树、自动补采、根因分值/排序算法、
   唤醒链、窗内可消除量或任何 trace 数值。
3. rank channel 只分隔本来就独立的 ordinal domain，不改变任何席位的
   rank/value/tier/fix-direction。
4. TestSurface 的 `HasTestSignal` 是精确信号；plan-touched runner 是软偏好。
   无真实测试面时保留既有 syntax/no-test fallback。
5. 时间权限只携带已接受 typed observation 的同事实 occurrence；不从
   aggregate label、用户问句或答案文本解析时间。

批 AC3 没有把“仓库存在任意测试”误作当前语言覆盖，而是复用既有
`VerificationLanguageFamiliesFromRunner` 做 typed family 相交：当
plan-touched 偏好没有实际 candidate，且 TestSurface 存在同 family 的
`HasTestSignal=true` candidate 时，选择后者；否则仍保留 inferred runner
作为 syntax/no-test fallback。由此 `.c/.cpp → cmake` 可以被真实
`make check` 的 C/C++ family 接应，而 Python 偏好不会被无关 Go 测试抢走。

回归固定两条相邻臂：C 仓只有 Makefile `check` 时队列仅含 make、不再合成
cmake；Python 变更面对只有 Go 测试的仓库仍由 plan-touched Python fallback
先行。专项测试与完整 `go test ./internal/tool -count=1` 通过
（160.817s）。

批 AC1 在 answer-side roster identity 中增加
`BoardChannel=ChainRelevance`，没有修改 projection 原有 board key、去重、
聚合或排序。相同 artifact/target/window/params 的 `on_chain` 与
`adjacent` 因而各自从 #1 连续校验；空 channel 明确落入 `unspecified`，
不继承另一榜的权限。writer 逐板发布 `board_channel`，并明确序数只能在
同 channel 内比较。

回归覆盖 types 层双 channel 各有 #1/#2 且均 complete，以及生产
`renderAnswerDocObservationLedger` 同时输出两张 complete roster、零
`duplicate_rank`；skill 的 soft handoff 同步 channel 边界。完整
`go test ./internal/types ./internal/agent ./internal/skill ./internal/tool -count=1`
通过（types 18.371s、agent 3.096s、skill 0.359s、tool 157.491s）。

批 AC2 新增通用 `TraceValueOccurrenceAuthority`，没有按 Binder、H1 或固定
数值分支。候选必须同时满足：

1. deterministic `trace_query` + runtime artifact + hard grounding；
2. subject 匹配 typed user `RuntimeTarget`（exploration cursor 排除）；
3. 属于 root-cause / critical-blocking / evidence-pack typed dimension；
4. 单位为 ms，且 `ObservationSpan` 宽度与发布值在 trace 六位时间戳和
   三位毫秒显示容差内一致。

最后一条是 value ownership 的精确信号：查询包络、链路锚或 transaction
其他阶段即使也有时间，都不能借给该值。相同 artifact/subject/type/value
的重复发布在同一 interval 去重；若存在多个不同 interval，则只发布
`ambiguous_multiple_occurrences` 和 occurrence count，start/end 保持空，
禁止按到达顺序任选。

answer-writer 的 `Trace Value-Owner Temporal Authority` 明确 exact interval
优先于 command aggregate、transaction send/receive phase、邻近事件或叙事
timestamp；skill 仅同步 soft guidance，不新增答案扫描或 hard reject。测试
固定 H1 形的正确 `13762.835861..13762.837270`，确认错误 command timestamp
不参与铸造；另固定多 occurrence fail-closed、聚合包络和非目标拒绝、生产
writer 接线。完整
`go test ./internal/types ./internal/agent ./internal/skill ./internal/tool -count=1`
通过（types 18.271s、agent 3.007s、skill 0.295s、tool 161.506s）。

### B11 r3 人工审计与批 AD 规划（2026-07-31）

在 revision `428656a0fed0` 重建后，以严格 `parallel=2` 回放：

- `eval/results/github_issue_libgit2_foreach_worktree_symptom-20260731-143025`
- `eval/results/real_trace_h1_binder_true_false_attribution-20260731-143025`

runner 2/2，人工 1/2。三项 AC 原始 witness 均已闭合：

1. C 写用例选中真实 `make@.` / `make check`，不再合成未配置 cmake；
   首轮模型只修 visit 分支，项目测试以 `lookup negative status: got 1,
   want -7` 暴露同构漏修，durable replan 修复第二处，最终补丁与项目测试
   均正确。首轮推理不一致被验证闭环兜住，暂按模型波动处理，不对 C
   表达式或错误码词面建立硬门。
2. H1 的 on_chain 与 adjacent 榜已分别发布 complete roster；正文根因
   #1..#4 与 on_chain 榜一致，1.409ms binder component 保持
   `target_self_state` 无榜席。
3. H1 的 1.409ms binder value-owner occurrence 已正确发布并转录为
   `13762.835861..13762.837270`。

显式用户窗、Trace 因果投影、自动补采、目标四态、唤醒链、根因有效归因和
窗内可消除量均保持在场，没有因状态查询降级或新 authority 而消失。

H1 新暴露的是“measurement caliber 冒充”，不是 Binder 单点：

- typed `critical_blocking` 只证明目标线程一个 `binder_wait` occurrence：
  13762.835861..13762.837270，1.409ms，peer
  `binder:496_9-10961`；
- explorer 另把四个 `sync_like` IPC 的 send→reply/transaction latency
  相加为 1.558ms，并命名为“真正等在对端回复上的时间”；
- send→reply 是 IPC 运输/事务耗时，只有与目标调度阻塞 occurrence 同事实
  绑定的区间才能进入线程阻塞墙钟；同步调用计数本身不证明目标在整个事务
  区间处于阻塞；
- 正文又用“65 次均为 S 态、零 D-state”否定 binder 导致的睡眠，但
  interruptible S 正是合法 binder 等待形态，D-state 为零只能排除
  uninterruptible 等待，不能反证已发布的 S-state binder occurrence。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B11-AD1 | P1 | target blocking wall-clock caliber | 同步事务数/往返 latency 与目标线程真实 blocking interval 无 typed 量纲边界，aggregate 可冒充阻塞墙钟 | 从 deterministic critical-blocking observations 按 artifact/typed target/selected window/type 构造 occurrence union；只接纳 value 与自身 span 同宽记录，去重并集，transport latency 排除 | covered |
| EVAL-B11-AD2 | P1 | scheduler-state blocking semantics | D-state absence 被误作“无 counterpart wait”，与已证 S-state wait 冲突 | 同一 typed authority 明示 S 与 blocking 兼容；D=0 只排除 uninterruptible，不反证列出的 blocking occurrence | covered |
| EVAL-B11-AB4 | P2 | incomplete enumeration wording | `enumeration_status=incomplete` 仍与全部/仅有措辞并存 | 后续独立 typed wording authority；不扫描输出原文做 hard reject | reproduced-filed |

批 AD 不变量：

1. 不读取 raw request、case ID、模型 thinking 或答案原文做判定。
2. 不改变 trace_query、显式窗、因果投影、榜位/分值、wakeup chain、自动
   补采、窗内可消除量和任何已测数值。
3. blocking wall clock 只来自目标自有、hard-grounded、
   deterministic critical-blocking typed observation；IPC graph、command
   aggregate 和模型摘要不得铸值。
4. 每条候选必须满足 `span width ≈ published ms value`；聚合包络或另一
   event phase 无法进入。重复 interval 去重，重叠 interval 做墙钟并集。
5. source capacity 截断时只发布 observed lower bound，禁止授予 exhaustive
   total 权限。

批 AD1/AD2 新增 `TraceBlockingWallClockAuthority`。authority identity
包括 artifact、typed selected window、typed target subject 和 blocking
type；跨窗、跨目标、跨类型记录不会相加。候选必须来自
`critical_blocking` 维度和 predicate，且记录自身 `ObservationSpan` 宽度与
value 在六位 trace timestamp/三位毫秒显示容差内一致。这样 Binder、
futex、lock、D-state 等类型共享同一规则，而 IPC transaction/reply latency
即使数值存在，也因不拥有目标 blocking occurrence 被隔离。

同 interval 的 rank/critical 重复发布按物理发生区间去重；不同但重叠的
目标阻塞区间按 interval union 计墙钟，避免双算。若记录携
`capacity_truncated=true`，发布
`coverage_status=lower_bound_capacity_truncated`，只能报告已证下界；
完整 rowset 才发布 `complete`。

answer-writer 同时获得逐 occurrence 的 interval、duration、peer、flags 和
source records，并明确：

- sync request count、send-to-reply latency、peer execution 与 model
  aggregate 是不同量纲，不得加入 blocking wall clock；
- S-state 与真实阻塞兼容，D-state=0 不得反证列出的 S-state wait；
- lower-bound 状态不得写 total/all/only。

该接线是 soft typed guidance，无任何请求/答案关键词扫描或 hard reject。
完整 `go test ./internal/types ./internal/agent ./internal/skill ./internal/tool
-count=1` 通过（types 21.443s、agent 3.600s、skill 0.978s、tool
161.564s）。

### B12 r1 人工审计与批 AE/AF 规划（2026-07-31）

在 revision `5951683f3873` 重建后，以严格 `parallel=2` 回放：

- `eval/results/real_trace_h1_binder_true_false_attribution-20260731-145329`
- `eval/results/github_issue_pyo3_iter_nth_overflow_symptom-20260731-145329`

runner 1/2，人工 0/2。

H1 已验证 AD1/AD2 的核心目标：正文不再把多个 IPC send/reply latency
相加成 1.558ms，而是报告 target-owned binder wait 1.409ms；on-chain
根因 #1..#8 仍与 complete typed roster 一致。显式 233.190ms 用户窗、
Trace 因果投影、自动补采、目标四态、wakeup chain 和窗内可消除量均在场。

但阻塞 authority 的首版只接 `critical_blocking` 行。该行的 drill 包络是
`13762.835811..13762.837270`，宽 1.459ms，与发布的 1.409ms 不同，按
value-owner 安全校验正确拒绝；同一根因结果里的
`root_cause_target_self_state` 行已携精确
`13762.835861..13762.837270`，却没有进入 blocking authority。这使正文
虽然从 value-owner lane 拿到正确区间，仍缺少
`blocking_occurrences_present=true` 的明确反否定权限。

更关键的新 gap 是事务 census 与阻塞 occurrence count 没有分型。真实
`ipc_graph` 完整结果包含 20 条 edge：

- `sync_request=5`；
- `oneway_request=10`；
- `reply=5`；
- transaction 12145859 自身字段是 `flags=0x10, code=0x19`。

正文却把“1 个已证 blocking occurrence”写成“窗口只发出 1 次同步事务”，
并从相邻原始行借来 `code=0xa`。这是 count caliber 与 native-row identity
双重错配，不是 Binder/H1 单点。

PyO3 写用例没有进入 apply：write analyzer 同时发布
`affects_public_api=false`、`changes_persistence=false`、
`changes_build_system=false` 和 `overall=high`，审批门据此正确 fail-closed
为 `high_write_risk`。计划所述改动是 package-local、保持签名的
`nth/nth_back` 边界 bugfix；这里需要校准的是模型风险口径，不能放宽或
绕过 high-risk 审批。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B12-AE1 | P1 | IPC request count caliber | IPC 请求数与目标 blocking occurrence 数共用叙事通道，1 个阻塞区间被写成 1 个同步事务 | 从完整 typed ipc_graph 按 sender/window 构造 request census，分别发布 sync/oneway/unknown；与 blocking authority 明确隔离 | covered-pending-replay |
| EVAL-B12-AE2 | P1 | native IPC row identity | transaction id/flags/code/peer/timestamp 无同一行权限，字段可从相邻 IPC 事件串台 | 每个 sync request 发布同一 typed edge 的 transaction/flags/code/peer/send/receive/known-state；writer 逐 tuple 转录 | covered-pending-replay |
| EVAL-B12-AE3 | P1 | exact target-self blocking admission | critical drill 包络包含相邻阶段，按 span/value 校验被拒；精确 target-self-state 行未被 blocking authority 消费 | 保留 span≈value 硬条件，同时接纳 typed `tier=target_self_state` 根因行；发布 occurrence-present 反否定位 | covered-pending-replay |
| EVAL-B12-AF1 | P1 | write risk caliber | 变更前问题严重度被误当成补丁误施爆炸半径，三风险轴全 false 的局部 bugfix 被标 high | 只增强 write-analysis soft rubric：overall 衡量改动 blast radius；局部保持签名、无持久化/构建/安全面的 bugfix 通常 low/medium；不改审批门 | covered-pending-replay |
| EVAL-B11-AB4 | P2 | incomplete enumeration wording | incomplete 仍可与全部/仅有措辞并存 | 保持已立案；不扫描答案原文做 hard reject | reproduced-filed |

批 AE 不变量：

1. 不读取 raw request、case ID、模型 thinking 或最终答案词面做判定。
2. IPC census 与 native tuple 只来自 deterministic `ipc_graph` typed
   edge；完整性来自 typed compaction 状态。
3. IPC send→receive 仍是 transport latency，不进入 blocking wall clock。
4. target-self-state 只有同时满足 typed target、`tier=target_self_state`、
   selected window 和 `span width≈value` 才能进入 blocking authority。
5. 不改 trace_query 的边构造/配对、用户窗、因果投影、榜位、wakeup
   chain、窗内可消除量或自动补采。

批 AE 已实现独立的 `Trace IPC Request Census Authority`。生产
`traceQueryTypedObservations` 按 sender 对非-reply edge 分组，发布
request 总数及 sync/oneway/unknown 精确分解；每条 sync request 另带同一
typed edge 的 transaction id、flags/code known-state、peer、
send/matched-receive 和 receiver source。edge compaction 存在时只发布
lower bound；分解不守恒、sync roster 不完整或目标不匹配均 fail-closed/
降级，绝不从 blocking occurrence 或模型摘要补字段。

blocking authority 同时接纳精确的 `root_cause_target_self_state` no-seat
症状行，仍保留 deterministic runtime artifact、hard grounding、typed
user target、selected window 和 `span width≈published ms` 全部条件。
因此 H1 的宽 critical drill 包络继续被拒，精确 1.409ms target-self 行
取得唯一 occurrence；查询、投影和数值完全不变。

answer-writer 明示 request count 与 blocking occurrence count 分型，并
发布 `blocking_occurrences_present`；explorer/finalizer 只增加 typed soft
guidance。专项测试覆盖真实 H1 数量/字段形、宽包络拒绝与精确 target-self
接应、破损分解拒绝、roster 缺失降级、生产 tool/writer 接线及 note-key
注册闭包。完整
`go test ./internal/types ./internal/agent ./internal/skill ./internal/tool
-count=1` 通过（types 18.420s、agent 3.047s、skill 1.576s、tool
164.525s）。

批 AF1 只更新 `write-analysis-skill` 的 soft rubric：`overall` 明确衡量
“拟议补丁若误施”的 mutation blast radius，不是既有故障的严重度；保持
公开签名、且不触及持久化、构建、安全、权限、远程执行或不可逆数据面的
package-local bugfix 通常应为 low/medium，high 留给真实宽面或高影响改动。
审批 evaluator、typed diff/path corroboration、high-risk 人工批准和
critical deny 逻辑均未修改。专项测试钉住上述口径及
“never overrides deterministic approval gate”，完整
`go test ./internal/skill -count=1` 通过（0.713s）。

### B12 r2 人工审计与批 AG 规划（2026-07-31）

在 revision `487f89a3c8c4` 重建后，以严格 `parallel=2` 回放：

- `eval/results/real_trace_h1_binder_true_false_attribution-20260731-152238`
- `eval/results/github_issue_pyo3_iter_nth_overflow_symptom-20260731-152238`

runner 1/2，人工 0/2。

AE 的值/字段修复已由真实回放确认。H1 的完整 `ipc_graph` 被正确投影为
`requests=15, sync_request=5, oneway_request=10, unknown=0,
coverage_status=complete`；五条 sync roster 保持同一 edge tuple，
transaction 12145859 的 native code 正确为 `0x19`。target-owned binder
blocking occurrence 正确为 `13762.835861..13762.837270`、1.409ms；
on-chain rank board 仍按 58.320/7.405/4.710/3.956/3.670/3.598/3.309ms
发布。显式用户窗、Trace 因果投影、自动补采、目标四态、wakeup chain 和
窗内可消除量均未回退。

但 typed blocking authority 的 coverage 是
`lower_bound_capacity_truncated`，正文仍写成“整个窗口总计、全部、唯一，
其余请求未产生阻塞”。这说明 prompt-level soft authority 不足以稳定承载
coverage caliber。修复不能扫描答案中的“唯一/全部”等词做 hard gate；
应从 typed authority 无条件追加确定性 coverage disclosure，明确 observed
lower bound 只证明已列出的 occurrence，不能证明全窗 exhaustive absence。

同一答案还把 blocked-reason census 的 50 条记录
（`fscache_page_get_an×39, Σ14.756ms` +
`fscache_page_wait_o×11, Σ1.602ms`）扩成“65 段、70.338ms 睡眠全部归属
fscache”。这两个量纲不等价：blocked-reason 是 caller-linked record census
及记录自报 delay，scheduler sleep 是状态区间墙钟；没有 typed 完整对账时
只能陈述 50 条已记录 caller，不能外推全部 sleep partition。

PyO3 的 AF1 已生效：write analyzer 改为 `overall=medium`，审批自动执行；
`nth/nth_back` 的 checked arithmetic、越界耗尽语义和 `usize::MAX` 测试
均正确落盘，root `make check` 也以 exit=0 输出
`PyO3 iterator skip regression checks passed`。最终失败来自验证归属：
TestSurface 的 `make@.` 是真实 `check` target，但 changed-path coverage
仍把 Make 固定映射为 C/C++，拒绝授权同根 Rust 文件。这是跨语言 project
runner 的 typed ownership gap，不是补丁或测试失败。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B12-AG1 | P1 | polyglot project verification ownership | language-agnostic Make test target 成功后仍按 C/C++ family 判覆盖，跨语言 behavioral oracle 被丢权 | 只有当成功命令精确匹配 filesystem-derived TestSurface candidate（runner/root/target/command/test-signal）时，允许 meta runner 覆盖其 working root 内的 recognized changed paths；裸命令继续 fail-closed | covered-pending-replay |
| EVAL-B12-AG2 | P1 | lower-bound publication authority | typed lower-bound 只进 prompt，模型仍可发布 exhaustive total/absence | 从 typed blocking authority 无条件 materialize coverage caveat；不扫描请求/答案词面，不改模型正文和数值 | planned |
| EVAL-B12-AG3 | P1 | record census vs state-duration caliber | blocked-reason occurrence count/record delay 被外推成完整 scheduler sleep 分区 | typed caveat 明确 caller-record census 只解释其自身记录；除非与完整 state occurrence roster 对账，不授权“全部 sleep” | planned |
| EVAL-B11-AB4 | P2 | generic incomplete enumeration wording | 其他 enumeration authority 仍可能只靠 prompt 被写成 exhaustive | AG2/AG3 先覆盖本轮高危 typed calibers；通用结构化 coverage carrier 后续统一收敛，不扫描 prose | reproduced-filed |

批 AG 不变量：

1. 不读取 raw request、case ID、模型 thinking 或最终答案词面做 hard gate。
2. 不改显式窗、Trace 因果投影、系统自动补采、根因榜位/数值、wakeup
   chain、窗内可消除量、IPC edge 或 scheduler interval 构造。
3. Make 跨语言权限必须绑定真实 TestSurface 的
   `HasTestSignal=true + exact working_dir + exact MakeTarget + exact command`；
   任一不匹配继续按语言 family fail-closed。
4. coverage caveat 只消费 deterministic typed authority；不重写模型正文，
   不从答案用词反推触发条件。
5. complete IPC request census 与 lower-bound blocking occurrence 各守自身
   量纲，永不互相补齐；blocked-reason record census 不等于 sleep wall-clock
   partition。

批 AG1 已把 TestSurface 的附着提前到 changed-path coverage 判定之前，并
新增 typed polyglot meta-runner admission。当前仅 Make 进入该臂：成功命令
必须逐字段匹配 filesystem-derived candidate 的 runner、working directory、
declared target 和 canonical command，candidate 必须有真实 test signal；
匹配后只覆盖该 working directory 内的 recognized changed paths。不存在
typed candidate、target/command 不同或 arbitrary model-selected Make
命令均继续 fail-closed，不把“任意成功命令”升级成项目验证。

回归同时固定正负臂及生产 E2E：root `make check` 可验证同根 Rust 改动并在
`ExecutedCommand.covered_paths` 记录精确路径；同一仓若 typed target 是
`make test`，裸 `make check` 不得越权。专项
`go test ./internal/tool -run
'TestChangedPathCoverage|TestRunTestsTypedPolyglotMakeSurfaceCoversRustChanges|
TestRunTestsMakeReVerifyRunsRealTargetNotRunnerName' -count=1` 通过（1.276s）。
