# HMC314 两例真实评测人工审计汇总

2026-10-29，main，冻结后只读审计。A原15步、B原50步，各n=1并行；原题、原材料、原退出门不改。细节及完整原件路径见[A独立审计](results/hmc_repair_pairing_representative_20261029_v1/sealed-v1/0131-codrax-hmc314-A-audit-v1.md)、[B独立审计](results/hmc_repair_pairing_representative_20261029_v1/sealed-v1/0150-codrax-hmc314-B-audit-v1.md)。本汇总不覆盖原机器结果。

## 1. 分层判定

| 检查层 | A | B |
| --- | --- | --- |
| 正式进程 | exit0，正常非恢复，前后源验真 | exit0，正常非恢复，前后源验真 |
| 原机器/结构判据 | run-1.verdict PASS | 原Go结构审计实际exit1，FAIL |
| 新子能力自然覆盖 | 显式native重选0patch，NOT_TRIGGERED | 8行规格/逐成员尝试交接限定PASS |
| 正确原生事实/表格 | 31/35、2ms、47背景及业务/查询窗保留 | 六资源查询/统计/帧展示限定PASS |
| 完整人工 | FAIL | FAIL：原resource_stack代表计划未建立/选择 |

完整人工0/2；新增完整能力0、产品子能力2组、评测基础修复1项。79唯一任务仍26已交付/53开放。下列剩余原因归原ID，不新增任务，不因局部绿销父项。

## 2. A：已有状态事实与最终解释冲突

实际原始材料是2312B文本Trace，2408B为预览包装，不混大小。4次模型trace_query＋2次确定性补采＝6次view执行、5个唯一payload；不是5次调用。调查首次缺confidence拒绝、第二次受理，最后一次完整答案提交accepted，0次patch；没有恢复稿或被隔离的图。

实际[最终MD](results/hmc_repair_pairing_representative_20261029_v1/sealed-v1/0035-20261029-062358.904-20049.md)与[HTML](results/hmc_repair_pairing_representative_20261029_v1/sealed-v1/0034-20261029-062358.904-20049.html)同源。下表MD行号指该完整MD，不是独立primary提取文件。

| 事实/表面 | 实际判定 |
| --- | --- |
| 三主体、业务名与原生线程 | 保留；长rt_thread ID是语法节点ID，不是另一个可见长角色名 |
| 两条唤醒及上CPU | storage80→worker200于1.040010；worker200→main100于1.045；worker1.041010/main1.046上CPU，方向与原件相符 |
| 业务与查询窗 | OpenDocument50ms=5运行＋1就绪＋44睡眠；查询52ms=7＋1＋44；LoadDocumentIndex40ms=8＋1＋31，本次未再混用 |
| 原四排名/物理候选 | 原四记录保留；主线程重复排名身份不增根因席位，三个物理候选31/1/1；允许未显式选root-causes，不增门 |
| 31/35/44与2ms | 闭合睡眠31、请求驻留35、主线程睡眠44分别保尺；1＋1=2ms合法同向小计不丢 |
| 原始段6/41.5 | 原值和组成/范围包络保真，不误作业务或单状态量尺 |
| 背景47ms | backup请求仍是链外背景，不进入目标主因 |
| 完成点与唤醒点 | **FAIL**：源完成1.040000，唤醒1.040010；MD34正文和57图写1.040010“完成并唤醒”，MD46–47正确表不能抵消 |
| 图注释顺序 | MD55较晚worker事件在MD56较早main1.001睡眠之前；时序表达欠项，不否认两条已验证唤醒 |
| 醒后区间解释 | **FAIL**：MD38/69把1.040010–1.045000称未定价交接等待；实际含已排名1ms就绪＋3.990ms运行。未知同步机制不能抹掉已知状态 |

原生INIT03:457/474已发布就绪/运行，347分别给请求和睡眠两量尺；同时605–615整体交付advisory unit[1]，仍把剩余约13ms说成IO完成后交接、把1.040..1.045说成只有sleep/runnable。原解释没有native_refs，仅有建议Trace坐标；标签“advisory”不等于冲突已消除。最终继续采用交接等待表述，因此这是已证上下文冲突，不是凭空推定模型没看见数据。

旧unit[0]含50ms/7ms错误，但被整单元预算省略，没有进入这次摘要，最终业务值也已正确；不据其未到场内容否定终稿。INIT披露1/2视图prompt发布不等于源不可读；四原生表已实际发表。受理后context-pressure HARD跳过可选修补是原实况，无隔离实验证据，不归因本批代码。

三业务角色映射仍明确未证；图的两条原生关系已验证，两种权限不混。所有Note均为合法单行，没有用户早期游离文本格式错误。四张原生展示与九张全部表的MD/HTML单元格一致；HTML源码可核，不声称已做浏览器像素QA。

## 3. B：规格交接正确，不代表语义选择正确

原自然问题比较两线程、同检查窗的分配/释放与调用位置，固定原合同为resource_stack、TID101/202、[10,10.05)、8计划/2组/4选中/4未选。实际首调用 `request_plan.view=thread_timeline`；系统保留了这个已受理值，不能说丢弃了已受理的resource_stack。

最后普通status同次三元组已由根及独审分别核原日志1183–1185。唯一原Go auditor实际exit1，`cluster specification or full membership differs`；没有成功JSON是失败结果，不是未触发。[结构收据](results/hmc_repair_pairing_representative_20261029_v1/sealed-v1/0168-codrax-hmc314-B-structural-audit-v1-receipt.json)和[失败日志](results/hmc_repair_pairing_representative_20261029_v1/sealed-v1/0171-codrax-hmc314-B-structural-audit-v1.log)保留。

| 代次/成员集合 | 精确结果 |
| --- | --- |
| 最后status | 12记录=8声明＋4观察，8attempt；原计划4success/4not_executed；资源2success/2empty |
| 最终INIT | 14记录=8声明＋6观察，10attempt；原计划仍4success/4not_executed；资源4success/2empty |
| 选中四成员 | C101/B101/B202/D202，各timeline success1次；完整规格/attempt_outcomes到场 |
| 未选四成员 | A101/D101/A202/C202，原timeline各0尝试；未选原因和完整规格到场 |
| 合法额外查询 | 6个resource observed_query_only，不能代签原计划；未选C202/D101可以查，但不改选择身份 |

资源事实均在原半开窗口，不混10.050边界事件：

| 来源/线程 | 事件/帧 | 事实 |
| --- | ---: | --- |
| C101 | 3/8 | 2申请合768 bytes、1释放512；Gallery/gallery-main |
| B101 | 1/3 | 申请512 bytes；ImageEditor/editor-main |
| B202、D202 | 各0/0 | 实际查询为空，不是未执行，也不证明目标存在或全集无活动 |
| C202 | 1/3 | 申请128 bytes；Background/background |
| D101 | 1/3 | 申请512 bytes；ImageEditor/editor-main |

实际[最终MD](results/hmc_repair_pairing_representative_20261029_v1/sealed-v1/0428-20261029-062707.943-20005.md)与[HTML](results/hmc_repair_pairing_representative_20261029_v1/sealed-v1/0427-20261029-062707.943-20005.html)正确保事件流量而非净存量、独立source/target/window、未知/重复/缺深度，不拼调用边，不由资源事件推CPU时长/泄漏/停顿因果。六个事件表及三原件元数据到场；当前未复现旧跨源C202身份借D、错误配对生命周期或负载断言。

五次调查提交前四次因unknown_id或非当前native地址拒绝，最后受理；一个不合法capture维度的可选aggregate被丢弃，两空查询仍有真实原生证据。一次完整最终提交accepted，不是恢复稿。失败重试原件保留，不称全流程首发成功。

主文分开timeline与resource进度。C202/D101原timeline零尝试不如A两项明确，仅记说明不足，不无证断言它捏造timeline成功。四代表为跨组成员，末尾明确三原件，不把模糊“份”字当四唯一原件的已证错计数。含混“C202之外的空结果”保表达债，不另设机器门。无图不是失败，原合同不要求图；HTML仅source QA。

完整FAIL仅凭原resource代表规格未建立/选择已足够。新增导航真实限定PASS、六资源取数限定PASS，不换算完整能力交付。

## 4. 评测与证据完整性

两进程、全仓均正常真实终态，源码前后验真；未用进程exit代签机器结果。实际请求13/42对，独核55对/44个正交集，最大24.550秒；本次无自然歧义，sticky关联分支由公开测试/旧只读回放证明，不冒自然覆盖。

唯一全仓98包PASS、28538顶层/33827子例PASS，零FAIL/重复/未结束；13解析器测试独立通过。474件封存及旧4475件逐字节保全，见[完整性独审](results/hmc_repair_pairing_representative_20261029_v1/sealed-v1/0249-codrax-hmc314-integrity-v1.md)、[根交叉复核](results/hmc_repair_pairing_representative_20261029_v1/sealed-v1/0445-codrax-hmc314-root-review-v1.md)和[批汇总](parallel_selected_summary_hmc_repair_pairing_representative_20261029.md)。

## 5. 留账与下一批

两产品原因组归11.2/16.4及16.4/12.5/18.2；新图重选自然覆盖归12.5/18.4。先修已证事件/状态与解释交接冲突，完整能力槽轮换只读登记原负向终态；新二进制材料出现才扩17.6。旧IO/采样/权限未触发及扩大race不删账，WAL生产NO-GO。冻结后不修改源或重跑第三例；普通main提交只证明实现已保留，不抹掉本次FAIL或NOT_TRIGGERED。
