# HMC239：采样身份与 sample-only 二进制运行摘要

起点 `8208fd17b`，79 唯一任务：21 实现已交付、58 开放。完整能力本批新增0；旧失败不倒签。

两个自然问题各运行一次，19:56:31 / 19:56:32 PDT 启动，实际模型时段重叠。共用标准make冻结版本 `87e74451e171-dirty`，dirty仅来自文档。操作员oracle未提供给模型，不修改问题要求模型背诵系统约束。

| 例 | CLI退出 / 耗时 | 机器结果 | 完整人工结果 |
| --- | --- | --- | --- |
| A 无损采样路径 | 0 / 137s | PASS | FAIL：正文合并跨来源分母、遗漏未知栈、误述采样量尺 |
| B 完整原始 sample-only 二进制 | 0 / 124s | FAIL `data_terminal_missing`（评测配置错误） | FAIL：正文状态解释越权、系统逐热点质量错配 |

合计机器1/2、人工0/2。两个模型问题都完成取数及答案；B不是“模型前失败”。`406b25d27`修后续case的read fixture选择，通过语法/配置及完整runner脚本测试，不修改原FAIL，不追加第三次真实评测。

## 已验收子路径

- `123c1ccf7`：共享采样总体区分未知身份与真正零样本，保来源范围；真实finalizer保留精确查询不可用/空结果，不授主体或因果身份。五种公开查询入口、跨源/窗/代次/角色负控及race通过。
- `7f5133fcb`：已有typed事件投影唤醒目标、切出、切入线程/进程字段，摘要、原生表、无边事实图同源；未知不补零，显示不授owner/因果权。相关六包和结构保护通过。
- `87e74451e`：完整30,062,585B原始perf二进制实际默认准备为12,000记录，再由真实模型同窗查询poll_report/TID1609得到164样本/259权重。两个热点82/174与82/85正确；无调度记录保持未知，系统Trace投影与自动补齐仍运行。

以上不代表采样完整答案、唤醒目标有限清单、合法idle0或全部二进制能力已交付。子能力不按提交笔数重复计功。

## 原始证据

- [A 机器收据](results/hmc_sampling_identity_20261013/trace_lossless_sampling_tree-20261013-195631/run-1.verdict)
- [A 最终答案](results/hmc_sampling_identity_20261013/trace_lossless_sampling_tree-20261013-195631/run-1.answer-transcript.md)
- [B 机器收据](results/hmc_sampling_identity_20261013/trace_binary_sample_only-20261013-195632/run-1.verdict)
- [B 最终答案](results/hmc_sampling_identity_20261013/trace_binary_sample_only-20261013-195632/run-1.answer-transcript.md)
- [人工审计摘要](parallel_selected_summary_hmc_sampling_identity_20261013_manual_audit.md)

完整原件、准备、payload、日志/上下文、HTML/Markdown/sidecar、开发失败、人工审计与SHA索引保存在忽略目录 `eval/results/hmc_sampling_identity_20261013/`；不归档模型凭证。两例没有query失败后的自然重试，A无图，B仅缺数据Trace投影，不宣称原生采样图或像素级验收。末版全仓正式退出与发布收据统一记于[账本§239](../docs/design/hmosperf_capability_gap_audit_20260917.md)。
