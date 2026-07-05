# Citation POOL detach 风险/收益评估(CPD 批 #58,2026-07-05)

**批性质**:纯评估。未改任何生产代码;探针测试(3 条)运行后已删除,工作树保持干净。
**用户裁定框架**:先评估后决 —— 收益大∧风险小才做,否则只交本报告。结论=三选一(做/不做/降级),见 §7。

---

## 1. 立项原文(考证)

议题源头是 **OOM 审计 Batch O2 的显式挂起裁定**(2026-07-03),两处原文:

**docs/design/oom_citation_quote_normalize_20260703.md:34(O-4 原始提案)**:

> Batch O2(P1,防线):O-4 citation file 命中 typed 工件路径(AttachedHitrace/AttachedLog/bundle 源路径 verbatim/basename 匹配)→ 跳过 quote 归一化 + **pre-emit 确定性 detach/改挂 runtime provenance**(双语 caveat,复用 G6 detach 链)

**同文档 :43(交付时的挂起裁定)+ commit 5861b4cf 提交信息**:

> **裁定:citation POOL detach 本批不做**(pool 变更与不可软化 citation floor 交互需单独裁定;尺寸墙+跳过已灭绝 OOM 类)

> Citation POOL detach is deliberately not done here: pool mutation interacts with the non-softenable citation floor and needs its own ruling; the OOM class is fully closed by the size bound plus these skips.

**名词澄清**:
- **POOL** = `AnswerDocumentV2.Citations`(emit_answer_document_v2 顶层 `citations[]` 数组;items 以 `citation_ref` 下标索引它)。它同时是:最终答案的"**引用**"参考文献节的渲染源、每条 item `引用：file:line` 后缀的解引用表、citation floor 三表面的计数底数。
- **detach 提案** = 把 file 命中 typed 运行时工件拼写的 pool 条目**从 pool 里删除**(连带 item ref 置 -1 / remap),而不是仅仅跳过对它的读取。O2 实际交付的只有"两个读取面跳过"(quote 归一化 + metadata surface-term 读取),pool 条目本身原样保留。
- **挂起原因** = pool 条目数直接喂进三个 citation floor 硬门表面(§3.2),floor 属"不可软化门",删条目可能把 floor 打穿。

---

## 2. 关键事实先行(TL;DR)

1. **pool-detach 机械已存在且已 pin**:`normalizeRuntimeArtifactCitationRefs` → `dropAnswerDocumentCitationsByIndex`(internal/tool/answer_document_runtime_citation_normalize.go:183/:261)完整实现了"删 pool 条目 + ref 置 -1 + remap",25 条测试看护(pre_emit_artifact_citation_test.go),observation-only/crash-plan 场景已在删。**议题不是"造 detach",而是它的门为什么在真实 run 里关着。**
2. **新鲜标本证明 gap 现行存在**:eval/results/trace_query_donghu_real_frame_multicausal-20260703-111818(O2 交付同日、HEAD 含 O2)——"只分析这份 trace,不分析代码" run,模型 emit 4 条 blob 绝对路径伪引用(`.codrax/blob/…/attached_trace.txt:2917` 等),pre-emit 链全程未删(仅 soft advisory),最终答案渲染出"**引用**"参考文献节列出 4 条机器本地 trace 路径——紧邻其下的系统补充块却声明"它们**不是当前仓库源码引用**"。答案面自相矛盾。verdict PASS(oracle 盲区)。
3. **门关闭的机制已定位**:该 run 日志 `current_source_lane=excluded; current_source_required=false; **current_source_satisfied=true**`。`CurrentSourceSatisfied`(types/runtime_source_answer_authority_view.go:181,由 ledger 里存在 current-source 记录置真)⇒ `KeepsCurrentSourceLaneLoadBearing()`(:232)⇒ `AllowsRuntimeEvidenceWithoutCurrentSource()==false`(:246)⇒ 清理门两条 arm 全部否决。**在一个用户明示"不分析代码"的 run 里,一条来历未查明的 current-source ledger 记录否决了工件伪引用清理**——typed 用户边界(ExcludesCurrentSource,精确信号)对这个门不可见,与 QCE §7.13 "role 门隐身"同构。
4. **floor 交互(挂起的核心顾虑)在需要 detach 的类里实测为惰性**:同日同型姊妹 run(donghu_real_short_runnable)0 条引用 PASS;tier1_floor.go:196/:233 对 ExcludesCurrentSource 直接豁免 current-source 要求。**在混合 run(current-source 车道 load-bearing)里 floor 交互是真的**(探针 3:工件伪引用被 contractCitationsFromAnswerDocument 照数,契约 floor 三表面同源消费)——但混合 run 恰恰不是本议题要 detach 的类。

---

## 3. 现状测绘:pool 的生产/消费面全量清单

### 3.1 生产/变异面(写 pool)

| # | 面 | 位置 | 说明 |
|---|---|---|---|
| W1 | emit 装配 | internal/tool/emit_answer_document_v2.go:198 | `convertEmitCitationsToTyped(p.Citations)` + `enrichCitationsWithEnclosingFunction`(graph 查询,不读文件) |
| W2 | 首轮 quote 归一化 | emit_answer_document_v2.go:250(注释 240-249 pin 顺序)→ answer_document_citation_quote_normalize.go:13-63 | 改 `.Quote`;O-5 exclude 惰性(:22)、O-4 typed 拼写跳过(:48)、O-1 有界读(:91) |
| W3 | 被拒草稿引用回携 | emit_answer_document_v2.go:934-955(`carryForwardCitationsFromRejectedDraft`) | 从上一轮 pool **补条目**(工件条目可经此复活) |
| W4 | pre-emit 修复链铸造 | appendOrReusePreEmitCitation 调用点:answer_document_principal_enum_compile.go:442/1059/2833;answer_document_pre_emit_check.go:947/1445/1700/1736/1774/2093/2098 | 从 accepted 证据行铸 `{File,Line}` 裸条目(QCE GAP-B 的"重建引用永远裸 file:line"即此) |
| W5 | 工件条目删除(既有 detach) | answer_document_runtime_citation_normalize.go:183-222(runtime 工件)、:231-245(伪 current-source 载体)、:261-295(drop+remap 共享内核) | 调用点 emit_answer_document_v2.go:731;门=authority 投影(§3.3) |
| W6 | GAP-B 门控 quote 回填 | emit_answer_document_v2.go:747-760(链尾)+ persist 侧 | 门显式排除工件引用(citation_quote_normalize.go:149-166) |
| W7 | 未引用条目 GC | emit_answer_document_v2.go:298;answer_document_mutation_runtime.go:238-240 | drop+remap 复用同一内核 |
| W8 | patch 路径 pool 操作 | emit_answer_document_patch.go:56/91/96(replace/append_citations);answer_document_mutation_runtime.go:100/112(pool 完整性拒绝) | patch 路径同样跑完整 pre-emit 链(emit_answer_document_patch.go:275),W5 覆盖 |
| W9 | pre-persist 行归一化再铸造 | answer_document_mutation_runtime.go:223-243(`normalizeAnswerDocumentRowsBeforePersist`) | **位于 W5 之后**——W5 若门开删了条目,此处仍可从证据行再铸(本标本未发生:4 条全为模型 emit;但链序孔存在) |

### 3.2 消费面(读 pool)

| # | 面 | 位置 | 性质 |
|---|---|---|---|
| R1 | 参考文献节渲染 | internal/render/answerdoc.go:59-60 → :1416(`renderAnswerDocV2Citations`) | **用户可见**;pool 全量列出 |
| R2 | item `引用：` 后缀 | render/answerdoc.go:1304-1322 | 用户可见;经 citation_ref 解引用 |
| R3 | 权威 hedging | render/apply_authority_hedging.go:55-69 | 用户可见措辞强度 |
| R4 | 契约 floor 表面 1(CitationReq) | orchestrator/contract_check.go:172-178(doc→draft)→ :2180(`contractCitationsFromAnswerDocument`)→ analysis/contract/checker.go:188 | **硬门计数底数**;granularity 只筛 line>0,工件伪引用照数(探针 3 实证) |
| R5 | 契约 floor 表面 2(acceptance citation_count_ge) | analysis/contract/checker.go:600-616 | 同底数 |
| R6 | 契约 floor 表面 3(finalize SuccessCriteria) | orchestrator/contract_check.go:2296-2349(`finalizerCitationSupportCountFrom` → criterion.Env.DraftCitations)→ analysis/criterion/eval.go:711-726/:893-909 | 同底数;外部源/waiver 豁免 :912-919 |
| R7 | floor 豁免 chokepoint(F1-T2 三面同关) | orchestrator/contract_check.go:155-159/:626-661/:668-685/:687-703 | StableEvidenceFloorWaiver / LogTriage・PerfTrace 外部源 / ForBus(disposition+观测支持) |
| R8 | pre-emit 结构检查 | answer_document_pre_emit_check.go:396(citation_pool_integrity,advisory)、:774(pool 完整性 fix hint)、artifact_observed_frame_citations(:399) | D1-G95 后 citation 载体全 advisory,不硬拦 |
| R9 | O-4 跳过双面 | citation_quote_normalize.go:48/:158;answer_document_pre_emit_check.go:8349-8380 | 读取面防线(非 pool 变异) |
| R10 | current-source 载体探测 | answer_document_runtime_citation_normalize.go:384-397 | 决策块清理的旁路输入 |
| R11 | Tier-1 floor | orchestrator/tier1_floor.go:196/:233 | ExcludesCurrentSource ⇒ current-source 不要求 |

### 3.3 既有 detach 的门(为什么标本里没删)

`answerDocumentRuntimeArtifactWithoutRequiredCurrentSource`(runtime_citation_normalize.go:30-45)是 ≥6 个合取项跨 4 层 helper 的 authority 投影:plan(RuntimeGroundingDisposition/CurrentStatusDiagnosticRequired/CurrentSourceEvidenceOrigin)× authority snapshot(Active/HasRuntimeCarrier/CurrentSourceSatisfied/KeepsCurrentSourceLaneLoadBearing/Allows…)× ledger(current-source 观测记录)。**它不消费 `ExcludesCurrentSource` 也不消费 `ArtifactCitationsExternalOnly`**——两个恰好为"本 run 不该有 repo 源码引用"而生的 typed 精确信号(O-5 已把前者接进 quote 双面,却没接进 pool 清理门)。

工件识别谓词也分叉成两条不一致车道:
- **shape 车道**(pool 清理用):`types.LooksLikeRuntimeArtifactPath`(source_path_ext.go:125-155)——认识保留 blob 名 `attached_trace.txt` 等(:137),但属路径形状启发;
- **typed 拼写车道**(O-4 跳过用):`runtimeArtifactCitationPathSet`(citation_quote_normalize.go:179-202)——只装 `AttachedHitraceSource` 用户拼写,**不认识 blob 物化路径**(探针 2 实证:blob 路径 typed 车道 false / shape 车道 true)。

---

## 4. 实证记录

### 4.1 标本(系统级,run 于 O2 交付同日的 HEAD)

eval/results/trace_query_donghu_real_frame_multicausal-20260703-111818,run 日志(codrax-20260703-111820-000-5208.log):

- `:1849` — `emit_answer_document blocks=7 citations=4`(**模型直接 emit 4 条 blob 路径伪引用**;prompt 软引导"should stay uncited"失效,与 OOM 事故 run 的 citations=5 同型);
- `:1856` — pre-emit 对齐检查**看见了**全部 4 条 `current_citation=.codrax/blob/…/attached_trace.txt:*`,`candidate_citations=[]`,按 D1-G95 姿态收为 soft advisory,no retry;
- `:1865` — `mutation: replace_all blocks=8 citations=4` — 4 条全量存活过整条 pre-emit 链(W5 未触发,零 WARN);
- finalize authority 行 — `current_source_lane=excluded; requirement_precision=none; current_source_required=false; current_source_satisfied=true`(门关闭根因,§2.3);
- run-1.out:195-208 — 最终答案渲染 `**引用**：` 节 + 4 条机器本地绝对路径(其一还是模型笔误年份 `20270703` 的坏路径),下方系统补充块同页声明"不是当前仓库源码引用"。verdict PASS(oracle 只查文本关键词,盲于此矛盾)。

对照:姊妹 run donghu_real_short_runnable 同型 0 引用 → 无参考文献节,PASS。openharmony_bytrace(2026-07-05)0 引用 PASS。**模型是否犯规是随机的,犯规时系统无兜底。**

### 4.2 探针(单元级,3 条,已删除)

| 探针 | 内容 | 结果 |
|---|---|---|
| P1 | 最小 ExcludesCurrentSource+external_only ctx + blob 伪引用 → `normalizeRuntimeArtifactCitationRefs` | **fixed=2, pool=0, ref=-1 —— 门在最小上下文里是开的**;真实 run 关闭纯因 authority 状态(CurrentSourceSatisfied 污染),不是缺机械 |
| P2 | blob 物化路径过两条识别车道 | typed 拼写车道 **false** / shape 车道 true;用户拼写双 true —— 车道谓词分叉实锤 |
| P3 | `contractCitationsFromAnswerDocument` 对 `berlin.systrace:941657` | **counted=1** —— 工件伪引用照常喂 floor 三表面 |

---

## 5. 收益量化

| 收益 | 量级 | 依据 |
|---|---|---|
| 消除旗舰 trace 场景答案面自相矛盾(参考文献节渲染机器本地 trace 绝对路径 vs 同页"非源码引用"声明) | **中**:近期 4 个 trace eval run 中 1 个命中(4 条);OOM 客户事故 run 同型(5 条);模型依赖、间歇复发;trace 分析是近一个月客户主战场(berlin/aweme/donghu) | 标本 §4.1;可审计观测已由独立的"系统补充"块承载,删伪引用**零信息损失** |
| floor 语义修正:混合 run 中伪引用不再虚增 floor 计数(floor 被非源码引用"错边满足") | 小:无错边满足导致误 PASS 的标本;理论正确性收益 | 探针 3 |
| GAP-B 关联(任务问):**无直接关联**——GAP-B 是"修复链铸造的 current-source 条目裸无 quote"的回填问题,其门已显式排除工件引用(citation_quote_normalize.go:149-166),与工件条目去留正交;detach 落地后该排除变成双保险,不冲突 | — | 代码核对 |
| **不含** OOM/崩溃收益 | 0:O1 尺寸墙 + O2 跳过已灭绝崩溃类 | oom 文档 :42-43 |

## 6. 风险清单

| 风险 | 评估 | 依据 |
|---|---|---|
| citation floor 三表面被打穿(O2 挂起的核心顾虑) | **exclude/observation 类:实测惰性**(姊妹 run 0 引用 PASS;tier1 :196/:233 豁免;R7 waiver 链同点豁免)。**混合 run:真实**(P3 照数;R4-R6 三表面同底数)——**任何方案必须不触碰混合 run** | §4;4-surface 红线(feedback_citation_gate_three_surfaces)要求三表面豁免必须同切,现有 F1-T2 chokepoint(contract_check.go:626)已保证,方案不新增豁免点即不踩 |
| QCE 刚落的 keep-gate/披露体系被破坏 | 低,但有一条硬要求:`dropAnswerDocumentCitationsByIndex` 置 ref=-1 **不产 DetachedCitationDisclosure 记录**(QCE 披露 ferry 只覆盖 G6 detach 链)。扩大删除范围而不披露=重演 GAP-A 披露失实类。方案必须挂披露(复用既有 runtime-provenance 双语边界文案或接入 QCE ferry) | answer_document_runtime_citation_normalize.go:261-295 vs emit_answer_document_v2.go:762-775 |
| 回归面 | 中低:直接触碰 2-3 个生产文件;既有 pin 42 条(pre_emit_artifact_citation_test.go 25 + pre_emit_qce_citation_audit_test.go 17)**方向全部一致**——:472 pin 的就是 observation 场景删 pool;:217 混合 keep pin 的 pool 里没有工件条目,exclude-scoped detach 不与之冲突 | 测试文件核对 |
| R2' 六处同步 | **不触发**:推荐方案只消费既有 typed 枚举(ExcludesCurrentSource / ArtifactCitationsExternalOnly / AttachedHitraceSource / 保留 blob 名),不新增 schema 字段。若未来改走"新 typed detach 记录载体"路线才触发 R2' | feedback_typed_signal_six_spot_sync |
| 链序孔:W9(pre-persist 再铸造)在 W5 之后,W3(被拒草稿回携)可复活工件条目 | 低(标本中 4 条均为模型 emit,W9 未参与);但 detach 若只坐在链中位,理论上可被绕。需一条 witness 测试确认 W4/W9 的证据行来源在 exclude run 里是否可能携工件路径 | §3.1 |
| 精确信号红线 | shape 车道(扩展名启发)驱动内容变异属边缘;但保留 blob 名(attached_trace.txt 等)是系统保留拼写(context/builder.go:2662),AttachedHitraceSource 是 typed verbatim——推荐谓词= typed 拼写 ∪ 保留 blob 名,均为精确信号 | feedback_precise_signals_for_hard_gates |
| 掩盖上游根因 | **最大的架构风险**:标本的第一性根因是 exclude run 里 `CurrentSourceSatisfied=true`(某条 ledger 记录被归为 current-source,来历未查明——候选:blob 路径 repo 包含性把工件记录错归 current-source,或探索期偶发源码读)。该 flag 同时驱动完成门/prompt 预算/source-audit 抑制等多个消费者(runtime_source_answer_authority_view.go:232 的全部调用方)——只做显示层 detach 会把这个 authority 车道 bug 埋起来 | §2.3;feedback_root_cause_only |

## 7. 结论(三选一):**降级**

**不做**"citation POOL detach"作为新造 pool 变异机制——机械已存在、已 pin、在最小正确上下文里工作(P1);照原提案再造一条无条件 detach pass 是第二套平行机械,且混合 run 里 floor 交互真实存在。
**也不是无事可做**——标本证明 gap 现行、用户可见、发生在旗舰场景,prompt 软引导已实证失效,按"噪音从源头消除/系统兜底"红线该上系统侧确定性守卫。

**降级替代动作**(一个小批,预计 2-3 个生产文件 + pin;此处为方案概要,非本批交付):

1. **门修复(根)**:`answerDocumentRuntimeArtifactWithoutRequiredCurrentSource` 增加 typed 用户边界 arm——`rm.ExternalObservationPolicy.ExcludesCurrentSource()`(必要时并列 `ArtifactCitationsExternalOnly()`)且工件上下文 active 时,**越过 authority 投影直接放行清理**(用户明示边界 > 派生 authority;与 O-5 在 quote 双面的接法同构,补齐当年漏接的第三面)。标本形态即刻被清(blob 名在 shape 谓词内,:137)。
2. **O-4 typed 拼写集补 blob 名**:`runtimeArtifactCitationPathSet` 并入保留 blob 基名(attached_trace.txt/attached_hitrace.txt/attached_atrace.txt/attached_log.txt),消除 P2 实证的车道谓词分叉(顺带堵住非 exclude run 里 quote 归一化把 ≤8MiB blob 行当"当前源码"回写的错车道残口)。
3. **披露**:经此 arm 删除的条目必须留用户可见披露(复用既有 runtime-provenance 双语边界文案,或接 QCE ferry 按最终存在性成文),不得静默。
4. **pin**:标本回放(exclude+blob 伪引用→pool 清空+披露在场)、混合 run 不动(:217 pin 保持)、floor 三表面在 exclude 类零扰动、W9 再铸造 witness。
5. **独立立案(不并入、不静默)**:exclude run 中 `CurrentSourceSatisfied=true` 的 ledger 记录来历取证——它是本标本门关闭的直接原因,且污染面不止 citation 清理(authority 谓词的全部消费者)。这属 authority 车道正确性问题,与显示层各自成账。

**混合 run 的 pool detach 明确不做**:floor 交互真实(P3)、无受害标本、QCE keep-gate 语义需整体重谈——收益不明∧风险实在,维持 O2 的谨慎。若未来出现混合 run 伪引用错边满足 floor 的实锤标本,先按 4-surface 红线重开豁免口径再谈删除。

---

## 附:探针复原记录

- 新建 `internal/tool/zz_probe_pool_detach_eval_test.go`、`internal/orchestrator/zz_probe_pool_floor_eval_test.go`(均为新文件,未触碰既有文件)→ 运行(结果见 §4.2)→ 已删除;`git status` 干净(除本报告)。

---

## 8. 降级批已实施(2026-07-05,主会话采纳 §7 结论后同 session 交付)

五项全部落地;`go build ./...`、`go vet`(touched 包)、`go test ./internal/tool/ ./internal/types/ ./internal/context/`、全仓 `go test ./...` 全绿。CSP(authority 车道 CurrentSourceSatisfied 污染根因取证)已独立立案为 **#63**,本批的门 arm 是显示层防线、非根修,代码注释已明示两层关系与 CSP 指针。

### 五项落点(file:line)

| 项 | 落点 | 内容 |
|---|---|---|
| ① 清理门 typed 用户边界 arm | internal/tool/answer_document_runtime_citation_normalize.go:37-62(arm 判据 :59-62;分层注释 + CSP #63 指针 :52-58) | `ExcludesCurrentSource()` ∧ `RuntimeArtifactContextActiveFromBus` ⇒ 越过 authority 投影直接放行清理;`CurrentSourceSatisfied` 的否决被短路 |
| ② O-4 拼写集补保留 blob 名 | internal/types/source_path_ext.go:122-146(常量 :131-134 + `ReservedRuntimeArtifactBlobBasenames` :139;shape 车道 switch/carve 同源消费 :161-163/:360);internal/tool/answer_document_citation_quote_normalize.go:201-213(typed 拼写集并入 :211-213);internal/context/builder.go:2656-2665(blob 写入方常量对齐同一权威;裸字面量消灭覆盖写入方/shape switch/carve 三个消费场所) | 两谓词成员对齐;pool 清理的 observation 分支同时改为 shape ∪ typed 拼写并集(runtime_citation_normalize.go:223-236)。**核验 P3 残口(并入 CSP #63 批)**:两车道成员已对齐但大小写折叠语义未对齐——shape 车道 lower 比对、typed 车道大小写敏感;pool 清理走并集不受影响,只 consult typed 车道的三个读取面(quote_normalize:48/:158、pre_emit_check:8439)对 case-variant blob 引用不跳过(窄面、O-1 有界 8MiB、user-spelling 条目历史同姿态);修法=typed 集比对折叠,与 CSP 根因批同轮裁定 |
| ③ 删除必挂披露 | internal/types/context.go:5406-5436(`DetachedCitationDisclosureKind` :5410,零值=QCE legacy 车道,`runtime_artifact` 新 lane :5423,记录 Kind 字段 :5435,**内部载体非 LLM schema,零 R2' 面**);internal/tool/answer_document_pre_emit_check.go:112-160(单一 recorder 加 kind 参数,`recordDetachedCitationItemKind` :133,legacy 入口委托 :126,**禁第二套 ferry**);internal/tool/answer_document_runtime_citation_normalize.go:209-311(`normalizeRuntimeArtifactCitationRefsWithContext` :219 + `recordRuntimeArtifactCitationDetachDisclosures` :283,老签名薄壳保留 :209-211);链内调用点 internal/tool/emit_answer_document_v2.go:731;persist 链尾措辞双 lane internal/tool/emit_answer_document_v2.go:785-842(runtime_artifact kept/removed 双语 :828-841,legacy 措辞字节不动) | 删了就说删了:每条被摘 item 经 QCE ferry 在 persist 链尾按**最终存在性**成文;诚实边界(crash remove-all 分支的非工件条目、无 item 引用的纯 pool 条目维持历史静默姿态)在 :283 注释如实声明 |
| ④ 标本回放 pin + 裁定 pin | internal/tool/pre_emit_cpd_pool_detach_test.go(新文件,4 pin):标本回放(blob 伪引用×4 + exclude + **播种 `CurrentSourceSatisfied=true` 复现标本否决机制** → 全删+ref -1+4 条 runtime_artifact 披露+材料化措辞,pool 空=参考文献节零渲染)/混合 run 负对照(真源码引用+工件条目双双不动、零披露,pin"混合 run 明确不做 detach"裁定)/车道对齐(4 个保留 blob 名双车道一致+仓内源码路径负例)/措辞 lane(removed 措辞不得称"内容保留"=GAP-A 红线;双 kind 共存互不改写) | — |
| ⑤ 报告收尾 | 本节 | 报告与实现同工作树交付,不 add/commit(推送前须用户确认) |

### 突变实证(4 组,cp 备份 → 突变 → 必红 → cmp 还原,全部 RESTORED_OK)

| 突变 | 红线输出(摘) |
|---|---|
| M1 arm 摘除(`ExcludesCurrentSource()` 判据置 false) | `typed exclude boundary must open the pool cleanup (CPD #58 arm); fixed=0` —— 播种的 satisfied 否决即刻复现标本 |
| M2 拼写集回退(删 reserved blob 循环) | `typed spelling lane must recognize reserved blob path "/abs/repo/.codrax/blob/…/attached_trace.txt"` |
| M3 披露 recorder 静默 | `disclosure records = 0, want 4 (one per detached item)` |
| M4 措辞 lane 折叠(kind switch 恒 legacy) | Pin1+Pin4 双红:`runtime-artifact kept wording missing: "4 处条目的来源引用无法对应到任何已验证来源…"` —— 恰为本 lane 防止的假措辞类 |

### 确认项

- **R2' 零新字段**:`DetachedCitationDisclosureKind` 为内部 ferry 载体,不进任何 LLM-facing schema/prompt/decode 面;emit_analysis / emit_answer_document schema 零改动。
- **floor 三表面零触碰**:未改 CitationReq/citation_count_ge/SuccessCriteria 任何路径与豁免 chokepoint;exclude 类 floor 本就惰性(§6),混合 run 行为被负对照 pin 冻结。
- **既有 pin 全绿**:pre_emit_artifact_citation_test.go 25 条(含 :100 accepted-proof-keeps-source、:217 mixed-keeps)、pre_emit_qce_citation_audit_test.go 17 条未动且通过——新 arm 的 drop 谓词仅匹配工件拼写,current-source 引用在任何姿态下不受影响。
