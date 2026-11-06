# HMC357 — I/O 量尺和报告闭环／整件启动窗与真实二进制分段

2026-11-05 本地，`main`。基线 `aeab72e3ed3d06125b61e8cbdbe25e07267b202a`，源码 `06972da83780a5241bba98817fcb2d2a7a2ff91e`（13 Go，396 增、100 删）。核账 79 唯一、29 完整、50 开放；17 部分／29 待实施／2 待验收／1 验收中／1 持续，所有 ID、状态与勾选不变。**本批完整能力交付 0、已交付子能力 1**：I/O 活动指数缺测语义及同源报告限定。**剩余验收**：本批两例整答人工 FAIL；旧 I/O、采样、栈、图/权限、只读矩阵、原件身份与结构化范围欠项不销账。原 [§356 失败记录](parallel_selected_summary_hmc_io_activity_qualification_registration_20261105.md)和原始产物保持不变。

## 共享缺陷轨：报告跨消费者闭环

§356 已公开复现原生工具/成文把未计入的 I/O 操作数显示为零；漏覆盖 `tracediag` 旧报告断言导致冻结失败。§357 再用冻结旧候选 CLI 的真实东湖 `DH-IO2` 和只有阻塞标记的原始行复现：generic 480 B 值裁剪将必要边界截断。把生产者同一指数的数值账户、计数比较方法和解释限定分为有界段，离线报告仍同一行输出，原每步和总行预算不变。原生 JSON/数值、排序和背景权限保持，无模型 schema 新字段或问答文本硬门。

真报告保 `d_state=3.859ms`、两种 `0.936ms`、438.195、churn 2167、inode、原线程与原始事件，同时钉住 `io_wait=not_accounted`、完整限定和禁止裸零；独立可证明的实测零及极小正值均保。借鉴参考 `core/preprocess/io_ops.py` 的重叠桶与发起桶人口分离、`uninterruptible_ops.py` 的原生状态与 caller 约束；不复制近似桶、缺值补零或将背景晋升为因果。保持明确查询窗、原四根因/闭合睡眠、链上业务线索、自动补齐和合法计算/重试。

[修前两项 RED](results/hmc_io_report_qualification_trace_binary_20261105_v1/sealed-v1/0229-codrax-hmc357-report-red-v1-receipt.json)／[修后绿](results/hmc_io_report_qualification_trace_binary_20261105_v1/sealed-v1/0226-codrax-hmc357-report-green-v1-receipt.json)：4 顶层/3 子通过。相关 5 包 111/102，离线全包 210/166，race×2 5 包 22/64，整件实际 query→Finalizer 1/2 PASS。用户完整网易云 82,838,676 B/559,592 行、元宝 63,659,046 B/412,546 行的原生窗口统计及目标线程状态对 §356 逐字段一致，因果投影保持；零模型整件测试不当第三例。

[冻结](results/hmc_io_report_qualification_trace_binary_20261105_v1/sealed-v1/0204-codrax-hmc357-frozen-v1.json) 7653 输入/66 外部/13 快照，SHA `d1a2f96102b21ceaf7826f1674744ef31e2b426f469b2d0d0e32ba8737f2a6e4`，冻后停写。唯一全仓 UTC 01:55:02.640156–02:09:11.283251，OS0，源前后一致：98 包 PASS/12 无测试、28783 顶层 PASS/58 SKIP、34635 子 PASS/14 SKIP、268761 纯 JSON、FAIL/重复/未终态 0。[精确统计](results/hmc_io_report_qualification_trace_binary_20261105_v1/sealed-v1/0205-codrax-hmc357-full-stats-v1.json)。

## 完整能力轨：两个原自然问题，各一次真实并行

同一冻结版本，原问题、原附件、n1/15 步，独立 oracle 不喂模型。唯一实际完成请求 A15/B14、歧义/未配 0、16 对重叠、最大 30.884 秒，[时间配对](results/hmc_io_report_qualification_trace_binary_20261105_v1/sealed-v1/0183-codrax-hmc357-actual-request-overlap-v1.json)。两例机器通过，不以进程或机器替人工，不追加第三次。

| 自然例 | 已核事实 | 完整人工及欠项 |
| --- | --- | --- |
| 网易云完整启动窗 | 15 个物理实例的总/自身/运行/睡眠正确；目标 627.645 ms 分区及 3735 状态行保真 | FAIL：原始端点串其它线程，嵌套睡眠重复相加，低排队外推整机无供给问题。自动表过大、窗标题和表内端点说明不一致 |
| 原二进制分段采样 | 原件 30,062,585 B；10 个 20 ms 分段逐格对参考 oracle、164 记录/259 权重、两热点 | FAIL：样本命中升执行路径；原生表脚把准备清单标原件；无关 current_source 义务触发二次探索 |

没有激活根因/图合同，旁路未输出候选。A 原生查询有新的 I/O 限定，但实际成文未选该分支；新代自然覆盖 NOT_TRIGGERED，不能用数字正确代签。§356 只读正向独立恢复有限 PASS 保持，不代全 CLI/负向或 B2–B6。详[人工审计](parallel_selected_summary_hmc_io_report_qualification_trace_binary_20261105_manual_audit.md)。

## 剩余 50 项按 ROI

对照[统一清单](../docs/design/hmosperf_implementation_tasks_20260917.md)，表中省略 HMC-；排序不是改变退出合同。

| 优先 | 数量 | 全部开放 ID | 推进 |
| --- | ---: | --- | --- |
| P0 共享 | 7 | 16.4、01.3、02.4、16.5、01.2、18.4、12.5 | 先统一原件/准备/成员与查询角色，修无关源码义务；再以同一结构传窗端点、非重叠计算及成文整组预算，保自动补齐和图关系 |
| P1 实材/验收 | 5 | 08.3、18.5、03.2、14.1、18.2 | 先有新结构证据再复验旧 IO；轮换只读登记、资源、真实格式，不追单句误述 |
| P2 复用 | 11 | 04.3、04.5、08.1、08.5、12.2、12.3、17.6、17.7、05.2、10.2、14.2 | 共享实例/帧/资源/时钟/生命周期，依真实材料和协议闭环；活跃 WAL 保持禁入生产 |
| P3 协议 | 21 | 09.3、04.4、04.6、05.3、06.2、06.3、08.6、08.7、08.8、09.1、09.2、10.3、12.4、12.6、13.1、13.2、13.3、14.3、14.4、14.5、15.1 | 提前 09.3 的 BRBE 实材子范围，先证布局、顺序、有效性、缺值与明确上限，不猜未知尾部 |
| P4 新域 | 6 | 07.1、07.2、07.3、07.4、15.2、15.3 | 媒体/Sendable/写验证，原隔离、批准和风险门保持 |

BRBE 仅预审：独立原文件边界和参考解析均为 12000 样本／11978 有栈／369677 条／894 周期未知。实际 attr `branch_sample_type=11`，条目后余 16/24/32/40 B，不按参考的尾部猜测；命令特征是长度前缀文本，`-j` 不足以单独确定 ABI。下一步保采样内顺序、有效端点/未知周期，不冒 SPE、硬件 IPC、调用路径或因果。`raw_perfdata` 当前只保分支计数并披露跳过条目。

## 保原与发布

[封存索引](results/hmc_io_report_qualification_trace_binary_20261105_v1/sealed-index-v1.json) 257 件，SHA `0e76e060712d586531a1d57a72f7d8576c268868c92dc97115a4983fb8e4fd8a`；[封存收据](results/hmc_io_report_qualification_trace_binary_20261105_v1/sealed-receipt-v1.json) SHA `3055e0eba7c2c220a523df4e1f4612c9f0261c9c839fc8df64e26bf50ebfc8c1`；[根决定](results/hmc_io_report_qualification_trace_binary_20261105_v1/sealed-v1/0182-codrax-hmc357-acceptance-v1.json) closure=none。累计 59 代 27067 件，按代非全局去重。逐件复核本代和前 356/355/354/353/失败 352，其他历史链头；不纳入凭证，大原件用完整路径、字节、SHA 引用。仅提交确定性源码与五份文档，不把二进制、运行产物或失败答案提交到 Git。
