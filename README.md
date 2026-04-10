# AI Agent 系统架构

一个围绕 YAML 驱动的编排器状态机构建的 5 层 AI Agent 系统。编排器将任务路由到最多 8 个流水线阶段，调度配备了技能、工具和 MCP 集成的专业 Agent，所有这些都由 LLM 推理驱动。

> **编排器**决定*谁做什么* | **Agent** *执行* | **技能**定义*怎么做* | **工具/MCP**提供*能力* | **LLM**是*大脑*

## 快速参考

| 层级 | 名称 | 组件 | 职责 |
|------|------|------|------|
| 1 | 编排层 | 编排器 | Agent 选择、流水线控制、状态管理、终止决策 |
| 2 | 执行层 | Agent（8 种类型） | 接收提示词、调用 LLM、使用工具、产出输出 |
| 3 | 策略层 | 技能（8 种技能） | 工作流步骤、工具建议、输出格式、约束条件 |
| 4a | 能力层 | 工具 | 本地操作（文件、执行、搜索、测试） |
| 4b | 能力层 | MCP | 外部系统集成（GitHub、数据库、Notion 等） |
| 5 | 智能层 | LLM | 推理、决策、文本生成 |

## 运行

```bash
# 构建
go build .

# 交互模式（默认，无 -request 即进入）
go run .
#   进入后使用 You> 提示符，支持 /exit /clear /history /compact /help 斜杠命令
#   多轮对话自动保存到 memory/MEMORY.md + memory/turns/，重启续接

# 单次运行
go run . -request "your task description"

# 诊断模式（debug 级别 ReAct trace 写入 logs/ 同时镜像到 stdout）
go run . -log-level debug -log-stdout -request "your task"
```

## 配置

三个 YAML 文件，分工明确，互不重复：

| 文件 | 负责 | 典型键 |
|---|---|---|
| [`config/orchestrator.yaml`](config/orchestrator.yaml) | **流水线形状** — stages / transitions / policies / agents / skills / 每阶段限额 | `max_retries_per_stage`, `max_stage_visits`, `require_review` |
| [`config/providers.yaml`](config/providers.yaml.example) | **LLM 凭证与路由** — 每个 agent 用哪个 provider | API key, model ID |
| [`config/codrax.yaml`](config/codrax.yaml.example) | **本次运行怎么跑** — 日志 / memory / 语言 / 每次运行步数预算 / 目标 repo / 指向上面两个文件的路径 | `log_level`, `memory_dir`, `lang`, `max_steps`, `repo`, `branch` |

三层优先级（低到高）：**代码默认 < `config/codrax.yaml` < 命令行 flag**。每个字段都可以在任一层覆盖。通过 `CODRAX_SETTINGS=envs/prod/codrax.yaml` 环境变量可以一键切换整套环境（因为 `orchestrator_config` 和 `providers_config` 路径也可以写在 codrax.yaml 里）。

## 功能亮点

- **分级日志**：error / warning / info / debug 四档，文件按 4MB 滚动、保留 7 份、文件名带时间戳（`logs/codrax-YYYYMMDD-HHMMSS-mmm.log`），重启自动续写未满的最新文件
- **交互多轮**：REPL 模式下每一轮自动带前轮上下文；超过 6 轮或 20KB 触发 LLM 摘要压缩成 `MEMORY.md` 索引条目，下次问到相关话题可按关键词召回原文
- **跨重启恢复**：进程退出后 `memory/turns/` 里未压缩的最近 6 轮会在下次启动时回灌到 recent，不依赖优雅退出
- **默认语言**：`-lang=zh` 默认简体中文作答；`-lang=off` 关闭；任一非空值都会保留"用户若用其他语言提问则跟随"的兜底

## 文档

- **[架构设计文档](docs/architecture.md)** — 完整的系统规范，包括组件详情、数据结构、状态机和生命周期
- **[编排器配置示例](config/orchestrator.yaml)** — stages/transitions/policies 参考
- **[运行时配置示例](config/codrax.yaml.example)** — 运行期可调项的完整列表与分工说明
