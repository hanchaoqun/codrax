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

用户粘贴一段运行时日志（panic / exception 堆栈 / sanitizer 诊断 / traceback），codrax 的 analyzer 会**跨语言**解析其中的栈帧，把函数名、包名、错误字面量抽出来喂给关键词检索，并把栈帧的 `file:line` 作为强信号注入 `EvidencePlan.RequiredFiles`，explorer 优先读这些文件。当前支持：Go panic / Java stacktrace（含 `Caused by` 链）/ C/C++ ASAN+UBSAN+gdb / Python traceback / Node.js V8 stack。

日志正文不混进问题字符串，走独立的 `BusContext.AttachedLog` 通道——normalizer 只看到你的提问，不会被日志里的噪声污染实体识别。

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

# 附加日志跨 turn 粘滞（sticky）——用户通常要围绕同一条 panic 问多个问题：
❯❯ /log /tmp/crash.txt
❯❯ 这个 panic 的根本原因是什么？
❯❯ 修复方案呢？会不会有副作用？    # 附加日志仍在，无须重新粘贴
❯❯ /clear                         # 只清对话历史，attached log 不动
❯❯ /log clear                     # 显式清才丢日志
```

### 支持的日志格式示例

```
# Go panic
panic: runtime error: index out of range [5] with length 3

goroutine 1 [running]:
main.crashy()
        /src/app/main.go:42 +0x1e

# Java
Exception in thread "main" java.lang.NullPointerException: x
        at com.example.Service.run(Service.java:42)
Caused by: java.lang.IllegalStateException: y
        at com.example.Inner.boom(Inner.java:5)

# C/C++ ASAN
==12345==ERROR: AddressSanitizer: heap-use-after-free on address 0x...
    #0 0x401234 in Foo::bar() /src/repo/foo.cpp:42:5
    #1 0x401567 in main       /src/repo/main.cpp:10:3

# Python
Traceback (most recent call last):
  File "/src/app.py", line 42, in handle
    result = compute(x)
ValueError: bad input

# Node.js
TypeError: Cannot read property 'x' of undefined
    at Object.handler (/src/app.js:42:13)
    at async main (/src/main.js:10:5)
```

全局开关在 `codrax.yaml`：`logparse_enabled: false` 让整条通路退化成 no-op（`--log` 仍读文件但不解析）。`logparse_source_prefix` 等效于 CLI `--log-source-prefix`，CLI 显式传值时优先。

明确**不做**：glibc backtrace（只有地址没 `file:line`）、实时日志 tail / 订阅、Loki/ES/CloudWatch 远端源、自定义应用日志格式——需要的话走 Tier 2/3 功能扩展。

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
- **日志分诊**：`--log / --log-text` 或 REPL `/log` 粘贴 panic / Java exception / C/C++ ASAN+UBSAN+gdb / Python traceback / Node.js stack，analyzer 跨语言解析栈帧并把 `file:line` 注入到 `EvidencePlan.RequiredFiles`，`reconcileIntent` 强制切到 root_cause 模式；日志正文走独立通道，不污染关键词识别

## 文档

- **[架构设计文档](docs/architecture.md)** — 完整的系统规范，包括组件详情、数据结构、Turn A/Turn B 分离、分析后处理管线、运行时子系统
- **[运行时配置示例](codrax.yaml.example)** — 所有可调项的完整列表与分工说明
- **[Providers 配置示例](providers.yaml.example)** — LLM provider 凭证 + 每 agent 模型路由
