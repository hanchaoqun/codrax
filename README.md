# codrax

一个只读代码分析工具，由 LLM 驱动的 4 阶段确定性流水线组成。输入一个关于仓库的自然语言问题，输出一份有 citation 的结构化答案。不修改源文件。

## 流水线

`analyze → explore → extract → finalize`，每个阶段一个 agent、一个 skill，硬编码在 [`internal/orchestrator/topology.go`](internal/orchestrator/topology.go)：

| 阶段 | Agent | Skill | 做什么 |
|---|---|---|---|
| analyze | analyzer | analysis-skill | 一次 LLM 调用，通过 `emit_analysis` 工具输出 `AnalysisIR`（意图/场景/复杂度 + TaskGraph + EvidencePlan + AnswerContract + HypothesisSet + 风险矩阵） |
| explore | explorer | explore-skill | **Turn A** 调查：read_file / grep / repo_map ReAct 循环，通过 `emit_evidence` 累积证据，用 ERM（Evidence Requirement Model）跟踪"还缺哪些证据" |
| extract | extractor | extract-skill | **Turn B** 结构化：无文件 IO，只读 Turn A 的冻结快照，通过 `emit_answer_symbol` / `emit_hypothesis_verdict` 产出带 completeness claim 的答案面板 |
| finalize | finalizer | answer-document-skill | 通过 `emit_answer_document` 产出 typed AnswerDocument，deterministic renderer 转成用户可见的散文/列表/代码块 |

AnalysisIR 在 analyze 阶段之后走一条确定性后处理管线：`normalizer → compiler → risk → hdp → counterfactual → gate`（全部在 `internal/analysis/*` 子包）。

分析答案的 AnswerContract（shape + cardinality + citation 约束）由 analyzer 产出、finalizer 响应；cardinality 违规会触发 Turn A + Turn B 的回溯重跑直到 retry budget 耗尽。

## 构建

Tree-sitter 语法解析需要 CGO：

```bash
# Linux (Debian/Ubuntu)
sudo apt-get install gcc musl-tools

# macOS
xcode-select --install

# Windows (通过 MSYS2 或 scoop 安装 mingw-w64)
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
#   You> 提示符，支持 /exit /clear /history /compact /help 斜杠命令
#   多轮对话自动保存到 memory/<repo-slug>/MEMORY.md + .../turns/，重启续接
#   /clear 会显示当前还有几个其它实例在用同一份 memory，并要求确认

# 单次运行
./codrax -request "解释 explorer 的 ShouldStop 是怎么决定的"

# 诊断模式（debug 级别 ReAct trace 写入 logs/ 同时镜像到 stdout）
./codrax -log-level debug -log-stdout -request "your question"

# 多目标仓使用：日志和 memory 自动按 -repo 路径生成 hash slug 隔离
./codrax -repo /path/to/repoA -request "..."
./codrax -repo /path/to/repoB -request "..."   # 不会和 repoA 混在一起
```

## 配置

两个 YAML 文件，分工明确：

| 文件 | 负责 | 典型键 |
|---|---|---|
| [`config/providers.yaml`](config/providers.yaml.example) | **LLM 凭证与路由** — 每个 agent 用哪个 provider | API key, model ID |
| [`config/codrax.yaml`](config/codrax.yaml.example) | **本次运行怎么跑** — 日志 / memory / 语言 / 目标 repo / 流水线预算 / 工具 blob 大小 / 指向 providers.yaml 的路径 | `log_level`, `memory_dir`, `lang`, `repo`, `branch`, `pipeline_max_steps`, `pipeline_max_retries_per_stage`, `pipeline_max_stage_visits`, `blob_*` |

流水线拓扑（4 阶段 × 4 agent）是硬编码的，没有 YAML 对应项。

优先级（低到高）：**代码默认 < `config/codrax.yaml` < 命令行 flag**。每个字段都可以在任一层覆盖。通过 `CODRAX_SETTINGS=envs/prod/codrax.yaml` 环境变量可以一键切换整套环境（因为 `providers_config` 路径也可以写在 codrax.yaml 里）。

`config/codrax.yaml` 的查找顺序：`$CODRAX_SETTINGS` → `<CWD>/config/codrax.yaml` → `<exeDir>/config/codrax.yaml` → `<exeDir>/../config/codrax.yaml`（覆盖 `bin/<exe>` 安装布局）。命中后，文件所在的 `config/` 父目录成为「锚点」，所有相对默认路径（`log_dir`、`memory_dir`、`providers_config`）都基于锚点解析，所以二进制扔到任何 CWD 都能找到自己的配置和写日志。

## 功能亮点

- **分级日志**：error / warning / info / debug 四档，按 4MB 滚动、保留 7 份；文件名 `logs/<repo-slug>/codrax-YYYYMMDD-HHMMSS-mmm-<pid>.log`，每个 codrax 进程独占自己的文件，多实例并发也不会撕日志
- **交互多轮**：REPL 模式下每一轮自动带前轮上下文；超过 6 轮或 20KB 触发 LLM 摘要压缩成 `MEMORY.md` 索引条目，下次问到相关话题可按关键词召回原文
- **跨重启恢复**：单实例从崩溃中恢复时，`memory/<repo-slug>/turns/` 里未压缩的最近 6 轮会自动回灌到 recent；多实例并发场景下检测到 peer 时跳过这一步，避免双方互相把对方的对话挪到自己头上
- **多目标仓隔离**：log 和 memory 默认按 `-repo` 绝对路径的 hash slug 自动分目录（`logs/foo-a3f9c2b1/...`），同一 codrax 安装可以服务多个目标仓而互不污染
- **多实例并发安全**：同一目标仓多开 codrax 时，日志按 PID 隔离、`MEMORY.md` 周期写入由 `flock` 串行化、`/clear` 会提示当前还有几个 peer 在用、retention sweep 跳过仍存活进程的活跃文件
- **跨平台**：Linux / macOS 用 `flock(2)`，Windows 通过 `kernel32.dll!LockFileEx` 实现等价语义，全程零非 stdlib 依赖
- **默认语言**：`-lang=zh` 默认简体中文作答；`-lang=off` 关闭；任一非空值都会保留"用户若用其他语言提问则跟随"的兜底
- **Answer contract**：finalizer 输出结构化 `AnswerDocument`（typed symbols / steps / value / boolean / explanation + citation pool），cardinality 验证器会把"谎称 complete 但 slate 不足基线"的 claim 自动降级为 lower_bound 并附警告渲染

## 文档

- **[架构设计文档](docs/architecture.md)** — 完整的系统规范，包括组件详情、数据结构、Turn A/Turn B 分离、分析后处理管线、运行时子系统
- **[运行时配置示例](config/codrax.yaml.example)** — 所有可调项的完整列表与分工说明
- **[Providers 配置示例](config/providers.yaml.example)** — LLM provider 凭证 + 每 agent 模型路由
