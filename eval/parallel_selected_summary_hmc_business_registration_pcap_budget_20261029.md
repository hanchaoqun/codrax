# HMC310：读取预览兼容、图关系覆盖与 PCAP 整组预算

## 范围与稳定计数

从干净候选 `3249f2243888f066d79d5a96ce7e831ea66ad04c` 继续，分支 `codex/hmc299-wait-interval-candidate`；main 保持 `788588e365d40a48b30827ffd8db08dae4e2ae9d`。根代理与独立代理逐稳定 ID 核对 **79 唯一＝25 已交付＋54 开放，18 类**。开放状态为19部分实施、29待实施、3待验收、2验收中、1持续；01→18类分别为2/1/1/5/2/3/4/6/3/2/1/5/3/5/3/2/2/4。候选子能力施工、完整父项交付与最终解释验收分别记账。

双轨为：①06.1已有真实PCAP的最后共享整组展示欠项；②16.4/16.5/18.4已证读取兼容和图身份/关系适用域冲突。只读登记缺来源终态作固定轮换，不另跑第三模型例。旧采样、IO、代表选择、栈关系及权限未触发继续原账；WAL未满足零副作用，生产NO-GO。

参考仓实际核读 `core/preprocess/context_index.py` 与 `core/preprocess/network_pcap_ops.py`，图轨对照 `core/llm_contract.py` 的精确成员/字段校验。导航路径不是完成权限；PCAP摘要和有界明细分开，过滤需要明确时钟关系。复用这些意图，不复制全量list、异常变空结果、输入库重写、名称推身份或文件名校时。§309汇总中的 `core/context_index.py` 是历史路径笔误，正确位置是 `core/preprocess/context_index.py`，不改原封存材料。

## 泛化修复与公开反例

1. **读取兼容与下游权限**：原 `TestDispatchRepositoryReadVersionPublicPreview/one_truncated_line` 不改，真实可读文件恢复成功预览；零完整可见行保来源、原件引用、总行数与0/0坐标，仍不授源码读取/版本/完成权，不猜下一行。原件留存失败不伪称成功保存，runtime身份独立。公开读取→extractor、观察账及调查完成反例证明旧消费者会将0/0补成第1行，甚至从隐藏RawRef恢复成员并错误闭合；三消费者现在共守实际正坐标。合法完整空文件保文件级0计数；已有RepoRoot贯通，绝对/相对路径一致。没有关键词硬门。
2. **图覆盖交接**：保原精确业务角色门，不能按标签推角色对应。公开EmitAnalysis→TraceQuery→EmitAnswerDocument→render复现“已证两条唤醒边，却统一声称关系未证”的展示冲突。现对当前块内可见、精确端点且有私有原生凭证的关系签仅供展示的收据，区分角色映射未知与原生关系已证。快照绑定图/锚/边界/选择，内存clone保留，JSON不能恢复，改图或借兄弟块旧锚无权。共享INIT和修补教学保业务首行、完整线程次行及原lane，不增加重复主体或因果资格。
3. **PCAP整组预算**：原DTO字段、原生64条预览、450行上限和完整统计不变。分类/未归组原因先行，每条记录/流成员的全部字段路径作为不可分物理行，内嵌换行转义；以实际写入计数区分原生预览/实际显示/省略。复用原步骤终态行，v2不新增预算外状态行。partial/真实空/完整查询与展示省略仍分开，epoch、捕获帧字节及端点方向不升格为Trace时钟、应用载荷、会话或因果。

Trace因果投影、自动补齐、原4根因合同、31ms闭合睡眠、35ms请求驻留、2ms合法小计和独立背景47ms均保原断言；L1调度循环未改。HMC309新长行测试只纠正与旧成功预览合同冲突的布尔/载体形状，继续断言无可见行权限；旧Dispatch断言未降。PCAP旧时间字段断言仅从行首改为组内包含同一精确路径和值，不删字段/顺序检查。

## 冻结前验证（原件前缀 `/private/tmp/codrax-hmc310-`）

| 范围 | 结果与原始证据 |
| --- | --- |
| 旧读取兼容 | `read-compat-red-v1.log`→`read-compat-green-v1.log`，旧消费者原断言 |
| 隐藏行/完成权限 | `zero-range-consumers-red-v3.log` 用仓外Go overlay映射精确旧实现，真实typed完整性义务下错误闭合；`zero-range-consumers-green-v3.log` 两包PASS |
| 绝对路径空文件 | `zero-range-absolute-empty-red-v1.log`→最终green-v3，合法文件级0凭证保留 |
| 读取相关race | `read-related-race-v2.log` 四包PASS；v1包路径写错的原失败保留 |
| 图相关/保护 | `graph-related-v1.log` 四包111顶层/182子例PASS；`graph-race-v1.log` 四包43/122 PASS |
| PCAP完整tracediag | `pcap-tracediag-v1.jsonl` 208顶层/166子例PASS |
| 最终统一相关 | `final-related-v1.log` 六包PASS，SHA `89d2180f43fc47de3673bb4a0023b2529419429e8bc540adf673a7e46a34bb35` |

扩大四包race **未全过**：3包PASS/1包FAIL，1791顶层/1321子例PASS、1顶层FAIL、2顶层SKIP；`tracequery`另达默认10分钟包级超时，未完成部分不算PASS。原 `TestNextInfoParseAllocRatchet` 测得base14/with22/delta8，上限5；没有DATA RACE报告不代表全race通过。本批未改ParseLine/NextInfo路径，也没有基线隔离证据，不能归类既有/环境波动。未降断言、未重跑求绿，留到后续精确诊断。

过程失败全部保留：图新测试临时编译/新断言拼写、clone漏传、宽筛选带入未完工读取反例；读取新fixture缺typed完整性前提；PCAP新fixture路径规范化。明确区分测试前提失误与真实产品RED。旧实现overlay三原件在 `zero-range-red-overlay-WxfIPp/`，不放回仓库或以`.go`污染全仓。

## 唯一构建、真实静态PCAP与冻结

标准CLI构建UTC08:24:37.564771–08:24:45.949125；测试二进制仅编译08:24:38.708952–08:24:47.633852。两次各一次、输入前后相同，CLI SHA `36c9e95d3252ddfc539b953d68ae1eed4bffcfb046e75fb757e1eb6b33d543b8`；orchestrator.test SHA `435b020418c58badcb7d5efef75d116cd6fa08f942799f13cb006c405d56f836`。没有编译后替换或隐藏fallback。

唯一零模型公共CLI于UTC08:25:05.814115–08:25:06.949709成功，执行原 `examples/tracediag/collect_packet_capture.yaml`。参考实采classic-PCAP副本3499182B，SHA `099ddab49c3c957b5e311d11e8f9ba2e2d7a380f0910bd8becf27a394fbac91a`。复用§308封存独立字节oracle并明确旧源/本批副本绑定，不重新解析或预填模型目录。

报告173物理行/149051B，SHA `17f4a2e3e69bc69d0ad7ff88854f9eb1c4edf226727da594a196ebeed6158695`；记录正文79/79行、流正文73/73行，**64条记录与64流组均完整**，分类7/7及未归组1/1显示。完整总体6101记录、503流组、5963归组/138未归组、4次逆序；不把64预览当503明细。独立机械核对1111路径、6167检查、零差异。核验器v1因系统Ruby无`Array#tally`中断，v2仅换等价计数与新输出名，原断言不改；未重跑产品CLI。实采范围是SLL2/IPv4，IPv6/坏尾/容量来自公开合成回归，不冒称新实采。

冻结 `frozen-v1.json` SHA `63b4f194e37d01e97d14640a56bf439df8ca075d7dd9d6e215aaf81fa1ce85a6`：**7494仓库输入＋132外部输入＋36仓外快照**。冻结后源码、测试、注释及helper停写，只增审计输出和本批五份文档。旧11组封存2990件（含252快照源身份，18个历史污染身份是其子集）及原污染路径隔离不变；provider内容未读/哈希/复制。

## 固定两例、全仓与最终结论

唯一全仓 UTC08:26:42.306839–08:38:14.318040，exit0，冻结输入前后相同：**98测试包PASS／12无测试包，28514顶层＋33670子例PASS，43顶层＋14子例SKIP，零FAIL、零未终止、零重复终态**。原旧读取预览断言（含one_truncated_line）全部PASS；非race分配棘轮也PASS，但不能抵消扩大race的原FAIL。完整日志260838行，SHA `8ea1f3a980094d46be5cd1a97d833d8dd09da346f2c76b90e71bf0e9aa9da613`。没有第二次全仓或冻结后追修。

| 实际例 | 执行范围与结果 |
| --- | --- |
| A 原自然业务Trace题 | 标准read CLI、15步、n=1；UTC08:26:45.020699–08:31:23.690581，case275s。**机器PASS／完整人工FAIL**。13次trace_query、9份唯一原生JSON、3实际finalizer INIT、1 HTML；不把payload数当调用数。新角色未证／两条原生边已证说明自然通过；原子修补拒绝所教双行身份，最终结构图丢技术标签。正文64扇区→64字节→设备服务时间越权、原4排名与3投影候选混用及区间措辞继续FAIL。 |
| B 原只读登记缺来源终态 | 原TestNativeRegistrationLiveRestoredMissingTerminalReceipt、n=1；UTC08:26:45.018643–08:26:56.287030。**机器PASS／限定负向人工PASS**。真实planner两请求＋scripted controller＋独立恢复进程公共RunTests，不是full CLI或自然controller。当前1结果／2命令／1verify；历史1／2独立留存，source仍0verify且无完成凭证，总体unverified，proof verified。两索引各4原件／1attempt／1link同代。 |

A22、B2个唯一配对完成请求区间，已证真实请求重叠 **4.495秒**；A另1个歧义响应排除，不拿包装进程重叠冒充模型并行。恰好两例、各一次，无第三例或重放。A read_file=0，B两次正常小文件读取；新超长首行/零可见分支自然 **NOT_TRIGGERED**，仅公开确定性路径已验。HTML仅源码/表图语义审计，不冒称像素QA。

独立人工审计原件为 `/private/tmp/codrax-hmc310-A-audit-v1.md`、`-B-audit-v1.md`、`-pcap-manual-v1.md`；综合判定与触发矩阵见[人工审计](parallel_selected_summary_hmc_business_registration_pcap_budget_20261029_manual_audit.md)。B参数辅助索引空数组遗漏了原日志3次实际调用；实际参数与计划原件齐全，登记产品负向可PASS，采集索引缺口另留18.4。A四次拒绝及179047上下文估算保留，不据此直接断言预算导致事实错误。

## 完整能力、子能力与剩余验收

按原HMC-06.1有限合同，最后整组显示缺口已由真实公共CLI、独立字节oracle、人工审计、必要回归和唯一全仓闭合；候选代码及文档普通推送成功后，**本批完整能力交付1，累计26，开放54→53，main新增0**。18类仍有开放项；状态为18部分实施／29待实施／3待验收／2验收中／1持续。05.2跨钟校准依赖仍是跨源使用前提，06.2会话独立；不追加PCAPNG、自然选脚本等退出门，也不倒签此前PCAP模型正文FAIL。

已交付候选子能力 **3组**：读取兼容及零行权限、块内已证关系的展示交接、PCAP整组预算与实际显示计数。第三组属于上述完整能力，不再相加成4项；图组只是局部实现和触发通过，不冒称整图人工通过。

本批仍留 **4组验收欠项＋1项新自然分支未触发**（均归旧稳定ID，不新增计数）：

1. **16.4/16.5/18.2 图共享入口**：full Emit可见身份与atomic ensure_visible不一致；正文内图与结构图重复不同步、runnable/上CPU时序未复用。下一批先用本轮真实多行patch公开RED，统一精确首行身份与局部修补所有权，保node/anchor/未证业务映射，不放宽门。
2. **16.4/18.2 原生到成文解释**：单位、请求量尺、业务交集/完整区间和原排名/物理候选混用。正确字段已到场，保留原FAIL；先处理已证跨入口冲突，不无界追跑同题或重写正确解析器。
3. **18.4 审计采集**：B辅助参数索引未识别真实planner日志形态。修索引须以原日志/计划对照，不能把空数组记零调用。
4. **18.4 race验收**：分配棘轮FAIL及包级超时待基线隔离；普通全仓PASS不抵消。不抬阈值、不贸然归环境；如证实直接影响06.1，重开该项。
5. **自然未触发**：超长首行零权限；B自然controller、源码终态正向及context-pack durability仍只按原覆盖声明，不能扩签。

下一完整能力槽按ROI轮换11.2代表选择、08.3 IO在途或17.6具备新真实格式/工具/宿主材料的二进制；已完成PCAP整组不重复施工，只读登记仍轮换负向终态。旧采样、IO、栈关系及权限未触发原账不变；WAL生产NO-GO。A失败所以整批不进main，候选发布不是整批验收通过。

## 原件封存与候选发布

本批 **351件** 原始证据按canonical源路径＋SHA封存，不合并不同来源的同字节文件；索引SHA `b5fc868cb059cc524b934edf75dc6a13acc552dd49df8fc7f1b99150efaadfc7`。包含三独立人工审计、原失败/真实INIT/调用参数/最终产物、旧实现overlay三原件及实际源码快照（Go副本改为.go.txt，仓外快照不进入go test）。旧2990件不变，冻结再次通过。

[封存索引](results/hmc_business_registration_pcap_budget_20261029_v1/sealed-index-v1.json)、[封存收据](results/hmc_business_registration_pcap_budget_20261029_v1/sealed-receipt-v1.json)逐件保留真实源坐标/字节/hash，封存不自动推导PASS。A额外材料清单增列仓外旧实现overlay目录，自动生成的原清单另存为A-extra-artifacts-generated-v1.json；不改原运行产物。

候选代码 `2af92e11714e6b28af54a93ec466034da96171df`（24文件）与本批五文档相邻分笔提交；正式26/53以普通推送后[publication收据](results/hmc_business_registration_pcap_budget_20261029_v1/publication-v1.json)为准。收据检查两提交同冻结源字节、远端等于文档HEAD、clean、79唯一ID、所有封存字节及main不变；不是整批人工验收或main准入凭证。
