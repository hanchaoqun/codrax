# Trace 因果投影展示层 v3:目标锚定单主树(2026-07-02)

> 状态:**已裁定,分批实施中**。用户裁定三项:①全删 mermaid(0 图);②聚合容差用严格档(同 ms 3 位小数 + 同证据行区间);③按 数据聚合层 → 窗口字段 → 树渲染 → golden 重写 分批实施。
>
> 前史:`trace_layered_root_cause_methodology_audit_20260701.md` §7(v1 表+两图,§7.9)→ §7.12-7.14.2(v2 multi-view 7 块)。本文档是 v3 全量重设计,替代 v2 的展示结构;v2 的 typed 数据层(StateKind/UndrillableReason/Effective+ActualImpactMS/DrilldownTarget 等)全部保留并继续承重。

## 1. 触发与需求(用户逐字)

"要求表达准确,清晰,层次关系,唤醒链/依赖链 清晰 最好树状表达。投影时长也要有清晰的层次表达,能一眼看清楚根因投影在用户关注时间窗内各层次根因的影响时长,关注点等等。主动再发现一下投影信息展示的 UX 设计上还有什么更优的展示形式和方案。要清晰,友好,关系清晰,唤醒链清晰,时长投影清晰,不繁琐,也不丢失信息。" markdown+HTML 必须完美;REPL/CLI 尽量无损,可简化不能错。

## 2. 现状审计(18 条,基于真实端到端渲染 + 真实客户输出)

合成多层样例(berlin 形)真实渲染审计出 1-10;老版本真实客户输出(aweme.lite 滑动卡顿)增量暴露 11-18:

1. 同 5 个 on-chain 节点在 6 处重复出现(overview 卡片/树状列表/on-chain 表/影响时长表/intro 链 glyph/唤醒链 flowchart)→ 繁琐。
2. 「树状链路」不是真树:每行 `└─`+全角缩进无 `├` 分支;**目标线程不在树里**;同深度兄弟无分支视觉。
3. 时长层次与链路层次分离:bar 只在影响时长表(bucket 序,无深度),树有数字无 bar;全章无「占窗口 %」。
4. 唤醒链 flowchart 链上节点无时长无状态 icon(背景节点反而带 ms);终端 ascii 渲染巨大空旷。
5. sleep 下钻图 undrillable 节点是孤立浮岛;整图与主链节点完全重复。
6. **用户关注时间窗不可见**(无起止/时长标尺),「窗口投影」无从感知占比。
7. 背景 bar 与 on-chain bar 同尺度同纹理,视觉与分层优先级打架。
8. 窗口投影/链上累计/有效归因/实际状态 4 数值列稀疏(常空),解释句冗长。
9. on-chain 表以「上游/下游」表达边,与树/卡片三重重复。
10. 树缩进方向(越深越上游)与 intro 链 glyph(左=最上游)阅读方向相反。
11. **首屏卡片被微小事实刷屏**:8 张 P0 卡 6 张是 0.374–0.568ms io_latency(rank 5-10 桶序填充),真正的 112ms 主线沉底。
12. **同一事实跨 predicate 双行重复**:io_latency 行与 udk-irq-\*(critical_blocking)同值同行区间各渲染一行,19 行 on-chain 表约 12 行近重复。
13. **`Subject → Object` 箭头语义崩坏**:Object 可能是状态(running/s_sleep)/原因词(page_cache_churn)/线程(udk-irq-10-90)/unknown-thread/trace_span,`→` 使全部读成"影响某下游节点";`s_sleep`、`low_frequency` 出现在「下游或影响点」列。
14. **数字自相矛盾无解释**:block_io_by_inode 累计 1.136ms 而有效归因 112.175ms(承自等待区间),无行内标注,用户视角=bug。
15. on-chain 表行序混乱(深度 2,2,1,2,1…),目标线程自己出现在「上游」列,还有深度空行。
16. 影响时长表跨层混排:最大 bar 是 adjacent 层 133.2ms 排第一,主根因 112ms 在第 8 行;背景 bar 比主根因长。
17. unknown-thread 污染半个报告(背景 8 行+on-chain 2 行影响点全未解析),应聚合。
18. 证据索引 29 条同文件名截断刷屏,应按文件分组。

结论:排版是表象,**信息架构 + 渲染前数据整形**才是根。

## 3. 方案裁定过程

3 个偏置方案(D1 单一主树融合 / D2 分层窗口标尺 / D3 精简双视图)+ 新形式侦察 + 双评审(用户视角/工程视角)7 维打分(准确/层次/链树/时长一眼/不繁琐/无损/三端渲染):

- **D1 胜出**(61.5 / 59),骨架采纳。
- D2 否决主因:层 Σ 把墙钟可重叠的链上投影逐层求和当承重数(自认可 >100% 需 capped)——嘈声聚合信号放上硬承重位,结构性误导;但其**窗口缺失确定性回退规格**嫁接。
- D3 否决主因:12 列表终端爆宽最严重;模板「建议」句踩 `feedback_no_system_backfill`(系统代写用户面板答案)红线;但其**结论先行(降级为纯事实复述)+ ⚠跨窗标记**嫁接。
- 侦察员代码级查证:gantt/timeline 在 `internal/mermaidcompat/keywords.go:17,20` unsupportedKeywords,终端必降级 text fence;subgraph 经 `flattenMermaidSubgraphs`(mermaid_render.go:676-694)终端丢分组标题;终端白名单仅 flowchart/graph/sequenceDiagram。→ **0 mermaid + 等宽 ```text fence 是三端逐字节一致的长期正确路线**(用户已裁定全删)。
- D1 自身致命缺陷已修入终稿:语义 span 的树位置必须忠实 typed 影响点(按 path 画在其影响点之下作兄弟分支,行内注记「span 位于 <subject> 内」),不得静默改写确定性 typed 边。

## 4. 终稿结构:4 块,0 mermaid

每个节点全章恰出现 2 次(树 1 行 + 明细表 1 行;现状 6 次)。

1. **导语块**(Text)——三段:
   - **纯事实结论行**(3 秒即答):主根因 subject·object ms(占窗 %)+ 其下钻目标一句;**禁止**模板「建议」句(红线:系统不代写用户面板答案)。
   - **窗口锚点 + 覆盖率行**:`关注窗口 <start>s → <end>s 共 <ms>`;`on-chain 已归因 <深度1累计>ms/<pct>%,未归因残差 <窗口-深度1累计>ms/<pct>%`(减法,**禁止逐层求和**——链上值墙钟可重叠不可加和)。
   - **一句树读法**:自上而下=向上游追溯;`└─唤醒─`=该行唤醒其父行;💤 是症状非根因、其唤醒子行即下钻结果;⛔=窗口内无匹配 sched_wakeup,链止。
2. **因果投影主树**(Text 内 ```` ```text ````fence,等宽三端一致)——语法见 §5。
3. **无损明细表**(BlockTable)——列:层级 | 因果位置·优先级 | 节点/原因 | 关系▸影响点 | 影响形态 | 窗口投影 | 链上累计 | 有效归因 | 实际状态 | 证据·置信。4 值 3 位小数;空值显式「—」;末列恒非空(规避行尾空单元格裁剪);只用 Cells 不用 Label。
4. **证据索引**(BulletList)——**按文件分组**:首行完整文件名一次,per-E 行只列行区间 + typed 审计摘要。

## 5. 主树语法(渲染规格)

```text
🎯 <目标线程> ‹用户关注线程›            bar满格=窗口<ms>   占窗   层级·关注点
│     [目标自身状态行:💤 s_sleep <ms> …(存在时)]
│
└─下钻─ 💤 <线程> · <原因>             ██████░░░░ <ms>  <pct>%  ‹主根因›主要关注 · <关注点短语> [E#]
    ├─唤醒─ ⚙ <线程> · <原因>          ███░░░░░░░ <ms>  <pct>%  ‹主根因›主要关注 · 执行/算力 · 链上累计<ms> [E#]
    │   └─唤醒─ ⏳ …                    …                        … ⚠跨窗(实际<ms>)[E#]
    │       └─唤醒─ 💤 …                …                        … ⛔无匹配唤醒·链止 [E#]
    └─语义─ ✦ <span名> · <class>       …                        ‹优化点›确定优化 ·(span 位于 <subject> 内)[E#]
    ├─成因─ ◦ <原因词>                  …                        ‹dN› …(同 subject 的成因分解行,非上游线程)
    └─链上·深度未解析─ ◦ <线程> · <原因> …                       ‹链上›(ChainDepth 缺失时不编造树位)

◇ 邻近链 — 与主链时间相邻,不在唤醒路径上
    <线程> · <原因>                     █░░░░░░░░░ <ms>  <pct>%  ‹邻近›邻近参考 · <关注点> [E#]

▒ 背景压力 — 环境证据,不计入链上归因,需结合 on-chain 证据解读
    <线程> · <原因>                     ▒▒▒░░░░░░░ <ms>  <pct>%  <StateKind 短语> [E#]
    其余 N 项合并                        ▒░░░░░░░░░ <ms>(<min>–<max>)[E#–E#]
```

要点:
- **边标签四种**:`└─下钻─`(父行 💤 的下钻结果)/`└─唤醒─`(该行唤醒/依赖其父行)/`├─语义─ ✦`(语义 span,画在 typed 影响点之下)/`├─成因─ ◦`(同 subject 成因分解,非上游线程)。裸箭头禁止。
- **状态 icon**:💤 sleep(症状)/⚙ running/⏳ runnable/✦ 语义 span/◦ 成因·未定状态;背景行用文字短语不用 icon。
- **bar**:10 格;on-chain/邻近用 `█`,背景用 `▒`(纹理区分修 #7/#16);分母=窗口时长;**窗口缺失回退**:省略 % 列、分母退化为本批最大单值、标尺行显式声明「窗口起止未采集…回退尺度」,**禁止估算窗口造占比**。
- **⚠跨窗**:`ActualImpactMS > ImpactMS` 或 `WithinRequestedWindow==false` 时行尾 `⚠跨窗(实际<ms>)`(确定性派生,零新字段)。
- **⛔ 链止**:`Undrillable()` 内联行尾,孤岛消除(修 #5)。
- **树位忠实 typed 边**:ChainDepth+WakeupPath 决定父子;深度缺失 → 「链上·深度未解析」旁支,不编造位置(修 #15)。
- 对齐:渲染层按显示宽度(runewidth,CJK=2 格)补齐;边标签定长(2 个 CJK);窄宽 icon 后补空格归一。终端精确对齐;HTML 等宽 pre 下 ≤1ch 抖动可接受(bar 10 格定宽,长度对比不受起点微抖影响)。

## 6. 渲染前确定性聚合(数据整形层,修 #11/#12/#17;全部纯比较,非模型 prose)

按序应用,证据号全程 union 保留:

- **R1 跨 predicate 同事实合并(严格档,用户裁定)**:同 `Subject` + 同 ms(3 位小数字符串相等) + 同证据行区间 → 合并为一行多 typed 视角(`[E3≡E14]`);Object 取信息量更大者,另一 Object 降为「影响点」注记。零误合风险;差 9µs 的行(E10 类)**保留独立**。
- **R2 微小同类聚合**:同 `(Subject, Object)` 重复 ≥3 次 → `×N 合并`行:总 ms + 单次 min–max + `[E#–E#]`。
- **R3 unknown-thread 影响点聚合**:影响点未解析(`unknown-thread` 哨兵)的背景/支撑行,除 Top-2(按 ms)外合并为「其余 N 项合并」行(修 #17)。
- **R4 primary/hop 双视角合一**:同 `(Subject, StateKind/Object)` 的 root_cause_primary 与 hop 行 → 单树节点,本层=hop 值、链上累计=primary 值、`[E1≡E12]`。
- **矛盾数值行内自洽**(修 #14):`EffectiveImpactMS > 10×CumulativeImpactMS` 时,有效归因渲染为 `<ms>(承自等待区间,非本行实测)`。
- **首屏排序**(修 #11):树内兄弟按 本层投影 ms 降序;结论行取「主根因层中 ms 最大者」,不按 rank 桶序填充。

## 7. 数据层新增(确定性,非模型 prose;无需 R2' 六处同步——非 LLM-emitted 字段)

- 投影级 `WindowStartTs` / `WindowEndTs`(float64 秒):来源 frame_target_resolution anchor(`window_source=query_window` / explicit-union 变体,即 `WithinRequestedWindow` 既有填充路径的同源窗口);缺失时保持零值触发 §5 回退。
- 渲染期派生(不入数据):WindowDurationMS、占窗 %、bar 格数、⚠跨窗、覆盖率行、聚合行。

## 8. Mockup

### 8.1 berlin 合成样例(窗口已知模式)——见会话 Mockup A

结论行 + 窗口/覆盖率 + 读法 → 主树(binder d1 下钻 → RenderService d2 唤醒 → DispatchQueue d3(⚠跨窗)→ IOWorker d4 ⛔;✦ VerifyClass 画在 binder 下 d2 兄弟位并注记「span 位于 RenderService 内」)→ ◇ 邻近 1 行 → ▒ 背景 2 行 → 明细表 8 行 → 证据索引。

### 8.2 aweme.lite 真实客户数据(窗口缺失回退模式)——见会话 Mockup B

60 可见行 → 约 22 行;E1≡E12(running 双视角)、E9≡E22(同 ms 同区间跨 predicate)、E3–E8≡E14–E19(io_latency↔udk-irq)、io_latency ×6 合并 2.858ms、背景 Top-2 + 其余 6 项合并 35.962ms、block_io 有效归因 112.175 行内标注「承自等待区间」;E10(112.214,差 9µs)按严格档保留独立行。

## 9. 实施批次(每批全量 build+test 后提交)

- **B0**:本设计文档 + §7.20 指针。
- **B1 数据整形层**:`types/trace_causal_projection.go` 聚合 R1–R4 + 矛盾标注信号 + 单测(两份样例形)。
- **B2 窗口字段**:WindowStartTs/EndTs 从既有 anchor 路径填充 + 单测。
- **B3 渲染层**:7 块 → 4 块(导语/主树 fence/明细表/证据索引按文件分组);删两张 mermaid 图、on-chain 树列表、on-chain 表、影响时长表、背景表、overview 卡片;保留 block 契约(CitationRef:-1、ClaimExternalObservation lane、SurfacePrincipal、块上限降级路径、ZH+EN 双语)。
- **B4 golden 重写**:ZH+EN golden + 聚合/回退/⚠/⛔/树形断言 + 全量测试 + 推送(推送前向用户确认)。

## 10. 非目标与红线

- 不改 trace_query 引擎与 typed 证据语义;只动投影编译的聚合出口 + 渲染。
- 不写任何「建议/should」句进确定性渲染(系统不代写用户面板答案)。
- 层 Σ 求和禁止(嵌套墙钟不可加和);覆盖率只做深度1累计与窗口的减法。
- 聚合只在严格档(同 ms 3 位+同行区间/同 subject-object 完全相等)内合并;不做 ±ε 近似合并。
- 0 mermaid;不新增 LLM-facing prompt 面(纯系统渲染,R6 不涉)。
