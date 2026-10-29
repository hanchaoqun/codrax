# HMC312 综合人工审计

2026-10-29。根代理核对完整MD、真实查询/INIT和独立报告；子代理分工审A、B及原件完整性。两例完整人工均FAIL，唯一全仓也FAIL。完整能力交付0、已交付子能力0，1组候选实现待验收；79唯一任务仍26已交付＋53开放。HTML仅源/语义审计，没有像素QA。

## 1. 独立报告及范围

- [A逐面审计](results/hmc_display_representative_20261029_v1/sealed-v1/0119-codrax-hmc312-A-audit-v1.md)：真实工具、实际INIT、530行最终MD及HTML源，含恢复稿权限边界。
- [B逐面审计](results/hmc_display_representative_20261029_v1/sealed-v1/0138-codrax-hmc312-B-audit-v1.md)：27次查询、15唯一payload、292行MD及800行HTML main、精确选择/执行及结构FAIL。
- [冻结/终态/并行独立审计](results/hmc_display_representative_20261029_v1/sealed-v1/0200-codrax-hmc312-integrity-audit-v1.md)：7646项hash、真实wait退出、独立重建请求并行与全仓计数，不代签功能。
- [唯一全仓失败归因](results/hmc_display_representative_20261029_v1/sealed-v1/0194-codrax-hmc312-full-failure-audit-v1.md)：实际失败原证＋静态具体因果链，未新增执行实验。
- [冻结前代码审查](results/hmc_display_representative_20261029_v1/sealed-v1/0167-codrax-hmc312-code-review-v1.md)：当时未找到问题不是最终PASS，后续旧合同失败及归因优先；原报告保留不改写。

## 2. A：局部复用成立，完整答案失败

原问题/15步/2312B authored ftrace不变，4次实际原生查询，机器FAIL degraded_answer_checks_skipped:1。首次完整提交把runtime observation ID放入源码evidence_ids；随后4次相同参数的facet_ids为object而非字符串数组，均not staged，最终恢复旧草稿。正确schema/精确引文边界已有到场证据，尚未证明应接纳这些形状；不能放宽门来消除失败。

actual INIT03:420–422、517–518至少两个独立当前timeline selector共用measurement-content-1；完整12列/4行及注释保留，最终只有一份主线程原生状态表。三个payload都有同窗target_window_states不等于三个selector均参与；公开8查询绿不冒自然8查询覆盖。本例CPU频率、atomic图局部修补、只读登记及新二进制未触发。

| 检查面 | 事实与裁定 |
| --- | --- |
| 业务/查询窗 | 业务50ms与查询52ms、main业务5/1/44与全窗7/1/44分开，PASS |
| worker | 业务40ms＝8/1/31，31ms闭合睡眠与35ms RQ驻留区分，PASS |
| 图 | 3主体、业务首行＋线程次行、80→200→100两已证唤醒及1.041010/1.046实际上CPU点保留，PASS；未证业务角色映射仍披露 |
| 因果投影 | 原4榜单仍在；IO31与调度合法2ms小计、47ms背景不升根因，PASS；没有借同窗当链上 |
| 单位 | MD20/37称64字节，原件32768B及64sectors、INIT单位换算已到场，FAIL |
| 系统附加合计 | 主线程1ms旁“线程合计6ms”；worker31ms旁“线程合计41.5ms”，与当前原生runnable1/sleep36.5及业务31不合，FAIL |
| 完成权限 | 连续修补失败，answer-surfaces/root-causes标unavailable，最终恢复稿，不是正常接受答案 |

6/41.5冲突在上一批20261029-021651.351-11711.md:290/325同样存在，明确是旧系统缺口，不归本批新指纹回归。静态位置answer_document_mutation_runtime_tree.go:20222读取ActualImpactMS/ActualTotalMS，但没有新的公开实验证明上游如何累加；下一批先核源/代次/量尺/实例，不用固定6/41.5拟合。

已接受调查原件仍有单边优先级/CPU供给外推，实际INIT只完整交接6单元中的1个，最终没有照抄那两项。只能保留旧调查欠项，不能声称本轮终稿再排除全部反转。事实充足的64字节误述继续留档，不修改正确IO解析器。

## 3. B：选择已触发，但不是原资源栈计划

原自然问题/50步、4份只读SQLite副本、独立原oracle不变；不是四次设备实采。discover选event_search，真实8计划/2组/4选中/4未选。选中各自事件检索执行；后续resource_stack为合法extra，不能把旧event_search任务改签为resource_stack。原有限合同的资源栈线程×窗口矩阵仍未建立。

最后真实成功status同返回三元组的16记录/16尝试有效；后续实际INIT27记录/27尝试也有效。不同时间的这两组计数不是自身矛盾，不能拼接成一个终态。用真实同返回三元组执行原Go audit一次，exit1明确cluster specification or full membership differs。缺成功JSON不是未执行，不得写NOT_TRIGGERED或补成功文件；不改validSpec、不只验数字。

真实27次查询＝10event_search＋17resource_stack；15个唯一payload不能代调用计数。原窗口八个source×thread资源查询事实均到场：a/c线程101为3事件/8帧、2分配768B/1释放512B；b/d线程101为1事件/3帧、512B分配；a/c线程202为1事件/3帧、128B分配；b/d线程202为0/0且身份未知。另查其他窗独立保留，不进入原窗口统计。

| 已发表的冲突 | 证据及裁定 |
| --- | --- |
| 跨源分母 | b/202写0/21，实际0/5已在INIT03:149/151，FAIL |
| 未知主体 | b/202没有观测身份，却借c/202的background/Background，FAIL |
| 释放与地址 | a/101一释放说成两次，第三条4096地址也说全一位型，原帧/事件已到场，FAIL |
| 窗口 | 主摘要写闭区间，resource_stack是半开且边界确有事件；event_search自身inclusive不可混用，FAIL |
| 栈关系 | 深度记录被箭头和明确“调用链”升权；缺深度/重复/未知已保留，不能以尾注否定执行序抵消，FAIL |
| 执行完成 | 称四份都做事件与资源栈查询，实际d仅资源栈extra，event_search计划仍未执行，FAIL |

已接受原生事实与当前执行计数进入真实成文INIT，未找到原错误interpretation unit被直接重放的证据，故不凭草稿存在认定摘要污染根因。手写摘要内2表与另外2table blocks重复是模型自写内容，不是本批仅timeline/CPU凭证分支自然回归。原生表/metadata对未知和范围正确不抵消正文。B无图不是失败，未打印所有帧不是新增退出门；合法extra、不同limit、自然重试均保留。

## 4. 新全仓回归不得被局部绿覆盖

全仓97包PASS/1FAIL/12无测试，唯一叶子是原B1638量测域隔离针；顶层祖先和包fail只表示同一失败传播。静态链明确：TimelineResult包含MeasurementDomain → json.Marshal整体进入native摘要 → private witness落ObservationRecord → 旧针只豁免MeasurementSources后DeepEqual仍不等。该私有字段也影响显示键，属于新确定性metadata隔离回归；没有证明它直接泄漏给模型或A已错误合并，不扩大症状。

其它4子例因JSON丢producer-private timeline而未走相同分支，不是活体其它view已经安全。下一批保持旧针原样，展示等价输入做值拷贝仅去已分类来源域元数据，原JSON与MeasurementSources照存；精度/端点/状态/来源/完整投影等其它字段仍全部精确，补活体路径对偶。不能为过针删private字段比较或放宽断言。本批冻结后未实施此修复，也没二跑全仓。

## 5. 统一留账与下一批边界

5个原因组，沿旧ID继续：新metadata隔离（16.4/18.4）；旧附加合计（16.4/16.5）；代表规格与执行归属（11.2）；原生引文/JSON修补失败（16.4/18.2，先公开定位）；同源正文/表格/图重述越权（16.4/18.2）。优先确定性系统缺口，能力槽保代表选择或轮换只读终态；旧采样、IO、权限未触发不销账。没有因本批原件完整或候选推送增加交付，WAL生产NO-GO，main不变。

执行完整性：冻结7498/130/18全一致，真实A/B请求最大重叠23.744秒，原始非法UTF-8与歧义响应保留；唯一双例各1、唯一全仓、无冻结后源码/测试/helper写入。422件原件与独立报告已封存，[批汇总](parallel_selected_summary_hmc_display_representative_20261029.md)给出实际终态、索引和候选publication。封存/提交/完整性PASS不能倒签本页任何功能FAIL。
