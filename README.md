# codrax

一个只读代码分析工具，由 LLM 驱动的 4 阶段确定性流水线组成。输入一个关于仓库的自然语言问题，输出一份有 citation 的结构化答案。不修改源文件。

## 流水线

主流水线 `analyze → explore → extract → finalize`，每个阶段一个 agent、一个 skill，硬编码在 [`internal/orchestrator/topology.go`](internal/orchestrator/topology.go)。当用户附加了运行时日志时，analyze 之前会条件触发一个独立的 log_triage 阶段：

| 阶段 | Agent | Skill | 做什么 |
|---|---|---|---|
| log_triage *(条件触发)* | log_triager | log-triage-skill | 仅当用户通过 `--log` / `/log` 附加运行时日志时运行。LLM 把日志解析成结构化 bundle（错误类型、栈帧、因果链、信号分类），系统做路径校验和派生；阶段失败不阻塞主流水线 |
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
# 交互模式（默认，无 --request 即进入）
./codrax
#   You> 提示符，支持 /exit /clear /history /compact /log /help 斜杠命令
#   多轮对话自动保存到 memory/<repo-slug>/MEMORY.md + .../turns/，重启续接
#   /clear 会显示当前还有几个其它实例在用同一份 memory，并要求确认

# 单次运行
./codrax --request "解释 explorer 的 ShouldStop 是怎么决定的"

# 诊断模式（debug 级别 ReAct trace 写入 logs/ 同时镜像到 stdout）
./codrax --log-level debug --log-stdout --request "your question"

# 多目标仓使用：日志和 memory 自动按 -repo 路径生成 hash slug 隔离
./codrax --repo /path/to/repoA --request "..."
./codrax --repo /path/to/repoB --request "..."   # 不会和 repoA 混在一起
```

## 日志分诊（log triage）

粘贴一段运行时日志（panic / exception 堆栈 / sanitizer 诊断 / traceback / 自研应用日志 / 编译器错误都可以），codrax 会在 analyze 之前先跑一个独立的 log_triage 阶段：LLM 把日志读成结构化数据（错误类型、栈帧、因果链、信号分类），系统做路径校验、仓内存在性过滤、重复消噪，然后把解析出来的文件定位和关键词喂给下游，explorer 优先读这些文件。

因为是 LLM 做抽取而不是写死的正则 parser，支持的格式不是一个固定列表——凡是模型能看懂的结构都能处理：Go panic、Java exception 含 `Caused by` 链、C/C++ ASAN / UBSAN / gdb、Python traceback（包含 `During handling` 嵌套）、Node.js V8 stack、Rust `#[source]` 链、Ruby backtrace、结构化 JSON 日志……都是一套代码路径。

日志正文不会混进问题字符串，走独立通道——提问的实体识别只看你的问题本身，不会被日志噪声污染。系统侧把每个文件路径 `os.Stat` 验证一遍，仓外路径和运行时内部文件（Go stdlib、`node:` URI、`java.base/*` 等）会被过滤掉，不会影响答案。

### CLI

```bash
# 从文件加载
./codrax --repo . --request "这个 panic 哪来的？" --log /tmp/panic.txt

# 从 stdin 读（适合 `kubectl logs` 管道）
kubectl logs pod/api | ./codrax --repo . --request "analyze this crash" --log -

# 脚本化调用：内联日志
./codrax --repo . --request "分析 ASAN 报告" --log-text "$(cat /tmp/asan.out)"

# C/C++ 场景：build 机器的绝对路径不在目标仓里 → 给 codrax 一个前缀提示
./codrax --repo . --request "trace" \
  --log /tmp/asan.out \
  --log-source-prefix /home/jenkins/workspace/build/src/
```

`--log` 和 `--log-text` 互斥；日志体超过 1 MB 自动截断到前 1 MB。

### REPL 子命令

```
❯❯ /log /tmp/panic.txt           # 从文件载入（替换当前附加）
❯❯ /log                          # 进入粘贴模式，以单独一行 /end 结束
ctrl 粘贴 stack ...
/end
❯❯ /log show                     # 打印前 20 行 + 总字节
❯❯ /log clear                    # 丢弃当前附加

# 附加日志跨轮次粘滞——通常围绕同一条 panic 会问多个问题：
❯❯ /log /tmp/crash.txt
❯❯ 这个 panic 的根本原因是什么？
❯❯ 修复方案呢？会不会有副作用？    # 附加日志仍在，无须重新粘贴
❯❯ /clear                         # 只清对话历史，attached log 不动
❯❯ /log clear                     # 显式清才丢日志
```

### 长日志处理

单次抽取默认一次性读完日志。当日志体积超过阈值（默认 32 KB）或单次抽取覆盖率偏低时，系统自动切到两步模式：先让 LLM 按栈/因果块/上下文切片定位字节范围，再逐段抽取，最后合并结果。全流程 LLM 调用次数有硬上限（默认 8 次）。

### 可调项

所有 log_triage 相关配置都在 `codrax.yaml`，前缀统一为 `log_triage_*`：

| 键 | 默认值 | 作用 |
|---|---|---|
| `log_triage_enabled` | `true` | 关掉后附加日志只会被读入但不会触发抽取 |
| `log_triage_source_prefix` | 空 | 提示 build 路径前缀；等价于 CLI `--log-source-prefix`，CLI 显式传值时优先 |
| `log_triage_min_bytes` | 50 | 低于此字节数的日志直接跳过（太短往往没栈帧） |
| `log_triage_two_step_bytes` | 32768 | 超过此大小直接走两步模式 |
| `log_triage_two_step_coverage` | 0.3 | 单次抽取覆盖率低于此阈值升级到两步 |
| `log_triage_max_llm_calls` | 8 | 单次请求内 log_triage 阶段的 LLM 调用次数硬上限 |
| `log_triage_max_retries` | 1 | 抽取被 schema 拒绝后的重试次数 |

### 暂不支持

- 实时日志 tail / 订阅、远端日志源（Loki / ES / CloudWatch）
- glibc 裸 backtrace（只有地址没有 file:line，缺少足够锚点）

## 配置

两个 YAML 文件平铺在二进制同目录，分工明确：

| 文件 | 负责 | 典型键 |
|---|---|---|
| [`providers.yaml`](providers.yaml.example) | **LLM 凭证与路由** — 每个 agent 用哪个 provider | API key, model ID |
| [`codrax.yaml`](codrax.yaml.example) | **本次运行怎么跑** — 日志 / memory / 语言 / 目标 repo / 流水线预算 / 工具 blob 大小 / 指向 providers.yaml 的路径 | `log_level`, `memory_dir`, `lang`, `repo`, `branch`, `pipeline_max_steps`, `pipeline_max_retries_per_stage`, `pipeline_max_stage_visits`, `blob_*`, `log_max_files`, `blob_max_sessions` |

流水线拓扑（4 阶段 × 4 agent）是硬编码的，没有 YAML 对应项。

优先级（低到高）：**代码默认 < `codrax.yaml` < 命令行 flag**。每个字段都可以在任一层覆盖。通过 `CODRAX_SETTINGS=path/to/codrax.yaml` 环境变量可以一键切换整套环境（因为 `providers_config` 路径也可以写在 codrax.yaml 里）。

### 路径锚点

一个二进制跨多目录、多仓库运行时需要两个锚点：

- **配置锚点**（`<exeDir>`，即二进制所在目录）：`providers_config` 的相对路径都在这里解析。部署时把 `codrax` + `codrax.yaml` + `providers.yaml` 三个文件扔一起就行。
- **运行产物锚点**（`<CWD>/.codrax/`）：`log_dir` / `memory_dir` / `cache_dir` / blob 会话目录的相对路径都在这里解析。用户在哪个工作目录启动 codrax，日志就落在哪个工作目录的 `.codrax/` 下。

### `codrax.yaml` 查找顺序

1. `$CODRAX_SETTINGS`（绝对路径，显式指定）
2. `<exeDir>/codrax.yaml` ← 主路径
3. `<exeDir>/codrax/codrax.yaml`（bin + share 分离布局）
4. 以下为 legacy 路径，找到时会打 deprecation warning：
   - `<exeDir>/config/codrax.yaml`
   - `<exeDir>/../config/codrax.yaml`
   - `<CWD>/config/codrax.yaml`

### 目录结构

```
<exeDir>/                        ← 配置
  codrax
  codrax.yaml                    (复制自 codrax.yaml.example)
  providers.yaml                 (复制自 providers.yaml.example)

<CWD>/.codrax/                   ← 运行产物（单个隐藏根，.gitignore 一条搞定）
  logs/<repo-slug>/
    codrax-<timestamp>-<pid>.log
  memory/<repo-slug>/
    MEMORY.md
    turns/
  blob/<timestamp>-<pid>/        (每个进程一个 session，保留最近 N 个)
    <tool>-<sha8>.txt
```

`log_max_files` 和 `blob_max_sessions` 两个 YAML 键控制保留数量（默认都是 7）；PID 还活着的 peer 永远不会被清理。

## 功能亮点

- **分级日志**：error / warning / info / debug 四档，按 4MB 滚动、保留 7 份；文件名 `.codrax/logs/<repo-slug>/codrax-YYYYMMDD-HHMMSS-mmm-<pid>.log`，每个 codrax 进程独占自己的文件，多实例并发也不会撕日志
- **交互多轮**：REPL 模式下每一轮自动带前轮上下文；超过 6 轮或 20KB 触发 LLM 摘要压缩成 `MEMORY.md` 索引条目，下次问到相关话题可按关键词召回原文
- **跨重启恢复**：单实例从崩溃中恢复时，`.codrax/memory/<repo-slug>/turns/` 里未压缩的最近 6 轮会自动回灌到 recent；多实例并发场景下检测到 peer 时跳过这一步，避免双方互相把对方的对话挪到自己头上
- **多目标仓隔离**：log 和 memory 默认按 `-repo` 绝对路径的 hash slug 自动分目录（`.codrax/logs/foo-a3f9c2b1/...`），同一 codrax 安装可以服务多个目标仓而互不污染
- **多实例并发安全**：同一目标仓多开 codrax 时，日志按 PID 隔离、`MEMORY.md` 周期写入由 `flock` 串行化、`/clear` 会提示当前还有几个 peer 在用、retention sweep 跳过仍存活进程的活跃文件
- **跨平台**：Linux / macOS 用 `flock(2)`，Windows 通过 `kernel32.dll!LockFileEx` 实现等价语义，全程零非 stdlib 依赖
- **默认语言**：`-lang=zh` 默认简体中文作答；`-lang=off` 关闭；任一非空值都会保留"用户若用其他语言提问则跟随"的兜底
- **Answer contract**：finalizer 输出结构化 `AnswerDocument`（typed symbols / steps / value / boolean / explanation + citation pool），cardinality 验证器会把"谎称 complete 但 slate 不足基线"的 claim 自动降级为 lower_bound 并附警告渲染
- **日志分诊**：`--log / --log-text` 或 REPL `/log` 附加运行时日志，LLM 驱动的 log_triage 阶段把任意格式的 panic / exception / sanitizer 诊断 / traceback / 结构化应用日志解析成结构化锚点（错误类型、栈帧、因果链、信号分类），系统侧做仓内路径校验把解析结果注入下游，explorer 优先读栈帧文件；超长或复杂日志自动走两步抽取；日志正文独立通道不污染提问的关键词识别

## 文档

- **[架构设计文档](docs/architecture.md)** — 完整的系统规范，包括组件详情、数据结构、Turn A/Turn B 分离、分析后处理管线、运行时子系统
- **[运行时配置示例](codrax.yaml.example)** — 所有可调项的完整列表与分工说明
- **[Providers 配置示例](providers.yaml.example)** — LLM provider 凭证 + 每 agent 模型路由
