# Codrax

**CODRAX — Code Of Deterministic Reasoning, Anchored eXplanation.**

Codrax 是一个面向工程现场的 AI 协作工具。它的核心目标不是“聊代码”，而是把代码、日志、trace、外部工具和变更流程放到同一个可审计的工作台里，让复杂问题有证据、有边界、有后续动作。

## 快速开始：问第一个代码问题

Codrax 默认把**启动时所在目录**当作要分析的代码仓库。第一次使用时，先 `cd` 到目标代码仓，再启动 Codrax：

```bash
cd /path/to/your/code-repo
codrax
```

进入 REPL 后直接提问，例如：

```text
这个项目的入口在哪里？请给出关键文件和调用链。
```

也可以用单次命令：

```bash
cd /path/to/your/code-repo
codrax --request "这个项目的入口在哪里？请给出关键文件和调用链。"
```

如果你不想切换当前目录，可以显式指定仓库：

```bash
codrax --repo /path/to/your/code-repo --request "这个配置项在哪里定义和消费？"
```

LLM provider、MCP、trace、operation skills 等详细配置见 [docs/user_guide.md](docs/user_guide.md)。

## 编译

Codrax 是 Go 项目，推荐 Go 1.22.5 或更新版本。因为仓库使用 CGO 和本地解析依赖，编译机器需要可用的 C/C++ 工具链。

```bash
make          # 编译当前平台二进制 ./codrax
make test     # 运行测试
make info     # 查看当前平台、Go、工具链信息
```

常见平台准备：

- macOS：安装 Xcode Command Line Tools。
- Linux：安装 `gcc` / `g++` 等基础编译工具。
- Windows：可用原生 MinGW 工具链；需要 Linux static 包时使用 WSL。

## 跨平台编译

跨平台编译同样受 CGO 目标平台工具链约束。Makefile 已提供常用目标：

| 目标 | 命令 | 说明 |
|---|---|---|
| 当前平台 | `make` | 生成 `./codrax` |
| Linux 静态包 | `make static` | Linux/musl 静态构建；macOS/Windows 会给出工具链提示 |
| Linux amd64 | `make cross-linux` | 需要目标平台 C 工具链 |
| Linux arm64 | `make cross-linux-arm64` | 需要 `aarch64-linux-gnu-gcc` 等工具链 |
| macOS amd64 | `make cross-darwin` | 推荐在 macOS 上构建 |
| macOS arm64 | `make cross-darwin-arm64` | 推荐在 macOS 上构建 |
| Windows amd64 | `make cross-windows` | 需要 MinGW 目标工具链 |
| 发布矩阵 | `make release` | 按当前平台可用工具链构建可发布包 |

## 适合什么场景

### 代码理解与影响分析

当你面对一个不熟悉的仓库、模块或函数时，Codrax 可以帮助你快速回答：

- 入口在哪里，调用链怎么走。
- 某个配置、接口、状态字段在哪里定义和消费。
- 一次改动可能影响哪些模块、测试和运行路径。
- 多仓或大仓里，相关代码散落在哪里。

它适合做带 `file:line` 证据的代码解释，而不是只给一段泛泛总结。

### 运行日志与故障定位

当问题来自线上日志、崩溃栈、告警或客户粘贴的运行片段时，Codrax 可以把外部现象和当前仓库源码放在一起分析：

- 根据日志中的错误、栈帧、关键字定位源码锚点。
- 区分外部观察和当前源码证据，避免把日志内容误当源码引用。
- 在证据不足时保留不确定性，而不是强行编出结论。

### Trace / 性能根因分析

Codrax 支持面向 Android、Harmony / HiTrace、东湖等运行时 trace 的性能调查场景，适合分析：

- UI 卡顿、丢帧、长时间 sleep、runnable wait。
- 多跳唤醒链、binder 链路、D 状态、IO wait、锁竞争。
- 同时间窗 CPU 压力、频点、IRQ、内存、后台负载等辅助证据。
- trace 缺口、平台优先级语义、时间单位等容易误判的边界。

这类问题不应该只靠模型手写 grep/awk 反复试探，Codrax 会尽量把可确定的 trace 事实结构化为证据包。

### 本地数据清洗与汇总

当任务的核心是读取本地结构化或半结构化材料并计算结果时，Codrax 会尽量走独立的数据处理通道，而不是把数据材料误当源码文件：

- CSV / TSV / JSON / JSONL / 文本化附件索引的清洗、过滤、去重、join、汇总。
- 需要抽取的非文本材料会先作为客观材料进入目录；只有任务确实需要且配置了对应抽取器时，才会转成文本证据后继续计算。
- 条目级判定、数值字段计算、异常记录筛选。
- 严格输出 JSON-only、CSV-only、单行字符串或 Markdown 表格。
- 对需要过滤、归一、汇总和对账的任务，会尽量生成可审计的材料覆盖、贡献记录和对账摘要，减少“脚本跑通但结果漂移”。

纯只读数据计算不需要 command-operation 审批，也不走源码 citation gate；如果用户同时要求检查代码实现，才会进入数据 + 源码的混合分析。

### 外部工具与电脑操作工作流

Codrax 可以通过 MCP 或 operation skills 接入外部能力，例如：

- 公司内告警、工单、日志检索系统。
- 本地脚本、二进制工具、PPT / 文档 / 表格生成器。
- 需要先读说明、提炼参数、再调用下游工具的多步骤 workflow。

这些外部结果会进入独立的外部观察 / operation handoff 通道，不会混进当前源码 citation。低/中风险安全动作可按 operation 策略自动推进；高风险动作需要用户批准，特别危险的操作会被直接拒绝。

### 变更方案与沙箱执行

在明确开启写模式后，Codrax 可以辅助完成代码变更闭环：

- 先产出改动方案。
- 在隔离 worktree 中应用修改。
- 执行验证并区分本次改动引入的问题、已有问题和环境缺失。
- 由用户决定是否接受和合并。

它的定位是“带审批和验证的工程助手”，不是自动改主仓的脚本。

## 不适合什么场景

- 没有仓库、日志、trace 或外部证据，却要求确定性结论。
- 需要绕过审批执行高风险电脑操作。
- 希望模型凭感觉给出无法复核的性能或代码根因。
- 把外部日志、MCP 输出、trace 行号当成当前源码引用。

## 设计取向

Codrax 的默认取向是：

- 精确信号用于硬门，噪声信号只做软提示。
- 源码证据、运行时证据、外部工具结果分 lane 传递。
- 用户能看到关键过程，最终答案能回到具体证据。
- 普通代码分析、trace/log 分析、外部 skills、写代码流程彼此隔离，按 typed 意图进入对应路径。

## 进一步了解

- 用户使用手册：[docs/user_guide.md](docs/user_guide.md)
- 架构说明：[docs/architecture.md](docs/architecture.md)
