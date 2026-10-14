# HMC238：二进制 REPL / 只读登记终态运行摘要

起点 `d768245f1`，79 唯一任务：21 实现已交付、58 开放。本批不改旧评测结果。

原定两个入口并行启动，各一个自然问题。A1 在模型运行前失败；修复并通过真实 CLI 零模型启动测试后，以新目录补齐 A 的首次真实模型评测（A2）。B 不重复。因此实际有两个模型案例、三个入口执行记录；A2 与 B 不在同一时段运行，不冒称最终两个模型运行仍同时并行。

| 记录 | 入口与构建 | 正式退出 / 耗时 | 机器结果 | 人工结果 |
| --- | --- | --- | --- | --- |
| A1 | 真 CLI，`6cfe04b4f3f5-dirty` | exit 1 / 0.916s | 启动 FAIL，0 模型调用 | FAIL：非法 CLI `--mode=read`，未分析 |
| B | 真实 planner / 生产验证 / 独立进程恢复，`6cfe04b4f` 源码 | exit 0 / 测试13.440s，包14.774s | PASS | 限定路径 PASS；controller 决策 scripted |
| A2 | 修后真 CLI REPL，`b867f533e9bf-dirty` | exit 0 / 65.620s | 8/8 PASS | 输入子路径 PASS、整答 FAIL：线程编号误称进程编号 |

dirty 均是文档改动；各次模型输入与构建独立存证。A1 原件不覆写；A2 的问题、RMQ fixture 和 oracle 与 A1 相同，修的仅启动路径。不存在第三个模型问题或修改问题追绿。

两个实际模型案例机器 2/2，人工 1/2（B 仅限定流程）；A1 零模型失败单列，不从分母中偷偷抹掉。既有人工 FAIL 不倒签，58 开放不减少。

## 输入与通过范围

A 自然问题：“这份采集在 1.0105 到 1.012 秒唤醒了哪个线程？请列出时间、线程名称和线程编号。”12 页构造 RMQ，49,740 字节、12 个事件；1,024 字节预览不含尾部目标。操作员 oracle 不进入 child workspace。A2 实际 child preparation 绑定原件，实际 query 窗为 1.0105–1.012 秒，返回唯一 `1.011 / tail-target / 424242`。输出 sidecar 绑定 Markdown SHA，原件 SHA 不变。机器检查不验证“PID”自然语言角色，不证明所有根因/图/格式正确。

B 自然问题：“实现已修好，请运行保留的现有测试，确认返回值正确。不要再修改源码、测试、配置或依赖。”实际 planner 读取完整文件并登记原断言；新进程 50451 重新执行，调用不同于旧/来源报告。来源、补证、整轮分别 verified；文件及 HEAD 不变，五份 context-pack 快照稳定。只是 Python unittest 单合同/单断言，不覆盖完整 CLI、自然 controller 或所有框架。

## 原始证据

- [A1 启动失败](results/hmc_repl_terminal_20261013/repl_rmq_binary_tail/receipt.json)
- [A2 机器收据](results/hmc_repl_terminal_20261013/repl_rmq_binary_tail_fixed/receipt.json)
- [A2 最终 Markdown](results/hmc_repl_terminal_20261013/repl_rmq_binary_tail_fixed/workspace/.codrax/output/20261013-182705.166-98432.md)
- [B 原始终态收据](results/hmc238_native_registration_terminal_20261013/receipt.json)
- [本批人工审计](parallel_selected_summary_hmc_repl_terminal_20261013_manual_audit.md)

原始大载荷/日志按仓库规则保留在本地忽略目录；统一任务清单、差距账本和本摘要入库。末版全仓、race、输入 SHA、失败原件与发布结果由差距账本 §238 统一登记，不在测试尚未正式退出时标绿。
