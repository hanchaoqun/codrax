# codrax 使用指南

> **CODRAX — Code Of Deterministic Reasoning, Anchored eXplanation.**
> **CODRAX —— 推理确定,解释锚定。**
> 每条结论锚到字节,每段推理可被复核。
> **确定性诚实,不是态度,是契约。**

> **文档对应版本**:`0.1.20260424`(CalVer,`make` 产出的二进制 `codrax --version` 打印准确值)

本文档面向从未用过 codrax 的用户,讲清楚:**怎么装、怎么配、怎么用、各模式怎么选、界面怎么看、典型场景怎么跑**。不讨论编译和内部实现。

- [1. codrax 是什么](#1-codrax-是什么)
- [2. 安装与启动](#2-安装与启动)
  - [2.1 运行环境依赖](#21-运行环境依赖)
- [3. 配置](#3-配置)
  - [3.1 `providers.yaml` — 精简版](#31-providersyaml--精简版)
    - [3.1.1 HTTP / HTTPS 开关](#311-http--https-开关--靠-base_url-协议头决定)
    - [3.1.2 上下文窗口 — `context_window`](#312-上下文窗口--context_window)
    - [3.1.3 流式开关](#313-流式开关--stream)
    - [3.1.4 场景速查表](#314-场景速查表)
  - [3.2 `providers.yaml` — 复杂版](#32-providersyaml--复杂版)
  - [3.3 `codrax.yaml` — 运行时参数](#33-codraxyaml--运行时参数)
  - [3.4 配置文件查找顺序](#34-配置文件查找顺序)
- [4. 使用模式](#4-使用模式)
  - [4.1 REPL 交互模式(读模式)](#41-repl-交互模式读模式)
  - [4.2 单次命令模式(读模式)](#42-单次命令模式读模式)
  - [4.3 写模式 — plan → apply → verify](#43-写模式--plan--apply--verify)
- [5. 命令速查](#5-命令速查)
  - [5.1 CLI 命令行参数](#51-cli-命令行参数)
  - [5.2 REPL 斜杠命令](#52-repl-斜杠命令)
- [6. 输出界面速读](#6-输出界面速读)
- [7. 场景实操](#7-场景实操)
  - [7.1 问一段代码的行为](#71-问一段代码的行为)
  - [7.2 分析 panic / 异常日志](#72-分析-panic--异常日志)
  - [7.3 写模式:小步修复一个 bug](#73-写模式小步修复一个-bug)
  - [7.4 脚本化批处理](#74-脚本化批处理)
  - [7.5 同一台机器操作多个仓库](#75-同一台机器操作多个仓库)
  - [7.6 长对话与记忆管理](#76-长对话与记忆管理)
- [8. 常见问题](#8-常见问题)

---

## 1. codrax 是什么

codrax 是一个**代码分析 + 变更提议**工具,默认只读,可选写:

- **读模式**(默认 / 开箱即用):输入关于代码仓库的自然语言问题(中文 / 英文均可),可选附带一段运行时日志(panic、异常栈、编译错误、traceback 等);输出带 `file:line` 级别 citation 的结构化答案。**不修改**任何源文件。
- **写模式**(opt-in,需 `codrax.yaml :: write_enabled: true`):驱动 `plan → apply → verify` 流水线在一个**沙箱 git worktree** 里完成文件写入 + 测试验证;主仓库 HEAD 字节永不自动变更,必须用户 `/approve` 显式审批才会落地进 worktree,合并到 main 仍由用户手动 `git cherry-pick` / `rebase`。
- **共同规则**:不执行仓库外有副作用的外部调用;不上传代码到任何第三方(除你在 `providers.yaml` 里指定的 LLM)。

使用 codrax 前,你只需要准备:

1. 一个 codrax 可执行文件(见下一节)。
2. 一把能访问 OpenAI 兼容接口的 LLM API key(OpenAI、DeepSeek、Qwen、Ollama、vLLM、Together 等都兼容)。
3. 一份想问问题 / 改动的代码仓库。

---

## 2. 安装与启动

### 拿到可执行文件

从官方 Release 下载对应平台的二进制,或请团队管理员给你一份。文件名通常是 `codrax`(Linux/macOS)或 `codrax.exe`(Windows)。本指南统称为 `codrax`。

> 编译源码不在本文档范围;如需自行编译请参见 [README.md](../README.md) 的"构建"一节。

### 推荐目录布局

把 `codrax` 放在一个固定目录(例子用 `~/tools/codrax/`),两份配置文件**紧挨着二进制**:

```
~/tools/codrax/
├── codrax              ← 可执行文件
├── codrax.yaml         ← 运行时参数(可选,首次可跳过)
└── providers.yaml      ← LLM 凭证(必填)
```

### 把 codrax 加进 PATH(一次性操作)

```bash
# bash
echo 'export PATH="$HOME/tools/codrax:$PATH"' >> ~/.bashrc
exec "$SHELL" -l

# zsh
echo 'export PATH="$HOME/tools/codrax:$PATH"' >> ~/.zshrc
exec "$SHELL" -l
```

加进 PATH 后,无论你 `cd` 到哪个仓库,都可以直接敲 `codrax` 启动;运行时产物(日志、对话记忆、工具输出)都会**自动生成在当前工作目录下的 `.codrax/` 文件夹**里,不会污染到源码仓本身,也不会和其它仓库混在一起。

### 首次启动

```bash
cd /path/to/your/repo
codrax
```

看到类似下面的 banner 就说明启动成功:

```
   CODRAX  v0.1.20260424  /help · /exit

❯❯
```

如果启动报 "providers config not found" 之类的错误,回到 [3.1 精简版](#31-providersyaml--精简版) 配一份最小的 `providers.yaml` 即可。

### 2.1 运行环境依赖

codrax 在调用外部命令行工具时坚持"能退则退":任何可选工具缺失都不会拒绝启动,但**具体能力会降级**。启动时 `--log-level info` 以上会打出三条 `*** backend:` 日志,直接告诉你当前用的哪一套。

**必需 / 强推荐**

| 工具 | 作用 | 缺失时的表现 |
|---|---|---|
| **POSIX shell** (`sh` / `bash` 或 Windows 的 `cmd`) | 承载 `exec_command` 工具 | Unix 上假设 `/bin/sh` 存在(几乎所有发行版保证);Windows 先尝试 Git for Windows 自带的 sh/bash,再退回 `cmd /C`(但 POSIX 语法命令会失败) |
| **git** | `git_diff` / `git_log` 工具;repomap 扫描加速 | 两个 git 工具返回带错误信息的 ToolResult(不 crash);repomap 自动回退到 `filepath.Walk` 扫描,变慢但能跑 |

**可选(有自动兜底)**

| 工具 | 作用 | 缺失时的表现 |
|---|---|---|
| **ripgrep** (`rg`) | 最快的代码搜索后端 | 依次回退:`grep` → **内置 Go regex 扫描**(`native` backend)。所有层级都能跑,仅速度差异 |
| **grep** (GNU / BSD) | ripgrep 缺失时的搜索后端 | 回退到内置 Go regex 扫描,仍然可用 |
| **find** | `keyword_search` 按文件名定位的加速 | 自动使用 Go `filepath.WalkDir` 兜底,无感知 |

> **最小可用环境**:没有 `rg` / `grep` / `find` 的裸 Linux(`FROM scratch` / `distroless`)启动后依然能跑,仅搜索阶段变慢(典型大仓 100-500 ms → 1-3 s)。
>
> **Windows 用户强烈建议**:装 [Git for Windows](https://git-scm.com/download/win) —— 它会同时提供 `git` / `sh` / `bash` / `grep` / `find` 这几个关键工具。再用 [ripgrep releases](https://github.com/BurntSushi/ripgrep/releases) 或 winget (`winget install BurntSushi.ripgrep.MSVC`) 装一下 ripgrep,搜索速度会有量级差。
>
> **macOS 用户**:系统自带 `git` / `sh` / `bash` / `grep` / `find`(BSD 版)。想获得最佳速度:`brew install ripgrep`。

**启动时 banner(示例)**

```
2026-04-23 ... INFO  search backend: ripgrep (/usr/bin/rg)
2026-04-23 ... INFO  shell backend: sh [-c]
2026-04-23 ... INFO  git backend: /usr/bin/git (git version 2.43.0)
```

缺少某个工具时每行末尾会附带一条**平台相关的安装建议**,例如:

```
WARN  search backend: native Go scanner (neither ripgrep nor grep found on PATH — install ripgrep for faster scans; ...)
WARN  git not found on PATH — repomap scanning falls back to filesystem walk; git_diff / git_log tools disabled.
      Install via your distro package manager (apt/yum/apk install git).
```

这几条位于 "paths: ..." 之后、真正开始分析之前,按 info 级别打,平时走日志文件;用 `--log-stdout --log-level info` 可实时看到。

**所有 git 调用都带 30-60 秒超时**,大仓即便 `git log --name-only` 扫到几十万提交也不会拖死 pipeline,超时后只丢失那一次调用的结果。

---

## 3. 配置

codrax 有两份配置文件,分工严格不重叠:

| 文件 | 负责 | 是否必填 |
|---|---|---|
| `providers.yaml` | LLM 凭证(API key)、每个 agent 用哪个模型 | **必填** |
| `codrax.yaml` | 语言偏好、日志目录、流水线预算、各种阈值、其他调参 | 可选,默认值开箱即用 |

### 3.1 `providers.yaml` — 精简版

> **硬性规则**:`provider` / `api_key` / `model` / `base_url` **四个字段必填**,缺一个 codrax 就直接拒启动并打印缺失字段名 —— 不会偷偷用任何公网 endpoint 或默认模型。

所有 4 个 agent(`analyzer` / `explorer` / `extractor` / `finalizer`,日志分诊时还有 `log_triager`)共用同一个模型就能跑:

```yaml
llm:
  default:
    provider: openai                       # 只实现 openai 协议;deepseek /
                                           # qwen / vllm / Ollama 等兼容此协议
                                           # 的服务共用同一段,换 base_url 即可
    api_key: "sk-xxx"                      # 换成你自己的 API key
    model: "your-model-id"                 # 换成你要用的模型名
    base_url: "https://your-endpoint/v1"   # http:// 或 https:// 都行
```

**常见第三方 provider 的改法**(只改 `base_url` + `model`):

```yaml
# DeepSeek
llm:
  default:
    provider: openai
    api_key: "sk-deepseek-xxx"
    model: "deepseek-chat"
    base_url: "https://api.deepseek.com/v1"

# 阿里云 DashScope / Qwen
llm:
  default:
    provider: openai
    api_key: "sk-qwen-xxx"
    model: "qwen-max"
    base_url: "https://dashscope.aliyuncs.com/compatible-mode/v1"

# 本地 Ollama / vLLM
llm:
  default:
    provider: openai
    api_key: "ollama"                      # 非空即可
    model: "llama3.1:70b"
    base_url: "http://localhost:11434/v1"
```

#### 3.1.1 HTTP / HTTPS 开关 —— 靠 `base_url` 协议头决定

codrax **不另外加** HTTP vs HTTPS 开关。协议由你 `base_url` 写的是 `http://` 还是 `https://` 自动决定:

```yaml
base_url: "https://api.provider.com/v1"   # → TLS(走系统 CA 池校验)
base_url: "http://internal-proxy:3000/v1" # → 明文 HTTP,无 TLS
base_url: "http://127.0.0.1:11434/v1"     # → 本地 Ollama,明文
```

HTTPS 遇到**企业自签 CA** / **私有 CA** 时,可以加两个额外字段(只在 `https://` 时生效;`http://` 会被忽略):

```yaml
llm:
  default:
    # ...
    tls_ca_file: /etc/ssl/my-corp-ca.pem        # 追加一份 CA 到系统 trust pool
    # 临时绕证书验证(**不安全**;会在启动打醒目 warning)
    # tls_insecure_skip_verify: true
```

#### 3.1.2 上下文窗口 —— `context_window`

让 codrax 知道你模型的最大输入 token 窗口,用于:

1. **fraction-form byte budget**(见 §3.3.5 / §3.3.7):`blob_max_inline_fraction` / `agent_max_tool_history_fraction` 这类"占比"配置依赖它做换算(fraction × context_window × 4 B/token)
2. **context-pressure watchdog**(见 §3.3.7):ReAct 循环每轮估算 prompt 字节,接近 soft 阈值写 warning,到 hard 阈值强制收尾并注入针对当前 agent 的"用哪个 emit_* 工具关阶段"hint

```yaml
llm:
  default:
    # ...
    context_window: 200000    # 模型的 input token 窗口(tokens)
```

缺省时 adapter 回落到 128000 的保守估计,所有 fraction-form / watchdog 逻辑仍可运作但不贴合模型真实能力。常见模型参考值:

| 模型家族 | `context_window` 参考 |
|---|---|
| OpenAI gpt-4o / gpt-4.1 / o-series | `128000` / `200000` |
| Claude 4 全家 | `200000`(Opus 4.6+ 1M 节流版可到 `1000000`) |
| DeepSeek V3 | `64000`~`128000` |
| Qwen 2.5 / 3.0 | 按具体型号文档 |
| 本地 Ollama llama3.1 8B | `8192`(默认) |

> agent-level override 遵循"非零覆盖"规则:`llm.agents.<name>.context_window` 非 0 时覆盖,0 时继承 default。典型场景:主 agent 跑大窗口模型、`memory_summarizer` 用便宜的小窗口模型,两者各自声明即可。

#### 3.1.3 流式开关 —— `stream`

| 值 | 行为 |
|---|---|
| `stream: true` | 走 SSE 流式,REPL 任务行会**实时显示** LLM 正在输出的内容(250ms 节流,80 字符 tail 预览),`/chat` 会以 typewriter 形式逐段上屏 |
| `stream: false` | 一次性收完整个响应 |
| 不写 | **默认 `stream: true`** |

```yaml
llm:
  default:
    # ...
    stream: false      # 想恢复一次性响应就显式写 false
```

即使你写 `stream: false`,部分 provider 依然会返 SSE(企业网关、特殊微调模型)。codrax 会**自动嗅探**响应开头,遇到 `data: ...` 直接走 SSE parser,不会因此报 `invalid character 'd'` 失败。

#### 3.1.4 场景速查表

| 部署形态 | `base_url` | `stream` | TLS 字段 |
|---|---|---|---|
| 公网大厂(OpenAI / DeepSeek / Qwen / Anthropic 代理 / ...) | `https://…` | 随意 | 通常不用 |
| 企业自签 CA 的 HTTPS | `https://…` | 随意 | `tls_ca_file` |
| HTTPS 证书一时搞不定,临时跑通 | `https://…` | 随意 | `tls_insecure_skip_verify: true`(用完关) |
| 本地 Ollama / vLLM | `http://…` | 随意 | 不用 |
| 内网不加密代理 | `http://…` | 随意 | 不用 |
| Provider 强制 SSE(我关了 stream 还报 JSON 错) | 任意 | 任意 | — (自动识别) |

### 3.2 `providers.yaml` — 复杂版

精简版对 4 个 agent 一视同仁。复杂版让你**针对每个 agent 单独选模型**(例如把便宜模型用在做文件摘要的 `explorer`,把强模型留给做问题分类的 `analyzer` 和写最终答案的 `finalizer`):

```yaml
llm:
  # default 永远必须存在;未在 agents 里显式覆盖的 agent 都继承它
  default:
    provider: openai
    api_key: "sk-shared-xxx"
    model: "your-cheap-model"              # 便宜快速的基准模型
    base_url: "https://api.provider.com/v1"

  agents:
    analyzer:
      model: "your-strong-model"           # 问题分类 / 场景推断 → 用强模型
    finalizer:
      model: "your-strong-model"           # 组织最终答案 → 用强模型
      # stream 默认继承 default(已经是 true),想关掉写 stream: false
    log_triager:
      model: "your-mid-model"              # 日志抽取 → 中等模型就够
    # explorer + extractor 没写,自动继承 default
```

**合法的 agent 键**(每个字段都是可选,空字段继承 default):

| agent | 作用 | 典型推荐 |
|---|---|---|
| `analyzer` | 对问题做一次性分类,决定答案形态与 TaskGraph | 最强模型,调用次数最少 |
| `explorer` | 反复读文件 / grep,收集证据;多轮调用 | 性价比模型,次数最多 |
| `extractor` | 把 explorer 收集到的证据结构化成答案素材 | 中等模型即可 |
| `finalizer` | 组织最终答案的散文与结构化字段 | 强模型,直接影响体验 |
| `log_triager` | 粘贴日志后,抽取错误类型 / 栈帧 / 因果链 | 中等模型即可,仅在附日志时启用 |

**per-agent 可覆盖的全部字段**(等同 3.1 里 `default` 能设的字段):

| 字段 | 说明 |
|---|---|
| `provider` / `api_key` / `model` / `base_url` | 凭证与模型路由 |
| `stream` | 每个 agent 独立开/关流式 |
| `tls_ca_file` / `tls_insecure_skip_verify` | 每个 agent 独立 TLS 策略(仅对走不同 `https://` endpoint 的 agent 有意义) |
| `think_aloud` | 是否注入 think-aloud 指令(对不带原生 reasoning 的模型有用) |

**在 agent 级别同时改 base_url**(例如让某个 agent 走本地模型):

```yaml
llm:
  default:
    provider: openai
    api_key: "sk-cloud-xxx"
    model: "your-strong-cloud-model"
    base_url: "https://api.cloud-provider.com/v1"
    stream: true

  agents:
    explorer:
      # 文件翻阅量大,放到本地省钱
      api_key: "ollama"
      model: "qwen2.5:32b"
      base_url: "http://localhost:11434/v1"   # ← 改走明文 HTTP
      stream: false                            # 本地模型想一次性取就显式关流式(默认是开)
```

### 3.3 `codrax.yaml` — 运行时参数

这份文件**完全可选**。没有它的时候一切走默认,也能正常使用。仅当你需要调优某条行为时才动。

完整示例请见 [`codrax.yaml.example`](../codrax.yaml.example)。下面按主题分组列出所有可调键。

#### 3.3.1 日志与诊断

| 键 | 默认值 | 作用 |
|---|---|---|
| `log_dir` | `logs` | 日志目录。相对路径相对于 `<CWD>/.codrax/` |
| `log_level` | `debug` | `error` / `warning` / `info` / `debug` 四档 |
| `log_stdout` | `false` | 设为 true 同时把日志打到 stdout(调试用) |
| `log_max_files` | `7` | 每个目录保留的日志文件数(按时间滚动) |

#### 3.3.2 对话记忆(REPL 场景)

| 键 | 默认值 | 作用 |
|---|---|---|
| `memory_dir` | `memory` | 记忆目录。相对路径相对于 `<CWD>/.codrax/` |
| `memory_max_recent_turns` | `6` | 最近多少轮对话逐字保留 |
| `memory_max_recent_bytes` | `20480` | 最近对话缓冲的总字节上限,任一边先超就压缩最旧轮次 |
| `memory_max_turn_body_bytes` | `16384` | 单轮对话 request + response 的最大字节,超过会尾部截断 |
| `memory_max_build_context_matches` | `3` | 每次提问最多召回多少条压缩历史作上下文 |

#### 3.3.3 响应语言与目标仓

| 键 | 默认值 | 作用 |
|---|---|---|
| `lang` | `zh` | 默认答案语言。`zh` / `en` / 任意字符串 / `off` / `none` |
| `repo` | `.` | 目标仓库路径(CLI `--repo` 覆盖) |
| `branch` | `main` | 默认分支(CLI `--branch` 覆盖) |

> `lang=zh` 时若用户用其他语言提问,codrax 会跟随用户语言作答(有兜底)。设 `off` / `none` 则完全让模型自选。

#### 3.3.4 流水线预算

| 键 | 默认值 | 作用 |
|---|---|---|
| `pipeline_max_steps` | `50` | 单次 Run 总步数上限(每个 LLM 调用或工具调用 = 1 步) |
| `pipeline_max_retries_per_stage` | `2` | 某个阶段连续失败多少次后放弃 |
| `pipeline_max_stage_visits` | `4` | 同一阶段最多被调度几次(防死循环) |

以上三个都有同名 CLI flag 可临时覆盖。

#### 3.3.5 工具产物(blob)大小

| 键 | 默认值 | 作用 |
|---|---|---|
| `blob_max_sessions` | `7` | 保留多少个历史 blob 会话目录(每次启动一个)。`0` 关闭持久化 |
| `blob_max_inline_bytes` | `32768` | 工具输出大于此阈值时转存为 blob,只在对话里保留头尾预览 |
| `blob_max_inline_fraction` | unset | 占比形式。`fraction × providers.yaml::context_window × 4 B/token` = 有效字节阈值;当 fraction 和 context_window **都**设置时覆盖 bytes 绝对值,否则回落到 bytes,再回落到代码默认。典型值 `0.02`(1M 窗口 → ~80 KB,8K 窗口 → ~640 B) |
| `blob_preview_head_bytes` | `24576` | 预览的头部字节数 |
| `blob_preview_tail_bytes` | `4096` | 预览的尾部字节数 |

#### 3.3.6 文件读取(`read_file`)

| 键 | 默认值 | 作用 |
|---|---|---|
| `readfile_small_limit_threshold` | `100` | 小文件读全保护阈值;`0` 禁用此保护,让 LLM 的 `offset/limit` 永远生效 |

#### 3.3.7 analyze 阶段质量门(`gate_*`)

| 键 | 默认值 | 作用 |
|---|---|---|
| `gate_coverage_min` | `0.6` | analyze 后的覆盖率下限,不达标会触发重试 |
| `gate_coverage_weight_symbol` | `1.0` | 符号类覆盖的权重 |
| `gate_coverage_weight_config` | `0.7` | 配置类覆盖的权重 |
| `gate_coverage_weight_concept` | `0.4` | 概念类覆盖的权重 |
| `gate_hypothesis_min_priority` | `0` | 假设优先级下限 |

#### 3.3.8 analyze 阶段运行时校验(`analysis_*`)

| 键 | 默认值 | 作用 |
|---|---|---|
| `analysis_warn_below_keywords` | `8` | 关键词低于此值打 warn |
| `analysis_reject_below_keywords` | `0` | 关键词低于此值直接拒 emit_analysis;`0` 禁用 |
| `analysis_generic_entity_blocklist` | `[count, function, thing, agent, handler, module]` | 通用名词黑名单,防止污染实体图。`[]` 关闭 |
| `analysis_reject_multiple_emit` | `false` | 同一 dispatch 内第二次 emit_analysis 是否直接拒 |
| `analysis_max_prescan_rounds` | `2` | analyze 阶段最多 pre-scan 轮次;`0` 禁用 |
| `analysis_warn_below_keyword_hit_ratio` | `0` | 关键词命中率下限告警;`0` 禁用 |
| `analysis_warn_below_entity_hit_ratio` | `0` | 实体命中率下限告警;`0` 禁用 |

#### 3.3.9 证据 grounding(`evidence_*`)

| 键 | 默认值 | 作用 |
|---|---|---|
| `evidence_grounding_floor` | `0.5` | 证据落地率下限;低于此值 `emit_investigation_complete` 不被接受。`0` 关闭,`1` 要求所有证据都必须落地 |

#### 3.3.10 CGEC 证据闭环(`cgec_*`)

| 键 | 默认值 | 作用 |
|---|---|---|
| `cgec_forced_reads_per_round` | `3` | 每轮 explore 最多"代读" 几个待读文件。`0` 关闭 |
| `cgec_stall_threshold_soft` | `2` | 连续多少轮 fingerprint 相同触发软停 + 补读。`0` 关闭 |
| `cgec_stall_threshold_hard` | `3` | 连续多少轮且未触发补读时降级完成。`0` 关闭强制完成 |

#### 3.3.11 agent 行为控制(`agent_*`)

| 键 | 默认值 | 作用 |
|---|---|---|
| `agent_max_iterations` | `20` | 每个 agent 单次 ReAct 循环最大轮次 |
| `agent_max_tool_history_bytes` | `153600` | 累积工具调用输出的字节上限,超出裁剪 |
| `agent_max_tool_history_fraction` | unset | 占比形式(同 `blob_max_inline_fraction`):fraction × context_window × 4 B/token = 有效字节。典型值 `0.3`(200K 窗口 → 240 KB,8K 窗口 → 9.6 KB) |
| `agent_context_pressure_soft_ratio` | `0.7` | 上下文压力软阈值。BaseAgent 每轮估 prompt 字节,`prompt / (context_window × 4) ≥ 此值` 时写 warning。置 `0` 关闭软告警 |
| `agent_context_pressure_hard_ratio` | `0.9` | 上下文压力硬阈值。达到即 force-stop ReAct 循环并注入针对当前 agent 的 HintComposer 指令(分 8 种 agent 定制,每种只指向该 agent 能调的终结工具;如 verifier 只看到 `run_tests`,coder 只看到 `apply_patch`)。置 `0` 关闭硬强停 |
| `agent_loop_min_inject_interval` | `3` | 两次 mid-loop hint 之间最少间隔轮次 |
| `agent_loop_max_continuations` | `5` | soft-stop 后最多追加多少条 continuation hint |
| `agent_loop_max_midloop_injects` | `6` | mid-loop hint 单次 dispatch 内最多注入次数 |
| `agent_loop_idle_stop_threshold` | `2` | 连续几轮空转就强停 |
| `agent_finalizer_max_correction_retries` | `2` | finalizer 缺 emit_answer_document 时最多再试几次 |
| `agent_finalizer_preserve_prior_prose` | `true` | 当 finalizer 重试时把前一版草稿保留到 summary |
| `agent_finalizer_shrinkage_min_prose_len` | `400` | 草稿长度下限,低于此长度不触发保留 |
| `agent_finalizer_shrinkage_ratio` | `0.5` | `len(summary)/len(prior)` 低于此值视作被压缩,触发保留 |
| `agent_extractor_max_correction_retries` | `1` | extractor 缺 emit_answer_symbol 时最多再试几次 |
| `agent_investigation_complete_policy` | `soft` | `soft` / `override` / `strict`;决定调度器如何处理 `emit_investigation_complete` |

**多 sub-topic 自适应预算**(当 analyzer 产出多话题时自动放大预算):

| 键 | 默认值 | 作用 |
|---|---|---|
| `agent_subtopic_prescan_extra` | `1` | 每 2 个 sub-topic 加几轮 pre-scan |
| `agent_subtopic_explorer_extra` | `3` | 每个 sub-topic 加几轮 explorer 迭代 |
| `agent_subtopic_planner_extra` | `3` | 每个 sub-topic 加几次 planner soft-cap 迭代（写模式 `--mode=plan`）；硬顶 20 |
| `agent_planner_complexity_extra` | `2` | analyzer 的 complexity 等级 × 此值额外加给 planner soft-cap：Simple +0、Moderate +2、Complex +4 |
| `agent_subtopic_pipeline_extra` | `5` | 每个 sub-topic 加几步 pipeline |
| `agent_subtopic_retry_extra` | `1` | 每 2 个 sub-topic 加几次 retry |

#### 3.3.12 explorer 启发式(`explore_*`)

常见默认都能用。调优时参考 [`codrax.yaml.example`](../codrax.yaml.example) 的完整注释。最常用的:

| 键 | 默认值 | 作用 |
|---|---|---|
| `explore_midloop_min_iteration` | `2` | 第几轮开始启用 mid-loop 启发式 |
| `explore_softstop_enum_coverage` | `0.8` | 枚举类问题的软停覆盖率阈值 |
| `explore_erm_suggest_limit` | `3` | 单次 mid-loop 最多建议几个证据缺口 |

#### 3.3.13 答案 summary 字数上限(`summary_cap_*`)

**默认全关**,finalizer 写多长就多长。需要严控长度时打开:

| 键 | 默认值 | 作用 |
|---|---|---|
| `summary_cap_enabled` | `false` | 主开关。`true` 才启用下面所有 summary 上限 |
| `summary_cap_explanation` | `2500` | 解释类答案正文字数上限 |
| `summary_cap_value` / `summary_cap_config_value` | `500` | 值/配置类答案 summary 字数上限 |
| `summary_cap_boolean` | `800` | 布尔类答案 summary 字数上限 |
| `summary_cap_step_list_base` / `_per_item` / `_max` | `1000/120/2500` | 步骤列表按条数滑动 |
| `summary_cap_symbols_base` / `_per_item` / `_max` | `1000/100/2500` | 符号列表按条数滑动 |
| `summary_cap_default` | `500` | 未分类 shape 的兜底上限 |

#### 3.3.14 Citation Quote 预览上限(`citation_quote_max_chars`)

emit_answer_document 每条 citation 可以附一行源码 quote。超长 quote **不会丢整条引用**——只截断预览文字到此字符数(UTF-8 边界),file+line 锚点始终保留;prose-smuggling 防御依赖 grounder 的 token 匹配,跟这个长度无关。深仓路径、长 import、多参 `fmt.Errorf`、长 SQL/regex 字面量场景经常超 200 字符,所以默认放宽到 500。

| 键 | 默认值 | 作用 |
|---|---|---|
| `citation_quote_max_chars` | `500` | citation quote 预览字符上限;非正数忽略。调大适用于 Kotlin DSL / Scala implicits / 生成代码等长行常见的仓库 |

#### 3.3.15 闲聊命令(`chitchat_enabled`)

REPL 识别 `/chat <message>` 斜杠命令:不走 analyze→explore→extract→finalize 流水线,单次 LLM Chat 调用直接回复用户。适用于打招呼、问工具能力、不需要读仓库的对话。单次 `--request` 模式不受此开关影响——`/chat` 是 REPL-only 特性。

providers.yaml 可选按便宜模型单独路由:

```yaml
llm:
  agents:
    chitchat_responder:
      model: gpt-4o-mini
      think_aloud: false
```

缺省配置时回落到 `llm.default`。

| 键 | 默认值 | 作用 |
|---|---|---|
| `chitchat_enabled` | `true` | `/chat` 命令开关。`false` 时 `/chat` 打印"未配置"警告,不发 LLM 调用 |
| `chitchat_classifier_enabled` | `true` | 自动分类器开关。默认开:每轮 REPL dispatch 前跑一次廉价 LLM 分类,判为 chitchat 自动走 `/chat` 路径;`repo_question` 或分类错误回落到流水线(fail-safe)。有附加日志时自动跳过。用 `providers.yaml` 把 `chitchat_classifier` 路由到小模型可控成本;设 `false` 可完全关闭 |
| `--chitchat-classifier` (CLI flag) | — | 本次 run 覆盖 `chitchat_classifier_enabled`(三层优先级:code 默认 `true` → yaml → flag)。`--chitchat-classifier` 或 `=true` 强开,`=false` 强关,不传则 yaml 生效。适合调试误判时单次开关 |

providers.yaml 分类器路由示例:

```yaml
llm:
  agents:
    chitchat_classifier:
      model: gpt-4o-mini
      think_aloud: false
```

记忆压缩 (memory compaction) 同样走一次独立的 LLM 调用(结构化 tool 输出 + 失败即回落启发式 IndexEntry);操作者可以单独路由到便宜模型:

```yaml
llm:
  agents:
    memory_summarizer:
      model: gpt-4o-mini
      think_aloud: false
```

没有 codrax.yaml 开关——加了 `memory_summarizer` 条目就生效,不加就复用 `llm.default`。

#### 3.3.16 日志分诊(`log_triage_*`)

仅当使用 `--log` / `/log` 附加日志时生效。

| 键 | 默认值 | 作用 |
|---|---|---|
| `log_triage_enabled` | `true` | 主开关。关闭后附加的日志只保存不分诊 |
| `log_triage_source_prefix` | `""` | CI 构建机绝对路径前缀(C/C++ 场景有用);CLI `--log-source-prefix` 覆盖 |
| `log_triage_min_bytes` | `50` | 小于此字节数的日志直接跳过 |
| `log_triage_two_step_enabled` | `true` | 大日志 / 低覆盖时是否启用两步抽取 |
| `log_triage_two_step_bytes` | `32768` | 日志超此大小直接走两步模式 |
| `log_triage_two_step_coverage` | `0.3` | 单次抽取覆盖低于此值升级两步 |
| `log_triage_max_llm_calls` | `8` | 单次 log_triage 阶段 LLM 调用次数硬上限 |
| `log_triage_max_retries` | `1` | emit_log_triage schema 拒后的重试次数 |

**接入上限（`log_attach_*` / `trace_attach_max_bytes`,在 log_triage / perf_triage **之前**生效）**

| 键 | 默认值 | 作用 |
|---|---|---|
| `log_attach_max_bytes` | `52428800`(50 MiB) | 日志通道字节上限,管 `--log <file>`(可重复) / `--log -` / `--log-text` / REPL `/log <path>` / `/log append <path>` / `/log` 粘贴 / 自动识别。超限尾部截断并打 `WARN [cmd] attached log truncated`;stdin 用 `io.LimitReader(N+1)`。`log_triage_enabled: false` 下也生效(管的是内存)。非正值视为默认,硬顶 1 GiB |
| `trace_attach_max_bytes` | `0` → 继承 `log_attach_max_bytes` | 性能 trace 通道独立上限,管 `--htrace`(可重复) / `--atrace` / `--htrace-text` / `--atrace-text` / REPL `/htrace` + `/atrace` + `append`。0 或未设时与日志同上限;显式设非零值可独立调整(例如 trace 200 MB / log 50 MB)。同 1 GiB 硬顶 |

> 多文件附加：`--log a.log --log b.log`(或 REPL `/log append <path>`) 把多份独立日志拼成一个 attachment,文件之间自动加 `# codrax-source: <path>` 边界头方便 LLM 区分独立 capture。性能 trace 同样支持 `--htrace foo --htrace bar`。stdin (`-`) 跨 `--log` / `--htrace` / `--atrace` 整体只允许一次(同一进程不能消费两遍 stdin)。
>
> ⚠️ **超大日志/trace 怎么办**:调高 cap 只解决"能喂进来"这一步;LLM 侧仍受 `log_triage_max_llm_calls` / `perf_triage_max_llm_calls` 约束。建议先用 `grep -A50 -B5 'panic\|Exception\|FATAL'` 预过滤,或把上限调到 16-20。分页读取**不是**靠 LLM `read_file offset/limit`,而是系统按字节窗口切片,每轮 LLM 只看一个窗口(见 `§4.5 日志分诊 / 性能分诊` 两步法)。

#### 3.3.17 REPL 交互

| 键 | 默认值 | 作用 |
|---|---|---|
| `repl_paste_fold_min_chars` | `100` | 单行粘贴超过此字符数才折叠成 `[Pasted text #N]` 占位 |

#### 3.3.18 providers.yaml 路径

| 键 | 默认值 | 作用 |
|---|---|---|
| `providers_config` | `providers.yaml` | 相对路径相对于 `<exeDir>`;绝对路径直接使用 |

#### 3.3.19 写模式(`write_enabled` + `pipeline_*`)

完整工作流程见 [4.3 写模式](#43-写模式--plan--apply--verify)。相关键:

| 键 | 默认值 | 作用 |
|---|---|---|
| `write_enabled` | `false` | 顶层开关。默认关;为 `true` 才允许 `--mode=plan / apply / verify` 或 REPL `/mode plan / apply / verify` 触发。不开启的情况下即使显式传 `--mode=plan` 也会 fail-loud 拒绝(避免误开) |
| `pipeline_write_retry_budget` | `3` | 测试失败后允许重新规划的次数上限。`0` 表示验证一失败就报告退出;`>0` 启用自动重试 —— 系统会把失败信息整理成精炼的诊断喂给规划阶段,生成新 plan 再次尝试。硬上限 5 |
| `pipeline_baseline_capture_enabled` | `false` | 修改前先跑一次测试作为基线,后续可把失败归类为新引入 / 已存在 / 已修复。测试时间翻倍,仓库已有 pre-existing 失败时打开 |
| `pipeline_keep_worktree_on_success` | `false` | 修改并测试通过后**不销毁** worktree,路径暴露给用户 review 或 cherry-pick。失败路径无条件销毁 |
| `pipeline_lint_enabled` | `true` | 静态检查总开关。新建文件在被采用前会先过一遍当语言的检查工具(Go / C / C++ / Python / JS / TS / Ruby / Rust / Java / Swift / ArkTS / Cangjie 共 12 种,系统按文件后缀和项目结构自动选)。规则只对新建文件强制,避免改动既有文件时引入风格波动。仓库本身有检查债务时可设 `false` 关闭 |

写模式用到的 agent(`planner` / `coder` / `verifier`)和分类器 / 闲聊 / 记忆摘要等辅助 agent 一样,都可以在 `providers.yaml :: agents.*` 下单独路由:

```yaml
llm:
  agents:
    planner:
      model: your-strong-model   # plan 阶段要想清楚,多花点钱值得
    coder:
      model: your-cheap-model    # 照着 plan 机械执行,便宜就够
    verifier:
      model: your-cheap-model    # 可选给 FailureSummary 写个 narrative
```

不写就全继承 `default`。

### 3.4 配置文件查找顺序

**`providers.yaml`**:只看 `codrax.yaml` 里的 `providers_config` 字段(默认 `<exeDir>/providers.yaml`)。通过 `CODRAX_SETTINGS=/path/to/codrax.yaml` 可以一键切换整套环境。

**`codrax.yaml`** 按下列顺序查找,找到第一个就停:

1. 环境变量 `CODRAX_SETTINGS`(绝对路径)
2. `<exeDir>/codrax.yaml` ← 推荐位置
3. `<exeDir>/codrax/codrax.yaml`(bin + share 分离布局)
4. 以下为历史路径,找到会打 deprecation warning:
   - `<exeDir>/config/codrax.yaml`
   - `<exeDir>/../config/codrax.yaml`
   - `<CWD>/config/codrax.yaml`

**`.codrax/` 运行产物目录**永远在 `<CWD>`(当前工作目录)下。因此同一个 codrax 二进制在不同仓库启动,日志和对话记忆会**自动隔离**到各自仓库的 `.codrax/` 里,互不污染。

---

## 4. 使用模式

codrax 有**读模式**(默认,分析代码 / 不动源文件)和**写模式**(opt-in,在沙箱 worktree 里提议 + 执行 + 验证代码变更)两套。读模式下还分 **REPL 交互**和**单次命令**两种触发方式。

### 4.1 REPL 交互模式(读模式)

**何时用**:想就一个话题反复追问,或者先让 codrax 给个大致答案再深挖细节。

**怎么用**(不传 `--request` 就进 REPL):

```bash
cd /path/to/your/repo
codrax
```

启动后输入问题,回车即分析。随后的 `❯❯` 继续提问,codrax **自动把前几轮对话作为上下文**一起喂给 LLM。典型一次完整会话像这样:

```
   CODRAX  v0.1.20260424  /help · /exit

❯❯ explorer 的 ShouldStop 是怎么决定的?

   ┌── analyze (analyzer) ⠋ thinking                       0.8s ──┐
   ... (流水线进行中的实时任务列表)
   └───────────────────────────────────────────────────────────────┘

  │ <最终答案文本,带文件:行号 citation>
  │

❯❯ 如果 ERM 要求未满足,会怎样?
   ... (会带上一轮问题作为上下文)

❯❯ /exit
  Goodbye!
```

**REPL 里的多行输入**:一行末尾加 `\`(反斜杠)可以续行,下一行出现 `…` 提示符,直到某一行不以 `\` 结尾才提交:

```
❯❯ 帮我对比 explorer.go 里 \
…  ShouldStop 和 PerformLookahead \
…  两个函数的逻辑差异
```

**粘贴多行内容**:多数终端支持 bracketed paste,贴一段 ≥ 100 字符(默认阈值)的文本会自动折叠成 `[Pasted text #0 +N lines +C chars]` 占位符,提交后完整内容依然会送给 LLM。SSH / 老版 tmux 场景 bracketed paste 会被吃掉,这时用 `/paste` 子命令兜底(详见 [5.2](#52-repl-斜杠命令))。

### 4.2 单次命令模式(读模式)

**何时用**:脚本批处理、CI 流水线、快速验证一个问题。

**怎么用**(传 `--request` / `-r`):

```bash
# 完整形式
codrax --request "explorer 的 ShouldStop 是怎么决定的?"

# 简写形式
codrax -r "explorer 的 ShouldStop 是怎么决定的?"

# 指定仓库与分支
codrax --repo /path/to/repo --branch develop -r "你的问题"
```

**输出特点**:
- 流水线执行期间任务列表实时刷新到 stderr,最终 **markdown 格式**的答案打到 stdout。
- 适合重定向:`codrax -r "..." > answer.md`。
- 单次运行完成后进程立即退出,不会进入交互。

**诊断模式**:遇到问题想看细节时加两个参数:

```bash
codrax --log-level debug --log-stdout -r "你的问题"
```

- `--log-level debug` — 打开 debug 级别日志(默认 `debug` 但写到文件里)
- `--log-stdout` — 把日志同步打到 stdout,便于一眼看到调度决策

### 4.3 写模式 — plan → apply → verify

写模式让 codrax **提议并执行**代码变更,但所有写动作都发生在一个 `git worktree add` 出来的**独立沙箱**里。主仓库 HEAD 字节永不自动变;你看到结果后决定要不要 `git cherry-pick` 合进 main。

**核心保证(写之前先记牢)**:

- 主仓 HEAD 字节永不被自动写入。任何修改都在沙箱 worktree 里,主仓的工作树不会被污染。
- 任何写都必须显式触发(REPL `/approve`,或 CLI `--mode=apply --auto-apply`)。仅生成 plan、查看 plan 不会动一个字节。
- 失败的 worktree 会被自动销毁,不会留下半成品垃圾;成功的 worktree 默认也销毁,除非你显式打开 `pipeline_keep_worktree_on_success`。
- LLM 想改的每一个文件都必须先在 plan 的 `TargetPaths` 列表里声明过(写闭包保护),想越界改文件会被拒绝。

#### 4.3.1 启用

在 `codrax.yaml` 显式打开,这是一次性操作:

```yaml
write_enabled: true
```

没这一行所有写模式入口都会 fail-loud 拒绝。

可选调优(下面四项默认值已经能用,需要时再开):

```yaml
pipeline_keep_worktree_on_success: true   # apply+verify 都过的时候,把 worktree 留下来给你 review / cherry-pick / /merge
pipeline_write_retry_budget: 3            # 测试失败后允许自动 re-plan 几次(0 关闭,硬上限 5)
pipeline_baseline_capture_enabled: false  # 改之前先跑一遍测试做基线对比(测试时间翻倍)
write_auto_init_repo: false               # 目标是裸目录(无 .git 或无 commit)时是否预先授权 codrax 自动 git init + 空 commit
```

`write_auto_init_repo` 默认关。详见 [4.3.15 裸目录自动 init](#4315-裸目录自动-init三档授权)。

#### 4.3.2 四种 Mode 一张图

| Mode | 做什么 | 改文件? | 跑测试? |
|---|---|---|---|
| `read` | 默认。普通问答,带 citation | 否 | 否 |
| `plan` | 读代码 → 产出 ChangePlan JSON 落盘 | 否 | 否 |
| `apply` | 加载 ChangePlan → 在 worktree 里逐文件 `apply_patch` → 自动接 verify | **是(在 worktree 里)** | **是** |
| `verify` | 对已 applied 且保留的 worktree 重跑测试 | 否 | **是** |

`apply` 一定自动接 `verify`(代码先落地,然后跑测试,出错会触发 re-plan)。所以日常你只需要 `plan` 和 `apply` 两个;`verify` 是给"我想再确认一下测试结果"或"flakey 测试想再跑一次"准备的。

#### 4.3.3 文件变更的四种 kind

planner 给你的 ChangePlan 里每条变更都有一个 `kind` 字段,语义如下。理解它们对你 review plan 至关重要:

| kind | 语义 | 失败条件 |
|---|---|---|
| `create` | 新建文件,`new_content` 是完整 body | 目标路径**必须不存在**,否则 apply 拒绝(避免误覆盖) |
| `modify` | 覆盖整个文件,`new_content` 是新的完整 body | 目标路径**必须存在**,否则 apply 拒绝 |
| `delete` | 删除文件 | 缺失文件视为幂等成功 |
| `patch` | 把 `patch` 字段(unified diff)喂给 `git apply -` | 上下文行号严格匹配;不匹配 → 拒绝 → re-plan |

planner 默认倾向 `patch`(改动小、上下文友好);需要整体重写时选 `modify`;新建文件用 `create`。

#### 4.3.4 端到端示例 A:在 REPL 里改一个小 bug(推荐路径)

适合大多数手工开发场景。这条路径让你在 apply 前看到完整 diff,confirm 后再写。

```bash
cd ~/code/myproject
codrax
```

```
❯❯ /mode plan
  mode → plan (sticky)

❯❯ internal/server/handler.go 里 ParseDuration 处理负值时会 panic,改成
…  返回 ErrInvalidDuration,同时给该函数补一条单元测试覆盖负值场景

   ... (planner 跑约 30-90 秒) ...

  plan saved: /home/me/code/myproject/.codrax/plans/plan-1730834521... json (2 changes)

❯❯ /plan show
  current plan: .../plan-1730834521-12345.json
    id:      plan-1730834521-12345
    status:  pending_approval
    changes: 2 file(s)
    targets:
      - internal/server/handler.go
      - internal/server/handler_test.go
    summary: 在 ParseDuration 里把负值校验从 panic 改为返回
             ErrInvalidDuration,并新增 TestParseDuration_Negative
             覆盖该路径。

  diff preview:

  ─── internal/server/handler.go (kind=patch) ───
  rationale: 用错误返回替代 panic,符合包的错误处理约定
  --- a/internal/server/handler.go
  +++ b/internal/server/handler.go
  @@ -42,8 +42,8 @@
   func ParseDuration(s string) (time.Duration, error) {
     d, err := time.ParseDuration(s)
     if err != nil { return 0, err }
  -  if d < 0 { panic("negative duration not supported") }
  +  if d < 0 { return 0, ErrInvalidDuration }
     return d, nil
   }

  ─── internal/server/handler_test.go (kind=patch) ───
  rationale: 锁定负值返回 ErrInvalidDuration 的契约
  --- a/internal/server/handler_test.go
  +++ b/internal/server/handler_test.go
  @@ -88,3 +88,11 @@ func TestParseDuration_Valid(t *testing.T) {
     ...
   }
  +
  +func TestParseDuration_Negative(t *testing.T) {
  +  _, err := ParseDuration("-3s")
  +  if !errors.Is(err, ErrInvalidDuration) {
  +    t.Fatalf("got %v, want ErrInvalidDuration", err)
  +  }
  +}

❯❯ /approve
  Approve plan plan-1730834521-12345 (2 change(s))?
  Apply inside a git worktree + run verify.
  > Yes

   ... (apply ~5s, verify ~30s) ...

  │ ## 变更应用成功
  │ - 2 个文件已写入 worktree
  │ - go test ./internal/server/ — 23 passed, 0 failed
  │ - 没有 regression

  worktree preserved: /home/me/code/myproject/.codrax/worktrees/<trace>-<pid>

❯❯ /exit
```

**合并回主仓**(推荐 — 最少操作):

```
❯❯ /merge
  Fast-forward 1 commit(s) onto main repo branch main?
  > Yes

  ✓ Fast-forwarded 1 commit(s) onto main.
  Next: git push (optional).
```

`/merge` 默认走 fast-forward 把 worktree 上的 commit 推到当前分支(`main`)。它会:

1. 自动找到最近一个 `applied` plan 的保留 worktree
2. 检查主仓工作区干净(有未提交改动会拒绝,避免误覆盖)
3. fast-forward 可行 → 直接合;不可行 → 回退到下一节的"新分支"路径(显式提示)
4. 成功后**自动销毁 worktree**(用过即销毁,磁盘干净)

**走 PR 工作流**(推荐用于团队协作):

```
❯❯ /merge --branch=fix/parse-duration
  Create branch fix/parse-duration on main repo and cherry-pick 1 commit(s) onto it?
  > Yes

  ✓ Branch fix/parse-duration created on main repo with 1 cherry-picked commit(s).
  Next: cd <main repo> && git push -u origin fix/parse-duration, then open a PR.
```

主仓 HEAD **不会动**,只是新拉了一个分支落了 commit。再 `git push` 即可开 PR。

**一步到位**(知道目标分支时,跳过中间审查):

```
❯❯ /approve --merge-to=fix/parse-duration
  ... (apply + verify pass + auto-merge) ...
  ✓ Branch fix/parse-duration created on main repo with 1 cherry-picked commit(s).
  Next: cd <main repo> && git push -u origin fix/parse-duration, then open a PR.
```

`--merge-to=<branch>` 让 `/approve` 在测试通过后立刻调用 `/merge`,等价"approve + 合并 + 清理 worktree"三步合一。

**冲突时的安全网**:

`/merge` 在 cherry-pick 遇到冲突时**不会**让你陷入"主仓里有 conflict marker、不知道怎么继续"的状态,而是:

1. `git cherry-pick --abort` 回滚
2. 删掉刚刚创建的目标分支
3. 主仓 HEAD 回到合并前的位置
4. 打印 git 的诊断输出,告诉你哪一行哪个文件冲突
5. worktree 不动 — 你可以 `cd` 进去查看,或者 `/worktree show`、`/reject` 后重新 plan

**纯手工路径**(任何时候都可用):

```bash
cd ~/code/myproject/.codrax/worktrees/<trace>-<pid>
git log --oneline main..HEAD     # 看 apply 产生的 commit
git diff main                    # 看完整 diff
cd ~/code/myproject
git cherry-pick <sha>

codrax
❯❯ /worktree discard plan-1730834521-12345     # 用完后清理
```

直接在文件系统层 `rm -rf .codrax/worktrees/<trace>-<pid>` 也行,但 REPL 里清会同时把 plan 的 `worktree_path` 字段清空,`/worktree list` 不会再显示它。

#### 4.3.5 端到端示例 B:不喜欢 plan 想换一个

```
❯❯ /reject 这版改动太大,我只想改 handler 不想动测试
  plan plan-1730834521-12345 rejected (status → rejected,reason 已记入 memory)

❯❯ 同样的问题,但**只改 handler.go**,不动测试文件
   ... (planner 重新生成,这次 TargetPaths 只有 handler.go) ...

  plan saved: .../plan-1730834612-12345.json (1 change)

❯❯ /plan show
   ...
❯❯ /approve
   ...
```

`/reject [reason]` 会把 plan 状态置为 `rejected`,理由写进 memory(让后续 plan 知道你为什么否了上一版)。如果不想留任何痕迹,用 `/plan clear` 直接丢弃。

#### 4.3.6 端到端示例 C:CI / 自动化批处理

适合"这是个一揽子小重构,我已经在另一份文档里描述过了,直接跑给我看结果"。

```bash
# 第一步:只生成 plan,不 apply。让人/脚本审查
codrax --mode=plan \
  --request "把 internal/util/httpx 里所有 http.Client 替换成全局 sharedClient" \
  --plan-out /tmp/refactor.plan.json

# 看一下 plan(标准 JSON)
jq -r '.summary, (.changes[] | "\(.kind) \(.path): \(.rationale)")' /tmp/refactor.plan.json

# 第二步:在另一个 step 里 apply + verify(--auto-apply 必填,绕过交互确认)
codrax --mode=apply --plan-file /tmp/refactor.plan.json --auto-apply

# 第三步:只想再跑一遍测试(例如 flakey 测试)— 需要 keep-on-success 已开
codrax --mode=verify --plan-file /tmp/refactor.plan.json
```

CI 集成提示:

- `--auto-apply` 是 `--mode=apply` 的**必填配套**,缺它会 fail-loud(防止 CI 误触)。
- 退出码:Run 整体成功 = 0;apply 拒绝(W1/W1b 违规、git apply 不匹配)= 非零;verify 测试有失败(且 retry 用尽)= 非零。
- ChangeReport 落在 `<plan-out 同目录>/<plan-id>.report.json`,可以用 `jq` 提取关键字段做断言。

#### 4.3.7 端到端示例 D:多个 worktree 并存的清理

如果你开了 `pipeline_keep_worktree_on_success: true`,跑了几次 plan 之后磁盘上会有多个保留的 worktree。

```
❯❯ /worktree list
  3 worktree(s) preserved:
    plan-1730834521-12345   /home/me/code/myproject/.codrax/worktrees/abc-12345
        applied 2 hour(s) ago — fix ParseDuration negative panic
    plan-1730841098-23456   /home/me/code/myproject/.codrax/worktrees/def-23456
        applied 30 minute(s) ago — replace http.Client with sharedClient
    plan-1730845210-34567   /home/me/code/myproject/.codrax/worktrees/ghi-34567
        applied 5 minute(s) ago — add retry to outbound HTTP

❯❯ /worktree discard plan-1730834521-12345
  worktree removed: /home/me/code/myproject/.codrax/worktrees/abc-12345
  plan plan-1730834521-12345 status retained as `applied` (history preserved)
```

`/worktree discard` 只删 worktree 文件,不动 plan 状态(`applied` 仍然显示在 `/plan list` 历史里);但 plan 的 `worktree_path` 字段会被清空,`/verify` 那条不再可用。

#### 4.3.8 测试失败后的自动 re-plan

`pipeline_write_retry_budget` 大于 0 时(默认 0,推荐设 2-3),verify 阶段测试如果挂了,系统会:

1. 提取最有信息量的失败信号:`FailureSummary`(verifier 写的 narrative)+ 前 3 条失败测试的关键错误行 + 上版 plan 的 `TargetPaths`(嫌疑文件)。
2. 把这些信号当作 PlanningHint 喂给 planner,让它**带着失败上下文**重写一份新 plan。
3. 新 plan 自动 `apply + verify` 再来一次。
4. 直到测试通过,或耗尽 retry budget。

REPL 里看到的样子:

```
   ... 第 1 次 verify:3 个 test 失败 ...
   ... 系统组装 retry hint:suspect=[handler.go,handler_test.go],top failure: ParseDuration("-3s") returned wrong error type ...
   ... planner 重新规划(retry 1/3)... 改成在 handler.go 用 errors.Is 匹配而不是 ==
   ... 第 2 次 apply + verify ... 全部通过 ✓
```

把它设为 `0` 切回严格模式 — verify 一挂就退出报告,适合你想保留确定性、不愿额外 LLM 成本的场景。

#### 4.3.9 关于 baseline(`pipeline_baseline_capture_enabled`)

打开后,apply 之前会先在 worktree(此时还是干净的 main 副本)上跑一遍测试当作 baseline。verifier 后续比对 baseline 和 apply 后的结果,把失败分成三类:

- **REGRESSION**:baseline 过 / 现在挂 — 这次 plan 引入的退化
- **PRE-EXISTING**:baseline 挂 / 现在还挂 — 与本次无关
- **FIXED**:baseline 挂 / 现在过 — 顺手修了

适合:仓库本身有已知的 flakey / pre-existing 失败,担心 verifier 把它们误判为 regression。代价是测试墙钟时间翻倍。

#### 4.3.10 写闭包保护(W1 / W1b)

`apply_patch` 是一个工具层强约束,任何 LLM 都越不过去:

- **W1**:`path` 必须出现在 `ChangePlan.TargetPaths` 声明的写作用域内。planner 在 plan 阶段就声明了"我打算改这些路径",apply 阶段不允许临时扩张写域。如果 coder 想写 `TargetPaths` 之外的路径,会被工具拒绝并把全部合法路径回 LLM,coder 自修正。
- **W1b**:每个 `DependsOn` 引用的前置变更必须已经 applied(已写入 `WriteClosure.AppliedSet`)。这保证了 plan 里"先改 A 再改 B"的拓扑序在执行层不能被打乱。
- **幂等**:同一个路径再 apply 一次,直接成功返回(不重复写 IO)。

W1/W1b 不是用 prompt 提醒 LLM 遵守的,是工具入口处直接验证的硬约束。

#### 4.3.11 12 种测试 runner 自动探测

verify 阶段先看 `run_tests` 的 `runner` 参数 — verifier agent 自己会在 worktree 里 list_files / read_file 一通,然后告诉 `run_tests` 用哪个 runner。这是首选。

参数留空时,系统按下表顺序在仓根扫 manifest 自动选(HarmonyOS / Cangjie 优先级排在通用语言之前,确保混合工程优先走 hvigor / cjpm):

| 探测文件 | Runner | 命令 |
|---|---|---|
| `oh-package.json5` / `build-profile.json5` / `hvigorfile.ts` | hvigor (HarmonyOS ArkTS) | `hvigorw --no-daemon test`,JUnit XML(复用 Java 解析器) |
| `cjpm.toml` | cjpm (HarmonyOS Cangjie) | `cjpm test`,Cargo 风格文本输出 |
| `go.mod` | go | `go test -json ./...` |
| `package.json` | node | `npm test -- --json --silent`(兼容 jest / vitest) |
| `pyproject.toml` / `pytest.ini` / `setup.py` | python | `pytest --json-report` (需 `pytest-json-report` 插件) |
| `Cargo.toml` | rust | `cargo test` |
| `Package.swift` | swift | `swift test` |
| `pom.xml` | java (Maven) | `mvn -B -q test`,读 `target/surefire-reports/*.xml` |
| `build.gradle` / `build.gradle.kts` | java (Gradle,含 Kotlin/Android) | `./gradlew --no-daemon --console=plain test`,读 `build/test-results/test/*.xml` |
| `Gemfile` | ruby | `bundle exec rspec --format json` |
| `CMakeLists.txt` | cmake | `ctest --output-junit`(需要 build 目录已配置) |
| `meson.build` | meson | `meson test --xunit-file` |
| `Makefile` / `makefile` / `GNUmakefile` | make | 优先 `make check` 再 `make test`,exit code 判定 |

多 manifest 并存时按上表顺序取**先找到的**(例如 CMake 生成的 Makefile 会被 CMakeLists.txt 压过)。构建失败(测试还没开始跑)会自动合成一条失败 `TestResult`,`FailureDetail` 里带从 stdout / stderr 提取的错误行,把真正有用的信息喂给 retry 循环。

不在上表的语言 / 构建系统,verify 会明确报错 "no supported test runner detected" 并列出查找过的所有文件名 —— 你可以补一份符合约定的 manifest,或者让 verifier agent 在 prompt 里指定 `runner=<其它>` 的兜底命令。

#### 4.3.12 "零测试发现" 不再是失败

历史上 pytest exit code 5("no tests collected")、jest "no tests found"、`go test ./...` 在没有 `_test.go` 的项目下退出 0 但没有 case 跑过 —— 这些情况会被旧版 verifier 误判为测试失败,然后触发一次"我得加测试文件"的错误重新规划,典型反例是 planner 凭空捏造一个 `test_*.py` 来"修"一个本就不存在的测试套件。

新版本把"零测试发现"作为一等信号 `NoTestsRunners` 单独记录,**Passed=true** 但同时把"verification ran but had no tests to execute"的事实暴露给下游;verifier 会据此走静态语法检查兜底,而不是去 fabricate 测试。

#### 4.3.13 测试 runner 缺失不会无脑重试

当 verify 阶段发现 runner 二进制本身不在 PATH 上(`pytest: command not found`、`go: command not found`、Windows `'pytest' is not recognized as an internal or external command`),codrax 会:

1. 把这次失败标记为 `FailureKind=runner_missing`(独立于 `tests_failed` / `build_failure`)
2. **跳过 verify→plan 重试循环** —— 重新规划解决不了"工具没装"
3. 给出清晰的安装提示:`runner "python"'s primary binary "pytest" is not installed in this environment — install pytest with pip install pytest pytest-json-report. Re-run verify after installing the tool; the planner cannot fix a missing dependency.`

适用所有 12 个 runner;每个 runner 的 install hint 都本地化在错误消息里(Go / Python / Rust / Node / Java / Ruby / Swift / CMake / Meson / Make / hvigor / cjpm)。

检测信号(任一即触发):
- shell 退出码 127(POSIX "command not found" 约定)
- `errors.Is(runErr, exec.ErrNotFound)`(直调 exec 时)
- stdout/stderr 含 `<binary>: not found` / `<binary>: command not found` / `executable file not found` / Windows `is not recognized as an internal or external command`

误报防御:`<binary>` 名称必须出现在 stderr 文本里才算数 —— 测试断言里出现 `'foo' not found` 这种字符串不会触发。

#### 4.3.14 写模式跳过读模式的质量门

analyzer 的 quality gate 有两条**只对读模式有意义**的检查:

- `hypothesis_coverage` —— 至少存在一条优先级 ≥ 30 的假设。读模式靠这个保证 explorer 的调查方向不空。
- `contract_complete` —— `AnswerContract` 的字段填齐了。读模式靠这个保证 finalizer 有完整的输出契约。

写模式(`plan` / `apply` / `verify`)既不消费 `HypothesisSet` 也不用 `AnswerContract`(写流水线走 `CritPlanReady` / `CritPatchApplies` / `CritTestsPass` / `CritNoRegression` 套件),所以 analyzer 在写模式下**只跑结构性检查**(`coverage` / `dag_closure` / `budget_sanity` / `criterion_resolvable` / `pending_fields_wellformed`),跳过这两条。

否则"用 python 写一个猜数字游戏"这种从零起步的请求会被读模式 gate 误拒:仓里没有可调查的实体 → 所有 hypothesis priority 全是 0 → `hypothesis_coverage` fail → analyzer 重试预算烧光在凭空捏造假设上,planner 永远等不到 RequestModel。

#### 4.3.15 verify_failed plan 可重新 /approve(环境修复重试)

verify 阶段失败的 plan 状态是 `verify_failed`,默认情况下:

- 未触发 verify→plan 自动重试(`pipeline_write_retry_budget=0` 或失败原因是 `runner_missing` 等环境类问题)
- worktree 已被销毁(失败路径无条件清理)

这种情形下用户可以:

1. 修复环境(装 pytest、启动测试数据库、修复网络等)
2. `/approve` 同一个 plan —— REPL 接受 `verify_failed` 状态,重新跑 apply + verify

`/approve` 拒绝的状态:`applied`(已落地)/ `applied_failed`(W1/W1b 拒,需要重新规划)/ `rejected`(用户主动否决,不能静默撤销)。

#### 4.3.16 Python pytest 自动用 venv

verify 阶段跑 pytest 时,codrax 会按下列顺序解析 Python 解释器:

1. **项目 venv**:`.venv/bin/python` / `.venv\Scripts\python.exe`(优先);其次 `venv/`、`env/`、`.virtualenv/`
2. **系统 PATH**:`python3` 优先(现代 Linux/macOS 标准),`python` 兜底(老系统 / Windows)
3. **裸 pytest**:都没有时回退

调用形式始终是 `<interpreter> -m pytest --json-report ...`,**不**直接调 `pytest`。这做了两件事:

- 自动用上 venv(无需 `source .venv/bin/activate`)
- `python -m pytest` 把 cwd 加进 sys.path,源码-only 仓库无需 `pip install -e .` 也能 import 自己的包(这是 pytest 官方文档推荐的形式)

如果 venv 里没装 pytest,失败信号是 `No module named pytest`;codrax 把它和 `python: command not found` 一并归类为 `FailureKind=runner_missing`,跳过 verify→plan 重试,直接给安装提示(中英双语,跟随 `--lang`)。

**其它语言不需要这种处理**:Go / Rust / Swift 把测试框架打包进语言工具链;Node / Java(Gradle)/ Ruby / hvigor / cjpm 用项目本地 wrapper(`npm test` / `./gradlew` / `bundle exec` / `hvigorw` / cjpm)自带依赖解析。Python 是唯一一个测试框架是**独立可装模块**的语言,所以是唯一需要 venv 感知的 runner。

#### 4.3.17 REPL 控制命令意外喂给 orchestrator 时立即拒

`/approve plan-XXX` / `/verify plan-XXX` / `/merge --branch=...` 这类字面量必须经 REPL slash dispatcher 拦截;如果通过 CLI `--request="/approve ..."` 或别的边缘路径直接喂到 orchestrator,**Run() 入口立即报错**(1 round-trip,不进 analyzer)。这避免了 analyzer 在 12+ 次迭代里反复拒绝自己的 `emit_analysis` 调用、最终被 SIGINT 杀掉的浪费场景。

合法的写模式 dispatch 字串(REPL 内部合成的 `Apply approved plan plan-XXX: ...` / `Verify applied plan plan-XXX: ...`)不受影响 —— 守门只识别 slash 字面量。

#### 4.3.18 进程级安全网

verify 阶段的子进程跑在隔离的进程组里,带内存 / CPU 上限:

- **Linux**:`Setpgid` 创建独立进程组,fork 的子进程可被一并 kill;`prlimit` 给 OOM/CPU 双重墙;失控测试不会拖死主进程。
- **Windows**:JobObject + `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`,等价语义。
- 资源耗尽时 `FailureKind` 会标记成 `oom` / `cpu_limit` / `timeout`(而不是 `tests_failed`),retry-hint 据此分类不会把"OOM 了"当成"测试逻辑挂了"重新规划。

`pipeline_max_steps` 超时时整个 Run 强制收尾,worktree 也会在 outer defer 里清理。

#### 4.3.19 合并到主仓 — `/merge` 命令

`/approve` 成功后,worktree 里有一条或多条 codrax 提交(基于主仓的 `r.branch` 顶点)。把这条 commit 合回主仓有三种通路,任选其一:

| 通路 | 谁动手 | 适用 |
|---|---|---|
| `/approve --merge-to=<branch>` | codrax 一步合并 | 你已经知道目标分支(PR 名),希望"approve 同时合"。最少操作。 |
| `/merge [--branch=<name>]` | codrax 显式合并 | 你想先 review worktree 再决定怎么合;或者 plan 已经 applied 了,后来才决定合。 |
| 纯手工 cherry-pick | 你自己 | 想要完全控制,或者团队规范要求开 PR 不允许直接 fast-forward。 |

**`/merge` 的安全契约**:

- 只在 `pipeline_keep_worktree_on_success: true` 时可用 — 没有保留的 worktree 就没合并素材。`/merge` 会扫 PlanStore 找最近一条 `Status=applied` 且 `WorktreePath` 还指向真实目录的 plan。
- 不传 `--branch=` 时默认目标分支是 `r.branch`(即 `--branch` 启动参数的值,通常 `main`),走 fast-forward。fast-forward 不可行(主仓已经移动)→ 自动回退到下一段说的"新分支"路径,**不会**强制 reset 主仓。
- 传 `--branch=<name>` 强制走 cherry-pick 到新分支。如果分支已存在 → 拒绝(避免误覆盖)。
- **主仓工作区脏不合**(有未提交 / 未 stash 的本地改动)— 会让你先 commit / stash,避免 fast-forward 越过你的本地未提交 work。
- **冲突 = 完整回滚**:cherry-pick 任何一步出错 → `git cherry-pick --abort` + 删掉刚拉的目标分支 + 主仓 HEAD 回到合并前 + worktree 不动。然后告诉你冲突的文件 + 行,你可以 `cd` 进 worktree review、或 `/reject` 重新规划。
- **永不 push**:`git push` 是用户的明确动作,codrax 不替你做。
- **合并成功后自动销毁 worktree**:用过即销毁,`/worktree list` 自动更新。

**fast-forward 模式工作流**:

```
❯❯ /approve
   ... worktree preserved ...
❯❯ /merge
  ✓ Fast-forwarded 1 commit(s) onto main.
  Next: git push (optional).
```

主仓的 `main` 分支移动到 worktree HEAD,worktree 销毁。一步搞定。

**新分支 / PR 工作流**:

```
❯❯ /approve --merge-to=fix/parse-duration
   ... apply + verify pass + auto-merge ...
  ✓ Branch fix/parse-duration created on main repo with 1 cherry-picked commit(s).
  Next: cd <main repo> && git push -u origin fix/parse-duration, then open a PR.
```

主仓 HEAD **不动**,只是新拉了 `fix/parse-duration` 分支落了 commit。再 `git push -u origin fix/parse-duration` + GitHub UI 开 PR。

**强制合入 verify_failed plan**(`--include-failed` / `--force`):

verify 失败 = 测试有问题。但有些场景失败是环境/CI 类原因(本地起不了集成 DB、外部服务挂了、需要 GPU 等),用户 review 完 diff 决定还是要合,扔到 CI 里再跑测试。这时可以:

```
❯❯ /merge --include-failed --branch=fix/parse-duration
  ⚠ 强制合入 plan plan-1730834521-12345 — 该 plan 的 verify 阶段曾失败。
  请先确认 /plan show 的 diff 与失败摘要,确保失败是环境/CI 类原因(非代码缺陷)。
  Create branch fix/parse-duration on main repo and cherry-pick 1 commit(s) onto it?
  > Yes
  ✓ Branch fix/parse-duration created on main repo with 1 cherry-picked commit(s).
```

`--force` 是 `--include-failed` 的别名,功能等价。仅 `verify_failed` 状态可被这个 flag 覆盖;`applied_failed`(W1/W1b 拒,代码没真正落地)和 `rejected`(用户主动否决)永远不能合 —— 前者没东西可合,后者覆盖用户决定。

#### 4.3.20 裸目录自动 init(三档授权)

写模式 apply 阶段需要 `git worktree add --detach HEAD` 跑,这要求目标目录:

1. 是 git repo(`.git` 存在)
2. HEAD 解析得到一条 commit(初始 commit 已经打过)

如果目标是裸目录或 `git init` 后还没 commit,默认 codrax 会 fail-loud 拒绝(避免在用户文件系统上做静默状态变更)。三档授权让"裸目录脚手架"成为合法用例:

| 档 | 触发 | 适用 |
|---|---|---|
| 1 | REPL 交互同意 | 单次手工跑,被弹一次 y/N 后再继续 |
| 2 | `codrax.yaml :: write_auto_init_repo: true` | 部署期统一开,所有 Run 都自动 |
| 3 | CLI `--auto-init-repo` | 单次 Run 显式 |

**REPL 交互档示例**:

```
❯❯ /mode plan
❯❯ 在这个空目录里创建一个 hello-world Go 项目
   ... (planner 跑出一个 create main.go + go.mod 的 plan) ...
   plan saved: ...

❯❯ /approve
  Approve plan plan-... (2 change(s))? Apply inside a git worktree + run verify.
  > Yes

  target /home/me/code/scaffold is not_initialized.
  codrax can run `git init` + an empty initial commit, then apply inside a sandbox worktree.
  Proceed?
  > Yes

  initializing git repo: /home/me/code/scaffold ...
   ... (apply + verify) ...
  ✓ 变更应用成功
```

**预授权档示例**(yaml 或 CLI):

```yaml
# codrax.yaml
write_enabled: true
write_auto_init_repo: true
```

或:

```bash
codrax --mode=apply --plan-file /tmp/plan.json --auto-apply --auto-init-repo
```

预授权后 codrax **不弹同意提示**,直接跑 `git init` + 空 initial commit,再 apply。

**安全规则**:

- 自动 init 只跑两条命令:`git init`(不存在 `.git` 时)+ `git commit --allow-empty -m "codrax: initial commit for plan-<id>"`(HEAD 不解析时)。不会 `add` / `push` / 改 user 已有 git config(只在 `user.email` / `user.name` 完全缺失时设默认值 `codrax@local` / `codrax`,你随时可以 `git config` 覆盖)。
- 已经是 ready 的 repo → 这条路径完全跳过(idempotent)。多次 `/approve` 不会重复造空 commit。
- 失败时(目录权限不足、git 不在 PATH 等)→ 报错,不留半成品。

#### 4.3.21 Plan 状态查询恢复

`/approve` 因为环境问题(目标不是 git repo、git 缺失、worktree 创建失败)失败时,plan **不会丢**:状态保持 `pending_approval`,你可以:

- `/plan show` — 看 plan 详情
- 修复环境(装 git、`/approve --auto-init-repo` 类的方式重试)再 `/approve`
- `/reject [reason]` — 显式弃掉

REPL 会自动从 PlanStore 找最近一条 `pending_approval` plan 重新绑定指针,**跨进程也能恢复**(关掉 codrax 再开,`/plan show` 还能找到上次没合并的 plan)。

#### 4.3.22 多个 pending plan 时定向 approve / show / 跳过 verify

`/plan list` 经常会列出多条候选(每次 `/mode plan` 会新生成一份);默认 `/approve` 操作的是**最新一条** pending/verify_failed plan。当用户想精确控制时:

```
❯❯ /plan list
  3 plan(s) in /home/me/.../plans (newest first):
    - [...] plan-1730845210-12345  status=pending_approval (4096 bytes)
    - [...] plan-1730841098-12345  status=verify_failed     (3210 bytes)
    - [...] plan-1730834521-12345  status=applied           (2400 bytes)

❯❯ /plan show plan-1730841098-12345     # 看历史 plan 的 diff
   ... 渲染 unified-diff 预览 ...

❯❯ /approve plan-1730841098-12345        # 定向 approve(verify_failed env-fix retry)
  • re-approving plan plan-1730841098-12345 (status was verify_failed; assuming env-fix retry)
  • note: 1 other approvable plan(s) exist (pending_approval / verify_failed). about to approve plan-1730841098-12345; target a different one with `/approve <plan-id>` (see /plan list)
  Approve plan plan-1730841098-12345 (3 change(s))? Apply inside a git worktree + run verify.
  > Yes
```

**支持的 /approve 子命令**:

| 形式 | 作用 |
|---|---|
| `/approve` | 最新一条 pending/verify_failed plan |
| `/approve <plan-id>` | 定向到 PlanStore 里的某个具体 plan(必须是 pending_approval 或 verify_failed) |
| `/approve --plan-id=<id>` | 等价上面的长 flag 形式 |
| `/approve --merge-to=<branch>` | apply+verify 通过后立即合并到该分支(可与 plan-id 组合) |
| `/approve --skip-verify` | 只 apply 不跑 verify(本地起不了集成测试时用,扔 CI 跑) |

**多 plan 提示**:当 PlanStore 里还有其它可批准的 plan,confirm 弹出前会自动打一行 `note: N other approvable plan(s) exist...`,提醒用户可能选错了。

**`/plan show <plan-id>`**:配合 /plan list 看历史 plan 的完整 diff(包括已 applied / verify_failed 的);执行后 pendingPlanPath 自动绑定到该 plan,下一句 `/approve` 不再需要重输 ID。

**`--skip-verify` 适用场景**:

- 本地起不了集成测试(数据库、外部服务、GPU 等)
- 改动小且明显正确(typo 修正、注释更新等)
- 想快速迭代,稍后让 CI 跑测试

注意:`--skip-verify` 不影响 plan 状态机 —— apply 成功后 plan 标 `applied`,跟 verify 通过的 plan 一样。如果担心引入回归,后续可以用 `/verify <plan-id>` 单独补跑测试。

#### 4.3.23 当前 git 分支感知 + `/branch` 切换 + `!shell`

**REPL prompt 显示当前 git 分支**:启动 banner 和每行 prompt 的 sticky tag 都会带 `[git:<branch>]` 标记,跨进程实时反映 — 用户在另一个终端 `git checkout` 后,下一次 prompt 自动显示新分支。Detached HEAD 显示 `[git:detached@<sha>]`;不在 git repo 显示空(无 git 标记)。

**`/branch` 命令**:

```
❯❯ /branch                    # 看当前分支
  current branch: main

❯❯ /branch develop            # 切换到 develop
  Switched to branch 'develop'
  ✓ now on branch: develop

❯❯ /branch -b fix/foo         # 创建并切换
  Switched to a new branch 'fix/foo'
  ✓ now on branch: fix/foo

❯❯ /branch -b fix/bar origin/main   # 从特定起点创建
```

`/branch <参数>` 把所有参数透传给 `git checkout`,所以 `-b new-name`、`-b new-name origin/main`、`<sha>` (detach) 等所有 git checkout 形式都支持。git 自己的输出(分支跟踪、divergence 警告)直接显示给用户。

**`/approve --merge-to=` 和 `/merge` 默认目标 = 当前 git 分支**:

之前默认是 `--branch` 启动 flag 的值(粘滞,不跟随)。现在默认跟随**实时 git HEAD**,fallback 到 `--branch` 仅在以下情况发生:
- detached HEAD(没有有意义的分支名)
- 不是 git repo 或 git 缺失

例子:用户启动时 `--branch=main`,过程中 `/branch feature-x` 切到 feature-x,`/approve` 和 `/merge` 自动以 feature-x 为基/目标(不再用 main)。

**`!<command>` 系统 shell 直通**:

```
❯❯ !ls
   ... 当前目录文件列表(repoRoot 为 cwd) ...

❯❯ !cat README.md | head -20
   ... 文件内容 ...

❯❯ !grep -rn "FailureKindRunnerMissing" internal/types/
   ... grep 输出 ...

❯❯ !cd ..
  ⚠ `cd` inside `!` doesn't persist — every `!` invocation spawns a fresh shell.
   Restart codrax with --repo /new/path, or chain in one command:
   `!cd /tmp && cat foo.txt`.
```

`!<cmd>` 的整个行被原样喂给系统 shell(Linux/macOS 用 `sh -c`,Windows 用 git-bash 或 cmd),工作目录是 `r.repoRoot`。stdout/stderr 直接显示给用户。退出码非 0 会用 `! exit ...` 提示。

`!cd` 特殊提醒:每次 `!` 是新 shell,bare `!cd ..` 不会改 codrax 的工作目录;要持久切换目录用 `--repo` 重启。但链式 `!cd /tmp && cat foo` 是有效的(在同一 shell 进程里完整执行)。

#### 4.3.24 当前限制

- 写模式**不支持** multi-plan concurrency。同一仓库不要并行跑两个 `/approve`(plan 文件名带 PID,但 worktree 操作不是并发安全的)。
- `git push` 永远是用户手动操作,`/merge` 不替你做(避免对远端的意外副作用)。
- `git apply -` 失败时 git 的 stderr 原样透传给 coder 自修正,但**不会**自动回退已 applied 的同 plan 其它单元(符合 git 默认行为);多文件依赖的 plan 用 `DependsOn` 明确排序,让失败单元及其下游整块被 retry 是最稳的方式。
- `--mode=verify --plan-file <path>` 只能在 worktree 仍然存在时跑(即 plan 是开了 `pipeline_keep_worktree_on_success` 后 applied 的)。否则会报 "no worktree associated with plan, re-apply first"。
- `/merge` 是单 plan 单 worktree 的合并;一次只处理"最近一条 applied 且 worktree 还在的 plan"。同时合并多个 plan 仍需多次 `/merge`。

---

## 5. 命令速查

### 5.1 CLI 命令行参数

所有参数对 REPL 和单次模式通用。

| 参数 | 简写 | 默认 | 作用 |
|---|---|---|---|
| `--request` | `-r` | (空) | 单次提问;传此参数即单次模式,不传即 REPL |
| `--repo` | | `.` | 目标仓库路径 |
| `--branch` | | `main` | 分支名 |
| `--lang` | | `zh` | 答案语言,`off` / `none` 关闭 |
| `--providers` | | (代码默认路径) | `providers.yaml` 显式路径 |
| `--log-dir` | | (代码默认) | 日志目录 |
| `--log-level` | | `debug` | `error` / `warning` / `info` / `debug` |
| `--log-stdout` | | `false` | 日志镜像到 stdout |
| `--memory-dir` | | (代码默认) | REPL 对话记忆目录 |
| `--cache-dir` | | 平台默认 | repo map 缓存目录 |
| `--pipeline-max-steps` | | `50` | 单次运行总步数上限 |
| `--pipeline-max-retries` | | `0` | 阶段重试上限;`0` 继承 codrax.yaml |
| `--pipeline-max-stage-visits` | | `0` | 阶段访问次数上限;`0` 继承 |
| `--log` | | (空) | 附加运行时日志:文件路径,或 `-` 从 stdin 读。**可重复**(`--log a.log --log b.log`),多文件之间自动加 `# codrax-source: <path>` 边界头 |
| `--log-text` | | (空) | 附加内联日志字符串(与 `--log` 互斥) |
| `--log-source-prefix` | | (空) | CI 绝对路径前缀,适合 C/C++ 场景 |
| `--htrace` | | (空) | 附加性能 trace(HiTrace / atrace / systrace / perfetto):文件路径或 `-`。**可重复**。`--atrace` 是别名(同一通道) |
| `--htrace-text` | | (空) | 内联 trace 文本(与 `--htrace` 互斥) |
| `--atrace` | | (空) | `--htrace` 的 Android 别名,等价用法 |
| `--atrace-text` | | (空) | `--htrace-text` 的 Android 别名 |
| `--auto-init-repo` | | `false` | 写模式:授权 codrax 在裸目录或无 commit 的 repo 上自动 `git init` + 空 initial commit。yaml 等价键 `write_auto_init_repo` |
| `--version` / `-v` | | | 打印构建版本并退出 |

`--log` / `--log-text` 互斥;`--htrace` / `--atrace` 是同一通道的两个别名,设其一即可,跨别名同时设会报错。stdin (`-`) 跨 `--log` / `--htrace` / `--atrace` 整体只允许一次。日志/trace 通道字节默认上限 50 MiB(`log_attach_max_bytes` / `trace_attach_max_bytes`,可独立配置),超限尾部截断。

### 5.2 REPL 斜杠命令

进入 REPL 后输入这些命令(**每条都支持反斜杠替代**,如 `\exit` 等价 `/exit`)。Tab 键触发自动补齐面板;对需要参数的命令,补齐后会自动留一个空格继续输入。

**通用**

| 命令 | 别名 | 作用 |
|---|---|---|
| `/exit` / `/quit` | `/q` | 退出 REPL |
| `/help` | `/h` | 显示斜杠命令列表 |
| `/version` | `/v` | 打印构建版本 |
| `/history` | | 列出当前记忆里的最近轮次 + 压缩索引 + 写模式 plan 历史 |
| `/clear` | | 清对话记忆(会二次确认,且提示当前几个对等进程在用同一份记忆) |
| `/compact` | | 主动把所有旧轮次压缩到 MEMORY.md(保留最新一轮) |
| `/log <path>` | | 加载文件为附加日志(替换现有附加) |
| `/log append <path>` | | 把另一个文件追加到现有附加日志后,自动加 `# codrax-source: <path>` 边界头 |
| `/log` | | 进入日志粘贴模式,以单独一行 `/end` 结束 |
| `/log show` | | 打印附加日志的前 20 行 + 总字节数 |
| `/log clear` | | 清除附加日志(不清对话记忆) |
| `/htrace <path>` | | 加载性能 trace(HiTrace / atrace / systrace / perfetto)文件,替换现有 |
| `/htrace append <path>` | | 追加 trace 文件,带 `# codrax-source:` 边界头 |
| `/htrace show` | | 打印 trace 头 800 字节 |
| `/htrace clear` | | 清除附加 trace |
| `/atrace ...` | | `/htrace` 的 Android 别名(同一通道,所有子命令通用) |
| `/paste` | | 通用粘贴兜底(与 `/log` 不同,内容会作为下一次提问的输入) |
| `/chat <message>` | | 闲聊通道:本条消息不走分析流水线,单次 LLM 直接回复。适合打招呼、问工具能力、不需要读仓库的对话。详见 [3.3.15](#3315-闲聊命令chitchat_enabled) |

**写模式**(需 `codrax.yaml :: write_enabled: true`)

| 命令 | 作用 |
|---|---|
| `/mode [read\|plan\|apply\|verify]` | 无参数查看当前 mode;有参数切换(粘滞,影响后续所有提问) |
| `/plan show` | 审阅 pending plan(含 kind=patch / modify 的 unified diff 预览,per-change 4 KB、总计 16 KB 上限) |
| `/plan list` | 列出 PlanStore 里所有 plan(显示状态:pending_approval / applied / applied_failed / verify_failed / rejected) |
| `/plan clear` | 丢弃 pending plan(不记 memory;要记 memory 用 `/reject`) |
| `/approve [--merge-to=<branch>]` | 二次确认后触发 `apply + verify`。**只接受 Status=pending_approval 的 plan**,已 applied / rejected 的一律拒绝。`--merge-to=` 可选,成功后立即合并到该分支 |
| `/reject [reason]` | 丢弃 pending plan 并把理由记入 memory 历史 |
| `/verify [plan-id]` | 对已 applied 的 plan 重跑 verify(不重新 apply)。需要 plan 有保留的 worktree(即 Run 时开了 `pipeline_keep_worktree_on_success`)。无参数时用 pending plan |
| `/merge [--branch=<name>]` | 把最近 applied plan 的 worktree commit 合回主仓。无参数 → fast-forward 到 `--branch` 启动参数对应的分支(通常 `main`);有 `--branch=` → cherry-pick 到该新分支(主仓 HEAD 不动)。冲突时回滚,主仓恢复合并前状态 |
| `/worktree list` | 列出所有保留的 worktree(带 plan-id、worktree 路径、summary) |
| `/worktree discard <plan-id>` | 手动清理指定 plan 的 worktree 并清 `plan.worktree_path` 字段(plan 状态保持 applied,历史不改) |

**两种粘贴兜底的区别**:

| 子命令 | 内容去向 | 何时用 |
|---|---|---|
| `/log` (粘贴模式) | **附加日志通道** — 会被 log_triage 分诊,结构化后喂下游 | 贴 panic、异常栈、traceback、编译错误、应用运行时日志 |
| `/paste` | **普通请求通道** — 作为下一条 prompt 的一部分,等价于手敲 | 贴代码片段、错误消息、别人的诊断结论等 |

自动路由:如果你直接在提问里粘了一段**明显的日志**(连续 3 行以上时间戳/栈帧),codrax 会**自动**识别并把那部分搬到附加日志通道,只保留本轮生效(一次性)。想让附加日志在后续多轮持久,用显式 `/log` 命令。

---

## 6. 输出界面速读

### 6.1 REPL 启动 banner

**正常环境**(rg + git 齐全):

```
   CODRAX  v0.1.20260424  /help · /exit
   memory: 4 recent turn(s) + 12 compacted, 8.3 KB total

❯❯
```

- 第一行:蓝底白字 badge + 构建版本 + 最常用的两条提示
- 第二行(可选):当前仓库下记忆摘要(最近几轮 + 压缩条数 + 总大小),为空时不显示

**降级环境**(缺工具时才显示对应行,根据 `--lang` 自动中英):

搜索后端分三档,**不是 rg 都会提示**:

| 后端 | REPL 提示 |
|---|---|
| `rg` (健康) | 无提示 |
| `grep` (rg 缺失) | `⚠ 搜索后端:grep (装 ripgrep 可进一步提速)` |
| `native` (rg 和 grep 都缺失) | `⚠ 搜索后端:Go 内置扫描器 (装 ripgrep 可大幅提速)` |

git 只要没装就报:

```
  ⚠ 未检测到 git (repomap 走文件遍历;git_diff / git_log 不可用)
```

完整的降级 banner 示例(两种都触发):

```
   CODRAX  vdev  /help · /exit
  ⚠ 搜索后端:Go 内置扫描器 (装 ripgrep 可大幅提速)
  ⚠ 未检测到 git (repomap 走文件遍历;git_diff / git_log 不可用)

❯❯
```

英文版(`--lang en`)相应渲染成 `Search backend: grep (install ripgrep for faster scans)` / `Search backend: native Go scanner (install ripgrep for a 10× speedup)` / `git not detected (...)`。更多安装提示见 [2.1 运行环境依赖](#21-运行环境依赖)。

### 6.2 流水线进行时的任务列表

每次提问后,codrax 会在 stderr(REPL 下直接在界面上,单次模式下会打到 stderr)绘制一块**可原地刷新的任务列表**:

```
   ┌────────────────────────────────────────────────────────────┐
   │ ⠋ analyze    (analyzer)        thinking              2.3s  │
   │ ✓ analyze    (analyzer)        done                  3.7s  │
   │ ⠹ explore[1] objective "...".  read_file foo.go     12.1s  │
   │ ...                                                         │
   └────────────────────────────────────────────────────────────┘
```

- `⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏` 是 braille spinner,提示该条正在运行
- `✓` 表示该条已完成
- 每行末尾是该条已运行的时间

### 6.3 实时推理 / 系统事件提示

流水线内部的调度决策会以**暗灰色 `💭` 行**打印在任务列表之上,不影响最终答案排版:

```
  💭 [analyzer-1] I need to classify the question subject first before...
  💭 [orchestrator] ⟳ 正在补齐调查证据
  💭 [orchestrator] › 调查就绪,准备作答
  💭 [orchestrator] ⟳ 正在组织最终答案
```

- `[<agent>-<iter>]` 是某个 agent 的 ReAct 轮次(`iter` 从 1 开始)
- `[orchestrator]` 是系统事件(补读、重试、准备答案、组织答案等),这些是**中英文本地化**的简短提示

### 6.4 最终答案

所有阶段完成后,答案渲染在一个**带左侧竖线**的块里:

```
  │
  │ # 回答
  │
  │ `explorer.ShouldStop` 的判定由若干布尔条件组合决定...
  │
  │ **关键位置**:
  │ - `internal/agent/explorer.go:382`  主入口
  │ - ...
  │
```

- 带 `#` 的 markdown 会被解析成终端可读的加粗 / 标题
- 所有 `file:line` 引用都可以被 VSCode / JetBrains 等终端点击跳转
- 代码块会保持等宽字体对齐

---

## 7. 场景实操

下面的例子全部以 codrax 本身为目标仓库,你可以原样复现。

### 7.1 问一段代码的行为

**目标**:了解一个函数做什么,找到它的关键位置。

```bash
cd ~/code/codrax
codrax
```

```
❯❯ explorer 的 ShouldStop 什么时候会返回 true?
```

流水线走完(通常 1-3 分钟,取决于你的 LLM)后,你会得到一段带具体 `file:line` 的散文答案,例如:

```
  │ `explorer.ShouldStop()` 在以下情况返回 true:
  │
  │ 1. **软停信号满足** — 当 `hasEnoughFacts` 为 true 且 ...
  │    ([explorer.go:382])
  │
  │ 2. **ERM 要求满足** — ...
  │    ([explorer.go:405])
  │
  │ 3. **Fallback S1 触发** — ...
  │    ([explorer.go:441])
```

### 7.2 分析 panic / 异常日志

**目标**:贴一段 Go panic 或 Java 异常栈,让 codrax 找出根因。

**方式 A:从文件加载**

```bash
# 保存日志到文件
kubectl logs pod/my-api > /tmp/panic.txt

# 启动 REPL 并附加
cd ~/code/my-service
codrax
```

```
❯❯ /log /tmp/panic.txt
  attached log loaded: /tmp/panic.txt (2843 bytes)

❯❯ 这个 panic 的根本原因是什么?
```

附加日志后每一次提问都会带上日志,直到 `/log clear` 或重启。这让你可以就同一个日志反复从不同角度提问:

```
❯❯ 修复方案呢?会不会有副作用?
❯❯ 有没有 regression 风险?
❯❯ /log clear          # 问完了清掉
```

**方式 B:直接在提问里贴**

如果日志不长(或从浏览器复制一段短栈),可以直接贴进 REPL:

```
❯❯ 帮我分析这段 panic:
…  goroutine 12 [running]:
…  main.handleRequest(0xc0001a8000)
…          /src/main.go:142 +0x1a5
…  panic: runtime error: invalid memory address
…  ...
```

codrax 会自动识别**连续时间戳 / 栈帧**,把日志部分搬到附加日志通道(只对本轮有效),剩下的"帮我分析这段 panic"作为提问主体。

**方式 C:管道 / 脚本**

```bash
# 单次运行,日志从 stdin 读
kubectl logs pod/my-api | codrax -r "分析这段 crash" --log -

# 或者内联字符串(适合短日志)
codrax -r "analyze this ASAN report" --log-text "$(cat /tmp/asan.out)"
```

**C/C++ 场景**:栈帧里的文件是构建机绝对路径(如 `/home/jenkins/workspace/build/src/foo.cpp:42`),你的本地仓库路径不一样,需要告诉 codrax 怎么剥离前缀:

```bash
codrax -r "trace this crash" --log /tmp/asan.out \
  --log-source-prefix /home/jenkins/workspace/build/src/
```

**多文件附加**:多份独立日志拼成一次分析:

```bash
codrax -r "对比两次 panic,找根因差异" \
  --log /tmp/panic-pid-1234.txt --log /tmp/panic-pid-5678.txt
```

REPL:`/log /tmp/a.log` → `/log append /tmp/b.log`。两份之间会自动插入 `# codrax-source: <path>` 边界头让 LLM 区分独立 capture。

### 7.2.1 HarmonyOS / Android 性能 trace 分析

**目标**:贴一份 HiTrace(`hdc shell hitrace`) 或 Android atrace(`adb shell atrace`) / systrace / perfetto 文本输出,让 codrax 找出 jank / 主线程阻塞 / 冷启动慢点。

**单次抽取**(适合 < 64 KB 的小 trace):

```bash
# HarmonyOS
hdc shell hitrace -t 10 ace app_startup graphic > /tmp/perf.trace
codrax -r "为什么这个页面打开有掉帧?" --htrace /tmp/perf.trace

# Android(等价,用 alias 名更顺手)
adb shell atrace -t 10 view gfx app > /tmp/perf.atrace
codrax -r "where is the jank coming from?" --atrace /tmp/perf.atrace
```

**两步分诊**(自动触发,trace ≥ 64 KB 或单次 coverage < 0.3):codrax 先调 `emit_perf_segmentation` 把 trace 切成 `frame_window / jank_region / startup / thread_run` 段,再对每段单独调 `emit_perf_trace`,最后 merge。LLM 调用上限 `perf_triage_max_llm_calls`(默认 12)。

**多 trace 合并**(多次抓样、多 PID、跨设备对比):

```bash
codrax -r "对比这三次冷启动" \
  --htrace boot-1.atrace --htrace boot-2.atrace --htrace boot-3.atrace
```

REPL:`/htrace boot-1.atrace` → `/htrace append boot-2.atrace` → `/htrace append boot-3.atrace`。

**双通道并行**(同时附 panic 日志 + 性能 trace,trace 内有相关 jank 帧时尤其有用):

```bash
codrax -r "应用卡顿后 crash,根因是什么?" \
  --log /tmp/hilog.txt --htrace /tmp/jank.atrace
```

两个前置阶段独立运行,bundle 各自写到 `Mutable.LogTriage()` / `Mutable.PerfTrace()`,analyzer 同时消费两份的 entities + ResolvedFiles。

**支持的来源**:HiTrace(HarmonyOS hdc) / atrace(Android adb) / systrace(Android 旧名) / perfetto 文本 dump,LLM 自动从 `# tracer: nop` 或 `tracing_mark_write: B|...|<tag>` 等头部判断 source 字段。

### 7.3 写模式:小步修复一个 bug

**目标**:用 codrax 自动生成 + 应用一个一行补丁,跑测试验证,review 后手动 cherry-pick 到主仓。完整的写模式介绍在 [4.3](#43-写模式--plan--apply--verify),这里给一个最小可复现示例。

**前置**:`codrax.yaml` 里开三行:

```yaml
write_enabled: true
pipeline_keep_worktree_on_success: true   # 成功后留下 worktree 让你 review
pipeline_write_retry_budget: 2            # 测试一次失败可以让 planner 自动再试 2 次
```

**操作**:

```bash
cd ~/code/myproject
codrax
```

```
❯❯ /mode plan

❯❯ utils/parse.go 的 ParseTimeout 在收到 "0" 时返回了 0 秒(应该返回
…  默认 30 秒),帮我修这个 bug 并补一条单元测试

   ... (planner 跑约 30-90 秒) ...

  plan saved: /home/me/code/myproject/.codrax/plans/plan-1730834521... json (2 changes)

❯❯ /plan show         # 检查 unified diff 预览
   ... (略,见 §4.3.4 的样例) ...

❯❯ /approve
  Approve plan plan-... (2 change(s))? Apply inside a git worktree + run verify.
  > Yes

   ... (apply + verify 约 1-3 分钟) ...

  │ ## 变更应用成功
  │ 2 个文件已在 worktree 里更新; go test ./utils/ 全部通过 (12 passed, 0 failed)

  worktree preserved: /home/me/code/myproject/.codrax/worktrees/<trace>-<pid>

❯❯ /exit
```

**验收 + 合并**:

```bash
cd ~/code/myproject/.codrax/worktrees/<trace>-<pid>
git log --oneline main..HEAD     # 看到 codrax 在 worktree 里产生的 commit
git diff main                    # 完整 diff

# 满意就 cherry-pick 回主仓
cd ~/code/myproject
git cherry-pick <sha>            # codrax 永远不会自动做这一步

# 用完后清理 worktree
codrax
❯❯ /worktree discard plan-1730834521-...
```

**出错时的自动恢复**:如果 planner 生成的 kind=patch 因为行号漂移导致 `git apply` 失败,`pipeline_write_retry_budget=2` 会让系统:

1. 把"hunk @@ -620,7 +620,7 doesn't match"这个错误 + 嫌疑文件喂回 planner
2. planner 重新生成一份带正确上下文的 plan
3. 自动再 apply + verify

直到成功或耗尽 retry。

**安全网总结**:

- 主仓 HEAD 字节永不被自动写;只要不 cherry-pick,主仓就保持原状
- W1/W1b 写闭包检查在工具层硬挡(不是靠 prompt 提醒 LLM 的"软"约束)
- 失败的 worktree 自动销毁;`pipeline_keep_worktree_on_success` 只保留**成功**的

### 7.4 脚本化批处理

**目标**:对一批问题跑回归 / 自动化生成代码概要。

```bash
#!/usr/bin/env bash
cd ~/code/my-repo

while IFS= read -r question; do
  echo "---- $question ----"
  codrax -r "$question" --lang en 2>/dev/null
  echo
done < questions.txt > report.md
```

- `2>/dev/null` 把实时任务列表丢弃,只留最终 markdown
- `--lang en` 强制英文输出(覆盖 `codrax.yaml` 里的 `lang: zh`)

### 7.5 同一台机器操作多个仓库

codrax 会**自动用 `--repo` 的绝对路径生成 hash slug**,把日志和对话记忆分目录。同一个 codrax 二进制可以服务多个仓库而互不污染:

```bash
# 仓库 A 的工作
cd ~/code/repoA
codrax

# 另开一个终端,仓库 B 的工作
cd ~/code/repoB
codrax
# 看到的记忆摘要是 repoB 自己的,不会混入 repoA 的历史
```

生成的目录结构:

```
~/code/repoA/.codrax/
  logs/repoA-a3f9c2b1/
  memory/repoA-a3f9c2b1/

~/code/repoB/.codrax/
  logs/repoB-7d51e04f/
  memory/repoB-7d51e04f/
```

甚至可以同一个仓库**多个 codrax 实例并发**,日志按 PID 隔离、记忆写入用 flock 串行化,`/clear` 前会提示当前还有几个 peer 在用。

### 7.6 长对话与记忆管理

REPL 模式下默认保留最近 6 轮(或 20 KB 总字节,哪个先到)。超出后最旧的一轮会被 LLM 压成一行 summary 放进 `MEMORY.md`,后续提问时若关键词匹配,这些历史 summary 会被重新召回作为上下文。

**查看当前记忆状态**:

```
❯❯ /history
   compacted index:
     - [turn-17321...] topic-t3  — keywords: explorer, ShouldStop, ERM
     - [turn-17321...] topic-t5  — keywords: analyzer, TaskGraph, complexity
   recent (4 turns):
     ...
```

**主动压缩**(大段粘贴 / 攒了很多轮后,想节省下一次提问的 token):

```
❯❯ /compact
  compaction done. recent=1 index=8
```

**清空重开**(注意会二次确认;若有对等进程在用同一份记忆会警告):

```
❯❯ /clear
  1 peer(s) currently using this memory directory.
  Type 'y' to confirm: y
  conversation memory cleared.
```

**附加日志独立于对话记忆**。`/clear` 不清附加日志,反之 `/log clear` 不清对话记忆,各走各的生命周期。

---

## 8. 常见问题

**Q: 启动报 `providers config not found`,怎么办?**
A: 去 `~/tools/codrax/`(或你放二进制的地方)检查有没有 `providers.yaml`。没有的话参考 [3.1 精简版](#31-providersyaml--精简版) 创建一份。查找路径可以在启动时用 `--providers /absolute/path/to/providers.yaml` 显式指定。

**Q: LLM 返回错误(`401 Unauthorized` / `403 Forbidden`),提问永远卡住或报错?**
A: 多半是 `api_key` / `base_url` / `model` 配错。用 `curl` 手动验证一下:
```bash
curl $BASE_URL/chat/completions \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"YOUR_MODEL","messages":[{"role":"user","content":"hi"}]}'
```

**Q: 答案里引用的 `file:line` 是错的,怎么办?**
A: 常见原因是 `--repo` 没指对(比如在 `~/code` 启动但想问 `~/code/my-project`)。codrax 默认把当前工作目录 `.` 当仓库根,所以**务必先 `cd` 到目标仓库再启动**,或者显式 `codrax --repo /abs/path`。

**Q: 想让 codrax 写英文回答?**
A: 两种方式:
- 临时:`codrax --lang en -r "..."`
- 永久:`codrax.yaml` 里写 `lang: en`

**Q: 我的 LLM 不稳定,经常超时;能不能加大重试?**
A: `codrax.yaml` 里:
```yaml
pipeline_max_retries_per_stage: 5
pipeline_max_stage_visits: 6
agent_max_iterations: 30
```

**Q: 想换一批便宜的模型节省开支?**
A: 见 [3.2 复杂版](#32-providersyaml--复杂版)。把 `explorer` 和 `extractor` 换成便宜模型 / 本地模型,保留 `analyzer` 和 `finalizer` 为强模型,是最常见的省钱配置。

**Q: 粘贴多行总是变成 `[Pasted text]` 占位符,想粘代码的时候很不方便?**
A: 默认单行 ≥ 100 字符才会折叠,代码片段多在这个量级。想更激进地折叠 / 完全禁用:
```yaml
repl_paste_fold_min_chars: 9999    # 等于禁用折叠
```

**Q: `.codrax/` 目录越来越大,怎么清?**
A: 直接 `rm -rf .codrax/`。codrax 会在下一次启动重建。内部已经有滚动策略(日志 7 份、blob 会话 7 个),所以正常不会爆。

**Q: 启动日志里看到 `search backend: native Go scanner` 的 WARN,严重吗?**
A: 不严重 —— 只是 `rg` 和 `grep` 都没找到,codrax 已自动切到内置 Go 正则扫描兜底。功能等价,速度大概慢 5-20 倍(仍然在可接受范围)。想恢复最佳性能:
- Linux:`apt install ripgrep`(或 `apk add ripgrep`)
- macOS:`brew install ripgrep`
- Windows:`winget install BurntSushi.ripgrep.MSVC` 或从 [ripgrep releases](https://github.com/BurntSushi/ripgrep/releases) 下 zip

见 [2.1 运行环境依赖](#21-运行环境依赖)。

**Q: 启动日志里看到 `git not found on PATH` 的 WARN,会影响什么?**
A: 影响两点:`git_diff` / `git_log` 工具不可用(但不会 crash,会返回带错误说明的结果);repomap 构建时无法走 `git ls-files` 快路径,只能用 Go 的 `filepath.Walk`,在大仓上慢一些。装个 git(`apt install git` / Git for Windows)即可恢复。

**Q: Windows 上跑 codrax 要装什么?**
A: 强制装一个: [Git for Windows](https://git-scm.com/download/win) —— 它同时提供 `git` + `sh` + `bash` + `grep` + `find`,几乎覆盖 codrax 需要的所有外部工具。再可选装 ripgrep 提速:`winget install BurntSushi.ripgrep.MSVC`。装完后打开一个新的 PowerShell / CMD(让 PATH 生效),`codrax -r "..."` 就能跑。见 [2.1 运行环境依赖](#21-运行环境依赖)。

**Q: 我把 codrax 放进 `FROM scratch` / distroless 容器,能跑吗?**
A: 能跑,但有两个注意点:
1. **必须在镜像里装 git**,否则 `git_diff` / `git_log` 工具失能,且 repomap 扫描会变慢。推荐底包 `gcr.io/distroless/base-debian12:debug` 或 `alpine:latest`(后者体积小)。
2. **建议加装 ripgrep** 提搜索速度;不装也行,走 Go native fallback。
3. Alpine 底包里 BusyBox `grep` 缺 ripgrep 的 `--json` 模式,但 codrax 的 keyword search 会自动选择 `grepFiles` 分支,不触发 `--json` 路径,**不影响功能**。

典型 Dockerfile 片段:
```dockerfile
FROM alpine:latest
RUN apk add --no-cache git ripgrep
COPY codrax /usr/local/bin/
COPY providers.yaml /etc/codrax/
ENV CODRAX_SETTINGS=/etc/codrax/codrax.yaml
ENTRYPOINT ["codrax"]
```

**Q: 我的日志格式比较特殊(自研 JSON 应用日志),能分析吗?**
A: 可以。log_triage 是 LLM 驱动的,不依赖固定正则 parser。支持 Go panic / Java exception(包括 `Caused by` 链)/ Python traceback / Node V8 / Rust `#[source]` / Ruby backtrace / 结构化 JSON / C/C++ ASAN/UBSAN/gdb / 编译器错误等。模型能看懂的结构都能处理。只有仓内真实存在的文件才会被注入下游,外部路径会被过滤掉。

**Q: 想把 codrax 接进 CI,怎么保证输出稳定?**
A:
1. 固定 model(不要用 `-latest` 等别名)
2. 用单次模式 `codrax -r "..." 2>/dev/null > out.md`
3. 把 `codrax.yaml` 纳入 CI 环境(或用 `CODRAX_SETTINGS=ci.yaml`)
4. 对答案做基于 `file:line` 的结构化断言,比对着散文更稳定

**Q: 怎么开流式响应?有什么区别?**
A: **默认就是开的**。开启后:
- 任务行实时显示模型正在产出的内容 tail(250ms 节流,80 字符)
- `/chat` 以 typewriter 形式逐段上屏,短回复也能秒响应
- 慢模型 / 长响应下用户不再干瞪 30s 空 spinner
- 功能等价,只是 UX 更快反馈

想关回一次性响应(例如 CI 想要 byte-stable 输出、或者本地模型流式收益微乎其微):`providers.yaml` 里写 `stream: false`(可以写在 `llm.default` 也可以写在单个 agent 下)。见 [3.1.2 流式开关](#312-流式开关--stream)。

**Q: 启动报 `x509: certificate signed by unknown authority`?**
A: 你的 HTTPS endpoint 用了系统不信任的 CA。两条路:
1. **正路**(推荐):把对方 CA 抓到本地,`providers.yaml` 里 `tls_ca_file: /path/to/ca.pem`
2. **紧急路**:`tls_insecure_skip_verify: true`(关证书验证,API key 可能被嗅探;debug 完立刻关掉)
抓 CA:
```bash
openssl s_client -showcerts -servername $HOST -connect $HOST:443 </dev/null 2>/dev/null \
  | awk '/BEGIN CERT/,/END CERT/' > /tmp/their-bundle.pem
```

**Q: 启动报 `invalid character 'd' looking for beginning of value`?**
A: 是你用了一个**强制返 SSE** 的 provider(即使你 `stream: false` 它也流)。codrax 有 **SSE 自动嗅探**,响应开头是 `data:` 时自动走流式 parser,一般不用手动处理。若仍然出现,通常说明你运行的是较早版本 —— 拉最新代码重编即可。

**Q: HTTP 和 HTTPS 怎么切换?**
A: 不存在"切换 flag",由 `base_url` 开头决定:
- `base_url: "https://api.xxx.com/v1"` → TLS
- `base_url: "http://localhost:8080/v1"` → 明文 HTTP
见 [3.1.1 HTTP / HTTPS 开关](#311-http--https-开关--靠-base_url-协议头决定)。

---

更多细节请看:

- [README.md](../README.md) — 项目概览 + 快速开始
- [architecture.md](architecture.md) — 完整架构设计(面向想理解内部原理的读者)
- [codrax.yaml.example](../codrax.yaml.example) — 所有可调参数的带注释示例
- [providers.yaml.example](../providers.yaml.example) — LLM 凭证配置模板
