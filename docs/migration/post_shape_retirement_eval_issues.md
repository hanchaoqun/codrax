# AnswerShape 退役后 eval 评测发现的问题汇总

> ⚠️ **状态:superseded(2026-05-04)**
>
> 本文档是合并版的初稿(基于 8 runs 实测数据)。在合并用户审计需求文档 + 对每个 P 编号做代码二次确认后,终版已写入 [`post_shape_retirement_consolidated_audit.md`](post_shape_retirement_consolidated_audit.md)(P01-P38 + Batch B1-B6 整改计划)。
>
> 本文档保留作为审计追溯历史,新工作请以终版为准。

> **归档元信息**
> - 状态:draft → superseded
> - 归档日期:2026-05-03
> - 数据来源:`origin/main@5e92dc5` 之上手动跑 eval
>   - `eval/cases/qf_architecture.case × 2 轮 × 2 runs` = 4 runs
>   - `eval/cases/m1a.case × 1 轮 × 2 runs` = 2 runs
>   - `eval/cases/u3a.case × 1 轮 × 2 runs` = 2 runs
>   - 共 **8 runs**(全部 PASS,但 6/8 在内部 retry 阶段命中各类问题)
> - 待合并:用户后续将提供一份补充文档,这份归档作为合并基线
> - 范围:V2 carrier 契约层 / 调度层 / 渲染层 / 性能层 的所有暴露问题
> - 关联前置:
>   - [`answer_shape_terminal_retirement.md`](answer_shape_terminal_retirement.md)(AnswerShape 退役主线)
>   - [`block_only_carrier.md`](block_only_carrier.md)(V2 block-only carrier 设计)

---

## 1. 数据基础(8 runs)

| Run | 案例 | verdict | 总耗时 | claim_use违规 | diagram违规 | 其它违规 | retry 次数 |
|---|---|---|---|---|---|---|---|
| qf 171310 r1 | architecture | PASS | 5:07 | 5 | 0 | — | 1 retry → 干净 |
| qf 171310 r2 | architecture | PASS | 7:51 | 11 | 4 | authority_overreach×3 | 3/3 耗尽 + caveat 漏 stdout |
| qf 171516 r1 | architecture | PASS | 7:37 | 11 | 0 | — | 3/3 耗尽 + caveat 漏 stdout |
| qf 171516 r2 | architecture | PASS | 8:15 | 14 | 4 | CitationReq×1 | 3/3 耗尽 + caveat 漏 stdout |
| m1a r1 | call_chain 协作 | PASS | 6:13 | 0 | 0 | CitationReq×1 | 0 retry — 唯一干净 |
| m1a r2 | call_chain 协作 | PASS | 10:23 | 5 | 1 (block_coverage_missing diagram) | — | 多次 retry,未耗尽 |
| u3a r1 | 对比类 ShouldStop | PASS | 9:15 | 6 | 0 | block_coverage(scalar=2 上限 1)×2 + CitationReq×4 | 多次 retry |
| u3a r2 | 对比类 ShouldStop | PASS | 15:38 | (含)| (含)| (含)| 3/3 耗尽 |

**关键事实**:
- `principal_claim_use_missing` 在 **7/8 runs 命中**(87%)
- 唯一全干净的是 m1a r1(LLM 偶然给对了 claim_use 字段)
- 8 runs 共 ~70 分钟运行,慢 run 普遍卡在 finalizer 多轮 retry
- 所有 verdict=PASS 不代表"没问题",只代表 EXPECT_CONTAINS 命中;系统内部其实处于"重复违规 + 用户面板偶发泄漏"状态

---

## 2. 完整问题清单(P01–P16)

| # | 标识 | 现象 | 触发位置 | 受影响 runs | 用户可见 |
|---|---|---|---|---|---|
| **P01** | `principal_claim_use_missing` | family 要求 block 上挂 `claim_use{claim_form}` 但 LLM 不知道 | finalizer emit | 7/8 (87%) | 间接(导致 P05/P10) |
| **P02** | `diagram_edge_unsupported` family 错配 | LLM 写 `kind=flow` 应写 `kind=architecture/call_dag/sequence` | finalizer emit | 4/8 (50%) | 间接 |
| **P03** | `block_coverage_missing` (diagram 缺失) | QFCallChain 强制 ≥1 BlockDiagram(sequenceDiagram),LLM 没出 | finalizer emit | m1a r2 | 间接 |
| **P04** | `block_coverage_missing` (scalar 上限 1) | 比较类 LLM 给 2 个 scalar block,family 上限 1 | finalizer emit | u3a 全部 | 间接 |
| **P05** | `validation exhausted` caveat 漏到 stdout | retry 耗尽时 caveat 串接在 `━━━` 之后 | render | 3/8 (38%) | **是,显著** |
| **P06** | `authority_overreach` 自激发 | schema fallback 自动注入 Authority 又抱怨缺失 | finalizer→validator | 1/8 | 是 |
| **P07** | retry 路径错(finalize 失败重跑 extract) | 调度层 stage_mapping 把合约失败回退到 evidence 节点 | scheduler | 5/8 (63%) | 间接(放大 P01-P04) |
| **P08** | `cannot unmarshal string into ... blocks` | LLM 把 `blocks` 写成 escaped JSON string | tool param parse | 1/8 | 否(retry 修) |
| **P09** | retry hint composer 不输出 V2 contract 修复 prose | switch 没覆盖新 violation kind | composer | 5/8 | 间接(retry 不收敛) |
| **P10** | retry 重复同种违规 3 次仍未修 | 信号不到 finalizer 或 finalizer 不知道修哪 | composer + scheduler | 4/8 | 间接 |
| **P11** | 答案自相矛盾("6 stage" vs "4-stage × 4-agent" 同段)| LLM 把"含 pre-stage 总数"与"主链设计"混为一谈,reviewer 没识别 | self-consistency reviewer | 1/8 | **是,直接** |
| **P12** | 同符号 file:line 漂移(:114 vs :135 引同一函数)| grounder 不做 cross-citation 一致性检查 | ground/coverage | 1/8 | 是(对仔细读者) |
| **P13** | 慢 run 反复重发 35K system prompt | finalizer retry 没有增量上下文,每次重做完整 dispatch | retry 路径 | 8/8 finalizer-retry runs | 否(只是慢) |
| **P14** | think_aloud 段在慢 retry 上越来越长 | 模型自我纠偏 schema 错误时反复反思 | provider 配置 + retry 触发 | 8/8 retry runs | 否 |
| **P15** | 总耗时 5-15 min/run | LLM 调用 14-22s × 30-44 次 | 模型层 + 重试放大 | 8/8 | **是,体感差** |
| **P16** | analyze 阶段固定 1.5-3.5 min(占 20-30%)| Round 1 + Round 2 pre-scan 大上下文 LLM 调用 | analyzer pre-scan | 8/8 | 否 |

---

## 3. 多层根因(L1 直接 / L2 结构 / L3 设计原则)

| # | L1 直接(代码行/prompt 句)| L2 结构(模块设计选择)| L3 设计原则(被违反的原则)|
|---|---|---|---|
| P01 | `internal/skill/defaults.go` answer-document-skill OutputFormat 没出现 `claim_use` token | PR3 `block-only carrier` 把 shape 切到 block 时只翻译"块名",未翻译"块上的 facet 契约" | **LLM 接口面 = LLM 必须做的 + LLM 必须知道的 — 两者必须同步**;只搬一半就把隐性契约变成隐性炸弹 |
| P02 | skill 同时教 "Architecture → flowchart TD" 和 schema 字段 `diagram.kind ∈ {architecture, ...}` | 字段命名(`kind`)与 Mermaid 关键字(`flowchart` / `sequenceDiagram`)同名,prompt 没 disambiguate | **typed-field 名称必须和 LLM 已知的 well-known 词汇错开**;否则 LLM 用先验把它绑到错误的语义 |
| P03 | QFCallChain family 契约要求 `block_coverage[diagram]≥1`,skill 里没体现 | family→required-block 映射只走 V2 contract checker,不走 prompt 编译 | **同一份契约规则必须有两个出口**:LLM 入口(skill prompt)+ 验证出口(contract checker);只走验证出口 = 永远靠 LLM 撞对 |
| P04 | family 契约 scalar 上限硬编码 1 | comparison-class 问题没专门 family,默认家族不允许多 scalar | **family 表必须覆盖 query 的语义簇**;否则比较/对比类问题被错家族强制收敛 |
| P05 | `render/answerdoc.go` failure-fallback 分支 `fmt.Println(violationDigest)` 走 stdout | render 层 caveat 与 final answer 共用 stdout 通道 | **用户面板 = 用户可读 + 系统自洽**;系统的"我没修好"应该走 logger,不走用户屏幕 |
| P06 | finalize 层 fallback 写一段 Authority disclosure → contract checker 又找它 | fallback 注入与 validator 异步执行,无单步收敛 | **同一字段的 emitter 与 validator 必须共享一个 finalize-pass**;两 pass 异步必互相打架 |
| P07 | `internal/orchestrator/scheduler.go::stageMapping` 把合约失败回退到 evidence 节点 | finalize-only contract violation 没专属 retry 路径,共用读模式 retry-to-evidence | **每种失败必须有最小不变路径**:能在 Stage-N 修的不应回到 Stage-N-1 |
| P08 | LLM 把 `blocks` 字段写成 `"[...]"`(string)而非 `[...]`(array)| V2 schema `blocks → items[] → {id,kind,title,text,facet_ids,diagram:{...},...}` 嵌套 4 层 | **JSON schema 嵌套深度 ≥ 3 层时,LLM 序列化 escape 错误率非线性上涨**;深层 schema 必须配 examples + golden block |
| P09 | `internal/analysis/hint/composer.go` switch 缺 case `principal_claim_use_missing` / `diagram_edge_unsupported` / `block_coverage_missing` | 新 violation kind 加在 contract checker 但 hint composer 没同步 | **violation kind ↔ retry-hint 必须一对一映射**;新增 violation 必须同时新增 hint case(枚举完整性应有 lint 兜底) |
| P10 | finalizer retry 看到的 hint 还是 N-1 轮的"修这字段"(无新增信号)| hint composer 没递增/没 escalation;3 retry 收同样文本 | **retry 必须信号递进**:第 N 次 retry 必须比 N-1 次更具体或更骇人,否则 LLM 只会重复同样的错误 |
| P11 | self_consistency reviewer prompt 比对的是字面陈述差异 | reviewer 不做"语义同一,表达不同"归约 | **一致性 != 字面匹配**;reviewer 必须能识别"含 pre-stage 总数 6 vs 主链 4-stage 设计"是同一事实 |
| P12 | grounder 逐条校验 file 存在 + line > 0 | 缺"同 symbol 的所有 citation 必须 line 一致"不变式 | **citation pool 必须在 closure 维度有一致性约束**;否则同一函数引出多个 line 是合法但有害 |
| P13 | finalizer retry 每次重发 35K system prompt | 没有 prompt-cache 增量重发,也没把 retry hint 单独贴在头部 | **retry 应该 = 短增量,不是重新启动 dispatch**;长 prompt 重发是 token 浪费且 latency 翻倍 |
| P14 | `think_aloud=true` 让模型先输出 think 段;慢 retry 上 think 越来越长(自我纠 schema)| think_aloud 是 provider 配置开关,无法 per-stage 关 | **think_aloud 应该 stage-aware**;analyze 用 think 是好的,finalizer-retry 用 think 是浪费 |
| P15 | 远端 LLM 14-22s/响应 × 30-44 次 | 模型 + 网络层耗时,无客户端缓存 | **LLM 调用次数是头号成本驱动器**;减少调用比加速调用收益高 10× |
| P16 | analyze pre-scan 跑 4-8 次 LLM 调用 | Round 1 + Round 2 同 system prompt 重复发送 | **同 stage 内 multi-round LLM 调用应共享 prompt cache**;或合并为一次 longer-context 调用 |

---

## 4. 根因聚类(同一深层原因驱动多个问题)

| 集群 | 涵盖 P# | 共享 L3 原则 | 一句话归纳 |
|---|---|---|---|
| **A. LLM 接口面契约缺口** | P01 P02 P03 P04 | 「typed-field 必须配 prompt-side 教学」 | V2 carrier 加了一堆 facet/family/claim_use 契约,但只在 contract checker 一侧落,prompt 一侧没补 — LLM 在"未告知规则"下被反复 reject |
| **B. retry 信号不收敛** | P05 P06 P07 P09 P10 P13 | 「失败信号必须在最近边界以最具体形式回灌」 | 合约失败信号发给错对象(extract)、用错形式(无 hint case 映射)、没递增、还重发整轮 prompt — retry 拉长但学不到东西 |
| **C. 系统副产物泄漏到用户面板** | P05 P06 | 「系统自洽的失败应当只走 logger」 | failure-fallback / authority disclosure / validation exhausted 这些"系统知道自己没修好"的副产物不应作为答案前缀 |
| **D. 答案语义一致性弱验证** | P11 P12 | 「一致性是 closure-级、跨 citation 的不变式,不是字面匹配」 | reviewer 比字面 / grounder 比单条 — 都没跨条比对,导致同事实多种说法、同符号多 line 都过 |
| **E. V2 schema 嵌套过深** | P08 P01 P02 | 「LLM 序列化深 schema 错误率非线性」 | blocks → items → facet_ids → diagram:{} 4 层 + 命名冲突,LLM 偶尔序列化错;depth ≥ 3 时必须配 schema-by-example |
| **F. 运行时被 retry 几何放大** | P13 P14 P15 | 「LLM 调用次数 = 头号成本」 | 不必要的 retry × 不必要的 think_aloud × 不必要的整 prompt 重发 三重几何放大 |
| **G. analyze pre-scan 固定开销** | P16 | 「同 stage multi-round 应共享 cache 或合并」 | analyze 占总时长 20-30%,基本不可压缩除非合并/缓存 |

---

## 5. 时间归因公式(慢 run 模型)

总耗时 ≈
- **基础**:analyze 2min + explore 3min + finalize-happy 30s ≈ **5.5 min** (m1a r1 / qf r1 ≈ 5min,基础值)
- **+ V2 contract retry × N**:每次 retry **45-90s**(LLM iter + JSON 重发 + extract 无效跳)→ N=3 满 retry 加 **3-5 min**
- **+ family 契约死锁**(u3a 比较 / m1a r2 call_chain diagram 必须):LLM 在两组冲突契约间反复 → **额外 3-7 min**

→ **慢 run = 基础 5.5 min + retry 3-5 min + 死锁 0-7 min = 8-15 min**

---

## 6. 泛化修复方案(每条修复同时解决一整个集群)

| 修复 | 解决集群 | 解决 P# | 原则化做法 | 难度 | 预期 ROI |
|---|---|---|---|---|---|
| **F1: 契约 ↔ Skill prompt 双向同步**(family table 增加 prompt-text 字段;contract checker 与 skill builder 共读这张表) | **A** | P01 P02 P03 P04 | family 表里给每个 family 同时填:(a) required_blocks (b) required_facet_id (c) prompt-side 教学文案;skill builder 在 BuildAnswerDocumentSkill 时遍历该表自动 inject。新增 family/facet 的人**只能**在一个地方加东西,prompt 自动跟随 | 中 | 退役 P01/P02 80%+,顺带消减一半 retry → **运行时-3 min/慢 run** |
| **F2: 失败信号最近边界回灌 + retry 路径专属化** | **B** | P05 P06 P07 P09 P10 P13 | (1) finalize-only contract failure → 只重 finalize,跳过 extract;(2) hint composer 加枚举完整性测试(violation kind ↔ hint case 必须 1:1);(3) retry 时不重发 system prompt,只增量发 retry hint(prompt cache 友好);(4) hint 有 escalation 概念:第 N 次 retry 文本必须比 N-1 信号"更具体或更约束" | 中-高 | 大幅缩短 retry 时长,**慢 run 直降至 6-8 min**;副作用:消除 P05 caveat 漏出 |
| **F3: 用户面板 vs 系统 logger 通道分离审计** | **C** | P05 P06 P14 | render 层定义 `userPanelOnly` vs `loggerOnly` 两条通道;所有 "validation exhausted" / "drift-bounded authority disclosure" / fallback prose 一律走 logger;最终答案 + caveat 走 user panel。新增一类系统 prose 时**必须显式选 channel**(默认走 logger) | 低 | 立即消除 P05 用户可见问题;为后续所有"系统知道自己没修好"做了通道隔离 |
| **F4: closure-级一致性 oracle**(独立 pass,跨 citation/symbol/text 验证) | **D** | P11 P12 | 加一个 finalize 后的 oracle,跑两类不变式:(a) cross-citation:同 symbol 的所有 line 引用必须一致(file 内同 symbol 同 line)(b) cross-paragraph:同名实体在同段提到的数量必须一致;reviewer LLM 兜底语义归约 | 中 | 治本 P11/P12 + 为未来"答案内自相矛盾"问题留好 hook |
| **F5: V2 schema 配套 golden example + flat-mode 兜底** | **E** | P08 P01 P02 | 给 emit_answer_document tool description 直接贴一个 minimal happy-path JSON 示例(每种 family 一个);schema validator 加 "blocks-as-string"宽容路径(detect string-shaped blocks → re-parse → accept),用 WARN 上报 | 低 | LLM 一发命中率 +20-30%;P08 直接消除 |
| **F6: stage-aware think_aloud + Round 合并** | **F G** | P14 P15 P16 | (1) `think_aloud` 在 finalizer-retry 阶段强制关闭(retry 不需要思维链,需要 schema 准确)(2) analyzer Round 1+Round 2 合并成一次 wide-context 调用,或共享 prompt cache | 中 | 慢 run 省 1-3 min;analyze 阶段省 30-90s |
| **F7: 慢 run 自动放弃合约严格性 + 用户告知**("尽力答案而非严格合约失败") | **B C** | P05 P15 | 当 finalize retry 用尽且 contract violations 仅是"非用户可感"类型(claim_use / diagram.kind / 等元数据级)→ 静默通过 + 仅 logger 警告;只有"用户可感"违规(citation 错 / 答案空 / 矛盾)才 caveat 用户 | 低 | 用户体感大幅改善;为合约渐进引入提供"过渡期" |

---

## 7. 修复优先级建议(基于覆盖度 + 难度 + ROI)

| 顺序 | 修复 | 解决集群 | 总价值 |
|---|---|---|---|
| 1️⃣ | **F1** Skill ↔ 契约同步 | A | 一发命中率 +30%, 慢 run -3 min, 同时治本 P01/P02/P03/P04 |
| 2️⃣ | **F3** Channel 分离 | C | 5 行代码消除最严重的用户可见问题(P05) |
| 3️⃣ | **F5** Golden example + 宽容 schema | E | 入门级修复 P08;为新 family 加入降低门槛 |
| 4️⃣ | **F2** Retry 信号回灌 | B | 系统级深修;但需小心调度路径变更影响其它 case |
| 5️⃣ | **F4** Closure 一致性 oracle | D | 答案质量长期投资,无紧急 |
| 6️⃣ | **F6** think_aloud + Round 合并 | F G | 性能优化,无 PR1-PR6 关联 |
| 7️⃣ | **F7** 尽力答案过渡期 | B+C | 与 F1+F3 互为冗余,实施 F1+F3 后可能不必做 |

---

## 8. 与 PR1–PR6 的归责关系

| 由 PR3 引入 | 与 PR1-PR6 无关 |
|---|---|
| P01 (claim_use 教学缺) | P02 (命名冲突) |
| P10 (一半:hint composer 没同步新 V2 violation) | P03/P04 (V2 family 契约设计) |
| | P05/P06 (render channel 设计) |
| | P07/P09 (调度+composer 设计) |
| | P08 (V2 schema 嵌套) |
| | P11/P12 (reviewer/grounder 设计) |
| | P13/P14/P15/P16 (运行时 / 模型层) |

PR3 真正引入的只有 **P01 + 一半 P10**。其余在 V2 carrier 落地时(更早)就埋了。所以**最优起点是 F1**——既能消除 PR3 遗留,又能顺带治本一整个集群,改动量小、可立即验证。

---

## 9. 待办

- [ ] 等待用户提供补充文档(可能涵盖本文未发现的问题或对同一问题的不同视角)
- [ ] 收到补充文档后:
  - 去重重叠条目(若同一现象在两份文档中分别命名,合并为单条 P-id)
  - 若补充文档识别了本文未列的问题,扩展 P 编号 + 重做根因聚类(可能新增集群 H/I/...)
  - 若补充文档对 F1-F7 提出修订或新修复,合并到统一 F-table
  - 重新跑修复优先级矩阵(覆盖度 × 难度 × ROI)
  - 产出统一终版 `post_shape_retirement_eval_issues.md`(覆写本文)
- [ ] 终版定稿后再决定是否启动 F1+F3+F5(低难度组合)
