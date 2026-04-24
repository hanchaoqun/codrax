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
| `pipeline_max_retries_per_stage` | `3` | 某个阶段连续失败多少次后放弃 |
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

**接入上限(`log_attach_*`,在 log_triage **之前**生效)**

| 键 | 默认值 | 作用 |
|---|---|---|
| `log_attach_max_bytes` | `1048576`(1 MB) | 每个附加日志的字节上限。适用于 `--log <file>` / `--log -` / `--log-text` / REPL `/log <path>` / `/log` 粘贴 / 行内自动识别 5 条路径。超限尾部截断并打 `WARN [cmd] attached log truncated`;stdin 用 `io.LimitReader(N+1)` 保证进程内存不因多 GB 管道爆表。`log_triage_enabled: false` 下也会生效(管的是内存,不管分诊)。非正值(含显式 0)视为"使用默认",避免意外把 cap 调成 0 后所有 `/log` 静默失效 |

> ⚠️ **10 M 以上日志怎么办**:调高 `log_attach_max_bytes` 只解决"能喂进来"这一步;真正的瓶颈在 LLM 侧。建议先用 `grep -A50 -B5 'panic\|Exception\|FATAL'` 预过滤关键段,或同时把 `log_triage_max_llm_calls` 提到 16(给分段-提取更多预算)。分页读取**不是**靠 LLM `read_file offset/limit`,而是系统侧按字节窗口切片,每轮 LLM 只看一个窗口(见 `§4.5 日志分诊` 两步法)。

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
| `pipeline_max_verify_retries` | `0` | verify→plan 重试循环上限。`0` 保守(verify 失败直接报告);`>0` 启用重试 —— verify 失败后生成 PlanningHint(失败 summary + 失败测试 + 嫌疑文件)喂给 planner 二次规划。硬上限 5 |
| `pipeline_baseline_capture_enabled` | `false` | apply 前跑一次测试套件作为 baseline 快照。verifier LLM 据此把失败归类为 REGRESSION / PRE-EXISTING / FIXED。测试墙钟时间翻倍,仓库有已知 pre-existing 失败时打开 |
| `pipeline_keep_worktree_on_success` | `false` | apply + verify 双双成功时**不销毁** worktree,路径暴露给用户 review 或 cherry-pick。失败路径无条件销毁 |

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

**启用**(一次性):在 `codrax.yaml` 里显式:

```yaml
write_enabled: true
```

没这一行所有写模式子命令都会 fail-loud 拒绝(避免误开)。

#### 三个阶段 + 四种 Mode

| Mode | 做什么 | 产出 |
|---|---|---|
| `read`(默认) | 走分析流水线,输出带 citation 的答案 | `AnswerDocument` |
| `plan` | 读代码,产出一份"要改哪些文件、每个怎么改、先后依赖"的 ChangePlan | `.codrax/plans/<id>.json` |
| `apply` | 在 git worktree 里执行 ChangePlan,每个单元一次 `apply_patch`(四种 kind:create / modify / delete / patch) | worktree 里的文件 + `ChangeReport` |
| `verify` | 对 worktree 跑测试套件(9 种 runner 自动探测) | `.codrax/plans/<id>.report.json` |

`apply` 自动紧跟 `verify`,二者成对;你通常不需要手工触发 `--mode=verify`,除非要**重跑 verify**(如 flakey 测试、修改配置后验证)。

#### 两种触发通路

**REPL(推荐,支持审阅)**:

```
❯❯ /mode plan                   # 粘滞切换到 plan 模式
❯❯ 把 handleFoo 里 X 的处理从 A 改成 B    # 正常提问,会生成 ChangePlan
  ... (流水线运行) ...
  plan saved: .codrax/plans/plan-<id>.json  (3 change(s))

❯❯ /plan show                   # 审阅 plan — 会渲染 unified diff 预览
  current plan: .../plan-<id>.json
    id:      plan-<id>
    status:  pending_approval
    changes: 3 file(s)
    targets: internal/foo/handler.go, internal/foo/handler_test.go, ...
    summary: <planner 写的 3-10 句话总结>

  diff preview:

  ─── internal/foo/handler.go (kind=patch) ───
  rationale: 把 X 分支的 A 策略改为 B
  --- a/internal/foo/handler.go
  +++ b/internal/foo/handler.go
  @@ -42,7 +42,7 @@
   func handleFoo(x int) int {
  -  return strategyA(x)
  +  return strategyB(x)
   }
  ... (更多改动)

❯❯ /approve                     # 二次确认后触发 apply + verify
  Approve plan plan-<id> (3 change(s))? Apply inside a git worktree + run verify.
  [Yes] / [No]

❯❯ /reject [optional reason]    # 不想要了 — 丢弃 plan,记入 memory
❯❯ /plan clear                  # 等价 reject 但不留记忆

# 如果开了 pipeline_keep_worktree_on_success,apply+verify 成功后
# worktree 会保留给你 review:
  worktree preserved: /path/to/repo/.codrax/worktrees/<id>-<pid>

❯❯ /worktree list               # 列出所有保留的 worktree
❯❯ /worktree discard <plan-id>  # 手动清理某个

❯❯ /verify <plan-id>            # 对某个已 applied 的 plan 重跑 verify
                                # (需要 worktree 还在)
```

**单次命令(适合 CI / 自动化)**:

```bash
# 只生成 plan,不 apply(人工或脚本后续审查)
codrax --mode=plan --request "描述变更" --plan-out /tmp/plan.json

# 应用一份已有 plan(auto-apply 绕过交互确认;适合 CI)
codrax --mode=apply --plan-file /tmp/plan.json --auto-apply

# 只重跑 verify(需要已 applied 且 worktree 保留)
codrax --mode=verify --plan-file /tmp/plan.json
```

#### 四种改动类型(FileChange.kind)

| kind | 语义 | 要求 |
|---|---|---|
| `create` | 新建文件,写入 `new_content` 完整 body | 目标路径**必须不存在**,否则 apply 拒绝(避免误覆盖) |
| `modify` | 覆盖整个文件 body,写入 `new_content` | 目标路径**必须存在**,否则 apply 拒绝(planner 若搞错应改成 `create`) |
| `delete` | 删除文件 | 缺失文件视作幂等成功(带 warning 日志) |
| `patch` | 把 `patch` 字段里的 unified diff 喂给 `git apply -` | git 会严格按上下文行号匹配;不匹配直接拒绝,coder 下轮自修正 |

Planner 默认倾向于 `patch`(小改),需要整体重写时选 `modify`。

#### 写闭包保护(W1 / W1b)

每次 `apply_patch` 调用都经过:
- **W1**:`path` 必须出现在 `ChangePlan.TargetPaths` 声明的写作用域内 —— 防止 coder 飘到未声明路径。
- **W1b**:`DependsOn` 里每个前置单元必须已 applied(写入 `WriteClosure.AppliedSet`) —— 保证拓扑顺序。

违规会被工具层直接拒绝,coder 在下一轮看到错误自修正。

#### 可选:verify→plan 重试循环

`pipeline_max_verify_retries`(yaml,默认 0,硬上限 5)大于 0 时,verify 失败会清空 plan + 生成一份 PlanningHint(失败 summary + 前 3 条失败测试名 + 每条的 FailureDetail 首行 + 前一 plan 的 `TargetPaths` 嫌疑清单),再 dispatch planner 做第二次规划。如果新 plan 也失败,继续 —— 直到成功或耗尽重试预算。

默认 0 是为了保守(fail-loud);开启后会花更多 LLM token,适合你相信 planner 能在几轮内收敛的场景。

#### 可选:baseline 测试快照

`pipeline_baseline_capture_enabled`(yaml,默认 false)打开后,apply 之前先跑一遍测试套件作为 baseline。verifier LLM 在写 FailureSummary narrative 时会看到"这些测试在 apply 前就挂了"的清单,可以明确把失败分成:

- **REGRESSION**:baseline 过,现在挂 — **这个 plan 造成的**
- **PRE-EXISTING**:baseline 就挂,现在还挂 — 与本次变更无关
- **FIXED**:baseline 挂,现在过 — 顺手修了(bonus)

开启会让测试墙钟时间翻倍;适合你仓库本身有已知的 flakey 或 pre-existing 失败、否则容易把 pre-existing 误认为 regression。

#### 可选:成功时保留 worktree

`pipeline_keep_worktree_on_success`(yaml,默认 false)打开后,apply + verify 双双通过时,Run 结束**不销毁** worktree,把路径暴露给用户 review。失败路径无条件销毁(避免磁盘垃圾堆积)。

典型工作流:

```bash
# 开启 keep-on-success 后走一轮 /approve
❯❯ /approve
  ... 成功 ...
  worktree preserved: /path/to/repo/.codrax/worktrees/abc-123

# 在另一个终端:
cd /path/to/repo/.codrax/worktrees/abc-123
git log --oneline      # 看 apply 产生的 commit
git diff main          # 看 worktree 相对 main 的 diff
git checkout <sha>     # 如果想 cherry-pick 回 main
cd /path/to/repo
git cherry-pick <sha>  # 合回去(codrax 不会自动做这步)

# REPL 里也能随时看 / 清:
❯❯ /worktree list
❯❯ /worktree discard abc-123
```

#### 9 种测试 runner 自动探测

verify 阶段通过仓根的 manifest 文件自动选 runner:

| 探测文件 | Runner | 命令 |
|---|---|---|
| `go.mod` | Go | `go test -json ./...` |
| `package.json` | Node | `npm test -- --json --silent`(兼容 jest / vitest) |
| `pyproject.toml` / `pytest.ini` / `setup.py` | Python | `pytest --json-report` (需 `pytest-json-report` 插件) |
| `Cargo.toml` | Rust | `cargo test` |
| `pom.xml` | Java (Maven) | `mvn -B -q test`,读 `target/surefire-reports/*.xml` |
| `build.gradle` / `build.gradle.kts` | Java (Gradle) | `./gradlew --no-daemon --console=plain test`,读 `build/test-results/test/*.xml` |
| `Gemfile` | Ruby | `bundle exec rspec --format json` |
| `CMakeLists.txt` | CMake | `ctest --output-junit`(需要 build 目录已配置,支持 `build/` / `builddir/` / `out/` / `cmake-build-debug/` / `cmake-build-release/`) |
| `meson.build` | Meson | `meson test --xunit-file` |
| `Makefile` / `makefile` / `GNUmakefile` | Make | 优先 `make check` 再 `make test`(读 Makefile 首行列扫),exit code 判定 |

多 manifest 并存时按上表顺序取先找到的(例如 CMake 生成的 Makefile 会被 CMakeLists.txt 压过)。构建失败(测试还没开始跑)时会自动合成一条失败 `TestResult`,`FailureDetail` 里带从 stdout / stderr 提取的错误行,给 retry 循环的 PlanningHint 真正有用的信息。

不在上表的语言 / 构建系统,`/verify` 会明确报错 "no supported test runner detected",并列出所有它查找过的文件名 —— 你可以补一份符合约定的 manifest,或者在 codrax.yaml 指定 Runner / 自行加装 wrapper。

#### 当前限制

- 写模式**不支持** multi-plan concurrency;同一仓库不要并行跑两个 `/approve`。
- worktree 不会自动 push 到任何 remote;合并到主仓永远是用户的手动操作。
- `git apply -` 失败时 git 的 stderr 会原样透传给 coder,但**不会**自动回退已 applied 的单元(符合 git 默认行为);如果你的 plan 有多文件依赖,建议用 `DependsOn` 明确排序,让失败单元及其下游整块被 retry。

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
| `--log` | | (空) | 附加运行时日志:文件路径,或 `-` 从 stdin 读 |
| `--log-text` | | (空) | 附加内联日志字符串(与 `--log` 互斥) |
| `--log-source-prefix` | | (空) | CI 绝对路径前缀,适合 C/C++ 场景 |
| `--version` / `-v` | | | 打印构建版本并退出 |

`--log` / `--log-text` 都只能用其一;日志体超过 1 MB 自动截断。

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
| `/log <path>` | | 加载文件为附加日志(会替换现有附加) |
| `/log` | | 进入日志粘贴模式,以单独一行 `/end` 结束 |
| `/log show` | | 打印附加日志的前 20 行 + 总字节数 |
| `/log clear` | | 清除附加日志(不清对话记忆) |
| `/paste` | | 通用粘贴兜底(与 `/log` 不同,内容会作为下一次提问的输入) |
| `/chat <message>` | | 闲聊通道:本条消息不走分析流水线,单次 LLM 直接回复。适合打招呼、问工具能力、不需要读仓库的对话。详见 [3.3.15](#3315-闲聊命令chitchat_enabled) |

**写模式**(需 `codrax.yaml :: write_enabled: true`)

| 命令 | 作用 |
|---|---|
| `/mode [read\|plan\|apply\|verify]` | 无参数查看当前 mode;有参数切换(粘滞,影响后续所有提问) |
| `/plan show` | 审阅 pending plan(含 kind=patch / modify 的 unified diff 预览,per-change 4 KB、总计 16 KB 上限) |
| `/plan list` | 列出 PlanStore 里所有 plan(显示状态:pending / applied / applied_failed / verify_failed / rejected) |
| `/plan clear` | 丢弃 pending plan(不记 memory;要记 memory 用 `/reject`) |
| `/approve` | 二次确认后触发 `apply + verify`。**只接受 Status=pending_approval 的 plan**,已 applied / rejected 的一律拒绝 |
| `/reject [reason]` | 丢弃 pending plan 并把理由记入 memory 历史 |
| `/verify [plan-id]` | 对已 applied 的 plan 重跑 verify(不重新 apply)。需要 plan 有保留的 worktree(即 Run 时开了 `pipeline_keep_worktree_on_success`)。无参数时用 pending plan |
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

### 7.3 写模式:小步修复一个 bug

**目标**:用 codrax 自动生成 + 应用一个小补丁,跑测试验证,review 后手动 cherry-pick 到主仓。

**前置**:`codrax.yaml` 里开:

```yaml
write_enabled: true
pipeline_keep_worktree_on_success: true   # 让 worktree 留下给你 review
pipeline_max_verify_retries: 2            # 如果 plan 写得有问题允许自动再试两次
```

**操作**:

```bash
cd ~/code/codrax           # 或你自己的仓库
codrax
```

```
❯❯ /mode plan
  mode → plan (sticky)

❯❯ internal/repl/input.go 里 needsArg 缺了 /worktree,帮我把它加上,
…  保持和其它几个命令同形状的 switch case

   ... (planner 跑约 30-90 秒) ...

  plan saved: /path/to/repo/.codrax/plans/plan-<id>.json  (1 change)

❯❯ /plan show
  current plan: .../plan-<id>.json
    id:      plan-<id>
    status:  pending_approval
    changes: 1 file(s)
    targets: internal/repl/input.go
    summary: Add "/worktree" to needsArg switch so Tab-complete leaves
             a trailing space for the subcommand argument.

  diff preview:

  ─── internal/repl/input.go (kind=patch) ───
  rationale: 与其它接参命令保持一致
  --- a/internal/repl/input.go
  +++ b/internal/repl/input.go
  @@ -620,7 +620,7 @@
   func needsArg(cmd string) bool {
    switch cmd {
  -  case "/log", "/chat", "/mode", "/plan", "/reject", "/verify":
  +  case "/log", "/chat", "/mode", "/plan", "/reject", "/verify", "/worktree":
      return true
    }
    return false
   }

❯❯ /approve
  Approve plan plan-<id> (1 change(s))? Apply inside a git worktree + run verify.
  > Yes

   ... (apply + verify 约 1-3 分钟) ...

  │ ## 变更应用成功
  │
  │ 1 个文件已在 worktree 里更新; go test ./internal/repl/ 全部通过
  │ (18 passed, 0 failed)

  worktree preserved: /path/to/repo/.codrax/worktrees/<trace>-<pid>

❯❯ /exit
```

**验收 + 合并**:

```bash
# review 一下 worktree 里的实际改动
cd /path/to/repo/.codrax/worktrees/<trace>-<pid>
git log --oneline main..HEAD
git diff main

# 满意就 cherry-pick 回主仓
cd /path/to/repo
git cherry-pick <worktree-commit-sha>

# 不要了就手动清掉 worktree(或回 REPL 用 /worktree discard)
rm -rf .codrax/worktrees/<trace>-<pid>
```

**出错示例**:如果 planner 生成的 kind=patch 因为行号漂移 `git apply` 失败了,`pipeline_max_verify_retries` 为 2 时你会看到:

```
   ... apply failed: hunk at @@ -620,7 +620,7 doesn't match ...
   ... retry attempt 2: PlanningHint seeded with suspect file ...
   ... planner 重新生成新 plan ...
   ... apply + verify 成功 ...
```

**安全网**:
- W1 写闭包保护 —— 哪怕 planner 想改未声明的文件,`apply_patch` 工具层也会拒绝。
- 主仓 HEAD 字节永不被自动写;哪怕 worktree 里所有 commit 都已落盘,只要你不 cherry-pick,主仓就还是之前的样子。
- 失败的 worktree 被自动销毁(`pipeline_keep_worktree_on_success` 只保留**成功**的)。

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
