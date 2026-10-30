# HMC320：业务事实交接、原合同验收与真实ftrace用例登记

日期：2026-10-30 UTC。持续main，基线a7705a69528472e962cae976d35dc2082c713eea；实现提交d92ca379098e767822bcb12ea2629b14b7692db0。紧邻文档提交及普通推送的精确SHA由[publication](results/hmc_business_representative_20261030_v1/publication-v1.json)校验。代码/模型输入冻结后不再改写；本文是验收记录，不是追跑模型输入。

## 1. 结果与计数

79个稳定ID初始26交付/53开放。本批仅HMC-04.2满足原有限完整合同并关闭：**27交付/52开放，18类仍有开放项**。18部分、29待实施、3待验收、1验收中、1持续；各类开放数依01–18为2/1/1/4/2/2/4/6/3/2/1/5/3/5/3/2/2/4。

| 交付口径 | 本批结果 |
| --- | --- |
| 完整能力交付数 | 1：04.2通用嵌套marker树原有限合同 |
| 已交付有界子能力 | 2组：查询发布记录量尺；已展示同实例完整字段交接 |
| 剩余验收问题 | 4组，挂原ID：代表规格、求和教学冲突、窗口/端点与重复展示、解释适用域/措辞 |
| 新真实材料用例 | 2个Donghu case，均NOT_RUN，不计完整能力或二进制验收 |
| 原始机器/结构结果 | A机器PASS；B原结构审计实际exit1/FAIL；两CLI exit0不等于机器2/2 |
| 人工结果 | A原有限完整合同PASS，带保留警告；B原11.2完整FAIL |

实现交付不代表全部正文无瑕疵，也不倒签旧FAIL。04.2不新增必须输出Async CPU投影、可选self小计或特定图语法的退出门；其他稳定勾选均不变。详见[独立人工审计汇总](parallel_selected_summary_hmc_business_representative_20261030_manual_audit.md)。

## 2. 参考发现与共享修复

实际只读参考HarmonyOS_PerfMcpServer-main的marker_ops、l3_markers及closure/select、full_package/full_merge，并读取对应指引。marker树依同itid实例/parent关系，self扣直接子区间，状态按实例求交；选择清单与实际执行/未选名册分开。借鉴的是原生身份和当前代次先行，不复制简单子时长加总、缺状态补零、显示名称当身份或后查结果反写旧计划。

两个公开缺口：同一物理实例在多查询中的发布被误叫多个实例；本轮已完整展示的background span有self5.5，但成文只收到部分字段，未知调度被扩大为self未知。修复在共享交接层，不扫用户或答案原文：

1. 原主lane保完整ObservationRecord去重与16条预算，仅纠正记录单位；不按名称合并查询或删除合法重复来源。
2. 新独立16条field lane只补本轮已完整展示的span。保新鲜量测合同的原始注释，整表rows/notes完整、同代同源同query的tree/span两个完整predicate库存先验真，再按owner、物理路径、起止行/时间、名称唯一匹配。已展示背景仍是背景，不获得主目标/因果/查询完成权限。主lane为空也保这片；状态未知不抹已知时长。

没有放宽原生decoder；装饰后的账本不重签原始来源。纯正文仅复用producer已有私有库存，不伪造DiagramHint。15个负控覆盖缺库存/错误表借selector/部分展示/跨源跨query/旧代/JSON/重签等，正向保同名不同闭合实例。原4及16+4计数、精确去重、Trace因果投影/自动补齐、闭合睡眠和31/35量尺保持。

公开复现历史如实保留：v1枚举编译前提错；v2计数RED但字段因请求已含900而NOT_REPRODUCED；v3请求只接受700后真正双RED。green-v1/diagnostic-v1暴露原始注释与编译显示别名接缝，终版由同一次原合同保留detached annotation，未松门。冻结前只修改新测试前提及旧测试两处单位文字，不改原数值断言。

## 3. 定向、冻结与全仓

| 检查 | 实际结果 |
| --- | --- |
| 代表规格基线，2包 | 63顶层/96子例PASS |
| 业务交接最终公开回归 | 11顶层/17子例PASS |
| 相邻保护 | 131顶层/376子例PASS |
| 业务交接race | 11顶层/17子例PASS |
| 因果投影及自动补齐保护，3包 | 64顶层/293子例PASS |
| 唯一全仓 | 98包PASS/12无测试；28587顶层/33947子例PASS；原43/14 SKIP；0FAIL |

标准CLI只构建一次，08:28:50.549893Z–08:28:58.209776Z，SHA7751633e732637c286e584c07077e83054b384911758aead936fb31b92c0bd8b。准备及23项零模型自检通过；冻结7526仓库输入/221外部/20仓外快照，SHA08f72c2e9928bb3145aa546aa29d3dab5b56f91d95254b7afc2b2475b72f3bdd。其后源码、测试、注释、helper、case、manifest停写。

全仓真实child40990于08:30:29.446548Z启动、08:42:07.945571Z正式exit0，source_verified_after=true。命令为go test -json ./... -count=1 -timeout=30m，仅一次。263425行日志SHA35735cc952fc252143919b95792717edbb7d25c76c59dae2045ad699a9d3a8a9，零非JSON/重复终态/未终态。见[全仓终态](results/hmc_business_representative_20261030_v1/sealed-v1/0220-codrax-hmc320-full-v1-receipt.json)及[独立统计](results/hmc_business_representative_20261030_v1/sealed-v1/0219-codrax-hmc320-full-stats-v1.json)。

## 4. 恰好两例并行真实评测

A保持原trace_query_business_tree.case QUESTION/EXPECT、15步/n1及1481B authored导出文本；B保持原代表题、50步/n1，四个只读SQLite副本来自两个既有fixture，无预注catalog、工具顺序、oracle或参考答案。不是四份新设备实采，也不是新Donghu模型回放。无第三例、无失败追跑、无原断言放宽。

| 侧 | 实际进程终态UTC | 原机器/结构与人工 |
| --- | --- | --- |
| A child40993 | 08:30:33.138572–08:35:07.204738，exit0 | 原机器PASS；原04.2有限完整人工PASS |
| B child40994 | 08:30:33.141488–08:33:58.168465，exit0 | 原公开结构审计08:36:43.281373–43.936131，exit1/FAIL；原11.2人工FAIL |

真实模型请求最大可证交集**14.129秒**，不以启动barrier代签。A15请求/14响应、4唯一闭合配对、1未闭合事件及歧义组留存；B19/19唯一配对，日志非法UTF8原字节保留。不能猜最近邻配对，也不能由欠一条响应日志断言已终态CLI仍运行。见[实际请求证据](results/hmc_business_representative_20261030_v1/sealed-v1/0179-codrax-hmc320-actual-request-overlap-v1.json)。

A 11次模型query全成功＋1次系统补采；9份去重payload不是9调用。完成2拒1受理，答案1受理/0patch。实际INIT03:465为12条查询发布，467–473为3条不同查询的同物理背景实例字段补全；两新正向分支自然触发，其他负控分支不冒自然覆盖。最终三层inclusive10/5/1、self5/4/1及状态、Async700 cookie41 7.5ms独立、background900 self5.5/调度未知、三节点两contain都保真，04.2原有限合同通过。

B自然discover明确提交window_stats；编译器没有丢失已提交的resource_stack请求。原结构审计使用同一次实际成功status的saved_index/selection_snapshot及SHA，不从latest拼接，真实失败为cluster specification or full membership differs。4个已选window_stats及4个独立resource_stack查询都成功，后补合法却不能倒签原选择view。成功status当时11记录/7attempt（6success1empty），第8查询后最终12/8（7success1empty），四未选保not_executed；实际导航支持当前计数，不制造错代缺陷。资源5事件/14帧carrier正确部分保留，11.2不关闭。

## 5. Donghu真实原件的认真应用与边界

只读清点/Users/han/opt/donghu/基线数据：75份ftrace、15组各5份，共9,995,542,762B。完整读取、哈希并独立整数微秒/发射线程栈校准两份代表材料，新增[来源清单](fixtures/donghu_real_trace_manifest.json)及两个自然窗case；三文件合计10879B，未把原件复制进Git。

| 原件与新case | 可复核结果 | 身份边界 |
| --- | --- | --- |
| 100版抖音极速版后台销毁后冷启动；[bindApplication](cases/real_traces/donghu_v100_douyin_bind_application.case) | L61440–167407，236627.820748–236628.490099；inclusive669.351ms/self227.101ms/子并集442.250ms | header TID/TGID57874，payload PID10266 |
| 105版元宝容器重启后冷启动；[同名activityStart实例](cases/real_traces/donghu_v105_yuanbao_activity_instances.case) | L154186–158632为59.695ms/self6.770；L172560–183577为111.487ms/self3.938 | header TID/TGID46571，payload PID2227 |

两原件SHA分别93b45bcca5c9e9b415478d81e176b633d70886328c49731b5258c806695de1ef、be59c0648ac7e1d8a2d117f5dc36efa0c92b5f3686671719ada435c16713a20c。六个B/E物理端点独立核验；每个完整目标流各1个所选实例外孤立E，0 payload冲突/0尾部未闭合，如实记录。header owner与payload PID分列，不猜宿主/容器映射；self是墙钟而非CPU。现有parser保SpanPID，树/状态使用header owner，尚无已证此处产品错绑。

**两例NOT_RUN**，只完成原件校准、case语法和来源审查；未加第三模型、未执行额外生产查询，不称新二进制通过。下一批可轮换，不能用case创建数抵完整能力数。源码设计及未覆盖边界见[只读审查](results/hmc_business_representative_20261030_v1/sealed-v1/0193-codrax-hmc320-donghu-source-review-v1.md)。

## 6. 剩余问题与ROI

1. 16.4/18.2共享规则适用域：A实际INIT01:55/60仍绝对禁跨行求和，与INIT03:452同源互斥self许可冲突。先修结构化量尺许可与旧教学适用域，不禁合法计算，不扫答案替句。
2. 16.4/18.2窗口与展示：A重复timeline/原生端点标题未知；B将含右端的window_stats EventCounts和半开resource_stack统一标半开。数值端点相同不意味着同一范围；不改合法原查询总体/端点实现。
3. 11.2自然规格原合同：代表入口确实触发但view错误；保当前执行归属、未选名册、合法额外查询/重试，不能用正确补查抵失败。优先定位可证明的结构交接缺口，不按题面关键词硬路由。
4. 18.2措辞：A“这三个关系”实为三节点两边、Async线程CPU指代欠精确；B零匹配与总体未知句缺范围。保原事实和警告，不把非阻断措辞变无限新退出门。

完整能力槽轮换两真实Donghu例和18.5只读登记负控；17.6仅新增真实格式/工具/宿主材料具备时扩验。旧IO、采样、权限、Windowed自然未触发、§310扩大race及18.5反馈债继续原ID。WAL未零副作用前生产NO-GO。

## 7. 保真与发布

当前**447件**按canonical source+SHA封存，[索引](results/hmc_business_representative_20261030_v1/sealed-index-v1.json) SHAdf3e105aa72ce3666e30c3bf4adc9b72c72938ec5ebc7dccdc48eb601c762e7b；[封存收据](results/hmc_business_representative_20261030_v1/sealed-receipt-v1.json)。旧21档6728件/450快照来源身份逐字节保真，18历史污染为子集，不递归重抄旧档。失败、中间前提错、未触发、实际INIT、参数、原生payload、MD/HTML及双审计均保原件。

源码快照在仓外且归档后以.go.txt保存，不回流可编译源码。原日志非法字节不清洗；HTML仅源码审计非像素QA。凭据不读取/哈希/复制。447件本地原始证据未冒称Git上传；Git包含实现/测试、两个小case、来源清单及5文档。

[根裁定](results/hmc_business_representative_20261030_v1/sealed-v1/0421-codrax-hmc320-root-acceptance-v1.md)只授权04.2。后置publication验证两笔紧邻main提交、冻结字节、79唯一ID且仅该勾选变化、remote main与HEAD一致和工作区清洁；它本身不推导模型PASS，不修改B/旧失败的原 verdict。
