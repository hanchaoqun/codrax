# HMC312：独立查询与原生展示复用、代表选择验收

2026-10-29（UTC）。候选分支 codex/hmc299-wait-interval-candidate。**本批验收失败，不合入main。**

## 1. 计数与结果

79唯一稳定任务＝26已交付＋53开放，18类仍有欠项；18部分／29待实施／3待验收／2验收中／1持续。完整能力交付新增0，已交付子能力新增0，候选实现1组待验收。剩余验收按5原因组归旧ID，不新增5个任务。

| 轨道 | 实际结果 | 不得扩签的范围 |
| --- | --- | --- |
| 共享展示复用 | 公开8查询及显式2selector回归通过；自然A至少2selector共用一份完整表 | 唯一全仓原量测域隔离针失败，未交付；CPU自然分支未触发 |
| 11.2代表选择 | 自然发现/选择/执行/status已触发；原结构审计FAIL | event_search代表不能由后续resource_stack extra改签原计划 |
| A完整答案 | 原case FAIL，人工FAIL | 正确表图不能代签连续4次非法修补后的恢复稿 |
| B完整答案 | 普通CLI exit0，结构FAIL，人工FAIL | 原生取数正确不抵消正文计数/身份/关系错误 |

基线58b5d71611a88e746b1829467b3533d661331c85；main仍788588e365d40a48b30827ffd8db08dae4e2ae9d。候选代码48c67d465f66ec40e4acb81ccac1751ab5038e33；五文档相邻提交、普通推送及最终清洁状态由publication收据证明，不能据提交推断功能通过。

## 2. 参考与泛化实现

只读对照参考core/query_engine.py:461–505的item/result配对累积、core/batch/closure/select.py完整与未选集合，借用的是独立来源和执行归属不丢失，不是默认同名合并或近邻关系。保本项目原生共享引擎：独立query/observation/payload不合并，完整时序/频率事实才有仅展示等价凭证，未格式化精度、来源代次、线程、范围、端点、行过滤、clock、表字段与注释继续精确绑定。

producer-only私有三字符串凭证不经JSON恢复；后置装饰不能重签被改的事实。原自动表与显式selector共用同一显示键及预算，完整原生表不被模型简表、summary或diagram替代；不依手写文本去重，不授全集、因果或完成权。Trace投影/自动补齐及原4根因/闭合睡眠合同不为此降低，没有问答关键词硬门。

原生Timeline含provenance-only MeasurementDomain，本批直接整体哈希使该元数据影响显示等价及旧观察比较：这是此次实现缺陷，不是旧断言过严。下一批最小方案是producer值拷贝只分离该已分类元数据，保原JSON/MeasurementSources和全部真实事实字段；不得删任意ID、放宽原针或从JSON恢复权限。

## 3. 公开反例与冻结前测试

全命令/真实退出见[原收据](results/hmc_display_representative_20261029_v1/sealed-v1/0171-codrax-hmc312-display-command-receipts-v1.json)。以下运行不合并为独立测试总数：

| 记录 | 结果与口径 |
| --- | --- |
| display-red-v1 | 8个实际独立查询显示8份，真实RED |
| display-green-v1 | 测试误用CPUWindowStats导致编译失败，前提失误原件保留 |
| display-green-v2 | 原目标路径通过 |
| display-planned-red-v1 | 实际PASS，缺完成生命周期，不冒RED |
| display-planned-red-v2 | 先验2个selector真实存在，最终2表，真实RED |
| display-targeted-v1 | 4顶层/46子例PASS |
| display-related-v1 | 79顶层/474子例PASS |
| display-race-v1 | 28顶层/234子例PASS |
| display-structural-v1 | 24顶层PASS，含私有三字符串/clone/JSON及相邻合同 |
| display-clone-race-v1 | 1顶层PASS |
| projection-protection-v1 | 4包326顶层/335子例PASS，原保护筛选不变 |

新CLI仅构建一次：SHA64c6adbbe4a05b278ff844a9032a93870e7eb53513ff6b8955c52655a06cd2b9。冻结7498仓库/130外部/18仓外快照，清单SHA66ead0d4b3b82042527b59ae4ad7025268a1119d3daa9e21ec1a2f922ad40bc7。13个helper已读/审/冻结；source/tests/comments/helper冻结后停写，仅新增审计及仓库文档。provider仅引用路径，内容不读/哈希/复制。

## 4. 唯一全仓：真实FAIL

原命令go test -json ./... -count=1 -timeout=30m。正式runner记录UTC10:13:52.161738至10:24:55.817179，exit1、无信号、recovered=false、source_verified_after=true。开始是parent pre-spawn观测，不冒内核精确开始。

97包PASS／1FAIL／12无测试；28522顶层/33735子例PASS，43/14 SKIP，1顶层及1子例FAIL。261523行全JSON，重复终态/未终态0。日志SHA25b859246046692065e0fa0a1f60209645f308222dce3e48fa57a2fe7b487af4。

唯一叶子TestB1638B1ActualQueryPayloadPreservesMeasurementDomain/thread_timeline/0，旧测试106行：去掉允许独立的MeasurementSources后两份观察必须相同；新增native指纹却受MeasurementDomain有无影响。其它四个view的JSON往返失去private fallback，不能借其PASS免验活体路径。原测试原样保留，未另跑实验或追修求绿。详见[只读归因](results/hmc_display_representative_20261029_v1/sealed-v1/0194-codrax-hmc312-full-failure-audit-v1.md)、[统计](results/hmc_display_representative_20261029_v1/sealed-v1/0195-codrax-hmc312-full-summary-v1.json)、[正式终态](results/hmc_display_representative_20261029_v1/sealed-v1/0196-codrax-hmc312-full-v1-receipt.json)。

## 5. 恰好两例真实并行

A原trace_business_io_sequence.case问题/EXPECT/15步与2312B构造ftrace未改；B原代表选择问题/50步，4份53248B既有构造SQLite只读副本，原始两种字节分别复制两次。不冒四次设备实采、新格式或本轮采样入口。B独立sqlite只读oracle在workspace外，没塞入QUESTION。

| 项 | A | B |
| --- | --- | --- |
| 自然入口 | 原eval/run.sh，n=1 | 普通CLI目录任务，n=1 |
| 正式进程完成UTC | 10:17:46.914460 | 10:18:41.374792 |
| 过程退出 | 0 | 0 |
| 实际查询 | 4 trace_query | 27 trace_query＝10 event_search＋17 resource_stack |
| 唯一原生payload | 4 | 15（不等于查询次数） |
| actual finalizer INIT | 3 | 3 |
| 独立验收 | FAIL degraded_answer_checks_skipped:1 | 原Go结构审计exit1，完整人工FAIL |

A14/B43有效request-response对，1组B歧义响应原样保留并排除；55对实际区间相交，最大23.743999958秒。独立原日志复算一致，不以进程/barrier重叠代签。A原日志非法UTF-8未改，真实INIT只是logger证据，不冒未见provider wire。[并行原证](results/hmc_display_representative_20261029_v1/sealed-v1/0158-codrax-hmc312-actual-request-overlap-v1.json)。

B最后实际成功status的index/selection/hash来自同一返回9345483b48f2cdfd78cc6824a6d1b3b74319ef0898a91c7364529637ae55f43a，26562B。selection摘要4764d3142c629578b95db32ea680b496bdd9fe9c7ea6e35bc19e6c20e6abe755。此代16记录/16尝试；后续INIT为27/27，不能回写旧快照。这三元组执行原Go审计恰好一次，真实exit1：cluster specification or full membership differs。失败没有current-index-audit.json，collector的not_available不等于未执行/NOT_TRIGGERED。[实际执行原证](results/hmc_display_representative_20261029_v1/sealed-v1/0156-codrax-hmc312-B-structural-audit-execution-v1.json)明确是工具运输结果留存，不冒冻结runner收据。

逐面裁定见[综合人工审计](parallel_selected_summary_hmc_display_representative_20261029_manual_audit.md)。两例均不销账；没有第三例、模型回放或二次全仓。

## 6. 留账、封存与发布

5原因组及ROI：先新metadata隔离回归（16.4/18.4），再旧系统线程合计来源/量尺（16.4/16.5）；能力轨修11.2受理目标到精确代表规格/执行归属；原生引文/JSON重试须先公开归因，正文表图的单位/身份/分母/栈关系另留16.4/18.2。合法extra/retry/未选查询不禁用，旧IO/采样/权限未触发不回签。只读登记终态继续轮换，17.6有新材料再扩，不把同字节重包算新二进制覆盖。WAL生产NO-GO，main不变。

422件按canonical realpath+SHA保独立来源归档，含失败/编译前提失误、全部真实参数/result/INIT/答案、3方独立审计、原全仓及新增根因审计；不同路径同字节未合并为同一来源。索引SHA1e14a734230c5a3e49cc4154faa821373f4280ef095f6ebd096c6a944fe680bb。[索引](results/hmc_display_representative_20261029_v1/sealed-index-v1.json)、[封存收据](results/hmc_display_representative_20261029_v1/sealed-receipt-v1.json)。旧13批3619件/5987251702B及302快照身份未变，18旧污染身份为子集，原污染路径继续隔离。

代码9文件（含原审计README同返回三元组说明），文档5文件相邻提交。代码checkpoint明确未接受；[publication收据](results/hmc_display_representative_20261029_v1/publication-v1.json)在普通候选推送后核remote、clean、两提交与全部冻结字节/旧档/本批归档，**只证明候选完整性，绝不把全仓FAIL或两例人工FAIL改成PASS**。当前26/53不变。
