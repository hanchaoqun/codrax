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
| EVAL-B1-R19 | P1 | Last-mile 发布权限 | 主 materializer 已按 focused-fact gate 禁止 raw observation dump，但 `renderTraceQueryObservationSupplement` 是独立最后一公里通道，未消费同一答案形状权威，仍把 220 条后台观测按 cap 发布 44 条 | last-mile renderer 复用 `IsFocusedRuntimeFactQuestion`；聚焦事实保留主答案、typed target authority、范围/算术等必要附注，但不发布通用因果/背景 raw dump；diagnostic/root-cause/call 负例保持 | 已施工，待回放 |
| EVAL-B1-R20 | P2 | 算术关系绑定 | duration→percentage 正则允许跨 96 字符自由连接，把前一指标 `sleep=85.915ms` 与后一指标 `io_wait<0.5%` 拼成一条关系，生成 59.433% 假警报 | 关系发现先按句内 clause/显式关系连接器绑定，同一片段出现新的 metric subject 时拒绝跨主体配对；保持 advisory，不提高为硬门 | 已施工，待回放 |
| EVAL-B1-E2 | P2 | Eval 表格语义 | principal regex 要求每个数值后重复 `ms`，但 Markdown 表格已经在“时长（ms）”列头声明单位，三行数值本身语义完整，runner 假 FAIL | 不降低产品事实 bar；为 principal 表格 oracle 增加列头单位继承或等价的结构化表格断言，禁止仅为一个 PID/数值放宽全答匹配 | 已施工，待回放 |
| EVAL-B1-R21 | P0 | 终局一致性接线 | typed occurrence 硬门只扫描 `BlockSummary` 或显式 `SurfacePrincipal`；模型可把同一主答案拆成普通 `section/table/list` 且省略 surface_role，错误可见事实因此绕过完整 roster | 一致性检查消费所有模型生成的可见 block，排除带不可伪造 `SystemGeneratedKind` 的系统块；仍只在 complete target roster + 已开始发布 occurrence 关系时激活，不解析题面/PID/数值 | 已施工，待最后回放 |
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
