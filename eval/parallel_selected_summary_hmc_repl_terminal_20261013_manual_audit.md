# HMC238：REPL 二进制完整材料 / 只读登记恢复终态

状态：两例已收齐，机器 2/2、整答/限定流程人工 1/2；另保留一次零模型启动失败。起点 `d768245f1`，79 唯一任务中 21 项实现已交付、58 项开放。

固定 A/B 首次入口并行启动。A1 在模型调用前启动失败（真实模型调用 0 次），B 完成一次真实评测；A 的启动修复经零模型测试后，使用新目录补齐其首次真实模型运行，问题、输入和 oracle 不变，不重复 B。A 使用 RMQ 生产格式构造输入，不冒充新实机采集；B 仅 planner 为真实模型，controller 决策仍为脚本化 verify/finish。先前采样、分组及图的人工 FAIL 不因本批通过而撤销。

| 例 | 能力入口 | 机器结果 | 人工结果 | 验收边界 |
| --- | --- | --- | --- | --- |
| A1 | 真实 CLI 启动 | FAIL | FAIL：模型未运行 | `--mode=read` 不是合法 CLI 模式，0.916 秒退出；不能归因模型或二进制解析 |
| A2 | 修后真实 REPL `/htrace` → 窗口内尾部唤醒事实 | 8/8 PASS | 整答 FAIL，输入子路径 PASS | 唯一尾部/时间/名字/编号正确，却把被唤醒线程编号写成进程编号；无该主体 TGID 证据 |
| B | 真实 planner 只读登记 → 持久化 → 独立进程执行 | PASS | 限定路径 PASS | 新进程重新执行；来源、补证、整轮三层 verified；不是完整 CLI / 真实 controller 验收 |

## 审计要求

A：核对实际命令输入、附件准备回执、有限预览、完整 query payload、当前来源/时间范围和最终正文/表格/图（若有）。仅答案包含线程编号不能证明完整材料路径通过；查询空值、预览替代、根因越权、系统事实展示遗漏分别记录。

B：核对原始模型日志的读取与发射顺序、来源计划与 verify 报告、登记计划、恢复前后 workflow、独立执行 invocation、具体断言绑定、上下文包身份与引用稳定性。普通测试进程退出成功不等于整体 verified；脚本化 controller 不计完整自然 controller 验收。

## 已收齐的证据

A1 原件目录 `results/hmc_repl_terminal_20261013/repl_rmq_binary_tail` 不覆写；修为合法 `--mode=code` 后，零模型实际 CLI 测试进一步发现 `Config.In` 未接管道输入，错误转向交互终端。此为真实入口缺口，修复沿用已有脚本读取器，不能改测试为直接注入私有 runner 来避开。

A2 使用 `b867f533e9bf-dirty`（只文档 dirty）首次完成真实模型运行，正式 exit 0、65.620 秒，机器 8/8。真实二进制原件准备→子进程快照→完整原生 query→输出 sidecar/Markdown SHA 均串通；预览 1,024 字节不含目标，原件 49,740 字节未变。实际查询指定 1.0105–1.012 秒，唯一成员为 1.011000 秒 tail-target / 424242，source_path 为完整派生 systrace、local_line=23。主语是被唤醒线程，原生字段 `wakee_pid` 没有对应 wakee TGID；emitter 的 pid/tgid=12 不授目标进程身份。终稿却称“进程编号（PID）为 424242”，MD/HTML/sidecar 同样保留错误。因此不能以取数正确销整答，亦不能擅自改为“模型波动”。本例无图输出或图请求，不宣称进行了图形像素验收。原件位于 `results/hmc_repl_terminal_20261013/repl_rmq_binary_tail_fixed`，逐阶段完整人工记录归档为 `codrax-hmc238-repl-manual-audit-v2.md`。

B 两次模型请求均 first attempt：完整读取两份原文件后精确登记，无自然失败重试。父进程 49558、独立子进程 50451，新 invocation `native:c513:18de405ce2ba38a0:1` 与旧及来源调用不同。来源/补证/整体 verified；历史 unverified attempt 保留，只有批级依赖在已验证后继到达后闭合。五快照各 6 个 context packs，内容和引用无累积；源码/测试/HEAD 不变。完整人工报告保存在本批 `verification-receipts/codrax-hmc238-registration-manual-audit-v1.md`（归档前原件位于 `/tmp`）。原结果在 `results/hmc238_native_registration_terminal_20261013`。

确定性采样修复只补直接流式入口遗漏的已有主体选择回执，不替换线程、不新增关键词门；相关测试与旧自然失败分开留档。单 PID 无名称、仅 source-only 身份的总体语义，以及完整采样答案和自然 retry 仍未验收。

## 记账与下一批

完整任务新增 0、累计 21，开放仍 58。两组子能力：采样选择回执一致性；真实 CLI 脚本输入与二进制完整尾部路径。A2 身份角色误述挂原 01.3/16.4，source-only/owner/自然采样留 09.4；sample-only 自然二进制负控留 18.3，真实 controller 与跨框架登记留 18.5。不重复普通 RMQ 问题或已 PASS 的 scripted-controller B，不为减少任务数缩小原验收范围。
