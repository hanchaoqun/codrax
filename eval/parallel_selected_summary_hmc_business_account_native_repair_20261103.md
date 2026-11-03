# HMC339：同源自身账户复用、可选展示修补与两原题验收

2026-11-03，main；基线8723443951391dccb9f7a3f462d532ba95f38e7d，源码949632a62f279deac998c8fa8ddf9003f83ce0e7。

79唯一＝28完整＋51开放；18部分/29待实施/2待验收/1验收中/1持续，勾选不变。**完整能力新增0；共享子能力交付2组；剩余验收欠项4组。** root closure=none；两原例smoke PASS、完整人工FAIL，原03.2有限FAIL，不增二进制/图/全栈/泄漏退出门槛，也不以正确表格代签正文。

## 1. 双轨与参考代码发现

完整轨轮换旧03.2资源身份原有限自然合同与旧338完整抖音原题；共享轨处理已证自身账户缺原生复用、可选展示所有权拒绝误指ID/kind。没有新业务type专门分支、问答原文关键词硬门或自动结论。

对照参考仓span_analysis.query_spans/render_index_text、window_utils、llm_contract及root/core/preprocess指引，采用源账户自包含、字段归属和窗口交集。独立发现query_spans按相交筛行后聚合原c.dur，不可直接当本项目裁窗墙钟。保已有子区间并集与精确状态交集，不复制裸SUM、名称前缀、容差/文本/fingerprint权限门。

8Go/625增6删（4生产/4测试；旧338只改注释、断言不动）：

- 一次索引当前已发布tree账户，完整来源/查询/窗/ObservationSpan/主体对象及线程/物理配对端点精确唯一匹配，复用完整自身墙钟/状态/分段/省略/未知Notes。保原13行引用坐标，summary/table/无边事实图同源；损坏/重复/跨域不补0，不从已展示孩子重算。
- 原binder拒绝可选requested_dimension_indices且仅去该字段后其余原生绑定合法，才给字段定点提示；模型须重新完整发射/patch过原门。无自动删字段、换ID/view/源/窗、复制值或授完成权限。共享教学区分generic计时与显式native状态，self-wall不冒running/等待机制/根因。

## 2. 公开失败保留、最终测试与冻结

缺self/错反馈旧码RED保留。初版附加行虽过新测却破旧13行合同，改Notes后旧断言保住；新测试nil前提、Extract错误全文要求、缺brace编译失败，以及过宽race/guard显式中断均保原。真实Finalizer早已收到两份self Notes；Extractor设计为reference-only，不为测试在每阶段复制全文。具体失败及最终代次见[尝试审计](results/hmc_business_account_native_repair_20261103_v1/sealed-v1/0589-codrax-hmc339-prefreeze-attempts-audit-v1.json)和[最终检查](results/hmc_business_account_native_repair_20261103_v1/sealed-v1/0590-codrax-hmc339-prefreeze-final-checks-v1.json)；中断不是PASS，v2语法失败不是产品逻辑RED。

最终相关7包356顶层/1148子PASS；race×2三包26顶层/64子事件PASS；因果守护3包21/141 PASS。真实原题公开发射及patch精确恢复、原13行/越界负控、跨源/窗/重复身份负控均通过，保原4根因/闭合睡眠。

完整用户抖音235422121B/1696481行，SHA93b45bcca5c9e9b415478d81e176b633d70886328c49731b5258c806695de1ef。冻前2次真实nativequery/0外部模型，在原全窗与中段裁窗核bind/make共4账户：全bind669351/self227101us，全make153066/self1078us，裁make152955/self967us，裁bind152955/self0us。整数微秒裁窗oracle基于旧338独立整文件LIFO结果的完整直子并集；本代不重跑旧LIFO，不注入模型，不冒独立scheduler oracle。实际Finalizer两完整Notes、Extractor两精确引用通过；Go临时raw/payload已清理的4路径见[可用性登记](results/hmc_business_account_native_repair_20261103_v1/sealed-v1/0417-codrax-hmc339-model-free-raw-reference-availability-v1.json)，不冒缺失字节已归档。

CLI正常build/startup0模型0工具；冻结7598输入/426外部/17仓外快照，SHAf8b29f4f516bc688135f2cdfa660e02f4f82628de7165aabbb44466427173ccf后代码/测试/教学/helpers/cases/oracles停写。唯一`go test -json ./... -count=1 -timeout=30m` 08:19:57.228530Z→08:34:13.553845Z，OS0/前后true：98包PASS/12无测试、28728顶层PASS/47SKIP、34356子PASS/14SKIP，267406纯JSON，FAIL/重复/未终态均0。log SHA2cf54ecd931c3b318a269ea2053520939b22e028c8ae3fa7d205ec32706776f5；只读统计不是第二次全仓。

## 3. 恰好两原例各n1，完整人工均FAIL

| 原例 | 材料与真实取数 | OS | 原smoke | 原有限/完整人工 |
|---|---|---|---|---|
| A资源身份 | 2503B完整构造导出文本；4query/4JSON | 0 | PASS | FAIL/FAIL，位型有效性与计数/解释 |
| B抖音启动 | 用户完整235422121B ftrace；8模型查询+1确定性补齐/8唯一JSON | 0 | PASS | 整答FAIL，状态量尺/集合/解释 |

实际request/response精确配对A11/B15、0歧义/未解、最大可证重叠24.934秒，不由barrier/进程并存推定。每例一次成文3条实际INIT、0patch；无图。原件/过程/最终MDHTML/旁路逐层审计见[手审](parallel_selected_summary_hmc_business_account_native_repair_20261103_manual_audit.md)。

A原五操作/五独立计数及原始整数、hex、子类/NULL/空串/未发布全部同源保真；两个selector-only表正确。正文仍将known大整数断占位、四已知编号说两个、-4064表达式倒向，并从局部running/runnable排除算力供给、从局部名称缺测外推字典缺失。4受理解释单元只传[0,2]、省1998B；坏unit1/3及aggregate哨兵note未传，不冒污染。审计初稿曾误认note已传，保初稿/初始root后按实际INIT纠正，FAIL不变。HTML五表值同源，未冒浏览器像素QA。

B32个已发布目标节点wall按原始行/名对旧独立oracle全核一致，16手写阶段的端点/inclusive/self正确。旧make完整/裁窗错误未复现；但attach的整个区间运行列填self103.841（应143.636）、S总体837/.204套扣三长等待后余集、956段累计8.714ms误当一次，另有CPU/IO排除。线程原生时序2783行其它11格按既有显示精度逐源保真；HTML四表16格差异仅名称code/backtick，不冒数值丢格。实际旧tree8账户已到成文；新native13行摘要未选，self Notes自然NOT_TRIGGERED。completion没有native_fact_handoff，坏reason全文未传，不冒直接污染。不能统称源缺失或新展示已自然通过。

新所有权反馈两例均NOT_TRIGGERED；公开PASS不代自然命中。只读登记、其它二进制/IO/代表本批NOT_RUN，不复制旧337负控或已完成18.3收据。

## 4. 四组剩余验收欠项与全部51项ROI

四组分别挂原ID，不新建重复父项：①A03.2原有限解释/计数；②B主体/量尺/集合绑定；③自然原生选择/同源复用及整组、全请求预算；④新展示/反馈/图的自然覆盖与轮换欠项。03.2充分事实后的偶发误述留原，不无限占用下一批。14.1增量/累计/reset协议和expected版本债、旧采样/IO/权限/代表/图、h2/h3/lower-cost-full及更多平台继续在案，WAL生产NO-GO。

| ROI | 项数 | 稳定ID（HMC-前缀） | 推进顺序 |
|---|---:|---|---|
| P0共享 | 7 | 16.4、01.3、02.4、16.5、01.2、18.4、12.5 | 公开复现自然账户选择、原生复用/范围量尺与完整单元预算，不强制view或加词门 |
| P1现有材料原合同 | 6 | 11.2、18.5、03.2、14.1、08.3、18.2 | 轮换08.3有限IO、只读缺来源负控、有材料剩余二进制；不以单次绿销持续项或猜统计协议 |
| P2能力复用 | 11 | 04.3、04.5、08.1、08.5、12.2、12.3、17.6、17.7、05.2、10.2、14.2 | 既有原生路线与完整真实材料，形成端到端能力 |
| P3来源/协议 | 21 | 04.4、04.6、05.3、06.2、06.3、08.6、08.7、08.8、09.1、09.2、09.3、10.3、12.4、12.6、13.1、13.2、13.3、14.3、14.4、14.5、15.1 | 先证明协议/材料/权限边界，WAL不绕零副作用 |
| P4新域 | 6 | 07.1、07.2、07.3、07.4、15.2、15.3 | 真实材料和依赖就绪后完整交付，不用示例冒验收 |

51ID无重漏见[本批核账快照](results/hmc_business_account_native_repair_20261103_v1/sealed-v1/0333-codrax-hmc339-census-roi-v1.json)；原合同仍以[统一清单](../docs/design/hmosperf_implementation_tasks_20260917.md)为准。

## 5. 封存与main发布完整性

ROOT `eval/results/hmc_business_account_native_repair_20261103_v1`；624件index SHA28f94a97f194d640a966ada9a566d059da5cc1b2871c00cf1e7c4cce9e4ae961，seal receipt SHAc60ad59e019edadee86958cec8ff7c7f6bc2faabcd18cb08598804d30bd3e7e3。root无闭项决定SHA8cc2d1bf0c6652b22aa003efbb30ca133d4324effe892367fecd2bf8dc87bb45，原失败/实际INIT/nativeJSON/提交/MDHTML/进程及full均保原。大原件完整引用，不复制进Git；providers（含synthetic）不读/哈希/复制。

旧40代16644各原ROOT不动，41代按代17268件，非全局去重；仅物理重核最新338的469件与更旧链头，不冒全历史payload重核，失败336不补成功publication。源码/恰好五文档相邻提交、clean main/远端同代和逐blob等于冻结输入由[publication](results/hmc_business_account_native_repair_20261103_v1/publication-v1.json)独立核验。本地忽略证据不冒已上传；源发布完整性不豁免任何FAIL/未触发。
