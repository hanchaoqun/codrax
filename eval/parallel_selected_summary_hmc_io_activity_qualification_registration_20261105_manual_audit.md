# HMC356 — 双例人工审计（2026-11-05）

两例原请求各一次；机器结论与人工有限范围单列。[汇总与全仓失败](parallel_selected_summary_hmc_io_activity_qualification_registration_20261105.md)，[原始封存索引](results/hmc_io_activity_qualification_registration_20261105_v1/sealed-index-v1.json)。**本批不销账，不发布候选源码。**

## A：原 I/O 在途自然问题 — 原机器 FAIL、人工 FAIL

- 原 `events.systrace` 1839 B，非用户整件；预览封装 1929 B，未混为原大小。实际查询 `[1.000,1.010)` s，四组汇总、8 个成员、13 个并发段，5 个独立端点组。人工核完同源表的线程、行号、实际端点、窗交集、驻留及所有统计单元格；Markdown 和 HTML 主体 13 张事实表一致，另 1 张附件表。无图、无图形像素检查；[旁路](results/hmc_io_activity_qualification_registration_20261105_v1/sealed-v1/0014-20261105-172403.049-82928.root-causes.json) root list 空，因果合同未激活。
- 以 RQ 8,0 读为反例：6 个窗内发起与 4 个和窗相交的已配对请求不是包含关系，其中一条起于 0.998 s。其完整时长 6/4/4/4 ms，窗内贡献 4/4/4/2 ms。正文却说“6 次其中 4 次配对”，还把裁窗 2 ms 写成完整耗时下界，同时引用全时长均值 4.5 ms。
- 原生缺时长、并非实测零，实际 [Finalizer INIT](results/hmc_io_activity_qualification_registration_20261105_v1/sealed-v1/0115-log-01-finalizer-init-03.txt) 已收到 `d_state=not_accounted`、`io_wait=not_accounted` 及完整量尺限定。实际上下文也有正数、身份、分母和覆盖。探索提交有四个独立解释单元；仅第 0 个带边界进入实际 INIT，却仍将正确表行外推为三个读线程和无队列，并混计数。其它坏单元和 RQ/BIO 错标总计只记接受的调查欠项，不冒进入成文。精确原生行引用验证地址，不给这些解释事实权。
- 最终结论称“没有排队压力”“串行地到达设备”并排除 I/O 卡顿，与自身末段和已知深度 2/配对不全矛盾。浅的观测深度不能断言物理队列；完成线程与发起线程只表示端点，不构成唤醒因果。保原机器 [verdict](results/hmc_io_activity_qualification_registration_20261105_v1/sealed-v1/0034-run-1.verdict)：它未匹配未配对/歧义的两项正则，正文确有“没有配到完成端点”“模糊的请求批次”的释义，不能说披露全无；人工实质 FAIL 独立于正则。保持 08.3 开放，不因正确表或尾注翻绿。

[完整 A 手审](results/hmc_io_activity_qualification_registration_20261105_v1/sealed-v1/0106-codrax-hmc356-A-audit-v1.md)／[原 Markdown](results/hmc_io_activity_qualification_registration_20261105_v1/sealed-v1/0013-20261105-172403.049-82928.md)／[原 HTML](results/hmc_io_activity_qualification_registration_20261105_v1/sealed-v1/0012-20261105-172403.049-82928.html)／[原生查询](results/hmc_io_activity_qualification_registration_20261105_v1/sealed-v1/0119-native-query-payloads.json)。后续从结构化证据绑定查询、统计对象、窗口和单位；不扫描正文、不禁合法计算，也不追跑已充分证据的偶发越权。

## B：既有只读登记自然正向终态 — 原测试 PASS、人工有限 PASS

- 模型驱动的父/独立恢复子 controller 与 planner，操作者走公开源准备与验测，不是完整 CLI/模型编写源或模型 verifier。父 `plan_batch` 缺 `batch`、子把嵌套 `missing_observations` 放到顶层分别被拒后修正，原始拒绝保留，不放松 JSON。已有 schema 把该字段放在嵌套计划内，无证据说明工具教了错误的顶层结构。
- 只读计划无改动，绑定已应用的源代次及现有 `test_value.ValueTest.test_increment`。新子 PID 86959≠父 82878；`native:153af:18e5502fb1232858:1` 不同于旧与源验证凭证。本轮 1 断言/0 失败，语法预检与现有测试执行分开；源、测试、HEAD、Git 状态不变。
- 复核 final/current/restart 三份报告同 SHA `5334a3e3d8b964022f9c24520189b6f5cbf618e1f595d12f6e63de2d69c5fcb0`。父/子归档各 7 原件、3 尝试、2 路径/报告绑定，逐哈希核验；同一中间 repair 被后续 proof 取代，不计一次独立通过。五种表现都保同六份 context pack，四个持久载体有相同六件字节，不把内存表说成又一份磁盘。最终 `all_batches_verified` 仅为本条正向合同。无答案/表图输出面，无全矩阵通行结论。

[完整 B 手审](results/hmc_io_activity_qualification_registration_20261105_v1/sealed-v1/0128-codrax-hmc356-B-audit-v1.md)／[独立恢复原件](results/hmc_io_activity_qualification_registration_20261105_v1/sealed-v1/0055-restart-child-audit.json)／[原 verdict 与进程](results/hmc_io_activity_qualification_registration_20261105_v1/sealed-v1/0131-process-and-case-outcomes.json)。
