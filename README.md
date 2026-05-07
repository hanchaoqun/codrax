# codrax

> **CODRAX — Code Of Deterministic Reasoning, Anchored eXplanation.**
> **CODRAX —— 推理确定,解释锚定。**
> 每条结论锚到字节,每段推理可被复核。
> **确定性诚实,不是态度,是契约。**

codrax 是一个 LLM 驱动的**代码分析 + 变更提议**工具。默认只读:输入一个关于仓库的自然语言问题,输出一份带 `file:line` citation 的结构化答案,不触碰源文件。开启写模式后,可以在一个**沙箱 git worktree** 里走完整的 `plan → apply → verify` 流程;主仓库 HEAD 字节永不自动变更,用户显式审批才会落地。

## 快速开始

```bash
# 1. 构建(CGO 依赖见下方"构建"一节)
git clone https://github.com/hanchaoqun/codrax.git && cd codrax
make
# 输出 ./codrax

# 2. 加进 PATH
echo "export PATH=\"$PWD:\$PATH\"" >> ~/.bashrc      # bash
echo "export PATH=\"$PWD:\$PATH\"" >> ~/.zshrc       # zsh
exec "$SHELL" -l

# 3. 配置 LLM 凭证 —— providers.yaml 必须和 codrax 二进制同目录
cp providers.yaml.example providers.yaml
$EDITOR providers.yaml                               # 填 api_key + model

# 4. 进入目标仓库启动 REPL
cd /path/to/your/repo
codrax
```

REPL 里直接发问:

```
   CODRAX  v0.1.20260424  /help · /exit

❯❯ explorer 的 ShouldStop 是怎么决定的?
```

答案带 `file:line` citation;`/help` 列出全部斜杠命令,`/version` 看构建版本,`/exit` 退出。

### 最精简 `providers.yaml`

所有 agent 共用同一个 provider 即可起跑:

```yaml
llm:
  default:
    provider: openai          # 仅实现 openai 协议;deepseek / qwen / vllm /
                              # Ollama 等兼容此协议的服务共用同一段,换 base_url
    api_key: "sk-xxx"
    model: "gpt-4o"
    base_url: ""              # 空 → https://api.openai.com/v1
```

需要按 agent 换模型,加 `agents:` 段;未列出的 agent 自动继承 `default`。合法的 agent 名:

- **读模式 4 个**:`analyzer` · `explorer` · `extractor` · `finalizer`
- **写模式 4 个**:`write_analyzer` · `planner` · `coder` · `verifier`
- **条件前置**:`log_triager` · `perf_triager`
- **辅助**:`chitchat_responder` · `chitchat_classifier` · `memory_summarizer` · `reflector` · `plan_critic` · `env_recommender`

每个 agent 槽都支持 `<name>_fallback`,主 provider 失败时自动切到 fallback,适合多区域容灾。

更多模板见 [`providers.yaml.example`](providers.yaml.example)。

### `providers.yaml` 放在哪

二进制同目录优先(`<exeDir>/providers.yaml`)。临时切换用环境变量:

```bash
CODRAX_SETTINGS=/etc/codrax/team-shared.yaml codrax
```

完整查找规则见 [用户指南](docs/user_guide.md#34-配置文件查找顺序)。

## 两种使用模式

### 读模式(默认)

`analyze → explore → extract → finalize`,4 阶段 × 4 Agent 硬编码拓扑。两条独立条件前置阶段:`--log` / `/log` 附加运行时日志触发 `log_triage`;`--htrace` / `--atrace` / `/htrace` 附加性能 trace(HiTrace / atrace / systrace / perfetto)触发 `perf_triage`。两个前置阶段可同时触发,失败均不阻塞主流水线。

| 阶段 | Agent | 做什么 |
|---|---|---|
| `log_triage`(条件) | `log_triager` | LLM 把日志解析成结构化 bundle(错误类型 / 栈帧 / 因果链 / 10 类信号),系统侧做路径校验 + 仓内存在性过滤 + Layer 4 派生(ResolvedFiles / Entities / IntentHint / Coverage)。32 KB+ 或单次覆盖率偏低时自动两步分诊(`emit_log_segmentation` 切段 → 逐段 `emit_log_triage` → `MergeBundles` 合并) |
| `perf_triage`(条件) | `perf_triager` | LLM 把 HiTrace / atrace / systrace / perfetto 解析成 `PerfBundle`(Frames / Janks / Stalls / Startup),按 16.6 ms 帧预算 / 100 ms 主线程阻塞 / 1.2 s 冷启动阈值自动打 signals。64 KB+ 自动两步分诊 |
| `analyze` | `analyzer` | 一次 `emit_analysis` 产出 v4 `RequestModel`(Intent / Scenario / Complexity / 7 个跨语言 SemanticPredicates / AnswerSubject / SubTopics 等);系统跑 33 步确定性后处理装配 `AnalysisIR`(TaskGraph + EvidencePlan + AnswerContract + HypothesisSet + RiskMatrix)。同时消费 LogTriage + PerfTrace 派生的 entities + ResolvedFiles |
| `explore` | `explorer` | Turn A 调查:read_file / grep / repo_map ReAct 循环,通过 `emit_evidence` 累积证据;`emit_investigation_complete` 显式收尾 |
| `extract` | `extractor` | Turn B 结构化:读 Turn A 冻结快照,产出带 completeness claim 的答案 slate + 假设判定;**禁止再读文件** |
| `finalize` | `finalizer` | `emit_answer_document` 产出 typed `AnswerDocumentV2`(9 种 typed block + citation 池),renderer 渲染成用户可见散文 |

Citation 由 finalizer 发射时同步校验(`internal/tool/ground/`),7 层落地(T1 line_text / T2 symbol_table / R1-R5 recovery)文件 / 行号 / 文本三重匹配,越界 / 编造 / 与源码不一致的引用进 "Unverified Leads" 段不入 citation 池。

### 写模式(opt-in)

默认关闭。在 `codrax.yaml` 里显式 `write_enabled: true` 才能启用;CLI 通过 `--mode=plan|apply|verify` 切换,REPL 通过 `/mode plan` 等切换(粘滞)。

完整生命周期:

```
                /mode plan                        /approve 或 --mode=apply           /merge
read ──► analyze ──► write_analyze ──► plan ──► apply ──► verify ──► applied ────► main
              │           │              │ (planner    │ (coder 在  │ (verifier      (fast-forward
              │           │              │  写 plan)    │  worktree │  跑 run_tests) 或 cherry-pick
              │           │              ▼              │  里逐单元                    到新分支)
              │           │          /plan show         │  apply_patch)
              │           │          /reject [reason]   ▼
              │           │                          /worktree list / discard
              │           ▼                          /verify [plan-id]
              │      WriteAnalysisIR
              │      (kind / scope / risk
              │       / 多阶段提议)
              ▼
       AnalysisIR(分类信号)
```

| 阶段 | Agent | 工具 | 产出 |
|---|---|---|---|
| `analyze` | `analyzer` | (与读模式同) | 复用读模式 analyzer 作分类器,产 AnalysisIR(读模式字节级保持不变) |
| `write_analyze` | `write_analyzer` | `emit_write_analysis` | `WriteAnalysisIR`:任务 kind(feature / fix / chore)/ scope / risk flag / 期望结果 / 可选多阶段拆分提议。pin 在第一份 ChangePlan 上,retry / multi-phase 复用 |
| `plan` | `planner` | `emit_change_plan` / `emit_plan_skeleton` / `emit_plan_change`(多轮流式) | `ChangePlan` JSON 落盘(`.codrax/plans/<id>.json`),含 Changes[](path / kind / rationale / depends_on / new_content / patch / new_path) + AcceptanceTests[] + TargetPaths[] + 嵌入的 WriteAnalysisIR |
| `apply` | `coder` | `apply_patch`(每 ChangeUnit 一次,schema 仅 `{path, kind}`,内容从 Mutable.ChangePlan 取防转抄错) | 在 git worktree 里按拓扑序写文件;支持 `create` / `modify` / `delete` / `patch` / `rename`;W1 / W1b 闭包检查;同路径连续 3 次拒 → plan-defect 升级 |
| `verify` | `verifier` | `run_tests` + 可选 `emit_test_results` | 在 worktree 里自动探测 12 种 runner,产 `ChangeReport`;LLM 可选发 narrative 区分 REGRESSION / PRE-EXISTING / FIXED,但 `Passed` verdict 来自 parser 不被覆盖 |

**`run_tests` 识别 12 种 runner**:Go(`go test -json`)、Node(jest/vitest `--json`)、Python(pytest-json-report)、Rust(cargo test)、Java / Kotlin(Maven `mvn test` 或 Gradle `./gradlew test`,JUnit XML)、Ruby(RSpec `--format json`)、Swift(`swift test`,Package.swift 探测)、CMake(ctest `--output-junit`)、Meson(`meson test --xunit-file`)、Make(`make check` / `make test`,exit-code 判定)、HarmonyOS hvigor(`hvigorw test`,JUnit XML 复用 Java 解析)、Cangjie cjpm(`cjpm test`,Cargo 风格文本)。探测通过仓根的 manifest 文件(`go.mod` / `oh-package.json5` / `cjpm.toml` / `package.json` / `Cargo.toml` / `pom.xml` / `build.gradle.kts` / `Package.swift` / `CMakeLists.txt` / `Makefile` 等),HarmonyOS / Cangjie manifest 优先级排在通用语言之前。**零测试发现**(pytest exit 5、jest "no tests found"、`go test ./...` 无 `_test.go`)被作为一等信号 `NoTestsRunners` 单独记录,不会被误判为测试失败。缺失 runner 会在 verify 阶段 fail-loud。

**沙箱语义**:
- 所有写操作都在 `git worktree add` 出来的独立 worktree 里执行,主仓库 HEAD 字节不变。
- `apply_patch` 强制 W1(`path` 必须在 `ChangePlan.TargetPaths`)+ W1b(`DependsOn` 必须已 applied)。违规直接拒绝,coder 看到错误可在下一轮自修正。
- 默认 Run 结束**销毁** worktree(清理磁盘)。开启 `pipeline_keep_worktree_on_success: true` 后,apply + verify 双双通过时保留 worktree,用户可 `cd` 进去 review / cherry-pick 到主仓。失败路径无条件清理。

**verify→plan 重试循环**:`pipeline_write_retry_budget` 控制 verify 失败后自动重新规划次数。失败 summary + top-3 失败测试 + 前一 plan 的 `TargetPaths`(嫌疑文件清单)喂给 planner 做二次规划。硬上限 5 次,默认 3。两条早停守门:`runner_missing` 一等信号(`pytest: command not found` 等)直接 fall-through 不重试,给安装提示;`verifyStallReason` fingerprint 比对——AppliedCount / VerifyPassed / VerifyFailed / FailureSummaryHash 完全 byte-equal → 视为"无进展"跳本轮 retry,避免 budget 烧光在原地重打。

**baseline 捕获**(可选):`pipeline_baseline_capture_enabled: true` 会在 apply 前跑一遍测试套件作为 baseline 快照。verifier LLM 提示里会列出 baseline 已失败的测试,引导区分"这个 plan 造成的 REGRESSION"与"预存 PRE-EXISTING 失败"。默认关闭(测试墙钟时间翻倍)。

**合并回主仓**(`/merge` + `/approve --merge-to=`):codrax 提供两条优雅的合并通路,默认 fast-forward 到当前分支(`main`),或显式 `--branch=<name>` 在主仓上拉新分支 + cherry-pick(走 PR 流程)。冲突或主仓工作区脏时自动回滚到合并前状态、不留半成品分支;`git push` 永远是用户手动操作。`/approve --merge-to=fix/x` 把 approve + 合并合成一步,对已知目标分支的场景把手动操作降到最少。

**裸目录脚手架**(三档授权):目标是空目录或 `git init` 后还没 commit 时,默认 fail-loud。三种方式可显式授权 codrax 自动 `git init` + 空 initial commit 后再 apply:CLI `--auto-init-repo`、yaml `write_auto_init_repo: true`、REPL 交互 y/N 同意。任何路径都不会动用户已有的 git 状态。

详见 [用户指南 · 写模式](docs/user_guide.md#43-写模式)。

## 构建

Tree-sitter 语法解析需要 CGO:

```bash
# Linux (Debian/Ubuntu)
sudo apt-get install gcc musl-tools

# macOS
xcode-select --install

# Windows (通过 MSYS2 或 scoop 安装 mingw-w64)
```

```bash
make               # 编译,输出 ./codrax
make static        # 全静态 musl 链接(仅 Linux)
make release       # 交叉编译全平台到 dist/
make test          # 运行所有测试
```

## 运行

```bash
# 交互(默认,无 --request 即进入)
./codrax

# 单次运行(读模式)
./codrax --request "解释 explorer 的 ShouldStop 是怎么决定的"

# 诊断(debug 级别 ReAct trace 写入 logs/ 同时镜像到 stdout)
./codrax --log-level debug --log-stdout --request "your question"

# 多目标仓 — 日志和 memory 按 --repo 绝对路径 hash 自动隔离
./codrax --repo /path/to/repoA --request "..."
./codrax --repo /path/to/repoB --request "..."   # 不会和 repoA 混

# 写模式单次(需 write_enabled: true)
./codrax --mode=plan --request "add X feature" --plan-out /tmp/p.json
./codrax --mode=apply --plan-file /tmp/p.json --auto-apply
./codrax --mode=verify --plan-file /tmp/p.json
```

REPL 斜杠命令(支持 `\` 前缀别名,如 `\exit` ≡ `/exit`):

```
通用:  /help  /exit  /quit  /version  /history  /clear  /compact
       /log   /htrace  /atrace  /paste  /chat  /env  /mermaid  /cancel
写模式: /mode  /plan  /approve  /reject  /verify  /worktree  /merge
       /branch  /baseline  /phase  /pitfalls
```

每条都支持 Tab 补齐;`!shell-cmd` 直通系统 shell。

### 粘贴兜底(SSH / tmux 环境)

交互 REPL 默认依赖终端 bracketed paste 折叠多行粘贴成 `[Pasted text #N +L lines +C chars]` 占位 token。SSH + 老版 tmux / 某些 `$TERM` / 部分 SSH 客户端会吃掉 `\x1b[200~` / `\x1b[201~` 标记,`/paste` 是兜底:

```
❯❯ /paste                    # 进入采集模式
  paste> <贴入多行内容>
  paste> /end                 # 单独一行 /end 结束
❯❯ 这个 stack trace 的根因是什么?   # 下一条提问自动带 [Pasted text #0] token
```

单次有效;与 `/log`(把日志送进 attached-log 通道,触发结构化分诊)是完全不同的通路 —— `/paste` 走普通 request 通道。

## 日志分诊 + 性能 trace 分诊

两条独立的条件前置阶段。粘贴一段运行时日志或性能 trace,codrax 会在 analyze 之前先跑对应阶段把它读成结构化数据,然后把文件定位 / 关键词 / 性能信号喂给下游 explorer。

### 日志分诊(`log_triage`)

支持的格式不是固定列表 —— LLM 抽取支持任意它能理解的栈格式:Go panic、Java exception 含 `Caused by` 链、Kotlin JVM 栈、C/C++ ASAN / UBSAN / gdb、Python traceback(含 `During handling` 嵌套)、Node.js V8、Rust `#[source]` 链、Ruby backtrace、HarmonyOS hilog、Android logcat、ArkTS V8 栈帧、Cangjie panic …… 走一套代码。

```bash
# 单文件
./codrax --request "这个 panic 哪来的?" --log /tmp/panic.txt

# 多文件(可重复 --log)
./codrax --request "对比两个 pod 的崩溃" --log a.log --log b.log

# 从 stdin
kubectl logs pod/api | ./codrax --request "analyze this crash" --log -

# 内联
./codrax --request "分析" --log-text "$(cat /tmp/asan.out)"

# C/C++ build 路径前缀
./codrax --request "trace" --log /tmp/asan.out \
  --log-source-prefix /home/jenkins/workspace/build/src/
```

REPL:`/log <path>` / `/log append <path>` / `/log show` / `/log clear`,跨轮次粘滞。

### 性能 trace 分诊(`perf_triage`)

读 HiTrace / atrace / systrace / perfetto 文本输出,emit 结构化 `PerfBundle`(Frames / Janks / Stalls / Startup),按 16.6 ms 帧预算 / 100 ms 主线程阻塞 / 1.2 s 冷启动阈值自动打 signals。

```bash
# HarmonyOS
hdc shell hitrace -t 10 ace graphic > /tmp/perf.trace
./codrax --request "为什么这个页面打开有掉帧?" --htrace /tmp/perf.trace

# Android(--atrace 是别名,等价 --htrace)
adb shell atrace -t 10 view gfx app > /tmp/perf.atrace
./codrax --request "where is the jank?" --atrace /tmp/perf.atrace

# 多 trace 合并(对比多次冷启动)
./codrax --request "对比这三次冷启动" --htrace boot1.trace --htrace boot2.trace --htrace boot3.trace

# 同时挂 panic 日志 + 性能 trace
./codrax --request "卡顿后 crash 的根因" --log /tmp/hilog.txt --htrace /tmp/jank.atrace
```

REPL:`/htrace <path>` / `/htrace append <path>` / `/htrace show` / `/htrace clear`(`/atrace ...` 是别名)。

### 大日志 / 大 trace

单次抽取默认读完全文。当超过两步阈值(`log_triage_two_step_bytes` 默认 32 KB / `perf_triage_two_step_bytes` 默认 64 KB)或单次覆盖率偏低时自动切两步:先让 LLM 按字节范围切片(`emit_log_segmentation` / `emit_perf_segmentation`),再逐段抽取,最后合并结果。LLM 调用次数有硬上限(`log_triage_max_llm_calls` / `perf_triage_max_llm_calls` 默认 12)。多文件附加之间会自动插入 `# codrax-source: <path>` 边界头。

字节上限:`log_attach_max_bytes`(默认 50 MiB)、`trace_attach_max_bytes`(未设时继承 log)。硬顶 1 GiB。

### 暂不支持

- 实时日志 tail / 订阅、远端日志源(Loki / ES / CloudWatch)
- glibc 裸 backtrace(只有地址没有 file:line,缺少足够锚点)
- 二进制 perfetto trace(只支持文本 dump;二进制需先用 `perfetto --query` / `traceconv` 转文本)

## 配置

两个 YAML 平铺在二进制同目录,分工明确:

| 文件 | 负责 | 典型键 |
|---|---|---|
| [`providers.yaml`](providers.yaml.example) | LLM 凭证与路由 — 每个 agent 用哪个 provider | `api_key` / `model` / `base_url` / `stream` / `tls_*` |
| [`codrax.yaml`](codrax.yaml.example) | 本次运行怎么跑 — 日志 / memory / 语言 / 目标 repo / 流水线预算 / 写模式开关 + 写模式调优 | `log_level`, `memory_dir`, `lang`, `repo`, `branch`, `pipeline_max_steps`, `pipeline_write_retry_budget`, `pipeline_baseline_capture_enabled`, `pipeline_keep_worktree_on_success`, `write_enabled`, `blob_*`, `log_max_files` |

流水线拓扑(读模式 4 阶段 × 4 agent + 写模式 plan/apply/verify)是硬编码的,没有 YAML 对应项。

优先级(低到高):**代码默认 < `codrax.yaml` < 命令行 flag**。每个字段都可以在任一层覆盖。

### 路径锚点

一个二进制跨多目录、多仓库运行时的两个锚点:

- **配置锚点**(`<exeDir>`):`providers_config` 的相对路径在此解析。部署时把 `codrax` + `codrax.yaml` + `providers.yaml` 三个文件扔一起即可。
- **运行产物锚点**(`<CWD>/.codrax/`):`log_dir` / `memory_dir` / `cache_dir` / blob 会话目录 / plans 目录在此解析。用户在哪个工作目录启动 codrax,日志就落在哪个工作目录的 `.codrax/` 下。

### `codrax.yaml` 查找顺序

1. `$CODRAX_SETTINGS`(绝对路径,显式)
2. `<exeDir>/codrax.yaml` ← 主路径
3. `<exeDir>/codrax/codrax.yaml`(bin + share 分离布局)
4. Legacy 路径,找到时打 deprecation warning:
   - `<exeDir>/config/codrax.yaml`
   - `<exeDir>/../config/codrax.yaml`
   - `<CWD>/config/codrax.yaml`

### 目录结构

```
<exeDir>/                        ← 配置
  codrax
  codrax.yaml                    (复制自 codrax.yaml.example)
  providers.yaml                 (复制自 providers.yaml.example)

<CWD>/.codrax/                   ← 运行产物(单个隐藏根,.gitignore 一条搞定)
  logs/<repo-slug>/
    codrax-<timestamp>-<pid>.log
  memory/<repo-slug>/
    MEMORY.md
    turns/
  blob/<timestamp>-<pid>/        (每进程一个 session,保留最近 N 个)
    <tool>-<sha8>.txt
  plans/                         (写模式 ChangePlan + ChangeReport)
    <plan-id>.json
    <plan-id>.report.json
  worktrees/                     (写模式 git worktree 根;默认 Run 结束销毁)
    <trace-id>-<pid>/
```

`log_max_files` / `blob_max_sessions`(默认都是 7)控制滚动保留数量;存活 PID 的 peer 文件永不清理。

## 功能亮点

- **分级日志**:error / warning / info / debug 四档,按 4 MB 滚动、保留 7 份;文件名 `.codrax/logs/<repo-slug>/codrax-YYYYMMDD-HHMMSS-mmm-<pid>.log`,每个进程独占自己的文件,多实例并发不撕日志
- **交互多轮**:REPL 每一轮自动带前轮上下文;超过 6 轮或 20 KB 触发 LLM 摘要压缩成 `MEMORY.md` 索引条目,下次相关话题可按关键词召回
- **跨重启恢复**:单实例崩溃恢复时,`.codrax/memory/<repo-slug>/turns/` 未压缩的最近 6 轮自动回灌;多实例并发下检测到 peer 时跳过,避免互抢对方轮次
- **多目标仓隔离**:log / memory 默认按 `--repo` 绝对路径 hash slug 分目录,同一 codrax 安装可服务多仓互不污染
- **多实例并发安全**:同一目标仓多开 codrax 时,日志按 PID 隔离、`MEMORY.md` 周期写入由 `flock` 串行化、`/clear` 提示对等进程数、retention sweep 跳过活进程文件
- **跨平台**:Linux / macOS 用 `flock(2)`,Windows 通过 `kernel32.dll!LockFileEx` 实现等价语义,全程零非 stdlib 依赖
- **默认语言**:`--lang=zh` 默认简体中文作答;`--lang=off` / `none` 关闭;任一非空值保留"用户若用其他语言提问则跟随"兜底
- **Answer contract**:finalizer 输出 typed `AnswerDocumentV2`(9 种 typed block:summary / section / ordered_list / bullet_list / scalar / decision / table / diagram / caveat + citation 池);8 种 QuestionFamily 各自的 RequiredBlocks 合同决定该题该填哪些 block;cardinality 验证器把"谎称 complete 但 slate 不足基线"的 claim 自动降级为 lower_bound;retry 用 `emit_answer_document_patch` 增量补丁结构性保留 byte-identical 的未改 block(避免 LLM 重写时把之前精心标注的 claim_uses / facet_ids 顺手丢了)
- **日志分诊**:`--log / --log-text` 或 REPL `/log` 附加运行时日志,LLM 驱动的 log_triage 阶段把任意格式 panic / exception / sanitizer / traceback / 结构化应用日志解析成结构化锚点;日志正文独立通道不污染提问关键词识别
- **写模式沙箱**:`analyze → write_analyze → plan → apply → verify → merge` 全部在 git worktree 里跑,主仓 HEAD 永不自动变;严格 W1/W1b 写闭包检查;可选 verify→plan 重试循环(fingerprint stall 早停)+ baseline 回归对比 + worktree TTL/quota 回收 + 12 种 test runner 自动探测;`NoTestsRunners` 一等信号区分"零测试发现"与"测试失败";`FailureKindRunnerMissing` 一等信号识别"runner 二进制没装"并跳过 verify→plan 重试直接给安装提示;analyzer 在写模式下跳过 `hypothesis_coverage` / `contract_complete`(读模式专属质量门),让"用 python 写一个猜数字游戏"这种从零起步的请求顺利分类;执行进程组隔离(Linux `Setpgid` / Windows JobObject)+ 内存/CPU 上限,防止失控测试拖死主进程;跨 Run Failure Taxonomy 持久化让 planner 看到"本仓库历史踩过的坑"
- **合并回主仓优雅化**:`/merge` 把 worktree 里的 commit 自动 fast-forward 到当前分支或 cherry-pick 到新分支(`--branch=fix/x`);`/approve --merge-to=fix/x` 一步合并;`--include-failed` / `--force` 在 review 后强制合入 verify_failed plan(适用于本地起不了集成测试、想扔 CI 跑的场景);冲突 = 完整回滚不留半成品,主仓工作区脏不动;裸目录 + commitless repo 三档授权(REPL y/N、yaml `write_auto_init_repo`、CLI `--auto-init-repo`)允许自动 `git init` 脚手架
- **测试 runner 跨 worktree 用主仓 deps**:verify 在沙箱 worktree 里跑,但 `.venv/` / `node_modules/` / `vendor/` / `oh_modules/` 这类 gitignored 依赖目录都在主仓里(worktree 里不存在)。codrax 自动:Python 通过 `<main>/.venv/bin/python -m pytest` 直接用主仓 venv 解释器(系统 python3 兜底);Node / Ruby / hvigor 把主仓的 `node_modules/` / `vendor/` / `oh_modules/` 软链到 worktree 里,让 `npm test` / `bundle exec` / `hvigorw` 自然解析依赖。runner_missing 信号区分"二进制没装"和"模块没装"两种 env 错误,跳过 verify→plan 重试,直接给中英双语安装提示。Go / Rust / Swift / Java(maven/gradle)/ CMake / Meson / Make / cjpm 用全局缓存或源码重建,不需要这套联动
- **REPL UX 调淡**:`SUCCESS` / `INFO` / `WARN` / `ERROR` 前缀去掉了 pterm 默认的高对比背景色,只用单字符 + 彩色前缀(`• ✓ ⚠ ✗`),消息体走终端默认色,不再咋眼
- **/help 显示子命令**:`/plan show|list|clear`、`/log show|clear|append`、`/htrace show|clear|append`、`/worktree list|discard`、`/mode read|plan|apply|verify`、`/approve [<plan-id>] [--merge-to=][--skip-verify]`、`/merge [--branch=][--include-failed]`、`/env show|probe|explain|cache|stats`、`/baseline`、`/phase`、`/pitfalls` 全部缩进显示在 `/help` 里 —— 子命令一目了然
- **多 pending plan 时精确 approve**:`/approve <plan-id>` / `/plan show <plan-id>` 定向到任意 PlanStore 条目;弹出 confirm 前自动提示"还有 N 个其它可批准的 plan";`/approve --skip-verify` 在本地起不了集成测试时只 apply 不跑 verify(扔给 CI 跑),自动**蕴含 keep-on-success**(无需手动开 yaml 旋钮),plan.WorktreePath 持久到磁盘以便 `/merge` 找得到
- **当前 git 分支感知**:启动 banner 和每行 REPL prompt 的 sticky tag 带 `[git:<branch>]`(detached 显示 `[git:detached@<sha>]`),跨进程切换分支也能在下一行 prompt 反映;`/branch [<name>]` 在 REPL 内查看 / 切换分支(`git checkout` 透传,支持 `-b new-name` 创建);`/approve --merge-to=` 和 `/merge`(无 flag)默认目标自动跟随实时 git HEAD,不再用启动时的 `--branch` 粘滞值
- **`!shell` 直通**:`!ls` / `!cat foo` / `!grep -rn ...` 等系统 shell 命令在 REPL 内直接执行,工作目录是 r.repoRoot,stdout/stderr 实时显示。`!cd` 特殊提醒(bare cd 不持久,链式 `!cd /tmp && ...` 有效)
- **环境诊断与推荐**:测试 runner 缺失或裸目录拒接时,`/env` 系列命令(`/env show|probe|explain|cache|stats`)启动 3 层管线——OS / shell / 包管理器 / 项目 manifest probe → 6 检测器 diagnose → LLM 推荐 + DocsLink 兜底。结果按 `Strategy`(Venv > Project > User > ToolchainBootstrap > Global > Docs)排序,每条命令带 `!` 前缀直接复制粘贴;90 天磁盘缓存避免重复调用,`recommend_global_install: false`(默认)下不产 sudo 命令
- **多阶段方案组**(可选):大改动 plan 可 split 成 sequential 多阶段,每阶段独立 plan→apply→verify + 独立 retry 预算,同 worktree 累积。`/phase` 命令查看阶段进度;每阶段过 LLM-driven acceptance check 分类 PhaseAccepted / PhaseRolledBack / PhaseAcceptanceUnverified
- **AnswerDocumentV2 carrier**:V2 block-only 答案载体替代 V1 shape enum——LLM 用 typed `blocks[]` 构造答案,validator 读 typed enum 不读 prose;`emit_answer_document_patch` 让 retry 时 LLM 声明"哪些 block 保留 byte-identical / 哪些替换 / 哪些追加 / 哪些删除",系统结构性保留所有 typed 注解(claim_uses / facet_ids / surface_role / edge_anchors)

## 文档

- **[用户指南](docs/user_guide.md)** — 面向使用者:安装、配置、两种模式、写模式全流程、场景实操、常见问题
- **[架构设计](docs/architecture.md)** — 面向开发者:组件详情、数据结构、Turn A/Turn B 分离、分析后处理管线、运行时子系统、写模式内部机制
- **[运行时配置示例](codrax.yaml.example)** — 所有可调项的完整列表
- **[Providers 配置示例](providers.yaml.example)** — LLM provider 凭证 + 每 agent 模型路由
