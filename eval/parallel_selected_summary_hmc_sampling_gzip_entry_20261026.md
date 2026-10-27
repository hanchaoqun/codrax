# HMC299：状态量尺区间交接与 sample-only／命名 gzip 轮换

基线 `788588e365d40a48b30827ffd8db08dae4e2ae9d`。逐稳定 ID 复核 **79 唯一＝25 完整交付＋54 开放**，18 类；31 待实施、17 部分实施、3 待验收、2 验收中、1 持续。完整能力本批新增 0；共享交接子能力实施 1 组，但通用渲染结构登记未完成，不能称完整验收通过。09.4／11.3／08.4 不重复交付。

## 公开反例与实现范围

§298 原真实答案把就绪 0.014ms、运行 0.245ms 和后继睡眠 8.230ms 混成一段等待。实际成文输入已有终点、8.230ms 和 `sleep_start_to_sched_wakeup` 量尺，但 producer 持有的精确睡眠段起点未随唤醒边传入；其它时序事实不能代替同一量尺操作数的精确绑定。公共 `TraceQuery.Execute` → 实际 Finalizer adapter 的完整／裁切窗口回归先 RED，再 GREEN。本反例证明字段交接缺口，不证明它是模型错述的唯一原因。

新增可选 `MeasuredWait` 直接保留 producer 已选段的起点、状态终点、状态、源行、已知原始起点以及实际匹配唤醒时刻；不由舍入后的毫秒倒推。量测区间采用半开表达；原有容差使唤醒点与状态终点不同时，两区间分别展示，不能把尾部自动标为睡眠。缺失原始起点明确 unknown，不补零。

独立 `display_only` note 绑定原件、查询、payload、原查询窗、被唤醒线程、唤醒行／时刻和唯一量尺；公共摘要、原生观察、等待摘要与最终有界事实行共用 formatter。缺失／异源／异窗／重复／错误量尺只撤新增展示，不撤合法唤醒关系。源行仅作定位，不以物理行号先后判事件时序。沿用既有 native/direct 信任入口，没有额外 private seal。

原 `RuntimeWakeupEvent` JSON、单点 Span、坐标、指纹、图关系 ID、优先级权限、统计器、链选举及根因排序均不改；无原文关键词硬门。保留原 12 条等待边、每事实族 10 条等预算，不倾倒完整时间线或增大上下文。Trace 因果投影、自动补齐、链上供给／D／IO／语义及业务线索不变。

IO 请求驻留的正确量尺、起止点和独立 issuer-blocked 事实此前已到成文上下文；把 4.262ms 请求驻留称为设备驻留属于尚未闭环的解释失败，不伪称本批修了 IO 取数缺口。

## 参考仓的设计取舍

只读对照 `/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main`：`core/preprocess/wakeup_chain_index.py:1–220` 保线程实例与精确唤醒端点；`sleep_ops.py:530–605,945–980,1080–1116` 保所覆盖 S/D 段的裁切起点，睡眠统计独立于唤醒匹配，避免 W-W 区间重复放大，并一起交付 duration/start_ns/end_ns。本批吸收的是量尺操作数与主体同源，不照搬最近时间匹配为权限门。`io_ops.py:1–190` 的请求起止／时长与重叠分桶也不能变成设备驻留或硬件并发结论。

## 定向验证与严格冻结

末版相关和 race 各 5 包、34 顶层／62 子例 PASS；新增 4 顶层／22 子例。Trace 投影／自动补齐／活跃流等保护 3 包、290 顶层／52 子例 PASS，标准 make 通过。原 public RED、中间状态枚举错误、旧 duplicate-carrier 夹具失效和乱序原格式前置失败均保留，不改写为成功。末版逆源行正控仅证明展示 decoder 不强制行号顺序，不声称 parser 支持任意乱序采集。

冻结清单 SHA `de8ec87a2194a3b1da12693fdf5c263bbe85fdbe52fd48a9e626a3bd69d39580`：7413 仓库输入、27 外部输入、18 快照。实际两 CLI 共用标准二进制 SHA `cc42f3b03a19e3b7a8b2d3748474aa19addee9d20a320e5de3fdb2f4509dd9cf`。冻结后生产、测试、注释和执行脚本均不改；两例终态 `source_verified_before/after=true`。没有冻结后迁移测试、第二次全仓或第三次模型追绿。

## 两例真实结果

| 原问题 | 实际入口与结果 | 人工验收范围 |
| --- | --- | --- |
| A：sample-only，指定线程／窗口的热点能否解释停顿 | 完整 30,062,585B PERFILE2 → 12000 样本；目标 164 记录／259 权重正确；机器 PASS | **完整 FAIL**：记录包络被当成运行时长，空白被当成未运行／阻塞，CPU 记录被当成调度迁移；18.3 不销账 |
| B：普通命名 capture.gz 的热点、CPU 分布和所记录栈 | 无附件预注入；命名路径→真实完整共享准备→6 次成功原生查询；机器 PASS | 原有限问题 **PASS**；保未知类型、权重单位和单帧限制，不回签 A 或其它格式／设备 |

恰好两例各一次，原 QUESTION 和 15 步预算未改，无用户侧防错护栏。两个包装来自同一真实采集，不冒充独立设备或新格式。实际模型请求重叠 22.348 秒：A 本地 23:38:32.824–23:40:10.929，B 23:39:12.436–23:39:34.784（UTC−07）。两 CLI 终态分别 UTC 06:40:15.124451 与 06:39:39.087495，exit0。细节见[逐例人工审计](parallel_selected_summary_hmc_sampling_gzip_entry_20261026_manual_audit.md)。

B 已有事实族覆盖，系统补齐记录 `families_present`，没有额外引擎查询；不签缺失事实自动补查自然触发。两例都是 sample-only，新睡眠量尺分支 **NOT_TRIGGERED**。无实际 Mermaid 实例，不签图形自然覆盖。A／B 上游调查中有错误理由，不等于该理由实际被投递；本批实际 Finalizer 没有收到这些错误 reason，不能据文本相似性断言调查摘要污染已传播。

## 唯一全仓与留账

唯一全仓 PID44787，UTC `2026-10-27T06:37:16.085560Z`–`06:52:04.579840Z`，**exit1，94 包 PASS／8 包 FAIL／12 无测试**。28351 顶层／33118 子例 PASS，4 顶层 FAIL，41 顶层／14 子例 SKIP；250762 行均 JSON，无重复终态或未结束项。日志 SHA `46c6175b9f38aaed0774c2abd747f2fa543d8ecf9b46a361ac50731ca0d7ca56`。冻结输入前后全部一致，不称全仓绿色。

8 个失败包必须拆开：5 个是 `eval/results/.../snapshots/source/internal/{agent,context,tool,types,tracequery}` 不完整影子包；真实失败包为 `internal/tracediag`、`internal/types` 和 `internal/tool`。4 个真实顶层失败如下：

| 测试 | 本批证据与处置 |
| --- | --- |
| TestNonEventPrioritySchemaPins | 新增 WakeupEdge 可选字段未迁移通用渲染结构登记；精确单字段差项已核，不绕过原 pin |
| TestInfoContractNoteKeyCarrierTruth | 新 key 注册 display_only，但 types 内有 decoder 消费，触发既有 carrier 分类协议；不是影子污染，也不能只凭展示意图豁免 |
| TestHMC232SleepInstanceActualBusinessFixture | 公共查询→因果投影中应有的闭合睡眠实例未出现（closed=0、tail=2）；归因仍待公开隔离验证，**不降为 pin、不声明 Trace 投影完整保护已通过** |
| TestP3MeasureAdvisoryOnlyConsumerAbsence | 4 个命中路径全在 snapshots/source；归档源码被全仓扫描视为新 P3 消费者，属于快照污染派生，不是新生产 P3 逻辑 |

结构差项经只读复核，仅新增 `MeasuredWait|*types.TraceWakeupMeasuredWait|measured_wait,omitempty`；精确删除这一项后旧 SHA 完全恢复。下一批需严格迁移该单字段结构登记、独立保护嵌套 DTO，并公开验证通用诊断 renderer 对合法零起点的展示（目前仅静态发现零值省略路径，尚未公开 RED），不能只改 hash 遮掩展示差异。5 个影子包则是冻结助手把不完整 `.go` 快照放在仓内、被 `go test ./...` 遍历，不是 5 个产品包故障；原日志保留，冻结期间不删快照或重跑求绿。

睡眠实例失败的只读排查发现一个需隔离验证的系统级候选链路：新增可选字段改变完整结果 JSON，进而改变内容寻址 payload／EvidenceID，旧投影排序末级及归并生存者存在使用证据 ID 的路径。**局部唤醒事件字节不变，不等于最终投影选择必然不变。** 尚无本批基线／候选逐层动态对照证明唯一根因，不据此修改统计、放宽睡眠绑定或改变原断言。该公共测试有 typed Observations；其 EvidenceItems 的128限制不是 typed observations 的截断证明，不能把丢实例轻率归因于摘要预算。

完整能力新增 0／累计 25，实施子能力 1 组但**未获主分支交付资格**，剩余开放 54。本批验收欠项按四组保留：结构／carrier 登记、真实睡眠实例投影回归、A 的采样执行态解释、归档目录污染测试发现；新等待量尺与缺失事实补查的自然覆盖分别记未触发。原代表选择、栈深度升调用关系、旧 IO、计算权限覆盖及只读更宽终态继续原项，不把重复错误称已证偶发。17.6 真实新格式／外部工具／原生宿主凭据仍缺；17.7 活跃 WAL 零副作用未达，生产 NO-GO。

下一 ROI：首先隔离并复现闭合睡眠实例回归，再完成通用结构／carrier／零起点展示的公开验证和迁移，不能用窄保护测试覆盖全仓反例；然后查已到场的采样原生事实如何在成文时被提升为执行状态，不重写正确统计器、不扫描自由错句造硬门。归档目录须在下一批冻结前安全隔离：仅加 go.mod 只能阻止 Go 包枚举，未必阻止独立源码扫描；应保留封存原字节并使用仓外或非源码扩展名归档，不能静默排除正式生产目录。能力槽轮换已有只读登记／代表选择的原有限合同，新真实二进制材料到齐可提级。

## 原件与发布

结果根 `results/hmc_sampling_gzip_entry_20261026_v1/`。原始失败、真实模型请求／结果／成文、独立审计、完整输入、查询原件、冻结清单及标准二进制按 canonical realpath＋SHA 封存；别名不重复计，字节相同但实际来源不同仍保来源。providers 仅传既有路径，未读取、哈希或归档。

鉴于真实睡眠投影回归尚未解释，本批候选仅分笔提交／推送到 `codex/hmc299-wait-interval-candidate`，**不进入 main**；不是已验收交付，也不修改冻结后代码或测试追绿。main 保持基线。原 close helper 的 publication 模式要求 main，故本批不调用该模式、不修改它，另记候选分支的提交／远端／冻结完整性收据；不伪称原 main 发布门通过。原快照保持不动，下一批先解决归档发现污染。

封存 `sealed-index-v1.json`：**209 个独立原件版本**，SHA `183d7b4c9cfbabf4f49902f724367e9e4da3cbe080c7046737e07fe7a1699174`；其收据保留 A/B exit0 与 full exit1，不推导验收 PASS。新增封存副本用 `.go.txt`，原18份冻结快照未改或移除。

候选代码／测试提交 `b94b2813e765a3398844b1e295c71f066f8ee3f5` 已普通推送上述分支；五份文档另笔。文档推送后的 `candidate-publication-v1.json` 只核两笔相邻提交、全部冻结文件、209份封存、候选远端和 main 未动，原 full FAIL／A人工FAIL／新分支未触发保持原结论。当前工作分支是候选分支，不是 main。
