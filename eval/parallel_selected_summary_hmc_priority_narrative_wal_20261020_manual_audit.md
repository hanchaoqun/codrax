# HMC273完整人工审计（2026-10-20）

本批恰好两例真实模型运行，各一次且并行。两例机器PASS，**完整人工0/2，不销账**。源码、CLI、输入、全过程、失败参数、最终MD/HTML及旁路均归档；本文件是索引，不替换逐项报告。HTML核对实际内容和全部表格单元格，未声称截图级视觉验收。没有实际图，不代签图能力。

| 真实例 | 确认通过的有限范围 | 人工FAIL/未触发 |
| --- | --- | --- |
| A：自然并行度量测 | 独立17行构造Trace，原生8表、全窗峰值/均值/深度分布、3ms末1ms桶、完整成员与窗内贡献均正确；旧峰值持续/桶忙时及upload/decode错配本次未复现 | 正文越权推断CPU资源充裕。最终legacy reason仍重传错误4/3峰值及旧待查状态；pure-restatement角色自然未采用。typed任务声明冲突将量测扩大为源码/flow完成义务，有实际两次flow降级及mixed-origin阻断证据 |
| B：普通命名ZIP目标线程窗口 | 完整54,122,686B设备文本→安全准备→实际查询/自然消歧重试→自动补采；100ms状态账、33段、8次唤醒/三线程两进程正确。6条选中边的原生优先级关系/口径/来源已传成文上下文 | 将1.088ms全runnable称为唤醒后等待，其中0.533ms实为4段R+切出后再调度，8个唤醒后等待仅0.555ms。正文优先级复述及新角色隔离NOT_TRIGGERED；既有bounded-runtime分支先隔离自由摘要 |

A实际源是构造样本而非实机；B是同一设备文本的操作员ZIP包装，不冒称新设备采集。独立oracle未注入用户问题；没有追加查询来修改终稿、没有第三次模型求绿。原生表正确不能抵消正文越权，原生事实到场也不等于自然角色采用。

逐项报告、真实路径清单和哈希：

- [A完整人工审计](results/hmc_priority_narrative_wal_20261020/verification-receipts-v1/process/codrax-hmc273-A-manual-audit-v1.md)，SHA `ee14134b1b7ec4dc6331933a2bf769595c272c5ab60bcc9882b326221506ee41`。
- [B完整人工审计](results/hmc_priority_narrative_wal_20261020/verification-receipts-v1/process/codrax-hmc273-B-manual-audit-v1.md)，SHA `afce0d17dd855852d27de74f1cabd5c232943d647d7b315907d74119d11a41e9`。
- A正式PID56418，UTC04:17:41.259646–04:24:34.999681；B正式PID56419，UTC04:17:41.259863–04:19:38.566518，均2026-10-21/exit0。对应`codrax-hmc273-acceptance-A-live-v1-receipt.json`和`codrax-hmc273-acceptance-B-live-v1-receipt.json`保存在同一process目录。
- [批汇总及最终验证](parallel_selected_summary_hmc_priority_narrative_wal_20261020.md)分列v1广域字段注册FAIL和后续v2登记同步复验；两个live属于v1，不倒签为v2自然PASS。

下一缺陷轨先收敛typed任务领域与源码义务、legacy摘要退路，再统一状态转换与量尺；保真实mixed源码、Trace因果投影与自动补齐，不扫问答关键词。08.4仍开放，旧采样/旧人工FAIL保持原件。能力轨有限轮换03.2、08.3及只读登记，不让同一语义误述无限阻塞其它高ROI项。
