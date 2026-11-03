# HMC336 人工审计汇总：机器2/2、人工0/2、全仓FAIL

本批source候选未发布，仅提交失败代文档；main基线b23d64445e0bd4df6594914b5295965ae619a0d6。冻结、原病例、原审计和唯一full不改，后继须新代，不倒签。

| 验收面 | A 原IO概览 | B 原二进制资源统计 |
| --- | --- | --- |
| CLI / 原smoke | OS0 / PASS | OS0 / PASS |
| 完整人工 | FAIL，量尺到物理压力解释越权 | FAIL，同源大小错写及完整展开越权 |
| 原生数字 | 4组24主数值/覆盖PASS | owner/4事件/11帧/3组768、128、512保真 |
| 实际调查单元 | 3/8整单元投递；3041B省略 | 2/6整单元投递；4376B省略 |
| 新坐标修补 | NOT_TRIGGERED | NOT_TRIGGERED |
| 图/patch | N/A，原题无此义务 | N/A，原题无此义务 |

原[完整A手审](results/hmc_coordinate_io_resource_20261102_v1/sealed-v1/0217-codrax-hmc336-A-audit-v1.md)与[完整B手审](results/hmc_coordinate_io_resource_20261102_v1/sealed-v1/0239-codrax-hmc336-B-audit-v1.md)保完整事实、原始错误、各层边界及上下文足量/不足的区分。A在途不授设备队列/压力或排除根因；B同深度/连续帧不授执行顺序/完整展开，合法大小差额不当泄漏判错。

A正文的IO排除/唯一方向是实质合同失败，不因附表正确免责；B512→256与展开完整为确证正文错误，不假称新helper已经修好。B未投unit2–4含业务调用关系和错误native row引用，不能说它们已被实际转发；已转发unit0/1有正确数量，原生表提供全部行和明确展开未知。

原case只presence检查，实际全字段审计另发现原expected的subtype_name_status三处与生产协议冲突：实际NULL随NULL ID，原HMC278及resourceSQLSubtype明确支持；不改原预期、不宣称全字段零差异，也不把此误作本批生产回归。原perf预检从不透明base64载体猜错时间和帧数，后续原生已纠正；错误字面不在finalizer INIT2/3，不能冒称造成最终512→256。它的输入可读性/受信预览是已证高影响待补缺口。

两原日志均有无效UTF-8，原件不修，collector/请求解析按字节安全；6条INIT有效，scope仅实际logged bodies而非provider wire。A14声明/13解析参数/14blob（首analyzer坏JSON保原）、B13/13/13，不相加；各1completion/1answer，无patch，无第三例。14/13真实请求均唯一配对，最大实证重叠40.931秒；不是日志最近邻猜测。

全仓唯一OS1：97包PASS、1包FAIL；失败是旧TestHMC279ResourceStatisticsPublicSurfaces三个展示分支在整JSON查`768`。现阶段不归因环境/模型/基线，需新代精确复现；[失败原审计](results/hmc_coordinate_io_resource_20261102_v1/sealed-v1/0335-codrax-hmc336-full-failure-audit-v1.md)及267045行原GoJSON保留，不能用相关/race绿覆盖。没有第二full，不签成功publication。

完整新增0、已交付子能力0、候选子片1；79/28/51不变。按[批汇总ROI](parallel_selected_summary_hmc_coordinate_io_resource_20261102.md)先闭全仓，再受信预览/同源复用、只读登记负控与有材料完整验收；旧IO/采样/权限/图、跨平台与WAL债保原，生产NO-GO。
