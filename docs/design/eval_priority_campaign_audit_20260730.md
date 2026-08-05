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
| EVAL-B12-AG1 | P1 | polyglot project verification ownership | language-agnostic Make test target 成功后仍按 C/C++ family 判覆盖，跨语言 behavioral oracle 被丢权 | 当时采用 exact TestSurface + declared-input roster；B43 证明“读取输入”不能代表“执行目标语言语义”，其硬权限已由 `EVAL-B43-METAAUTH1` supersede | superseded-by-B43 |
| EVAL-B12-AG2 | P1 | lower-bound publication authority | typed lower-bound 只进 prompt，模型仍可发布 exhaustive total/absence | 从 typed blocking authority 无条件 materialize coverage caveat；不扫描请求/答案词面，不改模型正文和数值 | covered-pending-replay |
| EVAL-B12-AG3 | P1 | record census vs state-duration caliber | blocked-reason occurrence count/record delay 被外推成完整 scheduler sleep 分区 | typed caveat 明确 caller-record census 只解释其自身记录；除非与完整 state occurrence roster 对账，不授权“全部 sleep” | covered-pending-replay |
| EVAL-B11-AB4 | P2 | generic incomplete enumeration wording | 其他 enumeration authority 仍可能只靠 prompt 被写成 exhaustive | AG2/AG3 先覆盖本轮高危 typed calibers；通用结构化 coverage carrier 后续统一收敛，不扫描 prose | reproduced-filed |

批 AG 不变量：

1. 不读取 raw request、case ID、模型 thinking 或最终答案词面做 hard gate。
2. 不改显式窗、Trace 因果投影、系统自动补采、根因榜位/数值、wakeup
   chain、窗内可消除量、IPC edge 或 scheduler interval 构造。
3. Make 跨语言权限必须绑定真实 TestSurface 的
   `HasTestSignal=true + exact working_dir + exact MakeTarget + exact command`
   以及目标文件的 exact declared-input witness；任一不匹配继续按语言
   family fail-closed。
4. coverage caveat 只消费 deterministic typed authority；不重写模型正文，
   不从答案用词反推触发条件。
5. complete IPC request census 与 lower-bound blocking occurrence 各守自身
   量纲，永不互相补齐；blocked-reason record census 不等于 sleep wall-clock
   partition。

批 AG1 已把 TestSurface 的附着提前到 changed-path coverage 判定之前，并
新增 typed polyglot meta-runner admission。当前仅 Make 进入该臂：成功命令
必须逐字段匹配 filesystem-derived candidate 的 runner、working directory、
declared target 和 canonical command，candidate 必须有真实 test signal；
并且只覆盖 target 的 `DeclaredCoveragePaths`。该 roster 由选中 Make rule
的现存直接 file prerequisites、recipe 中的现存 file arguments，以及直接
调用的本地测试脚本内 exact quoted repo-relative existing paths 做有限静态
展开；动态变量、生成路径、命令替换和嵌套 runner 均保持 unresolved/
fail-closed。不存在 typed candidate、target/command 不同、目标文件不在
declared-input roster，或 arbitrary model-selected Make 命令均不得升级成
项目验证。

完整回归首先抓住并守住 Java 负臂：`make check` 仅 echo 成功、没有 Java
输入 witness 时，不能授权 Java test path。PyO3 同构正臂的
`tests/check_iterators.py` 则 exact 引用并读取 list.rs、tuple.rs 和
iterators.rs，因此获得逐路径权限并写入
`ExecutedCommand.covered_paths`。typed target/command 不同和无
declared-input 的两条负臂均保持 unavailable。

批 AG2 不检查答案是否真的出现“唯一/全部”等词，而是在
`TraceBlockingWallClockAuthority.coverage_status` 为
`lower_bound_capacity_truncated` 时无条件追加用户可见口径：
`observed_blocking_lower_bound`、`observed_occurrences>=N` 只证明列出的
下界，不授权全窗总量、唯一 occurrence 或其余请求无阻塞。同时携同窗 IPC
request census 作为独立量纲，明确 complete request count 仍不等于
complete blocking-occurrence count。

批 AG3 从 hard-grounded deterministic `blocked_reason_census` typed record
发布 `caliber=caller_linked_record_census_not_scheduler_state_partition`：
保留 exact record count、caller roster 和 Σdelay，但明确它们只拥有自身
caller-linked records，不能单独证明每段 sleep 都有这些 caller，也不能
外推整段 sleep wall clock。两项 materializer 均只匹配 typed user runtime
target、共享 full-report shape gate、幂等且不改写模型正文。

专项覆盖 Make declared-input 正负臂、Java echo 负臂、typed lower-bound、
IPC count 分型、blocked-reason 非目标拒绝、幂等、正文 byte-preserve 和
生产 persist 接线；完整 `go test ./internal/tool -count=1` 通过
（164.447s）。

### B12 r3 人工审计与批 AH（2026-07-31）

在 revision `4cf624355c6b` 重建后，以严格 `parallel=2` 回放：

- `eval/results/real_trace_h1_binder_true_false_attribution-20260731-155419`
- `eval/results/github_issue_pyo3_iter_nth_overflow_symptom-20260731-155419`

runner 2/2，人工 1/2。

PyO3 已确认 AF1/AG1 全链生效。write risk 为 medium 并自动执行；首个计划
仍有模型波动，使用 `saturating_add` 和 raw reverse subtraction，真实
`make check` 精确报出 checked arithmetic 缺失，系统没有误签通过。durable
replan 后 list/tuple 均使用 `checked_add`/`checked_sub`，越界时耗尽 iterator，
补齐 `usize::MAX` 和 forward/backward 回归；第二次 `make check` exit=0，
`src/types/list.rs`、`src/types/tuple.rs`、`tests/iterators.rs` 三条
changed-path ledger 均为 `project_runner/make/covered`，最终 verified。

该用例耗时 420s、经历一次真实 red verify，但 correctness 已由既有闭环
保证。首计划偏离明确 acceptance outcome 属于模型波动；按用户裁定，不为
Rust token、`nth` 名字或单 case 增加 hard gate。后续若跨语言/多用例重复
出现“计划忽略 typed acceptance contract”，再立通用 plan-contract
corroboration，不在单 witness 上过拟合。

H1 的所有数据与红线正臂保持正确：显式 233.190ms 用户窗、Trace 因果投影、
系统自动补采、目标四态、wakeup chain、根因排序、窗内可消除量、5 sync +
10 oneway census、transaction `0x19` 和 1.409ms exact occurrence 均在。
AG2/AG3 的确定性文本也准确出现：

- blocking 1.409ms / occurrence>=1 是
  `lower_bound_capacity_truncated`，不能授权总量、唯一或其余请求无阻塞；
- 50 条 blocked-reason caller records 不是 65 段/70.338ms sleep partition。

但这两条被渲染在约 1100 行完整报告尾部，模型摘要和主列表仍先写“唯一、
其余未阻塞、全部 sleep”。语义纠错存在但用户很难在决策点看到，属于
authority placement gap，而非新值通道 gap。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B12-AH1 | P1 | typed correction placement | document-level Caveats 固定在全部 blocks 后渲染，关键 coverage correction 被千行 trace detail 淹没 | 将同一 typed authority 转成 authenticated runtime-trace caveat block，插入 summary 后；report hierarchy 给该 exact system ID 决策前置席位 | covered-pending-replay |
| EVAL-B12-AG1 | P1 | polyglot verification ownership | PyO3 真回放需 Make 接管 Rust behavioral oracle | 历史 replay 只证明脚本成功与输入依赖，不能证明 Rust 原生行为；B43 已撤销该跨语言硬确权 | superseded-by-B43 |
| EVAL-B12-AG2/AG3 | P1 | coverage caliber | typed 内容正确但原展示位过晚 | 内容 verified；placement 由 AH1 收尾 | verified-partial |

批 AH 不变量：

1. 不扫描 raw request、case ID、模型 thinking 或模型答案词面。
2. 不重写、删除或重排任何模型 block；模型 blocks 之间的相对顺序保持。
3. 不改显式窗、Trace 因果投影、自动补采、根因榜/数值、wakeup chain、
   窗内可消除量、IPC/blocked-reason 计算。
4. authority block 只由 AG2/AG3 已有 typed builders 触发；完整 coverage 或
   非目标记录不得铸块。
5. exact reserved block ID + in-memory `SystemGeneratedKind` 防止模型伪造
   或抢占；达到 block cap 时 fail-closed 不新增。

批 AH1 将两条 document-tail caveat 改为
`runtime_trace_blocking_coverage_authority` 与
`runtime_trace_blocked_reason_census_caliber` 两个 authenticated system
block。共享插入器把它们放在首个 summary 后；hierarchy 将这两个 exact ID
保留在 tier 0，因而模型 summary 仍是首块、authority 紧随其后、其余模型
明细仍保持原顺序，完整 Trace 因果投影随后照常发布。触发、数值和文本仍
全部来自 typed authority，不检查模型是否真的写了错误词。

专项测试钉住 direct materialization、生产 persist 后前三块顺序、reserved
ID/system marker、幂等、非目标拒绝和正文 byte-preserve。
完整 `go test ./internal/tool -count=1` 通过（159.722s）；因此 AH1
不是只在局部 fixture 上成立，既有 Trace 报告构造、投影与补采接线也保持
全包回归绿。

### B13 r1 人工审计与批 AI（2026-07-31）

在 revision `13ae05636` 重建后，以严格 `parallel=2` 回放：

- `eval/results/real_trace_h1_binder_true_false_attribution-20260731-161505`
- `eval/results/real_trace_h2_dstate_dma_fence_triform-20260731-161505`

runner 1/2，人工 0/2。

H1 的显式 233.190ms 用户窗、完整 Trace 因果投影、系统自动补采、根因榜、
wakeup chain、窗内可消除量、5 sync + 10 oneway census、transaction
`0x19` 与 1.409ms target blocking 下界均未回退。AH1 也确实把 blocking
coverage 与 blocked_reason caliber 放到了完整投影之前。但模型摘要仍先写
“总 binder 阻塞时间（4次同步请求）2.691ms”：该数实际是五条
send-to-receive transport latency 之和；同一答案的表又列五条请求，并继续
声称只有一次 blocker、其余没有阻塞。typed authority 内容正确，但仍位于
模型摘要之后，错误主值已先占据决策首屏。

H2 不是 oracle 波动。确定性 `target_window_states` 给出的目标
CompThread_0-2955 同窗状态账为 running 74.915ms、runnable 1.536ms、
sleep 118.586ms、non-IO D-state 36.757ms、IO wait 0。同一引擎结果还发布
`target_window_wait_occurrences=complete` 与 11 条独立 occurrence 行；
而 blocked_reason census 是另一量纲的 12 条 caller-linked records、
Σdelay=39.157ms。模型把后者当成 D-state 发生次数/墙钟，并自行叙述 9 次
sched exit。根因是精确用户窗的 D-state 请求被 focused-fact 优化收窄为仅
补 `window_stats`，导致 `root_cause_rank` 与 `critical_blocking_calls`
没有系统补齐；旧 oracle 要求的 4 条锚定发生段和
`自身·D-state 36.757ms` 因此一起消失。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B13-A1 | P0 | explicit-window supplement authority | 精确用户窗被窄事实优化降级，完整因果家族重新依赖模型 view 采样 | typed `RuntimeArtifactScopeProfile.ExplicitTimeWindow()` 优先于 narrow-D 优化；精确窗恢复 root rank + critical 两核心家族，无窗窄事实仍只取 state | covered-pending-replay |
| EVAL-B13-A2 | P1 | principal-value placement and caliber | 状态墙钟/发生次数、blocked_reason records、IPC transport 与 blocking 下界虽有 typed authority，但模型摘要先发布冲突主值 | 从完整同结果 occurrence rows 构造 fail-closed 主值卡；状态/occurrence、blocking coverage、blocked_reason 三个 authenticated authority block 全部置于模型叙事之前并声明冲突优先级 | covered-pending-replay |

批 AI 不变量：

1. 只读取 typed request scope、typed runtime target、hard-grounded
   deterministic ledger records 和 compiled projection；不扫描 raw request、
   case ID、模型 thinking 或答案原文做 hard gate。
2. 精确用户窗继续完整补齐根因排序、critical blocking、wakeup chain、
   自身状态发生段与因果投影；无时间窗的窄 D-state 事实仍走低成本
   `window_stats`，不把所有状态查询扩大成全量因果分析。
3. occurrence 主值只在 aggregate 声明 `complete`、`emitted==total`，且
   1..N 每条 hard-grounded row 均来自同一 result/source、同一 subject、
   位于 aggregate window 内、span 与 duration 自洽时发布；缺行、重复冲突、
   跨 artifact 拼接或越窗均 fail-closed。
4. blocked_reason record count/Σdelay、IPC request/transport、target
   blocking lower bound、target scheduler-state wall clock 永不互相补齐。
5. 不修改模型正文内容；authenticated system authority 领先模型 blocks，
   模型 blocks 之间相对顺序保持，完整报告和投影仍按既有 hierarchy 发布。

批 AI1 已使精确窗的窄 D-state 请求恢复
`root_cause_rank + critical_blocking_calls` 系统补采。端到端测试从仅一次
`event_search` 的模型采样出发，确认系统使用 typed 3.0..3.2 用户窗并恢复
锚定目标、自身 D-state、`dma_fence_default_wait` 等待对象和根因席位；无窗
窄事实正臂仍只请求 `window_stats`。

批 AI2 新增 target-thread state principal authority：沿用已选中的 compiled
projection 状态账，并从未受 prompt 八行上限影响的独立 occurrence typed
rows 复核完整 roster，发布精确发生次数及 occurrence 墙钟和；只有与
D/IO/S-iowait 状态账在 0.002ms 内一致时才声明恒等关系。blocking、
target-state 与 blocked_reason 三个 reserved system block 现均领先模型
叙事，明确后续模型冲突时以系统 typed authority 为准；模型正文逐字保留。

专项测试覆盖精确窗/无窗路由、真实 supplement 执行与投影恢复、完整
occurrence 正臂、缺行/冲突/跨 artifact/越窗负臂、主值渲染、三 authority
顺序、空 authority 不铸块、exact-fit cap 不挤占既有决策面、幂等和生产
persist 接线。完整回归通过：

- `go test ./internal/tool -count=1`（161.437s）；
- `go test ./internal/types ./internal/agent ./internal/orchestrator ./internal/skill -count=1`
  （types 18.901s、agent 4.080s、orchestrator 12.342s、skill 1.177s）。

首轮全包曾抓到固定 authority lead 在零 typed row 时仍铸空块，导致普通读
任务 reasoning graph 多一块、exact-fit trace 报告挤掉 next_steps。修复在
三 materializer 的 typed row 集为空时直接 no-op，并新增专门负臂；没有
提高 64-block cap、删除模型 block 或牺牲既有投影/行动面。

### B13 r2 人工审计与批 AJ（2026-07-31）

在 revision `42ecbb98a` 重建后，以严格 `parallel=2` 回放：

- `eval/results/real_trace_h1_binder_true_false_attribution-20260731-164336`
- `eval/results/real_trace_h2_dstate_dma_fence_triform-20260731-164336`

runner 1/2，人工 0/2。

H1 证明 AI 的三张系统 authority 已真正领先模型正文：1.409ms binder
blocking 明确是 `lower_bound_capacity_truncated`，目标线程状态账为
157.248/5.604/70.338/0ms，blocked_reason 为 50 条 caller-linked records
而不是 sleep partition。显式用户窗、完整 Trace 因果投影、系统自动补采、
根因排序、wakeup chain、IPC census 和窗内可消除量均未回退。但后续模型
正文仍声称“只有一次/其余未阻塞/49 段全部为 fscache”，且 39+11 的
blocked_reason roster 本身就是 50 条。typed authority 已降低误导风险，
但答案仍内部冲突，人工继续判 fail；不能用扫描“唯一/全部”等答案词的
hard gate 修补。

H2 的模型本轮已正确区分 11 条 D-state segment、36.757ms 状态墙钟与 12 条
blocked_reason record、39.157ms 自报 delay，却无 typed 证据地把差额归因
为跨窗。更深的确定性故障发生在 supplement 之前：analyzer 发出了
`entities=["CompThread_0-2955"]` 和精确
13762.791708..13763.024898 用户窗，却漏发必需的 `runtime_targets`；六次
模型调用又分别使用 `thread=CompThread_0-2955` 与 `thread=pid=2955`。
两者在 trace engine 中都是 TID 2955，但游标登记层保留原字符串，
`traceSupplementDeriveTarget` 因此误判多目标并记录
`skip reason=no_typed_target`。结果只有降级状态投影，没有系统补齐的
root rank、wakeup chain、锚定发生段与可消除量。这是 selector identity
归一 gap，不是 oracle 或模型波动。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B13-AJ1 | P0 | equivalent selector identity | 引擎接受的 numeric、`pid=`、bracket、space、`name-pid` 形在游标层各自成 target | 暴露并复用 engine 单一 selector parser；游标以 kind+positive PID+source 合并，名称只作展示，异 PID 继续歧义失败 | covered-pending-replay |
| EVAL-B13-AJ2 | P1 | analyzer target emission recovery | analyzer entity 存在但 runtime target 漏发时，补采可恢复证据而答案 authority 仍只认原请求 target | 仅在 strict typed entity 与已实际执行 supplement meta 同 PID 时，为答案 builder 私有克隆注入 consensus target；不回写请求模型 | covered-pending-replay |
| EVAL-B13-A3 | P1 | typed authority vs contradictory model prose | 系统主值虽领先，后续模型叙事仍可发布相反 exhaustive 结论 | 保持立案；优先寻找系统拥有结论结构的通用方案，不扫描/删改模型 prose，不在 H1 词面过拟合 | open |

批 AJ 不变量：

1. 不读取 raw request、case ID、模型 thinking 或答案原文做 hard gate。
2. selector 归一只复用 trace engine 已接受的结构语法；PID 是身份主键，
   不同正 PID、不同 target scope 或 user/cursor provenance 不合并。
3. 显式时间窗仍补齐 `root_cause_rank + critical_blocking_calls`，并保留
   wakeup chain、自身状态发生段、根因榜和窗内可消除量；无窗窄 D-state
   事实继续保持低成本 `window_stats`。
4. 模型 cursor 单独不能升级成用户 authority。AJ2 必须同时看到严格
   `name-pid` typed entity、至少一个已成功执行的 targeted supplement view、
   允许的 cursor/entities-fallback source 及完全相同 PID；仅 census-lite、
   失败/空 results 或任一其他条件缺失都 fail-closed。
5. AJ2 只返回 answer-time `RequestModel` 私有克隆，不修改 AnalysisIR、
   Mutable request model 或后续工具继承/补采选路。

批 AJ1 在 `internal/tracequery` 暴露唯一 selector identity parser，覆盖
bare numeric、`pid/tid/thread_id` token、bracket、space 与末尾
`name-pid` 形。`traceQueryRecordExplicitRuntimeTarget` 在登记前调用该
parser；同 cursor provenance、同 kind、同正 PID 的重复拼写合为一条：
空名与有名取有名，两条不同非空名降为 pid-only；不同 PID 保持两条并让
补采 fail-closed。端到端 fixture 从无 runtime target、只有 typed entity
和两种 cursor 拼写出发，确认精确用户窗重新执行 root rank + critical 两
核心 view 并保存 canonical target。

批 AJ2 新增 answer-time typed target consensus。已有非 cursor 用户 target
仍优先；缺失时必须由严格 entity parser 与实际执行的 system supplement
meta 对同一 PID 达成一致，且至少一个 targeted view 成功，才给 blocking、
IPC census、complete target wait occurrence 与 blocked_reason builder
提供私有 target。负臂覆盖 cursor-only、PID 不同、仅 census-lite、
失败 result 和 meta 无实际结果；
正臂确认完整 occurrence roster/墙钟恢复而持久请求模型不变。

完整回归通过：

- `go test ./internal/tracequery ./internal/tool ./internal/types ./internal/agent ./internal/orchestrator ./internal/skill -count=1`
  （tracequery 69.978s、tool 161.756s、types 19.817s、agent 3.517s、
  orchestrator 14.447s、skill 0.585s）。

### B13 r3 人工审计与批 AK（2026-08-01）

在 revision `9de705742` 重建后，以严格 `parallel=2` 回放：

- `eval/results/real_trace_h1_binder_true_false_attribution-20260731-170125`
- `eval/results/real_trace_h2_dstate_dma_fence_triform-20260731-170125`

runner 2/2，人工仍为 0/2。

H2 已恢复完整精确窗因果面：supplement 在
13762.791708..13763.024898、PID 2955 上执行 `root_cause_rank`，最终报告有
根因排序、wakeup chain、`自身·D-state 36.757ms`、窗内可消除量及 typed
覆盖边界；H1 的完整投影亦无回退。本轮 analyzer 自身发出了有效
`runtime_targets`，所以真实 replay 证明 AJ 不破坏正常 typed-target 路径，
但没有单独重演 r2 的漏发 + 双 selector 形；AJ 的该形由端到端 fixture
守住。

runner PASS 仍掩盖了 H2 的 principal-value gap。引擎 payload 中
`wait_occurrence_status=complete`，11 条不相交 D-state occurrence 从
13762.811273..13763.014598，墙钟和严格为 36.757ms，且 caller 一致为
`dma_fence_default_w+0x260/0x4dc[devhost.elf]`。12 条/39.157ms 则是另一
量纲的 blocked_reason record/delay census。AI2 的 complete-roster builder
其实已看到全部 11 行，却在最后关联处把 projection 的 artifact label
`attached_trace.txt` 与 wait authority 的 `attached_trace` 判成不同 artifact，
所以主值卡没有发布 11 次；模型随后用 12 次回答“发生几次”，又无 typed
证明地把差额说成窗口尾部触发。

同一 state account 的 running/runnable/sleep/D-state 合计为 231.794ms，
而显式窗为 233.190ms，差 1.396ms。`tail_open=8.793ms sleep` 已包含在
sleep，不是该差额的补数；剩余区间没有足够调度边界，不能猜分给任一状态。
旧主值卡写 `partition_total=231.794ms` 却没有 coverage status，容易被理解
为完整分区。

H2 还有一个较低优先级的既有词义债：因果树的 `4次(3.774~16.064ms)` 是
CPU3/1/2/7 四个“每 CPU 汇总桶”，其成员实际为 5/3/2/1 个 interval，总计
11 次。case oracle 明确把 legacy `4次` 当硬 token，所以 runner PASS 不能
回答用户真正的 occurrence count；应另批用 typed fold caliber 将其改成
“4组 CPU 汇总”，不能把该 oracle 继续当语义真值。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B13-AK1 | P0 | cross-authority artifact identity | 同一 SourceRef 在 projection 和 occurrence builder 使用 path label / ArtifactID 两套显示身份，完整 roster 末端失联 | occurrence authority 复用 causal projection 的单一 typed artifact identity resolver；path basename 优先，lane marker 不作物理 artifact | covered-pending-replay |
| EVAL-B13-AK2 | P1 | target state coverage caliber | 状态和小于显式窗时仍显示为 partition total，无未知余量/边界折入说明 | 从 typed window endpoints 与 accounted sum 计算 complete/partial_unaccounted；披露 unaccounted，head/tail carry 只说明已包含；超窗账户 fail-closed | covered-pending-replay |
| EVAL-B13-AK3 | P2 | aggregate group vs occurrence unit | per-CPU state aggregate bucket 的 merged_count 被通用 renderer 写成 N次，与真实 interval count 冲突 | 增加 typed merge caliber（per_cpu_state_aggregate），显示 N组 CPU 汇总；更新旧 oracle 钉正确单位，保持 11 occurrence authority | filed |
| EVAL-B13-A3 | P1 | typed authority vs contradictory model prose | H1 模型正文仍与领先系统主值相反 | 继续立案，寻找 system-owned conclusion 通用结构；不扫/删改模型 prose | open |

批 AK 不变量：

1. 不读取 raw request、模型 thinking、最终答案词面或 case ID。
2. artifact join 读取同一 `ObservationSourceRef` 的 typed path/id，复用
   projection 单一 identity；不同物理 path 仍不得合并。
3. complete occurrence 仅由同 result/source、同 subject、1..N 连续、
   窗内 span-duration 自洽的独立 typed rows构造；11 与 12 两量纲永不换算。
4. 状态 coverage 只做 typed endpoints 与状态总和的算术；未知余量不猜状态，
   `head_carry`/`tail_open` 明示“已包含”，不重复相加；超过窗口的账户拒绝
   成为主值 authority。
5. 不改显式窗 supplement 选路、根因排序、wakeup chain、critical blocking、
   窗内可消除量和无窗窄 D-state 优化。

批 AK1 将 wait authority 的 artifact label 改为调用
`traceCausalProjectionArtifactIdentity`：同一 attached trace 统一得到物理
basename，而不是一边 `attached_trace.txt`、一边 lane marker
`attached_trace`。真实 donghu fixture 回归确认主值卡现发布
`occurrences=11`、`d_state_occurrences=11`、
`occurrence_wall_clock_sum=36.757ms` 与完整 `wait_callers`。

批 AK2 给 target-state authority 增加 `window`、`accounted_total`、
`coverage_status`、`unaccounted`、head/tail boundary-fold typed 字段。
231.794/233.190ms 形发布
`coverage_status=partial_unaccounted, unaccounted=1.396ms`；明确未知部分不能
分配状态，且 tail-open 8.793ms 已包含在 sleep。完整账户保持 complete，
超过自身窗口 0.002ms 容差的账户 fail-closed。

批 AK 完整回归通过：

- `go test ./internal/types ./internal/tool ./internal/tracequery ./internal/agent ./internal/orchestrator ./internal/skill -count=1`
  （types 19.383s、tool 166.671s、tracequery 70.093s、agent 2.765s、
  orchestrator 11.595s、skill 2.163s）。

### B13 r4 人工审计与批 AL 设计（2026-08-01）

在 revision `02a429854` 重建后，以严格 `parallel=2` 回放：

- `eval/results/real_trace_h1_binder_true_false_attribution-20260731-172238`
- `eval/results/real_trace_h2_dstate_dma_fence_triform-20260731-172238`

runner 2/2（H2 181s、H1 191s），人工仍为 0/2；显式窗 Trace 因果投影、
system supplement、根因排序、wakeup chain、critical blocking 和窗内可消除量
均完整，无能力回退。

AK1/AK2 已在真实 H2 生效：答案顶部现在发布同一窗的完整主值：
`occurrences=11`、`d_state_occurrences=11`、
`occurrence_wall_clock_sum=36.757ms`、单一
`dma_fence_default_w+0x260/0x4dc[devhost.elf]` caller，以及
`accounted_total=231.794ms / window=233.190ms /
partial_unaccounted=1.396ms`；tail-open 8.793ms 明确已包含在 sleep。

但模型正文仍回答“12 条 sched_blocked_reason / 8 次 D-state 入口”，同一答案
形成三种次数口径。12/39.157 是 caller-linked record/delay census，不是
scheduler-state occurrence；8 则是模型在 capped 探索行上手工数出的漏计值。
H1 亦仍把 typed `coverage_status=lower_bound_capacity_truncated`、
`observed_occurrences>=1`、`observed_blocking_lower_bound=1.409ms` 写成
“只有/唯一一次”，并断言其余同步请求没有等待。

两例同根不是 trace 数据缺失，而是成文时序：system-owned principal card 在
模型发出 AnswerDocument 后才 materialize；finalizer prompt 的早期 Observation
Ledger 已有 blocking lower-bound，却没有 AK 的 11-row summary 紧凑重述，
且长提示后模型仍优先采用探索叙事。不能靠扫描/删除模型词面修复。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B13-AL1 | P0 | finalize principal-value handoff | 成文后 authority 正确，但 finalizer 未在提交前看到 complete occurrence summary 与最后一次 typed 口径重述 | 在 finalizer 尾部加入 typed-only principal-value recap：同一 authoritative RM、ledger 和 builders；complete 才授权 exact N/sum，truncated 只授权 ≥N/lower-bound | covered-pending-replay |
| EVAL-B13-AK1 | P0 | cross-authority artifact identity | path label / lane marker 曾使 complete roster 末端失联 | 单一 typed artifact resolver，真实 replay 已发布 11-row roster | covered |
| EVAL-B13-AK2 | P1 | target state coverage caliber | 状态和小于窗口却显示成完整 partition | typed complete/partial_unaccounted + boundary fold，真实 replay 已披露 1.396ms | covered |
| EVAL-B13-AK3 | P2 | aggregate group vs occurrence unit | 四个 per-CPU aggregate bucket 被 renderer 写成“4次” | typed merge caliber 后显示“4组 CPU 汇总”；保留真实 11 occurrence authority | filed |
| EVAL-B13-A3 | P1 | typed authority vs contradictory model prose | 模型忽略 completeness/主值 | 由 AL1 先做成文前软接应；不扫描/改写 prose，回放后再判断是否仍需 system-owned conclusion 结构 | subsumed-by-AL1 |

批 AL 不变量：

1. 只消费 typed RequestModel、ObservationLedger、system supplement
   meta/results 和确定性 authority builders；不读取 raw request、case ID、模型
   thinking 或 AnswerDocument 原文。
2. 仅是 finalizer 软指导，不新增 hard gate、contract reject、自动删改或答案词面
   检查；模型波动仍可留档。
3. complete occurrence rowset 必须同 result/source、同 target/window、连续
   1..N、行数/墙钟自洽；否则不授权 exact count/sum。
4. lower-bound blocking 与 complete IPC request census 永远分量纲；后者不能把
   前者补成“唯一/全部/其余无阻塞”。
5. 不改 supplement 选路和显式时间窗因果投影构造，继续守住 root rank、
   wakeup chain、critical blocking、窗内可消除量与无窗窄查询优化。

批 AL1 已实现 `Runtime Trace Principal Values — Final Typed Recap`，位置在
principal boundary 之后、submission checklist 之前。它与成文后主值卡复用
同一个 typed target resolver：正常 user RuntimeTarget 优先；缺失时仍必须由
严格 `name-pid` entity 与已成功执行的 targeted supplement 同 PID 共识，
cursor-only、空/失败/census-only 继续 fail-closed。recap 直接消费
`BuildTraceTargetWaitSummaryAuthorities`、
`BuildTraceBlockingWallClockAuthorities` 与 target-state authority：
complete 11-row 形发布 exact count/sum/caller；capacity-truncated 形只发布
`>=occurrence` 与 `>=wall_clock`，并提示 IPC/census/group 量纲不可替换。

结构 pin 覆盖 11-row 集合、lower-bound 权限、entity+supplement 共识及
recap→submission 的真实接线顺序。完整回归通过：

- `go test ./internal/types ./internal/tool ./internal/tracequery ./internal/agent ./internal/orchestrator ./internal/skill -count=1`
  （types 18.161s、tool 162.868s、tracequery 70.694s、agent 3.658s、
  orchestrator 13.004s、skill 0.850s）。

### B13 r5 人工审计与批 AL2（2026-08-01）

在 revision `2249e12c9` 严格 `parallel=2` 回放：

- `eval/results/real_trace_h1_binder_true_false_attribution-20260731-173547`
- `eval/results/real_trace_h2_dstate_dma_fence_triform-20260731-173547`

runner 2/2（H2 189s、H1 193s），人工 1/2。两份 raw finalizer prompt 均实际
包含 AL1 recap，证明接线有效；两例显式窗 Trace 因果投影、system supplement、
root rank、wakeup chain、critical blocking 和窗内可消除量继续完整。

H2 的 principal answer 已从 r4 的 12-record/8-entry 冲突收敛到正确的
11 个 D-state occurrence、36.757ms、`dma_fence_default_w`，人工主问题通过。
残余有两条次级债：

1. 模型无 typed relation 却猜 12-record/39.157ms 与
   11-occurrence/36.757ms 的差值“来自窗口边界外记录”；当前 authority 只证明
   两者不同量纲，未证明差值机制。
2. 仍把 CPU3/1/2/7 四个 per-CPU aggregate group 的 16.064/10.424/
   6.495/3.774ms 描述成“单次”，归入既有 AK3。

H1 prompt 明确携带
`permission=lower_bound_only, observed_occurrences=>=1,
observed_wall_clock=>=1.409ms`；模型末句也承认
`coverage_status=lower_bound_capacity_truncated`，但首段与标题仍写
“只有/唯一 1 次、总时长 1.409ms”。这已经不是 authority 缺席，而是结构化
权限到自然语言的转换失败。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B13-AL2 | P1 | authority-to-language composition | 模型看见 typed permission 但仍把下界写成 exact，或给跨量纲差值猜原因 | 从 typed authority 直接生成本地化 principal-conclusion 软模板；lower-bound 明说“至少/全窗总量未知”，complete 明说 exact；无 typed relation 时禁止猜差值机制 | covered-pending-replay |
| EVAL-B13-AL1 | P0 | finalize principal-value handoff | principal values 未在成文前到达 | 尾部 typed recap；H2 真实回放主值已收敛 | covered |
| EVAL-B13-AK3 | P2 | aggregate group vs occurrence unit | per-CPU bucket 被写成次数/单次 | typed merge caliber 显示“组/汇总”，另批施工 | filed |

AL2 继续遵守 AL 不变量：模板只由 typed count/value/coverage/status/language
构造，不读取用户原文或模型原文；仅作 prompt 软指导，不作 hard gate、retry、
删除或改写。

批 AL2 已在同一 recap 中加入本地化 `principal_conclusion_zh/en`。complete
rowset 直接给出“确切 N 次、目标等待墙钟合计 Xms”；capacity-truncated
直接给出“至少 N 次、至少 Xms、全窗总次数/总量未知”，并禁止“只有/唯一/
总计/其余无阻塞”。另加入通用 cross-caliber relation boundary：数值差本身
不铸造关系，除非另有 explicit typed relation，否则不得猜成窗口边界、重叠、
精度漂移或缺失闭合。

完整回归通过：

- `go test ./internal/types ./internal/tool ./internal/tracequery ./internal/agent ./internal/orchestrator ./internal/skill -count=1`
  （types 17.528s、tool 165.773s、tracequery 70.623s、agent 4.245s、
  orchestrator 14.206s、skill 2.836s）。

### B13 r6 收账（2026-08-01）

在 revision `1a70d38b8` 严格 `parallel=2` 回放：

- `eval/results/real_trace_h1_binder_true_false_attribution-20260731-174609`
- `eval/results/real_trace_h2_dstate_dma_fence_triform-20260731-174609`

runner 2/2（H1 197s、H2 225s），人工 0/2。AL2 的中文
`principal_conclusion` 与 relation-boundary 均真实出现在 raw finalizer
prompt，剩余问题不是接线遗漏。

H1 模型仍把下界写成“只有 1.409ms/只有一个产生等待”，还把其余 sleep 全归给
fscache；system-owned leading authority 仍正确说明 `>=1 / >=1.409ms /
lower_bound_capacity_truncated`。H2 主答仍含正确的 11 次/36.757ms/caller，
但另造“5 段合计约 43.541ms”并猜跨窗差值原因。相同代码与 typed 主值下，
H2 r5 人工通过、r6 失败，证明正文存在显著模型波动。

本轮停止继续强化 prompt：再围绕“只有/唯一/窗口边界”等词面加 gate 或重写会
违反本战役的泛化约束。B13 收账如下：

1. `AL1/AL2 covered`：typed 主值已在成文前后双面到达；complete 与 lower-bound
   权限有结构化值和本地化模板。
2. `explicit-window non-regression covered`：所有回放均保留完整 Trace 因果投影、
   root rank、wakeup chain、critical blocking、窗内可消除量与系统补采。
3. `model-prose residual accepted`：系统主值正确但模型正文可能矛盾，按模型波动
   留档，不再过拟合。
4. `AK3 filed/P2`：per-CPU aggregate group 的“4次/单次”展示仍需未来 typed
   merge-caliber 批次解决；不在 B13 修改 legacy oracle。

下一步离开 H1/H2，从全量 eval 按客户风险、能力覆盖、历史失败率、运行成本
四轴选择下一对，继续严格 `parallel=2`。

### B14 cases 四轴排序与第 1 对（2026-08-01）

本轮对当前 case inventory 以四轴排序，每轴 0..3：

1. `客户/生产风险`：错误是否会直接误导客户故障定位或代码变更。
2. `泛化覆盖`：能否覆盖一类 trace caliber、跨语言关系、exact resolution 或
   write safety，而不是一个词面。
3. `覆盖缺口`：当前分支真实回放次数；0 次优先，旧回放次之。
4. `执行杠杆`：一次回放能否同时验证多层接线，成本过高且重复覆盖者降序。

| 顺序 | case | 风险 | 泛化 | 缺口 | 杠杆 | 选择理由 | 状态 |
|---:|---|---:|---:|---:|---:|---|---|
| 1 | `real_trace_h3_iofam_one_seat` | 3 | 3 | 3 | 3 | 0 次回放；同一真实客户窗同时检验 IO wall-clock/composite caliber、同 episode 单席与非墙钟展示，能发现跨口径误归因 | selected-B14-r1 |
| 2 | `mr_poly_binding_chain` | 2 | 3 | 3 | 3 | 0 次回放；跨 repo、Python→native module→Rust 关系链与 fallback，补足 trace 战役之外的 source/relation 泛化 | selected-B14-r1 |
| 3 | `real_trace_h4_supply_thermal_witness` | 3 | 3 | 1 | 3 | CPU thread-state/供给/直接 policy-limit 权限高危，但已有 4 次旧回放 | queued |
| 4 | `read_combo_config_absent_present_mix` | 2 | 3 | 3 | 2 | 双目标 absent/present exact-resolution 绑定，0 次；客户风险略低于 trace | queued |
| 5 | `real_trace_h8_semantic_edge_anchor_sentinel` | 2 | 3 | 3 | 2 | semantic span 只有 typed wakeup edge 才能入链，0 次；适合后续 relation authority 对照 | queued |
| 6 | `patch_java_typo` | 2 | 2 | 1 | 2 | write-plan 精准 patch 安全重要，但已有 19 次旧回放且当前批未改 write path | queued-later |

B14 r1 仍严格 `parallel=2`，两例刻意跨能力域。Trace 例继续验收显式窗系统
补采与因果投影；polyglot 例验收完整 relation chain、原生模块桥和 fallback。
人工审计不只看 runner token，还检查因果/调用边是否有证据、量纲是否正确、
是否把 support/context 升格成 principal。

### B14 r1 人工审计与批 WS1 设计（2026-08-01）

在 revision `0f5a2f44e` 严格 `parallel=2` 回放：

- `eval/results/real_trace_h3_iofam_one_seat-20260731-175904`
- `eval/results/mr_poly_binding_chain-20260731-175904`

runner 1/2（polyglot PASS 136s、H3 FAIL 184s），人工 0/2。

H3 不是 trace 证据缺失。模型执行了 6 次有窗 `trace_query`，其中
`window_stats`、`root_cause_rank` 与各 IO 事件查询均使用正确的
`13762.791708..13763.024898 / pid=17267`。真正断点发生在分析 IR：

```text
requested_scope=bounded_selector
time_start=13762.791708
time_end=13763.024898
source_quote=<已锚定的用户精确窗口>
```

`parseRuntimeArtifactScopeProfile` 的 `bounded_selector` 分支随后清空 start/end，
使下游 `ExplicitTimeWindow()` 失败。模型已经采齐 core families，所以系统补采
以 `families_present` 跳过是正确 no-op；缺口是发布层随后把问题识别成
focused runtime fact，反而抑制这些已有 family 的完整报告。最终没有系统
IOFAM 席位/因果投影块，模型自由叙述漏掉
`完成端到端·IO延迟（io_latency）`、`块设备层·块设备IO(inode)` 与
`综合评分,非墙钟`，并把 blocked-reason interval Σ 错写成非墙钟内部估算。

这是 typed 载体内部自相矛盾，而不是要通过用户/答案关键词识别的场景。
最优修复是在 emit-analysis 解析边界 canonicalize：

1. runtime artifact 在场；
2. `bounded_selector` 的 quote 已通过当前请求 exact-anchor；
3. typed `time_start/time_end` 均在场、非负且 `end > start`；
4. 满足时将 scope 规范化为 `explicit_time_window` 并保留时间，记录 warning；
5. 其他 enum/time 冲突继续按原规则清空或 fail-open。

该规则只消费 schema-valid enum、数值和既有 quote-anchor 结果，不扫描用户
关键词或模型答案。它恢复所有精确窗问题的统一 authority，同时不改变真正
无窗状态查询的窄报告策略。

polyglot 的 runner PASS 掩盖另一通用缺口：typed relation ledger 与 finalizer
prompt 已有 Python fast path、pyo3 registration/wrapper、Rust core 和
`_tokenize_slow` fallback 全边；正文结论也正确。但 ordered-list 的
`citation_ref` 错位到相邻 claim，fallback 只留在 summary，未成为有证据的
分支行。当前 validator 只验证 citation index/quote 可用性，不验证一条
relation row 的 subject/predicate/object 是否与所引 citation 的 typed edge
一致。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B14-WS1 | P0 | runtime artifact scope canonicalization | 精确 start/end 与 `bounded_selector` enum 可同时通过，解析器按 enum 丢弃更精确 typed 窗 | 在 emit-analysis 边界以已锚定 quote + 合法 typed 数值规范化为 `explicit_time_window`；补 parse→report-authority 接线 pin | covered-pending-replay |
| EVAL-B14-RC1 | P2 | relation row ↔ citation semantic alignment | citation_ref 仅做结构/存在性校验，无法发现 call edge 行引用相邻定义/调用 | 从 typed relation surface 给 row 提供 endpoint-aligned citation候选或作 advisory；不得扫描答案 prose、不得因启发式相似度 hard reject | filed |
| EVAL-B14-MV1 | P3 | model narrative caliber drift | system IO family 被抑制后，模型把 interval Σ、rank absence 等自由解释为错误口径 | 先由 WS1 恢复 deterministic typed display；回放后仍出现才另行立项 | subsumed-by-WS1 |

WS1 不变量：

1. 不读取/匹配用户或模型答案关键词，不增加 case/type 特判；
2. 无有效双端时间值时绝不升级为显式窗；
3. full-artifact、真正 bounded-selector、unspecified 与无附件行为保持不变；
4. 显式窗继续保留 Trace 因果投影、root rank、wakeup chain、critical
   blocking、窗内可消除量及自动补齐；
5. focused、无时间窗的状态/计数查询继续使用窄报告，不重新套入全量因果合同。

批 WS1 已按上述边界落地。`full_artifact` 仍无条件清掉多余 query-time 字段；
`bounded_selector` 仅在 quote 已通过既有 current-request exact anchor 且双端
typed 时间合法时升级；缺端点、负起点、`end <= start` 均不升级并继续清空
时间。结构测试直接把 parse 结果送入
`RuntimeTraceReportShapeAuthority`，确认不依赖 analyzer 的 intent/scenario
词面即可恢复完整显式窗报告。

回归通过：

- 定向：
  `go test ./internal/tool -run 'TestParseRuntimeArtifactScopeProfileAnchorsUserScopeAndSoftensModelScope|TestRuntimeTraceReportMaterializationAuthorityMatrix|TestSupplementH3WindowMintsIOFacetEndToEndWord' -count=1`
  （3.824s）；
- 完整相关包：
  `go test ./internal/types ./internal/tool ./internal/tracequery ./internal/agent ./internal/orchestrator ./internal/skill -count=1`
  （types 19.173s、tool 169.448s、tracequery 69.743s、agent 3.009s、
  orchestrator 13.284s、skill 1.749s）。

### B14 r2 A/B 审计与批 WS2 设计（2026-08-01）

在 revision `ecc639def` 严格 `parallel=2` 同时回放：

- 显式窗正向：
  `eval/results/real_trace_h3_iofam_one_seat-20260731-181050`
- 无窗 focused 负向：
  `eval/results/real_trace_c2_dstate_iowait-20260731-181050`

runner 2/2（H3 156s、C2 193s），人工 0/2，但两个报告形状边界均符合设计：

1. H3 日志出现 scope canonicalization warning，system supplement 补齐 1 个缺失
   trace view，完整 Trace 因果投影、root rank、wakeup chain、critical
   blocking、窗内可消除量及 IOFAM 单席全部恢复。
2. C2 仍是 `full_artifact` focused runtime fact，只补采 whole-trace
   `window_stats`；没有 materialize 完整因果投影。因此 WS1 没有把状态查询
   错误套回全量因果合同。

H3 的正文仍把 blocked_reason interval Σ 和 scheduler state duration 说成
“非墙钟”；确定性投影正确，属于 typed caliber 到自然语言的模型残余，沿用
`EVAL-B14-MV1`，不加答案词面硬门。

C2 暴露两个同根高 ROI 缺口。typed
`target_window_wait_occurrences` 已明确
`status=complete / emitted=3 / total=3 / sum=0.635ms`，三行起止、duration、
state、iowait、caller 均齐全；finalizer recap 也授权
`permission=exact_complete_rowset`。初稿表格逐条抄对了 3 行，却被
`targetWaitOccurrenceSegmentConflict` 错误拒绝：它把同一 summary 中的分析窗
`34579.450000..34579.600000` 当成 occurrence interval，再把同段任意 duration
token 绑定给该窗。ForceHard 连续拒绝 8 次，最终 breaker/degraded export。

这不是缺一条 regex，而是架构方向错误：对模型自由文本做 hard consistency
scan，天然无法区分分析窗、发生窗、引用窗和表格行。最优方案是：

1. 将 occurrence prose consistency 从 pre-emit hard reject 降为日志/advisory；
2. 由同一 typed authority builder 在 system-owned principal-value card
   确定性发布 complete roster；
3. 只放宽这个小型主值卡在 focused no-window 问题中的发布，不放宽完整因果
   projection；
4. complete roster 明确声明不受无关 result/census
   `capacity_truncated=true` 降级，避免正文“实际可能多于 3 次”的假 caveat。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B14-HG1 | P0 | raw-prose hard-gate false conflict | 同一自由文本段可含分析窗、发生窗和多个 duration；正则式 hard gate 无 typed 归属，正确答案也会被拒绝 | occurrence consistency 仅保留 advisory；不再触发 retry/degraded export | covered-pending-replay |
| EVAL-B14-PV1 | P0 | focused principal-value publication | focused 无窗问题正确抑制 full projection，但连 exact typed roster 的窄权威卡也一起被抑制 | 新增 typed principal-value materialization authority；仅发布 state/complete roster，不发布 root/wakeup/eliminable | covered-pending-replay |
| EVAL-B14-CAP1 | P1 | coverage scope mismatch | 全结果或 blocked_reason census 的 capacity flag 被模型用于降级已 complete 的 target roster | system card 明确 producer-paired complete roster 优先；不同 scope 的容量边界不可跨载体传播 | covered-pending-replay |
| EVAL-B14-MV1 | P3 | model narrative caliber drift | 模型把墙钟 interval/state duration 写成非墙钟 | deterministic authority 已正确；先按波动留档，不做 prose 特判 | filed-model-variance |

WS2 不变量：

1. 不扫描用户原始输入或模型答案原文来决定 hard accept/reject；
2. 不修改 `runtimeTraceFullReportMaterializationAllowed` 的完整报告边界；
3. 显式窗仍发布完整因果投影并保留自动补齐；
4. focused 无窗只获得与问题目标匹配的 typed state/occurrence 主值卡，不获得
   root rank、wakeup chain、critical blocking、窗内可消除量；
5. complete roster 必须来自同 artifact、同 subject、同 selected window、
   连续 ordinal 和自洽 sum；冲突或不完整继续 fail-closed，不猜造行；
6. broad capacity flag 仍保留在其原 observation 上，本批只禁止它越权降级另一
   个已经 complete 的 typed roster，不以删证据方式“修复”。

WS2 已按上述边界落地：

1. `complete_target_wait_roster` 已从 same-turn hard policy 和 subgate hard lane
   移除；全文件 `ForceHard` 生产点 ratchet 从 2 收窄为 1。
   `CheckTargetWaitOccurrencePrincipalConsistency` 仍可提供可观测 advisory，但
   不再进入 reject/repair/breaker。
2. 新增 `runtimeTracePrincipalValueMaterializationAllowed`：完整报告允许时保持
   原行为；完整报告被 focused runtime fact 明确抑制时，只授权 target
   state/wait principal card。其他完整报告 materializer 仍逐个受
   `runtimeTraceFullReportMaterializationAllowed` 约束。
3. `TraceTargetWaitSummaryAuthority` 现在保留同 result 已校验的 occurrence
   结构行；主值卡逐条发布 ordinal、state、start/end、duration、iowait 和
   caller，不从模型正文或相邻 sched 事件重建。
4. 卡片发布
   `roster_scope=producer_paired_complete` 和
   `unrelated_capacity_truncation_does_not_downgrade=true`，明确别的
   observation/census 容量边界不能降级这份 complete roster。
5. 状态主值只保留与 typed RuntimeTarget 匹配的 subject；无状态分区但有完整
   wait roster 时仍可单独发布 wait 主值，冲突/缺行继续 fail-closed。

验证：

- 定向 authority/pre-emit/persist/shape/registry tests 通过；
- `go test ./internal/types -count=1`：20.785s；
- `go test ./internal/tool -count=1`：164.446s；
- 同一代码状态下
  `go test ./internal/tracequery ./internal/agent ./internal/orchestrator ./internal/skill -count=1`
  分别为 76.539s、4.313s、17.214s、3.624s。

### B14 r3 WS2 收账与批 REL1（2026-08-01）

在 revision `772f922b7` 重建二进制后严格 `parallel=2` 回放：

- `eval/results/real_trace_c2_dstate_iowait-20260731-183451`
- `eval/results/real_trace_h3_iofam_one_seat-20260731-183451`

runner 2/2（C2 210s、H3 217s）。C2 人工通过，H3 因模型跨集合平均值绑定错误
人工失败；两个 answer-shape 不变量均通过。

C2 的 WS2 结果：

1. `finalizer_rejects 8→0`，无 rewrite、breaker 或 degraded export；
2. system lead 发布 complete producer-paired roster：
   3 次、0.635ms、三条精确 start/end/duration/state/iowait/caller；
3. 模型正文同步收敛，不再把无关 capacity flag 写成“实际次数可能高于 3”；
4. `trace_query_final_projection_blocks=0`，无
   `materialized runtime trace causal projection`，证明 focused 无窗仍未获得
   root/wakeup/eliminable 全报告。

H3 的显式窗非回退结果：

1. `trace_query_final_projection_blocks=2`，完整因果投影、root rank、wakeup
   chain、critical blocking、窗内可消除量和自动补采均在；
2. IOFAM 的 `完成端到端·IO延迟（io_latency）`、
   `块设备层·块设备IO(inode)`、`综合评分,非墙钟` 全部在场；
3. 模型正文列出 6 条 0.865/0.884/1.056/1.058/1.248/1.347ms 后，却把
   `storage_latency_by_layer` 的 85-op `avg=0.343ms` 写成“6 次平均”。
   这是 aggregate denominator 错绑 visible subset，系统投影本身无误。

按本战役约束，不围绕“平均”词面增加 hard gate。该项作为
`EVAL-B14-MV2 / P2 / model cross-set caliber binding` 留档；若后续不同 case
重复出现，再设计 typed numerator/denominator relation surface。

本轮另发现一个确定性且可泛化的新 GAP：

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B14-REL1 | P1 | runtime artifact physical identity | 同一 immutable runtime blob path 可在不同 query/system supplement 上携带 `attached_trace`、`attached_trace.txt` 等 artifact_id 别名；pair builder 以 ID 优先分桶，生成 `same path ↔ same path` 的伪跨工件关系 | identity election 改为 canonical typed path 优先、artifact_id 仅作无 path fallback；同 path 多别名合并为一个 endpoint，真正不同 path 仍保留 unproven relation | covered-pending-replay |
| EVAL-B14-MV2 | P2 | aggregate denominator binding | 模型把 85-op aggregate avg 绑定到 6-row visible subset | 暂按波动留档；跨 case 复现后再提供 typed count/sum/avg relation，不扫描答案 prose | filed-model-variance |

REL1 不变量：

1. 只消费 `ObservationSourceRef.Path/ArtifactID/ArtifactKind` 的 typed 字段；
2. path 使用既有 canonical runtime-artifact identity 规范化，不扫描请求/答案；
3. 同 path 视为同一 immutable turn artifact，即使不同 query 使用不同 alias ID；
4. 不同 canonical path 即使本地 time-domain/alignment 标签相同，仍是独立 endpoint，
   relation status 保持 unproven；
5. 无 path 时继续用非 generic artifact_id；generic `trace_query` ID 继续
   fail over 到可用 path，避免跨工件合并；
6. 不改完整因果投影或 focused principal-value gate。

REL1 已落地：`runtimeArtifactRelationIdentity` 现在以既有
`runtimeArtifactIdentityPathKey` 为第一身份，统一 slash、相对路径以及 Windows
盘符/路径大小写；path 缺席时才使用非 generic artifact_id。endpoint 的显示
path 同样使用 canonical 形，避免同一物理路径因词面差异变成 `multiple`。

相反臂测试同时锁定：

- `D:\trace\one.sys` + `d:/trace/one.sys` 且 ID 分别为
  `attached_trace/attached_trace.txt` → 单工件、pair authority inactive；
- 同一个 `trace_query` ID + `a.systrace/b.systrace` → 两工件、一个 unproven
  relation pair；
- answer materializer 的 `one.systrace/./one.systrace` 别名不会生成 self-pair
  system block。

回归：

- 定向 types/tool relation tests 通过；
- `go test ./internal/types ./internal/tool -count=1` 通过
  （types 21.290s、tool 167.207s）。

### B15 r1 REL1 收账与频率权限审计（2026-08-01）

在 revision `dc624779e` 重建后严格 `parallel=2` 回放：

- `eval/results/real_trace_c2_dstate_iowait-20260731-184613`
- `eval/results/real_trace_h4_supply_thermal_witness-20260731-184613`

runner 1/2（C2 PASS 174s、H4 FAIL 117s），人工同为 1/2。

C2 证明 REL1 covered：答案/日志均不再出现 self-pair
`attached_trace.txt ↔ attached_trace.txt`，complete roster 和 focused 无窗
窄报告均无回退。它同时暴露 `EVAL-B15-AR1`：正文正确写
`0.635ms / 144.557ms ≈ 0.44%`，系统 arithmetic advisory 却把 144.557ms
误选为 numerator，发布“144.557ms / 0.440% 重算为 100%”。该错误附注不参与
hard gate，但会误导用户。

H4 的 trace 值与完整显式窗报告均齐：模型正文发布 157.248/5.604/70.338/
0.000ms 四态，CPU4 direct 2.10GHz policy ceiling，`132.041` 不在场；完整
Trace 因果投影、root rank、wakeup chain、窗内可消除量与自动补采继续在。

失败发生在 typed 权限到自然语言的最后一跳。finalizer prompt 已明确：

```text
policy_limit_status=present
binding_caliber=limit_row_proves_ceiling_presence
binding_impact_requires_separate_overlap_or_supply_evidence
```

并说明 actual/average/residency 低于 ceiling 不能反推无 policy limit，也不能
单独证明 workload hit ceiling 或其性能影响。模型仍写“558MHz 远低于 2.10GHz，
因此明确 thermal throttle，且不是 policy 层硬上限约束”。同一正确 authority
当前只写入 `doc.Caveats`，在约千行因果报告后的系统附注才出现，不能优先保护
客户阅读。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B15-FQ1 | P0 | frequency authority publication order | typed policy-limit/binding boundary 已到 prompt，但模型可忽略；正确 authority 仅在 footer Caveats，错误 principal 先到达用户 | 复用同一 typed authority 构造 system-owned leading block；分开 ceiling presence、binding/impact、thermal/policy mechanism，后续 prose 冲突时以 lead 为准 | covered-pending-replay |
| EVAL-B15-AR1 | P1 | arithmetic prose relation operand binding | 同一自由句中 value、window denominator、percentage 三个数值无 typed relation owner，regex/邻近启发式选错 operand | 不增加词面特判；审计现有 arithmetic compiler，优先只发布 typed/结构化 relation 或在多候选时静默 fail-open | filed-next |
| EVAL-B14-REL1 | P1 | runtime artifact physical identity | 同 path 不同 ID 生成 self-pair | canonical path primary identity；C2 真实回放不再发布 pair block | covered |

FQ1 不变量：

1. 复用 `TraceEvidenceAuthority` 的 typed counts、supply-evidence roster 与
   `TraceFrequencyLimitAuthority`，不读请求或模型答案原文；
2. 不重写/删除模型正文，不增加 hard reject 或 retry；
3. direct limit row 只授权指定 CPU/窗的 ceiling presence；
4. actual/residency 低于 ceiling 既不能否定 policy limit，也不能单独证明
   binding、性能影响或 thermal mechanism；
5. 独立 compute-supply evidence 可证明其自身口径的 supply deficit，但 thermal
   与 policy 的成因归属仍需对应 typed causal witness；
6. leading block 使用保留 ID + unforgeable system marker，先于模型正文；
7. 不改变显式窗因果投影、补采、root/wakeup/eliminable 选路。

FQ1 已按上述不变量落地：

- 原有 `runtime_trace_frequency_authority` 不再滞留在 `doc.Caveats` 尾部，
  而是使用和 blocking/state authority 相同的 system-owned lead 插入、保留
  ID、防碰撞 marker 与最终 hierarchy tier；
- authority 同源发布
  `policy_limit_status=present`、
  `binding_caliber=limit_row_proves_ceiling_presence`、
  `binding_impact_requires_separate_overlap_or_supply_evidence` 和
  `thermal_or_policy_mechanism=requires_typed_causal_witness`。低于 ceiling 的
  actual/residency 不能单独区分 workload demand、policy、thermal 或其他治理
  机制，也不能单独证明热节流；
- 不读取或匹配用户请求、模型正文；不重写模型块、不增加 hard gate/retry；
  `normalizeRuntimeTraceReportHierarchy` 只按 authenticated system kind + 精确
  block ID 将其放入既有 typed lead tier，模型块之间的相对顺序不变；
- production persist pin 以一段故意冲突的模型摘要验证：持久化后 typed
  frequency authority 必须位于摘要之前，模型原文保持不变；direct unit pin
  同时覆盖幂等、typed limit witness 与 thermal/policy 权限边界。

验证：

- 定向 authority/materializer/hierarchy/persist 测试通过（1.361s）；
- `go test ./internal/tool -count=1` 全包通过（160.880s）。

状态保持 `covered-pending-replay`：代码确定性出厂与顺序已由生产接线 pin
证明；仍须用 H4 真实回放确认 HTML 可见顺序，并同时固定显式时间窗的完整
Trace 因果投影、root rank、wakeup chain、窗内可消除量与自动补采均未回退。

### B15 r2 FQ1 收账与跨域 negative-scope 审计（2026-08-01）

在 revision `99703dbe9` 重建后严格 `parallel=2` 回放：

- `eval/results/real_trace_h4_supply_thermal_witness-20260731-185941`
- `eval/results/read_combo_config_absent_present_mix-20260731-185941`

runner 1/2（H4 FAIL 194s、config mix PASS 239s），人工 0/2；机器与人工
差异分别来自 legacy principal oracle 和 negative-proof scope 漏检。

H4 证明 `EVAL-B15-FQ1` covered：

1. 最终答案第一个可见块就是
   `runtime_trace_frequency_authority`，先于模型摘要；不是 footer caveat；
2. block 完整发布 CPU0 `1530000/16/line8048`、CPU4
   `2100000/28/line17113`，以及 ceiling presence、binding/impact 与
   thermal/policy mechanism 三层权限；
3. `trace_query_final_projection_blocks=2`；完整 Trace 因果投影、主根因、
   链/唤醒读法、58.320ms 供给折算可消除量和系统自动补充均在；
4. 模型正文仍写“结论：受限”“CPU12 受热节流约束”，并把窗尾 runnable
   误写成已计入 running。整体答案仍有正文冲突，故人工 fail；但 system
   authority 已领先并明确声明冲突时以自身为准。按本战役约束，该残余归
   `model-prose variance`，不增加用户/答案关键词 hard gate，也不删除显式窗
   因果能力。

config mix 的机器 PASS 掩盖了证据作用域 GAP。结果的两个值没有串位：
`explore_per_tool_default_cap=0` 与 `internal/config/runtime.go:1035` 正确；
但 `explore_xyz_phantom_unique_budget` 的三层 absence 只有一个 accepted
negative `EvidenceItem`，其作用域严格是
`internal/config/runtime.go` 单文件。另一次 `cmd` no-match 只存在于 typed
`ToolPathDiscovery`，YAML 则只读了另一个键的正值行。completion 仍用
`member_set` 加以下 support refs 宣布三层均 absent：

```text
internal/config/runtime.go:0
codrax.yaml.example:1171
cmd
```

其中 `:0` 不是正向源码锚点，1171 是另一个键的正值 witness，裸 `cmd` 也
不是可引用位置。最终答案进一步声称“codrax.yaml.example 中所有 flag 注释”
可证明 CLI 不存在，属于跨 target、跨 scope 越权。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B15-FQ1 | P0 | frequency authority publication order | 正确权限滞留 footer | typed system authority lead；真实 H4 已验证顺序和显式窗非回退 | covered |
| EVAL-B15-NEG1 | P1 | negative proof target/scope authority | `PathDiscovery.NoMatches`、negative `EvidenceItem`、aggregate claim 与最终结论没有共享的作用域 authority；局部 no-match 可扩写成全局 absence | 从 typed no-match 与 negative query 构建有界 authority roster；可见系统块只授权逐条列出的 target/scope，并声明未列 scope 未证；不读取模型/用户 prose，不把任意导航 miss 提升成 principal | covered-pending-replay |
| EVAL-B15-XR1 | P2 | mixed exact target binding | 双 config key 被路由成 source inventory，缺 `exact_targets/context_roles`；document-level exact_resolution 不能表达 A absent + B present | 设计 per-target resolution/target_ref + per-target requested roles；先保留当前防串值短期 guard，禁止按 case 关键词特判 | filed-design |
| EVAL-B15-AR1 | P1 | arithmetic operand binding | 多数值自由句的 relation owner 不明确 | 只消费 typed relation 或多候选 fail-open | filed-next |

NEG1 施工不变量：

1. 只消费 `EvidenceItem{scope=negative, negative_query,
   negative_scope}` 与成功、无截断的
   `ToolResult.PathDiscovery{kind=grep,no_matches=true}`；
2. target/pattern、file/path、scope、result_count 逐条保留，禁止把一个 scope
   合并成 repo-wide 或另一配置层；
3. 系统块只说明“这些查询在这些范围内为零命中”；未列 scope 明示 unproven，
   不自动断言目标全局不存在；
4. 不解析用户原文、模型 summary、aggregate label/member 或最终答案正文；
5. 不增加 hard reject/retry；导航阶段的无关 miss 不得污染答案；
6. 不触碰 Trace report-shape、显式窗因果投影或自动补采。

NEG1 已按不变量落地：

1. 新增 `current_source_negative_scope_authority` system-owned lead block；
   它只在 `ScenarioConfigTrace` 且至少存在一条 grounder 已接受的
   `EvidenceAbsent + ScopeNegative + NegativeQuery + NegativeScope` 时发布；
2. verified negative evidence 逐条保留 pattern、file 与
   `file/range/section/struct_fields` 口径；同轮成功、`result_count=0`、无
   truncated/skipped-large 的 explicit-path grep no-match 作为独立行加入；
3. 单独的 navigation miss 没有 verified negative anchor 时不发布；path
   为空的广域 grep、candidate/result truncated、skipped-large 均不能进入
   authority；
4. authority 明示
   `unlisted_scope_status=unproven` 和
   `cross_target_borrowing=forbidden`，不会把一个 `runtime.go(file)` 查询扩成
   YAML/CLI/repo-wide absence；
5. block 使用新的 `json:"-"` system marker；模型若占用保留 ID，系统选择
   无冲突后缀，不按标题或正文恢复 authority。system snapshot 的 sidecar
   capture/reauth pin 已覆盖新 marker；patch 前 summary canonicalization
   即使暂时把摘要移到前面，materializer 也会刷新并恢复 authority lead；
6. 实现不读取 RawRequest、模型 block 文本、aggregate label/member 或 renderer
   输出，也不增加拒绝/重试。模型原文保持字节不变，系统块只覆盖证据权限。

验证：

- 定向 materialize/persist/navigation-negative/incomplete-grep/patch-order
  测试通过（1.052s）；
- system marker capture/reauth 定向测试通过（2.694s）；
- `go test ./internal/types -count=1` 通过（19.779s）；
- `go test ./internal/tool -count=1` 通过（173.073s）。

状态为 `covered-pending-replay`：下一对仍严格 `parallel=2`，用 config mix
验证系统 lead 只列 `runtime.go` 与 `cmd`、明确 YAML 未证；另一席使用显式窗
Trace case，固定本次非 runtime 改动没有影响因果投影和自动补充。

### B15 r3 NEG1 收账与 mixed-target exact-resolution 审计（2026-08-01）

在 revision `3a25fc462` 重建后严格 `parallel=2` 回放：

- `eval/results/read_combo_config_absent_present_mix-20260731-191829`
- `eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260731-191829`

runner 1/2（config mix PASS 150s、H8 FAIL 177s），人工 0/2。两项人工失败
原因不同，不能合并成一个 production 修点。

config mix 证明 `EVAL-B15-NEG1` covered：

1. 系统首块逐条列出 phantom target 的三个已验证范围：
   `cmd/root.go (file)`、`codrax.yaml.example (file)`、
   `internal/config/runtime.go (file)`；
2. 每行 producer 都是 `verified_negative_evidence`，没有把另一 target
   `explore_per_tool_default_cap` 的正向行借给 phantom target；
3. block 明示 `unlisted_scope_status=unproven` 与
   `cross_target_borrowing=forbidden`；
4. 第二个 target 的注释示例值 0、runtime 字段和 CLI 接线仍正确在场。

但同一答案在 NEG1 block 之前仍发布：

```text
当前已验证范围内未找到完全一致的精确目标。
```

本轮 analyzer 已正确发出两个 `exact_targets`；错误不再是路由缺字段，而是
`AnswerDocumentV2.ExactResolution` 只有一个 document-level status/anchor，
无法表达 target A absent + target B present。模型给出全局 `absent` 后，
renderer 将其扩成覆盖两个目标的横幅，与后续 scoped 权威和 present scalar
自相矛盾。

H8 的 production typed 面没有丢：

1. `trace_query_final_projection_blocks=2`，完整 Trace 因果投影、根因榜、
   wakeup/边锚定、窗内可消除量与系统补充都在；
2. 语义席明确发布 `VerifyClass ...` / `类校验` / `0.285ms`；
3. 同席明确发布
   `最晚相关边 34579.496810s·凭证=直接裸边`。

runner 失败只因 case oracle 写死 ASCII 逗号
`34579.496810s,凭证`，而正式系统板使用中点分隔；这是 oracle drift，不应
反向修改生产文案。整体人工仍 fail，因为模型摘要把
`34579.495841–34579.496126` 错说成不在
`34579.490–34579.500` 目标窗内，与正确系统板冲突。按“不扫描模型原文做硬门”
红线，该项先作为模型窗口归属波动留档。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B15-NEG1 | P1 | negative proof target/scope authority | 局部 no-match 被扩写 | typed target/scope system lead | covered |
| EVAL-B15-XR1 | P1 | mixed exact target global verdict | 多目标 contract 只有一个无 target_ref 的 document-level resolution；mixed present/absent 时任何全局 absent 横幅都越权 | 短期 typed fail-closed：当 contract 有多个 distinct target 且 document verdict=absent 时抑制全局 exact-resolution 横幅，同时保留 scalar、scoped NEG1 与模型事实块；长期增加 per-target resolution/target_ref | covered-pending-replay |
| EVAL-B15-H8O1 | P3 | eval oracle punctuation drift | oracle 把非语义分隔符写死 | oracle 匹配 typed 内容并容许正式中点分隔；不改生产代码 | planned-eval-only |
| EVAL-B15-H8MV1 | P2 | model window-membership binding | 模型忽略 typed start/end 与 selected window，叙述成窗外 | typed projection 已正确；先观察跨 case 复现，不扫描请求/答案、不加 case-specific hard gate | filed-model-variance |
| EVAL-B15-AR1 | P1 | arithmetic operand binding | 多数值自由句 relation owner 不明确 | 同句 duration 候选 × typed window 的唯一算术自洽 pair 才允许 advisory 复算；多解/无唯一解 fail-closed | covered-pending-replay |

XR1 短期批不变量：

1. 只读取 `AnswerSemanticView.ExactResolution.Targets` 的 typed target 数量和
   `doc.ExactResolution.Status`；
2. 多目标 + global absent 时只清除无法绑定 target 的 document verdict，
   不删除任何 principal/scalar/evidence block；
3. 单目标 absent 的现有横幅、scalar 抑制和 exact-absence contract 不变；
4. exact/alias match 不在本小批推断或改写；长期 per-target carrier 独立立案；
5. 不读取 RawRequest、模型文本或 renderer 文案，不新增 hard reject/retry；
6. 不触碰 runtime trace report shape、显式窗因果投影或自动补采。

XR1 短期批已按上述不变量落地：

- `normalizeViewCompatibleAnswerDocument` 在所有 full emit、patch/persist 与
  recovery 共享的 typed normalizer 中，先统计
  `view.ExactResolution.Targets` 的非空 distinct target；
- 当且仅当 distinct target 大于 1 且模型给出 targetless
  `exact_resolution.status=absent` 时，将 document verdict 清空。renderer
  因此不再生成覆盖全部目标的全局 absent 横幅；
- blocks、citations、present scalar、NEG1 scoped authority 与模型正文均不
  改写；单目标 absent 的横幅/标量抑制保持不变，同一 target 的重复空白拼写
  也仍按单目标处理；
- production persist→render pin 同时固定：全局横幅消失、NEG1 system lead
  仍在、模型事实与 present-target scalar 均保留。实现没有读取请求或答案
  prose，也没有增加 reject/retry。

### B15 r4 XR1 收账与 negative-producer 分裂审计（2026-08-01）

在 revision `d717f6044` 重建后严格 `parallel=2` 回放：

- `eval/results/read_combo_config_absent_present_mix-20260731-193544`
- `eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260731-193544`

runner 1/2（config mix PASS 170s、H8 FAIL 198s），人工为 config uncertain、
H8 pass。

XR1 已由 config mix 真实收账：

1. 错误的 document-level
   `当前已验证范围内未找到完全一致的精确目标` 横幅不再出现；
2. 两个 target 的模型事实块均保留，present target 的示例值 0 和
   `runtime.go:1035` / `root.go:3799` 锚点没有被 scalar 归一化误删；
3. 单次 finalizer、0 reject/patch，说明修复不是靠重试波动生效。

本轮同时暴露 NEG1 的下一层 producer 分裂。explorer 两次尝试发 negative
EvidenceItem，但 source/negative_query file 分别写成抽象
`internal/tool` vs `production Go files`、以及测试文件 vs repo 根，grounder
正确拒绝，`evidence round ingest ... accepted_evidence=0`。因此 NEG1
system lead 按设计 fail-closed 不发布。

completion 另有一条 accepted typed aggregate：

```text
kind=negative_search
origin=repo_negative_search
target=explore_xyz_phantom_unique_budget
scope=production Go files excluding test/docs
result_count=0
```

现有 negative-proof supplement 在答案尾部忠实转录该范围，但模型正文仍把它
扩成 YAML/example/CLI 三层 absence。事实可能正确，权限链却不完整。不能通过
扫描模型文字拦截；也不能把 model-authored aggregate 无条件提升为系统真值。
后续最优方向是统一经过 completion 校验、且具精确 typed
target/query/scope/result_count 的 negative producer roster，在 lead 明示每条
权限上限；未验证范围继续 unproven。

H8 的显式窗非回退验证通过：

1. `trace_query_final_projection_blocks=2`；
2. 完整 Trace 因果投影、root rank、wakeup/边锚定、窗内可消除量和系统补充
   均在；
3. `VerifyClass` / `类校验` / `0.285ms` / 链上 #2 /
   `34579.496810s` / `直接裸边` 全部在，模型本轮也正确复述。

runner FAIL 是 eval harness 在 `LC_ALL=C grep -E` 下对 UTF-8 bracket
expression `[·,]` 的兼容问题，不是生产答案缺证。case 已固定为正式系统板
实际字面 `·`；不为测试改生产渲染。上轮的模型窗归属错误本轮未复现，继续按
波动留档。

| ID | 优先级 | 类别 | 泛化根因 | 最优方案 | 状态 |
|---|---:|---|---|---|---|
| EVAL-B15-XR1 | P1 | mixed exact target global verdict | targetless global absent 覆盖 mixed targets | multi-target absent fail-closed suppression；真实回放横幅消失且 scalar 保留 | covered |
| EVAL-B15-H8O1 | P3 | eval UTF-8 regex authority | C locale bracket expression 不可靠 | oracle 使用正式 UTF-8 字面，不改生产 | covered-pending-replay |
| EVAL-B15-H8MV1 | P2 | model window-membership binding | 模型一轮错读 typed 窗 | 本轮未复现；不做 prose gate | filed-model-variance |
| EVAL-B15-NEG2 | P2 | negative authority producer split | verified negative evidence 缺席时，accepted aggregate negative-search 只在 footer 出现，scope authority 不领先 | 审计 aggregate completion validation；仅对精确 typed target/query/scope/result_count 的 accepted producer 建统一 lead，保持每行有界、未列 scope unproven | filed-audit-next |
| EVAL-B15-AR1 | P1 | arithmetic operand binding | 多数值自由句 relation owner 不明确 | 只消费 typed relation 或多候选 fail-open | filed-next |

下一批优先级仍为 AR1 高于 NEG2：AR1 已由真实 C2 重现确定的“系统附注算错”，
属于 system-authored misinformation；NEG2 当前是 system fail-closed +
model overclaim，风险较低。施工时继续保留 H8 作为显式窗非回退席。

### AR1 typed arithmetic pair election（2026-08-01）

客户实形红测固定：

```text
总时长 0.635ms，占全窗 144.557ms 的约 0.44%。
```

修前 `runtimeTraceDurationPercentRelationRE` 只保留百分号前最近的 duration，
把 denominator `144.557ms` 当 numerator，系统发布
`144.557 / 0.44% = 100%` 的错误附注。它不是模型波动，而是
system-authored misinformation。

AR1 最优解不是增加“总时长/全窗/占比”等语言关键词，而是把关系解析改为
typed arithmetic pair election：

1. 每个模型 block 按句/行识别 percentage token；
2. 收集其前最多 96 rune、且通过既有 same-metric bridge 精度过滤的全部
   duration token；
3. 对 `同句 duration candidates × producer-typed selected windows` 计算全部
   pair；
4. 只有一个 pair 在该 percentage 显示精度导出的统一容差内自洽时，才允许
   复算或 completeness 附注；
5. 多个自洽 pair、多个 numerator 且零自洽 pair，均发布有界
   “未选出唯一 pair/未复算”说明，不按邻近 token 猜；
6. 单 numerator + 多窗的既有 denominator election、complete 自洽静默、
   mismatch-vs-all 与 cross-metric 拒绝臂保持不变；
7. 句界扫描明确区分 ASCII sentence period 与 digit`.`digit 小数点，防止
   `0.817ms` 被截成 `817ms`。

不变量：

- 只读 model block 的结构化可见 surface 和 typed observation window；
- 不读 RawRequest，不按用户/模型业务关键词硬门，不按 PID/case/value 特判；
- advisory-only：不改写正文、不 reject/retry；
- system-generated blocks 不进入扫描；
- 完整显式窗 Trace 因果投影、root/wakeup/eliminable/auto-supplement 不变。

验证面：

- 客户实形 `0.635/144.557/0.44%` 只选 0.635 为 numerator，禁止出现
  `144.557ms / 0.440%` 与 `100.000%`；
- 同形 completeness=complete 时唯一正确 pair 静默；
- 两个 numerator、零唯一 pair 时不猜任意一个；
- 原有中文/英文、cross-metric、重复 claim、无窗、多窗唯一/零/多解和
  persist 接线测试全通过；
- `go test ./internal/tool -count=1` 全包通过（167.520s）。

### B15 r5 AR1 收账与 B16 选批（2026-08-01）

在 revision `c47f45c15` 重建后严格 `parallel=2` 回放：

- `eval/results/real_trace_c2_dstate_iowait-20260731-195107`
- `eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260731-195107`

runner 2/2、人工 2/2（C2 189s、H8 210s）。

C2 收账：

1. 修前 system-authored
   `144.557ms / 0.440% → 100.000%` 错误附注完全消失；
2. 正确 principal 保持 3 次、0.635ms、约 0.44%，system authority 的
   complete 3-row roster、`d_state_occurrences=0`、
   `io_wait_occurrences=3` 和 caller 全在；
3. 无 finalizer reject/patch，focused 无窗仍为
   `trace_query_final_projection_blocks=0`，没有重新套入 full projection。

H8 收账：

1. 正式中点 oracle 在 `LC_ALL=C grep -E` 下通过；
2. `trace_query_final_projection_blocks=2`；
3. 完整因果投影、根因榜、窗内可消除量、0.285ms 类校验、精确
   34579.496810s 直接裸边与系统补采都在；
4. 模型 principal 本轮也正确把 semantic span 放到链上 #2。

状态：

| ID | 状态 |
|---|---|
| EVAL-B15-AR1 | covered |
| EVAL-B15-H8O1 | covered |
| EVAL-B15-H8MV1 | filed-model-variance（r5 未复现） |
| EVAL-B15-NEG2 | filed-audit-next |

B16 不继续围绕 C2/H8 拟合。按“客户错误严重度 × 证据口径风险 × 域多样性 ×
最近回放时间”排序，下一对冻结为：

1. `trace_query_perf_quality_simpleperf_proto_offcpu`：runtime/perf 证据口径；
   最近结果停在 2026-06-18。它验证 CPU unknown + off_cpu sample 不能被写成
   running CPU 执行，属于把样本性质写反的高影响风险；
2. `qf_called_by_typed_relation_query`：repo typed call relation 与完整生产调用者
   枚举；最近结果停在 2026-05-24。它覆盖非 trace 域，能检验 typed relation
   provider、排除测试调用者和逐成员文件绑定。

两席仍严格 `parallel=2`。不选择相邻的另一个 H/C trace case，避免样本集中；
也暂不优先 NEG2，因为 NEG2 当前是 system fail-closed + model overclaim，而
这两席覆盖的 off-CPU caliber 与 relation rowset 一旦出错会直接反转 principal
事实。

### B16 r1 perf caliber 与 typed called-by 审计（2026-08-01）

在 revision `c4cdb71b8` 重建后严格 `parallel=2` 回放：

- `eval/results/trace_query_perf_quality_simpleperf_proto_offcpu-20260731-195632`
- `eval/results/qf_called_by_typed_relation_query-20260731-195632`

runner 2/2、人工 2/2（perf 111s、relation 70s）。

perf 席：

1. `sample_kind=off_cpu`、`cpu=-1`、`cpu_known=false`、
   `sample_cpu_scope=unknown`、`weight_unit=ns_off_cpu_event`、
   symbolized/simpleperf source 全部在；
2. 正文明确 1/1 样本是 off-CPU，window running=0，sample_weight=7000
   是 event weight 而非 CPU 执行时长；
3. 没有把它归因到 CPU0，也没有写成 7000ms/7s；
4. system typed perf-quality caveat 明确 CPU unknown 只是不允许 concrete
   CPU/core attribution，off_cpu 才是不允许 running 叙述的独立权限。

模型有一句把 `cpu=-1` 单独解释为“线程不在任何物理核心上”。严格说该字段只
证明 CPU 归属未知；本例“不在运行”由 `sample_kind=off_cpu` 与
running=0 独立证明，因此主结论正确。作为
`EVAL-B16-PFMV1 / P3 / model prose caliber` 留档；不扫描答案、不添加
case-specific gate。

relation 席：

1. 完整列出两个生产 caller：
   `BuildTypedRelationQueryWithResolvedSources@219` 与
   `TypedRelationKindsForRequest@246`；
2. 两者逐行绑定 `internal/types/typed_relation_hint.go`，无 test caller、
   无跨文件/相邻行串位；
3. 模型列表与系统 accepted-enumeration supplement 有重复展示，但成员集合、
   call edge 与文件均正确；这只是呈现冗余，不影响 principal。

B16 r2 按同一多维排序选择：

1. `trace_query_path_question_multi_trace_files`：两个显式 trace path、两个不同
   target/window，验证 per-artifact 隔离、跨工件 relation authority 与各自
   结论不串位；
2. `qf_type_relation_loop_controller`：非 runtime 域，验证接口→主要实现类型
   的完整 typed relation inventory、文件绑定和 Mermaid 关系面。

这对同时覆盖 runtime multi-artifact 与 repo polytype relation；不会因上一对
通过就只跑同类 perf/caller 近邻。

### B16 r2 multi-artifact projection 与 type relation source-role 审计（2026-08-01）

在 revision `4d5e535ae` 重建后严格 `parallel=2` 回放：

- `eval/results/trace_query_path_question_multi_trace_files-20260731-200022`
- `eval/results/qf_type_relation_loop_controller-20260731-200022`

runner 2/2 PASS，但人工 0/2；两席分别揭示 system projection one-seat 与
typed relation source-role 的确定性缺口。

multi-trace 席中，系统正确完成：

1. 两个 path、target、window、state roster 完全隔离；
2. shared clock/direct alignment/device/session relation 全部诚实标
   `未证明`，没有用局部同形时间戳制造跨工件因果；
3. 两个工件的显式窗 Trace 因果投影都保留，最终
   `trace_query_final_projection_blocks=3`。

但 trace1 的同一 app-20 runnable 物理 aggregate 同时占了 root-rank 与
wakeup-causal-impact 两席，因果树和指标表都出现两次 5.000ms。这不是模型
波动，而是 system-generated projection 的重复计量，构成既有
`EVAL-B7-T2 / P1` 的 production witness。修复必须落在唯一 projection
absorption 点，读取 typed membership/provenance；只凭 subject/state/value
相等会误合并两个真实的等长区间，禁止采用。

模型另把 1.001200→1.010000s 的 endpoint 差写成 10.000ms；typed 板仍正确
区分 target sleep=10ms、worker effective=8.3ms 与 cumulative=9.0ms。
登记 `EVAL-B16-TRMV1 / P3 / model variance`，不扫描答案 prose、不为单值
加 gate。

LoopController 席中，relation edge 与文件绑定正确，但 12 个生产实现和
3 个 `agent_test.go` 测试桩同时进入“主要实现”主表/主图。模型能口头标出
测试项，不等于 system 有权限稳定分栏。新增
`EVAL-B16-REL1 / P1 / typed source-role authority`：

1. relation producer/normalizer 提供
   `source_role={production,test,fixture,generated,unknown}`；
2. 完整成员 rowset 不删除，principal 与 auxiliary projection 按 typed role
   分席；
3. caller/implementation/subtype 等 relation kind 共用同一协议；
4. role unknown 时 fail-open 且显式披露；
5. 不扫描 RawRequest 中的“主要”、不在答案阶段按 `_test.go` 做硬门。

该 case 另有 5 次 accepted investigation_complete、10 次 midloop、299s，
登记 `EVAL-B16-REL2 / P2` 探索成本；先审计 typed dispatch/complete reason，
不以 case/path 特判压缩。

当前施工/验证顺序冻结为：

1. B17：修复 `EVAL-B7-T2` one-seat，并严格并行回放
   `multi_trace_files + H8 explicit-window`，前者收账、后者守住完整
   root/wakeup/eliminable/auto-supplement；
2. B18：完成 `EVAL-B16-REL1` producer 设计与跨 relation-kind fixtures，
   再施工；不得与 B7-T2 混批；
3. B19：回到优先级矩阵选两个新的高风险、异维度 eval，不连续拟合当前
   witness。模型单轮 endpoint 波动继续留档。

| ID | 优先级 | 状态 |
|---|---:|---|
| EVAL-B7-T2 | P1 | production-witness-confirmed / implementation-next |
| EVAL-B16-REL1 | P1 | filed-design |
| EVAL-B16-TRMV1 | P3 | filed-model-variance |
| EVAL-B16-REL2 | P2 | filed-audit |

### B17 exact state-account one-seat 施工（2026-08-01）

针对 `multi_trace_files` 的生产实形，代码冷读确认 root-rank runnable 席与
`wakeup_causal_impact` 席虽然都来自同一 app-20 物理状态账，但两条发布线的
证据包络分别是行 4..23 与行 3..23；既有 one-seat 只认完全相同的行范围，
因此两条 5.000ms 都保留。仅用“同 subject/state/value + 包络包含”折叠不可
接受：两个真实、互不相交的等长 runnable 段也可能满足这些条件。

B17 落地为 producer-minted exact identity：

1. tracequery 只在持有完整状态段库存时，对
   `(pid, state, selected window, ordered exact interval inventory)` 计算
   `state_account:v1:<sha256-prefix>`；
2. 每个区间必须有限、正长、互不重叠，库存总长必须与发布值在既有 µs 口径
   内完全对齐；
3. 同一结果中必须恰好一个 active rank 席与一个 causal-impact 席产生同键，
   才把键同时发布到两边；任一侧缺失或一对多歧义均不发键；
4. typed observation 用注册键 `state_account_key` 携带，projection 只按
   verbatim key equality 合并；rank 席保留，impact 的 E# 并入审计 roster，
   数值不相加；
5. 合并发生在 per-artifact projection 分区内部、R2 聚合之前，因此不同
   trace 工件不会串账，同账也不会先被 ×N 求和；
6. 当前只开放 runnable：rank carrier 已保留其完整
   `runnableIntervals`。running、S、D、IO 在缺少同等级完整 rank 库存前
   一律 fail-open，禁止用标量、hull 或模型文字补身份。

新增的结构性防回归：

- 真实 `fragmentedChurnTrace` 固定 rank/impact 两条生产路径生成同一个 key；
- 同值但不同精确区间库存生成不同 key，重叠库存拒绝；
- exact 1:1 成功，一对多不发 credential；
- 不同行包络 + 同 key 折为一个 rank 席；不同 key、歧义 key 都保留多席；
- rank 与 impact 两条 observation wire 都必须发布注册键；
- thread-state universe 清册明确 runnable-only 准入与其他状态 fail-open；
- info-contract 清册新增通用 `projection_gate` disposition，机械扫描
  `internal/types` 非测试源码，诚实区分“投影引擎判官”和“显示层判官”，不为
  新字段开免检。
- `go test ./internal/tracequery ./internal/types ./internal/tool -count=1`
  三包全量通过（tracequery 69.153s、types 18.412s、tool 165.201s）。

不变量：

- 不读 RawRequest，不扫描模型答案业务关键词，不按 case/PID/value 特判；
- 不改变 root-cause rank、wakeup chain、selected-window、窗内可消除量、
  auto-supplement 的生成与准入；
- 显式窗 H8 必须继续发布完整 Trace 因果投影；
- 无 credential 的 legacy/unsupported/ambiguous 形保持原多席，宁重勿假并。

状态：

| ID | 状态 |
|---|---|
| EVAL-B7-T2 | implementation-complete / full-tests-pass / replay-next |
| EVAL-B16-REL1 | filed-design / B18 |

### B17 r1 one-seat 收账与显式窗非回退（2026-08-01）

在 revision `1aefc8b5e` 重建后严格 `parallel=2` 回放：

- `eval/results/trace_query_path_question_multi_trace_files-20260731-203253`
- `eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260731-203253`

runner 2/2、人工 2/2（multi-trace 104s、H8 115s），均为单轮
analyze/explore/finalize，0 midloop、0 finalizer reject、0 patch。

multi-trace 收账：

1. 两个 trace 的 path、target、selected window、状态账与跨工件 authority
   继续完全隔离，三个 projection block 均在；
2. app-20 的 root-rank 与 wakeup-impact 5.000ms 已收敛成唯一
   `E1(+1)` 席；树、指标表与 ◎ 窗内可消除量都只计一次，不再出现两个
   5.000ms 席；
3. `(+1)` 与 `2次同值` 是既有 duplicate-publication 审计词面，图例明确
   “同一测量被重复发布、数值不是 N 份”；两个原始 observation 仍保留在
   系统核对区，信息不丢；
4. 模型再次把 1.001200→1.010000 的端点跨度叙述成 10.000ms，typed 投影
   没有跟错。`EVAL-B16-TRMV1` 从单轮波动更新为 repeated model-prose
   issue，但仍不以答案关键词或具体数值硬门；其严重度低于 B18 typed
   source-role authority。

H8 非回退：

1. 完整显式 10ms Trace 因果投影仍在，包含根因排序、wakeup chain、
   ◎ 窗内可消除量、coverage boundary 与系统自动补充；
2. `VerifyClass com.baidu.zeus.mml.lac.LacUtils` 仍为链上 #2，
   effective=0.285ms；
3. 宿主→目标最晚相关边仍为精确 `34579.496810s`，凭证仍为
   `直接裸边`；
4. target 四态账、missing-wakeup 下界与 capacity-truncated 权限均未被
   one-seat 收窄影响。

状态：

| ID | 状态 |
|---|---|
| EVAL-B7-T2 | covered |
| EVAL-B16-TRMV1 | filed-repeated-model-prose（不做 raw-answer hard gate） |
| EVAL-B16-REL1 | next / B18 |

### B18 typed relation source-role authority 施工（2026-08-01）

对 `qf_type_relation_loop_controller` 的完整链路冷读确认，问题不是 graph
误判实现关系：15 条 implements edge 全都真实存在。缺口是同一份完整结构
roster 在进入 relation 协议时丢失了已有的 repo path role：

1. analyzer 没有发 `source_scope_profile`，按既有合同应使用默认
   production scope；
2. source-inventory lane 已把 12 个生产类型标为 principal candidate、3 个
   `agent_test.go` 测试桩标为 support；
3. `TypedRelationMember` 原来只有 name/file/line/kind/distance，relation
   dossier、completion member_set 和 finalizer 都拿不到 source role；
4. explorer 因而把完整 15 项 roster 重新铸成 principal `member_set`，
   principal contract 又忠实地把 15 项写进主表和主图。

B18 采用共享、typed、relation-kind 无关的协议：

1. `TypedRelationMember` 新增 `source_role` 与 request-scoped
   `scope_lane`。`source_role` 复用既有闭集
   `production/test/fixture/example/documentation/prompt_support/thirdparty/
   vendor/generated/unknown`，只由精确路径结构计算；
2. graph implements/extends/called-by/references/imports/exports、evidence
   registers/configures/routes-to、observation source-anchor 与 multigraph
   最终 prefix 边界共用同一 role normalizer；provider 不铸 request lane；
3. context 在唯一投影点把 role 与 analyzer-emitted `SourceScopeProfile`
   合成 `principal/auxiliary/unknown`。未声明范围时默认 production；
   显式 test/docs/aux/all 才改变 principal；完整成员 roster 永不删除；
4. repo_map implementers、Knowledge/Evidence row、relation dossier 与
   finalizer handoff 都显示 role/lane 和 principal/auxiliary/unknown 总数。
   explorer 被明确要求 principal `member_set` 只取 principal lane，
   auxiliary 进入 labelled support/excluded，unknown 必须披露核实；
5. completion 增加精确信号校验：只在结构化 member_set 的某成员能与 exact
   typed candidates 对齐、且该名字的所有候选都在 typed request scope
   之外时，要求模型把它移出 principal。相同名字同时存在 production/test、
   pathless/unknown、非 exact carrier 均 fail-open；
6. 硬校验只读 typed query、candidate、SourceScopeProfile 与结构化
   `aggregate_facts.members`，不扫描 RawRequest、closure reason 或最终答案
   prose。测试故意把 RawRequest 写成与 typed scope 矛盾的句子，结果仍由
   typed scope 决定。

通用性与防过拟合：

- 不识别“主要”、`main`、具体接口名、`agent_test.go` 字面或 case ID；
- implements、subtype、caller/reference、import/export 和 evidence relation
  共享 wire，不为 LoopController 开旁路；
- 普通 production 问题保留完整 auxiliary audit roster，不把测试实现假装成
  “不存在”；显式 test scope 会反向把 test 设为 principal、production
  设为 auxiliary；
- 不修改 Trace family resolver、显式窗、root rank、wakeup chain、
  eliminable projection、auto-supplement 或任何 runtime-artifact contract。

当前测试面：

- types：默认 production、显式 test、unknown fail-open、provider 不得铸 lane；
- context：完整三类 roster 不丢，typed scope 可逆投影，evidence/dossier
  必须显示 role/lane 与分席总数；
- provider：imports/exports/called-by/references/extends 全 kind role 清册，
  implements 由真实 probe fixture 固定；
- repo_map：完整 implementers 输出同时标 production/test；
- completion：exact auxiliary principal promotion 拒绝、同名跨 role 歧义
  fail-open、显式 test scope 放行；
- finalizer：完整 roster 的 principal/auxiliary/unknown handoff 与 typed test
  scope 反向投影。

全量回归同时发现一个与 B18 业务逻辑无关、但属于最近提交审计范围的基线
清册 drift：`answer-document-skill` 实际已有 40 条 Tier-B workflow，结构测试
仍把旧 32 条写死。逐项冷读确认新增的 8 条恰好是
data-gap、mixed-supply、ordered-rank、value-owner temporal、target-blocking
wall-clock、IPC request census、target-thread-vs-CPU 与 wakeup census typed
discipline；测试改为逐项固定 40 条前缀，没有删除规则或放宽清册。

验证：

- targeted source-role/projection/provider/completion/finalizer fixtures 六包通过；
- `go test ./internal/types ./internal/context
  ./internal/tool/repomap/relation ./internal/tool/repomap/render
  ./internal/tool/repomap/multigraph ./internal/agent ./internal/tool -count=1`
  通过；其中 types 18.814s、context 复跑 1.423s、relation 1.416s、
  render 1.870s、multigraph 2.577s、agent 4.768s、tool 168.145s；
- `go test ./internal/skill -count=1` 通过（0.592s）。

状态：

| ID | 状态 |
|---|---|
| EVAL-B16-REL1 | partial：source-role carrier covered；B18 r1 发现 completion authority convergence gap |
| EVAL-B16-REL2 | filed-audit（等待 REL1 回放后继续审计成本） |

#### B18 r1：runner PASS / human FAIL（2026-08-01）

严格并行 2 个回放：

- `qf_type_relation_loop_controller`：runner PASS，273s，人工 FAIL；
- `real_trace_h8_semantic_edge_anchor_sentinel`：runner PASS，152s，人工 PASS。

H8 非回退证据完整：显式 `34579.490–34579.500s` 窗、根因排序、两跳
wakeup chain、Trace 因果投影、窗内可消除量和成文前
`critical_blocking_calls` 自动补采均在，B18 没有影响 runtime family。

LoopController 的 B18 carrier 也确实生效：explorer/finalizer prompt 都显示
`complete=15, principal=12, auxiliary=3, unknown=0`。但答案仍错误，深层原因是
完成权没有随 typed relation carrier 收敛：

1. analyzer 发出 `intent=explain + architecture diagram +
   category_enumeration + source_inventory(type)`，却把
   `is_relational_lookup` 留为 false；
2. source-inventory 机械行集先铸造 16 个 principal type（接口本身、12 个
   production 实现、3 个 test 实现）；
3. B18 的 relation scope gate 原来只在 `RequiresRelationMemberSetHandoff`
   为真时运行，因此这次没有消费已经在场的 exact relation roster；
4. source-inventory completion、generic exhaustive completion 和 finalizer
   又把 16 行视为权威，relation 的 12/3 分席只剩 advisory；
5. 结果是 17 次 explore、10 次 midloop、8 次 completion 尝试，最终仍把
   3 个测试实现放入主表和主图。现有 oracle 只检查生产实现出现，没有禁止
   auxiliary 行，所以 runner 误报 PASS。

立案 `EVAL-B18-CONV1`（P1）：typed relation / source_inventory 完成权限
收敛缺失。它是“producer 正确、consumer 权限错误”的通用通道问题，不是
LoopController、Go `_test.go` 或“主要”一词的特例。

#### B18b typed relation completion authority convergence

方案只消费 precise structured signals：

1. request 必须已经是 typed relation member shape，或同时具备
   `category_enumeration + non-none diagram + source_inventory(type)`；
2. coverage provider 必须返回 exact typed relation candidates；
3. model-authored principal `member_set` 的每个成员必须能精确匹配 relation
   member 或其 source anchor；出现任意无关 generic inventory row即不接管；
4. exact auxiliary/test row 进入 principal 时，在
   `emit_investigation_complete` 入口硬拒绝。该拒绝不进入 low-delta
   convergence，不能在重试后被绕过；same-name 跨 role、unknown path 和
   非 exact carrier 继续 fail-open；
5. 校验通过后由系统给 principal member_set 铸造保留 provenance。入口先
   剥离任何 model-supplied 同名 token，防止模型伪造权限；
6. source-inventory row-set landing、resolved/lens completion 和 generic
   exhaustive gate 遇到该权威时让位；完整 source-inventory roster 保留为
   support/audit，不被删除；
7. AnswerSurfacePlan 和 finalizer 识别同一 provenance，保留 relation 主集合，
   不再用更宽的机械 type/function row set 覆盖；纯名称成员无需靠箭头或
   自然语言 label 猜 relation；
8. 普通“所有类型/函数”只要包含一个不能匹配 exact relation member/source
   的成员，就继续由 source_inventory 拥有完成权。

红线核对：

- 不读取 RawRequest、model thinking、closure reason 或 final answer prose；
- 不识别“主要”、`LoopController`、`_test.go`、case ID 或任何语言关键词；
- Trace resolver、显式窗、root rank、wakeup chain、eliminable projection、
  system supplement 未修改，H8 保持专门非回退席；
- source-inventory 不是被关闭，而是在 exact relation 主集合已成立时降为
  audit/support；没有 relation 证明时行为不变。

新增验证：

- analyzer-drift 结构（relation=false）仍能由 exact roster + exact member
  set 收敛；
- 去掉 diagram 或加入任一 unrelated type 后不激活；
- model 伪造 system provenance 会被剥离；
- production 主集合保留，test auxiliary promotion 为不可收敛硬拒绝；
- source-inventory 机械 4 行不能覆盖 relation 3 行；
- snapshot/finalizer 把机械行降为 audit/support，同时完整保留候选 roster；
- LOC ratchet 与 tool user-facing glossary lint 继续守住。

回归：

- `go test ./internal/types ./internal/context ./internal/agent -count=1`
  通过（19.074s / 2.533s / 3.615s）；
- `go test ./internal/tool -count=1` 通过（161.123s）；
- 新拆分的 `source_inventory_authority_snapshot_support.go` 按实际 63 LOC
  登记独立 ratchet；旧 snapshot/projection 文件分别降到 226/342 LOC，
  未提高任何既有 ceiling。

状态：

| ID | 状态 |
|---|---|
| EVAL-B18-CONV1 | implemented / full-tests-pass / replay-next |
| EVAL-B16-REL1 | B18b replay pending |
| H8 explicit-window non-regression | r1 human-pass；B18b replay must retain |

#### B18b r2：typed relation 权限收敛通过（2026-08-01）

严格并行 2 个回放均为 runner PASS / human PASS：

- `qf_type_relation_loop_controller`：199s，4 次 explorer invocation、3 次
  midloop、3 次 repo_map、0 次 source_lens；
- `real_trace_h8_semantic_edge_anchor_sentinel`：135s，1 次 explorer
  invocation、0 次 midloop、3 次 trace_query。

LoopController 的主列表与 Mermaid 均只保留 12 个 production 实现；3 个
test 实现仅以 excluded 身份披露。typed relation 完整清册仍为
`complete=15, principal=12, auxiliary=3, unknown=0`，source inventory
明确降为 `authority=false ... support_only`，说明结果是权限收敛而非删除
候选。相较 r1，重复探索从 10 次 midloop 降至 3 次；单次运行只作机制
证据，不据此承诺稳定性能比例。

H8 继续保留：

- 显式 `34579.490–34579.500s` 用户窗；
- 根因排序、直接唤醒点和两跳 wakeup chain；
- `trace-causal-projection` 与窗内可消除量总览；
- `enumeration_status=incomplete` 覆盖边界；
- 成文前 `critical_blocking_calls` 确定性自动补采。

因此 B18b 没有回退有具体时间窗的 Trace 因果投影、根因排序、唤醒链、
可消除量或自动补齐能力。

状态：

| ID | 状态 |
|---|---|
| EVAL-B18-CONV1 | covered：r2 runner/human 双 PASS |
| EVAL-B16-REL1 | covered：production principal 与 auxiliary audit 分席贯通 |
| H8 explicit-window non-regression | covered：B18b r2 human PASS |

#### EVAL-B18-DUP1：结构化成员载体重复发布（P2）

B18b r2 还暴露一个不影响成员正确性、但影响答案密度的通用 gap：

1. 模型已经用一个结构化 `ordered_list` 完整展示 12 个 principal members；
2. AnswerDocument 预发布归一化仍追加
   “来自已验收的结构化调查清单”的第二个 12 项 ordered list；
3. 同样症状已在 B16 `qf_called_by_typed_relation_query` 出现，因此不是
   LoopController、implements 或单次模型波动特例；
4. 归一化日志明确报告
   `materialized 12 principal aggregate member row(s)`，最终 block 数从模型
   提交的 3 个变为 5 个。

最优方案冻结为结构化 carrier 覆盖判定：

- 只读取 AnswerDocument block kind、item label/row cells 与 accepted typed
  aggregate member set；
- 若一个可见列表或表格的结构化成员键完整覆盖 principal member set，就不
  再追加系统清单；
- diagram 节点不单独视为可审计枚举载体；缺一项仍必须补齐；
- 不扫描 RawRequest、model thinking、summary/prose text 或用户关键词；
- 不修改 Trace family resolver、projection、root rank、wakeup、
  eliminable、supplement 等 runtime 路径。

状态：`filed / B18c next`。

#### B18c 结构化主键载体去重施工

代码冷读确认 DUP1 的直接机制：`normalizeAggregateMemberSetCarriers`
已经判定模型列表覆盖完整，但 relation label 不在 block title/text 时，又调用
`appendAggregateMemberSetCarrier`；该函数不是只补标签，而是重建完整成员行，
于是正确的模型列表后出现第二份系统列表。

B18c 使用一条通用 typed 规则修复：

1. 在同一个可见结构化 block 内，对 accepted member-set 与
   `items[].label` 做逐项、一对一匹配；
2. 若全部 member 都占有独立主键行，只给原 block 补 typed relation title、
   principal surface role 和 enumeration facet/claim，不新增 block、不重建行；
3. 匹配只看结构化 item label。成员仅出现在 item text、table detail column、
   prose、summary 或 diagram node 时不获得主键权限，原补充行为保留；
4. diagram 继续不是独立的枚举清单；缺一项、成员跨多个零散 block、或只有
   详情列巧合命中时，系统仍能补齐完整 member-set；
5. 不读取 RawRequest、模型思考/答案 prose 或 case/type 字面，不改变任何
   Trace runtime contract。

测试覆盖：

- 完整 relation ordered-list + diagram：保留一个列表，typed label 原位写入，
  不再出现 accepted-checklist supplement；
- `public classes` 表中 package 只作为详情列：继续单独发布
  `package declarations`，防止跨轴误去重；
- 完整 `internal/tool` 回归通过（159.461s）。

状态：`implemented / full-tests-pass / cross-relation replay next`。

## B52 r9：ReqCallChain 实体轴归一与复合条件调用表达（2026-08-02）

在 `820525016` 精确构建上继续严格并行 2 个：Java call-chain 147s、called-by 81s，
runner 均 PASS，但人工审计均 FAIL。

called-by 的 typed projection 与 closure 已证明只有两个 unique caller functions，aggregate fact
却把同一函数的三个 callsite observations 当作三个 members。根因不是某个 Go 函数或行号，
而是两个 typed 决策漂移：`TypedRelationKindsForRequest` 已让 closed
`AnalyzerHints.Kind=ReqCallChain` 选择 exact relation provider；
`HasTypedRelationMemberSetShape` 却仍要求 `PredicateAxis=call` 或
`IsRelationalLookup=true`。当 analyzer 只铸出前者时，同一请求可取精确候选但不能按候选身份归一。

登记 `B52i-REQCALLCHAIN-RELATION-IDENTITY`（P1），通用修复为：

1. member-set shape 直接消费 closed `ReqCallChain`，与 relation provider 选择对齐；
2. 仍要求 exact provider、principal fact 以及每个 member 到 candidate/source identity 的唯一匹配，
   不是对所有 call-chain 文本做硬去重；
3. 同一 candidate 的重复 observation 折成一个成员，首个 exact support ref 保留，全部原始
   callsite evidence 仍留在 evidence lane；同名不同文件或身份歧义继续 fail-open；
4. `member_notes` 只有与原 members 等长时才有位置身份；短数组在归一后清空，避免说明错贴，
   不影响 typed members/support/evidence；
5. 判定不读取 RawRequest、fact label/reason、thinking 或 final prose；Go、Java、Kotlin、
   JavaScript/TypeScript/ArkTS、C/C++、Rust、Python、Ruby、Swift、Lua、Cangjie 等所有
   executable relation lanes 共用。

Java 图层还暴露一种跨语言表达缺口：`if (countOpenVisits(...) >= max)` 同时包含真实 invocation
与 comparison。soft finalizer guide 现要求保留 caller→`countOpenVisits` 的 grounded call edge，
再单独以 Note/branch 表示比较；抽象 guard 不能取代 callee，也不能成为 post-guard operation 的
caller。该规则只提供证据与表达指导，不新增 label-text hard gate，不由系统重画模型图或接管结论。

`B52h` 在本轮因模型使用 bare method labels 而未触发，跨语言 preserve/repair/source-path
deterministic pins 仍是其验收 authority。

状态：`B52h=deterministic-pin-covered / replay-no-trigger`；
`B52i=implemented / directed-pass / full-tests-pass / replay-pending`；
`compound-guard-soft-guide=implemented / directed-pass / full-tests-pass / replay-pending`。

完整回归：`internal/types` 19.776s、`internal/tool` 159.961s、`internal/agent` 3.266s，
均以 `-count=1` 通过。

---

## B52 r10：跨语言 colon-qualified 图边与 typed relation authority（2026-08-02）

`e23168999` 精确构建严格并行 2 个：called-by 107s、Java call-chain 220s，runner 2/2 PASS；
人工 called-by PASS、Java FAIL。

called-by 已只发布两个 unique caller functions；重复 callsite 是同一函数行的详情，`B52i` 的
typed ReqCallChain identity projection 在 production replay 生效。

Java 图已保留 compound condition 中真实的 `countOpenVisits` invocation，但同一可视边只有 typed
guard anchor，call-DAG 的 exact call 权限可被 non-call enum 绕过。跨语言定向测试又证明旧
`ParseEdges` 会在整行首个 `:` 截断，导致 Rust/C++/Ruby/Cangjie `::` 节点标签让 edge 消失。

`B52j` 根修位于共享 Mermaid parser + QFCallChain typed evidence gate：

- sequence message 只在 sequence family、arrow target 后解析；flowchart colon labels 不再参与切割；
- same-endpoint exact typed call proof 要求 `relation_kind=call`，即使另有 guard anchor；
- 真 guard/outcome 与其它 typed non-call relations 保留；复合边可同时携带 call+guard；
- 规则覆盖全部 executable language identities，不读取用户/模型原文，不作用于 Trace causal family。

Java 回答还称 `AuditLog.record` 写入“只读日志”，而 fixture body 只证明 `System.out.println`；这记为
explorer/finalizer 模型证据措辞波动。没有精准 typed sink-semantics 信号，系统不得据此硬改模型结论。

状态：`B52i=production-replay-covered`；
`B52j=implemented / directed-cross-language-pass / full-tests-pass / replay-pending`。

完整回归：`internal/mermaidcompat` 0.759s、`internal/render` 2.011s、
`internal/tool` 165.286s、`internal/agent` 3.010s，均以 `-count=1` 通过。

---

## B52 r11：principal_path_edge 图集合闭包（2026-08-02）

`c32510694` 同 pair 并行 2/2 runner PASS：called-by 103s、Java 139s。人工 called-by PASS；
Java FAIL，但 `B52j` 已成功拒绝 guard-only 假装 invocation 的形态。

新 gap `B52k`：finalizer 的 principal path list 与 typed handoff 均含
`VisitService.schedule -> VisitRepository.insert`，最终图却只画 `Check -> insert`。系统已有可视边
soundness，却没有模型已选 principal typed-edge 的 completeness。

修复不把 evidence pool 全部边强塞给模型：只取模型 structured principal_path_edge items 作为 endpoint
universe，再要求 universe 内由 citable typed call evidence 证明的边出现在 strict call diagram；一端不在
principal universe 的 supporting calls 不扩图。hard gate 不读任何 prose，不改变模型成员或结论，
Trace causal family 不参与。

14 个 executable language identity 形态与非 principal 反臂已定向通过。

状态：`B52j=production-trigger-covered`；
`B52k=implemented / directed-cross-language-pass / full-tests-pass / replay-pending`。

完整回归：加入发布接线 pin 后 `internal/tool` 最终复跑 156.126s、`internal/agent` 3.004s，
均以 `-count=1` 通过。

---

## B52 r12：全语言 principal call 引用载体闭包（2026-08-02）

基线 `f50f21f85`，严格并行 2：

- `qf_called_by_typed_relation_query`：runner/human PASS，114s；2 个 unique production callers，
  多 callsite 只保留为同行详情，无重复系统清单；
- `sr_java_call_chain`：runner/human PASS with advisory，138s；最终 sequence diagram 包含全部五条
  grounded calls，reply/guard/branch 语义正确，`B52j/B52k` production replay 闭环。

新 gap `B52l`：principal hop item 可以用 `caller → callee` 作展示 label。此前 `B52k` 只承认
exact code-identity item labels，且按红线不能解析模型 label/text 来制造 hard authority，因此这类模型
已选择调用可能绕过 diagram completeness。

本批把闭包权限移到同一 item 的 typed `citation_ref`：只在 principal + `principal_path_edge` +
`call_edge` carrier 中，以 citation file/exact line 唯一反查 citable typed call EvidenceItem；随后检查该
调用是否由 strict diagram 的 typed call edge 表达。歧义 citation、non-call citation、supporting carrier
全部 fail-open。系统不补画图、不改模型 prose/结论，且不读取 RawRequest、thinking 或答案原文字词。

语言矩阵不再是手写声称：测试直接对照 `SupportedReadLanguages()`。Go、Python、JS、TS、Java、Kotlin、
Rust、C、C++、Ruby、Swift、Lua、ArkTS、Cangjie 14 种 executable languages 必须有同一 fixture；
Proto 明确走声明式反臂，不伪造 source call。未来语言注册表扩展会强制同步图语义测试。

独立记录 `B52m`：Java 第 3 hop 的 display label 是 `schedule → resolveMaxVisits`，citation 却指向
`countOpenVisits:18`。pre-emit 已从 typed evidence 正确发现并提示 line 17，但当前是 advisory。
它不影响 r12 的调用链/图结论，后续应评估仅机械修复 citation metadata；不得硬解析模型文本或接管答案。

验证：

- 聚焦 diagram evidence + production wiring tests 通过；
- `go test ./internal/tool ./internal/agent -count=1` 通过（161.590s / 2.725s）。

状态：`B52l=implemented / full-tests-pass / replay next`；
`B52m=filed-advisory / pending`。

---

## B52 r13：citation-selected missing calls 真实拒绝并恢复（2026-08-02）

`73f15610c` 精确构建严格并行 2：called-by 91s、Java 103s，runner/human 2/2 PASS。

Java 首稿漏画三条 principal typed calls。它们的 list label/carrier 不需要被解析；同一 items 的
`citation_ref` 已唯一指向 typed callsite，新 B52l 合同发出三条 `principal_call_edge_missing`，模型
一次 patch 后补齐全部五条 invocation。容量 guard 留在控制语义，不再冒充 caller。该轮 finalizer
rejects=2（r12=4），说明真实触发同时没有增加修补震荡。

called-by 再次只有两个 distinct production callers；多 callsite 没有重新膨胀实体轴。

状态：`B52l=production-trigger-covered / closed`。语言矩阵与注册表联动：14 executable lanes 正向、
Proto declaration 反向，ArkTS/Cangjie 均在同一规则中，无语言关键词分支。

另有两个非图层 advisory：

1. `B52m`：一个 item 同时叙述 callsite 与 callee 内部行为时，单 citation 无法同时证明两层；需 typed
   per-claim/multi-citation carrier 或拆项指导，禁止解析 prose 硬改；
2. `B52n`：aggregate 是 6 nodes，模型称“6跳”。缺 typed noun-axis 时不按自然语言关键词硬门，先记
   模型措辞波动。

补充全语言基础设施回归：`go test ./internal/tool/repomap/... ./internal/mermaidcompat ./internal/render -count=1`
全部通过，覆盖权威语言注册、parser/extractor、relation/retrieve、Mermaid compatibility 与最终 renderer。

### B52 r7：全语言调用图交接验收与模型表重复（2026-08-02）

同 pair 在 `759f1c859` 上 runner 2/2 PASS。人工审计结论：

1. Java 的四条 direct invocation 均已作为 grounded typed call edge 到达 finalizer，证明上一批
   evidence handoff 通用修复有效；最终图仍把两个 message operation/literal 写错，且首稿把 guard
   画成 self-call，这是模型消费/表达波动，不授权系统替换答案；
2. called-by 的模型主表正确、完整、仅 2 行，系统随后重复追加 2 行 ordered list。登记
   `B52g-PRINCIPAL-MARKDOWN-CARRIER-DUPLICATION`，它是确定性 presentation compiler gap；
3. 施工采用 typed exact-row carrier 复用：完整 principal enumeration Markdown table 仅在首列身份轴
   逐行覆盖时原位承载 relation label，缺行表或成员只在详情列出现时仍走独立补齐；不扫描请求/
   答案 prose，不新增硬门；
4. 同批增加语言中立的 `QFCallChain` soft diagram semantics：callee operation、guard/control edge、
   dynamic owner 与 declarative relation 分账。矩阵覆盖 Go、Java、Kotlin、JS/TS/ArkTS、C/C++、
   Rust、Python、Ruby、Swift、Lua、Cangjie 及其它 executable languages；Proto/RPC/import/
   inheritance/annotation 不凭图形需要升级为 call；
5. Trace root-cause、具体时间窗因果投影和系统补齐均不在该 guide 的 family gate 内。

工件：

- `eval/parallel_selected_summary_evalcampaign_b52_mergeaudit_callchain_r7_20260802.md`；
- `eval/parallel_selected_summary_evalcampaign_b52_mergeaudit_callchain_r7_20260802_manual_audit.md`。

完整回归：`internal/tool` 161.350s、`internal/agent` 2.999s，均通过。

状态：`B52f=covered`；`B52g=implemented / directed-pass / full-tests-pass /
same-pair-replay pending`。

### B52 r8：图语义闭环与 callable:line 引用归属（2026-08-02）

`51cb91ffc` 同 pair runner 2/2 PASS。Java 五条 callee-operation edge 完整出图、纯 guard 不再成为
self-call，reject/patch 从 10/5 降为 2/1；called-by 保持两个 direct production callers、零系统重复。

Java 人工仍 FAIL，原因是系统 citation normalizer 改坏了模型原本正确的 0..4 引用：方法 hop label
按 endpoint affinity 被绑定到上一跳。登记 `B52h-CALLABLE-LINE-CITATION-OWNERSHIP`。施工使用
语言无关的 `qualified_callable:line + typed evidence subject/owner + exact citation line` 三元匹配，
并排除 source-file labels；正确引用保留、错误引用唯一修复、歧义 fail-open。该信号同步供 normalize、
detach 与 pre-check 消费，不读请求/答案 prose，也不替模型判断结论。

工件：

- `eval/parallel_selected_summary_evalcampaign_b52_mergeaudit_callchain_r8_20260802.md`；
- `eval/parallel_selected_summary_evalcampaign_b52_mergeaudit_callchain_r8_20260802_manual_audit.md`。

状态：`B52g=no-regression-replay / production-trigger-pin-pass`；
`B52h=implemented / directed-pass / full-tests-pass（tool 163.592s）/ replay pending`。

---

### B52 r5：mixed relation 图已闭环，relation principal 仍有隐式兄弟竞争（2026-08-02）

`daf2c4268` 上固定 pair 严格并行 2：Java 与 called-by runner 均 PASS、零 finalizer
reject/patch。Java mixed call-DAG 的三条 call 与一条 guard 正常出厂，故 B52d 已覆盖；正文残余
方法名/跳数/持久化误述记为模型波动，不授权系统改写。

called-by 的 2 项 direct roster 正确且已获得 exact typed relation principal provenance；另一份未标
role 的间接 upstream member-set 却依旧按默认规则成为第二 principal，触发模型与系统 carrier 多次
重复，并扩大 direct 查询边界。登记 `B52e-TYPED-RELATION-PRINCIPAL-OWNERSHIP`。

本批把 exact typed roster 定为“默认 relation principal axis”的唯一所有者：只将其它 omitted-role
兄弟集归为 `supporting_coverage`，显式 `principal_answer` 原样保留，无 exact authority 时不介入。
确权提前到 source-scope 校验前；判断只消费 typed candidates、role 和 system provenance，跨 relation
kind/跨全部语言复用，不扫描用户或模型文本，也不触碰 Trace 显式窗、因果投影和自动补齐。

验证：定向含生产 `Execute` 接线回归通过（1.289s），`internal/types` 全包通过（22.829s），
`internal/tool` 全包通过（167.698s）。

状态：`B52d=covered`；`B52e=implemented / directed-pass / full-tool-test-pass / replay pending`。

---

### B52 r6：direct relation 干净，调用图仍受上游 evidence handoff 制约（2026-08-02）

`fb3c2ffe5` 固定 pair 严格并行 2：called-by runner/human PASS，只有 2 个 direct caller，零系统
重复/扩界；本轮未产生 omitted-role sibling，B52e 确定性验收仍由 production Execute pin 承担。

Java runner PASS / human FAIL：5 个答案文件均已读，但入口 call-edge 未发射，另一条 call 仍是
recovered lead；sequence 经 4 reject/2 patch 后删除。matcher 已能表达 class participant 间多 operation，
所以登记跨语言 `B52f-CALL-EDGE-EVIDENCE-HANDOFF`，不再调整 Mermaid 宽容度。

施工只对 typed `QFCallChain` 增加 soft explorer guidance：每个已读且 load-bearing 的直接调用各发
grounded call-edge；定义/read coverage/member-set/closure 不能代替调用方向；动态 receiver 不猜 owner，
Proto RPC 不伪装 executable call。规则覆盖全部 executable source languages，不要求图、不改模型结论，
不触碰 Trace 显式窗/因果投影/自动补齐。

状态：`B52e=r6-no-regression / production-pin-pass`；
`B52f=implemented-soft-guidance / directed-pass / agent-full-pass / replay pending`。

验证：typed family 正/反定向测试通过（2.612s），`internal/agent` 全包通过（3.445s）。

#### B52 r2：Java 只是见证，调用图端点缺口覆盖全语言（2026-08-02）

在 `bf6114879` 上严格并行回放同一对 case：called-by runner/human PASS（117s，零 reject）；
Java call-chain runner PASS/human FAIL（220s，12 rejects/5 patches）。reply 边修复持续生效，失败
全部收敛到 qualified call endpoint。

进一步按 `SupportedReadLanguages()` 冷读 15 种语言，确认不是 Java 特例：Python/JS/TS/Rust/C++
已有 call 却丢 receiver；Kotlin/Ruby/Swift/Lua/Cangjie 无普通 call；ArkTS 依赖 TS 主路径；Proto
只有 declarative rpc。故 B52a“裸方法 + 唯一定义”的 matcher 补全只能算 partial。

登记并施工 `EVAL-B52-CALLGRAPH-X1`：

1. emit evidence 优先使用同一 call relation 的 Graph.ResolveCallTarget；
2. Go/Java/TS/Kotlin/Rust/C++/Swift/Cangjie/ArkTS 在声明类型唯一时发布定义端点；
3. Python/JS/Ruby/Lua 只保留动态 receiver expression，不猜类型；
4. Kotlin/Ruby/Swift/Lua/Cangjie 补普通 call relation，C++ 补 qualified_identifier；
5. Proto 固定为 declarative rpc N/A；ArkTS TS-parser 不可用/超时时 fail-closed，不用 regex call
   为 diagram hard gate 造证据；
6. 全语言清册由 SupportedReadLanguages 完整性测试 pin，不读取 request/model prose/thinking。

索引包全测通过（1.367s），工具定向回归通过（1.019s），完整 `internal/tool` 回归通过
（最终复跑 173.722s）；同 pair r3 待执行。

状态：`implemented / index-full-pass / tool-full-pass / replay-pending`。

---

## EVAL-B52：MERGE-AUDIT call-chain authority 闭环回放（2026-08-02）

### B52 r1：runner 2/2 PASS，人工 1 PASS / 1 FAIL

严格并行 2 个用例：

- `qf_called_by_typed_relation_query`：108s，finalizer reject=0，人工 PASS。2 个 production caller
  清册与源码一致；第 295 行说明把 `TypedRelationKindsForResolvedSources` 写成 caller 自身，是不
  改变清册的轻微模型表述错误。
- `sr_java_call_chain`：300s，finalizer reject=12、patch=5，人工 FAIL。回复箭头 `-->>` 已不再
  要求反向 call authority，证明 MERGE-AUDIT T3-2 主修点生效；但 6 轮结构成文全部失败，最后
  降级发送 rejected document + raw thinking。答案还把 `System.out.println` 终端写成“审计落库”，
  超过 fixture 的真实持久化权限。

完整日志显示，剩余 reject 不是回复箭头，而是 typed endpoint 表达不闭合：call-site evidence
精确给出 `caller + bare operation`，definition evidence 精确给出 `owner + operation`，图端点则
合理使用 `owner.operation`。旧门只接受单条 call evidence 的字符串完全相等，无法消费两条
同席 typed 事实。

### B52a：唯一 typed definition 支持的限定端点投影

通用修复保持调用方向门不变，在 exact call lane 后增加一个窄合取：

1. `from` 必须与 citable call evidence 的 Subject 完全相等；
2. `to` 必须可确定拆为 `Owner.Operation`；
3. call Object 非空时必须完全等于短 `Operation`；只有 Object 为空时才允许 AnchorSymbol 完全
   等于 `Operation`。其它短名或已限定到其它 owner 都直接拒绝；
4. 必须恰有一个 citable definition identity，其 Subject/AnchorSymbol 分别完全等于
   `Owner/Operation`；同一 source+line 的重复证据去重，多个定义位置视为 overload/歧义并拒绝；
5. definition 只解析端点，不独立产生调用方向。回复、反向调用、class-only ambiguity、
   RootCauseTrace/显式时间窗/系统补齐均保持原合同。

这是 typed identity convergence，不读取用户或模型原文，不按 Java/fixture/type 做特例，也不
为了让某张图过门而放宽模糊匹配。定向 6 个新正反臂通过（1.117s），`internal/tool` 全包通过
（167.765s）；同对 replay 待本批提交推送后继续执行。

状态：`EVAL-B52-ENDPOINT1=implemented / full-tool-pass / replay pending`。

#### B52 r3：端点 authority 已闭环，成员轴/观测轴仍会改坏模型答案（2026-08-02）

`b50f49233` 上同 pair 严格并行回放均 runner PASS，且 Java 的四条限定方法边全部 exact，
`finalizer reject/patch=0/0`，故跨语言端点身份批从 replay pending 更新为 covered。Java 答案仍有
side branch 串行化、内存/日志伪称持久化以及 5/6 跳矛盾，暂列模型解释质量 residual；系统不得
替换模型结论。

called-by 则确定性复现新的系统错误：2 个 function members 携带 3 个 call-site support_refs，
旧完成态 enrichment 不处理非等长数组，导致第一成员无逐行引用；后续 mutation 把第二成员的
line 321 错配给第一成员，并追加一份仅 1 项的“完整性补充”。登记
`EVAL-B52-MEMBER-OBS-AXIS1`（P1）。

根修在 structured completion 层按 exact grounded support 做成员/观测轴归一：每个 ref 必须只
解析到一个 member，每个 member 必须至少有一个唯一 ref，才按原序选择一成员一主引用；歧义或
缺证全部 fail-open，额外观测继续留在 evidence ledger。等长 positional payload 不进入新臂。
不扫描问题、模型正文或 thinking，也不触碰 Trace/窗/因果投影合同。

状态：`implemented / directed-pass / full-tool-test-pass / same-pair-replay pending`。

#### B52 r4：member/observation 轴闭环，mixed call-DAG relation gap（2026-08-02）

called-by 复放恢复为 2 个正确 caller、2 个正确主引用，无系统补充矛盾，
`EVAL-B52-MEMBER-OBS-AXIS1=covered`。Java 用例的 endpoint evidence 同样正确，但这次模型画了
调用与条件拒绝共存的 call-DAG；系统把显式 typed `guard_condition` 边重新当 call，造成
12 reject、5 patch 和 raw thinking 降级出厂。

登记并施工 `EVAL-B52-MIXED-DAG-REL1`：call-DAG 的 call-evidence hard gate 先消费精确 typed
edge-anchor relation。call 继续强制同向 grounded call evidence；非 call relation 交还既有 relation
legality 层；无 anchor/unknown/错端点仍按 call fail-closed，sequence invocation 不放宽。修点语言
无关，覆盖全部支持语言，并继续隔离 Trace 因果投影。

状态：`implemented / directed-pass / full-tool-test-pass / orchestrator-diagram-pass /
same-pair-replay pending`。

---

### MERGE-AUDIT-3 H3：恢复计划不得丢失累计验证闭包（T7-1）

审计结论准确，且用完整控制器车道确定性复现：失败 verify 后 replan，planner 不生成
新计划但 typed probe 证明现有 worktree 时，scheduler 恢复原计划对象；随后累计域重建
先把该对象的 `CumulativeVerificationScope` 清空，又因它是 current plan 而跳过，导致
restore cutoff 前仍生效的旧批次路径、行为合同和验证探针全部退出终验。

通用修复只依赖 controller handoff 的精确信号：

1. 仅当候选与 active plan 是同一 Go 对象(`candidate == plan`)时，在清空前保存旧累计域；
2. 新 planner 对象即使复用 plan ID 也不可信，其累计域仍被丢弃后从 durable workflow
   attempts 重建；
3. 恢复种子只进入 verification scope，不进入 active apply paths，不改变风险/审批边界；
4. 单元 pin 覆盖 restore cutoff 前传递闭包，workflow pin 覆盖
   `apply -> failed verify -> no-plan replan/probe pass -> restored verify`，并断言旧路径、合同、
   探针在第二次 verify 仍在。

验证：`go test ./internal/orchestrator -count=1` 通过(10.479s)。

状态：`implemented / full-package-pass`；MERGE-AUDIT-3 三项高危(T3-1/T3-2/T7-1)
已全部闭环，转入当前代码基线复核与中危 ROI 排序。

### MERGE-AUDIT-3 M1：基线校准与 relation source 文件身份（T5-2）

基线校准先排除了历史项重复施工：审计时的 tracediag P0×2 和 T8-1 已由
`a064e2d33` 覆盖，当前 `go test ./internal/tracediag -count=1` 通过(7.331s)；
EventSearchCoverage 已显式 key-first、坐标固定小数，line-window scope 取实际选中域。

T5-2 则仍开放且准确。红测证明同名异文件 relation source 会因旧身份只有名字而
静默少一项。修复把 source 与 member 的 canonical identity 对齐为
`normalized name + canonical file`；有 support file 时精确匹配，typed 文件轴缺失或
身份歧义 fail-open，同名同文件重复 observation 才允许折叠。该规则覆盖所有 relation
kind，不依赖 raw request、模型 prose、case ID 或具体 symbol。

完整 `go test ./internal/tool -count=1` 通过(180.528s)，新增 source 三臂测试组复跑
通过(2.805s)。

状态：`P0-1/P0-2/T8-1=covered-by-a064e2d33；T5-2=implemented/full-package-pass`。

### MERGE-AUDIT-3 M2：日志 verbatim 权威门同源（T4-1）

红测确认 Windows CRLF 附件被 prompt/blob 规范化成 LF 后，模型忠实复制的多行
message/evidence 会被读取原始附件的三类 hard gate 同时拒绝。修复把平台换行规范化
提升为 `textfmt.NormalizeAttachedArtifactText` 单一源，模型可见表面和 error-message
存在性、同 message 基数、observation evidence 校验共用；只折叠 CRLF/CR，不做任何
模糊匹配或语义放宽，合成文本和超额基数继续 fail-closed。

验证：`go test ./internal/textfmt ./internal/context ./internal/tool -count=1` 全绿
(0.709s / 1.630s / 175.061s)。

状态：`T4-1=implemented / relevant-full-tests-pass`。

### MERGE-AUDIT-3 M3：coverage witness sideband 自动闭包（T2-1）

红测确认 cmd 的 9 项手抄表已经漏掉生产 `unknown_comm_witnesses`，超长 coverage
截断后客户诊断无法看到该 witness。修复废弃消费端名单，改由 hitraceconv 暴露统一
命名合同：安全的 `_witness` / `_witnesses` / `_witnesses_<reason>` payload 自动、排序
进入 8 KiB sideband；emitted/omitted/cap accounting 与非法键不提升。未来新增 witness
family 不再需要第二处同步，900 行总界和 receipt 均保持。

验证：`go test ./internal/hitraceconv ./cmd -count=1` 全绿
(130.346s / 9.090s)。

状态：`T2-1=implemented / full-package-pass`。

### MERGE-AUDIT-3 M4：raw/DB scheduler 时间对齐软诊断（T1-1）

风险确认，但 `db>0 ∧ raw>0 ∧ enriched=0` 不是精确信号，不能驱动事件抑制。修复只新增
typed observation：统计 exact timestamp+CPU coordinate overlap；两侧非空且零交集时
标记 `unproven_no_exact_timestamp_cpu_overlap` 并在 semantic-quality caveat 明示
“不证明时钟域偏移、合法非重叠/过滤也可能产生、advisory only、未据此门控或改写事件”。
exact key join、生命周期门和发布计数均不变。

same-input accounting golden 首轮捕获非适用车道多发指标，传播条件已收窄到 typed
observation 真正存在，固定输入收据恢复字节不变。最终
`go test ./internal/hitraceconv -count=1` 通过(96.379s)。

状态：`T1-1=implemented-soft-diagnostic / no-unsafe-hard-gate / full-package-pass`；
跨时钟自动去重继续等待独立 typed calibration authority。

### MERGE-AUDIT-3 M5：runtime profile 独立错误同轮 census（T6-2）

逐面清点确认 finding 的收窄形：单个 profile 已能聚合自身缺字段，开放 gap 是
`runtime_artifact_scope_profile / runtime_target_profile / runtime_question_profile`
之间的先败先返。施工在共同解析点先构造三份 typed 声明，再按稳定 schema 顺序一次返回
所有独立错误；`runtime_targets` 自身非法时跳过依赖它的 target-profile 语义检查，避免把
上游 roster 失败级联成虚假的 missing-target 错误。soft warnings 仅在 census 通过后发布，
后续跨字段 consistency gate 保持原位。

该批不读取用户/模型 prose，不按 case 或 runtime type 分支，也不修改显式时间窗、Trace
查询、因果投影、根因排序、唤醒链、窗内可消除量或自动补齐。两个回归分别固定同轮三错
完整 roster 和依赖级联负例；`go test ./internal/tool -count=1` 通过(160.919s)。

状态：`T6-2=confirmed-narrowed / implemented / full-package-pass`；下一项审计 T7-2。

### MERGE-AUDIT-3 M6：meta-runner coverage 只认 typed declaration edge（T7-2）

红测确定性复现：直接执行的测试脚本仅在 `SKIP` 与日志字符串中提到 changed path，旧
`exactQuotedStringLiterals` 仍把它纳入 `DeclaredCoveragePaths`；成功 Make target 随后
将其升级为 source-static changed-path coverage，且没有下游纠偏门。

施工删除脚本正文的 arbitrary quoted-literal 推断。跨语言 meta-runner 的成员 roster 只由
Make exact prerequisites 和 recipe exact existing file arguments铸造；普通同语言 runner
不变。真实脚本访问若要扩展，必须提供 runtime access receipt 或可证明 data-flow 的 typed
证据，不能用 API/关键词扫描替代。正例改用 prerequisite 明确声明成员，并固定多成员、
部分成员、Python 与 Rust cross-language 执行边界。`go test ./internal/tool -count=1`
通过(161.874s)。

状态：`T7-2=confirmed / implemented / full-package-pass`；下一项 T7-3。

### MERGE-AUDIT-3 M7：resume 缺失 durable plan 时 fail-closed（T7-3）

workflow envelope 只有 PlanID/ref、batch goal 与 attempts；target paths、edits、behavior
contracts、verification probes、approval record 仍只在 companion plan JSON。旧恢复路径找不到
JSON 时仅 warning，controller 可从 batch goal（最差为 `continue active write workflow`）重规划，
形成不可见域收窄。

现在只在 typed resumed active PlanID 没有同 ID live/durable plan 时，于所有 controller/planner/apply
dispatch 前 fail-closed（异 ID stale Mutable plan 也不能冒充）；进度账写
`resume_plan_artifact_missing`，用户面说明补回 artifact 或
clear 后按完整原请求重开。run 保持 `in_progress` 以允许恢复，不从 goal/progress prose 猜计划。
正常 plan hydration、handoff、retry budget、pending approval 和 deadline resume 均有正臂。
`go test ./internal/orchestrator -count=1` 通过(11.136s)。

状态：`T7-3=confirmed / implemented / full-package-pass`。相邻 report-artifact 缺失形登记
`T7-4/P2` 待独立审计（已由 M17 闭环）；下一项 T5-3。

### MERGE-AUDIT-3 M8：relation claims 独立违规同轮 census（T5-3）

审计确认 typed `relation_claims` 仍是逐字段首错即返；同一 payload 的关系、算术权限、成员、
subtotal、单位和 closure 缺席会逐轮消耗重试。已在共享 validator 改为稳定顺序聚合：已知
authority 的每个独立字段同时报告 got/want；缺失、重复、未知 authority 只报身份错误并跳过
依赖字段，禁止伪级联；closure 缺席全量追加。错误显示 cap=12，首部保留总数并以 `+N` 披露
截断。该门仍只比较 structured model claim 与 typed authority，不检查或改写任何答案 prose。

九违规单轮 pin 及 `internal/types + internal/tool` 完整包通过（21.774s / 163.137s）。
状态：`T5-3=confirmed / implemented / full-package-pass`；下一项 T6-4。

### MERGE-AUDIT-3 M9：apply 后 owner anchor changed-path 撤权与重建（T6-4）

确认跨批 owner 复用缺少陈腐臂：后批实际改写文件后，前批 line/symbol anchor 仍可满足 hard
localization requirement。post-apply 现在从 typed PatchEffect old/new path、AppliedPaths 与 applied
change records 建 changed-path 集，先把命中路径的 context anchor 标 stale，再从当前 worktree +
actual diff hunk 重建 replacement。未改路径继续跨批复用；删除/改名/owner 消失则 requirement
自然 reopen。OwnerAnchorView 额外拒绝 stale P0 anchor，普通 P0 constraint 行为不变。

回归固定旧 owner 撤权、新 owner 重建和 untouched owner 保留；`internal/types +
internal/orchestrator` 完整包通过（22.007s / 10.134s）。状态：
`T6-4=confirmed / implemented / full-package-pass`；下一项 T1-2。

### MERGE-AUDIT-3 M10：越界 profiler timestamp fence 反证（T1-2）

复核否证原 finding。`uint64 timestamp > MaxInt64` 超出 systrace signed-ns 输出域，发布时做
截断/回绕/钳位都会伪造时间；现有 fixed ledger 已记录 read/emitted=0 与精确 typed reason。
F2FS/Block 在 envelope 判决前解析 exact lane，拒绝分支 poison lane/family；MMC 关闭 family，
普通 instant row 则没有 pair state 可 fence，不应升级 source-global 丢弃。

新增 `MaxInt64+1` F2FS endpoint 夹在合法 begin/end 间的生产链 pin，确认 overflow 诊断在场且
两条合法 endpoint 被 exact-lane withheld；`internal/hitraceconv` 全包通过（96.130s）。状态：
`T1-2=disproved / no-production-change / regression-pinned`；下一项 T3-4。

#### 批 M11：T3-4 主值状态 roster 截断 authority（2026-08-02）

复核发现旧代码看似有“额外 N 行省略”提示，实际是死分支：projection compiler 已在上游把
artifact partitions 限为 4，故 `states` 不可能超过 4。最终因果投影块消费了 typed
`OmittedArtifactLabels`，模型成文前的 principal-value recap 却没有消费，构成同源权限面的
完整性漂移。

M11 将该既有 typed census 接到 prompt 软上下文：发布
`principal_state_roster_coverage` 的可见账户数、额外被 cap 的 artifact partition 数、
`capacity_truncated / complete=false`，并把被省略分区中的 state-account 数明确标为
`not_evaluated`，不猜总量。该行不生成结论、不改模型 prose、不扩大或收窄因果投影权限，
也不读取原始问题或模型答案。

以 5 artifact → 4 visible + 1 omitted 的生产链回归验收；
`go test ./internal/agent -count=1` 全包通过（2.643s）。状态：`implemented / full-package-pass`。

#### 批 M12：T6-3 bare 栈帧源码宇宙收敛（2026-08-02）

确认 `GlobByBasenames` 的 git 快车道会纳入已跟踪的 vendor/node_modules/hidden 同名文件，
而无 Git 的 walk 车道会排除它们；这会让同一 bare stack-frame basename 随环境改变解析结果。
M12 在 resolver 内抽出单一 visibility predicate 给两条车道共同使用，仅约束歧义 basename
发现；显式 repo-relative/absolute frame path 的验证保持不变。真实 git fixture 固定四类同名文件
只返回 production 候选。

`go test ./internal/analysis/logtriage -count=1` 全包通过（1.024s）。状态：
`implemented / full-package-pass`。

#### 批 M13：T4-2 perf-triage task-language context（2026-08-02）

确认 perf-triage 的模型输入混入 “validator stamps / downstream must/agents” 内部管线叙述。
M13 把同一权限边界改写为模型当前可执行的证据任务：jank 的 trigger/reason/tags 是 navigation
candidate 而非 causal proof；observation 用于定位区域；数值、调度类别、机制和因果以
deterministic trace_query 为准。没有新增用户原文或模型答案关键词门，也没有改变 typed
authority、schema 或最终答案。

`go test ./internal/skill -count=1` 全包通过（0.316s）。状态：
`implemented / full-package-pass`。

#### 批 M14：T2-3 conversion sideband path authority（2026-08-02）

确认 `--diagnostic-report` 只避让 3 席且使用弱 `Abs+Clean` 比较。M14 由 hitraceconv 单源导出
route-neutral path reservations（input/systrace/tracebundle/perftrace/DB/companion）与 conversion
canonical alias 判定，CLI 在创建 sideband 前消费，不再复制 sidecar 路径构造。测试覆盖清册
每一席及 prospective symlink-parent alias。

`go test ./cmd ./internal/hitraceconv -count=1` 全包通过（7.022s / 92.890s）。状态：
`implemented / full-package-pass`。

#### 批 M15：T2-4 conversion diagnostic 文档契约（2026-08-02）

CLI 已有有界 `--diagnostic-report`，但日常入口和 architecture trace diagnostics 只介绍转换后
`--tracediag`。M15 在 `CLAUDE.md` 增加单命令，在 architecture §13.7.1 固定 success/failure、
900 行帽、O_EXCL、不 alias 全部转换保留路径、status-only 冲突及与 tracediag 的能力边界。
零生产代码改动。

CLI flag/diagnostic 定向测试通过（1.320s）。状态：`docs-implemented / targeted-test-pass`。

#### 批 M16：T2-2 diagnostic capability 权限分层（2026-08-02）

确认 capability 清册是构建声明而非本次执行证明；静态清册测试无法证明 86 条生产路径均实际
运行。M16 保留版本可比的能力词表，紧邻增加 typed authority：scope 固定为
`build_advertisement`、`proves_observed_execution=false`，并把 provider decision、artifact、
coverage、DB coverage 和 typed error 列为本次转换的执行证据席。数量从清册单源派生。

没有改转换行为，也没有把 86 个名字扩成答案硬门。定向回归通过（1.250s），cmd 全包通过
（6.452s）。状态：`implemented / full-package-pass`。

#### 批 M17：T7-4 resumed verify evidence 有限权限恢复（2026-08-02）

独立审计确认 durable plan 在场时 mutation scope 未丢，不应因 ChangeReport 缺失整体
fail-closed；但旧恢复会让 needs-replan planner 的 failure handoff 彻底为空。M17 对完整报告
新增 `Passed=false + exact PlanID` 合取；报告不可用/错身份时，仅从 typed attempt 恢复
plan/batch/attempt、reason、diff/surface refs，并标记 report evidence unavailable。planner 把
所有 report-backed 字段发布为 `not_evaluated`，不从 reason code 猜失败测试或命令。

真实 resume→replan→apply→verify 回归和 types/agent/orchestrator 全包通过
（24.536s / 2.926s / 14.504s）。状态：`implemented / full-package-pass`。

### B47：运行日志语义所有权 × write 跨语言检查权限（2026-08-02）

严格并行 2 个跨模式 case：

- `read_combo_log_current_code_dimensions`：runner PASS / human FAIL，258s；
- `github_issue_commons_lang_random_ascii_symptom`：runner FAIL / human FAIL，426s。

完整人工审计结论：

1. read/log case 的日志/当前源码两条证据通道已经分离，Explorer 实际读取 6 个源码文件，
   finalizer 也没有系统重写；但 Log Triage 首先把
   `⟳ 4/4 模型响应出错,正在重新撰写答案` 解释成“四个模型全部失败”。当前 renderer
   的 `K/N` 是 pipeline stage ordinal/total：read finalize 正是第 4/4 阶段，与模型数无关。
   该错误结构化 observation 随后污染 Analyzer、Explorer aggregate 和最终答案。
2. 同一 read 答案又把日志的 `attempt 1/3`、当前源码的
   `finalizerIdenticalErrorStreak=4`、`finalizerLocalRetriesUsed`、
   `finalizeRepairHardCapValue=2` 串成一条历史执行链。它们属于不同 owner/domain；
   “当前定义存在”只能解释候选机制，不能证明历史日志实际跨过每一个 gate。
3. write case 首次实现被项目 `make check` 正确打红；replan 后源代码以
   `end <= 0x7f` 约束 ASCII fast path，非 ASCII 字母/数字测试也改为明确码点，第二次
   `make check` exit=0。最终却因检查脚本运行于 Python、变更目标为 Java 而把两个精确
   changed paths 都判 uncovered，交付降为 unverified。
4. 这不是“Python 是否能证明 Java 行为”的二选一。系统混淆了三个维度：
   driver execution family、exact checked artifact、behavior-contract strength。changed-path
   coverage 只回答成功检查是否精确触及变更路径，不应冒充目标语言 project runner，
   也不应替代独立 behavior-contract authority。

| GAP | 优先级 | 泛化问题 | 最优方案 | 状态 |
|---|---:|---|---|---|
| `EVAL-B47-XCOV1` | P1 | repository-declared meta-runner 的 driver family 被要求与 exact target family 相等，Python 静态/行为检查无法覆盖 Java/C/Rust 等精确输入 | 分离 driver 与 target 两轴；只有 exact TestSurface candidate + exact Make target + concrete driver + exit=0 + exact bounded roster member 合取才授权该路径；新增 `declared_project_check` caliber，禁止目录/语言扩权 | implemented/full-tests-pass |
| `EVAL-B47-RUNTIMESEM1` | P1 | Log Triage 对 renderer progress 与独立 retry counters 缺少 typed owner/domain，上游错误结构化后全链放大 | 在日志解释前提供 typed operational event semantics；`K/N` 明确 carrier kind/owner/numerator/denominator，retry/cap 明确 domain/owner；历史因果连接必须有 typed transition witness。只给模型准确上下文和软指导，不扫描/替换模型正文 | implemented/full-tests-pass；replay next |
| `EVAL-B47-ANAFORM1` | P3 | Analyzer 对 runtime scope 与 request-only quote 连续 3 次 schema 修复 | 已 fail-loud 且最终收敛；待更高优先问题完成后审计 schema ergonomics，不为本 case 加宽校验 | watch |

#### B47-XCOV1：exact declared project check

本批实现：

1. 普通 runner 保持既有 `target language family ∩ runner family + working_dir` 权限；
2. meta-runner 只有匹配精确 TestSurface key/MakeTarget、携带 concrete driver family、
   有界 `DeclaredCoveragePaths` 且命令成功时，才以 exact roster membership 授权；
3. 跨语言 exact member 的 caliber 为 `declared_project_check`，不伪装成目标语言
   `project_runner`；driver family 也不能授权同目录 sibling；
4. roster 单独存在、动态 executor、target 不匹配、失败命令、部分 roster 的缺席成员均
   fail-closed；普通 Python pass 仍不能覆盖未声明 Java 路径；
5. changed-path presence 与 behavior-contract proof 保持分层，系统没有从测试脚本 prose
   猜合同，也没有读取用户输入/模型答案做硬门。

施工清单：

- [x] B47-T1：严格并行 read/log + write/Java，完整人工审计答案、日志、patch、两次报告；
- [x] B47-T2：实现 driver/target 双轴与 `declared_project_check`；
- [x] B47-T3：覆盖同语言正例、跨语言 exact 正例、部分 roster sibling 负例、失败命令、
  无 execution-family roster、普通跨语言 runner 和 suite mismatch；
- [x] B47-T4：完整 `internal/types + internal/tool` 回归通过（types 22.711s、tool 162.029s）；
  提交推送；
- [x] B47-T5：实现 typed runtime-event semantics；核心四包与完整 tool 回归通过，独立提交推送；
- [x] B47-T6：干净 HEAD 严格并行同 pair r2；runner 2/2 PASS，人工 0/2 PASS，登记下一层
  value-semantics 与跨 plan proof closure GAP；
- [x] B47-T7：修复 SEMCAL1；独立测试/提交推送后同 read case 与一个新高优先 data/operation
  case 严格并行 2 个；
- [ ] B47-T8：修复 REPLANPROOF1/CAPCAL1，独立提交推送；后续 write case 验证累计路径与
  capability caliber，不以最终“verified”字样为 oracle。

#### B47-RUNTIMESEM1：producer-owned operational semantics

进一步冷读确认：静态 log-triage skill 原本已经要求模型不得猜 numeric prefix、progress
ordinal 和 attempt counter 的 producer/meaning，但 r1 的 Log Triager 仍先生成错误 structured
observation。说明单纯再写一条自然语言纪律不是高 ROI 修复；必须让拥有协议的系统在模型解释前
发布精确字段，同时保持模型对“这些事实意味着什么”的结论权。

本批实现的通用边界：

1. `LogBundle` 新增只由系统铸造的 Layer 4 `OperationalSemantics`。它不接受模型 tool
   payload，也不读取用户 request、模型 thinking、summary 或 final prose；
2. decoder 只接受锚定 producer 的精确协议行。renderer 行必须同时满足 `[render]`、已知
   本地化 stage/lifecycle 词组，以及 producer 自己的 `progressForStageKey` 的 K/N；视觉相似的
   customer/collector 行、嵌套引号和错误 ordinal 均不 mint 权限；
3. r1 witness 被明确解码为
   `producer=render,event_kind=pipeline_stage_lifecycle,stage=finalize,lifecycle=retry,`
   `counter_domain=pipeline_stage_progress,value=4/4`，并显式排除 model/LLM/fallback/repair
   count；orchestrator 的 `finalizer attempt 1/3 failed` 属于独立
   `counter_domain=agent_dispatch_attempt`；
4. `transition_authority=event_local_only`：不同 counter domain 不能相加、比较或串成执行链，
   除非另有 typed transition witness。当前源码存在 streak/local-fallback/hard-cap 常量只提供
   mechanism context，日志未发布相应 domain 时不能证明历史事件穿过那些 gate；
5. typed 字段在 Log Triager 初始上下文中先于模型观察出现，triage 完成后再从原始完整日志
   重铸进 immutable-copy bundle。下游 context 将其前置；同一 artifact line 上冲突的模型
   observation 不删除，只降为 `supporting_coverage` 并记录
   `triager_interpretation_superseded_by=log:protocol:N`；
6. 因此系统只纠正证据权限，不替模型生成“根因”、不删除或重写答案。没有新增 answer-text
   scanner、case/type 关键词门或用户原文门，也没有修改 Trace resolver/query/materializer、
   显式时间窗因果投影、自动补采或自动补齐。

上下文精准性/充分性审计：

- 精准：K/N、owner、stage、lifecycle 与 excluded meanings 都由同一个 producer grammar
  决定，不从邻近自由文本或当前源码猜取；每行保留 exact artifact line 和 line span；
- 充分：模型现在可回答“4/4 是什么”“1/3 与 4/4 是否同一计数”“当前源码的 4/2 是否能
  证明该次历史事件”三个 r1 核心问题；
- 有意不充分：typed row 不证明故障最终根因、未观测 gate 的 branch result、运行二进制与
  当前源码版本相同，也不证明两个 domain 间存在跳转。这些仍需模型基于其它证据判断；
- 接线：pre-triage instruction、post-triage bundle、final context、observation ledger 四面均有
  测试；producer/ordinal/nested-quote 负例防止“看起来像日志”的文本获得硬权限。

验证：

- targeted operational-semantic/ledger/context tests：四包通过；
- `go test ./internal/agent ./internal/context ./internal/render ./internal/types -count=1`：
  全通过（types 22.198s）；
- `go test ./internal/tool -count=1`：全通过（162.889s）；
- 完整测试另发现已有注册表已从 40 扩展至 42 项，而 Tier-B 结构 pin 仍冻结 40；精确补入
  `INDEPENDENT MECHANISM CONTRAST` 与 `RUNTIME RULE INSTANTIATION` 两个既有前缀，
  只修测试合同，不改变 production prompt/behavior。

#### B47 r2：typed domain 生效，但 value semantics 与累计证明仍不闭环

同 pair、干净 `main@9e187caf8` 严格并行 2 个：

- read/log：159s，runner PASS / human FAIL；
- write/Java：312s，runner PASS / human FAIL。

read 全链人工审计：

1. producer decoder 四面接线真实生效。Log Triager 首轮输入、Analyzer/Explorer 结构上下文均
   明确显示 `agent_dispatch_attempt=1/3`、`pipeline_stage_progress=4/4`、
   `transition_authority=event_local_only` 与不同 namespace 不可连接；
2. 但当前字段只说 counter domain，没有明说值的量纲。Log Triager 仍把 renderer 行解释为
   “4/4 个模型失败 / 4 次完整重试”；Explorer 和 Finalizer 虽改口为 pipeline progress，仍把
   4 当 retry count，并进一步推出“耗尽、全部失败、无法继续复用输出”的因果链；
3. observation compiler 已生成 `log:protocol:N` principal row，并把同 artifact line 的 model
   observation 降权；最终 compact ledger 却在 94 条中优先展示 model-authored aggregate，
   system protocol rows 落入 dropped principal runtime records。也就是说 source of truth 存在，
   但 compact selection 没给它保留席位，后生成的错误 aggregate 再次遮蔽它；
4. 因而 `RUNTIMESEM1` 是部分生效，不应通过追加“不要误解”提示收口。最优泛化补强是 typed
   value semantics：`value_kind=stage_ordinal`、numerator=one-based stage position、
   denominator=total configured pipeline stages，并显式排除 retry/attempt/failure/budget/
   exhaustion；compact ledger 对 `log_protocol_decoder` principal row 保留有界席位。系统只提供
   权威事实和关系边界，仍不扫描/删除/替换模型答案。

write 全链人工审计：

1. 第一 plan 删除错误 ASCII 收窄并新增非 ASCII Java 测试，第一次 `make check` 因缺
   `end <= 0x7f` 正确失败；replan 只补主文件 fast-path guard，第二次 `make check` exit=0；
2. XCOV1 对 active repair plan 生效：主 Java 文件获得
   `caliber=declared_project_check`，不再因 Python driver 跨语言而误判 uncovered；
3. 但最终 proof 只闭包 latest plan。前一 plan 仍留在 worktree 的 Java test-file 改动没有进入
   最终 changed-path closure；成功 `make check` 的 exact declared roster 本可覆盖它，系统却没有
   以 cumulative applied paths 为验证目标；
4. 最终 `profile_status=strong/state=verified` 还把 Go probe 读 Java 源码查 token、Python
   `make check` 查源码/测试 token 计作两个 tests。它们能证明结构条件和 exact path/check，
   不能证明 Java runtime behavior；最终 proof ledger 没有继承前一 plan 的 behavior-contract
   obligations，却宣称全部 obligation covered；
5. 产品 patch 对 fixture 是正确的，但交付权限错误，因此人工仍 FAIL。修复必须分开：累计
   still-applied paths/contracts 跨 replan 继承；验证 capability 增加 source_static/check 与
   target_behavior 等 typed caliber，changed-path coverage 不等于 behavior proof。

新增 GAP：

| GAP | 优先级 | 泛化问题 | 最优方案 | 状态 |
|---|---:|---|---|---|
| `EVAL-B47-SEMCAL1` | P1 | counter domain 精确但 value/unit 未铸造，stage ordinal 仍被升级成 retry count/exhaustion；system protocol principal row 又被 compact ledger 排掉 | protocol carrier 增加 value kind、分子/分母语义与 typed exclusions；compact principal selection 为 system protocol rows 保留 bounded seats，冲突 model rows 保持 advisory。无答案扫描/替换 | covered/live-replay |
| `EVAL-B47-REPLANPROOF1` | P1 | repair plan 只验证自己的 target path/contract，前 plan 仍应用的测试/源码和行为义务从最终 proof closure 消失 | workflow verification input 取 cumulative still-applied path + contract closure；successful exact roster 可逐成员授权，失败历史保留但由后续成功证据 supersede，不能静默遗忘 | implemented/full-tests-pass；replay next |
| `EVAL-B47-CAPCAL1` | P1 | Go/Python 源码 token scan 被计成“2 tests/strong”，虽无目标语言行为执行且无 contract coverage | 每个 command/probe 发布 typed verification capability/caliber；source-static 只授权结构/path/check，target behavior 必须由执行型、身份绑定且 contract-bound 证据授权；最终强度按能力而非 test_count | implemented/full-tests-pass；replay next |

上下文充分性结论：read 不是信息数量不足，而是 value semantics 少一层且 compact authority
排序反转；write 不是 runner 没运行，而是最终 proof context 丢失跨 plan 目标与验证能力等级。
两者都属于 system-to-model/context authority GAP，不能归为模型随机波动。

#### B47-SEMCAL1：值量纲与 compact authority 补强

施工结果：

1. protocol row 新增 typed `value_kind`：renderer K/N 为 `stage_ordinal`，orchestrator
   attempt K/N 为 `attempt_ordinal`；
2. renderer 的 numerator/denominator 明确为
   `one_based_pipeline_stage_position / total_configured_pipeline_stages`；attempt 行明确为
   `current_dispatch_attempt_ordinal / maximum_dispatch_attempts`。attempt 1 是初始尝试，不能
   直接叫“一次 retry”；
3. renderer exclusions 扩为 model/LLM attempt、stage retry、failure、retry budget、exhaustion、
   fallback、repair-round；attempt exclusions 包括 pipeline progress、retry count、model count、
   identical-error streak、local fallback、repair cap；
4. lifecycle 与 value kind 分席：`lifecycle=retry` 只说明该事件处于 recoverable state，不能把
   `stage_ordinal` 的 K/N 改造成 retry/attempt/failure/budget/exhaustion count；
5. observation protocol principal row 同时携带简短 `value_semantics` note。prompt 排序对 exact
   `producer=log_protocol_decoder + id=log:protocol:* + principal` 保留分档 bounded seats：
   limit≤4 保留 1、≤8 保留 2、其它保留最多 4；请求 origin floor 仍先保留，因此 log+source
   混合问题不会被协议行挤掉 current-source lane；
6. 这仍是 typed record 展示优先级，不读取 RawRequest、model aggregate prose、thinking 或 final
   answer，不拒绝/改写模型结论。Trace 路由、时间窗、因果投影、自动补齐全部未改。

测试：

- 精确 producer/ordinal/lookalike、value semantics、两种 counter domain、pre/post-triage 接线；
- 20 条强 current-source records 压力下两个 protocol rows 均保留且 current source 不丢；
- `go test ./internal/agent ./internal/context ./internal/render ./internal/types -count=1` 全通过
  （types 22.893s）；
- `go test ./internal/tool -count=1` 全通过（161.256s）。

#### B47 r3：value semantics 通过；事件邻接仍被升级为因果跳转

干净 `main@fe34ac2ee` 严格并行 2 个异构 case：

- `read_combo_log_current_code_dimensions`：177s，runner PASS / human FAIL；
- `operation_system_inventory`：44s，runner/human PASS。

operation 上下文审计结论为健康：首轮组合 awk 把三个 sysctl 值逐行误解，虽 exit=0 但模型
根据畸形输出主动续采；第二轮三条独立 sysctl 分别得到 18、18、137438953472，GPU 命令得到
M5 Max 40-core。最终 128 GiB 可复算，坏的派生行没有取得最终权限。这说明 command result 的
原始值、状态和补采上下文足以支撑正确答案；不应把一次 planner 脆弱命令硬化成固定 OS 模板。

read/log 的真实回放确认 `EVAL-B47-SEMCAL1=covered`：Log Triager 已在解释前读取
`stage_ordinal`/`attempt_ordinal`，最终不再把 4/4 说成四个模型或四次重试；两个 system
protocol principal rows 也在 94 条 ledger 压力下保留。新失败位于关系权限：模型仍把相邻的
dispatch attempt failure 与 renderer lifecycle 行串成“驱动、重置/推进、回到 pipeline 起点”，
并将 current-source retry predicates/EventAdapterRetry 升级为该历史运行已走过的完整链。
现有 `transition_authority=event_local_only` 精确但不够醒目地表达：附件只证明各事件和行序，
没有 correlation/typed transition witness。

新增项：

| GAP | 优先级 | 泛化问题 | 最优方案 | 状态 |
|---|---:|---|---|---|
| `EVAL-B47-RELSEM1` | P1 | 多个 producer-owned 事件各自精确，但模型把时间邻接升级成跨 counter/subsystem 因果跳转 | 在 exact OperationalSemantics 集合上发布 typed relation fence：`relation_authority=observed_log_line_order_only`、`cross_event_transition=unproven`、`typed_transition_witness=absent`；同样携带到 compact protocol rows。只提供证据权限和软指导，不扫描/改写答案 | implemented/full-tests-pass；replay next |
| `EVAL-B47-LOGCITE1` | P2-watch | 最终 runtime citation 出现原始 line 2→3、3→4 偏移 | 先用异构日志复现并定位 citation compiler/join；不按四行 fixture 添加偏移修正 | watch |

`RELSEM1` 施工边界：

1. 仅当两个以上系统解码事件均无 typed transition witness 时发布 relation fence；单事件不虚构
   跨事件关系，未来真实 transition authority 存在时不发布 absent；
2. prompt 明确行序只能证明 observed order，不能证明 drive/reset/advance/re-enter/return-to-start；
   current-source call path 也必须与 exact runtime event identity 联结才能证明历史跳转；
3. 每个 compact `log:protocol:N` principal row 携带同一 typed relation caliber，避免独立关系行
   额外挤占 source/protocol seat；
4. structured LogBundle 即使没有任何 model-authored observation，只要 OperationalSemantics 非空也
   不再被 empty-bundle 快路径丢弃；
5. 不读取 RawRequest、model thinking/summary/final，不做 answer hard gate 或 answer mutation；
   Trace query/materializer、显式时间窗因果投影和系统自动补齐未触碰。

验证：`go test ./internal/render ./internal/types ./internal/context ./internal/agent -count=1`
全通过（render 1.699s、types 22.603s、context 1.239s、agent 2.570s）；
`go test ./internal/tool -count=1` 全通过（161.419s）。

#### B47-REPLANPROOF1a：重规划后仍应用改动的累计验证闭包

冷读 write 回放的 workflow、两个 plan/report 与最终报告后确认：产品补丁本身同时保留了前一
plan 的 Java 测试改动和后一 plan 的 Java 源码修复，但三处 typed 链断裂：

1. `PlanPath` 是稳定别名，replan 写入新 plan 时覆盖旧 JSON；workflow 虽保留 exact plan ID，
   final loader 却已无法按 ID 取回旧计划；
2. `run_tests` 的 runner 选择、probe、contract、changed-path coverage 与 syntax fallback 都只读
   active plan/slice，前一 plan 仍在工作树中的路径和行为义务不再属于验证输入；
3. final proof artifact 只按 completed batch 的最后 `PlanID` 取一个报告，同 batch 内
   verify→replan→verify 的前一 plan/report 被静默丢弃。

本批采用 controller-owned `CumulativeVerificationScope`，而不是合并 `TargetPaths`：

- 每次 plan 快照同时写当前别名和 `<plan-id>.json` 不可变 sibling，后续按 workflow plan ID
  可恢复；既有空/非法 plan 加载仍 fail-closed；
- controller 只从 restore-aware `writeFinalReportAppliedPlanIDs` 和 durable plan artifact 重建
  `source_plan_ids + target_paths + behavior_contracts + verification_probes`，任何 planner 自带值
  都先清空，不能伪造累计权限；
- apply 继续只用 active plan/slice；verify 专用 helpers 合并 active scope 与 earlier still-applied
  scope，接到 runner preference、Django selector、syntax fallback、probe 执行、contract coverage、
  changed-path coverage 和 confidence；
- cumulative actual-diff review 的 owned paths 同样改读 restore-aware still-applied plan IDs，不再把
  已 rewind/discard 的 apply attempt 混入；
- final proof artifact 按每个 exact verify attempt 反向收集仍应用 plan 的 report，不再由
  `batch.PlanID` 抹除同 batch 的前序证据。

边界：本批只恢复“验证了什么”的闭包，尚未改变证据能力等级，也未把旧失败静默覆盖成成功。
failed→later success 的 typed capability supersession 与 `target_behavior` 缺席降级归入下一独立
`CAPCAL1` 批共同完成。没有读取用户/模型原文，没有答案拒绝或改写；Trace 时间窗、因果投影和
自动补齐路径未改。

测试：

- 新增 apply/verify 作用域分离、跨 replan 累计 path/contract/probe、固定别名覆盖后旧 plan ID
  仍可加载、同 batch 双 plan proof artifact 与热文件行数护栏；
- `go test ./internal/tool -count=1`：161.614s；
- 修正 fixture 后 `go test ./internal/orchestrator ./internal/types -count=1`：12.284s / 17.654s；
- 定向 `internal/orchestrator + internal/types + internal/tool`：全部通过。

#### B47-CAPCAL1：路径覆盖与行为证明分轴

`ChangedPathCoverage=covered` 只回答“该精确路径是否进入一次成功检查”，不能回答“检查执行了
目标实现的行为”。本批在既有 exact path/language/TestSurface 铸造点增加 typed capability，
没有分析 command text、probe code、用户问题或模型答案：

| 已有证据类型 | capability | 可授权范围 |
|---|---|---|
| 同语言 project runner + exact working-dir/path | `target_behavior` | 目标路径行为 |
| 同语言 path-bound probe，无 contract refs | `target_execution` | 目标实现被有界执行，但不签具体行为合同 |
| 同语言 path-bound probe + typed contract refs | `target_behavior` | 对绑定路径的行为合同 |
| repository-declared 跨语言 exact check | `source_static` | 精确路径静态结构/check；不能升级目标语言行为 |
| exact source/syntax fallback | `syntax_only` | 语法/可解析性；不能升级行为 |

proof profile 新增四档 path count；存在 hard behavior contracts 而累计证据没有任何
`target_behavior` 时，终态为 `weak` 并发布
`target_behavior_verification_missing`，不再由 `test_count`、command 数或
`project_runner` 标签签成 `strong`。proof ledger 同时逐路径输出 caliber + capability，模型/用户
可以直接看到“路径已检查但只属静态能力”，无需猜测 runner 名称。

impact/patch-review 投影也消费同一 typed capability：
`behavior_contract` 和 `behavior_contract_without_verify_coverage` 只有对应路径具备
`target_behavior` 才能从 unverified 提升 verified；`source_static` 与 `syntax_only` 继续可用于
路径/结构证据，不会被一刀切成失败。

历史失败采用窄而可审计的 supersession：只有 terminal primary report 已通过，并且其
`runner + framework + working_dir + suite + command` typed identity 与旧失败完全相同，旧 command
和其 local report 才从 failed 降为 advisory，reason=`superseded_by_terminal_exact_command_pass`。
旧行仍保留；不同命令、无终态成功或仍有其它失败均不 supersede。没有模糊相似度或原文扫描。

测试覆盖同语言 runner、跨语言 exact check、syntax、无合同 probe、合同绑定 probe、静态证据
不得确权行为合同、target behavior 正例、proof ledger capability 与 terminal exact rerun
supersession。全相关回归：

- `go test ./internal/types -count=1`：19.180s；
- `go test ./internal/orchestrator -count=1`：12.413s；
- `go test ./internal/tool -count=1`：163.182s。

状态：`REPLANPROOF1 + CAPCAL1 implemented / full-tests-pass / clean write replay next`。

#### B47 r4：关系权限与答案 facet 冲突；计划路径身份跨层漂移

干净 `main@b667ccc7c` 严格并行 2 个异构 case：

- `read_combo_log_current_code_boundary`：160s，runner PASS / human FAIL；
- `github_issue_commons_lang_random_ascii_symptom`：313s，runner FAIL / human FAIL，workflow
  在 verify 前 blocked。

read/log 上下文逐层人工审计：

1. `RELSEM1` 已真实到达 Log Triage、Explorer 与 Finalizer：
   `relation_authority=observed_log_line_order_only`、
   `cross_event_transition=unproven`、`typed_transition_witness=absent`；4/4 与 1/3 的
   carrier/value/domain 也准确，说明 producer-owned 上下文的精度已经足够；
2. Explorer 只读了 semantic decoder/message 定义，没有读到把 dispatch failure 连接到 renderer
   rewrite 的 production caller→callee 边；final prompt 也明确“无 grounded edge”，因此正确回答
   应把 runtime 跳转保持为未证，而不是补猜；
3. 模型仍把附件 line order、decoder 定义和当前机制候选写成“failure chain / root cause / 驱动
   retry/rewrite”。其中一部分是模型未服从清晰 typed fence；但系统合同同时硬要求
   `principal_path_edge`，在 typed authority 明确没有 path edge 时仍要求一个 path-shaped facet，
   对模型形成反向结构诱导；
4. 最优方案不是扫描 answer 词面或删除模型结论，而是在 Finalizer 上下文构造前，用 typed
   evidence authority 调和 required facet：无 grounded transition 时，把 path-edge 义务投影为
   `independent_facts + transition_unproven` 的不确定性 facet；一旦有真实 typed edge，原调用链
   facet 保持不变。残余越权若仍发生才归模型波动。

write 全链人工审计：

1. replan 的 source path 使用了
   `src/main/java/org/apache//commons/lang3/RandomStringUtils.java`；Git apply、actual diff 与
   patch-effect 合理地报告单斜杠规范路径；
2. planner `changes[].path`、`target_paths`、active slice 与 patch-review 各有独立弱 normalizer，
   scope gate 用原始字符串比较，因而把同一文件误报为
   `patch_effect_path_outside_plan_scope`；
3. 前一 plan 的 immutable JSON 与 controller-owned cumulative scope 均存在，证明
   `REPLANPROOF1a` 生效；本轮在 verify 前被 PATH identity 误报阻断，尚不能验收 CAPCAL1；
4. 这是 plan/dependency/slice/effect/verification 共用主键缺失的系统 GAP，不是 Java fixture 或
   某个双斜杠的特例。

新增项：

| GAP | 优先级 | 泛化问题 | 最优方案 | 状态 |
|---|---:|---|---|---|
| `EVAL-B47-PATHID1` | P1 | planner、controller、slice、patch-effect 对仓库相对路径使用不同 lexical identity，合法补丁可被误判越界，依赖/rename 也可能漂移 | 在零 internal 依赖的 `canonpath` 建立 safe repo-relative identity；emit seam 先统一 path/new_path/depends_on，controller 与 patch-review 防御性消费同一 canonicalizer；绝对路径、UNC、盘符、parent traversal 保持 fail-closed | implemented/full-tests-pass；write replay next |
| `EVAL-B47-FACETAUTH1` | P1 | typed relation authority 明确 `transition=unproven`，答案合同仍硬要求 path-edge facet，精确上下文与结构要求互相冲突 | required facets 在 prompt 前按 typed evidence authority 投影：无 edge 时要求 independent facts + uncertainty，有 edge 时保留 path；不读 RawRequest/answer prose，不拒绝或改写模型答案 | implemented/full-tests-pass；cross-log replay next |
| `EVAL-B47-REPLANPROOF1b` | P1 | restore cutoff 后只携带直接 retained plan 的来源与路径，三轮以上交替修改时，更早仍应用路径可能从 verify-only 闭包消失 | 从 controller-stamped retained durable plan 递归继承 SourcePlanIDs/TargetPaths；当前 planner scope 仍清空重建，apply scope 保持当前计划 | implemented/full-tests-pass；write replay next |
| `EVAL-B49-OPERMAX1` | P1 | 第 5 个 command 执行后 material evaluator 被 `< max` 短路，末轮材料可在无 complete/partial/budget typed 裁定时直接进入 finalizer | 末轮始终评估；typed incomplete material 可一次 5→8 有界扩展，仍未闭环则发布 `budget_exhausted`；所有追加 plan 保留风险/审批 | implemented/full-tests-pass；operation replay next |
| `EVAL-B49-HTMLBODY1` | P1 | 长 HTML/日志/命令输出只重复暴露首个 4000-rune excerpt；任意 shell 抽取产生新 ref，却没有来源身份、连续范围和完整覆盖证明 | 系统从已记录 payload 建立有界 normalized pages，发布 source identity、rune range、truncation 与仅在连续闭合时铸造的 coverage receipt；模型消费材料并保留结论权 | implemented/full-tests-pass；operation replay next |

#### B47-PATHID1：安全仓库相对路径的单一身份

本批实现：

1. `internal/canonpath` 增加 `CanonicalRepoRelativeIdentity`：trim、跨平台 slash、重复分隔符与
   `.` segment 归一；POSIX absolute、Windows volume、UNC、空/`.` 和任意 `..` segment 拒绝；
2. single-shot `emit_change_plan` 与 split `emit_plan_skeleton` 的公共 canonical shape 在任何 graph
   validator、TargetPaths、slice 或 cumulative scope 构造前，统一
   `changes[].path / new_path / depends_on`；canonical alias duplicate 因而按同一文件拒绝；
3. graph hard gate 新增 typed unsafe reason code，Windows 绝对路径在 Linux/macOS host 上也不能
   借平台 `filepath.IsAbs` 差异混入 repo；路径安全没有因修复 alias 而放宽；
4. controller 与 patch-review 改为消费同一中立 canonicalizer，actual diff 的单斜杠与 plan 的
   重复斜杠不再产生 scope escape；真正的 outside-worktree/path event 仍保持 hard block；
5. 没有读取用户原文、模型 thinking/summary/final、patch 代码内容或 case ID；未触碰 read/Trace
   路由、显式时间窗因果投影、自动补采和系统自动补齐。

回归覆盖：重复 slash、`.`、反斜杠、path/new_path/depends_on 一致化、alias duplicate、patch
effect 与 active slice 同一身份，以及 POSIX/Windows/UNC/parent traversal 负例。相关完整包回归
在 production 修改后通过：`canonpath 0.650s / writeflow 0.453s / tool 162.914s /
orchestrator 12.131s`；新增定向测试四包全部通过。

#### B47-FACETAUTH1：证据关系权限先于答案块形状

冷读最终 prompt 证实冲突不是 facet validator 误拒绝，而是更早的 semantic-view 编译：

- FacetCoverage 已因没有 `ClaimCallEdge` 候选而把 `principal_path_edge` 从 hard facet 降走；
- `rootCauseTracePrincipalListRequirement` 却无条件要求 `ordered_list + principal_path_edge`，并把
  rationale 固定为“principal cause chain / order matches causation”；
- 后置 mechanism-relation instruction 同时说“无 grounded caller→callee edge，transition
  unproven”。模型因而在同一 prompt 收到相互矛盾的硬结构与精确权限边界。

本批采用前置 typed projection，而不是答案扫描/替换：

1. 新增 `LogOperationalRelationAuthority` 单一解析：少于两事件为 `not_applicable`；任一 producer
   发布 non-local transition authority 才是 `typed_transition_witness`；否则多事件只能得到
   `observed_log_line_order_only`；
2. prompt renderer 和 semantic-view compiler 共用该解析，不再分别复制“有没有 witness”的
   判定；
3. `root_cause_trace` 在 `observed_log_line_order_only` 时把 principal contract 从有序原因链改为
   `bullet_list(current_code_path + uncertainty_boundary)`，只要求独立 grounded facts；summary
   明确要求陈述各 lane 能证明什么以及 transition 未证，diagram 也不再要求 call edge；
4. 单事件没有跨事件问题，保持既有 cause-chain shape；producer 有真实 transition witness 时也
   保持；这两条避免“凡日志一律降级”；
5. quote-anchored explicit time window 具有更高 typed authority，始终保留原 ordered causal
   contract，因此 Trace 因果投影、根因排序、唤醒链、可消除量和系统自动补齐不受影响；
6. 系统只改变交给模型的证据/结构合同，不读取用户原文或模型 prose，不拒绝结论词，不删除、
   覆盖或代写最终答案。模型仍负责解释、总结与优化建议。

上下文充分性结论：r4 已有足够信息做出“模型响应异常已观测、校验失败未观测、line 3→4
transition 未证”的正确回答；缺口是合同自相矛盾，不是再塞更多源码。修复后若模型在清晰且一致
的 typed contract 下仍越权，作为模型波动记录，不引入词面 hard gate。

测试固定多事件 local-only、单事件、typed witness、显式 Trace 时间窗四臂，并固定 relation
renderer 继续发布同一 authority。全相关回归通过：`types 19.914s / render 0.921s /
agent 2.885s / tool 165.332s / orchestrator 13.663s`。

#### B48 r1：路径身份通过；成功证明未到达，发现验证闭包二阶丢失

干净 `main@e70c70eac` 严格并行 2 个异构 write/plan case：

- `patch_cpp_typo`：56s，runner PASS / human PASS；
- `github_issue_commons_lang_random_ascii_symptom`：319s，runner FAIL / human FAIL。

plan-only C++ case 的最终计划是精确单行 patch，且 fixture 仓库字节未改变。前两次计划分别因
不支持的 bash probe 和 tab/space `old_text` 不匹配被 typed feedback 纠正；当前上下文足够，不登记
产品 GAP。

Java write case 中 PATHID1 已覆盖：三轮计划均无 path-scope 误报，均真实 apply 后执行
`make check` 并诚实失败；Java probe 因本机无 `javac` 保持 unavailable，changed paths 保持
uncovered，最终因步骤预算耗尽 blocked，没有伪造 verified。第三轮已经读取 checker，却把
`0x3B1` 误判为匹配 `0x(?:4e00|370|400)`，属于模型对精确可见证据的理解波动；不为该数值、
正则或 Java fixture 增加硬门。因为没有成功 proof，CAPCAL1 仍待另一个成功 write case 验收。

上下文深审另外确认 `REPLANPROOF1b`：`writeFinalReportAppliedPlanIDs` 在 restore cutoff 后可以只返回
最新 retained plan；该 plan 的 controller-owned cumulative scope 已证明更早计划仍在，但旧的
`stampCumulativeVerificationScope` 只加入 retained plan 自身 ID 和直接路径，导致 provenance/path
不像 contracts/probes 那样传递。最优修复是在同一 verify-only 铸造点继承 retained scope 的
`SourcePlanIDs + TargetPaths`，再加入 retained plan 自身：

1. 当前计划上的 planner-provided scope 继续先清空，不能注入权限；
2. retained plan 来自 controller durable artifact/candidate，继承的是既有 typed authority；
3. apply 仍只消费当前计划 `TargetPaths/active slice`，累计闭包只供 verify；
4. 三代 restore 回归固定 A(test) → B(source, carries A) → restore B → C(follow-up)，C 必须验证
   A+B 的路径/合同/探针且不得吸收 C 中伪造的 planner scope。

本项不读取用户输入或模型答案，不修改结论，不影响 read/Trace 路由；显式时间窗的 Trace 因果投影、
根因排序、唤醒链、可消除量及自动补齐不在这条写模式 verify-only 路径上。

完整回归通过：`types 18.612s / orchestrator 14.147s / tool 166.620s`。

#### B49 r1：write capability 成功闭环；operation 末轮覆盖裁定旁路

`main@73e93b2fa` 严格并行 2 个 write/operation case，runner 2/2 PASS；人工 1/2 PASS：

- `patch_c_typo`：106s，runner/human PASS；
- `operation_web_manual_summary`：141s，runner PASS / human FAIL。

write 正例只修改 `main.c` 一行，真实 `make test` exit=0。ChangeReport 将唯一 path 标为
`project_runner + target_behavior`，FinalReport 为 `proof=strong / completion=verified`，delivery 与
source owner 均指向同一 plan。由此 `CAPCAL1` 成功路径首次 live covered；静态 token/source checker
没有冒充行为证明。

operation 的导航和双截断 context 精确：系统从首页取得 `user_guide.html`，原始 HTML 为
248,161 bytes / 4,103 行。第 5 轮 Python 脚本明确只输出前 300 行，所得抽取工件 13,687 bytes，
正文止于 §3.1；§3.2–§8 只有目录标题。最终答案却宣布“所有章节/完整提取”，runner 的
手册+使用+URL regex 没有发现这个 false green。

深层根因分两层：

1. 新 GAP `OPERMAX1`：CLI 与 REPL 的 material evaluator 都带
   `commandRounds < commandOperationMaxCommandRounds` 条件。第 5 个命令执行完成后不再评估，
   `status=executed` 直接进入 finalizer；旧 `ContinueAfter` budget guard 又只覆盖 plan 自报
   continue，无法覆盖 evaluator-owned material continuation。
2. 既有 `HTMLBODY1`：shell 产生的新 payload 没有 upstream source hash/range/page/remaining，
   即使增加轮数也不能机械证明多个抽取片段闭合原材料。

`OPERMAX1` 本批按用户允许“预算不足可适当提升”的要求通用落地：

1. material evaluator 在 base-limit 末轮也必须执行；不再允许最后一个 payload 绕过 typed 裁定；
2. 只有系统记录存在 payload，evaluator 发出 `continue_command` 且 material coverage 为
   `partial/not_evaluated` 时，授予一次固定 5→8 扩展；普通短查询、无材料命令、complete 或
   not-applicable 不扩容；
3. 扩展仅增加 command round capacity；后续每个 plan 仍经现有 deterministic risk/approval，
   高风险、写入或不可证明命令不会因扩容自动执行；
4. 扩展上限仍未完成时，结果现在使用真实 `StatusBudgetExhausted`，final prompt 和用户前缀明确
   “部分结果/预算上限”，不再把 budget failure 填成 generic failed 或 executed；
5. CLI 生产回归从已有 4 轮状态执行第 5 个大材料命令，确认 evaluator 被调用并只因 typed partial
   获得第 6 个 bounded read；另固定第 8 轮仍 partial 时必须 budget-exhausted。REPL/CLI 共用同一
   round-limit 与 budget helper。

系统仍未读取用户/答案关键词，也没有删改 final prose 或系统替答；本批只修控制面 typed
状态和容量。完整回归：`operation 0.790s / repl 32.723s`。`HTMLBODY1` 保持下一独立高 ROI 批：
first-class bounded material reader + source-range coverage ledger。

#### B49-HTMLBODY1 batch A：系统持有的分页材料与覆盖收据

预算审计确认：5→8 自适应扩容已经消除“末轮不评估”，但如果每一轮仍重复看同一 source 的
4000-rune 前缀，增加预算并不会增加证据覆盖。batch A 先建立与 CLI/REPL 无关的统一证据载体：

1. 只读取 command result 已记录的 payload ref；普通文件、HTML visible text、日志与大命令输出
   共用一套实现，不识别 URL、站点、章节名、case ID 或用户/答案词面；
2. 每个材料页携带 source ref、source identity、representation、source bytes、page ordinal、
   `[start_rune,end_rune)`、visible runes 和两层 truncation；单源最多读取 2 MiB、最多发布
   24×6000 runes、单轮最多两个 source，边界均显式披露；
3. 只有 source 未截断、normalized page 集合从 0 连续覆盖到末尾时，系统才铸造
   `material-coverage:v1:<sha>:<representation>`；单个 page ref 永远不能冒充整源 complete；
4. evaluator 与 replan/continuation terminal validator 共用系统 authority map：既兼容旧的完整小型
   extraction ref，又接受系统 receipt；模型伪造、只引用某页或引用不完整原件会进入既有 typed
   repair，不由系统改写为 partial；
5. 最终材料 caveat 改为读取真实“完整 excerpt 或 coverage receipt”，不再把任意可渲染前缀当作
   已覆盖。答案本身仍原样保留，系统不删除、替换或代写结论；
6. 本批只增加 operation lane 的 context carrier/authority，没有接入 Trace/read/write/data 路由，
   不触及显式时间窗、Trace 因果投影、根因排序、唤醒链、可消除量或自动补齐。

定向测试覆盖多页连续闭合与尾部 sentinel、page ceiling 拒绝 receipt、receipt 正向授权、单页
反向拒绝、最终答案 byte-preserve。batch B 才把载体接到 CLI/REPL 每次 material evaluator 前，
并固定同一 source/hash 不重复注入及真实 operation 生产路径。

#### B49-HTMLBODY1 batch B：CLI/REPL/replan 生产接线

生产接线完成，且扩容与材料覆盖现在是两个正交控制面：

1. CLI 与 REPL 在每个 command result 入账后立即从其系统记录的 payload refs 构建 pages；因此
   base-limit 第 5 轮 evaluator、后续 continuation planner 和 final answerer 看到同一份精确 ledger，
   不再靠模型重复规划 `cat/head/python print` 来搬运同一材料；
2. REPL 将 pages 与 evaluator typed verdict 同步回 operation result history，后续本地继续/上下文渲染
   不会退回只有 4000-rune prefix 的旧状态；同 source identity + representation 在同一 records 内只
   注入一次，复制同一完整 payload 不会重复消耗 prompt；
3. failed-command replan 也在 planner 边界从前序 result 建立同一 pages/receipt，并把 ledger 同时放入
   初次 replan prompt、compact repair context 和 terminal coverage validator，关闭 evaluator 之外的
   完成权限旁路；
4. page reader 不执行新命令，不增加 side effect 或 approval 权限；超过 2 MiB/144k normalized runes、
   binary/invalid UTF-8/非普通文件均不发 receipt，模型仍可选择有界 search/extract，typed partial 可按
   OPERMAX1 获得 5→8 扩容，达到 8 仍不完整则 budget-exhausted；
5. 系统没有从用户输入或模型 prose 扫关键词，也不推导“哪些章节重要”或写最终结论。它只提供
   source/range/content/closure authority；相关性、摘要、结论和下一步建议仍由 evaluator/planner/
   answerer 模型作出；
6. Trace/read/write/data 的路由与上下文构造未改。显式时间窗 Trace 因果投影、系统自动补采/补齐、
   根因双维度、唤醒链和可消除量保持原权限与行为。

生产回归固定两条接线：CLI base-limit 大输出在 evaluator 前已有 pages + receipt，且 typed partial
仍只获得既有一次扩容；REPL 大材料的 evaluator/continuation/final prompts 均出现 ledger/receipt，
history 同步保留。完整回归：`operation 0.534s / repl 33.617s`。

任务状态：

- [x] B47-T7：SEMCAL1 实现、全量测试、独立推送，并以 read + operation 严格并行 2 个回放；
- [x] B47-T7a：完整人工审计两例的系统上下文、原始结果与最终答案；
- [x] B47-T7b：RELSEM1 完整 tool 回归、提交推送；下一异构日志批验证关系 fence，并观察
  LOGCITE1 是否复现；
- [x] B47-T8a：REPLANPROOF1a 累计作用域、不可变 plan 工件、多 plan proof artifact 独立实现并回归；
- [x] B47-T8b：实现 CAPCAL1 与 failed→later-success typed supersession，完整相关回归；
- [x] B47-T8c：从干净 HEAD 严格并行 2 个异构 case并完成人工审计；累计 scope 工件在场，
  但 PATHID1 于 verify 前阻断，CAPCAL1 仍待下一 write 回放；
- [x] B47-T8d：实现 PATHID1、跨平台安全负例与 patch-review 生产 witness 回归，独立提交推送；
- [x] B47-T8e：审计并实现 FACETAUTH1；证据权限与 answer block 单源投影，不以 answer/request
  关键词 hard gate，不系统替答；完整相关回归通过后独立提交推送；
- [x] B47-T8f：从干净 HEAD 严格并行下一对异构 case；PATHID1 live covered，成功 capability
  ledger 未到达；人工审计同时定位 REPLANPROOF1b；
- [x] B47-T8g：完成 REPLANPROOF1b 全量回归；
- [x] B47-T8h：独立提交推送后以成功 write + operation 严格并行 2 case；CAPCAL1 live covered，
  operation 定位 OPERMAX1；
- [x] B47-T8i：末轮 material evaluator、typed 自适应预算和真实 budget-exhausted 状态实现并全量回归；
- [x] B47-T8j：独立提交 OPERMAX1，并完成 HTMLBODY1 batch-A 分页/receipt 证据载体；
- [x] B47-T8k：独立提交推送 HTMLBODY1 batch A；batch B 接入 CLI/REPL/replan material context，
  完整相关回归通过；
- [x] B47-T8l：独立提交推送 HTMLBODY1 batch B；用 operation case + 异构高优 Trace case 严格并行
  2 个回放，人工验收材料闭环、上下文精度与最终答案。

#### B50 r1：长材料闭环通过；Trace 模型前权限上下文仍有矛盾

从干净 `main@9f2a00e5a` 严格并行 operation + 显式窗 Trace：

- `operation_web_manual_summary`：93s，runner/human 双 PASS；
- `real_trace_h5_smr_multirow_disposition`：260s，runner/human 双 FAIL，但 runner 的失败面与人工失败面不同。

operation 只执行两次 curl：首页的 32,655 bytes/3,390 visible runes 先形成完整 ledger，模型从 typed
href 找到 `user_guide.html`；手册的 248,161 bytes/118,802 visible runes 随后被 20 个连续页面覆盖，
`source_truncated=false / pages_truncated=false` 并铸造 complete receipt。evaluator 依据 receipt 发布
complete，模型自行形成覆盖八章的使用总结，没有 shell 搬运循环、截断前缀冒充全文或系统代写。
`HTMLBODY1` 因此获得首个生产回放闭环见证。耗时从历史同 case 的 141s 降为 93s，但单次运行受模型
和网络波动影响，不把它宣称为稳定性能收益。

Trace 的旧 oracle 仍只因缺固定中文词形 `等待对象 dma_fence_default_w` 判负；相同 typed 内核调用点
已出现在模型正文、根因席位和系统投影中，继续归 `EVAL-B46-ORACLE1`，生产代码不拟合词形。能力面
没有回归：233.190ms 显式用户窗被保留，模型查询了 window stats、root-cause rank、wakeup chain、
critical blocking calls，系统在成文前补采 frame root-cause bundle；最终仍有实际占用/潜在新修向与
现规则可消两维、根因排序、唤醒链、窗内可消除量和完整 Trace 因果投影。

人工仍判 Trace FAIL，原因有两层：

1. 模型开头宣称“全部主要等待与占用来源已完整覆盖”，但 deterministic appendix 明确给出
   `enumeration_status=incomplete`，并列出 `critical_blocking_calls/root_cause_rank/span_window` 的
   emitted/total capacity boundaries。不能把 capped samples 写成全部。
2. 模型把根因排序的 overlap-folded 有效影响 `3.670ms` 写成“6 段有效归因”，又列出六条 raw
   occurrence；模型前的精确 target-blocking authority 已写明 `>=6 occurrences / union wall clock
   >=4.611ms / lower_bound_capacity_truncated`。这证明第二层当前是模型没有服从足量 typed context，
   先留作 `EVAL-B50-H5IO1/P2-model-variance-watch`，不得扫描答案词面、替换模型结论或按该 trace
   类型写专门门。

上下文审计同时找到一个系统自身矛盾：runtime-only optional-source snapshot 精确写着
`current_source_satisfied=false / current_source_records=0`，renderer 却因为先匹配
`CanCompleteWithCombinedProof` 而提示“runtime and current-source proof are both present”。登记并修复
`EVAL-B50-CTXAUTH1/P1-small`：combined 文案必须同时满足 `CurrentSourceSatisfied`；runtime-only 与
soft-caveat 分支明确只授权 runtime artifact claim，不虚构 source citation。修复只消费 typed boolean，
不读取 RawRequest、模型 thinking/summary/final，不拒绝、删除、替换或代写回答；显式窗 Trace 路由、
查询、自动补齐与最终投影代码零改动。`go test ./internal/agent -count=1` 通过（3.058s）。

`EVAL-B50-ENUMCTX1/P1` 冷读后确认：精确 `ToolResult.EnumerationAuthority` 随每次 trace_query 和
runtime-artifact paged read 存在，但 trace producer rows 没有把 result-level authority 复制到每条
ObservationRecord；AnswerDocument 模型前因此只有零散 compaction 摘要。回答后的 deterministic
appendix 才独立遍历 ToolResults，发布完整的 status/scope/dimension/emitted/total。模型写“完整覆盖”
时并未拿到与 appendix 同等的 concise 权限面，所以该层不是纯模型波动。

最优修复落在 result-level 单一编译器，而不是把同一边界复制到每条 observation：

1. `types.BuildRuntimeArtifactEnumerationAuthority` 统一消费 deterministic trace_query 与带
   `RuntimeArtifactRead` 身份的 read_file；普通源码分页不进入。只接受精确 status=`incomplete`，
   对完整 boundary 去重并按 scope/dimension/value 稳定排序。
2. AnswerDocument 在 Trace Decision Inputs 后、较大的 Observation Ledger 前发布最多 16 条精确边界；
   超出时给 omitted count 且保持 incomplete。每条保留 TotalKnown，未知总数不伪装 0。
3. 回答后的 Trace coverage appendix 改用同一编译器，删除原来的第二份 in-scope/dedupe/sort 判定；
   模型前与模型后不再可能因消费名单或排序漂移而给出不同枚举权限。
4. 该面只提示 emitted rows 是 bounded sample/lower bound，并要求结论保持在权限内；明确保留模型对
   diagnosis、priority、summary、recommendation 的所有权。没有读取 RawRequest、thinking/summary/final，
   没有关键词 hard gate、答案拒绝或系统改写，也没有生成产品结论。

定向正反测试覆盖 trace incomplete、runtime artifact read、重复 boundary、run-suffix producer、普通
source read 排除、complete 静默，以及模型前 section 顺序。完整相关回归：types 19.195s、tool
165.223s；agent 首轮被 glossary lint 捕获内部措辞，改成面向模型的所有权句后完整包通过（2.731s）。

任务状态：

- [x] B50-T1：严格并行 operation + 显式窗 Trace 恰好 2 case；
- [x] B50-T2：人工读完命令/Trace 查询链、模型前上下文、答案与系统 appendix；
- [x] B50-T3：确认 HTMLBODY1 live covered、Trace 能力无回归、旧词形 oracle 仍为 eval debt；
- [x] B50-T4：修复并回归 CTXAUTH1；
- [x] B50-T5：确认 result-level Enumeration authority 未进入模型前 concise context；以共享 typed
  compiler 接通 AnswerDocument 和 deterministic appendix，相关完整包回归通过；
- [ ] B50-T6：从优先队列选择下一对异构 eval，继续严格并行 2 个并人工审计。

#### B50a：合入审计 P0 复核与 EventSearchCoverage 权限闭合

远端 `main@e479df784` 新增 `colleague_merge_audit_20260802.md` 后，本席先复跑而非直接采信：
`go test ./internal/tracediag -count=1` 确定复现 schema pin 与柏林量级时间坐标两类红测，后续 eval
不能在红基线上继续。冷读同时确认审计归因需要细化：四个 schema drift 不是都由
`EventSearchCoverage` 引起；`Result` 是该字段，`WindowStats` 是 CPU frequency sample 与
clock-set-rate 两个独立计数，`RootCauseRankItem/WakeupCausalImpact` 是共享 exact scheduler-segment
inventory 的 `StateAccountKey`。

本批逐字段裁定并修复：

1. `EventSearchCoverage` 的九个字段不再进入低优先级 reflective detail；event-search 保证可见的
   accounting header 现在一次发布 scope kind、scope/matched 时间包络、物理时间行报告状态、
   scope complete、matched total、engine emitted 与 enumeration complete。所有秒坐标统一走
   `formatSecondsToken`，不再出现 `6.7932221e+06` 或精度损失，也没有第二份冲突 ruler。
2. indexed event-search 的 scope authority 改为从选中域独立计算，不受 event type/pattern/thread
   filters 缩窄。line bounds 存在时保持既有引擎语义：line 优先、time 不参与过滤；scope 包络取
   line 域实际事件的 min/max，不再冒用全索引 `FirstTs..LastTs`。time-only 与 windowed index 同样
   返回实际选中域包络；artifact 全量查询保持原行为。
3. indexed lane 的 `ScopeTimestampRows==0` 按既有合同不把 parsed events 冒充 raw physical-row
   census；报告明确写 `zero_or_not_reported`，避免把不可区分零值伪装成实测 0。streaming lane 的
   正数物理行 census 原样发布。
4. 其余三个 pin 分别记录真实字段语义后重钉：frequency 两计数是互不替代的 scalar census；
   `StateAccountKey` 是跨视图 exact identity。它们保留 generic scalar detail，不获得排序、因果或
   完整性权限，也不建立重复显示面。

回归新增柏林量级固定小数、coverage 单一必达面、line+time 冲突时 line 权限、选中域不逃逸全索引
四类 witness。`go test ./internal/tracequery ./internal/tracediag -count=1` 全通过
（tracequery 67.248s，tracediag 5.431s）；`go test ./... -count=1` 全仓通过。本批只修 trace query 的 event-search 范围账和零 LLM
诊断报告；没有触及显式时间窗根因查询、Trace 因果投影、自动补采/补齐、模型 answer blocks 或
写模式。

任务状态：

- [x] B50a-T1：复现并确认远端合入审计的 tracediag 双红；
- [x] B50a-T2：逐字段完成 key-first 裁定，拒绝盲重钉；
- [x] B50a-T3：修复 fixed-point coverage 与 T8-1 line-window scope 过界；
- [x] B50a-T4：tracequery/tracediag 完整包回归；
- [x] B50a-T5：全仓回归通过，独立提交推送绿色基线后回到 B50-T5。

### B51：Java 调用链 × data 单值结果；pre-emit 系统越权覆盖模型结论（2026-08-02）

按未饱和维度从干净 `main@e2a745448` 严格并行 2 个 case：

- `sr_java_call_chain`：139s，runner PASS / human FAIL；对应可见工件
  `.codrax/output/20260802-193009.078-37048.html`；
- `data_basic_sum_with_rules`：35s，runner/human PASS。

data lane 在一个批次内消费 `rules.md + orders.csv`，typed 材料覆盖完整，结果为单行
`17.0`，无 repair、无源码读取、无系统结论补写。该规则只是“逐行求 amount 之和”，因此
`contributions/reconcile` 保持 optional 是准确合同，不登记产品 GAP。

Java case 的 runner 绿是严重 false green。模型第一个结构化草稿已经给出五个可见成员和五个
匹配 citation；随后系统 pre-emit 链连续发生三步破坏：

1. analyzer 合法地只保留逐字来自请求的 `exact_target=VisitController.create`，并丢弃预扫描后
   才知道的 `AuditLog.record/VisitService.schedule/...`。但
   `CompileRequiredMechanismAnchors` 又从可见身份义务得到恰好两个 anchor：
   `VisitController`、`VisitController.create`。这两个是 owner/type 与 member 身份，不是
   source/sink 角色。
2. `compileCallChainReachability` 仅凭“anchor 数量恰为 2”就把两者当 source/target。BFS 当然无法
   从 class definition 到 method definition，于是 `normalizeCallChainReachabilityAuthority`
   删除模型 summary，并把五项 principal ordered list 覆盖为两行“未证明可达”。日志中的五条
   精确 call edge 实际已经证明
   `VisitController.create -> VisitService.schedule -> VisitRepository.insert -> AuditLog.record`；
   系统结论与自身 typed evidence 相反。
3. 同一 pre-emit 链更早已经把模型探索期 `member_set(5)` 铸成 hard principal enumeration。
   reachability normalizer 后置删除其中三行后，member-set hard check 又要求补回三行；模型每次
   patch 都补齐，normalizer 每次再删，形成三次完全相同 reject，最终降级泄漏模型 thinking。

这不是“模型没遵守系统”。是两个系统 authority 互相打架，并且系统直接替换模型结论。责任提交
为 `8cfef5158 fix: compile typed call chain reachability`：该批把 visible-anchor identity 当 endpoint
role，并明确在 unproven 时覆盖模型 summary/path。它落在 B26-OWN 当时仅保护 runtime-trace
persist choke point 之外，因此既有“非 system block wire 不变”测试没有捕获跨 family 的同类复发。

模型自身仍有两个独立质量问题：把 `countOpenVisits` 侧调用平铺成主路径第 3 跳，以及
“条件为真拒绝、超限则放行”的自相矛盾。最终 prompt 已列出五条逐边 grounded edge 和 guard
原式，信息精确且足够；这两项先记 model-variance，不增加用户/答案词面 gate。系统不得因为模型
可能犯错而获得代写结论权限。

新增台账：

| ID | P | GAP | 泛化方案 | 状态 |
|---|---:|---|---|---|
| `EVAL-B51-OWN1` | P0 | pre-emit normalizer 删除模型 summary/path 并写入系统 reachability 结论，复发 B26 所禁止的系统替答 | 从 production normalization 链撤销结论替换；typed call-edge 图只进模型前 relation authority 与结构化 edge 校验。系统可拒绝无证 structured edge，但不得改写模型结论 | covered；B51-A |
| `EVAL-B51-ENDROLE1` | P0 | `RequiredMechanismAnchors` 的 identity 集合被当成 source/sink role；“恰好两个”不构成端点权限 | reachability 必须等待独立 typed source/sink role carrier；在该 carrier 缺席时 fail-open，不从 visible anchors、顺序、owner/member 或 prose 猜端点 | covered；B51-A |
| `EVAL-B51-SETAXIS1` | P1 | `is_relational_lookup` 单独把 narrative call chain 提升为 exact operation-site member set，探索模型的路径解释被硬化成 exhaustive roster | source-operation-site set 权限必须由 enumerate/category/per-member/completeness 等 typed set boundary 授予；relation-only call chain 继续使用 ordered path/support lane，不生成 hard enumeration rows | covered；B51-A |
| `EVAL-B51-MUTLOOP1` | P1 | 两个 deterministic normalizer 对同一 principal block 一删一补，导致不可自愈 reject loop | 撤销 OWN1 后增加生产接线 pin：模型五行 wire 保持、无 principal-enum hard roster、同 payload 一次通过；任何系统 repair 不得制造后续 hard obligation | covered；B51-A |
| `EVAL-B51-ORACLE1` | P1-eval | broad answer regex 虽已排除分隔线前的进度草稿，但仍让终态 citation footer 与降级 raw-model appendix 重复的符号满足断言；错误主结论仍 PASS | 新增 opt-in `primary_answer` oracle，只消费终态主答案体，按稳定 renderer 边界排除 citation/recovery/Trace supplements；不静默改变 200+ 历史 case，逐案显式迁移 | covered；B51-B r2 真实 replay 已由主答案六步路径满足 |
| `EVAL-B51-AMPSET1` | P1 | analyzer 已明确 `trace/call_chain/call/category=false`，R1 仍仅凭多个参与实体放大为 category enumeration，进而强制探索交付 member_set 并将多轮集合并成重复清册 | R1 将 typed call-chain 始终视为有序路径，不依赖可选且波动的 exact-target/relational；真正的集合由 enumerate/category/kind=enumeration 承载。不从请求或答案文本推断 | covered；B51-C3 r5 真实 replay |

施工顺序冻结：B51-A 先撤销 call-chain 结论替换并收窄 relation-only set 权限；相关 types/tool 全包
通过后独立提交推送。B51-B 再修 eval final-answer oracle。完成本节审计和 B51-A 交付后，才继续
`colleague_merge_audit_20260802.md` 的 T3-1/T3-2 与中危清单。显式时间窗 Trace 的因果投影、
根因排序、唤醒链、窗内可消除量、双轴占用与自动补采不经过源码 QFCallChain pre-emit 路径，
本批不得改动。

B51-A 实现记录：删除 `8cfef5158` 引入的 call-chain reachability 结论重写器及其“覆盖模型答案”
测试，production pre-emit 不再从 `RequiredMechanismAnchors` 猜 source/sink；新增接线 pin 保证两个
identity anchor、五成员探索 ledger 同时存在时，模型 summary/path 字节保持且不会生成 missing-row
或 relation-table 硬义务。聚合权限同时收窄：`is_relational_lookup` 不再单独构成 operation-site set
boundary；无 enumerate/category/per-member/completeness 权限的 relation-only `member_set` 作为模型
叙述上下文保留，但不进入 deterministic visible rows / hard principal roster。显式 enumerate 和
per-member table 正臂均保留。未读取用户原文或模型输出词面，未改 Trace runtime family。

验证：定向 types/tool pin 通过；`go test ./internal/types ./internal/tool -count=1`、
`go test ./internal/agent ./internal/orchestrator -count=1` 通过。B51-B 的终态答案 oracle 与回放仍为
独立下一批，不能用本批单元测试代替真实 replay。

B51-B 冷读纠正：`scope_stdout` 已从首个 `━━━` 后取值，所以“中间第一稿直接刷绿”不是精确
机制；旧终态主答案只含 `VisitController/VisitController.create` 与容量说明，但 renderer citation
footer 和降级 `模型最后一轮原文` 又重复了 `VisitRepository.insert/AuditLog.record`，宽域
`EXPECT_MATCHES_REGEX` 因而误绿。新增 `EXPECT_PRIMARY_*` 四个 opt-in 通道：它们仍只审计最终产品，
并在稳定的 `引用/Citations`、raw recovery、Trace projection 边界停止；不扫描用户问题或猜模型
语义。runner 正/负 fixture 已证明进度、citation、raw fallback 均不能满足 primary oracle；原有
full/principal/write oracle 语义保持。`sr_java_call_chain` 已迁移到 primary regex，旧失败工件在该
域只剩 Controller 与 Service，Repository/Audit 两项确定性缺席，因此会正确判 FAIL。

B51 r2 使用已推送的 B51-A/B 严格并行回放 Java call-chain + data 单值。两个 runner
均 PASS；data 人工 PASS，Java 人工 FAIL。正向收益是 Java 主答案保留了模型六步路径，
没有再被系统换成两行“未证明”；primary oracle 也确实由主答案满足，不是引用/
raw recovery 偷渡。但页面又出现系统附加的 3 行与 14 行“完整调用链操作节点”，
并伴随 8 次 finalizer reject。这不是 B51-A 下游权限函数失效，而是其上游 R1 在
amplifier 阶段将 analyzer 明确的 `category=false` 补成 true：`relational=true` 使旧结构链 guard
直接退出，而 relation query 的 `ExactResolutionTargets` 又为 nil，最终 9 个 context entity 被
错当成主集合信号。

B51-C 在 amplifier 铸点根修：relational call-chain 同时具有 `intent=trace / axis=call /
kind=call_chain / exact_targets>=1`，且没有 per-member、completeness、declared enumeration boundary 或
多 bucket 时，它是一条有序路径，R1 不得仅凭参与实体数量创建 category 权限。
反向 pin 保留显式 per-member 边界时的 enumeration 放大。判定只读 typed IR，不扫描用户原文、
模型 thinking/summary/final，不按 Java/类名/case ID 适配。

B51 r3 中 Java 降至 93s，explorer 4→1、read 18→5、finalizer reject 8→0，模型主答案给出
正确四层路径和容量检查位置，不再有系统追加的 3/14 行重复清册。但冷读 prompt 发现
同轮 analyzer 合法地把 `is_relational_lookup` 设为 false，R1 仍因 exact target 在
`ExactResolutionTargets` 路径中不可用而放大 category；只是本轮 member_set 刚好与四个路径节点对齐，
没有再显式破坏终稿。因此 C1 仍是对 relational 布尔的过拟合，C2 将稳定边界收敛为
`trace+call_chain+call+exact_endpoint+no_set_boundary`，并新增 relational=true/false 双臂 pin。

r3 的 data 异构对照另行失败：实际 `reconcile expected=17/actual=17/pass`，但源规则覆盖与
decision/contribution 之间没有 `rule_id` 关联，终验 fail-closed。模型后续用 custom_transform 修账时又
调用了受限环境未提供的 Python `type`，累计 10 data rounds/6 repair rounds 后失败。这与 B51
调用链权限无关，登记 `EVAL-B51-DATAREF1/P1/open`：先复查既有 typed 规则关联修复动作与受限
script builtin 合同，排在 MERGE-AUDIT-3 三个 high 之后，不用放宽 provenance 终验换取通过。

B51 r4 选用 Java call-chain + `called-by` 真关系集合做正反例，runner 2/2 PASS。
`called-by` 人工 PASS：结果只列 2 个真实 production caller，无 reject，证明集合正臂未被压掉。
Java 主答案的 5 步路径大体正确，但继续出现 2+5 行系统清册；冷读 analyzer 发现本轮
`exact_targets` 合法缺席，使 C2 仍失效。这证明 `exact_target>=1` 也是不应支配家族的可选信号。
旧看护 `TestR1_FiresOnTraceEnumerationWithoutEndpointTargets` 用 `kind=call_chain` 强制放大本身就过硬：
call-chain 的多实体是有序 hops，非 category members。C3 将正例改为 typed `kind=enumeration`，
并对 `trace+call_chain+call+category=false` 无条件跳过 R1。

r4 Java 同时确定性复现 MERGE-AUDIT-3 `T3-2`：模型的 sequence diagram 经历 10 次
pre-emit reject/5 次 patch，最终删除图才通过，耗时 370s。该 witness 与集合权限分属两条独立责任链，
B51 只固化“不再产生主集合”；图边合同按用户指定在 MERGE-AUDIT-3 首批继续修复。

B51 r5 严格并行同一正反例，runner 2/2 PASS。冷读 Java explorer 完整 prompt 确认三个错误
集合指令（Structured Aggregate / Source Operation Site / Attribute-bearing Enumeration）全部消失，
终稿也没有 deterministic member roster；`called-by` 仍精确列出 2 个 production caller。
因此 `EVAL-B51-OWN1/ENDROLE1/SETAXIS1/MUTLOOP1/ORACLE1/AMPSET1` 按系统权限目标全部收账。
Java 人工仍 FAIL 的原因已完全转入独立 T3-2：26 次 reject/11 次 patch 后图仍未通过，
降级稿移除了大部分条目引用；模型另有 `VISIT_CREATED/CREATE_VISIT` 与源码 `visit.insert`
不一致的词面波动。这两项都不再授权系统重写调用链结论。

- [x] B51-T1：严格并行 Java call-chain + data 恰好 2 case；
- [x] B51-T2：人工读完 HTML、完整 stdout/log、analyzer payload、五个源文件、首稿与降级稿；
- [x] B51-T3：确认 data 上下文/答案正确，Java runner false green；
- [x] B51-T4：定位 `8cfef5158` 的 endpoint-role 与结论替换责任链；
- [x] B51-T5：实现 OWN1/ENDROLE1/SETAXIS1/MUTLOOP1，相关全包回归后提交推送；
- [x] B51-T6：primary final-product oracle 已实现并通过 runner contract；r2 真实回放证明 Java 由主答案本体满足；
- [x] B51-T6b：C3 r5 确认 call-chain 三个错误集合 prompt 和 deterministic roster 消失，true relation enumeration 正臂保留；
- [x] B51-T7：B51 系统权限目标收账；进入 MERGE-AUDIT-3 高 ROI 批，首先 T3-1/T3-2。

### MERGE-AUDIT-3 H1：T3-1/T3-3 席位级帧因果权限（2026-08-02）

审计结论准确，且旧实现的影响面比头行更宽：会话级
`runtimeTraceCoverageAuthority.causalUnproven` 同时控制头行、图例、明细、对比表和下一步提示，
一次无关的探索空窗会把最终已证席位全页降级，并启用备案词族。

本批按用户 §7 裁定完成根修：

1. 构造 `runtimeTraceProjectionSeatAuthorityIndex`，按 observation ledger 编译器同一套
   origin 合法性、fallback ID、first-ID-wins 规则，把每个 `trace_query` 的 typed frame
   authority 绑定到它实际发布的 observation ID；不解析请求、tool summary、模型答案或 case 名。
2. 投影模型完成最终选举后，仅查询当选 lead 的 `EvidenceID + MergedEvidenceIDs`。只有该席位
   同一结果同时满足 `TypedCausalRowCount>0` 与 frame absent/unavailable 或 typed
   frame-flow unproven，才铸造 `FrameCausalityUnproven` 限定。无精确 ID 关联则不猜。
3. 会话级 `causalUnproven` 仍保留在覆盖边界，诚实披露“本轮曾有未证查询”；但它不再控制
   席位选举、加冕、对比列名或下一步标题。由此保留信息而不让探索噪声越权。
4. 加冕始终保持裁定形“主根因(=已证链上单项最大可消除量)”；席位级 frame authority
   只追加“帧因果未证”。备案的“首要可消除候选”中英文生产发射点全部退役。
5. 新建 `runtimeTraceProjCrownWords` 单一词源，统一头行、限定语、图例、因果位置、对比列名、
   下一步称谓；T3-3 同页三面分叉随 T3-1 一并关闭。

验收：

- `TestEarlierUnprovenProbeCannotDecrownLaterProvenSeat` 固定“先 unproven、后 proven”的
  会话反例，后者加冕字节与定义括注不变且不带无关帧限定；
- 中英文 seat-level 正例固定“链已证 + 帧因果未证 = 保冠 + 限定”，并对退役词族作输出负 pin；
- 模型原有 summary/list/timeline wire 保持不变，系统仅追加 sibling typed projection；
- `go test ./internal/tool -count=1` 通过（187.077s）。

状态：`implemented / full-tool-suite-pass / T3-2 next`。

### MERGE-AUDIT-3 H2：T3-2 sequence 回复边与 participant identity（2026-08-02）

同事关于 `-->>` 的审计结论准确；B51 r4/r5 真回放还证明了第二个同根阻塞面：即使删掉
reply，模型按教学使用 `participant C as VisitController` 等短 actor，typed call evidence
却是 `VisitController.create -> VisitService.schedule` 方法级身份，旧校验只允许端点逐字相等，
因此 5 条真实正向调用全部报 `call_edge_unproven`。仅豁免 reply 会假关账。

本批的通用修复：

1. 在 Mermaid parser 已发布的 `Edge.Operator` 上分类：仅 sequence 的 `-->>` 作为
   response/return presentation edge，不要求反向 `relation_kind=call` anchor，也不进入反向
   call evidence 门；`->>` invocation 与 call_dag 继续严格校验。
2. reply 不能隐藏模型显式声明的反向 call anchor：若模型仍给 `callee -> caller` 添加
   `relation_kind=call`，该 anchor 继续独立要求同向 typed call-site 证据。
3. method-qualified participant 仍走原 exact lane；class/actor 短标签只允许一个结构化扩臂：
   participant 必须逐字等于 typed Subject/Object 的 owner，message 首操作必须逐字等于同一
   EvidenceItem 的 `AnchorSymbol`（缺席时取 typed Object terminal operation），且匹配的
   `Subject + Object + operation` 候选恰好一个。空 label、`invoke` 泛词、错方法或多候选均
   fail-closed；没有 prefix/token-overlap/用户原文或答案 prose 扫描。
4. `BuildDiagramRelationContractDoc`、最终回答 skill 示例与 same-turn 修复提示同步说明
   invocation/reply 区别和唯一别名规则，消除“prompt 教 -->>，硬门拒 -->>”的自冲突。

测试覆盖 forward/reverse、reply 无 anchor、reply 伪装 reverse-call anchor、真实 Java
class participant + 同 owner 多方法、无精确 operation 的歧义负例、短 callee
`AnchorSymbol`、call_dag 严格性、Trace 因果图不进入本合同。

全相关回归：

- `go test ./internal/tool ./internal/skill -count=1` 通过
  （tool 175.833s，skill 0.508s）。

状态：`implemented / relevant-full-tests-pass / T7-1 next / same-pair replay after high-risk batches`。

### B42：生成物写模式 × 日志/源码机制对比审计（2026-08-02）

本批按跨模式优先级严格并行：

- `github_issue_napi_force_wasi_env_symptom`：真实上游生成模板写模式；
- `read_combo_log_current_source_explanation`：attached log 与 current source 的机制边界解释。

runner 2/2 PASS，人工 0/2 PASS，两个都是结构性 false green。

#### EVAL-B42-GENART1：producer token 验证掩盖生成物未定义变量（P0）

写模式生成的补丁为：

```typescript
export function renderNativeBinding(localName: string): string {
  const forceWasi = process.env.NAPI_RS_FORCE_WASI === 'true' ||
    process.env.NAPI_RS_FORCE_WASI === 'error'
  return `
  // ... generated loader ...
  if (!nativeBinding || forceWasi) {
```

布尔语义接近上游 PR #3236，但 declaration 在 generator 运行时作用域，reference 在
返回的 JavaScript 字符串里。最终生成 loader 没有 `forceWasi` 声明，运行即引用未定义
变量。verify 仍判通过，原因是 `tests/check_force_wasi.py`、`make check` 与现有 TypeScript
测试都只在 producer source 上扫描/替换 token，没有渲染后解析、名称解析或执行。

这不是 TypeScript 或某个变量名特例，而是 generator/template/transpiler/serializer/code
emitter 的共同验证边界 gap。第一施工批：

1. change-plan 共享 soft guidance 要求验证 produced artifact；生成代码使用的 declaration、
   guard、import/reference 必须位于生成物自身 lexical/runtime scope 且先定义后使用；
2. 优先 render/build 后 parse/import/compile/execute；缺少原生 runtime 时允许确定性的
   generated-output parser/scope check，但必须披露较窄边界；
3. fixture oracle 改为先提取实际返回的 loader 模板，再在该生成物内检查 force 条件、
   declaration 与 emission order；producer 外层同名 token 不再构成通过证据；
4. 修正 eval ground truth：`false` 是“不强制 WASI、native 优先”，不是 native 缺失时
   禁止正常 fallback；`true/error` 保持强制语义。

新 oracle 已证明：原始 fixture 以裸 truthiness 失败；B42 r1 应用树以生成物缺少
`true` 判定和 `forceWasi` 未声明失败。产品不增加 case/标识符 hard gate。

#### EVAL-B42-CONTRAST1：把机制 A 的 complement 当成机制 B 的实现（P1）

读模式正确判断 attached log 只证明 `phase=llm_request` 的 first-byte timeout，不能证明
后续 answer contract validation 的结果；但对代码机制给出错误结论：

- `IsStreamLevelRetryable=true` 确实承载 stream/transport retry；
- 它的 false 分支包含 HTTP API error、重试耗尽、auth/schema/config 等多种非流错误，
  并不等于 content validation failure；
- content contract failure 的 check/reject/retry-budget/scheduler requeue 位于独立控制路径，
  本轮没有读取；
- explorer 仅读超时 classifier 后，把 false/complement 臆断成第二套机制，并将该臆断
  铸入 evidence aggregate，finalizer 只能忠实扩写错误前提。

第一施工批增加 explore 与 answer-document 共用的 typed-mechanism soft directive：任何
机制 distinction/comparison 必须把每一侧分别追到自己的 producer/control path，并证明
join/handoff；A 的 predicate false/complement 不得充当 B 的证据，缺一侧就明确边界。
它只由 `RequiresMechanism` typed shape 选择，不扫描 RawRequest、thinking、summary/final，
不拒绝或重写模型结论。

任务：

- [x] B42-T1：严格并行 2 case，读取完整日志、最终答案、计划、应用 diff 与验证报告；
- [x] B42-T2：登记 GENART1/P0 与 CONTRAST1/P1，确认均为跨项目机制类问题；
- [x] B42-T3：共享 independent-mechanism contrast soft guidance，explore/finalize 同源且
  typed mechanism gated；
- [x] B42-T4：生成物验证 soft guidance + fixture generated-output scope oracle；
- [x] B42-T5：`internal/skill` 0.478s、`make`、Python 编译与 oracle 负证验证通过；
  原始 fixture 与 r1 应用树均被拒绝，第一施工批提交推送；
- [x] B42-T6：干净 `main@409d8452a` 同对严格并行 r2；写模式 runner/human PASS，
  `GENART1=covered`；读模式 runner/human FAIL，`CONTRAST1=partial`；
- [x] B42-T7：r2 深审登记 `EVAL-B42-MECHCARRIER1/P1`：模型虽然采用“两条正交路径”
  措辞，却以 timeout 单侧两锚构造双机制 grouped_count，第二机制没有独立 source member/
  support_ref，且误借 write-mode budget；
- [x] B42-T8：第二施工批复用 member_set 的 index-aligned member/member_note/support_ref
  作为机制对比载体；explore/finalize shared directive 与 completion schema 同步教学；
  eval regex 拆除“日志词与源码词必须同一行”的显示形过硬条件；
- [x] B42-T9：`internal/skill` 0.698s、tool schema 定向 0.892s、
  `internal/tool` 全包 159.367s 与 `make` 通过；第二施工批提交推送；
- [ ] B42-T10：干净 HEAD 做 r3 同对回放；若结构化软引导已给足但模型仍波动，
  不增加硬门，转后续
  高优先级跨模式 pair。

#### B42 r3：载体声明、运行时规则实例与 eval 终态真值（2026-08-02）

干净 `main@13e504315` 严格并行同一对 case，runner 2/2 PASS，人工 0/2 PASS。
第二施工批的结构化 carrier 已生效：read explorer 发出了两个成员、两个 note、两个
support ref 的 principal `member_set`，因此 `EVAL-B42-MECHCARRIER1=covered`；但深读证明
两个更底层的证据权限 gap：

1. `EVAL-B42-PRODUCERROLE1/P1`：第二机制的 support ref 只是
   `NoticeAnswerCheckRetry` enum 声明。它能证明事件名字存在，却不能证明谁 emit、什么
   条件触发、谁消费以及如何 requeue；模型仍据此宣称
   `answerDocumentEvaluator -> NoticeAnswerCheckRetry` 完整行为路径。
2. `EVAL-B42-RULEINSTANCE1/P1`：模型读取 `first_byte_floor_warn.go` 的 soft warning
   predicate 后，把它说成本次 40s timeout 的根因；但该 rule 还要求
   `reasoningFamilyModelName(model)`，attached log 的 model=`demo` 不满足。源码规则存在不等于
   本次 runtime instance 命中，诊断/advisory predicate 也不等于 enforcement path。

write case 的生成 loader 生产补丁已经正确，但新增 TypeScript 测试只检查生成源码是否包含
`wasiBinding = require`。该语句对所有 env 分支都存在，因此 false/0/undefined 的
`doesNotMatch` 在 Node 可用时必败。本机 Node/npm 不可用，旧 Python fallback 只检查 token，
局部 `post_apply_verify` 仍 passed；产品的 `WriteFinalReport` 已诚实给出
`unverified/verification_proof_incomplete`，eval runner 却只消费局部 report 而误报 PASS。

新增/归并 GAP：

| ID | 优先级 | 泛化方案 | 状态 |
|---|---|---|---|
| `EVAL-B42-PRODUCERROLE1` | P1 | mechanism 对比 soft guidance 明确：enum/constant/type/schema/event-name declaration 只具 identity 权限；行为必须打开 actual producer/callsite 与 consumer/handler branch | implemented/tests-pass/replay-next |
| `EVAL-B42-RULEINSTANCE1` | P1 | typed log+mechanism soft guidance 要求从 runtime artifact 绑定 predicate 的全部 load-bearing operands；缺失/矛盾时只能报 possible/uninstantiated advisory，不能铸 runtime cause | implemented/tests-pass/replay-next |
| `EVAL-B42-WRITETEST1` | P0 | fixture 回归测试渲染 generated loader、提取并实际执行 branch condition，在 native-present 前提验证 true/error 与 false/0/empty/undefined；fallback oracle 要求同一行为结构 | implemented/negative-proof-pass/replay-next |
| `EVAL-B42-EVALSTATUS1` | P0 | 并入 B20 typed truth 闭环：apply eval 同时要求 current-plan ChangeReport passed 与 same-plan typed final report `run_status=complete, completion.verdict=verified`；最终 unverified/failed/missing 均 fail loud | implemented/runner-tests-pass/replay-next |

本批没有给行为结论加 hard gate，也没有扫描 RawRequest、模型 thinking、final prose 或 case
关键词。`aggregate_facts` schema 只同步证据载体语义；最终结论仍由模型形成。eval verdict
消费的是 `ChangeReport`/`WriteFinalReport` typed fields，不从“未完全验证”等显示文本反推。
未修改 Trace resolver、显式时间窗、因果投影、自动补齐、根因排序、唤醒链或窗内可消除量。

任务状态：

- [x] B42-T10：r3 回放与完整人工审计；
- [x] B42-T11：producer-role 与 runtime-rule-instantiation shared soft guidance，explore/
  answer-document 同源、typed filter 选用；
- [x] B42-T12：generated-loader 行为测试与 fallback oracle 加固；原始 buggy fixture、r3
  无效测试应用树均被拒；
- [x] B42-T13：eval apply 终态 typed truth 接线与正/负 runner contract；
- [x] B42-T14：提交推送第三施工批，在干净 HEAD 做 r4 严格并行复放并人工验收。

#### B42 r4：上下文路由不得覆盖模型 typed 意图；路径身份不得冒充符号（2026-08-02）

干净 `main@083095d28` 严格并行同一对 case。runner 为 1/2 PASS：read 的浅
`answer_regex` PASS、write 因 same-plan typed final report
`unverified/verification_proof_incomplete` 正确 FAIL。人工审计为 read FAIL、write 代码正确
但验证边界未闭合。

read 深审结论分成两层：

1. 三轮 analyzer 原始 typed 输出均是
   `intent=explain / scenario=architecture_explain / question_kind=mechanism`，系统因同一 payload
   的 `is_diagnostic_question=true` 每轮都静默改写成 `root_cause/root_cause`；最终 family 变成
   `root_cause_trace`，explorer 扩至 8 轮、上下文峰值约 65k/200k（33%），并注入根因 facets、
   diagram 与枚举合同。对“当前源码如何区分两个机制、附件日志能证明到哪里”而言，这些上下文
   不够精准，还诱导模型拼出单一因果链。
2. producer-role 与 runtime-rule-instantiation 软指令已经准确到达 explore/finalize，7 次源码
   读取也足够继续打开真实 validation check/requeue 路径；模型仍把 timeout classifier 的
   complement 当第二机制，并把不满足 `model=demo` operand 的 soft warning rule 实例化为
   本次原因。该残余记为 model-variance watch，不再增加 hard gate、答案替换或原文扫描。

登记 `EVAL-B42-CONTEXTROUTE1/P1`：typed 分类矛盾时，系统只能 canonicalize enum spelling
或 fail-loud 让 analyzer 重试，不能把一个答案语义静默替换成另一个。施工删除
`normalizeDiagnosticRoute` 的 intent/scenario 改写，保留现有 typed 自洽校验；
`is_diagnostic_question=true + explain/architecture` 现在拒绝且不持久化 RequestModel，
已对齐的 root-cause 与 performance diagnostic 继续通过。此修改不读取 RawRequest、模型
thinking/summary/final，也不碰 Trace resolver。显式时间窗仍由
`RuntimeTraceReportShapeAuthority` 最高优先授权完整因果投影/自动补齐。

write 深审确认生产 patch 与既有生成 loader 行为测试均正确；Node runner 缺失使最终
`unverified` 是诚实边界，runner fail 不应放宽。但出现独立的
`EVAL-B42-PROOFIDENT1/P1`：计划把
`./cli/src/api/templates/js-binding` 这个 extensionless module import 填入
`changed_symbol_refs`，后续无条件铸成 `changed_symbol` obligation；实际变更身份是
`cli/src/api/templates/js-binding.ts` 文件，因而制造
`changed_symbol_without_probe_coverage / source_localization_weak /
impact_targets_unverified` 假缺口。

通用施工方案：

1. 保持 wire 字段兼容；语言级名字（如 `Axis.convert`）仍是 symbol，显式文件用
   `path:<repo-relative-file>`；
2. unprefixed module/path 只在对 `changes[].path`（含 extensionless、working-dir-relative、
   index module）得到唯一匹配时提升为 `path:`；歧义和未匹配值原样保留，禁止猜测；
3. `ImpactObligationSetFromChangePlan` 将 `path:` 铸成 changed-file/path-ref obligation，
   不再进入 symbol lane；
4. schema、planner soft guidance、skeleton 与 verify schema 使用同一身份语义。

定向测试覆盖 unique module、working-dir-relative、index module、真实 symbol、显式 path、
ambiguous module、canonical plan factory 接线，以及 impact obligation file/symbol 分流。
冷读曾发现 helper 首版接在 probes 赋值之前、只有 helper 单测为绿；已将铸点移到 plan
组装完成后，并覆盖 contract enrich 与 no-change proof-followup 两个后续入口，避免再次出现
“算法存在但 production 未消费”。Node/runtime runner 缺失仍保留 unavailable，不通过身份
归一化借文件覆盖伪造行为验证。

验证：`internal/types` 22.452s、`internal/skill` 2.485s、`internal/analysis/...`
全通过；`internal/tool` 首轮 157.978s、production 接线冷读修正后复跑 159.009s，
`internal/orchestrator` 11.178s、`eval/runner_lib_test.sh` 与 `make` 通过。

状态：`CONTEXTROUTE1=implemented/full-tests-pass`；
`PROOFIDENT1=implemented/integration-test-pass/full-tests-pass`；
`PRODUCERROLE1/RULEINSTANCE1=model-variance-watch`；
`WRITETEST1/EVALSTATUS1=covered`。

任务：

- [x] B42-T15：r4 完整日志、答案、应用树、ChangeReport/WriteFinalReport 与系统上下文人工审计；
- [x] B42-T16：删除 diagnostic typed 冲突的语义 auto-rewrite，改为自洽失败重试；
- [x] B42-T17：verification probe module/path identity 唯一归一化与 impact obligation 分流；
- [x] B42-T18：types/skill/analysis/tool/orchestrator、runner contracts 与 build 完整相关回归；
- [ ] B42-T19：接线冷读修正后 tool 全包已通过；提交推送，在干净 HEAD 做 r5 同对双并行验收；
- [ ] B42-T20：r5 收口后转下一组更高优先级跨模式 pair，不继续围绕模型波动做单 case 加固。

#### B42 r5：semantic rewrite 已消失；修复上下文仍有单向偏置（2026-08-02）

干净 `main@9e3efb4d5` 同对严格并行。runner 仍为 read PASS、write 因 typed final
`unverified/verification_proof_incomplete` FAIL；人工为 read FAIL、write 代码正确但行为验证
未闭合。

read 的系统上下文审计：

1. `CONTEXTROUTE1` 已覆盖：第一次、第二次
   `explain + diagnostic=true` payload 均 fail-loud，系统不再写回替换后的 RequestModel；
   第三次 `root_cause` 是 analyzer 自己重发的选择。
2. 新登记 `EVAL-B42-ROUTEREPAIR1/P1`：现有 reject 文本只说
   `is_diagnostic_question=true requires intent=root_cause`，等价于只给一条修复方向。对当前这种
   `explain/architecture/mechanism + attached-log conclusion boundary`，另一个合法方向是保留
   explain/mechanism 并清除 diagnostic/current-risk flags。系统虽不直接改字段，修复上下文仍
   单向推动 root-cause family、8 轮探索与约 68k/200k 上下文。
3. 最终答案仍把 `IsStreamLevelRetryable=false` complement 当 content validation，把
   `canUseFinalizerOutputAfterTransientProgress` 的 degraded fallback flags 当校验 verdict，
   又把 render status 分类说成触发重试。真实边界是：dispatch error 在 finalize stage request
   返回错误时处理；成文合同校验只在成功获得 StageOutput 后进入 `runContractCheck`，再由 typed
   violations/retry/requeue 处理。软指导已准确、模型读取 8 个文件仍未闭合，继续增加答案 hard
   gate/替换属于过拟合，记 model-variance watch。

`ROUTEREPAIR1` 的通用施工只修改 typed 自洽失败反馈：同时列出两个合法修复方向——真正的
cause/remediation/current-risk 选择 root-cause/performance；普通机制、架构、实现或附件日志边界
解释则保留非 root-cause intent/scenario 并清除 diagnostic profile。系统不自动选择、不扫描
RawRequest 或模型答案。

write 的 `PROOFIDENT1` 已覆盖：canonical plan 中两个 probes 均携带
`path:cli/src/api/templates/js-binding.ts`，impact obligations 只有 changed-file/path-ref，四个文件
义务全部 covered，旧 `changed_symbol_without_probe_coverage` 消失。JS behavior probe 因 Node
runner 缺失未执行，两个 behavior contracts 仍真实 unverified；typed final fail 必须保留。

同时登记 `EVAL-B42-LOCALIZE1/P1`：final localization review 对
`cli/src/api/templates/js-binding.ts` 同时携带两个 `strength=owner` 的 plan-edit anchors，却仍给出
`plan_source_paths_missing_owner_context`。根因是 `LocalizationRequirementsFromWritePlanContext`
用新的 `batch-1-cumulative-review` 过滤 owner view，而 prior-path/anchor 附注走的是 workflow
unscoped 读取，于是同一 typed 事实一边展示、一边不授权。

通用修复：owner-localization proof 按 consumer + source identity 在同一 workflow 内持久，repair、
cumulative-review 或后续 slice 可复用前批对同一路径的 owner anchor；普通 verify failure、planner
prose 等仍继续按 batch scope 隔离。路径匹配保证其它文件的 owner proof 不能串用。回归直接构造
前批 owner anchor + 后批 cumulative review，要求 requirement satisfied、review supported、
missing/owner_missing 均为空。

状态：`CONTEXTROUTE1=covered`；`PROOFIDENT1=covered`；
`ROUTEREPAIR1=implemented/full-tests-pass`；
`LOCALIZE1=implemented/full-tests-pass`；read answer=`model-variance-watch`。

完整相关回归：`internal/types` 20.414s、`internal/loopkernel` 1.846s、
`internal/writeflow` 1.351s、`internal/tool` 158.809s、
`internal/orchestrator` 9.427s，`make` 通过；`git diff --check` 通过。
改动文件清册不含 Trace resolver、query、runtime authority 或 answer mutation；显式时间窗的
因果投影、根因排序、唤醒链、窗内可消除量与自动补齐均未改动。

任务：

- [x] B42-T19：r5 双并行回放，人工审计完整 read/write 日志、答案、plan/report/final 与 context；
- [x] B42-T20：typed analyzer 自洽 retry 提示改为对称二选一，不替模型改字段；
- [x] B42-T21：跨批 durable owner anchor 消费与 cumulative-review 直接回归；
- [x] B42-T22：完整相关回归并提交本批；用 r6 验证上下文，不以模型答案绿灯为施工目标；
- [x] B42-T23：r6 收口 `ROUTEREPAIR1/LOCALIZE1`，模型波动不加固；转下一组更高优先级 pair。

#### B42 r6：typed 上下文接线覆盖，停止单案硬化（2026-08-02）

干净 `main@fb49ccd9c` 同对严格并行：read runner PASS（228s），write 因 typed final
`unverified/verification_proof_incomplete` 正确 FAIL（179s）。

read 的 analyzer 第 2 次 payload 收到对称修复说明后，模型在第 3 次明确解释当前请求是机制与
日志边界说明，自行保留 `intent=explain`、改为 `scenario=architecture_explain` 并清除全部
diagnostic flags；第 4 次仅补齐 `bounded_fact_set.fact_families` 后成功。由此
`EVAL-B42-ROUTEREPAIR1=covered`，且日志确认系统没有语义替换。

最终答案仍错误地构造了一个统一 `errors.As` 分发器，把 first-byte timeout 放进只匹配
`StreamNoVisibleOutputTimeoutError` 的 fallback，并把 render 显示措辞当作 timeout 与
post-success validation 共用的控制信号。真实 answer validation 仍须在 finalize 成功返回后经
`runContractCheck`、typed violations 与 retry/requeue；本轮模型已收到 independent-mechanism、
producer-role、runtime-rule-instantiation 指导并读取 6 个源码文件，却没有打开该 join。这里没有
新的精确系统事实缺口，继续按最终措辞追加 hard gate、rewrite 或关键词路由属于单案拟合，状态
保持 `model-variance-watch`。

write final report 已变为 `localization_status=supported / owner_supported`，旧
`plan_source_paths_missing_owner_context` 消失，`EVAL-B42-LOCALIZE1=covered`。生产 patch 正确；
Node/npm 不在环境，JavaScript probe 未执行，模型本轮还显式声明了 `renderNativeBinding` symbol
与三个 behavior contracts，因此对应 unverified 是真实证明边界，不得以 file coverage 覆盖。

本轮 context audit 结论：读模式路由宽度与问题一致，系统提供的是 typed 事实/软指导而非结论；
写模式 localization 与 verification context 不再自相矛盾。未修改 Trace resolver/query/authority/
mutation，显式时间窗的因果投影、根因排序、唤醒链、窗内可消除量和自动补齐保持原状。

### B43：显式窗 Trace 双轴 × 跨语言写验证权威（2026-08-02）

优先级选择：一项覆盖用户反复强调的显式时间窗、Trace 因果投影、自动补齐与“两种根因
维度”；另一项覆盖高风险写模式终态验证。干净 `main@30a993a43` 严格并行 2 case，runner
均 PASS（Trace 158s、Write 391s），人工分别 partial / FAIL。

Trace 案中系统接线符合红线：显式 `5.000..5.007s` 用户窗未被窄问题路由吞掉，执行了两次
windowed `trace_query`，并同时发布：`VerifyClass` span 墙钟 5.000ms、CPU/规则可消除量
4.600ms、runnable 0.800ms、`worker-200 -> app-100 @ 5.005s` 唤醒边，以及
`frame_causality=unproven / frame_evidence_status=absent`。因果投影把首项称为“可消除候选，
不等于已证帧因果”，自动补采在场，系统未替换模型正文。模型仍写成“全程 CPU 运行”及
“阻塞主线程的根因”，是已有 `EVAL-B19-CAUSAL1` 的模型边界违背；typed context 已精确且
足量，本批不增加答案关键词 gate、系统 rewrite 或单案硬约束。

写案暴露两个可泛化的 authority gap：

| ID | P | gap | 最优方案 | 状态 |
|---|---:|---|---|---|
| EVAL-B43-METAAUTH1 | P0 | `cargo test` 缺失后，`make check` 只调用 Python 文本扫描；系统因脚本字面读取 Rust 文件，便把 Make 的 `DeclaredCoveragePaths` 铸成 Rust `project_runner`，最终错误签为 verified | declared-input roster 只保留为 dependency/audit context；硬 changed-path authority 必须由 runner language family 与目标 family 相交，跨语言 meta runner 即使成功且精确命中输入也不得升级 | implemented/tests-pass |
| EVAL-B43-PROBEBIND1 | P1 | probe 的 `contract_refs` 可在未执行、导入或绑定任何变更身份时覆盖行为合同；文本 oracle 因而可把“读到了文件”冒充“验证了行为” | 仅从实际 PASS 且由 typed `changed_symbol_refs + language family + active target path` 绑定变更身份的 probe 汇总 contract/placement refs；无绑定时记录 uncoupled/missing，不授予行为合同 | implemented/targeted-tests-pass |

`METAAUTH1` 明确 supersede `EVAL-B12-AG1` 的硬权限结论：B12 的 exact candidate +
declared-input roster 比任意 Make 命令更精确，但 B43 证明它仍只回答“脚本依赖了什么”，不能
回答“哪种语言的编译/运行语义被验证”。该 roster 仍可用于候选选择、日志说明和人工审计，
不能进入跨语言硬确权。修复只读 typed runner/language/path/outcome；不扫描用户输入、模型
thinking/summary/final，也不接管模型结论。

任务清单：

- [x] B43-T1：严格并行 2 case，人工读取 Trace 日志/答案与 Write plan、patch、report、final；
- [x] B43-T2：确认显式窗因果投影、自动补齐和双轴上下文完整，模型越界留作 variance watch；
- [x] B43-T3：撤销 declared-input roster 的跨语言 project-runner 硬权限，保留 typed audit context；
- [x] B43-T4：定向 changed-path 与真实 RunTests meta-runner 负例通过；
- [x] B43-T5：probe contract refs 与实际 PASS、变更身份、语言 family 建立 typed 合取；
- [ ] B43-T6：完整相关回归，分批提交推送；干净 HEAD 同对双并行回放并人工验收。

#### B43-b：probe 合同必须由通过且身份耦合的执行证明

第二施工批复用 changed-path authority 的同一解析器，避免合同账本另造一份判定：

1. 先由 `TestResult{suite=verification_probe/*, passed=true, assertion_id}` 得到实际通过的
   probe ID；未运行、失败或仅存在于 plan 的 probe 不参与任何 contract/placement/changed
   coverage。
2. `path:` identity 必须精确命中 active changed source path 且 probe language family 相交；
   bare symbol 只在兼容 source target 唯一时可归属，多文件同 family 继续要求 `path:`。
3. 只有上述 identity-coupled probe 的 `contract_refs` / `placement_refs` 可进入 satisfied
   集合。probe 自报合同、stdout 成功、代码中出现目标文本都不能单独确权。
4. 有 changed refs 但无法解析到兼容目标时，发布 typed
   `verification_probe_changed_symbol_uncoupled`；没有 refs 则保留
   `verification_probe_missing_changed_symbol_ref`。所引用行为合同保持 missing/unverified。
5. soft/fallback contract 与 placement contract 同样受 identity 合取，不再只有 hard
   contract 才要求 changed-symbol proof。

定向测试覆盖跨语言 Python→Rust path claim、未通过 probe 不能覆盖合同、同语言单目标
bare symbol 正例，以及原有 hard/soft/placement partial coverage。未读取 probe code、用户
原文、模型 thinking/summary/final；系统只决定证据权限，不生成或替换模型结论。

#### B43 r2：验证权威收口；进度生命周期仍冒充测试 verdict（2026-08-02）

干净 `main@3f96383f7` 同对严格并行：Trace runner PASS（113s），Write 按 typed final
正确 FAIL（644s，`write_final_verdict:unverified:verification_incomplete`）。

Trace 的显式用户窗、两次 windowed query、双轴、根因排序、唤醒链、完整因果投影与自动
补齐零回归；覆盖边界仍为 `frame_causality=unproven / frame_evidence_status=absent`。
模型继续把首要候选写成主根因，并把 `lower_priority_waker` 升级成已发生的优先级反转，
而 injected typed hint 已逐字说明该关系只证明低优先级依赖候选、没有 positive typed PI row
时不能声称 measured inversion。本项保持 `EVAL-B19-CAUSAL1/EVAL-B19-SCHEDPROSE1`
model-variance-watch，不增加关键词 gate、final rewrite 或系统替换。

Write 的权限目标已覆盖：本轮 Go probe 执行失败，Rust `cargo test` runner missing，Make
只运行脚本且成功；两个 Rust path 均为 `uncovered`，ChangeReport=`unavailable /
changed_path_verification_uncovered`，proof 的 7 个 impact target 全部 unverified，最终
`completion.verdict=unverified`。系统不再复现 r1 的 false green。patch 本身仍未经 Rust 编译：
`const fn try_milliseconds` 使用派生 `PartialOrd` 比较，且模型的边界推理仍有明显漂移，故
人工结果仍为 FAIL；这正是终态不得越权确证的原因，不是应由系统伪造成功的缺口。

新登记 `EVAL-B43-VSTAT1/P1`：每次 verifier agent 成功完成一次 `run_tests` 调用后，dock
节点均显示“✓ 已通过测试验证改动”，即使 typed ChangeReport 是 unavailable/unverified。
根因是 `verify` 的 done phrase 表达 agent lifecycle，却使用 test verdict 词形。通用修复是
把 done 改为 verdict-neutral 的“已完成测试验证 / Verification run complete”；真实
passed/failed/unavailable 继续只由 ChangeReport、workflow attempt 与 final report 发布。
不根据日志文本或模型答案决定措辞，也不改变验证门。

- [x] B43-T6：干净 HEAD 同对双并行回放、完整人工审计与 METAAUTH1/PROBEBIND1 验收；
- [x] B43-T7：verify done 生命周期词形去 verdict 化，新增中英双车道负例；
- [x] B43-T8：render/orchestrator 相关回归通过；提交推送 VSTAT1 小批，转下一组跨模式 pair。

### B44：严格 JSON 数据契约 × 外部手册操作上下文（2026-08-02）

为避免持续停留在 Trace/Write，本批选择 data 与 operation 两个低近期覆盖维度，严格并行 2
case。runner 2/2 PASS（data 52s、operation 121s）；人工为 data PASS、operation partial。

data 最终精确输出 `{"ids":["u1","u3"]}`。首 plan 虽漏调度 `instructions.md`，但 B41 的
terminal material scheduling guard 在脚本执行前以 typed `required_material_scheduling` 拒绝，
模型修复后一次执行消费两份材料；这是应有 fail-loud，不是新 gap。新 gap 位于更下游：

| ID | P | gap | 最优方案 | 状态 |
|---|---:|---|---|---|
| EVAL-B44-DATACONTRACT1 | P1 | terminal `custom_transform` 的 workflow/plan 明确 `json_only, explanation_allowed=false`，但内部 relaxed sub-plan 的 `freeform,true` 泄露到 `result.output_contract`，终态同时向模型提供两个相反 typed 权威面 | relaxed contract 仅限 action 内部执行；ActionRunner 发布 terminal result 前恢复 validation plan 的输出契约。`continue_after=true` 的中间批继续保持 freeform；不读答案文本推断契约 | implemented/tests-pass |
| EVAL-B44-OPEXTRACT1 | P1 | operation 已下载完整长 HTML，但 evaluator/finalizer 每份只见 4000-rune material excerpt；`source_truncated=false` 只表示 256KiB 读取未截断，无法表达 prompt excerpt 已截断，模型却宣布“完整”并写出错误 `/focus` 和无来源“最近 3 轮” | 增加独立 typed `excerpt_truncated`，保留 `source_truncated` 的源读取语义；继续依靠已有软 continuation 指导做分页/正文定向提取，不按 URL、用户关键词或答案文本做 hard gate | implemented/tests-pass |
| EVAL-B44-EVALMETRIC1 | P2 | eval 把 operation 进度、HTML title、文档名中的任意“Trace 因果投影”子串计成 9 个系统投影块 | 只计最终答案中的精确 `## Trace 因果投影` / `## Trace Causal Projection` 标题（允许系统目标后缀）；其它正文提及不计 | implemented/tests-pass |

`DATACONTRACT1` 的修复点是 typed publication boundary，不是对这个 JSON case 做格式拟合：

1. custom transform 仍在 relaxed sub-plan 中运行，允许内部产出 artifact/ledger；
2. terminal batch 的发布契约来自 `actionRunnerValidationPlan`，与 workflow/output projection
   使用同一 authority；
3. 中间 batch 的 validation plan 本来就是 freeform，因此不被错误收紧；
4. 新测试同时固定 terminal JSON、terminal plain-single-line 和 intermediate freeform 三臂。

operation 人工审计还发现量具本身跨模式污染：该 case 无 trace attachment、无 trace_query，
却报告 `trace_query_final_projection_blocks=9`。原因是旧 metric 统计任意子串。修复只识别系统
发布块的精确 Markdown heading，不扫描用户输入做系统行为门，也不改变运行时答案。

任务清单：

- [x] B44-T1：严格并行 data + operation，读取完整日志、执行材料与最终答案；
- [x] B44-T2：修复 terminal custom-transform typed output contract 发布边界，相关 dataquery /
  dataworkflow / repl 回归通过；
- [x] B44-T3：修复 trace projection eval metric 跨模式污染，runner contract 测试通过；
- [x] B44-T4：补 operation material 的 source-read/excerpt 双截断 typed context 与测试；
- [x] B44-T5：分批提交推送；干净 HEAD 同对回放，确认 JSON context 单一且 operation 会在广域
  材料仍截断时优先继续定向抽取。

#### B44-b：operation 长材料上下文必须同时披露两层截断

`commandPayloadMaterialExcerpt` 现在为每份材料发布两个互不替代的 typed boolean：

- `source_truncated`：256 KiB 有界源读取是否未覆盖完整 payload；
- `excerpt_truncated`：HTML 正文抽取/文本压缩后，交给 evaluator/finalizer 的 4000-rune
  prompt excerpt 是否仍只是一段前缀。

小材料固定 `false/false`，大于 4000 rune 但小于读取上限固定 `false/true`，超过读取上限的
长材料固定 `true/true`。operation evaluator 与 final answerer 只获得软、证据边界型指导：当
遗漏部分与用户范围有关且 payload ref 可读时，继续安排 bounded page/search/extraction；否则
披露边界。系统不从 URL、用户原文或模型答案关键词判断相关性，不硬拒绝 `complete`，也不替
模型生成结论。

#### B44 r2：截断边界已送达但模型忽略；reconcile 错用中间展示契约（2026-08-02）

干净 `main@7d4575a13` 同对严格并行，runner 0/2；人工 operation partial、data FAIL。

operation 在首页一次 fetch 后，evaluator 的思考和 typed complete reason 明确只引用
`source_truncated=false`，忽略同一 material row 的 `excerpt_truncated=true`，没有继续读取
已经发现的 `user_guide.html`。最终答案仍正确给出安装、PATH、provider、启动、REPL/CLI 与
八类场景，并诚实说详细文档“需进一步读取”，但不满足用户要求的长页分页/正文完整总结。
精确 context 已存在、软规则也逐字要求 omitted scope 相关时继续，因此本轮归为
`EVAL-B44-OPEXTRACT1=implemented / model-variance-watch`；不继续堆 hard completion gate、URL
特判或系统答案替换。

该 operation 的 runner FAIL 另有看护过硬：oracle 只接受连续短语“用户使用手册”或英文
manual/guide，答案的“CODRAX 用户手册摘要”语义正确却未匹配。eval 评分放宽为中文
`用户(使用)?手册|使用指南`，仍要求使用/配置/启动等内容与 codrax.net 来源；这不影响产品
行为，也不把答案关键词变成系统硬门。

data 则暴露 P0 新 gap：模型本轮选择完整 typed ledger 路径，系统最终已有 5 条 decisions、
3 条 rules 和 2 条 target contributions（u1/u3，operation=count）。
`reconcile_artifacts` 由贡献账确定性得到 single-group expected/actual=`2`，但 validator 又把
早期 `- rules_artifacts... - coverage_records...` artifact summary 当作最终答案比较，导致
11 data rounds、6 repair rounds、9 failed actions 后 terminal failure。

| ID | P | gap | 最优方案 | 状态 |
|---|---:|---|---|---|
| EVAL-B44-RECONAUTH1 | P0 | intermediate batch 为继续规划而发布 freeform contract；reconcile validator 复用该展示契约，因而把任何旧摘要当可比较的最终答案，和 contribution-derived expected/actual 硬冲突 | 将“中间结果如何展示”与“什么 answer 可参加 reconcile”分权：结果继续 freeform；reconcileComparableAnswer 单独消费原始 workflow/output contract。只有满足该 contract 的 answer 才做 answer-level 交叉核对 | implemented/tests-pass |
| EVAL-B44-OPORACLE1 | P2 | operation oracle 将“用户使用手册”固定成不可变连续词，误杀“用户手册摘要” | 只在 eval 中接受同义中文标题；保留来源和实质使用内容两个独立断言 | implemented |

`RECONAUTH1` 不根据摘要前缀或答案关键词扩充 heuristic：`validateRunnerResult` 接受独立的
reconcile output authority，ActionRunner 即使使用 relaxed intermediate validation plan，也
把原始 `plan.OutputContract` 传给 reconcile。定向回归固定：旧 artifact summary 不参与 JSON
final-answer 对比、贡献 count 仍严格核成 2、intermediate result 仍发布 freeform；真正满足
JSON contract 的答案仍会接受 expected/actual 交叉核对。

#### B44 r3：HTML 导航证据丢失 × 聚合域冒充答案域（2026-08-02）

干净 `main@84aba148c` 同对严格并行，runner 0/2；人工 operation FAIL、data FAIL。两案均不是
“再提示模型一次”可以稳健解决，而是系统送给模型的 typed context/authority 不精确。

operation 连续执行 5 轮：首次下载的首页 HTML 内已有精确
`href="./user_guide.html"`，但 `extractHTMLVisibleText` 在把 payload 变成可见正文时删除所有 tag，
所以 evaluator/planner 只见“使用手册”锚文本，看不见目标。随后模型使用 BSD grep 不支持的
`-P`、引用不存在的临时路径、用中文锚文本搜索 href，最终猜 `/man` 并得到 404；答案又把
不存在的 `/doc`、`/trace` 当成链接。`source_truncated/excerpt_truncated` 都无法补回已被结构化
转换删除的导航字段。

data 已形成 5 条 decisions、9 条 rules、2 条 target contributions，成员身份是 u1/u3。模型先
用 `operation=add` 处理字符串而被 numeric typed gate 正确拒绝，后改成 count。系统得到
single group `all/ids=2`；模型随后真实执行出正确 `{"ids":["u1","u3"]}`，但 validator 报
`expected_answer "2" does not match result.answer ...`，最后 workflow 明知生成答案正确仍回退发布
旧候选 `2`。

| ID | P | gap | 最优方案 | 状态 |
|---|---:|---|---|---|
| EVAL-B44-OPLINK1 | P1 | HTML material context 保留可见文本但丢失 anchor target，模型无法从结构化证据定位下一页 | 从同一 256 KiB 有界源前缀抽取 source-ordered、去重、数量/单值/总 rune 三重有界的 literal href 清册；发布 `html_link_targets` 与 `html_link_targets_truncated`。它只提供证据，不自动选择或执行链接 | implemented/tests-pass/commit `8da6a0dc1` |
| EVAL-B44-RECONSCOPE1 | P0 | `reconcile_artifacts` 仅凭 `len(groups)==1` 把 contribution aggregate 铸成 final-answer expected/actual；单一 count group 因而否决合法 object/list projection | group reconciliation 与 answer projection 分权：reconcile action 只保留每组 expected/actual，顶层 answer claim 留空并显式标 `answer_comparison_status=not_evaluated`；`assemble_answer` 才标 pass 并绑定 answer。已有显式 answer-scope 报告仍照常 hard check | implemented/tests-pass |
| EVAL-B44-ACTIONPARAM1 | P1 | `DataAction.Params` 是开放 map；compute_contributions 对未知 `include` 静默忽略，模型以为请求 member list，runner 实际 count | 在该通用计算 action 边界按完整支持参数词表 fail-loud；错误列出未知/允许键并准确说明 count 与 include/set/rank+value_field。只读 typed params，不读 purpose/用户/答案 prose | implemented/tests-pass |
| EVAL-B44-OPORACLE2 | P2 | eval 同义词 regex 覆盖“用户手册”“用户使用手册”，遗漏普通“使用手册” | eval-only 扩充同义短语，继续独立要求实质使用内容与 codrax.net 来源 | implemented |

上下文精度不变量：

1. HTML link inventory 与正文 excerpt 并列，不能取代 source/excerpt truncation；若源读取被截断，
   清册也只代表已读前缀。
2. 系统不根据 URL、anchor 文案或用户关键词自动导航；模型根据 typed target + 正文决定下一步。
3. contribution group `pass` 只证明组内核对，不再暗示 final answer 已核对；answer comparison 的
   `not_evaluated/pass` 状态必须随 prompt clamp 保留。
4. 未知 action param 是精确 schema 违例，可 hard reject；任务语义、答案是否好仍由模型/evaluator
   判断，不能从 purpose 或输出原文反推。
5. 本批不触及 Trace 路由、显式窗识别、因果投影或自动补齐代码面；Trace projection eval metric
   仍为 0，跨模式没有再次污染。

任务状态：

- [x] B44-r3-T1：严格并行同对，人工读完命令链/data repair 链和终态答案；
- [x] B44-r3-T2：HTML href typed inventory + 三重限额 + 截断披露；
- [x] B44-r3-T3：贡献聚合域与最终答案投影域分权，并让 prompt 保留 comparison status；
- [x] B44-r3-T4：compute_contributions phantom param fail-loud；
- [x] B44-r3-T5：相关 dataquery/dataworkflow/repl 全包回归；
- [x] B44-r3-T6：提交推送 data 批；下一步从干净 HEAD 同对 r4 回放。

#### B44 r4：材料持有被误当成内容覆盖（2026-08-02）

干净 `main@72ecc977e` 同对严格并行，runner 2/2 PASS；人工 data PASS、operation FAIL。

data 在 35s 内一批完成、零 repair，实际消费 `instructions.md + users.json`，终态只输出
`{"ids":["u1","u3"]}`。本轮走 direct terminal custom-transform，没有触发 contribution
reconcile，但证明 r3 的 authority 分权没有破坏稳定路径。

operation 的 href 修复也已生效：模型从首页 typed link inventory 精确取得并抓取
`user_guide.html`。它看到 `excerpt_truncated=true` 后继续两轮提取，但一次把 CSS 混进正文，
另一次把 `.doc-body` 到 `.doc-kicker` 写成起止相邻的空范围。第 4 轮 evaluator 明确认知
“第 2–8 章未获取”，随后却因完整的 32,901-byte HTML 已保存在 payload ref 而发出
`status=complete/high`；最终答案反称“完整获取 8 章正文”。这不是 source/excerpt 边界不精确，
而是系统没有表达“材料在手”和“相关正文已被模型可见地消费”之间的权限差。

| ID | P | gap | 最优方案 | 状态 |
|---|---:|---|---|---|
| EVAL-B44-MATCOVER1 | P1 | evaluator 把完整 payload 已下载/保存当成全文内容已覆盖；即使所有 prompt excerpt 都是 truncated 或失败抽取，也能发布 complete | 新增 typed `material_coverage_status` 与 `coverage_material_refs`。当记录中存在截断 payload 且 evaluator 要 complete 时，只允许两条路：模型明确判定该材料与用户目标不相关（not_applicable），或引用本轮记录内、`source_truncated=false && excerpt_truncated=false` 的有界抽取。否则通过既有 structured-tool repair 要求继续/partial/budget，不替换结论 | covered/B45-r2 |
| EVAL-B44-HTMLBODY1 | P1 | operation planner 对 HTML/长文本缺少可靠的结构化分页与来源覆盖载体；任意 shell 输出只形成新的 payload ref，没有 upstream source/range lineage，完整的 177KB 正文输出也只给下一轮 4000-rune 前缀 | 提供通用 bounded material read/extract primitive：记录 source ref/hash、representation、byte/rune range、page ordinal、complete/remaining，并用 coverage ledger 合并非重叠页；HTML/日志/手册/大命令输出共用，不能为某个 URL、CSS class 或章节写特例 | implemented/full-tests-pass；operation replay next |

`MATCOVER1` 的边界设计：

1. hard check 只读取系统记录的 command payload ref，以及 material carrier 上精确的
   `source_truncated/excerpt_truncated` 位；不读取用户原文、evaluator reason、thinking、summary
   或最终答案。
2. evaluator 仍决定哪些材料与目标相关、抽取内容是否足够。`not_applicable` 保留模型的相关性
   判断；系统只阻止“截断材料 ref 本身就是 complete coverage”这一 typed 矛盾。
3. `coverage_material_refs` 必须来自已经执行的 command records，且每个引用的 prompt excerpt
   完整可见；不能凭空引用或引用尚未读取的文件。
4. 首次违例进入已有 compact structured-tool repair；修复后仍矛盾则 fail loud。系统不会把
   complete 改写成 partial，也不会生成或替换用户答案。
5. 该合同只在 operation evaluator 的 `status=complete` 且确有不完整 payload 时生效；不触及
   Trace 路由、显式窗、因果投影、自动补齐、read/data/write 的结论面。

上下文审计结论：r4 的链接目标、payload 路径、字节数、source/excerpt 两层截断均准确，足以让
模型继续探索；缺失的是跨轮“哪些完整可见抽取真正支撑完成”的 typed closure。新字段补的是
证据覆盖权限，不是系统结论。`HTMLBODY1` 仍开放，因为阻止 false complete 不能自动让正文
抽取变高效。

任务状态：

- [x] B44-r4-T1：严格并行同对，人工读取完整 operation 命令链、evaluator 决策和最终答案；
- [x] B44-r4-T2：增加 typed material coverage carrier、schema、prompt 与完成态一致性检查；
- [x] B44-r4-T3：固定截断 payload 触发 repair、完整有界抽取可完成两个正反臂；
- [x] B44-r4-T4：operation/repl 全包回归通过；
- [x] B44-r4-T5：提交推送本批；下一步从干净 HEAD 用严格并行 2 case 验收并继续跨模式优先队列。

### B45：operation 覆盖合同旁路 × 有限目标读模式被扩成全仓清册（2026-08-02）

干净 `main@fccb65d23` 严格并行 operation + read，runner 2/2 PASS；人工 0/2。

operation 的 evaluator 在前两轮正确消费 `excerpt_truncated=true` 并要求继续。正文抽取命令失败后，
replan planner 直接发出 plan `status=complete`；CLI 的
`commandOperationTerminalAfterReplan` 随即终态化，没有再调用 evaluator，因此 B44 r4 新增的
`material_coverage_status/coverage_material_refs` 完全没有执行机会。最终答案比 r4 诚实，末尾承认
只摘要第 1–2 章、3–8 章仍在原 HTML，但开头仍称“任务完成”，并写入未完整可见章节的细节。

read 的最终正文事实正确，却用了 419s、20 次 source_inventory、42 explorer iterations、9 次
completion 调用。typed 根因十分明确：analyzer 对“比较两个已命名工具的 Name() literal 与
finalizer 关系”发出 `answer_subject=string_literal + exact_targets=2`，同时误发
`source_inventory_profile(function)`。完成门把后者解释成 repo-wide 227 个 function 的主清册债务，
强迫模型遍历与答案无关的 fixture、testdata、skill；两份精确源码早已读完也不能关闭。

| ID | P | gap | 最优方案 | 状态 |
|---|---:|---|---|---|
| EVAL-B45-OPREPLAN1 | P1 | replan plan 的 terminal `complete` 绕过 evaluator material coverage 合同；同一 operation 有两套完成权限 | 给 command-plan terminal 复用同一 typed coverage schema/validator。只有 replan/continuation 存在截断 payload 时检查；`not_applicable` 保留模型相关性判断，`complete` 必须引用本轮已记录且完整可见的抽取，否则进入 compact structured-tool repair。不能由系统把 complete 改成 partial | implemented/tests-pass/batch-B |
| EVAL-B45-SISCOPE1 | P1 | 有限 exact-target 的标量/字面量比较被 source-inventory profile 扩权成 repo-wide exhaustive function inventory | 在共享 `SourceInventoryCompletionIsSupportOnly` 增加 schema-only 边界：已验证 finite exact targets + scalar source-literal answer subject，且没有 count/member-set/completeness/const-set 义务时，source_inventory 仅作导航支持。精确 observed row-set mismatch 仍由既有 gate 管；真正全量清册不放宽 | implemented/tests-pass/batch-A |
| EVAL-B45-ANCHORDUP1 | P3 | 模型正文已给注册/合法检查位置，系统又追加 6 行 owner 表，形成低价值重复 | 留样；先观察其它 read case。除非跨题复现，不做答案 prose 去重或系统删改 | watch |

上下文精度审计：

1. operation 的 href、payload、双截断位都准确；缺口是 replan completion 没有消费这些 typed 位，
   不是需要更多 prompt 文案。
2. read 的精确目标、字符串字面量、两个源码文件、finalizer 关系证据都准确足量；错误上下文来自
   同时注入的 repo-wide function universe，且被 hard completion gate 放大。
3. 两项修复都只消费 schema enum/boolean、exact target 数、系统 payload refs 和截断位；不扫描用户
   原文、模型 thinking/reason/final，也不替模型写结论。
4. 本批无 trace attachment/query，projection metric=0；不修改 Trace 路由、显式窗、因果投影或
   自动补齐。

Batch B 已落地：

1. `emit_command_operation_plan` 增加与 evaluator 同义的
   `material_coverage_status/coverage_material_refs`；它只约束 replan/continuation 的 terminal
   `complete`，初始无观测 complete 和非终态计划保持兼容。
2. planner 与 evaluator 共用 `validateCompleteCommandMaterialCoverage`。系统仅枚举当前 command
   records 的 payload refs，并从有界 excerpt 的精确
   `source_truncated/excerpt_truncated/skipped` 位判断“完整可见”；不读取用户原文、模型 reason、
   thinking 或 final prose。
3. 不完整材料上的无证据 complete 进入既有 compact structured-tool repair。repair context 只增加
   `material_ref=<ref> fully_visible=<bool>`，让模型自己选择继续有界抽取、部分完成、预算耗尽、
   `not_applicable` 或引用完整抽取后的 complete；系统不降级、不改写结论。
4. 生产见证固定两条相反路径：失败抽取后的 replan-complete 必须修成一个可执行的后续抽取批；同时
   存在长原件和完整可见的小型抽取物时，模型引用后者可一次通过 terminal complete。
5. 定向测试与 `go test ./internal/operation ./internal/repl -count=1` 全通过，`make` 通过；
   Trace 代码零改动。

施工顺序：

- [x] B45-T1：严格并行 2 case，读完两案全过程与答案，runner PASS 不作为人工结论；
- [x] B45-T2（batch A）：有限 exact-target scalar comparison 将 source inventory 降为 support-only；
- [x] B45-T3（batch B）：command replan/continuation terminal 复用 typed material coverage contract；
- [x] B45-T4a：batch A/B 相关全包测试、构建、提交推送；
- [x] B45-T4b：从干净 HEAD 恰好并行 2 case 回放，人工复核全过程、答案与上下文。

#### B45 r2：合同闭合，但长材料 lineage 与模型结论波动仍在（2026-08-02）

干净 `main@86102e418` 严格并行同对，runner 2/2 PASS，人工 0/2：

1. operation 用例确认 `MATCOVER1 + OPREPLAN1` 均已生效。两次无证据 complete 都被 typed
   validator 拒绝并进入 compact repair；模型继续抽取，最终 evaluator 发布
   `partial_answer_possible`，系统首行也诚实显示“部分结果”，没有再从 replan 旁路 complete。
2. operation 最终自由文本仍在末尾写“任务已完成/完整内容”，并再次把 `/repos focus` 写成
   `/focus`。final answer prompt 已精确携带 `status=partial_answer_possible` 和
   “Do not call the task fully complete”，所以这是模型对精确上下文的单次违背；按红线不扫描
   final prose 做 hard gate，也不由系统删除或替换该句，登记 `EVAL-B45-OPFINAL1/P2-watch`。
3. 更值得施工的是 `HTMLBODY1`：第三轮只打印前 8000 字符，第四轮打印完整正文，但每个任意 shell
   输出都成为没有 source/range lineage 的新 payload，下一轮仍只见 4000-rune excerpt。系统能证明
   “本轮 ref 的 prompt 可见性”，不能证明多个抽取页是否覆盖原材料，也不能给 planner 一个确定的
   next offset；该项从 P2 升 P1，方案冻结为 first-class bounded material reader + coverage ledger。
4. read 用例从 419s 降至 111s，`repo_map=1/read=4/explorer=5/completion=1`，没有再进入 227-function
   清册循环。但本次 analyzer 恰好没有发 source-inventory profile，因此真实运行只证明性能恢复，
   没有执行 batch A 的 support-only 分支；该分支仍以 production 接线测试为权威证据。
5. read 最终把“有 prev 时优先 patch、完整重写仍可 full emit”缩成“有 prev 必走 patch”。源码第 49
   行和 evidence snippet 已准确完整提供复合条件，r1 同案也曾答对，故登记
   `EVAL-B45-READREL1/P2-model-variance-watch`；不为这个句子增加答案文本门或系统改写。若跨 case
   再现，再考虑通用 typed mechanism-comparison carrier，而不是匹配工具名。

上下文结论：两个答案都不是“系统没有材料”。read 上下文准确足量但模型缩窄了条件；operation
终态上下文准确，但大材料的内容分页上下文不完整。前者留作波动观察，后者是可泛化、高 ROI 的
系统能力缺口。两案均无 Trace 输入，projection=0；本批没有改变显式窗、因果投影或自动补齐。

后续任务排序：

- [ ] B45-T5（P1，独立设计/施工批）：bounded material read/extract + source-range coverage ledger；
- [ ] B45-T6（P2 watch）：跨题观察 operation typed partial 与自由正文、mechanism compound condition
  是否再次矛盾；未复现前禁止添加原文扫描、结论替换或具体工具名规则；
- [x] B45-T7：r2 结果、人工裁定、上下文充分性和任务排序入统一台账并单独提交推送。

### B46：显式窗 Trace 修向桶越权 × write 跨语言项目测试被合成 runner 遮蔽（2026-08-02）

干净 `main@bda6687e0` 严格并行以下两案，runner 0/2 PASS，人工 0/2：

1. `real_trace_h5_smr_multirow_disposition`：runner 因答案正文未出现固定 regex
   `等待对象 dma_fence_default_w` 失败；显式用户窗、6 次带窗 trace_query、自动补采、两维根因、
   根因排序、唤醒链、窗内可消除量和完整 Trace 因果投影全部存在。
2. `github_issue_memoclaw_text_search_multirepo_py`：补丁自身正确，sync/async 行为探针和 fixture
   `make check` 均通过，但系统最终错误降为
   `unverified:verification_proof_incomplete`。

| ID | P | gap | 泛化最优方案 | 状态 |
|---|---:|---|---|---|
| EVAL-B46-XMAKE1 | P1 | typed TestSurface 已发现真实 `make check`，且 bounded declared roster 精确包含变更 Python 文件；plan-touched Python 偏好仍合成 pytest，并由同目录去重把 Make 从执行队列删掉。行为探针虽通过，却因缺 contract refs 被作为 probe-only proof 记成十项 missing | 新增 repository-declared coverage selection lane：只有 candidate 的 filesystem-verified roster 覆盖全部 recognized changed source paths 时，作为优先项目测试选路；仅决定执行哪个真实 suite，不给 roster 任何 changed-path/behavior proof 权限。部分覆盖不得遮蔽其它变更面 | implemented/full-tests-pass；replay next |
| EVAL-B46-REMEDY1 | P1 | `fix_direction=frequency_thermal` 是修复方向桶，模型却升级成“受频率热节流”；同一上下文又明确策略上限/非最高频不能单独证明热机制，形成内部矛盾 | explore/finalizer 的 typed guidance 与 rank handoff 明确拆开 `remedy_bucket` 和 `mechanism_authority`：正值只证明 compute-delivery head-room/供给折算缺口；具体 thermal/governor/core-placement 机制必须有独立正向 typed observation。保持模型结论权，不做答案扫描/替换 | implemented/full-tests-pass；replay next |
| EVAL-B46-FENCE1 | P2 model-variance/watch | `dma_fence_default_w` 已在 projection 树、detail、typed 校验事实中多次进入最终上下文，模型正文只写 D-state 数值，没有把内核等待点提升到优化摘要 | 现有上下文准确足量，先跨例观察。不得为固定“等待对象”词形增加 regex hard gate，也不得由系统替模型补结论；若跨题再现，考虑通用 typed actionable-lead priority guidance | watch |
| EVAL-B46-PROBE1 | P2 | planner 生成了能直接验证六个 exact behavior contracts 的两个 probe，却没有填写任何 `contract_refs`；累计 proof-only batch 又重验原计划，未补元数据 | 保持 refs 为显式 typed claim，禁止从 probe code/答案文本自动猜合同。先由 XMAKE1 恢复独立项目 suite；后续单独审计 proof-only scheduler 是否应把 missing refs 送入 probe-only replan，而非原样 reverify | open/watch |
| EVAL-B46-ORACLE1 | P3 eval | 旧 Trace oracle 把自然语言前缀 `等待对象` 与内核符号绑成单一 regex；事实已完整发布仍判 FAIL | eval 应优先钉 typed row/entity/value/authority，不以模型自由表达的固定前缀代替事实覆盖；先更新人工裁定，后续集中治理旧 oracle，生产代码不为其拟合 | open/eval-debt |

上下文精度与红线审计：

1. Trace 的事实载体足量，但角色语义不够精确：`frequency_thermal` 同时像“修向名称”和“已证机制”。
   B46-REMEDY1 只改模型前的软指导与 typed handoff 元数据；不读 RawRequest、thinking、final prose，
   不拒绝、不删除、不替换模型答案。
2. Trace projection 在模型三块正文之后以“系统生成内容”边界追加，未取代模型正文；本批不改投影
   materializer、显式窗优先级、根因算法、唤醒链、可消量或自动补采。
3. write 的 API reference、源码、diff、probe 结果和 TestSurface inventory 均精确；错误发生在系统选路：
   精确的 declared Make lane 已存在却未消费。XMAKE1 只使用 typed candidate、exact roster 和
   recognized changed source paths，不扫描任务/模型原文。
4. declared roster 仍不是验证权威：既有
   “cross-language Make declared input 不得直接证明 changed path behavior”测试保持。执行成功的项目
   suite、identity-coupled probes 和 proof ledger 继续各司其职。
5. 当 passed pre-suite probe 后面确实执行项目 suite 时，两个结果现在都会进入最终 ChangeReport。
   旧路径只保留 project report，会丢掉 probe 的 typed changed identity；本批只合并 `passed` probe，
   unavailable/failed probe 仍按既有降级/失败语义处理，不能被项目 suite 静默洗白。

本批施工：

- [x] B46-T1：严格并行 Trace + write，逐段人工审计最终答案、tool calls、typed handoff、patch、
  probes、TestSurface 与 final proof ledger；
- [x] B46-T2（write）：让完整覆盖全部 recognized changed sources 的 declared test candidate 优先于
  extension-inferred synthetic runner；增加全覆盖正臂、部分覆盖负臂和 probe+Make 执行接线测试；
- [x] B46-T3（Trace）：在 explore/finalizer 双面及 rank authority handoff 明确
  remedy bucket ≠ proven mechanism；不增加任何正文 hard gate；
- [x] B46-T4：`go test ./internal/tool ./internal/agent ./internal/skill -count=1` 全通过
  （tool 158.367s、agent 4.602s、skill 0.889s）；`make` 与 `git diff --check` 通过；
- [x] B46-T5a：审计结果与修复状态更新入统一台账，write/Trace 两个代码批分开提交推送；
- [ ] B46-T5b：从干净 HEAD 严格并行同对回放，之后继续覆盖优先队列。

#### B46 r2：选路已通但 meta-runner 权威未贯通；Trace 修向正确后暴露单位与关系 GAP

从干净 `main@8592d6b2c` 严格并行同一 Trace/write pair。runner 仍为 0/2，人工也为 0/2，
但失败层已经收窄：

1. write 的 `make check` 这次由 `declared_coverage_test_surface` 正确选中并真实执行，退出 0，
   输出 `python text search contract ok`；补丁仍只修改 `memoclaw/client.py` 且行为正确。
   随后的 changed-path ledger 仍失败，因为它只把外层 runner=`make` 映射成 C/C++，不知道
   该精确 `check` 目标实际执行的是 Python。上一批解决的是 execution selection，本轮证明
   execution semantics→coverage authority 仍断链。
2. Trace 的 `EVAL-B46-REMEDY1` 已有生产见证：模型明确 58.320ms 是 compute-delivery
   head-room/供给折算缺口，并明确 2.34GHz 热控轨上限不证明 thermal throttling；显式窗、
   5 次 windowed trace query、自动补采、双轴、根因排序、唤醒链、可消量及完整投影均保留。
   `dma_fence_default_w` 已在模型正文和系统事实中，runner 仍只因固定自然语言前缀
   `等待对象` 缺失而失败，继续归 `EVAL-B46-ORACLE1`，不得污染生产逻辑。

新增/收窄项：

| ID | P | gap | 泛化最优方案 | 状态 |
|---|---:|---|---|---|
| EVAL-B46-XMAKE2 | P1 | 已成功执行的 meta runner 丢失其具体执行语言，导致同语言精确项目检查也不能覆盖 changed path | TestSurface 为有界 Make 目标携带 `declared_execution_language_families`；只有“成功执行的精确目标 + execution family 与变更语言相交 + 变更路径在 exact declared roster”合取才生成 `CoveredPaths`。动态 executor 不猜；roster 单独无权；Python 检查 Rust 的负例保持 uncovered | implemented/full-tool-tests-pass；commit/push next |
| EVAL-B46-OCCUNIT1 | P1 | TWODIM 主要时间占用表把 `page_cache_churn` 计数当量 81.616 渲染成 `81.616ms`，同时 `max=84.300ms > total=81.616ms`；树/可消面已正确标成非墙钟，只有 occupancy path 漏 guard | occupancy path 复用单一 `runtimeTraceProjNonWallClockValueCaliber`，所有 count-equivalent/composite-score 行从墙钟占用候选排除；值仍留在口径旁栏/证据面，不删除信息、不改因果排序 | implemented/full-related-tests-pass；commit/push next |
| EVAL-B46-REL2 | P1 guidance | 模型把“不同维度”直接推成“相互独立且不可相加”；typed context 只对部分同段镜像/embedded row 给出关系，没有为任意跨行 mint independent/non-additive 权限 | finalizer 高显著 handoff 增加通用 `relation_authority=typed_pair_only`：只在 typed relation/fold/state-partition 明确时说包含/重叠/互斥/可加；不同 metric family 本身不证明关系，缺载体时说 physical relation unresolved，只有 cross-row addition not authorized。只做软上下文，不扫描/改写答案 | implemented/full-related-tests-pass；commit/push next |
| EVAL-B46-DUP1 | P3 model-variance/watch | 模型正文重复发布一遍根因清单 | 不影响 typed 值与 projection，单次模型表达波动；禁止系统删除/合并模型块，跨模型/跨例复现后再考虑结构化 answer schema guidance | watch |

上下文精度结论：Trace 的 mechanism authority 修正已足以让模型自行纠偏，但系统自身的
TWODIM occupancy 单位不精确，必须先修确定性事实面；跨行关系则是 typed authority
缺席导致模型自由推断，适合通用软指导。两项都不能通过关键词扫描、final prose gate 或
系统替换答案处理。write 的源码/API/patch/test context 一直精确，错误只在验证权威的
meta-runner 语义断链。

施工状态：

- [x] B46-T5b：r2 严格并行与完整人工审计；
- [x] B46-T6a：XMAKE2 typed carrier、成功命令合取与 Python/Rust 正反臂；
- [x] B46-T6b：XMAKE2 `go test ./internal/tool -count=1` 全通过（159.804s）；提交推送；
- [x] B46-T7：OCCUNIT1 + REL2 独立 Trace 批；`go test ./internal/agent ./internal/tool -count=1`
  全通过（agent 2.465s、tool 166.143s）；提交推送；
- [x] B46-T8：干净 HEAD 同 pair r3 验收；write 与 occupancy 已通过，关系上下文发现新的
  authority 缺口，先修后进入下一优先级 exactly-two batch。

#### B46 r3：write 验证闭环、非墙钟占用闭环；关系 handoff 自身铸造了伪 authority

从干净 `main@83cf8cb44c35` 严格并行同一 Trace/write pair。runner 1/2 PASS，人工 1/2：

1. write 为 runner/human 双 PASS。补丁只修改 `memoclaw/client.py`，真实
   `make check` 由 `declared_coverage_test_surface` 选中并执行，输出
   `python text search contract ok`；ChangeReport 为 `verification_status=passed`，
   `memoclaw/client.py` 的 Python changed-path coverage 为 `covered`。这为
   `EVAL-B46-XMAKE1/XMAKE2` 提供了完整生产见证。
2. Trace 仍仅因旧 regex `等待对象 dma_fence_default_w` 判 FAIL；符号已在模型可见
   typed projection、最终树和 detail 中出现，继续归 `EVAL-B46-ORACLE1`，生产逻辑不拟合。
   显式用户窗、4 次 windowed query、自动补采、两维占用/可消分析、根因排序、唤醒链和
   完整因果投影全部保留。
3. `EVAL-B46-OCCUNIT1` 已闭环：`page_cache_churn` 不再进入墙钟“主要时间占用”表；
   81.616 继续以“计数当量(非墙钟)”保留在 projection 口径旁栏，信息未删除。
4. `EVAL-B46-REMEDY1` 继续通过：模型把 58.320ms 写成 compute-delivery/
   supply-fold headroom，没有再由 2.34GHz 轨上限推成已证 thermal throttling。
5. `EVAL-B46-REL2` 必须重新打开，而且性质从“缺一条软提示”升级为系统上下文错误：
   prompt 中虽有 `relation_authority=typed_pair_only`，模型仍把“不同通道/维度”升级为
   independent、mutually exclusive、physical non-overlap 和 additive；更关键的是系统 handoff
   把任何正值 `DStateSplitMS/IOWaitSplitMS` 无条件写成
   `embedded_components / already_inside_parent_row`。这两个字段只是该 observation 的状态拆解；
   post-finalize renderer 只有在“同主体同 IO 状态族 + 主值=D+IO + 唯一对席 + 值恒等 +
   窗兼容”等合取成立后才铸跨行包含指针。handoff 跳过该判定，反而给模型提供了伪 typed authority。

| ID | P | gap | 泛化最优方案 | 状态 |
|---|---:|---|---|---|
| EVAL-B46-XMAKE1/XMAKE2 | P1 | 项目测试选择与 meta-runner execution semantics→changed-path coverage 曾断链 | exact target + concrete execution family + successful command + exact declared roster 的合取已通过生产回放 | covered |
| EVAL-B46-OCCUNIT1 | P1 | 非墙钟计数当量混入墙钟 occupancy | 共享 caliber guard 已通过生产回放；值保留在旁栏 | covered |
| EVAL-B46-REL2A | P1 | pre-finalizer handoff 把 row-local state split 无条件伪装成 cross-row containment | 已删除伪关系；D/IO 数值只标为 `row_state_breakdown`，并带 `this_observation_only / not_provided_by_state_breakdown` 权限边界 | implemented/agent-tests-pass |
| EVAL-B46-REL2B | P1 | 用户要求跨行关系时，准确关系只在 post-finalize renderer 的内部 tree model 中，模型成文前拿不到同源的精确 pair roster | 第一段已补共享 typed facts：目标四态明确“互斥且仅可加到闭合总量”，self-runnable two-ruler 的合法性下沉为 types 单一校验，handoff 携带同尺席/小计、跨尺禁加和物理关系 unresolved；SMR/AXIOM/RSPA renderer-only pair roster 仍待共享化 | partial/replay-first |
| EVAL-B46-REL2C | P2 model-compliance | 即使负向边界在高显著 prompt 中，模型仍可能越界陈述无载体关系 | 先修上下文自相矛盾并提供正向 exact pair facts；仍跨例复现时走普通语义审阅/重试，不做字符串 gate 或系统替答 | watch-after-context-fix |

上下文审计结论：本轮不是“信息不够多”，而是“关系信息太多但 authority 不一致”。数值、单位、
窗口、排序、唤醒边、修向边界都足以支持正确回答；relation handoff 与最终 renderer 不共享一个
判定源，导致模型同时收到正确禁令和错误正向载体。优先修复单源性，不能用更长 prompt 掩盖。

后续任务：

- [x] B46-T9a：删除 `DStateSplitMS/IOWaitSplitMS => embedded cross-row relation` 的伪映射，
  改为诚实 row-local breakdown；正向固定数值继续传递，负向固定不再铸 containment；
  `go test ./internal/agent -count=1` 通过（2.917s）；
- [x] B46-T9b-a：修正关系上下文自相矛盾：两条“决策轴”不再称物理 independent；任意跨行
  禁加改为“仅 exact additive carrier 授权”；互斥分区成员可加还原其 typed 总量；
- [x] B46-T9b-b：新增 types 共享 two-ruler admission authority，renderer 与 finalizer handoff
  同源消费；handoff 逐尺列 rank/value/subtotal，跨尺只授权禁止相加且物理关系 unresolved；
- [ ] B46-T9b-c：若 r4 仍需要更多关系事实，再把 SMR/AXIOM/RSPA renderer-only pair roster
  抽为共享 typed surface；不得在 agent 复制第二套 pair 判定；
- [x] B46-T9c：`go test ./internal/types ./internal/agent -count=1` 通过
  （types 22.595s、agent 2.819s），`go test ./internal/tool -count=1` 通过（159.683s）；
  本批提交推送后从干净 HEAD 严格并行同 pair r4；
- [ ] B47：r4 后按优先矩阵切换到下一对 read/data/log/operation 用例，避免停留在单一 Trace/write。

#### B46 r4：typed 关系上下文已准确，但跨阶段旧结论污染与模型服从性仍未闭环

从干净 `main@18e276dfae76` 严格并行同一 Trace/write pair。runner 1/2 PASS，人工 1/2：

1. write 再次 runner/human 双 PASS。sync/async 补丁、真实 `make check`、
   `python text search contract ok`、ChangeReport=passed 和 Python changed-path=covered
   全部一致，`EVAL-B46-XMAKE1/XMAKE2` 保持 covered。
2. Trace 的两个 runner failure 均为 eval oracle 债：自然语言 multiplicity 的 `×2`
   被旧“禁止退休计数词面”规则误杀；`dma_fence_default_w` 已完整出现在 typed projection/
   tree/detail，仅缺固定前缀 `等待对象`。生产逻辑不得拟合这两个词面。
3. Trace 人工仍 FAIL。模型把不同 ruler 的 rank #4 3.956ms 与 rank #10 1.648ms
   相加成 5.604ms；把 row-local `d_state=3.598ms` 写成 priority-inversion 跨行包含；
   把 blocked_reason 总和写成 target sleep 的已证子集；又把四个同修向席位相加为
   18.853ms，而系统只发布了其中 exact mutually-exclusive pair 的 12.115ms subtotal。
4. r4 同时证明 `REL2A` 与 `REL2B` 第一段已经生效：finalizer handoff 明确发布目标
   四态互斥闭合、two-ruler 逐尺 seats/subtotal、跨尺禁加/物理关系 unresolved，以及
   row-local state breakdown 不提供跨行关系权限。模型答案内部甚至复述“two-ruler
   禁止跨口径相加”，随后仍执行跨尺加法。
5. 深一层的上下文链路审计：错误关系已出现在 Explorer 的
   `emit_investigation_complete` 总结，发生在精确 finalizer handoff 之前；Finalizer 同时
   收到旧错误总结与新正确 authority，形成冲突输入。合同检查耗时约 104ms，日志没有
   reviewer dispatch/skip；semantic/self-consistency reviewer 默认关闭，所以
   `semantic_quality_dispatches=0` 不是 observation-only skip，不能针对该 skip 函数误修。

| ID | P | gap | 泛化最优方案 | 状态 |
|---|---:|---|---|---|
| EVAL-B46-REL2A | P1 | row-local split 被伪铸跨行包含 | r4 生产见证显示 handoff 已诚实发布 row-local scope | covered |
| EVAL-B46-REL2B | P1 | 精确 pair/ruler authority 未在模型决策全过程单源共享 | two-ruler/four-state 已进 finalizer；下一步把同一共享关系载体前移到探索结论形成前，并继续抽取 SMR/AXIOM/RSPA pair roster，禁止 agent 复制判定 | partial |
| EVAL-B46-REL3 | P1 | Explorer 先形成错误关系结论，Finalizer 后收到准确 authority；模型保留两者且默认没有语义 reviewer | 为因果/关系型 Trace 建立 model-authored structured relation claims：声明 seat/ruler/relation/subtotal authority，精确校验 against shared typed carrier，失败只触发模型自修复；不读原文关键词，不扫描最终 prose，不由系统替答。普通 reviewer 可作为可选第二意见，不能假定默认已开 | filed/next |
| EVAL-B46-ORACLE2 | P3 eval | `EXPECT_NOT_CONTAINS=×2..×9` 把合法 multiplicity prose 当退休 count face | 后续 oracle 治理改钉 typed merged-count carrier/表格位置；生产不适配 | open/eval-debt |

上下文充分性结论：r3 的“authority 不一致”已消除；r4 已不是证据缺失，而是精确 typed
关系只在最终阶段出现、Explorer 旧结论先入为主，以及模型没有可校验的结构化关系声明。
继续增加自然语言禁令的 ROI 很低，还会扩大 prompt；应把 authority 变成跨阶段共享 typed
协议，并让模型自己在协议失败后重写答案。这符合“系统供证据和边界、模型拥有判断”的红线。

第一批施工采用更小、可回退的跨阶段 transport，而不是立即扩大 AnswerDocument schema：

1. `trace_query` 的 compact root-cause board 后立即发布 `relation_authority`，使 Explorer
   在第一次形成总结前就能看到同源 typed 边界；长正文、lossless JSON、finalizer handoff
   与最终投影继续保留，不删除任何信息。
2. self-runnable two-ruler 不复制合法性判定：engine carrier 转换后复用
   `TraceCausalProjectionSelfRunnableTwoRulerValid`，只发布逐尺 seats/subtotal、同尺按已发布
   小计可加、跨尺禁加及物理关系 unresolved。非法/不闭合 carrier fail-closed，只保留通用
   `typed_pair_only` 边界。
3. row state breakdown 明确限定为本行；相同 `fix_direction` 仅是 repair classification，
   没有 exact typed subtotal 就没有相加权限。blocked-reason census 明确区分 kernel record
   count、vendor delay sum，并声明 census 自身不足以证明与 scheduler state 的包含/重叠关系，
   需要 typed interval join；不同 PID 的 census 不与目标账户建立关系。
4. 该批只读 engine/types typed 字段，不读 RawRequest、Explorer/Finalizer/final prose，不做
   hard reject，不生成或替换根因结论；显式窗 query、自动补齐、因果投影、根因排序、唤醒链
   和可消除量路径均未改动。是否还需 model-authored structured relation claim，留待 r5
   生产回放判断，避免在 transport 已足够时引入重 schema。

定向回归覆盖 relation authority 位于长 root-cause body 前、非法 two-ruler fail-closed、
跨线程 blocked-reason 不误配，以及原 compact rank/frame-bundle head 行为；
`go test ./internal/tool -run 'TestTraceQuerySummary(CarriesTypedRelationAuthorityBeforeExploreClosure|SilencesInvalidTwoRulerCarrier|DoesNotInventBlockedReasonStateRelationAcrossThreads|PublishesTypedRootCauseRosterBeforeLongBody|UsesFrameBundleRankForCompleteHeadPreview)$' -count=1`
通过（1.024s）。此前同代码形态的完整 `go test ./internal/tool -count=1` 已通过
（159.274s）；措辞精化后的提交态完整复跑也通过（160.918s）。

后续任务：

- [x] B46-T10a：完成人工审计，纠正“semantic reviewer 被 observation-only skip”的误判；
- [x] B46-T10b：盘点现有 AnswerDocument/claim schema；先选用共享 typed carrier 的
  pre-Explore transport，暂缓 schema 扩张，不增加答案字符串硬门；
- [x] B46-T10c-a：补齐 pre-Explore 单源关系 authority 与正反单测；
- [x] B46-T10c-b：r5 若仍出现同类关系越权，再设计 model-authored structured relation
  claim 与精确 typed 校验；之后才评估 typed-complexity 驱动的可选 LLM reviewer，
  不得直接默认开启高延迟 reviewer；
- [x] B46-T10d：相关单测通过后同 pair r5；确认关系正确再进入 B47 多维批次。

#### B46 r5：跨阶段 transport 生效，但软提示不能约束模型关系声明

从干净 `main@0423e44fe478` 严格并行同一 Trace/write pair。runner 1/2 PASS，人工 1/2：

1. write runner/human 双 PASS。补丁仍只改 `memoclaw/client.py`，sync/async 都使用
   POST `/v1/search` + JSON body；verification probe 与真实 `make check` 都通过，后者输出
   `python text search contract ok`。ChangeReport=`passed`，Python changed path=`covered`，
   execution source=`declared_coverage_test_surface`；写模式继续无回归。
2. Trace runner 唯一失败是旧固定词形 `等待对象 dma_fence_default_w`；typed 内核调用点在
   projection/tree/detail 均完整存在，继续归 `EVAL-B46-ORACLE1`，生产代码不拟合。
3. 新 transport 已形成生产见证：root-cause tool result 的第 20–24 行、在长 body 之前，
   明确发布 `#4+#13=5.149ms` 自身墙钟尺、`#10=1.648ms` 唤醒边尺、跨尺禁加、物理关系
   unresolved；同时给出 row-local state scope、fix-direction 非加法权限，以及
   blocked-reason census 不能代替 typed interval join。说明模型上下文在结论形成前已精准且足量。
4. 人工仍 FAIL。Explorer closure 明知上述载体，仍把 #4 3.956ms 与 #10 1.648ms 拼成
   5.604ms，把 CompThread running 8.294ms 与 row-local D-state 3.598ms 拼成 11.892ms；
   Finalizer 收到同源准确 handoff 后继续沿用，还把 JankManager priority-inversion/runnable
   及 keva running/io_wait 各自跨行相加。最终系统附加的 Trace projection 则正确显示
   two-ruler 分账、CompThread 同源二分关系和 typed 小计 12.115ms，因而答案内部形成硬矛盾。
5. 显式用户窗、5 次 windowed trace query、自动补齐、真实占用/现规则可消两轴、根因排序、
   唤醒链和完整 Trace 因果投影全部保留。失败不是能力被窄化，而是模型在结构化证据面前仍
   越权声明关系；继续追加自然语言提示已无高 ROI。

结论：`EVAL-B46-REL2B` 的 transport 部分可判 covered；`EVAL-B46-REL3` 升为
`confirmed/P1`。下一批采用 model-owned structured relation claim：模型在 Explorer closure
和/或 AnswerDocument 中显式提交 seat identity、relation kind、typed carrier/subtotal 引用；
校验器只 against 同源 typed authority 做精确集合/数值比较。无 carrier 的 containment、
overlap、mutual-exclusion、addition 声明触发模型重试，由模型自己修正总结/答案。不得扫描
RawRequest、thinking、summary/final prose，不得由系统删除、替换或撰写结论，也不得启用
依赖固定词形的 hard gate。普通 semantic reviewer 仍非默认依赖。

- [x] B46-T10d-a：完成 r5 同 pair 严格并行与全链人工审计；
- [x] B46-T10d-b：确认 pre-Explore typed transport、显式窗投影/补齐、write 回归均生效；
- [x] B46-T11a：设计最小 model-authored relation-claim schema，并复用现有 types admission
  authority；先覆盖加法/包含/重叠/互斥四类，不按 case/type 造专用规则；
- [x] B46-T11b：Explorer closure 与 AnswerDocument 的 claim 必须由模型提交；typed 精确失败
  只触发模型重试，系统不改正文。补正反/absent/invalid-carrier/普通非 Trace 回归；
- [ ] B46-T11c：相关全量测试、独立提交推送后同 pair r6；通过后进入 B47。

#### B46 REL3 第一批：模型自有结构化关系声明（2026-08-02）

r5 已证明上下文“准确且足量”仍不等于模型会服从：继续增加自然语言提示不能解决
跨尺相加、row-local 拆分跨行拼接这一类关系越权。本批改为可校验但仍由模型拥有的协议：

1. 新增通用 `AnswerRelationClaim`，把 `physical_relation` 与 `addition` 分成两个正交轴。
   物理轴支持 `unresolved / mutually_exclusive / overlap / contains / contained_by`；算术轴
   支持 `authorized_to_published_subtotal / forbidden`，并用可选 `subtotal_value/unit`
   携带唯一已发布小计。这样“物理关系未知但禁止跨尺相加”不会再被错误压成“相互独立”。
2. 首批 authority producer 复用现有严格 typed 载体：self-runnable two-ruler 生成墙钟同尺、
   唤醒边同尺、跨尺边界三条权限；目标 state partition 生成闭合互斥/可加权限。
   authority ID 由 typed 内容指纹构造，使 `trace_query` 头部、Explorer ledger、Finalizer
   partition compile 在单/多工件下命名一致，不依赖本地化标题、投影顺序或 case ID。
3. `trace_query` 在长正文前输出每条 `relation_claim_required`。当 two-ruler typed record
   存在时，Explorer 的 `emit_investigation_complete.relation_claims` 必须完整承认三条权限：
   同尺只能按精确成员集合复现已发布小计；跨尺必须 `unresolved + forbidden` 且不得带小计。
   缺条目、伪 authority、错成员、错单位或错数值均精确拒绝，由 Explorer 模型重写自己的
   closure。校验不读取 `reason`，也不解析 RawRequest/thinking/summary/final prose。
4. accepted claims 进入 Mutable current/retained/fork/merge 生命周期，并在 Finalizer 的
   `Trace Decision Inputs` 中以 `accepted_model_relation_claims` 交接。模型必须把它们复制到
   使用这些值的 model-owned AnswerBlock `relation_claims`；full emit 与 patch 共用 normalizer、
   quarantine、deep clone 和 persist validator。缺失/漂移只让 Finalizer 重试，系统不删除、
   替换、重排或撰写模型正文；既有 model-owned wire-preservation 红线继续校验。
5. 硬门只由“typed authority 是否存在 + 模型结构化 claim 是否精确匹配”触发。普通非 Trace、
   没有 relation authority 的 Trace、写模式均零义务；显式时间窗 query、自动补齐、根因排序、
   唤醒链、真实占用/现规则可消两轴和最终 Trace 因果投影路径均未改动。
6. 施工时发现一个上下文精度 gap：`TraceCausalProjection.Active()` 是展示树谓词，不把
   projection side-channel 算作 active；若校验器只从 Active projection 取权限，极端的
   “two-ruler record 存在但无活跃树节点”会静默漏门。现在 completion/finalizer 从统一
   observation ledger 用同一个 strict parser 编译 side-channel authority，再与投影 authority
   按 content-stable ID 去重；展示活跃度不再决定关系证据是否存在。

本批不声称已抽完全部 renderer-only pair roster：SMR/AXIOM/RSPA 的 `overlap/contains`
authority producer 仍属于 `B46-T9b-c`；但 wire schema、生命周期、Explorer/Finalizer 校验与
重试协议已经泛化，不需要再为每种 relation/case 建新答案门。

验证：

- relation authority/错跨尺/缺 claim/content-stable ID、Mutable reset/fork/merge/deep-clone、
  full AnswerDocument persist、pre-Explore preview、Finalizer handoff 正反测试通过；
- `go test ./internal/types -count=1` 通过（18.397s）；
- `go test ./internal/agent -count=1` 通过（2.975s）；
- `go test ./internal/tool -count=1` 通过（161.615s）。

任务状态：

- [x] B46-T10c-b：r5 已确认软 transport 不足，启动 structured claim；
- [x] B46-T11a：通用 physical/arithmetic 双轴 schema、typed content-stable authority；
- [x] B46-T11b：Explorer closure + AnswerDocument model-owned claims，精确失败只触发模型重试；
- [x] B46-T11c-a：提交推送本批；
- [x] B46-T11c-b：从干净 HEAD 严格并行同 Trace/write pair r6 并人工审计；
- [ ] B46-T9b-c：后续抽取 SMR/AXIOM/RSPA renderer-only exact pair producer；
- [ ] B47：r6 验证后切换下一对高优先 read/data/log/operation cases。

#### B46 r6：model-owned 校验闭环成立，relation authority producer 覆盖仍不足

从干净 `main@c2e65b6c270f` 严格并行同一 Trace/write pair。runner 1/2 PASS，人工
1/2 PASS：

1. write runner/human 双 PASS。补丁只改 `memoclaw/client.py`，sync/async 都使用
   POST `/v1/search` + JSON body，保留可选 namespace；API reference 与测试未改。
   verification probe、真实 `make check` 均通过，ChangeReport=`passed`，changed path
   `covered/project_runner`，执行来源为 `declared_coverage_test_surface`。规划模型经历
   contract refs、精确 old-text 与 probe 结构的数轮自修，但没有扩大改动面。
2. Trace runner 仍只被旧固定词形 `等待对象 dma_fence_default_w` 判负；typed kernel
   callsite 在 projection/detail 中存在，继续归 `EVAL-B46-ORACLE1`，不修改生产逻辑拟合。
3. REL3 首批协议已形成端到端生产见证。Explorer 在 closure 中提交并获准三条结构化
   model-owned claim：`#4+#13=5.149ms` 墙钟同尺、`#10=1.648ms` 唤醒边同尺、三席
   cross-ruler=`physical unresolved + addition forbidden`。Finalizer 第一稿漏带 claims 被
   persist validator 拒绝；第二稿由模型自己复制 accepted claims，并把正文的跨尺合并改为
   禁止。日志 `finalizer_rewrites=0`，系统没有删除、替换或代写模型答案。
4. 显式用户窗、4 次 windowed query、确定性自动补齐、真实占用/现规则可消两轴、根因排序、
   唤醒链与完整 Trace 因果投影均保留。REL3 没有破坏已有 Trace 能力，write 也零义务。
5. Trace human 仍 FAIL，但失败边界已收窄为 authority producer coverage。模型对首批未建
   authority 的其它 pair 继续声称：target running 包含 runnable、IO latency 与 IO burst
   部分重叠、CompThread/JankManager 的链上与 adjacent 行存在包含/重叠/独立。至少第一条
   明确错误：closed target state partition 中 running/runnable 是物理互斥成员，却可按该
   精确分区相加重构总窗；“互斥”不等于“不可相加”。现有 finalizer 软提示其实已准确说明
   这两个正交轴，模型仍未服从。
6. 因此上下文审计结论分两层：two-ruler carrier 的上下文精准、足量且已能强制模型自修；
   其它 pair 没有 model-visible structured authority，只有 `typed_pair_only` 软边界，尚不足以
   支撑正确回答。不能通过扫描 RawRequest、thinking、summary/final 中的“包含/重叠”等词
   加硬门，也不能让系统重写结论。最优方案是复用现有 model-owned wire/validator，从 typed
   closed partition、same-source exact fold、interval join 与 renderer-only exact carriers
   批量铸造关系 authority。

登记 `EVAL-B46-REL4/P1`：关系协议已 covered，producer roster 不完整。下一施工批：

- [x] B46-T12a：确保 target closed state partition 始终铸造
  `mutually_exclusive + authorized_to_published_subtotal` authority，并固定
  “互斥但可加”的双轴正反回归；
- [x] B46-T12b-a：把 RSPA engine-minted same-source anchored/remainder exact
  bipartition 抽为统一 authority producer；
- [ ] B46-T12b-b：继续评估 interval overlap/contains producer；只有查询头、ledger、
  final handoff 能共享同一 typed identity/resolver 时才发布，无 carrier 保持 unresolved；
- [ ] B46-T12c：盘点并迁移 SMR/AXIOM/RSPA renderer-only 关系载体，避免 renderer 与
  model authority 使用两套判定；
- [ ] B46-T12d：全量测试、独立提交推送，然后同 pair r7；通过后转 B47，不长期停留单 case。

#### B46 REL4 第一批：closed target-state partition 双轴权限

本批先关闭 r6 最明确且最高杠杆的 producer 缺口，不一次把 renderer 全部关系判定搬入
wire：

1. target state authority 现在只有在五条 engine raw lane
   `running/runnable/sleep/non-IO d_state/io_wait` 非负、三位小数下精确求和为 typed total，
   且已有 projection anchor 时 account window 与选中窗口同窗、total 也精确闭窗时才铸造。
   renderer 可把 d_state+io_wait 合成一个人类可读 D-state 项，但不会改变五条 raw lane 的
   互斥/加法恒等式。失衡、错窗、partial account 均 fail closed。
2. closed partition 现在是 closure-critical authority：
   `physical_relation=mutually_exclusive` 与
   `addition=authorized_to_published_subtotal` 同时必填，成员固定为五条 raw lane，总值/单位
   精确复现 typed total。这样模型不能再用一个“不可相加”词混淆两个正交轴。
3. root-cause query 的 compact head 直接从 typed `TargetWindowStateAccount` 发布相同
   content-stable authority，保证 Explorer 在长 body 之前拿到；ledger/finalizer 仍从
   observation projection 编译，同一数据得到同一 ID。无平衡账时 preview 静默，不造权限。
4. Finalizer 校验从“只精确等于 Explorer accepted claims”提升为：所有 closure-critical
   authority 必须提交，同时 accepted claims 必须作为精确子集保留。由确定性自动补采在
   investigation closure 后新增的 closed partition 因此可由 Finalizer 模型追加；系统只比较
   结构化 metadata，不扫描/改写正文。
5. handoff 明确列出全部 required authority 与最终复制义务；`glossary_lint` 拒绝了首稿中的
   内部术语，已改成普通面向模型表述。没有读取 RawRequest、thinking、reason、summary/final
   prose，也没有按状态词扫描答案。

验证：

- target partition 平衡/失衡/同窗/错窗、compact head 正负、Explorer completion、Finalizer
  accepted-subset + post-closure supplement superset、handoff 与模型自有持久化测试通过；
- `go test ./internal/types -count=1` 通过（19.147s）；
- `go test ./internal/tool -count=1` 通过（163.326s）；
- `go test ./internal/orchestrator -count=1` 通过（11.139s）；
- 首轮 `internal/agent` 仅 glossary lint 拒绝内部词，改词后完整复跑通过（2.734s）。

状态：`B46-T12a=implemented/full-tests-pass`。按小批守战果先提交推送；下一批处理
same-source exact fold / interval carrier，再决定是否需要 r7，避免把单个 Trace case 变成长期
拟合场。

#### B46 REL4 第二批：RSPA 同源二分关系权限（2026-08-02）

本批完成 same-source exact fold 的高置信子集，但没有把 renderer 的 E# 互指判定照搬为
模型硬合同：

1. authority producer 直接读取 projection 的完整 `RankedSeats` typed roster。只有同一
   rank board 中恰好一条 `on_chain` 锚定席与一条 `adjacent` 余段席，且 subject、type、
   line envelope、`ChainAnchorFullMS/ChainAnchoredMS` 全部相同，才形成候选；renderer 的
   树 cap、fold、merge 和 E# 分配不参与判定。
2. 两席都必须带 engine-published effective value，且在生产 wire 的三位小数精度下分别
   精确等于 `anchored` 与 `full-anchored`。缺对席、跨板 lookalike、重复候选、错发布值、
   非 on-chain/adjacent 车道或 `ChainAnchorOwnershipDivergent` 均 fail closed；没有通过
   “同线程/同方向/相近数值”之类噪声猜关系。
3. 通过时铸造 closure-critical
   `same_source_partition`：`physical_relation=mutually_exclusive` 与
   `addition=authorized_to_published_subtotal` 同时成立，subtotal 只能复现 typed full account。
   member ref 使用两席角色 + subject/type + 精确行包络，不借用不稳定的渲染 E# 或跨车道
   裸 `#N`。
4. root-cause compact head 只镜像上述 producer 所需字段，再调用同一个 types compiler；
   observation ledger/final handoff 从投影调用同一 compiler。集成测试固定 raw engine float
   经三位 wire 后 authority ID、member set、双轴与 subtotal 在 pre-Explore / post-ledger 两端
   完全一致。
5. Explorer/Finalizer 仍由模型自行提交 `relation_claims`、自行修正和撰写可见结论；系统只
   校验结构化 metadata，不读 RawRequest、thinking、reason、summary/final prose，不删除、
   替换或新增模型正文。普通 read/write、无该 typed pair 的 Trace 均零义务；显式时间窗、
   自动补齐、根因排序、唤醒链、双分析轴与最终 Trace 因果投影接线未改。
6. 上下文审计结论：RSPA 的 typed carrier 已足以支撑“锚定+余段=全窗”结论；AXIOM-V2
   cross-direction overlap 虽有 typed interval-intersection roster，但当前同板/互指消歧仍在
   renderer 私有 row resolver。未先抽成共享 typed resolver 前，不把它升级为 hard authority，
   以免形成第二套判定或让模型在 Explorer 关账前看不到合同。

验证：

- same-source 正向、缺对席、错发布值、权属失合、跨板负向及 model-authored claim 校验通过；
- compact-head 见证与 head/ledger content-stable ID 一致性测试通过；
- `go test ./internal/types -count=1` 通过（20.696s）；
- `go test ./internal/agent -count=1` 通过（2.692s）；
- `go test ./internal/orchestrator -count=1` 通过（11.511s）；
- `go test ./internal/tool -count=1` 通过（162.527s）。

状态：`B46-T12b-a=implemented/full-tests-pass`；`B46-T12b-b` 与 `B46-T12c`
保留为共享 resolver 架构件，不为单个 H5 case 复制 renderer 逻辑。先提交推送本批，再按
优先级进入下一组双并发 eval；若后续 case 提供 typed interval-relation 真实 witness，再以
同一 producer 协议扩展。

### B41：data 终批材料 × Binder 方向语义审计（2026-08-02）

本批严格并行 `data_text_filter_count` 与 `trace_query_binder_ipc_peer`。runner
2/2 PASS，人工 1/2 PASS。data 最终值正确但先执行了一次已知不完整的计算；Binder
目标、事务号、直接唤醒边正确，但 fixture 与答案把 `reply=1` 的发射方向解释反了。

#### EVAL-B41-DATATERM1：终批执行前丢失显式用户材料义务（P1）

用户明确要求按 `instructions.md` 处理 `notes.txt`，初始 exact-path workflow
material floor 也包含两者。planner 却只把 `notes.txt` 放进 current required，把
`instructions.md` 降为 optional，并发出 `continue_after=false` 的
`custom_transform`。现有 staging guard 只读收窄后的 current-batch contract，故先
执行；result validator 才从 workflow contract 发现遗漏并触发 repair。最终答案
`2` 正确，但留下 `data_rounds=2 / repair_rounds=1 / action_failed=1`。

通用修点在 current-batch contract，而不是为某个文件名加特判：

1. 复用 `dataworkflow.PlanMayProduceFinalAnswer` 判断 typed final-answer producer；
2. 仅对 `continue_after=false` 且可能产出最终答案的批次，恢复 workflow contract
   中尚未覆盖的 explicit-user-material floor；
3. 恢复发生在 terminal scheduling guard 之前，因此不完整批次不再先执行；
4. 已消费材料不重复要求；derive/inspect 等中间批即使 legacy plan 没显式填写
   `continue_after`，也不被误扩为全材料批；
5. planner 同时获得 soft instruction：显式规则、说明、schema、example 都是
   workflow obligation，终批必须真实消费，非终批可通过 typed continuation 延后；
6. 不扫描答案 prose，不改写模型结论，也不把单个文件名或题型编码进 gate。

第一次施工只修复了 current-batch 声明面，r2 证明它仍是 partial：初始计划已经把
`instructions.md` 与 `notes.txt` 同时声明为 required/input，但实际 Python script 只
调用 `read_text("notes.txt")`。`dataTaskScheduledMaterialConsumption` 把声明
`input_paths` 当成实际消费，终批 staging 继续放行，直到执行后 result validator 才
失败。因此更深的 gap 是“声明消费≠可执行消费”，登记为
`EVAL-B41-DATAPREFLIGHT1/P1`。

B41 第二施工批使用 Python tree-sitter AST 对可执行结构做保守证明：只统计 data
runner 规范 reader helper（`read_text/csv_rows/tsv_rows/json_load/json_records/
jsonl_rows/open`）的 literal 第一参数；语法错误、动态 path、reader alias、转义/计算
literal 或无法证明的调用形态都返回 non-authoritative，硬门回退到原 declared-input
语义而不是误拒。该检查不读取用户原始文本、模型答案 prose、case/type 字面；它只读
准备执行的结构化 script。由此，声明了说明文件但脚本未读取的 terminal batch 会在
执行前进入 repair，不再制造 `action_failed`。

#### EVAL-B41-BINDERSEM1 / EVAL-B41-EVAL1：发射方向缺口与 fixture 自相矛盾（P1）

原 fixture 声称 client 发起同步 Binder 请求，但 raw row 是
`client ... binder_transaction ... reply=1`。确定性 `ipc_graph` 正确输出
`call_semantics=reply`；模型随后一边保留 REPLY，一边解释成“client 请求完成、server
正在返回”，把 row emitter 与 destination 的角色倒置。`sched_wakeup` 的
`binder:100_1-101 -> client-20` 直接边本身没有问题。

修复分成两个互不越权的面：

- fixture 改成 `reply=0 flags=0`，使“client 同步请求”与 typed
  `call_semantics=sync_request` 一致；
- perf pre-triage 与 finalizer 加同一条通用 soft guidance：transaction row 的线程
  是 emitter，destination 是 receiver；`reply` 只允许“emitter 正在回复”，只有
  typed `sync_request` + blocking authority 才允许同步阻塞请求叙述；后续 wakeup
  保持独立 typed edge，只有图连接时才合并故事。

没有新增用户/模型原文关键词门、emit reject、answer mutation 或系统结论替换；
显式时间窗的 Trace 因果投影、根因排序、唤醒链、可消除量与自动补齐路径不变。

#### B41 r2：有限关系事实被强制扩成全量因果报告（P1）

r2 的方向语义已经完全正确：`reply=0`、`call_semantics=sync_request`、目标
PID 100/TID 101、transaction 42、直接 waker `binder:100_1-101` 均一致。但最终
答案长达约 25.3K 字符，额外注入根因排序、主要时间占用、窗内可消除量、完整
Trace 因果投影和系统补采；用户只问三个有限事实，人工仍判 FAIL。

日志给出无歧义的接线 witness：analyzer 首次正确发出
`runtime_question_profile=bounded_fact_set`，source quote 正好是目标、transaction、
waker 三字段；`emit_analysis` 仅因 payload 同时含 call/relation shape 就 hard reject。
模型被迫重发 `relation_analysis/call_chain`，后续 full-report authority 与
assembly-time supplement 才扩出整份因果报告。登记
`EVAL-B41-RELWIDTH1/P1`。

架构裁定是“关系形状”和“答案广度”正交：一个 peer、transaction id、direct waker
仍可属于有限事实集；只有要求 caller/wakeup/IPC/dependency 的路径、拓扑或展开分析
时才用 `relation_analysis`。共享权限顺序调整为：

1. 显式 user time window；
2. typed runtime breadth：bounded 窄化，causal/broad relation/overview 扩宽；
3. legacy typed call/relation shape；
4. legacy fallback。

显式时间窗仍是第一权限，所以带具体窗的根因排序、唤醒链、可消除量、因果投影与
自动补齐完全不受影响；无窗口但明确要求 causal diagnosis 或 broad relation 的请求
也继续获得完整报告。本修复不扫描 RawRequest/answer prose，不做 answer mutation，
系统只选择需要交给模型的 typed evidence/report breadth。

#### B41 r2：planner 成功标准被 evaluator 当成真值（P2 watch）

data 修复计划自己在 `success_criteria` 幻觉出预期计数 `(3)`，实际执行结果为正确的
`2`。evaluator 把 planner-authored criterion 当成独立 ground truth 并建议 blocked；
确定性 completion gate 根据已执行结果、材料覆盖和输出合同将终态归一为 complete。
该 control-plane 裁定正确，不能让模型自写的 criterion 否决 typed 结果。只增加 soft
guidance：goal/success_criteria 是流程意图，不是数值权威；只有用户材料、确定性
result/metrics、贡献账本或 reconcile 可支撑 expected value。保留现有确定性终态门，
不改写最终答案值。

#### B41 r3：共享宽度权限仍有两个 control-plane 旁路（P1）

r3 runner 仍为 2/2 PASS，人工为 1/2 PASS。data 最终值 `2` 正确；初始 terminal
script 虽声明两个材料却只读取 `notes.txt`，新的 AST 消费证明在任何执行前拒绝，repair
脚本真实读取 `instructions.md + notes.txt` 后一次执行成功。指标从 r2 的执行后失败收敛为
`data_rounds=1 / repair_rounds=1 / action_failed=0`；evaluator 也不再用 planner 自写的
expected value 阻断。因此 DATAPREFLIGHT1 与 DATAEVAL1 在该生产回放关闭。

Binder analyzer 第一次就接受 `runtime_question_profile=bounded_fact_set`，模型仅执行
`ipc_graph + binder_transaction event_search + sched_wakeup event_search`，正文三个事实及
方向语义均正确，且结构化 Trace 因果投影已经消失。但答案仍约 15.3K，日志证明两条旧
接线绕过 shared report authority：

1. `traceSupplementViewsForRequest` 在非 D-state 分支仍按缺失 family 无条件选择
   `root_cause_rank + critical_blocking_calls`；
2. `traceObservationDrillRetryLensActive` 仍把 legacy call/relation shape 当成全量钻取
   合同，pre-finalize floor 发出 `window_sweep + heavy views` 覆盖债。

分别登记 `EVAL-B41-SUPPBREADTH1/P1` 与 `EVAL-B41-FLOORBREADTH1/P1`。二者同根修复：
先消费 `types.RuntimeTraceReportShapeAuthority`，`decided && !allowed` 时禁止扩到 full
causal contract；D-state 的 typed state-only 特例继续只补 `window_stats`；显式用户时间窗
仍以最高权限穿过 guard，保留根因排序、唤醒链、可消除量、因果投影及自动补齐。决策
只读 typed enum/profile，不读 Binder 字样、RawRequest 或模型/答案 prose。

#### B41 r4：breadth profile 与诊断 typed lanes 自相矛盾（P1）

r4 runner 2/2 PASS、人工 1/2 PASS。data 的 AST 执行前 guard 连续生效，仍是一次 repair、
一次执行、最终严格值 `2`。evaluator 对“当前批 freeform”与“workflow effective strict
contract”短暂混淆并重复权衡，但 deterministic projection/terminal gate 正确，最终无
可见错误；按用户要求记为 P3 模型波动 watch，不在高优先级批上做答案拟合。

Binder 这次没有复现 r3 的 `bounded_fact_set`：analyzer 首次发出
`runtime_question_profile=causal_diagnosis`，同时发出 `intent=trace`、
`scenario=generic`、`predicates.is_diagnostic_question=false`、diagnostic profile 四腿全
false。共享 report/supplement/floor authority 因而按设计放行，系统补采与完整因果投影
再次出现；这不是 T11/T12 旁路回退，而是上游 breadth enum 与其余 typed 诊断面自相
矛盾。登记 `EVAL-B41-PROFILECOHERENCE1/P1`。

最优通用方案是 emit-time schema coherence，而不是 case oracle 或下游答案接管：

- `causal_diagnosis` 必须至少有一个 typed attribution carrier：root-cause intent、diagnostic
  predicate/profile、root-cause/performance scenario；
- 全部为 negative 时 fail-loud，提示有限字段用 `bounded_fact_set`、实际路径/拓扑用
  `relation_analysis`；由 analyzer 重发，不静默归一成系统想要的 scope；
- `bounded_fact_set` 与 call/relation 仍可共存；显式时间窗正权限不变；
- gate 只读 schema-validated enum/boolean，不读 source quote 语义、RawRequest、模型推理或
  最终答案，不包含 Binder/type/case 字面。

这也回答“看护用例是否过硬”：该 eval 仍只核对用户要求的三个可见事实，没有新增
`runtime_question_profile=bounded_fact_set` 的 case-specific hard oracle；生产 hard gate
约束的是所有 runtime 请求共有的 typed schema 自洽性，不要求模型给出某个样例答案。

#### B41 r5：完整扩面关闭，残留状态主值相关性与 data 路由目标漂移

r5 runner 1/2 PASS、人工 1/2 PASS。Binder analyzer 首次发射因无 value 的可选
artifact profile 被既有门拒绝，第二次正确发出 `bounded_fact_set`；模型只跑
`ipc_graph + wakeup_chain`。日志无 pre-finalize heavy debt，system supplement 明确
`skip reason=families_present`，最终无根因排序、可消除量或 Trace 因果投影。三个用户
事实和方向语义正确，说明 RELWIDTH/SUPPBREADTH/FLOORBREADTH/PROFILECOHERENCE 的
主链已关闭。

但 final answer 仍追加一条与三个 IPC 字段无关的 `client-20` 五态 typed 事实。根因是
`runtimeTracePrincipalValueMaterializationAllowed` 把所有 `bounded_fact_set` 都视为
target-state/target-wait principal authority；`bounded` 只说明答案宽度，不能说明事实种类。
登记 `EVAL-B41-PRINCIPALREL1/P1`。共享修复：完整 causal report 可带状态主值；窄报告
只有 `IsFocusedRuntimeFactQuestion`（typed target + state/condition/mechanism，非 call
relation）可带。tool 的确定性块与 agent 成文前 typed recap 消费同一 types authority；
finite IPC peer/transaction/waker 不再收到状态卡。无 RawRequest/answer prose 扫描。

data 本轮没有进入 data workflow：router 在 reasoning 中反复争论 txt 应属于 data 还是
operation，最终选 operation；虽然 `cat + grep -c` 得到 2，operation finalizer 输出标题、
解释、列表和代码块，违反“只输出一个数字”，runner 正确 FAIL。登记
`EVAL-B41-DATAROUTEOBJ1/P1`。这不是 AST/material guard 回退，而是路由把打开文件的
手段误当成任务目标。修复保持 soft guidance：

- route 按 objective，不按是否需要 cat/grep/awk；
- 读取 rules/instructions + 本地输入，计算/filter/count/reshape 派生值，即使是 txt/md
  也属于 data；
- operation 保留给用户显式要求 computer/file operation、机器/文件系统状态、side
  effect，或只做原样检索/展示而不产生派生数据结果；
- 保留“请作为电脑操作读取文件”既有正例，不加 filename/extension/原文关键词硬门。

#### B41 r6：路由收敛；`bounded` 仍缺事实种类轴（P1）

r6 runner 2/2 PASS，人工 1/2 PASS：

- data 已稳定走 `route=data`；terminal script 首次仍漏读 `instructions.md`，AST guard
  在任何执行前拒绝，repair 后真实读取两份材料，指标为
  `data_rounds=1 / repair_rounds=1 / action_failed=0`，最终严格只有 `2`。
  `EVAL-B41-DATAROUTEOBJ1` 关闭。
- Binder analyzer 正确接受 `runtime_question_profile=bounded_fact_set`，仅执行一次
  `wakeup_chain`；system supplement 为 `skip reason=families_present`，没有 pre-finalize
  heavy debt，也没有根因排序、可消除量或 Trace 因果投影。模型正文三个事实、Binder
  emitter/receiver 方向与同步语义均正确。
- 但最终仍追加 `client-20` 五态“目标线程状态与等待明细”，随后系统校验附注又追加
  同一五态 typed 事实。说明 T20 的 shared predicate 接线已到位，但谓词语义仍过粗：
  `IsFocusedRuntimeFactQuestion` 把“typed target + 非诊断 mechanism”统一视为状态事实；
  `bounded_fact_set` 只回答“答案有多宽”，没有回答“用户要哪类主值”。

登记 `EVAL-B41-FACTFAMILY1/P1`。通用修复在既有 typed profile 上增加正交的
`fact_families` 闭集，而不是从 Binder 字样、trace_query view 或最终正文反推：

1. bounded scope 必须声明一个或多个 principal fact family：
   `target_scheduler_state / target_wait_occurrences / recorded_reason /
   occurrence_time / count_or_duration / relation_peer / transaction_id /
   direct_waker / resource_pressure / frequency_residency / other_observed_value`；
2. peer + transaction + direct waker 不授予 target-state/target-wait 主值；只有显式
   `target_scheduler_state` 或 `target_wait_occurrences` 才能在窄报告中追加状态/等待卡；
3. tool 确定性状态块、agent pre-finalize recap、orchestrator 尾部 per-thread/state
   juxtaposition 共用同一 types authority；尾部不能再通过扫描模型提到的线程名绕开
   typed 用户意图；
4. 老序列化 RequestModel/合成 fixture 的空 family 保留 legacy fallback；新 analyzer
   的 bounded emission 对空 family fail-loud 重试，避免生产请求进入粗粒度兼容臂；
5. causal/relation/overview full report 与显式 user time window 继续允许状态主值；显式窗
   仍是最高权限，根因排序、唤醒链、窗内可消除量、Trace 因果投影和自动补齐完全不变；
6. 该 schema 只提供精确信息面选择，不生成、删除、替换或改判模型结论；不扫描
   RawRequest、source_quote 语义、thinking/summary/final prose 或 case/type 字面做硬门。

第六施工批已完成 schema/parser/prompt/shared authority 与两条发布路径接线。定向测试
覆盖 bounded IPC 负臂、target-state 正臂和 explicit-window/full-report 兼容臂；完整回归
通过：`internal/types` 19.669s、`internal/tool` 162.574s、`internal/skill` 1.219s、
`internal/agent` 2.930s、`internal/orchestrator` 14.610s、`internal/repl` 34.270s，
`make` 通过。下一步从干净提交重建并做 r7 同对回放。

#### B41 r7：事实族轴已在场，但 state/wait 主值权限仍被合并（P1）

r7 使用干净 `main@d9c2fd150`，runner 2/2 PASS，人工 1/2 PASS：

- data route 与最终值均正确，输出严格只有 `2`；本轮 planner 多次跨 dependent DAG
  ranks，被确定性 guard 分批收敛，最终
  `data_rounds=6 / repair_rounds=3 / wall=149s / reconcile=pass`。这是效率模型波动
  watch；不为单一简单计数增加 plan/action 特判。
- Binder 仍无 full supplement、heavy floor、根因排序、可消除量或因果投影；模型三个
  principal facts 正确。analyzer 发出
  `fact_families=[target_wait_occurrences,recorded_reason,direct_waker]`：它把“直接唤醒者”
  误带成 wait occurrence，但没有发 `target_scheduler_state`。
- 当前 `RuntimeTracePrincipalValueMaterializationAllowed` 把
  `target_scheduler_state OR target_wait_occurrences` 合并成一个布尔权限；tool 状态块、
  agent recap 和 cross-check 只要该布尔为真就同时拿到 state partition。因此事实族
  schema 本身生效，但 consumer 又把两个正交 family 合并，状态卡仍误扩。

`EVAL-B41-FACTFAMILY1` 保持 P1/partial，施工修正为三条独立权限：

1. `RuntimeTraceTargetStateMaterializationAllowed` 只接受
   `target_scheduler_state`，只发布 running/runnable/sleep/D/io_wait 分区；
2. `RuntimeTraceTargetWaitMaterializationAllowed` 只接受
   `target_wait_occurrences`，只发布完整 occurrence roster/count/wall-clock；没有 typed
   wait roster 时不得回退到 state partition；
3. full causal report 与显式 user window 对两类权限均为 true；legacy 空 family 仍走旧
   compatibility；共享 union 仅留给确实同时承载两类主值的兼容调用方。

tool materializer 与 agent recap 先分别计算 state/wait authority，再分别构造 rows；
orchestrator per-thread/state juxtaposition 只消费 state authority。prompt 同时明确
`target_wait_occurrences` 是用户要求 scheduler wait 区间的次数/清单/总墙钟，不等于
direct waker、wakeup event 或 wakeup latency。这是 typed 语义澄清与权限分离，不扫描
用户原文或模型正文，不硬拒绝模型结论，也不替换模型答案。

完整无缓存回归通过：`internal/types` 19.821s、`internal/skill` 1.013s、
`internal/agent` 2.857s、`internal/orchestrator` 12.962s、`internal/repl` 33.435s、
`internal/tool` 最终复跑 158.503s；`make` 通过。首次 tool 全包仅命中旧结构 tripwire
仍要求 union helper，更新为同时 pin state/wait 两条真实调用后，全包无其它失败。

#### B41 r8：FACTFAMILY1 关闭；调度转移区间仍被模型混称（P1）

r8 使用干净 `main@d1098e065`，runner 2/2 PASS，人工 1/2 PASS：

- data 44s，`data_rounds=1 / repair_rounds=1 / action_failed=0`，最终严格只有 `2`；
  r7 的 149s/6 rounds 未复现，确认为 planner 效率波动 watch。
- Binder 最终已经没有 target-state card、状态 typed 附注、full supplement、heavy
  floor、根因排序、可消除量或 Trace 因果投影；target PID/TID、transaction=42、direct
  waker 三项均正确。analyzer 本轮只发 `transaction_id + relation_peer`，但即使沿 r7
  的 wait-only profile，新的分面权限回归也保证不会回退成 state partition。
  `EVAL-B41-FACTFAMILY1=covered`。
- 新的人工 FAIL 来自模型额外派生的未请求时长：正文先正确写
  `sched_wakeup@3.020`，随后把 `3.015→3.030=15ms` 称为“睡眠到被唤醒”。真实三阶段是
  `t_sleep=3.015 → t_wake=3.020` 的 5ms sleep/blocking，随后
  `t_wake=3.020 → t_run=3.030` 的 10ms runnable scheduling delay；15ms 只能称为
  sleep-entry 到 switch-in 的总 non-running，不能叫 wakeup latency/sleep-to-wakeup。

登记 `EVAL-B41-TRANSITIONPHASE1/P1`。这与既有 Z4/B26 的“pre-wakeup dependency 与
post-wakeup runnable 是互斥阶段”同类，但本轮只使用 `event_search`，没有触发专用
wakeup census authority，因此最终通用 scheduler prompt 缺少三时间点的区间命名。
最优小修是把以下不变量放到所有 runtime-trace finalizer 的 soft guidance：

1. `sched_switch prev_state=S|D @ t_sleep` = 进入 sleep/blocking；
2. `sched_wakeup|sched_waking @ t_wake` = 离开等待并进入 runnable；
3. `sched_switch next_pid=target @ t_run` = switch-in running；
4. 三个差值必须分别命名，total non-running 不得冒充 wakeup latency；bounded facts
   未请求 duration 时不要仅因时间戳可见而主动派生。

该修复只给模型精确的通用调度语义，不扫描用户/答案原文，不做 regex 硬门，不拒绝、
删除、替换或改写模型结论。`internal/agent` 完整回归 2.866s、`make` 通过。

同时登记 `EVAL-B41-CITEINFO1/P3-watch`：runtime citation rows 被正确重定向为正文
`artifact_spans` selector 后，尾部仍写“3 处引用已移除”，虽然技术上指 citation pool
条目而非 selector，用户可读性较差。本批不以显示文案项阻塞 P1 调度语义收口。

#### B41 r9：转移区间收口；优先级错分属于已有 typed 权威下的模型波动

r9 使用干净 `main@6c8756050`，runner 2/2 PASS，人工 1/2 PASS：

- data 42s，`route=data`，一次执行、一次执行前 material repair，最终严格只有 `2`；
- Binder 请求的目标 PID 100/TID 101、transaction=42、direct waker
  `binder:100_1-101` 与 emitter/receiver 方向全部正确；没有 target-state card、full
  supplement、heavy floor、根因排序、可消除量或 Trace 因果投影；r8 的 15ms
  sleep-to-wakeup 误称也未复现，`EVAL-B41-TRANSITIONPHASE1=covered`；
- 模型主动扩写了用户未请求的 `prio=53 属于 CFS`，但同一轮在三个独立输入面已经
  明确携带 `prio=53/ohos_rt`：perf typed observation、`trace_query` JSON/event row，以及
  finalizer 的 Harmony `1-40=CFS / 41-159=RT / >159=raw` 提醒。工具输出和 prompt 均无
  事实缺口，模型仍违反精确信号。

因此将该现象登记为 `EVAL-B41-PRIOWAVE1/P3-watch`，不再施工产品硬门。若为它增加
正文扫描、emit reject、系统替换 CFS/RT 文案，都会重新越过“模型负责结论、系统只给
准确事实和引导”的红线；而为具体 53 加例子也属于单值过拟合。后续若在不同模型、
不同优先级值、且已经携带 typed class 的多个高优先级 eval 中重复，再考虑提升为模型
遵循性/供应商路由问题，仍不得由系统改写答案。

本轮 finalizer 因 artifact selector 完整性要求发生一次 patch，并把内部
`artifact_spans=...` 形式直接写进正文；归入既有 `EVAL-B41-CITEINFO1/P3-watch`，不阻塞
B41 主机制收口。analyzer 5 次尝试、总耗时 171s 也记为效率 watch，不为该 case 增加
分类特判。

任务：

- [x] B41-T1：严格并行 2 case，完整日志/答案人工审计；
- [x] B41-T2：终批 typed material-floor 恢复与多批/历史消费回归；
- [x] B41-T3：Binder 方向 soft guidance 与同步请求 fixture 校正；
- [x] B41-T4：`internal/repl`、`internal/dataquery`、`internal/dataworkflow`、
  `internal/skill`、`internal/agent` 无缓存全包与构建通过；
- [x] B41-T5：首批提交推送后同对严格并行 r2 回放并人工审计；方向语义修复生效，
  发现 DATAPREFLIGHT1/RELWIDTH1/DATAEVAL1；
- [x] B41-T6：terminal script literal-consumption AST proof + 非权威形 fail-open；
- [x] B41-T7：runtime breadth 与 relation shape 解耦，显式时间窗第一权限回归 pin；
- [x] B41-T8：evaluator success-criteria 非真值 soft guidance；
- [x] B41-T9：第二施工批完整相关包测试与构建通过：`internal/repl` 34.739s、
  `dataquery` 2.760s、`dataworkflow` 0.707s、`skill` 1.626s、`agent` 4.493s、
  `types` 20.312s、`tool` 159.156s，`make` 通过并已提交推送；
- [x] B41-T10：同对严格并行 r3 回放并人工收账；data human PASS，Binder 方向与模型
  正文 PASS，但 control-plane 宽度旁路导致整体 human FAIL；
- [x] B41-T11：system supplement 接 shared typed breadth authority；有限非 D-state 事实
  零补采，D-state state-only lane 保留，显式时间窗 full supplement 正臂保留；
- [x] B41-T12：pre-finalize trace-drill lens 接同一 authority，bounded relation 不再收到
  heavy causal coverage debt，显式时间窗正臂保留；
- [x] B41-T13：补 helper 与真实 `RunTraceQuerySystemSupplement` 接线 pin，以及 floor
  bounded/explicit-window 对偶 pin；定向测试通过；
- [x] B41-T14：完整 `internal/tool` 160.072s、`internal/orchestrator` 9.899s
  回归及 `make` 通过；第三施工批提交推送；
- [x] B41-T15：同对严格并行 r4 回放并人工收账；data human PASS，Binder 因 analyzer
  profile 自相矛盾导致 human FAIL；
- [x] B41-T16：增加 runtime breadth typed schema coherence gate；拒绝不带任何诊断/
  attribution carrier 的 `causal_diagnosis`，保留 bounded relation 与真实 performance
  diagnosis 正臂；
- [x] B41-T17：helper 正反臂与 `EmitAnalysis.Execute` 真实接线测试通过；
- [x] B41-T18：完整 `internal/tool` 158.620s 与 `make` 通过；第四施工批提交推送；
- [x] B41-T19：同对严格并行 r5 回放并人工收账；Binder 主扩面关闭，发现
  PRINCIPALREL1；data 路由漂到 operation，发现 DATAROUTEOBJ1；
- [x] B41-T20：types 增加 target-state principal-value 共享 authority，tool/agent 两发布
  面接线；bounded IPC relation 负臂、focused state 与 explicit-window 正臂；
- [x] B41-T21：router schema/system prompt 增加 objective-vs-mechanism 通用 soft guidance，
  txt/md rules+input 计算与显式 computer-operation 对偶示例；无 hard input scan；
- [x] B41-T22：完整 `types` 19.817s、`tool` 162.127s、`agent` 3.219s、
  `repl` 32.531s 与 `make` 通过；第五施工批提交推送；
- [x] B41-T23：同对严格并行 r6 回放并人工收账；data human PASS，Binder 主链 PASS
  但两个状态发布面仍误扩，登记 FACTFAMILY1；
- [x] B41-T24：runtime question fact-family typed schema + bounded 非空校验；tool/agent/
  orchestrator 状态发布面共用精确 family authority，legacy 空 family 仅作兼容；
- [x] B41-T25：完整相关包测试与构建通过；第六施工批提交推送；
- [x] B41-T26：干净 HEAD 重建后同对严格并行 r7 回放并人工收账；data human PASS，
  Binder 发现 state/wait family consumer 合并 gap；
- [x] B41-T27：拆分 state partition 与 wait roster 两类 materialization authority，
  tool/agent/orchestrator 分别消费；补 wait-only/state-only/full-report 对偶 pin；
- [x] B41-T28：完整相关包回归与构建通过；第七施工批提交推送；
- [x] B41-T29：干净 HEAD 重建后做 r8 同对回放并人工收账；FACTFAMILY1 covered，
  新增 TRANSITIONPHASE1 与 CITEINFO1 watch；
- [x] B41-T30：所有 runtime-trace finalizer 增加三阶段 scheduler transition soft
  guidance，不增加 hard gate/answer mutation；agent 全包与构建通过；
- [x] B41-T31：提交推送后做 r9 同对回放；TRANSITIONPHASE1 与 FACTFAMILY1 均
  covered，请求主事实正确；额外优先级错分确认为 typed 权威已在场时的模型波动，
  登记 PRIOWAVE1/CITEINFO1 watch，不增加硬门或答案改写。

### B40：analyze retry 回放 × blocked-reason Trace 语义审计（2026-08-02）

本批严格并行 `read_combo_analyze_retry_anchor` 与
`trace_query_blocked_reason_chain`。runner 为 1/2 PASS，人工为 0/2 PASS。前者证明
B39 empty-IR join 修复已生效，但模型仍误读 producer/attempt/phase 边界；后者是
runner false PASS，暴露调度驻留语义、跨主体因果权限和 eval ground truth 三者错位。

#### EVAL-B40-TRACESEM1：等待驻留被写成 CPU 占用并跨主体确权（P1）

确定性查询只证明：worker-30 在 2.030–2.150s 为 `io_wait` 120ms，caller 为
`fscache_page_wait_on_page_bit`。`wakeup_chain(pid=30)` 没有 edge，确定性投影明确
`chain_shape=flat_untraceable`。模型却写成 worker 在 io_wait 时“持续占用 CPU”，并
进一步断言它导致 main-20 的 doFrame 等待；还把 `R+` 切出说成主动 yield，把只有
blocked-reason 记录的 waker-10 说成唤醒者。

这是两条通用语义边界：

1. S/D/io_wait 是非 running 驻留，不能称为占用 CPU；
2. 一个主体的 wait 与另一个主体的慢 span 重叠，不等于二者存在因果。跨主体归因
   必须有 typed wakeup/IPC/lock/flow/dependency connector，且区间时间相容。

施工只增加两处 soft guidance：perf pre-triage 在铸造 navigation observation 前看到
同一规则，finalizer 在模型成文前再次看到；不扫描用户或模型正文，不增加 emit
reject，不删除、替换或追加系统结论。显式窗 Trace 因果投影与自动补齐保持原样。

#### EVAL-B40-EVAL1：fixture 叙事 ground truth 过硬（P1）

case 注释原称“frame stalls because a worker went into D sleep”，但 fixture 没有
worker→main 的 wakeup/IPC/lock/flow edge，自动 oracle 也只验证 worker/state/iowait/
caller。这是注释先验与可验证合同矛盾。已把注释改为“slow frame 与 worker wait
重叠，只证明 worker 自身等待，跨主体关系未证”，不增加最终答案关键词禁令。

#### B39-T5 replay 与 EVAL-B40-READMODEL1（P2-watch）

新 join 已被模型正确读到：只有 clean `StageOutput` + 非 nil IR + 非空 TaskGraph 才
成功，故 `ANZERO1=covered`。但同案答案仍有四处模型层错误：

- missing emit 实际返回非 nil `StageOutput{Error: ...}`，不是 nil output；
- semantic attempt 在 auto-correction 前已递增，修正只是不再 dispatch，不是本次
  attempt 未消费；
- 动态增量乘 `SubTopicRetryBudgetExtra`，不是恒定每两子话题 +1；
- analyze 先由外层 `Run` 执行并安装 degraded IR，`runTaskGraph` 后续只消费它；
  最终还漏写 `runTaskGraph`，runner FAIL。

source localizer 已提供 `internal/agent/analyzer.go`，模型没有读取 producer。typed
代码合同与证据入口均在，当前按模型 evidence-selection 残余观察；不为一个题面写
固定答案，不用答案关键词门，也不由系统改写结论。

任务：

- [x] B40-T1：严格并行 2 case，完整日志/答案人工审计；
- [x] B40-T2：纠正过硬 fixture 注释，保留原 typed 事实 oracle；
- [x] B40-T3：perf-triage + finalizer 两处通用 soft guidance；
- [x] B40-T4：`internal/skill`、`internal/agent` 无缓存回归与 CGO build 通过；
  提交推送待收口；
- [ ] B40-T5：转入下一组不同模式的 2-case eval。

### B39：真实 write/apply × analyze retry read 审计（2026-08-02）

本批严格并行 `github_issue_dayjs_duration_nan_symptom` 与
`read_combo_analyze_retry_anchor`。runner 2/2 PASS，人工 1/2 PASS。

#### B39-write：真实 symptom-only 修复通过

模型从 `PT1H` 症状定位到正则缺失 capture 经 `Number(undefined)` 变成 NaN，计划和
实际 patch 都只修改 `src/duration.js` 一行，将缺失分量折为 0；已有 PT1H 回归测试
本来就是正确期望，未修改测试绕过。npm runner 不可用后 verifier 按 typed test
surface 升级到 `make check`，Python 行为 oracle 检查生产表达式仍做数值转换、具备
missing-component guard，并钉住 PT1H 的 parse/render 断言。补丁 1+/1-、目标路径、
审批、applied commit 与 changed-path coverage 一致，人工 PASS。

#### EVAL-B39-ANZERO1：非 nil 零值 AnalysisIR 被成功 join 接受（P1）

read 答案命中所有 case 关键词，但人审代码后为 FAIL：

1. `runAnalyzePhase` 注释承诺不接受 nil 或 zero-value IR；旧成功条件却只有
   `err==nil && out!=nil && out.Error=="" && out.AnalysisIR!=nil`。
   `&types.AnalysisIR{}` 因指针非 nil 会直接返回成功，后续 `runTaskGraph` 才因空
   TaskGraph fail closed。这是生产代码/注释的真实矛盾，不是模型波动。
2. final 答案声称 runAnalyzePhase 还校验 QualityGate；代码没有这一步。gate 失败是
   上游以 `StageOutput.Error` 带到 join，runAnalyzePhase 不应复制一套 quality gate。
3. `MaxRetriesPerStage` 经 `dynamicAnalyzeRetries` 后直接作为 `attempt < max` 上限，
   在该函数中是总 semantic attempts，不是「初次尝试之外再重试 N 次」。transport
   retry 另有预算且不消耗 semantic attempt。
4. missing-emit/quality exhaustion 后的 explicit degraded IR 由外层 `Run` 安装；
   `runTaskGraph` 只消费已安装 IR，并对 nil/empty graph 防御失败。答案把责任归给
   runTaskGraph，层次错误。

通用修复只改 typed join：新增 `analyzeStageOutputUsable`，要求 clean StageOutput
同时具有非 nil IR 和至少一个 TaskGraph node；同一谓词复用于「stream error 但已有
可用 IR」保留臂与正常成功臂。空 IR 产生精确 lastErr、消耗 semantic attempt，预算
内可被后续 clean IR 替换；耗尽则 fail-loud，让既有外层 recovery 负责降级。批准计划
的 apply/verify `--plan-file` stub fast path 在循环前返回，是唯一显式例外。

该修复不扫描请求/答案 prose，不重做 QualityGate，不修改模型结论，也不触碰 read
scheduler、write controller、Trace 投影或补采。

测试：

- 首轮空 IR、第二轮 clean graph：必须两次 attempt 且最终提升 clean IR；
- 连续空 IR：必须耗尽并在 error 中披露 empty AnalysisIR/TaskGraph；
- clean retry、transport retry 原行为保持。

任务：

- [x] B39-T1：2 case 并行执行与完整人工审计；
- [x] B39-T2：write patch/report/test surface 核验；
- [x] B39-T3：ANZERO1 typed join 修复与定向测试；
- [x] B39-T4：orchestrator/agent/types 无缓存全包与构建通过；提交推送待收口；
- [ ] B39-T5：read 同案重放，人工核对 attempt/recovery 层次。

状态：`write=human-pass`；`ANZERO1=implemented/full-related-tests-pass/commit-next`。

### B38：日志显式错误身份 × data 结果通道审计（2026-08-02）

本批严格并行 2 case：`logtri_goroutine_dump` 与
`data_jsonl_filter_count`。runner 为 2/2 PASS，人工为 1/2 PASS。两案都不是
特定错误名或数字的拟合，而是 typed 载体在「观测次数/结果信封」边界上的通用
契约错位。

#### EVAL-B38-LOGOCC1：线程快照被提升为显式错误（P1）

日志只有一个 `fatal error` header，后接 goroutine 15 的 throw/mapassign/
writeSession 栈；goroutine 87 与 120 只有 `[running]` 栈块，没有自己的 panic/error
header。正确权限是：Errors 只有一个显式错误；87/120 是
`thread_snapshot`，只证明采样时执行到了对应 frame。

本轮 triager 无视已有 soft prompt，把同一 verbatim message 复制到 3 个 peer
Errors，并把三线程合成 diagnostic runtime_event。下游虽然已有 thread-snapshot
权限说明，但错误 typed 输入让 analyzer/explorer/finalizer 一致升级为「三者同时
panic、都写同一 map」；最终又在源码未解析时断言 `writeSession` 缺少 mutex。
runner 的宽关键词 oracle 只看 `map`/并发/竞争，未识别该事实身份错误。

通用修复分三层：

1. `errors[].message` 已要求来自附件逐字文本；在同一精确信号上增加基数对账：
   若 N 个 peer/cause error 携同一 message，附件中该精确 message 至少出现 N 次；
   超发拒绝 typed emission，并明确把无独立 header 的线程块放入
   `thread_snapshot`。不解析语言、错误词、用户请求或最终答案。
2. message provenance、message cardinality、observation evidence provenance 一次
   返回全部 violation，避免有限 triager retry 被逐项耗尽。
3. 将 `LogBundle.Errors` 与 prompt 标题中误导性的 `parallel snapshots` 统一改为
   `explicit error occurrences`；真正 snapshot 只属于 Observations。

该 gate 只校验模型申请进入「直接 runtime 事实」车道的结构化字段，属于
verbatim 精确信号硬门；不替模型写最终结论，也不扫描用户/模型 prose。两个相同
message 在附件中真实出现两次的多错误正例保持通过。

#### EVAL-B38-DATARESULT1：非对象结果通道与 Go Result 信封不一致（P1）

data 最终答案 `2` 正确且材料覆盖完整，但过程发生两次 repair：第一次模型给
`custom_transform` 却漏 script；第二次脚本已正确计算 count，却调用 `emit(count)`，
Python runner 输出裸 JSON number，Go 侧固定按 `dataquery.Result` object 解码而失败。
模型改成 `emit_result(count)` 才完成。missing script 已被 typed plan guard 精确修复，
本轮先按模型计划波动记档；裸值失败则是公共结果通道的确定性契约 gap，因为
plan guard 和 runner 文案同时允许 `emit(...)` 或直接 `result` 赋值。

通用修复在唯一 Python→Go runner 边界做无语义规范化：

- 非 dict scalar 转成 `{answer: str(value)}`；
- list/tuple 转成紧凑 JSON answer；
- consumed_paths 随标准 Result 信封继续交付；
- dict 保持原语义：可能是完整 Result，也可能是由 output_contract 决定是否提升的
  普通 JSON payload；账本、规则和业务判断不由系统补造。

测试覆盖 `emit(2)`、直接 `result="ok"`、`emit(["a",2])`，并保留既有
`emit_result` 信封、普通 dict payload 与 ledger merge 行为。

施工任务：

- [x] B38-T1：严格并行 2 case、阅读完整日志/答案、人工判定；
- [x] B38-T2：记录 LOGOCC1/DATARESULT1 及 runner oracle 盲区；
- [x] B38-T3：实现 data 非对象 Result 规范化与回归；
- [x] B38-T4：实现 log verbatim message cardinality、合并拒绝与术语纠偏；
- [x] B38-T5：全相关测试通过（dataquery/dataworkflow/tool/context/types/skill）；
  `f6981d22b` 已提交推送；
- [x] B38-T6：同一 2 case 严格并行重放；data schema repair 消失，log pre-stage
  权限闭环，最终模型残余按既有 model-owned gap 留档。

#### B38-R2：确定性合同闭环，模型所有权残余不再硬化

R2 runner 仍为 2/2 PASS，人工为 1/2 PASS：

- data 最终 `2` 正确，`data_repair_rounds=0`，R1 的裸 scalar
  `result_schema_mismatch` 已消失，DATARESULT1 closed。模型本轮选择了完整 typed
  ledger DAG，7 个 rounds 中有 2 个可恢复 `missing_action_inputs` suffix；这是
  planner 路径/效率波动，答案与审计账正确，登记
  `EVAL-B38-DATACHURN1/P2-watch`，不为 4 行 JSONL 加特殊捷径或题面门。
- log pre-stage 已正确输出 1 个 explicit Error，以及 goroutine 87/120 两个
  `severity=info / diagnostic=false` 的 thread_snapshot；下游 prompt 逐字显示
  snapshot 不能证明 crashed、same resource 或 causality。模型仍在 explorer 生成
  一个无 support refs 的「同时出错=3」aggregate，并在最终答案称 87/120
  在 mapassign 中崩溃。该残余与 `EVAL-B35-LOGMODEL1` 完全同类：系统 precise
  authority 已正确，模型结论仍错。继续扫描 aggregate label/答案词面、禁止模型
  列出 snapshot、或由系统替换主结论都会违反所有权红线，故合并到既有 gap 后停止
  单例加硬。

状态：`DATARESULT1=covered@f6981d22b/live-replay`；
`LOGOCC1=covered@f6981d22b/live-prestage`；
`LOGMODEL1=open/model-owned/no-system-rewrite`；
`DATACHURN1=P2-watch`。

### B37：显式窗 Trace × 精确单文件 read，模型答案所有权审计（2026-08-01）

严格并行 2 case、连续 3 轮。runner 结果分别为 2/2 PASS、1/2 PASS、1/2
PASS；逐份日志和最终答案人工审计后，三轮均为 0/2 human PASS。发现集中在两类：

1. Trace 正例始终保留显式用户窗优先级、自动补采、主要实际占用/新修向与现有
   规则可消除量两轴、根因排序、唤醒链和 Trace 因果投影，证明前述 report-shape
   窄化没有伤到显式窗能力。但模型连续三轮把 neutral 的 frequency/policy ceiling
   升级成「热控轨/热治理主根因」，并在修向独立与物理重叠之间自相矛盾。typed
   频率结论边界已经正确发布；该项属于模型结论权限/推理质量 gap，禁止通过系统
   扫描或替换正文来“修成想要的答案”。
2. read R1 中，模型原始 11 行 Markdown 表含完整职责，确定性编译器却将其改成
   11 行空说明；R2/R3 中，typed exclusion 后处理把模型路径或 10 个类型名改写成
   `[excluded]`；R3 又在调查端强制接受缺 1 行的 handoff，并由系统追加
   `CompletenessClaim`。这是确定的模型答案所有权违例，不是普通模型波动。

#### B37-a：exact-file closure 与成员身份

`fa8bf100e` 已完成：

- analyzer `EvidencePlan.RequiredFiles` 为精确文件、且无显式 broad source scope 时，
  执行完的 complete exact-file source_inventory lens 可关闭该文件 universe，不再被
  陈旧 repo-wide source-class 债务拖回无关目录；
- 闭合要求 lens count/total/row parity、principal member_set 覆盖或显式排除每一行，
  且每个 included row 有同文件位置；
- type/function 成员身份不再把 containing file 当成成员 key，避免文件内任一类型
  覆盖全部同文件 sibling；只有 file/config_file 清单可用路径本身作 identity；
- partial、different required file、显式 broad scope 均保持 fail-closed。

#### B37-b：模型所有权边界与精确修复路由

本批实现：

1. 新增 exact requested-file coverage gap；缺口直接给出遗漏成员与位置，并将 follow-up
   路由固定回 RequiredFiles，而不是 repo-wide 同语言样例目录。
2. 精确 typed 行遗漏不再进入 low-delta convergence force-complete；只有 pagination、
   candidate budget 等非精确导航债务保留有界 caveat 收敛。
3. `negative_observation.excluded` 只可用于该事实的 target auto-repair，不再成为全局
   文本 deny-list；只有 `excluded_count` 拥有结构化排除语义。
4. 生产发布删除 post-hoc exclusion token rewrite。typed exclusion 继续进入模型 prompt
   和结构化验证，但系统不得在模型 emit 后替换 prose/label/cell/citation 文本。
5. source_inventory principal rows 仅作为 finalizer 输入与 exact coverage oracle；生产
   pre-emit/persist 不再改写表格、补 requested fields、追加 missing-member carrier 或
   删除 supporting row。缺行/多行均以精确结构化 repair 返回模型，由模型重发。
6. 新的 extraneous-row gate 只读 typed row set 与结构化 item label/cell/citation，不扫
   用户原文、thinking、summary/final prose；它明确点名多余行并拒绝，绝不静默删行。
7. Trace 系统补采与因果投影走独立的 runtime supplement 路径，且继续使用
   model-owned block wire preservation；本批没有关闭或改写该能力。

验证：`go test ./internal/tool`、`go test ./internal/types ./internal/agent
./internal/orchestrator ./internal/skill` 全部通过。新增 pin 覆盖 exact-file partial gap、
精确缺口不可 force-complete、negative-observation 排除不污染、发布器不做 token
rewrite、缺行不补写、多行不静默裁剪。

状态：`B37-a=committed/pushed`；`B37-b=committed/pushed@0a42a5e83`。Trace 模型
过度热归因仍为 open model-guidance 项，只允许基于 typed
`GovernanceCapWitnessed=false` 的软提示/证据组织优化，禁止系统正文改写或关键词
硬门。

#### B37-R4：所有权红线恢复，但暴露两个新的权威冲突

R4 继续严格并行 2 case。runner 为 2/2 PASS；人工为 1/2 PASS：

- read 人工 PASS：模型自己给出精确 11 个 enum 类型、职责和引用；没有系统替换
  清单、空说明或 `[excluded]` 改写，证明 B37-b 的模型所有权边界已生效。
- Trace 人工 FAIL：四次模型 `trace_query` 都使用用户显式窗
  `13762.791708..13763.024898`，主状态表也使用 233.190ms；但系统发布的因果投影
  根窗却变成 `13762.795456..13762.795569`（0.113ms），同时仍携带 157.248ms 等
  全窗根因量，导致分母、百分比和重叠核算失真。runner 只证明“投影存在”，未证明
  投影窗口正确。

##### EVAL-B37-SCOPECLASS1：精确文件权威被 production 类别扩成全仓（P1）

read 最终答案正确，但过程用了 503s、27 次 read、15 次 repo_map、15 次
source_lens 和 28 次 midloop。日志显示 complete 的
`scope=internal/types/evidence.go count=11` 已经取得，系统仍把 follow-up 扩到
`cmd`、fixture 和 `internal/skill` 等无关目录。

根因不是模型“不会停”，而是 analyzer 同时给出：

- 高置信 `EvidencePlan.RequiredFiles=[internal/types/evidence.go]`；
- `SourceScopeProfile.requested_scope=production`，且 `source_quotes` 也是同一精确文件。

旧逻辑一见非空 SourceScopeProfile 就拒绝 exact-file closure，并把 production 类别
解释为 repo-wide；宽类别权威覆盖了更精确的文件边界。

通用修复：新增 typed exact requested-file boundary。只有 source inventory 已激活、
高置信 RequiredFiles 全是具体源码/配置文件，且结构化 source_quotes 规范化后的文件
集合与之完全相等时，文件边界才优先于 production 类别。集合不等、额外文件、宽泛
quote 或非文件目标全部 fail-closed，继续走 repo-wide。判断不读取 RawRequest、
rationale、thinking 或最终答案文本。exact-file complete lens 可据此关闭陈旧根级债务，
explorer 首轮也固定在精确文件。

无缓存测试额外发现两条旧测试仍在要求系统给 model-owned source inventory 自动铸造
facet，以及结构门只允许一个 `ForceHard` 生产点。前者改为断言“保持模型文档不变并
返回精确修复缺口”；后者登记缺行/多行两个 closed typed member-set 硬门。两者都不
扫描自由文本，也不允许系统静默补写或裁剪。

状态：`partial / first arm committed@381161a4a / nil-class arm found by R5`。

##### EVAL-B37-PROJWIN1：因果投影根窗与显式用户窗权威分裂（P1）

这是确定性系统 gap，不是模型波动。显式用户窗、模型查询窗、系统补采披露窗和状态
账一致为 233.190ms，只有系统因果投影选择了 0.113ms 的派生 frame span 作为根分母。

代码追踪确认补采参数与引擎没有缩窗。`frame_target_resolution` 同时承载两个合法但
语义不同的时间面：Observation `Span` 为 selected frame 的引用定位区间；typed
`window=` 为 `FrameTargetResolution.Window`，即分析和分母权威。旧投影编译器优先拿
有效 Span，仅在 Span 无效时才读 `window=`，把引用区间误升为根窗。

通用修复将优先级纠正为：白名单 `window_source=query_window`/显式 union 的 frame
anchor 必须优先读取严格解析的 typed `window=`；只有旧记录完全没有该 note 时才回退
Span。note 已存在但格式损坏时 fail-closed，不把 selected-frame Span 猜成用户窗。这样
派生 frame/span 仍保留为证据 witness，显式 frame/span selector 的 resolution window
也照常生效，但不会覆盖根分母。

Trace 答案中“热治理”“精确 12 因素”“PI/fscache 修复关系”等无 typed proof 的升级
仍归 model-guidance lane；只改善 typed 边界和 prompt 软引导，禁止系统重写模型结论。

验证：新增真实数值 pin（233.190ms 用户窗 × 0.113ms selected-frame Span），断言投影
根窗保留 233.190ms；另钉 malformed typed window 禁止 Span 降级。`internal/types`
无缓存全包与 projection 相关 `internal/tool` 测试通过。

状态：`covered / committed+pushed@58c2c7074 / R5 replay verified`。

#### B37-R5：投影窗闭环；精确文件 nil-scope 与模型校准仍开放

R5 在 `58c2c7074` 上严格并行同一 2 case。runner 仍为 2/2 PASS，人工为
0/2 PASS，但失败面已经分离清楚：

1. Trace 的确定性窗修复通过：投影根窗为
   `13762.791708..13763.024898`（233.190ms），四态账恰好平衡 233.190ms，时长
   标尺明确为 233.190ms；0.113ms selected-frame 只保留证据定位，不再充当分母。
2. Trace 模型仍把 frequency supply fold 升级成热治理，把 overlap/incomplete
   enumeration 下的若干席位相加成“理论最大总消除量 98.01ms”，并宣称调度供给与
   其他方向正交、PI/lock 与 fscache 修复具有已证因果。typed 权限块已明确否定这些
   升级，因此这是重复模型校准 gap。只允许增强 typed authority 的软引导，禁止
   系统替换、删除或重写模型结论。
3. read 从 503s 降到 123s，read 27→1、lens 15→6，且最终 11 行仍由模型撰写，
   证明第一批有效且模型所有权未回退。但开头写成“共10个，另有1个”，与完整 11 行
   自相矛盾；这是模型算术/表述波动，先不以正文硬门处理。

##### EVAL-B37-SCOPECLASS2：具体文件请求无 source class 时仍被扩成全仓（P1）

R5 analyzer 正确省略 `source_scope_profile`：用户给的是具体文件边界，不是
production/test/all 路径类别。它同时给出 0.95 置信的精确 RequiredFileHint 和唯一
EvidencePlan.RequiredFiles。旧 nil-profile 分支仍无条件要求 repo-wide lens，导致：

- explorer 首次尝试精确 `internal/types` scope 被拒；
- prompt 明示 `typed bounded scopes: .`；
- 先跑全仓 lens，再跑多组无关 follow-up，共 6 次 lens。

不能把 LLM RequiredFileHint 的置信度单独硬化成边界。最优通用方案是复用现有
`AnalyzerHints.MentionedEntities` 的确定性 provenance lane：将高置信、具体源码/配置
RequiredFileHint 也作为候选交给既有“当前请求逐字出现”校验；只有 hint 文件集合全部
被这条 typed user-mentioned lane 精确佐证，且无显式 all/test/docs/auxiliary 类别冲突
时，nil/production class 才可形成 exact-file boundary。后续 hard gate 只读该 typed
provenance 与规范化文件集合，不再读取 RawRequest，也不读 rationale、thinking 或
答案正文。显式 `all` 始终优先并保持全仓。

施工：analyzer 的既有 `MentionedEntitiesFromRawRequest` provenance 校验候选，从
PrimaryEntities 扩为 PrimaryEntities + 具体且置信度≥0.8 的 RequiredFileHints；低置信
hint、目录和非代码/配置路径不进入候选。`SourceInventoryHasExactRequestedFileBoundary`
随后要求全部高置信 required files 获得该 typed user-mentioned 佐证；production 的同集
source quote 仍是另一条合法佐证，显式 `all/test/docs/auxiliary` 一律优先。任何新增 hint
未被用户提及即回退 repo-wide。

测试覆盖：当前 R5 的 nil SourceScope + 精确文件正例；同批额外未佐证 hint 反例；
显式 all 反例；analyzer 仅把逐字出现的高置信具体文件写入 MentionedEntities；explorer
首轮 scope 保持 `internal/types/evidence.go`。所有判断下游只读 typed provenance，
不扫描答案/思考/模型 prose。

状态：`covered / committed+pushed@9b936d4e9`。

### B36：配置映射 × write plan 权限与模型结论所有权（2026-08-01）

本批严格并行 2 个 case：

- `sr_java_config_precedence`：read/config，多层配置键映射、默认值证据与运行态
  不可观测边界；
- `patch_cpp_typo`：write plan，一行最小修改与 plan-only 安全边界。

选择这组不是继续拟合单个配置题，而是用一个“源码事实完整、运行态不完整”的
read 面检验系统与模型的权限分工，并同时用 write case 看修复是否污染写模式。
七轮 runner 都是 2/2 PASS；人工审计直到 r7 才是 2/2 PASS。r1-r6 的自动绿
不能替代人工正确性。

#### EVAL-B36-CONFIGSTATE1：配置映射被误铸成当前运行值（P1，covered）

r1 把 `application.properties` 的源码值 50 表述成当前生效值，然而进程环境变量
未观测。根因是 exact scalar 合同把“配置层映射”压成“单一当前值”。

通用修复：

1. 新增 typed config precedence 语义，区分 source/default mapping、override
   mechanism 与 observed runtime value；
2. 未观测 runtime 时只允许模型说明候选层级和优先关系，不把源码默认值提升为
   当前主值；
3. scalar lookup 的精确值能力不受影响；显式不存在也仍可精确披露；
4. 不扫描用户或模型原文，不识别 Java、键名或数值字面。

落地：`c49dd3ea0`、`98b7f266b`。状态：`covered`。

#### EVAL-B36-SYSAUTH1：系统补充逐层接管模型答案（P1，covered）

r2-r6 先后暴露同一架构根因的不同表面：

- 非枚举配置题追加两份 enumeration completeness 表；
- 追加泛化 uncertainty caveat；
- 把环境变量名当枚举值补错误 label；
- 在答案最前追加“精确目标命中”主值；
- 把未观测运行环境误报成模型遗漏的 required role；
- 在定位成功时追加 `owner_supported` 系统表。

这些都不是信息不准确这么简单，而是系统把内部验证、成功遥测或证据权限直接
发布为模型结论，违反“系统提供 typed facts/guidance，模型拥有推理与结论”的
红线。

通用修复：

1. 系统 answer supplement 只披露会实质改变可相信范围的 typed 缺口、冲突或
   降级，不发布成功遥测；
2. enumeration label supplement 只在 typed enumeration authority 下进入答案，
   内部 validator 仍可保持活跃；
3. 配置映射的未观测 runtime 不再合成 `missing_requested_roles`；
4. 非标量正向 exact evidence 保留给探索、归一化和安全校验，但从 answer
   surface 隐藏；scalar lookup 与 proved absence 的答案权限保持；
5. 所有调整都基于结构化 IR/authority，不扫描 RawRequest、模型 thinking、
   summary、items 或 final prose。

落地：`b916b0165`、`a7f19b80d`、`9cef53c39`、`a8035ce3b`、
`3f9cdb89b`。状态：`covered`。

#### EVAL-B36-EXACTDUAL1：内部精确证据与答案确权缺少双通道（P1，covered）

直接删除 exact authority 会破坏多目标 targetless absence 的持久化安全；直接
发布又会越权。因此采用双通道：

- `ExactResolution` 继续服务探索、证据归一化、absence 安全和内部 contract；
- `AnswerSemanticView.SuppressExactResolutionAnswerSurface` 仅控制答案权限；
- dynamic schema、finalizer prompt 和 pre-emit metadata normalization 共同消费
  answer view；不改模型已写 block；
- trace 日志显式记录 `exact_resolution_answer_surface`，便于审计。

这是权限分离，不是删除精确信息。状态：`covered`。

#### EVAL-B36-CITATION1：配置值引用到查找逻辑而非值来源（P1，covered）

r6 模型用 Java 的 lookup 行支撑 `50`，该行只能证明 key/机制，不能证明 literal。
修复放在 config carrier 的软指导：陈述配置字面值时优先引用同时包含 key 与
literal 的来源；lookup call 只证明读取机制。系统不替模型改引用、不做 prose
硬门。r7 已引用 `application.properties:1`。状态：`covered`。

#### EVAL-B36-LOCSTATUS1：成功定位遥测进入答案（P1，covered）

`owner_supported` 且无 missing/conflict/more-context 时，源码定位状态只留在
document/log telemetry，不再追加到用户答案。`weak / observed-only / missing /
conflicted` 仍可披露，避免吞掉真实证据边界。落地：`3f9cdb89b`。

#### EVAL-B36-DEGRADEFOOT1：过程降级 footer 噪声（P3，open/watch）

r7 正文完全正确，末尾仍出现“引用摘录回填 ×3、必答面硬转软 ×1”。该 footer
属于过程透明信息，没有替换结论，也没有制造事实矛盾。本批不为消掉一行文案而
增加硬门；只有跨类型重复造成用户误判时再统一处理。

#### EVAL-B36-LABELADV1：block label advisory 噪声（P3，watch）

r7 日志仍有 `block_items_label:1` advisory，但未进入答案，也未改变事实或结论。
目前判为软校验噪声/模型波动观察项，不做特例匹配。

#### B36 验收与不变量

- r7 config：模型自主给出三层映射、正确引用 value source、明确运行值未观测；
  无系统 exact 前缀、枚举表、missing-role、成功定位表或泛化 caveat；
- r1-r7 write plan：连续七轮均为最小一行计划，无主仓修改；
- `go test ./internal/types ./internal/agent ./internal/tool ./internal/orchestrator`
  相关全包通过；
- 没有新增基于用户输入或模型输出原文的硬门；
- 没有新增系统改写/替换模型结论；
- 显式时间窗优先级、Trace 因果投影、自动补齐和内部 exact safety 未改变。

本批提交：`c49dd3ea0`、`98b7f266b`、`b916b0165`、`a7f19b80d`、
`9cef53c39`、`a8035ce3b`、`3f9cdb89b`。

状态：`r7 runner 2/2 PASS / human 2/2 PASS / P3 telemetry watch only`。

### B26 PHASE/ALIAS r1 与 B27 absence-authority 根修（2026-08-01）

严格并行 2 个回放：

- `real_trace_c2_dstate_iowait`：runner PASS，140s；human
  `pass_with_advisory`。请求主窗 3 段 io_wait（0.138 + 0.147 + 0.350 =
  0.635ms）、非 IO D-state=0、caller=`sync_buffer_read_wi` 均正确；没有被
  扩成 Trace 因果投影。模型把 `udk-irq` 行解释为“代为采样/内核跨线程采样”
  没有 typed 权限，记低优先级 semantic advisory，不由系统改写正文。
- `trace_query_donghu_real_frame_multicausal`：runner PASS，159s；human
  FAIL。`EVAL-B26-ALIAS1` 已覆盖：同一附件只产生一套主投影、明细、指标和
  证据索引；`EVAL-B26-PHASE1` 部分覆盖：finalizer 收到每行
  `impact_phase=pre_wakeup_dependency`，旧的“RT 目标唤醒后被 CFS 线程抢占
  11.103ms”误判消失，但模型仍把未证 holder/waiter 的 candidate 称为核心
  优先级反转。

本轮更高优先级的新 GAP：

| ID | 优先级 | 机制证据 | 泛化处置 |
|---|---:|---|---|
| `EVAL-B27-MWAUTH1` | P0/P1 | `missing_wakeup` 的真实含义是“所选窗内没有匹配 `sched_wakeup` 行”；producer 却把 target-self rank 副本铸成 `principal_answer/observed_direct_cause`，root-evidence 副本也用 direct-cause provenance；随后 value-owner 与 target-blocking authority 又把 3.584ms 发布成正向 `proven_blocking_wall_clock`，最终诱导“直接唤醒缺失/直接阻塞” | 建立 exact typed evidence-boundary 分类；保留 sleep 区间、行号、下钻失败与原始 token，但拒绝进入正向值归属/已证阻塞 authority，producer 两个副本都降到 supporting coverage；finalizer 只接收 evidence-boundary 语义和区间，不接收系统结论 |
| `EVAL-B27-REL1` | P2 | 模型把同一链节点的 D-state、io_wait、io_latency 三种可能重叠/不同口径值直接相加，generic `cross_row_additivity=forbidden` 未被稳定消费 | 冷读现有 overlap/reconciliation typed carrier，优先补行间 relation/所有权输入；不得扫描答案做算术硬门或替换模型结论 |
| `EVAL-B27-CALLER1` | P2 | `sched_blocked_reason.caller` 被模型写成“持有等待对象”，`udk-irq` 事件线程被写成“代为采样” | 明确 typed caller/event-thread 角色语义；只有 holder/owner carrier 能授权“持有”，不从 caller 名或正文词汇猜测 |

#### B27a `missing_wakeup` evidence-boundary 单一语义

施工规则：

1. `TraceObservationIsEvidenceBoundary` 是 Observation 层单一分类点，当前闭集
   为 `missing_wakeup / trace_gap`；只读 exact typed type/object/predicate，
   不读 summary、用户原文、模型 thinking/final。
2. `trace_query` 发布 `missing_wakeup` 的 target-self rank 与
   root-evidence 两个 lossless 视图时，统一使用
   `supporting_coverage + artifact_span`；值、区间、line span、token 保留。
   RootEvidence 已存在的 typed `StartTs/EndTs` 补通到 Observation span，避免
   下游拿行号包络猜时间。
3. `TraceValueOccurrenceAuthority` 与
   `TraceBlockingWallClockAuthority` 拒收 evidence boundary。正向
   binder/futex/D/IO wait 行不变；`missing_wakeup` 的 3.584ms 仍是目标 sleep
   症状/下钻覆盖边界，不再是“已证 blocker”。
4. `Trace Decision Inputs` 增加高显著度、prompt-only 的
   `evidence_boundary` 行：没有匹配行不等于物理唤醒未发生，不提供 blocker
   identity，也不拥有正向 causal/eliminable amount；窗口边界、事件覆盖/丢失、
   未表示的唤醒源均待独立 typed 证据区分。
5. 系统投影的 reader-facing label 收窄为“窗内未找到匹配唤醒记录” /
   “no matching wakeup record found in window”，raw `missing_wakeup` token 和
   `⊘链止` 保留。该显示说明数据边界，不生成/删除/替换模型结论。
6. 看护测试只固定结构所有权与 typed transport：absence 不得进入 positive
   authority、模型 block 保持原文、显式窗 handoff 保留。测试不要求模型必须
   得出某个根因、采用某句措辞或选择某项优化，避免看护用例自己越权/过硬。

定向测试覆盖 producer 双副本降权、两类 positive authority 拒收、正向 wait
反向保留、prompt evidence-boundary、系统 materializer 不铸正向阻塞卡、ZH/EN
精确边界措辞。

状态：`EVAL-B26-ALIAS1=covered`；`EVAL-B26-PHASE1=partial`；
`EVAL-B27-MWAUTH1=implemented / focused-tests-pass / full-regression-next`；
`EVAL-B27-REL1/P2`、`EVAL-B27-CALLER1/P2=open`。本轮原始机器汇总与人工审计：

#### B27a r1 与 OWNGUARD1：absence 形未触发，模型 wire 看护发现真实越权

严格并行 2 个回放均为 runner PASS；人工结论：

1. `real_trace_c2_dstate_iowait` 为 `pass_with_advisory`：D-state=0、3 条
   io_wait、0.635ms 均正确；
2. `trace_query_donghu_real_frame_multicausal` 为 human FAIL：本轮模型只调用
   两次 trace_query，没有形成 `missing_wakeup` 行，因此
   `EVAL-B27-MWAUTH1` 只能记为 `not_observed_in_this_replay`，不能把生产者和
   authority 单测冒充客户形回放覆盖；
3. PHASE handoff 已明确给出 `impact_phase=pre_wakeup_dependency`、非 consumer
   post-wakeup、无 holder/waiter 证明，模型仍把候选写成根因并混成唤醒后调度；
   这是模型未消费精确信息，不授权系统改写答案；
4. system projection 已有 typed overlap/non-additive relation，但 finalizer 的
   高显著度 decision handoff 只有泛化 `cross_row_additivity=forbidden`，登记的
   `EVAL-B27-REL1` 仍应从 typed 行间关系补强，而不是扫描模型算式。

同时发现 `EVAL-B27-OWNGUARD1/P0`：bounded case 的前两稿均报
`runtime trace hierarchy violated model-answer ownership: model block 2 changed`。
冷读证明 OWN4 看护本身正确，真正错误在 hierarchy sorter：它把未带系统
marker 的 model caveat 单列 tier 9，却把随后由 pre-emit mechanical repair
补出的 model-owned carrier 留在 tier 0，因而改变模型块子序列。该异常来自
`c60400844a` 对 `e920a5d8c1` 全局排序的收窄仍保留 caveat 例外；不是本轮
`629828739` 所加 OWN4 造成，OWN4 只是首次 fail-loud 暴露旧越权。

修复：所有非 runtime-trace system blocks（包括 model caveat）保持完整原序
子序列；只允许 authenticated runtime-trace system blocks 参与 tier 排序，
排序后的 system 序列整体插入首个非 trace caveat 边界之前。这样兼容既有
“caveat 之前是可读正文/附录”的消费者，同时不再把 caveat 跨过随后由机械
repair 补出的 model-owned carrier。增加 model caveat 位于机械 carrier 前的
wire 级回归，既允许模型自由组织多块答案，也继续拒绝任何系统删除、改字或
调换模型块。该看护不读取用户输入、模型正文语义或关键词，不钉具体结论。

验证：7 个 hierarchy/materializer 定向回归通过；完整
`go test ./internal/tool -count=1` 通过（166.478s）。

状态：`EVAL-B27-OWNGUARD1=implemented / full-tests-pass`；
`EVAL-B27-MWAUTH1=implemented / replay-not-observed`；
`EVAL-B26-PHASE1=partial`；`EVAL-B27-REL1/CALLER1=open`。

#### B27b REL1/CALLER1：typed 行间关系与角色进入模型决策输入

冷读确认，system projection 明细已经消费了完整关系载体，但 finalizer 高显著度
决策输入只给出泛化 `cross_row_additivity=forbidden`：

- `CrossDirectionOverlaps` 已持有 overlap_ms、对端 line envelope、修向与
  basis；
- `DStateSplitMS/IOWaitSplitMS` 已声明父席内含分量；
- `ResourceCompletionClosure` 已声明 IO completion 对 anchored wait 的闭合；
- `BlockedReasonCaller` 是 `sched_blocked_reason.caller`，却在窄事实 recap 被
  系统主动措辞成“已解析等待对象”，诱导模型进一步写成资源持有者；
- `BlockingSubjectIsHolder + BlockingPeer` 才是锁 holder/waiter 角色权限。

本批不新增结论、不扫描模型答案，也不增加 emit hard gate：

1. Axis A/B 每行携带 `row_identity`，同一 typed node 跨两轴可识别为同一行，
   不是两个可相加事实；
2. 随行发布 `embedded_components=...`、
   `component_relation=already_inside_parent_row` 和
   `addition_with_parent=forbidden`；
3. 随行发布最多 3 个已编译 `physical_overlap` 关系，包括 overlap_ms、对端
   lines、peer_fix_direction、basis 与 `overlap_addition=forbidden`；上限只控制
   prompt 体积，不改变系统 lossless projection；
4. `resource_completion_closure` 明确是 completion path→anchored wait，且
   `completion_thread_holder_authority=not_provided`；
5. `blocked_reason_caller` 明确为 kernel-reported wait call-site/symbol，单独
   声明 `holder_authority=not_provided_by_caller`；
6. 仅 `BlockingSubjectIsHolder=true` 输出 `subject_lock_role=typed_holder`；
   waiter-subject 形输出 `typed_lock_holder=<peer>`；
7. bounded fact 的 principal recap 同步把中英文“等待对象/resolved wait
   object”纠正为“等待调用点/符号”，不再由系统先铸造错误角色。

定向覆盖包含 D/io_wait 父子账、6.673ms 物理重叠对端、IO completion 角色、
caller 非 holder 与 typed lock holder 正向臂；无这些 typed 载体时不注入关系
段。完整 `go test ./internal/agent -count=1` 通过（2.878s）。

状态：`EVAL-B27-REL1=implemented / full-agent-tests-pass`；
`EVAL-B27-CALLER1=implemented / full-agent-tests-pass`；下一步同一双 case 回放，
人工检查模型是否消费关系；模型波动不升级为系统答案改写。

#### B27b r1：CALLER 覆盖、REL 改善，发现 coverage shard 假总量

严格并行 2 个回放 runner 均 PASS，人工结论：

- bounded D/io_wait：human PASS，114s，finalizer reject=0；3 段与 0.635ms
  全部正确，模型明确说明 caller 是内核等待调用点而非锁/资源 holder。
  `EVAL-B27-CALLER1=covered`；`EVAL-B27-OWNGUARD1=covered`。
- 显式窗复杂 trace：human FAIL，230s。REL1 被实际消费：模型明确写出
  7.386ms io_wait 已含于 10.433ms D/IO 父席，没有再加 D+io_wait+latency；
  `EVAL-B27-REL1=covered_with_residual`。但模型仍把 pre-wakeup dependency
  写成持有者/CPU 竞争，并把 sleep 解释为“被唤醒后立即切出”；精确 PHASE
  输入已在 finalizer，系统不据此改写模型答案，`EVAL-B26-PHASE1=partial`。

新登记 `EVAL-B28-SHARD1/P1`：Trace Observation Coverage 的 shard aggregate
按 distinct window 去重后无条件 Σ value，却不检查子窗是否相交。本次 6 个
`resource_pressure/block_rq` 查询窗大面积重叠，系统仍发布
`total_impact=14.204ms`；模型据此写成 block_rq 累计。答案后的 arithmetic
advisory 虽披露“证据面未单独发布”，但错误数已经由系统 prompt 提供，不能
归咎于模型，也不能靠答案扫描修补。

#### B28a SHARD1：重叠查询窗撤销跨 shard 总量权限

在 coverage aggregate 单一铸点做区间权限判定：

1. root-cause shard 与 state/resource shard 共用严格区间相交判定；相邻
   half-open 窗可加，任意正交集即不可加；
2. 窗不重叠时保留 Σ，并发布
   `additivity_status=disjoint_windows_additive`；
3. 窗重叠时 `TotalImpactMS` 从 authority wire 撤销为 0，保留 shard_count、
   union window、example windows、support refs 和 max_shard，并发布
   `overlapping_windows_non_additive`；
4. explorer stage report 与 finalizer coverage handoff 均渲染
   `total_impact=unavailable` +
   `cross_shard_additivity=forbidden_overlapping_windows`，不再把错误 Σ 暴露
   给模型；
5. 不扫描用户输入、模型输出或具体 resource type；所有 root/state/resource
   shard 共用，且不新增答案 hard gate。

定向覆盖：不相交窗口维持 21ms/32ms 旧 Σ；重叠 4.262ms+3.100ms 不再发布
7.362ms，max=4.262ms 与 union coverage 保留；两条生产 prompt 消费面均钉。

完整回归：`go test ./internal/types ./internal/agent -count=1` 通过
（types 21.867s，agent 2.859s）。

状态：`EVAL-B28-SHARD1=implemented / full-tests-pass / replay-next`。
`eval/parallel_selected_summary_evalcampaign_b26phasealias_r1_20260801.md`、
`eval/parallel_selected_summary_evalcampaign_b26phasealias_r1_20260801_manual_audit.md`。

#### B28a r1 与 B29：shard 权限覆盖，读模式暴露 source-span 扩权

严格并行 2 个回放 runner 均 PASS，人工均 FAIL；机器 oracle 只验证结构面，不能
替代事实审计。

显式窗 trace 的 B28 目标已覆盖：6 个相交 `resource_pressure/block_rq` shard
现在发布 `total_impact=unavailable`、
`cross_shard_additivity=forbidden_overlapping_windows`，日志与答案均不再出现
14.204ms 伪总量。模型仍把 pre-wakeup dependency 写成“直接优先级反转/持有调度
依赖”，并在 `frame_causality=unproven` 下声称“卡顿完全来自调度”；精确 phase、
caller/holder 与 causal ceiling 已在 finalizer 输入，故维持
`EVAL-B26-PHASE1=partial/model-consumption-watch`，不以答案扫描、系统改写或硬化
结论处理模型波动。

读模式用例暴露两个层次：

1. 事实真相是代码默认 `MaxRetriesPerStage=3`，runtime YAML 的 `*int` 只是可选
   override；`dynamicAnalyzeRetries` 收到的是已合并的 `int`，小于 1 才规范为 1。
   read analyze 非 transport 耗尽后安装 `buildDegradedSemanticIR`，无任何 partial IR
   时才委托 `buildDegradedFallbackIR`；`fallbackWriteAnalysisIR` 只属于随后独立的
   write_analyzer 路径。最终答案却写成“默认 5/nil 回退 1/read 走 write fallback”。
2. `emit_evidence` 已声明 call/definition/span entailment，finalizer 也已有 mechanism
   relation authority；但 `callChainEvidenceSupportDetail` 又无条件把每个
   `EvidenceItem.Summary` 作为主支持 lane 的 `Evidence note`。于是定义行旁的模型自由
   summary 可重新承载函数体、兄弟函数、配置默认值与跨模式时序，直接绕过
   `ClaimFormOf` 和 grounded span。

登记：

| ID | 优先级 | GAP | 泛化方案 |
|---|---:|---|---|
| `EVAL-B29-SPAN2` | P1 | 非 load-bearing 的模型自由 summary 被支持 lane 再提升为 answer-authority detail，复现并具体化 B21-CALLEE1/SPAN1 | 支持 lane 与 `EvidenceAuthoritativeSurfaceText` 使用同一权限：默认只发布 typed anchor/condition/surface metadata；仅显式 `LoadBearingSummary` 可携带 summary。跨行/跨函数行为必须拆成各自 grounded evidence，不扫描最终 prose |
| `EVAL-B29-LANE1` | P1 | 当前没有 typed execution-lane/path membership；同文件中互斥的 read/write 相邻实现都可能进入 principal mechanism evidence | 后续建立从 principal endpoint、typed call edge/flow path 到 evidence 的 lane membership；无路径证明的相邻定义只能作 enrichment/boundary。禁止按 `fallbackWriteAnalysisIR`、case、模式词或答案文本特判 |

#### B29a：support lane 与 source-span authority 收敛

`callChainEvidenceSupportDetail` 不再无条件读取 `EvidenceItem.Summary`：

1. typed `claim_form`、anchor kind、condition 与 surface terms 保持；定义、调用、
   guard、return、assignment 等精确载体不受影响；
2. 默认 false 的自由 summary 不再以 `Evidence note` 进入主支持面；
3. 只有 producer 显式声明 `LoadBearingSummary=true` 的窄值载体保留 summary，和
   `EvidenceAuthoritativeSurfaceText` 既有合同一致；
4. 测试把过去“任意 summary 都应保留”的错误预期改成“不扩张 grounded span”，
   并固定 typed metadata 保留与 load-bearing 正向臂；
5. 不读取 RawRequest、模型 thinking/final、函数名、语言或 case ID；不新增答案硬门，
   不触及 Trace query、显式窗、因果投影、自动补采或系统/模型所有权边界。

状态：`EVAL-B28-SHARD1=covered`；`EVAL-B29-SPAN2=implemented/focused-tests-pass`；
`EVAL-B29-LANE1=P1/filed`。本轮工件：
`eval/parallel_selected_summary_evalcampaign_b28shard_read_r1_20260801.md`、
`eval/parallel_selected_summary_evalcampaign_b28shard_read_r1_20260801_manual_audit.md`。

#### B29a r1：summary 扩权已闭合，源码合同自身仍相互矛盾

同一 `main@11759e6dc` 二进制严格并行 2 个回放，runner 2/2 PASS，人工 0/2：

- read live witness 已证明 `EVAL-B29-SPAN2` 生效。finalizer 的 principal support
  不再出现 default/nil/write fallback 自由 Summary Evidence note，最终答案也不再把
  `fallbackWriteAnalysisIR` 拼入 read 路径。
- read 仍错误声称 missing emit 时 `out.Error=""` 且预算耗尽后整个 Run 终止。
  `analyzerEvaluator.ParseOutput` 的 production 代码实际返回 populated Error；Run 在
  `runAnalyzePhase` 之后按 error class 分流：stream transport hard-fail，missing emit /
  gate rejection 安装 `buildDegradedSemanticIR`（无 partial IR 时委托
  `buildDegradedFallbackIR`）并继续 bounded phase 2。
- 错误并非纯模型波动：`analyzer.go` 顶部 Fail-loud contract 和
  `runAnalyzePhase` 函数注释仍明确写旧的“whole Run terminates”，与同文件 production
  control flow、架构文档和 degraded tests 冲突；`runTaskGraph` 的 nil guard 也未注明
  它只是 join-contract defensive arm，不是常规 analyze exhaustion 路径。
- trace 显式窗、因果投影、根因排序、唤醒链、窗内可消除量与系统补采均保留，B28
  重叠 shard 仍撤销 total。模型仍越过 `frame_causality=unproven`/pre-wakeup phase 给出
  强因果；继续按 PHASE1 model-consumption-watch，不由系统替换。

登记 `EVAL-B29-DOC1/P1`：production control flow 与源码注释合同漂移会把错误陈述作为
高可信当前源码证据暴露给 analyzer/explorer/finalizer；所有机制解释、代码审计和后续开发
都会受影响。修复应统一更新 owner/callee/defensive guard 注释，行为测试继续作为真相，
不能靠 final answer 关键词纠正。

#### B29b：analyze exhaustion 源码合同收敛

本批只修正代码内权威说明，不改变 runtime：

1. `analyzerEvaluator` 明确 attempt 级 fail-loud：missing emit 返回 populated Error +
   nil IR；预算耗尽后由 Run 分类，transport hard-fail，semantic/gate 进入 degraded IR；
2. `runAnalyzePhase` 注释只承诺“向 Run 返回 error”，不再越权声明整个 Run 终止；
3. `runTaskGraph` nil/empty guard 明确为 join-contract defensive failure；正常非 transport
   exhaustion 在进入该函数前已经安装 degraded task graph，transport failure 不进入；
4. `buildDegradedFallbackIR` 的诊断说明改为真实 `SoftAnalyzerError + QualityGate detail`，
   不再声称 caller 保留 hard `LastError`；
5. 不修改调度/重试/降级分支，不扫描用户或模型文本，不触及 Trace/写模式所有权。

状态：`EVAL-B29-SPAN2=covered`；`EVAL-B29-DOC1=implemented/replay-next`；
`EVAL-B29-LANE1=P1/filed`。工件：
`eval/parallel_selected_summary_evalcampaign_b29span_r1_20260801.md`、
`eval/parallel_selected_summary_evalcampaign_b29span_r1_20260801_manual_audit.md`。

#### B29b r1 与 B29c：恢复合同主体已纠正，内部配置载体仍残留旧口径

同一 `main@7f0a2a607` 二进制严格并行 2 个 case：runner 2/2 PASS；人工审计为
read FAIL、write PASS。

- read 主体已经正确说明 missing emit 返回 populated `Error` + nil IR，预算耗尽后
  semantic/gate failure 安装 degraded IR，无 partial IR 时才委托 fallback builder；
  `fallbackWriteAnalysisIR` 不再进入 read 答案。说明 B29a 的 span 收敛和 B29b 的
  owner/callee 注释修正均已生效。
- 但最终答案仍把 `runAnalyzePhase` 说成由 `runTaskGraph` 调用。真实 production 拓扑是
  外层 `Run` 先调用 `runAnalyzePhase`，之后才进入 `runTaskPhase -> runTaskGraph`。
  Current-Source Mechanism Relation Authority 已明确“无 edge 不得连接”，模型仍沿用用户
  前提和同文件邻接形成假边。该项归并 `EVAL-B29-LANE1`：软关系提示已有但模型消费不稳；
  后续方案必须是 item/claim 级 typed edge/path membership，不能扫描或替换模型答案。
- `internal/types/config.go::PipelineSettings.MaxRetriesPerStage` 仍保留两层旧合同：声称预算
  耗尽后 whole Run terminates，并让内部 carrier tag `max_retries_per_stage` 看起来像公开
  `codrax.yaml` 键。实际公开 runtime override 是
  `pipeline_max_retries_per_stage`，`cmd/root.go` 将其覆盖到默认值 3 后才构造已解析的
  `PipelineSettings`。本轮已补齐这处残余，明确 public key、default/override merge 与
  semantic degraded 分流；不改变 runtime。
- 模型把 `base + (estimated/2)*extra` 说成“复杂度与 base 的乘积”，并声称存在
  `QualityGate.Error` 字段；源码实际是加法并封顶，`GateReport` 只有
  Passed/Rejected/Retryable/Checks/Fingerprint。两项没有系统生成错误值，先记为模型
  措辞/字段幻觉观察；若跨机制 case 复现，再提升为公式/schema typed support gap，当前
  不为单例增加答案硬门。
- write case 只修改 `main.go` 一行 `retrun -> return`，applied-tree 无其它生产差异，
  验证通过。analyzer 为满足 owner-qualified field-value shape 用了 4 次 emit，属于效率
  观察，不影响改动正确性。

状态：`EVAL-B29-SPAN2=covered`；`EVAL-B29-DOC1=implemented-residual-fixed/replay-next`；
`EVAL-B29-LANE1=P1/open`；公式/schema 单例=`watch`。本批仍不改 Trace 显式窗、因果
投影、根因排序、唤醒链、窗内可消除量、自动补采或模型答案所有权。工件：
`eval/parallel_selected_summary_evalcampaign_b29doc_write_r1_20260801.md`、
`eval/parallel_selected_summary_evalcampaign_b29doc_write_r1_20260801_manual_audit.md`。

#### B30 r1：跨车道阶段污染、三面状态账分裂与 prose 所有权残余

`main@787be79c4` 严格并行 2 个不同维度 case，runner 2/2 PASS、人工 0/2。

显式 11.000..11.008s Trace 的主分析有实质进步：正文以 app-20 runnable
5.000ms/62.5% 为 #1，把碎片化 state_churn 作为 absorbed 信号而非第二根因，把
rival-30 running 作为同 CPU 背景；「主要时间占用/关键路径候选」与「规则可消除量」
两轴明确分开，显式窗、根因排序、因果投影和自动补采均在。没有 frame/deadline
connector，因此只能证明所选窗口的主要占用候选，不能证明具体掉帧 deadline。

但同一答案暴露三个泛化 gap：

| ID | 优先级 | GAP | 最优方案 | 状态 |
|---|---:|---|---|---|
| `EVAL-B30-ACCOUNT1` | P1 | 同一 app-20/同一查询窗/同一五态分区在 `state_churn` 与 `wakeup_causal_impact` 两面分别发布 19次/20段和20次/21段。B4-T2 只靠 predicate 推测 occurrence scope；已有 `state_account_key` 仅连接 wakeup/rank，没有连接 canonical churn | producer 将已经由精确 segment inventory 铸造且唯一的 `state_account_key` 扩到匹配的 whole-window churn；显示层仅在同 artifact + 同 key 时保留 canonical churn，真实独立 occurrence 无 key 时继续 fail-open | implemented/focused-tests-pass |
| `EVAL-B30-LANE2` | P1 | read workflow 答案把真实但异车道的 `StageWriteAnalyze/runWriteAnalyzePhase` 接到 read 主链。production 只在 plan/apply/verify 调用该阶段；read 直接进入 `runTaskPhase -> runTaskGraph -> runReadSchedulerLoop`。事后正确 StageBinding 表不能消除正文矛盾 | 在 finalizer 成文前按 typed `AgentContext.Mode` + requested `stage_or_workflow` dimension 发布 canonical read main/pre-stage membership，并明确其它 stage 是跨模式上下文、不得接入当前 read path；信息只指导模型，不改写答案 | implemented/full-package-tests-pass；并入/具体化 LANE1 |
| `EVAL-B30-OWN2` | P1 | 旧 `proseLexiconBoardResidualFindings` 扫描模型自由 prose 后，系统附注误称“正文首因=rival-30”，正文首句和 typed #1 实为 app-20。系统再次越权评价模型结论且产生假阳性 | 退役基于自由 prose 的 primary/board 偏离用户可见判词与修复提示；继续把 typed board/事实并置给模型和用户，由模型作结论。若保留机械检查，只能消费结构化 claim relation，不能用正则绑定自由正文主体 | implemented/full-package-tests-pass |

`ACCOUNT1` 已完成：`ThreadStateChurnSummary` 新增可选 `StateAccountKey`；只有现成
rank↔wakeup exact segment identity、thread/state/window/dominant impact 和五态分区全等且
唯一时才传播；typed observation 发布该 key；快照仅在同 artifact+同 key 时让 canonical
whole-window churn 胜出。无 key 的真实 chain occurrence 继续双席，跨 artifact 不折叠。

定向覆盖：三面同 key 正臂、五态不等 fail-open、typed wire 三种 predicate、同 key 快照
单席、无 key occurrence 双席。`go test ./internal/tracequery ./internal/tool -run
'Test(StateAccount|StampResultStateAccount|TraceQueryPublishesStateAccount|RuntimeTraceMetricSnapshot_)'
-count=1` 通过。

`LANE2` 已完成：answer finalizer 只在 typed read mode、typed
`stage_or_workflow` 请求维度、且已有 grounded stage authority source 三者同时成立时，
向模型发布 canonical `analyze -> explore -> extract -> finalize` 主序列与条件前置
`log_triage/perf_triage`。未在两张 typed 清单中的真实符号只是跨模式/背景证据，
没有当前 read control-flow edge 不得接入当前链。实现不扫描用户或模型原文、不重写
模型答案、不制造函数调用边，apply 模式不激活，与 Trace 管线无关。
`go test ./internal/agent -count=1` 通过。

`OWN2` 已完成：从 contract evaluation 和 ship-time system cross-check 两个生产
choke point 同时断开 `prose_lexicon_board` 自由正文扫描。系统不再从模型文字用
正则推断“首因”、“双首因”、“榜序偏离”或“正文自排席位”，也不再将未发布
snake_case 词汇的噪声集合成员判断写入用户附注。typed 根因席位、状态/时间数值、
事实并置和机械数值复算继续保留，但系统不替模型宣告它们意味着什么。

看护不固定客户 case、线程、根因或答案措辞；只结构性钉住上述两个生产路径不得重接
自由正文结论扫描，并验证“正文先说 app-20、后提 rival-30 背景”时系统不再
输出“正文首因”判词。`go test ./internal/orchestrator -count=1` 通过。

状态：`EVAL-B30-ACCOUNT1=implemented/focused-tests-pass`；
`EVAL-B30-LANE2=implemented/full-package-tests-pass`；
`EVAL-B30-OWN2=implemented/full-package-tests-pass`。下一步严格并行 2 个原 case 回放，
同时验收账目合并、read/write 车道分离与模型结论所有权。工件：
`eval/parallel_selected_summary_evalcampaign_b30trace_lane_r1_20260801.md`、
`eval/parallel_selected_summary_evalcampaign_b30trace_lane_r1_20260801_manual_audit.md`。

#### B30 r2：typed 账目主键已贯通，但诊断引文仍被误授发布权

`main@a7f7c1065` 严格并行回放同两个 case：Trace runner PASS，read runner 因
`missing:Mutable` FAIL；人工审计两者均为 partial/FAIL。

Trace 主体完整保留模型结论，显式 11.000..11.008s 窗、实际占时/关键路径候选与
规则可消除量两轴、根因排序、唤醒链、Trace 因果投影和自动补采均在。OWN2 的生产
断线也已生效：系统不再输出基于模型 prose 推断的“正文首因”判词。但指标快照仍并列
同一 app-20 状态账的 `19 次切换/20 段` 与 `20 次切换/21 段`。

逐层回查 typed query JSON 后修正 ACCOUNT1 的残余归因：

1. canonical `state_churn`、root rank、wakeup impact 已共享精确
   `state_account_key`，所以不是 key 铸造或跨 payload 去重再次失败；
2. 第二行来自 `wakeup_chain.root_evidence` 的 typed `trace_gap`，其
   `gap_kind=no_eligible_wait` summary 为解释链为何停止而引用了完整状态账；
3. `runtimeTraceMetricSnapshotValues` 过去只要求 deterministic producer、正影响和
   完整数值 token，没有消费 typed predicate，于是把诊断引文误认成第二个测量发布者。

登记并实施 `EVAL-B30-ACCOUNT2/P1`：状态快照发布权改为闭集 typed predicate，允许
canonical `state_churn`、`state_drilldown`、wakeup causal 与 root-cause 派生面；
`trace_gap`、`missing_wakeup` 和未知 predicate 即使引用完整状态账也 fail-closed。
这不是按 19/20、20/21、app-20 或 case 名去重；不读取请求/答案原文、subject 或数值
相等关系。模型查询与 exact system supplement 继续可凭精确 key 合成一个席位；无 key
的真实独立 occurrence 保持 fail-open。

read 回放证明 `EVAL-B30-LANE2` 已 covered：答案不再出现
`StageWriteAnalyze/runWriteAnalyzePhase`，四个 read canonical stages、图和表均在。
但答案仍把 read 时序描述成 `Run` 遍历 `AllMainStages` 并逐阶段
`dispatchStage/applyStageOutput`，还声称后者把 `FinalAnswer` 写进
`BusContext.FinalAnswer`。生产事实是外层先单独执行 analyze，之后
`runTaskGraph -> runReadSchedulerLoop` 依据 analyzer-emitted TaskGraph 调度；
`applyStageOutput` 明确不消费 FinalAnswer，read scheduler 直接把返回值写入 task result。

其中第二个错误有确定的源码污染源：`internal/agent/agent.go` 的旧字段注释与生产实现
相反。本批登记并实施 `EVAL-B30-DOC2/P1`，只纠正文档合同，不改变运行时。第一个错误
登记为 `EVAL-B30-EDGE2/P1-watch`：现有成文前 typed lane guidance 已明确 stage
membership/order 不证明函数调用边，一次仍误推不足以授权系统重写答案；继续在其它机制
case 观察，若跨 case 复现则优先增强探索阶段的 grounded call-site 证据，而不是扫描最终
正文设硬门。

runner 的 `missing:Mutable` 单独记为 oracle watch：用户原文为“例如”，答案已覆盖状态
载体维度和 `BusContext`，不能为了 exact token 通过而注入答案关键词。只有后续出现
MutableState 角色的实质性遗漏才升级为系统 gap。

本批不改变 Trace 请求范围、投影/根因/唤醒/可消除算法、系统补采或模型答案所有权。
定向回归覆盖 diagnostic predicate 拒绝、canonical 单席、same-query 双 payload 单席、
正文已覆盖时不重复发布，以及中英文窗基准。工件：
`eval/parallel_selected_summary_evalcampaign_b30_replay_r2_20260801.md`、
`eval/parallel_selected_summary_evalcampaign_b30_replay_r2_20260801_manual_audit.md`。

验证：`go test ./internal/tool -count=1` 通过（158.346s）；
`go test ./internal/agent -count=1` 通过（2.816s）；`git diff --check` 通过。
状态：`EVAL-B30-ACCOUNT2=implemented/full-package-tests-pass`；
`EVAL-B30-DOC2=implemented`；`EVAL-B30-EDGE2=P1-watch`；
`EVAL-B30-LANE2=covered-live-replay`；`EVAL-B30-OWN2=covered-live-replay`。

#### B31 r1：多日志事实完整但可见分组失去 artifact identity；operation 正常

`main@9bfb4dbbb` 严格并行两个此前无本轮 campaign 记录的异构 case：

- `operation_system_inventory` runner/human PASS（27s）；
- `log_path_question_multi_runtime_files` runner/human FAIL（83s）。

operation 路由为 typed `operation/computer_operation`，四条低风险只读命令均 exit 0；
最终 macOS 版本、18 CPU cores、128 GiB 内存和 40-core Apple M5 Max GPU 均与原始
输出一致，没有进入 repo/write lane，未发现生产 gap。

多日志 case 的失败不是“未读取第二个文件”：CLI 已接纳两个 exact request-path log，
explorer 对每个文件各执行一次 `read_file`，panic 的 nil dereference、
`(*Store).Get/store.go:88`，以及 timeout 的 `context deadline exceeded`、
`(*Client).Fetch/client.go:41` 全部正确。最终两个 principal ordered-list 内容也正确，
但 block `title` 均为空；文件名只在末尾 caveat 一起出现，用户无法从可见结构确定哪个
列表属于哪个文件，因而 runner 的逐文件关系 regex 失败。

登记 `EVAL-B31-ARTGROUP1/P2-watch`：系统实际上已向 finalizer 提供完整 typed
RuntimeArtifactSelectionView（两个 artifact ID/kind/source）、两个逐文件 sub-topic，且
prompt 明确要求每 topic 有清晰标签；模型 thinking 也计划逐文件分组，最后仍漏 title。
单次成文波动不足以授权系统因 optional title 为空就拒绝事实完整答案，更不能扫描正文
文件名或事后代写标题。若该问题跨 multi-log/trace/data case 复现，通用方案是让 principal
block 通过 `claim_uses.evidence_id` 或新的 typed subject 字段绑定 artifact selection ID，
再验证结构化绑定；不使用答案 prose 关键词。

另登记 eval 可观测性 `EVAL-B31-RUNMETRIC1/P3`：runner 对 request-path admitted +
direct `read_file` 的 runtime 工件仍显示 `runtime_artifact_attached=none /
runtime_authority_path=none`。应在审计 telemetry 中增加
`direct_named_artifact_read`，避免把“直接读取精确工件”误看成无 runtime 权威；它不影响
本轮用户答案，优先级低于生产正确性。

本轮不修改生产代码，不为单次模型 title 遗漏增加硬门。工件：
`eval/parallel_selected_summary_evalcampaign_b31_log_operation_r1_20260801.md`、
`eval/parallel_selected_summary_evalcampaign_b31_log_operation_r1_20260801_manual_audit.md`。

#### B32 r1：data 当前态误携带已修复失败；write 端到端通过

`main@a33518189` 严格并行两个不同模式 case：

- `data_json_strict_ids` runner PASS / human FAIL（44s）；
- `github_issue_libgit2_foreach_worktree` runner/human PASS（163s）。

data case 的最终值没有错误：第一批因未消费 `instructions.md` 被正确拒绝，第二批用
`read_text("instructions.md")` 补读后消费 `instructions.md + users.json`，输出严格为
`{"ids":["u1","u3"]}`。但第二批 evaluator 的 live `workflow_state_json` 同时发布：

1. `material_coverage_sufficient=true`、两份必需材料均 covered、missing=0；
2. `has_answer=true` 且 output projection satisfied；
3. 第一批的 `required_material_not_consumed` 仍在 `workflow_violations`，current
   `decision_status=blocked`。

模型把第 3 项识别为历史残留并选择 complete，deterministic completion gate 也以当前
完整事实收口，所以最终答案正确；terminal journal 同样已经把旧错误降为
`last_nonterminal_error` 并保留 process/action lineage。真正的 gap 是 live current-state
reducer 直接消费 `WorkflowViolationsFromRecordExecution` 全历史，迫使 evaluator 自行调和
矛盾。runner 只看最终 JSON，未发现该过程权限分裂。

登记并实施 `EVAL-B32-DATASTATE1/P1`：新增
`ActiveExecutionViolationsFromRecords`，以最近一次结构化成功进展为 current/history
边界；`BuildWorkflowStateViolations` 只消费 active suffix，reasoning/journal 继续调用
原函数保留全历史。边界与 admission guard 一致，只读 `Result != nil && Err == ""`，
不扫描错误内容、action 名、用户请求、模型 thinking/rationale/final，也不为 JSON 或材料
消费类型写特例。

三层回归固定：

- 全历史函数仍返回旧失败和成功后新失败；active 投影只返回后者；
- 当前 state 不再重发成功前的 execution failure；
- REPL 真实接线中，补读两份材料并生成合规答案后，旧 material failure 不再进入
  `WorkflowViolations`/blocked graph，current decision 为 complete。

实现提交 `bab0fe0b4`；`go test ./internal/dataworkflow ./internal/repl -count=1` 通过，
`git diff --check` 通过。

write case 在隔离 worktree 只改 `repository.c` 两处运算符括号，准确保留 callback/lookup
的负错误码；未修改测试，`make check` 以 `-Wall -Wextra -Werror` 编译并执行 1/1 通过，
无 unverified/fallback。write analyzer 为 behavior contract 做四轮、planner 曾尝试不支持的
C verification probe 后自行删除，记为 `EVAL-B32-WRITEITER1/P3-watch` 效率观察；单次
出现不授权生产硬门或语言/case 特化。

状态：`EVAL-B32-DATASTATE1=implemented/full-related-tests-pass/replay-next`；
`EVAL-B32-WRITEITER1=P3-watch`。本批不修改 Trace 显式窗、因果投影、根因排序、
唤醒链、窗内可消除量、自动补采或模型答案所有权。工件：
`eval/parallel_selected_summary_evalcampaign_b32_data_write_r1_20260801.md`、
`eval/parallel_selected_summary_evalcampaign_b32_data_write_r1_20260801_manual_audit.md`。

#### B32 r2：execution violation 已分层；历史 evaluation 仍覆盖新 result

`main@e70441d90` 严格并行两个 data case：

- `data_json_strict_ids` runner/human PASS（49s）；
- `data_join_entity_reconcile` runner PASS / human FAIL（137s）。

strict JSON 再次真实经历“首批未消费 instructions→第二批补读”而非绕过 witness。第二批
live state 已无 `required_material_not_consumed`，coverage、answer、output projection 与
decision 一致为 complete；terminal 仍在 `last_nonterminal_error`/历史事件保留首批失败。
因此 `EVAL-B32-DATASTATE1=covered-live-replay`。

不同的 join/entity/reconcile 拓扑正确产出 30：三份材料全覆盖，canonical join 完成两个
Alpha 别名归一，5 条决策、2 条贡献、`expected=30/actual=30/reconcile=pass`，8 个 typed
阶段后严格只输出单行数字。新 gap 位于最后评估前的 live state：最新
`assemble_answer` result 已令 ledger/output 全部 satisfied，但 `decision` 仍发布上一批
evaluation 的 `continue_data`，reason 还声称 decisions/contributions/reconcile missing。
模型只能再次用其它 typed 图推翻这个 stale current decision 后选择 complete；runner 只验
最终值，未报过程矛盾。

登记并实施 `EVAL-B32-DATASTATE2/P1`。根因是 `latestDataTaskEvaluation` 无条件向后查找
任意历史 evaluation，不判断其后是否已出现新的执行 outcome。共享
`ActiveEvaluationFromRecords` 现在以结构化 record 边界裁定：

1. evaluation 附着在它所判断的 result record 上时拥有当前权限；
2. 更新的 `Result`、`Err` 或 typed `Violations` 到达且尚无自己的 evaluation 时，旧评估只
   留在历史，不再覆盖 current reducer；
3. 新 outcome 后附着新 evaluation 时，新评估正常恢复权限；
4. answer-face sticky repair contest 继续走原有专用 open/clear authority，避免普通时效规则
   洗掉尚未修复的 answer contest。

实现不扫描错误/reason/答案内容、用户请求、模型 thinking 或 case/action 名。单元测试固定
新 result、新 failure、最新附着 evaluation 三臂；REPL 接线测试固定旧 `continue_data`
不得跨越新 complete answer result。实现提交 `fc533bfb8`；
`go test ./internal/dataworkflow ./internal/repl -count=1` 通过，`git diff --check` 通过。

状态：`EVAL-B32-DATASTATE1=covered-live-replay`；
`EVAL-B32-DATASTATE2=implemented/full-related-tests-pass/replay-next`。本批仍不修改 Trace
显式窗、因果投影、自动补采或模型答案所有权。工件：
`eval/parallel_selected_summary_evalcampaign_b32_replay_r2_20260801.md`、
`eval/parallel_selected_summary_evalcampaign_b32_replay_r2_20260801_manual_audit.md`。

#### B32 r3：evaluation 时效已覆盖；live output graph 与 completion gate 双权威

`main@cb5b4c2a9` 严格并行两个多批 data case，runner 均 PASS：

- `data_join_entity_reconcile` human PASS（205s）；
- `data_multifile_reference_projection` human FAIL（163s）。

join/reconcile 本轮采用与 r2 不同的批次序列：rule→material extract→join→derive→filter→
contribution→reconcile→assemble。最终 5 条决策、2 条贡献、对账 pass 和答案 30 全部正确，
最后 evaluation 前的 current decision 直接为 complete，没有再携带旧 evaluation。

multi-file 也最终正确：inactive 行排除，10 条 entity resolutions、22 条决策、4 条贡献、
reconcile pass；targets 的 `GroupA, GroupX, GroupC` 完整投影为 `17,0,5`。因此
`EVAL-B32-DATASTATE2=covered-live-replay/cross-topology`。

但 multi-file 的首个 assemble result 只生成已有 GroupA/B/C 的标签化 roster，既不符合
纯逗号数字格式，也漏掉 reference universe 中的 GroupX=0。模型主动返回 repair，后置
completion gate 随后用 typed reference candidate 精确拒绝
`output_projection_incomplete_reference` 并生成 zero-fill 投影，因此最终答案没有错。
然而 evaluator 调用前的 live `OutputProjectionGraph` 与 `decision` 已先发布
`satisfied/complete`。根因是：

1. completion gate 使用 `dataTaskOutputReferenceProjectionGap`，已有 reference path/field、
   key count、answer item count 与 declared standing；
2. live `BuildWorkflowStateView` 私自构造简化 OutputGraph，只看 answer/reconcile/projection
   artifact presence，未消费上述精确事实；
3. `BuildWorkflowDecision` 在输入 status 为空时又先按 `NextStage=complete` 推导 complete，
   没让已存在的 output graph blocker 优先。

登记并实施 `EVAL-B32-DATASTATE3/P2`：completion output graph 的精确 input 现在由 live
state reducer 复用；选取的 answer result 继续走现有 terminal-answer single authority，
所以后续 answerless helper 或已验证 incumbent 不会被误判。decision 默认顺序固定为
typed violations → graph blocker → stage complete。reference gap 时 live state 直接发布
`incomplete_reference`、精确 key/item count 和合法 `assemble_answer` 下一步。

实现不扫描用户/模型文本，不根据 GroupX、数字或 case 名判断，也不修改答案；系统只把
completion gate 已有 typed 精确信息提前提供给 evaluator。定向测试覆盖 generic reducer
与真实 REPL reference-complete 接线；实现提交 `62c279edf`；
`go test ./internal/dataworkflow ./internal/repl -count=1` 通过，`git diff --check` 通过。

状态：`EVAL-B32-DATASTATE2=covered-live-replay/cross-topology`；
`EVAL-B32-DATASTATE3=implemented/full-related-tests-pass/replay-next`。工件：
`eval/parallel_selected_summary_evalcampaign_b32_replay_r3_20260801.md`、
`eval/parallel_selected_summary_evalcampaign_b32_replay_r3_20260801_manual_audit.md`。

#### B32 r4：live output authority 覆盖；同源伪关系与输出 stage 双权威

严格并行 2 个 data case，runner 2/2 PASS，人工 2/2 PASS：

- `data_multifile_reference_projection`：657s，15 批、5 次 repair、7 个失败 action，最终
  `17,0,5`；
- `data_basic_sum_with_rules`：155s，11 批、1 次 repair、2 个失败 action，最终 `17`。

多文件例对 `EVAL-B32-DATASTATE3` 给出真实回放正证：第一次 assemble 只输出已有分组、
遗漏完整 reference universe 时，live graph/decision 已发布
`incomplete_reference / continue`；补齐 GroupX=0 并形成纯 CSV 三项后才 complete。因此
`EVAL-B32-DATASTATE3=covered-live-replay`。

审计新增并已修两项共享 GAP：

1. `EVAL-B32-DATAREL1/P1`：简单求和的 typed contract 没有 entity obligation，但自动
   fallback 把同一 `orders.csv` 的 aggregate artifact 与 child record alias 当作独立
   source/reference，先 normalize，再 apply，再 mapping，形成无业务意义的关系循环。
   `ba6bf1b6a` 在自动 join/normalize scaffold 铸造点加入 typed lineage 独立性：当两侧均
   有 lineage 时，各侧必须至少有一个对侧没有的 root；缺 lineage 的兼容 producer
   fail-open，显式 model-authored self-join 不受影响。真实 observations/labels 双源正例保留。
2. `EVAL-B32-DATASTATE4/P1`：输出图和 decision 已要求修复时，ledger-only
   `next_stage` 仍为 complete，`allowed_next_actions` 仍为空。`b218cf65c` 让精确
   OutputProjectionGraph 在业务账本完成但最终投影未满足时，将有效 stage 重开到
   `emit_output_contract_answer`，并从同一 stage authority 开放 `assemble_answer`；业务
   ledger 未完成时仍优先原 stage，普通 satisfied 输出仍 complete。

完整 `go test ./internal/dataworkflow ./internal/repl -count=1` 两个代码批次均通过。过程中
还修正 `EVAL-B32-DATASTATE1` 的时效边界：同一 record 可同时带 Result 和 typed
Violations，只有 `Result present / Err empty / Violations empty` 才能让更早执行违规退为
历史，避免结果自我清除当前违规。

剩余观察：多文件例 657 秒和多次 typed 参数/输出 repair 成本很高，但现有 witness 混合了
filter 值选错、投影格式错误和模型重试，尚不足以授权某字段/某 case 的硬门。登记
`EVAL-B32-DATAPERF1/P2-watch`，后续用不同 schema 的 reference/join case 复现后再决定
是否收敛 action 参数契约。不得扫描用户或模型原文，不得由系统替模型生成业务答案。

状态：`EVAL-B32-DATASTATE3=covered-live-replay`；
`EVAL-B32-DATAREL1=implemented/full-related-tests-pass/replay-next`；
`EVAL-B32-DATASTATE4=implemented/full-related-tests-pass/replay-next`；
`EVAL-B32-DATAPERF1=P2-watch`。本批不修改 Trace 显式窗、因果投影、根因排序、唤醒链、
窗内可消除量或自动补采路径。

工件：

- `eval/parallel_selected_summary_evalcampaign_b32_replay_r4_20260801.md`
- `eval/parallel_selected_summary_evalcampaign_b32_replay_r4_20260801_manual_audit.md`

#### B32 r5：同源伪关系已覆盖；同数量错槽位暴露 live grounding 与看护权限 gap

`main@75767b8ba` 严格并行 2 个 data case，runner/human 均 PASS：

- `data_basic_sum_with_rules`：192s，7 批、2 次 repair，最终严格单值 `17`；
- `data_multifile_reference_projection`：234s，8 批、2 次 repair，最终
  `17,0,5`。

基础例没有再把 `orders.csv` 的聚合/子制品当成独立关系侧，执行序列收敛为
custom fallback 失败后的 extract→rule→filter→contribution→reconcile→assemble，
entity resolutions 从 4 降到 0，r4 的 11 批降到 7 批。因此
`EVAL-B32-DATAREL1=covered-live-replay`。多文件例从 r4 的 657s/15 批降到
234s/8 批，说明关系收窄同时消除了大部分级联参数修复；
`EVAL-B32-DATAPERF1` 降为 P3-watch，不再是当前高优先级生产 gap。

多文件例首个 answer 为 `17,4,5`：数量同为 3，但第二槽的 reference key 是无贡献的
`GroupX`，必须为 0，实际却被非 reference 的 `GroupB=4` 占据。终态
`output_reference_grounding_mismatch` 已精确给出 source path、key field 和逐槽差异，
模型随后重发 assemble 得到 `17,0,5`；但终态校验前的 live graph/decision 仍为
`satisfied/complete`。根因不是 DATASTATE4 stage 接线失效，而是 live
`OutputProjectionGraph` 只消费 reference cardinality gap，没有消费已存在的 typed
per-slot grounding report。

登记并实施 `EVAL-B32-DATASTATE5/P1`，同时审计出并纠正
`EVAL-B32-DATAAUTH1/P1`：旧看护用例只禁止未声明子集被确定性 zero-fill，却允许结构
census 继续以另一错误码硬阻塞同一合法答案，属于看护自身过硬。

通用裁定与实现如下：

1. live OutputProjectionGraph 新增 typed
   `reference_grounding_mismatch / cardinality_mismatch /
   ledger_domain_mismatch / mismatch_count`，同一状态驱动 decision、next stage 和合法
   `assemble_answer` action；终态 gate 仍保留更丰富的 path/field/slot violation。
2. hard grounding 的“参考集合是什么”和“用户是否要求全量投影”分离：结构 key-table
   census 只做候选发现；硬权限必须来自 typed `complete_reference` 合同，或模型已发出的
   `assemble_answer.reference_path + reference_key_field`。
3. `reference_path` 若是生成别名（真实 replay 为 `target_list`），别名本身不得成为参考
   权威；只允许通过唯一、相容的 artifact `source_paths` 回溯到原始 source material，
   再从原文件字节解析 key universe。多源/冲突 lineage fail-open，不猜测。
4. 显式 complete-reference 若指向不能回溯到相容源字段的生成物，发布 typed
   `output_reference_authority_invalid` 让模型修复，禁止系统偷偷换成另一个结构候选。
5. 同数量的 value/slot grounding mismatch 只重开模型 answer stage，不进入 deterministic
   projection plan；model repair 和 continuation 均失败后，既有 validator-proposal 候选车道
   才可工作。系统没有直接写值、替换答案或抢先于模型。
6. 已声明的纯 cardinality gap 保留既有 `incomplete_reference` 机械投影权限；未声明子集
   新看护明确要求 completion guard 完全为空，不能只排除某一个错误码。
7. 全部硬判只读 typed contract/action/artifact lineage/ledger/reference bytes，不扫描用户
   原文、模型 thinking/reason/final、case ID、字段业务词或答案 prose。

实现提交 `5bdfe9ac9`。`go test ./internal/dataworkflow ./internal/repl -count=1`
全部通过（dataworkflow 0.709s，repl 30.724s），`git diff --check` 通过。

另记 `EVAL-B32-DEFERAUTH1/P2-watch`：基础例一个使用不存在
`amount_status/id_status` 的延后 qualification candidate 被正确拒绝后，旧 field-contract
失败曾短暂与已完整 ledger/output current state 竞争；最终答案未受影响。单个 specimen
不足以授权新的 active-suffix 规则，待不同 schema 第二次复现。

状态：`EVAL-B32-DATAREL1=covered-live-replay`；
`EVAL-B32-DATASTATE5=implemented/full-related-tests-pass/replay-later`；
`EVAL-B32-DATAAUTH1=implemented/full-related-tests-pass`；
`EVAL-B32-DEFERAUTH1=P2-watch`。本批不修改 Trace 显式窗、因果投影、根因排序、唤醒链、
窗内可消除量、自动补采或模型答案所有权。

工件：

- `eval/parallel_selected_summary_evalcampaign_b32_replay_r5_20260801.md`
- `eval/parallel_selected_summary_evalcampaign_b32_replay_r5_20260801_manual_audit.md`

### B33：Trace 有限事实 / 显式因果窗权限对照（2026-08-01）

`main@21959f219` 严格并行 2 个 real Trace case，runner 2/2 PASS：

- `real_trace_c2_dstate_iowait`：129s；
- `trace_query_donghu_real_frame_multicausal`：142s。

权限对照正确：前者由 typed `runtime_question_profile=bounded_fact_set` 收窄，最终
`trace_query_final_projection_blocks=0`，没有把状态/时间/caller 的有限事实查询扩大成调用链
或全量因果报告；后者由 `explicit_time_window + causal_diagnosis` 保持完整
`## Trace 因果投影`、确定性自动补采、根因席、唤醒链、实际占用和规则内可消除双轴。
因此“窄事实不注入投影”没有破坏显式时间窗的因果投影能力。

人工审计结论：

1. 有限事实答案的主值正确：目标 D-state=0，io_wait 恰 3 段、合计 0.635ms，三段
   时间/时长和 `sync_buffer_read_wi` caller 均完整。但首次 `thread_timeline` 文本预览只露出
   前两段，第三段虽已存在于 typed payload/ledger，模型仍为它重复查询并回读 141KB JSON
   4 次、调用一次 grep。登记 `EVAL-B33-WAITPREVIEW1/P1`。
2. 同一 caller 的角色在系统内自相矛盾：final prompt 正确写明它只是
   `kernel-reported wait call-site`，系统交叉核验/投影/上下文旧面却显示为“等待对象”或
   `cause`，模型最终也写成“由该函数引起”。`sched_blocked_reason.caller` 不提供资源对象、
   owner 或 holder 身份。登记 `EVAL-B33-CALLERROLE1/P1`。
3. 显式窗的 typed wakeup path 确有
   `ThreadPoolForeg -> NetworkService -> CookieMonsterCl -> target`，但模型把边进一步写成
   “同步阻塞/等待某线程”。唤醒依赖边本身不证明锁/资源持有，也不保证所有 hop 构成一条
   连续同步阻塞调用链。登记 `EVAL-B33-WAKEWORD1/P2`。
4. 模型明示 `causal_conclusion=unproven`，但仍把相互重叠的 23.994ms 与 19.041ms 写成
   “约43ms”后又称“非叠加”，且对“真实耗时/新探索方向”综合弱于“规则内可消除席”复述。
   finalizer 已提供 model-owned 双轴、cross-row additivity forbidden 和 unproven ceiling；单次
   witness 暂记 `EVAL-B33-MODELSYNTH1/P2-watch`，等待不同 trace/model 第二次复现。禁止据此
   增加正文关键词硬门或由系统替模型写结论。

已实施的通用处置：

- `e881ea321`：目标线程的小型 typed wait occurrence 清单在 trace_query 工具文本头部完整
  发布，最多 8 条；超过容量同时发布 `status=incomplete`、`account_status`、omitted 数和
  `payload_ref` continuation。只有完整 account 才使用 exact count/sum 字段，不完整 account
  使用 observed lower-bound 字段。该变更不读取问题原文、不选择答案范围，也不缩窄任何
  causal view。另在 model-owned decision handoff 增加 wakeup-path 语义边界软提示；不拒绝、
  不改写模型正文。`internal/tool` 全量 157.295s、`internal/agent` 全量 3.267s 通过。
- `4b3c538f4`（`B33-CALLERROLE1`）：系统交叉核验、Trace 投影和成文上下文统一显示
  “内核调用点 / kernel wait call-site”，明确 caller 不能铸造 waited-on object/owner/holder；
  底层 typed caller 值、call-site family grouping、排序、有效归因、因果投影、自动补采和
  model block wire 均不变。context / orchestrator / tool 三个相关包全量回归通过
  （0.907s / 9.399s / 160.584s）。

状态：`EVAL-B33-WAITPREVIEW1=implemented/full-related-tests-pass/replay-later`；
`EVAL-B33-CALLERROLE1=implemented/full-related-tests-pass/replay-later`；
`EVAL-B33-WAKEWORD1=implemented/soft-guidance/full-related-tests-pass`；
`EVAL-B33-MODELSYNTH1=P2-watch`。

工件：

- `eval/parallel_selected_summary_evalcampaign_b33_trace_scope_r1_20260801.md`
- `eval/parallel_selected_summary_evalcampaign_b33_trace_scope_r1_20260801_manual_audit.md`

### B34：Trace 语义 span × read history/current-source 跨模式对照（2026-08-01）

`main@153da8e7d` 同一二进制快照严格并行 2 个 case，runner 2/2 PASS，但人工 0/2：

- `trace_query_frame_semantic_span_optimization`：146s；
- `read_combo_git_diff_hunk_current_code`：174s。

Trace typed 事实本身完整：系统根因席 #1 为 `worker-200 class_verification`
4.600ms、#2 为目标自身 runnable 0.800ms，`frame_causality=unproven`、
`frame_evidence_status=absent`，并已向模型明确 lower-priority dependency 与 wakeup edge
均不单独证明同步阻塞/持锁/优先级反转。模型仍把它写成“低优先级 CFS 线程迫使 RT 线程
等待”的反转机制，同时 caveat 又承认没有 typed priority-inversion impact，形成模型内部
矛盾。系统没有删除或改写这段模型 principal；末尾只做了 advisory 交叉核验并指出正文与
typed #1 不一致。因此该项不能通过恢复系统改写来修，登记
`EVAL-B34-MODELCAL1/P1-model-owned-open`。

Trace 系统面另有独立确定性错误：主要时间占用表把同一个
`app-100 runnable 5.005000..5.005800 / 0.800ms` 物理区间显示两次，只因 chain state
与 ranked target-self 两个发布 lane 使用不同 EvidenceID。登记并实施
`EVAL-B34-TRACEPHYS1/P1`：占用表以
`canonical subject + typed physical state family + exact start/end` 去重物理镜像；保留首个富
载体，D-state/io_wait 不合并，projection 节点、根因排序和可消除账均不改。提交
`7527c29cc`，`internal/tool` 全量 161.401s 通过。

read 模式的模型 principal 基本正确地区分最新 docs commit、历史 diff 线索与当前源码，
但系统「清单完整性补充」产生了错误表：3 个纯文件 members 与 3 个一般机制
support_refs 被按下标强行 zip，导致 `trace_query.go:7907` 错配到
`answer_document_trace_decision_handoff.go`，下一锚又错配到测试文件。登记并实施
`EVAL-B34-READANCHOR1/P1`：当完整 member_set 的所有成员都是 typed 源码路径时，优先
按 canonical file path identity 连接 support anchor；同路径多锚优先原槽，唯一同路径锚可
跨槽重连，无同路径锚保持 uncited，禁止借用别的文件。配置层/符号等非纯路径 roster 的
既有 positional contract 不变。提交 `1bf335d90`，`internal/types` 与 `internal/tool`
全量 21.461s / 162.238s 通过。

两 case 都是 runner PASS、人工 FAIL，登记 `EVAL-B34-ORACLE1/P1-open`。Trace oracle
只验证 span/线程/时间窗存在，未校验 causal caliber；read oracle 只验证 diff/current-source
词面，未校验系统补表的 member→anchor identity。由此可确认当前看护并非“过硬”，而是
偏弱、会假绿；不能用 `EXPECT_NOT_CONTAINS` 某个根因词或扫描用户/模型原文补洞。后续应
采用模型自有的 typed diagnosis decision/独立模型评审，以及 typed inventory row identity，
系统不得替模型写首因或改正文。

状态：`EVAL-B34-READANCHOR1=implemented/full-related-tests-pass/replay-later`；
`EVAL-B34-TRACEPHYS1=implemented/full-related-tests-pass/replay-later`；
`EVAL-B34-MODELCAL1=P1/model-owned-open/no-system-rewrite`；
`EVAL-B34-ORACLE1=P1/open`。

工件：

- `eval/parallel_selected_summary_evalcampaign_b34_crossmode_r1_20260801.md`
- `eval/parallel_selected_summary_evalcampaign_b34_crossmode_r1_20260801_manual_audit.md`

### B35：write 最小修改 × log 错误权限与路径解析性能（2026-08-01）

B35 首批在 `main@30ed39eab` 严格并行 2 个 case：

- `patch_go_typo`：runner/human PASS，85s。只执行 `retrun`→`return` 一行 patch，隔离
  worktree 与 `go test -json ./...` 均正确。case 的 `kind=patch` 是能力探针，不是产品正确性
  的唯一表示；等价、精确的 modify 操作不应被产品审计判错。登记
  `EVAL-B35-WRITEORACLE1/P2-eval-semantics`，当前无需修改生产路径。
- `logtri_goroutine_dump`：runner PASS / human FAIL，73s。旧 case 注释和 triage prompt 均把
  panic dump 中每个 goroutine block 当 sibling error；事实上日志只有一个 fatal header，87、
  120 只有 capture-time stack snapshot。最终进一步编造三者都崩溃、共享同一 map、没有锁。

分批实施与复放：

1. `b707cf4ed` 规定 `errors[].message` 必须是 held log 的精确子串，并把无独立
   error/exception header 的线程块移出 errors。r2 的 `errors[]` 已从 3 条降为 1 条，但
   observation 解释仍被升级为主事实。
2. r2 Java 例的 `emit_log_triage` tool call 阻塞约 444s、总耗时 522s。根因是每个 bare
   Java basename 都独立 `filepath.Walk(repoRoot)`，并反复扫描 Git ignored 的海量 eval
   产物。`93bebf522` 改为一次 Git-aware inventory（tracked + non-ignored untracked），非 Git
   仓库才单次 walk 回退；仍逐候选 stat、语言排序、函数标识核验。r3/r4 tool return 约
   41ms/40ms，整例降到 115s/91s。登记 `EVAL-B35-LOGPERF1=covered-live-replay`。
3. `2768e0e32` 新增通用 `thread_snapshot` typed kind；validator 将其强制归一为
   `severity=info / diagnostic=false`，ledger 进入 artifact-span supporting/soft lane，所有下游
   prompt 明示它只能证明 dump 时正在执行的 frame，不能单独证明 crashed/emitted error/
   same resource/causality。旧 case 的“三个 peer errors”错误规格同步纠正。
4. r3 暴露新的旁路：triager 额外生成不存在于附件的 `runtime_event.evidence` 整句，重新
   铸成 principal。`09e9cba4d` 对所有 `observations[].evidence` 增加精确 held-log 来源校验；
   没有 evidence 的 model summary 永远只能 supporting/soft。该门只比较 schema 字段与持有
   工件，不扫描用户请求、thinking、final prose 或关键词。
5. r4 系统数据合同已正确：一个 explicit error、两个 thread snapshots、快照 supporting/
   soft、authority boundary 在 analyzer/explorer/finalizer prompt 可见；但模型仍自行发出
   “同时出错 goroutine=3”的 principal aggregate 并在最终答案声称 87/120 崩溃、共享 map/
   无锁。系统没有删除、替换或重写模型块。登记
   `EVAL-B35-LOGMODEL1/P1-model-owned-open`；继续添加答案关键词门或系统结论替换会违反
   model ownership 红线，因此停止在此单例上加硬约束。

看护审计结论：

- `logtri_goroutine_dump` 只检查 map/race/并发词面，四轮均可在错误答案上 PASS，登记
  `EVAL-B35-LOGORACLE1/P1-open`。优先补结构化 producer oracle（error count、typed snapshot
  count/kind）；最终“谁实际触发错误”的语义仍应由模型基于 typed authority 作答或交给独立
  模型评审，不能用 `EXPECT_NOT_CONTAINS` 扫最终原文。
- r3 Java 偶发在 `resolved_files=0` 下探索 Codrax 当前源码并引用日志解析器，r4 未复现，记
  `EVAL-B35-EXTSCOPE1/P2-watch`，待第二个不同外部栈复现后再改 typed origin/scope 路由。
- Java r2-r4 均把 `Caused by` 扩写成 return-null、catch/wrap 和 PostgreSQL 实现机制；异常
  nesting、消息和 frame order 是直接事实，具体控制流是模型推断。登记
  `EVAL-B35-CAUSECAL1/P2-watch`，不得按类名、端口或题面关键词硬拟合。
- 当前 watchdog 同时存在两类边界：`patch_go_typo` 的 patch-kind 只适合作能力探针，若拿来
  判产品 correctness 就过硬；goroutine 的现有答案 regex 则明显过松。runner verdict 与人工
  correctness 必须继续分栏。

状态：`EVAL-B35-LOGPERF1=covered-live-replay`；
`EVAL-B35-LOGAUTH1=implemented/full-related-tests-pass/live-typed-contract-covered`；
`EVAL-B35-LOGMODEL1=P1/model-owned-open/no-system-rewrite`；
`EVAL-B35-LOGORACLE1=P1/open`；`EVAL-B35-EXTSCOPE1=P2-watch`；
`EVAL-B35-CAUSECAL1=P2-watch`；`EVAL-B35-WRITEORACLE1=P2-eval-semantics`。

工件：

- `eval/parallel_selected_summary_evalcampaign_b35_write_log_r1_20260801.md`
- `eval/parallel_selected_summary_evalcampaign_b35_write_log_r1_20260801_manual_audit.md`
- `eval/parallel_selected_summary_evalcampaign_b35_log_authority_r2_20260801.md`
- `eval/parallel_selected_summary_evalcampaign_b35_log_authority_r2_20260801_manual_audit.md`
- `eval/parallel_selected_summary_evalcampaign_b35_log_authority_r3_20260801.md`
- `eval/parallel_selected_summary_evalcampaign_b35_log_authority_r3_20260801_manual_audit.md`
- `eval/parallel_selected_summary_evalcampaign_b35_log_authority_r4_20260801.md`
- `eval/parallel_selected_summary_evalcampaign_b35_log_authority_r4_20260801_manual_audit.md`

### B26-OWN：Trace 精确信息与模型结论的职责边界回裁（2026-08-01）

客户/人工 witness：
`.codrax/output/20260801-142120.269-13967.html`。最终答案首段只有固定的
“尚不能证明 + 两个维度”声明，随后堆叠系统表格，缺少丢帧根因综合、关键优化方向和
可消除影响的结论性语言。对照同轮原始模型工件
`.codrax/blob/20260801-141800-000-13967/tool-call_function_spdvrqqvd67s_1-emit_answer_document-params-16aa3155.json`
确认：模型原本提交了 7 个块，包含总结、四跳唤醒依赖、根因排序、修向、代表窗和
边界；持久化阶段在 `causal_conclusion=unproven` 下删除所有模型块，只保留系统生成的
因果投影。此次“泛泛而谈”的直接原因不是模型没写，而是系统越权清空了模型推理。

#### 破坏红线的提交责任链

`git log -S` 与逐提交 diff 复核后的责任链如下（提交时间按原提交记录）：

| commit | 时间 | 引入的越权行为 | 当前裁定 |
|---|---|---|---|
| `1491ff97d585194e8c4eade30c22e735553e45bf` | 2026-06-29 05:59 +08:00 | 首次把 Harmony priority typed 事实接成 persist-time 模型标题/正文/item/caveat 字符串改写 | 生产调用撤销；事实只进入 prompt/typed 系统明细 |
| `5d91b433d8fd3b104a50421c28cf95f9d1708fdc` | 2026-07-10 18:30 +08:00 | 新增 priority-inversion 与 low-coverage 根因措辞扫描/替换；当系统表缺席位时还曾允许删除最后一个非系统块 | 三类 model-prose rewrite 的生产调用撤销；非系统块驱逐已由 `c60400844` 先行修成禁止，保留系统块让位/跳过逻辑 |
| `cf6b31cc520a3a529fb33b892f8b96119ba267cd` | 2026-08-01 03:47 -07:00 | 新增 `materializeRuntimeTraceCausalConclusionBlock`，在 unproven 下删除全部模型块并生成系统 principal summary；本次客户答案退化的直接提交 | 整条 materializer、reserved ID、层级席位删除 |
| `62ee975e955da2e2f45dc885ccaccacd4f12c634` | 2026-08-01 04:29 -07:00 | 新增 bounded-wait 系统 principal，同样过滤掉所有模型块 | 整条 materializer、reserved ID、层级席位删除 |

其中 `cf6b31cc5` 与 `62ee975e9` 是“系统删除模型回答并自行替换”的两个直接破坏
提交；`1491ff97d` 与 `5d91b433d` 是更早形成“持久化层可以改模型结论”错误方向的源头。
修复不采用反向关键词表，而是在持久化 choke point 增加精确结构不变量：runtime-trace
补充与层级整理前后，所有非 system-marker block 的完整 JSON wire 与相对顺序必须一致；
否则本次 emit fail-loud，不允许悄悄发布被系统改写的答案。

#### 红线裁定

Trace 因果投影是“两路采集、一个确定性编译器”：

1. 模型探索产生 accepted `trace_query` typed observations；
2. 系统按请求范围、目标和覆盖缺口产生 deterministic `system_supplement` observations；
3. 投影编译器把两路观测合并为根因席位、唤醒链、实际占用、可消除量、coverage 和
   证据索引。

系统只拥有第 3 步的精确信息与因果上限，不拥有用户可见的诊断结论。最终职责冻结为：

- 系统可在成文前提供 typed 精确值、口径、候选与写作引导；
- 系统可在成文后追加确定性因果投影/明细/边界作为审计面；
- 模型必须自行综合“实际占时/新探索方向”和“现规则可消除/优先修复方向”，给出关键
  优化建议；
- 系统不得删除模型块、用系统 summary 替换模型 summary、扫描并改写模型结论措辞，
  也不得让系统投影冒充最终诊断；
- `causal_conclusion=unproven` 约束结论强度，但不等于删除推理。模型仍应给出“最值得先
  验证的候选、理由和下一步”，只是不能宣称已经证明具体 frame/deadline 因果。

#### GAP 与通用处置

| ID | 级别 | GAP | 处置 |
|---|---|---|---|
| EVAL-B26-OWN1 | P0 | `materializeRuntimeTraceCausalConclusionBlock` 在 unproven 下删除全部模型块并生成系统主结论 | 删除整条 replacement materializer；模型块保持原字节/原顺序，投影作为后续 sibling 数据面 |
| EVAL-B26-OWN2 | P0 | 窄事实 `materializeRuntimeTraceBoundedWaitConclusionBlock` 同样删除模型正文、由系统接管 summary | 删除 replacement materializer；完整 wait roster 继续作为系统精确明细追加 |
| EVAL-B26-OWN3 | P0 | persist 链仍调用 priority-inversion、low-coverage、Harmony priority 三类 model-prose rewrite | 三类 production 调用全部撤销；精确 authority 与口径迁到 finalize prompt/系统明细，不再改模型字符串 |
| EVAL-B26-OWN4 | P0 | 只删当前函数仍可能换名复发，过去没有“系统补充不得改变模型 block wire”的 choke-point 不变量 | persist 在 runtime trace 补充前后和 hierarchy 前后各做一次 system-marker 隔离的精确模型 block wire 校验；删除/替换/改字/调换模型块均 fail-loud |
| EVAL-B26-SYNTH1 | P1 | finalizer 虽能看到大 ledger，但缺少一个高显著度、同源的双维度决策输入，模型容易只列证据或漏掉未计价占用 | 在 finalizer 成文前编译 `Trace Decision Inputs`：目标状态症状、选定唤醒路径、Axis A typed state/span 占用、Axis B 排名/有效归因/修向，并逐行保留 exploration/system-supplement 来源；明确模型拥有结论且必须总结方向 |

`Trace Decision Inputs` 只进入 prompt，不生成用户可见 AnswerBlock，不参与 hard gate，
不读取 RawRequest、case ID、模型 thinking/summary/final prose。Axis A 只收 typed
state/semantic/business-span 时间，不把无底层状态的 priced composite 假装成真实占时；
Axis B 只收正 `rank/effective_impact` 席位。跨行、跨修向、wall-clock 与 cpu·ms 继续禁止
相加。多工件按 projection partition 分节，禁止跨 artifact 合成一个根因。

看护强度回裁：模型所有权使用 hard structural invariant，因为输入是精确的
`SystemGeneratedKind` 与完整 block wire；双维度综合只使用 soft prompt，不拒绝、不重试、
不修改答案。prompt 复用 `RuntimeTraceReportMaterializationAllowed` 的同一 typed 权限：
`bounded_fact_set` 即使探索偶然采到 causal row 也不注入；显式 typed time window 仍优先
授权。提示内容按实际数据自适应：两轴都存在才要求比较两轴，只有一轴时只提示该轴并
禁止虚构另一轴。测试只固定这组权限与 typed 值透传，不把客户线程名、case ID 或期望
根因作为生产门控。

不变量：显式时间窗的 Trace 因果投影、根因排序、唤醒链、代表窗、窗内可消除量、
双维度占用表、coverage 和系统自动补齐全部保留；无窗有限事实仍不扩张为全量因果报告。

验证：

- `go test ./internal/agent -count=1`：通过（3.048s）；
- `go test ./internal/tool -count=1`：通过（169.020s）；
- 两条旧回归曾要求 persist 改写 low-coverage 根因句和 Harmony priority 正文，已反向
  改为“模型 block wire 不变、typed 边界/事实以 sibling 系统面发布”。

状态：`EVAL-B26-OWN1/2/3/4=implemented-full-tests-pass`；
`EVAL-B26-SYNTH1=implemented-full-tests-pass`；下一步用同一显式窗 case 与一个无窗
有限事实 case 严格并行 2 个回放。

#### B26-OWN r1：所有权通过，暴露工件身份与调度阶段语义新 GAP

当前 `main@629828739` 重建二进制后严格并行 2 个回放（sweep
`20260801-152610`）：runner 2/2 PASS；人工为 bounded fact PASS、显式窗 FAIL。

OWN 验收本身通过：

1. bounded fact 仍是模型的单一 summary，系统只追加完整 target-wait roster；没有
   `Trace Decision Inputs`、没有 Trace 因果投影、没有 replacement summary；
2. 显式窗 finalizer 收到两轴 typed handoff，模型一次 emit 的 6 个 blocks 在最终答案中
   全部保留，系统投影作为 sibling 追加；日志只有 materialize/add/reorder，已无
   `replaced model ... conclusion` 路径；
3. 模型给出了总结、四跳链、根因排序、资源背景、代表窗与修向，不再退化为固定系统
   声明；`EVAL-B26-OWN1/2/3/4` 因此保持 covered。

人工 FAIL 来自两个独立、可泛化的新 GAP：

| ID | 级别 | 现象与机制 | 最优方向 |
|---|---|---|---|
| EVAL-B26-ALIAS1 | P1 | 同一附件的探索行以 blob `attached_trace.txt` 标识，系统补采以原始 `donghu_tieba_frame.systrace` 标识；projection partition 未消费二者的 attachment lineage/content identity，误当成两个互不校准工件，整套投影、明细、证据索引重复一次 | 在 ObservationLedger/ProjectionSet 分区前建立 typed runtime-artifact canonical identity；只合并能由同一 attachment ID、不可变快照 lineage 或已验证 digest 证明同一内容的 alias，不用 basename/时间戳相似度猜测；保留 exploration/system-supplement source lane |
| EVAL-B26-PHASE1 | P0 | 模型把 wakeup-chain 的 11.103ms 投影解释成“主线程被唤醒后的 runnable 调度延迟”，进而声称 RT 主线程因 CFS 调度器优先调度 CFS 任务而延迟；但目标全窗 runnable 仅 3.636ms。这里混淆了 pre-wakeup dependency/sleep 投影与 post-wakeup ready-to-run 等待，随后又给出“把中间节点移入 RT/设置 PI”的过强建议 | 在 trace producer→observation→projection→finalizer handoff 贯通 typed `impact_phase`/`scheduler_role`，明确 dependency completion before wake 与 target runnable after wake 是互斥阶段；Harmony RT/CFS 只作为精确类事实和软语义说明，不扫描/改写模型答案。只有 confirmed holder/waiter 才建议 PI；仅 dependency candidate 要求先验证等待原语/临界区/CPU 竞争 |

`EVAL-B26-SYNTH1` 的 prompt 接线有效但不升级为硬验收：本轮模型确实消费了两轴输入，
仍可能在自然语言中侧重某一轴，这是模型波动；系统不得为追求固定段落结构再接管答案。
后续只修 ALIAS1 的 typed 身份收敛和 PHASE1 的精确信息/软引导，不增加用户原文或模型
输出关键词门。

#### B26-PHASE1：用既有 chain depth 单一权限补齐调度阶段语义

代码冷读后对原方案作一处架构收敛：不再新增一份可与 chain depth 漂移的
`impact_phase/scheduler_role` wire。`tracequery.expandChain` 已经给出了精确且更强的
单一权限：depth=0 是目标分支自身，depth>0 的节点只可能来自
`expandChainSleepSegment` 对 `[下游睡眠开始, sched_wakeup 时刻]` 的上游递归；该正
depth 已经经 `depth/chain_depth` rich note 进入
`TraceCausalProjectionNode.ChainDepth`。因此 finalizer handoff 只在
`ChainDepth>0` 时派生闭集语义 `impact_phase=pre_wakeup_dependency`，零值/旧记录保持
unknown，禁止从线程名、state、priority class 或模型文字猜相位。

本批实现：

1. 只要决策输入中存在正 depth 节点，prompt 明示该席位测量的是“下游尚未唤醒时”的
   上游线程状态/工作；它可解释下游 sleep/blocked，但不是下游唤醒后的 runnable/
   dispatch delay。
2. 唤醒后延迟只能由下游线程自己的 typed runnable interval 与同 CPU scheduler
   ordering 支撑；不得用上游 CFS class 推导“CFS 抢占/延迟了已唤醒 RT 目标”。
3. Axis A / Axis B 的每条正 depth 行都携带
   `impact_phase=pre_wakeup_dependency`。优先级候选另带
   `priority_candidate_scope=dependency_scheduler_supply`，并明确该席位不提供
   post-wakeup preemption 或 holder/waiter 权限。
4. PI mutex、RT promotion 等机制性建议仍归模型，但 candidate-only 时提示先验证
   等待原语/临界区/CPU 竞争；另有 typed 证明时模型可以升级结论。
5. 全部改动只在 prompt handoff；不创建 AnswerBlock、不扫描 RawRequest/case ID/
   模型 prose、不拒绝/重试、不删除/替换/改写模型答案。显式窗的投影、根因排序、
   唤醒链、窗内可消除量与系统补齐权限不变。

看护强度：测试固定 typed depth→phase 的单向投影、候选权限边界和 depth=0/absent
不猜相位；不固定客户线程名、11.103ms、期望根因、最终答案段落或模型措辞。模型所有权
结构不变量继续由 B26-OWN4 独立保护。

验证：

- `go test ./internal/agent -count=1`：通过（2.884s）；
- 定向覆盖双轴 handoff、显式窗接线、bounded-fact 不扩张，以及 legacy depth=0
  反向臂。

状态：`EVAL-B26-PHASE1=implemented / full-agent-tests-pass / replay-with-ALIAS1-next`。

#### B26-ALIAS1：补齐当前 session 附件快照的 source 归一化

真实日志证明不是 projection partition 再次缺少合并算法，而是既有 capture identity
接线的生产前置条件没有成立：

1. 主附件原始身份为
   `eval/fixtures/real_traces/donghu_tieba_frame.systrace`，当前 session 将其不可变
   快照物化为 `<WorkDir>/attached_trace.txt`。
2. prompt 给出了该绝对快照路径，模型以 `source=path,path=<该路径>` 调用
   `trace_query`。
3. `traceQueryPathDefaultsToAttachedTrace` 只识别空/`.`/RepoRoot/WorkDir，遗漏
   `<WorkDir>/attached_trace.txt` 这个真实文件地址，所以探索记录被铸成
   `artifact_id=trace_query`。
4. ObservationLedger 的安全门只允许 `artifact_id=attached_trace` 的保留 blob 使用
   attachment lineage；它正确拒绝把普通 `trace_query + basename` 猜成附件。系统补采
   记录带原始附件身份，于是两边无法共享 `CaptureIdentityPath`，投影被分为两件。

最优修复前移到 trace_query source 解析单点：当且仅当 resolved path 精确等于当前
`WorkDir/attached_trace.txt` 时，把 `source=path` 规范成 `source=attached_trace`。
由此 producer 自然发出 `artifact_id=attached_trace`，既有 ledger requalify 再把它绑定
到 run-entry preflight 的唯一 attachment 原始身份；projection、pair relation、目标状态
等全部既有消费者自动共享一件 capture。没有新增 basename、时间戳、内容相似度或模型
文字启发式。

反向不变量：RepoRoot、其它目录或其它 session 中即使也叫 `attached_trace.txt`，仍是
普通显式 path，保留 `artifact_id=trace_query`；多附件时既有 unique-attachment 门继续
fail-open，不猜合并；Path/SupportRefs 仍保存各自真实坐标，只有 CaptureIdentityPath
用于物理工件分区。

验证：

- 精确 session blob path：`source=attached_trace`，typed observations 的
  `ArtifactID=attached_trace`，ledger `CaptureIdentityPath=customer.systrace`；
- 同名 repo 文件反向臂：`source=path`、`ArtifactID=trace_query`；
- capture identity / attachment ledger 定向回归通过（types 0.648s）；
- `go test ./internal/tool -count=1`：通过（166.530s）。

状态：`EVAL-B26-ALIAS1=implemented / full-tool-tests-pass / same-pair-replay-next`。

### B24 r1：写模式通过，调用边方向权限缺失（2026-08-01）

严格并行 2 个 case，机器均 PASS；人工结果为 1 PASS / 1 FAIL：

1. `patch_c_typo` 完成 plan→apply→verify，applied tree 只有
   `main.c` 的 `retrun`→`return` 一行，编译与 diff 验收均通过。计划摘要出现一次
   “误写成 retrun”自反文案，但 ChangePlan rationale、实际 patch 和最终验证均正确，
   暂按模型文案波动保留，不立硬规则。
2. `qf_sequence_analyzer_gate` 的 grounded source 已明确
   `buildAnalysisIR -> gate.RunWith`；`gate.go:134-135` 的真实调用方向是
   `gate.Run -> RunWith`。最终 principal item、summary 和 Mermaid 却统一写成
   `gate.RunWith -> gate.Run`，只引用 `func Run(...)` 定义行。定义证据证明符号存在，
   不能证明 caller→callee 方向。
3. 登记 `EVAL-B24-EDGEAUTH1/P0`：现有 diagram gate 只校验端点出现、typed
   `relation_kind` 与标签关系一致，没有把结构化边方向与
   `EvidenceItem.Subject/Object + ClaimFormOf(call_edge)` 合取。模型因此可用 definition
   citation 铸造反向调用边。该项是一般调用图/时序图权限缺口，不是
   `gate.Run` 特例。
4. 登记 `EVAL-B24-EVALDIR1/P1`：eval runner 只用名称/正则存在性判断时序图，
   没有方向 oracle，所以错误答案仍机器 PASS。后续应让 case 声明 typed expected
   edge，runner 比较 from/to，不用答案关键词近似方向。
5. 登记 `EVAL-B24-KEYSET1/P2`：用户要“关键中间函数”，答案却发布 18 项主列表、
   9 项完整性补充和 27 项完整清单；条件调用也被压成无条件顺序。这是
   `key subset` 与 `complete roster` 缺少 typed scope/cardinality 区分，不应通过扫描
   “关键”原文或删某个固定函数解决。
6. `EVAL-B22-COUNTDOMAIN1/P3` 再现：可见整数 18、1570、1820、2663 与 aggregate
   27 被软计数器放在同一比较域；未改变最终内容，维持 P3。
7. B21 CALLEE/SPAN 软提示不足再次复现；本轮把可结构化的 diagram call edge
   提升到精确证据合同，非结构化 prose 的通用实体归属仍保持开放，禁止关键词硬门。

#### B24-a：structured call-edge evidence direction authority

本批采用共享 typed authority，而不是对答案文本做箭头/函数名特判：

1. 仅当 `AnswerSemanticView.Family == QFCallChain` 时启用；Trace 根因家族、显式
   时间窗因果投影、根因排序、唤醒链、窗内可消除量和自动补齐不进入该谓词。
2. 从 `AnswerBlock.EdgeAnchors[].relation_kind=call` 取得待验证边，从 Mermaid
   participant/node declaration 结构取得 alias→visible symbol 映射；不读取 edge
   message、summary、item prose、RawRequest 或模型 thinking。
3. 权威匹配只接受 citable typed `EvidenceItem`，且要求
   `ClaimFormOf(evidence)==call_edge` 与 `Subject == from`、`Object == to` 精确同向。
   definition、反向 call、不可引用 evidence 均不能授权该边。
4. 同一判定函数同时接入 emit-time 诊断与 post-emit V2 oracle，避免只补 prompt
   而没有发布接线；post-emit 以 `ViolCitation` 返回 finalizer-local repair，明确要求
   同时修正/删除 diagram edge 与对应 principal-list claim。
5. 回归固定：participant alias 正向通过、反向失败、pre-emit 调度接线、post-emit
   oracle 接线，以及 `QFRootCauseTrace` 负例不受影响。

验证：`go test ./internal/types ./internal/tool ./internal/orchestrator -count=1` 均通过
（types 18.116s、tool 161.167s、orchestrator 10.026s）；定义证据负例、方向正反例、
双接线与 Trace family 隔离测试均通过。

当前状态：`EVAL-B24-EDGEAUTH1=implemented / full-tests-pass / replay-next`；
`EVAL-B24-EVALDIR1=P1/open`；`EVAL-B24-KEYSET1=P2/open`。

#### B24-a r1：方向识别正确，但复用 citation 软权限导致零拒绝

同一组 read/write case 严格并行 2 个复放，runner 仍为 2 PASS；人工为
`patch_c_typo=PASS`、`qf_sequence_analyzer_gate=FAIL`：

1. 写模式补丁继续只有 `main.c` 一行 typo 修复，plan/apply/verify 稳定。
2. 调用链 case 的新 matcher 精确报出 19 条无同向 typed call-site 证据的边；日志
   `advisory_by_field.diagram_call_edge_evidence=1` 证明生产接线已生效。
3. 但 producer 复用了 `ViolCitation`；该 kind 在商业默认策略中是
   `SoftByDefault=true`，所以 `strict_findings=0 / fin_reject=0`，19 条伪边仍出厂。
   `EVAL-B24-EDGEAUTH1` 因此从 implemented 修正为 `partial`：检测已覆盖，权限未覆盖。
4. 最优修正是独立的 `diagram_call_edge_unproven`：仅消费 call-chain family、
   structured `relation_kind=call`、Mermaid alias/node 与 citable typed
   `EvidenceItem Subject->Object`；默认 High、finalizer-local、same-turn hard。它不读取
   RawRequest、模型 prose/label，也明确排除 `QFRootCauseTrace`。
5. 新登记 `EVAL-B24-DIAGKIND1/P1`：typed analyzer 把明确请求的 Mermaid 时序图
   归类为 `call_dag`，最终生成 flowchart。修复应加强 analyzer schema/prompt 中
   “显式 presentation modality 优先于主题推断”的规则，不在 Go runtime 扫描原文硬改。
6. 新登记 `EVAL-B24-ENDPOINT1/P1`：答案以 `gate.RunWith` 替代精确请求终点
   `gate.Run`，且没有先披露“当前源码不存在 buildAnalysisIR→gate.Run 路径”。需在
   硬边修复复放后审计 typed exact-target token 权限，禁止用 substring/prefix 视为命中。
7. `EVAL-B24-KEYSET1/P2` 复现：主列表 20 项、补充 1 项、系统清册 26 项；“关键
   subset”与“complete roster”仍缺 typed 枚举范围，继续保持开放，不扫描“关键”字样。

状态：`EVAL-B24-EDGEAUTH1=partial / hard-authority-fix-in-progress`；
`EVAL-B24-DIAGKIND1=P1/open`；`EVAL-B24-ENDPOINT1=P1/open`；
`EVAL-B24-EVALDIR1=P1/open`；`EVAL-B24-KEYSET1=P2/open`。

#### B24-b：typed call-edge hard authority + explicit visual modality priority

已完成本轮 corrective batch：

1. 新增独立 `diagram_call_edge_unproven`，默认 High、retry eligible、
   finalizer-only repair；不再借用全局软 `citation` 权限。
2. pre-emit policy 显式新增第三条 typed hard lane；post-emit V2 oracle 使用同一 kind，
   policy report、registry golden、CGEC producer、retry field path、hint composer、finalizer
   skill、co-occurrence singleton 均同步登记，不存在隐藏 hard row。
3. 比较两侧仍只有 closed typed carriers：`QFCallChain`、structured call edge、Mermaid
   node/participant declaration、citable `ClaimForm=call_edge` evidence 与精确
   `Subject->Object`。空 evidence、definition evidence、反向 evidence 均不能授权边。
4. Mermaid node 的第二行 file:line 仅作为显示 metadata；确定性取首行 symbol 后做精确
   比较，不使用 prefix、substring、token overlap 或自由 prose 推断。
5. `QFRootCauseTrace` 负例继续固定，因此显式时间窗 Trace 因果投影、根因排序、唤醒链、
   窗内可消除量和自动补齐均不进入 source call-edge hard gate。
6. `diagram_hint` schema 与 analyzer skill 明确：用户已通过 typed Presentation Directive
   指定 sequence/timeline/interaction 时，`sequence` 优先于 call-chain / `axis=call`
   的主题推断；这是 emit-time 软分类指导，不在 runtime 扫描 RawRequest 或模型答案。

验证：相关 `types/tool/orchestrator/analysis-hint/skill/agent` 全量回归通过；新增测试覆盖
正向、反向、definition、absent、带 file:line 显示后缀、pre/post 双接线、默认 High retry、
三条 hard policy 清册及 Trace family 隔离。

状态：`EVAL-B24-EDGEAUTH1=implemented / full-tests-pass / same-pair-replay-next`；
`EVAL-B24-DIAGKIND1=implemented-soft-guidance / replay-next`；其余 B24 gap 保持开放。

#### B24-b r1：kind 修复覆盖；hard authority 暴露三条绕行面

同一 read/write 组合严格并行 2 个复放，runner 2 PASS，人工 1 PASS / 1 FAIL：

1. `patch_c_typo` 继续通过；`qf_sequence_analyzer_gate` 的 analyzer 已明确发出
   `diagram_hint=sequence`，最终主图也使用 `sequenceDiagram`，所以
   `EVAL-B24-DIAGKIND1=covered`。
2. typed hard gate 确实触发 6 次 pre-emit reject。第一次拒绝后，模型准确自诊断：
   `normalizer.Normalize`、`compiler.Compile` 等都是 `buildAnalysisIR` 的直接 callee，
   不能画成 sibling→sibling 链。这证明方向 authority 与 retry guidance 有效。
3. 新登记 `EVAL-B24-EDGEDECOR1/P0`：模型第二次已改成正确 star，但 participant label
   使用 `buildAnalysisIR (analyzer.go:1820)` 同行 support-ref；authority 只剥离换行/
   `<br/>` 后缀，未复用现有 typed support-ref parser，因此把整串当 Subject，与
   evidence `Subject=buildAnalysisIR` 不等，造成正确边连续误拒。
4. 新登记 `EVAL-B24-EDGEANCHOR1/P0`：模型最终把 `edge_anchors=[]`，而 matcher 只遍历
   anchors，不核对 Mermaid body 中仍存在的 10 条 edge；硬门被 metadata omission
   绕过。最优方案是在 call-chain 的 typed `sequence|call_dag` 图上，要求每条 parsed
   Mermaid edge 都有同向 `relation_kind=call` anchor，再与 typed evidence 合取；不能
   通过删 metadata 放行。
5. 新登记 `EVAL-B24-ATTACH1/P0`：每次 rejected draft 的 diagram 被保存在
   `AnswerDisplayAttachment`；接受新 doc 后，filter 只删除 body 完全相同的 attachment，
   因而首个已被 hard gate 拒绝的旧伪图又以“系统保留内容”追加。最优规则：接受文档
   已有 diagram block 时，删除所有非 system-authored rejected-draft diagram attachments；
   只有结构化文档没有图时才允许恢复图附件。
6. runner 仍因 `EXPECT_CONTAINS=gate.Run` 可被 `gate.RunWith` substring 满足，且 regex
   不校验 typed kind/edge direction 而 PASS；`EVAL-B24-EVALDIR1` 再次确认。
7. 运行日志显示 `required_mechanism_anchors=0`。call-chain 同时要求列出中间函数后，
   reconciled category-enumeration flag 会提前关闭 endpoint anchors；这是
   `EVAL-B24-ENDPOINT1` 的 typed 编译侧原因之一，安排在 P0 绕行面关闭后的独立批次。

状态：`EVAL-B24-DIAGKIND1=covered`；`EVAL-B24-EDGEAUTH1=partial`；
`EVAL-B24-EDGEDECOR1=P0/open`；`EVAL-B24-EDGEANCHOR1=P0/open`；
`EVAL-B24-ATTACH1=P0/open`；`EVAL-B24-ENDPOINT1=P1/open`；
`EVAL-B24-EVALDIR1=P1/open`。

#### B24-c：关闭 call-edge authority 的三条 retry 绕行面

本批按同一 typed authority 边界完成，不读取 RawRequest、thinking、summary 或 final prose，
也未改动 `QFRootCauseTrace` 的时间窗因果投影/自动补齐路径：

1. `EVAL-B24-EDGEDECOR1`：Mermaid endpoint label 先确定性剥离 `<br/>`/换行显示行，
   再复用 `ParseAnswerSupportRefMemberLocation` 解析同一行 `symbol (file:line)`；最终仍以
   精确 symbol 与 EvidenceItem Subject/Object 比较，不使用 prefix、substring 或相似度。
2. `EVAL-B24-EDGEANCHOR1`：对 `QFCallChain` 且 typed diagram kind 为
   `sequence|call_dag` 的图，逐条解析 Mermaid body edge；每条 body edge 必须存在同向
   `relation_kind=call` anchor，随后该方向还必须存在 citable typed call-site evidence。
   因此清空 `edge_anchors` 不再能绕过 hard authority；缺 anchor 与缺 evidence 以
   `missing_call_anchor` / `call_edge_unproven` 分开诊断。
3. pre-emit 与 post-emit 均增加 omission 接线钉：即使 evidence pool 已有正确 call edge，
   只要 diagram body edge 没有 typed anchor，两个发布边界都必须拒绝。
4. `EVAL-B24-ATTACH1`：接受文档已经含非空 diagram block 时，所有非 system-authored
   rejected-draft diagram attachments 都视为 retry telemetry 并删除；接受文档没有图时
   仍保留恢复图，system-authored deterministic attachment 也保留独立权限。
5. 负例继续固定 `QFRootCauseTrace` 在 matcher 入口返回，因此不会把源码 call-chain
   规则伸入 Trace 因果投影、根因排序、唤醒链、窗内可消除量或系统补齐。

验证：`go test ./internal/tool ./internal/orchestrator` 全量通过（tool 159.658s，
orchestrator 11.937s）；新增测试覆盖同行 support-ref、sequence/call_dag 空 anchor、
pre/post 双接线、接受图删除 rejected model 图、无接受图继续保留、system-authored 保留。

状态：`EVAL-B24-EDGEDECOR1=covered`；`EVAL-B24-EDGEANCHOR1=covered`；
`EVAL-B24-ATTACH1=covered`；`EVAL-B24-EDGEAUTH1=implemented / replay-next`；
`EVAL-B24-ENDPOINT1=P1/open`；`EVAL-B24-EVALDIR1=P1/open`。

#### B24-c r1：metadata omission 已封；typed callee 双表面与降级附件仍有缺口

同一 read/write 两 case 严格并行复放，runner 2 PASS，人工 1 PASS / 1 FAIL：

1. `patch_c_typo` 93s 继续通过；read case 458s，出现 12 次 finalizer reject。
2. `EVAL-B24-EDGEANCHOR1` 已覆盖：首稿空 anchor 的 11 条边全部以
   `missing_call_anchor` 拒绝；没有再次通过 metadata omission 出厂。
3. 模型收到诊断后准确改为 `buildAnalysisIR -> each callee` 的 star，证明方向 guidance
   生效。但新登记 `EVAL-B24-CALLEEALIAS1/P0`：同一 grounded call EvidenceItem 同时携带
   `Object=normalizer.Normalize` 与 schema 明确定义的 `AnchorSymbol=Normalize`（callee
   name）。图用精确短 callee label `Normalize` 时，matcher 只比 Object，故把真实边误判
   为 `call_edge_unproven`。这不是 fuzzy alias：两种表面来自同一 grounded typed record。
4. `EVAL-B24-ATTACH1` 降为 partial：正常 accepted-doc 路径已覆盖，但本轮没有最终
   accepted doc，finalizer 改为发运 retry-state structured draft；该 draft 已含最新 star 图，
   恢复车道却未调用共享 attachment filter，旧 rejected chain 图仍以“系统保留内容”追加。
5. `EVAL-B24-ENDPOINT1` 仍开放：答案识别实际调用是 `gate.RunWith`，但未把请求的
   `gate.Run` 裁为当前源码无精确路径；runner 的 substring oracle 继续误报 PASS。

状态：`EVAL-B24-EDGEANCHOR1=covered`；`EVAL-B24-CALLEEALIAS1=P0/fix-in-progress`；
`EVAL-B24-ATTACH1=partial / degraded-lane-open`；`EVAL-B24-ENDPOINT1=P1/open`；
`EVAL-B24-EVALDIR1=P1/open`。

#### B24-d：同一 grounded call record 的 callee 双表面 + 降级附件优先级

已完成本轮 corrective batch：

1. call-edge authority 继续要求 exact Subject；callee 只允许匹配同一 citable
   `ClaimCallEdge` EvidenceItem 的 exact Object 或 exact AnchorSymbol。后者在
   `emit_evidence` schema 中明确定义为“调用行上由 grounder 验证的 callee name”；因此
   `normalizer.Normalize` / `Normalize` 是一个 typed record 的两个权威表面，不是运行时
   从字符串后缀猜 alias。
2. 新增负例保证 `Other` 不能匹配 `Object=normalizer.Run, AnchorSymbol=Run`；没有
   suffix、prefix、substring、token overlap 或模型 prose 参与 hard gate。
3. retry-state / last-rejected structured draft 成为降级发运 doc 时，在渲染前调用共享
   `FilterAcceptedAnswerDisplayAttachments`：doc 已有非空 diagram 时删除旧的非系统
   model diagram attachment，并同步清理 MutableState；system-authored attachment 权限不变。
4. `QFRootCauseTrace` 的 matcher 入口隔离未改，Trace 时间窗因果投影与自动补齐无影响。

验证：`go test ./internal/tool ./internal/orchestrator ./internal/agent` 全量通过（tool
161.575s、orchestrator 12.831s、agent 2.731s）；新增 exact AnchorSymbol 正/负例和
retry-state doc 图覆盖旧 rejected attachment 的端到端测试。

状态：`EVAL-B24-CALLEEALIAS1=covered`；`EVAL-B24-ATTACH1=implemented / replay-next`；
`EVAL-B24-EDGEAUTH1=implemented / replay-next`；`EVAL-B24-ENDPOINT1=P1/open`；
`EVAL-B24-EVALDIR1=P1/open`。

#### B24-d r1：call-edge authority 通过，精确 endpoint 合同成为唯一 P1 主阻塞

同一 read/write 两 case 在 `main@6fd46c10b` 严格并行复放，runner 仍为 2 PASS；
人工为 `patch_c_typo=PASS`、`qf_sequence_analyzer_gate=FAIL`：

1. 写模式 102s，plan/apply/verify 与单行 typo patch 继续稳定。
2. read case 从上一轮 458s / 12 rejects 降到 252s / 4 rejects。首稿伪 sibling
   sequence 被 hard authority 拒绝；最终只剩一张 `buildAnalysisIR -> each grounded
   callee` 的 star 图，`AnchorSymbol` 短表面正确授权，旧 rejected 图没有回流。
   因此 `EVAL-B24-EDGEAUTH1`、`EVAL-B24-CALLEEALIAS1`、
   `EVAL-B24-ATTACH1` 均由真实复放覆盖。
3. `EVAL-B24-ENDPOINT1/P1` 仍导致人工失败：用户精确终点是 `gate.Run`，源码主路径
   终点是 `gate.RunWith`；答案静默采用后者，没有披露“收集到的 typed call-edge
   证据未证明到 `gate.Run` 的路径”。runner 仍因 substring oracle 将
   `gate.RunWith` 误当成 `gate.Run` 而 PASS。
4. 深层原因有两层：semantic view 日志始终为 `required_mechanism_anchors=0`，精确
   source/sink 没有成为 call-chain 可见义务；即使义务存在，现有 required-anchor
   matcher 也会把 qualified required key 分解成 owner/member，导致 `gate.RunWith`
   通过共享 owner `gate` 冒充 `gate.Run`。
5. 最优通用修复冻结为：call-chain family 的 typed endpoint 优先于 relation/category
   抑制；qualified **required** anchor 只允许 exact qualified/compact-qualified 命中，
   但 present qualified surface 仍可展开 owner/member，以保留
   `StageOutput.AnalysisIR` 同时承载两个简单 anchor 的旧能力。不得扫描 RawRequest、
   summary/final prose、case ID 或用 prefix/substring/fuzzy symbol 匹配。
6. `EVAL-B24-KEYSET1/P2` 仍开放：主列表 17 项、系统清册 19 项、summary 使用 19；
   这是 key subset 与 complete roster 的 typed scope/cardinality gap，不与 endpoint P1
   混批，不通过扫描“关键”等字样解决。

状态：`EVAL-B24-EDGEAUTH1=covered`；`EVAL-B24-CALLEEALIAS1=covered`；
`EVAL-B24-ATTACH1=covered`；`EVAL-B24-ENDPOINT1=P1/fix-in-progress`；
`EVAL-B24-EVALDIR1=P1/open`；`EVAL-B24-KEYSET1=P2/open`。

#### B24-e：call-chain endpoint identity authority

本批完成 typed endpoint 的编译和匹配修复：

1. `QFCallChain` 成为 relation/category suppression 的显式例外。源码 call-chain 的
   source/sink 是 endpoint identity，不是待枚举的 answer-member；即使 analyzer 同时
   发出 `is_relational_lookup` / `is_category_enumeration`，current-request 的 typed
   `MentionedEntities/ExactTargets` 仍编译为 required mechanism anchors。scalar、count、
   config 和 runtime external-source suppression 保持原样。
2. qualified **required** anchor 只生成 exact primary 与 exact compact-qualified key；
   不再降级成 owner/member。present qualified surface 仍展开 owner/member，所以
   `StageOutput.AnalysisIR` 继续能同时承载简单 anchors `StageOutput`、`AnalysisIR`，但
   `gate.RunWith` 不能再通过 owner=`gate` 冒充 `gate.Run`。
3. table-cell 兼容面同步改为 qualified token boundary；`gate.Run (requested endpoint)`
   可以承载 exact endpoint，`gate.RunWith ...` 不可通过 substring 绕行。
4. pre-emit repair guidance 只读取 typed missing-anchor 集：若证据仅证明 sibling/nearby
   symbol 或未证明到 exact endpoint 的路径，要求保留 exact requested label 并在 item
   text 披露证据边界，不得静默替换。硬判定仍不读取 item text、RawRequest、summary、
   final prose 或 case ID。
5. `QFRootCauseTrace` 不在启用 family，显式时间窗的 Trace 因果投影、根因排序、唤醒链、
   窗内可消除量和自动补齐路径未改。

验证：`go test ./internal/types ./internal/tool ./internal/orchestrator ./internal/agent -count=1`
全部通过（types 18.901s、tool 162.403s、orchestrator 11.128s、agent 3.663s）；新增回归
固定 relation/category call-chain 正臂、qualified sibling 的 label/cell 负臂、annotated exact
cell 正臂及旧 owner/member 承载能力。

状态：`EVAL-B24-ENDPOINT1=implemented / full-tests-pass / same-pair-replay-next`；
`EVAL-B24-EVALDIR1=P1/open`；`EVAL-B24-KEYSET1=P2/open`。

#### B24-e r1：endpoint 编译/精确匹配通过，soft 发布权限仍放行

`main@ac4b9ea8b` 下同一 read/write 两 case 严格并行，runner 2 PASS；人工仍为
1 PASS / 1 FAIL：

1. 写模式 99s，单行 patch 与 plan/apply/verify 继续通过。
2. read case 207s / 2 rejects；`required_mechanism_anchors` 从 0 恢复为 3，日志的
   missing set 明确包含 `gate.Run`、`analyzer.go`，证明 call-chain relation/category
   编译例外已生效；`gate.RunWith` 没有再通过 owner/substring 冒充 `gate.Run`，证明
   qualified exact matcher 已生效。
3. 但 pre-emit 日志明确写出 `accepted as soft advisory`。根因是 call-chain endpoint
   miss 仍复用 `principal_support_member_omitted`；该 kind 面向普通枚举/机制覆盖，注册为
   Medium + soft-by-default。最终文档因此继续只发布 `gate.RunWith`，未单独裁定
   `gate.Run`，人工仍 FAIL。
4. `EVAL-B24-ENDPOINT1` 的编译/匹配部分已经覆盖，剩余 gap 收敛为 typed publication
   authority。最优修复不是全局硬化旧 kind，而是新增只适用于 `QFCallChain` 的
   `call_chain_endpoint_omitted`：输入仅为 semantic family、compiled endpoints 与
   structured label/cell/title/edge endpoint，High、finalizer-local、same-turn hard；普通
   mechanism/architecture/enumeration 保持原 soft policy，`QFRootCauseTrace` 保持隔离。

状态：`EVAL-B24-ENDPOINT1=partial / publication-authority-fix-in-progress`；
`EVAL-B24-EVALDIR1=P1/open`；`EVAL-B24-KEYSET1=P2/open`。

#### B24-f：typed call-chain endpoint publication authority

本批完成独立 violation 与双发布边界接线：

1. 新增 `call_chain_endpoint_omitted`，默认 High、retry eligible、finalizer-local，
   repair phase 为 consistency；只在 `AnswerSemanticView.Family=QFCallChain` 且
   `RequiredMechanismAnchors` 存在缺项时产生。
2. pre-emit 新增第四条明确 hard lane `typed_call_chain_endpoints`；普通
   `required_mechanism_anchors` 继续使用旧的 soft
   `principal_support_member_omitted`。同一 helper 只负责 exact structured carrier
   比较，不读取答案 text/summary/final prose。
3. post-emit `validateRequiredMechanismAnchorsRendered` 对 call-chain 返回新 kind，
   其它 family 返回旧 kind；cluster/IR field 固定为
   `root:call_chain_endpoints / answer_contract.call_chain_endpoints`，confidence=1.0。
4. registry、policy report、CGEC producer/closure、retry field path、repair plan、hint
   composer、legacy golden 与 same-turn hard-policy ratchet 全部接线；不存在“检测到了但
   仍按 soft 出厂”的第二条绕行面。
5. guidance 只根据 typed missing endpoint 说明：保留 exact requested label；若 evidence
   仅证明 sibling/nearby target，则披露未证明该 exact path，不得静默替换。
6. `QFRootCauseTrace` 不产生该 kind，显式时间窗 Trace 因果投影、根因排序、唤醒链、
   窗内可消除量和自动补齐不受影响。

验证：`go test ./internal/types ./internal/tool ./internal/orchestrator ./internal/agent
./internal/analysis/hint ./internal/skill -count=1` 全部通过（types 18.382s、tool
160.997s、orchestrator 12.308s、agent 3.238s、hint 0.377s、skill 1.785s）；定向测试
固定 pre-emit hard split、post-emit kind/root metadata、exact endpoint 正臂与四条 hard
policy 清册。

状态：`EVAL-B24-ENDPOINT1=implemented / full-tests-pass / same-pair-replay-next`；
`EVAL-B24-EVALDIR1=P1/open`；`EVAL-B24-KEYSET1=P2/open`。

#### B24-f r1：endpoint 正确性覆盖，跨轴 pruning 形成 hard retry loop

`main@84e31677d` 下同一 read/write 两 case 严格并行，runner 与人工均 2 PASS：

1. 写模式 116s，单行 patch 与 plan/apply/verify 继续稳定。
2. read case 最终明确保留 `gate.Run` 结构化行，并说明源码实际调用
   `gate.RunWith`；图中仍只有 exact typed call evidence 支持的 star edges，旧 rejected
   diagram 没有回流。因此 `EVAL-B24-ENDPOINT1` 的用户可见正确性已覆盖。
3. 但 runtime 从 207s / 2 rejects 恶化到 472s / 10 rejects。日志显示模型多轮已经在
   `hops` 中加入 exact `buildAnalysisIR` / `gate.Run` / `analyzer.go` labels，下一轮却又
   报缺；最终改成独立 table 后才通过。根因是 principal enumeration 规范化/裁剪把
   endpoint identity 行按“中间函数完整 member-set”轴判成 extraneous 后删除，随后
   endpoint hard gate 又要求补回。
4. 新登记 `EVAL-B24-ENDPPRUNE1/P1`：typed endpoint 与 principal member-set 是两个
   正交身份轴。最优修复是在 pruning 前编译 call-chain required endpoint set，并保护
   exact endpoint structured rows；不能放软 endpoint hard gate，也不能把全部模型行免裁剪。
5. 新登记 `EVAL-B24-ENDPOINTSCOPE1/P1`：`MentionedEntities` 的源码文件上下文
   `analyzer.go` 也被提升为 endpoint。call-chain endpoint 编译应排除 typed
   code/config path surface，但保留 qualified code symbol（例如 `gate.Run`）；不能按
   “含点号”粗判，因为 qualified symbol 也含点号。
6. 当前 system required-anchor materializer 因 `RequiresAnchorSkeleton=false` 不服务
   call-chain，导致可由 typed system block 安全完成的 endpoint disclosure 全靠模型重试。
   B24-g 将允许 call-chain 专用 materialization：exact label；无 exact evidence 时明确写
   “本轮 typed evidence 未解析该 endpoint，nearby symbol 不替代”；为避免 citation
   扩权，系统补位 endpoint 不借用 sibling citation。
7. `EVAL-B24-KEYSET1/P2` 加剧：模型主列表 19、系统完整清册 32、补充 2、endpoint
   表 3。正确性已恢复但宽度失控；该项继续独立等待 typed key-subset/complete-roster
   scope，不扫描“关键”或模型 prose。

状态：`EVAL-B24-ENDPOINT1=covered`；`EVAL-B24-ENDPPRUNE1=P1/fix-in-progress`；
`EVAL-B24-ENDPOINTSCOPE1=P1/fix-in-progress`；`EVAL-B24-EVALDIR1=P1/open`；
`EVAL-B24-KEYSET1=P2/open`。

#### B24-g：调用链 endpoint 与成员集双轴解耦（2026-08-01）

本批按 B24-f r1 的 production witness 完成泛化修复：

1. `CompileRequiredMechanismAnchors` 在 `QFCallChain` typed family 内排除
   `HasCodeOrConfigPathSuffix` 命中的源码/配置路径上下文，因此 `analyzer.go`、
   `internal/agent/analyzer.go` 不再升级为 source/sink；`gate.Run`、
   `StageOutput.AnalysisIR` 等 qualified symbol 原样保留。未读取 raw request 或答案 prose。
2. call-chain 现在可以由 system required-anchor materializer 建立 exact structured endpoint
   行。系统行不借用 sibling citation；当 evidence 只有 `gate.RunWith` 而请求端点是
   `gate.Run` 时，typed disclosure 明确报告“未精确解析、相邻/同名前缀不可替代”，不会
   猜造调用边。
3. principal enumeration pruning 在非 strict-source-inventory lane 内读取同一 typed
   `RequiredMechanismAnchors`，只保护 exact structured label/cell 命中的 endpoint。
   item text/prose 不参与判断；普通 extraneous row 继续删除，strict source inventory 的
   排他语义不变。
4. `call_chain_endpoint_omitted` hard gate 继续保留为最终 backstop；materializer 与 pruning
   共享 `MissingRequiredMechanismAnchors` 的 exact qualified-symbol identity，不再出现
   “补入→裁掉→再 hard reject”的跨轴循环。
5. 负向边界已钉：`QFRootCauseTrace` 不获得 endpoint pruning 豁免；因此带明确时间窗的
   Trace 因果投影、根因排序、唤醒链和系统自动补齐路径未被改写。

验证：定向 endpoint/path/pruning 测试全绿；完整
`go test ./internal/types ./internal/tool ./internal/orchestrator ./internal/agent ./internal/analysis/hint ./internal/skill -count=1`
全绿（types 19.311s、tool 161.421s、orchestrator 10.668s、agent 3.901s、hint 1.041s、
skill 2.395s）。下一步在提交推送后复放同一 read/write 两 case，确认 finalizer rejects 与
时长真实收敛，再决定是否关闭两项 P1。

状态：`EVAL-B24-ENDPPRUNE1=P1/implemented/replay-next`；
`EVAL-B24-ENDPOINTSCOPE1=P1/implemented/replay-next`；
`EVAL-B24-ENDPOINT1=covered`；`EVAL-B24-KEYSET1=P2/open`。

#### B24-g r1：双轴修复生效，endpoint presence 仍不等于 reachability

`main@9745226b3` 同一 read/write 两 case 严格并行：runner 2 PASS，人工 1 PASS / 1 FAIL。

1. write apply 129s/0 reject，单行变更与 plan/apply/verify 不回归。
2. call-chain 从 472s/10 rejects 降至 293s/8 rejects；required endpoint 数从 3 收敛为
   2，`analyzer.go` 不再进入 endpoint set，日志也不再出现 endpoint 被 pruning 删除后重复
   hard reject。因此 `ENDPPRUNE1`、`ENDPOINTSCOPE1` 均由真实 replay 覆盖。
3. 剩余 8 rejects 全部来自 diagram call-edge authority：validator 逐轮删除
   `RunWith → Run` 反向边、把 check 函数画成 self-loop 的无证边，最终图只保留
   `buildAnalysisIR → .../gate.RunWith` 的 accepted typed edges。这个约束工作正常。
4. 人工仍判 FAIL：最终 summary 写“通过 `gate.RunWith` 调用 `gate.Run`”，principal path
   list 也写“完整调用顺序”，但已校验图没有 source→`gate.Run` 路径；源码实际 wrapper
   方向是 `gate.Run → gate.RunWith`。endpoint identity 行在场只证明两个符号都被说明，
   不证明两者有向可达。
5. 新登记 `EVAL-B24-REACH1/P1`：对 `QFCallChain` 且恰有两个 typed required endpoint
   的请求，从 accepted citable `ClaimCallEdge` 编译 directed reachability。若 source→sink
   未证明，由 deterministic system authority 将 summary 与 principal-path carrier 收敛成
   `unproven`，保留 verified diagram 边；不扫描 raw request、model prose 或 rendered text，
   不把 definition/sibling/prefix 当成 edge。
6. `EVAL-B24-EVALDIR1/P1` 继续开放：diagram 初稿仍从 model 生成大量错误边，正确性 gate
   最终兜住但代价 8 rejects。先完成 REACH1 后再决定是否提供 typed diagram seed/patch，避免
   把正确性与效率混成一个 case 特补。

状态：`EVAL-B24-ENDPPRUNE1=covered`；`EVAL-B24-ENDPOINTSCOPE1=covered`；
`EVAL-B24-REACH1=P1/fix-in-progress`；`EVAL-B24-EVALDIR1=P1/open`；
`EVAL-B24-KEYSET1=P2/open`。

#### B24-h：typed call-chain reachability authority（2026-08-01）

`EVAL-B24-REACH1` 已按“精确信号用于收敛、无原文关键词门”完成：

1. 激活条件仅为 `AnswerSemanticView.Family == QFCallChain` 且
   `RequiredMechanismAnchors` 恰有两个；超过两个端点时，当前 typed 模型没有明确
   source/sink role，保持不动作而不是猜 first/last。
2. 图只消费 accepted citable evidence 中 `ClaimFormOf(ev)==ClaimCallEdge` 的
   `Subject → Object/AnchorSymbol`。`Object` 与 `AnchorSymbol` 仅因同一 grounded call-site
   记录而作为目标别名；不做 prefix、substring、owner 或 prose 推断。
3. 对 exact source→sink 做 directed BFS。endpoint definition 在场、同名前缀 sibling、
   `source → X ← target` 汇聚形都不能证明可达；真实 multi-hop typed path 才得到 proven。
4. proven 时模型答案字节不动；unproven 时 deterministic normalizer 覆盖首个 summary 与
   `principal_path_edge` 载体，明确“端点在场不等于有向可达”，并保留两个 exact endpoint
   structured labels，确保 endpoint hard contract 仍闭合。已验证 diagram 与辅助完整清册不删。
5. 这不是新 hard retry gate：收敛在 pre-emit normalizer 同轮完成。production wiring 测试
   直接调用完整 `normalizeAnswerDocumentForPreEmit`，防止仅 helper 绿而接线被删。
6. `QFRootCauseTrace` 负向 fixture 明确保持原 summary/path；带时间窗 Trace 因果投影、
   根因排序、唤醒链、窗内可消除量与自动补齐不进入源码 call-chain reachability authority。

验证：定向 directed/unproven/definition/sibling/production-wiring/Trace-negative 全绿；完整
`go test ./internal/types ./internal/tool ./internal/orchestrator ./internal/agent ./internal/analysis/hint ./internal/skill -count=1`
全绿（types 19.280s、tool 161.079s、orchestrator 11.305s、agent 4.716s、hint 3.280s、
skill 2.111s）。下一步提交推送后复放同一 read/write 两 case；若人工正确性闭合，再转入
更高优先级的显式时间窗 Trace + 另一模式双 case，而不是继续对本 case 做措辞拟合。

状态：`EVAL-B24-REACH1=P1/implemented/replay-next`；
`EVAL-B24-EVALDIR1=P1/open`；`EVAL-B24-KEYSET1=P2/open`。

#### B24-h r1：reachability 正确性与重试成本同时闭环

`main@8cfef5158` 同一 read/write 两 case 严格并行，runner 与人工均 2 PASS：

1. write apply 114s/0 reject，applied tree 仍只有 `main.c` 的单行 typo 修复。
2. call-chain 158s/2 rejects；相比 B24-f 的 472s/10、B24-g 的 293s/8，重试和时长
   连续收敛。剩余一次答案 patch 只修正 diagram source 简称
   `ParseOutput`→`analyzerEvaluator.ParseOutput`，没有 endpoint/pruning/reachability 循环。
3. 最终首段与 principal path 均明确：accepted typed call edges 未证明
   `buildAnalysisIR → gate.Run` 有向路径；两个 exact endpoints 都在场但不等于可达。
   diagram 只展示 `buildAnalysisIR → gate.RunWith` 等逐边已证关系，不再反向拼接。
4. `required_mechanism_anchors` 仅补一个未解析的 `gate.Run` 边界行，未借用
   `gate.RunWith` citation；与 reachability verdict 一致。多份 enumeration supplement 仍偏宽，
   但没有改变首要结论。
5. 因此 `EVAL-B24-REACH1` 关闭；`EVAL-B24-EVALDIR1` 从 P1 降为 P2/observe，
   `EVAL-B24-KEYSET1` 继续 P2/open。遵守 campaign 原则，不再对本 case 做函数名/措辞拟合；
   下一批切到显式时间窗 Trace + 另一模式，检查保护边界和更高 ROI gap。

状态：`EVAL-B24-REACH1=covered`；`EVAL-B24-ENDPPRUNE1=covered`；
`EVAL-B24-ENDPOINTSCOPE1=covered`；`EVAL-B24-EVALDIR1=P2/observe`；
`EVAL-B24-KEYSET1=P2/open`。

### B25 r1：plan-only 隔离通过，单附件物化别名被误判为双 trace（2026-08-01）

`main@1634ee1e6` 严格并行执行一个显式时间窗 Trace read case 与一个 Java
plan-only write case：runner 均 PASS，人工 1 PASS / 1 FAIL。

1. `patch_java_typo` 54s：ChangePlan 只包含 `Main.java` 的
   `retrun`→`return` 单行 patch；repo 中 `Main.java` 无 diff，未进入 apply/verify。
   plan-only 隔离正证通过。
2. `trace_query_donghu_real_frame_multicausal` 194s：显式用户窗
   `34579.472865..34579.587805`、主要时间占用、窗内可消除量、根因排序、唤醒链、
   代表性时间窗、Trace 因果投影和系统自动补采全部保留；真实占用与规则可消除量仍是
   两个独立维度。`frame_causality=unproven / frame_evidence_status=absent` 也正确出厂。
3. 但最终答案发布了两套完整板：一套标为 `donghu_tieba_frame.systrace`，另一套标为
   `attached_trace.txt`；主要占用、因果投影、可消除量、代表窗和关键指标各出现两次，
   Evidence Index 中同一事实也以相差一行的 locator 成对出现。系统还生成
   `attached_trace.txt ↔ donghu_tieba_frame.systrace` 的伪跨工件关系边界。
4. 日志证明两者是一个物理输入：用户/模型查询原始路径；system supplement 通过
   `resolveTraceQuerySource` 查询 Codrax 为同一 `AttachedHitrace` 写出的保留 blob。
   path attachment 固定多一行 `# codrax-source: <path>` 包装头，因此证据行号整体 +1。
   当前 partitioner 只合并同路径的相对/绝对拼写，不知道“原附件路径 ↔ 保留 blob”这条
   typed 物化关系，于是把两组确定性查询记录分成两个 capture。

登记 `EVAL-B25-TRACEALIAS1/P1`。最优通用方案不是按 basename、内容相似度或答案文字
去重，而是在 Observation Ledger 的唯一运行时工件身份入口增加 typed materialization
alias：仅当记录携带 `ArtifactID=attached_trace`、路径命中 Codrax 保留 trace blob 形状，
且 run-entry preflight 恰有一个 `carrier=attachment` 的 trace 源时，把 blob 的
`SourceRef.Path/ArtifactID` 归一到该附件源。随后既有 projection duplicate-publication
规则在一个 partition 内消除重复发布，cross-artifact authority 也不会再铸假 pair。

安全边界：

- 两个真实 trace 路径永不合并；同名不同目录也不合并；
- 用户自己的 `attached_trace.txt` 若没有 `ArtifactID=attached_trace` 结构标记，不合并；
- 多个 attachment 候选或无法唯一绑定时 fail-open，保留分区；
- 不读取 RawRequest、模型 thinking/summary/final，不修改窗口、target、root rank、
  wakeup、可消除量、frame authority 或 supplement 选取逻辑。

状态：`EVAL-B25-TRACEALIAS1=P1/filed / implementation-next`；plan-only mode=`covered`。

#### B25-A：运行时 capture 身份与可寻址载体解耦

`EVAL-B25-TRACEALIAS1` 已在 Observation Ledger 的单一身份铸造口完成：

1. `ObservationSourceRef` 新增 system-minted `capture_identity_path`。`Path` 继续表示
   producer 实际读取的载体及其行坐标；capture identity 只用于物理工件分组。因而原文件
   第 N 行与含一行 `codrax-source` 包装头的 blob 第 N+1 行都保持可核验，不做危险的
   locator 改写。
2. preflight source index 只有在以下 typed 合取成立时才给保留 blob 绑定 capture：
   `ArtifactID=attached_trace`、`ArtifactKind=trace`、路径是 Codrax 保留 trace blob、
   且恰有一个可寻址的 `carrier=attachment` trace 源。request-path 先登记、attachment
   后登记的生产顺序也可建立绑定。
3. projection partition、跨工件 relation authority、IPC census、value occurrence 与
   blocking wall-clock authority 共用 `RuntimeArtifactCaptureIdentityPath`；引用、raw read、
   payload 仍使用各自 `Path`，身份和坐标不再互相污染。
4. 正臂固定原路径查询 + system supplement blob 形成一份 projection，重叠的确定性事实
   继续由既有 duplicate-publication 规则折叠，不生成 comparison 或 cross-artifact
   boundary。反臂固定普通用户同名文件、多 attachment、inline source 均 fail-open；
   两个真实路径的既有多工件测试保持。
5. 生产成文回归使用显式 `34579.472865..34579.587805` scope，确认单 capture 的
   Trace 因果投影仍发布，只有重复的第二套板和伪 relation 被删除。

完整相关回归通过：`go test ./internal/agent ./internal/orchestrator ./internal/types
./internal/tool ./internal/skill -count=1`（agent 4.074s、orchestrator 12.350s、
types 18.775s、tool 163.421s、skill 1.379s），`git diff --check` 通过。

状态：`EVAL-B25-TRACEALIAS1=P1/implemented / same-pair-replay-next`。

#### B25-A r1：同对回放关闭 capture 别名 gap

`main@815f2071d` 使用同一构建产物严格并行回放原 Trace 与 plan-only 两 case，runner
与人工均 2 PASS：

1. Trace 最终答案只剩一份主要时间占用、一份窗内可消除量、一份代表性时间窗、一份
   关键指标和一份 Trace 因果投影；`attached_trace.txt ↔ 原附件` 的伪跨工件关系消失。
2. 用户显式窗 `34579.472865..34579.587805`、根因排序、唤醒链、系统自动补采、
   `frame_causality=unproven / frame_evidence_status=absent` 全部保留。真实占用方向与
   规则可消除方向继续独立发布，答案共 928 行，没有通过删能力或压证据掩盖重复。
3. plan-only 的 ChangePlan 仍只含 `Main.java` 中 `retrun`→`return` 的单行 patch；
   `Main.java` 无 diff，未进入 apply/verify。
4. 因此 `EVAL-B25-TRACEALIAS1` 关闭。typed capture identity 已证明既能合并一个物理
   capture 的两个载体，也不改变载体坐标和显式时间窗 Trace 合同。

同批登记两个不阻塞关闭的通用残余：

- `EVAL-B25-LOCATOR1/P2`：system supplement 成为最终选中记录时，Evidence Index 会
  如实显示私有物化载体 `attached_trace.txt`，但客户难以持久定位。不能直接改名，因为
  blob 含一行包装头，行号与原附件相差一行。后续最优方案是让系统铸造 typed locator
  transform（capture source、carrier path、精确 header offset），仅在变换被证明时渲染
  原附件坐标，否则同时显示 capture source 与实际 carrier；禁止按文件名或行号启发式换算。
- `EVAL-B25-QUOTE1/P2`：analyzer 连续两次给
  `runtime_question_profile.source_quote` 生成非连续原文，第三次才用 `卡顿原因` 通过，
  额外消耗约 31 秒。问题属于 exact-source-span 的结构化修复反馈不足，不是 Trace
  推理波动。通用解应返回字段级 typed repair metadata 或由 analysis 输入携带已验证的
  source span；不得按“卡顿”等关键词解释用户原文，也不得把它扩成答案发布硬门。

状态：`EVAL-B25-TRACEALIAS1=covered`；`EVAL-B25-LOCATOR1=P2/open`；
`EVAL-B25-QUOTE1=P2/open`；plan-only mode=`covered`。

### B26 r1：span 成员形正确，但 requested-dimension 主结论与 typed 真值冲突（2026-08-01）

本批按“客户风险、泛化覆盖、当前回放缺口、跨模式杠杆”选择一个从未在本战役回放的
真实 Trace span-family case 与一个仅有一次旧回放的 C++ write/apply case；
`main@bf2e5653c` 严格 `parallel=2`。runner 2/2 PASS，人工 1 PASS / 1 FAIL。

1. `github_issue_nlohmann_long_double_symptom` 人工 PASS：applied tree 只把普通头和
   single-include 头中的 `%.*lg` 各改为 `%.*Lf`；`make check` 以
   `-Wall -Wextra -Wformat -Werror` 编译并运行通过。只有一个修改批、一次 apply、
   一次 verify 和 verified finish。planner 在 typed read budget 已耗尽后仍尝试两个
   不可用读工具，属 P2 过程效率样本，不影响本轮正确性，不为 C++/格式串单例加硬门。
2. `real_trace_h10_spantop_member_subrows` 的系统投影正确发布
   `Jit thread pool-17284 / JIT编译 2次`，合计 `2.388ms`；两个 constituent 分别为
   `1.781ms（行5969..6114）` 与 `0.607ms（行12611..12664）`。SPANTOP 子行形、
   成员守恒、显式窗 `13762.791708..13763.024898`、主要占用/新修向、现规则可消除、
   根因排序、唤醒关系和系统补采均在。
3. 但模型拥有的首块连续断言“所有搜索零匹配”“窗口内不存在 JIT/类校验等 span”。
   同一答案后部的 deterministic semantic-class 投影直接反证它。模型探索只用目标线程
   查询 JIT，而系统最终 projection 在同一请求窗发现链路/邻近线程的 JIT family；当前
   `materialized runtime trace deterministic optimization points` 只是模型块之后的加法
   补表，没有接管用户明确要求的枚举主结论。
4. 自动 oracle 假 PASS：它只在全答找到 JIT/2.388/成员值，没有约束 principal 结论与
   typed semantic rowset 一致。该问题不是某个 JIT 名或时间值的拟合点，而是所有
   deterministic runtime fact family 的“后置真值不能纠正前置模型结论”共同缺口。

登记 `EVAL-B26-SEMCONCL1/P1`。最优方案：从最终 projection 的 typed
`semantic_class` rows 编译一个 requested-scope semantic-span authority，保持原始窗、
宿主、类别、完整 member roster、逐项 duration/locator 与 completeness；在成文前给
finalizer 一个紧凑、不可被 Top-N 挤出的 handoff，并由同一 authority 生成自然语言的
用户结论槽。结构化请求确属 runtime bounded enumeration/fact-set 且该 authority 可用时，
模型拥有的同维度 `member_set` 不得作为主值；其他等待/根因内容仍保留。硬决定只读
analysis enum、validated scope、typed projection rows 与 completeness，不读取
RawRequest、keywords、sub-topic/free-form label、模型 thinking/summary/final。不得改动
trace query、root rank、wakeup、可消除量、双轴或自动补采。

另登记 `EVAL-B26-ORACLE1/P2`：增加 principal/system relation oracle，使“主块说 0、
typed 完整 rowset 说 N>0”不能再由后置 token 假 PASS；它是 eval 关系约束，不是生产
答案词面硬门。

状态：`EVAL-B26-SEMCONCL1=P1/filed / implementation-next`；
`EVAL-B26-ORACLE1=P2/filed`；C++ write/apply=`covered`。

### B21-GREP：literal/regex 查询语义进入 typed 证据链（2026-08-01）

`EVAL-B21-GREP1` 已按软恢复而非硬拒绝施工：

1. `ToolPathDiscovery` 新增 `match_mode` 与
   `literal_regex_syntax_hint`。它们直接来自 grep schema 参数和工具侧语法检查，
   不解析用户原始输入、模型 thinking/summary/final，也不把噪声分数用于硬门。
2. `fixed_string=true` 且零命中时，若 pattern 含未转义的 `|` 或已有正则语法标记，
   返回 typed refinement
   `grep_fixed_string_regex_syntax_zero_match`，建议下一步仍用 grep，但显式切换
   `fixed_string=false`。runtime artifact/trace query 的专用恢复仍保持更高优先级。
3. 该调用仍可合法搜索带 `|` 的精确字面量，不会被拒绝；工具只披露“本次零命中仅
   证明完整字面量不存在，不能证明各 regex alternative 均不存在”。因此这是查询语义
   澄清，不是 absence 事实扩张。
4. typed match mode 与 ambiguity hint 进入 Observation Ledger 的 summary/notes，
   后续 agent 即使只消费结构化证据，也不会把 literal zero-match 当成 regex alternatives
   的排除证明。
5. 定向回归固定 B21 的三符号 alternation 形、typed recovery 参数、ledger 投影；
   `go test ./internal/types ./internal/tool -count=1` 全量通过
   （types 20.991s，tool 159.954s），`git diff --check` 通过。

不变量：未修改 Trace query/family、显式时间窗报告、根因排序、唤醒链、窗内可消除量、
因果投影或系统自动补采；未增加任何答案原文关键词硬门。

状态：`EVAL-B21-GREP1=implemented/full-tests-pass/replay-next`。下一步重建当前二进制，
严格并行 2 个 B21 读模式用例，联合回放 B21-C/S/GREP；然后转入显式 Trace 窗与
write/plan 等异构模式。

### B21 r5：runner 双 PASS，人工暴露 citation 优先级与证据跨度缺口（2026-08-01）

严格并行 2 个读模式 case（同一 `main@df8cdbd0b` 二进制快照）：

- diff-hunk/current-code：runner PASS，177s；
- latest-merge/current-source：runner PASS，212s；
- 人工审计均 FAIL。runner 的 answer regex 未检查引用身份、阶段顺序或 evidence span，
  因此不能替代人工判定。

已获得的正证：

1. 两次 typed history selection 均选中有序结果第 1 项：latest commit=`df8cdbd0b`，
   latest merge=`2a58a60d7`；`EVAL-B21-ORD1` 继续 covered。
2. 历史 diff 与 current checkout 分席有效。没有把历史行号当作当前行号，也没有复发
   `explicitRuntimeArtifactLog` 被投射到邻近 helper 的旧错位；
   `EVAL-B21-TRANS1/MAP1` 的主体边界 covered。
3. 两份答案均未出现旧的“系统按已验证证据补充缺失成员”或同义内部协议补表；
   `EVAL-B21-SUP1=covered`。
4. `EVAL-B21-GREP1` 代码和 typed 单测已覆盖；本轮模型使用默认 regex grep，未走
   literal-zero-match live arm，因此状态为 `implemented/test-covered/live-arm-not-exercised`，
   不伪称真实复放正证。

新登记：

| ID | 优先级 | 系统 GAP | 代码/日志真相 | 泛化方案 |
|---|---:|---|---|---|
| `EVAL-B21-CIT2` | P1 | 精确 visible source location 的正确 citation 可被后续低权限 code-surface heuristic 覆盖 | model 提交 Hunk 1 `citation_ref=0 -> builtin.go:2097`；`normalizeVisibleSourceLocationCarriers` 后，`normalizeItemCitationRefsByUniqueBacktickCitationQuote` 又改 1 项，最终 prune `:2097` 并把条目显示为 `:2142` | 冻结证据优先级：item 中唯一、可解析且与 current citation 对齐的 exact `path:line` 是更精确信号；后续 backtick/label 候选只能修无引用、越界或与 exact location 不一致的引用，不能覆盖正确精确绑定。不得扫描 RawRequest/答案整体语义，只消费结构化 item 与 citation |
| `EVAL-B21-CALLEE1` | P1 | call-site 证据被允许承载被调函数内部行为与跨阶段时序 | latest-merge 答案只引用 `parallelExploreMustWaitForSiblingHandoffs` 调用点，却声称其保证“证据在 emit_analysis 前完成”；真实 pipeline 先 analyze/emit_analysis 后 explore，该说法顺序相反 | 为 mechanism/call-chain 证据建立 typed entailment：call-site 只授权“调用发生”；callee guard/return/时序必须有 callee definition/body 或 typed topology edge。软引导补读，答案侧按 claim form 限权，不做 prose 关键词门 |
| `EVAL-B21-SPAN1` | P1 | 单行 grounded anchor 可携带跨函数行为摘要，summary 文本没有 evidence span 权限 | `pathDiscoveryObservationSummary:2099` 的证据被扩写为 summary 和 `pathDiscoveryObservationNotes` 均投影；最终甚至写成 Summary 函数“在 notes 中追加” | 将 evidence behavior claim 与 typed source span/endpoint 合取；跨函数说明要求分别 grounded 的 evidence item，不能用一个 anchor 的自由 summary 扩权。优先改证据铸造/claim binding，不扫描最终 prose |

施工顺序：先修完全确定且低耦合的 `CIT2`；`CALLEE1/SPAN1` 合并设计成统一的
claim-entailment 批，避免分别对“emit_analysis”或某两个函数做 case 拟合。两批都不修改
Trace 查询、显式窗因果投影、根因排序、唤醒链、窗内可消除量或自动补采。

#### B21-CIT2：精确 source location 高于 code-token 软修复

已在统一 pre-emit citation normalization 链修复：

1. ordered/table item 若只有一个可解析的显式 source location，且当前
   `citation_ref` 已与该 `path:line[/range]` 对齐，则冻结该精确绑定；
2. 后续 `normalizeItemCitationRefsByUniqueBacktickCitationQuote` 不得再用另一个位置的
   反引号 token 命中覆盖它。显式位置是单一坐标，code token 可能在同函数的条件、赋值、
   分支中重复，前者权限更高；
3. item 没有显式位置、位置不唯一、当前引用越界或与显式位置不一致时，原 code-surface
   软修复继续生效；既有 unique-repair 与 ambiguous-fail-open 回归保持；
4. 判定只消费结构化 item fields、parsed source-location surface 与 citation pool，
   不读取 RawRequest、case ID、模型 thinking/summary/final，也不按 Go/Hunk/具体符号拟合。

验证：定向回归固定“正确 `:2097` 与附近唯一 token `:2142` 竞争”时必须保留 `:2097`，
同时保留无显式位置的旧修复臂；`go test ./internal/tool -count=1` 全量通过（157.439s），
`git diff --check` 通过。

状态：`EVAL-B21-CIT2=implemented/full-tests-pass/replay-later`。下一批进入共享
claim-entailment 设计审计；若安全施工范围过大，则先保持 filed 并按既定顺序转入
显式 Trace 窗 + write/plan 的严格并行 2-case 异构 eval。

#### B21-ENT1：anchor/span entailment 共享软边界

代码审计结论：现有系统已经有 typed `ClaimFormOf(EvidenceItem)`、`anchor_kind`、
`line_start/line_end`、block-level `claim_uses` 与部分 citation-role gate；但 item 级没有
稳定的 claim→evidence binding，section/summary 也没有逐句 citation carrier。当前若直接加
硬拒绝，只能依赖答案 prose 反推“行为/阶段”含义，违反 noisy-signal 不做硬门的红线，且会
误伤合法机制说明。因此本批先落高覆盖软边界，不伪称 hard authority 已完结：

1. `emit_evidence` 在 investigator 写入点明确 typed entailment：call-site 只证明
   caller→callee，不证明 callee 内部 guard/return/side effect/stage ordering；definition 只证明
   declaration/signature；condition/return/assignment 只证明各自 grounded span；
2. 跨函数、跨 source span 的行为必须先读对应源码并拆成独立 evidence item，禁止一个
   line anchor 的自由 summary 合并 sibling functions；
3. finalizer 全局 skill 使用同一边界，要求跨函数/阶段解释为每一 hop 分别引用；动态
   ordered-hop checklist 同步强调 call-site 权限上限；
4. 全部是 typed workflow guidance，不扫描用户原文或最终答案，不拒绝结构合法答案，
   不认识 `emit_analysis`、本轮函数名、case ID 或语言/框架。

定向测试固定 investigator 与 finalizer 两个 producer 都携带边界文案。该批是
`EVAL-B21-CALLEE1/SPAN1=soft-mitigation`；真正 complete 需要未来通用 item/section
claim→evidence 载体能表达逐项/逐句权限后，再让精确 enum/id 对齐承担硬校验，不能以
prose matcher 代替。

全相关回归通过：agent 3.213s、skill 0.790s、tool 160.398s；`git diff --check` 通过。

不变量：该指导不触及 Trace evidence 的 typed causal authority、显式窗口、根因/唤醒/
可消除计算或系统自动补采；Trace 外部观察继续由 `external_observation` 独立承载。

### B22 r1：显式 Trace 窗 + write/plan 异构回放（2026-08-01）

同一 `main@a4cff1c42` 二进制快照下严格并行 2 个不同模式 case：

- `trace_query_frame_semantic_span_optimization`：runner PASS，148s，人工 FAIL；
- `patch_cpp_typo`（`MODE=plan`）：runner PASS，78s，人工 PASS。

write/plan 正证：系统只生成 `main.cpp` 单文件单行 `kind=patch`，没有修改主仓；
write analyzer 的未绑定 hard expected 和 planner 的 tab/space `old_text` 各经一次 typed
拒绝/repair pack 后收敛，最终 patch 可 apply、验收项与 fixture 一致。这里的受控重试没有
扩大变更范围，也没有形成新的高 ROI gap。

Trace 正证：

1. 用户显式 `5.000000..5.007000` 窗保持最高 scope 权限；报告完整保留主要时间占用、
   可消除候选、根因排序、唤醒链、代表窗、完整投影明细与系统自动补采。
2. typed `frame_causality=unproven / frame_evidence_status=absent` 已结构化接管首结论，
   model 自写的 definite-cause blocks 没有进入最终文档；系统正文统一使用“候选/优先验证、
   不等于帧因果”的词面。`EVAL-B19-CAUSAL1` 因而获得真实显式窗正证，状态更新为
   `covered`，且没有牺牲 Trace 因果投影能力。
3. 两个时间维度的引擎值本身正确：`VerifyClass` 完整窗内 span 为 5.000ms；它与目标
   等待链窗 `5.000000..5.005000` 的精确交集为 4.600ms，后者才是 typed
   `effective_impact_ms` 和可消除候选排序值。worker 的链窗 running 4.000ms 与完整
   scheduler running 4.600ms 也是同样的“投影/实际”双口径，不应相加。

新登记：

| ID | 优先级 | 系统 GAP | 代码/日志真相 | 泛化方案 |
|---|---:|---|---|---|
| `EVAL-B22-SEMAXIS1` | P1 | “确定性优化点”表把语义 span 的原始窗内墙钟回填为“有效成本”，把真实占用与规则可消除量混成一个轴 | 因果投影树/明细正确发布 `窗口投影=5.000ms / 有效归因=4.600ms`；但 `runtimeTraceSemanticOptimizationParts` 只读 `EffectiveImpactMS`，单 span 的有效交集实际在 typed `SemanticChainProjectedMS`，该槽为空时函数回退 `runtimeTraceProjNodeDisplayImpact=5.000ms`，最终表写成“有效成本 5.000ms / 71.4%” | 将优化表拆成两个 typed 列：`窗内 span 墙钟` 只读 observation union；`规则可消除` 只读已发布 effective 或 typed chain intersection，缺失时为 `—`，禁止拿 raw span 回填；百分比明确绑定可消除值。复用已有 `runtimeTraceProjSemanticChainIntersectionMS`/family helper，不重算区间、不扫描答案 prose |

本项是跨所有 semantic class、单 span/同类 family、on-chain/off-chain 的共同消费面，
不是对 VerifyClass 或 5.000/4.600 的特例修复。施工只改确定性展示值路由与中英文列名，
不改 trace_query 的区间计算、根因排序、唤醒链、显式窗选举、可消除值生产或自动补采。

#### B22-A：semantic span 原始占时 / 规则可消除双轴

已按 typed 值权限完成：

1. 确定性优化表扩为 `窗内 span 墙钟 / 规则可消除 / 可消占窗%` 三个值面；前者只读
   semantic observation 的窗内 union，后两者只读 `EffectiveImpactMS`（含 published-zero）
   或 `SemanticChainProjectedMS` 精确链窗交集。
2. 缺少 typed effective/intersection 时，规则可消除及其百分比显示 `—`；不再以 raw
   span 墙钟回填。已发布 0 仍显示 `0.000ms/0.0%`，与 unavailable 保持可区分。
3. 单 span partial-overlap 回归固定 `raw=5.000ms / eliminable=4.600ms /
   eliminable-share=65.7%`；另固定 authority 缺席和 published-zero 两臂。
4. family 表头、计数折叠指针、多工件 evidence label 和中英文 7 列同步更新；完整
   `go test ./internal/tool -count=1` 通过（157.681s），`git diff --check` 通过。

不变量：没有重算区间或改变 root rank；显式窗选举、唤醒链、两维投影、系统自动补采、
typed causal ceiling 及 trace_query 输出保持原样。

状态：`EVAL-B22-SEMAXIS1=implemented/full-tests-pass/replay-next`。下一批严格并行
2 case 覆盖 partial-overlap semantic span 与另一种高优先模式。

#### B22 r2：双轴正证 + write/apply 的 Go 同包探针缺口

同一 `main@7e8b03c98` 二进制快照下严格并行 2 个 case：

- `trace_query_frame_semantic_span_optimization`：runner/human PASS，146s；
- `patch_go_typo`（`MODE=apply`）：runner/human PASS，158s。

Trace 正证中，确定性优化表已同时发布 `窗内 span 墙钟=5.000ms` 与
`规则可消除=4.600ms / 65.7%`，不再把 raw span 当作有效收益；显式
`5.000000..5.007000` 窗、主要占用、根因排序、唤醒链、代表窗、完整
`Trace 因果投影` 和自动补采均保留。typed
`frame_causality=unproven / frame_evidence_status=absent` 继续接管最终主结论，
证明 B22-A 与 B19 causal ceiling 可共同工作。状态：
`EVAL-B22-SEMAXIS1=covered`，`EVAL-B19-CAUSAL1=covered`。

write/apply 的最终 patch、隔离 worktree 和 deterministic `go test` 均正确；但规划过程
连续三次触发同一 coupling reject：

1. 旧 Go verification probe 只能由 standalone `package main` 通过 `import` 变更包；
2. 当变更包本身是 `package main` 时，该包不能作为普通库导入；需要验证未导出符号时
   也存在同样结构障碍；
3. planner 已生成合法形状的同包 `TestGreet(*testing.T)`，但 validator 仍只读取 imports，
   因而拒绝；删掉 `changed_symbol_refs` 也无效，最终只能删除整个 probe；
4. 项目既有测试兜底使本 case 仍然正确，但无测试仓库会失去本可执行的 bounded proof。

登记 `EVAL-B22-GOMAINPROBE1/P1`。它覆盖所有 Go command package 和未导出符号，
不是 `greet`/typo 单例，也不能靠放松 `changed_symbol_refs` 或接受自报引用解决。

#### B22-B：Go external-import / same-package test 双载体

已实现通用 typed 方案：

1. `language=go` 的外部 import 探针保持原路径和原耦合门；
2. 新增同包载体，只接受 AST 可证、Go test 会实际执行的
   `TestX(*testing.T)` 完整源码；假的 helper、缺 `testing` import、错误签名不会进入该车道；
3. emit-time coupling 要求 probe 的 repo-relative `working_dir` 精确等于某个变更 Go
   production package 目录，且 probe package declaration 精确等于该变更应用后的 package；
   package/目录错配继续 fail-closed；
4. verify 时不向 worktree 写测试文件：系统创建私有临时 backing source 和 overlay JSON，
   用 `go test -overlay ... -run ^TestX$ -count=1 .` 仅执行该 bounded test，随后清理；
5. planner soft guidance 明确 `package main`/未导出符号使用同包测试；没有读取或扫描
   用户原文、模型 thinking/summary/final，也没有 case ID/函数名规则；硬门只读取 typed
   probe code AST、working_dir、变更路径和应用后 package clause；
6. 定向测试固定 external import 不回归、同包 command probe 真执行、临时载体清理，
   以及同包 coupling 的正例和 package/目录错配负例。

完整 `go test ./internal/tool -count=1` 通过（158.208s），
`go test ./internal/skill -count=1` 通过（0.480s），`git diff --check` 通过。

不变量：该批只触及 write-mode bounded verification probe 的 Go 执行/耦合车道；read
mode 字节路径、Trace 显式窗、因果投影、根因排序、唤醒链、可消除计算和系统自动补采
均未改变。

状态：`EVAL-B22-GOMAINPROBE1=implemented/full-tests-pass/replay-next`。

#### B22 r3：Go 同包载体接通，验证权威四处失配；配置答案出现复合 API 幻觉

同一 `main@1e1e0c03a` 二进制快照下严格并行 2 个 case：

- `patch_go_typo`（`MODE=apply`）：runner PASS / human FAIL，170s；
- `qf_config_precedence`（read）：runner PASS / human FAIL，123s。

write 正证与新 gap：

1. 第一版 external-import probe 被 coupling gate 拒绝后，planner 按 repair pack 改成
   `package main + TestX(*testing.T)`，证明 B22-B 接线生效；patch 也精确修正目标字节。
2. 登记 `EVAL-B22-GOFAILSIG1/P1`：合法同包测试使用 `t.Errorf`，旧 failure-signal
   检测只接受 `t.Fatal/t.Fatalf`，把 Go test 原生失败语义误判成无失败信号。
3. 登记 `EVAL-B22-GOMULTITEST1/P1`：一个 probe 声明两个合法 `TestX`，解析器只返回
   第一个名字，`go test -run ^first$` 导致第二个真正行为测试从未执行。
4. 登记 `EVAL-B22-NATIVEDIAG1/P2`：Go/Java 成功由 process exit=0 表达，不生成解释型
   wrapper status；诊断器却把缺 status 误记为 `verification_probe_unclassified` warning。
5. 登记 `EVAL-B22-PROOFSKIP1/P1`：probe 没有引用 3 个 fallback behavior contracts 时，
   pre-suite 决策把它们视为 advisory 而跳过已发现的 `go test ./...`；下游 proof ledger
   又把相同缺口视为开放义务，追加 cumulative review。follow-up 只能重跑相同 probe，
   因而必然以 `verification_proof_incomplete / accept_unverified` 结束。这是同一 typed
   contract 在上下游权限不一致，不是 Go/typo 特例。

read case 的默认值 50、`code default → codrax.yaml → CLI` 结论正确，但精确 API 被写成
不存在的 `flagMaxSteps.Changed()`；源码实际为
`cmd.Flags().Changed("pipeline-max-steps")`。Explorer 已读取真实行，第一次因 gutter 未读被拒
后补读；第二次用 `anchor_kind=condition, anchor_symbol=flagMaxSteps` 再被拒，模型识别出
应锚定 `Changed` 却直接 complete。最终 citation pool 没有 2664 行，finalizer 仍从未授权
上下文拼出错误 receiver。登记 `EVAL-B22-COMPOUNDREL1/P1`：单个 token 分别可见不等于
receiver→method 复合关系已证。它与 B21-CALLEE1/SPAN1 同属 typed source relation
entailment，继续保持 open，禁止用答案 prose 的 `.Changed()` regex 硬门拟合。

#### B22-C：结构化 Go 语义与 verification contract 权威收敛

本批完成四个通用闭环：

1. Go failure signal 改由 `go/parser + AST` 判定，支持 builtin `panic`、实际导入别名的
   `os.Exit/log.Fatal*`，以及真实 `TestX(*testing.T)` 参数上的
   `Error/Errorf/Fail/FailNow/Fatal/Fatalf`；字符串、注释、假对象及局部 shadow 不再能
   冒充失败信号。硬门读取结构化代码 AST，不扫描用户请求或模型答案 prose。
2. 同包 carrier 返回全部合法 `TestX` 名字，runner 用转义后的精确 regex union 一次执行
   全部声明测试；定向负例让第二个测试失败，确认不会再出现“首测通过掩盖后续未运行”。
3. 对 Go/Java 的 native process，`exit=0 + outcome=executed + 无 typed reason/status`
   明确表示成功，不再生成 unclassified authoring warning；非零退出和 wrapper typed status
   的原诊断保持。
4. probe 只有在覆盖全部 required typed contract refs（包括 fallback）时才可跳过已发现的
   deterministic project suite。缺 fallback ref 本身仍只是 soft obligation，不成为 emit
   hard reject；但既然已有项目 suite，就实际运行它提供独立 concrete proof，避免下游创建
   不可能闭合的重复 probe follow-up。全 refs 已覆盖时仍允许 bounded probe 快速跳过 suite。

测试覆盖 AST alias/shadow、`t.Errorf`、多 Test 执行、native success 零误告警、fallback
缺引用续跑与引用完备不续跑。`go test ./internal/tool -count=1` 全量通过（160.441s），
定向追加回归通过，`git diff --check` 通过。

不变量：本批仅修改 write verification probe/项目 suite 的 typed 执行与证明闭环；read
mode 事实抽取、Trace 显式时间窗、因果投影、根因排序、唤醒链、窗内可消除量和自动补采
均未改动。

状态：`GOFAILSIG1/GOMULTITEST1/NATIVEDIAG1/PROOFSKIP1=implemented/full-tests-pass/replay-next`；
`COMPOUNDREL1=open/next-design`。

#### B22-D：typed boolean condition 等价与行内 anchor 收敛

对 `COMPOUNDREL1` 冷读后，先修其确定的上游根因，不直接约束 final prose：

1. qf 证据提交的 typed condition 是
   `cmd.Flags().Changed("pipeline-max-steps") == false`，源码是
   `if !cmd.Flags().Changed("pipeline-max-steps")`；两者是精确布尔等价，旧 grounder 只做
   归一化子串比较，错误 demote 为 unsupported condition。
2. 同一 item 的 `anchor_symbol=flagMaxSteps` 来自 guard body 下一行，不在 condition 行；
   旧系统没有在“condition 已与当前行等价”的前提下收敛 navigation anchor，最终整条
   2664 行 evidence/citation 消失。
3. 新增语言中立、保守的 atomic boolean canonicalizer，仅识别单一表达式上的
   `!x / not x / unless x / x == false / false == x / x != true` 等精确同义；相反极性、
   引号内 `"false"`、顶层 `&&/||/and/or/比较` 一律不等价，不尝试通用逻辑证明。
4. 只有 typed condition 与 read_file 当前行精确/上述等价时，才允许把不可见的
   `anchor_symbol` 改为 condition 字段中当前行可验证的 durable token；selector chain
   取最外层可见成员，例如 `Changed`。它只修导航锚点，不把 condition 偷换成 call claim。
5. 原有错误 condition 继续 ungrounded；没有读取 RawRequest、模型 thinking/closure、
   final answer 或 case ID，也没有针对 pipeline_max_steps、Go 或某个 receiver 写规则。

定向测试覆盖 qf 原始 payload 形状并确认 evidence 变为 grounded、anchor 收敛为
`Changed`；另覆盖 Go/Python/Ruby 等价正臂以及 quoted-false、相反极性、compound
precedence 负臂。`go test ./internal/tool -count=1` 全量通过（167.761s），
`git diff --check` 通过。

状态：`EVAL-B22-CONDNEG1=implemented/full-tests-pass`；
`EVAL-B22-COMPOUNDREL1=upstream-root-fixed/replay-required`。只有回放确认 2664 citation
进入最终证据池且精确 receiver 不再漂移后才可 closed；否则继续设计 typed
receiver→method relation carrier，不以答案文本扫描代替。

#### B22 r4：Trace 主合同通过；稳定证据 amendment 形成混合载体（2026-08-01）

同一 `main@6bbb5896f` 二进制快照下严格并行 2 个 read case：

- `trace_query_frame_semantic_span_optimization`：runner/human PASS，127s；
- `qf_config_precedence`：runner PASS / human FAIL，266s。

Trace 正证完整：显式 `5.000000..5.007000` 窗、目标四态分区、主要时间占用与规则可消除
双轴、根因排序、`worker-200 -> app-100` 唤醒链、代表窗、完整 `Trace 因果投影` 和系统
自动补采均在；首结论及边界保持
`frame_causality=unproven / frame_evidence_status=absent`。因此本轮新 gap 不触及 Trace
值计算和发布合同，后续施工必须继续以该 case 作反向不变量。

配置 case 的默认值 50 与总体优先级方向仍正确，但最终把源码
`if !cmd.Flags().Changed("pipeline-max-steps") { flagMaxSteps = mergedMaxSteps }` 写成
`flagMaxSteps != defaultMaxSteps`，且叙述成反向赋值 `mergedMaxSteps = flagMaxSteps`；末尾
标量 `50` 又引用了 `PipelineMaxStepsCeil` 字段行。日志显示这不是 B22-D 的布尔等价器
失效，而是后发 amendment 破坏了已经 grounded 的完整载体：

1. 首次 item 是 `anchor_kind=condition`，含完整 Condition；B22-D 把不可见 anchor
   收敛为当前 guard 行可见的 `Changed`。
2. 模型补读后再次发出同 stable-ID item，但删除 Condition，并把 anchor 改回
   `flagMaxSteps`，同时把三元组改成 `flagMaxSteps assigns mergedMaxSteps`。
3. `MergeEvidenceItemByStableID` 对非空字段逐字段采用新值，却因新 Condition 为空保留
   旧 Condition；结果是旧 guard、新 anchor、新 assignment triple、新 multiline snippet
   的不可能混合体。
4. Answer support 再把 condition anchor 当 assignment relation 输出，并把 body line 的
   文本挂到 guard 行坐标；finalizer 据此反转真实赋值方向。一次 504 重试只影响耗时，
   不改变该确定性系统 gap。

新登记：

| ID | 优先级 | 系统 GAP | 泛化方案 |
|---|---:|---|---|
| `EVAL-B22-EVMERGE1` | P1 | 同 stable-ID 的后发、同 authority 但更稀疏 amendment 可逐字段覆盖完整 typed carrier，形成跨版本混合事实 | 合并按结构化 coherence bundle 与 authority/完整度单调进行：低完整度版本只能补空/set-like 字段，不能拆散已有 Condition/anchor/claim triple；完整的新版本仍可整体纠错。测试同时覆盖 sparse retry 不降级与 richer correction 可替换 |
| `EVAL-B22-STMTSPAN1` | P1 | `anchor_kind=condition` 的多行 snippet 和 assignment triple 被支持面解释为 guard 行上的赋值语句 | claim-form 权限优先：condition item 只发布 guard/Condition；body assignment 必须由 body 精确行上的独立 assignment evidence 承载。emit-time 用 typed anchor/predicate/scope 提示拆项，不扫描答案 prose |
| `EVAL-B22-CITNEIGH1` | P1 | citation quote repair 可把相邻 body 行文本挂到原 guard 行坐标，形成 `line=2664 / quote=line2665` | citation 坐标与摘录保持原子一致；若使用邻行，必须以已 grounded typed evidence 同步改坐标，否则保留真实当前行或拒绝回填 |
| `EVAL-B22-SCALARCIT1` | P1 | scalar block 的 `50` 可引用不含 50、且语义属于另一配置键的行 | scalar block 只接受 typed scalar/value carrier，或标量 token 在 cited quote 精确可见；派生值走显式 aggregate/external-observation 通道，不以最终 prose 关键词硬门 |
| `EVAL-B22-NEARKEY1` | P2 | exact target 为 `PipelineMaxSteps` 时，相邻 sibling key `PipelineMaxStepsCeil` 进入 principal answer | 先修 typed support/citation 权限后回放；若仍复现，再在 exact-target typed role 上做 principal/context 分层，不加配置名特例 |

状态：`EVAL-B22-CONDNEG1=implemented`；`EVAL-B22-COMPOUNDREL1` 不能关闭，根因已收窄为
`EVMERGE1 + STMTSPAN1`。下一批先修这两个共同载体问题，再用恰好 2 个异构 case 并行回放；
`CITNEIGH1/SCALARCIT1` 独立成后续批，避免把引用和值权限混成一个大改。

#### B22-E：evidence coherence bundle + claim-form 权限收敛

本批完成 `EVAL-B22-EVMERGE1` 与 `EVAL-B22-STMTSPAN1` 的通用结构修复：

1. `MergeEvidenceItemByStableID` 不再逐字段采用“最新非空值”。Kind/scope、
   subject/predicate/object、Condition、anchor kind/symbol/owner 与 snippet 组成同一个
   claim carrier bundle；只有 grounding 不低、coherence 与结构完整度不低的新版本才可
   原子替换。
2. 已有 coherent carrier 遇到更稀疏版本时，后者只能合并 summary、surface terms、
   provenance 等独立/set-like 字段，不能把空位补成跨版本混合事实；更高 grounding 的
   完整 assignment/definition 等纠错仍可整体替换并清掉旧 Condition。
3. condition 的 typed `ClaimFormOf=guard_condition` 现在压过自由三元组和 multiline
   snippet：authoritative evidence surface、两套 exact-resolution seed、relation dossier、
   AnswerDocument relation handoff 都只发布 guard/Condition。body assignment/return 必须由
   对应 statement 行的 assignment/return evidence 独立承载。
4. Condition 存在时不再回放可能包含 body 的整段 snippet；Condition 缺席时最多显示
   snippet 第一行，禁止把后续正文挂到 guard line 上。
5. 定向测试固定本轮真实形状：完整 `Changed` guard 后跟 condition-less assignment
   retry，合并结果保持完整 guard；另固定更高 grounding 的完整纠错可替换，以及四个
   下游面均不再输出 `flagMaxSteps -> mergedMaxSteps relation=assigns`。

这不是 Go、某个配置键或 `Changed` 的特例：硬决策只消费 typed grounding status、
anchor kind、Condition 与结构完整度；不读取 RawRequest、模型 thinking/summary/final
prose 或 case ID。Trace external observation、显式窗、投影计算、根因排序、唤醒链、
可消除量和系统补采均未改动。

全相关回归通过：

- `go test ./internal/types ./internal/context ./internal/agent ./internal/tool -count=1`；
- types 18.790s、context 0.777s、agent 3.105s、tool 170.604s；
- `git diff --check` 通过。

状态：`EVAL-B22-EVMERGE1=implemented/full-tests-pass`；
`EVAL-B22-STMTSPAN1=implemented/full-tests-pass`；
`EVAL-B22-COMPOUNDREL1=replay-required`。`CITNEIGH1/SCALARCIT1` 保持下一独立批。

#### B22-E r1：read/write 双模式正证（2026-08-01）

同一 `main@db3b1e2d5` 二进制快照下严格并行 2 个异构 case，runner/human 均 2/2 PASS：

1. `qf_config_precedence`（read，137s）：2664 的 prompt 支持面只发布
   `initApp guard condition IF !cmd.Flags().Changed("pipeline-max-steps")`；relation dossier
   也为 `verified guard`，不再出现 `flagMaxSteps -> mergedMaxSteps relation=assigns`。
   最终准确说明“CLI 未显式传入时，`mergedMaxSteps` 回写 `flagMaxSteps`；否则保留 CLI
   值”，默认值 50 引用 `cmd/root.go:88`，没有 sibling ceiling 或行号/摘录错位。
2. `patch_go_typo`（write/apply，108s）：单一 ChangePlan、单一一行 patch，只将
   `retrun` 改为 `return`；隔离 worktree 内 `go test -json ./...` 成功，changed path 为
   project-runner covered，最终 verified 与 typed report 一致。worktree 保留来自 eval fixture
   显式 `pipeline_keep_worktree_on_success=true`，用于 post-apply oracle，并非产品清理失效。
3. 本轮证明稀疏 amendment、condition claim-form 和 write verification 可以同时工作；
   `EVAL-B22-EVMERGE1/STMTSPAN1/COMPOUNDREL1` 状态更新为 `covered`。
4. `CITNEIGH1/SCALARCIT1/NEARKEY1` 本轮均未复现，但前一轮已证明系统允许错误形状，
   因而暂不以一次绿样本关闭；继续按独立引用/标量权限批施工。

新增低优先级 eval 基建项 `EVAL-B22-EVALMODE1/P3`：write/apply 固定的安全控制器流程本轮
有 10 个合法 dispatch（分析、控制器 plan/apply/verify/finish 等），通用效率阈值仍以
`limit=8` 标记 `high_pipeline_dispatches`。它不影响产品结果，但会把正常 write 状态机噪声
混进高耗时审计；最优方案是按 typed mode/workflow phase 校准期望预算，不按 case ID 或日志
关键词豁免。该项低于引用权限和下一轮高优先 eval，先 filed。

#### B22-F：atomic scalar → value-bearing citation authority

对 `CITNEIGH1` 冷读后修正分类：`normalizeCurrentSourceCitationQuotes` 始终按 citation 的
精确 `file+line` 回填该行，没有把 2665 摘录写入 2664；r4 的错位来自 condition evidence
surface 优先回放 multiline snippet/body，已由 B22-E 的 claim-form 修复关闭。因此
`EVAL-B22-CITNEIGH1=covered-by-B22-E`，不重复修改 citation normalizer。

`EVAL-B22-SCALARCIT1` 则是真正独立的值权限 gap，本批按以下合同修复：

1. 新增 atomic scalar literal citation 归一化，作为既有 code-identity scalar 车道的并列
   typed 车道；只处理 `BlockScalar` 且 claim form 为 source-code
   definition/assignment/return/literal-value 的单一原子值。
2. 若当前 citation 的精确 source-line quote 不含 scalar token，则在当前 citation pool 与
   grounded EvidenceItem 的 snippet/authoritative typed fields 中寻找唯一 value-bearing
   `file+line`；唯一则重绑，多候选且当前 mismatch 可证则 detach 并走既有 typed disclosure。
3. token 匹配复用代码边界规则，`50` 不会命中 `500`；反引号包裹的单值可归一，未加引号且
   含空白的解释性 text 不进入此车道。
4. external observation、absence 与 derived aggregate 保持原 typed 权限：它们的值无需在
   源码行字面出现，不会被本车道重绑或硬拒绝。
5. 生产接线 pin 直接调用完整 `normalizeAnswerDocumentForPreEmit`，固定错误 sibling citation
   被重绑到唯一承载 `50` 的常量行；另覆盖多候选 detach、external/absence/prose 负臂。

硬决策不读取 RawRequest、summary/thinking/final prose 或 case ID，只读取 block kind、
claim-form enum、block scalar value、精确 citation quote 和 grounded evidence typed fields。
它不触及 Trace 显式窗、因果投影、根因排序、唤醒链、可消除计算与自动补采。

验证：定向与生产接线测试通过；`go test ./internal/tool -count=1` 全量通过（159.008s），
`git diff --check` 通过。

状态：`EVAL-B22-CITNEIGH1=covered`；
`EVAL-B22-SCALARCIT1=implemented/full-tests-pass/replay-next`；
`EVAL-B22-NEARKEY1=repeat-after-authority-fixes`。

#### B22-F r1：Trace 不变量通过，标量重绑被后置发布链撤销（2026-08-01）

严格并行 2 个案例，runner 2/2 PASS；人工审计为 Trace PASS、配置 FAIL：

1. 显式 5.000000..5.007000 Trace 窗完整保留目标状态账
   `running=1.200ms / runnable=0.800ms / sleep=5.000ms`、真实占用与规则可消除
   双轴、`class_verification raw=5.000ms / eliminable=4.600ms`、根因排序、
   `worker-200 -> app-100` 唤醒链、代表窗、`Trace 因果投影`、
   `frame_causality=unproven / frame_evidence_status=absent` 和 45 条系统补采。
   因此 B22-F 没有破坏显式时间窗的核心能力。
2. 配置答案的 mechanism 正确，`PipelineMaxStepsCeil` sibling key 也未进入最终答案，
   `EVAL-B22-NEARKEY1` 可更新为 `covered-after-authority-fixes`。
3. 但 `EVAL-B22-SCALARCIT1` 尚不能关闭。生产日志的
   `normalizeScalarLiteralCitationRefsWithContext×1` 最初被误读为“已重绑”；结合代码
   与完整 citation ledger 冷读后纠正：`50` 同时出现在代码常量定义、示例注释和
   CLI 注册的证据摘要里，旧实现把所有 token 命中视为同权，`len(seen)!=1` 后走
   typed mismatch detach。随后 unused prune 删除无人引用的 `cmd/root.go:88`，这是
   摘除后的正常级联，不是 prune 自己改坏了已绑定引用。
4. 登记 `EVAL-B22-SCALARAUTH1/P1`：atomic scalar 候选只有“值是否出现”的集合，
   缺少 typed claim/anchor/origin/实际源码行的 authority 排序。source literal 定义、
   precedence 注释与 load-bearing 摘要被放在同一档，多个正确相关位置反而导致主值
   完全失去引用。该问题适用于所有同值多出处的配置、常量和返回值，不是 `50` 特例。
5. 最优方案：候选先按真实当前源码行是否含值、`ClaimFormOf` 是否属于
   `literal/definition/assignment/return`、结构化 snippet/authoritative surface、
   grounding/salience 分层；只接受唯一最高 authority，最高档仍并列才 fail-closed
   摘除。低权 precedence 注释和 summary 只在没有更强值载体时兜底。以完整生产链
   测试固定 `wrong sibling citation -> typed authority winner -> prune/remap -> persist`。
   仍不读取 RawRequest、thinking 或 summary/final answer prose，也不新增硬门。

状态：`EVAL-B22-SCALARCIT1=partial`；
`EVAL-B22-SCALARAUTH1=P1/open`；`EVAL-B22-NEARKEY1=covered`。

#### B22-G：atomic scalar typed authority 排序

已按上述冷读结论完成通用修复：

1. scalar 候选由“所有值命中必须唯一”改为“唯一最高 authority”；同一位置只保留
   最高分，最高档并列仍 fail-closed，不用 map 顺序或首条猜测。
2. authority 只来自当前源码 canonical quote、`EvidenceItem` 的 grounding、
   `ClaimFormOf`、anchor kind、typed authoritative surface 与 salience。
   `literal_value` 最高，`definition/assignment/return` 次之，`precedence_role` 与
   `text_reference` 只作低权兜底；load-bearing summary 不能压过真实源码行。
3. doc citation pool 只在 canonical current-source quote 确实包含 atomic value 时进入
   候选，模型提交的陈旧/错 quote 不能自己制造权威。
4. 完整 full-emit 组合测试覆盖 sibling 错引、同值 precedence 注释、typed definition
   胜出、枚举载体物化、unused citation prune 与最终 persist；原有同档双定义歧义
   摘除、`50` 不匹配 `500`、external/absence/prose 负例继续通过。
5. 无 RawRequest、case ID、模型 thinking/summary/final prose 扫描；没有新增 hard reject，
   显式 Trace 窗路径不在修改面。

验证：`go test ./internal/types -count=1` 通过（21.612s）；
`go test ./internal/tool -count=1` 通过（159.957s）；`git diff --check` 通过。

状态：`EVAL-B22-SCALARAUTH1=implemented/full-tests-pass/replay-next`；
`EVAL-B22-SCALARCIT1=implemented/full-tests-pass/replay-next`。

#### B22-G r1：typed scalar authority 与 Trace 不变量回放（2026-08-01）

严格并行 2 个案例，runner 2/2 PASS，人工审计 2/2 PASS：

1. `qf_config_precedence` 最终把默认值 `50` 引用到真实定义
   `cmd/root.go:88`，引用池未混入 sibling key `PipelineMaxStepsCeil`；解析、YAML
   回填与 CLI 显式覆盖的机制也正确。因此
   `EVAL-B22-SCALARCIT1/SCALARAUTH1/NEARKEY1=covered`。
2. 本次模型首次提交已经选择正确 value carrier，生产回放没有触发 B22-G 的重绑分支；
   因而它是最终行为正证，不冒充修复臂的直接动态证据。修复臂由完整 production-chain
   单测固定：错误 sibling citation 与同值低权 precedence carrier 同时存在时，typed
   definition 唯一胜出，并穿过枚举物化、unused prune 与 persist。
3. `trace_query_frame_semantic_span_optimization` 完整保留显式
   `5.000000..5.007000` 窗、`running=1.200ms / runnable=0.800ms /
   sleep=5.000ms`、主要时间占用/规则可消除双轴、
   `class_verification raw=5.000ms / eliminable=4.600ms`、根因排序、
   `worker-200 -> app-100` 唤醒链、代表窗、`Trace 因果投影`、
   `frame_causality=unproven / frame_evidence_status=absent` 与 45 条系统补采。
   B22-G 未触及也未回归显式 Trace 窗合同。
4. 新登记 `EVAL-B22-COUNTDOMAIN1/P3`：配置日志的软结构提示把“配置优先级层共 3 项”
   与 summary/scalar/table 中独立事实 `50` 比较，连续产生
   `expected_count=3 visible_count=50`。根因是 aggregate member-set count 校验没有按
   typed facet/carrier domain 隔离数字；当前只产生一次 soft advisory，不硬拒、不改变
   最终答案，故低于跨模式高优先 eval。后续应只读取该 aggregate 自身的 typed count
   claims/成员载体，不能扫描无关 block 文本，也不能按 case、配置名或数字特判。

状态：`EVAL-B22-SCALARCIT1=covered`；`EVAL-B22-SCALARAUTH1=covered`；
`EVAL-B22-NEARKEY1=covered`；`EVAL-B22-COUNTDOMAIN1=P3/filed`。

### B23 r1：plan/read 异构回放与 aggregate origin 越权（2026-08-01）

同一 `main@b292678ca` 二进制快照下严格并行 2 个案例：

- `patch_python_typo`：runner PASS，64s，人工 PASS；
- `read_combo_git_current_source_explanation`：runner FAIL，127s，人工 FAIL。

正证与机器判定边界：

1. Python plan-only 正确生成 `main.py` 单文件单行 `kind=patch`，
   `retrun -> return`，pre-apply 与 Python dry-build 通过，主仓未修改。首次把同一
   verification probe 同时放在 change/top-level，被 typed repair 精确拒绝；第二次收敛并
   规范化成一个顶层 probe。这是安全自恢复，不登记新 gap。
2. read case 的 typed `latest_one/merge` 与
   `git_log count=1/merges_only=true/first_parent=true` 正确锁定
   `2a58a60d...`，3/3 changed paths 完整；历史/current path 错绑和旧 member supplement
   均未复发。runner FAIL 仍是 `EVAL-B21-E1/P3` 的单行 regex 假阴性：答案分段具备
   history 与 current-source 两维，不为机器分数修改生产文案。
3. 人工失败有独立真因。`AnswerAggregateFactEvidenceOrigins` 只因
   `RequestModel.Predicates.IsHistoryLookup=true`，就把带两个精确 current-source
   `support_refs` 的“受影响的生产代码链路”member-set 同时铸成
   `current_source + vcs_metadata`。Finalizer prompt 因而收到同一 fact 的 factual 当前源码
   binding 与 historical binding，绕过已经存在的
   `historical_transition=unproven` 分席边界。
4. 最终把通用 `detectRunnerMissingForPlan` 写成“该逻辑仅在 pytest 相关路径生效，
   不影响 go test/make 等其他 runner”。源码函数实际先对所有 runner 执行通用缺失检测与
   exit 126/127 判断，再有 Python/unittest 的窄例外。该错误再次复现
   `EVAL-B21-CALLEE1/SPAN1`：definition anchor 与自由 summary/section 缺少逐 claim 的
   source-span entailment。现有软提示没有阻止模型扩写，因此状态保持 P1，不按 pytest、
   函数名或最终答案词面追加硬门。

登记 `EVAL-B23-ORIGIN1/P1`：request evidence-source shape 不能给每个 aggregate fact
批量继承相同 origin。history/current、runtime/current、command/current 混合问题都必须由
fact 自身的显式 origin 与精确 support carrier 决定；请求形状最多在 fact 完全没有来源时
提供兼容 fallback。

#### B23-A：history fallback 不再覆盖 fact-local origin

已在唯一 origin 铸造点完成通用收窄：

1. `AnswerAggregateFactEvidenceOrigins` 先计算 fact 自身的显式 origin 和精确
   current-source support。history request 只有在二者都不存在时，才为可承载类型补
   `vcs_metadata` fallback。
2. 带精确源码行的 current fact 不再因题目同时问历史而获得 historical binding；
   `CompileAnswerClaimBindingsFromAggregateFacts` 生产接线测试固定最终只能出现
   `current_source` hard binding，不能再出现 `vcs_metadata/vcs_diff`。
3. producer 显式给出 `vcs_metadata` 且同时带源码 support 时，双 origin 保持；没有来源
   的 latest-commit scalar 仍走 VCS fallback；`git_history_search` 等既有 typed provenance
   不变。因此修的是来源权限，不是关闭混合证据答案。
4. 规则只读 aggregate 的 typed dimensions/support refs 和 RequestModel enum；不读取
   RawRequest、commit subject、case ID、模型 thinking/summary/final prose，也不增加
   answer hard reject。

验证：`go test ./internal/types -count=1` 通过（17.140s）；
`go test ./internal/tool ./internal/agent -count=1` 通过（tool 159.695s、agent 3.056s）；
`git diff --check` 通过。

状态：`EVAL-B23-ORIGIN1=implemented/full-tests-pass/replay-later`；
`EVAL-B21-ORD1/MERGE1/TRANS1/MAP1/SUP1=covered`；
`EVAL-B21-CALLEE1/SPAN1=P1/reproduced-after-soft-mitigation`；
`EVAL-B21-E1=P3/filed`。本批未修改 Trace query/family、显式时间窗 authority、
因果投影、根因排序、唤醒链、窗内可消除量或系统自动补采。

### B19a r1：Trace 主合同保留，精确集合暴露静默改写（2026-08-01）

严格并行 2 个用例，runner 2/2 PASS，人工审计 0/2：

1. `trace_query_donghu_real_frame_multicausal` 完整保留指定
   `34579.472865..34579.587805` 窗、根因排序、wakeup chain、
   `◎ 窗内可消除量总览`、`Trace 因果投影`、coverage boundary 和系统补采。
   因果边界也诚实披露 `frame_causality=unproven`、
   `frame_evidence_status=absent`、`enumeration_status=incomplete`。
2. 但 explorer 的结构化 payload 同时声明
   `member_set value=19` 与 7 个 members，其中末项还是“其他13个线程”占位。
   当时的归一化器把合法整数 19 静默覆盖为 `len(members)=7`，final 又据此写成
   “全窗口共有7个 D/IO 阻塞线程”。这是把“总数 + 部分样本”伪装成“精确完整
   成员集”，人工判 FAIL。
3. `read_combo_git_two_diffs_current_code` 不再发布
   “系统按已验证证据补充缺失成员”主表，证明 `EVAL-B19-HIST1` 已覆盖。
   但模型把定义计入所谓 10 个调用点，同时漏掉 `facet_plan.go` 和
   `answer_intent_contract.go` 的另一处调用，并把部分调用位置写成不存在的
   函数名；人工判 FAIL。该错误与权限修复不同族，当前按 P3
   repeat-before-code，禁止针对函数名或数字做答案文本硬门。

状态：

| ID | 优先级 | GAP | 状态 |
|---|---:|---|---|
| EVAL-B19-INF1 | P1 eval | 真实 Trace fixture 路径漂移 | covered：仓内同 SHA fixture，回放启动成功 |
| EVAL-B19-HIST1 | P1 | mixed history/current-code 被强制补 principal member table | covered：本轮系统补表消失 |
| EVAL-B19-SET1 | P1 | 显式合法 member_set 整数与 members 不一致时被静默覆盖 | implemented / full-tests-pass / replay next |
| EVAL-B19-GREP1 | P3 | 行/文件/调用点单位或完整清册被模型误述 | observed twice in related shapes / soft investigation next |

#### B19b exact member-set cardinality fail-loud

通用修复只消费结构化 `aggregate_facts`：

1. `member_set.value` 缺省时仍从非空 `members[]` 派生；
2. `value="1+"` 这类 schema-adjacent 非整数仍可从精确 members 低成本修形；
3. 显式合法非负整数若与 members 长度不一致，保留矛盾并交给既有 cardinality
   validator 拒绝，不能猜 count 对还是 roster 对；
4. 若整数原本与 pre-dedupe slots 一致，仅因确定性 alias/duplicate
   normalization 缩短 members，则允许同步派生新值，避免安全去重被误拒；
5. 历史 SUPPREF-TOL 原样 witness 继续保留其 `3/5` 矛盾作为档案；只在验证
   后续 support-ref 机制的测试输入中显式做单字段 `3→5` 修正，不篡改见证；
6. 不扫描 RawRequest、模型 reason/thinking/final prose，不识别“其他”、
   D/IO、线程名或任何具体 case/type；Trace projection、窗口补采与 answer
   mutation 路径均未修改。

回归：

- `go test ./internal/types -count=1`：25.724s；
- SUPPREF-TOL 两条定向回归：1.128s；
- `go test ./internal/tool -count=1`：166.756s。

状态：`implemented / full-tests-pass / same-pair replay next`。

### B19b r1：精确集合覆盖，Trace 展示与因果权限继续暴露（2026-08-01）

严格并行 2 个用例：

- Git runner/human PASS，128s；上轮调用点/文件数误述未复发，按模型波动处理，
  `EVAL-B19-GREP1` 不施工；
- Trace runner/human FAIL，172s。显式窗、根因排序、唤醒链、窗内可消除量、
  因果投影、coverage 和成文前补采全部保留，且本轮没有 `19/7` 矛盾；
  runner 因最终正文没有代表性时间窗而失败。

Trace 调查闭环已经发布三个具体代表窗，root-cause ranked seats 也携带 typed
occurrence interval；finalizer 却只提交一个 summary block。当前 requested
dimension evaluator 只对 count/member-set/boundary/evidence-source 角色启用
精确 patch retry，`stage_or_workflow` 仍是软展示提示，系统因果投影也没有
紧凑代表窗面。因此这是“证据已在、发布面缺席”，不是需要再查 trace。

同时登记两个独立权限问题：

| ID | 优先级 | GAP | 状态 |
|---|---:|---|---|
| EVAL-B19-SET1 | P1 | exact member-set count/roster 矛盾被静默覆盖 | covered：本轮无伪精确集，Trace 主合同无回归 |
| EVAL-B19-TWIN1 | P1 | typed ranked-seat occurrence windows 没有稳定可见发布面 | implemented / tests next |
| EVAL-B19-FRAME1 | P1 | `frame_causality=unproven` 在报告尾部，模型首段仍断言确定帧结果/根因 | filed：typed authority-first，禁止扩大 prose phrase rewrite |
| EVAL-B19-ARITH2 | P2 | 重叠根因席被模型相加成“累计超过53ms”，仅尾部 caveat 纠正 | filed：与 principal claim authority 合并设计 |
| EVAL-B19-GREP1 | P3 | 行/文件/调用点计量误述 | not reproduced / model variance |

#### B19c typed representative occurrence-window publication

通用方案直接扩充系统因果投影，不做请求/答案文本硬门：

1. 从 `TraceCausalProjection.RankedSeats` 读取 rank、subject、cause、
   `StartTs/EndTs`；旧载体无 RankedSeats 时回退现有 primary/on-chain nodes；
2. 仅接纳 `end > start` 且 start 非负的 typed interval，按 rank 稳定排序，
   以 subject/predicate/start/end 精确去重，最多发布前三席；
3. 在投影 lead 后生成“代表性时间窗”紧凑表，明确每行只是一个 occurrence，
   排序席位值仍是全查询窗聚合，禁止把两者混为单窗时长；
4. single/multi-artifact 共用同一 builder；多工件仍保持所有 decision surface
   在 lossless details/evidence 前；
5. 新 block id 纳入 exact reserved system family、cap budget 与 hierarchy；
   无合法 interval 时零输出；
6. 不读取 RawRequest、requested dimension label/source quote、模型
   reason/thinking/final text，不触发 retry/reject，不改变因果排序、投影、
   自动补采或 coverage 计算。

验证：

- 代表窗接纳/排序/去重/非法区间/精确 reserved-id 定向测试通过；
- 多工件顺序钉死为每工件
  `lead → representative_windows → key_metrics`，全部 lossless
  details/evidence 仍在决策面之后；
- `go test ./internal/tool -count=1`：160.413s。

状态：`implemented / full-tests-pass / replay next`。

### B19c r1：代表窗正负边界覆盖，结论权限顺序待修（2026-08-01）

严格并行 2 个回放，runner 2/2 PASS，人工 1 PASS / 1 FAIL：

1. `trace_query_donghu_real_frame_multicausal`：153s。显式窗、根因排序、
   唤醒链、窗内可消除量、因果投影、coverage 和成文前系统补采全部保留；
   projection lead 后稳定发布三行 typed 代表性时间窗，且每行明确单次
   occurrence 与全窗席位聚合不可互换。`EVAL-B19-TWIN1` 转 covered。
2. `real_trace_c2_dstate_iowait`：109s。无用户时间窗，0 次
   root/wakeup/blocking 查询，`trace_query_final_projection_blocks=0`；
   最终没有因果投影或代表窗表。模型探索虽在错误的 2段/0.351ms 与
   3段/19.671ms 间摆动，typed complete roster 仍使系统主值和正文收敛到
   `0.138+0.147+0.350=0.635ms`。人工通过。

因此 B19c 的产品边界已由同批正负 witness 覆盖：它没有把无窗状态查询重新
套进全量因果合同，也没有削弱显式窗因果与自动补齐。

显式窗正文仍有两个同源权限失败：

- typed coverage 明确
  `frame_causality=unproven/frame_evidence_status=absent`，模型首段却断言
  窗后 VSync 证明“上一帧仍在渲染并造成视觉卡顿”；
- 模型把可能重叠的 #1/#2 席相加成 42.9ms，并以“超过窗口一半”加固主张，
  而 typed 投影明确禁止跨席/跨修向简单相加。

这不是 B19c 的接线回归，也不是再增加 trace 查询能解决。原计划新增
`typed causal conclusion authority-first`，经用户审阅后回裁：不得继续在
答案最前面堆叠“系统权威”块。现有 frequency/target-state/blocking/
blocked_reason authority 已经把内部优先级协议暴露给用户，并抢在按意图组织
的正文之前；新建第五个前置块会继续恶化答案。

下一批改为通用 `user-first typed accuracy surfaces`：

1. 成文 prompt 继续只消费 validated coverage 与 causal projection typed
   seats，安全权限不删；
2. 用户可见标题/正文只保留与问题有关的精确值、证据范围和口径，不再出现
   “系统权威/以本块为准/后续模型正文”及裸 `typed authority` 实现协议；
3. frequency、target-state/wait、blocking lower-bound、blocked_reason
   caliber 和 current-source negative-scope 块不再强制占据第 0 位；保持
   模型正文及因果 decision surfaces 在前，确定性明细进入后续“数据与口径”
   层；
4. FRAME1/ARITH2 不另加前置块。先用既有 projection lead/coverage 的用户
   可读权限表达和结构顺序解决；若仍需系统拥有的 principal claim，必须先
   建立可与正文结论槽融合的 typed schema，而不是平行再造一份答案；
5. 不扫描 RawRequest、模型 thinking/reason/final prose，不做 phrase
   rewrite/reject，不改变 trace query、因果构造、投影、自动补采或无窗窄
   报告 gate；
6. 结构测试覆盖显式窗 projection 正例、无窗 focused 负例、精确 roster、
   frequency/coverage 边界和中英文用户可见词面。

状态：

| ID | 状态 |
|---|---|
| EVAL-B19-TWIN1 | covered：正例 deterministic table + 无窗负例零泄漏 |
| EVAL-B19-FRAME1 | next：user-first typed conclusion integration，不新增前置 authority |
| EVAL-B19-ARITH2 | next：复用 projection 主结论/不相加口径，不新增平行答案 |
| EVAL-B19-GREP1 | deferred P3：本轮不出现，按模型波动保留 |

#### B19d user-first typed accuracy surfaces

用户审阅指出“系统权威/系统权威主值”属于内部安全协议，不应强行占据答案
开头。全仓审计确认五类同源面：

- Trace frequency、target-state/wait、blocking lower-bound、
  blocked_reason caliber 均由同一个 lead helper 插到 tier 0；
- config/current-source negative-scope 独立把自己搬到 block 0；
- 它们混合了 provenance 优先级、精确值和用户所需口径，既重复正文，又把
  `typed/authority/roster_scope/后续模型正文/以本块为准` 暴露给用户。

本批保持所有 typed 构造、内部 trusted system marker、finalizer handoff 和
fail-closed 规则不变，只收敛用户可见组织：

1. 模型正文保持第一层；显式窗 Trace 的因果 projection lead/key metrics
   仍是第二层；
2. 四类 Trace 精确值/边界进入统一 data-caliber tier，位于因果决策面之后、
   lossless drill-down 之前；无窗 focused fact 仍是正文后精确明细，且不生成
   因果投影；
3. 标题改为“频率证据与结论边界”“目标线程状态与等待明细”“目标阻塞的
   观测范围”“blocked_reason 的记录口径”；正文只说明值、覆盖和不能外推
   的边界；
4. 完整等待 roster 继续逐段发布开始/结束/时长/state/iowait/caller，但去掉
   内部协议字段；
5. negative-scope 改为“未命中结果的搜索范围”，正文后发布，producer 映射
   为用户可读来源；
6. 中英文 tripwire 均禁止 `System authority/系统权威`、`后续模型正文`、
   `以本块为准/takes precedence` 泄漏；不读取 RawRequest 或模型 prose，
   不做 claim rewrite/reject。

验证：

- 定向中英文、顺序、精确 roster、frequency limit、negative scope 测试通过；
- `go test ./internal/types -count=1`：21.151s；
- `go test ./internal/tool -count=1`：最终复跑 166.543s。

状态：`implemented / full-tests-pass / explicit-window + focused-negative replay next`。

#### TWODIM-2：Trace 根因双轴的现状与下一批验收

用户再次明确 Trace 性能根因必须并列覆盖两个维度：

1. **实际时间占用/关键路径**：无论当前是否存在可消除公式，单个长 span、
   同类短 span 高频聚族后的累计、真实 running/runnable/D/IO/sleep 和链上
   等待都要可见，用于探索新的修复方向；
2. **已知规则可消除收益**：优先级反转纠正、算力供给折算、已证 IO/唤醒
   依赖等按现有规则计算、排序，回答“修什么预计能回收多少”。

代码冷审显示 `TWODIM-1/SPANVIS-1` 已实现重要基座，并非从零开始：

- engine 已按 `(tid, verbatim span name)` 聚族，使用
  `单次最长 TOP8 ∪ 合计最长 TOP8`，自身和 typed 链上 span 均可进入
  `◈ 业务线索`，族间不可相加；
- semantic-class spans 有独立确定性优化表；
- 可消除量计价为 0 的 on-chain context-only 原始占用，会以
  `未计价占用/真实占时/自身工作量(新方向)` 留在辅助账；
- tool description/parameters 与结构测试已经明确“root causes have TWO
  dimensions”。

但 B19c 真实输出仍证明展示闭环是 partial：

- 报告唯一醒目主结论仍定义为“已证链上单项最大可消除量”；
- 44.836ms 的未计价真实占用只在辅助行显示“3 行/最大值”，没有独立列出
  占用主体和时间维度排序；
- 业务 span、semantic span、scheduler raw occupancy、CPU cpu·ms 分散在
  不同区块，没有统一的“主要时间占用/关键路径”决策面；
- 因而高占时但暂未计价的工作仍可能被客户误读为非根因或不重要。

登记 `EVAL-TWODIM-2`（P1，下一 Trace 结构批）：

1. 复用现有 typed raw-window occupancy、BusinessSpanMention 和
   SemanticSpans，不新造第二套 span 聚类或链判定；
2. 增加独立“主要时间占用/关键路径候选”面，按各自合法口径展示主体、
   单次最长、族累计、发生窗和证据指针；wall-clock 与 cpu·ms 分栏，禁止
   混排或求和；
3. 现有“可消除收益”榜保持原值、原排序、重叠不可相加和修复方向；
4. 两轴互相引用但不互相改值：高占时可以是“新方向候选”，不自动获得可
   消除量；高可消除席必须同时保留其实际占时口径；
5. `frame_causality=unproven` 时只能称“该窗主要占用/候选瓶颈”，不得宣称
   已证明导致具体丢帧；有 typed frame/deadline 边时才升级帧结果措辞；
6. 以 long-single span、many-small clustered span、unpriced on-chain
   occupancy、cpu·ms 多核占用和无 span 负例组成结构矩阵；不得按 span 名、
   case ID 或用户关键词触发。

#### B19d r1：用户优先顺序通过，typed scope 与双轴展示仍开放（2026-08-01）

严格并行 2 个同批回放，runner 1 PASS / 1 FAIL，人工 0 PASS / 2 FAIL。
本轮先确认 B19d 的目标本身已经覆盖：

- 两个答案都由模型按用户意图组织的正文起首，不再出现“系统权威”“系统
  权威主值”“后续模型正文”“以本块为准”等内部协议；
- 显式窗样本的 `Trace 因果投影`、根因排序、唤醒链、`◎ 窗内可消除量
  总览`、三行代表窗、coverage 和成文前确定性补采全部保留；
- frequency/state 等确定性数据与口径位于因果决策面之后、lossless 明细
  之前；
- focused 无窗事实样本保持 0 个 projection/root/wakeup/blocking 面，没有
  被重新套进全量因果合同。

人工失败揭示两个不能由前置系统块掩盖的系统问题。

**`EVAL-B19-SCOPEJOIN1`（P1）：多个 typed 查询窗没有选出唯一请求主范围。**

无窗问题要求“这份 trace”全文件清单。ledger 同时携带：

- 探索子窗 `34579.450000..34579.470000`：2 段、0.285ms；
- 全工件请求窗 `34579.450627..34579.595184`：3 段、0.635ms。

确定性数据块完整发布了两份 roster，但 finalizer 正文把全工件的
“3 次/0.635ms”与探索子窗的两条明细拼在一起，第三段
`34579.471372..34579.471722` 没有进入主体答案；同时把 typed
`非 IO D-state=0、io_wait=3` 叙述成“io_wait 内核又称 D 状态”。runner
的 principal-answer oracle 因而正确失败。

通用最优方案冻结为：

1. 只消费 `RuntimeArtifactScopeProfile`、请求窗 authority、query
   coverage/roster completeness 等 typed 字段，选出一个
   `principal requested scope`；
2. finalizer 权限面只把该范围的 complete roster 作为主值；嵌套探索窗保留
   为 supporting/drill-down，并明确标记，不能与主范围总量拼接；
3. `D-state`、`io_wait`、`S-state iowait` 继续按 typed state kind 分开，
   不新增同义词硬门；
4. 确定性数据块仍在正文后，但主范围先列、探索范围后列；
5. 不读 RawRequest、模型 thinking/final prose，不按此 case 时间戳或 caller
   名称拟合，不把系统权威块搬回答案前面。

**`EVAL-B19-FRAME1` 仍开放。** 显式窗为
`34579.472865..34579.587805`，模型却把窗外
`34579.595130 Choreographer#onVsync` 写成窗口结束证据；同一答案后部的
typed coverage 已明确
`frame_causality=unproven/frame_evidence_status=absent`。这是已有 typed
帧权限没有收敛到主体结论槽的生产见证，不能靠追加另一个并列答案块解决。

**`EVAL-TWODIM-2` 获得生产见证。** 投影中确有 44.836ms 的链上 sleep
真实占用，但用户可见的独立摘要只有“未计价占用 2 行/最大 44.836ms”，
没有主体、单次/聚族口径或时间窗；相比之下可消除收益已经有完整独立总览。
因此双轴要求仍是 partial：

- 轴 A：实际占时/关键路径候选，用来发现新修复方向；
- 轴 B：现有规则可消除收益，用来量化优先级反转、算力供给、已证 IO/唤醒
  等已知机制。

两轴必须并列可见、各守各的量纲和重叠纪律；不得把 raw occupancy 自动铸成
可消除量，也不得用可消除榜替代实际占时榜。无 typed frame/deadline 证据
时，轴 A 只能称“窗口主要占用/关键路径候选”，不能升级为“已证明丢帧
根因”。

下一施工顺序：

| 批次 | 内容 | 边界 |
|---|---|---|
| B19e | `SCOPEJOIN1`：requested-scope principal roster 选择与子窗降级 | 不改 projection gate，不改模型块顺序 |
| B19f | `TWODIM-2`：独立主要时间占用/关键路径候选面 | 复用现有 typed 占用与 span 聚族，不新造计价规则 |
| B19g | `FRAME1`：typed frame conclusion 槽融合 + ARITH2 不相加关系 | 不新增前置 authority，不扫描答案原文 |

状态：

| ID | 状态 |
|---|---|
| B19d user-first surfaces | covered：正负回放均确认顺序、词面与 projection gate |
| EVAL-B19-SCOPEJOIN1 | filed P1 / B19e next |
| EVAL-TWODIM-2 | implemented / full tests pass / same-pair replay next |
| EVAL-B19-FRAME1 | open P1 / production witness reconfirmed / B19g |
| EVAL-B19-ARITH2 | open P2 / 与 FRAME1 同批做 typed relation 融合 |

#### B19e：requested-scope principal wait roster 收敛

本批只修复 `EVAL-B19-SCOPEJOIN1`，不修改 trace 查询、projection gate、根因
排序、唤醒链、可消除量、自动补采或模型正文顺序。

实现：

1. `TraceTargetWaitSummaryAuthority` 增加内部 typed
   `RequestedScopeRole`：
   - 显式时间窗必须与 quote-anchored `time_start/time_end` 精确匹配；
   - full-artifact 的模型查询必须有**同一 result prefix** 的
     `runtime_artifact_scope_coverage`；
   - full-artifact 的确定性 supplement 只在当前 validated profile 也是
     full-artifact 时获得 principal；
   - 仅仅“窗口更大/包含另一个窗口”不能铸造主范围。
2. 一个 requested-scope principal 存在时，finalizer recap 只为它发布
   `principal_conclusion` 与完整逐段 occurrence 清单；探索子窗保留紧凑
   count/total，但明确为 supporting，不能替代主范围。
3. 确定性用户数据块同样把“请求主范围”排在“探索子范围”之前；两个范围都
   保留完整值，不删除证据。
4. typed state 种类在两面统一声明为独立口径：
   `D-state`、`io_wait`、`S-state IO wait` 不互相改名；尤其
   `d_state_occurrences=0` 时不得把 `io_wait` 行叙述为 D-state。
5. legacy 未分类范围保持旧行为；相同 roster 去重时 principal 角色优先，
   冲突 roster 继续 fail-closed。
6. 全过程不读取 RawRequest、模型 thinking/final prose，不新增答案
   rewrite/reject，不按时间戳、caller 或 case ID 拟合。

结构矩阵覆盖：

- full-artifact supplement principal + narrow exploration；
- 同 result unbounded-query coverage principal；
- unrelated coverage 不得给另一查询铸权；
- explicit requested window；
- finalizer 只给 principal 发布完整 occurrence rows；
- 用户数据块 principal-first；
- D/io_wait 分型文案。

完整回归：

- `go test ./internal/types ./internal/agent -count=1`：18.373s / 2.871s；
- `go test ./internal/tool -count=1`：167.975s。

状态：`implemented / full-tests-pass / same-pair replay after B19f`。为减少
真实 trace 回放成本，按冻结批次先完成 B19f 双轴结构，再用同一正负 pair
同时验收 scopejoin 与双轴；若 B19f 改动超出纯展示结构则拆开回放。

#### B19f：Trace 根因双轴结构闭环（TWODIM-2）

状态：`implemented / full-tests-pass / same-pair replay next`。

本批把用户再次裁定的两个性能根因维度做成并列、互不替代的决策面：

1. **主要时间占用 / 关键路径候选**回答“时间实际花在哪里、下一步应探索
   什么新修向”：
   - 复用最终 projection tree model 已完成降级、同段 twin 合并、同状态
     occurrence fold、窗域过滤后的真实行，以 `ImpactMS` 展示墙钟占用；
   - 目标线程 `running/runnable/sleep/S-state IO wait/non-IO
     D-state/io_wait` 继续使用同窗 typed 状态分区，不把等待症状直接计价；
   - deterministic semantic span 展示原始窗内墙钟；
   - business span 继续复用既有 `(tid, verbatim span name)` 双 TOP
     聚族，分别给出族累计、单次最长、次数和行区间，覆盖“单个长段”和
     “频繁小段累计”两种新修向；
   - 进程 CPU 占用复用 `WindowStats.CPUOccupancy.TopProcesses`，通过
     `cpu_occupancy_process` typed side channel 携带同窗进程/线程/CPU/
     core-class 清单，单位固定为 `cpu·ms`，与墙钟分组且不进入因果席位。
2. **窗内可消除量 / 根因排序**保持原实现：优先级反转、算力供给折算、
   已证 IO/唤醒依赖等继续按原规则、原值、原榜位和原重叠纪律发布。
3. 两轴只互相引用，不互改数：
   - 高占时但未被现有规则计价，标成“真实占时 / 新修向候选”，不铸造
     可消除收益；
   - 已计价席同时保留原始占时和“现规则可消 Xms 另见可消除榜”，两值
     不替代、不相加；
   - `ms` 与 `cpu·ms` 不混排求和；多核 CPU time 可超过墙钟窗口，只作
     资源背景，不自动证明关键路径。
4. 帧权限继续 fail-closed：本表明确声明自身不能证明某占用已导致具体
   丢帧；没有 target-bound frame/deadline 证据时只能称“所选窗主要占用 /
   关键路径候选”。`FRAME1` 的主体结论融合仍由 B19g 处理。
5. 用户可见顺序为：模型正文 → 每工件“主要时间占用 → Trace 因果投影
   （含原可消除榜）→ 代表窗 → 关键指标”→ 后续建议/精确数据/无损明细。
   无窗 focused fact 仍不生成 projection，因此也不会单独生成此因果双轴
   cluster。

第一次全量回归发现并修复了一个重要的二次判定风险：初稿直接遍历
`OnChainCauses/PrimaryRootCauses`，会把既有树模型已经降级到背景的
aggregate/unknown 行重新叫成关键路径，也会把既有同段 twin fold 再拆成
两行。最终实现删除该平行判定，轴 A 的路径/状态行只消费**同一个最终树
模型**的 chain/cause/depthless/self/semantic 行；背景降级、身份判定、合并
和窗域权威因此与原因果面完全同源。

typed 契约同步：

- `threads/top_thread/top_thread_ms/cpus/core_classes` 从 display-only
  note 升级为 projection compile 的 `TraceNoteKeyCPUOccupancy*` 常量和
  hard-consumer 登记；
- `CPUOccupancyProcesses` 登记到 projection 信息契约表，明确为 displayed
  non-seat side channel；
- 自动补采 value census 因新增一条 CPU 占用 typed 值记录，从 fixture 的
  `54/其他29` 如实演进为 `55/其他30`，中英文同值。

结构矩阵：

- long-single business span；
- many-small business span family（累计、最大单次、次数）；
- on-chain raw occupancy 但现规则计价为 0；
- 同一行 raw occupancy 与规则可消值分账；
- deterministic semantic span；
- 目标五类状态与 S-state IO refinement 分型；
- process CPU `cpu·ms` 与 wall-clock `ms` 隔离；
- no-span 负例不造 span 行；
- aggregate/unknown 背景降级不被抬升；
- same-segment twin fold 不被拆开；
- 中英文 frame-unproven 边界；
- 单工件、多工件顺序与原可消除榜值保持。

完整回归：

- `go test ./internal/types ./internal/agent -count=1`：19.782s / 2.967s；
- `go test ./internal/tool -count=1`：173.447s。

下一步仍按冻结顺序执行同一正负 pair 回放，同时验收 B19e 的主范围选择和
B19f 的双轴可见性；随后进入 B19g `FRAME1 + ARITH2`，不得通过用户关键词
或模型输出原文硬门实现。

#### B19f r1：双轴正例通过，无窗事实被误升级为因果报告（2026-08-01）

严格并行 2 个真实 Trace 用例，runner 1 PASS / 1 FAIL，人工 0 PASS / 2
FAIL。两类结果必须拆开判断。

显式窗 `trace_query_donghu_real_frame_multicausal` 证明 B19f 本体有效：

- `主要时间占用 / 关键路径候选` 已独立展示目标状态、链上 raw occupancy、
  semantic/business span 和 process CPU `cpu·ms`；
- 原 `Trace 因果投影`、根因排序、唤醒链、`◎ 窗内可消除量`、代表窗和
  system supplement 全部保留，窗口仍为
  `34579.472865..34579.587805`；
- raw 占时与规则可消值没有互相替代，B19f 结构目标判定为 covered。

但 human FAIL 重新确认 `EVAL-B19-FRAME1`：typed coverage 明确
`frame_causality=unproven/frame_evidence_status=absent`，主体仍断言
“114.94ms 的帧窗口未能完成”“16ms 帧预算超约 7 倍”；同时把 typed
优先级反转候选/依赖边扩大成“NetworkService 持有共享锁”。这些结论没有
frame deadline 或 lock-owner typed 证据，不能由后置 caveat 抵消。

同一正例还为 `EVAL-B19-ARITH2` 增加了通用账目关系 witness：占用表把
CookieMonsterCl 的 `sleep 47.282ms ×1` 与 `sleep 44.836ms ×6` 以同名同态
并列，却没有说明两者来自不同覆盖集/账目体系；目标状态分区的 sleep
84.358ms 与路径候选 76.599ms 也没有可见关系。它们不是可直接相加的独立
耗时。B19g 应复用 projection 已有 `账目关系` typed 判定，在双轴摘要中做
关系融合或择一主账，不能按数值近似或主体名称猜测。

无窗 `real_trace_c2_dstate_iowait` 暴露 `EVAL-B19-NOWINPROJ1`（P1）：

1. 用户 typed 范围是 `full_artifact`，目标只有
   `com.baidu.tieba-59566`，scenario=generic，diagnostic=false；问题要求
   “是否发生、何时、内核原因、总量”，不是根因诊断；
2. analyzer 却发出 `question_kind=call_chain + predicate_axis=call`，而
   `is_relational_lookup=false`；现有单 runtime-target consistency 允许
   `AxisCall` 单独放行，QuestionFamily 因而变成 `call_chain`；
3. `traceSupplementNarrowDStateQuestion` 被该 family 否决，系统以
   `no_typed_window` 为由 windowless 补跑 full-trace
   `root_cause_rank`，而不是原设计的最小 `window_stats`；
4. full report 权限同样把这个错误 call-chain 标签当最高权威，最终注入
   B19f 占用块和完整因果投影；报告同时携带全 trace
   `34579.450627..34579.595184` 与另一探索窗
   `34579.472865..34579.587805`，形成跨窗拼接；
5. 主体把正确的 3 段
   `0.138/0.147/0.350ms`、Σ`0.635ms` 写成
   `0.197/17.903/19.565ms`、Σ`37.665ms`。这是相同 scope-join 症状在
   错误报告扩张下的稳定复现，不按单次模型波动关闭。

最优修复拆成两层，均只消费 schema-validated typed 字段：

1. **B19g-a analyzer runtime relation consistency（已实现）**：一个 runtime
   target 的 `call_chain` 不能再由 `predicate_axis=call` 单独放行；必须有
   `is_relational_lookup=true`（例如“哪些 waker/caller 与该目标存在关系”）
   或至少两个 distinct runtime targets。单目标状态/时长/次数/原因问题应
   重试为 conditional/mechanism。源码两端点 call chain、两个 runtime
   target 关系、显式时间窗均保持。
2. **B19g-b publication defense（随后评估）**：如果 production replay 仍
   出现 analyzer 把事实查询误铸为 relational call chain，再增加独立 typed
   runtime request-purpose enum；不得从 RawRequest、analyzer keywords、
   thinking、closure reason 或 final prose 扫词硬门。不得用
   `full_artifact` 单独否决真正的无窗调用链。
3. **B19g-c FRAME1 + ARITH2**：把 frame authority 与账目 relation 融入
   一个用户结论槽；无 frame/deadline 不得升级帧结果，无 lock-owner row
   不得升级锁持有；不新增答案前置“系统权威”块，不扫描模型原文做硬门。

状态：

| ID | 状态 |
|---|---|
| EVAL-TWODIM-2 | covered：正例确认两轴可见且原 projection 能力无回归 |
| EVAL-B19-NOWINPROJ1 | implemented / full tests pass / same-pair replay next |
| EVAL-B19-SCOPEJOIN1 | partial：B19e typed 载体在场，主体仍未收敛；与 NOWINPROJ1 联合复放 |
| EVAL-B19-FRAME1 | open P1 / B19g-c |
| EVAL-B19-ARITH2 | open P2 / 新增双轴账目关系 witness / B19g-c |

下一批先提交这份 production witness，再落 B19g-a 并运行完整 types/tool
回归；随后仍按严格并行 2 个回放同一正负 pair。只有分类/发布扩张消失且
显式窗投影保持，才进入 FRAME1/ARITH2 融合，避免多变量同时变化。

##### B19g-a 实现与结构回归

实现位于 analyzer typed emit 边界，而不是答案层：

- `validateRuntimeArtifactCallChainConsistency` 对恰好一个 runtime target 的
  `call_chain` 要求 `predicates.is_relational_lookup=true`；
- `predicate_axis=call` 继续表达关系动作轴，但不再单独授予调用链/全因果
  报告权限；单目标状态、时长、次数、原因、压力或当前状态必须重试为
  conditional/mechanism；
- 单目标真实 waker/caller 关系可通过 relational predicate 保留；两个
  runtime endpoints 继续通过；不带 runtime target 的普通源码 call chain
  仍按原有 axis/endpoint 规则处理；
- analyzer skill 同步说明该合取，避免只依靠失败重试教学；
- 没有读取 RawRequest、analyzer keyword、模型 thinking/final prose，也没有
  修改 trace_query、projection、root rank、wakeup、可消除量或 supplement
  的实现。

回归：

- 定向 analyzer/tool：2.245s；
- `internal/skill`：0.718s；
- 完整 `internal/types`：19.442s；
- 完整 `internal/agent`：3.494s；
- 完整 `internal/tool`：169.531s；同 PID 多 target identity pin 加入后复跑
  166.517s。

状态：`implemented / full-tests-pass / same-pair replay next`。下一步先构建
同一 HEAD，并严格并行 2 个回放显式窗正例与 full-artifact D/IO 负例；若
模型在 retry 后仍把负例铸成 relational call chain，才启动 B19g-b 独立
runtime request-purpose enum，禁止继续叠加通用字段启发式。

##### B19g-a r1：分类已收敛，focused predicate 漏 explain+conditional

严格并行 2 个回放仍为 runner 1 PASS / 1 FAIL，但 B19g-a 的 analyzer 修复
已经命中：无窗 D/IO 用例从前轮 `trace/call_chain` 收敛为
`intent=explain + question_kind=conditional + predicate_axis=condition +
family=generic`，没有 analyzer retry/reject，说明不需要新增 request-purpose
enum。

主体答案已经正确发布：

- 3 段 `34579.451701/0.138ms`、`34579.452934/0.147ms`、
  `34579.471372/0.350ms`；
- Σ`0.635ms`，三段均 `iowait=1`；
- caller 均为 `sync_buffer_read_wi+0x60/0x11c[sysmgr.elf]`；
- `non-IO d_state=0` 与 `io_wait=3` 分型正确。

仍然出现 `root_cause_rank` system supplement、`主要时间占用` 和完整
`Trace 因果投影` 的原因是 `IsFocusedRuntimeFactQuestion` 的枚举不闭合：它
承认 trace/root_cause+conditional 与 explain+mechanism，却漏了同样合法的
非诊断 explain+conditional。QuestionFamily 已经 generic，但 supplement 和
report authority 仍把 `focused=false` 当成可扩大，随后被已收集 causal rows
反向授予 full-report 权限。

`B19g-a2` 已实现：

1. 非诊断 `IntentExplain ∧ (ReqMechanism ∨ conditional/condition) ∧
   runtime_target ∧ ¬call_relation` 统一成为 focused runtime fact；
2. 该 authority 同时驱动 supplement 窄视图和所有 full-report
   materializer；偶然收集到 root rows 也不能反向扩大问题；
3. deterministic principal value 继续允许；显式 typed 时间窗仍是最高权限，
   保留完整 projection/root/wakeup/eliminable/supplement；
4. 不读取 RawRequest、keywords、thinking、closure/final prose。

结构测试覆盖 explain+conditional family、report authority 即使有 root row
仍收窄、D-state supplement 只选 `window_stats`、principal-value 保留。
完整回归：types 19.800s、agent 3.037s、tool 172.272s。

本轮 runner 的无窗唯一失败是 eval 顺序假设：主体数值全部正确，但旧
`EXPECT_PRINCIPAL_MATCHES_TEXT_REGEX` 只接受“清单在前、count/Σ 在后”。
现拆成两个仍然严格的 principal 断言：三行+单位完整；count=3 与 Σ0.635
同现。二者顺序不再参与事实判定；没有放宽任一数值、caller 或分型要求。

状态：

| ID | 状态 |
|---|---|
| EVAL-B19-NOWINPROJ1 | B19g-a2 implemented / full tests pass / replay next |
| EVAL-B19-SCOPEJOIN1 | principal prose 已正确；等待无 full-report 回放后关闭 |
| EVAL-B19-ORACLE1 | covered：principal roster/count/Σ 精确且顺序无关 |
| EVAL-B19-FRAME1 | open P1；本轮模型改为“丢帧风险主导”，仍强于 unproven authority |
| EVAL-B19-ARITH2 | open P2；Cookie sleep 两套账仍无关系披露 |

下一步提交推送 B19g-a2，使用同一 pair 再回放一次；负例必须同时满足
`system supplement=window_stats`、`final_projection_blocks=0` 和主答案精确
roster，正例必须保持双轴与完整显式窗因果能力。

#### B18f r1：图兼容与显式窗非回归回放（2026-08-01）

严格并行 2 个回放均为 runner PASS / human PASS：

- `qf_type_relation_loop_controller`：270s，最终 typed roster 为
  `principal=12, auxiliary=3`，可见清单只列 12 个 production 实现；
  Mermaid 为合法 `flowchart TD`，没有 `codraxNode` 伪节点；
- `real_trace_h8_semantic_edge_anchor_sentinel`：135s，3 次
  `trace_query`、0 次 midloop。

LoopController 本轮模型原始提交已经使用合法
`LoopController -->|implements| implementation` 边，因此生产回放没有再次
触发 `<|--` 的兼容改写。不能把“最终图正确”夸大为“本轮运行命中改写分支”；
B18e 的真实坏图是 production witness，`Parent <|-- Child`、
`Child --|> Parent`、quoted label 与真正 `classDiagram` 的精确行为由
`internal/mermaidcompat` 结构测试覆盖。

H8 再次逐项确认显式窗能力未回归：

- 用户窗保持为 `34579.490000..34579.500000`；
- 根因排序主因仍为 NetworkService 的 5.951ms 有效归因；
- 唤醒链仍为
  `NetworkService -> CookieMonsterCl -> com.baidu.tieba-59566`；
- `◎ 窗内可消除量总览` 与 `trace-causal-projection` 均发布；
- `Trace 因果投影覆盖边界` 保留
  `frame_causality=unproven` 和 `enumeration_status=incomplete`；
- 成文前确定性 `frame_root_cause_bundle` 系统补采仍绑定同一个显式窗。

H8 的 `style=1` 只来自“值得注意的是”一次；运行日志明确写明
`answer style advisory (observation-only, never gates)`，不是答案缺证或
合同失败。Loop 的 270s/8 次 midloop 相比前轮偏高，但语义面正确，当前仅记
单次模型/探索成本波动，不为它新增硬门或单 case 拟合。

状态：

| ID | 状态 |
|---|---|
| EVAL-B18-DIAG1 | covered：production witness + exact structural fixtures + valid replay；本轮未命中 rewrite 分支 |
| H8 explicit-window non-regression | covered：B18f r1 runner/human PASS |
| B18 campaign | closed；下一批回到异构高优先级矩阵 |

### B19：异构高优先级批（真实 Trace × Git diff/current source）

#### B19 r0：runner 失败拆分与人工审计（2026-08-01）

严格并行 2 个用例，runner 结果为 0/2，但失败性质不同：

1. `trace_query_donghu_real_frame_multicausal` 在 0s LAUNCH_FAIL，没有生成
   result dir。case 的 `HTRACE_FILE="../../customlogs/xxx_all.systrace"`
   按仓库 cwd 解析到不存在的 `/Users/han/customlogs`；仓内
   `eval/fixtures/real_traces/donghu_tieba_frame.systrace` 与
   `/Users/han/opt/customlogs/xxx_all.systrace` 的 SHA-256 同为
   `f5d85dd9723d75c9121def4cbf20fc1ace8e2b6f35422cd794c9d8d68573d78d`。
   这是 eval fixture 路径漂移，不是 Trace 产品失败，也没有产出可审计答案。
2. `read_combo_git_two_diffs_current_code` 运行 120s；主体正确比较最新纯文档
   commit 与上一笔 Mermaid 代码 commit，并把代码改动落到当前
   `NormalizeSourceForMarkdown` 链路，但 runner 因系统追加
   “系统按已验证证据补充缺失成员”命中禁词而 FAIL。

Git 用例 human FAIL 还包括一个独立事实错误：grep 原始结果明确是
`50 matching lines` 且带 `result_truncated`，模型却写成“50 个文件”。
当前仓库实际为 30 个匹配行、7 个文件；排除定义和测试后有 6 个 production
调用点。工具原始头已经披露单位，故当前先登记为低优先级的计量语义/模型
波动；若异构用例复现，再统一补 typed unit 的 prompt projection 与软指导，
不对答案原文加数值或关键词硬门。

登记：

| ID | 优先级 | GAP | 状态 |
|---|---:|---|---|
| EVAL-B19-INF1 | P1 eval | 外部机器相对路径使真实 Trace case 无法启动 | implemented：改为仓内同 SHA fixture |
| EVAL-B19-HIST1 | P1 | typed `diff_clue + current_key_code` 未恢复 mixed history/current-code 权限，支持 member_set 被铸成 principal 补表 | implemented / full-tests-pass / replay next |
| EVAL-B19-GREP1 | P3 | matching-line count 被模型误述为 file count | filed / repeat-before-code |

#### B19a typed history/current-code authority convergence

最优方案只消费规范化 typed 字段：

1. `IsHistoryBackedCurrentCodeExplanation` 新增精确合取：
   `IsHistoryLookup ∧ IntentExplain ∧ required(diff_clue) ∧
   required(current_key_code)`；
2. 单独一个 `current_key_code` 角色不生效，避免 analyzer 偶发误标把纯历史或
   其它问题改道；Active `CurrentSourceExplanationProfile` 仍是更直接的
   mixed-lane 证明；
3. 即使 analyzer 保持 `question_kind=history`，这个维度对也会把 current
   source/VCS 混合 member-set 保持为 supporting coverage，不再生成强制
   principal enumeration supplement；
4. 不读取 dimension label/source quote、RawRequest、模型 thinking、closure
   reason 或最终答案文本；不新增 answer rejection；
5. Trace runtime family、显式时间窗、projection、root rank、wakeup、
   eliminable 与 supplement 路径均未修改。

同时把两个使用旧外部路径的东湖 case 改为仓内固定 fixture，消除开发机目录
结构依赖。

回归：

- `go test ./internal/types -count=1`：25.609s；
- `go test ./internal/tool -count=1`：163.734s。

状态：`implemented / full-tests-pass / same-pair replay next`。

#### B18d r1：candidate identity 收敛通过（2026-08-01）

严格并行 2 个回放均为 runner PASS / human PASS：

- called-by：85s。completion 记录
  `canonicalized 1 duplicate typed relation member row(s)`，最终只有一个
  2 行 caller 表；同一 caller 的 line 290/291 两个调用点保留在同一成员行
  的详情中；
- implements：131s。最终主列表与图仍为 12 个 production 类型，无重复
  aggregate carrier。

`EVAL-B18-AXIS1` 状态：`covered`。

### B19g-a2 r1：无窗口目标事实再次扩成全量报告（2026-08-01）

严格并行 2 个 Trace 用例，runner 为 1 PASS / 1 FAIL，人工均 FAIL：

- `trace_query_donghu_real_frame_multicausal`：runner PASS。显式
  `34579.472865..34579.587805` 窗仍完整保留主要时间占用、窗内可消除量、
  根因排序、唤醒链和 Trace 因果投影，证明 B19f 双轴与显式窗能力未退化；
  但模型开头把优先级反转/IO 候选写成“卡顿根因、直接原因、传导链”，系统
  尾部却明确 `frame_causality=unproven / frame_evidence_status=absent`，同一
  答案自相矛盾。登记 `EVAL-B19-CAUSAL1/P1`：typed causal authority 已有，
  但模型 principal block 没有按该权限降格候选措辞。
- `real_trace_c2_dstate_iowait`：runner/human FAIL。Analyzer 最终发出
  `intent=return_value + question_kind=return_value + predicate_axis=condition +
  scalar=true`，却把用户明确的 `com.baidu.tieba 59566` 只放进普通
  `entities`，`runtime_targets` 为空。系统随后从模型的 trace_query 游标和
  analyzer entity fallback 猜出目标，在模型选取的 19ms 窗补跑
  `root_cause_rank + critical_blocking_calls`，把普通状态事实扩成两份因果
  投影；正文也回退成错误的 2 次、0.285/0.351ms，而完整 artifact 权威值是
  3 次、0.635ms。

这不是 B19g-a 的 explain/conditional 分支再次失效，而是更上游的“目标存在性
只靠 prompt 约定”加上下游两处噪声扩权：`entities_fallback` 把普通字符串升级
为执行目标，答案层又把 entity + cursor + supplement success 合成为用户目标。

### B19g-b：运行时目标声明与无目标 fail-narrow

通用方案冻结如下：

1. `AnalysisIR v16` 新增必填 `runtime_target_profile`，闭集为
   `not_applicable / no_named_target / named_target / unspecified`；时间/范围继续
   由 `runtime_artifact_scope_profile` 独立承载，身份与时间窗不混为一个字段。
2. `named_target` 必须同时提供逐字锚定的当前请求 `source_quote` 和至少一个
   结构合法、`source=user_explicit` 的 `runtime_targets`；缺项 fail-loud，要求
   analyzer 同轮修正。`no_named_target/unspecified` 与 target rows 同时存在也
   fail-loud，禁止一边否认一边偷偷带目标。
3. 删除补齐层的 analyzer `entities` 目标恢复；答案数值 authority 同样不再把
   entity + 模型游标 + 补采成功合成为用户身份。普通 entity 只能继续作为软
   搜索提示。
4. 新增 `IsNarrowRuntimeArtifactFactShape`：非诊断的 typed scalar return-value
   即使 target 意外缺失也 fail-narrow，不能因模型探索到 root rows 而发布全量
   报告；既有 focused conditional/mechanism 复用同一入口。
5. 模型游标只可在 typed diagnostic/root-cause/call-relation 或用户显式时间窗
   下驱动自动补齐；普通无目标单值事实不得扩权。显式时间窗在报告 materialize
   和补齐 target 派生中仍是最高优先级，故系统窗内因果投影、根因排序、唤醒链、
   可消除量和自动补齐不受窄事实规则影响。
6. 所有硬决定只消费 schema 枚举、typed predicates/intent/diagnostic、
   quote-validated scope/target 和结构化 RuntimeTargets；不扫描用户题面关键词、
   模型 thinking/summary/final prose。`source_quote` 只做分析器主动提交值的逐字
   存在性校验，不从原文反向检索/猜目标。

专项覆盖：named 缺 target 拒绝、named quote 锚定正例、no-named 与 targets
冲突拒绝、普通 scalar+cursor 不补因果、同一 shape 加显式窗后 cursor 自动补齐
仍可用、entity/cursor/supplement consensus 不再铸答案目标、显式窗优先于 scalar
窄化、runtime profile schema/枚举闭包。

验证：

- `go test ./internal/agent ./internal/orchestrator ./internal/types ./internal/tool ./internal/skill`
  全部通过（agent 3.197s、orchestrator 13.079s，types/tool/skill 命中缓存）；
- 首次 `go test ./...` 暴露并修正了一条旧的 agent 单测期望：旧测试仍要求
  entity + cursor + supplement 合成用户目标，现已改为负例，确认不再扩权；
- 全仓还有两类与本批修改文件无关的既有基线失败：
  `internal/tracequery::TestNonEventPrioritySchemaPins` 的 schema hash 漂移，
  以及 `internal/tracediag::TestRunBerlinMagnitudeCoordinatesFixedPoint` 的科学计数法泄漏。
  两者不在 B19g-b 目标和 diff 内，不与本批混修；按独立 GAP 后续排序。

状态：`implemented / relevant-full-tests-pass / same-pair replay next`。

#### B19h-b r1：问题广度收口，因果结论权限成为下一主缺口

严格并行 2 个相同 Trace case，runner 2/2 PASS；人工为 1 PASS / 1 FAIL：

- 无用户时间窗的 D/IO case：analyzer 仍发出
  `intent=root_cause / scenario=root_cause / diagnostic=true`，但同时发出
  `runtime_question_profile=bounded_fact_set`。新 typed carrier 成功压过 legacy
  标签噪声；主答案只发布完整 3 段
  `0.138+0.147+0.350=0.635ms` 和内核 caller，没有 Trace 因果投影、根因榜、
  背景大盘。`EVAL-B19-FACTSET1` 转 covered。
- 显式 114.940ms 窗：主要时间占用/关键路径与现规则可消除量两轴均在；根因排序、
  唤醒链、代表窗、Trace 因果投影和系统自动补采均完整，证明 B19h-b 没有伤害
  显式窗能力。
- `EVAL-B19-CAUSAL1/P1` 第四次复现：typed finalizer guidance 和系统覆盖块均为
  `causal_conclusion=unproven / frame_evidence_status=absent`，model principal 仍写
  “直接根因”，并把“低优先级唤醒高优先级”本身当成优先级反转机理。该问题已经
  证明不是单次模型波动，下一批必须让系统发布的主结论消费 typed causal authority；
  不通过扫描/替换模型原文解决。

新增两个次级 typed-declaration gap：

1. `EVAL-B19-DECL1/P1`：同一 analyzer 在无窗 case 把明确的
   `com.baidu.tieba 59566 主线程` 声明成 `runtime_target_profile=no_named_target`；
   在显式窗 case 又把明确上下界声明成 `runtime_artifact_scope=full_artifact`。
   现有结构校验只能拒绝“已提供但非法”的 target/window，不能证明 analyzer 没有
   漏报用户声明。最优解需要请求解析阶段已有的 typed declaration witness 与
   analyzer profile 做一致性合取；禁止新增题面关键词 hard gate。
2. `EVAL-B19-NARROWCAVEAT1/P2`：窄事实主答案已正确收口，但末尾仍发布“部分补充
   钻取未执行”和“部分睡眠未定位唤醒者”两条全量因果族 caveat。它们不改变事实
   正确性，却把非请求覆盖义务带回窄答案。应让 caveat publisher 消费同一个
   `RuntimeTraceReportShapeAuthority`，保留范围/值完整性 caveat，过滤仅服务于
   全因果报告的缺口。

状态：`EVAL-B19-FACTSET1=covered`；`EVAL-B19-CAUSAL1=P1/in-progress-next`；
`EVAL-B19-SCHEDPROSE1=P1/open`（本轮最终答案未复现，但已有 production witness）；
`EVAL-B19-DECL1=P1/open`；`EVAL-B19-NARROWCAVEAT1=P2/open`。

#### B19h-c：typed 因果权限接管主结论

本批实现 `EVAL-B19-CAUSAL1` 的泛化修复，不扫描用户问题或模型答案原文：

- 发布条件只消费 `TraceEvidenceAuthority.causal_conclusion=unproven` 及系统已经构造出的
  非空 Trace 因果投影。没有真实投影时不制造结论；窄事实报告、
  `bounded_by_typed_rows` 和已证因果车道均不触发。
- 当 typed 因果权限为 unproven 时，系统生成的主结论成为首块；系统投影与全部系统
  补齐块保持原顺序。首版仅移除模型 `summary/principal`；r2 人工审计证明模型的
  supporting 时间线同样可能越权，现已收紧为该形下不发布任何模型拥有块。
- 主结论明确保留两个互不替代的分析维度：主要时间占用/关键路径候选用于探索新的
  优化方向；窗内可消除量用于按既有规则安排修复验证。候选排序和可消除量均不等于
  帧因果裁定。
- 单 trace 投影、多 artifact 投影和跨 artifact 对比表统一把主冠名降为
  “首要可消除候选（不等于已证帧因果）”；rank、effective impact、唤醒链、
  代表窗、证据索引和自动补齐数据均不删减。
- 新增正反结构测试：unproven + 非空系统投影必须接管整个发布面；bounded typed
  因果不得接管。`go test ./internal/tool -count=1` 全量通过
  （157.813s）。

状态：`implemented / internal-tool-full-tests-pass / same-pair replay next`。

#### B19h-c r1：主结论接管有效，发现系统投影内部残余与声明波动

严格并行 2 个相同 Trace case，runner 1/2 PASS；人工审计结论如下：

- 显式时间窗正例的人工作答主要求通过：答案第一块已由 typed authority 接管，明确
  `frame causation=unproven`，且主要时间占用/新修向与窗内可消除量/既有规则两轴、
  根因候选排序、唤醒链、代表窗、Trace 因果投影、系统自动补采均保持。
- 同一正例暴露 `EVAL-B19-CAUSAL1` 的系统内部三处残余：动态图例仍定义
  “主根因=已证链上候选”，逐节点明细仍标“因果位置:主根因(优先处理)”，下一步仍写
  “对主根因…下钻”。这不是模型原文，而是确定性投影的旧词面。已追加统一 typed
  分支：unproven 时分别改为候选榜定义、首要可消除候选位置、针对候选的验证建议；
  已证车道继续走原有词面。新增结构测试遍历所有发布块，禁止上述已证因果残留。
- r2 又发现模型 supporting 时间线写出“真正的瓶颈在于…”。这证明仅按
  `SurfacePrincipal` 删除模型块不能封闭因果权限；模型块的角色标签不保证其内部
  每句话都只作 supporting。最终方案不扫描模型原文：在
  `unproven + 非空系统投影` 下只发布系统拥有块。系统投影已完整承载两轴、排序、
  唤醒链、代表窗、逐节点明细、证据索引与自动补齐；已证/窄事实车道不受影响。
- 无窗有限事实 case runner 与人工均失败：analyzer 本轮发出
  `runtime_question_profile=causal_diagnosis`，虽然其 typed rationale 只描述“是否、
  时间、原因、总量”；紧邻上一轮同 case 则正确发出 `bounded_fact_set`。因此系统按
  当前 typed 声明合法展开了全因果报告，三段和 `0.635ms` 事实本身仍正确。
  该现象把 `EVAL-B19-DECL1` 从字段漏报扩展为 typed question-profile 稳定性问题；
  不通过扫描用户原文/模型 rationale 修补，需从结构化声明一致性与重试稳定性处理。

本轮后续修补的定向测试已通过；`internal/tool` 全量测试曾被表词面单源 tripwire
捕获新增徽章字面量，已改为复用 `tracefence.BadgeGlyphs()` 单源；第二次
`go test ./internal/tool -count=1` 全量通过（157.674s）。

r2 在内部词面修复后 runner 2/2 PASS：无窗 case 恢复 `bounded_fact_set` 并正确
收口，显式窗 case 的主结论、标题、图例、明细位置、下一步均服从 unproven。
人工审计捕获 supporting 时间线越权后，完整发布权限修补的定向测试与
`go test ./internal/tool -count=1` 再次全量通过（166.046s）。

状态：`EVAL-B19-CAUSAL1=implemented/full-tests-pass/final-replay-next`；
`EVAL-B19-DECL1=P1/open`；`EVAL-B19-FACTSET1=covered-but-declaration-unstable`。

#### B19h-c r3：因果发布权限闭环

最终同对回放 runner 2/2 PASS，人工结论为 1 PASS、1 PASS-with-open-gaps：

- 显式窗正例中，模型草稿仍在日志里虚构“持有关键锁/延迟传导”等确定机理，证明
  本批不是为一次措辞波动修补；最终答案中这些模型块已全部消失。用户可见面只剩
  typed canonical lead 与系统证据报告，且没有“真正瓶颈/直接根因/主根因=已证”残留。
- 主要时间占用/关键路径新修向、窗内可消除量既有规则、候选排序、唤醒关系、
  代表窗、完整投影、证据索引、目标身份和自动补齐均保留；答案前部没有
  “系统权威/系统权威主值”式内部权限标签。
- 无窗 case 再次发出 `bounded_fact_set` 并正确收口，确认 projection 权限修复不影响
  窄事实车道；r1 的相反结果已坐实 analyzer declaration 波动。

状态：`EVAL-B19-CAUSAL1=covered`；`EVAL-B19-FACTSET1=covered`（声明稳定性转
`EVAL-B19-DECL1`）；`EVAL-B19-SCHEDPROSE1=P1/open`；
`EVAL-B19-NARROWCAVEAT1=P2/open`。

#### B19h-d：有限等待事实的 typed 主结论与 caveat 范围

针对 r2/r3 的窄事实残余，采用同一 typed report-shape authority 完成两项泛化修复：

1. `bounded runtime fact + typed user target + complete target-wait roster` 形成系统主结论。
   结论直接发布请求范围、线程、完整段数、墙钟合计、非 IO D-state/io_wait/
   S 态 IO 等待/其他四个互斥统计分栏及 caller；明确这些分栏只说明
   `trace_query` 记账口径，不推导内核状态标签之间的包含或排斥关系。逐段时间、
   时长、iowait 与 caller 继续由现有完整明细块承载。
2. 上述窄车道不再发布“补充定位/钻取未执行”和“睡眠未定位上游唤醒者”两条只服务
   全因果调查的通用 caveat。判定只复用 `RuntimeTraceReportShapeAuthority`；定义位置、
   强制阅读、解析降级等其他 caveat 不受影响。显式时间窗仍有最高全报告权限，
   因果投影和相同两条 coverage disclosure 均保留。

该方案不扫描用户原文、模型答案或模型 rationale。完整 typed 清单存在时，模型拥有块
不进入窄事实最终答案；缺少/冲突/多窗无法选出请求主范围时 fail closed，仍保留模型
车道和已有数据边界，不猜造主值。定向测试通过；
`go test ./internal/tool ./internal/orchestrator -count=1` 全量通过
（tool 158.920s，orchestrator 12.486s）。

状态：`EVAL-B19-SCHEDPROSE1=implemented/replay-next`；
`EVAL-B19-NARROWCAVEAT1=implemented/replay-next`；显式窗同对负回归待跑。

#### B19h-d r1：有限等待事实与显式窗负回归均闭环

严格并行 2 个相同 Trace case，runner 2/2 PASS，人工 2/2 PASS：

- 无窗有限事实 case 的最终答案由 typed 完整等待清单接管：请求范围
  `34579.450627..34579.595184`、目标 `com.baidu.tieba-59566`、3 段、
  墙钟合计 `0.635ms`、caller
  `sync_buffer_read_wi+0x60/0x11c[sysmgr.elf]` 与逐段明细一致。D-state、
  io_wait、S 态 IO 等待和 other 以互斥统计分栏发布，并明确只代表本报告记账口径，
  不推导内核状态标签间的包含/排斥语义。
- 模型探索阶段把两次 `sched_switch` 时间差误算成 `19.671ms D-state`，但该值没有
  进入最终答案，证明 typed 完整清单已真正拥有主值权限，而非只增加提示词。
  最终答案也不再发布 Trace 因果投影、根因榜、全量钻取未执行或上游唤醒者缺失
  caveat；来源说明和输出维度核对没有扩张证据或结论。
- 同批显式 114.940ms 时间窗仍发布 typed unproven 主结论、主要时间占用/新修向与
  现规则可消除量双轴、板内根因候选排序、唤醒链、代表窗、Trace 因果投影、
  证据索引及系统自动补采。模型草稿里的确定性因果过 claim 被发布权限隔离，窄事实
  接管和 caveat 过滤没有外溢。
- analyzer 本轮为无窗 case 正确声明 `bounded_fact_set`，但跨历史回放仍出现过一次
  `causal_diagnosis`。`EVAL-B19-DECL1` 保留为 typed declaration 跨运行稳定性样本；
  当前证据符合模型波动，且系统已有 fail-closed 权限边界，暂不以题面关键词或模型
  rationale 扫描增加硬门。后续若跨 case 复现，再按结构化声明重试一致性统一处理。

证据：

- `eval/parallel_selected_summary_evalcampaign_b19hd_r1_20260801.md`；
- `eval/parallel_selected_summary_evalcampaign_b19hd_r1_20260801_manual_audit.md`；
- 结果目录时间戳 `20260801-043001`。

状态：`EVAL-B19-SCHEDPROSE1=covered`；`EVAL-B19-NARROWCAVEAT1=covered`；
显式时间窗因果能力负回归=`pass`；`EVAL-B19-DECL1=P1/open-model-variance-watch`。

### 后续 eval 维度扩展（用户追加，2026-08-01）

当前 Trace P0 收口并回放后，继续维持每批严格并行 2 个，按风险交叉覆盖：

| 顺位 | 模式维度 | 首选 case 形状 | 核心人工检查 |
|---:|---|---|---|
| 1 | write/apply 症状定位 | `github_issue_gson_lazy_number_symptom` + 一个跨语言写 case | 首计划机制、最小修改、负例测试、verify-only 不二次改码 |
| 2 | read 混合权威 | log/trace + current-code、history + current-code | 外部事实与当前源码边界、principal/supporting 权限、引用单位 |
| 3 | read 结构关系 | called-by / implements / references | typed roster、成员身份、scope/exclusion、关系图语法 |
| 4 | write/plan 隔离 | `patch_java_typo` + 一项 plan-only | worktree/risk/approval 不变量、计划不越权、无 apply 泄漏 |
| 5 | Trace 复杂根因 | supply/thermal、state churn、cross-trace | 双轴、时基/覆盖、根因凭证、性能与查询复用 |

模型单次措辞波动只登记样本，不据此增加硬门；只有跨 case/跨运行复现、或有 typed
权威冲突时才提升为施工项。每批都更新本节、manual audit 和统一 GAP 状态后再进入
下一批，避免未记档积压。

### B20 r1：跨语言 write/apply 症状定位（2026-08-01）

严格并行运行 Java Gson 与 Python dateutil 两个症状定位 case，runner 2/2 PASS，
人工 1 PASS / 1 FAIL：

- Gson：补丁只触及 `LazilyParsedNumber.java`，新增 value-based `equals/hashCode`，
  保留全部 Number 转换方法和测试文件。Maven/JDK 缺失后，项目声明的
  `make check` source oracle 真执行并覆盖唯一 changed path；final proof=`strong`，
  completion=`verified`，无第二计划或重复 apply。首次 plan 因冗余重叠 edit 被
  `old_text` guard 正确拒绝；模型修复时漏掉原 Java probe，但项目 runner 已补足，
  暂记过程波动。
- dateutil：生产 diff 对 fixture 的 4 个要求实际正确，人工在 applied tree 独立执行
  `python3 -m unittest discover -v` 为 4/4 PASS；产品本身却只执行一个覆盖 months 与
  non-integer 的局部 probe，已发现的 `python/unittest@.` 被记录为 `suite_skipped`。
  ChangeReport 明确给出
  `verification_probe_missing_soft_contract_ref`（含 `bc-float-years-apply`）和
  `verification_probe_baseline_not_run`；WriteFinalReport 同时发布 proof=`weak` 与
  completion=`verified/all_batches_verified`，形成 typed 权威自相矛盾。
- 日志进一步固定状态机链：controller 的 `finish/all_verified` 先被 truth ledger 改为
  `verify_batch/truth_ledger_weak_requires_proof`，随后 transition validator 因 workflow
  已 complete 又将其改回 `finish/workflow_already_complete`。因此这不是最终措辞问题，
  也不是 Python 特例，而是“probe-only 成功→弱 proof→完成态”这一整类写模式闭环缺口。

新增 GAP 与最优批次：

| ID | 优先级 | 系统 GAP | 泛化方案 | 状态 |
|---|---|---|---|---|
| `EVAL-B20-W1` | P0 | passed bounded probe 即使遗漏 planner 声明的 required 非 fallback 合同，也会跳过已检测到且有 test signal 的项目 suite | `run_tests` 只读 typed plan/probe refs/TestSurface：当 probe 未覆盖任一 required、非 `expected_outcome_fallback` 合同时继续项目 suite；fallback 文案缺口和 baseline-only 警告不单独触发昂贵 suite | implemented/full-tests-pass，待同 case 回放 |
| `EVAL-B20-W2` | P0 | `changeReportHasConcretePassedTestResult` 把 probe-only TestResult 当“具体本地测试”，过滤 proof follow-up；truth ledger 随后对 complete workflow 发出不可执行的 verify | 以 `ExecutedCommand` typed provenance 区分 project runner 与 `verification_probe`；probe-only 不得压掉 proof repair。若最终仍 weak，完成态不得铸 `verified`，应进入一次 bounded proof follow-up 或诚实 `accept_unverified` | implemented/full-tests-pass，待同 case 回放 |
| `EVAL-B20-W3` | P2 | `expects_baseline_failure=true` 但没有 baseline 结果；报告只告警，不能证明补丁前失败/补丁后修复 | 保留 typed warning；先观察其他 case，若跨 case 复现再设计一次性 baseline snapshot/probe，不以题型或语言硬门 | watch |
| `EVAL-B20-W4` | P2 | plan repair 时模型删除了原 Java verification probe | 本轮已有 strong project runner，不影响正确性；按模型过程波动留样，跨语言复现后再增强 repair pack 的结构字段保留提示 | model-variance-watch |

不变量：所有决定均基于 ChangePlan contracts、verification probe refs、TestSurface 与
ExecutedCommand/ChangeReport provenance；不扫描用户原文、模型 thinking 或最终答案。
写模式修复不得改变 read/Trace materializer，显式时间窗 Trace 因果投影及自动补齐
继续由既有负回归看护。

证据：

- `eval/parallel_selected_summary_evalcampaign_b20r1_20260801.md`；
- `eval/parallel_selected_summary_evalcampaign_b20r1_20260801_manual_audit.md`；
- 两个结果目录时间戳 `20260801-043649`。

#### B20-W1：required plan contract 决定 probe 后是否续跑 suite

`run_tests` 新增 typed continuation reason
`verification_probe_missing_plan_contract_ref`。通过的 bounded probe 只有覆盖全部
required、非 fallback 行为合同，才可跳过已经检测到且 `has_test_signal=true` 的项目
suite；缺一个就继续既有 TestSurface，让项目 runner 决定最终 pass/fail。

性能边界保持：`expected_outcome_fallback` 仅是请求摘要的兼容载体，缺失时仍只记
confidence telemetry，不触发项目套件；baseline-only 告警也不触发。本批不读合同正文、
用户请求或模型答案，只比较规范化 contract ID/source 与 probe `contract_refs`。

测试覆盖完整 probe 仍跳过、required analyzer contract 缺失时续跑且失败 suite 拥有
判决、fallback-only 缺失仍跳过三臂。定向测试和
`go test ./internal/tool -count=1` 全量通过（158.045s）。

状态：`EVAL-B20-W1=implemented/full-tests-pass/replay-next`；
`EVAL-B20-W2=implemented/full-tests-pass/replay-next`。

#### B20-W2：probe provenance 与完成态 truth 一致性

`changeReportHasConcretePassedTestResult` 现在要求当前格式的 ChangeReport 至少有一条
`outcome=executed` 且 runner 不是 `verification_probe` 的 typed 命令，才把已通过结果
视作可降级 proof warning 的项目测试。只有 bounded probe 的报告即使 TestResult 全绿，
也不能过滤带 contract/symbol ref 的 proof follow-up；无 `ExecutedCommands` 的旧持久化
报告保留原 score fallback，避免破坏 resume 兼容。

完成态修正为两级闭环：有 typed ref 的弱 proof 仍只追加一次 bounded proof batch；该批
已请求且仍无可行动 proof 时，truth ledger 不再发出 complete 状态下不可执行的
`verify_batch`，而是发出 `finish/accept_unverified`。在聚合完成 verdict 前，活动批最后一条
passed verify attempt 和 batch completion 同步降为 `unverified`，防止
`ApplyWorkflowDecisionToRun` 再从旧 passed attempt 恢复 `all_batches_verified`。这只改变
证据强度，不把 proof 缺失误判成代码失败，也不触发重复 replan/apply。

测试覆盖：真实 project runner 通过时 soft proof 继续作为 telemetry；probe-only 通过但
contract ref 缺失时追加一次 proof batch；已用尽 follow-up 后不递归且最终 verdict 保持
unverified；纯 truth-ledger complete fallback 同样固定 `accept_unverified`。完整回归
`go test ./internal/orchestrator ./internal/writeflow ./internal/types -count=1` 通过
（orchestrator 11.209s、writeflow 0.517s、types 17.384s）。

不变量：全部决策只消费 plan/report/workflow 的 typed provenance；没有读取用户原文或
模型输出，也未触及 read/Trace 查询、显式时间窗、因果投影、根因排序、唤醒链、窗内
可消除量和自动补齐代码。

状态：`EVAL-B20-W2=implemented/full-tests-pass/replay-next`。

### B20 r2：W1/W2 回放通过，终态呈现暴露独立 GAP（2026-08-01）

同一 `e66739944` 二进制快照严格并行 2 个原 case，runner 2/2 PASS，人工
1 PASS / 1 FAIL：

1. dateutil 明确记录 probe 通过后
   `continuing to project suite reason=verification_probe_missing_plan_contract_ref`，
   随后真实执行 `python3 -m unittest discover -v`，4/4 PASS；ChangeReport
   `caliber=project_runner`，无 confidence warning，WriteFinalReport
   proof=`strong`、completion=`verified/all_batches_verified`。独立 applied-tree
   回放同样 4/4 PASS。`EVAL-B20-W1=covered`。
2. Gson 的一文件补丁正确，独立 `make check` source oracle 通过；但本机无 JDK，
   Java behavior probe 为 `runner_missing`，Maven 也缺失。累计 proof 合理保持
   `weak`，第二批和总 workflow 均为
   `unverified/verification_proof_incomplete`，没有再次被 passed ChangeReport 洗成
   verified。`EVAL-B20-W2=covered`，且这是它的 fail-closed 正臂。
3. Gson 的用户可见 stdout 却只追加了两张同名“测试通过”卡，末尾没有发布最终
   `unverified`、缺失 JDK 或 proof incomplete；也没有区分初验与 cumulative review。
   机器 runner PASS 只检查补丁，不会捕获这一 typed final-report 与用户结果矛盾。

新增通用 GAP：

| ID | 优先级 | 系统 GAP | 泛化方案 | 状态 |
|---|---|---|---|---|
| `EVAL-B20-W5` | P0 | StageVerify 按局部 ChangeReport 逐次追加“测试通过”，而 ActionFinish 仅在 Result 为空时写 completion；已有 apply/verify 文本时最终 typed workflow verdict 完全不出厂 | 在所有 write terminal completion choke point，以 `WriteWorkflowRun.Completion` 和 batch attempts 生成一张简洁终态卡并追加/设置；verified、unverified、accepted_failed 分席，unverified 区分 no-tests、runner unavailable、proof incomplete。不得扫描或改写已有 stdout/model prose | implemented/full-tests-pass，待回放 |

W5 是终态权威发布，不改变测试判决：Gson 在 source oracle 通过但 Java runtime
behavior 未执行时仍应是 unverified，不能为了让表面“更绿”把 source shape check
升级成行为证明。实现只消费 typed workflow completion/attempt reason，不读取请求、
模型输出或 case/语言；也不触及 read/Trace 的显式时间窗、因果投影、自动补齐。

证据：

- `eval/parallel_selected_summary_evalcampaign_b20r2_20260801.md`；
- `eval/parallel_selected_summary_evalcampaign_b20r2_20260801_manual_audit.md`；
- 结果目录时间戳 `20260801-050919`。

状态：`EVAL-B20-W1=covered`；`EVAL-B20-W2=covered`；
`EVAL-B20-W5=implemented/full-tests-pass/replay-next`。

#### B20-W5：write workflow 终态权威卡

所有 apply workflow completion choke point 现在统一调用
`publishWriteWorkflowCompletionResult`：已有 apply/verify 内容原样保留，随后从
`WriteWorkflowRun.Completion` 和每批 completion reason 追加唯一终态卡；没有既有结果
时才使用原 completion fallback。合并过程不读取或匹配已有 prose，只做顺序拼接。

终态按三席发布：`verified` 明确所有批次完成验证；`unverified` 列出未闭合 batch 与
typed reason，并分别解释 `verification_proof_incomplete`、`no_tests`、
runner/parser unavailable；`accepted_failed` 不伪装成成功。这样局部“测试通过”仍是
真实 stage observation，但不会再成为用户看到的最后权威结论。中英文均有固定正臂。

结构上新逻辑放在独立 `write_workflow_terminal_render.go`，没有扩大
`write_verify_render.go` 的 hot-file 行预算。专项测试固定“已有测试通过卡 + 最终 proof
incomplete”顺序、英文 runner-missing 文案及正常 verified completion；
`go test ./internal/orchestrator ./internal/writeflow ./internal/types -count=1` 全部通过
（orchestrator 11.448s、writeflow 0.866s、types 17.138s）。

状态：`EVAL-B20-W5=implemented/full-tests-pass/replay-next`。

### B20 r3：write 终态权威回放闭环（2026-08-01）

在 `main@18174ef4d` 的同一二进制快照下，严格并行回放 Gson 与 dateutil 两个
write/apply case，runner 均 PASS，人工审计均 PASS：

1. Gson 的一文件 `equals`/`hashCode` 补丁和 `make check` source oracle 通过；Java
   runner 与 Maven 不可用，因此累计 proof 保持 weak。stdout 最后一块现在是
   “最终交付状态：未完全验证”，精确列出 `batch-1-cumulative-review` 和
   `verification_proof_incomplete`，与 final JSON 一致；此前两张局部“测试通过”卡
   不再占据最终权威位置。
2. dateutil 的生产补丁正确，未修改的项目 suite 实跑 4/4 PASS；模型新写的三个
   Python probe 各多一个右括号，被确定性归类为
   `verification_probe_syntax_error`。最终同样诚实发布未完全验证及同一 typed reason，
   没有把项目 suite 成功夸大成完整合同验证。
3. dateutil probe 的语法错误属于模型过程波动，产品已 fail-closed 且未误伤正确
   patch；本轮不新增语言、case、异常字符或答案原文硬门，仅作为 `EVAL-B20-W4`
   的第二个观察样本保留。

证据：

- `eval/parallel_selected_summary_evalcampaign_b20r3_20260801.md`；
- `eval/parallel_selected_summary_evalcampaign_b20r3_20260801_manual_audit.md`；
- 结果目录时间戳 `20260801-052638`。

状态：`EVAL-B20-W1=covered`；`EVAL-B20-W2=covered`；
`EVAL-B20-W5=covered`；`EVAL-B20-W4=model-variance-watch`。下一优先批转向读模式
混合 authority，避免继续围绕两种语言和单一补丁形状过拟合。

### B21 r1：读模式 mixed authority 机器双 PASS、人工双 FAIL（2026-08-01）

在 `main@766e2f715` 的同一二进制快照下严格并行 2 个读模式 case：

- `read_combo_log_current_source_bucketed_units`：runner PASS，134s；
- `read_combo_git_current_source_explanation`：runner PASS，219s；
- 人工审计均 FAIL。

日志用例保留了 `concurrent map writes` 三个 artifact-local 栈帧，也真实读取了
当前 `RankGraph`、`RankGraphScoresWithOptions` 和 `buildTaskMapData`。但本轮没有
artifact build revision、版本映射、Git diff 或变更 commit，模型仅凭旧栈行号漂移
和当前路径不同，就断言“旧版直接调用该实现、当前已经修复”。当前源码只能证明
当前 checkout 的路径和风险边界，不能证明历史工件来自哪个 revision、是哪次变更
修复。最终 decision 还携带 `current_status_verdict=still_present`，渲染成“仍然存在”，
而同一段 rationale 又称“风险已消除”，机器 oracle 未覆盖这类 typed verdict
一致性与跨来源 transition authority。

Git 用例正确找到最近 merge `2a58a60d` 及主题；但 `git_show --stat` 的
`exact_changed_paths` 明确有 3 个文件，模型 handoff 却把“受影响的测试文件”写成
2 个并漏掉 `internal/tool/test_surface_test.go`。更上游的 route 已正确发出
`current_source=required`，analyzer 却保持 `question_kind=history /
scenario=architecture_explain` 且遗漏 `CurrentSourceExplanationProfile` 和
`diff_clue + current_key_code` 维度对。现有 `IsHistoryBackedCurrentCodeExplanation`
把 `ReqHistory` 提前判成纯历史，未消费正确 route，B19-HIST1 的系统补表因而复发。

登记与批次：

| ID | 优先级 | GAP | 泛化方案 | 状态 |
|---|---:|---|---|---|
| `EVAL-B21-MIX1` | P0 | route 已 typed 声明 current source required，但 history+architecture analyzer 漏 profile 时 mixed lane 仍丢失 | 在分析归一化铸造 route-backed typed obligation：只接受 `history lookup ∧ explain/trace ∧ architecture_explain ∧ route current_source=required`，排除 scalar/count/relation/diagnostic/enumerate；让统一 history/current-code 谓词消费该信号。不得读取 route reason、RawRequest、答案 prose | covered；B21 r2 生产信号/混合证据链正证 |
| `EVAL-B21-VCS1` | P1 | `exact_changed_paths` 只存在 blob/summary 文本，`ToolVCSHistory` 只有 commits；下游无法 typed 检测漏文件 | 为 `git_log`/`git_show` 发布有界 per-commit changed-path roster、total/complete/omitted；ledger/prompt 使用 carrier，完整路径清册不得由 `--stat` 缩写或模型计数代替 | partial；普通 commit 8/8 正证，merge 父边由 B21-B2 修复 |
| `EVAL-B21-TRANS1` | P1 | runtime artifact 与 current checkout 同时有证据，却没有“历史变化是否已证明”的独立 authority | 建立 typed artifact↔checkout transition authority；无 artifact revision/version mapping 或 VCS transition witness 时只允许说明当前差异，确定性披露 historical transition=`unproven`，不改写模型正文 | Batch B21-C |
| `EVAL-B21-M1` | P3 | 模型把 mutex 保护路径描述成会因两个调用者并发而 crash，并选错 current-status enum | 先由 MIX/TRANS authority 限定可声称范围；作为 model-variance 样本保留，跨 case 复现再设计结构化 side-specific proof，不加 Go/mutex/答案关键词门 | watch |

不变量：上述三批不得修改 Trace family/query、显式时间窗 full-report、根因排序、
wakeup chain、窗内可消除量、因果投影、系统补采或任何基于用户/模型原文的 hard
gate；日志与 VCS 坐标继续分席，route 元数据只决定证据义务，不成为答案事实。

证据：

- `eval/parallel_selected_summary_evalcampaign_b21r1_20260801.md`；
- `eval/parallel_selected_summary_evalcampaign_b21r1_20260801_manual_audit.md`；
- 结果目录时间戳 `20260801-053446`。

#### B21-A：route-backed history/current-code authority

`EVAL-B21-MIX1` 已按双 producer 合取施工：

1. 新 carrier `route_backed_history_explanation` 不存 route reason、请求 quote、
   dimension label 或模型 prose，只记录 typed obligation kind；
2. 铸造条件同时要求 router 的闭集枚举 `current_source=required`，以及 analyzer 的
   `history lookup + explain/trace + architecture_explain`；scalar、role lookup、count、
   relation、diagnostic、enumerate、change-impact、field/runtime value、diagram 和多 bucket
   全部排除；
3. `IsHistoryBackedCurrentCodeExplanation` 在旧 `ReqHistory` 纯历史 carve 之前消费该
   carrier。普通纯历史、optional route 和单一 commit 标量保持原行为；
4. 生产接线测试从 `EmitAnalysis.Execute` 输入 route hint 与 analyzer payload，固定
   signal 铸造、summary 计数和 mixed predicate 三个端点，避免只有 helper 测试而挂点
   可删除；
5. 该 carrier 只恢复证据义务和答案形状，不成为 VCS/current-source 事实，不生成
   citation，也不改写已有答案。

验证：`go test ./internal/types ./internal/tool -count=1` 全包通过（types 21.064s、
tool 159.840s）；新增生产接线/类型专项通过；`go test ./internal/agent
./internal/orchestrator -count=1` 通过（2.754s/12.900s）；`git diff --check` 通过。

状态：`EVAL-B21-MIX1=implemented/full-tests-pass/replay-with-B21-B`。不单独消耗一次
模型回放；先补 VCS typed changed-path carrier，再用同一 Git case 验证两项，避免在
同一题面连续拟合。

#### B21-B：exact VCS changed-path authority

`EVAL-B21-VCS1` 已按工具真值载体施工：

1. `ToolVCSHistory` 增加逐 commit 的 `ChangedPathSets`，每组保留 full commit/ref、
   exact paths、`total/complete`、不可用原因及被 commit cap 省略的组数。路径来自独立
   `git show --name-only`，不从可能含 `...` 的 `--stat` 文本反解析；20 commits、
   每 commit 24 paths 的既有有界策略继续保留并显式披露边界。
2. `git_log --stat/--name-only` 和 `git_show --stat/--name-only` 只采集一次 exact
   roster，同一结果同时用于人类可读 summary 与 typed carrier，避免两条查询路径漂移。
   `git_show --no-patch` 发布 metadata-only carrier；默认 patch 在尚无 typed patch-body
   carrier 前故意保持 `VCSHistory=nil`，让现有 ledger 继续从 origin banner 编译
   `vcs_metadata + vcs_diff`，防止新 carrier 遮蔽补丁证据。
3. Observation Ledger 从 carrier 铸造独立 `origin=vcs_diff / predicate=changed_paths`
   记录，并携带 `Enumeration{emitted,total,total_known,reason}` 与 exact path
   `SurfaceTerms`。完整性与路径清册因此不再依赖 blob、summary 截断或模型自行计数。
4. finalizer prompt 增加 typed changed-path authority：只有模型声称该 commit 的变更文件
   数量/列表时，才要求保持 carrier 的精确清册；当前源码相关性仍是独立 current-source
   claim。这里是 typed 软约束，不扫描用户请求、模型 thinking/final，不增加 answer
   hard gate，也不重新注入“系统权威/系统权威主值”确定性块。
5. 测试固定 `git_show` 三种 typed 模式、默认 patch 兼容臂、`git_log` exact roster、
   carrier→ledger enumeration 以及 ledger→finalizer prompt 的三路径生产接线。

验证：定向 tool/types/agent 测试通过；`go test ./internal/types ./internal/agent
./internal/tool -count=1` 全包通过（types 18.792s、agent 2.876s、tool 162.628s）；
`git diff --check` 通过。

状态：`EVAL-B21-VCS1=implemented/full-tests-pass`；下一步重建同一二进制并严格并行
2 个 Git/current-source 读模式 case，同时回放 B21-A/B，不用单一原题作为唯一正证。
显式 Trace 时间窗、根因排序、唤醒链、窗内可消除量、因果投影和系统自动补采未改。

#### B21 r2：mixed lane 恢复，merge 父边与 citation 身份暴露（2026-08-01）

在 `main@6b1d5156b` 同一二进制快照下严格并行 2 个读模式 case：

- `read_combo_git_current_source_explanation`：runner PASS，113s，人工 FAIL；
- `read_combo_git_diff_hunk_current_code`：runner FAIL，169s，人工 FAIL。

本轮先确认两个正证：

1. 最近 merge 用例由 router 的 `current_source=required` 与 analyzer 的
   `history_lookup + explain + architecture_explain` 铸出
   `current_source_obligation_signals=1`；当前源码读取和 VCS 历史同时在场，旧的
   “系统按已验证证据补充缺失成员”表没有复发。因此 `EVAL-B21-MIX1=covered`。
2. 普通最新 commit `6b1d5156...` 的 typed authority 精确携带 8/8 paths，且 finalizer
   逐项收到全部文件，证明 B21-B 的 carrier→ledger→prompt 主链有效。

但 merge 用例暴露 P0 真值错误：`git_log --merges --first-parent --stat` 可见最新 merge
`2a58a60d...` 实际改变 3 个文件，而独立 `git show --format= --name-only <merge>` 默认
不选择 merge 父边，返回空输出；collector 随即把它铸成
`emitted=0/total=0/complete=true`。这不是模型漏抄，而是 producer 发布了错误 typed
authority。模型在空 authority 下又把当前 checkout 中无关的后续测试混成“新增四个测试
场景”，并一面说 test helper 直接影响 production candidate construction，一面说不影响
生产路径，故机器 PASS 不能算人工正确。

第二例的 8/8 roster 正确，最终也分别写出 diff hunk、当前源码依据、作用和边界；runner
只因这些内容位于相邻段落而不是同一行，未匹配单行 regex。这是 eval oracle 的 P3
假阴性，不应推动生产答案硬门。但答案的 principal scalar
`observationRecordForVCSChangedPaths` 借用了 `ToolVCSChangedPathSet` 定义的 citation，
且把仅适用于 `git_show` 默认 patch 的 `VCSHistory=nil` 边界扩写成 `git_log/git_show`，
人工仍判 FAIL。

新增台账：

| ID | 优先级 | GAP | 泛化方案 | 状态 |
|---|---:|---|---|---|
| `EVAL-B21-MERGE1` | P0 | merge commit 的 exact changed-path producer 未选父边，却发布 complete zero | 单 commit 清册统一使用 first-parent diff；普通单父提交语义不变。真实 no-ff merge 固定红/绿，不按 commit id 或题面识别 merge | B21-B2 已施工 |
| `EVAL-B21-CIT1` | P1 | principal scalar 的 visible symbol 与 citation anchor identity 可不一致，清理器仍保留错误引用 | 从 block/item 的 typed claim form 与 evidence/citation anchor 构造 symbol identity 校验或精确重绑；无法唯一匹配则去引用/软披露。不得扫描自然语言段落 | implemented/full-tests-pass；replay-next |
| `EVAL-B21-E1` | P3 | read-combo regex 要求 diff/current-source 词面同一行，分段等价答案假 FAIL | eval-only oracle 改为两个独立语义存在臂或结构化 facet；不修改生产 finalizer，不扫描生产答案做 gate | filed-low |

证据：

- `eval/parallel_selected_summary_evalcampaign_b21r2_20260801.md`；
- `eval/parallel_selected_summary_evalcampaign_b21r2_20260801_manual_audit.md`；
- 结果目录时间戳 `20260801-060743`。

#### B21-B2：merge first-parent exact roster

`gitChangedPathsForCommit` 现统一调用
`git show --format= --name-only --no-ext-diff --first-parent <commit>`。first-parent 对普通
单父 commit 与原语义等价；对 merge 明确选择“相对第一父、最终落地主线”的文件集，
不再把未指定 merge diff format 的空输出误铸成 measured zero。

新增真实临时 Git 仓库测试：feature branch 新增 `feature.txt`，主分支执行
`merge --no-ff`，collector 必须发布单一 set、`paths=[feature.txt] / total=1 /
complete=true`。该测试在旧命令下得到空清册，在新命令下通过；原有 GitShow/GitLog
定向测试同时通过。`go test ./internal/tool -count=1` 全包通过（159.400s），
`git diff --check` 通过。

状态：`EVAL-B21-MERGE1=implemented/full-tests-pass/replay-next`；
`EVAL-B21-VCS1=implemented，待 merge 回放正证`。本批仍未修改任何 Trace family、查询、
显式窗 authority、因果投影、根因/唤醒/可消除计算或系统补采逻辑。

#### B21-CIT：scalar code-identity citation alignment

`EVAL-B21-CIT1` 已在统一 pre-emit normalization 链施工：

1. scalar 与 list/table 的可见载体不同：scalar 的事实在 `block.Text`，
   `items[]` 只是 citation anchor，因此原有 item-label alignment 永远看不到 principal
   scalar 的符号。本批增加独立但复用同一 evidence endpoint 匹配器的 scalar 消费面。
2. 适用条件是精确 typed 合取：`kind=scalar`，claim form 属于
   `definition/call/guard/assignment/return/import`，且 `block.Text` 是含 ASCII 字母的单一
   code identity。数值 literal、external observation、absence、precedence、text reference
   与含空格自由文本全部不进入此 lane。
3. 当前 citation 已与 evidence subject/object/anchor/owner 或 source-verified quote 对齐时
   保持；只有一个 grounded candidate 时重绑到该 file:line；当前 endpoint 已被证明不符
   且候选多义时移除错误 ref，并复用既有 typed detached-citation disclosure。证据不足时
   fail-open，不猜第一个候选。
4. 生产接线测试从完整 `normalizeAnswerDocumentForPreEmit` 入口固定
   `observationRecordForVCSChangedPaths` 从错误类型定义引用重绑到它自己的 call evidence；
   另有双候选 detach、numeric/external/absence/free-text 四个负臂，避免扩成所有 scalar
   的文本匹配器。
5. 判定不读取 RawRequest、route reason、模型 thinking/final 段落或关键字；只消费
   schema 字段 `kind/text/claim_uses/citation_ref` 与 typed evidence endpoint。它是可恢复
   normalization，不新增 answer hard reject。

验证：专项测试通过；`go test ./internal/tool -count=1` 全包通过（160.861s）；
`git diff --check` 通过。

状态：`EVAL-B21-CIT1=implemented/full-tests-pass/replay-next`。Trace 显式窗口及
因果投影、根因排序、唤醒链、窗内可消除量、系统自动补齐均未改。

### B21 r3：merge/path/citation 回放通过，首项选择权威缺失（2026-08-01）

在 `main@e34510ffb` 同一二进制快照下严格并行 2 个读模式 case：

- `read_combo_git_current_source_explanation`：runner FAIL，177s，人工 FAIL；
- `read_combo_git_diff_hunk_current_code`：runner PASS，191s，人工 PASS。

已闭环的生产链路：

1. merge 用例的 `ToolVCSHistory` 现在把最新 merge `2a58a60d...` 发布为
   `emitted=3 / total=3 / complete=true`，三个 exact paths 全部进入 ledger 和
   finalizer typed authority；`EVAL-B21-MERGE1`、`EVAL-B21-VCS1` 的 merge 正臂通过。
2. 普通最新 commit 用例正确解释 `e34510ff...` 的 diff hunk、当前源码调用链、作用与
   边界；所有 scalar code identity 均引用各自函数 endpoint，
   `EVAL-B21-CIT1` 回放通过。
3. 第二例 runner PASS、人工 PASS；第一例 runner 的单行 regex 仍有 P3 形状问题，但
   本轮人工失败有独立真实原因，不能只归咎于 oracle。

第一例的 Explorer 在第 2 轮明确识别“最近一次 merge 是 `2a58a60d`，修改 3 个测试
文件”，却在下一轮自行裁定它“只是 test fix”，把前一 merge `ab6f9cba` 改称
“真正的功能特性”，最终整份 current-source 解释围绕后者展开。工具有稳定有序结果，
changed-path authority 也正确，但系统没有独立表达“请求选择的是有序结果第 1 项”；
因此模型把用户的选择标准从 recent 改成 substantive。末尾再次出现的
“系统按已验证证据补充缺失成员”表，是错误 principal target 对后续聚合事实的级联，
不是 merge 清册 producer 回退。

新增台账：

| ID | 优先级 | GAP | 泛化方案 | 状态 |
|---|---:|---|---|---|
| `EVAL-B21-ORD1` | P0 | 请求要求有序历史中的单一首项，但 AnalysisIR 没有选择序数/cardinality authority；模型可在拿到正确 ordered tool result 后自行跳项 | 先复用或补充 typed `selection_profile`（single first/latest、single last/earliest、top-N、range、unspecified），与承载 query order/filters 的 `ToolVCSHistory` 合取后铸造 principal selection；finalizer 只把 selected row 作为主目标，其余为上下文。不得扫描 RawRequest、模型 thinking/final、commit message 或“重要/功能”等词面 | P0/open，B21-D |
| `EVAL-B21-E1` | P3 | 单行 regex 不能识别跨段落满足 diff/current-source 两个维度 | 仅调整 eval oracle 为独立 facet/typed presence；不改生产答案 | filed-low |

施工约束：优先检查现有 request schema 是否已有 cardinality/order/selection 载体；若已有则
只补消费面，若没有才扩闭集 enum。selection 只约束“哪个 typed row 是 principal”，不把
commit 主题、文件类型或模型判断硬编码为价值标准；不生成新的“系统权威”前置块。
显式 Trace 时间窗的因果投影、根因排序、唤醒链、窗内可消除量与自动补采保持原权限。

证据：

- `eval/parallel_selected_summary_evalcampaign_b21r3_20260801.md`；
- `eval/parallel_selected_summary_evalcampaign_b21r3_20260801_manual_audit.md`；
- 结果目录时间戳 `20260801-062943`。

状态：`EVAL-B21-MERGE1=covered`；`EVAL-B21-VCS1=covered`；
`EVAL-B21-CIT1=covered`；`EVAL-B21-ORD1=P0/open`。

#### B21-D：typed history selection × ordered VCS result

`EVAL-B21-ORD1` 已按请求权威与工具真值合取施工：

1. `AnalysisIR v18` 新增必填 `history_selection_profile`，把历史证据来源与选择形状
   正交拆开。闭集 mode 为 `latest_one / earliest_one / recent_n / oldest_n /
   bounded_range / unspecified / not_applicable`，item kind 为
   `commit / merge / non_merge / matching_commit / unspecified / not_applicable`；具体选择要求
   exact current-request `source_quote`，但下游从不读取 quote 语义。
2. `git_log` 的 `ToolVCSHistory` 现在携带工具真实参数：`query_order/query_limit /
   merges_only/no_merges/first_parent`。commit 顺序仍来自独立 structured Git 查询，不从
   summary、commit subject、`--stat` 或答案文本反解析。
3. Explorer 在工具调用前收到 typed selection：merge 选择指导
   `merges_only=true + first_parent=true`，并要求最小充分 count；无论窗口多宽，都禁止按
   subject、patch size、文件角色或主观“重要性”跳过已选序数。
4. Finalizer 只在 request profile 与 compatible typed `git_log` 顺序/过滤器一致时铸造
   `VCSHistorySelectionAuthority`。`latest_one + merge` 即使工具查 5 条，也只把第 1 条
   commit 作为 principal，其余仅作 context；若无匹配工具结果则发布 prompt 内
   `unproven` 边界，不猜一个目标。
5. 这是 prompt authority，不是答案硬门：不扫描 RawRequest、模型 thinking/reason/final，
   不扫描 commit message，不确定性改写正文，也不注入用户可见“系统权威/主值”块。
   `matching_commit`、earliest/oldest 等当前工具不具备兼容 ordered carrier 时会诚实
   fail-open，后续随相应工具载体扩展，而不会借 prose 猜造。
6. 测试覆盖：latest merge 从 5 条窗口选首项、merge profile 拒绝普通 commit stream、
   recent-N 选择最小充分 typed 窗、GitLog producer 参数、Analyzer profile 解析与 schema、
   Explorer 生产 prompt、Finalizer carrier→principal prompt；负臂固定 summary 文本不能
   冒充 typed merge filter。

验证：

- `go test ./internal/types ./internal/skill -count=1`：18.869s / 0.352s；
- `go test ./internal/agent ./internal/tool -count=1`：2.721s / 164.484s；
- `go test ./internal/context ./internal/orchestrator -count=1`：0.749s / 10.170s；
- `git diff --check` 通过。

状态：`EVAL-B21-ORD1=implemented/full-tests-pass/replay-next`。本批没有引用 Trace
family/query、显式时间窗 authority、因果投影、根因排序、唤醒链、窗内可消除量或系统
自动补采；这些路径不受 `history_selection_profile` 影响。

#### B21 r4：有序 principal 已收敛，历史→当前映射与非枚举补表仍有缺口（2026-08-01）

在 `main@c0866048c` 的同一二进制快照下严格并行 2 个 Git/current-source case：

- `read_combo_git_current_source_explanation`：runner FAIL，200s，人工 FAIL；
- `read_combo_git_diff_hunk_current_code`：runner FAIL，159s，人工 PASS。

`EVAL-B21-ORD1` 已获得真实正证。两次 analyzer 分别发出
`latest_one/merge` 与 `latest_one/commit`；Explorer 分别调用
`git_log merges_only=true first_parent=true` 和 `git_log count=1`；Finalizer 的
typed principal 分别固定为 `2a58a60d...` 与 `c0866048...`。模型不再按
commit subject、文件角色或主观“功能性”跳过第 1 项，因此该项转为 covered。

两个 runner FAIL 都命中 `EVAL-B21-E1`：答案已分段覆盖 diff/current-source/作用/边界，
旧 oracle 却要求相关词面出现在同一行。普通 commit 答案的 principal 结论正确，人工通过；
不为修机器分数给生产答案增加词面硬门。

merge 答案仍有真实错误：正文把 `explicitRuntimeArtifactLog` 写成定义在
`internal/tool/test_surface_test.go`，而当前 checkout 的真实定义在
`internal/agent/agent_test.go`。日志显示 Explorer 多次以 `fixed_string=true` 调用 grep，
同时把 `A|B`、`foo|bar` 形模式当正则交替；工具忠实执行字面搜索返回零命中，模型据此一度
断言两个测试函数“不存在”。后续局部纠正没有形成每个历史符号到当前 path/definition 的
独立对齐，最终错误位置仍进入正文。

普通 commit 答案还复现“系统按已验证证据补充缺失成员”表。当前请求是四维解释，不是完整
枚举；Explorer 自行发出的 `principal_answer member_set` 仍可触发 deterministic backfill，
把内部英文 reason 和二次清册追加到正文末尾。这与 B19d 已收敛的前置 authority block
同源但不是同一发布器，不能视为已覆盖。

新增/刷新台账：

| ID | 优先级 | GAP | 泛化方案 | 状态 |
|---|---:|---|---|---|
| `EVAL-B21-ORD1` | P0 | 有序历史第 1 项可被模型按主观重要性跳过 | typed request selection × typed ordered tool result 已接线并获 merge/commit 双正证 | covered |
| `EVAL-B21-MAP1` | P1 | VCS diff 中的 symbol/path 事实可在没有 current-checkout 逐符号对齐时，被写成当前源码位置；历史坐标与当前定义席位混用 | 将历史 changed-symbol/path observation 与 current-source definition observation 分席；只有 exact canonical path+symbol 对齐或显式 rename/transition witness 才铸 `aligned`，否则只允许分别陈述历史改动与当前查得事实。复用并推广 `EVAL-B21-TRANS1` 的 transition authority，不扫描答案 prose | open；并入 B21-C |
| `EVAL-B21-GREP1` | P2 | `fixed_string=true` 与含 regex metachar 的 pattern 组合会产生高置信静默零命中 | grep 返回 typed query-semantics advisory：明确 literal 模式，并在零命中且含正则元字符时软建议以 regex 模式重试；不拒绝合法的字面 `|` 搜索，不把 advisory 当 absence 事实 | open |
| `EVAL-B21-SUP1` | P1 | 非 enumeration 请求中，模型自铸 principal member-set 也能触发用户可见 deterministic 补表，泄漏内部协议并重复正文 | 补表发布需同时具备 typed enumeration/relation/inventory obligation 或系统拥有的完整 roster；普通解释题的 model-authored aggregate 只留在 handoff/审计层。保持 Trace typed projection、根因榜、窗内可消除量和系统自动补采独立发布权限 | open |
| `EVAL-B21-E1` | P3 | 单行 regex 假阴性 | 改 eval-only 为分维度 oracle；不改生产成文 | filed-low |

证据：

- `eval/parallel_selected_summary_evalcampaign_b21r4_20260801.md`；
- `eval/parallel_selected_summary_evalcampaign_b21r4_20260801_manual_audit.md`；
- 结果目录时间戳 `20260801-065801`。

下一批按杠杆排序：B21-C 先统一历史/外部工件→current checkout transition authority，并
吸收 `MAP1`；B21-S 收窄非枚举补表权限；`GREP1` 作为软工具语义提示小批。三批均不修改
Trace 显式窗的 full-report 判定、因果投影构造、根因排序、唤醒链、窗内可消除量与自动补采。

#### B21-C：historical observation ↔ current checkout typed boundary

`EVAL-B21-TRANS1` 与 `EVAL-B21-MAP1` 的共享边界已落地：

1. 新增 `HistoricalCurrentSourceAuthority`，只从 accepted Observation Ledger 编译
   `runtime_artifact / vcs_metadata / vcs_diff` 历史席、typed VCS changed-path 清册和
   Grounded/Recovered 的精确 current-source line spans；不读取 RawRequest、模型 thinking、
   closure reason 或最终答案。
2. 当前没有任何 producer 携带“客户工件 revision 映射”或完整 behavioural transition
   witness，因此历史席与当前源码席同时在场时，唯一合法 transition 状态为
   `unproven`。同 symbol、同 path、相近 line 都不能自行升级；未来要增加 proven 状态必须
   先增加 typed producer，不能靠 prompt/prose 猜造。
3. VCS changed-path 完整清册存在时，current binding 只列 changed path 内已验证的
   current-source records；每项逐条发布 `symbol + current_path + current_lines`。
   `historical_path_match=true` 只证明路径在历史变更清册中，不证明 symbol continuity 或
   行为延续。历史 symbol 不在 exact current binding 清册中时，Finalizer 被明确要求保持
   当前位置/status unproven，不能借附近 helper 的 citation 代位。
4. Current-status enum 仍表示“当前 checkout 的同类风险是否存在/已被 guard 阻断”，不再
   暗示已经证明客户历史事件发生了版本迁移。中文用户可见标签由“仍然存在/已修复”收窄为
   “当前代码仍存在同类风险/当前代码已阻断同类风险”；schema 与 finalizer prompt 同步说明
   无 transition witness 时不能声称“是哪次改动修复客户事件”或“客户构建已包含该修复”。
5. 这是成文前 typed ceiling 与用户可读 enum 口径，不扫描、拒绝或改写模型答案正文。
   历史 diff 事实和当前实现事实仍都可回答，只禁止把两席自动连接成未经证明的历史变化。

测试矩阵：VCS changed-path + 精确 current definition 正臂、changed-path 外当前记录排除、
runtime artifact 与同名 current symbol 仍保持 unproven、单席 inactive、Finalizer 生产 prompt
接线、current-status 中英文词面与全 enum 映射。完整回归：

- `go test ./internal/types ./internal/agent ./internal/render ./internal/tool -count=1`：
  types 18.505s、agent 2.797s、render 0.871s、tool 160.855s；
- `git diff --check` 通过。

状态：`EVAL-B21-TRANS1=implemented/full-tests-pass/replay-next`；
`EVAL-B21-MAP1=implemented/full-tests-pass/replay-next`。本批没有改动 Trace query/family、
显式用户窗优先级、因果投影、根因排序、唤醒链、窗内可消除量或系统自动补采。

#### B21-S：非枚举解释不再被 model member_set 强制改造成系统补表

`EVAL-B21-SUP1` 已按 answer-set 权限而不是题面/标签施工：

1. history lookup 若没有 typed set/count/completeness obligation，即使 Explorer 自行把
   coverage/member ledger 标成 `principal_answer member_set`，也不再启用
   principal-enumeration hard gate、missing-row table 或 legacy ordered-list carrier。
   历史 diff 与当前源码的解释仍保留在模型正文和 Observation Ledger，系统不再把辅助
   hunk/module 清册变成第二份用户答案。
2. 以下闭集继续拥有完整性补表权限：`IntentEnumerate`、category enumeration、count、
   per-member table、`CompletenessObligation/EnumerationBoundary/buckets`、recent-N/oldest-N
   history selection、relation member-set、source-operation site set、source inventory 和
   change-impact。单个 latest/earliest history target 不因模型自铸 member_set 获得集合权限。
3. 合法补表的用户标题由“系统按已验证证据补充……”改为按用途表达：
   “清单完整性补充 / 可核验字段补充 / 成员说明补充 / 成员清单补充”；英文对应
   `List completeness / Verifiable field / Member note / Member list supplement`。typed
   provenance、完整性和 SystemGeneratedKind 保持内部可审计，不再泄漏到标题。
4. eval runner 的 `enumeration_push` 统计同步新标题，避免可见文案改名造成监测盲区。
5. 判定只消费 RequestModel enum/profile；不读取 RawRequest、aggregate label、commit
   subject、模型 thinking/closure/final prose。Trace 的因果 projection、根因榜、唤醒链、
   occurrence/state 主值与窗内可消除量均由独立 typed materializer 发布，不经过本次
   history-only suppression。

测试覆盖 narrative history 的 row compiler + legacy carrier + hard gate 三端同时关闭，
recent-N/completeness 正臂仍开，中英文标题禁止内部 authority 词面；既有 enum/relation/
source inventory/runtime 矩阵全部回归。`go test ./internal/tool -count=1` 通过（157.904s），
`git diff --check` 通过。

状态：`EVAL-B21-SUP1=implemented/full-tests-pass/replay-next`。下一项为低耦合
`EVAL-B21-GREP1` 软语义提示；完成后用严格并行 2 case 回放 B21-C/S/GREP，再转 Trace
显式窗与 write/plan 模式，避免长期停留在 Git 单一维度。

### B19g-b r1：target authority 通过，有限事实集仍被扩张（2026-08-01）

同一二进制快照下严格并行 2 个 Trace case，runner 均 PASS，人工均 FAIL：

1. 无窗口 D-state/IO 事实查询已发出
   `runtime_target_profile=named_target`，且 PID 59566 仅来自
   `runtime_targets.source=user_explicit`。最终主值恢复为 3 次/0.635ms，证明
   `entities_fallback` 和 entity+cursor+supplement 的噪声扩权已关闭。
2. 本轮 analyzer 将同一请求标为
   `intent=trace / kind=mechanism / diagnostic=false / relation=false`。请求仍只要“是否、
   时间、kernel reason、总量”这个有限运行时事实集，但旧的窄形状只接受
   trace+conditional 或 explain+mechanism，因而系统仍补跑
   `root_cause_rank + critical_blocking_calls`，追加完整因果投影。
3. 登记 `EVAL-B19-FACTSET1/P0`：这是 typed 问题形状的缺口，不是目标身份缺失。
   最优小修是将“有 runtime target、非关系、非诊断、非 performance
   bottleneck 的 trace+mechanism”纳入共享窄事实谓词；该谓词同时约束 family、
   supplement 和 materializer，不增加 case 名/原文关键词规则。
4. 显式时间窗正例仍完整发布主要占用、可消除量、根因排序、唤醒链、
   代表窗和 Trace 因果投影，自动补采也在；这是 B19g-b 的不变量正证。
5. 正例仍复现 `EVAL-B19-CAUSAL1/P1`：model principal 把候选链写成结构性瓶颈、
   次根因/三级原因，而 typed footer 明确
   `frame_causality=unproven / frame_evidence_status=absent`。该项要用 typed
   causal ceiling 改变系统结论面的权限/词法，不通过扫描模型原文做替换门。

状态：B19g-b target authority=`covered`；`EVAL-B19-FACTSET1=P0/open`；
`EVAL-B19-CAUSAL1=P1/open`。

#### B19h-a：finite runtime fact-set authority

已实现：

1. `IsFocusedRuntimeFactQuestion` 现在同时接受非诊断的
   `intent=trace|explain + kind=mechanism + typed runtime target`；`trace` 只表示证据
   来源，不再自动授权全窗根因报告。
2. 判定显式排除 call-chain/`is_relational_lookup`、diagnostic、
   `scenario=root_cause|performance_bottleneck`；另外显式 user time window 仍在
   `RuntimeTraceReportShapeAuthority` 中先于窄事实判定，不会伤及窗内因果投影。
3. family、system supplement、AnswerDocument materializer 继续共用一个 typed
   谓词；没有新增任何用户原文、模型 thinking/summary/final 或 case ID 扫描。
4. 回归固定 trace/mechanism 只补 `window_stats`、已有完整事实则零因果补采、
   最终文档保留 3 段/0.635ms typed roster 但不形成 Trace 因果投影；并固定
   performance/root-cause/relation/显式时间窗负例。

验证：`go test ./internal/types ./internal/tool -count=1` 全部通过
（types 22.784s，tool 167.889s）。

状态：`EVAL-B19-FACTSET1=implemented / same-pair replay next`；
`EVAL-B19-CAUSAL1=P1/open`。

#### B19h-a r1：legacy label 组合不能承载运行时答案范围

严格并行 2 个 case 均 runner PASS，人工均 FAIL。无窗口请求第三次出现新的
analyzer 合法形：

`intent=root_cause / scenario=root_cause / kind=conditional / axis=condition /
diagnostic=true`。

这证伪了继续罗列 intent/scenario/kind 组合的方案：同一个“是否、时间、记录原因、
总量”请求已先后落到 return-value、trace/mechanism、root-cause/diagnostic 三种组合。
主值 3 次/0.635ms 仍正确，但整份因果报告再次被追加。

同时登记 `EVAL-B19-SCHEDPROSE1/P1`：model 把 `sched_switch prev_state=D`
解释成“被 Hilogd 强制抢占的瞬时快照”。实际上 `prev_state=D` 表示被切出任务已
进入不可中断等待；只有 `R/R+` 才能成为 still-runnable preemption 候选。现有软 prompt
已教授该语义，但没有一个可直接成文的 typed scheduler-event interpretation 卡。

显式 114.940ms 窗仍完整保留两维占用、可消除量、根因排序、唤醒链、
Trace 因果投影和自动补采。但 `EVAL-B19-CAUSAL1` 第三次复现：principal 把候选写成
“直接原因”和低优先级线程“持续持有 CPU”，typed 边界则明确 frame causality
unproven。该项已不能归为单次模型波动。

#### B19h-b：runtime question-scope authority

通用方案：

1. `AnalysisIR v17` 新增必填 `runtime_question_profile`，闭集为
   `not_applicable / bounded_fact_set / causal_diagnosis / relation_analysis /
   system_overview / unspecified`。
2. 该 profile 与 artifact scope（全工件/显式窗/selector）、target identity 正交；
   legacy intent/scenario/kind 继续用于路由，但不再独自决定运行时报告宽度。
3. `bounded_fact_set` 包含一个或多个有限观测字段：状态是否存在、时间、次数、
   总时长、压力/驻留值、kernel/tool-recorded reason。“recorded reason”是观测字段，
   只有请求进一步问它为何导致故障时才是 `causal_diagnosis`。
4. 具体 scope 要求 exact current-request `source_quote`；本批曾令
   `bounded_fact_set` 与 typed call/relation 同时出现时 fail-loud。B41 r2 的真实 Binder
   witness 已证伪该合取：有限 direct peer/transaction/waker 事实同样带 relation shape；
   该历史约束由 B41 正式取代。硬决策仍只读 typed enum，不扫描 quote 语义、
   RawRequest、thinking/summary/final 或 case ID。
5. 本批当时冻结的权限顺序为“显式 window > typed call relation > runtime scope >
   fallback”；B41 r2 将其纠正为“显式 window > runtime breadth > legacy typed relation >
   fallback”。显式窗的投影/根因/唤醒/可消除/补齐始终不受影响。
6. family、supplement、full-report materializer 共用同一 typed 窄化谓词；
   principal-value materializer 也改用该谓词，保证窄化后仍发布 complete typed
   occurrence/state 卡。

验证：

- `go test ./internal/agent ./internal/orchestrator ./internal/types ./internal/tool ./internal/skill -count=1`
  全部通过（agent 5.093s、orchestrator 11.734s、types 23.024s、
  tool 171.101s、skill 0.698s）；
- 回归同时固定 root-cause/diagnostic label 噪声被 `bounded_fact_set`
  收窄、`causal_diagnosis` 反向压过 scalar label，显式 user window 仍压过
  bounded scope。本行记录历史验收；typed relation 与 bounded 冲突的断言已由 B41 r2
  supersede，并新增 finite-relation 与 explicit-window 双向回归。

状态：`implemented / relevant-full-tests-pass / same-pair replay next`。

#### EVAL-B18-SCOPE1：typed exclusion 被自动 all-scope 覆盖（P1）

implements 的最终答案虽然正确，但 r1 日志暴露权限层冲突：

1. analyzer 发出精确 typed
   `answer_exclusion_policy.excluded_candidate_roles=["test"]`；
2. 同一 payload 把 `internal/agent/agent_test.go` 放入 `irrelevant_files`；
3. source-inventory negative-channel reconciliation 为避免隐藏辅助源，提升该
   路径为 required，并合成
   `source_scope=all/include_auxiliary_as_principal=true`；
4. typed relation projection 只看 source scope，没有消费 exclusion policy，
   最终把 3 个 test rows 标成 principal，清册变为
   `principal=15, auxiliary=0`；
5. 本轮模型手工只写 12 个 production，因而 human PASS，但换一次模型就
   可能再次把 test 提升进主表。

这是 typed policy 之间的优先级/合取 gap，不是“主要”一词或 test 文件名
特例。

#### B18e exclusion × source-scope authority convergence

通用方案：

1. `AnswerExclusionPolicy` 增加
   `ExcludesSourcePathRole`，把 typed candidate-role exclusion 映射到
   deterministic path role（test/docs/example/fixture/generated）；
2. `reconcilePrincipalScopeIrrelevantFiles` 在提升 auxiliary irrelevant path
   前先消费该 typed exclusion：明确排除的路径留在 negative channel，不得
   触发 synthetic all-scope；
3. `ProjectTypedRelationMemberScopeLane` 同样先消费 exclusion，再消费
   SourceScopeProfile；即使其它未排除的辅助类要求 all-scope，被明确排除的
   类仍保持 auxiliary；
4. production、unknown 及同名符号行为不变；所有 implements/called-by/
   references/registers relation kinds 共用；
5. 不扫描 RawRequest、答案 prose、模型 thinking/rationale 或具体路径字面。

定向测试覆盖 exclusion 阻止 irrelevant test 路径合成 all-scope，以及
all-scope 与 test exclusion 同时存在时 test=auxiliary、production=principal。

全量回归：

- `go test ./internal/types -count=1`：20.407s；
- `go test ./internal/tool -count=1`：162.585s。

状态：`implemented / full-tests-pass / replay next`。

#### B18e r1：scope authority 通过，图语法另有独立失败（2026-08-01）

严格并行 2 个回放：

- called-by：runner/human PASS，117s，roster=2 principal；
- implements：runner PASS / human FAIL，126s。

B18e 的权限目标已经覆盖：implements 清册恢复为
`complete=15, principal=12, auxiliary=3, unknown=0`，最终列表/图仅含
12 个 production，3 个 test 只在 caveat。`EVAL-B18-SCOPE1` 状态：
`covered`。

implements 的 human FAIL 来自独立 Mermaid 语法：

1. structured diagram 声明 `flowchart TD`；
2. model body 却使用 classDiagram generalization operator `<|--`；
3. 现有 flowchart unsafe-node compatibility pass 把 operator 片段别名成
   `codraxNode1["<"]|codraxNode2["--"]`；
4. 最终虽保留节点和数量，但实现关系边变成伪节点，不能算合格关系图。

登记 `EVAL-B18-DIAG1`（P2）：diagram grammar/operator mismatch 的兼容层
缺口。

#### B18f Mermaid mixed-grammar soft normalization

采用 source compatibility soft rewrite，不增加 answer hard gate：

1. 仅当 structured Mermaid body 首指令为 `flowchart/graph` 时生效；
2. 在 unsafe-node aliasing 前，把 class generalization
   `Parent <|-- Child` 转为
   `Parent -->|generalization| Child`；
3. 对反向写法 `Child --|> Parent` 交换端点后生成同一 parent→child 语义；
4. class relation 的 `: extends/implements` 后缀保留到 quoted edge label；
5. quoted node labels 内的 `<|--` 不改，真正的 `classDiagram` byte-preserve；
6. 这是语法兼容修复，不拒绝模型答案，不扫描 RawRequest、summary、items
   或其它 prose。

定向 `internal/mermaidcompat` 与 `internal/render` 回归通过
（0.753s / 1.856s）。

全相关回归通过：

- `internal/mermaidcompat`：0.640s；
- `internal/render`：1.552s；
- `internal/tool`：166.060s。

状态：`implemented / full-tests-pass / Loop+H8 replay next`。

#### B18c r1：implements 通过，called-by 暴露实体轴冲突（2026-08-01）

严格并行 2 个回放：

- `qf_type_relation_loop_controller`：runner PASS / human PASS，160s；
- `qf_called_by_typed_relation_query`：runner PASS / human FAIL，212s。

implements 答案只保留一个 12 项 production 主表和一张 Mermaid，系统没有
再追加 accepted-checklist 清单，B18c 的结构化主键去重生效。

called-by 的重复是更上游的新问题，不能由显示层去重掩盖：

1. typed graph 权威 roster 是 2 个 caller members；
2. explorer 把 3 个 call-site observations 当成 `member_set`：
   同一 `BuildTypedRelationQueryWithResolvedSources` 的 line 290、291 各一项，
   加上 `TypedRelationKindsForRequest` line 317；
3. exact relation authority 只验证每行能否匹配 candidate，却没有验证多行
   是否映射到同一 candidate identity，于是把 value=3 的 observation set
   铸成 principal member-set；
4. 模型主列表按 function axis 正确列 2 项，系统 aggregate carrier 按
   observation axis 列 3 项，答案形成“2 个函数 / 3 个函数”的硬矛盾。

登记 `EVAL-B18-AXIS1`（P1）：typed relation entity-axis normalization
缺失。它适用于 called-by、references、registers 等“一成员多 observation”
关系，不是某个函数或行号特例。

#### B18d typed relation candidate-identity canonicalization

最优方案在 relation authority 铸造点统一处理：

1. 只对已通过 exact typed relation 匹配、将获得 system provenance 的
   principal member-set 生效；
2. 每个结构化 member 解析为 typed candidate identity：
   `member name + canonical source file`，relation source 使用独立 source
   identity；
3. 同一 identity 的多条 observation 只保留第一条主成员，`value` 重新从
   canonical members 派生；全部原始 call-site evidence 仍保留在 Evidence
   Pool / relation edge lane；
4. 同名但位于不同文件的 candidates 保持两个身份；source/member 同名或
   无法唯一映射时 fail-open，不做折叠；
5. Members、MemberNotes、SupportRefs 同索引投影，避免成员去重后引用错位；
6. 不读取用户原文、模型 thinking/reason/final prose，不识别 caller、
   function、行号或具体 relation kind 字面；所有 relation kinds 共用。

定向测试已覆盖：

- 同一 candidate 的两条 observation 折成一个 member；
- relation source 仍可与 member roster 同席；
- 同名不同文件的 candidates 不折叠；
- 同名但 support_ref 指向非 candidate 文件时 fail-open；
- 旧 analyzer-drift relation authority / source inventory convergence 保持。

完整 `internal/tool` 回归通过（最终复跑 161.534s）。

状态：`implemented / full-tests-pass / cross-relation replay next`。
## EVAL-B53：显式窗 Trace 关系权限跨阶段一致性 × C++ 写模式（2026-08-03）

### B53 r1：写模式真实收敛；Trace 出现 typed authority 必带/必拒死锁

在 `main@b1aa5e58f` 的同一二进制快照下严格并行 2 个 case：

- `trace_query_wakeup_causal_runnable`：runner FAIL / human FAIL-system，539s；
- `github_issue_fmt_tm_year_overflow_symptom`：runner PASS / human PASS，310s。

C++ 写模式通过真实项目验证：首计划只把 `parts.year_offset + 1900` 提升到
`int64_t`，随后仍窄化成 `int`，`make check` 精确打红；replan 改为保留宽整数并直接
`std::to_string(static_cast<long long>(calendar_year))`。最终 applied tree 只改
`include/tmfmt.hpp`，项目 runner 实际编译/执行，普通年份与 `INT_MAX → 2147485547`
均通过；final `verified` 与累计 typed proof 一致。模型第一次修错、被真实测试拦截后自愈，
属于写控制器应当容纳的正常推理过程，不登记 type-specific gate。

Trace 的查询、自动补齐和模型前决策上下文本身已经提供：

1. 用户显式窗 `1.000000..1.010000`；
2. `worker-200 → app-100` 唤醒路径；
3. 实际占用轴：目标 sleep 10.000ms、唤醒后 runnable 0.020ms；
4. 现有规则可消除轴：`worker-200` pre-wakeup dependency 8.300ms；
5. 精确 phase ceiling：候选标记不证明锁 holder/waiter，也不证明唤醒后 CFS 抢占 RT。

因此根因排序、唤醒链、两类根因、可消除量与系统补采均未缺失。真正失败发生在 typed
关系权限跨阶段：`target_window_states` 的账户窗为 `1.000000..1.010020`，投影 anchor 为
`1.000000..1.010000`。账户接入器按全系统 F-2 ±1ms 同窗规则合法接入；探索期 compact
preview 据此铸造并接受 `trace:target_state_partition:*`。成文期关系编译器却要求账户总量
等于 anchor 字面时长，用 10.000ms 拒绝五状态自身闭合的 10.020ms，导致同一个 authority
从最终 roster 消失。handoff 仍要求模型复制已接受声明，validator 又判该 ID 不存在：
模型无论携带还是省略都必拒，最终 20 轮、40 次成文 reject 后降级，答案为空。

新增台账：

| ID | 优先级 | GAP | 泛化方案 | 状态 |
|---|---:|---|---|---|
| `EVAL-B53-RELWIN1` | P0 | 同一个 typed target-state account 在投影接入、探索 closure 与最终关系校验使用不同同窗/闭合规则，形成 authority 必带/必拒死锁 | 关系编译复用唯一 F-2 typed endpoint 容差；subtotal 对账户自身 typed window 闭合，不对容差接入后的 anchor 字面长度二次裁决；新增 preview→anchored-final authority ID/validation 一致性 pin。全程不读取 RawRequest、模型 thinking/final 或关键词 | implemented / full-related-tests-pass / replay-next |

#### B53-A：target-state partition authority 同窗单源

`answerRelationTargetStatePartitionClosed` 已改为：

1. 投影 anchor 与账户窗仍必须在共享 `TraceCausalProjectionSameWindowToleranceS`
   （±1ms）内，超过容差继续 fail-closed；
2. 五状态之和仍须在 3 位发布精度精确等于 typed `TotalMS`；
3. closure total 改为核对账户自身 `WindowStartTs..WindowEndTs`，不再核对容差内但字面略有
   差异的 projection anchor；
4. 20µs 端点差的生产形新增 preview/final authority ID 恒等 pin；2ms 真跨窗负臂继续拒绝。

这是同一 typed 信号在三个消费面的语义统一，不放宽跨窗加法，不修改模型正文，不新增
答案接管或题面/答案关键词 gate。显式时间窗的 Trace 因果投影、根因排序、唤醒链、窗内
可消除量和系统自动补齐仍沿原路径运行；修复只保证它们交给模型的关系权限可以被最终
validator 同样接受。

证据：

- `eval/parallel_selected_summary_evalcampaign_b53_trace_write_r1_20260803.md`；
- `eval/parallel_selected_summary_evalcampaign_b53_trace_write_r1_20260803_manual_audit.md`；
- result dirs：`20260803-050542`；
- 完整 `go test ./internal/types ./internal/tool -count=1` 通过（20.180s / 168.763s）；
  `internal/agent` Trace-decision relation handoff 专项与 `git diff --check` 通过。

### B53 r2：空答案恢复；最终 background 席未进入模型前上下文（2026-08-03）

修复 rebased 并推送为 `main@3ff195b90` 后，以同一两例、同一二进制快照严格并行回放：

- `trace_query_wakeup_causal_runnable`：runner PASS，167s；核心答案恢复，人工仍判
  FAIL-system；
- `github_issue_fmt_tm_year_overflow_symptom`：runner PASS / human PASS，203s。

`EVAL-B53-RELWIN1` 获得真实正证：最终 `Trace Decision Inputs` 发布当前
`trace:target_state_partition:aebff006a573b1a3` 和 10.000ms subtotal；模型首轮漏带后收到
精确 typed repair，第二轮携带并一次通过。模型完整 summary、时间线、优先级候选 caveat
均保留，系统 Trace 投影在其后按既有 typed 路径追加，没有删除或替换模型结论。空答案从
539s/40 次成文拒绝降为 167s/一次关系修形，故 `RELWIN1=covered`。

人工审计仍发现一个独立上下文矛盾：模型摘要写“无 CPU 压力证据”，同页系统投影稍后却
发布 `调度压力(需求积压)=3.500ms`（跨线程累计、非墙钟、background）。该行是最终
projection 已持有的 typed `BackgroundCauses`，但模型前 `Trace Decision Inputs` 只投影
axis A、axis B、target state、wakeup 和 evidence boundary；background/adjacent 行没有
handoff。系统没有直接改写模型，但把模型未见过的材料追加到同页，导致模型无法避免与其
冲突。修复不能扫描“无 CPU 压力”等答案词面，也不能隐藏系统行或把 background 升格为
因果。

新增台账：

| ID | 优先级 | GAP | 泛化方案 | 状态 |
|---|---:|---|---|---|
| `EVAL-B53-CTXBG1` | P1 | 最终会发布的 typed adjacent/background 行缺席于模型前 decision handoff，模型可否认系统随后展示的证据 | 从同一 `TraceCausalProjectionSet` 有界投影 `contextual_noncausal_rows`；逐行携带 lane、typed kind/value/unit/caliber、source lane 与 `target_causal_authority=not_provided / cross_axis_addition=forbidden`。只作模型上下文，不创建 block、不改 prose、不参与硬门 | implemented / targeted-tests-pass / replay-next |

#### B53-B：最终投影 context 行提前只读交接

`renderAnswerDocTraceDecisionHandoffSet` 新增最多 6 行的非因果上下文面：

1. 数据仅来自最终 projection 的 `AdjacentCauses` / `BackgroundCauses`，并复用
   `WithinRequestedWindow`、evidence-boundary 排除和 evidence identity 去重；
2. aggregate metric 明示 `aggregate_context_non_target_wall_clock`，普通行保持
   `context_observation`，禁止模型把跨线程累计改写成目标墙钟；
3. 每行固定 `target_causal_authority=not_provided` 和 `cross_axis_addition=forbidden`，
   只约束“证据是否存在”的认知，不授予根因、可消除量或跨轴加法权限；
4. 不读取 RawRequest、问题关键词、模型 thinking/final，也不新增 emit-time validator。

agent 专项正臂固定 deterministic supplement 的 3.500ms aggregate background 行进入 handoff，
同时仍不进入 axis A/B。Trace 显式窗口、因果投影 materializer、自动补采和模型结论所有权
均未改。

证据：

- `eval/parallel_selected_summary_evalcampaign_b53_trace_write_r2_20260803.md`；
- `eval/parallel_selected_summary_evalcampaign_b53_trace_write_r2_20260803_manual_audit.md`；
- result dirs：`20260803-052614`；
- 新 HTML：`.codrax/output/20260803-052859.277-29088.html`；
- `go test ./internal/agent -run 'TestTraceDecisionHandoff' -count=1` 通过。

### B53 r3：background 上下文闭环；关系载体与混合证据车道新 GAP（2026-08-03）

在 `main@a14cadc8f` 下严格并行 2 个 case：

- `trace_query_wakeup_causal_runnable`：runner/human PASS，193s；
- `read_combo_trace_current_source_explanation`：runner PASS / human FAIL，314s。

Trace 回放给 B53-B 提供了生产正证：finalizer 前的 `Trace Decision Inputs` 已有
`contextual_noncausal_rows`，包含 adjacent `worker-200 sleep=0.200ms` 与 background
`supply_pressure=3.500ms`；后者明确标为
`aggregate_context_non_target_wall_clock / target_causal_authority=not_provided /
cross_axis_addition=forbidden`。模型不再声称“无 CPU 压力证据”，但也没有被要求把背景行
升级为目标根因。显式时间窗、唤醒链、8.300ms 优先级候选、实际占用/规则可消两轴、
系统自动补采和 Trace 因果投影完整保留，故 `EVAL-B53-CTXBG1=covered`。

同轮暴露两个通用系统 GAP：

| ID | 优先级 | GAP | 泛化方案 | 状态 |
|---|---:|---|---|---|
| `EVAL-B53-RELCARRIER1` | P1 | `relation_claims` 是 block metadata，但 missing-claim 诊断只报字段名；模型两次放到 document root 后，unknown-field compat 又静默 quarantine，造成同一 closure reject 重复 3 次 | 对精确 JSON key `$.relation_claims` 不再静默丢弃，立即返回合法 carrier `blocks[i].relation_claims`；handoff、schema、validator 同词面。只拒绝结构化错位，不搬运 claim、不读 prose、不改结论 | implemented / full-related-tests-pass |
| `EVAL-B53-MIXLANE1` | P1 | mixed runtime/source 问题的 runtime subtopic entities 被强制送进 repo symbol resolver；合法的 `86.111ms / frame budget` 因 sibling source symbol 可解析而触发 R1.5 hallucination hard reject | 当 typed runtime artifact 与 required current-source lane 同时存在时，将 resolver hit/miss asymmetry降为 advisory；各 lane 仍由 origin-specific evidence gate 验证。不扫描用户/模型原文 | implemented / full-related-tests-pass |

混合读答案本身仍不能人工判绿：它把 `B|2000|H:RenderService:DoFrame` 的 name 内 `H:`
误解为 flow-end action，并声称 B/E 端需同 name；真实语义是 action=`B`、完整 name=`H:...`，
匿名 `E` 在同一 ftrace 线程栈闭合且不重复 name。系统 performance instruction 已明确提供正确
规则，因此登记 `EVAL-B53-MARKERSEM1/P2-watch` 为一次模型推理错误；按“避免单 case/type
过拟合”裁定，本批不新增答案关键词 gate、正文替换或专名规则。后续用 B/E、S/F、G/H
不同 marker 形状回放，若跨模型重复，再考虑由 parser typed endpoint verdict 提供一个通用
动作/name/闭合键事实卡。

#### B53-C：relation claim 精确承载路径

1. full emit 在 quarantine 前检测精确顶层 JSON key `relation_claims`，明确拒绝并指向
   `blocks[i].relation_claims`；patch 路径对应指向
   `replace_blocks[i].relation_claims / add_blocks[i].relation_claims`。
2. missing closure claim 的 typed validator 同样附带 carrier path；Trace handoff 与 block schema
   统一写明 relation claim 不是 document-level field。
3. 不自动把顶层 claim 移到某个 block，因为多 block 场景下“哪个正文使用该关系”仍属于模型
   所有权；系统只做精确结构校验，不替模型选择、撰写或修改结论。
4. 回归固定 full/patch 两个顶层误放臂 fail-loud，并固定 missing claim repair 必须含正确路径。

#### B53-D：mixed runtime/source resolver authority

R1.5 的 repo-symbol hit/miss 本来只适合单一 current-source 实体宇宙；当
`HasExternalOnlyRuntimeArtifact/typed artifact reference` 与
`CurrentSourceLaneDecision=required` 同时成立时，mixed hit/miss 是预期形而非精确信号。
本批只把这个组合降为 advisory telemetry，原始 subtopics 不改，repo symbol resolver 不放宽，
后续 evidence plan、runtime artifact authority 与 current-source citation gate 均保持。定向测试固定
`parseTraceMark` 可解析而 `86.111ms/frame budget` 不解析时 quality gate 仍通过并保留 advisory。

闭环复核补正（2026-08-03）：首次实现直接调用 legacy
`CurrentSourceLaneDecision().RequiresCurrentSource()`，被全仓 authority chokepoint lint 正确拦截。
现改为组合共享的 request-only authority API：
`RuntimeSourceRequestCurrentSourceRequirementPrecision != none` 与
`RuntimeSourceRequestHasExternalObservationCarrier`。行为边界不变，但 soft/precise/excluded 与 artifact
carrier 的判定不再由 coherence gate 二次派生；mixed 正例和结构 lint 均通过。

证据：

- `eval/parallel_selected_summary_evalcampaign_b53_trace_read_r3_20260803.md`；
- `eval/parallel_selected_summary_evalcampaign_b53_trace_read_r3_20260803_manual_audit.md`；
- result dirs：`20260803-053450`；
- HTML：`.codrax/output/20260803-053801.905-32777.html`、
  `.codrax/output/20260803-054002.063-32776.html`。
- 完整 `go test ./internal/analysis/gate ./internal/agent ./internal/tool -count=1`
  通过（gate 0.414s、agent 2.847s、tool 168.305s）。

## EVAL-B54：探索期关系快照与最终补采 authority 冲突（2026-08-03）

### B54 r1：显式窗 Trace 复现不可满足 hard contract；mixed 读例暴露伪调用边

在 `main@cc73549c7` 同一二进制快照下严格并行 2 个 case：

- `trace_query_wakeup_causal_runnable`：runner/human FAIL-system，538s；
- `read_combo_trace_current_source_explanation`：runner PASS，人工核心答案通过但图关系不通过，298s。

Trace 的查询与模型结论内容本身仍然完整：显式用户窗、`net-300 -> worker-200 -> app-100`
唤醒路径、目标实际状态、8.300ms 规则可消除候选、background 非因果上下文和两维根因提示都在。
失败发生在成文 typed 合同：

1. Explorer 收口时，`validateCompletionRelationClaims` 按当时 ledger 接受并 retained
   `trace:target_state_partition:c737...`，subtotal=15ms；
2. closure 后 deterministic trace supplement 用用户精确窗重编译出最终
   `trace:target_state_partition:aebf...`，subtotal=10ms，旧 ID 不再属于最终 authority roster；
3. handoff 一边要求复制最终 10ms authority，一边要求原样保留 15ms accepted claim；
4. final validator 对“只带 10ms”报缺 accepted，对“10+15ms 都带”报 15ms ID 无 authority。

模型明确识别到 include/omit 均失败，仍被迫交替重试。实际连续 17 次
`emit_answer_document` 拒绝后人工终止；旧 eval 指标把每次 DEBUG toolresult 与 INFO
“成文校验未通过”镜像相加，误报 34。

新增台账：

| ID | 优先级 | GAP | 泛化方案 | 状态 |
|---|---:|---|---|---|
| `EVAL-B54-RELSTALE1` | P0/red-line | retained investigation relation claim 被当成永久 hard obligation；补采替换同族 authority 后，旧 claim 与最终 roster 互斥 | final typed authority 为唯一校验源；以 ID+成员+关系+加法+小计精确分区 accepted claims。仍被 final slate 支持的保留；失去支持的 typed 标为 superseded，只提示模型按最终值修订自己的正文，不搬运/替写结论 | implemented / full-related-tests-pass / replay-next |
| `EVAL-B54-RETRYMETRIC1` | P2/audit | eval 把一次 reject 的 toolresult/render 两条镜像日志双计 | 两个精确控制面 census 取 max；任一日志面缺失仍可观测，普通镜像不重复 | implemented / runner-tests-pass |
| `EVAL-B54-DIAGCALL1` | P1 | generic explanation 图在 `explicit_caller_callee_edges=0` 时仍可提交任意 `edge_anchors[].relation_kind=call`；本例把 5 条跨子系统逻辑顺序伪装为 direct call | 对所有 diagram family/语言统一复用 typed call-edge authority + endpoint alias resolver；显式 `relation_kind=call` 无对应 exact edge 时 fail-closed。逻辑流程改用 observe/contain/precedence，不扫描 Mermaid label、用户原文或答案 prose 来铸权 | implemented / full-related-tests-pass / replay-next |

#### B54-A：final authority 单源与 superseded investigation claims

本批实现三个层次：

1. `PartitionAnswerRelationClaimsByCurrentAuthorities` 仅按 typed 字段精确匹配当前 authority；
2. final handoff 不再发射已失效的 accepted claim，改发 typed superseded count，并要求模型消费
   final authority、自行修订可见结论；系统不修改任何 block text；
3. final validator 只要求 accepted/current 交集，同时仍要求所有 closure-critical final authority。
   补采“新增独立 authority”的旧正例保持，只有被最终证据替换的快照撤销。

回归覆盖：stale/current 精确分区、handoff 不泄漏旧 ID、最终文档只带 current claim 可通过、
补采新增 authority 仍要求 accepted+新增 superset、20µs 容差窗 ID 恒等保持。全程不读取
RawRequest、模型 thinking/final 或错误文案，不放宽 relation 校验，也不系统代写结论。

同类结构审计：`StableInvestigationRelationClaims` 的生产消费点只有 handoff 与 final validator，
两处已统一走分区。aggregate facts 使用 typed merge/supersede 且没有“旧 authority ID 必须保留 +
final roster 拒绝旧 ID”的双门；source inventory 在 final 前使用同一快照，Trace supplement 不改其
universe；causal/coverage stamps 从最终 ledger 编译；write replan 走 append-only iteration/verification
ledger。当前没有确认第二个同形不可满足合同，但这些面保留在后续异构回放矩阵中。没有增加通用
A/B 循环硬熔断：它只能更快交付空/降级答案，无法修复 authority；本批在矛盾产生前撤销旧合同。

mixed 读例的模型上下文审计：typed current-source/runtime 双 lane 最终足以促成 10 次 read 和正确
源码锚点，首轮“仓内无实现”是模型过早闭合，后续 DAG evidence window 已纠正，暂不加题面硬门。
但 final prompt 已明确 `explicit_caller_callee_edges=0 / ordered_path_authority=unproven`，模型仍提交
5 个 `relation_kind=call` edge anchors，CGEC 只记 2 条 diagram advisory。该精确 typed 伪边是
`DIAGCALL1` 的生产 witness，不能归为纯模型波动。

证据：

- `eval/parallel_selected_summary_evalcampaign_b54_trace_mixed_r1_20260803.md`；
- `eval/parallel_selected_summary_evalcampaign_b54_trace_mixed_r1_20260803_manual_audit.md`；
- result dirs：`eval/results/*-20260803-070344`；
- mixed HTML：`.codrax/output/20260803-070840.297-88408.html`。
- `bash eval/runner_lib_test.sh` 通过；
- `go test ./internal/types ./internal/agent ./internal/tool -count=1` 全绿
  （types 18.537s、agent 2.996s、tool 167.363s）；`git diff --check` 通过。

#### B54-B：显式 typed call authority 跨 family 单源（2026-08-03）

`EVAL-B54-DIAGCALL1` 已按关系声明本身修复，而不是按单一问题、语言或 Mermaid 文案拟合：

1. 除 `QFRootCauseTrace` 外，任意 answer family 的任意 block 只要显式提交
   `edge_anchors[].relation_kind=call`，都必须由一条同向、可引用、typed call-site
   `EvidenceItem.Subject -> Object/AnchorSymbol` 授权；generic、architecture、comparison、
   enumeration、role/config 等家族不再成为逃逸通道。
2. `QFCallChain` 仍保留更强的 sequence/call-DAG body 全覆盖和 model-selected principal path
   完备性；此次没有把该更强合同扩散到普通解释图。普通图未声明 typed call 的箭头仍可作为
   展示关系，显式 `observe/contain/precedence/guard/import` 也继续走各自关系合同。
3. `QFRootCauseTrace` 明确隔离：其 wakeup、blocking、frame-flow、显式时间窗因果投影和系统
   自动补采使用 runtime typed relation authority，不被源码 caller/callee contract 解释或拦截。
4. 校验只读取 `QuestionFamily`、`edge_anchors` enum、Mermaid 结构端点和 typed evidence 字段；
   不扫描用户原文、模型正文或 edge label 来制造 hard signal。模型仍可选择删除伪 call，或把
   逻辑顺序改为准确的非调用关系；系统不重写图和结论。
5. 同一实现同时服务 Go、Java、C/C++、Rust、Python、TypeScript/ArkTS、Cangjie 等语言；
   语言差异只由既有 exact endpoint/owner/operation alias resolver 消化。本批 family 全矩阵 pin、
   sibling block carrier pin、generic pre-emit/post-emit 双接线 pin，与仓内已有全部 executable-language
   call-DAG/participant alias 矩阵共同防回归，没有新增任何语言专名分支。

这一问题与 `RELSTALE1` 的关系也已区分：`RELSTALE1` 是两个阶段快照形成“旧声明必带、最终
roster 又必拒”的不可满足合同；`DIAGCALL1` 是 typed 声明在部分 family 漏检，属于 authority
绕过而非互斥。对 aggregate facts、source inventory、causal/coverage stamps、write replan 的同类
审计暂未复现第二个同形不可满足环：前两者有 typed merge/final snapshot，stamp 从 final ledger
编译，write verification ledger 为 append-only。此结论不是“全系统绝无”，上述面继续进入后续
read/write/多语言异构 eval 否证矩阵。

验证：

- `go test ./internal/tool -run 'TestDiagramCallEdgeEvidenceMismatches|TestRunPreEmitChecks_(GenericExplicitCallEdgeEvidenceAlignmentIsWired|DiagramCallEdgeEvidenceAlignmentIsWired|DiagramBodyEdgeWithoutAnchorIsWired|MixedCallDAGGuardStaysOutsideCallAuthority)' -count=1` 通过；
- `go test ./internal/types ./internal/skill ./internal/analysis/hint ./internal/agent ./internal/tool ./internal/orchestrator -count=1` 全绿（types 22.110s、skill 1.941s、hint 1.397s、agent 3.055s、tool 169.879s、orchestrator 13.806s）；
- `git diff --check` 通过；生产双例回放排在下一批，仍严格并行 2 个。

#### B54-C：r2 生产复验与同类红线面继续审计（2026-08-03）

在 `main@4dc46e393` 同一二进制快照下再次严格并行 2 个 case：

- `trace_query_wakeup_causal_runnable`：runner PASS，195s，实际 final reject=1；
- `read_combo_trace_current_source_explanation`：runner PASS，246s，actual final reject=0。

`RELSTALE1` 已获得生产闭环证据。最终 handoff 只发射当前
`trace:target_state_partition:aebff006a573b1a3`（10.000ms），旧 15ms authority 不再成为
accepted obligation；唯一拒绝是首稿漏带当前 block-level `relation_claims`，提示给出的动作
“把当前 authority 复制到 block carrier”可执行，第二稿即通过。旧日志一轮 reject 计两次的问题也已
消失，runner 正确记为 1。显式用户窗、3 次 `trace_query`、目标状态、唤醒链、根因排序、
8.300ms 规则可消除候选和 Trace 因果投影全部保留。

`DIAGCALL1` 的生产软引导也生效：mixed 例 final prompt 明示
`grounded_callsite_facts=0 / explicit_caller_callee_edges=0 /
ordered_path_authority=unproven`，模型选择不画图、不声明 `relation_kind=call`，零成文拒绝；hard
negative path 由跨 family pre/post wiring pin 覆盖。该结果证明修复没有把普通逻辑流程全部硬化为
call-chain，也没有影响 runtime Trace 独立关系 authority。

回放同时确认两个新系统 GAP：

| ID | 优先级 | GAP | 泛化方案 | 状态 |
|---|---:|---|---|---|
| `EVAL-B54-ARITHSUBJ1` | P1/red-line-near | 系统尾部 wall-clock 算术附注扫描模型自由文本，以邻近词/数字猜主体，把 worker-200 的 runnable 8.300ms 与其他上下文 10.000ms 绑定到 app-100，发布 28.300ms 假矛盾；同页 typed 行又正确显示 app running/runnable=0 | 用户可见的确定性主体-数值断言只消费结构化 item/typed carrier 的精确绑定；自由 prose 扫描不得铸造主体归属。无法精确绑定时静默，不把 noisy heuristic 包装成系统事实，不触发重试、不修改正文 | implemented / orchestrator-full-pass / commit-next |
| `EVAL-B54-EVROLE1` | P1 | `EvidenceItem` 的文件/行/quote 真实，但 Explorer 自写 Summary 把字段定义 `MarkerPID=marker payload 内 PID` 升级为“B/E span 配对键”；closure 与 handoff 又把该 Summary 当成源码机制权威，最终答案据此出错。实际同步配对键是 artifact source + ftrace header TID，MarkerPID 是 payload owner/namespace process identity | 将“定义/声明存在”与“作为运行时 key/order/route 使用”分级；定义行只具 local-fact authority。机制角色必须由实际 consumer/control-use/call-site 或直接实现该角色的 cited line 支撑；无 typed 角色载体时只作软探索提示，不能在 handoff 中加冕为已证机制 | confirmed / design-before-code |
| `EVAL-B54-MODELCAUSAL1` | watch/model | Trace 正文把已证 wakeup 时序进一步解释为“等待 I/O 完成”和“直接根因”，而 typed handoff 已明确 wakeup path 不证明 continuous blocking/holder | 不加原文关键词硬门、不由系统替写结论；继续异构用例观察。若重复出现，优先提升 typed relation semantics/上下文显著性，而非按该句拟合 | model-watch |

`ARITHSUBJ1` 不是普通展示瑕疵：该块标为“系统生成的确定性内容”，错误主体绑定会与真正 typed
状态分区正面矛盾，反而降低证据可信度。现有实现虽不触发 retry，仍违反“噪声信号只作软引导”的
精神：自由 prose 邻近启发式可以决定是否提示，却不能生成带具体线程名和数值的系统事实。因此优先
切断不精确的用户可见发布臂，而不是继续增加中文代词/词距特例。

`EVROLE1` 与 `DIAGCALL1` 有共同根：位置/符号真实不等于关系/运行时角色真实。call edge 已有 typed
关系 carrier，所以本批可硬校验；“keyed_by/used_as”目前缺少同等 typed carrier，不能直接照搬 hard
gate。下一批先核对现有 EvidenceItem/ClaimForm/consumer-use 数据面，选择可泛化的 typed 角色通道；
在没有精确信号前只收窄 handoff 确权，不能扫描 `MarkerPID`、`pairing` 等词来拟合本例。

#### B54-D：系统 prose verdict 发布面退役（`ARITHSUBJ1` 施工）

对 `collectSystemCrossCheckFindings` 全部生产 consumer 反查后，确认 8.300ms 错绑不是单一词距 bug，
而是同一架构问题的一个 witness：系统尾部仍有四组 provider 读取模型自由文本，再发布带“系统生成”
权威外观的判断——scalar 无法复算/窗与线程绑定、wall-clock 主体算术、headline 主因/供给/漏修向、
显式/隐式 prose 等式。它们不触发 retry，也不直接改正文，但仍会把 noisy 语义解析升级为系统结论，
与“系统只给准确事实，模型负责判断”冲突。

本批按数据来源而非中文词形统一切断：

1. shipping appendix 不再消费 `proseScalarResidualAppendixInputs`、
   `proseWallClockConservationFindings`、`proseHeadlineElimFindings`；
2. `proseFactJuxtapositionFindings` 拆成 offline diagnostic 与 production typed-only 两个入口；生产入口
   排除 prose equation/implicit subtraction verdict，只保留 observation ledger 直接生成的线程状态、
   席位、调用点、CPU 频点和 typed degradation 事实；
3. 自由文本中的精确实体 token 只可作为“选择展示哪条真实 typed fact”的噪声选择器，不能铸造
   主体-数值、因果、遗漏或正确性判断；
4. 两个 shipping exit 仍统一挂 appendix，Trace projection/补采和模型正文均不改；新增生产 witness
   精确复现 app-100/worker-200/8.300ms 形，禁止 `28.300ms`、`状态时长之和`、`正文中线程`
   等 inferred verdict 出厂，同时要求 app-100 的 typed `running=0/runnable=0/sleep=10` 仍可见；
5. structural pin 禁止四个 free-prose verdict provider 重新接入 `system_crosscheck_appendix.go`。

这不是放弃校验：结构化 `relation_claims`、typed diagram call edge、Trace authority、值守恒等精确
合同继续 hard/typed 校验；只是没有 typed subject/expression carrier 的自然语言复算不再伪装成系统
权威。`go test ./internal/orchestrator -count=1` 通过（12.286s），`git diff --check` 通过。

#### B54-E：Tier-2“soft 规则、hard 外观”与可提升硬门退役

继续从所有 `NoticeRetry`/`FilterFinalizerRetryRootViolationsForBus` 生产点逆向审计，确认四个
Tier-2 完备性维度存在同一红线邻近问题：

- `scalar_count` 从 block prose 扫整数；
- `cardinality` 从 markdown prose 数列表项；
- `path_depth` 从 block/item prose 提取 identifier-shaped token；
- `entity_parity` 从 block prose 计 bucket anchor 出现次数。

它们在默认配置虽为 soft，但 registry 仍允许 operator strict-promote；调度器还无条件打印
`Tier 2 hard-gate`、置 `res.Passed=false` 并发送 `NoticeRetry`，之后才在最终 root filter 中因 soft
重新放行。结果不是 `RELSTALE1` 那种不可满足双合同，却会制造“校验失败/正在重试”的假象；一旦
配置提升，noisy prose 信号会真实进入成文重试，违反用户明确裁定。

新增 `EVAL-B54-TIER2PROSE1`（P1/red-line）并完成根修：

1. 四种 Violation registry 统一设为 `SeveritySoft + SoftByDefault + Promotable=false`；即使
   `pipeline_contract_strict_kinds` 明列也不能提升为 retry root；
2. 调度器只在 `isStrictViolationForBus` 为真时才置失败、发 `NoticeRetry`；soft finding 仅写 debug
   telemetry，不再伪称 `hard-gate`；
3. validator 仍可作为探索完备性的软指导，不删除高 ROI 信息；恢复 strict 的前置条件冻结为：
   count/member/path/bucket 每个判据均有完整 typed carrier，且不再读取模型 prose；
4. pin 覆盖四 kind 的 registry、默认/strict profile、operator strict no-op 和 retry-root 空集。

验证：`go test ./internal/types ./internal/agent ./internal/orchestrator -count=1` 全绿
（types 24.251s、agent 2.735s、orchestrator 12.130s）。

截至本节，同形“两个阶段各持一份 authority，旧值必带而 final roster 必拒”的第二实例仍未确认：
aggregate 使用 typed merge/supersede，source inventory 在 final 前冻结同一快照，causal/coverage
stamp 从 final ledger 编译，write verification ledger append-only。已经确认并处置的是两个近邻类：
typed authority 漏检（`DIAGCALL1`）与 noisy prose 权威化/可提升硬门（`ARITHSUBJ1`、
`TIER2PROSE1`）。后续异构 read/write eval 继续否证，而不是宣称全系统绝无第二例。

#### B54-F：Evidence 位置真实但角色虚构的双旁路收口（`EVROLE1` 施工）

继续对 r2 mixed 例逐层回放后，确认错误不是 `MarkerPID` 的单点解析问题，而是同一条
`EvidenceItem` 在跨阶段传播时存在两套不等价的 authority：

1. typed 字段只证明 `Source/Line/AnchorKind/AnchorSymbol/Snippet`；本例源码仅说明
   `MarkerPID` 是 `tracing_mark_write` payload 内 PID，默认仍来自 `HeaderTGID`，并保留
   namespace PID；
2. Explorer 自写 `Summary` 和 `StableInvestigationReason` 却把它提升为“B/E span 配对键”；
3. Primary Evidence 与 `Current-Source Mechanism Relation Authority` 已正确使用 summary-free
   typed 表面，明确 local definition 不证明 key/order/call/path；但 `Accepted Closure Status` 与
   `Observation Ledger` 又把同一段自由文本重新注入 finalizer，形成旁路确权；
4. 真正同步配对使用 artifact source + ftrace header TID。文件/行真实只能证明局部定义真实，
   不能替代 consumer/control-use 证据。

本批按 carrier 类型统一修复，不扫描 `MarkerPID`、`pairing`、用户原文或模型答案：

- 对 typed mechanism/current-source 请求，只要已有 grounded current-source fact 或 supported typed
  flow，extractor/finalizer 的 authority 区不再发射自由文本 closure reason；保留 result kind、absence、
  boundary 和 typed aggregate carriers，模型仍从 typed 事实自行形成结论；
- `ProjectObservationPromptRecords` 不再把任何 `evidence:*` 记录的 `EvidenceItem.Summary` 投影到
  prompt-facing ledger。typed claim、source/span、grounded excerpt 和 dedicated support lane 保留；
  VCS、external document、runtime tool 等以 Summary 为原生 producer payload 的非 evidence 记录不受影响；
- mechanism relation authority 继续区分 local fact、explicit call edge 与 supported ordered flow；系统只
  约束证据权级，不改写模型正文、不替模型选择结论；
- exact witness 同时把伪角色放入 Evidence Summary 与 closure reason，要求 extractor/finalizer 全文均
  不得看到该伪角色，同时要求 grounded fact count=1、ordered path=unproven 和 local-fact boundary 保留。

状态更新：`EVAL-B54-EVROLE1 = implemented / full-related-tests-pass / production-replay-next`。
验证：`go test ./internal/types ./internal/agent ./internal/orchestrator -count=1` 全绿
（types 22.355s、agent 2.559s、orchestrator 8.669s）；`git diff --check` 通过。

证据：

- `eval/parallel_selected_summary_evalcampaign_b54_trace_mixed_r2_20260803.md`；
- `eval/parallel_selected_summary_evalcampaign_b54_trace_mixed_r2_20260803_manual_audit.md`；
- result dirs：`eval/results/*-20260803-073808`；
- HTML：`.codrax/output/20260803-074121.991-60419.html`、
  `.codrax/output/20260803-074211.965-60418.html`。

#### B54-G：可见端点 materializer 退役（答案所有权红线）

继续沿“成文校验未通过”生产链反查，确认一条比互斥合同更直接的红线：
`normalizeRequiredMechanismAnchorCarriersWithContext` 会在模型漏写 required anchor 时，向最终答案
自动追加 `Key anchors/关键锚点` block、端点 item 和“typed 当前源码已/未精确解析该端点”的可见
中英文正文；之后 `preCheckRequiredMechanismAnchorSet` / call-chain endpoint hard gate 又读取这段
系统自产内容并签绿。这样形成了“系统代写 carrier -> 系统用自产 carrier 通过自己的校验”的自证环，
减少了重试，却越过了“系统给事实和约束、模型负责成文与结论”的答案所有权边界。

历史核对：端点 carrier 自动补写的早期形来自 2026-05 的 `20d917fe2d` / `8ef0e28b98`；
`9745226b3`（2026-08-01）把它扩展到全部 `QFCallChain`，并新增 exact resolved/unresolved 可见
披露，使该红线在当前 call-chain 主路径上稳定可触发。根因不是单个词面，而是 correctness gate 与
repair path 没有 ownership 分层；“减少 retry”的局部目标把 typed request obligation 错当成了系统
创作可见答案的授权。

新增 `EVAL-B54-ENDPOINTAUTHOR1`（P0/red-line）并完成第一批根修：

1. 从 pre-emit normalization pipeline 删除 required mechanism anchor 的可见 materializer，并删除
   自动建块、自动 item、自动 citation 和 resolved/unresolved prose 全链；
2. 保留 analyzer/final handoff 中的 typed `RequiredMechanismAnchors`、模型可执行的 skill prompt 要求，
   以及只读 structured item carrier 的 precise hard gate。模型漏端点时可以据 typed hint 自行修订，
   系统不再代写答案来消灭 retry；
3. 新增所有权结构 pin：归一化前后可见 `AnswerDocumentV2` 字节等价；缺端点时
   `ViolCallChainEndpointOmitted` typed hard hint 必须仍在，防止未来以“降校验”换取少重试；
4. 不读取用户原文、模型 prose 或 Mermaid label 来铸造权限；不改 Trace runtime projection、显式
   时间窗因果补采或系统拥有的 typed Trace supplement。

同类面初筛显示仍有三类需要下一批逐项裁定，不能因本批通过就宣称全系统已无同类问题：

- metadata/citation-only repair：只修 `citation_ref`、行范围、hidden edge metadata，原则上不拥有
  结论，但仍需证明不会改变可见 claim identity；
- visible identity rewrite：`normalizeQualifiedItemLabelsByUniqueEnclosingFunction`、
  `normalizeDiagramDefinitionLabelsByEvidence` 等会修改模型可见 label/diagram body，属于高风险；
- deterministic enumeration/member-set compilers：可能追加或改写可见 row/block。只有明确标成
  system-owned typed fact panel、且不生成因果/优先级/修复结论时才可能合规；否则与本件同形。

下一批按“是否改变可见正文/声明身份”而不是函数名或语言逐个分类；任何 D 类（修改/替换模型
可见声明）先撤出 shipping path，再补 typed prompt/hint。Trace 因果投影保留独立 owner，不与源码
answer normalizer 混审。

验证：`go test ./internal/tool -count=1` 全绿（158.635s）；`git diff --check` 通过。

#### B54-H：shipping 可见正文重写器统一断路（跨语言答案所有权）

`ENDPOINTAUTHOR1` 之后继续枚举 full emit、patch emit、recovery 共用的 normalization pipeline，
确认同一红线不是端点单例。以下旧路径会直接改写或删除模型的可见表达：

| ID | 旧行为 | 最早引入点 | 判定 |
|---|---|---|---|
| `EVAL-B54-DIAGRAMAUTHOR1` | 按 Evidence definition 自动替换 Mermaid 节点 label、路径和行号 | `55f1f8229` | confirmed red-line |
| `EVAL-B54-LABELAUTHOR1` | 把模型 item label 的 owner/method 改成 citation enclosing function | `db4bf9cba` | confirmed red-line |
| `EVAL-B54-LOCATIONAUTHOR1` | 正则扫描 title/text/item/cell/diagram 后改写可见 `path:line` | `1b2538bb9` | confirmed red-line |
| `EVAL-B54-SURFACETERMAUTHOR1` | 把 ArkTS decorator、Cangjie/其他语言声明词等 evidence surface term 拼入模型 item/table | `42efa7dce` | confirmed red-line |
| `EVAL-B54-DECISIONAUTHOR1` | runtime-only 车道直接删除模型 `BlockDecision` | `6eb0b67fe` | confirmed red-line |
| `EVAL-B54-SENTINELAUTHOR1` | 正则扫描模型正文并把 `citation_ref=-1` 等片段替换成系统文案 | `697301314` | confirmed red-line |

这些实现大多源自合理的局部目标（修正 citation、避免内部字段泄漏、减少 final retry），但把
“系统知道一条精确事实”错误推导成“系统有权改写模型怎样表达该事实”。结果与端点件同根：校验、
repair、render 三层没有统一的 answer-owner 类型，局部 normalizer 被当成低成本 rewrite engine。

本批按数据所有权统一施工，而非按 Java/ArkTS/Cangjie 或某个 eval 词形拟合：

1. full emit、patch emit、recovery 共用链全部断开上述 visible mutator；diagram body、item label/text、
   markdown table、runtime decision 和普通正文不再被这些路径修改；
2. 精确 `surface_terms` 继续由 `preCheckModelSurfaceTerms` 给模型 typed soft advisory，不触发
   “成文校验未通过”，也不拼进正文；源码 identity/call relation 继续走现有 typed prompt、citation
   metadata 与 precise hard contract；
3. `normalizeDiagramEdgeAnchorMetadata` 只保留 hidden typed metadata 对齐，并加 pin 证明 Mermaid body
   字节不变；citation pool、行范围、quote 等非可见 provenance repair 保留；
4. 新增 AST shipping tripwire，直接检查 `executeAnswerDocumentV2`、patch `Execute` 和共享
   `normalizeAnswerDocumentForPreEmit` 不得调用七类可见 mutator。该测试扫描仓内 Go AST，不扫描用户
   原文或模型答案内容；
5. Trace runtime typed projection、显式时间窗自动补采和标记为 `SystemGeneratedKind=runtime_trace`
   的系统 supplement 保持不变。系统可在独立、明确标识的 typed fact panel 中发布事实，但不能借此
   删除或改写模型结论。

验证矩阵包含 ArkTS attached decorator、source-inventory surface term、runtime trace decision、VCS/
command-only 文本、Mermaid metadata 和 call-chain endpoint；全部改为“模型可见内容保持 + typed 信息
仍在”的结构断言。`go test ./internal/tool -count=1` 全绿（159.821s）；`git diff --check` 通过。

仍需下一批处理的确认项是 `EVAL-B54-ENUMAUTHOR1`：
`normalizePrincipalEnumerationRowBlocks` 仍会在非 source-inventory 主车道改写 markdown/structured row、
剪除 model item、合并 summary；其追加块已有 `SystemGeneratedKind`，但“独立系统 supplement”与
“就地修改模型 block”混在同一函数。最优方案是只保留单独标记的 typed supplement，删除对模型
 block 的 in-place rewrite/prune；随后再审 `normalizeViewCompatibleAnswerDocument` 中所有删除型分支，
区分无损 schema cleanup 与可见结论删除。

#### B54-I：enumeration compiler 拆成 append-only system fact lane

`EVAL-B54-ENUMAUTHOR1` 冷读确认：shipping 原来依次调用
`compileEnumerationDisplayTableRows`、`normalizeEnumerationDisplayRequestedFieldSurfaces`、
`normalizePrincipalEnumerationRowBlocks`。三者会填充模型 table cells、清空 item text、追加列、修改
summary/count、重写 markdown/structured row、剪除所谓 extraneous item，再在必要时追加系统表。
`normalizeAggregateMemberSetCarriers` 还会给模型现有 list/table 就地补 title、`SurfaceRole`、facet 和
claim-use。即使成员值来自 typed authority，这种混合仍让 presentation compiler 实际拥有了模型答案。

本批完成 owner 分离：

1. shipping pipeline 不再调用三个 in-place enumeration normalizer，统一改为
   `appendPrincipalEnumerationTypedSupplements`；它只读取 typed display set 与结构化覆盖，缺成员时追加
   `SystemGeneratedKind=principal_enumeration_missing` 的独立 block，不编辑、删除、重排任何既有 block；
2. aggregate member-set 完整但缺 relation/category label 时，不再给模型 carrier 就地补 title/facet，
   改为追加 `principal_enumeration_rows` 系统块；普通已覆盖 member set 不重复补；
3. aggregate member-set 与 principal support surface-term 的所有新增块都显式设置不可由模型提交的
   in-memory `SystemGeneratedKind`。系统块只携带 accepted typed member/value/source facts，不生成原因、
   优先级、修复方向或模型结论；
4. AST ownership tripwire 新增禁止 shipping 重连三个 legacy in-place compiler；partial markdown 回归把
   原模型 blocks 做 JSON 字节对比，relation list/table 回归分别锁定 title/text/items/facet/claim-use
   不变，并核验新增块 owner；
5. source-inventory principal model-owned 车道、Trace runtime projection、显式时间窗与因果补采保持原
   owner，不进入本编译器。

这保留了完整枚举的 deterministic 价值，同时消除了“系统修改模型表格以通过完整性门”的自证环。
验证：`go test ./internal/tool -count=1` 全绿（155.730s）；`git diff --check` 通过。

`EVAL-B54-VIEWAUTHOR1` 继续开放：`normalizeViewCompatibleAnswerDocument` 中 metadata-only 分支与
删除/合并可见 block 的分支仍混在一起。下一批只保留无损结构归一化；任何删除 model block 或清除
会改变渲染结论的字段，必须改成 typed guidance/校验，不能静默修正文档。

#### B54-J：persist/recovery 最后旁路与“合同自证”红线收口

沿 full emit 已通过的 normalization 继续追到共享 persist 和 rejected-draft recovery，确认
`VIEWAUTHOR1` 不是单个兼容函数问题，而是一组同根的合同自证路径：

1. pre-emit 已切到 append-only enumeration supplement，但 persist 又重新调用旧
   `compileEnumerationDisplayTableRows` / `normalizePrincipalEnumerationRowBlocks`，能绕过前置 AST pin，
   就地修改或剪除模型行；
2. `dedupeVisibleAnswerBlocks` 对所有 owner 生效，能以“语义等价”为由删除模型 block；失败草稿也先
   经同一去重器，导致下一轮看到的并非模型真实失败载荷；
3. full、patch、text recovery 和 persist 都会把 summary 移到首位；
4. `normalizeViewCompatibleAnswerDocument` 会清空模型 verdict、合并 summary、删除 scalar、删除
   `exact_resolution`，还会替模型补 `claim_use/facet_id`，随后同一合同读取这些系统自产字段签绿；
5. `normalizeObservedArtifactClaimUseCarriers` / `normalizeCitationBackedPrincipalClaimUses` 同样会根据
   合同允许集合替模型选择 claim form，属于非可见 metadata 上的同构自证；metadata 虽不直接显示，
   却决定后续 hard gate，因此仍越过答案所有权。

这解释了“为什么会出现互相冲突的校验合同”：历史优化以减少 retry 为局部目标，把三种 authority
混在一份可变文档里——模型答案、系统事实补充、合同控制字段；同时探索期与 final authority 又各自
独立累积、缺少 supersede/联合可满足性约束。于是旧合同可要求某个声明保留，final 合同又按新 roster
拒绝它；normalizer 再通过删改答案或自铸标签让其中一侧局部变绿。每个单测只验证自己的 happy path，
没有验证跨生命周期 interleaving 和 owner 不变式，因而长期未被发现。

本批按 owner 而不是具体答案词形完成根修：

- persist 只可追加带 `SystemGeneratedKind` 的 enumeration/member-set typed supplement；禁止重新连接旧
  in-place compiler，并在 supplement 前后对 model-owned block wire 做精确等价校验；
- 去重域收窄为 system-generated block 之间，模型提交的重复/相似 block 和原顺序均保留；rejected
  draft 原样保存；
- 删除所有 shipping summary reorder 调用和实现；full/patch/recovery/persist 不再替模型决定叙事顺序；
- view compatibility 变为只读 chokepoint，不再清 verdict、合并/删除 block、删除 exact resolution 或
  补写 claim/facet；缺失/冲突由 typed validator/prompt 指引模型自行修订；
- observed-artifact/citation-backed claim-use 自动铸造退出 shipping；精确结构本身已足够证明的合同应由
  validator 直接识别，不需要先写回模型 payload；
- AST tripwire 扩到 persist、row-normalization、view compatibility 和 text recovery；明确禁止上述
  mutator 重新接回。回归覆盖 full、patch、rejected draft、summary order、verdict/scalar/exact resolution、
  claim/facet 和 system-only dedupe。

状态：`EVAL-B54-VIEWAUTHOR1 = implemented / full-tool-pass`；新增并关闭
`EVAL-B54-PERSISTAUTHOR1`、`EVAL-B54-DRAFTAUTHOR1`、`EVAL-B54-METASELFCERT1`。
验证：`go test ./internal/tool -count=1` 全绿（157.181s）。Trace 的显式时间窗、模型探索结果和系统
补采仍是两个事实来源；本批不修改 Trace query、causal projection 计算或 system-owned runtime trace
supplement，只禁止它们回写/删除模型答案。

尚不能宣称“全系统不存在类似问题”。静态枚举后保留三组后续审计项：

- `EVAL-B54-TABLEAUTHOR1`（P0 待核）：`compileCitationBackedTableRows` 仍可能把 incomplete table carrier
  编译为可见行；需判断是纯结构恢复还是替模型创作表内容，若是后者改为独立 system block；
- `EVAL-B54-SUPPORTAUTHOR1`（P1 待核）：principal support member/surface supplement、required caveat
  materializer 需逐个证明只追加有 owner 标记的事实块，且不能成为自身 gate 的唯一通过证据；
- `EVAL-B54-SCHEMAAUTHOR1`（P1 待核）：block ID 去重、diagram kind 修正、reserved runtime ID 改名属于
  schema 容错，不直接写结论，但仍会改变模型载荷；最优方向是 schema reject + 精确修复提示，或把
  canonical form 限在不可见存储层，不能改变渲染/合同身份。

后续判据冻结：任何系统 pass 只允许二选一——修改纯 provenance/不可见且不参与语义确权的字段，或
追加明确标记、只含 typed fact 的 system block；凡是删除、重排、改写模型可见内容，或补写会被后续
gate 当成模型声明的 metadata，均按红线处理。减少“成文校验未通过”必须来自合同联合可满足性、final
authority 单源和精确结构直接判定，不得来自系统代写答案。

#### B54-K：system fact owner 补齐与 prose→hidden carrier 自证断路

按 B54-J 的开放项继续冷读，结论如下：

- `EVAL-B54-TABLEAUTHOR1` 可关闭：当前 `compileCitationBackedTableRows` 已是明确 no-op；旧的
  cells/columns/text 编译实现不在 shipping path，后续只允许 renderer 对既有空列做显示压缩；
- current-source anchor、aggregate negative proof、required uncertainty caveat 都是系统依据 typed
  evidence/coverage 追加的事实边界，方向合规，但原 block 没有 `SystemGeneratedKind`，经过 snapshot /
  patch recovery 后会被误认成模型声明，并进入 model-owned dedupe/contract 域；
- `normalizePrincipalSupportMemberCarriers` 会扫描模型可见 text，命中 support member 后修改现有
  `citation_ref/id` 或追加不渲染的 hidden item；后续 coverage gate 再读取这个自产 carrier。即使用户看
  不到 hidden item，它仍决定 hard/soft contract 结果，属于 `METASELFCERT1` 的同构旁路，并违反“不以
  模型输出原文扫描结果做硬门/铸权”。

本批施工：

1. 新增不可由模型提交的 `evidence_supplement` 与 `evidence_scope` owner；current-source、negative-proof
   和 scope caveat 三类系统块均显式标记，snapshot sidecar 会保持 owner；
2. principal-support hidden carrier normalizer 退出 shipping 并加入 AST 禁回接清单；模型已有精确
   structured carrier 时 validator 直接识别，只有 prose mention 时保持 soft typed guidance，不扫描
   prose 铸造通过条件，也不触发“成文校验未通过”；
3. system fact 仍可作为独立补充发布，但不得删除/改写模型内容或伪装成模型 claim；owner 回归分别
   覆盖源码锚点、负向证明和范围 caveat。

状态：`EVAL-B54-TABLEAUTHOR1 = closed/already-no-op`，
`EVAL-B54-SUPPORTAUTHOR1 = partial-implemented`（hidden member carrier 已断；system fact owner 已补；
recovered diagram metadata 仍并入 `SCHEMAAUTHOR1` 继续审计）。

#### B54-L：recovery 只恢复模型内容，不继承合同声明

`SCHEMAAUTHOR1` 第一项确认：malformed/text recovery 能从模型原始载荷找回完整 Mermaid body，这是
必要的无损恢复；但 `promoteRecoveredDiagramBlocks` 随后读取 `RequiredBlocks`，把合同中的
`FacetIDs/SurfaceRoleHint` 写到恢复块上。实际 diagram body 属于模型，facet/role 却由系统代选；后续
facet/role gate 再消费这些字段，仍是 metadata self-cert，只是藏在 recovery 车道。

本批保留 recovered diagram 的 ID、diagram kind/language/body 和显示位置，不丢图；删除从 contract
向恢复块复制 facet/role 的逻辑。实际 diagram block 可以直接满足精确的 kind/shape 合同；缺失的
facet/role 由 typed advisory/hard policy 提示模型，不由恢复器自签。回归锁定 diagram body 保留、
FacetIDs/SurfaceRole 均为空。

状态：`EVAL-B54-SUPPORTAUTHOR1 = implemented`；`EVAL-B54-SCHEMAAUTHOR1` 仍开放两项低一档风险：
纯内部 block ID 容错和“diagram payload 明确存在时将 discriminator 归一为 diagram”。二者不写可见
事实或结论，且前者不参与 claim authority、后者由 payload shape 精确蕴含；暂按 P1 继续异构回放，
不与本轮红线可见/metadata 自证混为 P0。

#### B54-M：schema-owner 两项最终裁定与 persist 兜底删除

针对用户追问，`SCHEMAAUTHOR1` 两项已逐条完成，不再只记观察：

1. **block ID 容错**分成三种精确信号：缺失 ID 只生成不可见 patch handle；ID list 按集合语义 trim/
   去重；完全相同 typed block 且 ID 相同只视为 transport stutter。它们均不读取用户原文或模型 prose
   语义，不改变 title/text/items/diagram/facet/claim/role。非完全相同的重复 ID 保留并由 shared
   validation 精确拒绝。runtime reserved-ID 碰撞仅把 unmarked model block 移到 `model_` 内部命名空间，
   不授予 system owner；新增结构 pin 把 ID 恢复后要求整个 block `reflect.DeepEqual`，并要求 visible
   surface 字节恒等。该类属于安全 schema canonicalization，不是答案改写或合同自证；
2. **diagram discriminator**只保留 raw emit decoder 的无歧义 canonicalization：模型实际提交了非空
   typed `diagram` object 时，`kind=diagram` 是 payload shape 的精确蕴含，且 body/title/metadata 不变。
   shared persist 原先还有第二个 `normalizeMergedDiagramPayloadKinds` 兜底，能让 patch/internal typed
   document 绕过正常 decoder/validation；本批删除该调用和实现。现在 typed persist 的 stale kind 会以
   精确 `kind=diagram` 修复信息拒绝，不静默改 metadata；raw decoder 的 shape canonicalization继续由
   既有正/负 pin 约束（不会从普通 text 或 Mermaid 关键词推导 diagram）。

状态：`EVAL-B54-SCHEMAAUTHOR1 = implemented/closed`。至此 B54 当前已确认的答案所有权红线均完成代码
收口；后续进入两例一批的异构生产 eval，若只出现模型措辞波动则记 watch，不新增原文关键词 gate。

## EVAL-B55：Trace/混合读回放与重复格式合同收口（2026-08-03）

### B55 r1：两例 PASS，但 typed 上下文存在重复义务与重复行

在 `main@6c815a4f9` 重建二进制后，以 `PARALLEL=2` 同时回放：

- `trace_query_wakeup_causal_runnable`：runner PASS，130s，actual final reject=1；
- `read_combo_trace_current_source_explanation`：runner/human PASS，180s，actual final reject=0。

Trace 核心能力没有回退：显式 1.000–1.010s 用户窗、3 次窗口化/目标过滤查询、
worker-200→app-100 唤醒链、#1 优先级反转候选 8.300ms、目标 10.000ms 状态分区、实际占时与
现规则可消两轴、Trace 因果投影和成文前确定性补采均完整。混合读例也不再把 `MarkerPID` 字段定义
升级为同步 B/E 配对键；最终答案使用匿名 E/同 ftrace 线程栈闭合，并把 `heavy-compute` 明确限制为
pretriage navigation candidate。

本轮确认两个同根的模型前上下文/格式合同 GAP：

| ID | 级别 | 机制 | 泛化处置 | 状态 |
|---|---:|---|---|---|
| `EVAL-B55-RELCOPY1` | P1/red-line | Explorer 已在 `emit_investigation_complete.relation_claims` 提交并通过同一 10.000ms closed-partition authority；Finalizer 又被硬要求把隐藏 metadata 机械复制到 block。首稿可见结论和值均正确，却稳定触发一次“成文校验未通过”。该 hard gate 不检查 prose，也不增加可见结论证明，且与 B26 冻结的“Trace Decision Inputs 只作 prompt，不作 hard gate”冲突 | 保留 Explore closure 的精确 relation 校验；Finalizer 的 relation carrier 改为 optional。未提交不重试；一旦模型主动提交，仍以最终 typed authority 精确验证 unknown ID、成员、关系、加法与 subtotal。handoff/schema 同词面说明“决策输入不是复制义务”。不读取/改写模型 prose，不自动搬运 claim | implemented / targeted-tests-pass / replay-next |
| `EVAL-B55-CTXDEDUP1` | P1/context | exploration 与 deterministic supplement 对同一窗发布同一 seat/census，但 final prompt 的 root-cause board 以 record ID 当席位，#1 重复两次；target wakeup census 以 query-result scope 分组，同一 roster 重复三次。72K final prompt 因而放大同一事实并制造虚假多源权重 | board 按完整 typed 席签名去重；target census 按 target/window/complete/total/split/有序 pair/timestamp 精确去重，明确排除 provenance scope/record ID。任何值、口径、窗或 roster 差异都保留为独立 authority，禁止相似度合并 | implemented / targeted-tests-pass / replay-next |

人工审计另记两个模型观察项，不升级成系统硬门：

- `EVAL-B55-PRIOWORD1/model-watch`：正文同一节先写 41–159=RT，随后把 idle/1、idle/2 的
  prio=120 称为 system；同页 typed facts 明确为 `prio=120/ohos_rt`，finalizer 输入也只有正确归一化
  三行，因此属于一次模型内部措辞矛盾。禁止扫描“prio=120/system”等答案词面做硬拒；后续换线程名、
  优先级值和语言再回放，若重复，优先提升通用 typed enum carrier/显著度；
- `EVAL-B55-MARKERWORD1/model-watch`：混合读例个别“H: 名称前缀属于平台”和“同进程重叠可能歧义”
  句子宽于本次源码引用，但核心 parser 机制与证据边界正确；继续用 B/E、S/F、G/H 异构 marker 观察，
  不按 `H:RenderService` 专名拟合。

代码不变量：本批不改 trace_query、根因排序、唤醒链、窗选举、因果投影、系统自动补采或
`SystemGeneratedKind=runtime_trace` 发布；不扫描用户原始输入、模型 thinking/summary/final prose；系统
不删除、重排、替换模型 block。精确去重只作用于模型前 typed handoff，且冲突形保持可见。

证据：

- `eval/parallel_selected_summary_evalcampaign_b55_ownerauth_r1_20260803.md`；
- `eval/parallel_selected_summary_evalcampaign_b55_ownerauth_r1_20260803_manual_audit.md`；
- result dirs：`eval/results/*-20260803-181328`；
- 定向验证：`internal/context` board、`internal/types` census、`internal/agent` handoff、
  `internal/tool` relation pipeline 均通过。

### B55 r2：调用链终点有向权威与写模式补证引用（read/write 各一例）

在 `main@2fd40538c` 固定二进制并行回放：

- `qf_sequence_analyzer_gate`：runner PASS / human FAIL，183s，investigation 3/1，final reject=2；
- `github_issue_commons_lang_random_ascii_symptom`：runner FAIL，393s，最终
  `write_final_verdict=unverified:verification_incomplete`。

读例确认 `EVAL-B55-CALLSINK1`（P0/red-line）：源码只有
`buildAnalysisIR -> gate.RunWith`；`gate.Run` 自身再调用 `RunWith`，方向与用户请求终点相反。旧链路有三层
同根误判：Explorer 的 terminal coverage 接受任意定义/支持条目；endpoint matcher 以 contains/prefix
把 `RunWith` 当 `Run`；pre-complete 只检查同文件端点跨度，不验证 typed call-edge 的 source→sink 有向
可达。于是探索 completion reason 错误宣称完整链；最终图表硬门正确删除 `RunWith -> Run`，但 summary 和
ordered item 仍保留同一错误结论，runner 的 answer_regex 未发现。

本批按关系图权威而非该 Go 符号特判施工：

1. bounded source call-chain 的 terminal readiness 只接受指向终点的 grounded `ClaimCallEdge`；定义、
   Owner、read-window 命中只作导航/身份证据，不能铸造 reachability；
2. shared endpoint compatibility 删除 contains/prefix sibling，短名/限定名只允许完整 token 或 exact tail；
   `RunWith` 不再覆盖 `Run`；
3. pre-complete 从所有 accepted source-code call-edge 构造语言无关有向图，支持 `. / :: / #` identity
   presentation，且只在 typed `ExactTargets + PredicateAxis=call + ReqCallChain` 上启用；runtime artifact
   source 和 `QFRootCauseTrace`/显式时间窗 Trace 因果投影不进入该合同；
4. 有向图无路时不逼模型造边：新增模型声明的 typed
   `principal_span_waiver.reason=no_directed_path`。首次 completion 精确提示补 call edge 或声明边界；模型
   声明后系统只携带 reason/rationale 与 caveat 给成文模型，禁止把定义改成 call edge，不代写最终结论；
   若 typed graph 后来出现路径而旧 `no_directed_path` 仍在，则精确拒绝并要求 clear；
5. 15 种 `SupportedReadLanguages`（Go/Python/JS/TS/Java/Kotlin/Rust/C/C++/Ruby/Swift/Lua/Proto/
   ArkTS/Cangjie）共用一张 directed-path 参数化测试；另有 `RunWith != Run`、反向 wrapper、模型边界携带
   与 runtime 排除 pin。

状态：`EVAL-B55-CALLSINK1 = implemented / full-types-agent-tool-pass`。这不是答案关键词门：判据只读取 analyzer
typed shape、accepted EvidenceItem 的 ClaimForm/Subject/Object/Source 和模型显式 typed waiver；不扫描用户
原文、thinking、completion reason 或最终正文。

写例确认 `EVAL-B55-VERIFYREF1`（P1）：功能 patch 已应用，replan 后 `make check` 和 Python
source probe 通过，但 Maven 不在环境且 Java 未编译/运行，最终 fail-closed 是正确行为。系统浪费来自
proof-followup 计划遗漏 `contract_refs/changed_symbol_refs`，直到末端 `changed_path_verification_uncovered`
才再次发现；同时 Python 正则读取 Java 只能拥有 `source_static`，不可升级为 Java behavior/target execution。
本批随后完成施工：只在 typed verify-failure handoff 的 reason/Confidence 明确为
`changed_path_verification_uncovered` 时，提取其中 exact `path:` refs，并在计划正式进入 apply/verify 前要求：

1. 对系统存在 inline probe executor 的未覆盖路径，至少有一个 probe 显式携带完全相同的
   `changed_symbol_refs=path:...`；
2. 仅当系统确有该目标语言的 inline probe executor（当前 Python/JavaScript/Ruby/Java/Go）时，probe
   language family 必须与目标文件 language family 相交；Python 等跨语言 source regex 即使读取 exact Java
   路径也不能取得 target execution/behavior authority；
3. 若已有 required behavior contracts，目标语言 probe 必须显式绑定至少一个有效 `contract_refs`；
4. 缺字段返回 `PlanRepairPack.reason_code=verification_probe_proof_followup_refs_failed`，在计划轮直接修复，
   不再走完昂贵 apply/verify 后重复同一失败。

判据不从 probe code、用户原文、summary 或模型说明推断目标，也不由系统自动补写签名 metadata；普通计划、
无 typed handoff、其它 failure reason 均不触发。C/C++、Rust、Kotlin、TypeScript、ArkTS、Cangjie 等当前没有
inline executor 的语言不继承不可满足的“目标语言 probe 必带”合同，其 target execution/behavior 仍由项目原生
runner 与 test surface 证明；跨语言 probe 只保留 `source_static` 辅助等级。状态：
`EVAL-B55-VERIFYREF1 = implemented / full-tool-pass`。

证据：

- `eval/parallel_selected_summary_evalcampaign_b55_readwrite_r2_20260803.md`；
- `eval/parallel_selected_summary_evalcampaign_b55_readwrite_r2_20260803_manual_audit.md`；
- result dirs：`eval/results/*-20260803-183504`。

验证：CALLSINK 批 `go test ./internal/types ./internal/agent ./internal/tool -count=1` 全绿
（tool 170.133s）；VERIFYREF 批 `go test ./internal/tool -count=1` 全绿（166.083s）；
`git diff --check` 通过。

## 56. 2026-08-03 B55 r3 双例复放：member_set 有向权威旁路（CALLMEM1）

在 `main@248a206a4` 固定二进制并行复放（严格 parallel=2）：

- `qf_sequence_analyzer_gate`：runner PASS / human FAIL，385s，Explorer 13 轮，finalizer reject=6；
- `github_issue_commons_lang_random_ascii_symptom`：runner FAIL / human uncertain，535s，最终
  `unverified:verification_incomplete`。

读例暴露 `EVAL-B56-CALLMEM1`（P0/red-line）：CALLSINK1 的 typed directed-path 校验实现本身正确，但生产
接线放在 `callChainAggregateMemberSetCompletesPrincipalBoundary(...) == false` 的分支中。Explorer 发出一个
逐项有 support_refs 的 principal `member_set` 后，旧分支把“成员清单完整”同时当成“跨度闭包完整”和“有向
终点已证”，直接跳过 exact reachability。因此现有证据 `buildAnalysisIR -> gate.RunWith` 加反向 wrapper
`gate.Run -> gate.RunWith` 仍被接受为到 `gate.Run` 的链。Finalizer 的 call-edge 门正确连续拒绝错误终点，
但六轮耗尽后 `answer_document_retry_state_recovered` 发布了未通过结构化校验的旧草稿；eval regex 又把它判成
PASS，形成 runner false green。

最优修复保持两个合同正交：

1. exact endpoint directed reachability 先于任何 aggregate shortcut；它只读 typed
   `ExactTargets + PredicateAxis=call + ReqCallChain` 与 grounded source-code `ClaimCallEdge` 图；
2. principal member_set 仍可关闭“内部跨度是否已枚举”的重复补采，但只能在有向可达已经成立、或模型显式
   声明 typed `principal_span_waiver=no_directed_path` 后生效；
3. 新 pin 证明：完整且逐项引用的 `member_set` 不能把 `RunWith`/`Run` sibling 与反向 wrapper 铸造成路径；
   另一个正臂证明真实 source→middle→sink 图存在时 member_set 仍能跳过冗余 span expansion。

本修复不读取用户原文、模型 reason/thinking 或最终答案，不影响 runtime Trace、显式时间窗 Trace 因果投影、
系统补齐或答案所有权。状态：`EVAL-B56-CALLMEM1 = implemented / targeted-tests-pass`。

同时立案 `EVAL-B56-EVALGREEN1`（P1/open）：eval runner 在日志明确
`answer_document_retry_state_recovered`、结构化检查被跳过时仍只靠 answer regex 判 PASS。应在独立小批把 typed
degraded-finalize 状态接入 eval verdict；不能通过放松调用边证据门解决。

写例不立 verifier bug：第二计划的 patch 命中 fixture 静态 oracle，`make check` 通过；但 `javac`/Maven
不可用，Make/Python checker 仅有 `source_static` 权限，不能签 Java target execution/behavior。最终 unverified
诚实且应保留；不得为 eval 变绿提升静态检查权限。

证据：

- `eval/parallel_selected_summary_evalcampaign_b55_readwrite_r3_20260803.md`；
- `eval/parallel_selected_summary_evalcampaign_b55_readwrite_r3_20260803_manual_audit.md`；
- result dirs：`eval/results/*-20260803-192814`。

验证：`go test ./internal/tool -count=1` 全绿（169.749s）；`git diff --check` 通过。状态更新为
`EVAL-B56-CALLMEM1 = implemented / full-tool-pass`。

### B56-S：降级答案不可被普通答案 oracle 误签绿

`EVAL-B56-EVALGREEN1` 已在独立 eval-infra 小批关闭。runner 现在从每轮专属日志目录聚合后的控制面日志读取
唯一精确信号：orchestrator 以 WARN 发布“finalizer returned degraded answer; skipping structured answer checks”时，
普通 read case 追加 `degraded_answer_checks_skipped:N` 失败原因。答案正文即使包含预期 symbol、通过
`EXPECT_CONTAINS`/regex，也不能掩盖该轮没有完成结构化校验的事实。

边界与防过拟合约束：

1. 计数器要求完整时间戳、WARN level、`[orchestrator]` owner 和固定控制面事件前缀；用户输入、模型
   thinking/final、引用文字中即使逐字复述 reason 也不会命中；
2. 这只是 eval verdict，不改变产品 finalizer、答案文本、Trace query/投影/补采，也不放松任何现有合同；
3. 专门验证 degraded recovery 本身的 case 可显式设置 `ALLOW_DEGRADED_READ_ANSWER=1`，避免把故障车道的
   针对性测试变成不可满足合同；默认值仍为 fail-closed；
4. metrics 新增 `degraded_read_answer_check_skips`，后续两例回放可区分“答案内容错误”与“结构化校验已被
   跳过”，不再由弱 regex 制造 false green。

回归包含两个关键反例：模型 DEBUG prose 逐字引用事件时计数仍为 0；真实 orchestrator WARN 与相同
答案 oracle 并存时 verdict 必为 `FAIL degraded_answer_checks_skipped:1`。显式 opt-out case 必为 PASS。

状态：`EVAL-B56-EVALGREEN1 = implemented / runner-contracts-pass`。验证：
`bash -n eval/run.sh eval/runner_lib.sh eval/runner_lib_test.sh` 与 `bash eval/runner_lib_test.sh` 全绿；旧 r3
生产 witness 的控制面精确计数为 1，证明新 verdict 能阻止该次历史 false green，而不会回读模型原文。

## 57. 2026-08-03 B56 r4：typed 双端点 fallback 与真实 Python 写闭环

在 `main@67f9cdc45` 重建二进制后严格并行 2 例：

- `qf_sequence_analyzer_gate`：runner PASS / human FAIL，285s，Explorer 8 轮、completion 3 次、finalizer
  reject=4，`degraded_read_answer_check_skips=0`；
- `github_issue_dateutil_relativedelta_float_symptom`：runner/human PASS，232s，写模式最终
  `verified:all_batches_verified`。

### EVAL-B57-CALLTYPED1（P0）：可选 ExactTargets 缺席会关闭有向终点合同

r4 证明 CALLSINK1/CALLMEM1 仍有第三条生产旁路。Analyzer 已发出精确 typed 形态：
`ReqCallChain + PredicateAxis=call + IsRelationalLookup=true`，`MentionedEntities` 去掉两个 `.go` 路径后恰好是
`buildAnalysisIR` 与 `gate.Run`；但本轮模型漏填了可选 `ExactTargets`。旧
`completionExactCallChainEndpointShape` 以 `len(ExactTargets)>0` 作为整个有向图检查的前置条件，因此跳过
已经正确实现的 directed-path oracle。Explorer 最后一次不带 member_set 的 completion 被接受；答案继续把
唯一已证边 `buildAnalysisIR -> gate.RunWith` 叙述为到达 `gate.Run`。四次 finalizer reject 只修复图中每条
call edge 的锚点形态，没有纠正终点身份。

本批按 typed endpoint authority 根修：

1. `CallChainRequestedEndpointHints` 改为优先消费 `AnalyzerHints.ExactTargets`；只在 exact lane 过滤后没有
   任何符号时，才按 `MentionedEntities -> PrimaryEntities -> Entities` 回退。上下文实体不能覆盖明确端点；
2. 有向 reachability 新增独立 shape：显式 `ExactTargets` 仍直接启用；缺席时，只有
   `ReqCallChain + AxisCall + relational=true` 且 typed、非路径端点**恰好两个**才启用。三个及以上 fallback
   符号仍视为端点歧义，不硬判；
3. reachability shape 与 phase-1 forced-read shape 分离。后者继续要求显式 ExactTargets，避免 fallback
   实体把所有导航候选文件升级为阻塞读取；
4. prefix sibling、definition、member_set 和 completion reason 均不能铸造方向。无路径时仍由模型选择：
   补同向 `ClaimCallEdge`，或声明 typed `principal_span_waiver=no_directed_path` 并自行写出边界；系统不替写
   最终结论。

判据不读取 RawRequest、thinking、completion reason、final prose 或关键词；只消费 analyzer enum/boolean、
typed entity carriers 与 accepted `ClaimCallEdge`。参数化语言图仍由已有 Go/Python/JS/TS/Java/Kotlin/Rust/
C/C++/Ruby/Swift/Lua/Proto/ArkTS/Cangjie 套件覆盖；新增生产同形 pin 证明漏 ExactTargets 时
`RunWith != Run`，并钉住三 fallback 符号不启用硬门。

状态：`EVAL-B57-CALLTYPED1 = implemented / full-types-tool-pass / replay-next`。验证：
`go test ./internal/types -count=1` 全绿（22.525s），`go test ./internal/tool -count=1` 全绿（167.380s）。

### 写模式人工审计

Python 例改动仅触及 `relativedelta.py`：整数值 float 在构造期转 int，非整数 float 抛 ValueError；未修改
测试。verification probe 实际运行，随后 `python3 -m unittest discover -v` 真实执行并通过 4 个仓库测试，
合计 5 个 test result；changed path 权威为 `project_runner/target_behavior`，最终 verified 可信。

观察项 `EVAL-B57-PROBEDUP1/model-watch`：初始 probe 未带 contract_ref，报告记录
`verification_probe_missing_plan_contract_ref` 后继续真实 suite；suite 随后完整覆盖四项 outcome，因此不构成
权限或正确性 gap。它可能造成一次重复 suite/约 40ms 开销，先在不同语言与 replan 形继续观察；不得为了这
一个 fixture 给所有首计划新增不可满足硬合同。

证据：

- `eval/parallel_selected_summary_evalcampaign_b56_readwrite_r4_20260803.md`；
- `eval/parallel_selected_summary_evalcampaign_b56_readwrite_r4_20260803_manual_audit.md`；
- result dirs：`eval/results/*-20260803-195918`。

本批不修改 Trace query、显式时间窗、Trace 因果投影、系统补采或答案发布所有权。

## 58. 2026-08-03 B57 r5：调用链等价 typed 编码与显式窗 Trace 守护

在 `main@8fdbf60c9` 重建并严格并行 2 例：

- `qf_sequence_analyzer_gate`：runner PASS / human FAIL，315s，Explorer 9 轮、completion 1 次、finalizer
  reject=2；
- `trace_query_wakeup_causal_runnable`：runner PASS / human uncertain，142s，3 次 trace_query，finalizer
  reject=0，runtime authority=`perf_triage+trace_query`。

### EVAL-B58-CALLENC1（P0）：schema-valid 等价 predicate 编码不应改变安全合同

r5 的 analyzer 与 r4 不同：仍为 `ReqCallChain + PredicateAxis=call`，文件路径过滤后仍恰好两个 symbol endpoint，
但这次 `IsRelationalLookup=false / IsCrossComponent=true`。这是 `emit_analysis` 自一致检查明确接受的编码：
`AxisCall`、relational boolean、或两个 named endpoints 任一均可证明 call-chain 关系形。B57 fallback 又额外要求
relational boolean，造成 schema-valid 的同义 typed IR 获得不同安全检查。

生产结果证明该差异有实质影响：模型读到 `buildAnalysisIR -> gate.RunWith`，又读到
`gate.Run -> gate.RunWith` 的 wrapper 定义，却声称存在“外部调用者把 AnalysisIR 传入 gate.Run”，最终图把两个
不连通段拼成一条链。该外部 caller 没有 typed call edge；真实有向图不存在 `buildAnalysisIR -> gate.Run` 路径。

根修：two-endpoint fallback 不再要求冗余 `IsRelationalLookup`；只要求
`ReqCallChain + AxisCall + exactly two non-path typed endpoint hints`。这是与 analyzer schema 相同的 typed
disjunction，不是扩大到普通机制/架构问答。三个及以上 endpoint hint、非 AxisCall、非 ReqCallChain 仍不启用
硬有向检查；显式 ExactTargets 仍最高优先。测试直接复刻 r5 编码（cross-component=true、relational=false）并
证明 `RunWith` sibling 不能关闭 `Run`；三端点负臂保持 advisory。

系统只要求模型补已证同向边或声明 `principal_span_waiver=no_directed_path`，不写外部 caller、不修改模型答案、
不扫描 RawRequest/thinking/completion/final prose。状态：
`EVAL-B58-CALLENC1 = implemented / full-tool-pass / replay-next`；验证
`go test ./internal/tool -count=1` 全绿（168.795s）。

### Trace 守护结论与模型观察项

Trace 核心能力未受调用链源代码合同影响：

- 用户窗保持 1.000000..1.010000，三次 query 均有 window/target filter；
- worker-200→app-100 唤醒边、app-100 四态 10.000ms、worker 链累计 9.000ms、有效归因 8.300ms 分离；
- 模型正文出现“主要时间占用 / 关键路径候选”和“窗内可消除量”双轴，明确两轴不可相加/替代；
- `Trace 因果投影`、root-cause #1、代表窗、证据索引和成文前 root_cause_rank / critical_blocking_calls
  系统补采完整；系统块只对照 typed 事实，没有替换模型主结论。

新增 `EVAL-B58-TRACEWORD1/model-watch`：模型首段把工件跨度 10.020ms 写成目标阻塞时长，又说目标等待“完全由”
8.300ms runnable 造成；同页 typed context 实际已清楚给出 selected window=10.000ms、chain cumulative=9.000ms、
effective elimination=8.300ms，并说明主根因是量与凭证选举而非机理裁定。因精确信息足够且本轮零 reviewer /
contract reject，先判模型措辞波动，后续更换窗长/链深复放；不得扫描“完全由”或具体数值正文做产品硬门。

证据：

- `eval/parallel_selected_summary_evalcampaign_b57_tracecall_r5_20260803.md`；
- `eval/parallel_selected_summary_evalcampaign_b57_tracecall_r5_20260803_manual_audit.md`；
- result dirs：`eval/results/*-20260803-201544`。

## 59. 2026-08-03 B58 r6：端点实体词界权威与 Cangjie 清单守护

在 `main@1c453d9ce` 重建后严格并行 2 例：

- `qf_sequence_analyzer_gate`：runner PASS / human FAIL，240s，Explorer 9 轮、completion 4 次、finalizer
  reject=1；
- `cangjie_repomap_fixture`：runner/human PASS，95s，2 次 source-inventory lens、finalizer reject=1。

### EVAL-B59-CALLBOUND1（P0）：标点抹平子串会给派生实体铸造用户端点权威

r6 证明前两批的 directed-path 合同已经启用，但入口实体本身仍被污染。用户只点名
`buildAnalysisIR -> gate.Run`；Analyzer 额外发出上下文类型 `AnalysisIR`。公共
`MentionedEntitiesFromRawRequest` 旧实现同时做裸子串与“删除全部标点后的子串”，于是把
`buildAnalysisIR` 内部的词尾 `AnalysisIR` 判为用户独立点名。后果不是单一回答措辞问题，而是一条跨阶段级联：

1. typed endpoint hint 变成三个，two-endpoint 有向合同正确地拒绝猜测；
2. principal-span 兄弟车道仍把实体顺序当端点身份，选中 `buildAnalysisIR -> AnalysisIR`，而非用户终点；
3. Explorer 对该错误跨度连续三次 forced read，消耗 9 轮探索和 4 次 completion；
4. 最终答案仍把唯一已证的 `gate.RunWith` 写成 `gate.Run` 的“实际/底层函数”。真实源码只有
   `buildAnalysisIR -> gate.RunWith` 与反向 wrapper `gate.Run -> gate.RunWith`，不存在请求方向路径。

本批按公共实体权威根修，而非拟合 `AnalysisIR`：

1. `MentionedEntitiesFromRawRequest` 改用候选级、大小写无关的**完整词界**匹配；不再删除标点后做任意子串。
   `AnalysisIR` 不能从 `buildAnalysisIR` 获权，`Run` 也不能从 `gate.Run` 或 sibling 中冒充独立点名；路径、
   配置键、限定符号仍按其完整 surface 匹配；
2. 该入口只检查 Analyzer schema 字段中逐个 typed candidate 是否出现在当前 request surface，属于既有
   exact-target provenance sink；不枚举关键词、不分类用户原文，也不扫描 thinking/final prose；
3. source-code `ReqCallChain + AxisCall` 在**真实独立** typed symbol 超过两个时，要求 Analyzer 用
   `exact_targets` 成对声明 caller/source 与 callee/sink；上下文/中间实体继续留在 `entities`，实体数组顺序
   永不取得端点权限；
4. 调用链一旦使用 symbol `exact_targets`，只给一个端点也会在 Analyzer 边界被拒绝并要求成对或全部省略。
   这是为了防止 `CallChainRequestedEndpointHints` 的 exact-first 规则用单点覆盖完整双端点；path-only exact
   target 仍回退 typed entity lane；
5. 实际 r6 形态（只在长标识符内嵌一个派生词尾）现在保持 two-endpoint **零重试**车道；只有用户真的同时
   点名第三个上下文符号才请求一次精确 source/sink 声明，避免把端点猜测债推到探索/成文阶段。

状态：`EVAL-B59-CALLBOUND1 = implemented / full-types-tool-pass / replay-next`。回归包含内嵌 identifier 负臂、
独立第三实体歧义臂、双 exact 正臂、单 exact 拒绝臂；`go test ./internal/types -count=1` 全绿（22.986s），
`go test ./internal/tool -count=1` 全绿（171.567s）。本批不改 Trace query、显式时间窗、Trace 因果投影、系统
补采或答案发布所有权。

### Cangjie 人工审计与 EVAL-B59-INVROW1（P1/open）

Cangjie 清单答案正确：声明级 package 分别来自 `demo.bridge`、`demo.cart`、`demo.app`；完整列出
`Cart@30` extend、`native_add@6` foreign func，以及 `Bridge@15`、`Cart@14`、`App@11` 三个 public class。
这证明 source-inventory lens 对 Cangjie 的声明形与 package 载荷可支撑答案，不需要 Java 特判或正文接管。

但一次成文拒绝暴露泛化观察项：同一文件同名 `Cart` 同时有 class 行与 extend 行，principal row coverage
把模型标签 `Cart (extend 块)` 判成“typed principal row set 外”，要求删除/移出；patch 后同一条目又被接受。
这说明 exhaustive inventory 的可见 item identity 可能仍主要依赖 label，尚未稳定携带
`source + line + declaration-family/role` 复合身份。当前最终答案未丢条目，故不把它升级为正确性 P0；下一批应
用重载函数、同名声明跨 package、class+extension 三种异构 fixture 否证后，再决定是否统一修复 row identity。
禁止为 Cangjie 的 `extend` 字样增加答案关键词门。

证据：

- `eval/parallel_selected_summary_evalcampaign_b58_callcangjie_r6_20260803.md`；
- `eval/parallel_selected_summary_evalcampaign_b58_callcangjie_r6_20260803_manual_audit.md`；
- result dirs：`eval/results/*-20260803-202843`。

## 60. 2026-08-03 B59 r7：调用边短名/限定名同源权威与 ArkTS 清单守护

在 `main@b2b2e47d1` 重建后严格并行 2 例：

- `qf_sequence_analyzer_gate`：runner/human FAIL，418s，Explorer 27 轮/4 dispatch，completion 7 次，
  finalizer reject=6，runner 失败原因为 `degraded_answer_checks_skipped:1`；
- `arkts_repomap`：runner/human PASS，126s，4 个 `@Entry` 与 2 个 `@Builder` typed row 全部命中，finalizer
  reject=0。

### B59 replay：CALLBOUND1 生效，但暴露下一层编码不等价

词界修复已生效：Analyzer 虽仍把派生上下文 `AnalysisIR` 留在宽泛 `Entities/PrimaryEntities` 供导航，但它不再
从 `buildAnalysisIR` 的词尾取得 `MentionedEntities`/端点权限；directed-path 合同按真实双端点
`buildAnalysisIR -> gate.Run` 启用。Explorer 最终正确接受模型声明的 typed
`principal_span_waiver=no_directed_path`，并把结论冻结为：

- 最近已证请求方向止于 `buildAnalysisIR -> gate.RunWith`（analyzer.go:2666）；
- `gate.Run -> RunWith`（gate.go:135）是另一个真实、**反向** wrapper 关系；
- 禁止把 `gate.Run` 定义或 `RunWith` sibling 铸成 `buildAnalysisIR -> gate.Run` 的静态路径。

因此 r7 不再是 Explorer 猜错终点；runner 的 degraded 闸也正确阻止弱 answer regex 把失败答案签绿。

### EVAL-B60-DIAGQUAL1（P0/red-line）：同一 typed call edge 在短名与限定名编码间被连续拒绝

成文输入已经同时携带：

- citable call edge：`Run -> RunWith @ internal/analysis/gate/gate.go:135`；
- typed 用户端点：`gate.Run`（`RequiredMechanismAnchors`）；
- citable 限定 callee：`buildAnalysisIR -> gate.RunWith @ analyzer.go:2666`；
- 模型图：`participant Run as gate.Run`、`participant RunWith as gate.RunWith`，边方向仍为
  `Run -> RunWith`。

旧 diagram evidence resolver 只实现了“短 callee + 唯一定义 -> 限定 callee”的桥，caller 必须字节相等。
于是同一真实边在 Explorer 图中是 `Run -> RunWith`，在 Finalizer 图中是
`gate.Run -> gate.RunWith` 时被判 `call_edge_unproven`；principal completeness 又依据模型已选的 `gate.Run`
与 `gate.RunWith` 要求这条真实边必须可见。模型删除边会收到 `principal_call_edge_missing`，加回边又收到
`call_edge_unproven`，连续 6 次后降级出厂。从模型视角这是不可满足合同；从代码根因看并非两个语义规则冲突，
而是**同一精确事实的两种 typed 编码缺少同源等价桥**。

本批按语言无关 identity authority 修复：限定 caller 只在以下三项同时成立时可投影到短 call edge：

1. 限定 caller 字节等于 `AnswerSemanticView.RequiredMechanismAnchors` 中的 typed symbol（用户端点权限）；
2. 限定 callee 字节已出现在另一条 citable typed call record 的 Subject/Object/AnchorSymbol 中（不是图标签自证）；
3. 短 caller→callee 在 accepted evidence 中只有一个 `source + line + direction` call-site；同一短名出现在多个
   位置时继续 fail-closed。

该桥不读取 raw request、Mermaid message prose、thinking/final 或路径推断，不接受 prefix/sibling，不从定义生成
调用边；它只让已有 call edge 的 caller/callee presentation 与 typed endpoint owner 对齐。反例钉住：去掉
required caller authority 必须拒绝；增加第二个同短名 call-site 必须拒绝。所有 Go/Python/Java/ArkTS/Cangjie
等源码调用图共用此 resolver，不含语言特判。

状态：`EVAL-B60-DIAGQUAL1 = implemented / full-tool-pass / replay-next`；验证
`go test ./internal/tool -count=1` 全绿（175.352s）。本批不进入 `QFRootCauseTrace`，不改 Trace 时间窗、
因果投影、系统补采或系统生成块，也不修改/替换模型正文。

### ArkTS 守护与剩余 churn 观察

ArkTS 最终答案完整且准确：`Index`、`ParentComponent`、`StyledPage`、`ListPage` 四个
`@Entry + @Component` struct，以及 `defaultHeader`、`GlobalCard` 两个 `@Builder` 函数，文件/行号均与 typed
rowset 一致；`EntryAbility` 因无 `@Entry` 被正确排除。零成文拒绝，未复现 Cangjie class+extend 同名复合身份
问题，因此 `EVAL-B59-INVROW1` 保持 open，等待重载/跨 package 同名异构 fixture，不能凭单一 Cangjie 形硬修。

另登记 `EVAL-B60-CLOSURECHURN1`（P1/open）：调用链 Explorer 在正确识别无向路径前，先经历 member_set
`value=7/members=8` 一次硬拒绝；修正为 8 后，虽然提交了 8 条 positional support refs，decorated member
`analyzerEvaluator.ParseOutput (caller)` 仍触发一次 DOWNGRADED 和已读跨度补读。下一批需核对“positional
support refs 可接受”的提示与归一化后的实际判据是否同源；若不一致则属于另一个可满足性 gap，不能靠禁止模型
写括注来拟合。

证据：

- `eval/parallel_selected_summary_evalcampaign_b59_callarkts_r7_20260803.md`；
- `eval/parallel_selected_summary_evalcampaign_b59_callarkts_r7_20260803_manual_audit.md`；
- result dirs：`eval/results/*-20260803-204831`。

## 61. 2026-08-03 B60 r8：调用图复验与跨验证通道不可用回退

在 `main@05fd3a39a` 重建后严格并行 2 例：

- `qf_sequence_analyzer_gate`：runner/human PASS，206s，Explorer 16 轮，finalizer reject=2；
- `github_issue_memoclaw_text_search_multirepo_ts`：runner/human FAIL，288s，补丁正确，最终
  `unverified:runner_missing`。

### B60 replay：限定 caller 桥生效，剩余两次拒绝可满足

最终答案保持了两个不相连的源码事实：`buildAnalysisIR` 在同一函数体内按顺序调用各阶段并止于
`gate.RunWith`；裸 `gate.Run` 没有该方向调用点。第一次成文拒绝纠正了模型把这些并列直接调用误画为
“中间函数逐个互调”，第二次要求列表保留用户点名的精确 `gate.Run` 锚并披露 sibling 边界。第三轮通过，图中
只剩 `buildAnalysisIR -> callee` 的已证方向边。因此 `EVAL-B60-DIAGQUAL1` 关闭；本轮没有再次出现“边必须保留
同时又无法证明”的不可满足合同。

两次拒绝仍有成本，但都对应模型首稿的真实结构错误/精确锚点遗漏，且 validator 给出的修复动作可执行；不能为了
把 reject 计数降到零而放松 typed call-edge 证据。`EVAL-B60-CLOSURECHURN1` 继续保持 P1/open，等待异构
member-set fixture 复验。

### EVAL-B61-PROBEFALLBACK1（P0）：不可用预探针遮蔽独立项目测试

TypeScript 写例的实际补丁只修改 `src/client.ts`，正确完成 `POST /v1/search + JSON body`。测试面同时发现
`node@.` 与带 `check` 目标的 `make@.`，报告的 selected surface 也是 `make check`。但 plan 自带的 JavaScript
verification probe 依赖 Node/ts-node；当前环境没有 Node，探针返回 typed `runner_missing`。`run_tests` 虽记录
“继续 typed project test surface”，进入首选 Node plan 的 no-test preflight 后却把这份 unavailable probe 再次
插入 `projectReports` 并 `continue`，所以既没有形成 `synthetic_no_tests -> make` 升级，也从未执行仓库已有的
`make check`。最终把“一个可选验证通道不可用”误报成“本地验证整体不可用”。

本批按验证通道权威而非语言特判修复：

1. 仅当 probe 的 typed `FailureKind=runner_missing` 且 test surface 确有独立 `HasTestSignal=true` 候选时，
   unavailable probe 才只保留 command/diagnostic warning，并标记为 non-authoritative + consumed；parser/authoring、
   assertion、timeout/OOM 等其它失败不进入该回退；
2. 继续走既有 typed test-surface 状态机：首选 runner 无测试时保留 syntax/no-tests fallback，再升级到尚未执行且
   `HasTestSignal=true` 的项目候选；
3. 独立项目 suite 的真实结果进入最终聚合，changed-path authority 仍独立判定它是否足以覆盖改动；若所有通道
   均不可用，仍按原规则 fail-closed 为 unverified；真实 probe assertion failure 仍立即失败，不允许项目 suite
   覆盖；
4. 回归用“源文件语言首选 runner 无测试且 probe runner 缺失 + 独立 Make 项目检查”复刻状态机，代码和判据
   不含 TypeScript、端点、文件名或用户/答案文字特判，可覆盖其他语言与 runner 组合。

状态：`EVAL-B61-PROBEFALLBACK1 = implemented / full-tool-pass / replay-next`。验证：6 个关键分支定向
回归全绿（含 runner-missing 正臂与 4 个 parser/authoring 负臂），`go test ./internal/tool -count=1` 全绿
（171.805s）。

证据：

- `eval/parallel_selected_summary_evalcampaign_b60_callwrite_r8_20260803.md`；
- `eval/parallel_selected_summary_evalcampaign_b60_callwrite_r8_20260803_manual_audit.md`；
- result dirs：`eval/results/*-20260803-211048`。

本批不修改 Trace query、显式时间窗、Trace 因果投影、系统补采、答案成文合同或模型答案所有权。

## 62. 2026-08-03 B61 r9：Trace 因果双轴守护与无进展补证重试

在 `main@5687128b8` 重建后严格并行 2 例：

- `trace_query_wakeup_causal_runnable`：runner/human PASS，151s，3 次 trace_query，finalizer reject=0；
- `github_issue_memoclaw_text_search_multirepo_ts`：runner/human FAIL，235s，补丁正确，最终
  `unverified:patch_review_semantic_uncovered:behavior_contract_without_verify_coverage`。

### Trace 守护：显式窗、自动补采、因果投影与两类根因维度均保留

Trace 例保留用户显式窗 `1.000000..1.010000`，三次 query 均带 window 与 pid/thread 目标过滤。模型正文先总结
目标线程 10.000ms 的四态占用，再给出 `net-300 -> worker-200 -> app-100` 唤醒链和 worker 优先级反转候选；
链累计 9.000ms、有效可消除 8.300ms 分开呈现。“主要时间占用 / 关键路径候选”用于探索真实耗时与新优化方向，
“窗内可消除量”用于已建模的反转/供给规则，正文明确二者不可相加或互相替代。

`Trace 因果投影`、代表窗、root-cause #1 与证据索引仍由模型探索结果和系统 typed 补齐共同构造，位置在模型结论
之后；没有删除、替换或重写模型主结论。本批新增代码完全位于 write workflow，不进入 trace query、时间窗、投影、
补采或 answer mutation 路径。

### B61 replay：独立项目测试恢复，但行为权威仍诚实 fail-closed

TypeScript 写例证明 `EVAL-B61-PROBEFALLBACK1` 已生效。计划中的 JavaScript 行为探针因本机缺少 Node 返回 typed
`runner_missing` 后，`run_tests` 没有再把单通道不可用误作整体不可用，而是继续：

1. 对计划命中的 `src/client.ts` 做 node syntax fallback；
2. 从 typed test surface 升级到独立 `make@.` 候选；
3. 实际执行 `make check`，仓内 `check_search_client.py` 通过；
4. 保留 probe command、confidence warning 与 changed-path `source_check/syntax_only` caliber。

补丁只修改 `src/client.ts`，正确实现 `POST /v1/search + JSON body`。最终没有升级成“已验证”也是正确边界：仓内
静态检查可以证明源码形态和回归断言，但不能伪装成该 JavaScript 客户端的运行时 HTTP 行为证据；现有
`RequiresTargetBehaviorForBehaviorContract` pin 正确守住该红线，不应为了 eval PASS 放松。

### EVAL-B62-PROOFRETRY1（P1）：稳定不可用探针被 verify-only 补证批原样重跑

r9 的确定性无进展序列为：首轮 `run_tests` 在 21:44:03 记录
`verification_probe_runner_missing`，随后 syntax fallback 与 `make check` 通过；控制器在 21:44:14 把模型正确的
`finish/accept_unverified` 归一化为 `verification_proof_followup`；verify-only 批在 21:44:28 对同一 plan、同一 probe、
同一未改 worktree 再次运行，得到完全相同的 runner_missing 与 make pass，最后仍因相同 proof ledger 缺口
unverified。第二轮没有可能凭现有 typed 状态增加证据，却额外消耗一次控制器、一次 verifier 和完整测试面。

根修不扫描用户原文、模型思考、答案或错误摘要，只在以下精确合取成立时抑制**即时**重复补证：

1. report 属于当前 plan 的 `post_apply_verify`，整体项目验证 passed；
2. 工作流只有一个已应用 plan，排除跨批累计 actual-diff 复验；
3. 当前 plan 只有一个 verification probe；
4. confidence 与 executed-command 两条 typed 记录同时精确为
   `pre_suite_verification_probe/probe_execution/unavailable/verification_probe_runner_missing` 与
   `verification_probe/runner_missing/verification_probe_runner_missing`；
5. 所有待补项目仅为该 probe 明确绑定的 behavior-contract / changed-symbol proof；任何 dependent、test-surface、
   placement、未绑定 proof 或其它失败类型均 fail-open，继续原补证路径。

同一判据同时接入控制器 decision normalization 与 cumulative actual-diff 两个补证入口；抑制时记录 typed
`verification_proof_followup_suppressed_stable_unavailable`，然后沿既有 `accept_unverified` 完成，不把不可用改写成
已验证。回归覆盖正常 proof follow-up 保留、单 plan/sole probe 正臂、增加第二个 applied plan 的 fail-open 臂，
以及 cumulative 入口接线 pin。

状态：`EVAL-B62-PROOFRETRY1 = implemented / full-orchestrator-pass / replay-next`。定向接线回归与
`go test ./internal/orchestrator -count=1` 全绿（12.234s）。

证据：

- `eval/parallel_selected_summary_evalcampaign_b61_writetrace_r9_20260803.md`；
- `eval/parallel_selected_summary_evalcampaign_b61_writetrace_r9_20260803_manual_audit.md`；
- result dirs：`eval/results/*-20260803-214048`。

## 63. 2026-08-03 B62 r10：通过探针后的同路径重改安全闸与无向端点成文债

在 `main@4e7ddf3df` 重建后严格并行 2 例：

- `qf_sequence_analyzer_gate`：runner PASS / human FAIL，369s，Explorer 16 轮、investigation 7/2、
  finalizer reject=1；
- `github_issue_memoclaw_text_search_multirepo_ts`：runner/human FAIL，378s，最终
  `unverified:verification_incomplete`，且第二个 plan 把原本正确的 TypeScript 文件改坏。

### B62 replay：稳定不可用的原样补证抑制没有误伤本轮新证据

r10 的写例不是 `EVAL-B62-PROOFRETRY1` 已修的“同一 runner_missing 探针原样重跑”。首个 plan 的 Python
probe 因错误相对路径在 assertion 前失败；replan planner 随后读取当前已应用文件，并运行一个新的 typed probe，
该 probe 在 22:03:32 对当前 worktree 成功。这是新信息，控制器允许 replan 是正确的；真正的安全洞出现在成功
probe 之后仍允许对已应用同路径继续盲改。因此上一批 precise conjunction 保持不变，没有为追求 PASS 扩成宽泛
“失败后不再补证”门。

### EVAL-B63-REPLANPASS1（P0/write-safety）：最新 typed probe 已通过，协议仍接受已应用同路径二次破坏

确定性复现链：

1. 首个 plan 正确把 `src/client.ts::textSearch` 改为 `POST /v1/search + JSON body`；
2. planner 自带 Python probe 从 worktree root 调用 `../tests/check_search_client.py`，路径不存在，verify 以
   `tests_failed` 进入 replan；这是 probe authoring/path 问题，不是源码断言失败；
3. replan planner 重新读取 `src/client.ts`、测试和 API 参考，明确识别当前代码已经正确，并运行新的 bounded
   typed probe；该 probe 对当前 worktree 全部通过；
4. 仓内已有 `changes: [] / status=no_change_required` sentinel，但同一轮 tool result 没有明确给出 disposition，
   shared plan validator 也没有阻止新的非空同路径 change；
5. 模型先发 stale raw patch、再发被 micro scope 拒绝的 whole-file modify，最终用无 `old_text` 的行区间 replace
   只替换到原方法第一个闭括号；现有后缀 `return res.json(); }` 被保留，形成重复 return 和两个多余 `}`；
6. 当前环境没有 Node/tsc，第二个 Python source-scan probe 虽通过，却没有 typed changed-symbol / behavior-contract
   authority；验证诚实停在 incomplete，但损坏字节已经成为第二个 applied plan。

本批按 typed workflow protocol 根修：

1. 新增共享 `qualifyNoChangeReplanForCurrentState`，只消费 `VerifyFailureHandoff`、当前 prior plan、
   `PlanStageProbeReports` 和 `RequireAppliedWork=true`；不扫描 request、plan summary、probe code、thinking 或答案；
2. 当 shared qualifier 判定“最新 bounded planner probe 已对已应用 worktree 全通过”时，shared full-content
   validator 拒绝新 plan 对 prior plan **唯一已应用路径**的二次 mutation；多文件 prior plan 只保护 latest
   planner report 以 `ChangedPathCoverage=covered` 精确绑定的 applied path，未绑定路径 fail-open，避免一个窄 probe
   冻结整个 plan。typed reason 为
   `replan_passing_probe_applied_path_mutation`，并要求发既有 `changes: [] / no_change_required`；
3. 只封闭同路径：不同路径仍可用于独立测试/配套修复；如果确有同路径剩余缺陷，planner 先运行一个更新的
   failing typed probe，latest-report 规则会重新开放修改；没有 typed applied work 时也 fail-open；
4. `run_tests` 在同轮刚写入 passing planner-probe report 后立即用同一 qualifier 给软指引，避免模型先撞三种
   payload 拒绝；hard validator 与 tool guidance 共用一个判据，禁止再造矛盾合同；
5. 作为 compiler-unavailable 的第二道独立安全网，shared full-content seam 对 brace-family 源码比较 current 与
   planned bytes 的词法定界符栈。只有 current 可判定且平衡、planned 可判定且新失衡时硬拒绝；字符串、转义、
   triple/raw string、嵌套注释、JS regex 走词法跳过，任何未知/歧义形 fail-open。编译器/linter 仍是更强权威，
   legacy 本来不平衡的文件不被该兜底接管。

回归覆盖：同路径拒绝、不同路径放行、latest failing probe 重开、无 applied work 不触发、shared
full-content 接线 reason pin、r10 重复 return/多余 brace 形、字面量/注释/regex 负臂、legacy/lexical-unknown
fail-open，以及 Go/Java/Kotlin/Rust/Swift/JS/TS/ArkTS/Cangjie/C/C++ 等 brace-family 扩展矩阵。`go test
./internal/tool -count=1` 全绿（158.832s）。

状态：`EVAL-B63-REPLANPASS1 = implemented / full-tool-pass / replay-next`。本批不修改 Trace query、显式
时间窗、Trace 因果投影、系统补采、read-mode finalizer 或模型答案发布所有权。

### EVAL-B63-NOPATHFINAL1（P0/implemented）：错误 waiver 越权放行，成文与系统清单重建伪链

QF r10 的源码事实与 r7/r8 一致：

- `buildAnalysisIR -> gate.RunWith @ analyzer.go:2666`；
- `gate.Run -> RunWith @ gate.go:135`；
- 因而不存在用户请求方向 `buildAnalysisIR -> gate.Run`。

深读日志后需要纠正上一版判断：Explorer **没有**提交 `no_directed_path`，而是错误提交
`principal_span_waiver=endpoints_directly_adjacent`，其 rationale 证明的是 `buildAnalysisIR -> gate.RunWith`，并非用户请求的
`buildAnalysisIR -> gate.Run`。完成门当时只要看到“任意 active waiver”就绕过 exact endpoint 的无向路径检查，导致错误
adjacency waiver 越权签绿。后续 finalizer 又把 `gate.Run -> RunWith` 反写为 `RunWith -> gate.Run`；call-edge hard gate
正确拒绝该图边，patch 也删除了它，但 unchanged summary 仍保留错误方向，系统追加的 enumeration/principal roster 区块
继续把 `gate.Run` 放进“buildAnalysisIR 到 gate.Run 的调用链”。因此 runner 的 answer regex 签绿，人工必须判 FAIL。

本批按 typed source-code authority 根修，未扫描用户原文或答案原文：

1. exact endpoint reachability gate 只允许 `principal_span_waiver=no_directed_path` 放行“无同向路径”；其余六种 span
   waiver 仅在同向 source→sink 路径已经存在时处理“是否有独立可引用中间代码”，不能用 reachable sibling 冒充 sink；
2. `AnswerSemanticView.CallChainEndpointBoundary` 新增有限 typed disposition，只由 `QFCallChain`、active
   `no_directed_path` 和 analyzer typed endpoint hints 铸造 `source_endpoint / requested_sink`；free-form rationale 只留作
   审计轨迹，不能进入 prompt authority；waiver set/clear 同步 bump answer-surface revision，cached view 每次重新叠加
   当前 boundary，避免 stale；
3. finalizer 新增 `Typed Call-Chain Endpoint Boundary` 上下文：模型保留 nearest proven directed path，将 reverse/parallel
   call 按真实方向另列，并把 exact requested sink 放在 boundary/caveat/structured item 中；summary、roster、diagram 必须
   共用该 disposition。系统不代写结论；
4. 对 `QFCallChain + no_directed_path` 这一精确交集，两个系统成文器
   `appendPrincipalEnumerationTypedSupplements` 与 `normalizeAggregateMemberSetCarriers` 都不再把 endpoint definitions 或
   member-set label 铸成“可达链清单”。缺失结构行继续由 pre-emit 合同要求模型补齐，sink 仍必须可见，但其语义位置由
   模型按 typed boundary 成文；普通 call-chain、enumeration 与 Trace 专用投影/自动补采完全不变；
5. Explorer skill 的 canonical waiver 教学从六种修正为七种，明确 `(a)-(f)` 不具备 missing-path 权限，`(g)
   no_directed_path` 才是 endpoint boundary，减少模型在矛盾合同上反复试错。

定向 pin 覆盖：错误 adjacency waiver 不能绕过 missing path、正确 no-directed-path 可闭环、boundary 仅从 typed signal
铸造、cached view 的 set/clear 刷新、finalizer prompt 不泄漏 rationale、两个系统补齐器不再发布 reachable roster。
`go test ./internal/types ./internal/skill -count=1` 全绿（22.260s/1.100s），`go test ./internal/tool
-count=1` 全绿（159.686s）；`internal/agent` 新增定向 pin 通过，但该包全量仍有一条与本批无关且单独可复现的 main
基线红项：`TestAnalyzerMentionedEntityCandidatesRequiredFileNeedsVerbatimRequestProvenance` 仍要求 basename 与完整路径
同时保留，而当前实现只返回完整路径。该残余另立下一小批，不用修改本批 call-chain 语义来掩盖。

状态：`EVAL-B63-NOPATHFINAL1 = implemented / types+skill+tool-full-pass / agent-targeted-pass / replay-next`。

证据：

- `eval/parallel_selected_summary_evalcampaign_b62_callwrite_r10_20260803.md`；
- `eval/parallel_selected_summary_evalcampaign_b62_callwrite_r10_20260803_manual_audit.md`；
- result dirs：`eval/results/*-20260803-215948`。

### EVAL-B63-PATHPROV1（P0/implemented）：main 单测红，文件 basename 的 exact provenance 被符号边界修复误伤

`EVAL-B63-NOPATHFINAL1` 全包回归暴露 `internal/agent` 当前 main 的确定性红项：
`TestAnalyzerMentionedEntityCandidatesRequiredFileNeedsVerbatimRequestProvenance`。测试输入明确包含
`internal/types/evidence.go`，typed analyzer lanes 同时给出 `PrimaryEntities=[evidence.go]` 与高置信
`RequiredFileHints=[internal/types/evidence.go]`；应保留 basename 与完整路径两个不同身份，但当前只留下完整路径。

根因不是候选编译器丢值，而是前一批为防止 `AnalysisIR` 从 `buildAnalysisIR`、`Run` 从 `gate.Run` 获得伪用户权限，
把 `. / : #` 全部视为一般 symbol 的非边界。该规则对符号正确，却把“完整路径最后一个 component 中的 typed 文件
basename”也误判为嵌套标识符。修复只扩一条 path-specific lexical arm：candidate 必须是无 `/`/`\\` 的 typed
file basename（有代码/配置文件后缀），且其出现位置左侧恰为 `/`、右侧仍满足完整边界；部分子路径、无后缀 token、
更长后缀如 `evidence.go.bak` 均不获得权限。一般 symbol 的严格边界保持不变，`Run` 仍不能从 `gate.Run` 派生。

这是对既有 schema-typed candidate 的 exact identity 验证，不是关键词表、意图分类或答案原文扫描。新增 types 正负 pin，
`go test ./internal/types -run 'Test(RawRequestExplicitlyMentionsEntity_AllowsTypedFileBasenameAtPathBoundary|MentionedEntitiesFromRawRequest_RejectsNestedIdentifierAuthority)' -count=1`
、`go test ./internal/types -count=1` 与 `go test ./internal/agent -count=1` 全绿（定向 0.928s、types 全量
22.794s、agent 全量 2.641s）。Trace query、显式窗口、因果投影、系统补采和
answer-document 成文路径均未改。

状态：`EVAL-B63-PATHPROV1 = implemented / agent-full-pass / commit-next`。

## 64. 2026-08-03 B64 r11：显式 Trace 窗口口径与 command_measurement 证据路径

在 `main@f275387e5` 重建后严格并行 2 例：

- `trace_query_wakeup_causal_io_chain`：runner PASS / human FAIL，237s，4 次 trace_query，finalizer reject=0；
- `read_combo_command_current_source_explanation`：runner PASS / human FAIL，544s，Analyzer dispatch=2（首轮 provider
  首字节 3m 超时），Explorer 20 轮，finalizer reject=2。

### Trace 正面守护：能力没有被前批修复破坏

Trace 用例保持用户显式窗 `2.000000..2.020000`，四次 trace_query 均在该窗和目标线程上完成。模型正确给出
`threadpool-400 -> network-300 -> cookie-200 -> app-100` 的唤醒链，把链首
`fscache_page_wait_on_page_bit` 对应 11.000ms iowait 排为 #1，并把 20.000/17.000/14.000/11.000ms 的真实关键路径
占时与 11.000ms IO、3 个各 1.000ms runnable 的现规则可消量分轴呈现。`Trace 因果投影`、代表窗、证据索引和
系统补采仍在模型结论之后；系统没有删除、替换或重写模型主结论。

### EVAL-B64-TRACECAL1（P1/implemented）：整件附件跨度污染显式窗内目标状态

机器 PASS 漏过了一个确定性口径冲突：perf-triage 的 `time_semantics` 只证明整件附件首尾
`2.000000..2.020020 = 20.020ms` 和秒制时间单位；用户选择的 trace_query 窗是
`2.000000..2.020000 = 20.000ms`，typed `target_window_states` 也明确 app-100 在该窗 sleep=20.000ms。模型却在
investigation aggregate fact、摘要和链终点行把整件 20.020ms 写成“窗内纯睡眠”。系统附录随后给出了正确
20.000ms，但没有阻止正文口径混用；这证明“附录正确”不能替代给模型的前置口径分层。

本小批不扫描请求/思考/答案文字，不新增结论硬门，也不代写答案：

1. deterministic `time_semantics` 仍保留首尾值与 DurationMs，但 summary 明确它是 whole-attachment
   extent/unit provenance，不是 selected query window 或任何 target-thread state duration；
2. Explorer 显式 Trace 起步合同补同一条口径优先级：窗内线程状态 aggregate 必须复制 trace_query
   `target_window_states`，不能取 perf-triage 工件跨度；
3. FIN-BIND 的 state-duration caliber separation 增加第三种 artifact-global caliber，和 segment/full-window 两种既有
   口径统一教学；
4. Finalizer 最后 typed recap 明确 `principal_state` 是窗内线程四态唯一主值，早期 narrative/model aggregate 若用了
   attachment extent，只校正数值口径而保留模型诊断所有权。

状态：`EVAL-B64-TRACECAL1 = implemented / tool+agent+skill-full-pass / committed(ec95280d0)`。显式时间窗、Trace 因果投影、自动补采、
两类根因维度和模型答案所有权均未改变。

### EVAL-B64-CMDPATH1（P0/open）：Analyzer 自铸不存在的 EmitAnalysis 数据通路

Read 用例的数值 253 正确，命令实际递归统计 `internal/tool/**/*.go` 并排除 `*_test.go`。但机制答案错误：

1. Analyzer 已获得预计算 task_map，其中首批 owner 正确指向 `internal/tool/builtin.go`、
   `internal/types/context.go`、`internal/types/observation_ledger.go`；它仍调用 recursive list_files，把 29KB 文件名清单
   推入分类上下文，首个 dispatch 随后首字节超时；
2. 第二个 Analyzer 从清单里的 `emit_analysis.go` 自生
   `command_measurement -> emit_analysis struct`，又把 `required_files/source_inventory_profile` 写成证据传输机制；这些
   token 不在用户请求里，也没有 task_map relation authority；
3. Explorer 沿错误 RequiredFiles 读了大量无关源码，虽找到 `ToolResult.CommandMeasurement` 和
   `observationRecordForCommandMeasurement`，仍没有退回真实编译喉部；
4. 真实路径是：`execCommandMeasurement` 从 exec_command 输出铸 typed carrier →
   `ToolResult.CommandMeasurement` → `CompileObservationLedger` 的 `compileToolResultObservations` →
   `observationRecordForCommandMeasurement` → finalizer 的 ledger/aggregate handoff。`EmitAnalysis` 更早把用户意图铸成
   RequestModel；RequiredFiles/SourceInventoryProfile 只引导后续源码取证，不携带后来产生的 command measurement；
5. Finalizer 首稿给三条概念/dataflow 边错误标 `call`，hard gate 一次列全 3 条。模型第一 patch 只修 2 条，第二次仍被
   同一合同拒绝；第三轮靠删除 anchors/改一条虚线通过，但正文和其余概念箭头仍保留假数据流。说明 validator 守住了
   typed call 语义，却无法靠“删 anchor”替模型补出缺失的正确证据路径。

最优施工拆为下一独立批：

1. 以存在 `ToolResult.CommandMeasurement` 为精确信号，给 Explorer/Finalizer 提供 typed evidence-path authority：明确
   producer、carrier、single compile throat、record compiler 和 consumer，并明确 EmitAnalysis 是 pre-explore 导航 lane；
2. Analyze 阶段 `list_files` 只允许浅层 location pass，禁止 recursive 全树清单进入分类上下文；schema 描述和 runtime
   validator 共用同一规则，不依赖问题语言/文件类型/用户原文；
3. 回归钉定 authority 仅在 typed carrier 存在时发射、零 raw request/model scan、正确路径含 CompileObservationLedger、
   不含 measurement→EmitAnalysis；同时钉定 Analyze 浅清单、Explore 递归清单不受影响；
4. `EVAL-B64-COUNTBIND1`（P2/open）：pre-emit soft advisory 把独立 scalar 253 错配成“8 个结构节点”member_set 的
   visible_count。它未触发 reject/正文改写，本轮先登记，后续从 typed block/fact binding 修，禁止靠标签关键词拟合。

证据：

- `eval/parallel_selected_summary_evalcampaign_b64_traceread_r1_20260803.md`；
- `eval/parallel_selected_summary_evalcampaign_b64_traceread_r1_20260803_manual_audit.md`；
- result dirs：`eval/results/*-20260803-225431`。

### EVAL-B64-CMDPATH1（P0/implemented）：typed carrier 触发真实编译路径指导，分类期全树清单退场

本批按上一节冻结方案完成两条同根施工，均不读取用户原文或模型输出做控制判断：

1. 新增 `commandMeasurementEvidencePathActive`，只在 analyzer 已铸造的 typed
   `CurrentSourceExplanationProfile.Active()` 与成功工具结果携带 `ToolResult.CommandMeasurement` 同时成立时启用；普通纯计数
   问题、只有 profile 没有 carrier、或只有 carrier 没有 current-source explanation lane 均不注入内部机制说明；
2. Explorer 在命令测量实际出现后的 mid-loop 读取同一个 typed carrier，发射一次性 soft guidance，明确
   `execCommandMeasurement -> ToolResult.CommandMeasurement -> ObservationLedgerInputFromAgentContext ->
   CompileObservationLedger/compileToolResultObservations -> observationRecordForCommandMeasurement` 的真实数据路径；提示不终止
   exploration、不签证据充分、不替模型写结论，并要求相邻调用关系继续由实际已读源码证明；
3. Answer-document finalizer 在 observation ledger 后消费同一个 carrier 集合，给模型同源路径与 lane boundary：
   `EmitAnalysis/RequiredFiles/SourceInventoryProfile` 是 exploration 前的分类/导航约束，不承载之后才生成的 command
   measurement。图边区分 carrier/compile/consume 与 call，避免删 anchor 后仍保留伪数据流；
4. Analyze 阶段 `list_files` 新增 `recursive=false` 精确门，schema 描述和 runtime reject 共用
   `analyzerListFilesShallowOnlyRule`；浅层 location pass 仍可用，Explore/Extract/Finalize 的递归枚举完全不受影响。门只检查
   `StageAnalyze + tool name + recursive boolean`，不依赖语言、路径 token、问题类型、用户措辞或模型答案；
5. 回归 pin 覆盖 typed profile/carrier 合取、缺任一臂不发射、Explorer one-shot soft hint、finalizer 结论所有权声明、Analyze
   recursive reject/shallow accept、Explore recursive accept，以及 analyze/explore schema 描述分离。`go test ./internal/agent
   -count=1` 全绿（3.177s）。

状态：`EVAL-B64-CMDPATH1 = implemented / agent-full-pass / commit-next / replay-next`。本批没有修改 Trace query、显式时间窗、
Trace 因果投影、系统自动补采、根因选举或答案发布/改写路径。

`EVAL-B64-COUNTBIND1` 仍保持 `P2/open`：它需要从 typed block/fact identity 修复 scalar 与 member-set 的归属绑定，不能把本批
evidence-path 提示扩大成标签匹配或答案硬门。

## 65. 2026-08-03 B64 r2：调查型 operation 越权与未验证 scalar 的系统再发布

在 `main@c852fd933` 构建后严格并行回放同两例；runner 仍为 PASS 2/2，人工审计为 FAIL 2/2：

- `trace_query_wakeup_causal_io_chain`：188s，Analyzer 1、Explorer 1、trace_query 3、finalizer reject=0；
- `read_combo_command_current_source_explanation`：130s，但 pipeline dispatch=0，落入独立 operation lane。

### Trace 守护与残余边界

Trace 的显式窗 `2.000000..2.020000`、3 次 bounded trace_query、
`threadpool-400 -> network-300 -> cookie-200 -> app-100` 唤醒链、#1
`fscache_page_wait_on_page_bit` IO 等待 11.000ms、真实占时/规则可消双轴、因果投影和系统自动补采均正常。finalizer 上下文已经
同时给出 deterministic `principal_state sleep=20.000ms/window_ms=20.000ms`，并明确 whole-attachment
`2.000000..2.020020=20.020ms` 不得替代 selected-window state；模型仍在摘要和第 4 步把 20.020ms 写成“窗口内总阻塞/完整
周期”，而其表格及投影正确使用 20.000ms。该正文冲突先记为 `EVAL-B65-TRACECAL2 = P2/model-variance-observe`：禁止扫描
答案原文硬改，也不让系统替模型重写结论；继续异构回放确认是否稳定复现。

确定性的新红线问题是系统随后自动追加：`系统补充：结构化指标摘录：app=20.020ms`。来源不是 deterministic query，而是
Explorer 自写的 `aggregate_facts[0]`：label=`app-100总阻塞时长`、value=`20.020ms`、typed
window=`2.000000..2.020020`，ledger 已诚实标为 `historical/repairable`；发布器
`runtimeAggregateMetricCompactRows` 却通过 requested-dimension token 与 model-authored label 的词元交集再次选中它并作为系统内容
发布。这既违背“精确信号才可系统发布”，也与同页 hard `target_window_states=20.000ms` 冲突。

### EVAL-B65-ROUTEINV1（P0/open）：`operation=investigate` 被嘈声 target surface 抢成电脑操作

Read r2 的 classifier 原始结构为：`raw_route=hybrid`、`operation=investigate`、`source=mixed`、
`needs_repo_access=true`、`current_source_evidence_mode=required`，同时漂出 `needs_operation_access=true` 和
`target_surface=desktop`。`ApplyTurnPolicyGuards` 的 `isAnalysisOnlyPolicy` 本应把 investigate 送回 repo，但当前又要求 target surface
为空/unknown；desktop 令它返回 false，随后 `concreteOperationSignal` 把 desktop 当具体操作，最终整条 source-evidence pipeline
被绕过。该冲突与 schema 明文“operation=investigate 是 analysis pipeline；source implementation analysis 不是 operation”相反。

最优方案只读 typed schema 字段：当 `operation=investigate`、source 属于 repo/mixed/artifact/external_tool、无 concrete side
effects、operation_kind 不是具体 operation，且 needs_repo 或 current_source=required 时，调查身份优先，target_surface 的嘈声值不能
铸造 operation 权限；保持在 repo/hybrid 分析管线、needs_repo=true、needs_operation=false，并保留 current-source obligation。真实
computer_operation/artifact_generation/browser/document 等具体 kind、明确 side effect 与非 investigate operation 不受影响。

### EVAL-B65-ROUTEINV1（P0/implemented）：typed 调查身份不再被显示面噪声夺权

本批把上述路由权限冲突按 schema 层级修复，没有读取请求或模型回答文本：

1. `isAnalysisOnlyPolicy` 仍要求精确 `operation=investigate`、repo/mixed/artifact/external_tool source、无 side effect、无具体
   operation kind，且 risk 只允许 none/low；仅当 `needs_repo_access=true` 或
   `current_source_evidence_mode=required` 已证明当前源码调查义务时，容忍 classifier 同时漂出的非空 target surface；
2. local/repo/hybrid → operation 的提升门现在显式排除同一个 analysis-only typed policy，避免先清掉 operation boolean、又被
   `target_surface=desktop` 二次提升；该修复只改变权限路由，不改变分析器、Explorer 或 finalizer 的答案合同；
3. 生产 witness pin 完整复刻 r2 结构：route=hybrid、investigate、source=mixed、needs_repo=true、needs_operation=true、
   current_source=required、target_surface=desktop，守住 hybrid 分析管线并清除 operation access；对照 pin 把
   operation_kind=computer_operation 的其余相同结构保留在 operation lane，证明显示面容忍没有吞掉真实电脑操作。

`go test ./internal/repl -run 'TestApplyTurnPolicyGuards_OperationRoute' -count=1` 与
`go test ./internal/repl -count=1` 全绿。状态：`EVAL-B65-ROUTEINV1 = implemented / repl-full-pass / commit-next / replay-after-B65-B`。
Trace query、显式窗口、因果投影、系统自动补采、write/data route 和模型结论成文均未修改。

### EVAL-B65-SCALARPUB1（P0/open）：系统发布未验证 model aggregate

`renderRuntimeAggregateMetricCompactSupplement` 的 model aggregate 分支注释已经承认“不做 numeric re-verification”，却仍使用
requested dimension tokens × aggregate label tokens 选择并发布值。最稳妥的泛化根修不是给 app/20.020 写特例，而是禁止该**系统发布
面**消费 model-authored aggregate facts；保留 deterministic trace/perf quality observation rows。模型 aggregate 仍留在 ledger/prompt
供模型综合，不删除模型调查结论；系统只是不再把 historical/repairable 值再次伪装成自己的补充。该根修同时削弱
`EVAL-B64-COUNTBIND1` 的发布级联，但 pre-emit scalar/member-set 归属误配仍单独保持 P2/open。

### EVAL-B65-SCALARPUB1（P0/implemented）：系统 compact supplement 只消费 deterministic observation

本批删除 `runtimeAggregateMetricCompactRows` 中对 `answerDocStableAggregateFacts` 的系统发布分支，而不是删除模型事实：

1. investigation aggregate facts 仍完整保留在 mutable state、observation ledger 与
   `renderAnswerDocAggregateFacts` finalizer context 中，模型继续拥有取舍、校正和结论权；
2. 系统 compact supplement 不再使用 requested-dimension label token overlap 选择 model-authored scalar，也不再把
   historical/repairable 值包装成系统摘录；相关 key/value/unit 拼接辅助器一并删除，避免以后从旁路复活；
3. deterministic `trace_query` observation 中的 `perf_quality` 与 `perf_quality_caveats` 仍可发布，生产者、origin、typed note
   三重约束和原有去重/排序均保留；中英文标题统一使用“摘录/excerpt”，不声称额外 numeric check；
4. 正向 pin 先证明 model aggregate 仍进入 finalizer prompt、模型正文仍保留，再证明系统不追加这些值；对照 pin 继续要求
   typed trace perf-quality 系统补充存在。因此这是 authority boundary 收窄，不是关闭 Trace 自动补齐。

定向两臂与 `go test ./internal/agent -count=1` 全绿（2.591s）。状态：
`EVAL-B65-SCALARPUB1 = implemented / agent-full-pass / commit-next / replay-next`。
`EVAL-B64-COUNTBIND1` 的 pre-emit 归属误配仍为 P2/open；`EVAL-B65-TRACECAL2` 仍按模型波动观察，不新增答案文本硬门或系统改写。

施工顺序冻结：`B65-A = ROUTEINV1`（路由权限独立批，提交推送）→ `B65-B = SCALARPUB1`（系统发布红线独立批，提交推送）→
严格并行 2 例回放。两批均不修改 trace_query、显式时间窗、Trace 因果投影、系统自动补采、根因选举、模型正文或 write mode。

证据：

- `eval/parallel_selected_summary_evalcampaign_b64_traceread_r2_20260803.md`；
- `eval/parallel_selected_summary_evalcampaign_b64_traceread_r2_20260803_manual_audit.md`；
- result dirs：`eval/results/*-20260803-233053`。

## 66. 2026-08-03 B65 r3：可选 operation 副轴夺权与显式 Trace 窗的跨窗 family 假满足

在 `main@d8fffd7a3` 构建后严格并行同两例，runner PASS 2/2、人工 FAIL 2/2：

- read 130s，但仍为 pipeline dispatch=0；
- trace 140s，Analyzer/Explorer/finalizer 各 1，模型 trace_query=2，系统补采 1，finalizer reject=0。

### B65-A 首修的反证与 EVAL-B66-ROUTEINV2（P0/implemented）

r2 的 `operation_kind` 为空，首修只证明 noisy target surface 不应铸电脑操作权限；r3 classifier 发出更完整的矛盾结构：required
主轴 `raw_route=hybrid + operation=investigate + source=mixed + current_source=required`，同时 optional operation-only 副轴漂出
`operation_kind=computer_operation + target_surface=desktop`。schema 明确 operation_kind 对非 operation 路由必须为空，但首修把具体 kind
视为绝对优先，仍将 turn 送进 command-operation lane。该 lane 首个未正确 shell quoting 的 find 失败，replan 后统计 253 并 broad grep
全仓 52 命中；最终答案把测试、枚举和答案显示点拼成错误 dataflow，仍漏掉
`ObservationLedgerInputFromAgentContext -> CompileObservationLedger/compileToolResultObservations ->
observationRecordForCommandMeasurement`，B64 typed guidance 因四阶段管线没运行而不可达。

根修按 schema 字段层级而非问题文字：

1. required `route=repo/hybrid + operation=investigate` 与 needs_repo/current_source obligation 一致时，optional
   operation_kind/target_surface 不能反向铸 operation 权限；无 side effect、允许 source/risk 等既有精确条件仍必须同时成立；
2. guard 清空矛盾的 operation-only refinements、needs_operation 与 confirmation；真实
   `route=operation + operation=computer_operation`，以及 side-effect-bearing artifact/browser/desktop operation 均不受影响；
3. classifier schema/prompt 增加软语义：只读命令若只是当前源码调查的测量/定位/验证工具，属于分析管线取证步骤；只有机器、环境、
   文件系统或 UI 操作本身是用户目标时才属于 computer operation。没有用户/模型原文扫描或命令关键词硬门；
4. 生产 witness pin 把 optional operation_kind=computer_operation 纳入 r3 全结构；route=operation 对照臂保持具体电脑操作权限。

`go test ./internal/repl -run 'TestApplyTurnPolicyGuards_OperationRoute' -count=1` 与 `go test ./internal/repl -count=1` 全绿。
状态：`EVAL-B66-ROUTEINV2 = implemented / repl-full-pass / commit-next / replay-after-trace-window-batch`。

### EVAL-B66-TRACEWIN1（P0/open）：跨窗 query 的 family presence 抑制了用户窗确定性补采

Trace 模型正文这轮正确使用 20.000ms sleep、#1 threadpool-400 fscache iowait 11.000ms、完整
threadpool→network→cookie→app 唤醒链、真实占时/现规则可消双轴和下一步；B65-B 删除的 model aggregate 系统摘录没有复发。
但 Explorer 两个 model query 都使用 2.000..2.021，而 analyzer 已铸 quote-anchored explicit user window
2.000..2.020。系统补采正确把执行参数改回 2.000..2.020，却因全 ledger 的 family detector 只看 predicate presence、不看同一结果的
typed selected_window，误以为 wider root/rank/chain/state families 已齐，只补 critical view。后果是 projection anchor 仍由 wider
root/rank 的 2.021 窗控制，系统发布 principal_state=21.000ms（含窗外 running 0.980ms），与用户窗和系统补采 meta 矛盾。

最优方案：family completeness 必须相对于 quote-anchored explicit window 判断。用 deterministic query result identity（payload/raw ref，
否则 typed record-id prefix）把同次结果分组，仅当该结果有严格解析的 selected_window 与 explicit window 在共享 tolerance 内相等时，
其 rank/chain/window-state/census/critical family 才可抑制该用户窗的补采。wider/narrower query 仍留在 ledger 作为 exploration，不能伪装
用户窗 complete；无 explicit window 的既有行为保持。随后 exact-window root_cause_rank 会同时重铸 projection anchor 与 target-state
account，critical 仍按缺失补齐，最多两 view 的预算不变。

### EVAL-B66-TRACEWIN1（P0/implemented）：family completeness 绑定 exact requested window

本批在 `RunTraceQuerySystemSupplement` 的既有 typed family detector 前增加 requested-scope 过滤：

1. 只有 `RuntimeArtifactScopeProfile.ExplicitTimeWindow()` 成立时才收窄；无 explicit window 的 full-artifact、selector、derived-window
   与 legacy 行为继续调用原 detector，未扩大 hard scope；
2. 候选 record 必须来自 deterministic runtime query；以 `SourceRef.PayloadRef`、其次 RawRef、最后 producer-owned `ID` 的 `#` 前缀
   作为同次结果 identity。该结果任一 record 的严格 `selected_window` 与用户窗在共享 tolerance 内相等后，同结果的全部 family rows
   才进入 presence census；不解析 summary、label、请求或回答文字，也不拿事件自身 Span envelope 猜 query window；
3. wider/narrower model query 继续完整留在 ledger 供模型探索，但不能抑制 exact-window root/rank/chain/state/census/critical 补采。
   exact `root_cause_rank` 作为后发布 deterministic result 重铸 projection anchor，exact target-state account 随之成功 attach；
4. 两条 end-to-end pin：wrong-window 的 rank+critical 已把六类 family 全铸齐，仍必须 exact-window 两 view 补跑，且最终 projection 与
   target-state 窗必须等于用户窗；model 本身已在 exact window 铸齐六类时保持 `families_present` 零执行 no-op。

定向两臂通过；`go test ./internal/tool -count=1` 全绿（169.481s）。状态：
`EVAL-B66-TRACEWIN1 = implemented / tool-full-pass / commit-next / replay-next`。本批未约束模型探索窗、未改变 trace 引擎数值、根因排序、
因果投影算法或模型正文；只修复系统自动补齐的 completeness 作用域。

证据：

- `eval/parallel_selected_summary_evalcampaign_b65_traceread_r3_20260803.md`；
- `eval/parallel_selected_summary_evalcampaign_b65_traceread_r3_20260803_manual_audit.md`；
- result dirs：`eval/results/*-20260803-234734`。

## 67. 2026-08-04 B66 r4：主 operation 轴漂移与 selected-window 主值显著性

在 `main@e22eb427f` 构建后严格并行回放同两例；runner PASS 2/2，但人工仍为 FAIL 2/2：

- read 86s，pipeline dispatch=0，command operation 连续失败 3 轮；
- trace 208s，Analyzer/Explorer/finalizer 各 1，模型 trace_query=2，系统 exact-window 补采 2，finalizer reject=0。

### B66 确定性守护结论：显式窗补采已经闭环

Trace 的 exact-window family 修复真实生效：即使探索记录不能证明用户窗 family complete，系统仍在
`2.000000..2.020000` 补跑 root/rank 与 critical；最终 projection anchor、target-state partition、证据索引和系统核对块都固定为
20.000ms，窗外 2.020000..2.020020 的 switch-in 不再进入 principal state。`Trace 因果投影`、系统自动补采、
`threadpool-400 -> network-300 -> cookie-200 -> app-100` 唤醒链、11.000ms fscache IO 主席，以及“真实关键路径占时 / 现规则
可消量”双轴均保留。B65 删除的 model aggregate 系统摘录也没有复发。

### EVAL-B67-ROUTEINV3（P0/implemented）：required route/current-source 四轴不再被主 operation 轴夺权

r4 不再只是 optional `operation_kind/target_surface` 漂移。classifier 原始结构为：`raw_route=hybrid`、
`needs_repo_access=true`、`current_source_evidence_mode=required`、`source=mixed`，但同时把主 `operation`、optional
`operation_kind` 都铸成 `computer_operation`，并设 `needs_operation_access=true`。现有 `isAnalysisOnlyPolicy` 第一行要求
`operation=investigate`，因此 B66 的副轴容错不可达，后续 operation signal 把 hybrid 改成 operation。

这不是给某条中文命令写特例。schema 已规定真正电脑操作必须 `route=operation`；`route=repo/hybrid + current_source=required +
needs_repo=true + source=repo/mixed` 是当前源码证据调查的四个 concordant typed 轴。最优 guard 应在无 concrete side effect、
target surface 为空/unknown/repo/source、risk 仅 none/low 时，以该四轴收敛到 analysis pipeline，并把矛盾的
computer_operation 主/副轴归一为 investigate；真实 `route=operation`、机器/桌面目标、artifact/browser 生成和 side-effect-bearing
操作保持不变。判定只读取 schema enum/boolean，不扫描用户或模型原文。

实现已按该边界落地：`isAnalysisOnlyPolicy` 只为 `repo/hybrid + needs_repo + current_source=required + source=repo/mixed +
non-concrete target` 容忍主 operation 的单一 computer_operation 漂移；analysis guard 同时把主 operation 归一为 investigate，并清除
operation-only 权限副轴。guard 正反 pin 与真实 REPL dispatch pin 证明 pipeline 恰执行一次、operation planner/local responder 均零调用；
desktop concrete surface 和 route=operation 对照臂仍走 operation。`go test ./internal/repl -count=1` 全绿（33.678s）。
状态：`implemented / repl-full-pass / committed(ff15111d5)`。

r5 又给出更强反证：classifier 把 `raw_route` 本身也铸成 operation，并同时发出 desktop；但
`needs_repo=true + current_source=required + source=mixed` 仍完整保留。第二小修把这组三轴定义为不可丢失的
current-source pipeline obligation：仅对无副作用 computer_operation 冲突，将 route/operation/desktop 一并降回 repo/investigate；
没有该 obligation 的真实 desktop operation 继续走 operation。guard 与真实 dispatch pin 覆盖 exact r5 结构，
`go test ./internal/repl -count=1` 全绿（33.060s）。状态补充：`route-axis-drift-closed / commit-next`。

### EVAL-B67-TRACEVALUE1（P1/implemented-soft）：最终上下文有正确主值，摘要仍选了附件跨度

finalizer 的 typed handoff 已明确 `selected_window=2.000000..2.020000`、`target_state_symptom sleep=20.000ms`、
`principal_state sleep=20.000ms`；早期 perf-triage 也明确 whole-attachment 20.020ms 只是 extent/unit provenance。模型调查完成记录已经
纠正为 20.000ms，但最终摘要首句仍写“进入 S 态睡眠 20.020ms”，而同页主要占用、因果投影和系统事实均为 20.000ms。

该残余不授权系统删除、替换或改写模型正文，也不授权对答案/请求做关键词或数值扫描硬门。先实施 prompt-only 的 typed 口径
显著性增强：在每个 projection 的 selected_window 与 target_state_symptom 邻位明确——凡描述所选窗内目标状态，必须复制该
typed partition；附件总跨度、窗后 switch-in 和 pre-triage navigation hypothesis 不得替代。它不产生 AnswerBlock、不检查模型原文、
不触发 retry，模型仍拥有诊断和建议。

实现只修改 `renderAnswerDocTraceDecisionHandoffSet` 的 prompt-only handoff；新增
`selected_window_value_authority` 紧邻 typed partition，明确三种异口径不得替代 selected-window 值，并再次声明结论/措辞归模型所有。
既有 pin 证明 handoff 发射；`go test ./internal/agent -count=1` 全绿（2.628s）。状态：
`implemented-soft / no-answer-gate / no-system-rewrite / replay-next`。若后续仍仅偶发一行而 typed 主体正确，按模型波动留档，不升级为
系统接管。

施工顺序冻结：`B67-A = ROUTEINV3`（路由权限 + dispatch pin，独立提交推送）→ `B67-B = TRACEVALUE1`
（prompt-only typed 口径显著性，独立提交推送）→ 严格并行 2 例回放。两批不改变 trace_query 数值/排序/因果投影/自动补采，
不修改答案正文，不触碰 write/data 路由。

证据：

- `eval/parallel_selected_summary_evalcampaign_b66_traceread_r4_20260803.md`；
- `eval/parallel_selected_summary_evalcampaign_b66_traceread_r4_20260803_manual_audit.md`；
- result dirs：`eval/results/*-20260804-001754`。

## 68. 2026-08-04 B67 r5：route 主轴漂移与 relation carrier 重试风暴

在 `main@0f11651d1` 构建后仍严格并行 2 例。Read 29s 机器/人工 FAIL；Trace 在第 21 个 Explorer 轮次由人工
fail-fast 中止（约 572s、context≈103k/51%），因为同一 completion 合同已经确定性重复 20 次。中止只停止浪费预算，完整日志与
result dir 均保留。

### EVAL-B68-ROUTEINV4（P0/implemented）：raw route=operation 也不能抹掉 required current-source obligation

Read r5 的 typed 结构比 r4 更强烈冲突：`raw_route=operation + operation/operation_kind=computer_operation + desktop`，同时
`needs_repo=true + current_source=required + source=mixed`。planner 没有执行源码分析，反而要求用户补充已在请求中明确给出的命令目标；
pipeline dispatch 仍为 0。

根修不判断请求词面：后三个字段组成不可丢失的 current-source pipeline obligation。仅当 computer_operation 无 concrete side effect、
risk 为 none/low 时，这个 obligation 将矛盾的 route/operation/desktop 一并 fail-safe 到 repo/investigate；没有该 obligation 的真实
desktop/browser/machine operation 保持 operation。exact r5 guard pin 与 REPL dispatch pin 全绿，`go test ./internal/repl -count=1`
全绿（33.060s）。状态：`implemented / committed(dba723a30) / replay-after-B68-relation`。

### EVAL-B68-RELTAIL1（P0/implemented）：misplaced typed relation 被漏恢复，format-only copy 又被硬要求

Trace 本身的取证已经正确：4 次 exact-window trace_query 形成 20.000ms target partition、完整
threadpool→network→cookie→app 链与 11.000ms fscache IO 主席。失败发生在闭环载体，不是 Trace 数值：

1. provider 把 `aggregate_facts` 编成 JSON string，并把 sibling `relation_claims` 连在该 string 的数组尾部；
2. `decodeAggregateFactsPayload` 已有通用 misplaced-tail 恢复，能恢复 reason/confidence/result_kind/absence justification，却没有消费同一个
   map 里的 schema-known `relation_claims`；
3. 模型前几轮提交了精确 authority，但解码后顶层 claims 永远为空；后续即使改 member roster，仍收到完全相同的“missing required”拒绝；
4. Explorer completion 强制复制 every required authority，但 validator 不扫描 reason/prose，因此这个格式复制本身无法证明模型结论遵守
   关系；finalizer 已经按更合理合同把 carrier 定义为 optional、仅对已提交 claim 做 exact validation。两阶段合同因此互相不一致。

本批统一为架构正确形：

- misplaced-tail decoder 现在从同一 typed sibling map 严格恢复 `relation_claims`；unknown field/错误结构仍 fail-loud；
- investigation relation metadata 改为 optional，typed authority slate 仍进入 finalizer reasoning context；缺席不再制造格式重试；
- 任何模型主动提交的 claim 仍按 authority id、完整 member roster、physical relation、addition 和 subtotal 精确校验，错误 claim 继续硬拒；
- 不读取请求/答案/模型 reason，不改写结论，不降低 trace arithmetic authority。

三条定向 pin 覆盖 optional omission、wrong submitted claim reject、string-tail exact recovery；`go test ./internal/tool -count=1` 全绿
（168.432s）。状态：`implemented / tool-full-pass / commit-next / replay-next`。

`EVAL-B67-TRACEVALUE1` 本轮因 Explorer 未闭环而没有进入 finalizer，不能拿该中止样本判断 soft guidance 效果，保持
`implemented-soft / replay-next`，禁止伪报已验证。

证据：

- `eval/parallel_selected_summary_evalcampaign_b67_traceread_r5_20260804.md`；
- `eval/parallel_selected_summary_evalcampaign_b67_traceread_r5_20260804_manual_audit.md`；
- result dirs：`eval/results/*-20260804-003138`。

## 69. 2026-08-04 B68 r6：relation 风暴闭环，读链路所有权与 Trace 未绑定主值暴露

在 `main@808e6c465` 构建后严格并行回放同两例；runner PASS 2/2，人工 FAIL 2/2：

- read 122s，Analyzer/Explorer/finalizer 各 1，read=4、list=2、finalizer reject=0；
- trace 228s，Analyzer/Explorer/finalizer 各 1，trace_query=5、investigation completion=1/0、finalizer reject=0。

### EVAL-B68-RELTAIL1（P0/closed-by-replay）：relation carrier 不再制造完成重试风暴

Trace 在单次 `emit_investigation_complete` 中提交 exact typed relation claim 并直接闭环；r5 的 20 次同合同拒绝、22 轮 Explorer 与
fail-fast 中止均未复发。exact-window 自动补齐、20.000ms target-state partition、11.000ms fscache IO 主席、
threadpool-400→network-300→cookie-200→app-100 唤醒链和因果投影全部保留。因此 B68 的 misplaced-tail typed 恢复与 optional
relation metadata 合同已由真实回放验收，不是只靠单测签绿。

### EVAL-B69-CMDPATH1（P1/open）：正确路径提示缺少源文件所有权边，模型把 closure 状态拼进 ledger 数据链

Read 的 route/current-source 修复真实生效，四阶段管线执行，确定性 command measurement 值 253 正确进入答案。但机制正文存在三处
同根错误：

1. 把 `observationRecordForCommandMeasurement` 说成调用 `ObservationLedgerInputFromAgentContext`，实际方向是后者组装
   `ObservationLedgerInput`，`CompileObservationLedger` 再调用 `compileToolResultObservations`，由 carrier compiler 调用
   `observationRecordForCommandMeasurement`；
2. 把 `compileToolResultObservations` 错引到 `internal/tool/emit_investigation_complete.go`，实际定义和调用均在
   `internal/types/observation_ledger.go`；
3. 把 observation compile 链继续接到 `MutableState.InvestigationComplete/HasEnoughFacts`。后两者属于独立的 Explorer 闭环控制：
   `EmitInvestigationComplete` 写 stop flag，ParseOutput 读取它，不生产 command-measurement observation record。

现有 typed handoff 给了正确的函数序列，也要求“实际已读源码证明相邻调用”，但没有给出 file-owned edge；Explorer 只读提示渲染器与
closure type 后，仍可把两个同轮出现的机制拼成虚假桥。最优修复是把同一 prompt-only authority 升级为精确的 typed edge/boundary
清单：producer edge、context adapter edge、ledger compiler edge、carrier edge分别标注真实 owner file；另列 closure control 为
disjoint path，并要求若未读 owner source 不把 hint 当调用证据。它仍是软指导，不扫描用户/模型/答案原文，不触发硬重试，也不替模型写答案。

实现已落地：中英文 handoff 现在逐边给出 `builtin.go` producer、`observation_ledger_context.go` adapter、
`observation_ledger.go` compile/carrier 的源码所有权与正向调用关系，并把 `emit_investigation_complete.go` 明确列为不相交的 Explorer
stop/ParseOutput 控制路径；同时负向声明它不调用 ledger compiler、不生成 command-measurement record。三条 prompt/one-shot pin 与
`go test ./internal/agent -count=1` 全绿（2.618s）。状态：`implemented-soft / no-answer-gate / replay-next`。

### EVAL-B69-RUNTIMEBIND1（P0/open）：无 typed 窗/指标身份的 model scalar 可压过 deterministic 主值

Trace 的确定性面没有回归：系统 exact-window projection、target-state authority 和补采结果均为 20.000ms。但 Explorer 在
`aggregate_facts` 发出 `kind=scalar_value, value=20.020ms, role=principal_answer`，没有 dimensions、support_refs、artifact/window/metric
绑定。`AggregateFactIsRuntimeObservationAdvisory` 目前把 runtime scalar 只要“看起来是直接 runtime observation”就留在 principal；
finalizer 的通用 aggregate 合同又要求 principal scalar 必须原样保留。于是 prompt 同时出现 20.020ms principal model scalar 与
20.000ms deterministic selected-window partition，模型摘要和 caveat 选择前者，因果投影仍是后者。

这不是给 app/20.020 写特例，也不授权系统改写模型答案。泛化的 authority 修复应只读取 typed 结构：当 runtime 分析存在显式用户窗且
deterministic trace observation 已铸 selected-window principal scalar 时，缺少 artifact/window/metric/support 绑定的 model-authored scalar
不能成为 principal numeric authority，降为 supporting context 并披露 provenance；有精确 typed binding 的 model scalar、非 runtime
count/scalar、成员集、模型原因与最终结论均不受影响。系统只收窄“谁能成为精确主值”的权限，不自行选择诊断结论。

冷读后将根修进一步泛化：旧 `aggregateScalarRestatesDirectRuntimeObservation` 会把 model aggregate 的 label/value 与 log/perf 摘要做
substring overlap，并用该 noisy text match 绕过 runtime advisory demotion，这本身违反“精确信号才可作硬门”。本批删除整条文本匹配
铸权路径。external-only runtime 的 model scalar 仅在有 explicit support refs、typed scalar/count answer obligation 或 current-source
authority 时保留 principal；否则保留值但降为 supporting，并带
`demoted:runtime_observation_advisory_aggregate`。这覆盖所有 log/trace/perf runtime scalar，不依赖 app、20.020、指标名或用户/模型原文。
types 正反 pin 覆盖 text-overlap 不铸权、support-ref 与 scalar obligation 保权；agent production render pin 证明值仍进入模型上下文但不再
以 principal 发布。`go test ./internal/types -count=1`（18.303s）、`go test ./internal/agent -count=1`（3.035s）与
`go test ./internal/tool -count=1`（171.973s）全绿。状态：
`implemented / removes-noisy-hard-gate / no-answer-rewrite / full-pass / replay-next`。

施工顺序冻结：`B69-A = evidence commit` → `B69-B = CMDPATH1 prompt-only typed edge/boundary` →
`B69-C = RUNTIMEBIND1 typed authority demotion` → 同两例严格并行 2 个复放。两批均禁止请求/答案/模型原文关键词硬门，禁止系统答案改写；
不改变 trace_query、显式时间窗、因果投影、自动补齐、根因排序或 write mode。

证据：

- `eval/parallel_selected_summary_evalcampaign_b68_traceread_r6_20260804.md`；
- `eval/parallel_selected_summary_evalcampaign_b68_traceread_r6_20260804_manual_audit.md`；
- result dirs：`eval/results/*-20260804-005428`。

## 70. 2026-08-04 B69 r7：Trace 主值权限闭环，command measurement 并行载体边界仍缺

在 `main@70cb3be4d` 构建后严格并行回放同两例，runner PASS 2/2；Trace 人工 PASS，Read 人工 FAIL：

- trace 139s，trace_query=2、completion=1/0、finalizer reject=0；
- read 208s，read=13、repo_map=1、list=2、mid-loop injection=12、finalizer reject=0。

### EVAL-B69-RUNTIMEBIND1（P0/closed-by-replay）

Trace 终稿摘要、caveat、target-state partition、因果投影与系统 typed 对照全部使用 20.000ms；20.020ms 只在时间单位说明中作为
whole-attachment extent 出现，并明确“不是 selected-window duration 或 target-thread state duration”。模型 aggregate 的 scalar/member-set
仍完整进入 finalizer，但 role 均为 supporting；系统没有删除、扫描或改写模型正文。11.000ms fscache IO 主席、
threadpool→network→cookie→app 唤醒链、真实占时/现规则可消双轴、显式窗自动补齐与 Trace 因果投影全部保持。因此删除 noisy
runtime text-overlap 硬铸权已由生产回放验收，条目关闭。

### EVAL-B69-CMDPATH1（P1/partial）：所有权/方向已正，observation 与 aggregate 被误画成串行转换

Read 已不再把 `observationRecordForCommandMeasurement` 反向说成 adapter，也不再把 ledger compiler 错引到
`emit_investigation_complete.go`；最终明确 closure control 不参与数据流。253 的 current checkout count 与全部 owner file/call edge 正确。
但仍有两个同根残余：

1. Mermaid 与正文新增 `observationRecordForCommandMeasurement → AnswerAggregateFact`。实际 record 进入 compiled
   `ObservationLedger`/answer context；`AnswerAggregateFact` 是 Explorer 在 completion 中独立发出的 model structured handoff，若为 count，
   `reconcileCompletionAggregateFactsWithDeterministicCount` 可用 typed ToolResult measurement 校准。两者是并行载体，不是 record builder
   的下一条调用/转换边；
2. producer 分支把 history 条件下的 `Origin=VCSMetadata, History=true` 与普通确定性 count 的
   `Origin=CommandMeasurement, History=false` 混成同一个返回形，还把 payload rendering 说成负责写 ToolResult field；实际 payload 与
   ToolResult assignment 是 sibling consumers。

下一小批仍只更新 prompt-only typed authority：明确两条并行载体及 count reconcile 的单向校准关系；明确 history/generic 两个 producer
分支和 payload/ToolResult sibling assignment。它不要求某句答案字面出现，不扫描请求/答案/模型 prose，不触发 retry，不替模型下结论。
随后严格并行同两例复放；Trace 作为防回归 witness，Read 验证机制。

实现已落地：handoff 将 history-count 与 ordinary deterministic-integer 分支拆为互斥 typed 返回形；将 payload renderer 与
`ToolResult.CommandMeasurement` 标为 sibling consumers；将 observation record 的终点固定为 compiled ledger/answer context；把
model-emitted `AnswerAggregateFact` 与 deterministic count reconcile 明确成另一条并行载体及单向校准，负向禁止 record→aggregate
调用边。production prompt pin 与 `go test ./internal/agent -count=1` 全绿（3.248s）。状态：
`implemented-soft / parallel-carrier-boundary / no-answer-gate / replay-next`。

证据：

- `eval/parallel_selected_summary_evalcampaign_b69_traceread_r7_20260804.md`；
- `eval/parallel_selected_summary_evalcampaign_b69_traceread_r7_20260804_manual_audit.md`；
- result dirs：`eval/results/*-20260804-011337`。

## 71. 2026-08-04 B70 r8：Analyzer 可选 profile 漏发使精确 route 义务在 prompt 面断链

在 `main@e5c536da3` 构建后严格并行回放同两例，runner PASS 2/2；Trace 再次人工 PASS，Read 人工 FAIL：

- trace 168s，trace_query=4、completion=2/1、finalizer reject=0；
- read 179s，read=3、list=2、mid-loop injection=2、completion=1/0、finalizer reject=0。

Trace 的 20.000ms selected-window 主值、11.000ms IO 主席、完整 wakeup chain、occupancy/eliminability 双轴、系统自动补齐与因果投影
连续第二次生产回放正确；20.020ms 仍只作为 attachment extent 披露。一次 completion reject 来自模型主动提交了错误的 relation member
roster，exact validator 给出 typed 五态 roster 后下一次闭环；optional omission 不会被拒，旧 format-only storm 未复发。

### EVAL-B70-CMDPROFILE1（P0/implemented）：route 明确 required，Analyzer 漏 optional profile 后 guidance 不再不可达

Read 的 turn policy 是 schema 完整的 `route=hybrid + source=mixed + needs_repo=true + current_source=required + operation=investigate`。
但 Analyzer 本轮没有发 `CurrentSourceExplanationProfile`；`commandMeasurementEvidencePathActive` 原先把 active profile 作为唯一开关，
因此 typed command measurement 已产生后，新增的 owner/parallel-carrier guidance 仍未进入 prompt。模型只读三个错误关注文件，最终把
EmitAnalysis、EmitEvidence、EmitInvestigationComplete 说成 command measurement 的分类账/聚合链。

本批用已有 typed route carrier 补软桥，不解析请求/理由/答案：

1. active Analyzer profile 继续直接激活；
2. profile 缺席时，仅当 route hint 明确 `required + needs_repo + source=mixed + non-operation`，RequestModel 同时是
   `count + scalar + explain + mechanism/architecture`，并存在 producer-authored `ToolResult.CommandMeasurement`，才激活同一 prompt-only
   authority；
3. optional route、concrete operation、non-count incidental command 和无 typed carrier 的正反 pin 均保持不激活；
4. 该桥只显示 source-owned guidance，不铸 evidence、不改 CurrentSourceLaneDecision、不触发 retry/answer scan。

定向正反 pin 与 `go test ./internal/agent -count=1` 全绿（2.720s）。状态：
`implemented-soft / typed-route-fallback / no-prose-scan / replay-next`。B70 并行载体内容在本样本中没有被消费，修桥复放前不能伪报已验收。

证据：

- `eval/parallel_selected_summary_evalcampaign_b70_traceread_r8_20260804.md`；
- `eval/parallel_selected_summary_evalcampaign_b70_traceread_r8_20260804_manual_audit.md`；
- result dirs：`eval/results/*-20260804-012117`。

## 72. 2026-08-04 B71 r9：route fallback 验收闭环，数值残余降为模型波动，eval 递归范围缺合同

在 `main@0e3f0d45a` 构建后严格并行复放同两例，runner PASS 2/2，人工 FAIL 2/2：

- trace 167s，trace_query=4、completion=2/1、finalizer reject=0；
- read 260s，read=3、repo_map=1、list=2、mid-loop injection=3、completion=1/0、finalizer reject=0。

### EVAL-B70-CMDPROFILE1（P0/closed-by-replay）：typed route fallback 已进入生产 prompt

Analyzer 本轮仍没有依赖 active `CurrentSourceExplanationProfile`，但 exact typed route/carrier fallback 已使 Explorer 和 finalizer 收到
`Command Measurement Evidence Path`。最终答案主链正确落到
`ToolResult.CommandMeasurement -> observationRecordForCommandMeasurement -> compiled ObservationLedger`，并明确：

- `AnswerAggregateFact` 是 completion 的独立 model structured handoff；
- deterministic count reconcile 只做从 measurement 到 aggregate 的单向交叉校准；
- `EmitInvestigationComplete` 是不相交的 Explorer closure control，不生产 observation record。

因此 profile 缺席导致 guidance 不可达的系统 GAP 已由真实回放关闭。该修复只读取 typed route、RequestModel 和 producer carrier，
未扫描请求/答案 prose，未增加硬拒或系统代写。

### EVAL-B69-CMDPATH1（system gap closed / residual model variance）

主路径和并行载体边界均已正确进入上下文并被答案主体采用。局部段落仍把 ordinary deterministic measurement 错说成
`Origin=VCSMetadata, History=true`，而 prompt 已明确普通分支为
`Origin=CommandMeasurement, History=false`、history 分支才是 VCSMetadata。该残余已不具备“系统未提供足够信息”的证据，按模型波动
保留，不新增句子扫描、completion 硬门或答案 normalizer。

### EVAL-B67-TRACEVALUE1（system gap closed / alternating model variance）

Trace 的 exact-window deterministic authority、系统投影、finalizer context 均为 20.000ms；20.020ms aggregate 已被 typed authority
规则降为 `supporting_coverage`，且 provenance 明确。模型本轮仍在摘要选择 20.020ms；但 B69/B70 在相同合同下连续正确选择
20.000ms，形成“同精确上下文下交替正确/错误”的波动证据。按用户裁定，不为获得单一答案继续硬化：

- 不扫描用户输入或模型/答案原文；
- 不增加 final answer retry；
- 不让系统删改或替代模型结论；
- 不为 app、20.020ms 或单个 type 写特例。

显式时间窗、自动补齐、20.000ms target partition、11.000ms IO 主席、完整 wakeup chain、真实占时/现规则可消双轴与因果投影均保持。

### EVAL-B71-CASESCOPE1（P1/eval-quality open）：范围未声明，动态主值无 oracle

Read 本轮选择 `find internal/tool -maxdepth 1 ... | wc -l` 得 168，并在答案中披露“仅直属文件”边界；当前 checkout 递归计数为
253。case 请求只写“internal/tool 下”，没有声明递归还是仅直属，历史注释却按递归口径记录旧数值；runner 的
`answer_regex` 也不校验动态 count。于是 runner PASS 不能区分两种统计口径，人工审计又只能依赖隐藏意图。

泛化修向属于 eval 基础设施而非产品答案硬门：

1. case 题面显式声明“递归统计”或“仅直属目录”；
2. count oracle 在同一 checkout 动态计算 expected typed scalar，并携带 command/scope provenance；
3. 禁止固化当前 253，避免仓库增长后 oracle 陈腐；
4. 产品侧保留边界披露，不根据 case 名或请求关键词硬改命令。

下一批不再重复同一 Trace/Read 对，切换到恰好两个异构高优先级案例，优先覆盖 write mode 与另一条非 Trace 数据/操作车道。

证据：

- `eval/parallel_selected_summary_evalcampaign_b71_traceread_r9_20260804.md`；
- `eval/parallel_selected_summary_evalcampaign_b71_traceread_r9_20260804_manual_audit.md`；
- result dirs：`eval/results/*-20260804-012909`。

## 73. 2026-08-04 B72 r1：write 状态序列合同缺席，operation 完整材料车道稳定

在 `main@b4d38cdb3` 构建后严格并行两个异构案例：

- `operation_web_manual_summary`：84s，runner/human PASS；
- `github_issue_pyo3_iter_nth_overflow_symptom`：436s，runner/human FAIL，终态
  `unverified:verification_incomplete`。

### EVAL-B72-OPMAT1（covered）：大材料完成裁定有真实 receipt

operation 首轮抓取首页后，evaluator 根据 typed href inventory 继续获取 `user_guide.html`，没有把首页的完整下载误当用户手册完整覆盖。
第二轮正文 248161 bytes、118802 visible runes，被 20 个连续 material pages 覆盖；
`source_truncated=false`、`pages_truncated=false` 且存在 coverage receipt。最终答案概括全部 8 章，命令与手册内容一致。旧的首页签绿、任意 shell
截断、budget 到顶假完整均未复发。

### EVAL-B72-WRITELEDGER1（covered）：replan 没有清空累计验证域

第一计划新增 `tests/iterators.rs` 并改两个 Rust implementation；初次 Python probe 精确拒绝
`checked_sub(n + 1)` 的 overflow。第二计划只继续修两个 implementation。最终 changed-path coverage 仍同时列出第一计划的测试文件和第二计划的
两个源文件，证明 earlier applied bytes、cumulative scope 与 verification ledger 跨 replan 保留。环境没有 cargo/rustc，Python/static
checker 与 `make check` 的成功没有被提升为 Rust 行为权限，三个 Rust path 全部 unavailable，终态诚实 unverified。T7-1 类“重签空账”未复现。

### EVAL-B72-STATETRANS1（P1/open）：平面行为合同无法支撑共享状态的有序反例

人工审计发现最终补丁仍有语义错误：双端迭代器有效剩余区间为 `[index,current_length)`，但 `nth_back` 仅判断
`n >= current_length`。三元素场景先 `next()` 令 `index=1`，再 `nth_back(2)`，本应因跳过剩余 2 个而耗尽；当前实现计算
`target_index=0`，返回已经从前端消费的元素。

现有 typed 上下文为何没托住：write analyzer 生成的 contracts 主要是 `checked_add`、`checked_sub`、赋值等 code-shape，以及初始态
`nth(10)->next()`/`nth_back(10)->next_back()`；planner 指令只要求 boundary-like 正负例，没有载体明确：

1. setup：先执行一次改变共享 cursor 的操作；
2. action：从另一方向执行边界跳过；
3. observation：当前调用返回 None；
4. postcondition：两个方向后续都不能再次产出已消费成员。

这覆盖迭代器、parser/session、连接生命周期、事务状态机、双端队列、分页 cursor 等一类有序协议问题。最优方案不是增加
PyO3/nth_back 字符串规则，而是为 `WriteBehaviorContract` 增加可选 typed transition sequence，供 analyzer 明确
setup/action/observation/postcondition；planner 以该序列设计 non-initial-state 与 cross-operation probes。它是 prompt/typed context 能力，
不扫描请求或答案 prose、不直接硬拒计划、不替模型写实现。

### EVAL-B72-LANGPROBE1（P1/open matrix）：支持语言与 bounded probe runtime 不对称

当前 inline verification probe 仅支持 Python、JavaScript、Ruby、Java、Go；Rust、C、C++、ArkTS、Cangjie 等项目必须依赖本地 project runner。
runner 缺失时 fail-closed 是正确的，但不能用另一语言的 source-static checker获得行为权限。后续按语言能力矩阵补原生 bounded probe/overlay，
每种语言必须证明探针真实导入或执行 changed production code；在 runtime 不可用前保持 unverified，禁止为让 eval 变绿而提升静态检查。

施工顺序冻结：`B72-A evidence/docs` → `B72-B typed transition carrier + analyzer/planner soft guidance + structural pins` →
严格并行同一个 stateful write witness 与另一个非 write 防回归 witness。语言 probe matrix 拆独立批次，避免与状态合同混成大改。

#### B72-B implementation（implemented/full-related-tests-pass）

通用 state-transition carrier 已落地：

1. `WriteBehaviorContract` 新增可选 `transition.steps[]`，每步只接受 typed
   `setup|action|observation|postcondition` phase，并携带 operation、expected、evidence_ref；最多 8 步、有序保存；
2. normalizer 只处理 schema 字段，不从 Subject/Expected、请求或模型 prose 推断序列；未知 phase 在 emit-time fail-loud；
3. 父 behavior-contract context 只发步数与 phase 顺序；每一步另发同 `contract_id` 的 P1
   `behavior_transition_step`。首次测试发现原 240 字符 item 上限会截掉后半序列后，采用分步 carrier，而没有扩大全局预算；
4. write analyzer 被软提示：共享可变状态/有序协议需要 non-initial-state sequence，多个操作共享同一边界时需要 cross-operation sequence；
5. planner 被软提示按 transition 顺序执行 changed production code，禁止用 source-token check 或全新初始对象替代状态序列；
6. transition 不进入 risk/approval/plan hard gate，不取得 verification authority；只有真实 executed probe/project runner 能签行为通过。

结构、严格解码、prompt 与生产消费 pin 均在。验证：`go test ./internal/types ./internal/skill -count=1`（30.887s/1.131s）、
`go test ./internal/tool -count=1`（165.727s）、`go test ./internal/agent ./internal/orchestrator -count=1`
（3.050s/14.510s）全绿。状态：`EVAL-B72-STATETRANS1=implemented-soft/replay-next`；
`EVAL-B72-LANGPROBE1` 保持独立 open matrix，未用本批 carrier 冒充 Rust/ArkTS/Cangjie 等语言的执行能力。

证据：

- `eval/parallel_selected_summary_evalcampaign_b72_writeoperation_r1_20260804.md`；
- `eval/parallel_selected_summary_evalcampaign_b72_writeoperation_r1_20260804_manual_audit.md`；
- result dirs：`eval/results/*-20260804-013927`。

## 74. 2026-08-04 B73 r2：状态迁移载体未被模型消费；跨语言 probe 抢占真实测试面

在 `main@27b1632b1` 构建后，严格并行复放 B72 的 write + operation 两例：runner 1/2 PASS；人工审计 0/2 PASS。

- operation 128s，runner PASS / human FAIL；
- write 328s，runner FAIL / human FAIL，终态 `unverified:parser_error`。

### EVAL-B72-STATETRANS1（implemented-soft / production-not-consumed / watch）

新增的 `transition.steps[]` schema、分步 context carrier 与 STATEFUL VERIFICATION prompt 都在生产上下文，但本轮 write analyzer 两次发射仍只有
平面 `satisfies` 合同，没有 transition。planner 因而只补了 fresh-initial-state 的 `nth/nth_back` 测试。最终实现仍以
`n < current_length` 判定 `nth_back`，没有把已经前移的 `index` 纳入剩余区间：三元素先 `next()`、再 `nth_back(2)` 仍可返回已消费成员。

这证明载体“可表达”但尚未得到本样本的模型消费，不证明 schema 或执行器断线；同一模型在两个复放均未铸 transition，记为生产 adoption
观察项。按裁定不把 shared-state 从请求/答案关键词推断成硬门，不要求所有写计划强制发 transition，也不让系统生成测试或替代模型实现。
后续用异构状态协议样本继续观察；若多语言、多协议重复漏发，再考虑由 analyzer 自己发出的 typed stateful enum 驱动软提醒，而不是基于 prose
扫描增加成文重试。

### EVAL-B73-PROBELANG1（P0/implemented）：inline probe 的 typed runtime 与 exact target language 冲突

计划发出 `language=go` 的 probe，却同时精确绑定 `path:src/types/list.rs`。旧执行路径不检查这两个 typed 轴的相容性，直接把 Go test
overlay 挂到 Rust fixture；`NewPyListIterator` 未定义后产出 `verification_probe_go_compile_error`，并以 parser_error 抢先结束。
日志同时显示 `make@.` 仍是未尝试候选，终稿却把 probe authoring error 描述成“代码改动构建失败”，既浪费真实测试面，也给 controller
错误的 code-failure 语境。

根修不增加 plan hard reject，避免再制造“成文校验未通过”重试：

1. verify preflight 只读取 `verification_probe.language`、显式 `path:` ref、extension-derived source family 与累计 target paths；
2. exact ref 全部与 probe runtime 不相容时不执行 probe，发 typed
   `probe_authoring / verification_probe_language_target_mismatch` warning；裸 symbol 因歧义不猜；
3. authoring/import/unavailable 类 warning 不能抢占独立发现的 typed project surface，真实 runner 继续执行；
4. 不相容 probe 不取得 target execution/behavior coverage，项目 runner 仍按既有 changed-path caliber 决定能否签绿。

production test 构造 Rust target + Go probe + repository-declared Make check，钉住“probe 未执行、诊断在账、Make 随后成功”；原 polyglot
Make authority 与 probe confidence pins 同批通过。验证：定向 `go test ./internal/tool -run ... -count=1`（1.260s），完整
`go test ./internal/tool -count=1`（168.742s）。状态：`implemented / typed-only / no-plan-retry / no-prose-scan`。

### EVAL-B73-OPEVAL1（P1/eval-quality open）：部分完成答案被 lexical oracle 签成 PASS

operation 本轮只抓到首页 artifact（14606 bytes，内容停在 CSS），没有消费 B72 曾成功使用的 `html_link_targets`，猜测 `/docs`、`/doc`
并在五轮后输出“部分完成”；没有 `user_guide.html` 材料、20 页正文或 complete coverage receipt。相同系统合同的 B72 已完整抓取
248161 bytes/20 页，故本轮 retrieval miss 先按模型波动保留，不为 URL 或 grep 词形写产品硬门。

确定性 GAP 在 eval：case 仅用 answer/log regex，正文同时出现“用户手册、使用、codrax.net”即可 runner PASS，无法区分 partial 与 complete。
泛化修向是 case 可声明 typed operation terminal/coverage obligation，由 runner 读取 operation status、material kind、coverage receipt 与目标材料
身份；禁止通过扫描最终答案里的“完成/部分完成”决定。该基础设施项留待独立批次，不与 verification probe 根修混改。

上下文审计结论：write prompt 已提供 state-transition 能力但模型未采用；verify 上下文把 probe parser error 错投影为 production build failure，
本批已根修其 typed 路由。operation 首页材料的 typed href 本轮未被模型消费，但 B72 对照证明载体可达，暂不升级为产品确定性 GAP。

证据：

- `eval/parallel_selected_summary_evalcampaign_b73_writeoperation_r2_20260804.md`；
- `eval/parallel_selected_summary_evalcampaign_b73_writeoperation_r2_20260804_manual_audit.md`；
- result dirs：`eval/results/*-20260804-020722`。

## 75. 2026-08-04 B74-A：动态 scalar/scope eval authority（B71-CASESCOPE1 施工）

复核确认 B71 的确定性 GAP 完全位于 eval：产品已诚实披露“仅直属文件”，但 case 自身没有声明递归口径，
runner 也没有 checkout-dependent 主值，导致 lexical regex 可以把 168 与递归真值同时签绿。修复不进入 Codrax
产品链，新增 opt-in case carrier：

1. `EXPECT_DYNAMIC_SCALARS` 声明有名字的 scalar；每个 ID 必须同时提供 command、data scope、answer
   surface 与带 `{{VALUE}}` 的 binding regex；
2. runner 在该 run 实际使用的 checkout 根执行 versioned case command，输出必须是唯一非负整数；fixture/data/
   multirepo 均绑定自己的 scratch，而不是误读 Codrax 根；
3. surface 显式选择 `primary|principal|answer`，可选 `_text` 只做换行折叠；默认 primary，防 citation、raw
   recovery 或 Trace supplement 替主体答案刷绿；
4. 每轮生成 `run-N.dynamic-scalars.tsv`，保存 ID、动态值、surface、data-scope、checkout root 与 command；
   command 失败、非整数、多行、scope/binding 缺席或动态值未绑定均 fail-loud；
5. 该命令来自已被 shell source 的可信 versioned case，不读取用户/模型输入，也不向产品 prompt 回灌；其结果
   只影响 eval verdict。

`read_combo_command_current_source_explanation` 题面现明确“递归、包含全部子目录、非仅直属”；oracle 用同一
checkout 动态执行 `find internal/tool -type f -name '*.go' ! -name '*_test.go'` 计数，不固化 253。绑定要求动态值
与 `internal/tool/非测试/Go 文件/递归` 范围词在 primary answer 同一近邻，旧 168 直属口径不能再 PASS。

看护覆盖动态值正臂、陈腐值负臂、command/data-scope/checkout receipt，以及 `run.sh` 接线负例；shell syntax
与 runner 全合同通过后独立提交。状态：`EVAL-B71-CASESCOPE1=implemented/eval-only/tests-pass/replay-later`。

下一批仍遵守 B71 冻结：不重复 Trace/Read 对，严格并行恰好两个异构高优先级 case（write + operation），人工
审计模型是否消费 state-transition typed context、operation 是否取得真实 material coverage receipt，以及过程中的
成文/补证重试。显式时间窗 Trace 因果投影与自动补齐完全不在本批 diff 中。

## 76. 2026-08-04 B74 r1：operation 压缩修复丢 typed 权威；write 看护正则制造伪失败

在 `main@5c68e8ebb` 构建后严格并行两个异构案例：runner 表面 1/2 PASS，人工审计 0/2 PASS。

- `operation_web_manual_summary`：163s，runner PASS / human FAIL；
- `github_issue_pyo3_iter_nth_overflow_symptom`：900s，TIMEOUT / human FAIL。

### EVAL-B74-OPREPAUTH1（P0/open）：compact repair 把完整 ledger 压成单一 preview false

operation 本轮正确抓取首页并沿 typed link 获取 `user_guide.html`。第二轮系统材料账本已证明：20 个连续 page、
`visible_runes=118802`、`source_truncated=false`、`pages_truncated=false`，并铸造 system-owned coverage receipt。
第一版 evaluator 也判断 complete，但误把 source payload path 写入 `coverage_material_refs`，没有选择 receipt；确定性
validator 因 source excerpt 的单条 prompt preview 不完整而正确拒绝。

随后 compact tool-param repair 只携带 `material_ref=<source> fully_visible=false`，没有携带同一 records 中已存在的
coverage ledger 或 receipt。修复模型据此把状态降为 `partial_answer_possible`。最终答案一面写“状态：部分结果”，
另一面又写“已提取用户手册全文（20 页，约 12 万字符）”，是 typed terminal 与已有材料权威发生冲突。

根修边界冻结：

1. compact repair context 必须携带 records 中 system-owned material coverage authority，明确 source/identity、截断位、
   complete receipt；不能从单条 output preview 的 `fully_visible` 反推整份材料覆盖；
2. 真正没有 receipt、source/page 确实截断的 raw payload 继续只能 partial/budget-exhausted；
3. 仍由模型按精确 typed context 重发 tool params，系统不把 complete 直接代写进模型答案；
4. CLI/REPL 的 typed evaluation log 同时披露 final material coverage status/ref，便于 eval 消费最后一条权威状态。

### EVAL-B73-OPEVAL1（P0/confirmed）：lexical oracle 对 typed terminal 失明

case 仍只校验答案和日志中若干词面，故上述 partial terminal 与自相矛盾答案仍被签为 PASS。施工升级为 P0 eval-quality：
新增 opt-in typed operation oracle，读取最后一条 command/provider evaluation 事件的 terminal status、material coverage
status 与系统 receipt，而不是扫描最终答案的“完成/部分”。中间一轮 complete 不能覆盖最后一轮 partial，避免同一
日志中的历史状态刷绿。

### EVAL-B74-WRITEORACLE1（P0/open）：eval 自带 DOTALL 正则跨测试函数铸造错误语义

write analyzer 第三次没有消费 `transition.steps[]`，日志明确 `phases=single/0`。第一计划因此写出矛盾测试：
`nth_back(usize::MAX)` 已越过全部剩余元素并收缩共享区间后，却期望正向 `next()` 仍返回全部元素。产品 verifier
没有签绿并触发 replan，这一方向正确。

但失败消息本身来自 fixture `tests/check_iterators.py` 的另一处伪判：
`nth_back(0).*?next().*None` 使用 DOTALL 扫描整个测试文件，把合法的“先逐个返回所有剩余 Some、最终自然 None”
也判成“`nth_back(0)` 立即耗尽”。这正是嘈声文本/源码形状取得行为硬门权限，且直接制造 replan 与 900s timeout。

处置原则：

1. fixture 的正则必须至少限定单一 test/function body，不得跨声明；更优是删除该否定词形门，改用原生语言执行
   non-initial-state/cross-direction probe；
2. 在 Rust runtime 不可用时继续 `unverified`，不得让 Python source-static checker取得 Rust 行为通过权限；
3. 不把请求或模型输出关键词升级为“必须有 transition”的产品硬门；`EVAL-B72-STATETRANS1` 继续以跨协议复放观察
   adoption，若多个异构协议都漏发，再由 analyzer 自己铸造的 typed statefulness 驱动软提醒；
4. replan 的 3.5 分钟首字节等待是模型/预算波动，只有移除伪失败后仍跨异构复现才另立调度 GAP。

施工批次：`B74-A evidence/docs` → `B74-B operation repair authority + typed eval terminal oracle` →
`B74-C write fixture oracle 降噪/运行时权限分层`。每批独立提交推送；修复后继续严格并行恰好两个异构案例。
显式时间窗 Trace 因果投影、系统自动补齐、read/write 路由与模型结论所有权均不在这些硬门之外发生变化。

#### B74-B implementation（implemented/tests-pass/replay-next）

operation repair 与 eval oracle 已按同一 typed authority 闭环：

1. compact repair 新增有界 `material_coverage_authority`，从 command records 的 system-owned pages 顺序派生；
   每个 source/representation 只发一行，包含 source identity、source/pages 截断位、`complete|partial` 与合法
   coverage receipt。单条 payload preview 的 `fully_visible=false` 继续披露，但不再覆盖整源 receipt；
2. complete 只在 receipt 存在且 source/pages 均未截断时发射；无 receipt 或任一截断仍明确 `partial`。新正臂
   证明 malformed evaluator params 经 compact repair 后可选择真实 receipt 并保持 complete，既有真截断负臂继续
   降级 partial；
3. CLI/REPL 的 system-authored evaluation event 现同时记录 `status`、`material_coverage_status` 与有界
   `coverage_material_refs`。这是观测面，不改模型 tool params 或最终答案；
4. eval case 新增 opt-in typed terminal/coverage/ref 合同。runner 只解析最后一条精确
   `[cli|repl/operation] command evaluation` INFO 事件，生成 `run-N.operation-terminal.tsv`；历史 complete 后最终
   partial 必须失败，模型/答案中出现“完整”不参与 verdict；
5. `operation_web_manual_summary` 现在要求 final `complete + complete + material-coverage:v1:<sha256>:html_text`，
   当前 B74 的伪 PASS 在新 oracle 下会被确定性拒绝。

验证：compact repair 四个正反定向用例、完整 `go test ./internal/repl -count=1`（33.847s）、shell syntax 与
`bash eval/runner_lib_test.sh` 全绿。状态：`EVAL-B74-OPREPAUTH1=implemented/replay-next`；
`EVAL-B73-OPEVAL1=implemented/eval-only/replay-next`。本批没有通过系统代写 complete，模型仍拥有最终判断；
系统只保证修复轮拿到与首轮相同的精确 coverage 权威。

#### B74-C implementation（implemented/eval-oracle-only/tests-pass）

全量扫描 `eval/fixtures/**/*.py` 的 DOTALL/跨行正则后，确认三套 GitHub issue 静态 checker 中有两处声明所有权
没有绑定：

1. PyO3 checker 的旧负门直接在整个 test file 上执行
   `nth_back(0).*?next().*None`。现先以 brace-balanced function body 分区，再只读取 action 后第一个同方向
   `assert_eq!` observation；合法的 `Some...Some...最终 None`、兄弟 test 的 None 均不再误判，立即 None 仍拒绝；
2. napi-rs checker 用“最后一个 backtick return 到文件末尾”当 `renderNativeBinding` 模板，另一个函数可取得
   错误 authority。现先精确提取具名 function body，再在该 body 内解析模板；decoy function 无法签绿；
3. chrono checker 的源码判断已先进入具名 function body；其唯一 test 跨行 pattern 又由同一 statement 的
   `[^\n;]*` 约束，不存在本次跨声明伤口，保持不动。

两套修改后的 checker 都增加自包含 scope self-test，并接入各自 Make `check`。同时重跑产品现有 verifier
caliber pins：cross-language/source-static probe 仍不能取得 target execution/behavior，且不能抢占独立项目 runner。
因此本批只纠正 eval oracle 的声明作用域，不提升 Python checker 对 Rust/TS 的行为权限；Rust runtime 缺席时仍
必须 unverified。

验证：两个 checker `--self-test` 全绿；
`go test ./internal/tool -run 'TestVerificationConfidenceRejectsCrossLanguageProbeContractClaim|TestRunTestsCrossLanguageExactPathProbeDoesNotPreemptTypedProjectSurface|TestChangedPathCoverageContractBoundProbeHasTargetBehaviorCapability' -count=1`
全绿（1.380s）。状态：`EVAL-B74-WRITEORACLE1=closed`；`EVAL-B72-LANGPROBE1` 仍是能力矩阵 open，未伪收账；
`EVAL-B72-STATETRANS1` 继续异构 adoption watch，未增加请求/答案关键词硬门。

证据：

- `eval/parallel_selected_summary_evalcampaign_b74_writeoperation_r1_20260804.md`；
- `eval/parallel_selected_summary_evalcampaign_b74_writeoperation_r1_20260804_manual_audit.md`；
- result dirs：`eval/results/*-20260804-050207`。

## 77. 2026-08-04 B75 r1：材料完整性未绑定目标身份；数据参考投影被自洽校验误签

在 `main@52cc3eda7` 构建后严格并行两个异构案例：runner 表面 1/2 PASS，人工审计 0/2 PASS。

- `operation_web_manual_summary`：54s，runner PASS / human FAIL；
- `data_multifile_reference_projection`：423s，runner FAIL / human FAIL。

### EVAL-B75-OPGOALMAT1（P0/confirmed）：complete receipt 只证明源完整，没有证明目标材料正确

B74 的修复已生效：最终 command evaluation 是 `status=complete material_coverage_status=complete`，并引用合法
`material-coverage:v1` receipt，compact repair 没有再把完整 ledger 降成 partial。但本轮模型只取得 Codrax 首页，
首页又明确暴露 `user_guide.html` link target；用户要求的是站内“用户使用手册”，最终摘要却把完整首页当成完整目标材料。

这不是 coverage receipt 语义错误，而是两个独立维度被错误合并：

1. source coverage：选中的 source 是否完整；
2. goal-material identity：选中的 source 是否满足 operation plan 要求的目标材料。

现有 eval oracle 只要求 `complete + html_text receipt`，任何完整 HTML 都能刷绿。根修先补 system-owned records
派生的 receipt→source ref/identity typed 观测面，并增加 opt-in `EXPECT_OPERATION_COVERAGE_SOURCE_REGEX`；case 可对
versioned 目标材料身份作精确声明。产品侧继续审计 operation plan 是否已有结构化 required-material carrier；若没有，
后续应在 planner IR 增加 typed goal-material obligation/selection receipt，而不是扫描用户原始输入或模型答案推断
`user_guide`。完整入口页只能证明入口页完整，不能自动取得任务完成权威。

### EVAL-B75-DATAREFPROJ1（P0/confirmed）：贡献集合代替参考集合，reconcile 循环自证

case 的确定性目标是按 `targets.csv` 顺序输出 GroupA、GroupX、GroupC：活跃且已映射的贡献分别为 17、0、5。
实际终稿为 `17,4,5`。终态仍记录 `status=complete reconcile=pass`，因为：

1. contribution 聚合正确得到 GroupA=17、GroupB=4、GroupC=5；
2. `reconcile_artifacts` 只比较 contribution groups，没有把 `targets.csv` 当输出域；
3. `assemble_answer` 又按 contribution `group_key` 排序，因此保留了不在 targets 的 GroupB，遗漏 targets 中无贡献的
   GroupX，也没有 zero fill；
4. expected/actual 都从同一 contribution 产物派生，形成“输出自验输出”的循环权威，错误的 17,4,5 被签 pass。

根修必须是通用 reference-complete projection：由 typed workflow contract 声明 reference artifact、key、order、缺席值
策略与 extra-key 策略；贡献集合只提供 value。reconcile/assemble 发 system-owned projection receipt，至少包含
`reference_total/emitted_total/missing_refs/extra_keys/order_preserved/zero_filled`。只有 reference coverage 完整且策略
满足时才可签 complete；不能从规则 prose 或最终答案反推这些字段，也不能由系统代写模型结论。

### EVAL-B75-DATALOOP1（P1/confirmed）：schema 非进展动作重复消耗轮次

该 data run 共 15 rounds、3 repairs、6 action failures。早期 normalize 丢失 `active` 后，planner 多次执行
entity-resolution，但 output schema 没有补回 filter 所需字段。系统已有 action kind、输入 lineage、output fields 与
unresolved required fields，可以构造 typed progress signature；同签名重复且 required-field 集不收缩时，应向 planner
发精确的 non-progress 诊断并要求换用保留/连接基础字段的变换。不得按 action 名、请求关键词或模型 prose 写单题门。

另有较低优先级观察项：模型曾假定 status=`matched`，实际枚举是 `resolved/unmatched`；先审计 action schema/value
distribution 是否已经在首次 filter 前可达，再决定是否补 typed enum context，避免为本 fixture 固化状态值。

施工顺序冻结：`B75-A evidence/docs` → `B75-B receipt→source identity + eval source oracle` →
`B75-C typed reference projection authority` → `B75-D schema-progress loop`。每批独立提交推送。后续回放仍严格并行
恰好两个异构 case；显式时间窗 Trace 因果投影、自动补齐、读写路由与模型结论所有权不在本批改动范围。

证据：

- `eval/parallel_selected_summary_evalcampaign_b75_operationdata_r1_20260804.md`；
- `eval/parallel_selected_summary_evalcampaign_b75_operationdata_r1_20260804_manual_audit.md`；
- result dirs：`eval/results/*-20260804-053425`。

### B75-B implementation（implemented/tests-pass/replay-next）

operation coverage 现拆成相互独立且可审计的两条 typed 轴：

1. 原有 `coverage_material_refs` 继续只回答所选 source 是否完整；
2. 新增 system-owned receipt provenance，将每个被 evaluator 采用的 receipt 回溯到 material page 的 payload ref、
   内容 identity，以及实际产生该 payload 的 command step/argv locator。只有记录中的 step result 与 plan step 精确对上
   才发 locator，模型在 reason/答案里提到 URL 不能铸造该权威；
3. CLI/REPL 最后一条 evaluation event 新增有界 `coverage_source_refs`、`coverage_source_identities`、
   `coverage_source_locators`。它们只是观测面，不改变 evaluator status 或最终答案；
4. eval 新增 opt-in `EXPECT_OPERATION_COVERAGE_SOURCE_REGEX`，只匹配最后一条 system-authored event 的 producer
   locator。web-manual case 现要求 receipt 所属 producer 精确指向 `user_guide.html`：完整首页 receipt 会 fail，完整手册
   receipt 才可通过；随机 payload 路径和内容 SHA 都不能替目标材料身份刷绿。

看护同时钉住 receipt→payload→producer 正向映射、错误 `index.html` source 负臂、最后一轮 terminal 所有权，以及
真正 partial/无 receipt 的既有降级车道。验证：定向 material-page tests、完整 `go test ./internal/repl -count=1`
（约 25s）、shell syntax 与完整 `bash eval/runner_lib_test.sh` 全绿。状态：
`EVAL-B75-OPGOALMAT1=observability+eval-contract implemented / product required-material IR still open`。

这批没有把 `user_guide` 写入产品规则；该字面仅存在于 versioned eval case 的预期 source。产品 planner 是否需要通用
`required_materials` typed carrier，将结合下一轮异构 operation evidence 再裁，不用请求/答案关键词做硬推断。

### B75-C implementation（implemented/tests-pass/replay-next）

冷读纠正了最初“投影执行器缺能力”的猜测：`assemble_answer` 已能按完整 reference set 顺序投影、为无贡献 key
补 0、丢弃 reference 外 contribution group；`17,4,5` 负例和 `17,0,5` 正例也已有单测。生产失效点在 adaptive
plan admission：模型曾在一个因跨 compute→reconcile DAG rank 而被拒的计划里正确声明
`complete_reference=true/reference_path=targets.csv/reference_key_field=canonical_label`，但下一轮 repair 只修动作，
省略了 reference 字段。旧运行时把动作拒绝和输出合同拒绝绑定，导致该 typed obligation 被静默降回普通
`plain_single_line`，最终 assemble 合法地按 present groups 输出 17,4,5。

根修将 output obligation 与 action batch 生命周期解耦：

1. CLI/REPL 各自维护 durable structured output contract；每个 schema-valid candidate 在 action preflight 前后都经过同一
   specificity merge。动作跨 rank 被拒不再抹掉 complete-reference 合同；
2. repair 只是省略字段时，较强合同继续生效；后来若发出同等 specificity 的完整新合同，则视为显式修订并由新合同
   胜出，避免把早期错误 locator 永久 sticky；
3. 不从 question、rules prose、final answer 或数字形状推断 reference。权威只来自 planner 已铸造的 typed
   `OutputContract`；reference path 仍须通过既有 workflow-material credential 和 grounding guard；
4. `assemble_answer` 的 system-owned artifact 新增 projection receipt：`reference_total`、`emitted_total`、
   `zero_filled_count`、`dropped_extra_count`、`unfilled_reference_count`、`order_preserved`，与既有
   `reference_projected/path/key_field/key_count` 同源构造。reference-order 投影的例子现在明确发
   `3/3/1/1/0/true`，不会再只凭 answer 自验 answer；
5. 结构 pin 钉住 CLI 与 REPL 的 plan protection 各有 pre/post 两个 durable merge 调用，防 helper 存在但生产接线被删。

验证：reference zero-fill/drop-extra/explicit-path/旧 projection group 定向测试、rejected-plan→broad-repair→explicit
revision 三臂、validator proposal witness，以及完整 `go test ./internal/dataquery ./internal/repl -count=1` 全绿。
状态：`EVAL-B75-DATAREFPROJ1=implemented/replay-next`。该批没有系统代写模型结论；系统执行的是模型 typed contract
已声明的数据投影，并把构造事实作为 receipt 披露。

### B75-D implementation（implemented/tests-pass/replay-next）

现有系统已经有 relation-no-progress typed guard，但 B75 的五次连续 `apply_entity_resolutions` 没有触发。根因不是
action kind 漏枚举：`apply_entity_resolutions` 本就在 relation materialization 集合中；而是 `ProgressSignature` 把累计
`ArtifactCount/ArtifactRows` 当成收敛进展。每生成一个同 schema 的新 alias，这两个数必然增长，签名就变化，连续
stall 计数永远清零。客户 run 中正是 artifact 9→10→11→12→13，而贡献数始终 0、reconcile 始终缺席。

根修保持已有 typed gate 和阈值，只纠正信号口径：

1. convergence signature 只由 stage、累积 field capability set、rule/decision/entity/contribution ledger 数、reconcile 与
   answer presence 构成；
2. artifact/row 累计数仍保留在 `ProgressFrame/ProgressDelta` 供模型和审计观察，但不再取得“已向下一 DAG stage
   推进”的硬门权限；
3. field set 或任一 ledger/stage 真变化仍立即重置 stall，允许合法 derive/enrich/join 链继续；只有 schema/ledger
   签名相同达到既有阈值，后续 relation action 才被精确拒绝，并继续开放 derive/extract/filter/qualify/compute/reconcile；
4. 新看护复现 artifact 6→9→10、rows 35→48→61 但 fields 恒定的三轮，要求 stall count=3，同时断言累计 delta
   仍可见。既有“新增 canonical_id 即真实 schema progress”正臂继续通过。

`resolved/unmatched` 枚举并非载体缺失：artifact access 已携带 bounded `field_samples`，continuation prompt 也明确要求
先读 samples；模型本轮先猜 `matched` 后在 zero-match typed diagnostics 中恢复。当前按 adoption/model 波动观察，不为
该枚举增加产品常量或词面硬门。

验证：完整 `go test ./internal/dataworkflow ./internal/repl -count=1` 全绿。状态：
`EVAL-B75-DATALOOP1=implemented/replay-next`；`EVAL-B75-DATASTATUS1=typed-context-covered/adoption-watch`。

## 78. 2026-08-04 B76 r2：来源身份闭环；typed action 参数可被静默忽略

在 `main@d8a66d01a` 构建后严格并行两个异构案例：runner 1/2 PASS，人工审计同为 1/2 PASS。

- `operation_web_manual_summary`：132s，runner PASS / human PASS；
- `data_multifile_reference_projection`：286s，runner FAIL / human FAIL，实际 `20,0,5`。

### EVAL-B76-OPGOALMAT1（closed）：完整 receipt 已绑定真实目标材料

operation 本轮先完整读取首页，但模型依据材料本身识别首页仅是入口，随后执行
`curl ... http://codrax.net/user_guide.html`。最终 system-authored evaluation event 同时给出：

1. `status=complete material_coverage_status=complete`；
2. `material-coverage:v1:c94c…:html_text`；
3. source identity `sha256:c94c…:bytes:248161`；
4. producer locator 精确指向 `user_guide.html`，而不是首页；
5. 最终答案实际消费手册内容，覆盖安装配置、读/写模式、CLI/REPL、Trace 与排错等主要使用面。

因此 B75-B 的 receipt→payload→producer 权威链与 opt-in eval oracle 均由生产回放证明。该机制没有把具体 URL
写入产品硬门，也没有替模型判断材料相关性；系统只披露所消费材料的 typed provenance。

### EVAL-B76-DATAPARAM1（P0/confirmed）：已声明参数通过 admission，但执行器未消费

B75-C/D 同样由本轮证明有效：原始 repair 省略 reference 字段后，durable output contract 仍保留；首次 assemble
被完整参考域校验拒绝，deterministic fallback 最终按 `targets.csv` 顺序发射三项并为 GroupX 补 0。round 数也由
B75 的 15 降至 9，累计 artifact/row 不再伪装 schema progress。

剩余错误发生在 reference 投影之前。模型为 `qualify_records` 发出结构化参数：

```text
source_filters=[active eq true, canonical_id not_empty]
source_filters_json=<同值>
```

plan admission 接受该 action；但 `runQualifyRecords` 只调用读取 `filters` 的 `parseContributionFilters`，从未读取
`source_filters`。于是六条 joined rows 全部签 include，inactive r3 的 GroupA=3 被计入，贡献变成 GroupA=20、
GroupB=4、GroupC=5；后续 reference projection 正确丢 GroupB、补 GroupX，忠实输出错误上游值 `20,0,5`。

这不是单个值或 fixture 拟合，而是 typed action 合同红线：一个参数键既能通过结构化 admission，又不被任何执行路径
消费，系统却静默成功。当前只有 `compute_contributions` 有明确 allowed-param validator（本轮也正确拒绝了无效
`aggregation=sum`）；其他 action family 缺统一的“参数已消费或明确拒绝”契约。

根修冻结为两层：

1. action-kind-aware canonical alias：对同一语义角色的已备案别名，在进入 executor 前归一到该 action 的 canonical
   param；例如 `qualify_records` 的 source/base/record include filters 归一到 `filters`。只能使用显式 typed alias table，
   不根据 rule prose、question 或 final answer 猜测；
2. parameter-consumption contract：每个 typed action 发布 canonical allowed keys/aliases；归一后仍有未知键必须 typed
   fail，并列出 action kind、unknown keys 与 allowed keys。不得继续静默 no-op，也不得为未知键猜造语义。

施工拆为 `B76-A evidence/docs` → `B76-B shared parameter contract + qualify filter aliases` →
`B76-C 按 action family 扩展并以现有 planner/scaffold 全量反查`。第一批需至少看护：alias 正臂、unknown-param 负臂、
canonical+alias 冲突 fail-closed、原有 reject filters 不回归，以及 compute 既有严格合同不被旁路。

### EVAL-B76-DATAINPUT1（P1/confirmed）：planner 两次把 workflow ledger 当 record input

planner 两次为 `qualify_records` 选择 `workflow_rule_coverage`，artifact schema guard 正确识别其
`node_class=workflow_ledger/json_shape=object` 并拒绝，随后模型改用 `joined_observations` 恢复。这说明安全门有效，但
record-consuming action 的候选输入仍把控制/审计 ledger 暴露在高位，制造两轮可避免 repair。后续应让 typed action
scaffold/候选 ranking 只优先兼容 node class，并保留现有 admission guard 作为最终防线；不能按 artifact 名字或当前 case
关键词硬排除。

证据：

- `eval/parallel_selected_summary_evalcampaign_b76_operationdata_r2_20260804.md`；
- `eval/parallel_selected_summary_evalcampaign_b76_operationdata_r2_20260804_manual_audit.md`；
- result dirs：`eval/results/*-20260804-060620`。

### B76-B implementation（implemented/tests-pass/replay-next）

新增 action-kind-aware parameter-consumption contract，并在 `ActionRunner` 的统一 dispatcher、action kind 归一化之后、
任何 executor 运行之前接线。第一批完整覆盖 `qualify_records` family：

1. canonical include filter 为 `filters_json`；`filters` 以及 source/base/record 的普通与 `_json` 形均是显式备案 alias。
   B76 原样的 `source_filters + source_filters_json` 现在只归一一次并实际参与每行资格判定，inactive 行不再静默通过；
2. reject/exclude/block filter 是独立 canonical group，不会与 include filter 混合。既有 block-filter 行为由正向回归钉住；
3. 同一 alias group 的多个非空值只有在 JSON 语义等价时才接受；不同值 typed fail，禁止 executor 猜优先级；
4. 归一后任何未列入该 action allowed set 的参数都以 `DataActionParamError/action_param_violation` fail-closed，并返回
   unknown/allowed key 清单。该信号完全来自 action kind 与 param map，不读取 question、rule prose 或 final answer；
5. 公共 registry/helper 位于独立 `action_param_contract.go`，后续 action family 复用同一 admission-to-execution 边界。
   原有 `compute_contributions` 严格 validator 暂不改写，避免其定制 member/count repair guidance 回归；B76-C 再逐族迁移；
6. 同批把 `actionParamKeys` concern 从 12.3k 行 god-file 拆出，`action_runner.go` LOC ratchet 从 12353 下调至
   12344，没有为接线抬高预算。

看护覆盖：role-filter alias 正臂、未知参数负臂、canonical/alias 冲突负臂、reject-filter 不回归，以及既有 qualify→
contribution、自动 status block、compute phantom-param 合同。验证：定向测试与完整
`go test ./internal/dataquery ./internal/dataworkflow ./internal/repl -count=1` 全绿。状态：
`EVAL-B76-DATAPARAM1=qualify-family implemented/replay-next`；其他 action family 的参数消费清册仍为
`B76-C=open`，未以本批局部覆盖冒充全仓闭环。

## 79. 2026-08-04 B77 r3：资格参数根修回放闭环；DAG rank 上下文仍制造失败边

在 `main@b91dd9c3e` 构建后严格并行两个异构 data 案例，runner 2/2 PASS，人工审计 2/2 PASS：

- `data_multifile_reference_projection`：297s，最终 `17,0,5`；
- `data_join_entity_reconcile`：175s，最终 `30`。

### EVAL-B77-DATAPARAM1（closed for qualify family）：静默 filter no-op 已转为真实消费

主案的资格决策不再包含 inactive r3：决策/贡献链最终为 rules=9、decisions=9、contributions=4、reconcile=pass，
GroupA 恢复 10+7=17。reference-complete projection 随后按 targets 顺序发射 GroupA、GroupX、GroupC，补零并丢弃
reference 外 GroupB，终稿精确为 `17,0,5`。全程没有 unknown-param 或 alias-conflict 误报。

异构案完整经过 derive-rules→normalize-entities→join/derive/filter→compute→reconcile→assemble，最终两条 Alpha
贡献 20+10、实体归一 3 条、决策 5 条、输出 `30`。这证明 dispatcher 上的 contract 没有越界套到其他 action family，
也没有读取业务名、规则文本或答案数字硬化结论。

### EVAL-B77-DAGCTX1（P1/confirmed）：typed stage 存在，但 planner 仍频繁构造未来 rank

两案都完成正确，却同为 9 rounds/2 repairs；主案记录 5 个失败边，归一案记录 3 个。唯一错误集合显示为通用 DAG
规划问题，而非计算值问题：

1. 在 typed action 可用时仍生成无 script 的 `custom_transform`；
2. `join_records` 明确最多两个输入，候选却携带三个 input paths；
3. current `next_stage` 仍在 material/rule/entity/decision rank 时，同一 batch 提前加入 compute、reconcile 或 final action；
4. resolution ledger 的 source lineage 与待 apply base lineage 不兼容；
5. 主案首次 assemble 仍省略 complete-reference，但既有 grounding validator 确定性拒绝并由 repair 恢复。

现有 hard guards 都正确 fail-closed，不应放松；问题是 model-facing typed scaffold 没有把“本轮可发的 action kind、每种 kind
的输入基数/兼容 node class、当前 rank 的 terminal boundary”压缩成足够靠近 emit schema 的结构化候选，导致模型先发一个
大计划再由 guard 拆。根修应从已有 `workflow_state_json.allowed_next_actions` 与 action schema registry 派生 bounded
`action_candidates`，每项携带 `min/max_inputs`、compatible node classes 和 `same_rank_only=true`；作为精确上下文与软引导，
最终 admission guard 继续保留。不得扫描 question/final prose，也不得自动替模型选择业务 action 或改答案。

优先级保持 P1：本轮答案与审计 ledger 均正确，且错误集合跨两个异构案但仍可能包含模型波动。先完成 P0 的 B76-C
parameter-consumption registry 清册，再以更多 data 异构回放判断候选 scaffold 的真实 ROI；若连续出现则施工。

`EVAL-B76-DATAINPUT1` 本轮未再出现 workflow ledger 被当 record input，但没有代码修复，状态降为
`adoption-watch`，不得据单轮波动收账。

证据：

- `eval/parallel_selected_summary_evalcampaign_b77_dataparam_r3_20260804.md`；
- `eval/parallel_selected_summary_evalcampaign_b77_dataparam_r3_20260804_manual_audit.md`；
- result dirs：`eval/results/*-20260804-062518`。

### B76-C1 implementation（implemented/tests-pass）：selection/diagnostic family 扩族

parameter-consumption registry 第二批覆盖 `filter_records` 与 `value_distribution`；加上已闭环的
`qualify_records` 和原有严格 `compute_contributions`，最易直接改变入选集合/聚合值的四个 action family 已不再允许
未知参数静默成功。

1. `filter_records` 的 source/base/record filters 作为显式 include-filter aliases 归一到 `filters_json`；canonical/alias
   冲突 fail-closed，`filter_field/op/value` shorthand、rule refs、item id、reason 与 record/output limits 保持可用；
2. `value_distribution` 的 input/record/base path 与 fields/fields_json/field/target_fields 进入明确 alias groups；未知的
   `top_values` 等键不会再被当作成功配置，需改用 canonical `top_n`；
3. 两者继续走 dispatcher 上同一 typed contract 边界，不在各 executor 内复制第二套判断；
4. 新看护覆盖 filter role alias 的真实行级消费、filter unknown/conflict 两个负臂、distribution unknown 负臂；既有
   malformed filter、decision ledger、数值/字段合同与 distribution 正臂全部继续通过。

验证：定向 selection-family 测试与完整
`go test ./internal/dataquery ./internal/dataworkflow ./internal/repl -count=1` 全绿。状态：
`EVAL-B76-DATAPARAM1=selection+aggregation high-risk families covered`；B76-C2 将审计多输入、多角色的
mapping/normalize/apply/enrich/join families，必须先按实际读取集建合同，不能把 source/reference aliases 错合并。

## 80. 2026-08-04 B78 r4：selection contract 异构格式回放通过；helper API 形态仍含糊

在 `main@cf32d6048` 构建后严格并行 text 与 JSONL 两个 data case，runner/human 均 2/2 PASS：

- `data_text_filter_count`：33s，1 round、0 repair、0 failure，严格输出 `2`；
- `data_jsonl_filter_count`：96s，最终 `2`，decisions=6、rules=2、contributions=2、reconcile=pass。

JSONL 案的 typed `filter_records` 正确筛出 service=api 且 level=error 的两行，随后 compute/reconcile/assemble 的值与
终稿一致；新增 filter/value-distribution 参数合同没有产生 unknown/conflict 误报。selection family 的生产回放闭环。

### EVAL-B78-DATAHELPER1（P1/confirmed）：注入函数被误解成可 import 模块

JSONL 首轮 custom script 失败于：

```text
ImportError: data task import is blocked: jsonl_rows
```

`jsonl_rows` 是 runner 注入的 callable helper，不是 Python module；import sandbox 正确 fail-closed，不应为通过 eval
开放任意 import。gap 在 model-facing script API：helper 名清单没有足够明确地表达“已在 env 中、直接调用、不得
import”，使模型按模块习惯猜测。后续又因 rule coverage/material stage 尚未完成而提前发 custom action，进一步印证
B77-DAGCTX1 的 current-rank 上下文问题。

根修方向：由 runner 实际注册表生成 typed `script_api`（name、kind=callable、arity/参数形、direct_call=true、
importable=false），和当前 `custom_transform_disabled`/allowed actions 同源注入 planner；这只做精确上下文与软指导，
import sandbox 与 action admission 继续硬守。不得扫描模型 script 字符串特判 `jsonl_rows`，也不得为单一格式自动改写
模型代码。先随 B77-DAGCTX1 继续跨格式观察，再按出现频率排期。

证据：

- `eval/parallel_selected_summary_evalcampaign_b78_selection_r4_20260804.md`；
- `eval/parallel_selected_summary_evalcampaign_b78_selection_r4_20260804_manual_audit.md`；
- result dirs：`eval/results/*-20260804-063817`。

### B76-C2 implementation（implemented/tests-pass）：mapping-candidate 与 join 多角色合同

多输入 action 第一批覆盖 `mapping_candidate` 与 `join_records`：

1. `mapping_candidate` 的 source/reference path、source/reference field selectors、canonical id/label、match mode 与 limits
   按实际 executor 读集列入合同。source/reference field selector 的多键合并语义保持原样，不被错误压成单值 alias；
2. mapping candidate 当前并不执行 source/reference filters，因此这类参数现在明确 typed reject，不再发一个看似筛选、
   实际对全表生成 candidates 的成功产物。需要先用 `filter_records` 物化角色输入，或改用真实支持双侧 filters 的
   `normalize_entities`；
3. `join_records` 把 left/right paths、left/right field aliases、shared join fields、join type、prefix/collision 与 limits
   归一到 executor 真实读取形。共享 `join_fields` 只在 left selector group 铸造，right 未声明时继续沿用 left fields；
   显式 right fields 仍可与 shared left fields 不同；
4. 同一侧多个 field aliases 值不一致时 fail-closed，未知 `join_on` 等键拒绝，避免“planner 以为按 X join、runner
   实际按另一键 join”的静默语义漂移。

看护覆盖 mapping 正常 candidate artifact、mapping ignored-role-filter 负臂、join unknown/conflict 负臂，以及全部既有
join alias/左右字段/推断/left-join/zero-match 正臂。完整
`go test ./internal/dataquery ./internal/dataworkflow ./internal/repl -count=1` 全绿。状态：
`EVAL-B76-DATAPARAM1=mapping-candidate+join covered`；B76-C3 继续 normalize/apply/enrich，三者的 source/base 与
reference/resolution filters 必须保持角色隔离。

## 81. 2026-08-04 B79 r5：多角色参数合同无回归；source-derived reference alias 丢失输出权威

在 `main@81b72a5ac` 构建后严格并行两个多角色 data case：

- `data_join_entity_reconcile`：127s，runner PASS / human PASS，最终 `30`；
- `data_multifile_reference_projection`：159s，runner FAIL / human FAIL，最终 `17,4,5`，应为 `17,0,5`。

异构 join 案完整得到两条 Alpha 贡献 20+10、reconcile pass 和严格单值终稿；新增 mapping/join 参数合同未产生
unknown/conflict 误报。主案的 filter、join、贡献聚合也都正确：GroupA=17、GroupB=4、GroupC=5。故本轮不是
B76-C2 回归，错误只发生在最终输出 reference authority。

### EVAL-B79-DATAREFROLE1（P0/confirmed）：typed assemble 输入别名未解析回 source reference

最终 `assemble_answer` action 已携带三项精确结构信号：

1. `input_paths=[reconciled_contributions, targets]`；
2. `reference_key_field=canonical_label`；
3. 输出是 strict `plain_single_line` values projection。

但它省略了 `complete_reference=true/reference_path=targets.csv`。系统不能、也不应读取模型 think、action purpose、
validation rule 或用户原文来猜这层意图；现有代码本来已把“assemble input + reference key field”定义为与显式
reference_path 同等级的 typed declaration。生产漏洞在该声明的别名解析：

- `dataTaskAssembleActionReferenceProjectionGap` 直接让 runner 从 action input alias 取 candidate；`targets` 能读出三条
  key，却仍以 workflow-generated alias 身份返回；
- 随后 `dataTaskReferencePathIsWorkflowMaterial(candidate.Path)` 只接受源材料路径，拒绝 alias `targets`；
- 兄弟硬校验 `dataTaskResolveDeclaredOutputReferenceSet` 已有“generated alias → 唯一 source material lineage → 重新读源
  bytes”的能力，但只消费显式 `params.reference_path`，不消费 assemble 的 typed input paths；
- 因此 grounding guard fail-open，普通 reconcile-group projection合法发出 `17,4,5`，内部 answer↔reconcile 自洽却未与
  targets reference set 对账。终态错误显示 `status=complete`、`reference_complete=true`，进一步掩盖缺失权威。

这是一类“source-derived data artifact role 丢失”问题，不是 targets 文件名拟合。根修冻结为：

1. 统一 declared-reference resolver：显式 `reference_path` 保持最高优先级；没有显式 path、但 assemble action 携带
   typed reference key field 时，才从 action input paths 逐个走既有 source-lineage credential；
2. 只有恰好一个可解析 source universe，或多个候选的 key sequence 完全一致时才建立 hard authority；候选冲突、无唯一
   source lineage 均 fail-open，不按文件名、purpose、字段词义或行数猜选；
3. grounding guard 只拒绝错误投影并给模型 typed repair proposal；系统不改贡献、不代写最终数值、不扫描 question/final
   prose；
4. 看护必须复现 `targets` alias 正臂、两个不同 reference universe 歧义负臂、generated-only alias 负臂、显式
   reference_path 优先臂，以及正确 `17,0,5` 放行臂。

状态：`EVAL-B79-DATAREFROLE1=confirmed/implementation-next`。`EVAL-B77-DAGCTX1` 继续为 P1；本轮主案 1 个、
join 案 2 个失败边，仍说明未来 rank 规划有成本，但没有高于当前错误终稿 P0。

证据：

- `eval/parallel_selected_summary_evalcampaign_b79_multirole_r5_20260804.md`；
- `eval/parallel_selected_summary_evalcampaign_b79_multirole_r5_20260804_manual_audit.md`；
- result dirs：`eval/results/*-20260804-064620`；
- 终态：`.codrax/data-audit/20260804-064854-752222-88658-terminal.json` 与
  `.codrax/data-audit/20260804-064824-430818-88657-terminal.json`。

### EVAL-B79-DATAREFROLE1 implementation（implemented/tests-pass/replay-next）

根修复用 `dataTaskResolveDeclaredOutputReferenceSet` 的单一 source-lineage 权威链，并把 assemble action 的声明优先级
明确为：`output_contract.complete_reference + reference_path` → action 显式 `reference_path` → 仅在前两者缺席时，
`reference_key_field + input_paths`。最后一臂不把 input alias 直接当 source material，而是逐一要求它能回溯到唯一真实源
材料并重读其 key bytes。

候选合取规则保持保守：无 source lineage 的 generated-only alias 不取得权威；两个 input alias 解析到不同 key sequence
时不按名字、重叠率、行数或字段词义排序，整臂 fail-open；显式 path 与 input alias 冲突时显式 path 胜出。由此 B79 的
`targets` 精确回溯到 `targets.csv#canonical_label`，错误 `17,4,5` 进入既有
`output_reference_grounding_mismatch` 并重新开放 model-owned `assemble_answer`，正确 `17,0,5` 放行。系统仍不改变
contribution ledger 或答案字节。

新增四组看护覆盖 source-derived input alias 红/绿双臂、不同 source universe 歧义、generated-only alias、显式 path
优先；live workflow state 还钉住错误答案必须呈现 `grounding_mismatch`、不得签 `complete`。完整
`go test ./internal/dataquery ./internal/dataworkflow ./internal/repl -count=1` 全绿。状态更新为
`EVAL-B79-DATAREFROLE1=implemented/replay-next`。

## 82. 2026-08-04 B80 r6：reference lineage gate 生效但无法被执行器解除；普通聚合不回归

在 `main@dc08c89d8` 构建后严格并行主 witness 与无完整 reference obligation 的普通聚合：

- `data_basic_sum_with_rules`：43s，runner/human PASS，最终 `44`；
- `data_multifile_reference_projection`：598s，runner/human FAIL，terminal status=failed，未发布错误答案。

普通聚合的 rows/rules/contributions/reconcile/终稿一致，证明 B79 的 input-alias authority 没有误扩到无 reference 声明的
任务。主案也证明 B79 核心修复真实接线：终态明确为 `output_reference_grounding_mismatch`，reference candidate 精确回溯到
`targets.csv#records#canonical_label`，错答案不再像 B79 那样签 complete。但端到端仍未闭环：15 data rounds、6 repair、
8 failed actions 后诚实失败。

### EVAL-B80-DATAREFEXEC1（P0/confirmed）：hard authority 与 executor activation 合同矛盾

grounding authority 的既有裁定是：显式 complete-reference output contract，或 `assemble_answer` 的 typed
`reference_path + reference_key_field`，任一都足以建立完整参考域。B79 修复只补齐前者遗漏时的 input-alias lineage；本轮
四个 repair action 已逐字携带：

```text
projection=values
reference_path=targets.csv#records
reference_key_field=canonical_label
```

但 `completeAssembleAnswerGroups` 仍只读取 `action.params.complete_reference` 或 output contract 的 bool。repair 省略这个
冗余 bool 后，执行器不进入 reference projection；它把 seed 中三条 business groups 与旧 `final_answer/projection` 组一起
按 values 发射，得到 `17,4,5,<旧 key_values 答案>`。旧 answer-scope group 随即与新混合答案不等，触发
`answer_reconcile_mismatch`。模型随后四次发出语义相同且 path/key 正确的 repair，全部被同一矛盾合同拒绝。

这不是模型波动：同一 typed pair 在 hard gate 侧“足以确权”，在 executor 侧“权限不足”，构成确定性的不可满足合同，
正是“成文校验未通过”重试轰炸的一类根因。根修必须让执行器与 declared-reference resolver 同源：仅 action 显式同时
携带非空 path+key 时，activation 可等价补足 complete-reference；output contract 只有 path/key、没有 bool 时仍不自动升级，
保持现有“不完整 contract 不投影”负臂。

### EVAL-B80-DATAREFSCOPE1（P0/confirmed）：action input reference 被全历史 artifact 最大集抢权

首次 `step8_assemble_answer` 已声明 `complete_reference=true`、`reference_key_field=canonical_label`，且 typed
`input_paths=[reconcile_result, targets.csv#records]`。执行器却把这些 input paths 与所有历史 artifacts 合并进一个 fallback
池，`referenceCandidateForAssemble` 再按最大 key count 选优，最终选择 `all_records` 的 4-key universe，而非 action 明确
携带的 targets 3-key universe。artifact receipt 亲证：`reference_path=all_records/reference_total=4`。

根修应建立两级 scope，不按名字或业务词义：先只在 `action.InputPaths` 内解析 typed key field；若得到唯一 candidate 就
立即使用，不得让未声明历史 artifact 抢权；仅 input scope 完全无 candidate 时，才允许既有 artifacts fallback。输入 scope
内部仍沿用现有结构候选规则，多个候选的处理不可增加领域常量。

两项同批看护：

1. action explicit path+key、无 bool，必须投影并替换 stale final-answer group；
2. contract path+key、bool=false、action 无显式 pair，保持 present-groups 旧语义；
3. input targets 3 keys + unrelated artifact 4 keys，targets scope 必须胜出；
4. input 无 candidate 时，历史 artifact fallback 仍工作；
5. projection receipt 的 path/count/zero-fill/drop-extra 与最终 answer 同源。

状态：`EVAL-B79-DATAREFROLE1=implemented/guard-proven`；
`EVAL-B80-DATAREFEXEC1=confirmed/implementation-next`；
`EVAL-B80-DATAREFSCOPE1=confirmed/implementation-next`。

证据：

- `eval/parallel_selected_summary_evalcampaign_b80_dataref_r6_20260804.md`；
- `eval/parallel_selected_summary_evalcampaign_b80_dataref_r6_20260804_manual_audit.md`；
- `eval/results/data_multifile_reference_projection-20260804-070345`；
- terminal `.codrax/data-audit/20260804-071335-851931-99810-terminal.json`。

### B80-C implementation（implemented/tests-pass/replay-next）：reference authority 与执行域同源

本批在 `assemble_answer` 执行边界消除了两份互相矛盾的完整参考域合同，没有增加题型、文件名或答案文本特判：

1. action 自身显式同时声明非空 `reference_path + reference_key_field` 时，与 hard grounding authority 使用相同的 typed
   充分条件，可启动 complete-reference projection；模型无需再补一个语义冗余的 `complete_reference=true`；
2. 只有 output contract 中存在 path/key、但 contract bool=false 且 action 未显式声明这对字段时，仍保持不启动，防止
   输出合同的残缺字段被执行器自行升级；
3. reference candidate 优先级冻结为“action 显式 path → action typed input scope → 历史 artifact fallback”。typed input
   scope 已得到 candidate 时，未被该 action 消费的更大历史全集不能按 key count 抢权；
4. input scope 内多个 candidate 的 key sequence 不一致时 fail-closed：整次 reference projection 不发 receipt，也不得再
   回退历史 artifact 猜选；只有 input scope 完全没有可读 candidate 时，既有历史回退才继续可用；
5. projection 成功后继续替换旧 `final_answer/projection` reconcile group，最终 answer、reference path/count、zero-fill 与
   drop-extra receipt 来自同一次确定性投影，避免修复轮把旧终稿当第四个业务值重新拼入。

新增看护覆盖：显式 pair 无 bool 的 `17,0,5` 正臂、targets 三键压过未声明 all-records 四键、无 input candidate 的历史
回退、两个不同 input universe 的歧义负臂；既有“不完整 output contract 不启动”负臂与 LOC ratchet 同时保持。验证：
定向看护及完整 `go test ./internal/dataquery ./internal/dataworkflow ./internal/repl -count=1` 全绿。

状态更新：`EVAL-B80-DATAREFEXEC1=implemented/tests-pass/replay-next`；
`EVAL-B80-DATAREFSCOPE1=implemented/tests-pass/replay-next`。下一批以同一 witness + 普通非 reference 聚合严格并行 2 个进行
production 回放，验收 repair 轮数不再被不可满足合同烧尽、正确 `17,0,5` 可由模型 action 正常生成且普通聚合无回归。

## 83. 2026-08-04 B81 r7：reference executor 已通；entity resolution 的隐式行号错接静默污染值

在 `main@43bfbe5e5` 构建后严格并行 B80 witness 与普通非 reference 聚合：

- `data_basic_sum_with_rules`：32s，runner/human PASS，严格终稿 `44`；
- `data_multifile_reference_projection`：341s，runner/human FAIL，错误发布 `33,0,0`，应为 `17,0,5`；15 data rounds、
  5 repairs、8 prior errors。

B80 两项修复已接入 production：targets 显式 path/key 能启动完整三键投影，后续 values repair 也能替换 keyed 终稿；普通
聚合没有被误升级。失败根因已经上移到 projection 之前，且不是 contribution 重放累积：终态只有 5 条独立 contribution，
但其中两条的实体身份在 `apply_entity_resolutions` 阶段已经被错误改写。

### EVAL-B81-DATARESID1（P0/confirmed）：source-value 身份被碰巧相等的隐式 locator 抢权

源材料真值为：active `A-one=10`、`A-two=7`、`Beta=4`、`Gamma alt=5`、`unmapped=11`；reference ledger 正确记录
`Gamma alt→GroupC`，并把 `unmapped` 标成 unresolved。可是 apply 产物逐字显示：

```text
r5 / Gamma alt / 5  -> canonical_id=A-one / canonical_label=GroupA / resolved
r6 / unmapped / 11  -> canonical_id=A-two / canonical_label=GroupA / resolved
```

因此 GroupA 被算成 `10+7+5+11=33`，GroupC 丢失；artifact receipt 还错误声称 `matched=6/unmatched=0`。随后 reconcile
只复算污染后的 contribution，自洽地签 pass，最终 reference projection 也只能诚实输出污染后的 `33,0,0`。

代码机制是双身份索引顺序错误：`apply_entity_resolutions` 已从 resolution ledger 构建
`source_value + source_field` 精确索引，但执行时先尝试 locator/行号索引，只有行号无匹配才回退 source value。多材料抽取、
child alias、过滤或重物化后，局部/全局数字索引可能与另一条 resolution 的 locator 数字碰巧相同；一旦命中，真实
source value 不再参与合取，形成静默错接。这个风险泛化到任何多源 flatten、filter、join 后的 resolution apply，不是本案
标签名拟合。

根修冻结为：

1. action 显式声明 `base_key_fields` 时维持最高优先级，不改变明确结构合同；
2. 未显式声明 base key、且 ledger 同时提供 source value/source field、base 也存在对应字段时，以 source-value 身份为
   authority；唯一等价 canonical choice 才可应用；
3. source value 在 base 可读但 ledger 没有 accepted match 时必须保持 unmatched，不得退回隐式行号猜配；多个不等价
   canonical choices 保持 ambiguous；只有 source-value 身份本身不可用时才允许既有 locator fallback；
4. 看护构造前置多材料行导致数字空间重叠，钉住 Gamma 正确解析、unmapped 保持 unmatched、matched/unmatched receipt，
   并保留显式 locator contract 的既有正臂。

状态：`EVAL-B80-DATAREFEXEC1=production-proven`；`EVAL-B80-DATAREFSCOPE1=production-proven`；
`EVAL-B81-DATARESID1=confirmed/implementation-next`。

证据：

- `eval/parallel_selected_summary_evalcampaign_b81_dataref_r7_20260804.md`；
- `eval/parallel_selected_summary_evalcampaign_b81_dataref_r7_20260804_manual_audit.md`；
- `eval/results/data_multifile_reference_projection-20260804-073033`；
- terminal `.codrax/data-audit/20260804-073603-259412-11745-terminal.json`；
- final result `.codrax/data-audit/20260804-073550-414362-11745-result-r15.json`。

### B81-C implementation（implemented/tests-pass/replay-next）：resolution 身份优先级收敛

施工只修改 `apply_entity_resolutions` 内一处 typed choice 选举，不改变 planner 文本、模型 action 或业务字段：

1. 显式 `base_key_fields` 继续拥有最高优先级；结构 key 命中即使用，结构 key 未命中时仍保留既有 source-value fallback，
   不破坏明确 locator/join-key 合同；
2. action 未显式声明 base key，且 resolution ledger 的 `source_field/source_value` 能在 base 字段中读取时，source-value
   候选成为权威；零 accepted choice 直接记 unmatched，不再让隐式 `_source_index`/row number 抢权；
3. 同一 source value 在多条源记录中产生完全相同 canonical id/label/status 时按结果身份合并，避免重复值被伪装成
   ambiguity；若 canonical 结果不同则仍保留多 choice，由原有 ambiguous 车道 fail-closed；
4. source-value 身份不可用时，既有 locator fallback 继续工作，兼容只携带 item locator、没有 source value 的 ledger。

新增多材料 extract → normalize → apply 的完整回归，前置 labels 四行使 observations 的数字索引与 resolution locator 发生
重叠：修复前稳定得到 `matched=6/unmatched=0` 并把 Gamma/unmapped 错接到 GroupA；修复后钉住
`matched=5/unmatched=1`、Gamma→GroupC、unmapped→unmatched。另有对抗用例钉住显式 base key 与 source value 冲突时
显式 key 必须获胜。

验证：全部 `TestActionRunnerApplyEntityResolutions*` 与新增身份用例通过；
`go test ./internal/dataquery ./internal/dataworkflow` 全绿；`internal/repl` 全包首次出现一次与本改动无关的
cancel-listener 一次性告警时序失败，单测与整包立即复跑均绿。LOC ratchet 保持原上限，`action_runner.go=12324 < 12344`。

状态更新：`EVAL-B81-DATARESID1=implemented/tests-pass/replay-next`。下一批仍严格并行 2 个：原多文件 witness + 一个
异构 entity-resolution case，验收值 ledger、receipt、reconcile 与终稿，而不是只验 runner 标签。

## 84. 2026-08-04 B82 r8：entity identity production 闭环；角色交换后的 parent lineage 反写

在 `main@ebbc26bc1` 构建后严格并行原 P0 witness 与异构 entity/join case，runner/human 均 2/2 PASS：

- `data_multifile_reference_projection`：232s，`17,0,5`；contributions 精确恢复为 4 条：GroupA=10+7、GroupB=4、
  GroupC=5，reconcile 与三键 reference projection 同源；13 rounds、2 repairs、5 prior errors；
- `data_join_entity_reconcile`：396s，`30`；Alpha 两条 contribution、reconcile pass；13 rounds、2 repairs、8 prior
  errors。

因此 `EVAL-B81-DATARESID1=production-proven`。主 witness 未再出现 Gamma/unmapped 错接，且显式 reference 终稿闭环。
异构案最终正确，但过程揭示另一个确定性 hard-contract 错误。

### EVAL-B82-DATALINROLE1（P0/confirmed）：自动 role swap 已生效，parent lineage 却按原输入顺序反写

`normalize_entities` 收到反序的 `input_paths=[canonical_records, items_records]` 后，字段合同正确触发 role swap；其 typed
children 亲证实际角色：

```text
items_records#entity_source          source_record_paths=[items_records]
canonical_records#entity_reference  reference_paths=[canonical_records]
```

三条 resolution 的 item_id 也都是 `items_records#N:raw_name`，canonical 值来自 canonical_records。但 parent
`name_resolutions` 最终却写成：

```text
source_record_paths=[canonical_records]
reference_paths=[items_records]
```

随后 `apply_resolution_lineage_contract` 读取错误 parent authority，两次硬拒把该 ledger 应用到真正 source
`items_records` 及其合法派生 `items_with_amount`，错误消息逐字声称 resolution source lineage 是 canonical_records。模型被迫
改走 join，额外经历 mapping_candidate 无进展、deferred 丢弃与参数 repair，才在 396s 后完成。

代码根因位于 `runNormalizeEntities` 的 parent lineage 铸造：reference-mode executor 已在字段推断后交换实际 roles 并写入
typed child，但 `entityResolutionLineageRoles` 又从原 action params/input 顺序独立推断第二次，覆盖了执行事实。这是精确
hard gate 读取了错误 typed authority，不应通过放宽 lineage compatibility 掩盖。

根修冻结为：

1. parent `SourceRecordPaths/ReferencePaths` 优先从本次 executor 产出的 typed source/reference child 单源汇总；
2. 只有 children 没有明确角色（例如 explicit inline resolutions）时，才回退 action 显式 source/reference params，再回退
   input order；
3. transitive descendant（derive/filter 等）继续由现有 artifact lineage 图证明兼容，不新增相似度、字段名或文件名猜选；
4. 看护需覆盖反序输入触发 swap 后 parent/child 角色一致、原 source 与 derive descendant 均通过 guard、真正无关 base 仍
   被现有 hard guard 拒绝。

状态：`EVAL-B81-DATARESID1=production-proven`；`EVAL-B82-DATALINROLE1=confirmed/implementation-next`。

证据：

- `eval/parallel_selected_summary_evalcampaign_b82_dataresid_r8_20260804.md`；
- `eval/parallel_selected_summary_evalcampaign_b82_dataresid_r8_20260804_manual_audit.md`；
- `eval/results/data_multifile_reference_projection-20260804-075237`；
- `eval/results/data_join_entity_reconcile-20260804-075237`；
- terminals `.codrax/data-audit/20260804-075620-593018-23468-terminal.json`、
  `.codrax/data-audit/20260804-075903-367269-23467-terminal.json`。

### B82-C implementation（implemented/tests-pass/replay-next）：parent lineage 消费 executor 实际角色

本批没有放宽 `apply_resolution_lineage_contract`，而是修正它读取的上游 authority：

1. `normalize_entities` parent 的 `SourceRecordPaths/ReferencePaths` 现在优先汇总本次 executor 已铸造的
   `entity_resolution/source` 与 `entity_resolution/reference` typed children；字段合同触发 role swap 后，parent 与 child
   必然同源；
2. generic structured normalization 的 `entity_resolution_source` child 同样可提供 source lineage；explicit inline
   resolution 没有 role child 时，才回退 action 显式 source/reference params，再回退 input order；
3. evidence paths 继续由 resolution records 与非 source/reference consumed material 构造，未丢失第三方证据；
4. lineage guard、alias matching、transitive lineage 图均未降级：source 的 derive descendant 通过，真正 unrelated base 仍
   发 `apply_resolution_lineage_contract`。

既有 reference-first input 测试由“只看映射结果与 swap note”扩展为 parent/所有 resolution child 的 role parity；新增
dataworkflow 看护验证 derived base 正臂与 unrelated base 负臂。定向 normalize/apply/guard 全族及完整
`go test ./internal/dataquery ./internal/dataworkflow ./internal/repl -count=1` 全绿；核心 LOC ratchet 保持，
`action_runner.go=12296 < 12344`。

状态更新：`EVAL-B82-DATALINROLE1=implemented/tests-pass/replay-next`。production 回放仍采用严格并行 2 个：原 join witness
观察合法 apply 是否不再被 parent 角色反写阻断，并配一个 read/write/operation 高优先级 case，避免 data 战役长期挤占其他
模式覆盖。

## 85. 2026-08-04 B83 r9：lineage role production 闭环；write 全链路无回归

在 `main@4a87c0aef` 构建后严格并行 data witness 与 write end-to-end，runner/human 均 2/2 PASS：

- `data_join_entity_reconcile`：388s，最终 `30`；2 条 Alpha contribution、reconcile pass；10 data rounds、2 repairs、
  7 prior errors；
- `patch_go_typo`：147s，单行 `retrun→return` patch，apply 成功、1 个测试通过、workflow verified，主仓未改。

与 B82 同一 data case 对比，错误的两次 `apply_resolution_lineage_contract` 已完全消失，data rounds 从 13 降到 10；合法
source descendant 能直接携带 canonical field 进入 filter/contribution。值、contribution count 与 reconcile 继续正确，说明
不是绕过 guard 换来的假绿。`EVAL-B82-DATALINROLE1=production-proven`。

剩余 7 个 prior errors 分别是空 action input、跨 dependency rank、rule/material stage 提前消费、allowed-next-action 越阶；
它们归属于既有 `EVAL-B77-DAGCTX1`，本轮没有新的 value/authority 错误。后续应以 planner 的 typed current-rank context 与
deferred readiness 为统一根修，不为 join case 新增 action 序列特判。

write 席人工核对 plan、patch、apply report 与测试结果一致：只有 `main.go` 第 25 行变化，验收三项完整，隔离 worktree 保留。
这也验证本轮 dataquery/dataworkflow 变更没有侵入 read/write scheduler 红线。

证据：

- `eval/parallel_selected_summary_evalcampaign_b83_lineage_write_r9_20260804.md`；
- `eval/parallel_selected_summary_evalcampaign_b83_lineage_write_r9_20260804_manual_audit.md`；
- `eval/results/data_join_entity_reconcile-20260804-080751`；
- `eval/results/patch_go_typo-20260804-080751`；
- terminal `.codrax/data-audit/20260804-081413-676815-30860-terminal.json`。

## 86. 2026-08-04 B84 r10：MERGE-AUDIT-5 复核后 write/operation 双席；结构化步骤不得降格为 shell

### 86.1 严格并行结果与人工结论

在 `main@aa62cb906` 构建后严格并行恰好两个异构 case：

- `patch_go_typo`：runner PASS / human PASS，124s；单行 patch、apply report、隔离 worktree diff 与 Go 验证一致，最终 typed 状态 `verified`；
- `operation_web_manual_summary`：runner FAIL / human FAIL，38s；失败原因为 `operation_terminal_event_missing`，但深因发生在任何手册材料读取之前。

write 席没有 answer-contract/finalizer reject，也没有系统改写答案。Analyzer 首次把 `is_role_locate_lookup=true`
与非 scalar 合用后自行纠正一次，属于单一 schema 约束反馈，不是“同一声明既必带又必拒”的矛盾成文合同，未登记新 gap。

### 86.2 `EVAL-B84-OPSTRUCT1`（P1/confirmed）：nested command-step 数据被当 shell 执行

operation planner 发出的 `steps` 是一段 stringified command-object array；其中 regex 参数含 nested JSON 非法转义
`doc\|manual`。旧链发生两次权限泄漏：

1. `flexibleCommandPlanSteps.UnmarshalJSON` 无法解码 nested array 后，把整个 container 兼容降格为单条 `Shell`；
2. `structuredCommandField` 只有 `json.Valid` 才识别结构化 command，因此同一非法转义让 lint/executor 也放行；
3. shell 实际收到 `[{"id":...}]`，以 `command not found` 失败。后续 replan 改为写本地 HTML，策略正确要求人工审批，eval 因而没有 terminal event。

这是 typed planner carrier 的结构边界问题，不应通过手册 URL、命令名或答案文本特判。最优根修已落地：

- 解析层识别“JSON object/array 形 + command step 精确字段”的 malformed container，触发既有 compact structured repair，禁止 fallback shell；
- operation lint/executor 共用同一结构识别函数，形成执行前第二道 fail-closed；
- 合法 stringified typed steps、普通 bare shell 与 `[ -f ... ]` shell test expression 均保留正向 pin；
- malformed witness 必须在执行前转成 typed repair，executor 直达负控必须留下 `invalid_plan / command was not run`。

定向与完整验证：`go test ./internal/operation ./internal/repl -count=1` 全绿。修复不读取用户输入或模型答案 prose，
不改变 read/write 路由、显式时间窗 Trace 因果投影、自动补齐或模型结论所有权。

状态：`EVAL-B84-OPSTRUCT1=implemented/tests-pass/replay-next`。证据见：

- `eval/parallel_selected_summary_evalcampaign_b84_writeoperation_r10_20260804.md`；
- `eval/parallel_selected_summary_evalcampaign_b84_writeoperation_r10_20260804_manual_audit.md`；
- `eval/results/operation_web_manual_summary-20260804-082011`；
- `eval/results/patch_go_typo-20260804-082011`。

下一批提交推送后仍严格并行 2 个：operation 原 witness 验证 malformed container 不再执行，并配一个非 operation
高优先级 case，避免只对单一 web/manual 场景拟合。`EVAL-B64-COUNTBIND1`、`EVAL-B59-INVROW1`、
`EVAL-B60-CLOSURECHURN1` 继续按 §10.15 的 typed-identity/异构复现门槛排队。

## 87. 2026-08-04 B85 r11：operation 结构边界生产闭环；Cangjie 复合行身份再次见证

### 87.1 严格并行结果

在已推送 `main@e014c06f4` 上严格并行恰好两个异构 case：

- `operation_web_manual_summary`：runner/human PASS，80s；
- `cangjie_repomap_fixture`：runner/human 主答案 PASS，53s，零 finalizer reject。

operation 先完整获取首页，从系统提取的 typed href 选择 `./user_guide.html`，再完整获取目标手册。terminal 同时携带：

- `material_coverage_status=complete`；
- 20 个 material pages、118,802 visible runes，source/pages 均未截断；
- 248,161-byte source identity；
- `user_guide.html` source locator；
- SHA-256 绑定的 coverage receipt。

因此不是“任意完整网页”伪装成目标完成。旧 malformed serialized steps 没有进入 shell，也没有触发 local-write replan/
manual approval，`EVAL-B84-OPSTRUCT1=production-proven/closed`。

### 87.2 `EVAL-B59-INVROW1`（P1/confirmed-repeat）：主清单正确但系统追加虚假不确定性

Cangjie 主表精确列出 `native_add@Bridge.cj:6`、`Bridge@15`、`Cart class@14`、`Cart extend@30`、`App@11`，
每行的 declaration family、package、file:line 均与 typed source-inventory row 一致，五条引用也完整发射。

但 `principal_support_member_coverage` 仍产生 4 条 soft advisory；系统随后追加“部分项证据支持稍弱”和“补充定位/
钻取步骤未执行”。这两句与 `complete=true + 5 rows + 5 citations` 不一致。根因方向与 B59 一致：两个 `Cart`
拥有相同 visible label，但以 `source + line + declaration family` 才能区分；label-only 匹配无法证明两席分别覆盖，
系统把身份表达不足误投影成证据不足。

本轮不为 Cangjie/`Cart` 增加关键词规则，也不删除 soft checker。下一独立批应先冷读确认 principal row matcher 是否已
携带 citation location/role；若没有，以 typed composite identity 统一修复 inventory、重载函数、同名跨 package、
class+extension 等一类问题。系统披露只能来自真实未覆盖席，不能用 label ambiguity 铸造不确定性。

证据：

- `eval/parallel_selected_summary_evalcampaign_b85_operationcangjie_r11_20260804.md`；
- `eval/parallel_selected_summary_evalcampaign_b85_operationcangjie_r11_20260804_manual_audit.md`；
- `eval/results/operation_web_manual_summary-20260804-083407`；
- `eval/results/cangjie_repomap_fixture-20260804-083407`。

状态：`EVAL-B84-OPSTRUCT1=closed`；`EVAL-B59-INVROW1=confirmed-repeat/root-audit-next`。显式时间窗 Trace 因果投影、
自动补齐、模型正文与结论所有权均未改变。

## 88. 2026-08-04 B85-C：复合清单行身份与机械定位 authority 根修

对 B85 日志和代码冷读后，原先归为一个“label identity”问题的两条披露实际来自两个独立 typed 消费面：

1. `PrincipalSupportMemberObligations` 把 `aggregate_fact:member_set` 与普通 definition evidence 共用
   `source + label` 去重。`Cart@14` 与 `Cart@30` 因此先合成一个带两个 equivalent anchors 的席位，五个真实行变成四个
   obligations；
2. `MissingPrincipalSupportMembers` 只索引 `items[] -> citation_ref`，不承认模型在 `block.Text` 中写出的 markdown 表格行。
   B85 的五行和五个 citation 都精确存在，但没有隐藏 sidecar，因此四个合并后的席位仍全部被误报缺失；
3. finalize 前的 read-localizer 是另一条链：完整 `source_inventory` 已给出
   `CanEnterMechanicalLanding=true / NeedsFollowup=false`，通用导航策略仍要求 `relation_map`，从而铸造虚假的
   `followup_coverage` termination caveat。

施工采用 identity/authority 单源，没有重接已经退役的答案 mutator：

- aggregate principal row 的去重键改为 `source + exact line + label`；普通 definition fact 仍保留同文件同符号的
  declaration/body equivalent-anchor 合并，避免破坏多语言既有证据等价；
- 只读覆盖索引新增 markdown principal carrier：必须在同一模型主块中同时出现成员与精确 typed `file:line`，且
  citation pool 中已有同位置 citation，才认定该席覆盖。label-only、citation-only、`line 14` 对 `line 140` 的前缀匹配
  均不能获权；
- 没有调用 `normalizePrincipalSupportMemberCarriers`，不新增 hidden item、不改 citation_ref、不改任何可见模型文字；
  `TestShippingAnswerPathsDoNotCallVisibleModelContentMutators` 保持绿色；
- read-localizer 仅在既有 source-inventory answer authority 明确
  `CanEnterMechanicalLanding && !NeedsFollowup` 时忽略通用 relation/navigation debt。请求 summary/source prose 时该 authority
  为 false，原定位合同继续生效；不完整、分页未完、预算截断清单也不受豁免。

看护覆盖同文件同名 class+extension 两席、完整 markdown 表格正臂、删除一席位置的负臂、line-prefix 负臂、机械清单
relation debt 抑制与 summary-bearing inventory 不抑制。完整
`go test ./internal/types ./internal/tool ./internal/orchestrator -count=1` 全绿（tool 164.284s）。

状态：`EVAL-B59-INVROW1=implemented/tests-pass/replay-next`；B85 两条虚假系统披露均已有根修，下一批严格并行
`cangjie_repomap_fixture` 与一个异构 source/read case 做 production 验证。显式时间窗 Trace 因果投影、自动补齐、
read/write/operation/data 路由与模型结论所有权均未改变。

## 89. 2026-08-04 B86 r12：Cangjie 闭环；typed construct family 被 coarse role 扩域

### 89.1 严格并行与人工审计

在 `main@b3aab5673` 构建后严格并行恰好两个 source/read case：

- `cangjie_repomap_fixture`：runner/human PASS，64s；五个复合 declaration rows、package、精确 file:line 与五条
  citation 全部一致，`principal_support_member_coverage=0`，两条旧虚假 caveat 均消失；
- `arkts_repomap`：runner PASS / human FAIL，142s；发生一次 finalizer reject，最终答案被扩成额外 20 个
  function/method rows，并把没有 decorator 的 `EntryAbility` 错列为 `@Entry`。

因此 `EVAL-B59-INVROW1=production-proven/closed`。ArkTS 的 runner PASS 同时是 eval 假绿：case 明示
`entry_page count=4`，但内部 rowset ID 没有对应可见标题时，旧 oracle 只确认四个期望行都出现，不检查同组多出的第五行。

证据：

- `eval/parallel_selected_summary_evalcampaign_b86_inventory_r12_20260804.md`；
- `eval/parallel_selected_summary_evalcampaign_b86_inventory_r12_20260804_manual_audit.md`；
- `eval/results/cangjie_repomap_fixture-20260804-090056`；
- `eval/results/arkts_repomap-20260804-090056`。

### 89.2 `EVAL-B86-SURFFAM1`（P1）：独立 marker family 的 typed 交集丢失

日志证明解析器并未丢证据：四个页面 row 带 `@Entry/@Component`，两个片段 row 带 `@Builder`，无 decorator 的
`EntryAbility` 没有 surface note。旧 `SourceInventorySurfaceFamilyKey` 只在 `base + base symbol` 二元 term 形上铸造
family；单一 `@Builder` 和并列 `@Entry/@Component` 都返回空。随后 analyzer 的 coarse carrier
`function/method` 被当成完整 requested universe，系统合成 20 行 principal set 并硬拒模型首稿。

根修是语言无关的 typed construct-family 代数：

1. 一个 parser row 可携带多个独立 family；base+specific 仍折叠成 base，独立 marker 保留各自带 sigil 的身份；
2. graph candidate filter、observation row filter、complete lens、closure、surface gap 与 requested-universe view 共用该
   multi-family 载体；任一 requested family 命中即可进入主集合；
3. exact family 一旦可从 analyzer-validated `source_quotes` 与 parser `SurfaceTerms` 相交得到，就覆盖 noisy query token；
   coarse role 只作 carrier，不能把同角色无 marker 的 helper/lifecycle 扩成主答案；
4. 无精确 family 的普通函数/类型 inventory 保持原 complete-role 语义；Cangjie `public class/extend/foreign func` 的
   base+specific family 行为不变。

新增 marker 多 family、同角色无关成员负臂、跨 role family、projection 不扩域与真实 repo_map narrow-scope 回归；
`internal/types`、全部 SourceInventory tool tests 与 `internal/tool/repomap/...` 全绿，LOC convergence ratchet 未放宽旧文件。

状态：`EVAL-B86-SURFFAM1=implemented/tests-pass/replay-next`；
`EVAL-B86-EVALROW1=confirmed/eval-fix-next`。下一批先修 eval-only 的显式 section-anchor 合同，再严格并行原 ArkTS
witness 与一个非 decorator source-inventory case。产品修复不读取原始用户输入或模型答案，不修改模型可见正文，且不触及
Trace 因果投影、自动补齐、write/operation/data 路由。

## 90. 2026-08-04 B86-E：清单精确计数绑定显式 section authority

`EVAL-B86-EVALROW1` 已按 eval-only 形根修。旧 runner 在内部 rowset ID 无法猜中可见标题时静默退回全文，并把
`expected_count` 解释成“期望行命中数”；因此四个正确 `@Entry` 行与一个错误 `EntryAbility` 同时出现仍会假绿。

新合同由 case 显式声明 `EXPECT_INVENTORY_SECTION_LABEL_<ROWSET>`：

1. exact count 只统计该可见 markdown section 内的表格行；不再从内部 ID 或答案语义猜标题；
2. 声明的 section 不存在时发射 `missing_inventory_section`，禁止静默回退全文；
3. section scope 按 markdown heading level 终止，H3 下的 bold 子标题仍属于 H3，避免嵌套展示把计数截成零；
4. 未声明显式 label 的既有 case 保持原兼容路径，产品 runtime、prompt、成文校验与答案正文均不改变。

ArkTS case 已分别绑定 `@Entry` 页面入口和 `@Builder` 复用片段两个可见 section。旧 B86 输出已被新 runner
确定性判为 `inventory_count_mismatch:entry_page:got5:want4`；section 缺失、嵌套标题 extra-row 与真实 `run.sh`
接线均有 contract pin，`runner_lib_test.sh` 与 shell syntax check 全绿。

状态：该第一形已由下一节收窄；“固定 section 标题是唯一合法载体”不再作为最终合同。该批没有触碰 Trace 显式时间窗、
因果投影、自动补齐、read/write/operation/data 路由或模型结论所有权。

## 91. 2026-08-04 B87 r13：产品根修回放通过；eval 标题格式门收窄

在 `main@e4b65b14e` 构建后严格并行 ArkTS 与 Cangjie 两个 source-inventory case：

- ArkTS 主答案人工 PASS：精确列出 4 个 `@Entry` 与 2 个 `@Builder`，每席 path:line 正确；无 `EntryAbility`、无旧 20 行
  coarse-role 扩域、零 finalizer reject/advisory；
- Cangjie runner/human PASS：1 个 extend、1 个 foreign func、3 个 public class，两个 `Cart` 仍按 family+line 区分，
  package 与五条 citation 完整，零旧虚假 caveat。

这证明 `EVAL-B86-SURFFAM1=production-proven/closed`，同时暴露 §90 第一形的新 false negative：ArkTS 正确答案采用两段
有序列表而没有固定类别标题，section-only oracle 因而报两个 `missing_inventory_section`。内容正确性不应要求模型复刻
某个标题措辞。

最终 eval 合同保持 case-owned typed authority，但允许两种等价可见载体：

1. 存在显式 section 时继续按 heading level 精确统计其中行；
2. section 缺席而 case 声明 `EXPECT_INVENTORY_ROW_MARKER_<ROWSET>` 时，仅在 terminal primary answer 中选择同时满足
   “markdown 清单行 + case marker + 精确 source path:line”的行；引用区、系统补充、普通叙述均不能获权；
3. 两种载体都缺席才发射 `missing_inventory_group`；marker 载体同样执行 exact count，额外行不能漏过；
4. 旧 B86 错答仍稳定得到 `got5:want4`，本轮正确无标题答案离线重放为 PASS。

新增 section/marker 正反臂、编号列表计数、引用重复不获权、missing-group 与真实 `run.sh` primary-surface 接线 pin。
状态：`EVAL-B86-EVALROW1=implemented/runner-contracts-pass/artifact-replay-pass`；记录的本轮 runner FAIL 是旧 snapshot
oracle 的假阴性，人工答案与新 oracle 均 PASS。下一批转入新的异构高优先级 eval，不再要求固定答案标题。

## 92. 2026-08-04 B88 r14：typed identity 绑定双例审计与首批根修

### 92.1 严格并行与人工判定

在 `main@51942da53` 上严格并行两个 read case：

- `qf_sequence_analyzer_gate`：runner PASS / human FAIL，518s；Explorer 36 轮/4 dispatch、14 reads、8 次 completion、
  2 次 completion reject，Finalizer 4 次 reject；
- `qf_multi_member_set_count_caveat`：runner PASS / human FAIL，285s；3/5/30 的集合与成员本身正确，但存在一条错误职责说明，
  且旧 oracle 假绿。

调用链图最终只画已证方向边，但正文和结构化 `gate.Run` item 仍把 `gate.RunWith` 说成 `gate.Run` 的包装/语义等价入口。
真实源码是 `gate.Run -> RunWith`，而 `buildAnalysisIR -> RunWith`；不存在请求方向的 `buildAnalysisIR -> gate.Run`。
数量例正确列出 3 个类型、5 个公开函数、30 个 Kind 常量，但把 `type Kind string` 描述成 int alias；该额外说明没有
typed underlying-type 支撑，先按模型错误记录，不新增 prose hard gate。

### 92.2 `EVAL-B88-PFXCIT1`（P0/red-line）：prefix sibling 借走精确 citation authority

结构化 item 为 `label=gate.Run, claim_form=call_edge, citation=analyzer.go:2666`，被 pre-emit 接受；该行唯一事实是
`buildAnalysisIR -> gate.RunWith`。根因是两个自称 code-surface exact/verbatim 的公共 matcher 使用 `strings.Contains`，
使 `gate.Run`/`Run` 从 `gate.RunWith` 获权。Mermaid typed edge gate 正确拒绝了同一伪边，但 item/citation gate 反向放行，
最终形成“图正确、正文错误、引用看似有效”的同页矛盾。

根修只消费 structured item label、citation 和 accepted EvidenceItem：

1. snippet 中的 code identity 改用共享 `CodeSurfaceAppearsAsToken` 完整边界；
2. endpoint identity 只接受大小写无关 exact 或 qualified/unqualified exact tail，不再接受任意互为子串；
3. `gate.Run -> gate.RunWith`、`Build -> BuildWith`、`foo -> foobar` 等 prefix sibling 均 fail-closed；
4. `RunWith` 与 `gate.RunWith` 的合法短名/限定名同源仍通过，所有语言共用，不含 Go/本题特判。

定向正负 pin 与完整 `go test ./internal/tool -count=1` 全绿（158.384s）。状态：
`EVAL-B88-PFXCIT1=implemented/full-tool-pass/replay-next`。

### 92.3 `EVAL-B88-COUNTORACLE1`（P1/eval）：源码行号可冒充集合数量

旧 Kind count regex 允许 24–26，并在本轮被类型行 `grammar.go:26` 满足；因此 runner PASS 没有证明“Kind 常量=30”与
Kind bucket 绑定。case 已改为 checkout-derived 三个独立 scalar：awk 从生产 `grammar.go/eval.go` 计算 exported type、
exported function、Kind const-block 成员数，分别绑定 terminal primary answer 的“类别 + 共 N 个”近邻。源码行号、列表序号、
引用区或系统 supplement 均不能满足；每轮生成 data-scope receipt。当前答案离线重放为 3/5/30 PASS。

状态：`EVAL-B88-COUNTORACLE1=implemented/eval-only/artifact-replay-pass`。

### 92.4 剩余任务排序

1. `EVAL-B88-SUPPCAVEAT1`（P1）：typed principal row sets 已 complete，模型以 `section.text` 枚举完整 38 行，
   deterministic review 只看 `items[]` 得到 `principal_items=0`，随后系统追加三个函数 owner 表与泛化“证据较弱”caveat。
   下一批应按 answer ownership/display relevance 处理：缺结构化 carrier 保留日志/状态审计，不能自动投影成“事实证据弱”；
   不得扫描自由 prose 来反向确权，也不得系统重写模型表。
2. `EVAL-B60-CLOSURECHURN1`（P1）：本轮再次确认高耗时，但没有发现“positional support 必带又必拒”的合同矛盾；第一次
   support-ref retry 是 members=11、refs=12 的真实不一致，第二次 no-directed-path retry正确。剩余成本集中在 Analyzer 宽实体
   生成无关 DAG window、accepted completion 后仍多 dispatch，以及 Finalizer participant identity 反复修改。先做 typed task/window
   provenance 冷读，不为本 case 的函数名或固定图形写优化。

本批没有触碰 Trace query、显式时间窗、因果投影、自动补采、根因双轴或模型正文所有权。

### 92.5 `EVAL-B88-SUPPCAVEAT1` 根因更正与闭环：同一 typed 清单的前后校验合同冲突

继续冷读后确认，问题不能只靠“压掉泛化 caveat”处理。Finalizer prompt 已明确要求把 `Principal Enumeration Rows`
渲染为真实 `ordered_list` / `bullet_list` / `table` 的 `items[]`，deterministic exhaustive reviewer 也只消费 typed
item identity；但 pre-emit member-set hard gate 同时允许 `blocks[].text` 通过。于是 B88 的 38 行自由 section 文本先被硬门接受，
后置 reviewer 又得到 `principal_items=0/missing=30`，最终把“结构化载体缺失”错误投影为“枚举证据较弱”，并触发部分 owner
anchor 补表。这是同一声明在相邻阶段使用不同权威载体的合同矛盾，不是证据本身不完整。

根修已统一为：

1. typed source-inventory 主清单的 hard gate 只读取 `RequestModel`、accepted aggregate member rows，以及模型发出的
   `items[].label/cells`；自由 `blocks[].text`、item 说明文本、diagram/prose 不再给成员身份确权；
2. 类别标题存在时，成员只在同类别结构化 block 中获权，避免同名 type/function/constant 跨桶借位；单集合或无标题表仍可
   使用其结构化 rows；
3. 普通 explanation/narrative 的 soft member-set 车道保持旧的 visible-text 兼容，不把本修复扩成全局格式硬门；
4. 系统不生成、删除或改写模型成员行。缺少结构化 rows 时，pre-emit 直接把完整 typed obligation roster 交回模型自修；修好后
   后置 deterministic reviewer、citation gate 与 read-audit supplement 读取同一 carrier，不再事后发布错误弱证据 caveat。

新增“自由正文拒绝 / 结构化 label+cells 接受 / narrative soft lane 不回归”正负 pin；两处旧测试中把 summary/Markdown
table text 当结构化行的矛盾断言已改为同等可见内容的真实 typed rows。定向测试与完整
`go test ./internal/tool -count=1`（158.214s）全绿。状态：
`EVAL-B88-SUPPCAVEAT1=implemented/full-tool-pass/replay-next`。

该批没有扫描用户原始输入或模型自由正文作硬门；没有系统代写答案；没有触碰 Trace 显式时间窗、因果投影、自动补齐或根因双轴。
下一批处理 B88 暴露的 source-inventory 目标 scope 被扩成无关仓库 scopes、导致 16 轮 completion churn 的 typed scope
provenance 问题。

### 92.6 `EVAL-B88-SCOPEPROV1`：请求目录边界在 analyzer prescan 后丢失（已施工）

B88 数量例的用户范围是 `internal/analysis/criterion`，analyzer 也实际执行了带
`scopes=[internal/analysis/criterion]` 的 source_inventory prescan；但 `SourceInventoryProfile` 只保存角色/字段，
`SourceScopeProfile` 只表达 production/test/docs/auxiliary 类别，两者都不能承载目录边界。模型给出的两个文件提示又因“请求
写的是目录而非逐个文件”被正确拒绝。最终 durable observation 把后续探索的 `.`、`internal/tool`、fixture 等 scope 合并进来，
completion authority 误判为 repo-wide，并按全仓 source-class samples 生成无关补查，导致 16 轮 convergence churn 和
`principal_scope=all` 的错误披露。

根修没有复用类别 scope，也没有回扫用户/答案 prose，而是增加独立的
`AnalyzerHints.SourceInventoryRequestedPathScopes` typed carrier。它只在以下精确合取成立时由系统铸造：

1. 当前请求是 active source inventory；
2. 成功的 repo_map observation 同时带 `repo_lens:tool_query` 与 `repo_lens:stage:analyze`；
3. observation 的 canonical 非根 scope 与 `MentionedEntities` 中已由当前请求 verbatim 验证的路径实体精确相同。

探索阶段 cursor、根 scope `.`、不匹配路径、绝对/越界/虚拟路径均不能获权。该边界现由同一 helper 贯通 repo-wide 判定、
completion scopes、follow-up debt、aggregate requested-universe 与 principal row projection；边界外的后续探索行进入 audit lane，
不再污染主清单。缺页/截断仍会补查，但只能在请求目录内，不能再按全仓 source-class sample 扩域。
请求目录边界还显式高于 prescan 投影出的文件样本：目录是用户授权范围，样本文件只是导航结果，后者不能把前者偷偷收窄。

新增 analyzer 发射持久化、wrong-stage/unmatched/root 负臂、completion/follow-up scope、sibling-row audit 正负 pin；LOC ratchet
要求把边界/行投影/follow-up 构造拆到独立 concern 文件，旧 convergence cluster ceiling 未提高。状态：
`EVAL-B88-SCOPEPROV1=implemented/types+tool-full-pass/replay-next`（`internal/types` 18.730s，`internal/tool` 166.728s）。

该批不改变模型结论或正文，不扫描模型输出确权，也不触碰 Trace 显式时间窗、因果投影、自动补齐与根因双轴。

## 93. 2026-08-04 B89 r15：目录范围生产载体补洞；调用边限定身份合同冲突

### 93.1 严格并行与人工审计

在 `main@570a5ec4ef` 构建后严格并行恰好两个 read case：

- `qf_multi_member_set_count_caveat`：runner PASS / human 主答案 PASS，667s；最终 3 个类型、5 个公开函数、30 个 Kind
  常量及逐行引用全部正确，但 Explorer 34 轮/2 dispatch、16 次 source_inventory、11 次 completion；
- `qf_sequence_analyzer_gate`：runner/human FAIL，256s；Finalizer 连续 6 次同类 reject 后降级出厂，正文仍把
  `gate.Run -> RunWith` 的 wrapper 方向反写。

工件见 `eval/parallel_selected_summary_evalcampaign_b89_scope_replay_r15_20260804.md`、对应 manual audit 与两个
`eval/results/*-20260804-175958` 目录。

### 93.2 `EVAL-B88-SCOPEPROV1-R1`（P1）：真实 analyzer 用 verified scope quote 保存目录

§92.6 的载体在构造单测中成立，但生产 analyzer 的 entity projector 把完整目录投影成 basename/文件实体；真实目录仍完整
保存在已经通过当前请求 verbatim 校验的 `SourceScopeProfile.SourceQuotes`。旧 producer 只与
`MentionedEntities` 相交，因而没有铸成 `SourceInventoryRequestedPathScopes`。后续 completion 再次把请求扩成 `cmd`、fixture、
`internal/skill` 等五个无关 scope；模型已经给出正确 bounded roster，系统仍反复要求全仓分页。

补洞仍不读取 `RawRequest`：producer 的 request-side 候选改为“typed MentionedEntities **或** 已通过 emit-analysis
verbatim 校验的 SourceScopeProfile.SourceQuotes”，再与 analyzer-stage successful repo-lens scope 精确相交。rationale、
explore-stage scope、不匹配 quote、根目录与越界路径仍不能获权。新增 verified-quote 正臂与 unmatched 负臂，定向测试通过。

状态：`EVAL-B88-SCOPEPROV1-R1=implemented/full-tool-pass/replay-next`（`internal/tool` 167.514s）。下一次同 case 回放必须把
repair scopes 固定在 `internal/analysis/criterion`，并显著降低 16 次 source lens / 667s；答案正确并不能掩盖本轮过程失败。

### 93.3 `EVAL-B89-CALLEDGEQUAL1`（P0/red-line）：同一 typed call 被限定形拒绝、裸形强制

调用证据明确为 `Run -> RunWith @ internal/analysis/gate/gate.go:135`。模型用 participant label
`gate.Run` / `gate.RunWith` 表达同一关系时，body evidence lane 报 `call_edge_unproven`；同轮 principal completeness lane
又报 `principal_call_edge_missing edge=Run -> RunWith`。因此一条边在限定展示形被拒，同时以裸形被强制，模型无法通过
等价重画解决，六次重试后降级。

该问题与先前 class participant alias 类同但不是单语言特例：现有 resolver 只有“exact qualified”或“短调用 + 唯一 definition
owner”两臂；生产证据携带精确 `OwnerSymbol=Run` 与唯一 call-site，却没有独立 definition row。最优方案冻结为共享 typed endpoint
identity join：优先 exact；其次使用 citable call 的 `Subject/Object/AnchorSymbol/OwnerSymbol` 与唯一 source location，再由已验证的
required endpoint / owner binding 完成限定展示；存在多 owner、重载、多位置或 contrary qualified object 时 fail-closed。sequence body、
edge anchor 与 principal completeness 必须调用同一个 resolver，不能各保留一份别名策略。测试矩阵覆盖 Go、Java/Kotlin、
TS/ArkTS、C/C++、Rust、Cangjie 等全部可执行语言的 `.`/`::`/`#`/裸符号形，不按语言关键词分支。

施工确认不需要放宽到 path/短名猜测。`emit_evidence` grounding 本来就会从 repomap 的 enclosing callable 铸造模型不可写的
`OwnerSymbol`，但 package-level callable 只保留短名。本批仅对 call-site owner stamp 保留 parser-owned package/module/receiver
限定身份；普通 condition/return/assignment 的既有短 owner 语义不变。图边 resolver 现有 definition 绑定臂继续保留，新增的
等价臂必须同时满足：exact required caller、同 owner 的限定两端、call record 上 exact qualified OwnerSymbol、唯一 source:line、
callee 为同 owner 的裸 operation；wrong owner、缺 owner、contrary object、多 call-site 继续 fail-closed。parsed body、anchor 与
principal completeness 原本已汇入同一 resolver，因而三面同时闭合。

看护覆盖生产 `gate.Run -> gate.RunWith` 形、无 owner/错误 owner/多位置负臂、`.`/`::`/`#` 三种限定身份，并对 Go、Python、
JavaScript、TypeScript、Java、Kotlin、Rust、C、C++、Ruby、Swift、Lua、ArkTS、Cangjie 的 package/module owner stamp 逐语言
固定预期。定向 diagram/evidence 套件与完整 `go test ./internal/tool -count=1`（167.735s）全绿。

状态：`EVAL-B89-CALLEDGEQUAL1=implemented/full-tool-pass/replay-next`。该修复只消费 structured document 与 typed evidence，
不解析用户/模型 prose，不放松无证 call edge；QFRootCauseTrace 已显式排除，因此 Trace 时间窗因果投影及自动补齐不受影响。

## 94. 2026-08-04 B90 r16：限定调用边生产正证；目录边界二次失效与无路径上下文污染

### 94.1 严格并行与人工审计

在 `main@e801d130f512` 构建后严格并行恰好两个 read case：

- `qf_sequence_analyzer_gate`：runner PASS / human FAIL，497s，27 次 read、39 个 Explorer 轮、2 次 Finalizer reject；
- `qf_multi_member_set_count_caveat`：runner/human FAIL，329s，10 次 source_inventory lens；答案给出 type=3、Kind
  constant=30，但 production function 只列 4 个，漏掉 checkout 中第 5 个导出函数 `SetExternalArtifactFloor`。

工件见 `eval/parallel_selected_summary_evalcampaign_b90_contract_replay_r16_20260804.md` 及对应 manual audit。

### 94.2 `EVAL-B89-CALLEDGEQUAL1` 生产结论：身份合同已闭，事实方向仍需模型自修

第一稿把 wrapper 方向画成 `gate.RunWith -> gate.Run`，被 typed edge gate 正确拒绝；第二稿删除该边，又被 principal
completeness 正确要求保留真实的 `gate.Run -> gate.RunWith`；第三稿使用限定 participant 后通过。这证明新 owner-context resolver
已经在真实 Finalizer 三面同时生效，旧“正确限定边永远过不了”的合同冲突不再存在。两次拒绝是对两个不同事实错误的有效看护，
不能为了减少重试而放宽。

但最终 prose 仍声称 `buildAnalysisIR` 通过 `RunWith`“间接到达”`gate.Run`，而最终 Mermaid 明确是两条并列边：
`buildAnalysisIR -> gate.RunWith` 与 `gate.Run -> gate.RunWith`，不存在 source→requested sink 的有向路径。系统其实已经追加
`principal_span_waiver=no_directed_path`，却同时保留模型早先自报的“完整调用链” principal member_set 和带错误方向的 evidence
summary/answer-chain，给 Finalizer 提供了互相冲突的上下文。新 P0 `EVAL-B90-CALLBOUNDCTX1` 冻结为：当 typed directed graph
判定 no-path 时，不允许“完整路径”类模型 aggregate 获 principal authority；Finalizer 的关系事实以 typed
subject/predicate/object + boundary 为准，模型 summary 只能是 advisory。不能扫描或改写最终 prose，结论仍由模型生成。

已施工为审计态与答案权威态分离：`MutableState` 继续原样保留模型 accepted reason/member_set，供日志、resume 与事后审计；
`BuildAnswerSurfacePlan` 在 typed `no_directed_path` disposition 下清除答案视图中的无方向 member_set 与 closure reason，旧窗口
preserved investigation notes 也不再注入 Finalizer。`emit_investigation_complete` 的成功 tool summary 同样只回显 exact endpoint
boundary，不回放与其冲突的自由 reason。grounded call-edge `Subject/Predicate/Object`、source:line 与 endpoint boundary 继续完整提供，
最终总结、图和优化说明仍由模型编写。新增 pin 证明 stale path reason/roster 不出现在 prompt、但 raw audit state 字节保留；普通
call chain、其他问题家族及 Trace RootCauseTrace 均不命中该 projection。

`internal/types`（19.936s）、`internal/agent`（2.753s）、完整 `internal/tool`（169.559s）全绿。状态：
`EVAL-B90-CALLBOUNDCTX1=implemented/full-pass/replay-next`。

### 94.3 `EVAL-B88-SCOPEPROV1-R2`：模型字段波动不能承担目录身份

§93.2 依赖真实 analyzer 把目录写入 `SourceScopeProfile.SourceQuotes`，但 B90 的同一请求只写入“列出公开符号”，目录既未进入
该字段也未进入 `MentionedEntities`。因此 producer 再次没有铸成 requested-path carrier，completion repair 四次扩到 `cmd`、
fixture、`internal/skill` 等五个无关目录；最终模型从混合 production/test census 手工推断 roster，漏掉当前新增的第 5 个公开函数。

最优补洞不能继续赌某个可选 analyzer 文本字段。精确信号应是三者合取：active source inventory、带
`repo_lens:stage:analyze + repo_lens:tool_query` 的成功 observation scope、该 canonical path 在当前请求中具有 lexical exact
path identity。这里复用共享 token-boundary matcher，只比较 typed scope 的完整路径字面量，不做关键词/语义/最终答案扫描；
explore scope、根目录、越界路径、请求中不存在的 analyzer 猜测仍 fail-closed。随后 scoped requested-universe rowset 必须直接给出
complete production roster，避免模型再从全仓数量倒推成员。状态：`R2=confirmed/next`，`INVENTORYFRESH1=confirmed/dependent`。

R2 已施工：producer 仍以 analyzer-stage `repo_lens:tool_query` 为前提，在 entity/verified quote 之外，逐个将 observation 的
canonical 非根 scope 交给共享 `RawRequestExplicitlyMentionsEntity` lexical boundary matcher；只有完整路径 token 精确在场才获权。
集成看护特意令 analyzer entities/quotes 均不含目录，仍能从真实请求与 prescan scope 铸成边界；`criterion_extra`、路径前缀粘连、
伪 `.go` 后缀、错误阶段与错误 quote 全部拒绝。没有 source-inventory 角色/关键词触发器，也不读取 finalizer 文本。
`internal/types`（22.297s）和完整 `internal/tool`（165.784s）全绿；LOC ratchet 未提高。状态：
`EVAL-B88-SCOPEPROV1-R2=implemented/full-tool-pass/replay-next`。`INVENTORYFRESH1` 仍须生产回放验证 scoped complete roster。

本批没有触碰 Trace query、显式时间窗、因果投影、自动补齐或根因双轴；两个新根修都约束 typed context authority，系统不得
替写模型结论。

## 95. 2026-08-04 B91 r17：repo-root 范围坐标丢失、来源类别污染与调用证据处理次序

### 95.1 严格并行与人工审计

在 `main@0a7e7838e5ba` 构建后严格并行恰好两个 read case：

- `qf_sequence_analyzer_gate`：runner/human FAIL，563s，19 次 read、33 个 Explorer 轮、6 次 Finalizer reject；
- `qf_multi_member_set_count_caveat`：TIMEOUT/human FAIL，1201s，16 次 source_inventory lens、26 个 Explorer 轮、2 次
  Finalizer reject。

工件见 `eval/parallel_selected_summary_evalcampaign_b91_contract_replay_r17_20260804.md` 及对应 manual audit。本批未运行 Trace，
不会据此改变 Trace 能力裁定。

### 95.2 `EVAL-B91-SCOPECOORD1`（P0）：执行坐标覆盖了请求范围身份

R2 的 exact request-path matcher 本身工作，但 production `repo_map` 在 `path=internal/analysis/criterion` 的 selected-sub-repo
车道上，会把同路径 `scope` 归一为 `.`。`SourceInventoryObservation` 只保存归一化后的 operational `Scopes=[.]`，producer
因此再无 repo-root path 可与当前请求精确合取。结果 completion 把全仓 `candidate_budget_truncated` 当作目标包债务，16 次 lens
扩散到 `cmd`、fixture、`internal/skill`、thirdparty corpus 等无关目录。

最优根修是分离两种坐标：保留现有 operational scopes 供 lens 执行，另由 repo_map engine 铸造 repo-root
`QueryPathScopes`，在 selected-sub-repo 归一化前后按可信选中根重基。它是工具查询 provenance，不是 request authority；producer
仍须同时满足 analyzer-stage 成功 observation、exact current-request canonical path token、非 root/wrong-stage 拒绝。禁止从
模型 entity/quote、用户关键词或最终答案文本猜范围。

B91-A 已施工：`SourceInventoryObservation` 新增独立 `QueryPathScopes`，repo_map 在最终 narrowing 后把 operational scope 按
可信 session repo root 重基；clone/merge/empty-lens 保留该载体。prescan producer 优先消费此坐标，并继续要求 analyzer stage、
成功 tool query provenance 与当前请求中的完整 canonical path token；旧 observation 仅作兼容 fallback。selected-root `.`、嵌套
scope、根仓 scope、越界 root 和请求碰撞均有 pin。LOC ceiling 未抬高。

### 95.3 `EVAL-B91-SOURCECLASS1`（P0）：production/test 语义未进入 typed rowset

本轮目标包 inventory 返回 type=3、function=56、constant=30；其中 function=56 实际由 5 个 production 导出函数与 51 个
`_test.go` 测试入口组成。Finalizer 首稿正确选择 5 个 production 函数，预发射合同却把 56 行全部当 principal roster，强制追加
测试函数；一次 89 行 patch 的校验从 19:08:45 到 19:14:39，最终整例超时。这不是模型少列，而是系统没有给模型/合同提供
“来源类别”这一精确信息。

根修须在 parser/index 侧为 inventory row 携带 production/test/generated/fixture 等 typed source class，并让 analyzer/contract
显式选择 source-class universe。Answer gate 只执行 typed 选择，不读取“公开”“生产”等用户原文关键词，更不能扫描模型答案删行。
未选择类别时要保持现有全集语义；已选择 production 时测试入口既不进入 principal member_set，也不产生全仓修复债务。

### 95.4 `EVAL-B91-CALLGROUNDORDER1`（P0，纠正原 `CALLEDGEWIRE1`）：调用方向规范化晚于 grounding

本轮前几稿的 `gate.RunWith -> gate.Run`、九条 `gate.RunWith -> gate.RunWith` 伪自环和未配对 reverse arrow 均应拒绝，不能为
减少重试而放宽。复核原始 `emit_evidence` 结果与持久 run JSON 后，原“正确 grounded 边被 resolver 误拒”判断不成立：
`ev-30b63a1cdbd5c9ca @ gate.go:135` 的真实 `grounding_status=recovered`，所以 diagram gate 拒绝它是正确的。

真实机制是模型发射 `anchor_kind=call, anchor_symbol=Run, object=RunWith`；`emit_evidence` 先用 caller 锚 `Run` 对
`return RunWith(...)` 做 grounding，落入 `nearest_call/recovered`，随后才用 parser graph 规范为
`subject=gate.Run, object=RunWith, owner=gate.Run`，却没有重新 grounding。Finalizer 的 `accepted_evidence_handoff` 又只打印
owner/anchor/role、不打印 grounding 状态，使 recovered 行看起来像严格权威行，模型据此连续对抗正确硬门。

根修不是放宽 resolver，而是把 parser-backed caller/callee/anchor 规范化放到首次 GroundItem 之前；规范后的 callee 锚必须重新走
同一个 line-text/symbol-table gate。Finalizer handoff 同时必须显式区分 grounded/recovered，recovered 继续保留在审计/修复上下文，
不得伪装成可直接支撑严格 call edge 的 accepted 权威。

### 95.4.1 `EVAL-B91-RANGECLOSURE1`（P1）：DAG 节点覆盖早期 read ranges

持久快照的 `read_ranges` 最终只含 `internal/agent/analyzer.go`，虽然首个节点真实读取了
`internal/analysis/gate/gate.go:134-233`。原因是 Explorer 每个 DAG 窗口都用当前窗口 tool results 构造
`StageCoverageSnapshot` 并设置 `ReplaceReadRanges=true`。Turn-A ToolResults 本轮仍保留旧 gutter，因此这不是上一问题的直接原因；
但 EvidenceClosure、恢复快照和后续覆盖判定会看到不完整的 run 级读取事实。修复应在同一 read run 内单调合并 read set/ranges/totals，
只在新 run 的既有 reset 点清空，不把一个窗口的空/窄快照解释为撤销此前真实读取。

### 95.5 `EVAL-B91-WAIVERRATIONALE1`（P1）：typed boundary 后仍有自由解释侧漏

§94.2 已使 no-path success summary 以 exact endpoint boundary 开头，但 `appendPrincipalSpanWaiverCompletionNote` 仍把模型自由
`waiver.Rationale` 追加在括号内。本轮该 rationale 错称 `gate.RunWith` 位于 `buildAnalysisIR` 外，虽未重新获得 typed path
authority，仍污染工具历史。最小正确形是 rationale 保留在 Mutable 审计态，发布摘要只含 disposition/source/sink；模型继续基于
typed triples 自己得出结论。

B91-A 同批已删除发布 gate note 中的自由 `waiver.Rationale`；`PrincipalSpanWaiver` 审计对象仍保留原 rationale 字节。看护以唯一
错误 token 证明 tool summary 不泄漏、Mutable audit state 仍可复核。

任务顺序冻结：B91-A `SCOPECOORD1 + WAIVERRATIONALE1`；B91-B `CALLGROUNDORDER1 + RANGECLOSURE1`；B91-C `SOURCECLASS1`（先载体与
默认兼容，再接 analyzer typed selection）；每批独立测试、提交、推送后再进行恰好两个用例回放。所有批次显式排除
QFRootCauseTrace、Trace 窗口投影、自动补齐和系统代写结论。

B91-A 验证：定向 pin、`internal/types`（20.590s）、`internal/tool/repomap`（2.438s）、完整 `internal/tool`（169.765s）全绿。
状态：`SCOPECOORD1 + WAIVERRATIONALE1=implemented/full-pass/replay-after-B/C`。

B91-B 已施工：`emit_evidence` 在首次 grounding 前，以 exact source:line parser relation 与 enclosing callable 统一
caller/callee/`AnchorSymbol`；caller 误填锚的生产形现在直接得到 `grounded/line_text`，没有放宽 diagram resolver。
Typed handoff 的展示字段改为 `evidence_refs`，同时输出汇总和逐行 `grounding=grounded|recovered|ungrounded|unspecified`，避免
recovered 修复线索伪装为同权 accepted 证据。Explorer 将 read coverage 作为同一 run 内的 delta 单调并入 EvidenceClosure，后续
DAG 窗口消费累计 read set；新 run 仍由原 reset 点清空。定向看护及 `internal/types`（21.679s）、`internal/agent`（4.224s）、
完整 `internal/tool`（172.255s）、`internal/orchestrator`（14.231s）全绿。状态：
`CALLGROUNDORDER1 + RANGECLOSURE1=implemented/full-pass/replay-after-C`。

B91-C 已施工。复核生产 prompt 后纠正 §95.3 的一处表述：系统在最终投影前已经能从路径把 5 个 production function
识别出来，真正的合同缺口是“覆盖”只做子集判断。模型 member_set 的 56 行包含全部 5 个 production 行时，旧逻辑即认为它
覆盖 typed universe，却没有拒绝额外 51 个 test 行，于是混合集合继续成为硬义务。

根修包含两层。第一，repo index 在 advisory → observation → principal rowset 的每一行携带 engine-derived
`source_class=production|test|fixture|generated|...`，模型上下文也逐行显示；旧持久 observation 字段缺席时只从该行 canonical
path 做兼容推导，不读取用户请求、模型正文或最终答案。第二，已受 typed scope/source-class/surface-family 约束的主集合采用同一
规范 row key 的**精确集合等值**，不再用“系统行是模型集合子集”确权；模型 superset 会降为 advisory，由系统 typed rowset
提供硬事实，Finalizer 仍自行组织结论。明确 repo-wide / auxiliary 请求的全集语义保持不变。

实现按 LOC convergence 约束把 advisory-to-observation 转换拆到独立 concern 文件，没有抬高既有大文件 ceiling。新增生产 lens
source-class 贯通、旧 observation fallback、5 production + 1 test superset 负臂，以及混合 scope/明确 auxiliary 兼容看护。
状态：`EVAL-B91-SOURCECLASS1=implemented/tests-pass/replay-next`。本批没有修改 Trace query、显式时间窗、因果投影、自动补齐或
答案 mutation；source class 是 typed repository fact，不是系统替模型作结论。

## 96. 2026-08-04 B92 r18：B91 来源隔离生效；隐式 selector 与复合 facet 仍破坏完整枚举

### 96.1 严格并行与人工结论

在 `main@b77e6fa9c768` 构建后严格并行恰好两个 read case，单例预算因 B91 的 1201s 边界提高到 1500s：

- `qf_multi_member_set_count_caveat`：runner/human FAIL，415s，5 次 source lens、1 次 Finalizer reject；
- `qf_sequence_analyzer_gate`：runner/human FAIL，681s，14 次 read、30 个 Explorer 轮、6 次 Finalizer reject。

工件见 `eval/parallel_selected_summary_evalcampaign_b92_contract_replay_r18_20260804.md` 及对应 manual audit。B91-C 的第一层生产
验收成立：最终 function hard roster 不再含 51 个 `_test.go` 入口，逐行 source class 与 superset demotion 已生效；但完整 roster
仍未建立，不能将该项关闭。

### 96.2 `EVAL-B92-INVENTORYSELECT1`（P0）：完整 category inventory 被隐式 token query 收窄

本轮 analyzer typed profile 同时请求 `type,function,constant`，但误置了 enum-only facets
`type_underlying=string + requires_const_set=true`。现有 `PrincipalTargetRoles` 只要看到这两个 facet 就无条件折叠成 type，未验证
role universe 是否仍含独立 function/constant 主类别。与此同时，模型未传 `query` 的五次 source lens 被系统自动补成整段
`source_quotes + analyzer_hints.entities`；这些载体包含类别说明与 prescan 样本，不是闭合 selector，最终在 163 个已索引符号上
返回零 member rows。模型只能手读两份文件并发布 3/2/30，漏掉 production `IsRegistered`、`RegisteredKinds`、
`SetExternalArtifactFloor`，真实口径为 3/5/30。

最优根修冻结为 typed selection authority 分层：显式 tool `query` 继续是 selector；parser surface-family 与 typed roles/scopes 继续是
精确 selector；`IsCategoryEnumeration=true` 时不得把 profile quote/entity 自动拼成 token query，因为它们是分类/导航提示而非成员
闭集。enum-only facets 只有在 raw role universe 仅含 type（以及作为 qualifier 的 constant）时才可折叠；一旦还含 function/method
等独立角色，就保留全部 principal roles，并走普通 parser inventory。该判定只读 enum/boolean/list 等 typed 字段，不扫描请求或
答案 prose。必须保留 @Entry/@Builder、Cangjie extend/foreign、Java/Kotlin/C/C++ 等 parser surface-family 精确过滤正臂。

### 96.3 `EVAL-B92-DIAGIDENT1`（P1）：同一 participant 的重复 alias 产生误导性全边拒绝

sequence 前几稿包含反向/虚构边，硬门拒绝正确。后期稿已只保留两条 source-proven edge，却为同一 `gate.RunWith` 声明
`RW`、`RW2` 两个 participant alias；validator 将连到这两个节点的真实边都报成 `missing_call_anchor`，没有披露真正的重复 identity，
模型继续改 endpoint/方向并耗尽 6 次。降级稿最终仍把两条汇入同一 callee 的边叙述成串行路径，人工 FAIL。

后续方案不是放宽 call gate，也不是扫描模型散文：在 parsed Mermaid participant 表上先检查“同 canonical label 多 alias”，以
typed `duplicate_participant_identity` 一次性要求复用一个 alias；只有 identity 唯一后再跑现有 edge authority。若不同 aliases 仅
展示相同 class/actor carrier 但 message operations 不同，继续走既有 operation resolver，不可盲合并。本轮另有一次 Finalizer
首字节约 3 分钟等待，归类 provider 波动，不立产品 gap。

代码与生产 payload 冷读补正：重复 identity 不是所有 `missing_call_anchor` 的唯一原因。schema 已明确要求 edge anchor 使用 diagram body
中的 verbatim node ID，而生产稿多次用 canonical symbol（如 `buildAnalysisIR -> gate.RunWith`）去锚 body alias（`IR -> RW`），该拒绝
本身正确。真实缺口是系统没有先指出同一个精确 typed endpoint `gate.RunWith` 同时声明为 `RW`/`RW2`，导致模型在 canonical symbol、
alias 和虚构 `RunWith2` 之间反复切换。修复不得把 canonical symbol 静默改写成 alias，也不得放宽现有 verbatim-ID 合同。

施工顺序：B92-A `INVENTORYSELECT1`（P0，先修并回放 inventory）；B92-B `DIAGIDENT1`（P1，精确错误与全语言/全图族结构 pin）；
之后继续两个异构 case。两批均不触碰 Trace RootCauseTrace、显式时间窗因果投影、自动补齐或系统答案 mutation。

### 96.4 B92-A 施工：类别全集与构造表面选择器分权

`EVAL-B92-INVENTORYSELECT1` 已实现。repo_map 与后台 advisory snapshot 两条同源车道在 typed
`IsCategoryEnumeration=true` 时都不再把 `SourceQuotes` 或 analyzer prescan entities 自动铸成 symbol token query；模型显式传入的
tool `query` 仍按原合同过滤。profile quote/entity 仅能在与 parser graph 发布的 `SurfaceTerms` 精确相交后参与构造族筛选，因此
ArkTS decorator、Cangjie extend/foreign 以及其他语言新增 parser surface family 继续自然接入，不靠语言关键词特判。

新增 `IsConstQualifiedTypeInventory` 统一判定 profile-wide facet 的适用范围：raw roles 只能是 type、constant（qualifier）和 unknown；
出现 function/method/file 等独立类别即禁止折叠。string enum 生成器在此基础上再要求 string underlying；历史 numeric const-qualified
type 的 principal 语义保持。回归覆盖 3 type / 5 production function / 2 constant + 1 test 的复合枚举、显式 query 正臂、ArkTS
quote/entity surface-family 两臂、快照车道和 LOC convergence。状态：`implemented/targeted-pass/full-suite-running`；B92-B 后按严格两并发
回放验收。`internal/types`、`internal/agent`、`internal/tool/repomap` 与 `internal/tool` 全量通过（tool 167.338s），状态更新为
`implemented/full-pass/replay-next`。没有读取原始请求或答案文本作硬门，也没有修改 Trace、系统补齐或 Finalizer 的结论所有权。

### 96.5 B92-B 施工：typed endpoint 的 participant identity 先验诊断

`EVAL-B92-DIAGIDENT1` 已实现定向修复。call-evidence 校验先从 parsed Mermaid declarations 建立 node-id → visible identity 表，只对
**精确等于 citable typed call Subject/Object** 且有两个不同 alias 实际参与 call edge 的 endpoint 发射
`duplicate_participant_identity`。诊断一次性要求：删除重复声明、复用一个 alias、body 所有边与 `edge_anchors` 都使用同一 verbatim
alias。随后原有 same-direction call evidence gate 原样执行，不接受 canonical symbol 替代 body alias，也不由系统改写模型图。

该判定适用于 sequence/call_dag 和所有非 runtime-trace question families；endpoint 形式覆盖自由函数、`.`、`::`、`#`，不绑定 Java
或任一语言。相同 class/actor 展示标签若由不同 message operation 精确消歧则继续合法；未参与 call 的重复展示声明也不硬拒。
RootCauseTrace/显式时间窗因果投影仍在入口处完全隔离。diagram 定向矩阵与 `internal/tool` 全量通过（172.707s），状态：
`implemented/full-pass/replay-next`。

## 97. 2026-08-04 B93 r19：B92 生产复验、bounded scope 血缘污染与 waiver 兼容解码缺席

### 97.1 严格并行与人工审计

在 `main@0d4cf7329907` 构建后，严格并行恰好两个 read case：

- `qf_multi_member_set_count_caveat`：runner/human PASS，405s，16 次 source_inventory、28 个 Explorer 轮、12 次 midloop 注入、5 次 context prune；
- `qf_sequence_analyzer_gate`：runner/human FAIL，542s，13 次 read、22 个 Explorer 轮、6 次 Finalizer reject，最终进入 degraded answer。

工件见 `eval/parallel_selected_summary_evalcampaign_b93_b92_replay_r19_20260804.md` 及对应 manual audit。本轮没有运行 Trace，不能据此改变显式时间窗、因果投影或自动补齐的任何裁定。

### 97.2 B92-A 生产通过，但确认 `EVAL-B93-SCOPELINEAGE1`（P0）

inventory 最终答案与 checkout 一致：type=3，production function=5，Kind constant=30，Kind const block=1。B92-A 对复合角色、隐式 selector 与 production/test universe 的修复生产生效。

但系统在第 7 个 Explorer 轮已经拿到 `internal/analysis/criterion` 的完整目标 roster，随后仍把此前 repo-wide、budget-truncated observation 的缺页/source-class debt强制附着到有界答案。调度器要求继续访问 `cmd`、两个 fixture、`internal/hitraceconv/testdata/embedprobe`、`internal/skill`，继而又切到 ArkTS/Cangjie thirdparty corpus；模型在日志中明确指出这些 scope 与用户目标无关，仍无法关闭。最终虽然答对，却消耗 16 次 lens、28 个 Explorer 轮、84,833 estimated context tokens 与 405s。

根因类不是“模型不肯停止”，而是 durable observation 采用全局 role 合并，`Scopes/SourceClasses/Page/Execution` 与每个具体 lens identity 没有同源绑定；较早 broad lens 的 incomplete 位与后来 bounded complete lens 被压成一份复合状态。completion/follow-up 再从复合状态推导债务，导致窄窗完成不能支配同角色的旧广域导航债。最优方案冻结为：

1. completion authority 必须先选取与 typed requested path、roles、source class/surface family 相符的 lens lineage，再判断 complete/page/execution；
2. bounded complete lens 只能关闭同一 requested universe，不得清除真实 repo-wide 请求的广域债；反之，旧 repo-wide incomplete lens 也不得污染明确的 bounded request；
3. follow-up query 必须继承被选 lens 的 identity，cursor 只能在同 identity 延续；scope 变化清 cursor；
4. 全部判定读取 engine-minted query/complete-lens carrier，不扫描用户/模型/答案 prose，不由系统替写结论。

状态：`EVAL-B93-SCOPELINEAGE1=confirmed/P0/next-code-batch`。

SCOPELINEAGE1 已施工。`SourceInventoryCompleteLens` 现在随每次查询保存仓库根坐标
`query_path_scopes`；`repo_map` 在 selected-subrepo 坐标回基后把同一 observation 与
complete lens 重新写入 durable carrier，不再只有可见返回值携带该坐标。归一化器只在
尚无 lineage 时从单次 observation 铸 complete lens，禁止从已合并的
`Scopes/Sets/Provenance` 联集制造不存在的“联合完整查询”。follow-up/authority 仅在
requested path 的每个主角色均有 `tool_query + explore/pre-explore` 执行凭证、路径覆盖且
`count=total` 时消除旧 root budget debt；路径不同、analyze-only、缺角色、部分计数均继续
精确补采，没有 request path 的真实 repo-wide 请求保持原债。系统仍不从 navigation
凭证生成答案行，精确成员覆盖门与模型结论所有权不变。正负/持久接线/无伪联合 lens pin
及完整 `internal/tool`（161.762s）全绿；LOC ratchet 初次正确拒绝增长后通过职责拆文件复绿，
未抬高既有 ceiling。状态：`EVAL-B93-SCOPELINEAGE1=implemented/full-tool-pass/replay-next`。

### 97.3 `EVAL-B93-WAIVERWIRE1`（P0）：兼容解码只恢复 aggregate，丢失同尾 typed waiver

sequence 的源码事实已经查清：`buildAnalysisIR -> gate.RunWith @ analyzer.go:2666` 与 `gate.Run -> RunWith @ gate.go:135` 是两条汇入同一 callee 的并列边；`buildAnalysisIR -> gate.Run` 不存在。completion gate 第一次拒绝并指导使用 `principal_span_waiver.reason=no_directed_path` 是正确的。

模型随后三次都提交了正确 reason/rationale，但生产 tool call 把 `principal_span_waiver` 放在字符串化 `aggregate_facts` 数组后的对象尾部。系统已有 `decodeAggregateFactsPayload` 会解析该尾部，并已恢复 `relation_claims` 以及部分 completion 字段；`loadFromRaw` 却没有从同一个 parsed `misplaced` map 恢复 `principal_span_waiver`、`evidence_floor_waiver` 与对应 clear flags。因此成员集被成功消费，waiver 同载荷静默丢失，系统再次返回“请提交 no_directed_path”，形成无法由模型理解的合同自矛盾。

最优修复不是放宽 directed-path gate，也不是扫描自由文本。应在同一 strict typed recovery helper 中恢复所有 schema-owned sibling fields，执行与 top-level 完全相同的 enum/rationale/互斥校验；top-level 值始终优先，尾部仅在 top-level 缺席时兼容恢复，未知字段继续拒绝/忽略按既有策略。新增 e2e pin 必须使用本次真实的 stringified aggregate tail 形，证明 no-path boundary 一次闭合，错误 enum、空 rationale、clear+new 冲突仍 fail-closed。

降级答案人工判 FAIL：正文用“随后”把两条并列边写成连续路径，与 typed graph 相反。状态：`EVAL-B93-WAIVERWIRE1=confirmed/P0/next-code-batch`。

WAIVERWIRE1 已施工：string-wrapped aggregate tail 的统一恢复面现在覆盖 `principal_span_waiver`、`evidence_floor_waiver` 及两者 clear flag，而非只恢复 aggregate/relation。顶层声明对整个 waiver family 保持优先；仅当顶层 family 缺席时才消费尾部；尾部 clear+new 仍进入既有互斥拒绝，非法 enum、空 rationale 与未知嵌套字段不获权。生产同形 e2e pin 证明 `no_directed_path` 一次闭合并发布 typed endpoint boundary，系统不再要求重复提交已在场字段。定向矩阵与完整 `internal/tool`（172.639s）全绿。状态：`EVAL-B93-WAIVERWIRE1=implemented/full-tool-pass/replay-after-scope-batch`。

### 97.4 `EVAL-B93-CALLIDENT2`（P1）：typed call owner 展示身份仍不对称

本轮 B92-B `duplicate_participant_identity` 未触发，因为稿件没有两个 active alias 精确指向同一 citable endpoint；因此 B92-B 仍是单测/全包验证通过，生产正证待后续异构回放。

Finalizer 的六次拒绝还暴露一项独立身份不对称：`ParseOutput` call 在 grounding 前被规范成 `analyzerEvaluator.ParseOutput`，而 package-level `gate.Run -> RunWith` 仍以短 `Subject=Run/Object=RunWith` 进入 evidence。用户、definition 与 participant 的自然展示却是 `gate.Run/gate.RunWith`。现有 qualified-caller resolver 要求两端同时限定 owner，故“限定 caller + 裸 callee”的 parser-grounded call 无法稳定绑定，模型在 `Run`、`GR`、`gate_Run`、`gate.Run` 间反复试错。

后续只能补 typed owner/presentation identity：由 parser-owned call site、enclosing callable、package/module/receiver 与唯一 source:line 铸造可展示限定身份，sequence body、edge anchor 与 principal completeness 共享同一 resolver；多 owner、重载、多位置、contrary object 均 fail-closed。不得按语言或本 case 符号特判。状态：`EVAL-B93-CALLIDENT2=confirmed/P1/after-P0`。

CALLIDENT2 已施工。冷读确认上游已在 call evidence grounding 后用 parser graph 铸造
`OwnerSymbol`，并覆盖 Go、Java/Kotlin、JS/TS/ArkTS、C/C++、Rust、Python、Ruby、Swift、
Lua 与 Cangjie 的 package/module/receiver 分隔形；缺口在共享 diagram resolver：它只接受
`gate.Run -> gate.RunWith`，却拒绝同一 typed row 的自然展示 `gate.Run -> RunWith`。
resolver 现允许“限定 caller = system-stamped OwnerSymbol，短 callee = 同行精确
Object/AnchorSymbol”，并供 visible body、edge anchor 与 principal completeness 共用；不再要求
caller 必须来自 request anchor。无 owner、错 owner、跨 owner 限定 target、多个 source:line
仍 fail-closed，跨 owner 解析继续要求唯一 definition-backed proof。三种限定符、三类 question
family、B93 principal 同形及全语言 owner 铸造矩阵已通过定向测试。状态：
`EVAL-B93-CALLIDENT2=implemented/full-tool-pass/replay-next`。完整 `internal/tool`
174.463s 通过。

## 98. 2026-08-04 B94 r20：scope lineage 生产生效；complete-lens 成员等值与 Mermaid code 展示仍断线

### 98.1 严格并行与人工结论

在 `main@3907d9995` 构建后启动严格并行恰好两个 read case。外层批跑器因交互转向提前退出，但其两个既有 worker 保持运行且没有
启动第三例；两个 worker 均自然完成，结果由各自 `run-1.verdict/metrics/logs` 人工回填：

- `qf_multi_member_set_count_caveat`：runner/human FAIL，205s，5 次 source lens、8 个 Explorer 轮、1 次 Finalizer reject；
- `qf_sequence_analyzer_gate`：runner/human FAIL，613s，17 次 read、34 个 Explorer 轮、7 次 Finalizer reject，最终无可用答案。

工件见 `eval/parallel_selected_summary_evalcampaign_b94_b93_replay_r20_20260804.md` 及对应 manual audit。本轮没有运行 Trace，不能据此
改变 RootCauseTrace、显式时间窗、因果投影或自动补齐的权限与行为。

### 98.2 B93 修复的生产验收

`EVAL-B93-SCOPELINEAGE1` 明确生效：inventory 相对 B93 从 16 次 lens、28 个 Explorer 轮、405s、42% context 降至
5 次 lens、8 轮、205s、23%，旧 root truncation 不再驱动 `cmd/fixture/thirdparty/internal/skill` 扩散。

`EVAL-B93-WAIVERWIRE1` 也获得正证：sequence 第一调查段提交的 top-level
`principal_span_waiver.reason=no_directed_path` 当轮即被接受，completion summary 发布 exact
`buildAnalysisIR -> gate.Run` 不可达边界；没有复发“解码器丢 waiver、后门又要求同一字段”的不可满足合同。

`EVAL-B93-CALLIDENT2` 的限定 owner 正臂未获得最终答案级生产验收：模型没有稳定画出
`gate.Run -> gate.RunWith` 的真实并列边，且更早被本轮 code-mark identity 缺口挡住，状态维持
`implemented/full-tool-pass/replay-after-B94-B`，不能虚报 production close。

### 98.3 `EVAL-B94-LENSPARITY1`（P0）：请求绑定 complete lens 没有获得成员等值权

inventory 的 typed tool result 已同时披露 `constant complete=true count=30` 与
`role constant count=30 len(members)=30 complete=true`，成员包含 runner 动态 oracle 检出的
`KindExternalArtifactDecoded`。模型因分页 `cursor=19` 与首屏重叠，先把 24+11 误算成 35，又手工扣除 6 项，最终只发布 24 个
Kind 常量。completion 接受该 member_set，Finalizer 只因首稿未用 structured item label 拒绝一次；补成同样 24 行后即通过。

根因是 `sourceInventoryCanProjectCompletePrincipalRowSet` 仍从**全局合并后的 role set**判断 complete。早期 broad/incomplete
同角色 set 会让 principal projection 整体 disabled；B93 新增的请求路径 complete-lens lineage 只接到了 follow-up debt 清理，尚未接到
principal row projection。于是“请求窗已经 complete”可停止探索，却不能把同一窗内 30 个 typed rows 变成模型必须逐项消费的精确清单。

最优方案冻结为请求绑定的 exact parity，而不是系统改写答案：仅当同一 executable tool-query complete lens 同时满足
`QueryPathScopes × principal role × source class/language/surface family × count=total`，并且 observation 中受该 lens 约束的唯一 typed
rows 数与 count 相等，才允许该 lens 驱动 principal rowset projection。模型 member_set 与 typed row keys 必须等值；缺项、额外项、
错 role/路径、分页不全或仅 analyze-stage provenance 都 fail-closed，并把 missing/excess typed roster 交回模型修复。系统不生成结论，
只提供机械清单权威；repo-wide、relation member_set 与 support-only 车道保持原语义。

### 98.4 `EVAL-B94-DIAGRAMCODEMARK1`（P0）：合法 inline-code 展示污染 typed endpoint identity

sequence evidence pool 在 Finalizer 前已有 13 条 `grounded` direct call edges，另有 grounded
`buildAnalysisIR -> gate.RunWith`；模型使用合法 Mermaid 展示形
``participant buildIR as "`buildAnalysisIR`"``。`diagramEvidenceLabelSymbol` 只去除 `<br/>`/位置后缀，不去除**完整包裹单一 code
identity** 的 Markdown backticks，resolver 因而拿 `` `buildAnalysisIR` `` 与 typed `buildAnalysisIR` 比较，所有真实边一起变成
`call_edge_unproven/principal_call_edge_missing`。模型连续改 alias、full ID、edge anchors、删除 anchors 均无法满足，7 次成文拒绝后
退化且无可用答案。

根修必须位于 Mermaid declaration 的 presentation normalization：只对解析后的 label 第一行，在它恰好由一对 backtick 完整包裹且
内部通过 code-identity 语法校验时剥离展示标记；非完整包裹、含 prose/多个 token、反引号失衡均原样保留并 fail-closed。visible body、
explicit edge anchor、principal completeness 与 duplicate identity 必须共享同一归一化结果。不得模糊匹配、不得从模型正文猜 symbol、
不得放宽方向或 recovered/ungrounded 权限；应覆盖 `.`, `::`, `#` 及全部语言的自由/限定 identity。

### 98.5 `EVAL-B94-CALLFANOUT1`（P1，确认症状、待根因收窄）

同一 sequence 在 typed no-path boundary 已接受后仍发生 4 次 Explorer dispatch、34 个 Explorer 轮、19 次 midloop、11 次 completion
提交（5 次拒绝），多次重复读取相同行段并重建同一 endpoint roster。这不是 code-mark 导致的 Finalizer 七拒本身。当前只冻结症状：
后续须按 `PlanID/task-node/evidence-closure generation` 区分必要的交叉验证与陈腐 sibling 重探，确认是否缺少“同 endpoint boundary 已闭合”
的共享 typed credential。未完成异构否证前不得增加 hard skip，更不得扫描 request/think/final prose 关键词短路调查。

施工顺序冻结：B94-A `LENSPARITY1`；B94-B `DIAGRAMCODEMARK1`；每批独立测试、提交、推送；随后仍以恰好两个用例回放两项修复，
并采集 `CALLFANOUT1` 的 node/generation 证据。两批均显式排除 Trace runtime authority、显式时间窗因果投影、自动补齐和系统答案 mutation。

### 98.6 B94-A 施工：请求绑定 complete lens 获得精确成员等值权

`EVAL-B94-LENSPARITY1` 已实现并完成全包验证。principal row projection 现在优先检查**每个请求 principal role**是否存在一条
同源 executable tool-query complete lens；只有 `QueryPathScopes × role × source class × language × surface family × count=total`
全部覆盖、且该角色的 typed principal rows 数与 lens count 精确相等时，才允许请求绑定 lens 越过旧 broad observation 留下的 merged
incomplete 状态。缺路径、analyze-only、partial count、行数不等、错角色/语言/源码类别/surface family 均 fail-closed。

该授权只把已有 typed rows 交给既有 member-set parity gate，不制造答案行、不替模型下结论；relation member set、无明确 requested path、
repo-wide 请求及 support-only 车道保持原合同。生产同形 types pin、completion landing 接线 pin、八类 lineage 负矩阵与 LOC convergence 均通过；
完整 `internal/types`（21.943s）和 `internal/tool`（162.979s）通过。状态：
`EVAL-B94-LENSPARITY1=implemented/full-pass/replay-after-B94-B`。

### 98.7 B94-B 施工：Mermaid inline-code 仅作为精确 endpoint 展示包装

`EVAL-B94-DIAGRAMCODEMARK1` 已实现并完成全包验证。共享 `diagramEvidenceLabelSymbol` 在既有首行与 location suffix 投影之后，
只剥离“首尾各一个反引号、全串只有这一对、内部无首尾空白且按跨语言 code identity segments 自洽”的展示包装；失衡反引号、多个
token、prose、嵌套/多对 backtick 原样留给既有 evidence resolver fail-closed。该归一化由 sequence/call-DAG visible body、explicit
edge anchor、principal completeness 与 duplicate participant identity 共用，没有新增第二套 alias 规则。

红测先稳定复现 14 种可执行语言的合法真边全部被 `` `identity` `` 污染；修复后 Go、Python、JS/TS/ArkTS、Java/Kotlin、Rust、
C/C++、Ruby、Swift、Lua、Cangjie，以及自由函数、`.`、`::`、`#` 形全部通过。反向 call 仍被拒、malformed/prose wrapper 不获权、
duplicate identity 仍优先诊断，`QFRootCauseTrace` 入口隔离 pin 保持。完整 `internal/tool`（161.197s）通过。状态：
`EVAL-B94-DIAGRAMCODEMARK1=implemented/full-pass/replay-next`。

## 99. 2026-08-04 B95 r21：B94 双例回放，正确性门生效但 pre-normalize 与跨节点 closure 仍断线

### 99.1 严格双并发与人工结论

在 `main@a58ee0d06` 构建并快照后，严格并行恰好两个 read case：

- `qf_sequence_analyzer_gate`：runner PASS / human FAIL，496s，3 个 Explorer dispatch、27 轮、16 次 read、2 次 Finalizer reject；
- `qf_multi_member_set_count_caveat`：TIMEOUT / human FAIL，1500s，4 个 Explorer dispatch、65 轮、34 次 source lens、21 次 completion
  （10 次 reject）、9 次 tool-history prune，最大 context 56%。

工件见 `eval/parallel_selected_summary_evalcampaign_b95_b94_replay_r21_20260804.md` 及 manual audit。本轮仍未运行 Trace；不得据此改变
RootCauseTrace、显式时间窗、因果投影、自动补齐或模型结论所有权。

### 99.2 B94 验收状态

`LENSPARITY1` 获得**部分生产正证**：模型提交 30 个 Kind member、却沿用 `value=24` 时不再静默出厂，completion 精确拒绝；但它尚未
闭环，见 PRENORMPARITY1。`DIAGRAMCODEMARK1` 的旧症状（合法 code-mark 令 grounded 真边批量变 unproven）未复发；本轮模型最终稿未用
inline-code participant，故状态仍为 `implemented/full-pass/production-positive-witness-pending`，不虚报 production close。

### 99.3 `EVAL-B95-PRENORMPARITY1`（P0）：typed exact roster 位于 aggregate Normalize 之后

首个 inventory completion 的 Kind fact 实际已有全部 30 个 typed 名称，但模型把旧分页计数 24 留在 `value`。当前执行顺序先调用
`NormalizeAnswerAggregateFacts` 做 `value == len(members)`，失败即返回；B94-A 的 request-bound principal row projection 位于其后，
因此永远没有机会用同一 authoritative roster 完成机械校准/等值交接。模型随后在 24→29→30 之间手工修补，并把 private/helper 函数
带入 principal set。

根修只能前移**精确信号限定的机械校准**：仅当 requested-path executable complete lenses 已对全部 principal roles 闭合，且当前
source-inventory principal fact 与该 authoritative row universe 有 typed overlap、后续必进入同一 row projection 时，才把该 fact 的
count canonicalize 为其结构化 `members[]` 长度并披露 repair note；随后既有 projection 仍用 typed exact roster 替换/约束 principal
universe。无 exact lens、错路径/角色/语言/源码类别、relation set、repo-wide 或无 overlap 继续原样拒绝。禁止对任意模型 member_set
普遍“数数组长度后改结论”。状态：`confirmed/P0/next-code-batch`。

### 99.4 `EVAL-B95-DAGCLOSURE1`（P0，吸收 CALLFANOUT1）：请求绑定 closure 未跨 sibling node 继承

sequence 的 `n0_probe` 已接受 `principal_span_waiver=no_directed_path` 并发布 exact boundary，调度器仍进入预建 `n2_validate`，携带
248 条 evidence 后重新读取、重发同一批 endpoint 边；inventory 首节点的 bounded complete lens 生效，后续 sibling node 却重新发出
`path="."` repair，并把 `cmd/fixtures/embedprobe/internal/skill` 等旧 root debt 接回请求，最终 34 次 lens 超时。

最优方案不是取消 validate 或按关键词 hard skip，而是给 DAG closure 增加 `request_universe_id × node_generation × typed credential kind`
的 durable receipt：后续 sibling 可验证 receipt 后直接消费已闭合 roster/endpoint boundary；validate node仍可检查新的 ordering/grounding
目标，但不得重新打开同一 universe 的 navigation debt。scope/role/source-class 改变或证据反证必须生成新 generation 并重新探索。
状态：`confirmed/P0/after-PRENORMPARITY1`。

### 99.5 `EVAL-B95-ENDPOINTFOCUS1`（P1）：终点边界被同层 call roster 淹没

sequence 最终答案知道实际终点是 `gate.RunWith`，却写“`gate.Run` 若存在”，遗漏源码已证 `gate.Run -> RunWith` 包装边，并把约 30 个
`buildAnalysisIR` 同层直接调用当作一条主链。typed no-path boundary 本身正确，问题是最终上下文没有把 requested endpoints、nearest
proven edge、reverse/parallel edge 做成一个高优先级小型 typed capsule，反而让多次 sibling 重探的巨大 aggregate roster占据主面。
先通过 DAGCLOSURE1 去重；若异构回放仍复现，再为 finalizer 增加 typed endpoint-boundary capsule/soft guidance。不得扫描答案原文、不得
由系统替写“无路径”结论。状态：`confirmed/P1/after-DAGCLOSURE1`。

施工顺序冻结：B95-A PRENORMPARITY1；B95-B DAGCLOSURE1；每批独立测试、文档、提交、推送；之后再严格双例回放，并决定
ENDPOINTFOCUS1 是否仍需施工。

### 99.6 B95-A 施工：authoritative roster 的 count 校准前移到 Normalize 之前

`EVAL-B95-PRENORMPARITY1` 已实现并完成全包验证。types 层新增 import-cycle-safe 的 request-bound roster authority，只在全部 requested
principal roles 具备 executable tool-query complete lens，且路径、count/total、typed row count、语言、源码类别与 surface family 全闭合时
返回 roster。completion 在 generic aggregate Normalize 之前读取该 authority；仅对 principal source-inventory member_set、且至少一个
structured member 与 authoritative roster 精确重合时，把 stale `value` 校准为当前 `members[]` 长度，并把每次校准写入 repair note。

这不是普遍“数组多长就改成多少”：无 requested path、inexact/analyze-only/partial lens、relation member set、非 principal、无 typed overlap
与非法非整数 value 均不获权。校准只消除旧分页数与已提交数组的机械矛盾；随后 B94-A 的 exact projection 仍是唯一 principal universe
authority，因此 production/test/private extras 不能借此升级为答案成员。生产同形红测（Kind 30 名/value 24 同类）、projection 接线、无路径权限、
relation 与 disjoint 负臂及 exact LOC ratchet 均绿；完整 `internal/types`（22.105s）和 `internal/tool`（170.076s）通过。状态：
`EVAL-B95-PRENORMPARITY1=implemented/full-pass/replay-after-DAGCLOSURE1`。

### 99.7 B95-B 施工：纠正 DAGCLOSURE1 根因——noisy subject repair 不得压过 typed completion

代码与 B95 sequence 全日志冷读推翻了“缺少新的跨节点 closure receipt”这一初判。现有 `MutableState.investigationComplete`、稳定 completion
reason、principal-span waiver 和请求绑定 complete lens 本就跨 DAG sibling 持久；scheduler 的 soft policy 也已有 accepted closure
auto-complete 路径。真实断点发生在 completion 成功之后：Explorer 的 ParseOutput 再次运行 heuristic chain subject ranker，因 97 条候选
的 `bestMatch=0.00 < 0.4` 新铸 `RepairRebindSubject(origin=chain_ranker)`；`RepairBlocksAcceptedClosure` 将这个噪声分数产物当作 blocking repair，
于是已经验收的 `n0_probe` closure 无法关闭 `n1_evidence/n3_analyze_refine/n2_validate`，同一 endpoint 被重复读取、重发。

最终修复没有增加第二套 receipt，也没有跳过 validate 的一般职责：`RepairRebindSubject` 在 investigation 尚未完成时仍保持
`principal_blocking`，继续作为模型的 Subject Constraint 指导；仅在专用的 **accepted-closure** 判定里降为非阻断，不能重开已经通过
pre-complete gates 的 typed closure。精确未读、混合来源缺失、结构化 handoff、view mismatch 和 completion-form 合同均保持原权限。
这符合“精确信号作 hard gate、嘈声分数只作 soft guidance”，且不扫描 request/think/final prose。

新增 production 生命周期 e2e：Explorer 同轮设置 accepted completion 并在完成后铸造 chain-ranker rebind，后续 evidence/validate sibling
仍由既有 auto-complete 路径关闭，Explorer 仅 dispatch 一次；另有 types 边界 pin 证明 rebind 在活跃探索期仍为 principal guidance、在
accepted closure 后不阻断。完整 `internal/types`（24.285s）与 `internal/orchestrator`（13.182s）全绿。状态：
`EVAL-B95-DAGCLOSURE1=implemented/full-pass/replay-next`。B95-A/B 均未触碰 RootCauseTrace、显式时间窗、因果投影、自动补齐或模型结论所有权。

## 100. 2026-08-04 B96 r22：accepted closure 生产闭环；库存 source-class 权威与成文派生缓存成为新主瓶颈

### 100.1 严格双并发与人工结论

在 `main@80d936fec` 重新构建后，严格并行恰好两个 read case：

- `qf_sequence_analyzer_gate`：runner PASS / human FAIL，239s，1 个 Explorer dispatch、13 轮、4 次 read、3 次 Finalizer reject；
- `qf_multi_member_set_count_caveat`：runner FAIL / human FAIL，953s，1 个 Explorer dispatch、32 轮、23 次 source lens、8 次 completion、
  2 次 Finalizer reject、9 次 tool-history prune，最大 context 46%。

工件见 `eval/parallel_selected_summary_evalcampaign_b96_b95_replay_r22_20260804.md` 及对应 manual audit。本轮没有运行 Trace，不能据此改变
RootCauseTrace、显式时间窗、因果投影、自动补齐或模型结论所有权。

### 100.2 B95-B 获得 production close；B95-A 仅在同 source-class 宇宙内成立

sequence 相对 B95 从 3 个 Explorer dispatch、27 轮、16 次 read、496s 降到 1/13/4/239s；accepted completion 之后的
`RepairRebindSubject(origin=chain_ranker,best=0.00)` 不再重开 sibling DAG，且精确 unread/origin/handoff/view 合同没有被跳过。
`EVAL-B95-DAGCLOSURE1=closed-by-production-replay`。

inventory 也只有 1 个 Explorer dispatch，说明同一 accepted-closure 修复没有对库存类退化；但 B95-A 的机械 count 校准没有拿到授权。
原因不是 pre-normalize 接线再次断裂，而是其输入权威本身没有区分 production/test 子宇宙，见下一节。因此
`EVAL-B95-PRENORMPARITY1=implemented/production-blocked-by-class-partition`，不能虚报闭环。

### 100.3 `EVAL-B96-CLASSPARTITION1`（P0）：complete lens 把混合 source class 总数绑定给 production 主席

目标包的完整 function observation 有 56 行：5 个 production function 与 51 个 `_test.go` test function。现有
`SourceInventoryCompleteLens` 每个 role 只有一条 `Count/Total`，同时把所有出现过的 `SourceClasses` 放入集合，因此 function lens 为
`count=total=56, classes=[production,test]`。默认 source-inventory 主席范围按既有请求合同是 production，principal typed rows 正确筛成 5 行；
但 exact authority 又要求 `lens.Count == len(principal rows)`，于是 56 != 5 后 fail-closed。系统随后把 broad/incomplete debt接回同一调查，
模型最终把 51 个测试函数也列为公开生产 function，动态 oracle 正确以 `exported_functions=5` 判 FAIL。

根修是由 engine 从一次完整 observation 铸造 `role × source_class` 的分区 complete lenses：保留 combined lens 供显式 all-source 请求使用，同时按
每个 typed row 的 canonical source class 生成 class-local `count=total`、language/surface/provenance 相同的精确子 lens。只有原 observation
本身 complete、查询坐标和 executable provenance 完整时才能派生；partial/page-truncated/merged union 不得伪造分区完整性。旧行缺
`SourceClass` 时只允许使用现有 canonical path classifier，不从用户、模型或答案 prose 猜。默认 production 请求选 5 行 lens；显式 test/all
仍分别选择 51/56。状态：`confirmed/P0/next-code-batch`。

### 100.4 `EVAL-B96-ANSWERPREEMITPERF1`（P0）：同一 typed 派生在 item×citation 热路上重复全图计算

inventory 第三版文档包含 89 个 structured items。成文前 normalize 在“按唯一 label/citation 修复 citation_ref”阶段长时间单核 100%，进程
物理内存约 1.1GiB、峰值约 1.5GiB；3 秒只读 sample 显示主调用链反复进入：

`normalizeItemCitationRefsByUniqueLabelCitationWithContext -> preEmitCitationSupportsAggregateItemWithContext ->`
`preEmitPrincipalAggregateMemberSetFactRefs -> preEmitStableAggregateFacts -> answerSurfacePlan ->`
`normalizeAggregateFactsForTypedExclusion -> answerDocumentExcludedCandidateNames -> 全 graph SymbolDefs census`。

这里每个 item/citation 都重新构建同一 `AnswerSurfacePlan`、稳定 aggregate facts、principal refs 与 exclusion candidate 集。它不是一次正常的
大列表线性成本，而是 document-size × facts × graph-size 的重复派生；最终单例 953s，掩盖了前面探索改进。

最优方案是在单次 `preEmitCheckContext` 内缓存 immutable typed 派生：surface plan、normalized stable facts、principal/relation refs、effective
excluded roles/candidates 与 source-inventory authority。缓存生命周期严格限定一次 emit/patch 校验，文档 mutation 不改变这些 BusContext
输入；跨 turn/patch 不共享，避免陈腐。所有 hard gate 仍消费相同 typed 值，不删校验、不放宽 citation/成员等值合同。增加构造计数/benchmark
证明大 roster 的全图 census 为 O(1) 次、结果与未缓存路径字节等值。状态：`confirmed/P0/after-CLASSPARTITION1`。

### 100.5 `EVAL-B95-ENDPOINTFOCUS1`（P1）异构复现后升级施工

sequence 已拿到 typed `principal_span_waiver=no_directed_path`，答案也知道实际 direct sink 是 `gate.RunWith`；但最终仍写
“`gate.Run` 与 `gate.RunWith` 的关系需要进一步确认”。源码/evidence 已证 `gate.Run -> RunWith` wrapper，同时已证
`buildAnalysisIR -> gate.RunWith`；大量 `buildAnalysisIR` sibling direct calls 却占据主席列表并被叙述成一条链。B95-B 去掉重复调查后症状仍在，
故不能再归因于单纯 context fan-out。

后续为 finalizer 构造小型 request-bound typed endpoint capsule：请求 source/sink、nearest proven source-side edge、sink wrapper/reverse/parallel
edge、directed-path grounding status 与精确 evidence refs。它只排序和压缩已有 parser/grounding 事实并给模型 soft guidance，不生成“有/无路径”
结论、不改写答案、不按 endpoint 名或自然语言关键词硬门；缺证据、多 wrapper、多定义均显式 typed unknown/fail-closed。
状态：`confirmed/P1/after-P0-performance`。

施工顺序冻结：B96-A `CLASSPARTITION1` → B96-B `ANSWERPREEMITPERF1` → B96-C `ENDPOINTFOCUS1`；每批独立测试、文档、提交、推送，
随后严格并行同两例验收。三批均不得触碰 Trace 显式窗、双轴根因、因果投影与自动补齐权限面，不扫描 request/think/final prose作 hard gate，
不允许系统代写模型结论。

### 100.6 B96-A 施工：complete observation 铸造 source-class 精确子 lens

`EVAL-B96-CLASSPARTITION1` 已实现。每个原始 complete role set 继续保留 combined lens；当且仅当全部成员都能由 persisted
`SourceClass` 或共享 `ClassifySourcePathRole(file)` 得到确定类别时，同步铸造 class-local lens。子 lens 的 count/total 来自该类别真实成员，
language/surface-family 也只从同类别成员计算；scope 不参与类别回填，避免 `pkg` 目录把 `pkg/*_test.go` 的 test partition 污染成 production。

任何成员仍为 unknown 时整组不铸子 lens，partial set、分页未闭合或 merged union 也没有入口；combined lens 仍在，因此显式 all-source 请求
保持原能力。production/test/all 三臂生产同形测试分别得到 2/1/3 行；legacy 缺字段但有路径的行可安全分类，缺字段且无路径继续 fail-closed。
现有 request-bound row parity 直接消费这些 lens，系统不制造成员、不修改模型答案。

验证：定向 source-class/unknown/partial/all-scope tests 通过；完整 `internal/types`（22.813s）与 `internal/tool`（166.054s）通过。
LOC convergence 首次精确拒绝新职责文件缺 ceiling，补充 49 行显式 ratchet 后复绿，未抬高旧 ceiling。状态：
`EVAL-B96-CLASSPARTITION1=implemented/full-pass/replay-after-B96-B-C`。

### 100.7 B96-B 施工：pre-emit immutable typed derivation 改为单 context 单次构造

`EVAL-B96-ANSWERPREEMITPERF1` 已实现。`preEmitCheckContext` 现在惰性缓存同一 emit/patch generation 的 `AnswerSurfacePlan`、stable aggregate
facts、principal member-set refs，并让既有 source-inventory authority 复用同一 stable facts。sample 命中的四条 item/citation 热路全部改读该缓存；
document block/item/citation 的逐项位置与成员匹配、所有 hard checks 和 mutation 后复验仍逐次运行。

缓存不跨 patch、Finalizer turn 或 tool invocation；每次 `newPreEmitCheckContext` 都产生独立 generation。只有从 `BusContext/MutableState` 派生且在
单次 pre-emit 生命周期内 immutable 的事实进入缓存，doc mutation 从未进入，因此不存在把前一稿签名借给后一稿的权限提升。语义等值 pin 将缓存
facts/refs 与旧直算结果逐字段对比，128 次 item citation + principal membership 消费只允许 plan/facts/refs 各构造一次。

Apple M5 Max 微基准（128 items，3 次）：cached 约 1.45ms、0.73MB、23.2k alloc；旧 uncached 对照约 9.3ms、6.64MB、136k alloc，
约 6.4× wall-time 改善、89% bytes 与 83% allocations 降低。完整 `internal/tool`（168.932s）通过。状态：
`EVAL-B96-ANSWERPREEMITPERF1=implemented/full-pass/replay-after-B96-C`。

### 100.8 B96-C 施工：请求端点边界携带共享 typed 调用图证据胶囊

`EVAL-B95-ENDPOINTFOCUS1` 已实现。此前 completion hard gate 内部能精确判定 source→sink directed path，但 finalizer 只收到
`no_directed_path/source/sink` 三个字段与原则性提示，拿不到已经 grounded 的 `buildAnalysisIR -> gate.RunWith`、
`gate.Run -> RunWith` 两条真实边；模型只能从大 fan-out support pool 再推一次，因而把并行汇合写成“关系待确认”。

本批把既有语言无关调用图上移为 `internal/types` 单一实现：只接收 citable、current-source、code/config path 的
`ClaimCallEdge`；definition/recovered/runtime artifact/自由 prose/source order/prefix sibling 均不能铸边。`.`、`::`、`#` 与唯一短名解析沿用同一
canonicalizer；歧义端点继续要求 coverage 闭合，否则 fail-closed。原 completion reachability 门已删除私有图实现并委托该共享 engine，防止调查门与
成文上下文产生第二套方向语义。

仅当 `QFCallChain + typed no_directed_path waiver + ordered endpoint profile` 已成立时，semantic view 才附加 bounded capsule。状态枚举覆盖
`no_grounded_call_edges / endpoint_unresolved / endpoint_ambiguous / directed_path_present / reverse_path / parallel_convergence /
disjoint_frontiers`；载荷只含真实方向 edge、evidence ID、source:line、shared frontier 及最多 3 条边界边。finalizer prompt 逐行展示这些 typed
事实并明确“模型自行综合与下结论”；系统不生成结论、不改写 AnswerDocument、不新增 answer hard gate，也不扫描 request/think/final prose。

生产同形 pin 证明并行形输出 `buildAnalysisIR -> gate.RunWith` 与 `gate.Run -> RunWith`，不翻转第二条边；reverse/disjoint/ambiguous/runtime-exclusion、
缓存深拷贝、多语言 completion parity 均覆盖。另加 `QFRootCauseTrace` 负 pin：即使带同名 endpoints/waiver 也不得进入该源码调用链胶囊，因此显式
时间窗、Trace 双轴根因、因果投影与自动补齐权限面不变。整包回归中发现一个旧 agent fixture 手工构造 zero complete lens 时漏了必需的 typed
`SourceClasses`，按既有生产 schema 补齐 fixture；未放宽 lens key 或 landing authority。

验证：定向 graph/capsule/renderer/RootCauseTrace-isolation pin 均绿；最新完整 `internal/types`（24.419s）、`internal/agent`（3.623s）、
`internal/context`（0.711s）、`internal/orchestrator`（12.771s）通过，完整 `internal/tool`（172.057s）与最终相关定向集（0.914s）通过。
状态：`EVAL-B95-ENDPOINTFOCUS1=implemented/full-pass/replay-next`。

## 101. 2026-08-05 B97 r23：B96 证据面生效；no-path admission 与 analyzer scope 坐标暴露新 P0

### 101.1 严格双并发与人工结论

在 `main@f4f0751fd` 构建并冻结二进制后，严格并行恰好两个 read case。runner 均 PASS，但人工均 FAIL：

- `qf_sequence_analyzer_gate`：236s，1 个 Explorer dispatch、11 轮、4 次 read、2 次 completion、4 次 Finalizer reject，最大 context 23%；
- `qf_multi_member_set_count_caveat`：558s，1 个 Explorer dispatch、24 轮、15 次 source-inventory lens、7 次 completion、8 次 prune，最大 context 44%。

工件见 `eval/parallel_selected_summary_evalcampaign_b97_b96_replay_r23_20260805.md` 与对应 manual audit。两案的 runner oracle 都出现
假绿：sequence 只钉名称/图形，不核实精确关系；inventory 的 production function=5 被“5 个生产函数”旁文命中，但主席又发布 function=56。
本轮没有运行 Trace，因此不得改变 RootCauseTrace、显式时间窗、因果投影、自动补齐或模型结论所有权。

### 101.2 B96 生产验收

`EVAL-B96-CLASSPARTITION1` 获得部分生产正证：目标包 complete lens 已将 function 分为 production 5 与 test 51；Explorer 首批 38 条
grounded evidence 只包含 3 types、5 production functions、30 Kind constants。最终错误 56 来自后续 root debt/模型重新扩域，不是分区 lens
本身失效。`EVAL-B96-ANSWERPREEMITPERF1` 获得 production 正证：相同 89-item 量级不再卡在成文前单核重算，Finalizer 单轮完成；总时长仍由
Explorer 的 15 次无关 lens 主导。

`EVAL-B95-ENDPOINTFOCUS1` 的 B96-C capsule 也在生产生效：finalizer 收到 `evidence_status=endpoint_unresolved`、14 条 grounded call edge 以及
`buildAnalysisIR -> gate.RunWith` 的精确 EID/source:line frontier。它没有越权写结论。但输入调查没有读取/发出已存在的
`gate.Run -> RunWith` wrapper，故 capsule 正确反映了一个错误接受的调查边界；根因转移到 admission。

### 101.3 `EVAL-B97-CALLENDPOINTPROOF1`（P0）：未查看 sink 也能声明 no-path

completion 的 prompt 已明确规定 `no_directed_path` 只用于“both requested endpoints exist”，并禁止把未查看的证据当 waiver；实现却在
call-edge graph 无 path 后，只要模型提交该 enum+rationale 就立即放行。该次只有 source 在 call graph 中，exact sink 无 node，且
`internal/analysis/gate/gate.go` 未读；模型仍成功关闭调查并误称 `gate.Run` 可能不存在。源码实际有 `gate.Run` definition，且其 body
精确调用 `RunWith`。

根修是把 no-path admission 绑定到 exact endpoint existence proof：source/sink 各自必须由 citable current-source typed evidence 唯一解析；
参与 call graph 的节点可由 call edge 证明，叶端点可由 exact `ClaimDefinitionFact` 证明。缺一端时返回定向 read/evidence repair；歧义、prefix sibling、
recovered/runtime/prose 均不得获权。两端已证但无 directed path 时继续接受 typed waiver，并把真实 reverse/parallel frontier 交给模型自行下结论。
不得从 waiver rationale、用户原文或最终答案扫描存在性。

### 101.4 `EVAL-B97-REQUESTBOUNDARY1`（P0）：selected-root operational scope 覆盖请求坐标

Analyzer 首轮使用 `repo_map(path=internal/analysis/criterion, scope=internal/analysis/criterion)`；tool 选择 4-file subgraph，但 observation 对模型与
prescan merge 只留下 operational `scopes=.`，`AnalyzerHints.SourceInventoryRequestedPathScopes` 因而为空。Explorer hard gate 将任务误判为
repo-wide，先拒绝两次 scoped call，再强制全仓 lens。之后请求包的 89-row complete lens 虽全闭合，旧 root candidate-budget debt 仍要求
`cmd/fixtures/thirdparty/internal/skill` 等无关跟进，造成 15 lens、24 轮与错误扩域。

最优方案是在 repo_map parameter normalization 中把 selected graph 的执行坐标与 repo-root query identity 分离：`path=<scope>` 与冗余同值
`scope=<scope>` 规范为一个 repo-root scope；不得因 selected-root 的 `.` 丢失，也不得拼成 `<scope>/<scope>`。只有 analyzer-stage
tool-query provenance 与当前请求精确 path identity 的 typed join 才能铸 requested boundary；任意 Explorer 推断 scope、RequiredFiles、模糊
目录词仍不能缩窄 hard universe。

### 101.5 排期与不变量

施工冻结为 B97-A `CALLENDPOINTPROOF1` → B97-B `REQUESTBOUNDARY1`，每批独立验证、文档、提交、推送，再严格并行相同两例。
`EVAL-B97-DIAGRAMALIAS1` 只列 P1 观察：sequence 的短 operation alias/qualified callee 导致 4 次 Finalizer reject，但最终能自修复；需异构复现后
再扩共享 resolver，不能据单例硬化。所有批次均禁止以 request/think/final prose 关键词作 hard gate，禁止系统改写模型结论，显式时间窗 Trace
因果投影与自动补齐保持负回归隔离。

### 101.6 B97-A 施工：no-path waiver 先证明两端存在

`EVAL-B97-CALLENDPOINTPROOF1` 已实现。共享 types 层新增与 reachability 正交的 endpoint-existence analysis：只消费 citable
current-source code/config 的 `ClaimCallEdge` 与 `ClaimDefinitionFact`；call edge 可同时证明其真实两端参与源码图，definition 只证明精确叶端点存在，
永远不能铸造边或路径。限定名只接受完整 identity，自由/短名只有唯一 tail 时才解析；`.`、`::`、`#` 统一，prefix sibling、歧义短名、
recovered、runtime artifact 与 prose 全部 fail-closed。

completion 现在先保留“若同向路径已证则 no-path waiver 自相矛盾”的既有门；在确实无路径且模型声明 `no_directed_path` 时，再要求 source/sink
均通过 existence proof。缺端点会返回精确 endpoint roster，并铸 typed repair，开放 `repo_map/grep/read_file/emit_evidence` 让模型定位、阅读和发证据；
不会从 waiver rationale 或答案猜文件。两端已证后仍由模型决定是否声明 no-path，系统只把真实边交给 finalizer，不生成结论。

生产同形测试证明只有 `buildAnalysisIR -> gate.RunWith` 时不能关闭；补入 grounded `gate.Run` definition 后才保留 typed escape。另覆盖
definition 不铸路径、RunWith/Run sibling、recovered/runtime、短名歧义与 15 种可执行语言表面。完整 `internal/types`（23.964s）和
`internal/tool`（168.462s）通过。状态：`EVAL-B97-CALLENDPOINTPROOF1=implemented/full-pass/replay-after-B97-B`。

### 101.7 B97-B 施工：selected execution scope 与 repo-root query identity 分轴

`EVAL-B97-REQUESTBOUNDARY1` 已实现。生产同形红测确认 `path=src/alpha + scope=src/alpha` 在 selected subgraph 上先形成错误的
`src/alpha/src/alpha` query coordinate；后续 no-row/required-files 路由可能把执行 scope 改回 `.`，但原请求坐标已经不可逆丢失。这解释了
B97 日志中“4-file scoped projection + rendered scopes=. + 无 SourceInventoryRequestedPathScopes”同时出现，并纠正了只看最终 `.` 的表层判断。

repo_map 现在在 graph load 前做单一 concern 的精确坐标归一化：以当前 active repository/sub-repo root 计算 selected path；仅当 `scope/scopes`
等于该 selected repo-relative path或以其完整 `/` 边界为前缀时，转成 selected-root 相对 `.`/suffix。执行 lens 因而扫描一次正确局部宇宙；既有
`repoMapSourceInventoryQueryPathScopes` 再把它回基为 workspace repo-root identity，写入 observation 与每条 complete lens。工具披露发生过的
坐标修复，但该 prose 不参与 authority。

根 `path=.`、已相对 scope、`src/alpha`/`src/alphabet` 前缀碰撞、路径外值均不变；多仓先以 active sub-repo root 归一，再由既有 rebase 保留
workspace 前缀。requested-path hard authority 仍需 analyzer-stage tool-query provenance 与当前请求 exact path identity join；Explorer scope、
RequiredFiles、模型 rationale 不能自行铸边界。生产同形、嵌套/碰撞/root 负臂、durable observation/complete-lens coordinate 与 prescan merge/
repo-wide policy 定向 pin 通过；完整 `internal/tool/repomap`（2.198s）、`internal/types`（19.794s）、`internal/agent`（3.019s）、
`internal/orchestrator`（14.221s）、`internal/tool`（171.294s）全绿。状态：
`EVAL-B97-REQUESTBOUNDARY1=implemented/full-pass/replay-next`。

## 102. 2026-08-05 B98 r24：请求边界闭环；范围外 aggregate 与修复载体合同成为新 P0

### 102.1 严格双并发与人工结论

在 `main@7fd42889d` 构建并冻结二进制后，严格并行恰好两个 read case：

- `qf_sequence_analyzer_gate`：runner PASS / human FAIL，231s，13 个 Explorer 轮、6 次 read、3 次 completion、2 次 Finalizer reject，最大 context 25%；
- `qf_multi_member_set_count_caveat`：runner FAIL / human FAIL，572s，12 个 Explorer 轮、6 次 source lens、4 次 Finalizer reject，最大 context 35%，最终走 degraded answer。

工件见 `eval/parallel_selected_summary_evalcampaign_b98_b97_replay_r24_20260805.md` 及对应 manual audit。runner 对 sequence 仍只核验名称/图形，未核验
wrapper 方向，故 PASS 是假绿。本轮没有运行 Trace；以下方案不得改变 RootCauseTrace、显式时间窗、双轴根因、因果投影、自动补齐或模型结论所有权。

### 102.2 B97 生产验收

`EVAL-B97-REQUESTBOUNDARY1` 获得 production close：Analyzer 虽仍提交 `path=scope=internal/analysis/criterion`，tool 已把 execution scope 规范为
selected-root 的 `.`，同时保留 repo-root `QueryPathScopes=internal/analysis/criterion`。Explorer 不再被强制转向全仓，6 次 bounded lens 就取得
3 types、5 production functions、30 Kind constants，并读取唯一 const block。旧 15-lens/root-debt 扩散未复发。

`EVAL-B97-CALLENDPOINTPROOF1` 获得 admission 正证：第一次 completion 因 exact `gate.Run` 未证被拒，Explorer 随后读取
`internal/analysis/gate/gate.go` 并发射 definition，两端存在后 `no_directed_path` 才获准关闭。它仍未闭环最终正确性：已读源码明确是
`gate.Run -> RunWith`，但 Explorer 没把该 wrapper call 发成 `ClaimCallEdge`，finalizer capsule 因而只有 source-side frontier；答案把方向写成
`RunWith -> gate.Run`，并把 `buildAnalysisIR` 的同层直调步骤误称为相互调用的线性链。

### 102.3 `EVAL-B98-SCOPEAGGREGATE1`（P0）：范围外模型集合绕过 principal row projection

请求的 typed path boundary 与默认 production principal scope 都已成立。canonical source-inventory row set 正确只有 38 行；但 Explorer 额外发射
`function 公开符号（测试代码）` 51 行并标为 `principal_answer`。`ProjectSourceInventoryPrincipalRowSetAggregateFacts` 只降级与 canonical row key 有
交集但不完整的集合；这个 test-only 集合与 production keys 完全不相交，因而被当作“另一个 principal family”保留。Finalizer 第一稿明确只列
5 个 production function，系统却把 51 个测试函数加入硬 obligation，直接违反 typed principal scope。

根修必须只消费 `SourceInventoryPrincipalRowSet.PrincipalScope`、每个 aggregate 的 row-local `support_refs` 与共享 source-class classifier：当一个
model-emitted principal member set 的全部可定位成员都确定落在当前 principal scope 外，且没有任何 in-scope 成员时，将其降为
`supporting_coverage`；混合、缺位置、unknown class、显式 all/test/auxiliary scope一律 fail-closed，不得从 label、request 或答案词面判断。实现应覆盖
Go `_test.go`、Java/Kotlin test roots、Rust tests、ArkTS/Cangjie fixture/corpus 等共享 source-class taxonomy，而非按语言特判。

### 102.4 `EVAL-B98-REPAIRSHAPE1`（P0）：硬修复提示遗漏自身的结构前提

第一次成文已把 38 个正确成员放入 section items，但这些 section 没有 `surface_role=principal`；硬覆盖计算因此正确判为未覆盖。拒绝提示却只说
“放到 `items[].label/cells`”，roster 又用 `label=<set label>, member=<member>` 展示上下文，没有明确：

1. 承载 block 必须是 `surface_role=principal`；
2. block 必须携带 enumeration facet/claim use；
3. item label 或 table cell 才承载逐字 member identity；
4. `label=` 在 roster 中是集合标签，不是要求复制到每个 item 的 label；
5. 每行必须引用 compatible row-local citation。

模型第二轮其实把 member 放进 item label，但仍漏 principal surface；第三、四轮被模糊提示误导，把集合 label 复制进每一项，连续四次失败后降级。
最小根修是让 hard gate 从自身 typed obligation 生成一份无歧义的 schema recipe，并加生产同形 pin：首轮缺 principal surface 后，提示必须完整列出
上述五项；按提示构造一次即通过。提示是 soft repair context，不能改变成员事实，也不能扫描模型原文。

### 102.5 `EVAL-B98-ENDPOINTTOPOLOGY1`（P1）：端点存在已证，但端点局部真实边没有进入胶囊

existence proof 允许 definition 证明叶端点存在，这是正确的；但本案 exact sink 的 definition body 本身就是 wrapper call。Explorer 已读取该行域，
却只发射 definition，未发射 `gate.Run -> RunWith`。系统不能从 definition 或 waiver rationale 猜造 call edge，也不能扫描答案纠错，因此 finalizer 对
wrapper 方向缺少 typed authority。

后续最优方案是新增 endpoint-inspection 的 typed completion debt：当 no-path 端点仅由 definition 证明时，要求端点局部拓扑被显式处置为有限枚举
（至少一条 grounded outgoing/incoming boundary edge，或 typed `no_local_call_edge_observed` 且有精确 read coverage）。真实边仍只能来自
`ClaimCallEdge`；系统不根据源码文本自行生成结论。先做多语言 wrapper/isolated leaf/overload/virtual dispatch 异构 pin，再决定是否升级 P0。

施工顺序冻结为 B98-A `SCOPEAGGREGATE1` → B98-B `REPAIRSHAPE1` → B98-C `ENDPOINTTOPOLOGY1`。前两批为确定性 P0，各自独立测试、文档、提交、
推送；B98-C 先以 typed soft guidance/债务与异构测试落地，不得新增依赖答案词面的 hard gate。三批均保持模型答案所有权与 Trace 权限隔离。

### 102.6 B98-A 施工：范围外 aggregate 与 principal scope 做 typed 行级对账

`EVAL-B98-SCOPEAGGREGATE1` 已实现。principal row projection 在既有 canonical row set 建立后，新增一个隔离 concern：只读取
`PrincipalScope`、请求角色、observation 的 row source class，以及 aggregate 每个 member 的 row-local support location。仅当一个非系统
principal member set 的所有成员都能按规范化 label + 路径后缀边界 + 精确行号唯一解析，且 active canonical set 确有 principal 行、每行 source class 已知并全部不被当前 principal
scope 接受时，才将该集合降为 `supporting_coverage`；canonical 系统集合保持原权威。

该判定不读取集合 label、用户请求、模型思考/答案、符号命名习惯或语言；Go test、Cangjie fixture 等异构形由共享 source-class taxonomy
统一处理。混合内外范围、缺少 row-local location、歧义/unknown class 与显式 all scope 均 fail-closed。生产同形测试证明 5 个 production
function 继续作为唯一 principal obligation，完全 disjoint 的 test/fixture 集合不再强迫进入公开 API 答案。新增 concern 为 132 LOC，并登记
140 LOC 独立 ceiling；旧 projection 文件回落为 343 LOC，未抬高 351 ceiling。完整 `internal/types`（23.721s）与 `internal/tool`
（171.359s）通过。状态：`EVAL-B98-SCOPEAGGREGATE1=implemented/full-pass/replay-next`。

### 102.7 B98-B 施工：hard repair 完整披露自身 structured-row schema

`EVAL-B98-REPAIRSHAPE1` 已实现。source-inventory principal member coverage 的判定与 hard/soft 权限不变；改动仅发生在拒绝后的模型 repair context。
提示现在把完整 recipe 放在可能被 roster cap/日志截断的成员清单之前：承载块须为可渲染 items 的 section/list/table 且
`surface_role="principal"`；保留合同要求的 `enumeration_item` facet 与合法 claim-use；逐字 member 必须进入 item label/table cell，item/block
prose 不承担 identity；成员清单中的 aggregate 分类字段改名为 `set_label`，并明确它只用于 block title/id、不能替代 member；有 row-local
support/citation 时，item citation_ref 必须引用同一 member 的 compatible citation，不能借相邻行。

生产同形 pin 从“一切 items/facet/claim-use/citation 均正确、唯独 section 缺 principal surface”起步，确认真实 `runPreEmitChecks` dispatcher
以 `ViolExhaustiveMemberSetCoverageDrift` 携带完整 recipe；仅补该 typed surface 字段后，同一 coverage gate 一次通过。系统没有新增 normalizer，
没有修改模型文档、事实或结论，也不解析这段提示反向作 gate。定向 repair/soft-lane 回归与完整 `internal/tool`（169.629s）通过。状态：
`EVAL-B98-REPAIRSHAPE1=implemented/full-pass/replay-next`。

### 102.8 B98-C 施工：端点存在证明与局部拓扑证据分轴

`EVAL-B98-ENDPOINTTOPOLOGY1` 已按 P1 soft-debt 方案实现，没有新增 completion/answer hard gate。共享 endpoint existence analysis 现在保留
`unproven / ambiguous / definition_only / call_edge / definition_and_call_edge` 五种 proof；definition 仍只能证明 exact endpoint 存在，只有真实
citable current-source `ClaimCallEdge` 才表明该端点已有 incident topology evidence，且两者都不能单独铸造请求方向的 source→sink path。

Explorer 的 QFCallChain typed 软指南补充 endpoint-local 处置：读到 exact endpoint definition 并评估 no-path 时，发射 bounded read evidence 中
实际验证的相关 incident call，即使它证明 reverse/parallel/disjoint；若没有可证 call edge，则保持 definition-only，不铸造“无调用”。Finalizer
胶囊同时披露两个 endpoint 的 existence proof；`definition_only` 另标 `incident_call_evidence=not_emitted`，明确它既不等于 leaf，也不证明无 caller/callee，
由模型结合真实 edge 自行下结论。系统不从 definition body、waiver rationale、request/final prose 推断边或修改答案。

异构 pin 覆盖 15 种语言 definition-only identity、definition+edge 合并、短名歧义、recovered/runtime 排除、parallel 与 B98 同形 endpoint-unresolved
胶囊、finalizer 无伪边、RootCauseTrace/显式时间窗隔离。完整 `internal/types`（18.809s）、`internal/agent`（2.689s）、
`internal/orchestrator`（11.330s）与 `internal/tool`（170.313s）通过。状态：
`EVAL-B98-ENDPOINTTOPOLOGY1=implemented-soft-guidance/full-pass/replay-next`。

## 103. 2026-08-05 B99 r25：B98 三项生效；未验证关键词与 patch 引用位移造成两次 runner 假绿

### 103.1 严格双并发与人工结论

在 `main@d6a3cae04` 构建并冻结二进制后，按 `PARALLEL=2` 严格并行恰好两个 read case：

- `qf_multi_member_set_count_caveat`：runner PASS / human FAIL，221s，8 个 Explorer 轮、3 次 source-inventory lens、1 次 Finalizer reject，最大 context 23%；
- `qf_sequence_analyzer_gate`：runner PASS / human FAIL，284s，22 个 Explorer 轮、7 次 read、1 次 completion reject、1 次 Finalizer reject，最大 context 27%。

工件见 `eval/parallel_selected_summary_evalcampaign_b99_b98_replay_r25_20260805.md` 及对应 manual audit。两次 runner PASS 都是假绿：inventory
oracle 只钉 3/5/30 与成员名，未核范围说明；sequence oracle 只钉名称/图形，未核 item→citation 的实际绑定。本轮仍未运行 Trace，以下修复不进入
RootCauseTrace、显式时间窗、双轴根因、因果投影或自动补齐。

### 103.2 B98 生产验收

`EVAL-B98-SCOPEAGGREGATE1` 获得 production close：canonical production roster 为 3 types、5 functions、30 constants，test-only principal
集合没有再次进入答案义务。`EVAL-B98-REPAIRSHAPE1` 获得 production close：首稿虽仍漏 principal enumeration carrier，但五步 recipe 让模型用一次
patch 完整补齐 38 行，旧四次连续拒绝和 degraded answer 未复发。

`EVAL-B98-ENDPOINTTOPOLOGY1` 获得 production 正证：Explorer 在读取 exact `gate.Run` definition 后额外发射真实
`gate.Run -> RunWith` edge；finalizer 正确区分 `buildAnalysisIR -> gate.RunWith` 与 `gate.Run -> RunWith` 两条并行汇合边，并由模型自行给出
“不存在 buildAnalysisIR→gate.Run 有向路径”的结论。系统没有代写结论。该项维持 soft-guidance 设计，不据单次回放升级为 hard gate。

### 103.3 `EVAL-B99-INVENTORYCONTEXTHYGIENE1`：analyzer 猜测绕过 subtopic 清洗进入证据链

inventory 的成员、数量、位置均正确，但答案把源码中没有 `iota` 的普通 `const` block 称为 `iota block`。日志证明污染发生在任何源码验证之前：
analyzer 自由生成的 `keywords` 含 `iota块`，而用户原文仅说 `Kind const block`。现有 source-inventory hygiene 已把带同类猜测的 `sub_topics`
重建为 validated source quotes，却保留 keywords；Explorer 随后从该上下文把 `iota` 写入 30 条 evidence summary、member_set label/note 和 closure
reason，最终形成看似 grounded 的错误机制词。

根修不扫描答案或用关键词作 hard gate：当且仅当 typed source-inventory profile 处于 mechanical-row 主车道时，AnalyzerHints keywords 由已经过
verbatim 校验的 `SourceInventoryProfile.SourceQuotes`、闭合 target roles 与 requested fields 重建。自由模型关键词不再进入后续 prompt；关系流、
Trace、普通解释和非机械 inventory 保持原样。它只是上下文卫生，不拒绝请求、不制造事实、不修改模型答案。

### 103.4 `EVAL-B99-PATCHCITATIONDRIFT1`：稳定条目把 citation pool 索引误当行序

sequence 首稿的 topology 与 17 条 edge citation 均正确；因 endpoint item 缺失及一个 Mermaid alias 写错触发一次 patch。模型在原列表前/中插入
两个 definition row 后，把所有既有 `citation_ref` 按新行序整体平移。`citation_ref` 实际索引 document citation pool，不是 row index；现有
normalizer 只通过唯一 label/candidate 修回 3 条，仍有 14 条 edge item 引用相邻源码行而通过。结论正确但证据绑定错误，属于高危呈现/权威 gap。

根修只在 patch replacement 上读取精确结构：citation pool 必须 inherited；block/item 均有唯一稳定 ID；item 除 citation_ref 外逐字段完全相同；
新 ref 必须精确等于旧 ref 加该 item 的实际 row-position delta。四条件同时成立才恢复旧 ref。新条目保持模型引用，same-position 显式改引不动，
replace-citations 车道不动。实现不解析 label/text 语义、不选择新证据、不改变模型结论。

### 103.5 施工与验证

两项已施工。`EVAL-B99-INVENTORYCONTEXTHYGIENE1` 的异构 construct-role 测试证明 source quotes/typed roles/fields 保留，未验证语言/语法猜测被移出
keywords；`EVAL-B99-PATCHCITATIONDRIFT1` 覆盖多插行位移、same-position intentional edit 负臂和真实 `EmitAnswerDocumentPatch.Execute`
生产接线。eval oracle 同步补强：inventory 禁止当前源码不成立的 `iota`；sequence 要求两个代表边的 visible line 与实际 inline citation line 一致。

定向测试通过；完整 `internal/tool`（166.474s）通过。状态：
`EVAL-B99-INVENTORYCONTEXTHYGIENE1=implemented/full-tool-pass/replay-next`，
`EVAL-B99-PATCHCITATIONDRIFT1=implemented/full-tool-pass/replay-next`。

## 104. 2026-08-05 B100 r26：B99 上下文修复获证；可见载体与端点准入断线成为新 P0

### 104.1 严格双并发与人工结论

在 `main@fc2d9faa5` 构建冻结后，以 `PARALLEL=2` 严格并行恰好两个 read case：

- `qf_sequence_analyzer_gate`：runner FAIL / human FAIL，228s，8 个 Explorer midloop、5 次 read、4 次 Finalizer reject/patch，最大 context 30%；
- `qf_multi_member_set_count_caveat`：runner FAIL / human FAIL，241s，3 次 source lens、1 次 Finalizer reject/patch，最大 context 19%。

工件见 `eval/parallel_selected_summary_evalcampaign_b100_b99_replay_r26_20260805.md` 及 manual audit。B99 上下文卫生获得生产正证：inventory 全链路不再出现未验证 `iota`。B99 patch 引用位移本轮未形成相同 stable-ID 插行形，继续保持 replay-pending，不能虚假收账。

### 104.2 `EVAL-B100-VISIBLECARRIER1`（P0）：隐藏 citation sidecar 冒充可见枚举行

inventory 的 typed 事实正确闭合为 3 types、5 functions、30 constants，模型第一稿也知道常量总数。但常量块只写了数量/const-block 说明，没有列出 30 个名称；与此同时它把 30 个名称放进 `table.items[]` 作为 citation sidecar。渲染器遇到 `table.text` 中的完整 Markdown 表后立即返回，不渲染 `items[]`；成员覆盖门却仍把这些隐藏 item 当作可见行，导致“验证通过、用户看不到”的确定性合同漂移。

根修不是禁止 sidecar，而是让 renderer/validator 共享 concrete block visibility：

1. 无 authored Markdown 的 structured table/list/section 继续由 items 作为可见行；
2. authored Markdown table 为 canonical carrier 时，隐藏 items 只能给表格第一列已经可见的同一成员补 citation；
3. 隐藏 item 不能新增表格不存在的 member identity；category、code-token、support-ref 与 multi-target 备用车道都消费同一可见性边界。

实现不读取答案关键词作分类，只解析 typed block kind 与 Markdown 表结构；不改写模型内容或补造常量名称。

### 104.3 `EVAL-B100-ENDPOINTADMISSION1`（P0）：call-chain schema 必填未形成 runtime 准入

sequence Analyzer 发射 `question_kind=call_chain + predicate_axis=call`，却完全漏掉 schema 声明为 required 的 `call_chain_endpoints`。Go strict decode 对缺失对象仍给零值，运行时没有拒绝；下游又正确禁止从无序 `entities/exact_targets` 猜方向，于是 endpoint reachability、no-path boundary、typed capsule 全部不激活。Explorer 交付 13 个无序成员，Finalizer 在 prose 中猜成错误的 `gate.RunWith -> gate.Run`。图门四轮删掉了伪箭头，却无权替换模型 prose，最终仍错。

根修在 Analyzer admission：非 runtime、非 scalar/role-locate 兼容车道的源码 `call_chain/call` 必须携带 active、request-validated 的 ordered profile；缺失、同端点、非源码 identity 或未获 request-mentioned authority 都在探索前要求模型重发。多上下文 symbol 仍额外要求 exact target pair。方向只来自 profile，entity/exact-target 顺序永不获权。

### 104.4 施工与验证

两项已实现。`AnswerBlockRendersStructuredItems` 与 `AnswerTextLooksLikeMarkdownTable` 成为 renderer/validator 的共享结构判定；生产同形测试覆盖“2 个隐藏 item + 0 个可见 member row 必须拒绝”和“第一列可见同名 row + citation sidecar 必须通过”。源码 call-chain admission 新增 missing/invalid endpoint 红测、先 profile 后多实体 pair 校验、两端正臂和既有 runtime/scalar 隔离；Analyzer skill 同步明确 entity set 无方向权限。

定向测试通过；完整 `internal/tool`（170.868s）通过。状态：
`EVAL-B100-VISIBLECARRIER1=implemented/full-tool-pass/replay-next`，
`EVAL-B100-ENDPOINTADMISSION1=implemented/full-tool-pass/replay-next`。

两项都不改变 RootCauseTrace、显式时间窗、双轴根因、因果投影、自动补齐或模型结论所有权。

## 105. 2026-08-05 B101 r27：B100 双项获生产正证；completion→finalizer typed evidence 交接断层

### 105.1 严格双并发与人工结论

在 `main@bc4101f61` 以 `PARALLEL=2` 严格并行恰好两个 read case：

- `qf_sequence_analyzer_gate`：runner FAIL / human FAIL，227s，11 个 Explorer midloop、4 次 completion 尝试、2 次 Finalizer reject，最大 context 31%；
- `qf_multi_member_set_count_caveat`：runner PASS / human PASS，236s，4 次 source lens、1 次 Finalizer reject，最大 context 25%。

工件见 `eval/parallel_selected_summary_evalcampaign_b101_b100_replay_r27_20260805.md` 及 manual audit。本轮未运行 Trace；以下改动不进入
RootCauseTrace、显式时间窗、双轴根因、因果投影或自动补齐。

### 105.2 B100 生产验收

`EVAL-B100-ENDPOINTADMISSION1` 已关闭：Analyzer 首次接受的 emission 携带 request-validated 的有序端点
`buildAnalysisIR -> gate.Run`，无序 entity/exact-target 未获方向权限。`EVAL-B100-VISIBLECARRIER1` 已关闭：inventory 第一稿 authored Markdown
表格未显示 item-only 成员时被正确拒绝一次，模型随后把 3 types、5 functions、30 constants 全部作为可见行并逐行引用；旧的隐藏 sidecar 假绿未复发。
`EVAL-B99-PATCHCITATIONDRIFT1` 本轮没有形成相同 stable-ID 插行/继承引用位移形，继续 replay-pending。

### 105.3 `EVAL-B101-BOUNDARYHANDOFF1`（P0）：相邻阶段消费了不同 evidence authority

Explorer 已发射并 grounding：`buildAnalysisIR -> gate.RunWith`、wrapper 真方向 `gate.Run -> RunWith`、exact `gate.Run` definition；completion 据此接受
`principal_span_waiver=no_directed_path`。但 Finalizer 的 endpoint boundary 重建只读取 mutable Turn-A/emitted buffer，没有读取已经交接到
`AgentContext.EvidenceItems` 的 accepted evidence；compaction/reset 后它错误输出 `requested_sink_existence_proof=unproven`。同一调查由此同时出现“端点已证并允许
no-path”和“端点未证”的 typed 合同冲突。模型最终在正文写成 `RunWith -> Run`，图门虽删掉无证箭头，却无权替换模型 prose。

根修是统一 typed carrier，不增加答案 hard gate：AgentContext 与 BusContext 的 semantic-view 构造都把 handoff `EvidenceItems` 传给同一个
endpoint analyzer，再与当前 mutable evidence lane 求并集；既有 grounding、current-source、identity/edge 去重和“definition 只证存在、不铸 call edge”规则保持不变。
系统仍只向模型提供准确 evidence capsule，不代写结论，也不扫描用户或模型原文。

### 105.4 施工与验证

新增 pin 覆盖 AgentContext/BusContext 两条交接路径，以及真实 Finalizer prompt：handoff 中只有 exact definition 时必须呈现
`requested_sink_existence_proof=definition_only`、`incident_call_evidence=not_emitted`，不得退化为 `unproven` 或伪造边。完整
`internal/types`（22.439s）、`internal/agent`（4.072s）、`internal/tool`（169.392s）通过；BusContext parity 增补后定向 suite 复绿。

状态：`EVAL-B101-BOUNDARYHANDOFF1=implemented/full-pass/replay-next`。

## 106. 2026-08-05 B102 r28：B101 交接修复关闭；图状态命名与 patch 原位目标成为通用收敛项

### 106.1 严格双并发与人工结论

在 `main@30967b332` 以 `PARALLEL=2` 严格并行恰好两个 read case，runner 均 FAIL、人工均 FAIL：

- `qf_sequence_analyzer_gate`：272s，6 个 Explorer midloop、4 次 completion 尝试、3 次 Finalizer reject，最大 context 29%；
- `qf_multi_member_set_count_caveat`：227s，3 次 source lens、2 次 Finalizer reject、1 次工具不可用提示，最大 context 22%。

工件见 `eval/parallel_selected_summary_evalcampaign_b102_b101_replay_r28_20260805.md` 及 manual audit。本轮仍未运行 Trace。

### 106.2 B101 生产验收与剩余模型波动

`EVAL-B101-BOUNDARYHANDOFF1` 已关闭：Finalizer 从 Explorer handoff 正确获得 exact `gate.Run` definition，输出
`requested_sink_existence_proof=definition_only`，旧 `unproven` 回退消失。sequence 模型在答案开头正确说明 `gate.Run -> RunWith`，末尾 caveat 却又写成
`RunWith -> Run`；diagram hard gate 删除了无证箭头，但没有改写模型 prose，符合答案所有权红线。由于同一答案内部先正确后错误、typed 边界已准确，本项作为模型波动保留异构回放，
不得新增答案关键词/方向句式 hard gate。

inventory 的 production 3 types、5 functions、30 constants 与 38 条 row-local citations 均正确，但答案重复输出一份 Markdown 表和一份 ordered list，
遗漏显式 `function=5` 分类头，并虚构源码不存在的 `iota` 机制。`iota` 首次出现在 Explorer completion，而非 AnalyzerHints；B101 未复现。暂不按单次模型波动做词面门。

### 106.3 `EVAL-B102-GRAPHSTATUS1`（P1）：graph resolution 与 endpoint existence 标签混轴

胶囊同时给出 `evidence_status=endpoint_unresolved` 和 `requested_sink_existence_proof=definition_only`。语义上前者只描述 exact endpoint 未成为 grounded
call-edge graph 节点，后者证明符号定义存在；通用 `evidence_status` 命名却容易被理解为全部证据未解析。渲染键已收窄为 `call_graph_status`，并显式说明 graph status 与两端
`*_existence_proof` 分轴。底层 enum、图分析、waiver、hard gate 与模型结论均未改变。

### 106.4 `EVAL-B102-PATCHTARGET1`（P1）：拒绝 patch 累积出重复 principal carrier

inventory 第二稿已用 authored Markdown table 显示全部 38 个 identity，但缺逐行 structured citation sidecar，coverage 正确拒绝。通用 repair 只说
“ADD missing rows”，没有点名既有表；模型下一轮用 `add_blocks` 新建完整 ordered list，被拒绝表仍作为 patch base 保留，最终两份 roster 同时发射。

根修只改善 typed repair context：复用 gate 的 concrete Markdown table classifier、principal/enumeration annotations 与第一列 identity matcher；当既有表已经显示 obligation
identity 时，提示精确列出 block ID 与 visible-row count，要求用 `replace_blocks` 原位补 `items[]` citation sidecar，并禁止 `add_blocks` 复制同一 roster。
系统不自动删除/改写任何 model block，不按答案关键词决定行为。

### 106.5 施工、验证与留项

定向 agent/tool 测试通过；完整 `internal/agent`（2.480s）、`internal/tool`（161.239s）通过。状态：
`EVAL-B102-GRAPHSTATUS1=implemented/full-pass/replay-next`，
`EVAL-B102-PATCHTARGET1=implemented/full-pass/replay-next`。

`iota` 与缺显式 per-role count 暂按模型波动观察；若另一 inventory family 复现，再评估 typed compact per-role census，不得以原文关键词 gate 修复。B99
patch-citation drift 仍未形成同形 witness，保持 replay-pending。上述两项均不进入 RootCauseTrace、显式时间窗、双轴根因、因果投影或自动补齐。

## 107. 2026-08-05 B103 r29：写模式安全拒绝生效；跨语言 eval checker 的边界与声明权威补齐

### 107.1 严格双并发与人工结论

在 `main@cc9ed4f2c` 构建冻结后，以 `PARALLEL=2` 严格并行恰好两个异构 case：

- `data_json_strict_ids`：runner PASS / human PASS，155s；
- `github_issue_pyo3_iter_nth_overflow_symptom`：runner FAIL / human fail-safe，350s，终态
  `unverified:verification_incomplete`。

工件见 `eval/parallel_selected_summary_evalcampaign_b103_writedata_r29_20260805.md` 及 manual audit。本轮未运行 Trace；本批不进入
RootCauseTrace、显式时间窗、双轴根因、因果投影或自动补齐，也没有扫描用户/模型/答案 prose 作产品 hard gate。

### 107.2 数据链路：typed repair 正确收敛，未形成确定性产品 GAP

严格 JSON 最终值为 `{"ids":["u1","u3"]}`，材料、规则、决策、贡献、reconcile 与 final projection 均闭合。三次 repair 分别处理
材料 DAG 顺序、冲突 filter aliases 与 `compute_contributions` 的 unsupported `include`。planner prompt 已提供 action scaffold、canonical
`filters` 数组形和 `operation=set + value_field` 指南；模型后续均消费 typed failure 精确修正。当前证据更符合模型参数波动，不能据此增加请求/答案词面门或
让系统替模型生成业务计划，保留异构 data replay 观察。

### 107.3 写链路：产品 fail-closed 正确，补丁与旧 eval oracle 都不正确

第一计划的 `checked_sub(n + 1)` 被仓库 checker 正确打红，replan 后模型改成
`(current_length.saturating_sub(1)).checked_sub(n)`。该形避开 `usize::MAX + 1`，却在空 iterator 上把 `0 - 1` 饱和回 0；
`nth_back(0)` 随后可能满足 guard 并访问 `items[0]`。旧 checker 只检查 `checked_sub`、禁 `n+1` 与 past-end assignment，因此把这份仍错误的补丁签绿。

Codrax 产品没有跟随该假绿：本机无 `cargo/rustc`，成功的 Python Make check 只声明了 checker 文件，没有精确声明三个 Rust changed paths；
changed-path gate 因而全部铸为 `uncovered` 并终止 `unverified`。这证明 `changed_path_verification_uncovered` 与
`accept_unverified=false` 正常守住交付，runner 红灯不是代码执行器误判。Analyzer 首次发射的无 grounded 精确 contract 也被正确拒绝一次；第二次改用
soft `satisfies` 后通过。`transition.steps` 仍未发射，因只有同一状态协议的重复 witness，`EVAL-B72-STATETRANS1` 继续 adoption watch，不升级请求 prose hard gate。

### 107.4 `EVAL-B103-METADECL1`（eval authority/implemented）：versioned meta-check 缺 exact input declaration

横向扫描全部 17 套 fixture Makefile 后，确认 10 套 Python/meta checker 的 `check:` 没有 prerequisites。既有产品
`declared_project_check` 只接受“exact candidate + exact target + concrete driver + exit=0 + exact bounded roster member”；因此 checker 即使真实读取 Rust、Java、
JavaScript、TypeScript 或 Python 源文件，也只能证明驱动脚本运行，不能取得被检查文件的 source-static authority。

本批没有放宽产品权限，而是在 versioned fixture 中按 checker 实际本地输入补 Make prerequisites，覆盖 Rust、Java、JavaScript、TypeScript、Python 与多仓 SDK；
另 5 套 conventional `check` 是 native C/C++ recipe，已直接列出编译输入，保持原状；其余 2 套 Makefile 不属于该 meta-check 车道。`make -n check` 对 10 套全部通过，产品既有
`TestRunTestsTypedPolyglotMakeSurfaceCarriesExactCheckWithoutPretendingRustExecution` 与 exact cross-language coverage pin 通过。
该方案不从 script body 猜路径，也不让 driver language 获得目录级权限；只有仓库显式声明的 exact member 可获得窄
`declared_project_check/source_static` caliber。

### 107.5 `EVAL-B103-EMPTYBOUND1`（eval oracle/implemented）：饱和算术掩盖空集合下溢

PyO3 fixture checker 新增 function-scoped 空边界审计：拒绝本轮实际产出的 lossy
`saturating_sub(1).checked_sub(n)` 形，并要求 list/tuple 各自有同一 test body 内的 empty `nth_back(0) -> None` 回归。checker self-test 同时覆盖：

1. lossy 饱和形必须拒绝；
2. lossless chained checked subtraction 不被误拒；
3. empty setup 不得被旧“跨方向立即耗尽”负门误判；
4. 一个 iterator family 的空边界测试不能替另一个 family 授权。

这是 eval-specific oracle 加固，不进入 Codrax 的规划、实现或成文 hard gate。fixture baseline 继续按设计失败，oracle `--self-test` 通过；下次回放只有模型先修复
空边界、再由 exact declared checker 通过时，才可能获得窄 source-static 验证。原生 Rust runtime 缺席仍会在报告中如实披露，不能由该静态 caliber 伪装成 native
target execution/behavior。

## 108. 2026-08-05 B104 r30：Trace 双轴正证与 operation 任务级覆盖权威归位

### 108.1 严格双并发与人工结论

在 `main@dc3560304` 构建冻结后，以 `PARALLEL=2` 严格并行恰好两个异构 case：

- `trace_query_donghu_real_frame_multicausal`：runner PASS / human FAIL，161s；
- `operation_web_manual_summary`：runner FAIL / human FAIL，101s。

工件见 `eval/parallel_selected_summary_evalcampaign_b104_traceoperation_r30_20260805.md` 及 manual audit。前者验证了显式时间窗、自动补齐、
Trace 因果投影以及“实际占时/关键路径候选”和“现规则可消除量”双轴仍同时存在；本批没有改变任何 Trace 查询、选举或投影代码。

### 108.2 Trace 人工失败属于模型未消费精确信号，暂不增加产品硬门

最终上下文已明确提供两条 typed 约束：每块 root-cause board 的 `cross_row_additivity=forbidden`，以及
`lower_priority_dependency`/`priority_inversion_candidate` 不证明低优先级线程已经阻塞高优先级线程。最终答案仍把窄查询窗的
16.617ms 席位与包含它的宽查询窗 23.994ms 席位相加成“超过 40ms”，并写出“实际阻塞了主线程的唤醒路径”。这两句均越过已有证据上限，
因此人工判 FAIL；但同页 caveat、系统投影和双轴表仍正确，context 占用 42%，不存在预算裁剪或事实缺失。

当前只有一个模型波动 witness。按“精确信号才作硬门”和“模型拥有结论”红线，不扫描用户/模型/答案 prose，不让系统替换结论，也不为该句增加
type-specific normalizer；保留异构 Trace replay。若后续跨模型/跨 trace 复现，再优先研究结构化 claim relation 的 soft 引导或模型侧校准，而不是文本硬门。

### 108.3 `EVAL-B104-OPLIST1`（eval authority/implemented）：多值 typed 字段被单值 anchored regex 假拒

operation terminal 日志以精确分隔符 ` | ` 序列化多个 `coverage_material_refs`/`coverage_source_locators`。旧 runner 把整条拼接值交给
单值 anchored regex，因而三个合法 receipt 必然无法匹配 `^material-coverage:v1:<hash>:html_text$`。修复在 eval 侧逐 member 解码并执行
any-match；last-event authority、缺失 ref 负臂和错误 source 负臂保持不变。该改动只读取系统 typed 日志字段，不触碰用户或答案原文。

### 108.4 `EVAL-B104-OPTASKCOV1`（operation/product/implemented）：全历史辅助材料覆盖任务级终验

本轮最终 typed evaluator 已发布 `status=complete`、`material_coverage_status=complete`，并绑定三个经系统 material authority 验证的 receipt；
但 `operationFinalReportWithRecordStatus` 仍只要发现任一历史 payload 未在后续命令出现，就向模型答案前强插“材料覆盖未完全验证”。本例额外抓取的
Trace 专页没有被最终任务级 receipt 集选中，于是可见状态与 typed 终验/模型正文冲突。

修复后，最新任务级 evaluator 的 `complete + complete`（每个选中 ref 均在 recorded available/complete authority 中）或显式
`complete + not_applicable` 可以抑制这个全历史降级前缀；未评估、partial、缺 ref、未知 ref 或非 complete evaluator 仍保留旧警告。
未选中的辅助 payload 继续留在审计 ledger，不获得任务证据权，也不被静默删除。系统只校验 typed receipt，不决定哪些材料回答用户目标，模型正文保持字节不动。

### 108.5 验证与状态

- `bash eval/runner_lib_test.sh`：PASS（含多 receipt 中间成员命中与缺失成员负臂）；
- operation final status focused tests：PASS（任务级 complete + 未消费辅助 payload 不再矛盾；partial 仍警告）；
- `go test ./internal/repl -count=1`：PASS（33.966s）；
- `EVAL-B104-OPLIST1=implemented/full-package-pass`；
- `EVAL-B104-OPTASKCOV1=implemented/full-package-pass`；
- `EVAL-B104-TRACECLAIM1=model-fluctuation/replay-watch`。

## 109. 2026-08-05 B105 r31：operation 覆盖修复获正证；语义端点与源码端点的准入合同统一

### 109.1 严格双并发与人工结论

在 `main@ccef37f60` 构建冻结后，以 `PARALLEL=2` 严格并行恰好两个异构 case：

- `operation_web_manual_summary`：runner PASS / human PASS，135s；
- `mr_poly_binding_chain`：runner PASS / human FAIL，230s，5 个 Explorer midloop、3 次 Finalizer reject/patch，最大 context 21%。

工件见 `eval/parallel_selected_summary_evalcampaign_b105_operationpoly_r31_20260805.md` 及 manual audit。本轮未运行 Trace；以下改动不进入
RootCauseTrace、显式时间窗、双轴根因、因果投影或自动补齐，也不扫描用户/模型/答案 prose 作答案 hard gate。

### 109.2 B104 operation 修复获得生产正证

operation 最终 typed event 为 `status=complete + material_coverage_status=complete`，并绑定三个已记录的 receipt/source member；可见答案不再被全历史未消费辅助
payload 强插“材料覆盖未完全验证”。B104 的 task-level authority 与多值 runner 解码均关闭。模型在完整材料已到位后仍多做一次失败 regex 和 `ls/head`，但只有单 witness，
现有 prompt 已提供完整覆盖语义，暂按效率/模型波动观察，不增加命令或用户文本硬门。

### 109.3 `EVAL-B105-ENDPOINTDISC1`（P0）：语义端点被 request-verbatim 准入门迫使降级

跨语言题明确要求 `FastTokenizer.tokenize -> Rust 实现` 的完整调用链。Analyzer 通过当前 checkout 的 typed pre-scan 已解析出具体 sink
`tokenize_bytes`，先后两次正确尝试 `question_kind=call_chain`；旧归一化器却要求 source/sink 都逐字出现在用户输入，把 discovered sink 当成无权端点硬拒。
模型最终降级为 `mechanism`，导致仅在 QFCallChain 激活的全语言 call-edge evidence guide 未进入 Explorer 上下文。Explorer 虽读到
`core-rs/src/lib.rs:42` 的 wrapper→core 与 `:13` 的 core→`best_merge`，却没有发射同向 typed call rows；图门正确拒绝无证箭头，最终图只剩
Python→FFI，并错误注明 Rust 内部“无法从源码逐行追踪”。正文/列表较完整不能弥补图层与源码相矛盾，人工判 FAIL。

### 109.4 通用修复：方向意图、调查端点、事实证明三层分权

根修没有放松 diagram/call-edge hard gate，而是统一 Analyzer 的互相冲突合同：

1. 当前请求决定 `source -> sink` 的语义方向；`call_chain_endpoints` 仍是唯一有序载体，无序 entity/exact-target 不获方向权；
2. 当请求用语言、层级或角色描述一端（如“Rust 实现”）时，current-checkout typed pre-scan 可把它具体化为结构合法的代码 identity，作为调查目标保留；不再用原始请求逐字成员关系作硬拒；
3. 未逐字出现的 concrete endpoint 产生 soft provenance warning，不能证明符号存在、可达或任一调用边；所有最终 endpoint/path claim 仍必须由 grounded definition/call evidence fail-closed；
4. free-form 角色文本不能直接成为 code endpoint；源码路径、空端、同端点和非 code identity 仍在结构层拒绝；
5. current-question primacy 仍禁止历史/记忆抬升，只为“当前请求语义角色 → 当前 checkout 具体符号”增加窄例外，消除同一 prompt 中“必须调用链”与“所有字段逐字来自请求”的矛盾。

该方案适用于 Go、Java/Kotlin、JavaScript/TypeScript/ArkTS、C/C++、Rust、Python、Ruby、Swift、Lua、Cangjie 等既有通用 call-edge guide，
没有按 pyo3、`tokenize_bytes` 或某一语言硬编码，也不修改模型最终答案。

### 109.5 验证与状态

- 新增语义 sink `Rust 实现 -> tokenize_bytes` 的 production-shaped Analyzer admission pin；
- 新增 endpoint 结构负臂、discovered endpoint soft-warning、current-question primacy 窄例外 pin；
- 既有全语言 Explorer call-edge guide 测试通过；
- `go test ./internal/types ./internal/skill ./internal/agent ./internal/tool -count=1`：PASS（`internal/tool` 161.223s）。

状态：`EVAL-B105-ENDPOINTDISC1=implemented/full-relevant-pass/replay-next`；
`EVAL-B104-OPLIST1=closed/production-replay`；`EVAL-B104-OPTASKCOV1=closed/production-replay`。

## 110. 2026-08-05 B106 r32：端点准入获正证；调用边载体在稀疏发射、纠正合并和同名 owner 三处断裂

### 110.1 严格双并发与人工结论

在 `main@0b2d13fc3` 构建冻结后，以 `PARALLEL=2` 严格并行恰好两个异构 read case：

- `mr_poly_binding_chain`：runner FAIL / human FAIL，481s，19 个 Explorer iteration、6 次 Finalizer reject、5 次 patch，最大 context 23%；
- `sr_ts_workspace_chain`：runner PASS / human FAIL，141s，最大 context 20%。

工件见 `eval/parallel_selected_summary_evalcampaign_b106_endpointpoly_replay_r32_20260805.md` 及 manual audit。B105 的端点准入修复获得生产正证：
polyglot case 保持 `question_kind=call_chain`，语义 sink `Rust 实现` 具体化为 `tokenize_bytes` 后仅产生 soft provenance warning，全语言 call-edge guide 也已进入
Explorer 上下文。失败已下移到调用证据的有向载体层，而不是端点 admission。

TS runner 的字符串 oracle 是假绿：答案正确解释 alias 与重试参数，却把不含完整 call-site hop 的四行称为“完整调用链”，遗漏
`ApiClient.fetchUser -> HttpTransport.send -> dispatchOnce/fetch` 的精确末端，且为两级调用复用了同一个 CLI cite。Analyzer 第一次 call-chain 发射缺 endpoint pair 后由模型降级
为 mechanism；这里只有一个 replay witness。不能从无序 entity 猜方向，也不增加用户/答案原文扫描 hard gate，先在调用载体修复后继续异构回放。

### 110.2 `EVAL-B106-CALLEDGEID1`（P0）：同一精确调用事实没有跨过三段载体合同

polyglot Explorer 已读取三个精确调用点，但首批 relationship 行只给出 `anchor_kind=call + object`，省略 subject/predicate。wire decoder 用
`predicate=relationship` 兼容占位；旧 `normalizeCallEvidenceDirection` 仅识别空 predicate 或显式 call-like predicate，因此没有从 parser/read-line 关系补齐调用方。
运行上下文最终出现自相矛盾的 `grounded_callsite_facts=3; explicit_caller_callee_edges=0`。

模型根据完成门提示，随后在同一 source/line 重发正确 `subject/predicate/object`；但 duplicate 判定只比较 anchor/grounding 等外围字段，忽略 claim carrier，纠正项被当
exact duplicate 丢弃。完成器在连续低增量后接受了与源码相反的 `no_directed_path` waiver。Finalizer 的调用图 hard gate 连续六次正确拒绝无证 guard/FFI/call 箭头，最终降级稿却反转
Rust core 与 PyO3 wrapper 的角色，并把 `super::tokenize_bytes` 写成递归。hard gate 不应放宽；上游 typed edge 才是根因。

即使 subject 能进入 evidence，旧图构造也只消费模型短名。Rust wrapper 与 core 都叫 `tokenize_bytes` 时会折叠为自环，而 grounding 后系统已在
`OwnerSymbol` 保存 `py::tokenize_bytes` / module-qualified caller。稀疏铸造、纠正合并、图节点身份是同一个“精确调用事实的有向载体连续性”问题。

### 110.3 通用修复：精确信号补齐载体，不替模型写答案

1. 仅对 `EvidenceRelationship + AnchorCall` 且 predicate 为空或 decoder 占位 `relationship` 的行，在现有 parser graph/read-line 能解析唯一调用关系时补齐
   `predicate=calls`、限定 caller、callee 与 call anchor；显式非 call predicate、动态歧义和无解析行保持 fail-closed。
2. duplicate/amendment 判定把 kind/scope 与 subject/predicate/object/condition/snippet 作为同一 claim carrier；同一 source/line/token 的纠正不再被外围键吞掉，
   仍由既有 `MergeEvidenceItemByStableID` 完整度/grounding 规则决定是否替换。
3. call graph 优先使用系统 grounding 后铸造的 `OwnerSymbol` 作为 caller identity，旧证据无 owner 时才回退模型 `Subject`；同名 wrapper/core、类方法和包函数不再折叠。
4. 给 Finalizer 的只是 soft diagram guidance：FFI/JNI/PyO3/native/generated binding 未被精确 call row 证明时，sequence 用 `Note over` 表示边界，
   两侧独立已证调用仍保留；不得为好看制造箭头。所有语言共用同一规则，无语言/type 特判。

以上不扫描用户请求、模型输出或最终答案 prose 作 hard gate，不生成结论、不修改模型正文，也不放松调用图证据门。

### 110.4 验证与状态

- 新增 sparse grounded call 对 Python 类方法限定 owner、callee surface 与 `calls` predicate 的 production-shaped pin；
- 新增同 source/line directed-carrier correction 必须进入 amendment 的负回归；
- 新增 Rust/Python 同名 caller 通过 system-stamped owner 保持图节点分离的 graph pin；
- 新增全语言 binding boundary `Note over` soft-guide pin；
- focused 测试通过；`go test ./internal/types ./internal/agent ./internal/tool -count=1` 全部通过（`internal/tool` 160.842s）。

状态：`EVAL-B105-ENDPOINTDISC1=closed/production-admission-replay`；
`EVAL-B106-CALLEDGEID1=implemented/full-relevant-pass/replay-next`；
`EVAL-B106-TSCHAIN1=runner-false-green/model-replay-watch`。

本轮未运行 Trace，且未改 Trace 查询、显式时间窗、系统自动补齐、因果投影或双轴根因代码；不得用本批结果改变这些既有能力。

## 111. 2026-08-05 B107 r33：调用载体回放改善；生产限定 anchor 与类参与者图门发生精确合同冲突

### 111.1 严格双并发与人工结论

在 `main@eba6ed68d` 构建冻结后，以 `PARALLEL=2` 严格并行恰好两个语言/身份形态不同的 read case：

- `mr_poly_binding_chain`：runner PASS / human PASS（事实层），191s，3 次 Finalizer reject/patch，最大 context 21%；
- `sr_java_call_chain`：runner PASS / human FAIL，411s，7 次 Finalizer reject/patch，最大 context 30%。

工件见 `eval/parallel_selected_summary_evalcampaign_b107_calledge_replay_r33_20260805.md` 及 manual audit。polyglot 不再出现 19 轮调用边补发、错误 no-path
waiver、降级答案或 Rust wrapper/core 角色反转；最终正文正确区分 Python facade、`_fastlex`、PyO3 wrapper、Rust core 与纯 Python fallback，并用
`Note over` 表示未由 source-level call row 证明的 binding 过渡。该案首批调用行本身已带 subject/predicate，故只能作为 B106 整体过程改善及非稀疏臂正证；sparse
自动补齐仍由 production-shaped test 保证，不能把单次耗时下降直接归功给某一提交。

### 111.2 `EVAL-B107-DIAGRAMOP1`（P0）：同一 typed call 的两个精确投影互相否定

Java Explorer 已发射并由 grounding 精确规范化五条调用：

1. `VisitController.create -> VisitService.schedule`；
2. `VisitService.schedule -> ClinicConfig.resolveMaxVisits`；
3. `VisitService.schedule -> VisitRepository.countOpenVisits`；
4. `VisitService.schedule -> VisitRepository.insert`；
5. `VisitRepository.insert -> AuditLog.record`。

第一稿 sequence 没有 `edge_anchors`，硬门拒绝正确。第二稿补齐五个 typed anchor，并以类 participant + exact message operation 表达；这正是 prompt 与已有测试声明的
合法形，硬门却把五条全部报 `call_edge_unproven`。根因是 production grounding 会把 `AnchorCall` 规范为限定 callee（例如
`AnchorSymbol=VisitService.schedule`），而类参与者 resolver 取得消息操作 `schedule` 后仍与完整 AnchorSymbol 字节比较。上下文展示层消费限定形，校验层只接受短形，构成
“同一精确事实同时被要求、被拒绝”的红线合同冲突。

模型随后围绕别名、participant 与 edge anchor 做无效重写，并三次把 patch array 串行化成字符串；第 7 次删除结构化图后才完成，系统又以“保留内容”附回最初模型图。
若第二稿被正确接受，后续六次 churn 不会发生。因此主因不是提高预算或弱化图门，而是统一 typed identity 投影。

### 111.3 通用修复与不变量

类/actor lane 现在从同一 grounded qualified `AnchorSymbol` 取精确 operation 尾部，与 sequence message operation 比较；caller/callee owner
兼容、同向 citable call row 和候选唯一性仍是前置条件。该变化：

- 不从用户请求、模型 prose、答案正文或自由文本 label 猜调用；
- 不接受 owner 不同但尾名相同的调用；
- 不放宽无消息、消息歧义、反向边、无 citable row 或重复 endpoint；
- 对 Go、Java/Kotlin、JS/TS/ArkTS、C/C++、Rust、Python、Ruby、Swift、Lua、Cangjie 等统一生效；
- 不修改模型答案，也不改变 runtime/Trace diagram authority。

新增 production-shaped pin：`OwnerSymbol=VisitController.create`、`Object/AnchorSymbol=VisitService.schedule`、类 participant
`VisitController -> VisitService`、消息 `schedule(...)` 必须通过；既有歧义、反向、跨语言矩阵负/正臂继续通过。

### 111.4 回放中另两项裁定

`EVAL-B107-ENDPOINTAMBIG1`（P1，next）：polyglot 的最终事实正确，但 typed handoff 只有 Python native/fallback 两条 call row；已经读到的 Rust line 42
wrapper→core 调用未发射。Analyzer 把语义终点“Rust 实现”具体化为裸 `tokenize_bytes`，同名 wrapper/core 使完成门可能在第一同尾节点提前结束。
下一批应从 endpoint 的 typed definition/owner identity 与调用图唯一解析入手，不能硬编码 PyO3、文件路径或函数名，也不能让系统替模型补结论。

`EVAL-B107-JAVACLAIM1`（watch）：Java 最终正文声称 `AuditLog.record` 写数据库，但源码只执行 `System.out.println`；最终上下文已有明确 stdout
grounded evidence，因此是单次模型越证波动。按红线不增加答案关键词 scanner、normalizer 或系统替写；保留异构 replay，若跨模型复现再研究结构化 claim→citation soft calibration。

### 111.5 验证与状态

- `TestDiagramCallEdgeEvidenceMismatches_ClassParticipantsResolveGroundingQualifiedAnchor`：新增并通过；
- class participant 唯一/歧义、duplicate carrier、direction 与全语言 inline identity focused 回归全部通过；
- `go test ./internal/tool -count=1`：PASS（158.969s）；
- `EVAL-B107-DIAGRAMOP1=implemented/full-package-pass/replay-next`；
- `EVAL-B107-ENDPOINTAMBIG1=confirmed/next-batch`；
- `EVAL-B107-JAVACLAIM1=model-fluctuation/replay-watch`。

本轮未运行 Trace，且没有改 Trace 查询、显式时间窗、系统自动补齐、因果投影或双轴根因代码。

## 112. 2026-08-05 B108/B109 r34：写证明桥接与数据引用域修复合同审计

### 112.1 严格双并发与人工结论

在 `main@1b8b0dd6d` 构建冻结后，以 `PARALLEL=2` 严格并行恰好两个异构 case：

- `github_issue_memoclaw_text_search_multirepo_ts`（write）：runner FAIL，205s；补丁与 `make check` 人工 PASS，但交付被判
  `unverified:proof_weak`；
- `data_multifile_reference_projection`（data）：runner/human FAIL，411s；曾多次得到期望 `17,0,5`，但贡献账本与对账域错误，18 轮后失败。

工件见 `eval/parallel_selected_summary_evalcampaign_b108_writedata_r34_20260805.md` 及 manual audit。本轮跨 write/data 暴露的是 typed
阶段衔接问题，不是增加答案关键词或为单一 fixture 改 oracle 可以解决的问题。

### 112.2 `EVAL-B108-WRITEPROOF1`（P0，B109 已施工）：弱证明没有进入补采队列

write 补丁正确完成 POST 路由、JSON body、limit 与可选 namespace，且静态项目检查通过。系统也正确把该检查标为 `source_static`，没有冒充运行时行为证明。
但 `BuildVerificationProofProfile` 会据“硬行为合同存在且 `TargetBehaviorPaths=0`”给出 `target_behavior_verification_missing`，
`BuildVerificationProofLedger` 却只消费 report、changed-path、confidence、patch-review 与 impact 行；若后几类没有另行产生合同义务，修复队列为空，控制器直接
`accept_unverified`。同一个 typed 缺口在评级层可见、调度层消失。

B109 的通用修复从 normalized plan/cumulative contract set 铸造缺失的 `behavior_contract` ledger obligation，并保留三条边界：

1. 仅硬 required 合同进入义务，soft/advisory 合同不制造追补；
2. `source_static` 继续不能满足行为证明；
3. coherent cumulative chain 已有 typed `target_behavior` capability 时不制造泛化缺口，精确 contract-ref probe 仍可经既有 resolver 关闭单项义务。

新增 profile→ledger pin 与 controller wiring pin，证明 source-static-only 场景会生成 verify-only
`verification_proof_followup`，携带合同 ref 与源码范围，而真实 target-behavior coverage 不残留缺口。系统只安排证据补采，不改补丁、不替模型宣称行为成立。

### 112.3 `EVAL-B108-DATAREF1`（P0，B110 待施工）：诊断要求重算，动作合同却禁止重算

data workflow 的贡献账本为 `T1=10,T1=7,T2=0,T3=5`：它在贡献阶段提前进入 target domain，丢掉源 GroupB=4，并把目标缺席槽 T2=0
伪造成源贡献。正确链应先保留 GroupA=10/7、GroupB=4、GroupC=5，再 reconciliation 得到 17/4/5，最后按目标 GroupA/GroupX/GroupC
投影为 17/0/5。现有 reference-grounding validator 正确铸造 `reference_ledger_domain_mismatch=true`，但 output graph 永远只发
`assemble_answer`，guard 同时提示“必须重算贡献”和“不要更改贡献记录”。这是 typed action contract 自相矛盾的红线 gap。

B110 冻结方案：

1. 将 ledger-domain mismatch 与普通 slot/cardinality mismatch 分车道；前者重开 `compute_contributions`，后者仍只需 assemble；
2. domain mismatch 发 typed `compute_contributions -> reconcile_artifacts -> assemble_answer` repair sequence；
3. compute 支持精确参数 `replace_contributions=true`，并在 durable action 记录上形成 contribution generation boundary；跨轮 seed 遇到该边界清除旧贡献，避免新旧域并存；
4. 由 latest typed successful-progress boundary 退休更早的 derived field-contract issue，保留边界本身及之后的新 issue；
5. completion gate、REPL guard、allowed-action contract 与 planner guidance 同源，不扫描用户或模型 prose。

在矛盾合同消除前不以提升 18-round 预算掩盖问题。

### 112.4 状态与不变量

- `EVAL-B108-WRITEPROOF1=implemented/full-relevant-pass`（`go test ./internal/types ./internal/orchestrator -count=1`）；
- `EVAL-B108-DATAREF1=confirmed/design-frozen/B110-next`；
- `EVAL-B108-DATASTALE1=confirmed/B110-same-root`；
- `EVAL-B107-ENDPOINTAMBIG1=design-required/deferred`：当前只有同尾名多 definition，缺少无歧义 typed endpoint authority，不能据此硬选。

本批未修改 Trace、显式时间窗、因果投影、自动补齐或双轴根因；没有读取用户/模型/最终答案原文作 hard gate。

## 113. 2026-08-05 B110：数据引用域对齐车道、贡献代次与陈腐诊断闭环

### 113.1 冷读修正：零域交集不能硬判只有贡献侧错误

`reference_ledger_domain_mismatch` 是精确结构信号：reference keys 与 numeric contribution group keys 零交集。它足以拒绝直接发布或只做
assemble，却不能单独判定“贡献键错”还是“typed reference_key_field 声明错”。因此最终方案没有把系统硬化为自动重算某一侧，而是新增
`repair_reference_domain_alignment` stage：

- output projection graph 同时公布 `compute_contributions`、`reconcile_artifacts`、`assemble_answer` 三类 producer；
- allowed-action contract 取 contribution preparation/recompute、reconcile、projection 的受控并集；
- guard 的主 repair action 为 compute，且明确允许先检查并纠正 typed reference declaration；普通 cardinality/slot mismatch 仍保持 assemble-only；
- 不生成最终值、不替模型选择哪一侧权威，模型仍基于 artifact schema、reference contract 与业务规则决定对齐方向。

这样既消除了“诊断要求重算、动作面却禁止重算”的矛盾，也没有把一个只能证明不一致的信号越权升级为单边结论。

### 113.2 可追溯贡献代次：`replace_contributions=true`

当模型根据 typed 域错配与材料证据选择重算贡献时，普通 append/dedupe 会让 T-id 旧行与 canonical-label 新行并存。B110 为
`compute_contributions` 增加严格布尔参数 `replace_contributions`：

1. action 成功后才切换代次；失败不会销毁旧审计证据；
2. 清除当前 seed 的旧 contribution ledger、旧 reconcile 和精确由旧贡献派生的 decision rows，保留独立资格判断行；
3. 新 contribution artifact 标记 `contribution_generation=replace`；
4. 跨 workflow round 的 seed 读取 durable plan action 参数作为代次边界，清除更早贡献/对账/答案代次，再接受该 result 的累计非贡献事实；
5. 普通 compute 默认仍为 additive，非法布尔值 fail-closed，避免静默忽略修复意图。

该机制按贡献账本这一类状态工作，不绑定 targets.csv、canonical_label、T1 或某个 case。

### 113.3 `EVAL-B108-DATASTALE1`：派生字段诊断按 typed 成功边界退休

`DiscoverFieldContractIssues` 原先逆序扫描全历史，早期失败在后续成功后仍持续进入 planner context。现在与 guard/execution violation 的生命周期原则一致，
但采用“最新成功记录 inclusive”边界：成功记录之前的失败退休；成功记录自身若携带 zero-match artifact，仍作为当前问题保留。没有从错误文本判断“是否修好”。

### 113.4 验证与状态

- B108 同形 pin：reference=`canonical_label`、贡献=`T1/T2/T3`、答案虽为 `17,0,5`，仍进入域对齐 stage，且不再出现“without changing contribution records”；
- 反向域错配 pin：reference=`target_id`、贡献=`GroupA/B/C` 同样保留双向 typed escape，证明没有拟合字段名；
- 普通 GroupX 槽位错值继续 assemble-only；
- replacement 本轮与跨轮代次、旧 reconcile/derived-row 清除、非法参数负臂通过；
- 陈腐 failure 退休与 latest-success zero-match 保留双臂通过；
- `go test ./internal/dataworkflow ./internal/dataquery ./internal/repl -count=1`：PASS（dataquery LOC ratchet 通过，新 concern 已拆到 sibling file）。

状态：`EVAL-B108-DATAREF1=implemented/full-relevant-pass/replay-next`；
`EVAL-B108-DATASTALE1=implemented/full-relevant-pass/replay-next`；
`EVAL-B108-WRITEPROOF1=implemented/full-relevant-pass/replay-next`。

本批没有改 Trace 路径，也没有扫描用户请求、模型过程或最终答案原文作 hard gate；系统只公开 typed 不一致、合法修复动作和账本代次，结论仍由模型给出。

## 114. 2026-08-05 B111/B112 r35：write/data 回放复绿与未知动作入口合同

### 114.1 严格双并发与人工结论

在 `main@87e944613` 构建冻结后，以 `PARALLEL=2` 严格并行恰好两个异构 case：

- `github_issue_memoclaw_text_search_multirepo_ts`（write）：runner/human PASS，179s；
- `data_multifile_reference_projection`（data）：runner/human PASS，220s，9 个 data round、2 个 repair round。

工件见 `eval/parallel_selected_summary_evalcampaign_b111_writedata_replay_r35_20260805.md` 及 manual audit。write 补丁正确改为
`POST /v1/search`、JSON body、limit 与可选 namespace，保持无关 API/测试不变，`make check` 通过，交付判定为 verified。

但该次 planner 把四个 outcome 都发为 soft `operator=satisfies`、`source=expected_outcome_fallback`，没有形成 hard behavior contract，故 B109 的
source-static→target-behavior 补采桥没有在生产回放中触发。本次只能证明写任务整体复绿，B109 仍由 profile→ledger 与 controller wiring 的结构 pin 保证；不能为了触发它而
扫描请求/模型 prose 并硬升格合同。该现象先作为 contract-strength 异构 replay watch。

### 114.2 数据账本已恢复，普通输出修复与引用域修复保持分车道

最终 contribution ledger 保留源域：`GroupA=10/7, GroupB=4, GroupC=5`；reconcile 为 `17/4/5`；按目标 reference 投影后的最终答案为
`17/0/5`。本轮一开始已使用正确源域，因此 B110 的 `replace_contributions=true` 代次臂没有生产触发，仍由结构测试保证。

本轮出现的是另一类普通输出错：模型先把三项用分号拼成一个字符串，typed cardinality/grounding 检查拒绝后只重跑 `assemble_answer` 并成功。这是正向证明：普通
slot/cardinality mismatch 没有被错误扩大为贡献重算，和 reference-domain mismatch 保持独立。

模型中间思考曾杜撰错误 fixture 数字及 `GroupY`，但这些内容没有进入 contribution/reconcile/final typed artifacts。系统账本成功隔离了模型波动；不据此新增答案原文
scanner、normalizer 或系统代写结论。

### 114.3 `EVAL-B111-ACTIONKIND1`（P1，B112 已施工）：未知 typed action 没有在入口被拒绝

首个数据计划同时包含未知 `read_instructions` 与缺 input 的 `derive_fields`。旧 staging 先执行 batch/stage/dependency/shape 检查，报告了后者；下一轮模型只保留
`read_instructions` 后 staging 又放行，直到 runner 才以 unsupported kind 失败。未知 action 不可能成功执行，晚拒绝且被次生错误遮蔽白烧了一轮。

B112 将 action capability registry 作为单一 action-kind authority，并在所有 batch、stage、dependency、field、shape 与 execution 检查之前逐项 admission：

1. `Action.Kind` 是 schema typed enum，未知值可安全 hard-reject，不读取用户或模型自然语言；
2. violation 保留精确 action ID/kind/idempotency，并优先给出当前 `workflow_state.allowed_next_actions`；若当前集合为空才从同一 capability registry 派生完整支持列表；
3. 空 kind 的既有 legacy custom-transform 兼容形与所有已注册 typed action 保持不变；
4. 新 pin 证明第一项未知 action 会先于第二项缺输入错误被报告，且不会进入执行器。

### 114.4 验证、状态与不变量

- `go test ./internal/dataworkflow ./internal/repl -count=1`：PASS；
- `EVAL-B108-WRITEPROOF1=implemented/structural-pin/production-arm-replay-watch`；
- `EVAL-B108-DATAREF1=implemented/human-pass`；
- `EVAL-B108-DATASTALE1=implemented/human-pass`；
- `EVAL-B111-ACTIONKIND1=implemented/full-relevant-pass/replay-next`。

本批没有改 Trace 查询、显式时间窗、因果投影、自动补齐或双轴根因；没有按单个 action 名硬编码修复，也没有扫描用户请求、模型过程或最终答案原文作 hard gate。

## 115. 2026-08-05 B113/B114 r36：Trace 决策尾界、runtime 引用合同与调用末端行为

### 115.1 严格双并发与人工结论

在 `main@bb4f676df` 构建冻结后，以 `PARALLEL=2` 严格并行恰好两个异构 case：

- `trace_query_donghu_real_frame_multicausal`：runner PASS，208s；human FAIL；
- `sr_java_call_chain`：runner PASS，138s；human FAIL。

工件见 `eval/parallel_selected_summary_evalcampaign_b113_tracejava_replay_r36_20260805.md` 及 manual audit。两案 runner 都只覆盖声明式 oracle，
没有捕获最终语义越证，故不能拿 2/2 PASS 代替人工正确性。

Java 的调用边、容量守卫、五条 invocation anchor 与 class-participant Mermaid 均正确，证明 B107 diagram operation 修复已进入生产路径；一次成文拒绝是
初稿漏 `edge_anchors` 后局部 patch 成功，不是合同矛盾。但源码 `AuditLog.record` 只有 `System.out.println`，答案仍写“写入磁盘/审计日志表”。
`EVAL-B107-JAVACLAIM1` 因第二次复现从 model-fluctuation watch 升为 confirmed context-salience gap。

Trace 的显式时间窗、状态五分账、根因席位、唤醒链、实际占用轴、既有规则可消除轴、覆盖边界和自动补齐完整存在；系统投影也正确保留
`frame_causality=unproven`，没有替写模型正文。但模型仍宣称“直接根因”，跨不同 ThreadPool row 组合 D-state/IO 数值，并把有向唤醒路径升级成
“双向强耦合”。这是 B104 同类第二个生产 witness，不能继续归为单次模型波动。

### 115.2 `EVAL-B113-TRACEDEC1`（P1，B114 已施工）：正确 typed 限界被后置通用提示稀释

Finalizer 已有 245KB 上下文与完整 `Trace Decision Inputs`，包含 `causal_conclusion=unproven`、`frame_evidence_status=absent`、
双轴、row-local breakdown、typed-pair-only relation、禁止无授权跨行相加与 wakeup path 语义。问题不在数据不足，而在这段之后仍追加主值、Checklist、
multi-topic 等结构提示，模型在真正发射前看到的最后边界不再是因果/关系 caliber。

B114 在动态上下文尾部追加小型 `Final Trace Decision Boundary (Typed Facts; Model-Owned Conclusion)`：

1. 只从最终 typed projection 与 evidence authority 读取 causal/frame ceiling 和两轴可用性；
2. 明确实际占用、现有规则可消除量、已证 frame causality 是三个不同 caliber，不能互相替代，也不据并存声称物理独立；
3. row-local breakdown 不跨 row，只有 exact typed relation/fold carrier 才能授权成员关系与加法；
4. wakeup/temporal/candidate/caller-symbol 不自动铸造同步阻塞、锁 owner、post-wakeup preemption 或物理耦合；
5. 明确模型拥有诊断、排序、优化方向和措辞。该段不创建 AnswerBlock、不读/拒绝/修改答案 prose。

显式窗口、系统补齐和因果投影继续复用同一 `RuntimeTraceReportMaterializationAllowed` authority；bounded fact request 仍不被扩大为完整因果综合。

### 115.3 `EVAL-B113-RUNTIMECITE1` / `TRACELOG1`（P1，B114 已施工）：同 prompt 引用冲突与日志串入 Trace 合同

runtime grounding disposition 已明确 artifact observation 不需要 current-repo citation，但 multi-topic 尾句无条件要求每节 citation，形成同一上下文的直接矛盾。
现改为：runtime-only 每节保留附件 provenance；只有独立 typed current-source anchor 直接支持该节时才加 repo citation。普通 source-backed multi-topic 保持原合同。

同时确认纯日志 ledger 也会编出空 projection partition，旧 `renderAnswerDocTraceDecisionHandoff` 因只看 projection 数量而给日志任务注入 Trace 决策合同。
B114 增加 typed runtime-trace source 准入；不扫描 artifact 名、用户原文或日志正文。日志与 Trace 的上下文职责重新隔离。

### 115.4 `EVAL-B107-JAVACLAIM1`（P1，B114 已施工待回放）：末端名称推断覆盖已读实现

系统已经给出 `AuditLog.java:5-6` definition/mechanism 证据，但最终模型只引用 caller line 并把 endpoint 名称扩写成数据库/磁盘行为。B114 对所有语言的
QFCallChain 在 prompt 尾部追加同一证据边界：call site 仅证明 caller→callee；callee body、side effect、storage medium、sync/completion semantics 必须
由独立 grounded endpoint implementation 支持并在需要时引用。若无 body proof，只能说链到达/调用 endpoint。

这是 soft evidence calibration，不按 Java、AuditLog、println、数据库等词做特例，也不以答案正文扫描 hard-reject 模型。

### 115.5 其余发现、验证与状态

- `EVAL-B113-DIAGRAMOP1=production-pass/closed`；
- `EVAL-B113-TRACEDEC1=implemented/full-agent-pass/replay-next`；
- `EVAL-B113-RUNTIMECITE1=implemented/full-agent-pass`；
- `EVAL-B113-TRACELOG1=implemented/full-agent-pass`；
- `EVAL-B107-JAVACLAIM1=implemented-soft-boundary/replay-next`；
- `EVAL-B113-TRACEALIASROW1=confirmed/P1-next`：同一物理 span 仍可能以 original/attached alias 出现在明细两次；
- `EVAL-B113-ANASYM1=confirmed-one-witness/P1-watch`：relation_map 已证 method symbol 与 analyzer subtopic census 的解析面不同源。

结构回归覆盖：Trace 尾界晚于通用 Checklist、bounded fact 不扩域、call-chain 尾界语言无关、runtime-only multi-topic 不要求伪 repo citation、纯日志不发
Trace 决策段；`go test ./internal/agent -count=1` PASS。

本批没有修改 Trace 查询、root rank、唤醒链、窗内可消除量、因果投影、系统补齐或最终答案持久化；系统仍只给模型精确事实与软引导，不接管模型结论。

## 116. 2026-08-05 B115/B116 r37：探索聚合权威越级与 typed Trace 单一事实面

### 116.1 严格双并发与人工结论

在 `main@ffb007885` 构建冻结后，以 `PARALLEL=2` 严格并行恰好两个异构 case：

- `trace_query_donghu_real_frame_multicausal`：runner PASS，193s，5 次 trace_query，finalizer reject=0；human FAIL；
- `mr_poly_binding_chain`：runner PASS，361s，finalizer reject/patch=9/9；human FAIL。

工件见 `eval/parallel_selected_summary_evalcampaign_b115_tracepoly_replay_r37_20260805.md` 及 manual audit。Trace 的显式窗、自动补齐、因果投影和
“实际耗时占用 / 现有规则可消除量”双轴完整存在，B114 尾界也确实位于 prompt 最后；失败不是这些能力消失，而是错误综合在更早阶段已被包装成结构化事实。

跨语言案的源调用与 fallback 大体正确，但 9 次 diagram 拒绝后才删除可选图；同一成员集机械补充在被拒 draft 上累计，最终仍显示两份六行清单。该案同时把
`EVAL-B107-ENDPOINTAMBIG1` 从一处观察升级为第二个生产 witness：owner-qualified method 与裸 operation 的 typed identity 没有统一，源码明确的
`FastTokenizer.tokenize -> tokenize_bytes` 被 validator 报成 missing/unproven。

### 116.2 `EVAL-B115-TRACEAGGAUTH1`（P1，B116 已施工）：模型探索综合不能与 deterministic trace 行共享事实权威

Explorer 的 completion payload 把以下内容放进 `aggregate_facts`：四跳 wakeup path 被称为直接因果链，根因排行里把不同成员/口径的 D-state 与 io_wait
相加为 17.819ms。虽然这些 aggregate 已被降为 supporting/soft/historical，`Structured Aggregate Facts` 仍逐字展示它们；Finalizer 随后把它们当作事实复用，
压过正确的 typed relation/addition boundary。

B116 在 `AnswerSurfacePlan` 的 runtime-trace authority 编译点加入单一投影规则：

1. 仅当 ledger 有 deterministic runtime query、可编出非空 Trace projection，且 `RuntimeTraceReportMaterializationAllowed` 允许完整报告时生效；
2. model-authored aggregate 若 typed origin 只有 `runtime_artifact` / `system_inference`，不进入最终事实 handoff、claim binding、成员硬合同或 pre-emit 完整性面；
3. 原始 aggregate 仍留在 Mutable/TurnAArtifacts，供审计复盘，不删除历史；
4. 混合 trace+source 场景中带独立 current-source 或其他非 runtime origin 的事实继续保留；
5. bounded fact-set 查询不扩成完整投影，也不受该投影规则影响。

该方案不解析 aggregate label/member/reason 或最终答案文字，不判断“17.819”是否正确，也不替模型生成结论；它只阻止未绑定 typed trace 凭证的模型综合冒充
系统事实。Finalizer 仍从 trace_query 投影、系统补齐和关系权限自行给出诊断与建议。

### 116.3 剩余任务排序

1. `EVAL-B115-ENUMACCUM1`（P1）：拒绝 draft 的系统成员补充必须幂等，且结构化模型清单已覆盖 accepted member set 时不得再发同集补充；
2. `EVAL-B115-DIAGRAMCHURN1`（P1）：相同 optional-diagram violation 无进展重复时，给模型 typed 终止/移除建议并允许重发，避免 9 轮 patch-first；
3. `EVAL-B107-ENDPOINTAMBIG1`（P1）：统一 owner-qualified endpoint / bare operation / participant message identity，覆盖所有语言和 FFI 边界；
4. `EVAL-B113-TRACEALIASROW1`（P1）：继续核查同一物理 span 的 attachment/original alias 行归并，未以本轮现象误关账。

### 116.4 验证、状态与不变量

- 新 pin：完整 typed Trace report 隐去 runtime-only model aggregate，但原始 Mutable 审计记录仍在；
- 反向 pin：bounded fact-set 保留 aggregate；无 deterministic trace row 的日志/runtime 场景保留 aggregate；混合 current-source 事实保留；
- `go test ./internal/types ./internal/agent -count=1`：PASS；
- `EVAL-B115-TRACEAGGAUTH1=implemented/full-relevant-pass/replay-next`；
- `EVAL-B115-ENUMACCUM1=confirmed/B117-next`；`EVAL-B115-DIAGRAMCHURN1=confirmed/B117-or-B118`；
- `EVAL-B107-ENDPOINTAMBIG1=confirmed-second-witness/P1`。

本批未改 trace_query 计算、显式时间窗、根因排序、唤醒链、窗内可消除量、因果投影或自动补齐；没有扫描用户请求、模型过程或最终答案原文作 hard gate，也没有
删除或重写模型答案。改动只收窄模型探索综合在最终事实合同中的权威级别。

## 117. 2026-08-05 B117：成员集补齐幂等与可选图失败的模型所有权恢复

### 117.1 `EVAL-B115-ENUMACCUM1`：完整模型清单不应为系统标签再复制一份

B115 的六个跨语言节点并没有缺项。旧 `normalizeAggregateMemberSetCarriers` 已能证明一个模型自有 structured block 完整覆盖 accepted member set，
但 relation-label 发布分支只检查该 block 是否重复 aggregate category 标题；标题未重复时，系统仍追加“标签 + 全量六成员”载体。被拒 patch 每轮都在上次
draft 上再次运行该补齐，虽然持久化前会去掉系统副本之间的重复，最终仍保留“一份模型清单 + 一份系统清单”。

B117 将单一 relation member-set 的标签披露与成员补齐分开：

1. 若 `preEmitPrimaryMemberCarrierIndex` 已用 typed member identity 证明恰有一个完整、结构化、模型自有主载体，不再为了重复 category label 追加第二份成员清单；
2. 多个 relation member-set、成员散落多个 block、非主载体或身份有歧义时，继续保留原有 typed 标签消歧补齐；
3. 不比较自然语言标题相似度，不读取用户请求，也不扫描最终答案 prose；完整性仍由 accepted members 与 structured block item identity 决定；
4. 原清单字节与模型顺序保持不变，系统不合并、重写或替换模型内容。

回归覆盖 B115 的六成员原形、普通 ordered-list 与 markdown-table sibling carrier；完整单载体均不再追加副本，歧义/分散场景仍走原补齐合同。

### 117.2 `EVAL-B115-DIAGRAMCHURN1`：区分必需图与可选图，停止无证边的别名试错

连续 9 次成文失败不是“同一声明既必带又必拒”的合同矛盾：该 case 的图是可选的，删除图一直是合法解；问题是 patch reject 的通用恢复提示只说继续修当前
block，没有把 typed 失败原因和图的 required/optional 合同组合起来。模型因此反复改 participant/operation 别名，希望绕过同一
`diagram_call_edge_unproven`，直到第 9 次才自行使用 `remove_block_ids`。

B117 增加一个严格受限的恢复提示车道：

1. 只接受 `ToolRepair.Code=answer_doc_pre_emit_contract` 且 `violation_kinds` 精确只有
   `diagram_call_edge_unproven`；混合 citation、coverage 等其他失败不能进入；
2. 只在初始化时由 typed `DiagramContract.Required=false` 证明图非必需时生效；必需图继续走普通修复车道，绝不建议删除；
3. 提示模型两条诚实路径：只保留已有 typed call-edge 授权的边，或用 `remove_block_ids` 删除可选图并保留 grounded 文字调用链；
4. 系统不执行删除、不重写 diagram、不恢复 rejected draft 为成功答案，最终 presentation 选择仍归模型；
5. 提示不解析 ToolRepair.Hint/Summary 的自然语言来选择车道，不扫描用户问题、模型思考或最终答案。

这不是降低调用边证据门。`diagram_call_edge_unproven` 仍然 hard reject；修复的是 reject 后的模型可行动信息与终止路径，避免把预算消耗在无 authority 的端点换名上。

### 117.3 验证、状态与后续

- `go test ./internal/agent ./internal/tool -count=1`：PASS（tool 160.679s）；
- `EVAL-B115-ENUMACCUM1=implemented/full-relevant-pass/replay-next`；
- `EVAL-B115-DIAGRAMCHURN1=implemented/full-relevant-pass/replay-next`；
- `EVAL-B107-ENDPOINTAMBIG1=confirmed-second-witness/P1-next`：本批不放宽边权威，后续统一 owner-qualified endpoint / bare operation / participant message identity；
- 下一批严格双并发回放 `trace_query_donghu_real_frame_multicausal` 与 `mr_poly_binding_chain`，同时核查 finalizer 上下文、补齐幂等、reject 次数和人工答案。

本批未触碰 Trace 查询、显式时间窗、根因排序、唤醒链、窗内可消除量、因果投影、自动补齐准入或双轴根因；没有用原始文本关键词作 hard gate，也没有让系统接管模型结论。

## 118. 2026-08-05 B117 r38 / B118：双回放与显式删图的恢复边界

### 118.1 严格双并发结果：runner 2/2 PASS，人工 0/2 PASS

在 `main@364f9730c` 构建冻结后，以 `PARALLEL=2` 严格并行恰好两个 case：

- `trace_query_donghu_real_frame_multicausal`：184s，finalizer reject=0，runner PASS / human FAIL；
- `mr_poly_binding_chain`：226s，finalizer reject=2，runner PASS / human FAIL。

工件见 `eval/parallel_selected_summary_evalcampaign_b117_tracepoly_replay_r38_20260805.md` 及 manual audit。

B117 的两个直接目标均获得生产正证：多语言答案不再出现第二份完整成员清单；optional diagram 的拒绝从 9 次降为 2 次，第二次 typed 提示后模型立即选择
`remove_block_ids=["d1"]`，没有继续换名试错。调用边 hard authority 未降低。

Trace 侧也证明 B116 事实权限收窄已生效：Finalizer prompt 中不再出现 explorer 的 `Structured Aggregate Facts`；最后一段 typed boundary 明确披露
`causal_conclusion=unproven`、frame evidence absent、跨 row 不可加、wakeup path 不证明 blocking。显式窗、因果投影、自动补齐和实际占用/规则可消除双轴均完整。

### 118.2 `EVAL-B117-ATTACHREMOVE1`（P0，B118 已施工）：模型删掉的 rejected 图不能被恢复层复活

多语言案最终接受 patch 已明确删除 `d1`，但旧 attachment filter 只按最终 document 判断：document 无 diagram 时，rejected draft 的 model diagram 允许恢复。
于是 hard gate 判无证的图在模型主动删除后，又以“系统保留内容”进入最终答案。这同时违反证据门和模型 presentation 所有权。

B118 将恢复过滤补上 mutation-aware typed 边界：

1. 仅 `MutationPartial` 且 `remove_block_ids` 精确命中 prev document 中 `kind=diagram` 的 block 时，认定模型显式放弃图；
2. persist 成功后删除所有非 system-authored rejected-model diagram attachments，避免失败的 persist 提前销毁审计证据；
3. system-authored cross-check diagram 保留独立 authority；删除普通 section/list 不影响 diagram recovery；replace-all 没有 typed 删除意图，沿用既有恢复规则；
4. `ApplyAndPersistMutation` 与 patch pre-validation 后的直接 persist 两条车道共用同一判定；不读取图 body、模型思考、用户问题或最终 prose。

这是服从模型的显式结构化删除，不是系统替模型删除答案。模型未删除、最终文档又无图的真正 recovery 场景仍保留可见附件能力。

### 118.3 `EVAL-B117-TRACEDECEXEC1`（P1）：错误入口已关闭，但决策载体仍不够可执行

本轮 Trace 模型仍将 10.433ms D-state、7.386ms io_wait、6.673ms io_latency 跨 row 相加为 24.5ms，并把 CookieMonsterCl 的 typed wakeup/PI candidate
写成“直接阻塞原因”；同时以 wakeup 往来推断“紧密交互、进一步延长链路”。因旧 aggregate 已消失且尾界精确存在，这不是 B115 的残留，也不能再靠重复同一句软禁令解决。

下一批最优方向冻结为 typed decision ledger，而不是答案文本 hard gate：

- 从 projection 中单独发布 `target_direct_blocking_authority`：只有目标 waiter/holder 的 exact typed blocking relation 才能命名直接 blocker；否则明确 unproven，wakeup path 单列；
- 每个 fix-direction 只发布本方向最高的单席 leader/ceiling 与 row identity；没有 exact fold authority 时显式 `direction_fold=not_authorized`，避免模型从长明细自行造 subtotal；
- 保留 actual occupancy 与 existing-rule eliminable 两轴，且模型继续拥有诊断、排序解释、优化方向和措辞；
- 不扫描最终答案作拒绝，不由系统生成或替换结论。

### 118.4 状态

- `EVAL-B115-TRACEAGGAUTH1=production-covered`；
- `EVAL-B115-ENUMACCUM1=production-covered`；
- `EVAL-B115-DIAGRAMCHURN1=production-covered`；
- `go test ./internal/tool -count=1`：PASS（157.895s）；
- `EVAL-B117-ATTACHREMOVE1=implemented/full-tool-pass/replay-next`；
- `EVAL-B117-TRACEDECEXEC1=confirmed-third-witness/B119-next`；
- `EVAL-B107-ENDPOINTAMBIG1=confirmed/P1-after-trace-ledger`。

B118 未改 Trace 查询、显式时间窗、因果投影或系统补齐；没有降低 diagram call-edge hard gate，没有扫描自然语言作 hard gate，也没有系统代写模型答案。

## 119. 2026-08-05 B119：Trace 尾部 typed 决策账本

### 119.1 目标直接阻塞权限与 wakeup path 分面

r38 证明一般性禁令存在却仍可能被长上下文稀释。B119 不新增答案扫描器或 hard reject，而是在最终 prompt 最后一段从同一
`TraceCausalProjectionSet` 计算紧凑 authority ledger：

- 有 typed target state 时，只有 projection node 的 `BlockingKind` 非空，且 target 精确处于 waiter 角色（waiter-subject row 的 Subject，或
  holder-subject row 的 BlockingPeer），才发布 `target_direct_blocking_authority=typed_waiter_holder`；
- 同时发布 exact waiter、holder、blocking kind 与 row identity；holder 未解析时保持 `unresolved`，不从 wakeup path 猜 peer；
- 没有精确 target blocking row 时发布 `not_provided_by_projection`，并单列 `wakeup_path_blocking_authority=not_implied`；
- 缺 typed target 时为 `unavailable_without_typed_target`，不会用第一个线程或用户名猜测目标。

该账本只限制模型可以从哪些 typed 关系推理“直接 blocker”；模型仍决定如何解释窗口、排序候选和给出验证建议。

### 119.2 每个修复方向只发布单席 leader，不铸造 subtotal

从已过滤的 positive ranked eliminable seats 按发布 rank 取每个 exact `FixDirection` 的第一个席位，尾部列出 direction、rank、subject、
effective attribution 与 row identity；同方向其余席位不作为 subtotal member 重复展开。同时声明：

- `fix_direction_summary_authority=single_published_leader_only`；
- `direction_subtotal_authority=not_provided_without_exact_fold`；
- 相同 direction label 本身不授权相加。

因此系统没有替模型算“IO 方向总收益”，也没有把 leader 宣布为物理根因；它只给出当前 typed 排名中可安全陈述的单席代表，避免模型从大量不同 row/口径拼接
24.5ms 之类伪小计。未来若 projection 提供 exact fold authority，可另行发布该 fold，不需要改答案文本规则。

### 119.3 回归、不变量与状态

- 正例：waiter-subject 与 holder-subject 两种 blocking 角色都保持正确方向；无 target match 的 holder row 不越权进入；
- 无 blocking row + 有 wakeup path：只发布 wakeup 非阻塞权限；
- 同方向两个席位只发布 rank 更高 leader，另一个方向独立保留；
- bounded fact-set 仍不产生 Final Trace Decision Boundary；
- explorer runtime aggregate 仍不回流 Finalizer；
- `go test ./internal/agent -count=1`：PASS（2.446s；新增定向补测 PASS）。

状态：`EVAL-B117-TRACEDECEXEC1=implemented/full-agent-pass/replay-next`；
`EVAL-B117-ATTACHREMOVE1=implemented/full-tool-pass/replay-next`；
`EVAL-B107-ENDPOINTAMBIG1=confirmed/P1`。

B119 是 prompt-only typed context 改进：未修改 trace_query、根因排序、唤醒链、窗内可消除量、因果投影、显式窗准入或系统补齐；不扫描问题/思考/答案文本，
不拒绝、不删除、不改写模型结论。

## 120. 2026-08-05 B119 r39 / B120：显式用户窗权威与跨窗决策隔离

### 120.1 严格双并发结果：runner 2/2 PASS，人工 0/2 PASS

在 `main@82153e3d1` 构建冻结后，以 `PARALLEL=2` 严格并行恰好两个 case：

- `trace_query_donghu_real_frame_multicausal`：196s，5 次 trace_query，finalizer reject=0，runner PASS / human FAIL；
- `mr_poly_binding_chain`：150s，finalizer reject=3，runner PASS / human FAIL。

工件见 `eval/parallel_selected_summary_evalcampaign_b119_tracepoly_replay_r39_20260805.md` 及 manual audit。

多语言案确认 B117/B118 已生产闭环：完整成员清单不再重复；模型使用 typed `remove_block_ids` 删除可选图后，rejected diagram 没有被系统附件恢复。人工失败仍在
跨语言/FFI endpoint identity：答案把 PyO3 wrapper 与 Rust core 合并成一个节点，遗漏 wrapper 调用 `super::tokenize_bytes` 的落点与独立引用；这是
`EVAL-B107-ENDPOINTAMBIG1` 的第三个 witness，不应通过放宽 diagram edge hard gate 修复。

Trace 案确认 B119 的同方向防加法有效：模型不再把 10.433/7.386/6.673ms 相加为 24.5ms，而是保留独立行并取单席 leader。直接 blocker 权限也已明确
发布为 `not_provided_by_projection`。但答案仍把 pre-wakeup dependency 描述成目标线程“唤醒后无法获得 CPU”，说明长上下文中的相位权威仍需在最终紧凑席携带。

### 120.2 `EVAL-B119-REQWIN1`（P0，B120 已施工）：已补齐的用户全窗不能被模型子窗夺权

日志证明自动补齐并未缺席：系统在 explore→extract 边界正确补跑 `frame_root_cause_bundle`，窗口精确为用户指定的
`34579.472865..34579.587805`（114.94ms），并产生完整全窗 root-rank、wakeup-chain 与 target-state account。错误发生在投影编译：

1. 较早的模型 50ms 查询产生 `frame_target_resolution(window_source=query_window)`；旧 anchor 规则让任意 frame anchor 绝对压过之后的 root-rank fallback；
2. projection 因而把 50ms 当“用户原始窗”，只挂接 50ms target state；全窗补齐行虽进入 root-cause 桶，却拿不到主窗分母；
3. wakeup path 的实体选举只按 subject/depth/发布序，不读取 typed 用户窗；同目标的 50ms path 继续成为主干；
4. 最终答案于是同页混用 114.94ms 标题、50ms sleep=34.307ms 和全窗 effective seats，甚至声称“50ms 子窗口全窗覆盖完整”。

B120 把 analyzer 已验证的 `RuntimeArtifactScopeProfile` 作为独立 typed 值带入 `ObservationLedger`，并在两个位置统一执行“精确匹配才提权”：

- 主窗 anchor：`explicit_time_window` 仅在 deterministic trace causal anchor family 存在相同 endpoints 时优先；请求意图本身不能虚构 coverage；
- wakeup-path election：若存在与用户窗精确同端点的 path candidate，先收窄到该窗，再沿原实体/深度/发布序规则选举；若不存在，旧有 bounded 证据原样保留；
- target state 随主窗用既有 same-window admission 自动挂接全窗 account，无第二套状态计算。

这不是禁用子窗。50ms micro-probe 仍完整保留在 observation ledger、query-window 清册和证据索引中，只是不再冒充用户请求主窗。

### 120.3 `EVAL-B119-MULTIWINLEDGER1`（P1，B120 已施工）：高显著决策席不能跨 query window 混池

B119 的 compact leader 从所有 rank boards 按 ordinal 取同方向首席；在 r39 中它把 50ms `io_dependency rank#1=2.202ms` 选作方向 leader，而同一投影已有
114.94ms 全窗 `rank#3=10.433ms`。rank ordinal 只在各自 board 内有意义，跨窗比较 ordinal 本身没有合同。

B120 在高显著 Trace Decision Inputs 与最终 compact ledger 共用的节点选择层增加 typed window-caliber 隔离：

- projection 有精确主窗、候选池中也存在 exact-window row 时，只让 exact-window rows 进入 actual occupancy、existing-rule eliminable、evidence boundary 与
  adjacent/background 决策摘要；
- 不存在 exact-window row 时保持原 bounded evidence，绝不因缺失而清空答案；
- compact leader 额外携带 row 自身 `query_window` 与 `window_role`；上游链席携带
  `impact_phase=pre_wakeup_dependency` 和 `post_wakeup_delay_authority=not_provided_by_this_seat`，避免最后一屏丢失相位限定；
- 其他窗口的行仍在完整投影/索引，系统不删除证据、不替模型选择最终结论。

### 120.4 回归、不变量与后续

- 新正例：显式 100ms 用户窗 + 较早 50ms frame probe + 后续全窗 causal board，主窗/target-state/wakeup path 均选全窗；
- 新反例：只有请求端点、没有 exact-window causal carrier 时，不虚构全窗 coverage，继续保留已测 50ms bounded account；
- ledger 对 scope profile 深复制，后续 request-model 变更不能回写投影权威；
- compact leader 在同方向跨窗 rank 冲突时选主窗 row，并携带 query-window/phase authority；
- `go test ./internal/types ./internal/agent -count=1`：PASS；`go test ./internal/tool -count=1`：PASS（159.532s）。

状态：`EVAL-B119-REQWIN1=implemented/full-types-agent-pass`；
`EVAL-B119-MULTIWINLEDGER1=implemented/full-types-agent-pass`；
`EVAL-B107-ENDPOINTAMBIG1=confirmed-third-witness/P1-next`。

B120 不扫描用户原文或模型/答案 prose，不增加答案 hard reject，不修改 trace_query 的根因公式、可消除量或系统补齐算法，也不生成/替换模型结论。它只把已有 typed 用户窗、
query window 与 chain depth 放回正确的权威层级，保留显式窗因果投影和钻取能力。

## 121. 2026-08-05 B120 r40：生产正证与模型答案所有权红线

### 121.1 严格双并发与人工结论

在 `main@607e851b8` 构建冻结后，以 `PARALLEL=2` 严格并行恰好两个 case：

- `trace_query_donghu_real_frame_multicausal`：246s，4 次 trace_query，runner PASS / human FAIL；
- `mr_poly_binding_chain`：296s，finalizer reject/patch=3/3，runner PASS / human FAIL。

工件见 `eval/parallel_selected_summary_evalcampaign_b120_tracepoly_replay_r40_20260805.md` 及 manual audit。B120 获得完整生产正证：Trace 投影锚、目标状态账、
wakeup path、主要占用和可消除席均使用用户指定的 `34579.472865..34579.587805` / `114.940ms` 窗；早期 50ms micro-probe 只保留为钻取证据，
不再夺取主窗。模型也不再把链席解释成唤醒后的 CPU 延迟，最终 projection 保留 `pre_wakeup_dependency` 语义。

多语言案继续证明 B117/B118 已关闭清单复制与 rejected diagram 复活；人工失败仍来自跨语言端点/绑定身份。答案比 r39 更接近源码，但仍虚构
`self._tokenize_fast`，PyO3 wrapper 行缺独立引用，并把 `lib.rs:10` 核心定义与 `:40` wrapper 描述成“两处定义协作”。Analyzer 为 endpoint 反复 5 轮，
Explorer 为同名 wrapper/core 的关系闭合耗费 19 轮，`EVAL-B107-ENDPOINTAMBIG1` 已是第四个生产 witness。

### 121.2 `EVAL-B120-TRACEOWN1`（P0，confirmed）：模型只剩 caveat 时系统报告替代了模型结论

Trace 成文的原始 tool payload 发生结构畸形；unknown-field quarantine 删除错误字段后，模型文档只剩一个 principal caveat。`preCheckRequiredBlocks`
准确报告缺 summary 与 ordered_list，但这两项在当前 registry 中仅是 soft advisory，调用仍被接受。随后 deterministic runtime materializer 增加 table、projection、
metrics、next-step、evidence appendix 等整套系统块，最终用户看到约 92KB 报告，却没有一块模型拥有的 summary/diagnosis/priority conclusion。

这不是“系统补齐丰富信息”的普通形，而是系统补齐在模型主答案缺席时成为整份答案，违反“typed facts/guidance 归系统、结论归模型”的所有权红线。最优修复冻结为：

1. 只在 typed `runtimeTraceFullReportMaterializationAllowed=true` 且 semantic view 存在 required principal block obligation 时启用；
2. 在任何 system materializer 运行前，要求模型文档至少有一块命中 required principal requirement；caveat、diagram、system block 均不能代替；
3. 缺席时同轮 hard-repair，要求模型补发自己的 summary/list/decision，系统不得从 trace 数值自动生成结论来顶位；
4. 判据只读 AnswerDocument 的 block kind/surface role、typed required-block contract 与 runtime report authority，不扫描用户问题、模型思考或答案正文；
5. bounded fact-set、非 Trace、已有模型主块以及正常系统事实补齐均保持原行为。

### 121.3 `EVAL-B120-DIRLEADER1`（P1，confirmed）：同窗不同通道 rank 不能决定修向主席

B120 已隔离跨窗 row，但 compact ledger 在同一 114.940ms 窗内仍按 rank ordinal 先到先得：`io_dependency` 选中邻近 board 的 `rank#1=0.171ms`，
而正式可消除板同方向发布的链上最大席是 `rank#3=10.433ms`。rank 只在各自 board/channel 内有意义，不能跨通道比较；这与 r39 的跨窗 rank 误用同属
“局部 ordinal 越域”。最优修复是按完整投影的正式链上可消除席、同方向 `EffectiveImpactMS` 最大值选 compact leader，并以 deterministic identity 稳定打破平局；
方向间仍不求和、不铸 subtotal，邻近/背景席保持上下文而不抢主席。

### 121.4 排期与不变量

1. B121-A：先修 `TRACEOWN1`，阻止系统报告在模型主答案缺席时冒充完整回答；
2. B121-B：修 `DIRLEADER1`，让 compact context 与正式可消除板共享同一方向主席语义；
3. B122：处理全语言/FFI binding、export、registration 与 call 的 typed identity，不放宽 call-edge hard authority；
4. 随后切换下一组 read/write 异构 case，恰好并发 2 个，避免继续只在 Trace/单一 fixture 上拟合。

状态：`EVAL-B119-REQWIN1=production-covered`；`EVAL-B119-MULTIWINLEDGER1=production-covered`；
`EVAL-B120-TRACEOWN1=confirmed/P0-next`；`EVAL-B120-DIRLEADER1=confirmed/P1-same-batch`；
`EVAL-B107-ENDPOINTAMBIG1=confirmed-fourth-witness/P1-after-B121`。

本轮审计没有改 Trace 计算、显式时间窗、自动补齐、因果投影或答案正文；没有用自然语言关键词作 hard gate。runner PASS 仍不能替代人工语义与所有权审计。

### 121.5 B121 施工结果

`EVAL-B120-TRACEOWN1` 已按所有权边界完成：

- 新增第五个显式 same-turn hard policy 席 `runtime_trace_model_principal`，并通过 route table、policy ratchet 与 ForceHard producer-count tripwire 正式登记；
- 只有成功的 typed `trace_query`、full-report materialization authority、以及 semantic view 的 required principal requirement 三者同时成立时才检查；
- 在 system materializer 运行前，模型文档必须至少命中一个 required principal block kind；仅 caveat/diagram 即使标了 principal 也不能替代模型答案；
- 正常 required-block 完整性仍是原 advisory 策略，本门只守“至少一块模型主答案”的最低所有权，不把所有结构缺项重新硬化；
- 非 Trace 与 bounded fact-set 反向测试保持旧行为，判据不读取任何 block text、用户原文、思考或最终 prose。

`EVAL-B120-DIRLEADER1` 同批完成：

- compact ledger 先用 `PrimaryRootCauses + OnChainCauses` 的 evidence identity 建正式链上席集合；同方向有链上席时，adjacent/background ordinal 无权夺主席；
- 同一方向主席按 `EffectiveImpactMS DESC`，再以 rank 与 deterministic identity 稳定打破平局；方向输出也按主席值降序；
- 没有链上 roster 的 legacy/synthetic 投影保留全席 fallback；方向 subtotal 仍未授权，任何行都没有被相加。

回归：定向测试 PASS；`go test ./internal/agent ./internal/tool -count=1` PASS（tool 160.029s）。

状态：`EVAL-B120-TRACEOWN1=implemented/full-tool-pass/replay-next`；
`EVAL-B120-DIRLEADER1=implemented/full-agent-tool-pass/replay-next`；
`EVAL-B107-ENDPOINTAMBIG1=confirmed-fourth-witness/B122-next`。

B121 没有改 trace_query、根因排序公式、唤醒链、窗内可消除量、显式时间窗或自动补齐，也没有生成、删除或重写模型结论；它只阻止系统补齐在模型主答案缺席时成为替代答案，并让最后一屏 typed 上下文与正式投影共用同一主席语义。

## 122. 2026-08-05 B122：跨语言绑定链的词法身份与关系表达

### 122.1 `EVAL-B107-ENDPOINTAMBIG1` 根因再核

连续四次生产回放并不是 PyO3 名称本身难识别，而是 repomap 的两条 typed 载体不连续：

1. Rust 调用扫描递归进入 inline `mod`，但声明扫描只遍历 source-file 根节点；因此
   `mod py` 内的 wrapper 与模块初始化函数不在 `FileInfo.Symbols`，同名 wrapper/core 无法
   获得不同 `SymbolID`；
2. `super::tokenize_bytes` 虽形成 call relation，旧 resolver 把 `super` 当普通 receiver，
   不能按调用方词法作用域解析父模块。grounding 随后只能使用模型短名或错误的顶层 enclosing
   callable，导致 wrapper/core 合并、关系提前闭合和重复探索。

这两个断点发生在模型成文之前，最优修复不是放宽图边证据门，也不是硬编码 `_fastlex`、
PyO3、文件路径或函数名，而是恢复语言解析器已经拥有的词法身份。

### 122.2 B122-A：inline module callable identity（已施工）

- Rust 声明提取改为递归遍历 lexical declaration container；inline module 内的 function、
  const、type 以及嵌套 type/member 均携带稳定 `Symbol.Parent`；module-owned callable 仍为
  function，不伪装成类型 method；
- Rust call relation 从 AST ancestor 写入精确 `FromEP.Name + FromEP.Receiver`；
  `self::`、`super::`、`crate::` 由同一 parser-owned caller scope 解析，不读取用户问题、
  模型输出或源码关键词；
- `py::tokenize_bytes` wrapper、crate-level `tokenize_bytes` core 与 `py::_fastlex` 初始化函数
  保持三个独立 identity；第 42 行规范化为
  `py.tokenize_bytes -> tokenize_bytes`；
- Rust extractor generation `6 -> 7`，确保旧暖缓存不会继续提供缺失嵌套声明的陈腐图。

新增 production-shaped 回归覆盖 nested callable census、同名 SymbolID 不折叠、
`super::` 父作用域解析和 emit-evidence 方向规范化。该修复适用于所有 Rust inline module，
并复用全语言共有的 `Parent`/`SymbolID`/call relation 载体，不增加框架特例。

状态：`EVAL-B107-ENDPOINTAMBIG1=partially-implemented/B122-A-tests-pass`。

下一小批 B122-B 处理 binding/export/registration 的 typed 非调用关系表达：注册边必须保持
注册关系，不能为了画连续调用图伪造成 call；同时审计所有支持语言的关系 carrier 与图层
能力，继续保持 call-edge hard authority 和模型结论所有权。

### 122.3 B122-B：typed registration edge（已施工）

跨语言审计确认：已有 `EvidenceRegistration` 与 relation provider 能表达 registry、route、
plugin、FFI/JNI/native-module 等绑定，但 `ClaimFormOf` 只按 anchor 把它降成
`definition_fact`，diagram relation enum 又只有 call/import/guard/contain/observe/precedence。
结果是同一注册事实无法成为可校验的图边；模型只能写 `Note over`、删除边，或错误把绑定画成
source-level call。该 GAP 位于共享语义层，不属于 Java、Rust、ArkTS、Cangjie 或任一框架特例。

通用修复：

1. 新增 `ClaimRegistrationEdge`。只有 `EvidenceRegistration` 同时携带非空 typed
   `Subject + Object` 时才投影成注册边；稀疏旧行继续按 anchor 保持 definition/事实，
   summary/label/prose 不能铸造方向；
2. 新增 `DiagramRelRegister`，与 `ClaimRegistrationEdge` 双向单源映射；schema、skill、
   diagram surface plan、architecture capability 与 member/citation role 同步登记；
3. `register` 明确为 typed-only：没有加入 `InferRelationFromLabel` 的字符串词典，模型写
   “register/绑定/导出”标签不会获得任何 hard authority；必须显式提交
   `relation_kind=register`；
4. 显式 registration edge 必须由一条 citable、同方向、exact Subject/Object 的 grounded
   registration evidence 支持；缺证、反向或稀疏端点均 fail-closed。call edge 继续只认
   grounded call-site，注册边不能掩盖或替代真实调用边；
5. sequence 的未证跨运行时边界仍使用 `Note over`；在 call-DAG/architecture 中可用
   `register` 表达准确绑定，再分别保留边界两侧已证调用。系统只提供关系事实与校验，
   不生成图、不改写正文、不替模型下结论。

语言矩阵结论：关系 schema/evidence/diagram 验证完全与语言无关，覆盖项目全部 read language；
各语言 parser 只负责提供自己的 declaration/call identity。Rust inline-module 缺口由 B122-A
补齐；Go/Java/Kotlin/JS/TS/ArkTS/Python/C/C++/Cangjie/Swift/Ruby/Lua 的既有 class/module/
receiver callable carrier 保持原实现，Proto 仍是 declarative、不会伪装 executable call。

回归：`internal/types`、`internal/skill`、`internal/agent`、`internal/orchestrator` 全量通过；
新增 registration 完整/稀疏投影、typed-only label 负臂、同向/反向/缺证图边和 schema
指导 pin。另修复 B121 测试中被 `go vet -copylocks` tripwire 发现的两个 BusContext 值拷贝，
统一改用 `ShallowClone`；生产代码不变。

状态：`EVAL-B107-ENDPOINTAMBIG1=implemented/B122-A+B/full-related-pass/replay-next`。

## 123. 2026-08-05 模型答案消失事件与 B122 r41 读写异构审计

### 123.1 `EVAL-B122-ANSWERCARRIER1`（P0）：合法 JSON 外壳掩盖结构融合，系统补齐误签完整答案

客户指出 `20260805-085448.711-42828.html` 没有模型回答。该工件生成于
`main@607e851b8`，早于 B121 `76b476153`。原始 `emit_answer_document` 参数只有一个
`kind=caveat` 块；该对象还带有两个字段名：

- `"}, {"claim_uses`
- `"}, {"facet_ids`

这两个名字把原本的 JSON 对象边界序列化进 property name。外层 JSON 语法有效，旧
unknown-field quarantine 因而把它当普通向前兼容 metadata 删除；剩余 caveat 被接受，随后
Trace materializer 补出因果投影、主要占用、指标、建议和证据附录。用户最终看到 800 余行
系统事实，却没有模型的诊断、总结与优化判断。

B121 已守住 Trace 特定的最终底线：有 full-report authority 和 required principal contract 时，
没有模型 principal block 不允许物化系统整稿。但这次复盘确认还有通用根：JSON 语法有效不等于
answer carrier 完整，结构污染字段不能进入普通 metadata quarantine。

最优处置采用三档恢复，而不是“全拒”或“尽量猜”二选一：

1. 唯一、无损的 delimiter/control-char/string-array 修复继续自动执行；
2. 可独立解码的模型 block/text/diagram 进入 typed recovery draft，先触发重试；
3. 块边界或字段归属不唯一时不铸正式结论；重试耗尽后只在 degraded lane 显式披露
   “模型结构化成文失败”，保留已恢复模型文本，并标记未经过完整结构校验；系统 Trace 补齐可作
   事实附录，但不能替代模型主回答。

本批新增精确信号：只扫描 JSON property name，若其中含对象 delimiter/quote（如 `{`、`}`、
`"`），发出 typed repair `answer_doc_structural_carrier_corruption`；不扫描用户请求、block text、
item text、diagram body 或最终 prose。普通 unknown metadata 仍按原兼容策略隔离。完整 emit 在拒绝前
保存可恢复 visible text，patch 拒绝则保持上一版已验文档。

### 123.2 r41 严格双并发结果：runner 1/2 PASS，人工 0/2 PASS

在 `main@5cdbf499f` 构建冻结后，以 `PARALLEL=2` 并行恰好两个异构 case：

- `sr_py_registry_dispatch`：288s，runner PASS / human FAIL；16 轮 explorer、2 次 finalizer reject；
- `github_issue_tokenizers_newline_run_multirepo_py`：1155s，runner FAIL / human FAIL，写工作流停在
  `verify_failed` 后的 replan，最终被 case timeout 取消。

工件见 `eval/parallel_selected_summary_evalcampaign_b122_readwrite_r41_20260805.md` 及 manual audit。

读用例证明 B122-B 的 `registration_edge` 已进入 Finalizer typed context，但 Explorer 的 endpoint
合同仍是纯 static-call 形：`run_pipeline -> resolve -> runtime registry binding -> plugin.handle` 被要求
闭成 `run_pipeline -> JsonPlugin.handle` 静态有向路径。`runtime_dispatched_call` waiver 的类型说明允许
动态分派，完成门却规定任何 active waiver 都不能豁免缺失 directed path，形成同一 typed 合同的
生产矛盾。模型遂反复伪造 endpoint，最后把 `TimestampMixin.handle` 的定义行签成
`JsonPlugin.handle`。答案虽命中 runner 字符串，却还存在三项事实错：

- `@register("json")` 实际执行 `REGISTRY[name] = cls`，并非与 `content_type="application/json"` 绑定；
- `resolve` 返回 `cls()` 实例，不是类引用；
- `JsonPlugin` 的实际 handle 来自 MRO 首席 `TimestampMixin.handle`，不能把定义 owner 改名为
  `JsonPlugin.handle`。

写用例则暴露验证上下文权威污染。model-authored probe 用 merge `(104,101,400)`（字节 `he`）测试
输入 `#el`，却断言输出应为 `[35,400,108]`。这个 comparator 无用户请求、既有测试或 grounded
behavior contract 支持，系统仍把整条失败标成 “Latest verification failure (authoritative)” 注入
replan。Planner 已识别期望不合常理，却只能围绕被加冕的伪 oracle 推理，最终 60K context、1155s
超时。首次 patch 还把长度 1 的单换行折叠，说明 boundary probe 只有正例、缺少“不满足 run 条件”
的负臂。

### 123.3 新 GAP 与冻结批次

1. `EVAL-B122-ANSWERCARRIER1`（P0，本批施工）：结构污染 key 不再静默 quarantine；可恢复文本保留，
   不完整 carrier 重试/降级披露，不能靠系统补齐签绿。
2. `EVAL-B122-PATCHQUOTE1`（P0，本批施工）：partial mutation 的非空 citation quote 也必须与当前源码
   file:line 重核；旧门只在 quote 为空时运行，导致错误摘录绕过。full emit 既有首轮校验保持不变。
3. `EVAL-B122-EXECGRAPH1`（P1，下一批）：建立 typed compound execution graph，区分 static call、
   registration/binding、runtime dispatch 与 inherited method owner；`runtime_dispatched_call` 只能在精确
   endpoints、grounded binding/dispatch witness 成立时豁免静态 path，rationale 不能自证。另为“最终由谁处理”
   提供 discover-sink endpoint mode，不能让 Analyzer 预铸错误 exact sink。
4. `EVAL-B122-PROBEAUTH1`（P0/P1，下一批）：verification failure 拆为 observed execution result 与
   expected comparator provenance。来自请求、既有测试、grounded contract 的 comparator 才能驱动代码
   replan；model-only comparator 失败应先重建 probe，不能升格为业务需求。
5. `EVAL-B122-PROBEPOLARITY1`（P1，与上项同批）：行为新增/边界变化的 probe 至少覆盖正例与保持旧行为的
   负例；单换行、无 `(10,10)` 规则等不满足触发条件的输入不能被新逻辑吞并。
6. `EVAL-B122-JSONTEACH1`（P1，协议批）：当前 Finalizer 同时收到 tool schema、Workflow 长清单和重复的
   semantic contract；“native array / claim_uses / citation_ref”多处重复，虽未发现本案由相互矛盾的 JSON
   指令直接触发，但心智负担过高。后续应以 projected JSON schema 为单一字段权威，skill 只保留任务流程、
   证据边界和一个最小 envelope，删除重复字段手册；用 schema/prompt parity test 防漂移。
7. `EVAL-B122-METRICDOWNGRADE1`（P2）：四次 completion call 中的 DOWNGRADED 路径未计入
   `investigation_complete_rejects`，runner 的 churn 面低报。

施工顺序冻结为：B123-A（carrier + patch citation）→ B123-B（probe provenance/polarity）→
B123-C（compound execution graph/discover sink/owner identity）→ B123-D（JSON 教学去重与 parity）。每批
独立提交推送；随后切换下一对 read/write/operation/data/Trace 异构 case。

### 123.4 B123-A 施工不变量

- 只用 JSON key、mutation kind、source file:line 与 typed contract 作硬判据；
- 不扫描用户原文、模型思考、模型答案正文或最终渲染文本；
- 不修改 Trace 显式窗、因果投影、根因排序、唤醒链、窗内可消除量和自动补齐；
- 系统可恢复/展示模型内容与精确事实，但不补写模型诊断或替模型选择根因；
- 普通 schema-unknown metadata 继续兼容；只有结构 delimiter 进入字段名才 fail-loud；
- patch citation 重核使用 bounded current-source reader，runtime artifact、negative search、敏感配置、
  越界和超大文件沿用既有隔离策略。

回归：新增客户原形 full emit、patch 结构污染、patch 非空错误 quote 三个正反 pin；
`go test ./internal/tool -count=1` PASS（171.018s）。

状态：`EVAL-B122-ANSWERCARRIER1=implemented/full-tool-pass/replay-next`；
`EVAL-B122-PATCHQUOTE1=implemented/full-tool-pass/replay-next`；
`EVAL-B122-EXECGRAPH1=confirmed/P1`；`EVAL-B122-PROBEAUTH1=confirmed/P0-next`；
`EVAL-B122-PROBEPOLARITY1=confirmed/P1-next`；`EVAL-B122-JSONTEACH1=confirmed/P1`。

### 123.5 B123-B：verification observation 与 comparator authority 分离

`EVAL-B122-PROBEAUTH1` 已按 typed provenance 完成根修：

1. `VerifyFailureHandoff` 新增 `failure_authority` 与 reason code。失败行来自独立项目测试或 build 时为
   `project_verification`；全部失败行仅来自 `verification_probe/<language>` 时为
   `model_probe_observation`；ChangeReport 缺席时为 `unavailable`。判定只读 typed TestResult kind/suite，
   不解析 probe code、用户请求、模型思考或错误输出 prose。
2. inline probe 断言/`expected_stdout` 失败仍保留精确进程结果、exit code 与 failure detail，但追加
   `probe_comparator_authority/model_authored_probe_comparator_unverified` 诊断。存在独立 runnable project
   suite 时不得由该 comparator 提前截断，必须继续执行项目测试；项目测试通过可签整体通过，同时保留
   comparator 诊断供审计。
3. 没有项目 suite 时仍保持 verify failed，绝不把未经证明的 probe 失败改成成功；但 replan 首屏改为
   “model-authored comparator unverified”，要求先对照 typed behavior contract 或既有测试核验 comparator。
   不受支持时只替换 probe/走 already-applied no-change proof，不能据此直接修改生产源码。
4. 独立项目测试/build 失败继续保留原 `authoritative` 车道；报告缺席继续保留 bounded durable evidence，
   三种权威不会互相冒充。

`EVAL-B122-PROBEPOLARITY1` 采用软设计约束而非不精确 hard gate：planner skill 明确 run-length、cardinality、
threshold 等边界规则应同时覆盖触发样例与邻近不触发样例（如 singleton / just-below-boundary）。系统不能从
任意 probe code 或用户/模型 prose 猜测“是否已经覆盖正负例”，因此不新增关键词扫描或文本 hard reject；
后续生产回放继续审计遗漏率。

新增回归覆盖：错误的 Go model comparator + 真实 Go suite 通过时必须继续并由项目 suite 签绿；probe-only
handoff 必须降为 observation；项目测试失败保持 project authority；缺报告保持 unavailable。
`go test ./internal/types ./internal/agent ./internal/tool ./internal/skill -count=1` PASS
（tool 164.062s，types 20.100s）。

状态：`EVAL-B122-PROBEAUTH1=implemented/full-related-pass`；
`EVAL-B122-PROBEPOLARITY1=implemented-soft-guidance/full-related-pass/replay-next`；
`EVAL-B122-EXECGRAPH1=confirmed/P1-next`；`EVAL-B122-JSONTEACH1=confirmed/P1-after-execgraph`。

### 123.6 B123-C1：未知运行终点不得由 Analyzer 预铸

r41 的问题不是只有 dynamic edge 缺表达。用户问的是“最终由哪个类处理”，终点类本身就是待调查答案，
Analyzer 却把一次 pre-scan 看到的 `JsonPlugin.handle` 写进 exact source/sink 合同。后续所有 completion hard gate
因而要求 Explorer 证明一个尚未调查、且把运行类与继承方法 owner 混成一体的预制终点。

本小批新增 `call_chain_endpoints.sink_mode=exact|discover`：

1. 当前请求已经命名 source 与 destination 时用 `exact`，原 source→sink directed-path hard gate、
   no-directed-path typed boundary 及图边权限保持不变；
2. 当前请求要求系统找出最终 implementation/class/handler 时用 `discover`，只保留明确 source，sink 必须为空；
   pre-scan 候选即使被模型错误填入也会确定性清空并留下 warning，不能成为答案或 hard-gate 权威；
3. `CallChainOrderedEndpointHints` 对 discover 返回 unavailable，禁止 entity/mention order 退化成方向；support lane
   只优先 source，候选必须来自后续 grounded evidence；
4. Finalizer 新增 typed target-discovery boundary：要求模型从 call、registration/binding、dispatch、
   inheritance/implementation 事实选择终点；registration 不得伪装 call；运行时选中的 class 与继承方法的
   definition owner 不同则分别披露。系统不选择终点、不生成结论。

JSON 教学与 schema 同批对齐：`sink_mode` 同时进入 `required` 和 `properties`，classification skill 明确
exact/discover 分界，并保留“pre-scan only selects investigation target / NEVER proves”既有红线。判据不扫描
用户原文或模型答案，全部消费 schema-validated mode 与 endpoint fields。

回归覆盖 discover 中错误预填 sink 自动撤权、discover 不生成 ordered pair、emit_analysis 接受未知终点、
Finalizer discovery context，以及 exact 旧车道。`types/tool/agent` 全包 PASS（tool 165.317s，types 19.861s）；
`skill` 修正 parity pin 后全包 PASS（0.565s）。

状态：`EVAL-B122-EXECGRAPH1=partially-implemented/C1-discover-mode-full-related-pass`；下一小批 C2 为
grounded compound execution path（static call + binding/dispatch + inherited owner），不把非调用关系铸成 call。

### 123.7 B123-D1：畸形 JSON 降级保文与教学单源（插队闭环）

客户对 `20260805-085448.711-42828.html` 的处置裁定为：能安全修复则修复；无法修复时尽量保留模型已经写出的
有用字符串，并明确披露模型输出问题导致降级。代码复核还发现两类系统自造心智负担：

1. `emit_evidence.Parameters()` 在 canonical map schema 后保留一份理论上不可达的手写 fallback；fallback 仍教
   旧字段 `kind`，并声称 absence 不能由该工具表达，与当前 `evidence_kind + scope=negative` 合同相反；
2. answer-document tool description 固定携带五套跨场景 JSON 示例，而实际 schema 会按本轮
   `AnswerSemanticView` 裁掉无关 block kind 并增加 per-kind conditional。模型同时看到“本轮只允许 A”与
   “通用例子请发 B/C”，会增加畸形载体、错误 kind 和无效字段重试。

本批根修：

- `emit_evidence` 删除第二份 schema，`Parameters()` 字节级只返回 canonical generated schema；registration
  的 `subject/object` 教学补齐为精确 registry slot/binding source → bound class/function/handler/value，避免
  端点只留在 summary prose；
- 删除 answer-document 的固定跨场景 worked examples。字段权威只来自本轮 projected schema；内容形状只来自
  Required Answer Blocks 和适用 workflow 规则。skill 同步说明 provider 看不到 conditional 时的唯一退路，
  不再让通用示例与精确 schema 竞争；
- 结构修复层维持原顺序：严格解码、trailing comma/control char/string-array 等无损修复、block 级可见恢复均
  先行；这些都失败后才进入 `visible_string_salvage`；
- 最后一档只扫描 answer schema 的可见字段名 `title/text/label/cells/columns/caveats`，有界提取原模型字符串。
  不扫描用户请求、答案关键词或结论词，不读取 `id/claim_form/citation`，不铸 typed 证据/引用/结论；字符串内
  看似 `"text":...` 的内容会随原字符串整体跳过，不能伪装第二个字段；
- 降级稿明确显示“模型返回畸形 JSON、仅提取可辨认字符串、未运行最终结构校验、内容可能缺段或失序”，保持
  `AnswerDegraded=true` 与 `SkipAnswerChecks=true`。系统 Trace 事实补齐不因此获得替代模型诊断的权限。

回归覆盖不可闭合 JSON 的中英文可见字符串恢复、嵌入伪 key 不二次铸值、降级披露、schema 字节单源、旧
`kind` 负 pin、negative typed fields 正 pin、固定示例退役与本轮 schema 指引。
`go test ./internal/agent ./internal/tool ./internal/skill ./internal/types -count=1` PASS
（tool 164.746s，types 19.708s）。

状态：`EVAL-B122-JSONTEACH1=partially-implemented/D1-schema-single-source+fixed-example-retired+malformed-visible-salvage/full-related-pass`；
后续继续审计其它 emit schema/skill parity 与真实回放，不能据此宣称所有 provider JSON 波动已消除。
`EVAL-B122-EXECGRAPH1` 下一批继续 C2 typed compound evidence。

### 123.8 B123-C2：跨语言 declared-type 关系进入 typed compound evidence

r41 中 `JsonPlugin(TimestampMixin, ValidationMixin, BasePlugin)` 并非解析器缺失。代码复核确认 Python、Java、
JavaScript/TypeScript/ArkTS、Kotlin、Rust、C++、Ruby、Swift、Go、Cangjie 的适用 extractor 已能生成
`inheritance` / `embedding` relation；Lua/Proto 等没有对应语言构造时不应伪造关系。真正的断点在 Explorer
handoff：旧实现只有在同一轮还提取到 concrete value 时，才在内部 Markdown 构造 hierarchy chain；
`len(allValues)==0` 或 `len(relevant)==0` 会提前返回，结构关系从未成为 Finalizer 可引用 evidence。

本小批完成通用修复：

1. 新增 `repomap_structural_relation` deterministic producer。激活只读 Analyzer 的 typed
   `call_chain_endpoints`（exact/discover）或 `predicate_axis=implement`，不扫描用户请求、模型输出或答案 prose；
2. 只接纳 provenance 为 `tree_sitter` / `cangjie_parser`、自身携带 subject/object/resolved_by、且 exact
   relation line 已被 EvidenceClosure 覆盖的 `inheritance|embedding` 行。regex fallback 明确不进入 typed/citable
   车道；confidence 仅复制供软排序，绝不作 hard threshold；
3. 结构 relation 与 concrete-value 生命周期解耦：即使一个 literal/value 都没提取到，已读 AST relation
   仍进入 Turn-A evidence。每行保留 source:line、原始 relation kind、两端点、extractor provenance 与稳定 ID；
4. Discover-sink Finalizer 新增有界 relation capsule，将 `static_call`、`registration_or_binding`、
   `value_or_factory_flow`、`declared_type_relation` 四族 typed facts 并置。只有 `static_call` 被说明为源码调用边；
   return/assignment、注册与继承关系不得被系统熔成虚构直连；
5. capsule 不选择运行终点、不替模型写结论。模型仍需综合“静态调用前缀 + 注册绑定 + factory/return +
   inherited owner”判断最终 class/handler，并在运行 class 与方法 definition owner 不同时分别披露和引用。

语言策略是“同一 graph relation 消费器覆盖所有现有及未来 extractor”，而不是为 Java/Python 分别写关键词表。
未来语言只要按 repomap 的前向兼容关系合同产出 AST/parser-grade `inheritance|embedding`，即可自动进入该车道；
没有这种结构语义的语言不承受额外 prompt。显式时间窗 Trace 的因果投影、补齐、根因排序、唤醒链和可消除量
路径均未改动。

新增回归锁定：无 concrete value 仍输出 Python 多继承三行；regex salvage 不升级；Cangjie partial-read 只允许
已读行；无关请求不增加 prompt；Finalizer 四族并置且不得生成 `run_pipeline -> JsonPlugin`、
`resolve -> JsonPlugin` 或 `JsonPlugin -> TimestampMixin` 的伪调用链。

状态：`EVAL-B122-EXECGRAPH1=partially-implemented/C1-discover+C2-cross-language-typed-relations/full-related-pass`；
剩余 C3 是在真实复放后判断是否需要为 typed-complete dynamic dispatch 增加独立 completion disposition，不能用
model rationale 自证或宽放 static directed-path gate。

### 123.9 B124 r42：模型正文在场，但 Trace JSON 教学与执行合同不同源

在 `main@97c11a993` 冻结构建后，严格并行恰好两个 case：

- `sr_py_registry_dispatch`：134s，runner PASS / human PASS，context 20%，finalizer reject=2；
- `trace_query_donghu_real_frame_multicausal`：163s，runner PASS / human FAIL，context 39%，finalizer reject=0。

Python 答案已正确区分运行时 `JsonPlugin`、`resolve` 返回的实例与
`TimestampMixin -> ValidationMixin -> BasePlugin` 定义 owner 链。两次拒绝只发生在可选 Mermaid 图把动态绑定画成
静态 `run_pipeline -> JsonPlugin.handle` 后；模型最终删图，正文仍完整。这证明 C1/C2 已显著缓解原 288s/16 轮
失控，但真实回放也发现 Python extractor 的通用断点：`decorated_definition` 调用 `pyExtractClass` 时丢弃返回的
relations，因此带 decorator 的 class 不论单继承、多继承，都会缺 declared-type 图层。capsule 同时纳入与分派端点
无关的 `content_type` return 行，仍有认知噪声。

Trace 回放证明 B121 模型所有权红线保持：模型 principal 正文在首部，系统因果投影与自动补齐只是事实附录；JSON
合法、无 salvage、无系统替换。但模型在同一答案内越过 typed ceiling：把 `causal_conclusion=unproven` 写成确定帧因，
把 34/36 次主导唤醒写成全部，把 wakeup/runnable 写成直接阻塞，并对明确禁止相加的同方向席和不同 IO 口径自行
求和，还扩写无 typed row 支持的时间窗。尾部已有紧凑 `Final Trace Decision Boundary`，继续添加同义警告不会根修。

更关键的系统矛盾已亲验：root-cause runtime-only dispatch 的 block compiler 已用 artifact observation 车道，Facet
renderer 却从未应用 override 的原始 `AnswerSurfacePlan.FacetCoverage` 发射 `current_code_path=HARD`；同一 prompt 又说明
“current checkout/source evidence is not required”。此外，通用异常栈模板硬要求 `ordered_list`，模型按客户要求写分层
sections 后，validator 又把缺 list 仅记 advisory。JSON schema、Required Blocks、Facet、Support Lane、Submission
Checklist 因而不是同一份 compiled contract，属于红线级系统自相矛盾，不是模型波动。

本批 `EVAL-B124-CONTRACTSST1` / `EVAL-B124-TRACEBLOCK1` 根修：

1. Required Answer Facets 改读 validator 同源的 post-override `AnswerSemanticView.FacetCoverage`；runtime-only 下
   `current_code_path` 只保留 optional enrichment，不再发射 HARD/MUST declare；
2. typed `runtime_question_profile.scope=causal_diagnosis` 且 runtime-only 时，principal carrier 改为
   `section|ordered_list|table|bullet_list` 等价集合，首选 section 表达诊断分层；异常 stack 等非该 scope 继续保留
   ordered list；
3. observed-artifact support lane 同步允许 section/table，Support Lane 总则不再预设所有 principal 都是 ordered list；
4. Submission Checklist 的 section `claim_uses` 从该 block requirement 的 `AcceptableClaimForms` 生成。Trace-only
   section 因而只教 `external_observation`，不再手抄 `definition_fact` 与当前源码合同冲突。

所有判定只读 typed runtime scope、grounding disposition、compiled view 与 claim-form enum；不扫描用户原文、模型思考、
模型答案或最终 prose。显式窗 Trace 查询、因果投影、系统补齐、根因排序、唤醒链、两维根因与可消除量均未修改；
系统仍不选择原因、不改写模型结论。

工件：`eval/parallel_selected_summary_evalcampaign_b124_jsonexec_r42_20260805.md`、
`eval/parallel_selected_summary_evalcampaign_b124_jsonexec_r42_20260805_manual_audit.md`。

状态：`EVAL-B124-CONTRACTSST1=implemented/full-types-agent-pass`；
`EVAL-B124-TRACEBLOCK1=implemented/full-types-agent-pass`；
`EVAL-B124-PYDECREL1=confirmed/P1-next`；`EVAL-B124-CAPSULEFOCUS1=confirmed/P1-next`；
`EVAL-B124-TRACECOGNITION1=confirmed/P1-after-context-audit`；
`EVAL-B124-JSONTEACH2=partially-implemented/compiled-view-single-source-next-replay`。

### 123.10 B124-B：decorated class 关系保真与 dynamic capsule 降噪

r42 真实工件纠正了 §123.8 的一项过早判断：Python 普通 `class_definition` 会生成 AST inheritance relation，
但 `@decorator` 包裹后的 `decorated_definition` 分支只接收 `pyExtractClass` 返回的 symbols/methods，明确丢弃第三个
返回值 relations。§123.8 的 Python 多继承测试使用手工构造 `FileInfo.Relations`，只证明下游 consumer 正确，未覆盖
生产 extractor；真实 `@register("json") class JsonPlugin(...)` 因此只剩另一个普通 class 的
`BasePlugin -> abc.ABC` 行。

本批按 extractor 合同根修而非拟合 registry fixture：

1. `pyExtractDecorated` 同时返回 symbols 与 relations；top-level decorated class 把所有 `pyExtractClass` 产出的
   继承边上送，decorated method 仍只消费 method symbols；新增三基类完整保真 pin；
2. Python `extractorVersions` 6→7，确保暖缓存不会继续返回缺关系的旧 `FileInfo`；全语言版本矩阵同步重钉；
3. discover-target capsule 对 broad concrete value 行增加 typed graph-connectivity 选择：value/factory 的 subject 或
   object 必须与 static-call、registration/binding 或 analyzer-declared source endpoint 的规范化 symbol tail 精确相等。
   `resolve returns cls()` 可保留，`CsvPlugin.content_type returns "text/csv"` 等邻近但不连通行不再抢预算；
4. target-discovery 软教学明确：非用户硬要求的图为 optional；若必须把 binding/return/inheritance/method-owner
   伪装为 call 才能连成箭头，就删除图，改用 grounded list/table/section。现有 call-edge hard gate不放宽。

连接筛选只消费 typed evidence endpoints 与 analyzer endpoint profile，不扫描请求/模型/答案 prose；它只决定 prompt
可见性，不创造关系、不选终点、不改写答案。Cangjie、ArkTS、Java、Rust、Go 等语言继续通过统一
`repomap_structural_relation` consumer 进入同一图层；本批修的是 Python decorated wrapper 丢 producer 输出的特有
实现断点，而非新增 Python 专属答案规则。

状态：`EVAL-B124-PYDECREL1=implemented/cache-bumped/full-index-agent-types-pass`；
`EVAL-B124-CAPSULEFOCUS1=implemented/typed-connectivity/full-index-agent-types-pass`；
`EVAL-B122-EXECGRAPH1=implemented/C1+C2+decorated-producer/full-replay-next`。
