# HMC348 原两例人工审计汇总

root结论：机器2/2 PASS，完整人工0/2 PASS；源码共享端点修复PASS、sample-only正常准备与取数有限PASS。稳定79/28/51不变，完整新增0、子能力1共享组，不修改任务勾选。没有第三次模型评测或第二次full替换失败。

原始逐项主审：[A](results/hmc_producer_contract_netease_binary_20261104_v1/sealed-v1/0250-codrax-hmc348-A-audit-v1.md)、[B](results/hmc_producer_contract_netease_binary_20261104_v1/sealed-v1/0272-codrax-hmc348-B-audit-v1.md)、[root裁定](results/hmc_producer_contract_netease_binary_20261104_v1/sealed-v1/0294-codrax-hmc348-acceptance-v1.json)。全部原始日志、JSON、实际INIT、提交参数、HTML/MD和失败工具输出由[索引](results/hmc_producer_contract_netease_binary_20261104_v1/sealed-index-v1.json)反查，不由本汇总文字替代。

## A：完整网易云启动

原题TID36434、5969.994941..5970.622586，82838676 B/559592行完整用户ftrace，没有切片替原件。15steps/n1、UTC10:06:11.867513→10:10:29.352128，OS0/源前后true。原机器只是非空答案smoke。3原生query/3JSON、17声明与17参数文件分列，不加成34工具。

独立B/E LIFO整件核对原live32个业务节点：原名、actual_start/end、inclusive/self墙钟、所有直接子数量相等，0差异。根627.645ms/直接子2131/子并集433.064ms/self194.581ms；原独立调度oracle给Running412.174、Runnable11.069、S200.474、D3.928ms。原生D拆0.099未标IO和3.829标IO，不能当子集重复加。self为Running122.521、Runnable1.498、S70.312、D0.250ms，1900段只预览16/省1884，但全部直接子均已扣除。

本批11组16成员冻前原始行核对通过，live actual INIT第三条296989 B中11个新selector全部显示半开`[5969.994941,5970.622586) s`，preview_omitted_rows=0，端点遗漏自然闭环。最终emit没有选择runtime_measurement新成员表/图，只自动保原生事实；公开三种surface绑定绿不代本例自然触发。

完整人工失败证据：

- 外层attachBaseContext睡眠50.090ms包含内层46.424ms，正文仍称“合计约百毫秒”。这是实例contain关系上的重叠相加，不是两个独立阶段。
- Verify dex22.183ms内有S0.383/Runnable0.040；Resources#getResources self33.304ms内有S16.804/Running15.289，正文把这些墙钟self称“纯执行段”错误。不能以业务名或self==inclusive判断CPU语义。
- 对相加的限制过度扩张：祖先/子孙inclusive不能加，但根self194.581＋所有直接子区间并集433.064＝627.645是合法、必要的账户关系，不能一律禁止。
- D/调度IO总量3.928本身正确，未评估IO完成闭合的目标等待却先说“真正IO只有/基本无影响”，和末尾正确未评估边界冲突。原题不要求完整因果诊断，不加root_cause_rank/源码义务制造失败。

实际Finalizer已给精确父ID、状态交集、完整self扣除语义与未知边界。上游reason6660 B有错行，但原文及关键错行本次未传；本例没有native_fact_handoff解释单元，不能把终稿错误冒作已转发旧reason污染。背景JIT/VerifyClass明确只作同期负载，没有升主因，这是通过项。

HTML3764746 B/MD635601 B，14表含3735条完整状态时序，主区638574 B/53295行。表单元源码诊断一致，0Mermaid/图/SVG；无像素QA和局部图修补。完整输出预算/同源复用不闭环。

## B：原sample-only二进制

原题TID1609、[486489.4,486489.6)，完整30062585 B test_brbe.data，15steps/n1、UTC10:06:11.871560→10:08:53.884419，OS0/源前后true。6原生query/6JSON、9总声明/9参数分列。

模型前参考解析器实算12000全记录/1393窗口记录/164目标/period259，CPU0/1各82与85/174；ns首末分别[486489464088122,486489465612497]、[486489588095622,486489589609372]。完整原件→共享准备产生12000样本、capture.perftrace及11596 B bundle；原件/预览14549907 B不混。没有把bundle或预览当完整成员，没把无trace body的thread_timeline空结果当真实零运行。

最终两个热点数字/身份/独立时间包络正确：内核IP82/174/67.18%、OnDataEvent_82/85/32.82%，记录CPU1/0。event_search目标164/returned40与内核地址宽查询82/returned10各自分母保留；期望总量来自原生聚合/采样树，不从预览外推。cycles权重未换算ms，无停顿因果结论，原有限取数PASS。

完整人工FAIL：终稿既承认sample_kind未知，又声称“只在线程占CPU期间取样”“停顿本质线程离开CPU”和“CPU用在哪里”。未知不是on/off-CPU，停顿也不能被一概限定为off-CPU而排除运行时供给不足。事件名、记录CPU、权重和正确数表均不授执行态或机制权限。

上游受理6单元，实际第三条INIT219399 B只完整转unit0/1；桶界冒包络unit2和“CPU时间”unit5未投。原生unknown、非时长、时钟assumed等限制在实际上下文中存在；不把未投错误算污染，不以精确引用地址认证文字。此类充分事实后的旧模型误述继续留账，不无限重跑。

四块原emit无原生selector，summary内手写热点表与table重复；HTML2795995 B、MD13875 B四表＝附件/热点/热点/元数据。源码单元一致、无图/像素QA。已闭18.3不再算新增完整能力；其它二进制格式/工具/平台、只读登记终态、代表/权限/图分支本轮不签。

## 共用判定与后继优先级

精确请求区间A16/B7，0歧义、最大真实重叠33.115s；不以同时启动或OS0证明真实并行。只有原两例各一次。唯一full98包/28756顶层/34534子PASS，53/14SKIP、12无测试包、268285纯JSON，零FAIL/非JSON/重复/未终态；源前后校验true。源码与测试冻结后未改，文档与root手审允许后置汇总。

剩余4组均保原ID：事实适用域/状态与合法集合运算；最终跨块同源复用/整组预算；有材料二进制/只读自然正负终态与代表/权限/图覆盖；其它协议/新域/WAL。先做已证共享缺口，再轮换实物能力；不通过关键词门、强制view、删正确事实或一律禁算术追绿。Trace投影、自动补齐、原4根因、闭合睡眠及链上业务语义不损。WAL未满足零副作用前生产NO-GO。

521件index SHAeddba9aa6204d4b24b397e02d67af3ee3a34ccc65a9a42f150dee73f0ed480af、seal SHA390d96a0b9b12cfb784989425b0570dfa3548e1dd366546875736cec22508bbc；本代加前49代合计50代22407件，按代非全局去重。失败336/340/341/344等不补成功publication；源码/五文档/远端同代另由[发布绑定](results/hmc_producer_contract_netease_binary_20261104_v1/publication-v1.json)证明，不声称所有原件已上传。
