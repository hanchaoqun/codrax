# HMC328：完整业务账户与代表终态交付（失败代，未独立发布）

本地2026-10-30／UTC2026-10-31，持续main，基线`41274552a308ce14f1a87e245cf741ae59154a29`。79个稳定任务＝27已交付＋52开放；完整能力新增0、正式交付子能力0、closure=none。本代有3组有界实现进展，但唯一全仓FAIL、两例完整人工FAIL，不能以局部通过销父任务或生成通过版publication。

后继[§329](parallel_selected_summary_hmc_teaching_compat_binary_registration_20261031.md)已另代修复测试兼容并通过唯一全仓，生产实现以81cc27c5f1f51ab6e83f606cbda251c4d126c5d7提交。该后继交付不改变本页当代0正式交付、完整人工FAIL或原全仓FAIL，不给§328补publication。

## 两轨实现与参考发现

完整能力轨推进代表选择的确定性终态交付：真实查询记账、同run多分支union与最终不可变index/selection/SHA一并交付，不再依赖模型记住额外status动作。来源授权前后复核、失败不发有效路径、同内容重复交付不改可变alias；选择与保存仍不代表所有材料分析完成。参考`core/batch/closure/select.py`的manifest/规格驱动思想，保留独立未选名册，不复制每簇固定Top常数、最新JSON恢复权限或正文裁决。

共享缺陷轨修复两类完整账户遗漏：明确物理线程时，完整业务树先完成所有计量，再在原32节点容量内优先展示该线程的已闭合长账户；没有精确线程则保原起始行顺序。全体节点数、遗漏、原父子身份及self完整并集不变。下游仅在既有唯一完整实例绑定成立时复用此排序，原无唯一实例合同保留。另一接缝为`span_window`的原生`SpanWindows`此前未进入只消费`WindowStats.TraceSpans`的typed发布层；新增普通已配对业务观察交接，不授语义机制、CPU、因果或根因权限。查找条件与原配对区间分开，端点相接仍按原入口合同，未发明半开查找门。

实际参考阅读范围与不移植项见[原记录](results/hmc_terminal_business_publication_20261030_v1/sealed-v1/0367-codrax-hmc328-reference-findings-v1.md)。吸收marker完整node_map/线程实例/状态交集与query_engine名称、单位、结果完整性；未照搬缺值归零、平面legend跨查询混用或近邻因果。Trace投影、自动补齐、链上语义和合法计算保留，未加原始问答关键词门。多配对窄窗发布、复合路径、单边查找条件等未覆盖边界继续留账。

## 验证与冻结

最终相关选集3包130顶层/258子例PASS，有限race3包30/39PASS；race后仅调整执行线程标签为“记录所属线程”，随后最终span5顶层/2子例通过，不冒race覆盖后改标签。原始RED、两次新夹具语法失败、窄查找保完整配对的错误测试前提、`/var`与`/private/var`别名夹具、阻断整个WorkDir误伤真实结果保存的夹具失败均保留；最终只纠正夹具前提，不松原产品门。

唯一CLI与代表operator构建SHA分别为`489e478680eab06296421d329b20bd6930592d3c1f0b0ebc3d9a1c0201aa4308`、`d8d4258118c5927aed5ec3cdaf14320a3e41c5b5c51524718967a9caab889b4f`。冻结7564仓库输入、264外部、32仓外快照，[freeze](results/hmc_terminal_business_publication_20261030_v1/sealed-v1/0275-codrax-hmc328-frozen-v1.json) SHA`da653c54a50892cee77026080de74034ad88d3b6071038dae1d8f56ef8b407de`；此后源、测试、注释、helper、计划和模型输入未改。

唯一full真实PID7445，UTC06:44:00.488967父spawn前记录至06:57:42.050933实际终态，exit1，signal=null，源前后核验true。96包PASS/2FAIL/12无测试；28664顶层PASS/7FAIL/44SKIP，34118子例PASS/14SKIP；266143条GoJSON，非JSON/重复/未终态均0。[统计](results/hmc_terminal_business_publication_20261030_v1/sealed-v1/0276-codrax-hmc328-full-stats-v1.json)、[进程收据](results/hmc_terminal_business_publication_20261030_v1/sealed-v1/0277-codrax-hmc328-full-v1-receipt.json)。日志SHA`724006a9fa64b36fb6be890779b094310cb0ad51a7af95f0c12752f6a25dfa1f`。

7个顶层失败：新代表夹具3处`AgentContext`值拷贝违反既有copylocks合同；工具教学的1个canonical golden和5个历史整串SHA合同未接入新增条件排序差分。后者是共享常量演进验证遗漏，不是IO数据回归或模型波动；[只读诊断](results/hmc_terminal_business_publication_20261030_v1/sealed-v1/0252-codrax-hmc328-description-evolution-audit-v1.md)。下一代用既有ShallowClone和固定literal精确反投影修复，保全部历史expected SHA/断言与golden演进流程；本代不修补或回签。

## 两例原题各一次

| 例 | 真实执行 | 验收 |
| --- | --- | --- |
| A：原105元宝ftrace，15steps/n1 | exit0、smokePASS；5查询/3INIT/1HTML | 四完整账户及原生表保真进展；完整人工FAIL |
| B：原代表选择自然题，50steps/n1 | exit0；6查询/3INIT/1HTML；唯一原operator exit0 | 同代终态实际触发并通过原结构合同；完整人工FAIL |

明确完成请求配对A17/B13、歧义0，最大实际交叠13.485秒，见[原请求区间](results/hmc_terminal_business_publication_20261030_v1/sealed-v1/0237-codrax-hmc328-actual-request-overlap-v1.json)。未加第三例、未向问句注入oracle、未用事后status制造B终态。

A使用完整63659046B原件`/Users/han/opt/donghu/基线数据/105版本/元宝/容器重启后冷启动/1、LOG_ftrace_20261019_17.17.44/ftrace-20261019_17.17.44.ftrace`，SHA`be59c0648ac7e1d8a2d117f5dc36efa0c92b5f3686671719ada435c16713a20c`。四账户total/self现在分别59.695/6.770、46.600/21.605、111.487/3.938、103.315/83.143ms；完整self、状态与31/18段收据进入实际INIT和主表。全窗174.935 Running＋4.620 Runnable＋119.302 Sleep＋3.610未标记D＋1.584标记IO＝304.051ms。原生2211时序行、24业务表、2配对表及29/11/13等待记录保真。

完整人工FAIL仍有self83.143被说成CPU运行、子并集24.995/20.172被说成填满父区间、后代混为直接子阶段、等待主导与D桶口径误述、两activity外等待被说成“无任何业务打点”。核心反证已在INIT；未展示调查unit不可误说直接污染了finalizer。完整原始审计见[手审索引](parallel_selected_summary_hmc_terminal_business_publication_20261030_manual_audit.md)。

B四物理SQLite是两份既有夹具副本，不冒四次真机采集。8声明计划＋2合法额外观察、6实际尝试，5success＋1empty；选中独立3success＋1empty，4未选未执行。5资源事件/14源帧及15表逐字段正确。同一正常终态原返回index SHA`44f7a7805ca7c1f93e85a60e2f15e5c561c57eb0427305105f8ee54d208ff750`、selection SHA`3066d0cff619d428607dce64a2dbb1bf577626cf538396d9c59c5673fea139fb`，完整未选名册和最终MD/HTML一致，原operator只运行一次。正文仍把连续深度写成malloc“进入”reserve/render；advisory解释也重传此错误，完整人工FAIL，正确表与后置提示不抵销。

## 原件与后继

旧29档11578件/716快照来源身份均已物理核验。本代525件[索引](results/hmc_terminal_business_publication_20261030_v1/sealed-index-v1.json) SHA`74c19f56ec58bfc4d986fca0b0087fd01097a3a23f853cc8b096f720eda72b59`，[收据](results/hmc_terminal_business_publication_20261030_v1/sealed-receipt-v1.json)保0/0/1进程与独立验收；封存不是通过。原大ftrace及同字节全文仅引用身份，query/INIT/答案不按大小裁剪；Go快照留编译树外，providers内容不读/hash/copy。A原日志含非UTF8，原始字节未修复覆盖。

ROI顺序：先新代闭环copylocks与教学兼容验证；轮换sample-only真二进制和既有只读登记缺终态负控；再推进有材料的完整能力与同源解释/关系权限。已有充分证据的旧采样/IO/栈关系与业务解释继续留账，不同题追绿。图、其它权限、新查找覆盖、100版抖音等未触发不补签，WAL零副作用前生产NO-GO。52开放名册与原勾选保持；本代不独立发布，后继验证不能倒签本代FAIL。
