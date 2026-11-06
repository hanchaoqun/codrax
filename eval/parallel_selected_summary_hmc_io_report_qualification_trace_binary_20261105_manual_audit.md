# HMC357 — 两例真实评测人工审计

各原自然题一次，机器 2/2 PASS、完整人工 0/2。数值、结构化交付、范围和解释分开判断，不销账；[冻结/测试/ROI 汇总](parallel_selected_summary_hmc_io_report_qualification_trace_binary_20261105.md)。

## A：网易云完整启动窗

独立扫描全部 559,592 行，核 15 行闭合物理区间、直接子区间并集和自身墙钟；用原生时序再核每行业务的运行/睡眠。`bindApplication` 的原行 77708/152774、真实区间 `[5969.994941,5970.622586)`，627.645 ms 总、194.581 ms 自身；运行 412.174、就绪 11.069、睡眠 200.474、未标记 D 0.099、I/O 等待 3.829 ms。3735 状态行的时间坐标、原始行、状态均对应本轮原生数据。六张表 Markdown/HTML 在规范化行内代码后相符；无图或像素验收。

正文却把其他线程的 5969.961003/5970.688457 当同一业务的原始端点，把相互嵌套的睡眠区间写成约 170 ms，并由观测并行度外推设备供给充足。保留正确的邻近 Binder/JIT 背景边界、零闭合不等于零等待及无根因旁路；不以尾注赦免误述。调查选中的原生时序被自动补充成 3735 行，623,192 B 输出；原已有同表复用有效，仍需整组展示预算。就绪表标题称端点未知，表内明确半开，按共享窗口来源修。实际 INIT 未投 I/O 指数限定，不冒自然覆盖。

[完整手审](results/hmc_io_report_qualification_trace_binary_20261105_v1/sealed-v1/0139-codrax-hmc357-A-audit-v1.md)／[实际 INIT](results/hmc_io_report_qualification_trace_binary_20261105_v1/sealed-v1/0148-log-01-finalizer-init-03.txt)／[原答案](results/hmc_io_report_qualification_trace_binary_20261105_v1/sealed-v1/0082-20261105-180134.233-15879.md)。

## B：真实原件 20 ms 分段采样

独立全件 oracle 与三次当前 20 ms 原生结果一致：10 段，两段各 82 条，权重分别 85/174，八段对该线程无采样；164 条/259 cycles 权重不是时间。用户表、两热点和附带质量/元数据的五张 Markdown/HTML 表相符，无调用箭头与根因旁路。正文仍把符号命中写成执行回调路径，保留旧解释 FAIL。实际 INIT 只投第二次成功调查的三单元中第 0，描述采样分布并披露边界，不把失败稿当成污染。

明确系统缺陷另记：表头原件与准备材料区分正确，表脚却将准备的 `capture.tracebundle.json` 标作原件；14,549,907 B 是准备载体而非 30,062,585 B 原件。两次 `empty_selection` 拒绝都经合法选择恢复；第一轮成功仍被 `missing_origin_lanes=current_source` 触发无关验证，共 9 次本轮实际查询。分析结构明示采样分布和成员清单，无源码问题；优先修任务到来源义务的交接，而非放松真正源码要求或合并不同过滤。有限样本路径和统计通过，不等于完整二进制、BRBE 或 SPE 交付。

[完整手审](results/hmc_io_report_qualification_trace_binary_20261105_v1/sealed-v1/0160-codrax-hmc357-B-audit-v1.md)／[十段独立 oracle](results/hmc_io_report_qualification_trace_binary_20261105_v1/sealed-v1/0187-codrax-hmc357-binary-oracle-v1.log)／[实际 INIT](results/hmc_io_report_qualification_trace_binary_20261105_v1/sealed-v1/0169-log-01-finalizer-init-03.txt)／[原答案](results/hmc_io_report_qualification_trace_binary_20261105_v1/sealed-v1/0078-20261105-175920.968-15880.md)。
