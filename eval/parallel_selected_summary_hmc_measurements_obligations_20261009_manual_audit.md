# §215 通用量测与事务交接人工审计

冻结代码 `bd3209a3a2ef`；批次 `20261009-023242`，结果目录 `results/hmc_measurements_obligations_20261009/`。两例各一次、并行2，runner正式exit0。机器2/2不代表真实问题正确；完整人工 **0/2**。未追加第三例或追跑，旧失败原文保留。测试/载体静态登记后续集成修正另取回归收据，不倒签本次live。

| case | 机器 | 完整人工 | runner耗时 | 结论 |
| --- | --- | --- | ---: | --- |
| trace_measurement_records | PASS | FAIL | 183s | 未选通用量测；漏跨入窗记录、同名序列混合、类型丢失、猜单位/状态/配对 |
| trace_transaction_handoffs | PASS | FAIL | 442s | 原生及主要数字/逐键事实正确，但图无边，并错误声称没有已证有向关系 |

## 量测：完整原值存在，但没有进入正确的问答路径

[主答案](results/hmc_measurements_obligations_20261009/trace_measurement_records-20261009-023244/run-1.primary.md)、[原日志](results/hmc_measurements_obligations_20261009/trace_measurement_records-20261009-023244/run-1.logs/codrax-20261009-023246-000-2199.log)、[独立oracle](fixtures/hmosperf_measurements/README.md)。本节行号对应该单份原日志，不是加了汇总头的all.log。

- 1175–1176：先查process_measurements及按gpufreq的event_search；1260–1261再查marker/名字。四次查询全部带1–2秒窗，未读源码，但没有调用measurements。
- 正确区间视图应13匹配、1时间未知、排除2秒右边界；点搜索返回12个窗内起点是其正确查询语义，不能替代完整区间总体。遗漏0.9秒起的334200000.5跨入记录。
- 1350完成说明合并filter10/20，猜Hz、gpu_state=0空闲、load百分比，误读时长；1427跳过extract。finalizer有搜索库存，没有可选择的通用量测原生表。
- primary:5–9按同名gpufreq合并两个序列；:17把TEXT "1"与INTEGER 1等同；:25/:35/:37/:41猜单位、active语义并由时间相近推硬件配对。:31总称12个区间/4种序列，表却只列9行，vendor精确大整数/BLOB/歧义filter未完整交付。错误留在最终答案，不是仅中间草稿问题。

系统因果链可复现，不认定模型波动：通用附件2KiB头+1KiB尾在本源19,355B中仅预览到排序为0的NULL时间行，尾落编码块碎片；198–200显示shown=1。语义投影虽正确标measure_interval，时间教学却仍称process_measure_interval。已登记measurements能力没有由已解析family/manifest库存生成同源同窗建议；搜索的紧凑语义又不含filter_id、引用状态及SQLite存储类，无法等价替代保真载体/三表。

748/777 analyzer还将“1到2秒”填为枚举2项，两次提交唯一gpufreq bucket；但779–780受理回执明确因缺当前请求provenance丢弃bucket，不能把未生效分类当作错误路由的已证因果，枚举2项仍受理。analyzer首轮57,539估计tokens；explorer71,128→95,702；finalizer65,915→69,043。三份inventory重复大量manifest/capture caveats。1次finalizer拒绝、1次patch，没有原值到原生表的自动纠偏。下一片按类型库存/能力目录/来源代次/精确窗口提供有界候选和原生交接，不扫描用户或答案关键词，不继续让用户描述系统守护条件。

## 事务：数值正确，关系图仍未验收

[主答案](results/hmc_measurements_obligations_20261009/trace_transaction_handoffs-20261009-023244/run-1.primary.md)、[完整日志](results/hmc_measurements_obligations_20261009/trace_transaction_handoffs-20261009-023244/run-1.logs.all.log)。本节行号统一对应all.log。

- 1349–1350：一次transaction_handoffs查询[1,1.05)，原载荷为归档目录`tool_payloads/20261009-023246-000-2198/trace-query-result-5e6aefa7.json`：7键、5提交、4物理消费；3唯一/1歧义/1缺消费/1缺提交/1身份未明。
- 2220–2227：finalizer收到3条精确observe交接，[102,8] 1.003→1.020、[101,7] 0.999→同一个1.020消费identity、[101,12] 1.040→1.050；2233–2261提供正确5/4及完整逐键事实。不是缺证或原生结果丢失。
- primary:1/:7–15/:34–42事实正确，窗外/歧义/身份边界保留，修正上批把tid999读成seq999。**:17–26只有两个孤立角色节点及空subgraph、零边/时间/事务；:28却说当前证据未证明请求的有向关系。完整人工FAIL。**

图退化过程：1472/1583/2111–2117图恢复Required，§214修复真实命中；2401首稿短节点ID与锚的人类标签失配，还画了不支持的歧义/缺端箭头，2410正确拒绝。错误同时将“应用/渲染服务”定为missing_unproven_boundary，未承接原生实例对业务角色的覆盖。2555–2558重复failure_ref删除未暂存；2604–2607全删关系，2643–2646再删12孤点。2706–2708、2850–2852、2898–2900中文节点ID被unsafe/used/family复合错误反复拒绝；2945–2950改ASCII后无边图被接受；2993–2997仅补表格principal归属，3020报告required=2/covered=2/unproven_boundaries=2。是主动删边后合同接受，不是renderer吞边。

确定性接口问题：公共图补节点辅助器只接受ASCII标识，动态schema只声明长度，复合诊断让模型误猜subgraph重名。下一片统一语法适配/准确诊断，并补“业务角色→原生实例关系→图覆盖”接缝；保歧义/缺端/身份未知不能冒箭头，不强迫用户使用内部命名规则。

## 有效请求义务、范围与计账

事务本次原route（92/321）已为answer/external_artifact，接受的analyzer（908）省略解释profile，并非false；962/2213均required=false/allowed_optional。**不得声称本批false精化分支live实命中**，该分支由公开正反/race证明。只查询一次、没有源码重调查，但不能把全部改善归因于本批实现。

824/870有两次图参与者provenance纠错；首轮analyzer仍约58,740tokens。1416要求模型再手写member_set，1471写错6提交，finalizer已由原生修正，不另判最终数字FAIL。成文11轮、9拒绝/10patch，context52,142→88,611，图修复约291秒。核心运行439秒，runner含外围总442秒，两种计时不混用。

完整能力累计19/79，本批父项新增0、仍60开放；通用原值查询/原生表切片及请求义务确定性修复已实现，但自然问答不签通过。5稳定验收父项与本批2份人工FAIL有交叉，不相加。未运行只读测试登记/写模式，不替旧FAIL签绿。失败按01.3/16.4/17.7、12.5/18.4挂原ID，不新增重复任务膨胀计数；下一优先完整能力候选为只读登记来源终态，高影响系统缺陷为来源驱动能力路由/保真投递和共享图覆盖。
