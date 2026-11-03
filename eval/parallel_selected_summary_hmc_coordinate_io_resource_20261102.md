# HMC336：原生坐标修补候选与失败代封存

日期2026-11-02（本地；终态11-03 UTC）。main基线b23d64445e0bd4df6594914b5295965ae619a0d6。本代**唯一全仓FAIL，源码候选不发布**；只提交本批审计文档，不创建成功publication。后继修复须新代，不能改冻结原件或重跑本代求绿。

## 核账与分账

79唯一＝28完整＋51开放（18部分/29待实施/2待验收/1验收中/1持续），任何勾选未变。

| 分账 | 本代结果 |
| --- | --- |
| 完整能力交付数 | 新增0，累计28 |
| 已交付子能力 | 0；1组共享坐标修补候选待全仓闭环 |
| 原A/B机器检查 | 2/2 PASS |
| 原A/B完整人工 | 0/2 PASS，不销08.3/14.1 |
| 全仓 | 97包PASS、1包FAIL、12无测试 |

## 公开反例与候选边界

上批completion漏`trace_query:`命名空间后反复重试，旧反馈只给失败索引与全部长名册，没有关联失败项与当前坐标。新公开测试通过真实TraceQuery→先接受有效completion→提交漏命名空间/错view，证明旧拒绝缺字段定点提示，保[原RED](results/hmc_coordinate_io_resource_20261102_v1/sealed-v1/0321-codrax-hmc336-coordinate-red-v1.log)。候选在原binder拒绝后，按结构字段提供提交坐标与当前候选整对象；只用完整ID相等或命名空间边界后缀作软导航，绝不自动改ID/view或接受别名。多ID歧义、无匹配、超预算或非坐标错误退原全名册；原metadata保全名册。4KiB/16完整提示单元，字段不截断、省略计数明示；source binder、既存状态与解释所有权未改。

对照参考core/llm_contract.py的manifest所有权、索引字段错误定位和context_index的来源导航：采用字段定位与独立诊断，不照搬文本子串/结论指纹作硬门或数值容差授权。参考io_ops的桶内计数/算术均值不替代本项目半开区间sweep/时间加权在途数。5Go文件候选（3生产/2测试），原断言、case、oracle未改；合法重试、Trace投影、原4根因、闭合睡眠和自动补齐守护相关通过，但不据此冒全仓通过或源码交付。

## 相关验证与唯一冻结全仓

相关4包73顶层/293子PASS、2可选SKIP；race×2为40顶层/106子PASS；原四根因/闭合睡眠等3包21顶层/141子PASS。完整网易云82838676B/559592行、SHA12349db5bbdc5a4343360d9633c7d6c36f18c8be5f39fb64e78b53c9e345bfe4，实际1次native query/0模型，保护原627.645/194.581ms和Extract/Finalize交接；不是第三例模型或全语义oracle重验。

[冻结](results/hmc_coordinate_io_resource_20261102_v1/sealed-v1/0334-codrax-hmc336-frozen-v1.json)7588输入/209外部/15仓外快照，SHAaeac1f55102d426be2995f0fa5ed4ded973c6f0bf9d25c1215d78b47fa356528；之后源/测试/教学/helper/case/oracle停写。

唯一full实际OS1、源前后true；28714顶层PASS/1FAIL/45SKIP，34300子PASS/3FAIL/14SKIP，267045 GoJSON，非JSON/重复/未终态0。[统计](results/hmc_coordinate_io_resource_20261102_v1/sealed-v1/0356-codrax-hmc336-full-stats-v1.json)。失败是原TestHMC279ResourceStatisticsPublicSurfaces的summary/table/diagram，均在整份JSON搜索`768`后报原生单元格复制；不透明ID/元数据误报与真实复制必须新代确定性区分，**现阶段不先归因环境、波动或基线**。详[原全仓失败审计](results/hmc_coordinate_io_resource_20261102_v1/sealed-v1/0335-codrax-hmc336-full-failure-audit-v1.md)。本代没有第二次full。

## 两例各一次、真实请求并行与人工结果

原A自然IO概览、原B自然资源统计，均15steps/n1，原题/断言不改。实际请求及精确唯一区间A14/B13，歧义0/未解0，最大可证重叠40.931秒，见[原配对](results/hmc_coordinate_io_resource_20261102_v1/sealed-v1/0262-codrax-hmc336-actual-request-overlap-v1.json)，不是barrier/进程推断。

A两原生查询：统计[1,1.01)、查找[0.9995,1.0105)。4组24主数值及配对覆盖核对一致，保预先发起/跨尾/同刻交接，全部9个选中summary按原窗展示；正文仍断言“IO不是主要压力来源”“存储侧没有排队堆积”、唯一候选方向及与硬件过载不符，越过原生量尺和不完整配对边界。实际只投3/8调查单元，unit1的“排队等待存储完成”已投，其他未投错误不冒直接来源。原08.3有限/整答FAIL。

B完整构造SQLite53248B→共享准备bundle108214B→一次resource_stack查询；不是设备实采。4事件/3组/11帧，owner、时间、768/128/512 bytes及NULL/歧义帧均保真，896/512/差384有非存量边界的派生计算合法。最终同一答案仍把512释放写成256、把连续记录帧说完整展开；原生给足，不能归因字段丢失。仅投2/6调查单元；未投的调用关系与错误行引用另留，结构合法地址不是文字正确证明。14.1仍部分实施，native_hook_statistic协议未证。

两例completion皆首次完整ID成功，**本批坐标修补未自然触发**；公开回归不补签自然覆盖。两原日志有无效UTF-8但原件保留，6条actual INIT均有效；声明/参数A14/13解析（1原坏JSON）、B13/13，不把blob数量相加。A10表、B5表与MD/HTML cell诊断一致，MD surface摘要匹配，无图/patch/像素QA扩签。详[人工汇总](parallel_selected_summary_hmc_coordinate_io_resource_20261102_manual_audit.md)。

## 留账、原件与后继ROI

[397件封存](results/hmc_coordinate_io_resource_20261102_v1/sealed-index-v1.json)，index SHA0448edcc2ef78b657be214505a3b867ea2da26ca65171e15cb112dafa64b59a0；root[明确不闭项](results/hmc_coordinate_io_resource_20261102_v1/sealed-v1/0261-codrax-hmc336-acceptance-v1.json)。旧37代15289件保原，38代按代累计15686件（非内容去重），只物理重核最新335的508件及旧链头；大原件引用，不冒上传Git全部原件，providers不读/哈希/复制。没有成功publication，不为失败source回签。

后继优先：①确定性复现/修正全仓复制检查的实际适用域，保原合同与复制负控，先恢复全仓；②转换后不透明载体的受信可读预览——本次perf预检把base64第一资源时间放大100倍、3帧写2，虽后续原生纠正且错误字面未投finalizer，也已证影响多二进制入口的上下文风险；③原生行身份/调查单元/成文的复用、引用schema密度与预算；④轮换有材料的完整能力和只读登记缺来源终态。已给足事实后的512→256暂留成文错误，不无限追同题。

另[逐字段核对](results/hmc_coordinate_io_resource_20261102_v1/sealed-v1/0375-codrax-hmc336-native-crosscheck-v1.json)发现原手工expected三组subtype name状态unavailable、实际null；现有resourceSQLSubtype明确按NULL ID令name也NULL，原HMC278测试同意，属协议/预期版本一致性欠项，不应盲改生产或冻结预期。旧采样/IO/权限/代表/图/其它平台继续原ID；WAL零副作用前生产NO-GO。

51项ROI继续按原ID：P0共享7项16.4/01.3/02.4/16.5/01.2/18.4/12.5；P1材料与验收6项11.2/18.5/03.2/14.1/08.3/18.2；P2复用11项04.3/04.5/08.1/08.5/12.2/12.3/17.6/17.7/05.2/10.2/14.2；P3依赖21项04.4/04.6/05.3/06.2/06.3/08.6/08.7/08.8/09.1/09.2/09.3/10.3/12.4/12.6/13.1/13.2/13.3/14.3/14.4/14.5/15.1；P4新增域6项07.1/07.2/07.3/07.4/15.2/15.3。当前新发现挂既有ID，不把别名或每次失败另计任务。
