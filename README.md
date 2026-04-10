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

## 构建

Tree-sitter 语法解析需要 CGO，构建前需安装 C 编译器：

```bash
# Linux (Debian/Ubuntu)
sudo apt-get install gcc musl-tools

# macOS
xcode-select --install

# Windows
# 安装 mingw-w64（通过 MSYS2 或 scoop）
```

```bash
# 编译（输出 ./codrax）
make

# 全静态 musl 链接（仅 Linux）
make static

# 交叉编译全平台到 dist/
make release

# 运行测试
make test
```

## 运行

```bash
# 交互模式（默认，无 -request 即进入）
./codrax
#   进入后使用 You> 提示符，支持 /exit /clear /history /compact /help 斜杠命令
#   多轮对话自动保存到 memory/<repo-slug>/MEMORY.md + .../turns/，重启续接
#   /clear 会显示当前还有几个其它实例在用同一份 memory，并要求确认

# 单次运行
./codrax -request "your task description"

# 诊断模式（debug 级别 ReAct trace 写入 logs/ 同时镜像到 stdout）
./codrax -log-level debug -log-stdout -request "your task"

# 多目标仓使用：日志和 memory 自动按 -repo 路径生成 hash slug 隔离
./codrax -repo /path/to/repoA -request "..."
./codrax -repo /path/to/repoB -request "..."   # 不会和 repoA 混在一起
```

## 配置

三个 YAML 文件，分工明确，互不重复：

| 文件 | 负责 | 典型键 |
|---|---|---|
| [`config/orchestrator.yaml`](config/orchestrator.yaml) | **流水线形状** — stages / transitions / policies / agents / skills | stage / transition / policy 的拓扑定义 |
| [`config/providers.yaml`](config/providers.yaml.example) | **LLM 凭证与路由** — 每个 agent 用哪个 provider | API key, model ID |
| [`config/codrax.yaml`](config/codrax.yaml.example) | **本次运行怎么跑** — 日志 / memory / 语言 / 目标 repo / 流水线预算与行为 / 工具 blob 大小 / 指向上面两个文件的路径 | `log_level`, `memory_dir`, `lang`, `repo`, `branch`, `pipeline_max_steps`, `pipeline_*`, `blob_*` |

优先级（低到高）：**代码默认 < `config/orchestrator.yaml` 中的 `pipeline_settings:` 旧块（仅 pipeline_* 适用，向后兼容） < `config/codrax.yaml` < 命令行 flag**。每个字段都可以在任一层覆盖。通过 `CODRAX_SETTINGS=envs/prod/codrax.yaml` 环境变量可以一键切换整套环境（因为 `orchestrator_config` 和 `providers_config` 路径也可以写在 codrax.yaml 里）。

`config/codrax.yaml` 的查找顺序：`$CODRAX_SETTINGS` → `<CWD>/config/codrax.yaml` → `<exeDir>/config/codrax.yaml` → `<exeDir>/../config/codrax.yaml`（覆盖 `bin/<exe>` 安装布局）。命中后，文件所在的 `config/` 父目录成为「锚点」，所有相对默认路径（`log_dir`、`memory_dir`、`orchestrator_config`、`providers_config`）都基于锚点解析，所以二进制扔到任何 CWD 都能找到自己的配置和写日志。

## 功能亮点

- **分级日志**：error / warning / info / debug 四档，按 4MB 滚动、保留 7 份；文件名 `logs/<repo-slug>/codrax-YYYYMMDD-HHMMSS-mmm-<pid>.log`，每个 codrax 进程独占自己的文件，多实例并发也不会撕日志
- **交互多轮**：REPL 模式下每一轮自动带前轮上下文；超过 6 轮或 20KB 触发 LLM 摘要压缩成 `MEMORY.md` 索引条目，下次问到相关话题可按关键词召回原文
- **跨重启恢复**：单实例从崩溃中恢复时，`memory/<repo-slug>/turns/` 里未压缩的最近 6 轮会自动回灌到 recent；多实例并发场景下检测到 peer 时跳过这一步，避免双方互相把对方的对话挪到自己头上
- **多目标仓隔离**：log 和 memory 默认按 `-repo` 绝对路径的 hash slug 自动分目录（`logs/foo-a3f9c2b1/...`），同一 codrax 安装可以服务多个目标仓而互不污染
- **多实例并发安全**：同一目标仓多开 codrax 时，日志按 PID 隔离、`MEMORY.md` 周期写入由 `flock` 串行化、`/clear` 会提示当前还有几个 peer 在用、retention sweep 跳过仍存活进程的活跃文件
- **跨平台**：Linux / macOS 用 `flock(2)`，Windows 通过 `kernel32.dll!LockFileEx` 实现等价语义，全程零非 stdlib 依赖
- **默认语言**：`-lang=zh` 默认简体中文作答；`-lang=off` 关闭；任一非空值都会保留"用户若用其他语言提问则跟随"的兜底

## 文档

- **[架构设计文档](docs/architecture.md)** — 完整的系统规范，包括组件详情、数据结构、状态机和生命周期
- **[编排器配置示例](config/orchestrator.yaml)** — stages/transitions/policies 参考
- **[运行时配置示例](config/codrax.yaml.example)** — 运行期可调项的完整列表与分工说明
