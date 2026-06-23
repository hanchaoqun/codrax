# codrax 使用指南

> **CODRAX — Code Of Deterministic Reasoning, Anchored eXplanation**
>
> **CODRAX —— 推理确定,解释锚定**
>
> 每条结论锚到字节,每段推理可被复核。

---

## 这本手册怎么读

- 第一次用 codrax → 直接看 **第 1 章 5 分钟入门**,跟着抄就能问出第一个答案。
- 想了解日常用法 → 第 2、3 章覆盖 REPL 模式、附加日志、闲聊、本地转换。
- 想做本地数据清洗、汇总、过滤或严格格式输出 → 看 **3.7 数据处理任务**。
- 想接外部工具、知识库或电脑操作 Skills → 看 **3.8 外部工具与外部 Skills**。
- 想让 codrax 真改代码 → 第 4 章 写模式 Auto Pilot。
- 想精调或排错 → 第 5–8 章:配置参考、命令参考、排错。

新手只需读到 4.2 就够了;5 章往后是参考文档,需要时查阅即可。

---

## 目录

- [1. 5 分钟入门](#1-5-分钟入门)
- [2. 基本用法](#2-基本用法)
  - [2.1 提问的三种姿势](#21-提问的三种姿势)
  - [2.2 看懂界面](#22-看懂界面)
  - [2.3 多轮对话与"接着上轮换格式"](#23-多轮对话与接着上轮换格式)
  - [2.4 中断 / 取消](#24-中断--取消)
- [3. 进阶能力](#3-进阶能力)
  - [3.1 附加运行时日志(panic / 异常 / traceback)](#31-附加运行时日志panic--异常--traceback)
  - [3.2 附加性能 trace(HiTrace / atrace / systrace / perfetto / perf sample)](#32-附加性能-tracehitrace--atrace--systrace--perfetto--perf-sample)
  - [3.3 闲聊与本地转换](#33-闲聊与本地转换)
  - [3.4 记忆与会话](#34-记忆与会话)
  - [3.5 一台机器多仓库](#35-一台机器多仓库)
  - [3.6 跨仓 workspace(multi-repo discovery)](#36-跨仓-workspacemulti-repo-discovery)
  - [3.7 数据处理任务](#37-数据处理任务)
  - [3.8 外部工具与外部 Skills](#38-外部工具与外部-skills)
    - [先选入口](#先选入口)
    - [外部 Skills / Operation Skills](#外部-skills--operation-skills)
    - [MCP 外部工具](#mcp-外部工具)
- [4. 写模式 — Auto Pilot](#4-写模式--auto-pilot)
  - [4.1 启用](#41-启用)
  - [4.2 完整流程](#42-完整流程)
  - [4.3 把改动合回主仓](#43-把改动合回主仓)
  - [4.4 失败排错](#44-失败排错)
- [5. 配置参考](#5-配置参考)
  - [5.1 providers.yaml(LLM 凭证)](#51-providersyamllllm-凭证)
  - [5.2 codrax.yaml(运行参数)](#52-codraxyaml运行参数)
  - [5.3 配置查找顺序](#53-配置查找顺序)
- [6. REPL 命令参考](#6-repl-命令参考)
- [7. CLI 参考](#7-cli-参考)
- [8. 排错](#8-排错)

---

# 1. 5 分钟入门

## 1.1 codrax 能做什么

- **读模式(默认)**:用自然语言问代码仓库的问题(中文 / 英文都行),返回**带 `file:line` 引用**的答案。不修改任何文件。每条 citation 都经过 7 层落地校验,LLM 编造或与源码不一致的引用进 "Unverified Leads" 段不入正文。
- **写模式(可选)**:在沙箱 git worktree 里产生改动 plan,低/中风险自动推进,高风险才审批,critical 自动拒绝。主仓 HEAD 永不自动变。支持 12 种测试 runner 自动探测,verify 失败 → planner 自动重新规划(带早停守门)。
- **附加日志 / 性能 trace**:`--log` 喂 panic / exception / sanitizer / traceback,`--htrace` / `--atrace` 喂 HiTrace / atrace / systrace / perfetto;LLM 抽成结构化 bundle,系统侧验证仓内文件后注入下游。
- **不会**上传代码到任何第三方,只调用你在 `providers.yaml` 里指定的 LLM。

## 1.2 准备

只需要 3 样东西:

1. `codrax` 可执行文件 —— 下载预编译发布包(最省事),或 `git clone` 仓库后 `make` 自行构建(构建需 CGO 工具链:macOS `xcode-select --install`、Debian/Ubuntu `gcc musl-tools`)
2. 一把能访问 OpenAI 兼容接口的 LLM API key(OpenAI / DeepSeek / Qwen / 本地 vLLM / Ollama 都兼容)
3. 你想问的代码仓库

## 1.3 把二进制放进 PATH

```bash
mkdir -p ~/tools/codrax
mv codrax ~/tools/codrax/

# bash
echo 'export PATH="$HOME/tools/codrax:$PATH"' >> ~/.bashrc && exec "$SHELL" -l
# zsh
echo 'export PATH="$HOME/tools/codrax:$PATH"' >> ~/.zshrc && exec "$SHELL" -l
```

加进 PATH 之后,在任何代码仓里直接敲 `codrax` 就能启动;运行时产物(日志、记忆、工具输出)会写到当前目录的 `.codrax/` 子目录,不会污染源码。

## 1.4 配置一份最小的 `providers.yaml`

把下面这段保存到 `~/tools/codrax/providers.yaml`(和二进制同目录):

```yaml
llm:
  default:
    provider: openai           # 协议;OpenAI 兼容服务都用这个值
    api_key: "sk-xxx"          # 换成你的 key
    model: "your-model-id"     # 例如 "gpt-4o-mini" / "deepseek-chat"
    base_url: "https://api.openai.com/v1"
```

> 4 个字段缺任何一个,codrax 启动时会**直接报错并打印缺哪一个字段**,不会偷偷连任何公网 endpoint。

常见服务商配置(只改 `api_key` / `model` / `base_url` 三处):

| 服务 | base_url | 备注 |
|---|---|---|
| OpenAI | `https://api.openai.com/v1` | 国内可能需走代理 |
| DeepSeek | `https://api.deepseek.com/v1` | model 例:`deepseek-chat`;保持 `thinking_mode: auto` 即可兼容 tools |
| 阿里云 DashScope | `https://dashscope.aliyuncs.com/compatible-mode/v1` | model 例:`qwen-max` |
| Ollama 本地 | `http://localhost:11434/v1` | model 是你 `ollama pull` 拉过的名字 |
| vLLM 本地 | `http://localhost:8000/v1` | api_key 任写;model = `--served-model-name` |

DeepSeek 用户一般不用额外调参:`thinking_mode: auto` 会在官方 DeepSeek endpoint 主动关闭 provider 原生 thinking,避免它和 tools / `tool_choice` 冲突。`think_aloud` 只是 Codrax prompt 侧的“出声思考”提示,不是 API thinking 开关。

## 1.5 第一个问题

> **codrax 把「你启动它时所在的目录」当作要分析的代码仓库。** 它不会弹窗问你「要分析哪个项目」—— 默认分析的就是当前工作目录(等价 `--repo .`)。所以使用前请务必先 `cd` 进你想分析的项目根目录,再运行 `codrax`;在别的目录启动,它就会去分析那个目录。

```bash
cd /path/to/your/repo    # ① 先进入你要分析的代码仓库(任何 git 仓)
codrax                   # ② 再启动;它会索引「当前目录」这个仓库
```

启动后 codrax 会在当前目录建一个 `.codrax/` 子目录,存放日志、对话记忆、缓存等运行产物;读模式只读你的源码、从不修改。想分析的不是当前目录时,用 `--repo /path/to/repo` 显式指定。

看到这个就是启动成功了(每一行都是真实输出,版本号和路径会按你的环境替换):

```
   CODRAX  v0.1.x  /help · /exit
   模式: auto · code · operation · data (write 已禁用) · /home/you/tools/codrax/codrax.yaml
   记忆: 3 recent + 0 compacted, 4.8 KB
   模型: your-model · ctx=200k · timeout=10m0s

[git:main]❯❯
```

提示符前可能带 sticky 标签,例如 `[git:<branch>]`、`[task:data]`、`[task:write]`、`[focus:repo-a]`、`[trace]`、`[trace:2+perf]`、`[log]`、`[log:2]`、`[plan]`,显示当前 git 分支、显式任务模式、多仓 focus、已附加 trace/log、附件数量/是否包含 perf sample、待处理 plan 等状态。默认 `auto` 模式不显示 task 标签。

直接打你的问题、回车。提交后,你打的内容会以 `> ...` 形式回显在分隔线下方,然后下方开始打印进度:

```
─────────────────────────────────────
> 这个项目的入口函数在哪里?
· 1/4 正在统计仓库索引 `your-repo` 文件
✓ 1/4 仓库索引 `your-repo` 已就绪：123 个文件 (120ms)
⇢ 分析 · 第 1 轮 调用工具 emit_analysis intent=lookup
✓ 1/4 已理解问题 · 第 1 轮 · 1 次工具调用 · 本 2s · 总 3s
⇢ 探索 · 第 1 轮 调用工具 grep pattern=func main
› 4/4 正在生成最终答案
...
✓ 已撰写最终答案

  │
  │  入口函数 main 定义在 cmd/<repo>/main.go;
  │  它调用 cmd.Execute() 启动 cobra 命令树。
  │  - cmd/<repo>/main.go:8
  │  - cmd/root.go:42
  ...
```

每一行结论都跟着 `file:line`,可以点击或复制去验证。

## 1.6 退出

```
[git:main]❯❯ /exit
```

或按两次 `Ctrl+C`。

---

# 2. 基本用法

## 2.1 提问的三种姿势

| 场景 | 推荐姿势 | 命令 |
|---|---|---|
| 探索式连问 | **REPL** | `codrax`,然后逐条提问 |
| 一次性 / 脚本 | 单次 CLI | `codrax --request "你的问题"` 或 `codrax "你的问题"` |
| 重定向到文件 | 单次 + 重定向 | `codrax -r "..." > answer.md` |

REPL 可以多轮对话、附加日志、记住上下文;单次模式适合 CI 脚本、跑批、或你只问一次。两种模式答案内容一致,REPL 有进度动画 + 颜色,单次模式默认输出干净的纯文本(适合 pipe 到文件 / markdown viewer)。

**虚构示例**(任何项目都能套用,把 `Foo` / `bar.go` 替换成你的实际名字):

```bash
# 启动 REPL
codrax

# 一次性问问题
codrax --request "main 函数定义在哪个文件?"
codrax -r "Foo 这个函数被哪些地方调用?"
codrax -r "config.yaml 里 timeout 字段的默认值是多少?"

# 切英文
codrax -r "what does Foo do?" --lang en

# 把答案重定向到文件
codrax -r "项目目录布局是怎样的?" > overview.md

# 操作非当前目录的仓库
codrax --repo /path/to/repo --branch dev -r "..."
```

**REPL 里几种典型问法**(也是虚构例子):

```
[git:main]❯❯ Foo 在哪里定义?有多少个调用点?
[git:main]❯❯ pkg/auth 这个包做什么?
[git:main]❯❯ 帮我画一下 ParseRequest 的调用链
[git:main]❯❯ config.yaml 里 timeout 有默认值吗?在代码里哪儿读?
[git:main]❯❯ 这个仓库有哪些可执行入口?
```

每个问题答案都会带 `file:line` 引用,可以点击或复制去验证。

## 2.2 看懂界面

启动后的屏幕(运行中的写模式 + git 仓库 + 几轮对话场景):

```
   CODRAX  v0.1.x  /help · /exit
   模式: auto · code · operation · data · write · /home/you/.../codrax.yaml
   记忆: 3 recent + 0 compacted, 4.8 KB
   模型: your-model · ctx=200k · timeout=10m0s

─────────────────────────────────────
> 这个项目的入口函数在哪里?
· 1/4 正在统计仓库索引 `your-repo` 文件
✓ 1/4 仓库索引 `your-repo` 已就绪：123 个文件 (120ms)
⇢ 分析 · 第 1 轮 调用工具 emit_analysis intent=lookup
⇢ 探索 · 第 1 轮 调用工具 grep pattern=func main
• 证据 2 条（累计 2 条）
› 4/4 正在生成最终答案
…
✓ 已撰写最终答案

  │
  │  入口函数 main 定义在 cmd/<repo>/main.go;
  │  它调用 cmd.Execute() 启动 cobra 命令树。
  │  - cmd/<repo>/main.go:8
  │  - cmd/root.go:42
  ...
```

要点:

| 元素 | 含义 |
|---|---|
| `CODRAX  v0.1.x` | 版本 + 常用命令提示 |
| `模式/记忆/模型` 状态行 | 当前可用模式、记忆概况、模型配置和配置文件路径;英文界面会显示对应的本地化文案 |
| `─────…` 分隔线 | 每轮请求开始前的视觉断点(在你的回显之上) |
| `>` 开头(青色) | 你刚提交的请求的回显(保留多行 paste 内容) |
| `[git:main]`、`[task:code]`、`[task:op]`、`[task:data]`、`[task:write]`、`[phase:apply]`、`[focus:repo-a]`、`[log]`、`[log:2]`、`[trace]`、`[trace:2+perf]`、`[perftrace]`、`[tracebundle]`、`[plan]`、`[mem!]` | sticky 标签,提示当前粘滞状态:git 分支、显式任务模式、少数内部阶段、多仓 focus、附加日志/trace/perf/bundle 的数量与类型、待处理 plan、记忆压力 |
| `K/N <stage 标签>` | 当前 pipeline 进度。读模式通常是 1/4 分析、2/4 探索、3/4 提炼、4/4 成文;写模式会显示对应 plan/apply/verify 阶段 |
| `⇢ <阶段> · 第 N 轮 调用工具 ...` | 本轮模型发起的工具调用,会显示阶段、轮次、工具名和关键参数摘要 |
| `• 证据 N 条（累计 M 条）` | 当前阶段已经落地的证据数量 |
| `› 正在...` / `↻ 正在...` | 等待模型、修复、重试或补证据时的过程状态 |
| `· 第 N 轮 · M 次工具调用 · 本 Xs · 总 Ys` | 该阶段汇总:轮数 / 工具调用次数 / 当轮 / 总耗时 |
| `│` 边框 | 围出最终答案,和过程性输出做视觉分割 |
| `chat ·` / `local ·` / `clarify ·`(灰色) | 这一轮没走完整流水线;后跟简短说明(详见 3.3) |

## 2.3 多轮对话与"接着上轮换格式"

REPL 自动记住最近若干轮的对话。多轮典型用法:

```
[git:main]❯❯ ParseConfig 怎么解析配置文件?
✓ 已撰写最终答案
  │ 1. 解析 ... 2. 聚合 ... 3. 渲染 ...

[git:main]❯❯ 把上面换成 mermaid 流程图
  local · derived from previous answer, no repo read     ← 灰色单行徽标
  │ ```mermaid
  │ flowchart LR
  │   parse --> aggregate --> render
  │ ```

[git:main]❯❯ 重新读一下仓库,确认有没有 IO 分析这一步
✓ 已撰写最终答案
  │ ...
```

codrax 会自动判断你的请求需不需要重读仓库:

| 你写的 | codrax 判定 | 行为 |
|---|---|---|
| 涉及代码细节("为什么...""哪里调用...") | repo | 走完整流水线 |
| 引用上一轮答案的格式转换("换成表格""总结一下""翻译成英文") | local | 复用上一轮答案,~10s |
| 既要新证据又要换形式("重读 + 换 mermaid") | hybrid | 重读 + 按指定格式输出 |
| 没有上一轮答案就说"换成 X" | clarify | 提示你直接描述要问什么,不编造 |
| "你好""你能干什么" | local(chat) | 闲聊回复 |

无需手动切换 — 这是基于 LLM 的结构化路由,也就是 `chitchat_classifier_enabled`(默认开)。如果你想看每轮路由判定,加 `--log-stdout --log-level debug`,会看到这样的行:

```
DEBUG [repl/turn_policy] route=local operation=transform needs_repo=false confidence=0.92 ...
```

## 2.4 中断 / 取消

| 操作 | 效果 |
|---|---|
| 第一次 `Ctrl+C`(REPL 运行中) | 取消当前 Run;在下一个流水线检查点生效(可能需等 ~30 秒 LLM 回包) |
| 2 秒内第二次 `Ctrl+C` | 强制退出 codrax,清理 worktree |
| `Ctrl+C` 在空闲提示符 | 优雅退出 + 清理 worktree |
| `/exit` 或 `/quit` | 优雅退出 |
| 管道/脚本模式发 `/cancel` 一行 | 取消当前 Run |

> 写模式 apply / verify 阶段中,即使中断,worktree 也会保留(可用 `/worktree list` 检查)。

---

# 3. 进阶能力

## 3.1 附加运行时日志(panic / 异常 / traceback)

如果你手上有一段 panic / 异常 / 编译错误日志,把它**作为附件**喂给 codrax,它会触发 `log_triage` 预阶段先抽出 stack frame、错误类型、可疑文件路径,再让主流水线只读真正相关的文件 — 比把日志贴进问题文本里准得多,也快得多。

四种附加方式:

```bash
# (A) 文件
codrax --log /tmp/panic.txt --request "这个 panic 哪来的"

# (B) 标准输入
kubectl logs pod/foo | codrax --log - --request "排查这个 crash"

# (C) 多个文件 — codrax 自动加边界头
codrax --log first.log --log second.log -r "比对一下两次 crash"

# (D) 内联文本(脚本里方便)
codrax --log-text "$(cat panic.txt)" -r "..."
```

REPL 里的等价做法:

```
[git:main]❯❯ /log /tmp/panic.txt
  ✓ 已附加日志:1234 字节
[git:main][log]❯❯ 这个 panic 哪来的
```

`[log]` sticky 标签会一直在,直到你主动 `/log clear`。

`/log` 子命令:

| 子命令 | 作用 |
|---|---|
| `/log <path>` | 加载文件作为附加日志(覆盖之前的) |
| `/log append <path>` | 追加另一段(自动加 `# codrax-source:` 边界头) |
| `/log show` | 打印附加日志的前 20 行 + 总字节 |
| `/log clear` | 清除附加日志 |
| `/log`(无参) | 进粘贴模式,贴完用 `/end` 结束(SSH/tmux 吞掉 bracketed paste 时用) |

附加日志 size 上限:512 MiB(`log_attach_max_bytes`),超过自动尾部截断 + 警告。

**自动检测**:如果你在普通问题里直接粘贴包含 panic/stack-frame 行的文本,REPL 会**一次性**自动把它转成附加日志(打印一行 `auto-attached log: N bytes`),只对当前这轮生效,不影响下轮。如果想阻止自动,先 `/log clear` 把粘性 log 占位即可。

**C/C++ 编译路径前缀**:CI build 出来的 stack frame 路径常是 `/build/src/foo.cpp:42`,你的 repo 是 `~/repo/foo.cpp:42`。用 `--log-source-prefix /build/src/` 让 codrax 把前缀剥掉再去仓库找文件。

## 3.2 附加性能 trace(HiTrace / atrace / systrace / perfetto / perf sample)

性能问题、卡顿、ANR、冷启动慢 — 把 ftrace 兼容的文本 trace 作为附件喂给 codrax,触发 `perf_triage` 预阶段抽出 frame / jank / stall / startup 信息:

```bash
# HiTrace(HarmonyOS / OpenHarmony)
hdc shell hitrace -t 5 graphic > /tmp/htrace.txt
codrax --htrace /tmp/htrace.txt -r "首页冷启动为什么慢?"

# atrace(Android)
adb shell atrace -t 5 -o /tmp/atrace.txt
codrax --atrace /tmp/atrace.txt -r "ListView 滑动卡顿哪里出问题?"

# systrace / perfetto 文本导出
codrax --htrace /tmp/perfetto.txt -r "..."

# 二进制 HiTrace 需要先手动转换;不会自动附加
# 纯 trace 默认 auto:有 trace_streamer 就走 SQL;没有 trace_streamer 才走内置
# 如果 SQL 已经开始执行但失败,不会再回退到内置;trace+perf 固定 SQL-only
codrax trace convert --trace-tools-status
codrax trace convert --input /tmp/capture.htrace.bin
codrax trace convert --input /tmp/donghu.sys --trace-engine=builtin
codrax --htrace /tmp/capture.htrace.bin.systrace -r "分析这段卡顿"

# 先看本机 perf.data 解析能力和缺什么官方工具
codrax trace convert --perf-tools-status

# 多文件比对
codrax --htrace before.trace --htrace after.trace -r "对比启动耗时差在哪"

# 标准输入
cat /tmp/atrace.txt | codrax --htrace - -r "..."
```

REPL 里 `/htrace` 和 `/atrace` 是同义命令,子命令同 `/log`:

```
[git:main]❯❯ /htrace /tmp/htrace.txt
  ✓ 已附加 trace
[git:main][trace]❯❯ 首页冷启动哪里耗时最长?
```

REPL 内也可以先看 trace 转换工具和 sys parity gate 状态:

```text
[git:main]❯❯ /htrace tools-status
  · trace 解析引擎：auto
  · trace_gate[sys_binary_parity_gate/no_perf_sys_binary_parity]：状态=等待代表性fixture ...
```

二进制 Harmony/OpenHarmony HiTrace 使用 `/htrace convert` 手动转成文本:

```text
[git:main]❯❯ /htrace convert /tmp/capture.htrace.bin
  ✓ converted hitrace: /tmp/capture.htrace.bin.systrace (N events)
  · next: /htrace /tmp/capture.htrace.bin.systrace
```

如果没有指定输出文件,默认写到 `<原文件名>.systrace`。如果目标文件已存在,codrax 会拒绝覆盖,提示先删除旧文件或重新指定输出路径。纯 trace 有两个转换引擎,但一次只会运行一个:`auto` 模式下如果发现 `trace_streamer` 就走 SQL;如果未发现 `trace_streamer` 才使用 Codrax 内置纯 trace 转换器;如果 SQL 已经开始执行但失败或没有产出 query-ready 行,不会再悄悄回退到内置转换。显式 `--trace-engine=trace_streamer` 要求 SQL 工具可用,显式 `--trace-engine=builtin` 只用于纯 trace。trace+perf htrace 固定走 trace_streamer/SQL,不会使用内置 trace body 解析。转换命令不会默认附加到当前会话;需要继续分析时,按下面两种方式之一把转换产物交给分析流程。

### 转换后如何分析 trace + perf 混合文件

OpenHarmony / HarmonyOS 的 HiProfiler 文件可能同时带 ftrace/bytrace 文本、`hiperf-plugin` 的 standalone `perf.data`,以及转换器生成的 `.perftrace`。这种场景优先使用 `.tracebundle.json`,因为它会把 systrace、perftrace、raw perf.data provenance 和 converter caveats 作为一组产物交给 `trace_query`。

推荐 attach 方式:

```bash
# CLI: 一次性附加 bundle
codrax --htrace /tmp/hiprofiler_data.htrace.tracebundle.json \
  -r "分析 com.example 主线程在 34579.47s 到 34579.59s 的卡顿原因,结合 runnable、D-state/IO、binder 和 perf 调用栈"

# 如果没有 bundle,也可以显式附加 systrace + perftrace pair
codrax --htrace /tmp/hiprofiler_data.htrace.systrace \
  --htrace /tmp/hiprofiler_data.htrace.perftrace \
  -r "分析这段卡顿"
```

REPL 里推荐附加 bundle,适合连续追问:

```text
[git:main]❯❯ /htrace /tmp/hiprofiler_data.htrace.tracebundle.json
  ✓ 已附加 tracebundle
[git:main][tracebundle][perftrace]❯❯ 分析 34579.47s 到 34579.59s 的卡顿原因
```

如果需要分别附加文件,先附 systrace,再 append perftrace:

```text
[git:main]❯❯ /htrace /tmp/hiprofiler_data.htrace.systrace
[git:main][trace]❯❯ /htrace append /tmp/hiprofiler_data.htrace.perftrace
[git:main][trace:2+perf]❯❯ 分析这一帧为什么丢帧
```

也可以不 attach,直接在问题里点名一个或多个路径。路径可以是绝对路径或相对当前启动目录的相对路径;文件名不要求一定有熟悉后缀,但问题里要表达清楚你是在分析这些 trace/log/perf 产物以及希望看的时间窗、线程、进程或现象:

```text
分析 /tmp/hiprofiler_data.htrace.tracebundle.json 里 com.baidu.tieba-59566
在 34579.47s 到 34579.59s 的卡顿原因,继续拆唤醒链,并结合 perf 样本看 running 线程在跑什么。

对比 ./before.tracebundle.json 和 ./after.tracebundle.json,
看同一帧窗口内 runnable、D-state/IO、binder、频点和 perf 热点有什么变化。
```

attach 和直接点名路径的差别:

- attach 适合 REPL 多轮追问;artifact 会成为当前会话的粘性 trace 上下文,提示符也会显示 `[trace]` / `[perftrace]` / `[tracebundle]`。
- 直接点名路径适合一次性问题或脚本化调用;Codrax 会根据用户请求、路径和文件内容进入 runtime trace/perf 分析,不需要先执行 `/htrace`。
- 对 trace + perf 混合文件,优先点名或附加 `.tracebundle.json`;只给 `.perf.data` 时通常还需要先 `trace convert` 生成 `.perftrace`,否则只能保留 raw perf provenance,不能做完整 CPU sample 聚合。

### perf.data / perf sample

如果 Harmony/OpenHarmony HiTrace 里包含 `hiperf-plugin` 的 standalone `perf.data`,或者你手上直接有 Android/simpleperf 的 `perf.data`,可以让 `trace convert` 生成 `.perftrace` 和 `.tracebundle.json`。后续 `trace_query` 会把 `.systrace + .perftrace` 合并成同一个时间窗证据流,用于回答“这个 runnable/running 线程当时在跑什么符号/调用栈”。

先做一次 preflight:

```bash
codrax trace convert --perf-tools-status
codrax trace convert --perf-tools-status --lang en
```

输出会列出:

- `official_harmony[openharmony_hiperf]`: 是否找到 OpenHarmony `hiperf_host` / `hiperf`
- `official_android[android_simpleperf_report_sample]`: 是否找到 Android simpleperf `report_sample.py`
- `raw_fallback[codrax_raw_perfdata]`: Codrax 内置 raw `perf.data` fallback 是否可用
- `perf_parser`: 当前策略,默认 `auto`
- `symbolization_expectation`: 输出是否可能是官方符号化结果、raw fallback 保存符号名,还是 raw IP/DSO fallback
- `check`: 用于确认该 provider 是否能在当前机器工作的检查命令
- `aux_check`: 符号目录、symfs、kallsyms 等辅助输入的检查提示
- `install` / `docs`: 官方工具获取入口或内置能力说明

推荐策略:

```bash
# 默认:优先官方工具;官方缺失/失败时尝试 raw perf.data fallback
codrax trace convert --input /tmp/capture.htrace
codrax trace convert --input /tmp/perf.data

# 只信官方工具,不走 raw fallback
codrax trace convert --input /tmp/perf.data --perf-parser=official

# 离线保底:只用 Codrax raw perf.data fallback
codrax trace convert --input /tmp/perf.data --perf-parser=raw

# 完全不生成 perftrace,只保留 perf.data sidecar
codrax trace convert --input /tmp/capture.htrace --no-perftrace
```

OpenHarmony / HarmonyOS 官方工具接入:

```bash
# 方式 1:显式指定
codrax trace convert --input /tmp/capture.htrace \
  --hiperf-host /path/to/hiperf_host \
  --hiperf-symbol-dir /path/to/symbols

# 方式 2:环境变量
export CODRAX_HIPERF_HOST=/path/to/hiperf_host
codrax trace convert --input /tmp/capture.htrace
```

`hiperf_host` / `hiperf` 来自 OpenHarmony `developtools_hiperf`。官方 lane 会运行 `hiperf report --proto`,再把 protobuf report 转成 Codrax `.perftrace`。OpenHarmony report proto 通常不携带 CPU id,所以 `.perftrace` 中可能显示 `cpu=-1`;这表示 CPU 未知,不能归因到任何具体 CPU/core。

Android simpleperf 官方工具接入:

```bash
# 方式 1:显式指定 simpleperf 脚本
codrax trace convert --input /tmp/perf.data \
  --simpleperf-report-sample /path/to/simpleperf/scripts/report_sample.py \
  --simpleperf-python /usr/bin/python3 \
  --simpleperf-symfs /path/to/symfs \
  --simpleperf-kallsyms /path/to/kallsyms

# 方式 2:环境变量
export CODRAX_SIMPLEPERF_REPORT_SAMPLE=/path/to/report_sample.py
export CODRAX_SIMPLEPERF_PYTHON=/usr/bin/python3
codrax trace convert --input /tmp/perf.data
```

`report_sample.py` 是 Android 官方提供的 `simpleperf_report_lib.py` wrapper。Codrax 执行 wrapper;如果你只传了 `simpleperf_report_lib.py`,系统会尝试使用同目录的 `report_sample.py` / `simpleperf_report_sample.py` / `report_sample`,缺失时 `--perf-tools-status` 会提示补 wrapper。官方工具负责完整解析、unwind、Java/ART/native 符号、symfs/kallsyms 等;Codrax raw fallback 只做有限字段提取。raw 输出会明确带上 `source=raw_perfdata_fallback`;如果 perf.data 里带 OpenHarmony hiperf 官方 `HIPERF_FILES_SYMBOL` feature,会直接使用其中保存的函数名(包括 ArkTS/HAP/JSVM 这类采集时已解析的符号字符串),否则仍按 IP/DSO/调用链地址做保底关联。raw fallback 也会保留官方 `EVENT_DESC` 事件名、采集命令/时间/设备/CPU/内存元信息,并在 `HIPERF_CPU_OFF + sched:sched_switch` 命中时标记 `sample_kind=off_cpu`;如果只看到 `HIPERF_FILES_UNISTACK_TABLE` 摘要,说明去重栈表存在但未展开,完整调用栈仍应优先使用官方 `hiperf report`。看到 `symbolization_status=unsymbolized` 或 `callchain_status=ip_only` 时,不要把 IP-only label 当成真实函数名。

转换产物:

- `.systrace`: ftrace/systrace 文本
- `.perf.data`: 从 OpenHarmony profiler 容器抽出的 perf sidecar
- `.perftrace`: Codrax 统一的 perf sample 文本格式
- `.tracebundle.json`: systrace/perftrace/perf.data 的 bundle 元数据

分析时可以直接传 `.tracebundle.json`、`.systrace` 或 `.perftrace`;如果同目录存在 sibling bundle 或 sibling `.systrace + .perftrace`,trace_query 会自动合并。

如果本轮保存 markdown/html 报告,报告正文会额外包含 `Runtime Artifacts` 表,列出本轮附加的 log/trace/perf/bundle 来源、大小和关键信息。只附加 `.tracebundle.json` 时,报告会展开 bundle 里的 systrace/perftrace/perf.data 成员、converter 和 caveats。raw fallback 产生的 perf 样本会在表里保留 `raw_perfdata_fallback`、`symbolized` / `unsymbolized`、`ip_only`、`event_name_source=event_desc`、`hiperf_cpu_off=true`、`hiperf_unistack_*` 这类标记,方便区分“官方/保存符号名可读调用栈”“off-CPU 事件样本”和“IP/DSO 级保底关联”。

常用提问模板:

```text
分析 <trace文件> 中 <线程名/pid> 在 <起始时间> 到 <结束时间> 之间的卡顿原因,
继续追踪 sched_wakeup 唤醒链,并结合 CPU、IO、binder、频点、IRQ、内存事件判断根因。
```

时间可以自然写。`2942.244845`、`2942.24484`、`2942.24484s`、`2942.24484 秒` 都会按 trace 秒解析;如果明确写 `ms` / `毫秒` 或 `us` / `微秒`,codrax 会换算成秒再分析。小数位少一两位时,系统会加很小的边界容差,避免刚好落在窗口边缘的 trace 行被漏掉。

客户不需要学习内部工具名。描述线程、时间窗口和要追踪的链路即可;codrax 会优先使用确定性的 trace 查询能力,必要时再用 grep / read_file 验证具体行号。

trace 的 size 上限独立于 log:`trace_attach_max_bytes`(默认 512 MiB)。

> `--log` 和 `--htrace` 是**两个独立的通道**,可以同时附:一份 panic + 一份 trace 同时给 codrax,两个 pre-stage 各自处理。

## 3.3 闲聊与本地转换

REPL 默认开了 `chitchat_classifier_enabled`,所以不需要手动 `/chat`,大部分场景 codrax 自动判路由(见 2.3)。

显式 `/chat` 在以下场景有用:

```
[git:main]❯❯ /chat 你能干什么?
  chat · 闲聊回复,未读仓库,未生成 plan         ← 灰色单行徽标
  │ 我是 CODRAX,...
```

`/chat` 路径**不读仓库、不调工具**,只复用对话记忆 + LLM 直接生成。

**本地转换徽标 `local ·`**:当你说"换成 mermaid""换成表格""总结一下"时,屏幕顶部会出现一条灰色单行徽标。徽标有两种文案,根据回答的来源切换:

| `policy.Source` | 徽标文案(zh) | 徽标文案(en) |
|---|---|---|
| `last_answer` | `local · 复用上一轮答案,未读仓库` | `local · derived from previous answer, no repo read` |
| 其他(包括 `current_message`) | `local · 未读仓库,纯模型生成` | `local · no repo read, pure model output` |

**澄清徽标 `clarify ·`**:第一轮就说"换成 mermaid"(没有上一轮答案可换),不会编造,而是打:

```
  clarify · 没有上一轮答案可复用 — 请直接描述你想问什么
```

## 3.4 记忆与会话

REPL 把每轮对话存到 `<CWD>/.codrax/memory/<repo>-<hash>/`,新一轮开始前会从中检索相关旧轮作为 context。

| 命令 | 作用 |
|---|---|
| `/history` | 看最近若干轮 |
| `/compact` | LLM 压缩老回合,腾出 buffer 空间 |
| `/clear` | 二次确认后清空所有 memory |

启动时 banner 会显示 `memory · 5 turns / 12 索引`,提示档位:`[mem!]` sticky 标签出现说明 buffer 偏满,建议 `/compact`。

memory 是**按仓库**隔离的:同一个 codrax 进程切到另一个仓库,看到的是另一份 memory,不混。

## 3.5 一台机器多仓库

`.codrax/` 目录默认按"当前工作目录的仓库 basename + FNV hash"切分子目录。在 repoA 跑出来的日志、memory、worktree 不会和 repoB 互相污染:

```
~/repoA/.codrax/logs/codrax-abc12345/   ← repoA 的日志
~/repoA/.codrax/memory/codrax-abc12345/ ← repoA 的对话记忆
~/repoB/.codrax/logs/codrax-def67890/
~/repoB/.codrax/memory/codrax-def67890/
```

所有跨项目的个人级状态(repomap 索引缓存、env-cache.json、未来的其它持久化)都放在 `~/.codrax/cache/`(Windows:`%USERPROFILE%\.codrax\cache\`)。每个仓在 cache 下有独立子目录,与各仓的 `.codrax/`(项目级状态)互不干扰。备份 / 清理只需关注这两个位置。

## 3.6 跨仓 workspace(multi-repo discovery)

> **场景**:父目录下有 N 个独立 git 仓共存(典型:`mono/{frontend,backend,sdk}/<repo>` / mobile-app + backend-service + shared-protos / 多语言异构子项目)。codrax 自动发现全部子仓,跨仓 typed lane 查询(symbol existence / implementers / drift detection)正确,内存可控,**单仓用户零影响**。

### 何时受益

| 你的场景 | 是否受益 |
|---|---|
| 单仓:`cd ~/myrepo && codrax` | 🟢 行为字节级不变(默认走 `parent/.git` fast path,~50µs 额外开销) |
| 多仓:`cd ~/workspace && codrax`(workspace 含多个独立 git 仓) | 🟢 自动发现,跨仓 entity 查询正确 |
| Monorepo(`go.work` / `Cargo.toml [workspace]` / pnpm workspace) | 🟡 这是**单仓内多 module**,本特性不处理 — 直接 `cd` 进 monorepo 根用单仓模式即可 |
| 写模式跨多个独立子仓 | 🔴 **禁止** — `task.scope=micro` 强制单子仓;ChangePlan 跨子仓 fail-loud,需要按子仓拆 Run |

### 默认行为(零配置)

无需任何配置,启动时自动:

1. 探测父目录是否本身是 git 仓 → 是 → **单仓 fast path**(立即返回,~50µs)
2. 否 → BFS 父目录(默认深 4 层),每发现 `.git/` 即创建 `SubRepo` 条目,**剪枝不再深入子仓**
3. 探测每个子仓的 top-3 主要语言(Go / Python / Rust / TypeScript / ArkTS / ...)+ 文件数 + manifest 指纹
4. 持久化拓扑到 `<runtime-anchor>/cache/topology/<parent-slug>.json`(下次启动直接复用,除非父目录有新改动)

首次提问时,如果你没有通过 `/repos focus` 或 `--focus` 明确指定子仓,codrax 会先把**紧凑拓扑摘要**交给模型做一次 focus 选择,再只构建被选中的 active 子仓索引。这个选择是结构化输出,不是从散文里猜;如果选择失败或置信度过低,才降级为 fallback preview。这样通常比按文件数默认挑 2 个子仓更准确,也避免一开始就扫描错误的大仓。

如果你已经明确指定 focus,系统会严格遵守你的选择,不会自动补其它子仓。显式 focus 数超过当前 cap 会直接提示你先减少 focus 或提高 `/repos cap`。

REPL banner 立即提示:

```
   CODRAX  v0.1.X  /help · /exit
   模式: auto · code · operation · data · write
   记忆: 3 recent + 0 compacted
   🗂  multi-repo: 5 sub-repos (active cap=2); not cross-repo? use --multi-repo=false; /repos focus
```

这行在终端里会用更醒目的颜色显示,因为 multi-repo 会改变路由范围并让 active 子仓图常驻内存。看到这个 Banner 但本次只是单仓任务时,优先重启并传 `--multi-repo=false`;如果确实要跨仓,再用 `/repos focus` 或 `--focus` 明确 pin 目标子仓。

### `/repos` 命令族

启动后任何时候可用:

| 命令 | 作用 |
|---|---|
| `/repos` | 列出已发现子仓 + active 状态 + cap + focus pin |
| `/repos focus <slug>` | 严格把子仓固定到 active 集合；显式 focus 后不会自动补其它子仓 |
| `/repos unfocus [slug]` | 释放固定(无参数 = 全释放) |
| `/repos refresh` | 强制重新探测父目录(子仓增删后用) |
| `/repos cap <N>` | 会话级覆盖 active cap(yaml 默认 2,硬上限 5 — 设更高自动 clamp) |

`/repos` 输出长这样:

```
multi-repo topology — parent=/home/user/workspace slug=workspace-1a2b3c4d sub-repos=5 cap=2
    api-go                          slug=api-go-aabbccdd      git=dir files=240   langs=go            tier=2
  * web-frontend                    slug=web-frontend-eeff0011 git=dir files=890   langs=typescript    tier=2
    mobile-arkts                    slug=mobile-arkts-2233aabb git=dir files=156   langs=arkts         tier=2
    shared-protos                   slug=shared-protos-cc445566 git=dir files=42    langs=proto         tier=1
    legacy-cangjie                  slug=legacy-cangjie-77889900 git=dir files=68    langs=cangjie       tier=1
  pinned (focus): web-frontend-eeff0011
```

`*` 标的是 focus 固定。`tier=2` 表示该子仓的 PrimaryLangs 已经从首次 graph build 校准过(更准);`tier=1` 是基于扩展名启发式的初步估计。

### 跨仓提问的最佳实践

#### ✅ 单子仓问题(80% 场景)— 不需要任何额外操作

```
❯❯ api-go 怎么处理 OAuth 回调?
```

routing fold 通过你提到的子仓名(channel B)+ 关键词语言匹配(channel D — `.go` 偏向 api-go)自动选中目标子仓,answer 正确。

#### ✅ 跨子仓问题(15% 场景)— 用 `/repos focus` 多 pin

```
❯❯ /repos focus api-go-aabbccdd
❯❯ /repos focus web-frontend-eeff0011
❯❯ api-go 暴露的 /v1/user 接口在 web-frontend 哪些组件被消费?
```

两个子仓都 active 后,跨仓 grep + entity 查询自然命中。Hallucination gate 不会误拦真实存在的跨仓 entity。

#### ✅ "全 workspace 范围"问题 — 调高 cap

```
❯❯ /repos cap 5
❯❯ workspace 里所有 *Test* 文件都用了什么 mock 框架?
```

让全部 5 个子仓 active,跨仓 keyword search 自动 aggregate ranking。

#### 🟡 写模式 — 必须 cd 进具体子仓

写模式(`/mode write` / `/write` / `--mode=write`)**禁止**跨子仓 ChangePlan。当前在父目录运行写模式时,plan 阶段如果 LLM emit 的 ChangePlan 触及多个子仓,会 fail-loud:

```
✗ write blocked: ChangePlan touches 2 sub-repos: api-go, web-frontend — multi-repo write banned by design
  Repair: split into 2 separate runs, one per sub-repo (cd into each sub-repo and re-issue the request,
          OR `/repos focus <slug>` then re-run with multi_repo_max_active=1)
```

正确做法:

```bash
cd ~/workspace/api-go && codrax --mode=write --request "..."
cd ~/workspace/web-frontend && codrax --mode=write --request "..."
```

#### 🔴 用户面板的 disclosure(R3 透明)

如果 routing fold 因 cap 限制把某些子仓挡在 active 之外,answer 会自动声明"未参与本次查询的子仓"。Run 退出时 telemetry 行也会列出:

```
INFO multigraph: typed-lane partial — sub-repos NOT consulted: [legacy-cangjie shared-protos]
                 (raise cap or `/repos focus <slug>` to include)
```

这是**精确信号**(R3 红线)— `partial_typed_lane` 是 typed enum boolean,不是噪声。看到这条 log 就知道答案可能漏掉了某些子仓,可决定是 `/repos focus` 后重跑还是接受当前覆盖。

### yaml 配置(`codrax.yaml`)

5 个开关,**默认值已经合理**,通常不需要改:

```yaml
multi_repo_enabled: true                    # 默认 true,false 走 legacy 单图 (绕过本特性)
multi_repo_max_active: 2                    # LRU 上限,默认 2,硬上限 5 (yaml > 5 自动 clamp)
multi_repo_inactive_preview_count: 2        # L0 prompt advisory 给 LLM 看几个 out-of-active 仓 (默认 2,硬上限 3)
multi_repo_discovery_depth: 4               # 父目录 BFS 深度
multi_repo_min_files: 1                     # 子仓 file count 下限,过滤空 .git fixture
```

**何时调整 cap**:
- 默认 2 覆盖典型跨仓场景(一个主仓 + 一个协作仓);3 子仓以上的跨仓调查较少
- 跨仓问题多 + 子仓总数 5-10 → 可按需调到 3-5(硬上限 5,yaml 设更高也强制 5)
- LRU thrashing 警告(`multigraph: thrashing detected (>5 evictions/60s)`)出现时 → 加 cap 或 `/repos focus`

### `--focus` CLI flag

在非 REPL / 脚本化 / eval 调用里,用 `--focus` 在启动时预 pin 子仓(等价启动后立刻跑 `/repos focus`):

```bash
# 单仓 pin
codrax --repo ~/workspace --focus repo-greet-go --request "..."

# 多 pin(repeatable 或逗号分隔)
codrax --repo ~/workspace --focus repo-go --focus repo-py --request "..."
codrax --repo ~/workspace --focus repo-go,repo-py --request "..."
```

每个值是子仓 **slug 或 RootRel 路径**,通过 `topology.Resolve` 解析,任一形态都可以。匹配不到的 token 会 Warning 提示并丢弃,不阻断 Run。**单仓 / 无 git workspace 静默忽略此 flag**(无 sub-repo 可匹配)。

### `--multi-repo` CLI flag

per-Run 覆盖 `codrax.yaml :: multi_repo_enabled`,无需改 yaml:

```bash
# 临时启用(yaml 关 / 默认覆盖)
codrax --repo ~/workspace --multi-repo=true --request "..."

# 临时关闭 — 父目录扫到一堆子仓但本次只想用单仓 legacy 路径
codrax --repo ~/workspace --multi-repo=false --request "..."
```

只有显式传 `--multi-repo=true` 或 `--multi-repo=false` 才会覆盖;省略 flag 时 yaml(或 yaml 缺省值 true)继续生效。生效会在启动 INFO 日志输出 `[multi-repo] CLI override: enabled=...`,可直观确认。启动 Banner 已提示 multi-repo、但问题不是跨仓比较/跨仓引用/全 workspace 汇总时,建议优先用 `--multi-repo=false`,避免无意进入多仓 active-set 路由。

`multi_repo_enabled=false` 时 `/repos focus|unfocus|refresh|cap` 会拒绝执行并提示同时给出 yaml 与 `--multi-repo=true` 两条启用方式;`/repos`(不带子命令)的列表仍可查阅,只是不会路由。

### 内存与性能预算

| 项 | 单仓 | cap=2 默认多仓 | 备注 |
|---|---|---|---|
| 启动开销 | ~50µs | warm cache 通常 ms 级; cold discovery 取决于子仓数 | BFS + 有界并行 per-sub-repo `git ls-files`(默认 4 路) |
| Active 内存 | ~100 MB / 万文件 | ~200 MB | 与 cap 线性;调到硬上限 5 时线性增加 |
| 拓扑 cache 磁盘 | 0 | <100 KB | 100 子仓也只 1 MB |
| 跨仓 typed lane 查询 | n/a | O(active 子仓数) | LRU 命中 → ms 级,miss → 子仓全量 build |

100 子仓 × 1 万文件场景:active 默认仍只 hold 2 个；即使调到硬上限 5,也只缓存被选中的 active 子仓。

如果只是从父目录误启动了单仓任务,不要通过调低 cap 来“勉强单仓化”;直接用 `--multi-repo=false` 跳过多仓路由,这是最稳定、内存占用最低的路径。

### 故障排查

| 症状 | 原因 | 修复 |
|---|---|---|
| answer 漏掉某子仓的 entity | 子仓未在 active 集 | `/repos focus <slug>` 然后重跑(REPL)或 `--focus <slug>` 启动(scripted)|
| `partial_typed_lane=true` 出现频繁 | cap 太低 | yaml `multi_repo_max_active: 3` 到 `5` 或 `/repos cap 3` 到 `5`(硬上限 5;默认 2)|
| `thrashing detected` Warning | 同上,LRU 抖动 | 同上 |
| 写模式跨仓 fail-loud | 设计限制 | cd 进具体子仓重跑 |
| 没看到 banner 多仓行 | 父目录是单 git 仓(不是 workspace) | 这是预期 — 单仓 quiet UX |
| 看到了醒目的 multi-repo Banner,但本次不是跨仓任务 | 从父目录启动且发现了多个独立子仓 | 重启并传 `--multi-repo=false`,或 cd 到目标子仓再运行 |
| banner 前等待较久 | 多仓 cold discovery 正在找子仓/统计 metadata | 2s 后会显示“正在发现工作区子仓拓扑”;后续 warm cache 不会被 `.codrax` 日志/缓存 mtime 误判失效 |
| `/repos` 列出空 / 漏子仓 | BFS 深度不够 / 子仓 file count < min | yaml `multi_repo_discovery_depth: 6` 或 `multi_repo_min_files: 0` |

### 完全关闭(回到单图)

```yaml
# codrax.yaml
multi_repo_enabled: false
```

这时:
- topology discovery 仍跑(REPL `/repos` 命令展示用);单仓是 ~50µs fast path,多仓父目录 cold discovery 会显示启动进度并走拓扑缓存
- BusContext.MultiGraph 不接入 → 5 个 BuildOrLoadGraph caller 全走 legacy `BuildOrLoadGraph(parent_root, query)`(等同 ship 之前行为)
- ArkTS leak 修复(Phase 0)依然生效(它绕过 MultiGraph 在 `IsArkTSProject` 内修)

适用场景:**多仓 + workflow 几乎不跨仓 + 单图行为已熟悉**。但默认 true 已经足够安全(单仓零开销),通常不必关。

---

## 3.7 数据处理任务

数据处理任务用于**只读地处理本地结构化或半结构化材料**，例如表格、记录集、清单、JSON/JSONL、文本化附件索引、抽取后的文档文本等。典型目标包括清洗、过滤、去重、join、汇总、排序、条目级判定、数值字段计算，以及“只输出 JSON / CSV / 单行字符串 / Markdown 表格”等严格格式结果。

这条路径和源码分析、trace/log 根因分析、电脑操作是分开的：

- 不把数据文件当成当前源码实现文件，不走源码 citation gate。
- 纯只读数据计算不需要 command-operation 审批。
- 系统先给模型客观的材料目录：路径、类型、大小、表格 header、行数、少量样例等；不会把某个文件硬判成“规则”“证据”或某个业务角色。
- 需要抽取的非文本材料会作为客观材料进入材料目录；如果模型判断它们是当前任务的必需材料，系统会优先使用已有文本证据，或在配置了 `agents.multimodal_material_extractor` 时调用抽取器生成文本证据，再回到数据处理流程。
- 模型根据当前用户目标声明需要覆盖哪些材料；系统只校验声明的必需材料是否真的被脚本读取、是否需要通用决策记录、最终输出是否符合用户要求。
- 对求和/计数、过滤清洗、多材料关联、名称/编号/类别归一等任务，模型会按任务需要生成规则覆盖、贡献记录、实体归一和对账报告；系统只校验这些通用结构是否完整和可对账，不把某个文件硬判成固定业务角色。
- 模型负责理解规则和生成计算计划，系统用受限的确定性 runner 读取候选数据文件并产出结果、材料消费记录、条目级决策记录、贡献/对账摘要和输出契约校验。
- 如果用户同时要求“算结果并检查代码实现”，才会进入数据 + 源码的混合流程。
- 数据处理会在 `.codrax/data-audit/` 写入计划、产物、错误、终态审计和 checkpoint。中断后只有在 CLI 显式提供 `--data-resume <checkpoint.json>` 时才会恢复；系统不会自动续跑旧任务。

示例：

```text
根据当前目录下的 CSV/JSONL，把同一分组键的数值字段汇总，只输出 JSON。

读取这些文本材料和记录集，按规则筛出符合条件的记录，最终只输出逗号分隔的记录 ID。

把 records.tsv 和 reference.jsonl 按共同键关联，列出字段不一致的前 20 条，输出 Markdown 表格。
```

如果数据源不明确，codrax 会先请求补充路径或规则；如果用户明确要求分析源码、日志或 trace，则仍走对应的源码/外部观察管线。

材料抽取器是可选配置，适合“材料本身不是可直接读取文本，但当前任务确实需要其内容”的本地数据任务：

```yaml
llm:
  agents:
    multimodal_material_extractor:
      provider: your-vision-provider
      model: MiniMax-M3
```

没有配置时，系统不会假装已经读懂非文本材料；如果这些材料被声明为必需材料，会提示需要文本证据或配置抽取器。

## 3.8 外部工具与外部 Skills

Codrax 有两类外部能力入口:

| 入口 | 适合接什么 | 进入哪条通道 |
|---|---|---|
| MCP 外部工具 | 内部知识库、告警系统、缺陷平台、监控快照、trace 预分析服务、规范文档检索器等**只读外部事实来源** | `mcp_resource` 外部观测通道 |
| 外部 Skills / Operation Skills | 本地脚本、二进制、公司内工具、PPT/文档/表格生成器、浏览器/桌面自动化等**可能有副作用的电脑操作或制品生成能力** | operation provider / operation handoff 通道 |

两类能力都不会伪装成当前源码的 `file:line` citation。MCP 更像“给探索阶段增加外部证据来源”;外部 Skills 更像“按 operation 策略执行一个外部工作流或生成制品”:低/中风险安全批次可自动推进,高风险需要审批,特别危险的操作会被拒绝。

### 先选入口

| 你要做什么 | 推荐入口 | 原因 |
|---|---|---|
| 查公司内知识库、工单、告警、监控、trace 预分析结果 | MCP 外部工具 | 结果是只读外部事实,适合进入外部观测通道并和源码/trace 证据一起分析 |
| 调本地脚本、公司内二进制、PPT/文档/表格生成器、浏览器或桌面自动化 | 外部 Skills / Operation Skills | 这类能力可能有副作用或产物,需要计划、审批、执行、验证和 handoff |
| 已有 MCP server,同时它也能执行制品生成或电脑操作 | MCP operation provider | 复用 MCP 协议,但按 operation provider 的审批和 handoff 规则执行 |
| 只是问当前仓库代码、配置、测试、架构 | 不需要外部入口 | 默认源码分析链路已经足够,外部能力不会抢路由 |

记住一个边界:外部能力可以补充证据或完成操作,但不会自动替代源码/trace/log 管线。用户没有明确禁止源码分析时,外部观察默认可以和当前源码一起分析;只有 typed intent 明确排除源码时,系统才关闭源码探索。

### 外部 Skills / Operation Skills

外部 Skill 是 Codrax 的通用电脑操作/制品生成扩展入口。它适合把本地脚本、二进制、公司内工具、PPT/文档/表格生成器、浏览器/桌面自动化包装成一个可审批、可懒加载、可 handoff 的 operation provider。

它和 MCP 的区别:

- MCP 外部工具偏“查事实”:返回工单、监控、知识库、外部行号等只读观察。
- 外部 Skill 偏“做事情”:生成文件、调用本地工具、学习说明书后驱动工具、跨多个 skill 编排 workflow。
- 外部 Skill 的 `description` / `when_to_use` / `workflows[]` 会进入 operation planner 的能力摘要,帮助模型根据 typed operation intent 选择 provider;它们不是系统指令,也不会作为 Go 关键字硬路由。
- `command` / `env` / `work_dir` 不进入 prompt。用户批准后,codrax 才按配置启动本地命令。

典型 workflow:

```text
用户请求:根据某工具说明生成一份 PPT
  ↓
turn-policy / analyzer:识别为 artifact_generation / presentation_generation
  ↓
operation planner:看到可用外部 Skills 的 selection card 和 workflows[]
  ↓
生成计划:先调用 manual_reader 读取说明,再把摘要交给 ppt_builder
  ↓
用户 /approve
  ↓
执行 skill,返回 artifact_refs / payload_ref / next_actions / workflow_state
  ↓
后续 planner 继续消费 handoff,必要时再请求下一次 /approve
```

一个最小 manifest 长这样:

```yaml
operation_skills:
  - name: manual_reader
    description: "Read a manual, extract command templates, and hand off."
    when_to_use:
      - "Use when the task requires learning an unfamiliar CLI/tool manual before generating an artifact."
    when_not_to_use:
      - "Do not use for pure source-code explanation without an operation goal."
    operation_kinds: ["presentation_generation", "artifact_generation"]
    operation_surfaces: ["local_file", "slides"]
    operation_side_effects: ["local_file_write"]
    operation_requires_confirmation: true
    workflows:
      - name: manual_to_deck
        summary: "Read a manual, extract key commands, then call a deck builder."
        entry: true
        operation_kind: external_skill_workflow
        target_surface: slides
        next_providers: ["skill:ppt_builder"]
        return_provider: "skill:manual_reader"
    input_schema: |
      {"manual_path":"string","output_path":"string"}
    output_contract:
      artifact_refs: true
      payload_ref: true
      next_actions: true
      return_action: true
      workflow_state: true
    command: "./tools/manual_reader_skill"
    args: ["--json"]
    input_mode: stdin_json
```

详细字段、skill-to-skill `next_actions` / `return_action` 输出契约、DAG workflow 预算和 JSON 兼容别名见第 5 章的 [`operation_skills` 配置参考](#operation_skills-配置参考)。

### MCP 外部工具

MCP(Model Context Protocol)让 codrax 可以接入你自己提供的**只读外部工具**或**只读资源**:例如内部知识库、告警系统、缺陷平台、监控快照、trace 预分析服务、规范文档检索器等。MCP 结果会进入 `mcp_resource` 外部观测通道,不会伪装成当前源码的 `file:line` citation。

### 一句话理解 MCP 在 codrax 里的位置

MCP 不是“替代源码分析”的入口,而是给探索阶段增加一组**外部只读证据来源**。默认情况下,codrax 会把 MCP 结果和当前仓库源码一起分析;只有当用户明确说“只看 MCP / 不要看源码 / 不要读取仓库”并且 analyzer 产出 typed 排除策略时,才会关闭当前源码探索。

### 什么时候用

| 场景 | 推荐方式 |
|---|---|
| 问题需要外部事实,例如工单、监控、trace 预处理结果 | 配一个 MCP server,让模型在探索阶段调用 |
| 外部资源有真实行号/行范围/JSON pointer/表格行 | 使用 typed line support,把坐标作为 MCP 外部观测交给 codrax |
| 只是在分析源码、配置文件、仓库结构 | 不需要配置 MCP;空配置时工具 schema、prompt、调度都不变 |

### 哪些 MCP 内容会进入模型上下文

理解这一点很重要:不是 `codrax.yaml` 里的所有东西都会进 prompt。模型真正能看到、并用来决定是否调用工具的内容如下。

| 来源 | 会不会进入模型上下文 | 进入位置 | 用途 |
|---|---:|---|---|
| `tools/list[].name` | 会 | 模型工具名,自动变成 `<server>__<tool>` | 让模型知道可以调用哪个外部工具 |
| `tools/list[].description` | 会 | 工具描述 / function schema | 告诉模型什么时候该调用、能查什么、不要查什么 |
| `tools/list[].inputSchema` | 会 | 工具参数 JSON Schema | 告诉模型参数字段、类型、必填项、枚举值和字段含义 |
| `resources/list[].uri/name/description/mimeType` | 会 | `External Guidance (MCP)` prompt 章节 + `mcp_read_resource` 的 URI 枚举 | 告诉模型有哪些外部资源可以读取,并限制只能读已枚举 URI |
| `prompts/list[].name/description` | 会 | `External Guidance (MCP)` prompt 章节 | 作为外部 runbook/使用建议,不会当系统指令执行 |
| MCP 调用结果摘要 | 会 | 后续探索 / 提取 / 成文阶段的外部观测池 | 作为 `mcp_resource` 外部证据继续传递 |
| typed observation 坐标 | 会 | ObservationLedger / finalizer 外部证据 | 支撑外部事实,如行号、JSON pointer、表格行 |
| `mcp_servers[].command/args/env` | 不会作为模型指令进入 prompt | 只用于启动本地 MCP 子进程 | 避免把本地路径、密钥、启动细节暴露给模型 |

`resources/list` 和 `prompts/list` 会被包在“外部、不可信指导”的提示里。它们能帮助模型选择工具,但不能覆盖系统规则,也不能要求模型绕过源码证据、写文件或执行危险操作。

### 如何写工具描述来教模型用 MCP

模型不会读你的 MCP server 源码。它主要靠 `tools/list` 返回的 `description` 和 `inputSchema` 来学会怎么调用工具。一个好工具描述应该包含四件事:

1. **什么时候用**:对应用户会怎么问。
2. **查什么**:返回的是工单、监控、trace 预处理、知识库段落,还是结构化表格。
3. **参数怎么填**:哪些字段来自用户问题,哪些字段可选,时间/ID/服务名格式是什么。
4. **调用后如何用结果**:是否需要继续读源码、是否需要用 typed observation 行号引用外部事实。

推荐写法:

```json
{
  "name": "search_incidents",
  "description": "Use when the user asks about production incidents, alerts, or ticket history. Fill query from the user's symptom words. Fill service only when the user names a service. Return concise incident rows with id, title, owner, status, and resource_uri; if a row supports a claim, return typed observations with row numbers.",
  "inputSchema": {
    "type": "object",
    "additionalProperties": false,
    "properties": {
      "query": {
        "type": "string",
        "description": "Natural-language symptom, error text, incident id, or alert name from the current user question."
      },
      "service": {
        "type": "string",
        "description": "Optional service/module name when explicitly mentioned by the user."
      },
      "time_start": {
        "type": "string",
        "description": "Optional inclusive start time if the user gives a time window, e.g. 2026-05-29T10:00:00+08:00."
      },
      "time_end": {
        "type": "string",
        "description": "Optional exclusive end time if the user gives a time window."
      },
      "limit": {
        "type": "integer",
        "description": "Maximum rows to return. Use 10 by default and do not exceed 50."
      }
    },
    "required": ["query"]
  }
}
```

不推荐写法:

```json
{
  "name": "search",
  "description": "Search everything.",
  "inputSchema": {"type": "object"}
}
```

这种描述太泛,模型不知道什么时候用,也不知道参数该从哪里来。

### 参数怎么动态传给 MCP 工具

用户一般不需要手写 MCP 工具参数。流程是:

1. 你在 MCP server 的 `inputSchema` 里声明字段、类型、必填项和描述。
2. 用户用自然语言提出问题。
3. 探索阶段模型根据“当前问题 + 工具描述 + 参数 schema + 已收集证据”生成一次 JSON tool call。
4. codrax 按 `<server>__<tool>` 路由到对应 MCP server,把模型生成的 JSON 原样作为 `tools/call.arguments` 传给 server。
5. server 负责校验参数并返回结果;如果参数缺失或格式不合法,应该返回清晰错误,让模型重试更窄的参数。

例子:

用户问:

```text
结合最近 24 小时的支付告警和当前代码,分析 checkout 超时是不是缓存配置导致的。
```

如果 server 暴露 `ops__search_alerts(query, service, since, limit)`,模型可能动态调用:

```json
{
  "query": "checkout timeout cache config",
  "service": "checkout",
  "since": "24h",
  "limit": 10
}
```

然后模型会把 MCP 返回的告警作为外部观测,再用 `grep` / `read_file` / `repo_map` 去当前仓库找缓存配置、调用链和证据。默认不是“只看 MCP”;默认是“外部观测 + 源码证据”一起分析。

给 MCP 工具设计动态参数时,建议:

- 用业务字段名,不要用 `arg1` / `data` / `payload`。
- 给时间字段写清楚单位和格式,例如“秒”“毫秒”“ISO8601”“Unix ms”。
- 对 mode/type/status 使用 `enum`,减少模型拼错。
- 对大查询加 `limit` / `page_token` / `time_start` / `time_end`,避免一次返回过大。
- 如果工具需要路径、URI、工单号、trace id,在字段描述里写“来自用户问题或上一步工具结果”。

### 触发原则和探索工作流

MCP 的调用发生在探索阶段,不是 analyzer 阶段。典型工作流如下:

```text
用户问题
  ↓
analyzer: 判断问题类型、实体、是否涉及外部观测;不直接调用 MCP
  ↓
explorer: 看到内置源码工具 + MCP 工具 schema + MCP 外部资源清单
  ↓
按需调用 MCP:
  - `<server>__<tool>`: 查询外部系统
  - `mcp_read_resource(uri=...)`: 读取 resources/list 已枚举资源
  ↓
把 MCP 结果作为 mcp_resource 外部观测保存
  ↓
继续用 repo_map / grep / read_file 收集当前源码证据
  ↓
extract/finalize: 同时使用 MCP 外部观测和源码 file:line 证据成文
```

触发原则:

- **显式触发**:用户明确说“使用 MCP / 查工单 / 查监控 / 查某个外部资源 / 读取 mcp://...”时,模型更应该优先考虑 MCP。
- **隐式触发**:用户没有点名 MCP,但问题明显需要外部事实,且已配置 MCP 工具描述匹配,模型可以调用 MCP。
- **源码优先场景**:纯代码结构、函数行为、配置解释等问题,通常不需要 MCP。
- **混合场景**:日志、trace、MCP、web、工单等外部观察提到当前代码时,默认要同时做源码探索;除非用户明确禁止源码分析。

REPL 面板会真实显示 MCP 工具调用。例如:

```text
⇢ 探索 · 第 2 轮 调用工具 ops__search_alerts query=checkout timeout
• MCP 返回 8 条外部观测，资源 mcp://ops/incidents/checkout，行 3,7
```

如果同一次探索派发里同一组 typed observation 再次返回,面板会显示“与前次相同”,但不会隐藏工具调用。大输出会显示 `blob ...` / `rows ...` 引用,不会把完整 payload 展开到面板。

### 安全边界

- 当前实现支持 **stdio server**。server 是本机子进程,通过 stdin/stdout JSON-RPC 通信。
- **显式触发**:用户问题明确要求使用 MCP / 某个 MCP server / 某个外部工具时,探索阶段模型可以直接调用 `<server>__<tool>` 或 `mcp_read_resource`。
- **隐式触发**:配置 MCP 后,工具会在探索阶段进入模型工具面,模型可根据问题和工具描述自行选择;系统不会在 analyzer 之前或代码问题里硬自动调用 MCP。
- MCP 工具只暴露给探索类 agent(explorer / sub-explorer)。analyzer、extractor、finalizer、写模式工具面不会看到这些外部工具。
- 工具名会自动命名空间化为 `<server>__<tool>`,避免和内置工具或其它 server 重名。
- `inherit_env` 默认建议 `false`,避免把 provider/API key 默认传给外部子进程。需要传环境变量时用 `env:` 白名单。
- `mcp_read_resource` 只能读取 server 在 `resources/list` 中枚举过的 URI;codrax 不会主动拼 URI。
- MCP 的 resources/prompts 只作为“外部建议”提示模型,不是系统指令;不要把不可信 MCP 内容当作 prompt 权限。
- MCP 结果是外部证据,不是当前源码证据。混合问题(“根据 MCP 结论再看源码”)需要同时保留 MCP observation 和源码 `file:line` 两条证据链。

### 配置示例

把下面放进 `codrax.yaml`:

```yaml
mcp_max_servers: 8

mcp_servers:
  - name: docs
    transport: stdio
    command: /opt/company-mcp/docs-server
    args: ["--readonly"]
    inherit_env: false
    env:
      MCP_MODE: readonly
    startup_timeout_ms: 3000
    call_timeout_ms: 10000
    max_response_bytes: 4194304
```

启动后,如果 server 暴露了 `search` 工具,模型看到的工具名会是 `docs__search`。如果 server 暴露了资源,探索阶段还会看到 `mcp_read_resource(uri=...)`。

完整字段说明见第 5 章的 [MCP 外部工具配置参考](#mcp-外部工具配置参考)。

### 快速上手:一个带动态参数和自定义工作流的 demo MCP

下面这个 demo MCP 模拟“告警/工单检索”,同时暴露一个“告警 + 源码联动分析”的工作流资源。用户只用自然语言提问,模型会根据工具描述和 `inputSchema` 自动提取 `query`、`service`、`since`、`limit` 等动态参数;如果需要更明确的探索步骤,模型还能读取你提供的 workflow 资源,再和当前 explorer 的 `repo_map` / `grep` / `read_file` 配合。

#### 第 1 步:准备 demo server

创建 `/opt/codrax-demo/mcp_incidents_demo.py`:

```python
#!/usr/bin/env python3
import json
import sys

INCIDENTS = [
    {
        "id": "INC-17",
        "service": "checkout",
        "title": "checkout timeout after cache config rollout",
        "severity": "p1",
        "status": "open",
        "line": 7,
        "summary": "checkout p95 latency rose after cache ttl changed from 30s to 1s",
    },
    {
        "id": "INC-24",
        "service": "payment",
        "title": "payment gateway retry storm",
        "severity": "p2",
        "status": "mitigated",
        "line": 12,
        "summary": "retry storm correlated with upstream 503 spikes",
    },
    {
        "id": "INC-31",
        "service": "checkout",
        "title": "checkout cache miss burst",
        "severity": "p2",
        "status": "closed",
        "line": 18,
        "summary": "cache miss burst caused extra database load during deploy",
    },
]


WORKFLOW_URI = "mcp://demo-alerts/workflows/alert-code-analysis"
WORKFLOW_TEXT = """Demo workflow: alert + source-code analysis

Use this workflow when the user asks whether a production alert, incident,
ticket, or monitoring symptom is related to the current repository.

Recommended explorer flow:
1. Call demo_alerts__search_alerts first. Fill query from symptom words, service
   from the service/module named by the user, and since from the user's time
   window such as 24h.
2. Treat returned rows as external observations. Do not convert MCP line numbers
   into source-code citations.
3. Continue source-code exploration unless the user explicitly forbids it:
   use repo_map for navigation, grep for config/error names, and read_file for
   current repository evidence.
4. Final answers should separate external alert facts from current source
   evidence and explain whether the source supports or weakens the alert theory.
"""

WORKFLOW_RESOURCE = {
    "uri": WORKFLOW_URI,
    "name": "alert-code-analysis-workflow",
    "description": (
        "Read this workflow when the user asks to combine production alerts, "
        "incidents, tickets, or monitoring symptoms with current source-code analysis."
    ),
    "mimeType": "text/plain",
}


TOOL = {
    "name": "search_alerts",
    "description": (
        "Use when the user asks about production alerts, incidents, tickets, "
        "or recent operational symptoms. Fill query from the user's symptom "
        "words. Fill service only when the user names a service or module. "
        "Fill since when the user gives a time range such as '24h'. Return "
        "line-backed external observations; after using this tool, continue "
        "source-code exploration unless the user explicitly says not to."
    ),
    "inputSchema": {
        "type": "object",
        "additionalProperties": False,
        "properties": {
            "query": {
                "type": "string",
                "description": "Symptom words, error text, or incident id from the current user question.",
            },
            "service": {
                "type": "string",
                "description": "Optional service/module name from the user question, e.g. checkout or payment.",
            },
            "since": {
                "type": "string",
                "description": "Optional relative or absolute time window, e.g. 24h or 2026-05-29T10:00:00+08:00.",
            },
            "limit": {
                "type": "integer",
                "description": "Maximum rows to return. Default 5; do not exceed 20.",
            },
        },
        "required": ["query"],
    },
}


def write(result_id, result):
    sys.stdout.write(json.dumps({"jsonrpc": "2.0", "id": result_id, "result": result}, ensure_ascii=False) + "\n")
    sys.stdout.flush()


def write_error(result_id, code, message):
    sys.stdout.write(json.dumps({"jsonrpc": "2.0", "id": result_id, "error": {"code": code, "message": message}}, ensure_ascii=False) + "\n")
    sys.stdout.flush()


def search_alerts(args):
    query = str(args.get("query", "")).lower()
    service = str(args.get("service", "")).lower()
    limit = int(args.get("limit") or 5)
    limit = max(1, min(limit, 20))

    rows = []
    for incident in INCIDENTS:
        haystack = " ".join([incident["id"], incident["service"], incident["title"], incident["summary"]]).lower()
        if service and incident["service"].lower() != service:
            continue
        if query and not all(part in haystack for part in query.split()):
            # Demo logic is intentionally simple. Real servers can use search engines.
            if not any(part in haystack for part in query.split()):
                continue
        rows.append(incident)
        if len(rows) >= limit:
            break

    envelope = {
        "version": "codrax.mcp.observation.v1",
        "summary": f"found {len(rows)} alert row(s) for query={args.get('query')!r}, service={args.get('service')!r}, since={args.get('since')!r}",
        "resource_uri": "mcp://demo-alerts/incidents",
        "observations": [
            {
                "summary": f"{row['id']} {row['service']} {row['severity']} {row['status']}: {row['summary']}",
                "line_start": row["line"],
                "line_end": row["line"],
                "selector": f"incident_id={row['id']}",
                "payload_ref": f"mcp://demo-alerts/incidents/{row['id']}",
            }
            for row in rows
        ],
    }
    return {
        "content": [
            {
                "type": "text",
                "mimeType": "application/vnd.codrax.observation+json",
                "text": json.dumps(envelope, ensure_ascii=False),
            }
        ]
    }


for line in sys.stdin:
    if not line.strip():
        continue
    req = json.loads(line)
    method = req.get("method")
    req_id = req.get("id")
    if req_id is None:
        # JSON-RPC notification, such as notifications/initialized. Do not reply.
        continue

    if method == "initialize":
        write(req_id, {"protocolVersion": "2024-11-05", "capabilities": {"tools": {}, "resources": {}}, "serverInfo": {"name": "codrax-demo-alerts", "version": "0.1.0"}})
    elif method == "tools/list":
        write(req_id, {"tools": [TOOL]})
    elif method == "tools/call":
        params = req.get("params") or {}
        name = params.get("name")
        args = params.get("arguments") or {}
        if name != "search_alerts":
            write_error(req_id, -32602, f"unknown tool: {name}")
        elif not args.get("query"):
            write_error(req_id, -32602, "query is required")
        else:
            write(req_id, search_alerts(args))
    elif method == "resources/list":
        write(req_id, {"resources": [WORKFLOW_RESOURCE]})
    elif method == "resources/read":
        params = req.get("params") or {}
        if params.get("uri") != WORKFLOW_URI:
            write_error(req_id, -32602, f"unknown resource: {params.get('uri')}")
        else:
            write(req_id, {"contents": [{"uri": WORKFLOW_URI, "mimeType": "text/plain", "text": WORKFLOW_TEXT}]})
    else:
        write_error(req_id, -32601, f"unsupported method: {method}")
```

赋予执行权限:

```bash
chmod +x /opt/codrax-demo/mcp_incidents_demo.py
```

这个 demo 只用 Python 标准库。真实生产 server 可以换成 Go、Python、Node.js 或公司内部服务适配器。最小工具能力需要响应 `initialize`、`tools/list`、`tools/call`;如果要提供用户自定义工作流,再响应 `resources/list`、`resources/read`。

#### 第 2 步:配置 codrax.yaml

在 `codrax.yaml` 中加入:

```yaml
mcp_max_servers: 8

mcp_servers:
  - name: demo_alerts
    transport: stdio
    command: /opt/codrax-demo/mcp_incidents_demo.py
    inherit_env: false
    startup_timeout_ms: 3000
    call_timeout_ms: 10000
    max_response_bytes: 1048576
```

生效后,模型看到的工具名不是 `search_alerts`,而是带 server 命名空间的 `demo_alerts__search_alerts`。这样多个 MCP server 都有 `search` / `lookup` 工具时也不会冲突。

同时,这个 demo 会在 `resources/list` 中暴露 `mcp://demo-alerts/workflows/alert-code-analysis`。codrax 会把资源名称、描述、URI 放进 MCP 外部指导区,并只允许模型通过 `mcp_read_resource` 读取这个已枚举 URI。

#### 第 3 步:用自然语言触发动态参数

启动 codrax 后,可以这样问:

```text
结合最近 24 小时的 checkout 告警和当前代码,分析 checkout timeout 是否和缓存配置有关。
```

模型不需要用户手写 JSON。探索阶段会根据问题和 schema 动态生成类似调用:

```json
{
  "query": "checkout timeout cache",
  "service": "checkout",
  "since": "24h",
  "limit": 5
}
```

REPL 面板中会看到类似:

```text
⇢ 探索 · 第 1 轮 调用工具 mcp_read_resource uri=mcp://demo-alerts/workflows/alert-code-analysis
⇢ 探索 · 第 1 轮 调用工具 demo_alerts__search_alerts query=checkout timeout cache service=checkout
• MCP 返回 1 条外部观测，资源 mcp://demo-alerts/incidents，行 7
```

模型不一定每次都读取 workflow;如果工具描述已经足够明确,它可能直接调用 `demo_alerts__search_alerts`。当用户问题复杂、流程特殊、或者你希望固定公司内的排查顺序时,workflow 资源会更有用。

随后 codrax 会继续用 `repo_map` / `grep` / `read_file` 探索当前仓库源码。最终答案里,MCP 告警和 workflow 是外部观测/外部建议,源码结论仍需要当前仓库的 `file:line` 证据支撑。

#### 第 4 步:自定义工作流应该怎么写

workflow 最适合描述“探索顺序”和“证据边界”,不要写成强制系统指令。推荐包含:

1. **适用条件**:什么类型的问题才使用这个 workflow。
2. **先调用哪些 MCP 工具**:例如先查告警、工单、监控,再查知识库。
3. **动态参数映射**:哪些参数来自用户问题,哪些来自上一步结果。
4. **如何回到源码探索**:明确提醒继续用 `repo_map` / `grep` / `read_file` 找当前仓库证据。
5. **证据边界**:MCP 行号是外部观测,不能当作源码 citation;源码判断仍要 current repo 的 `file:line`。

上面 demo 的 `WORKFLOW_TEXT` 就是一个最小可用模板。真实环境里可以换成“告警排查流程”“trace 分析流程”“缺陷单回归流程”“内部规范审查流程”等。只要它通过 `resources/list` 枚举出来,模型就能在探索阶段按需读取。

#### 第 5 步:怎么确认配置真的生效

- 如果探索阶段出现 `调用工具 demo_alerts__search_alerts`,说明工具已进入 explorer 工具面。
- 如果探索阶段出现 `调用工具 mcp_read_resource uri=mcp://demo-alerts/workflows/alert-code-analysis`,说明 workflow 资源已枚举并可读取。
- 如果只想检查 server 是否能启动,先在终端直接运行 `/opt/codrax-demo/mcp_incidents_demo.py`,再输入一行 JSON-RPC:

```json
{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}
```

应该能看到返回的 `search_alerts` 工具和 `inputSchema`。

也可以检查 workflow 资源:

```json
{"jsonrpc":"2.0","id":2,"method":"resources/list","params":{}}
```

应该能看到 `mcp://demo-alerts/workflows/alert-code-analysis`。

常见问题:

| 现象 | 原因 | 处理 |
|---|---|---|
| 探索阶段没有调用 MCP | 问题是纯源码问题,或工具描述太泛 | 在工具 `description` 里写清楚适用问题;或用户明确问“结合告警/工单/监控” |
| 模型没读 workflow | 工具描述已经足够完成任务,或 workflow 描述不够具体 | 在 resource `description` 里写清楚“什么时候读这个 workflow”;复杂任务里用户也可以明确说“按告警源码联动 workflow 分析” |
| 模型传的参数不准 | 字段名/描述太抽象 | 把 `query`、`service`、`since` 等字段写成业务含义,并给例子 |
| 返回太大 | server 把完整 payload 塞进 `text` | 返回摘要 + `payload_ref` / `row_set_ref`,让 codrax 面板保持轻量 |
| 最终答案没引用外部行号 | server 返回普通文本 | 使用 `codrax.mcp.observation.v1` 或 `application/vnd.codrax.observation+json` |

### Shell 脚本版完整 demo MCP

如果你的 MCP 只是把公司已有命令、HTTP 查询脚本、日志检索脚本包装成只读工具,用 shell 写一个 stdio MCP server 也可以。下面是一个完整可运行的版本,功能和上面的 Python demo 类似:

- 暴露 `search_alerts` 工具,支持动态参数 `query`、`service`、`since`、`limit`。
- 暴露 `resources/list` / `resources/read`,提供一个用户自定义 workflow。
- 返回 `codrax.mcp.observation.v1` typed observation,让外部行号进入 MCP 外部观测通道。

依赖:

- `bash`
- `jq`

#### 第 1 步:准备 shell MCP server

创建 `/opt/codrax-demo/mcp_alerts_shell.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  echo "mcp_alerts_shell.sh requires jq" >&2
  exit 1
fi

WORKFLOW_URI="mcp://shell-alerts/workflows/alert-code-analysis"
WORKFLOW_TEXT=$(cat <<'WORKFLOW_TEXT_EOF'
Shell workflow: alert + source-code analysis

Use this workflow when the user asks whether a production alert, incident,
ticket, or monitoring symptom is related to the current repository.

Recommended explorer flow:
1. Call shell_alerts__search_alerts first. Fill query from symptom words, service
   from the service/module named by the user, and since from the user time window.
2. Treat returned rows as external observations, not current source citations.
3. Continue source-code exploration unless the user explicitly forbids it.
4. In the final answer, separate external alert facts from current repository
   file:line evidence.
WORKFLOW_TEXT_EOF
)

INCIDENTS=$'INC-17|checkout|checkout timeout after cache config rollout|p1|open|7|checkout p95 latency rose after cache ttl changed from 30s to 1s\nINC-24|payment|payment gateway retry storm|p2|mitigated|12|retry storm correlated with upstream 503 spikes\nINC-31|checkout|checkout cache miss burst|p2|closed|18|cache miss burst caused extra database load during deploy'

reply() {
  local id_json="$1"
  local result_json="$2"
  jq -nc --argjson id "$id_json" --argjson result "$result_json" \
    '{jsonrpc:"2.0", id:$id, result:$result}'
}

reply_error() {
  local id_json="$1"
  local code="$2"
  local message="$3"
  jq -nc --argjson id "$id_json" --argjson code "$code" --arg message "$message" \
    '{jsonrpc:"2.0", id:$id, error:{code:$code, message:$message}}'
}

tool_schema() {
  jq -nc '{
    tools: [
      {
        name: "search_alerts",
        description: "Use when the user asks about production alerts, incidents, tickets, or recent operational symptoms. Fill query from symptom words. Fill service only when the user names a service or module. Fill since when the user gives a time range such as 24h. Return line-backed external observations; after using this tool, continue source-code exploration unless the user explicitly says not to.",
        inputSchema: {
          type: "object",
          additionalProperties: false,
          properties: {
            query: {
              type: "string",
              description: "Symptom words, error text, or incident id from the current user question."
            },
            service: {
              type: "string",
              description: "Optional service/module name from the user question, e.g. checkout or payment."
            },
            since: {
              type: "string",
              description: "Optional relative or absolute time window, e.g. 24h or 2026-05-29T10:00:00+08:00."
            },
            limit: {
              type: "integer",
              description: "Maximum rows to return. Default 5; do not exceed 20."
            }
          },
          required: ["query"]
        }
      }
    ]
  }'
}

resources_list() {
  jq -nc --arg uri "$WORKFLOW_URI" '{
    resources: [
      {
        uri: $uri,
        name: "shell-alert-code-analysis-workflow",
        description: "Read this workflow when the user asks to combine production alerts, incidents, tickets, or monitoring symptoms with current source-code analysis.",
        mimeType: "text/plain"
      }
    ]
  }'
}

search_alerts() {
  local args_json="$1"
  local query service since limit query_lc service_lc observations rows summary envelope
  query="$(jq -r '.query // ""' <<<"$args_json")"
  service="$(jq -r '.service // ""' <<<"$args_json")"
  since="$(jq -r '.since // ""' <<<"$args_json")"
  limit="$(jq -r '.limit // 5' <<<"$args_json")"
  if ! [[ "$limit" =~ ^[0-9]+$ ]]; then
    limit=5
  fi
  if (( limit < 1 )); then limit=1; fi
  if (( limit > 20 )); then limit=20; fi

  query_lc="$(tr "[:upper:]" "[:lower:]" <<<"$query")"
  service_lc="$(tr "[:upper:]" "[:lower:]" <<<"$service")"
  observations='[]'
  rows=0

  while IFS='|' read -r id svc title severity status line_no detail; do
    local svc_lc haystack ok word obs
    svc_lc="$(tr "[:upper:]" "[:lower:]" <<<"$svc")"
    if [[ -n "$service_lc" && "$svc_lc" != "$service_lc" ]]; then
      continue
    fi
    haystack="$(tr "[:upper:]" "[:lower:]" <<<"$id $svc $title $detail")"
    ok=1
    for word in $query_lc; do
      if [[ "$haystack" != *"$word"* ]]; then
        ok=0
        break
      fi
    done
    if (( ok == 0 )); then
      continue
    fi

    obs="$(jq -nc \
      --arg summary "$id $svc $severity $status: $detail" \
      --argjson line "$line_no" \
      --arg selector "incident_id=$id" \
      --arg payload_ref "mcp://shell-alerts/incidents/$id" \
      '{summary:$summary, line_start:$line, line_end:$line, selector:$selector, payload_ref:$payload_ref}')"
    observations="$(jq -nc --argjson arr "$observations" --argjson obs "$obs" '$arr + [$obs]')"
    rows=$((rows + 1))
    if (( rows >= limit )); then
      break
    fi
  done <<<"$INCIDENTS"

  summary="found ${rows} alert row(s) for query='${query}', service='${service}', since='${since}'"
  envelope="$(jq -nc \
    --arg summary "$summary" \
    --arg uri "mcp://shell-alerts/incidents" \
    --argjson observations "$observations" \
    '{version:"codrax.mcp.observation.v1", summary:$summary, resource_uri:$uri, observations:$observations}')"
  jq -nc --arg text "$envelope" '{
    content: [
      {
        type: "text",
        mimeType: "application/vnd.codrax.observation+json",
        text: $text
      }
    ]
  }'
}

while IFS= read -r line; do
  [[ -z "$line" ]] && continue
  id_json="$(jq -c '.id // empty' <<<"$line")"
  if [[ -z "$id_json" ]]; then
    # JSON-RPC notification, such as notifications/initialized. Do not reply.
    continue
  fi
  method="$(jq -r '.method // ""' <<<"$line")"

  case "$method" in
    initialize)
      reply "$id_json" "$(jq -nc '{protocolVersion:"2024-11-05", capabilities:{tools:{}, resources:{}}, serverInfo:{name:"codrax-shell-alerts", version:"0.1.0"}}')"
      ;;
    tools/list)
      reply "$id_json" "$(tool_schema)"
      ;;
    tools/call)
      name="$(jq -r '.params.name // ""' <<<"$line")"
      args_json="$(jq -c '.params.arguments // {}' <<<"$line")"
      if [[ "$name" != "search_alerts" ]]; then
        reply_error "$id_json" -32602 "unknown tool: $name"
      elif [[ "$(jq -r '.query // ""' <<<"$args_json")" == "" ]]; then
        reply_error "$id_json" -32602 "query is required"
      else
        reply "$id_json" "$(search_alerts "$args_json")"
      fi
      ;;
    resources/list)
      reply "$id_json" "$(resources_list)"
      ;;
    resources/read)
      uri="$(jq -r '.params.uri // ""' <<<"$line")"
      if [[ "$uri" != "$WORKFLOW_URI" ]]; then
        reply_error "$id_json" -32602 "unknown resource: $uri"
      else
        reply "$id_json" "$(jq -nc --arg uri "$WORKFLOW_URI" --arg text "$WORKFLOW_TEXT" '{contents:[{uri:$uri, mimeType:"text/plain", text:$text}]}')"
      fi
      ;;
    *)
      reply_error "$id_json" -32601 "unsupported method: $method"
      ;;
  esac
done
```

赋予执行权限:

```bash
chmod +x /opt/codrax-demo/mcp_alerts_shell.sh
```

#### 第 2 步:配置 shell MCP

在 `codrax.yaml` 中加入:

```yaml
mcp_max_servers: 8

mcp_servers:
  - name: shell_alerts
    transport: stdio
    command: /opt/codrax-demo/mcp_alerts_shell.sh
    inherit_env: false
    startup_timeout_ms: 3000
    call_timeout_ms: 10000
    max_response_bytes: 1048576
```

生效后,模型看到的工具名是 `shell_alerts__search_alerts`,workflow URI 是 `mcp://shell-alerts/workflows/alert-code-analysis`。

#### 第 3 步:验证 shell MCP

直接运行脚本,输入:

```json
{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}
```

应该返回 `search_alerts` 的描述和 `inputSchema`。再测试动态参数:

```json
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_alerts","arguments":{"query":"checkout timeout cache","service":"checkout","since":"24h","limit":5}}}
```

应该返回 `application/vnd.codrax.observation+json`,其中包含 `mcp://shell-alerts/incidents` 和行号观测。

在 codrax 里提问:

```text
结合最近 24 小时的 checkout 告警和当前代码,分析 checkout timeout 是否和缓存配置有关。
```

REPL 面板可能显示:

```text
⇢ 探索 · 第 1 轮 调用工具 mcp_read_resource uri=mcp://shell-alerts/workflows/alert-code-analysis
⇢ 探索 · 第 1 轮 调用工具 shell_alerts__search_alerts query=checkout timeout cache service=checkout
• MCP 返回 1 条外部观测，资源 mcp://shell-alerts/incidents，行 7
```

shell 版本适合快速把已有命令包装起来。生产环境里,如果工具需要复杂认证、分页、并发、缓存或更严格 JSON 校验,建议改成 Go/Python/Node.js server,但工具描述、动态参数、workflow resource 和 typed observation 的设计方式保持一致。

### MCP server 返回结果怎么写

普通 MCP text/resource 结果可以直接返回简短摘要;codrax 会把它作为外部观测。如果结果很大,建议 server 自己返回摘要 + 外部引用,不要把几 MB 原文直接塞进 `text`。

codrax 也会保护用户面板和上下文:

- 超过内联阈值的 MCP `Summary` 会写入 `.codrax/blob/...`,面板只显示 `blob ...` 引用。
- typed observation 很多时,面板只显示观测条数和少量行号摘要。
- `payload_ref` / `row_set_ref` / `page_ref` 会作为外部引用传递给后续阶段。

推荐返回:

```json
{
  "version": "codrax.mcp.observation.v1",
  "summary": "found 12 matching incidents; top 3 are shown, full row set in row_set_ref",
  "resource_uri": "mcp://ops/incidents/search/abc123",
  "row_set_ref": "mcp://ops/incidents/search/abc123/rows.jsonl",
  "payload_ref": "mcp://ops/incidents/search/abc123/full.json",
  "observations": [
    {
      "summary": "INC-17 checkout timeout owned by runtime team",
      "row": 1,
      "selector": "incident_id=INC-17"
    }
  ]
}
```

### typed line support

如果 MCP server 能给出明确坐标,推荐返回显式 typed observation envelope。codrax 只在看到 `version: "codrax.mcp.observation.v1"` 或 MIME `application/vnd.codrax.observation+json` 时保留行号;普通文本/普通 JSON 不会被猜测行号。

工具返回示例:

```json
{
  "version": "codrax.mcp.observation.v1",
  "summary": "trace sleep/wakeup facts",
  "resource_uri": "mcp://trace/run-42",
  "mime_type": "text/plain",
  "observations": [
    {
      "summary": "target enters S sleep",
      "line_start": 1102717,
      "line_end": 1102717,
      "selector": "pid=36379 event=sched_switch prev_state=S",
      "raw_ref": "mcp://trace/run-42#L1102717"
    },
    {
      "summary": "waker wakes target",
      "line_start": 1139180,
      "line_end": 1139180,
      "selector": "pid=36379 event=sched_wakeup",
      "raw_ref": "mcp://trace/run-42#L1139180"
    }
  ]
}
```

支持的坐标字段包括:

| 字段 | 含义 |
|---|---|
| `resource_uri` | 外部资源 URI,如 `mcp://trace/run-42` |
| `line_start` / `line_end` | 外部资源的行号或行范围 |
| `row` | 表格/结果集行号 |
| `json_pointer` | JSON 文档内坐标 |
| `selector` | 外部系统自己的选择器,如 DOM selector / trace event selector |
| `raw_ref` / `payload_ref` / `row_set_ref` / `page_ref` | 外部 payload 或行集引用 |

这些坐标会以 `origin=mcp_resource` 进入答案链路。它们可以支撑外部事实,但不会绕过源码 citation gate。

### 最小 server 协议

一个可用的 stdio MCP server 至少需要响应:

- `initialize`
- `tools/list`
- `tools/call`

可选但推荐:

- `resources/list` + `resources/read`:让模型读取明确枚举过的外部资源。
- `prompts/list`:告诉模型这个 server 适合什么任务。注意这仍只是外部建议。

排错时先看 `.codrax/logs/.../codrax-*.log`:

| 症状 | 常见原因 | 修复 |
|---|---|---|
| 启动时报 `load mcp servers` | command 不存在 / server 初始化失败 / JSON-RPC 响应非法 | 手动运行 server,确认 stdin/stdout 每行一条 JSON-RPC |
| 探索阶段看不到 MCP 工具 | `mcp_servers` 为空、server 没有 `tools/list`、当前不是探索类 agent | 检查 yaml、server 日志和问题是否进入探索阶段 |
| `mcp_read_resource` 拒绝 URI | URI 没有出现在 `resources/list` | 让 server 先枚举资源;不要让模型凭空拼 URI |
| 行号没有进入答案 | 返回的是普通文本/普通 JSON | 使用 `codrax.mcp.observation.v1` 或 `application/vnd.codrax.observation+json` |

# 4. 写模式 — Auto Pilot

写模式让 codrax **生成代码改动**(增删改文件),在沙箱 git worktree 里跑测试。日常主路径是 Auto Pilot:你描述目标,系统自动探索、拆批、应用、验证和必要的 replan；低/中风险不打断,高风险才暂停审批,critical 自动拒绝。主仓合并仍需要显式动作；组织可用 `write_enabled: false` 禁用写模式。

### 工作区单一不变量(写模式的核心规则)

> **同一个项目同时只能有一个未结算的改动方案。**

未结算 = `pending_approval`(待批)/ `applied_pending_verify`(已写入待验证)/ `applied`(验证通过但未合)/ `verify_failed`(真实验证失败)/ `unverified`(环境或测试面不可用,代码已写入但未被本地证明)/ `partially_applied`(部分文件已写入)。结算 = `merged`(已合并)/ `rejected`(显式丢弃)/ `applied_failed`(apply 阶段崩了,自动终止)/ `blocked`(typed policy 阻断)。`no_change_required` 是 controller 内部 replan 哨兵,不会作为用户待处理 plan 留存。

**为什么这条规则重要**:每个 plan 都是基于"当前主仓状态"生成的。如果一个 plan 已经在 worktree 里改了文件但没合回主仓,这时再生成第二个 plan,新的 plan 看不到第一个 plan 的改动 — 两个 plan 可能对同一文件给出冲突的修改,合并顺序也乱套。

实际行为:新的写目标进入 Auto Pilot 时如果存在未结算的 plan,系统会暂停并显示状态卡/高级收尾入口;REPL 启动时也会在 banner 提醒未结算 plan;数据层(PlanStore)同样硬约束,任何写入路径都过不去。收尾后直接描述下一个目标即可,不用再记一串模式切换命令。

```
[git:master]❯❯ 修复新的边界问题
  ✗ Auto Pilot 已暂停:还有未结算改动 plan-XXXX(状态:applied)。
    为避免两个写入批次基于不同仓库状态互相覆盖,先处理上一版:
      /merge         合并到主仓
      /reject        丢弃改动(保留事后审查记录)
      /plan clear    彻底删除(无审查记录)
    收尾后直接描述下一个目标即可。
```

三个高级收尾入口:

| 命令 | 适用状态 | 文件 | 工作区 |
|---|---|---|---|
| `/merge` | applied / unverified / verify_failed(后者需 `--include-failed`) | 状态改 `merged`,保留供审查 | 自动 discard |
| `/reject [reason]` | 任何未结算状态 | 状态改 `rejected`,reason 写进文件,保留供审查 | 自动 discard |
| `/plan clear` | 任何未结算状态 | **直接删除** plan 文件 + report,无审查记录 | 自动 discard |

启动 banner 提示长这样(灰色单行):

```
   plan-XXXX 已 apply 但未合并 — /merge · /reject · /plan clear
```

按当前 plan 的 status 不同,状态卡也不同 — `pending_approval` 展示风险、fingerprint、diff 和批准/拒绝动作;`applied_pending_verify` 展示已写入 worktree、等待 typed verifier verdict;`verify_failed` 优先展示失败证据、report/diff refs 和自动 replan/人工覆盖入口;`unverified` 展示环境/测试面不可用原因和可恢复的 patch refs。workflow 自身的 `status=complete` 只表示 controller 已经收敛;本地质量结论看 typed `completion.verdict`: `verified` 表示最近 post-apply verify 有权威通过信号,`unverified` 表示代码已交付但本地环境/测试面不可用,`accepted_failed` 表示通过显式 typed escape 接受了残余失败。REPL/CLI 状态卡直接读这个 verdict,不会从结果文案或日志散文猜。

## 4.1 启用

写模式开箱即用,无需配置。REPL auto 会先用结构化 TurnPolicy 判定当前 turn；当 typed `route=write` 成立且 gate 通过时,会自动进入写模式 Auto Pilot。你也可以用 `/write <需求>` 单次强制写模式,或 `/mode write` 让后续写代码请求保持 Auto Pilot。

需要全局禁写(如团队统一部署只读分析)时,在 `codrax.yaml` 里显式设置 kill switch:

```yaml
write_enabled: false
```

设为 false 后,任何 `/mode write` / `/write` / `/approve` / `--mode=write` 都会被拒绝并说明该配置。

## 4.2 完整流程

写模式主入口是 **Auto Pilot**:用户描述目标后,系统自动探索、拆批、生成 bounded plan、在隔离 worktree 中应用、跑验证、失败后按 typed 证据小批量 replan。`plan/apply/verify/audit` 仍是内部阶段和 CLI 高级 lane,但不是日常 REPL 主路径。

controller 每次只推进一个 bounded batch,必要时先跑只读探索并把优先级 handoff context 持久化到 `.codrax/plans/workflows/`。`write_analyzer` 只是 bounded task classifier,不会继承重型探索的 20 轮预算;真正的问题定位由 controller 的 `explore_code` 节点按需触发。探索如果超时/取消但已经产生 typed `emit_evidence` 或 read-set 进展,controller 会把这些结构化证据投影为写模式 handoff,在 ledger 中保留 `degraded` 记录,并继续让当前 batch 进入 bounded planning;不会用模型散文或日志文本做硬路由。探索完成后的 planner 会保留按任务复杂度计算出的 soft budget,避免现象型 bug 在读完证据后被过早截断;如果一轮 planning 没有落 `ChangePlan` 但产生了成功的只读工具结果,controller 会给一次额外 bounded re-dispatch。结构化 `emit_change_plan` validation rejection 也会把最新工具拒绝原因带入下一轮,最多两次修正机会,但 schema、路径和 old-text byte-match 硬门不放松。verify 失败后的 replan 如果用最新 typed `planner_probe` 证明当前已应用 worktree 已满足失败点,planner 可以发内部 `no_change_required` 哨兵;controller 会恢复前一个已应用 plan 并回到 post-apply verify,普通空 `changes[]` 仍然被拒绝。replan 后的 verify 会从 typed `VerifyFailureHandoff` 继承上一轮唯一 runner/cwd 谱系,并优先继承唯一失败 suite;如果失败用例分散但上一轮命令本身有真实 suite selector,则继承命令级 selector,避免一个小修复退化成全仓重跑。多个不同执行谱系、没有真实 selector、或显式 suite 时不会擅自收窄。`ModePlan` 可探索并生成当前批次计划但不写入;`ModeApply` 端到端执行 plan/apply/verify 并按 allow/ask/deny 策略处理审批;`ModeVerify` 验证已有 workflow run、active batch 或导入的 plan seed;`ModeAudit` 只加载已有 typed final report / workflow artifact 并输出审计 JSON,不进入 orchestrator、worktree、工具或模型调用。`--plan-file` 也会导入为单 batch workflow seed,不会绕过最终 risk/approval gate。安全可继续的 active run 会在下一次写模式自动续跑;REPL 启动 banner 和状态卡会主动显示当前 batch、审批需求、handoff 摘要和剩余预算。`/workflow show/list` 是审计入口,`/workflow resume` 仅用于手动选择某个保存 run 作为恢复对象,`/workflow clear` 清理 run 元数据和 context artifacts。没有显式 plan id 时,`/approve` / `/reject` 会优先绑定 active batch 的 `PlanID`。人工拒绝只标记当前 batch,不会把整个 workflow 直接污染为终态。

REPL 实际流程:

### 第 1 步:描述要做的事

```
[git:main]❯❯ 把 internal/foo/bar.go 里 ParseConfig 拆成两个函数,逻辑保持等价
[route=write 后进入 Auto Pilot]
[controller 探索 → 规划 → 低/中风险 apply 到 worktree → verify → 必要时 replan]
✓ apply + verify 完成,workflow 继续推进或结束。
```

如果 auto 路由没有命中,或你想明确进入写模式,用单次入口:

```text
[git:main]❯❯ /write 把 internal/foo/bar.go 里 ParseConfig 拆成两个函数,逻辑保持等价
```

需要连续做一组写代码任务时,用 `/mode write`;后续写目标会继续走 Auto Pilot,直到 `/mode auto`。

系统只在必要时打断:

| 场景 | 用户看到什么 |
|---|---|
| 低/中风险 | 自动 apply + verify + replan,无需输入命令 |
| 高风险 | 当前 batch 暂停,显示 plan/risk/fingerprint,等待批准或拒绝 |
| critical 风险 | 自动拒绝,显示 typed reason 和证据 |
| 验证失败可修 | 失败证据进入 P2 handoff,controller 小批量 replan |
| 预算耗尽/真歧义 | fail-loud 或 ask_user,保留 workflow/report/diff |

审批风险只来自结构化 `ChangePlan`、repo-relative 路径解析、typed diff/内容策略和 fingerprint。导出声明行/API 面变更本身按中风险处理,在 `auto_safe` 下可自动推进;只有叠加依赖/build manifest、CI/workflow、hook、持久化 schema、secret-like 路径/内容、越界路径、大范围变更等精确结构信号时才升为高风险或 critical。

更多虚构的 plan 请求示例(任何项目都能套用):

| 你想做的 | 一行写法 |
|---|---|
| 把一个长函数拆成两个 | `把 X 里 LongFn 拆成两个函数,逻辑保持等价` |
| 给现有函数加参数 + 改调用点 | `给 Foo() 加一个可选 timeout 参数,所有调用点默认传 5s` |
| 重命名一个对外符号 | `把 OldName 重命名为 NewName,同步改导出路径和文档` |
| 加一个新文件 + 接入注册表 | `创建 internal/mcp/sftp.go,在 cmd/root.go 的 mcp 注册表里挂上` |
| 改 yaml 默认值 | `把 codrax.yaml.example 里 pipeline_max_steps 默认值从 50 改成 80,加注释` |

### 审计/恢复:查看 plan 和 workflow

```
[git:main]❯❯ /plan show
[per-file unified diff,带颜色;每个文件独立段落]
- Summary: 拆分 ParseConfig...
- 文件 1/3: internal/foo/bar.go (modify, +24/-12)
[diff body...]
```

Auto Pilot 已经会自动执行低/中风险任务。`/plan show` 是审计入口,不是正常必需步骤。状态卡和启动 banner 会主动告诉你是否需要动作;需要看详细审计时再用:

```
[git:main]❯❯ /workflow show
[active batch、审批记录、handoff 摘要、验证报告、剩余预算]
```

如果高风险 batch 暂停,再用 `/approve` 或 `/reject <原因>` 处理当前 batch。审批提示会带上 run、batch、plan 和 fingerprint,避免先手动查 ID。`/reject` 会保留审计记录;`/plan clear` 只是删除本地副本。

### 高风险时才审批

```
[git:main][task:write]❯❯ /approve
  是否批准 plan plan-abc123 (3 处改动, fingerprint=...)?将在 git worktree 中 apply + 跑 verify。
  > y
[在 .codrax/worktrees/<plan-id>/ 里 git apply + 跑测试]
✓ apply + verify 完成,workflow 继续推进或结束。
```

注意:低/中风险不会要求你先 `/approve`。`/approve` 只处理 pending approval 或你主动选择的高级恢复场景。

Auto Pilot 内部自动:

1. 创建临时 worktree(基于当前 branch)
2. 在 worktree 里 `apply_patch` 每个文件改动(支持 create / modify / delete / patch / rename)
3. 自动检测 runner 跑测试 — 12 种自动探测:go / node(jest/vitest)/ python(pytest/unittest/Django runtests.py)/ rust(cargo)/ java(maven 或 gradle,含 Kotlin/Android)/ ruby(rspec)/ swift / cmake(ctest)/ meson / make / hvigor(HarmonyOS ArkTS)/ cjpm(Cangjie)
4. apply 成功先标记 `applied_pending_verify`;测试通过 → 标记 `applied`;测试/构建真实失败 → 标记 `verify_failed`(可重试);本地缺少 runner/依赖或没有可运行测试 → 标记 `unverified`。底层 `ChangeReport.verification_status` 只会是 `passed` / `failed` / `unavailable`,controller 和 eval 读这个 typed verdict,不会再把 legacy `passed=true + NoTestsRunners` 误当成已验证通过。durable workflow/batch 会同时写 `completion.verdict`,所以 `complete + unverified` 是明确的"已收敛但本地未证明"状态,不是验证通过。

> plan 阶段会先做轻量结构化校验:例如 Python `kind=patch` 会在 scratch overlay 中应用补丁后跑 `py_compile`,能在没有 pytest/ruff 的客户环境里提前拦住语法级坏补丁;缺少 pytest 仍不会被当作代码失败硬门。verify 阶段会对 plan-touched source 做 runner-aware 轻量验证:Python 跑 `py_compile`/静态名称检查,Node 对 JS 跑 `node --check`、对 TS 跑 `tsc --noEmit --pretty false`,Ruby 跑语法检查,Java/Kotlin 跑 Maven/Gradle compile 或 bounded `kotlinc`,Swift Package 跑 `swift build --skip-build`,Go 在没有 `_test.go` 时按 plan-touched package 跑 `go test -json` 编译兜底。语法/解析/编译失败会归入 typed `build_failure` / `failed`,不会被后续缺 pytest、缺依赖或 runner parser_error 降级成 `unverified`。若 `emit_change_plan` / `emit_plan_skeleton` / `emit_plan_change` 被结构化 schema 或 patch builder 拒绝(如同文件重复 change、`old_text` 范围不匹配、line-addressed insertion 的 `content` 复制了多行源码 `old_text` 锚点块、源码 patch 里相邻精确重复插入同一个 3+ 行代码块、Python structured edit 造成同作用域近邻重复 `def` / `class`;合法的 `@property` / `@name.setter` / `@name.deleter` accessor 组合会被结构识别并放行),planner 会基于 typed tool result 进入 soft→hard cap 内的受控修复窗口:允许必要的只读诊断工具重新读取精确 bytes/范围,再重新 emit;不会放松校验,也不会从模型散文猜测下一步。结构化 line edit 的 `old_text` 若与声明行号不匹配,但在当前文件里按行精确且唯一匹配,patch builder 会自动重定位到该唯一范围;找不到或出现多处仍 fail-loud。planner 还可以在 `ChangePlan.verification_probes[]` 里附带 typed 小探针(支持 Python / JavaScript(Node.js) / Ruby / Java(JDK `javac`/`java`) / Go inline code、repo-relative `working_dir`、短超时、可选 `expected_stdout`);ChangePlan probe 若没有 `expected_stdout`,emit 阶段会要求源码里存在对应语言的可执行失败信号(例如 `assert` / `raise` / `throw` / `panic` / 显式非零退出),避免只打印 `FAIL` 却以 0 退出的探针被误判为通过。若同语言 production 改动存在可推导的模块目标,Python / JavaScript(TypeScript) / Ruby / Java / Go 探针还必须通过 import/require/import path 触达被改模块,不能复制实现片段后只测探针内部变量。Java 探针可以是 main 方法体或完整 `CodraxVerificationProbe` 类;本地缺 JDK 会作为 `runner_missing/unavailable` 证据保留,不把环境缺失误判为代码失败。verify 会优先执行这些 bounded probes,用退出码/结构化命令记录给出明确 pass/fail,并把探针语言/cwd/timeout/源码片段写入 raw output,而不是从 `acceptance_tests` 散文里猜测硬逻辑。探针通过时可作为当前 batch 的本地行为验证,项目级 test surface 会被记录到 report,但全量套件不会作为硬门阻断交付;探针失败才按 typed `tests_failed` 进入小批量 replan;探针自身因 import/runner/parser 不可用时,系统再回落到项目 runner 或 `unverified`。
> `ChangePlan` 还会校验每种 file-change kind 的合法内容载体:`create` / `modify` 只能携带完整 `new_content`,`patch` 只能携带 patch 或 structured edits,`delete` / `rename` 不允许混入正文。plan-time 路径状态硬门会先解析 repo-relative `path/new_path`: `create` 不能指向已存在文件或目录,`modify` / `patch` / `rename` source 必须是已存在普通文件,`rename` destination 必须不存在,目录删除会被拒绝;这样模型把已有测试文件误报成 `create` 时会在计划阶段重试,不会进入半写入 apply。对已有大文件的 `kind=modify` 会和当前 repo bytes 做完整性检查:如果 `new_content` 看起来只是当前文件的严格前缀、截断子集,或按行数/字节数删除了绝大多数内容,系统会在 apply/export 前拒绝,并要求改用 surgical `kind=patch`、显式 `kind=delete` 或真正完整的 full-file rewrite。这个硬门只读 typed plan、repo-relative path 和当前文件 bytes;不会从用户问题、模型 summary/rationale 或日志散文推断意图。
> 当 controller 已经完成/降级完成一次只读探索,并留下 explore-stage 的 typed 定位证据(如 `target_file` / `evidence_ref` / `symbol` / `invariant` / `test_surface` / `pattern_hint`)时,planner 会自动获得一个小型 exact-byte synthesis 读预算来读取精确行号和生成 bounded `ChangePlan`;只有 write_analysis/risk/scope 这类粗上下文不会触发该收束。已定位 handoff 生效后,planner 不再渲染 generic likely-file rediscovery seed,只消费优先级 context pack、测试面和必要的当前 bytes。写模式 write_analyze/exploration/plan 阶段如果对超过 320 行的大文件调用 `read_file` 且没有显式正数 `limit`,工具策略会返回 typed repair(`write_analyzer_read_file_requires_window` / `write_exploration_read_file_requires_window` / `write_planner_read_file_requires_window`),让模型用 grep/repo_map 定位或用 `line_offset` + `limit` 窗口读取,避免一次整读把上下文和 wall-time 吃掉;小文件整读、有 `limit` 的分页读取和读模式不受影响。写探索子流程会使用 `WriteExplorationRequest.max_rounds` 或系统默认探索轮数作为硬 ReAct 上限,不只停留在 prompt 提示。读预算耗尽后,本轮 planner 工具面会收窄到 `emit_change_plan` / `emit_plan_skeleton` / `emit_plan_change` 和 `run_tests(dry_run=true, verification_probe={...})`,避免在已经定位后重新开启普通调查。若结构化 emit 被 schema 或 patch builder 拒绝,精确读工具会重新打开用于修复当前字节/范围。没有 explore-stage 定位证据时,controller/planner 仍可按正常写模式探索和规划。这个收束只作用于写模式的只读分析/探索/规划阶段,仍受全局 hard cap/step budget 约束;硬门只消费 stage/mode/tool 参数、typed request budget 和文件行数,不读取用户关键词、issue 文本、模型 summary/rationale 或 `<think>`。
> 当 typed 风险显示 public API、持久化/输出布局/状态、或 build/test/install 行为可能变化时,planner 会收到兼容性软指导:先检查相关配置注册、默认值、输出路径消费者和已有 option/default surface,再优先考虑默认兼容或显式 opt-in/config switch;如果有意改变默认行为,需要把兼容边界写入计划 rationale 和验证面。这仍然只是 prompt guidance;审批、apply、verify 的硬门只消费结构化 artifact。

> actual-diff PatchReview 的映射/容器边界信号不是 Python-only。当前 provider registry 会从真实 diff 新增行和 post-apply 文件 bytes 产生多语言 typed events:Python nested string-key mapping access、JS/TS nested string-key access、Ruby nested hash-key access、Java/Kotlin chained `Map.get`,以及 Go nested string-key map assignment。它们都是软 semantic coverage obligations,进入 P2 handoff 和 bounded follow-up;是否完成仍由 typed verification/coverage proof 决定,不会读取用户意图关键词、issue 文本、模型 rationale 或 `<think>`。

> verifier agent 也可以**绕过自动探测**,显式声明 `runner=<choice>` + `working_dir`(都在 worktree 内的白名单里);适用于多 manifest 仓 / 测试目录在子目录的场景。Python 的 `framework=pytest|unittest|django` 是结构化 enum,即使没有同时填 `runner`,也会被视为 `runner=python` 并跳过 manifest 自动探测,避免 Django 这类仓被无关 `package.json` / `Makefile` 打断。`run_tests.suite` 只接受测试 selector;`-v` / `--maxfail` 等 CLI flags 如果混在 suite 字段里会在执行前被 typed 参数校验拒绝,不会误拼进 pytest node-id 或 unittest selector;pytest 下多个 file/nodeid selector 可以用空格分隔,执行器会在每个 token 都像 pytest selector 时拆成多个 argv,而带空格的单个参数化 selector 会保持为一个 quoted argv;若 `suite` 误填成当前 typed TestSurface candidate id(如 `python/pytest@.`),也会在执行前拒绝并提示改用 `runner` / `framework` / `working_dir`。当 Python/pytest 同时指定嵌套 `working_dir` 时,如果 suite 仍带有 repo-root/package-root 前缀,执行器会用当前 runner cwd 下的真实文件存在性做一次安全 rebase,例如在 `working_dir=sklearn` 时把存在的 `sklearn/ensemble/tests/test_voting.py::case` 归一为 `ensemble/tests/test_voting.py::case`;原 selector 已存在、不是 file/nodeid selector、或是带空格的单个参数化 selector 时不会改写。Python verify 会把 active worktree 的 repo root 以及存在的 `src/`、`lib/` source roots 放到 import root 前列,并让 verification probe 内部再启动的 Python 子进程继承同一 `PYTHONPATH`,避免误测主仓源码或已安装包;Python verification probe 的 `working_dir` 是 repo-relative scope hint,当它指向包内部目录时会用结构化项目根标记(`pyproject.toml` / `setup.py` / `setup.cfg` / `pytest.ini` / `tox.ini` / `noxfile.py` / `manage.py`)提升到最近 Python 项目根执行,避免包内 `collections.py` 等文件遮蔽标准库;Django `tests/runtests.py` runner 在未指定 suite 时会从 typed ChangePlan 路径和 `tests/` 结构推导保守 scoped suite。

> 测试失败时,`pipeline_write_retry_budget`(默认 3,硬上限 5)允许自动重新规划:把失败摘要 + top-3 失败测试 + 嫌疑文件清单喂回 planner,重 plan 再 apply 再 verify。**这一步不用你手动操作**。三条早停守门避免烧 budget:`runner_missing` 一等信号(`pytest: command not found` 等)、`parser_error` 结构化信号(测试 runner 启动但没有执行到真实用例或没有产出可解析报告,常见于 collection/import 阶段环境兼容问题)、`no_tests` 结构化信号(typed runner 没有执行到任何测试,例如 suite selector / harness 不匹配)不会触发代码重规划,也不会把已应用代码硬拦成失败;它会落到 `unverified` 并保留安装提示/failure summary。pytest 零用例后升级到标准库 unittest 时,如果 unittest 只产出 `unittest.loader._FailedTest` loader 行而没有真实测试用例行,也会归入 `parser_error/unverified`,避免把 Python 版本或依赖兼容问题误当成补丁错误。项目 runner 启动前或 no-test 分支上的 plan-touched source compile fallback 是例外:Python `py_compile`/静态名称检查、Node JS `node --check`、Node TS `tsc --noEmit --pretty false`、Ruby `ruby -wc`、Java/Kotlin Maven/Gradle compile 或 bounded `kotlinc`、Swift Package `swift build --skip-build`、Go no-test package `go test -json` 失败时都属于 typed `build_failure` / `failed`,会硬拦并进入修复路径。Java/TypeScript/Rust/Go/Node/Ruby/C/C++/Swift 等 runner 解析出的 typed `BuildErrors[]` 会经过同一套 changed-line 归因:如果所有 file:line 诊断都精确落在当前 `ChangePlan` 未改行上,verdict 会降级为 `unavailable/preexisting_build_failure`,保留 P2 handoff 证据但不触发代码 replan;任一诊断落在改动行、缺少行号、路径无法归一或 plan surface 不精确,仍按 `build_failure` 硬拦。`ChangeReport.verification_status=unavailable` 是这类本地环境/测试面不可用的单一 typed verdict;workflow/batch completion 另写 `completion.verdict=unverified`,用于审计"已收敛但本地未证明";`no_tests` 会同时写入持久化 `failure_kind`,避免后端从 legacy `passed=true + NoTestsRunners` 自行推断;`passed` 只表示确实有权威执行信号通过,`failed` 才会驱动小批量 replan。`run_tests` 摘要会把这类结果渲染成 `verdict=UNAVAILABLE` 和 diagnostic rows,不会把 loader/import/parser 诊断行展示成普通 failed assertions。日志仍会透明展示模型的 `<think>` 和原始 toolcall;如果 controller 试图在 typed `unverified` terminal 后继续 `explore_code` / `verify_batch` / `replan_batch`,调度器会记录 `write controller decision normalized ... -> action=finish ... disposition=accept_unverified`,并且不会实际执行后续探索或重规划。若计划携带 `verification_probes[]`,verify 会先执行这些 typed bounded probes:探针通过则记录 `verification_probe` 执行证据和 `probe_primary_suite_skipped` 的 suite-skipped 证据,把项目级套件保留为 test-surface 诊断而不作为硬门;探针里的 assertion/raise/throw/panic、显式非零退出或被测代码运行时异常会按 typed `tests_failed` 进入小批量 replan;探针 import/syntax、runtime 缺失或顶层 probe 结构错误归入 `parser_error/unavailable` 或 `runner_missing/unavailable`,然后继续项目 runner 或按本地环境不可用交付,避免把脆弱探针脚手架当成产品代码失败。失败 handoff 会带上可直接 `read_file` 的上一轮 patch/test-surface artifact 路径、上一轮执行的唯一 runner/cwd 谱系、以及唯一失败 suite,让 replan 和下一次 verify 针对失败点收敛而不是重新调查或全仓重跑;如果失败分散在多个 suite 或多个不同执行谱系,系统不会自动合并成一个 selector。Planner dry-run 行为检查使用 `run_tests` 的 `verification_probe` 字段,`suite` 仍只表示测试 selector,不能承载 `python -c` / `node -e` / `ruby -e` / `javac` / `go run` 或 runner flags;typed validator 会拒绝这类混用,避免把脚本字符串误拼成测试选择器。通过的 planner probe 会把短 stdout/stderr 摘要内联回工具结果,失败 probe 会保留 failure summary,让下一轮规划直接消费刚测出的事实。仅靠 verification probe 通过的本地 verdict 如果改动 source 文件没有任何 P0/P1 prior context 覆盖,SWE-bench adapter 会把 `prediction_local_confidence` 降级并记录 `verification_probe_changed_source_not_context_covered`;这是 typed audit telemetry,不阻断官方 predictions JSONL 导出,也不是 apply/verify 硬门。若 replan 阶段的 typed `planner_probe` 已经证明当前 worktree 修好了失败点,controller 会记录 `no_change_required` 哨兵并复用原 plan 继续 post-apply verify,不会要求模型编造空改动或重新调查;若 replan 发出的 structured edit 对当前 worktree 是 no-op,或在最新 typed `planner_probe` 已通过后仍因 `old_text mismatch` 指向已变化的当前字节,validator 仍会拒绝该重复 edit,但会提示用 `changes: []` 进入 no-change 哨兵。pytest JSON/report 生成失败时会自动尝试禁用第三方插件的 verbose 文本 fallback;缺少 report 会被描述为"pytest 未产出请求的 JSON report",可能来自 collection/import/startup 中止或插件缺失;只有拿到用例级执行行才把结果当作真实 pass/fail,collection/import 启动失败仍保持 `parser_error`。fingerprint 比对(AppliedCount + VerifyPassed + VerifyFailed + FailureSummaryHash 完全相等 → 视为"无进展")跳过本轮 retry。

特殊场景:

| 场景 | 命令 |
|---|---|
| 跳过 verify(本地起不了集成测试) | `/approve --skip-verify` |
| 指定批某个非最新的 plan | `/approve <plan-id>` 或 `/approve --plan-id=<id>` |
| apply 通过后立刻合到某分支 | `/approve --merge-to=feature/xyz`(等价 approve + merge) |
| 已 apply 的 plan 重跑 verify(如改了环境) | `/verify <plan-id>` |

## 4.3 把改动合回主仓

`/approve` 通过后改动落在两个地方:worktree(可选保留)与主仓的固定引用 `refs/codrax/applied/<plan-id>`(**始终存在**,不依赖任何 yaml 开关)。两条落地通道任选其一:

**通道 A — ref cherry-pick(零配置,worktree 是否保留都可用):**

```
git cherry-pick refs/codrax/applied/<plan-id>
```

apply 成功的输出会原样给出这条命令(提交主题即 plan 摘要)。即使 worktree 已被清理、甚至 Run 已 blocked,这个 ref 仍固定在主仓,改动永远可取回。

**通道 B — `/merge`(REPL 流程,需要保留 worktree):**

```
[git:main]❯❯ /merge --branch=feature/refactor-bar
  在主仓上拉新分支 feature/refactor-bar 并 cherry-pick 3 个 commit?
  > y
  ✓ 已在主仓创建分支 feature/refactor-bar,cherry-pick 3 个 commit。
  下一步:cd <主仓> && git push -u origin feature/refactor-bar,然后开 PR。
  已自动切回 auto 模式 — 继续改代码时直接描述目标即可;需要强制写模式时用 /write <目标>。
```

| `/merge` 选项 | 行为 |
|---|---|
| (默认) | fast-forward 当前 branch 到 worktree 头 |
| `--branch=<name>` | 在主仓拉新分支 + cherry-pick(标准 PR 流) |
| `--include-failed` 或 `--force` | 把验证失败的 plan 也纳入候选(适合环境/CI 类失败,你 review 后决定强合) |

`/merge` 成功后:
- plan 状态从 `applied` 改成 `merged`(终态,可以下一个 plan 了)
- worktree 自动 discard
- REPL 自动切回 auto 模式

> `/merge` 需要 yaml 里 `pipeline_keep_worktree_on_success: true`,否则 worktree 在 apply 完就清掉了。worktree 被清不影响通道 A:`refs/codrax/applied/<plan-id>` 始终在主仓,`git cherry-pick` 随时可落地。

如果主仓只有 `.codrax/` 自己写入的文件(日志、记忆、blob 缓存)显示为 dirty,`/merge` 会自动把它们 `git rm --cached` 并补一条 `.gitignore` 提交,再继续合。也就是说第一次 `git init && git add -A` 误把 `.codrax/` 纳入 git 的人,不会被 `/merge` 拒绝。

## 4.3.1 能力边界

- **多仓 workspace 不支持跨子仓写**:一个 ChangePlan 只能落在一个子仓;跨子仓的修改请求会在 plan 阶段被硬性拒绝(fail-loud),按子仓拆成多次运行即可。多仓的读模式(对比/枚举/跨仓问答)不受影响。
- **写模式可以同时附加日志/trace**:`--log` / `--htrace` 的结构化解析阶段在任何模式下都按附件存在自动运行,产物供写模式的请求分类参考(如定位要修的文件);plan/apply 阶段不会把 artifact 行号当作仓库引用,主仓与 worktree 的写入边界不变。
- **裸目录(非 git 仓)**:写模式需要 `--auto-init-repo`(或 yaml `write_auto_init_repo: true`)显式授权初始化,否则在进入 apply 前拒绝。

## 4.4 失败排错

**apply 失败**(代码 patch 没打进去 / 写入冲突):
- Auto Pilot 会把结构化失败原因、diff/report/surface refs 写入 workflow,优先由 controller 小批量 replan 或 block
- 需要人工介入时,屏幕会给出状态卡;`/workflow show` 可查看当前 batch、失败证据和可恢复 refs
- 人工恢复入口:`/write <更具体的目标>` 重新启动一次目标清晰的 Auto Pilot,或 `/reject` 弃掉这版;也可以直接 `cd` 进 worktree 路径手工调,然后 `/worktree discard <plan-id>` 清掉

**verify 失败**(测试不过):
- `pipeline_write_retry_budget`(默认 3)允许 controller 自动把 typed build/test/path/line/command 证据写入 P2 handoff,再小批量 replan/apply/verify
- 重试用尽仍不过 → workflow 保留失败 report/diff/surface refs 并 fail-loud;`/workflow show` 查看证据后再决定是否缩小目标或人工处理
- 本地测试根本起不了(缺依赖、缺数据库等)时,系统会按 typed runner/surface outcome 区分 runner missing、zero tests、infra error、真实失败;跳过验证或强行合入都需要显式人工动作
- 如果 replan 或 controller 调度在已应用补丁后被 wall-time/取消打断,最终提示会明确列出 plan id、`refs/codrax/applied/<plan-id>` recovery ref、worktree 和最新验证状态,不会退化成“没有生成 ChangePlan”
- 探索阶段超时但已有 typed evidence/read-set 时,workflow 会记录 `exploration_degraded` 并继续当前 batch 的小批量规划,而不是丢掉证据重新开一轮大范围调查

**plan 阶段返回文字回答而不是改动方案**(planner 觉得这是咨询性问题):
- Auto Pilot 优先在探索、plan、replan 之间自动收敛;只有缺少 typed missing facts、预算耗尽或风险策略阻断时才暂停
- 真要继续改代码,直接补充目标或用 `/write <目标>`;只是咨询则继续普通提问即可

**目录还不是 git 仓库**(plan / apply 都需要 git 仓):
- 屏幕打印**两种(plan 阶段)或三种(apply 阶段)授权方式**任选一种:
  - 配置文件:在 `codrax.yaml` 设 `write_auto_init_repo: true`
  - 命令行:启动加 `--auto-init-repo`
  - apply 阶段额外:再次 `/approve` 时回 `y`(plan 阶段没有 y/N 提示,所以这个选项不在 plan 阶段列出)

**目录是空的,想从零创建新项目**(目录里没有任何源代码文件):
- `--auto-init-repo` 只授权"把目录变成 git 仓库",**不**授权 codrax 凭空生成文件 — 这是两件不同的事
- 想从零搭新项目需要再加 scaffold 授权,任选一种:
  - 配置文件:在 `codrax.yaml` 设 `write_scaffold_enabled: true`
  - 命令行:启动加 `--allow-scaffold`(和 `--auto-init-repo` 一起加)
- 默认关闭是为了避免在你没明确同意时擅自创建文件
- 如果目录已经有源代码、只是没 `git init`,**不需要** scaffold 授权 — `--auto-init-repo` 一项就够了

**模型反复卡住**(连续两次产不出可用方案):
- 屏幕打印"模型重复给不出可用的方案。在 codrax.yaml 换路由,或换更强的模型再试"
- 在 `providers.yaml` 把对应 agent(`planner` / `coder` / `verifier`)路由到更强的模型重试

**新生成的代码缺第三方依赖**(运行时 `ModuleNotFoundError` / `npm ERR! missing` 等):
- planner 已被要求在 `summary` 里显式列出新引入的第三方依赖 + 安装命令,**优先按那段提示装**
- 如果 planner 漏了,直接把报错信息原样贴进 codrax(它会用 LLM 推断该装哪个包),或 `/mode code` 后问 "这个 ModuleNotFoundError 怎么修?"

`/worktree list` / `/worktree discard <plan-id>` 管理保留的 worktree。

---

# 5. 配置参考

codrax 用两份 YAML,职责严格不重叠:

| 文件 | 职责 | 是否必填 |
|---|---|---|
| `providers.yaml` | LLM 凭证、每个 agent 用哪个模型 | **必填** |
| `codrax.yaml` | 语言、日志、流水线预算、各种阈值 | 可选,默认值开箱即用 |

## 5.1 providers.yaml(LLM 凭证)

### 5.1.1 静态 API Key 的最小配置

```yaml
llm:
  default:
    provider: openai
    api_key: "sk-xxx"
    model: "your-model-id"
    base_url: "https://your-endpoint/v1"  # http:// 或 https:// 都行
```

静态 API Key 模式下缺任何一个都会**拒启动 + 打印缺哪个字段**,不会偷偷连公网。

### 5.1.2 公司内 OAuth2 模型服务配置

如果公司内模型服务不是固定 `api_key`,而是先通过 OAuth2 获取 `access_token`,可以用 `auth.mode: oauth2_polling`。Codrax 会在首次需要 token 时显示授权 URL,授权成功后把 token 安全缓存到本地,后续启动会复用未过期 token。

```yaml
llm:
  default:
    provider: openai
    base_url: "https://your-model-gateway.example.com/api"

    # 显式配置 model 最稳定。若不填 model,必须配置 models_path;
    # Codrax 会展示模型列表并默认选择第一个。
    # model: "qwen3d6-35b"
    models_path: /models
    chat_completions_path: /chat/completions

    auth:
      mode: oauth2_polling
      auth_base_url: "https://your-sso.example.com/oauth"
      client_id: "your-oauth-client-id"
      scope: "your-scope"
      response_type: code
      scope_resource: "your-scope-resource"
      authorize_path: /oauth2/authorize
      callback_path: /oauth/callback
      token_path: /oauth/getToken
      access_token_header: X-Auth-Token
      access_token_format: "{token}"
      token_cache_file: "~/.codrax/auth/internal-oauth-token.json"
      poll_timeout_seconds: 1800
      poll_interval_seconds: 1
      refresh_before_seconds: 300

    headers:
      app-id: "your-app-id"
      x-snap-traceid: "@uuid_v4"

    request_extra:
      queue: true
      tool_stream: true
```

行为规则:

- `model` 显式配置时永远优先。
- 没有 `model` 时,系统只在启动/初始化 provider 时请求一次 `models_path`,展示模型列表并选择第一个非空模型名。
- `expires_in` 按秒解析,兼容字符串和数字。token 会保存 `issued_at` / `expires_at`,默认过期前 5 分钟重新认证。
- token 缓存文件权限为 `0600`,目录权限为 `0700`;日志不会打印 `access_token` 或 `refresh_token`。
- `request_extra` 只能补充 provider 私有字段;不能覆盖 `model`、`messages`、`tools`、`stream`、`tool_choice`、`max_tokens`、`thinking` 等核心协议字段。

### 5.1.3 流式开关

```yaml
llm:
  default:
    stream: true   # 默认 true:SSE 流式,REPL 里实时显示推理 / 答案
                   # 设 false:经典单次请求-响应,适合 byte-stable CI 或本地小模型
```

### 5.1.4 TLS / 自签证书

TLS 校验只作用于 `https://` 端点；但只要配置了 `tls_ca_file`,Codrax 启动时就会校验该文件是否可读且是有效 PEM。

```yaml
llm:
  default:
    tls_ca_file: /etc/pki/corp-bundle.pem    # 公司自签 CA(typical 错误:x509: cert signed by unknown authority)
    # tls_insecure_skip_verify: true         # 核武器:完全跳过证书验证,启动时会打高亮警告
```

`tls_ca_file` 可以写绝对路径,也可以写相对路径。相对路径从 codrax 可执行文件所在目录开始计算,不从当前仓库目录或启动命令的工作目录计算。一旦配置,文件必须可读且包含有效 PEM 证书;否则 Codrax 会在 REPL/CLI 启动阶段提示原因并停止,不会静默回退到系统证书池。

### 5.1.5 sizing / 超时 / 重试(每个字段都可选)

```yaml
llm:
  default:
    context_window: 128000              # 模型最大输入 tokens(默认 128000)
    max_output_tokens: 0                # wire-level max_tokens;0 = 不发,服务器用模型 ceiling
    max_output_fraction: 0              # 替代形式:context_window × fraction
    request_timeout_seconds: 240        # 非流式 HTTP 超时
    retry_max_attempts: 6               # 429 / 5xx 重试上限,默认 6
    stream_stall_timeout_seconds: 120   # SSE 启动后 N 秒无新字节,主动中止
    stream_first_byte_timeout_seconds: 40  # 请求被接受后 N 秒还没首字节(provider 死锁/cold-start),中止
    think_aloud: true                   # 是否要求模型在工具调用旁夹 1-2 句推理摘要
    thinking_mode: auto                 # provider 原生 thinking:auto|disabled|enabled|provider_default
```

`think_aloud` 和 `thinking_mode` 是两件事:

- `think_aloud`:只影响 Codrax 发给模型的 prompt,让模型在工具调用旁输出 1-2 句进度摘要;不会打开任何 provider 原生 reasoning / thinking API。
- `thinking_mode`:控制 provider wire JSON。默认 `auto`:对官方 DeepSeek endpoint 发送 `thinking: {type: disabled}`,其它 OpenAI-compatible provider 不发送私有 thinking 字段。
- `disabled`:总是请求关闭 provider 原生 thinking;`provider_default`:完全不发送 thinking 字段,交给 provider 默认行为。
- `enabled`:仅在你明确要测试 provider 原生 thinking 时使用。对官方 DeepSeek endpoint,Codrax 会保留并回传 `reasoning_content`,并在原生 thinking 开启时不发送 `tool_choice`,以满足 DeepSeek tools 兼容要求。除非已验证收益,生产环境建议保持 `auto`。

### 5.1.6 本地小模型 tool-call 兼容(可选)

对 OpenAI-compatible 本地 / 小模型，先只打开审计模式观察：

```yaml
llm:
  default:
    recover_text_tool_calls: true       # 文本里的完整工具调用 envelope → protocol tool_calls
    tool_param_compat:
      mode: audit                       # off | audit | repair; audit 不改执行
      split_string_arrays: true
```

确认日志里的 `[tool_param_compat] ... audit repairable` 都是机械类型错误后，再对需要的 provider / agent 切到 `repair`。它只按工具 JSON Schema 修确定性问题，例如 `"offset":"146"` → `146`、JSON 字符串里的 array/object 解包、schema 声明为 object array 时修复 string-carrier 里丢失 `{` 的对象片段、`"a,b"` → `["a","b"]`；不会补缺失字段、猜 path、删 unknown 字段或修 prose。远程大模型不需要打开 `recover_text_tool_calls` 的宽兼容档；系统会默认处理“assistant content 正好是完整显式工具调用 JSON envelope”的安全运输修复。

### 5.1.7 每个 agent 用不同模型(可选)

每个 agent 都从 `default` 继承缺省值,只覆盖你要变的字段:

```yaml
llm:
  default:
    provider: openai
    api_key: "sk-xxx"
    base_url: "https://api.openai.com/v1"
    model: "gpt-4o"

  agents:
    # 读模式 4 个主流水线 agent
    analyzer: { model: "gpt-4o-mini" }       # 分类器,可用便宜模型
    explorer: {}                              # 继承 default
    extractor: { model: "gpt-4o-mini" }
    finalizer: {}                             # 继承 default

    # 写模式 4 个 agent(write_analyzer 是 plan 之前的请求分类器)
    write_analyzer: { model: "gpt-4o-mini" }  # 写模式专属请求分类
    planner: {}
    coder: {}
    verifier: { model: "gpt-4o-mini" }

    # 条件前置
    log_triager: { model: "gpt-4o-mini" }
    perf_triager: { model: "gpt-4o-mini" }

    # 辅助
    chitchat_responder: { model: "gpt-4o-mini" }
    chitchat_classifier: { model: "gpt-4o-mini" }   # 每轮 1 次,务必廉价
    memory_summarizer: { model: "gpt-4o-mini" }
    reflector: { model: "gpt-4o-mini" }             # verify 失败时的 critic
    plan_critic: { model: "gpt-4o-mini" }           # apply 前 plan review(可选)
    env_recommender: { model: "gpt-4o-mini" }       # 环境诊断推荐
```

> agent 槽位**不是必填**;你只需要为想变的 agent 填字段,其他自动继承。

每个 agent 槽都支持 `<name>_fallback`:主 provider 失败(429/网络抖)时自动切到 fallback,适合多区域容灾:

```yaml
agents:
  finalizer:
    provider: openai
    base_url: "https://api.openai.com/v1"
    model: "gpt-4o"
  finalizer_fallback:
    provider: openai
    base_url: "https://backup.deepseek.com/v1"
    model: "deepseek-chat"
```

## 5.2 codrax.yaml(运行参数)

完整的注释模板见 `codrax.yaml.example`,这里按用途分组列出最常用的:

### 日志 / 记忆 / 缓存

| 键 | 默认 | 作用 |
|---|---|---|
| `log_dir` | `<CWD>/.codrax/logs/<repo>-<hash>` | 日志目录 |
| `log_level` | `debug` | error / warning / info / debug |
| `log_stdout` | `false` | 日志同时打到 stdout(调试用) |
| `log_max_files` | 7 | 日志轮转保留份数 |
| `memory_dir` | `<CWD>/.codrax/memory/<repo>-<hash>` | 多轮对话记忆 |
| `cache_dir` | 平台默认 cache 目录 | repomap 索引缓存 |
| `search_exclude_roots` | 空 | 仓库相对 root 列表,从宽泛 search/list/repo_map 扫描中排除客户仓生成目录、运行产物或报告目录;显式 target 仍可读取 |
| `output_dump_enabled` | `true` | 每次 read 模式答案落盘到 `<CWD>/.codrax/output/<时间戳>-<pid>.md`,并生成同名 `.html` 自包含预览;Markdown 两段 `# 问题` / `# 回答`,模型原文轻度排版 |
| `output_max_files` | 10 | output 目录按 Markdown 保留最近 N 份,旧 `.md` 及同名 `.html` 按 mtime 自动删 |
| `markdown_preview_server` | `auto` | REPL 在答案落盘后懒启动本地 Markdown 预览服务并显示浏览器 URL;`off` 关闭 |
| `markdown_preview_host` | `0.0.0.0` | 预览服务监听地址;远程终端可保持默认全监听,本机私有可设 `127.0.0.1` |
| `markdown_preview_port` | `0` | 预览服务端口;`0` 表示系统随机分配可用高位端口 |
| `lang` | `zh` | 答案默认语言;`off` 关闭 |

### 大仓内存韧性

扫描超大仓库(如 Linux 内核:约 6.4 万个待解析源文件)时,repomap 全量扫描在内存和 CPU 两个维度都会压满小机器:进程内存可能超出宿主 RAM 被 OOM 杀掉,所有核心被占满又可能饿死 sshd 导致远程 SSH 断连。内存相关的键默认即开、无需配置;CPU 那个键(`repomap_scan_reserve_cpus`)按需开启。详见 `docs/design/large_repo_memory_resilience.md`。

商用/受限宿主建议显式设置运行侧内存保护线,并低于容器或宿主硬上限留出余量:

```bash
GOMEMLIMIT=6GiB codrax --repo /path/to/big-repo --request "..."
```

`GOMEMLIMIT` 是 Go 堆目标,不是 RSS 硬天花板;它能让 GC 更早介入,但 mmap、C 栈、tree-sitter/native parser 内存仍需要额外余量。需要真正硬限制时,用容器/cgroup/ulimit 设置进程内存上限,再把 `GOMEMLIMIT` 设在该上限以下。yaml 的 `memory_soft_limit_*` 是默认保护,显式环境变量适合每次大仓运行按宿主容量精确控制。

| 键 | 默认 | 作用 |
|---|---|---|
| `memory_soft_limit_enabled` | `true` | 启动时设软堆上限(GOMEMLIMIT),让 GC 在宿主内存耗尽前提前回收。环境变量 `GOMEMLIMIT` 优先于本组所有键 |
| `memory_soft_limit_fraction` | `0.8` | 目标占检测到的宿主 RAM 的比例,范围 `(0,1]`。宿主 RAM 检测支持 Linux(`/proc/meminfo`)/ macOS(`hw.memsize`)/ Windows(`GlobalMemoryStatusEx`) |
| `memory_soft_limit_bytes` | `0` | 直接以字节指定软上限;`>0` 时跳过宿主 RAM 检测,低于 512 MiB 抬到该下限。RAM 检测覆盖不到的平台用这个键 |
| `repomap_resume_interrupted_scan` | `true` | 全量扫描复用上次被中断(如被 OOM 杀掉)的扫描已落盘的 chunk,经内容哈希校验后跳过重解析;重试逐步收敛而非从零重来。覆盖全部 15 种语言 |
| `repomap_scan_reserve_cpus` | `0` | 设为 `>0` 时,扫描期间把 `GOMAXPROCS` 压到 `核数 - 该值`,让整个 Go 运行时(解析 worker、**GC worker**、图构建/排序)都留出这么多核心给交互进程,避免占满 CPU 饿死 sshd 导致远程 SSH 断连。损失一些扫描吞吐(4 核留 1 ≈ 慢 25%),故默认 `0` 按需开启;小远程机掉 SSH 时设 `1` |
| `repomap_parse_timeout_enabled` | `true` | 是否启用单文件 tree-sitter 解析安全阀。默认开启,避免生成型/病态大文件让整仓扫描长期卡住 |
| `repomap_parse_timeout_seconds` | `120` | 单个 tree-sitter 源文件解析的安全阈值。超时文件降级为 path-only 并记录 fallback reason。设 `0` 或把 `repomap_parse_timeout_enabled` 设为 `false` 可关闭该安全阀 |

### 流水线预算

| 键 | 默认 | 作用 |
|---|---|---|
| `pipeline_max_steps` | 50 | 单次 Run 总步数上限 |
| `pipeline_max_retries_per_stage` | 2 | 每个阶段最多重试几次 |
| `pipeline_max_steps_ceil` | 100 | 多 sub-topic 动态扩容的硬上限 |
| `pipeline_transient_retry_budget` | 3 | 流式截断 / 网络抖 后的瞬时重试 |
| `pipeline_force_finalize_attempts` | 3 | 调度卡住后强制 finalize 的重试数 |

### 写模式

| 键 | 默认 | 作用 |
|---|---|---|
| `write_enabled` | `true` | **写模式 kill switch**;显式设 false 时任何写 workflow/plan/apply/verify/merge 都拒绝。REPL auto 可通过结构化 `route=write` 进入 Auto Pilot;低/中风险可自动 apply+verify,高风险暂停审批,critical 拒绝 |
| `write_auto_init_repo` | `false` | 允许把目标目录初始化为 git 仓库(`git init` + 空 commit;等价 `--auto-init-repo`,持久版) |
| `write_scaffold_enabled` | `false` | 允许在空目录里凭空生成新文件(从零创建项目;等价 `--allow-scaffold`,持久版)。空目录场景需要和 `write_auto_init_repo` 同时开启 |
| `write_approval_policy` | `auto_safe` | REPL `/approve` 审批策略: `manual` 全部人工确认;`auto_safe` 低/中风险自动推进、高风险人工确认、critical 拒绝;`auto_low_only` 仅低风险自动推进。导出声明行/API 面变更是中风险信号,危险路径/内容/大范围结构信号才升级为高风险或 critical |
| `write_auto_approval` | `false` | 兼容旧布尔配置;仅未设置 `write_approval_policy` 时生效。`true` 映射 `auto_safe`,`false` 映射 `manual` |
| `write_plan_dir` | `<runtime>/plans` | ChangePlan JSON 落盘目录 |
| `write_workflow_engine` | compatibility-only | 兼容旧本地配置的已废弃键;写模式始终使用 controller-first 动态 DAG、持久 workflow run 和主动状态卡;`/workflow show/list/resume/clear` 仅作高级审计/恢复 |
| `pipeline_write_retry_budget` | 3 | verify 失败后自动重 plan 的最大次数 |
| `pipeline_write_retry_budget_ceil` | 5 | 上面那个 budget 的硬上限 |
| `pipeline_baseline_capture_enabled` | `false` | 写模式开启前先跑一次基准测试,用于回归判定(双倍测试时间) |
| `pipeline_keep_worktree_on_success` | `false` | apply 通过后保留 worktree(`/merge` 必需) |
| `pipeline_lint_enabled` | `true` | emit_change_plan 时对新建文件跑 V5 静态检查(gofmt / ruff / node --check 等;工具链缺失会静默跳过) |
| `worktree_keep_ttl_hours` | 168 | 保留下来的 worktree 在启动时按 mtime 老化清理(0 关闭) |
| `worktree_keep_max_count` | 20 | 老化后还要按 LRU 配额裁剪(0 关闭) |
| `verify_mem_limit_mb` | 2048 | 每次 `run_tests` / `exec_command` 的内存硬上限(MiB);0 用 package 默认 |
| `verify_cpu_limit_seconds` | 600 | 同上,CPU 时间(秒);跟 wall 超时是两回事 |
| `verify_wall_timeout_seconds` | 900 | 每次 `run_tests` 套件执行的默认墙钟超时(15 分钟);单次工具调用的 `timeout_seconds` 仍可覆盖 |

### 闲聊 / 本地转换

| 键 | 默认 | 作用 |
|---|---|---|
| `chitchat_enabled` | `true` | `/chat` 命令 + 闲聊响应器开关 |
| `chitchat_classifier_enabled` | `true` | 自动 turn-policy 分类器(每轮 1 次便宜 LLM 调用) |
| `chitchat_recall_default_limit` / `chitchat_recall_max_limit` | 5 / 10 | recall_memory 工具的默认 / 上限 |
| `chitchat_list_default_limit` / `chitchat_list_max_limit` | 10 / 30 | list_memory 同 |

### 附加日志 / trace

| 键 | 默认 | 作用 |
|---|---|---|
| `log_attach_max_bytes` | `536870912`(512 MiB) | `--log` / `/log` / 自动检测的总字节上限 |
| `trace_attach_max_bytes` | `536870912`(512 MiB) | `--htrace` / `/htrace` 的字节上限(独立) |
| `log_triage_enabled` | `true` | log_triage 预阶段 |
| `log_triage_two_step_enabled` / `log_triage_two_step_bytes` / `log_triage_two_step_coverage` | `true` / 32 KiB / 0.3 | 大日志的两步 fallback |
| `log_triage_max_llm_calls` | 12 | 单次 Run log_triage LLM 调用上限 |
| `log_triage_source_prefix` | `""` | 等价 `--log-source-prefix`(yaml 持久版) |
| `perf_triage_enabled` | `true` | perf_triage 预阶段(同上结构) |

### MCP 外部工具配置参考

| 键 | 默认 | 作用 |
|---|---|---|
| `mcp_max_servers` | 8 | 启动时最多加载多少个 MCP server;这是 server 个数上限,不是并发数或每个 server 的工具数上限 |
| `mcp_servers` | `[]` | 可选 stdio MCP server 列表。为空时不改变 prompt、工具 schema、调度或答案落地 |
| `mcp_servers[].name` | 必填 | server 名;模型可见工具名会变成 `<server>__<tool>` |
| `mcp_servers[].transport` | `stdio` | 当前支持 `stdio`;其它 transport 会 fail-loud |
| `mcp_servers[].command` / `args` | 必填 / `[]` | 启动 MCP 子进程的命令和参数 |
| `mcp_servers[].inherit_env` | `false` | 是否把当前环境变量传给 MCP 子进程;默认不传密钥 |
| `mcp_servers[].env` | `{}` | 传给 MCP 子进程的白名单环境变量 |
| `mcp_servers[].startup_timeout_ms` | 3000 | initialize / tools/list 启动预算 |
| `mcp_servers[].call_timeout_ms` | 10000 | 单次 tools/call 或 resources/read 调用预算 |
| `mcp_servers[].max_response_bytes` | 4194304 | 单条 MCP 响应最大字节数;大输出应由 server 自己转成摘要 + 外部 payload ref |
| `mcp_servers[].operation_provider` | `false` | 可选:显式声明该 MCP server 可作为 operation/artifact provider。只用于 REPL operation 计划器的能力匹配 |
| `mcp_servers[].operation_kinds` / `operation_surfaces` | `[]` / `[]` | provider 支持的操作类型和目标界面,如 `presentation_generation` + `slides` |
| `mcp_servers[].operation_side_effects` | `[]` | provider 可能产生的副作用,如 `local_file_write` / `browser_ui` |
| `mcp_servers[].operation_tool` | 空 | 可选:该 server 中真正执行 operation 的 MCP tool 名。未配置时只显示计划和 provider 能力,不会 `/approve` 执行 |
| `mcp_servers[].operation_description` | 空 | 可选:给 operation planner 的紧凑能力说明。用于说明 provider 擅长做什么,不是系统指令 |
| `mcp_servers[].operation_input_schema` | 空 | 可选:给 operation planner 的参数契约摘要。用于帮助模型构造动态参数,实际执行仍由 `operation_tool` 的 schema 和 provider 校验 |
| `mcp_servers[].operation_examples` | `[]` | 可选:1-3 条典型用法示例,帮助模型选择最合适的 provider |
| `mcp_servers[].operation_lazy_start` | `false` | 可选:仅当 `operation_provider=true` 时生效。设为 true 后启动 REPL 时不拉起该 MCP server,只有 operation 需要调用它时才启动 |
| `mcp_servers[].operation_requires_confirmation` | `false` | provider 是否强制要求显式确认后才能执行。即使为 false,高风险 side effect 仍会由 operation policy 拦截或要求批准 |

operation provider 执行完成后,结果会进入独立的 operation handoff 和 operation memory:包括 provider 名、工具名、操作类型、摘要、外部观测数量、payload ref、artifact ref 等。下一次 operation 规划可以参考这些信息继续工作,例如“刚才 PPT provider 生成了哪个文件,下一步如何验证或修改”。这些信息仍是外部操作结果,不会被当成当前源码 citation;代码/trace/log 混合任务仍由 typed 路由决定是否进入源码或 runtime trace 管线。

如果 provider 返回很大的输出,codrax 只在 REPL 面板显示短摘要和少量 ref,完整内容通过 payload/artifact ref 保留。provider 不应该把大文件全文塞进 MCP text;更好的做法是返回摘要 + 明确的文件路径、资源 URI 或 rowset/payload ref。

### operation_skills 配置参考

外部 Skill 是 Codrax 的通用电脑操作/制品生成扩展入口。它适合把本地脚本、二进制、公司内工具、PPT/文档/表格生成器、浏览器/桌面自动化包装成一个可审批、可懒加载、可 handoff 的 operation provider。

外部 Skill 和其它扩展方式的边界:

- `operation_skills[]`: 本地 manifest 型外部 Skill。启动时只读 descriptor,需要调用时才启动本地命令。
- `mcp_servers[].operation_provider=true`: MCP server 型外部 Skill。适合已有 MCP server 或需要远程/多工具协议的场景。
- 内置 prompt skills: 只提供模型使用说明和工作流知识,不代表可执行的本地操作 provider。

外部 Skill 不会抢占源码、trace、log 或写代码管线。只有 typed operation route 识别为电脑操作、制品生成、外部 workflow 等场景时,operation planner 才会看到这些 provider 能力;低风险节点会自动推进,危险节点会要求批准或被策略拒绝。

如果你不想写 MCP server,可以用 `operation_skills[]` 把本地脚本、二进制或公司内工具包装成 operation provider。它和 MCP operation provider 的区别是:

- 启动时只读 `codrax.yaml` descriptor,不会启动脚本。
- 模型只看到能力摘要、输入契约和示例,看不到 `command` / `env` 作为 prompt 指令。
- `description` / `when_to_use` / `when_not_to_use` / `workflows[]` 会进入 operation planner 的能力摘要,只用于帮助模型按 typed intent 选择 provider,不是系统指令。
- 只有 typed operation route 命中该 provider,codrax 才会按 operation policy 启动本地命令;可安全执行的低风险节点会自动推进,危险节点会等待批准或被拒绝。
- 结果进入 operation handoff / operation memory,不会当成当前源码 citation。

一个带动态参数和 workflow catalog 的配置:

```yaml
operation_skills:
  - name: manual_reader
    description: "Read a manual, extract command templates, and hand off."
    when_to_use:
      - "Use when the task requires learning an unfamiliar CLI/tool manual before generating an artifact."
    when_not_to_use:
      - "Do not use for pure source-code explanation without an operation goal."
    operation_kinds: ["presentation_generation", "artifact_generation"]
    operation_surfaces: ["local_file"]
    operation_side_effects: ["local_file_write"]
    operation_requires_confirmation: true
    workflows:
      - name: manual_to_deck
        summary: "Read a manual, extract key commands, then call a deck builder."
        entry: true
        operation_kind: external_skill_workflow
        target_surface: slides
        next_providers: ["skill:ppt_builder"]
        return_provider: "skill:manual_reader"
        steps:
          - id: read_manual
            provider: "skill:manual_reader"
            operation_kind: external_skill_workflow
            target_surface: local_file
            description: "Read and summarize the manual."
          - id: build_deck
            provider: "skill:ppt_builder"
            operation_kind: presentation_generation
            target_surface: slides
            description: "Generate slides from the extracted summary."
    input_schema: |
      {"manual_path":"string","output_path":"string"}
    examples:
      - "Read ./docs/tool.md and generate ./out/tool-workflow.pptx."
    output_contract:
      artifact_refs: true
      payload_ref: true
      next_actions: true
      return_action: true
      workflow_state: true
    command: "./tools/manual_reader_skill"
    args: ["--json"]
    input_mode: stdin_json
    timeout_ms: 60000
    max_output_bytes: 262144
```

这里 `workflows[].entry: true` 会让同一个 `skill:manual_reader` 额外声明一个 typed provider 能力,例如 `operation_kind=external_skill_workflow`、`target_surface=slides`。当模型根据用户意图输出这个 typed intent 时,系统可以直接匹配 provider;不会通过 `description` 关键字做硬路由。

`stdin_json` 模式下,本地 skill 会收到如下动态参数 envelope:

```json
{
  "request": "用户原始请求",
  "operation": "presentation_generation",
  "operation_kind": "presentation_generation",
  "target_surface": "slides",
  "risk_level": "low",
  "side_effects": ["local_file_write"],
  "requires_confirmation": true,
  "provider": "skill:local_ppt",
  "tool": "run",
  "repo_root": "/abs/workspace"
}
```

如果这是被上一个 skill 排队的子流程, envelope 还会包含:

```json
{
  "input": {
    "source_payload_ref": "out/manual-notes.md"
  },
  "workflow_state": {
    "workflow_id": "manual-to-deck-001",
    "step": "manual_extracted",
    "return_to": "skill:manual_reader",
    "data": {
      "source_payload_ref": "out/manual-notes.md"
    }
  },
  "workflow_depth": 1
}
```

本地 skill 可以返回普通文本,也可以返回结构化 JSON:

```json
{
  "success": true,
  "summary": "Created 6 slides and verified render.",
  "artifact_refs": ["out/summary.pptx"],
  "verification_status": "verified",
  "verification_summary": "PPTX rendered without layout overflow.",
  "observations": ["used template default.pptx"]
}
```

本地 skill 也可以返回一个受控的后续 workflow 动作,用于“先读说明/提炼资料,再调用另一个 provider 生成 PPT/文档/表格”等多 skill 编排:

```json
{
  "success": true,
  "summary": "Read the manual and extracted a slide outline.",
  "artifact_refs": ["out/manual-notes.md"],
  "next_actions": [
    {
      "provider": "skill:ppt_builder",
      "operation_kind": "presentation_generation",
      "target_surface": "slides",
      "risk_level": "medium",
      "side_effects": ["local_file_write"],
      "requires_confirmation": true,
      "request": "Create slides from the extracted notes.",
      "input": {
        "source_payload_ref": "out/manual-notes.md",
        "output_path": "out/deck.pptx"
      }
    },
    {
      "provider": "skill:deck_verifier",
      "operation_kind": "presentation_generation",
      "target_surface": "slides",
      "risk_level": "low",
      "request": "Verify the generated deck.",
      "input": {
        "deck_path": "out/deck.pptx"
      }
    }
  ],
  "return_action": {
    "provider": "skill:manual_reader",
    "operation_kind": "artifact_generation",
    "target_surface": "local_file",
    "request": "Compose the final workflow report.",
    "input": {
      "deck_path": "out/deck.pptx"
    }
  },
  "workflow_state": {
    "workflow_id": "manual-to-deck-001",
    "step": "manual_extracted",
    "return_to": "skill:manual_reader",
    "data": {
      "source_payload_ref": "out/manual-notes.md"
    }
  }
}
```

`next_actions` 和 `return_action` 是 provider 给 Codrax 的后续建议,不是绕过策略的执行授权。Codrax 会匹配已配置的 operation provider,把所有有效 `next_actions` 写入 workflow DAG 并按顺序串行排队;如果 `return_action` 有效,会作为 return edge 排在子动作之后。可执行且低风险的节点会继续自动推进;遇到风险节点、显式 gate、策略拒绝、失败或预算耗尽时才停下。未匹配 provider、超出深度预算或格式不完整的动作只会作为 operation 诊断显示,不会回落到源码分析或普通 trace/log 管线。为了兼容小模型和脚本输出,系统也接受 `next_action` 单对象、`provider_name`、`kind`、`surface`、`requires_approval`、`args` / `arguments`,以及 `return_to_action` / `callback_action` 等常见别名。

operation 工作流相关 REPL 命令:

- `/approve`:执行当前 workflow action。
- `/workflow show`:查看当前 workflow 的节点、边、队列和当前 action；如果同时存在 active write workflow,会优先展示 write workflow,这时用 `/operation show` 查看 operation workflow。
- `/workflow resume` / `/workflow clear`:作用于 write workflow run；operation workflow 不使用这两个命令。
- `/workflow cancel`:取消当前 operation workflow。
- `/operation show`:也会优先展示当前 workflow 状态。

如果 stdout / stderr 很大,codrax 会把完整输出写到 `.codrax/operation/`,面板只展示短预览和完整输出路径。

### 电脑操作 / 制品生成路由

| 键 | 默认 | 作用 |
|---|---|---|
| `operation_skills` | `[]` | 本地 operation skill 列表。为空时不改变源码、trace/log、MCP 或写模式流程 |
| `operation_skills[].name` | 必填 | 本地 provider 名,operation planner 中显示为 `skill:<name>` |
| `operation_skills[].description` | 空 | 进入 operation planner 的短能力说明。只做软选择,不是系统指令 |
| `operation_skills[].when_to_use` / `when_not_to_use` | `[]` / `[]` | 进入 operation planner 的适用/不适用条件,帮助模型选择或避开 provider。不能作为 Go 硬路由条件 |
| `operation_skills[].operation_kinds` / `operation_surfaces` | `[]` / `[]` | 支持的 typed operation 类型和目标界面,如 `presentation_generation` + `slides` |
| `operation_skills[].operation_side_effects` | `[]` | 可能副作用,用于 planner 展示和审批心智,如 `local_file_write` |
| `operation_skills[].operation_description` | 空 | 给 operation planner 的能力说明。不是系统指令 |
| `operation_skills[].operation_input_schema` | 空 | 给 operation planner 的参数契约摘要。真正参数由本地 skill 自己校验 |
| `operation_skills[].operation_examples` | `[]` | 典型用法示例,帮助模型选择 provider |
| `operation_skills[].input_schema` / `examples` | 空 / `[]` | 推荐的新字段名,语义同 `operation_input_schema` / `operation_examples` |
| `operation_skills[].workflows` | `[]` | 懒加载 workflow catalog。进入 operation planner,用于描述可选子流程和 skill-to-skill handoff |
| `operation_skills[].workflows[].entry` | `false` | 为 true 且配置了 `operation_kind` 时,该 workflow 会额外生成一个 typed provider descriptor |
| `operation_skills[].workflows[].operation_kind` / `target_surface` | 空 / 空 | workflow entry 的 typed 能力。硬匹配依赖这些字段,而不是散文描述 |
| `operation_skills[].workflows[].next_providers` / `return_provider` | `[]` / 空 | 给模型说明常见后续 provider 和回跳 provider。实际后续调用仍由 provider 返回的 `next_actions` / `return_action` 决定 |
| `operation_skills[].output_contract` | 空 | 描述该 skill 可能返回 `artifact_refs`、`payload_ref`、`next_actions`、`return_action`、`workflow_state` 等结构字段 |
| `operation_skills[].operation_requires_confirmation` | `false` | 是否必须用户批准后执行。默认 false;高风险 side effect 仍会由 operation policy 拦截或要求批准 |
| `operation_skills[].operation_lazy_start` | `true` | 启动时只读 descriptor,需要调用时才启动命令 |
| `operation_skills[].command` / `args` | 必填 / `[]` | 本地命令和参数。命令不经过 shell 展开 |
| `operation_skills[].input_mode` | `stdin_json` | `stdin_json` 把动态参数写入 stdin;`args_json` 把 JSON 作为最后一个参数 |
| `operation_skills[].work_dir` | 仓库根 | 命令工作目录。相对值按仓库根解析 |
| `operation_skills[].inherit_env` | `true` | 是否继承当前环境变量。可用 `env` 覆盖或补充 |
| `operation_skills[].timeout_ms` | `30000` | 本地 skill 单次执行超时 |
| `operation_skills[].max_output_bytes` | `262144` | stdout inline 预览上限。超过后完整输出落到 `.codrax/operation/` |
| `operation_route_enabled` | `true` | REPL 分类器识别 PPT、文档、表格、浏览器/桌面操作、外部 skill workflow、通用命令行操作等请求时,进入独立 operation 路径。它不会把操作请求误转入源码分析流水线 |
| `operation_command_auto_approve` | `true` | operation 专用自动审批开关,和代码写模式 `write_enabled` 无关。默认开启:结构化命令没有命中高危/灾难性规则时自动推进;高危等待批准,灾难性操作直接拒绝 |
| `operation_command_auto_low_risk` | `true` | 保守低风险自动批准。若把 `operation_command_auto_approve` 设为 `false`,则只自动执行确定只读查询和无覆盖目录创建;再设为 `false` 可改回全部等待 `/approve` |
| `operation_command_timeout_ms` | `120000` | 单个命令 step 的默认超时 |
| `operation_command_output_preview_bytes` | `32768` | 命令输出在 REPL/memory 中保留的预览字节数。更大的输出会截断预览,完整输出落到 `.codrax/operation/` |
| `operation_command_allowed_write_roots` | `[]` | 可选写入根目录白名单。非空时,带本地写入副作用的命令必须能从结构化参数证明写入目标落在这些目录内,否则策略阻止 |
| `operation_command_network_policy` | `manual` | 网络类命令策略:`manual` 等待批准,`deny` 直接阻止。只影响 operation 命令计划,不影响普通分析工具 |
| `operation_command_install_policy` | `manual` | 安装/卸载类命令策略:`manual` 等待批准,`deny` 直接阻止 |
| `operation_command_overwrite_policy` | `manual` | 覆盖写入策略:`manual` 等待批准,`deny` 阻止结构化标记为覆盖或明显强制覆盖的步骤 |
| `data_task_max_repair_rounds` | `6` | 数据处理脚本失败后的最大自修复轮数。只影响 data lane,不影响源码分析、trace/log、operation 或写模式 |
| `data_task_max_data_rounds` | `18` | 数据处理 execute/evaluate/continue 的最大批次数。复杂多材料任务可适当调高,系统仍有硬上限防止无限循环 |

通用命令行操作的第一版工作流:

1. 用户用自然语言提出需求,例如"查询当前 node 版本"、"创建 reports 目录"、"把 a.log 移到 logs/"。
2. Codrax 先生成 typed 命令计划,并用策略判断 `ready` / `needs_clarification` / `blocked`。
3. 信息不足时进入 `needs_clarification`,系统会提问,不会猜命令。
4. 低风险自动执行:确定只读查询和无覆盖目录创建默认自动执行,中间过程只显示简短进度。
5. 危险动作才打断:未知命令、shell、网络、安装/卸载、覆盖、删除或无法证明安全的写入会等待 `/approve`,特别高危动作直接拒绝。
6. 每一轮命令执行结果会在过程面板里显示;最终答案只保留面向用户目标的提炼报告,不再附带整段执行详情。
7. 网络、安装/卸载、覆盖和写入目录限制只消费 typed side effects 与命令参数,不会通过用户问题里的关键词来判断意图。

### 环境诊断与推荐

| 键 | 默认 | 作用 |
|---|---|---|
| `env_recommend_enabled` | `true` | 在写模式 / runner missing 等场景跑环境诊断 |
| `env_recommend_llm_enabled` | `true` | 用 LLM 合成安装建议(关掉则只走 docslink 兜底) |
| `env_recommend_llm_timeout_sec` | 5 | LLM 诊断单次超时 |
| `env_probe_network` | `true` | 启动探测时探一下网络可达性 |
| `env_cache_ttl_days` | 90 | 环境探测结果缓存天数 |
| `recommend_global_install` | `false` | 是否允许 LLM 推荐 sudo / 全局安装 |

### REPL / repomap

| 键 | 默认 | 作用 |
|---|---|---|
| `repl_paste_fold_min_chars` | 120 | 粘贴长度 ≥ 这个字符数(Unicode rune 数,不是字节)折叠成 `[Pasted text #N …]` 占位符;多行粘贴无视长度直接折叠 |
| `repomap_tier_warn_ratio` | 0.30 | 单语言 Tier-2+ 降级率 ≥ 这个值时打 INFO("trending toward extractor maintenance") |
| `repomap_tier_alert_ratio` | 0.50 | 同上,但更高,触发 WARN("consider extractor / grammar update");ArkTS 内部用 0.40 / Cangjie 用 0.50,不受这个旋钮影响 |

### 颜色

颜色默认只用于交互式 REPL 的进度、预览和 diff 渲染。单次 CLI 请求默认保持无 ANSI 的纯文本,适合 pipe 到文件、CI 日志或其它命令;如果用户显式传 `--color=always`,则视为明确允许 CLI 过程输出带 ANSI。

`--color={auto,always,never}` 控制交互式渲染和显式 CLI 颜色通道;`NO_COLOR=1` 环境变量永远强制关闭(no-color.org 标准)。

### 还有更多

`gate_*` / `analysis_*` / `explore_*` / `agent_*` / `cgec_*` / `evidence_*` / `summary_cap_*` / `memory_*` / `memory_policy_*` / `citation_quote_max_chars` / `agent_prior_conversation_policy` / `agent_investigation_complete_policy` / `agent_planner_soft_iter_cap` 等几十个调参旋钮 — 它们影响 LLM 的 internal heuristic 阈值,**新手通常不需要改**。完整字段见 `codrax.yaml.example`,每个都有行内注释说明。

## 5.3 配置查找顺序

启动时按这个顺序找 `codrax.yaml`,首个命中即用:

1. `$CODRAX_SETTINGS` 环境变量指向的路径
2. `<可执行文件目录>/codrax.yaml`(推荐)
3. `<可执行文件目录>/codrax/codrax.yaml`
4. 兼容保留的 `config/` 路径(启动时会提示迁移到推荐位置)

`providers.yaml` 默认在二进制同目录;用 `--providers /path/to/providers.yaml` 覆盖。

**优先级**(低到高):

```
代码默认值 < codrax.yaml < 命令行 flag
```

只有这些 flag 会覆盖 yaml:`--repo` / `--branch` / `--multi-repo` / `--lang` / `--log-level` / `--log-dir` / `--log-stdout` / `--memory-dir` / `--cache-dir` / `--pipeline-max-steps` / `--pipeline-max-retries` / `--pipeline-max-stage-visits` / `--max-prescan-rounds` / `--log` / `--log-text` / `--log-source-prefix` / `--htrace` / `--htrace-text` / `--atrace` / `--atrace-text` / `--chitchat-classifier` / `--mode` / `--data-resume` / `--write-phase` / `--auto-apply` / `--plan-out` / `--plan-file` / `--auto-init-repo` / `--allow-scaffold` / `--color` / `--mermaid-render`。

---

# 6. REPL 命令参考

REPL 启动后,任何以 `/` 开头的输入是斜杠命令;TAB 自动补全。`/help` 默认只显示常用入口和 Auto Pilot 主路径,`/help all` 才展开完整命令和子命令表。

| 命令 | 用途 |
|---|---|
| `/help` | 精简帮助:常用入口、Auto Pilot 主路径、审计/恢复提示 |
| `/help all` | 完整命令和子命令表 |
| `/exit` / `/quit` | 退出 REPL |
| `/version` | 打印构建版本 |
| `/history` | 显示最近若干轮对话 |
| `/compact` | LLM 压缩老回合,腾出 buffer |
| `/clear` | 二次确认后清空所有 memory |
| `/log <path>` | 加载文件作为附加日志 |
| `/log append <path>` | 追加另一段 |
| `/log show` / `/log clear` | 查看 / 清除 |
| `/log` | 进粘贴模式,贴完 `/end` |
| `/htrace <path>` / `/atrace <path>` | 同 `/log` 但走 perf 通道 |
| `/htrace convert [--trace-engine=auto\|trace_streamer\|builtin] <binary> [out.systrace]` | 手动把二进制 Harmony/OpenHarmony HiTrace 转成文本 systrace;纯 trace auto 有 trace_streamer 走 SQL、缺 trace_streamer 才走内置、SQL 失败不回退;trace+perf 固定 SQL-only;默认输出 `<binary>.systrace`,不自动附加 |
| `/htrace tools-status` | 在 REPL 内查看 trace_streamer、trace engine、sys parity gate 状态 |
| `/htrace append` / `/htrace show` / `/htrace clear` | 同 `/log` 子命令 |
| `/paste` | bracketed paste 被 SSH/tmux 吞掉时的 fallback;贴完 `/end` |
| `/chat <message>` | 强制走闲聊路径,不读仓库,不调工具 |
| `/cancel` | 管道 / 脚本输入下取消 Run(REPL 用 Ctrl+C) |
| `/env show` | 环境快照(OS / Python / Node / 包管理器) |
| `/env probe` | 重新探测 |
| `/env explain <stderr>` | 对一段 stderr 输出诊断 + 推荐安装命令 |
| `/env cache list` / `/env cache clear` | 缓存管理 |
| `/env stats` / `/env stats reset` | 推荐管线计数器 |
| `/branch <name>` | 主仓 `git checkout <name>` |
| `/branch -b <name>` | 创建并切换 |
| `/mermaid <body>` | 把一段 mermaid 代码渲染成 ASCII / 终端预览(独立工具,不走流水线) |
| `!<shell-cmd>` | 在工作目录执行 shell 命令(单次) |

**模式与写模式**:

| 命令 | 用途 |
|---|---|
| `/mode auto` / `code` / `operation` / `data` / `write` | 切换粘滞任务模式;`write` 在 `write_enabled: false` 时被拒绝 |
| `/code <问题>` | 单次强制走代码/源码分析 |
| `/op <任务>` | 单次强制走电脑操作 |
| `/data <任务>` | 单次强制走数据处理 |
| `/write <改动需求>` | 单次强制走写模式;`write_enabled: false` 时被拒绝 |
| auto `route=write` | REPL auto 下明确的代码变更请求可自动进入 Auto Pilot;低/中风险自动写入 worktree 并验证,高风险才等待审批 |
| `/plan show` | 渲染当前 pending plan(per-file diff,16 KB 上限) |
| `/plan show <id>` | 按 ID 渲染任意 plan |
| `/plan list` | 列出 PlanStore 里所有 plan |
| `/plan clear` | 丢弃当前 pending plan(不入 memory) |
| `/plan clear <id>` | 删除指定 plan |
| `/plan clear --all` / `--status=<state>` | 批量清(交互 y/N) |
| `/approve` | 批准当前 high-risk pending plan/batch,在 worktree 内 apply + verify |
| `/approve <id>` 或 `/approve --plan-id=<id>` | 指定 plan ID |
| `/approve --skip-verify` | 仅 apply,跳过 verify |
| `/approve --merge-to=<branch>` | apply 通过后立即 merge |
| `/reject [reason]` | 拒绝当前 pending plan(理由记入 memory) |
| `/workflow show` | 高级审计:查看 active write workflow 的 batch、approval、handoff 和 budget;无 active write workflow 时回落到 operation workflow |
| `/workflow list` | 高级审计:列出 `.codrax/plans/workflows/` 下保存的 write workflow runs 和 typed next action |
| `/workflow resume [<run-id>]` | 高级恢复:手动选择某个保存 run 作为 active;正常安全续跑由 Auto Pilot 自动处理 |
| `/workflow clear [<run-id>]` | 删除指定或当前 active write workflow run 元数据和 context artifacts,不删除 plan/worktree |
| `/verify [<id>]` | 对已 apply 的 plan 重跑 verify |
| `/worktree list` | 列出保留的 worktree |
| `/worktree discard <id>` | 删除指定 worktree |
| `/merge` | 把 worktree 合回当前 branch(fast-forward) |
| `/merge --branch=<name>` | 在主仓拉新分支并 cherry-pick |
| `/merge --include-failed` / `--force` | 把 verify_failed plan 纳入候选 |
| `/baseline` | 显示当前 baseline 测试快照(`pipeline_baseline_capture_enabled` 打开时可用) |
| `/baseline clear` | 清掉 baseline 缓存 |
| `/phase` | 活跃 workflow run 的 batch 阶段视图;无活跃 run 时只读回落遗留方案组(PlanGroup 通道已退役,`next/rollback/resume/skip` 不再可用) |
| `/pitfalls` | 列出本仓积累的 active failure pattern(写模式 planner 会自动看到) |
| `/pitfalls clear` | 清掉本仓 failure taxonomy |

**多行输入**:行尾加 `\` 进多行模式;`/paste` + `/end` 是另一种粘贴 fallback。

**shell `!` 前缀**:`!ls`、`!grep -rn ...`、`!cd /tmp && cat foo`(同一个 shell 进程,`&&` 链式可以连用 `cd`;每次 `!` 是新 shell,所以单独 `!cd /tmp` 不持久)。

---

# 7. CLI 参考

```
codrax [flags] [request...]
```

`request` 是位置参数(等价 `--request "..."`)。提供 `request` 时是单次 CLI 模式,不开 REPL。

| flag | 默认 | 作用 |
|---|---|---|
| `--repo` | `.` | 目标仓库根 |
| `--branch` | `main` | 默认 git branch |
| `--multi-repo` | —(继承 yaml) | `=true` / `=false` 本次 Run 覆盖 `multi_repo_enabled`;省略则不覆盖(见 3.6) |
| `--focus <slug>`(可重复) | — | 多仓启动时预 pin 子仓,等价启动后立刻 `/repos focus`;单仓静默忽略 |
| `--request, -r` | — | 单次模式问题(等价位置参数) |
| `--providers` | `<exeDir>/providers.yaml` | 替代 providers.yaml 路径 |
| `--lang` | `zh` | 答案语言;`off` 关闭 |
| `--color` | `auto` | `auto` / `always` / `never`(`NO_COLOR` env 永远强制关) |
| `--mermaid-render` | `false` | 仅单次模式:把 `mermaid` 代码围栏渲染成对齐 ASCII;默认输出原始 Mermaid 源码。REPL 无视此 flag,总是渲染 |
| `--log-level` | `debug` | error / warning / info / debug |
| `--log-dir` | yaml 默认 | 日志目录 |
| `--log-stdout` | `false` | 日志同时打 stdout |
| `--memory-dir` | yaml 默认 | 记忆目录 |
| `--cache-dir` | 平台默认 | repomap 缓存目录 |
| `--pipeline-max-steps` | 50 | 总步数 |
| `--pipeline-max-retries` | 0(继承 yaml) | 每阶段重试 |
| `--pipeline-max-stage-visits` | 0(继承 yaml) | 每阶段最多访问次数 |
| `--max-prescan-rounds` | 0(继承 yaml) | 覆盖 analyzer 预扫轮数预算(`analysis_max_prescan_rounds`);多主题问题再 +1,上限 4 |
| `--log <path>` (可重复) | — | 附加日志文件;`-` 表示 stdin |
| `--log-text <inline>` | — | 内联日志文本 |
| `--log-source-prefix <prefix>` | — | 剥掉 C/C++ 编译路径前缀再去仓库找 |
| `--htrace <path>` (可重复) | — | 附加 ftrace 兼容 trace;`-` = stdin |
| `--htrace-text <inline>` | — | 内联 trace |
| `--atrace <path>` / `--atrace-text` | — | `--htrace` / `--htrace-text` 的别名 |
| `--chitchat-classifier[=true|false]` | — | 本次 Run 覆盖 yaml `chitchat_classifier_enabled` |
| `--mode <auto\|code\|operation\|data\|write>` | `auto` | 任务入口;显式 code/operation/data/write 可绕过自动分类 |
| `--data-resume <checkpoint.json>` | — | 仅 data 单次模式:显式从 `.codrax/data-audit/*-checkpoint-*.json` 恢复 workflow checkpoint,不会自动续跑旧任务 |
| `--write-phase <apply\|plan\|verify>` | `apply` | 仅 `--mode=write` 生效;默认 Auto Pilot apply;`plan` / `verify` 是高级 lane |
| `--auto-apply` | `false` | 兼容旧脚本的保留 flag;当前安全边界由 typed allow/ask/deny approval policy 控制 |
| `--plan-out <path>` | `.codrax/plans/<id>.json` | plan-mode 落盘路径 |
| `--plan-file <path>` | — | apply / verify 模式必填:已有 ChangePlan JSON 路径 |
| `--auto-init-repo` | `false` | 授权把目标目录初始化为 git 仓库(`git init` + 空 commit) |
| `--allow-scaffold` | `false` | 授权在空目录里凭空生成新文件(从零创建项目)。空目录场景需要和 `--auto-init-repo` 同时使用 |

**典型 CLI 示例**:

```bash
# 单次问问题
codrax -r "internal/repl/repl.go 里 dispatch 怎么决定走哪个 handler?"

# 单次 + 附加日志(scripted)
kubectl logs pod/foo | codrax --log - -r "排查这个 crash"

# 单次 + 多语言
codrax -r "what does dispatch do?" --lang en

# 单次 + 切到另一个 yaml
CODRAX_SETTINGS=/etc/codrax/prod.yaml codrax -r "..."

# 强制代码分析,不走自动分类
codrax --mode=code -r "这个配置项在哪里定义和消费?"

# 强制电脑操作
codrax --mode=operation -r "查看当前系统 CPU 和内存信息"

# 强制数据处理
codrax --mode=data -r "汇总当前目录 CSV 的数值字段总和,只输出数字"

# 显式从数据处理 checkpoint 恢复
codrax --mode=data --data-resume .codrax/data-audit/20260607-123456-1234-checkpoint-r4.json -r "继续完成这个数据任务"

# 写模式:Auto Pilot 自动探索、apply、verify
codrax --mode=write -r "把 foo 拆成两个函数"

# 写模式:高级 plan-only 产物 + 落盘
codrax --mode=write --write-phase=plan -r "把 foo 拆成两个函数" --plan-out /tmp/plan.json

# 写模式:高级 saved-plan apply(单次,不开 REPL;仍经过同一 risk/approval gate)
codrax --mode=write --write-phase=apply --plan-file=/tmp/plan.json

# 写模式:重跑 verify
codrax --mode=write --write-phase=verify --plan-file=/tmp/plan.json

# 写模式:高级离线审计,读取 final report 或 saved plan 的 sibling .final.json,输出 typed audit JSON
codrax --mode=write --write-audit .codrax/plans/plan-abc123.final.json
codrax --mode=write --write-audit .codrax/plans/plan-abc123.json
```

CLI 单次模式输出:

- **stderr**: 进度 / spinner / 调试信息
- **stdout**: 最终答案纯文本(mermaid / markdown 都按源码输出,方便重定向到文件 / 转给其他工具)
- **写模式 `.codrax/plans/`**: 单次 CLI 和 REPL 使用同一套 durable store;Auto Pilot 的 ChangePlan、workflow DAG、approval 状态和 context pack 会写到 `.codrax/plans/` / `.codrax/plans/workflows/`,用于 `/workflow show`、恢复和 SWE-bench adapter 审计遥测。终态写任务还会生成 `<plan>.final.json`,其中 `loop` 字段保存 typed event refs、active runtime unit、truth/proof/localization/permission authority 摘要,`patch.language_families` / `verification.runner_families` 记录补丁涉及语言族和本地验证 runner 覆盖语言族,方便审计补丁质量而不需要解析终端日志。`reasoning_graph` / `graph_audit` 字段由 reasoning graph projector/replay 层从 typed events 生成;外部字段保持稳定,内部不再要求用户或工具去解析终端输出补这些统计。高级 CLI `codrax --mode=write --write-audit <final-or-plan-path>` 会从这些 typed artifacts 输出 `final_audit` JSON;它不运行工具、不调用模型、不解析终端散文。
- **`.codrax/output/<时间戳>-<pid>.md` + `.html`**: 每次 read 模式问答的最终答案落盘留底,Markdown 文件分两段 `# 问题` / `# 回答`,内容是模型原文的轻度排版版本;同名 HTML 由系统从 Markdown 派生,自包含 CSS 与 Mermaid 浏览器运行时,可直接用浏览器打开查看图表/表格/代码块。REPL 多轮对话每轮一组。默认按 Markdown 保留最近 10 份,旧 `.md` 及同名 `.html` 按 mtime 自动删。失败的中间重试不会写盘 — 只留用户实际看到的最后一版。写模式 plan / apply / verify / audit **不生成**这种文件。开关 `output_dump_enabled`、份数 `output_max_files` 见 5.2 节。
- **REPL 浏览器预览**: 当 `markdown_preview_server: auto|on` 且 output dump 成功时,REPL 会在答案下方显示一个带随机 token 的本地 URL。预览服务只服务当前进程登记过的 `.md` 文件,默认监听 `0.0.0.0:0`(系统随机端口),可通过 `markdown_preview_host` / `markdown_preview_port` 固定。页面内嵌固定版本 Mermaid JS,适合查看终端里容易变形的 Mermaid、表格和长代码块。

---

# 8. 排错

## 8.1 启动时

**`providers config not found`** 或 `provider/api_key/model/base_url is required`
→ 5.1 节;4 个字段必填,缺一个就拒启动。

**`x509: certificate signed by unknown authority`**
→ 公司自签 CA。`tls_ca_file: /path/to/corp-bundle.pem` 写进 `providers.yaml`;相对路径从 codrax 可执行文件所在目录开始计算。

**banner 里 `WARN search backend: native Go scanner`**
→ 没装 ripgrep / grep。能跑,只是慢。装 ripgrep:`apt install ripgrep` / `brew install ripgrep` / `winget install BurntSushi.ripgrep.MSVC`。

**`WARN git not found on PATH`**
→ Windows 推荐装 [Git for Windows](https://git-scm.com/download/win),会同时拉来 sh/bash/grep/find。

## 8.2 运行时

**spinner 卡住几十秒不动**
→ 可能是流式 first-byte timeout。`providers.yaml` 调高 `stream_first_byte_timeout_seconds`,或换个 provider。

**`error: request interrupted (likely Ctrl+C ...)`**
→ 你按了 Ctrl+C,或网络断了。重试。

**`error: upstream LLM stream stalled with no bytes for Ns`**
→ 上游模型卡住。换 provider 或换模型(thinking model 长 reasoning 段可能正常 120s 无字节;调 `stream_stall_timeout_seconds`)。

**DeepSeek 报 `Thinking mode does not support this tool_choice`**
→ provider 原生 thinking 与 tools / `tool_choice` 冲突。保持 `thinking_mode: auto`(默认)或显式设 `thinking_mode: disabled`。不要用 `think_aloud: false` 当修复手段;它只控制 Codrax prompt 侧的进度摘要,不是 provider 原生 thinking 开关。只有明确需要 DeepSeek 原生 thinking 且确认模型支持 tools 时,才设置 `thinking_mode: enabled`。

**最终答案空白 / `(no content rendered)`**
→ analyzer 拒绝了请求,或 LLM 返空。看 `<CWD>/.codrax/logs/codrax-*.log` 的 ERROR / WARN。

**记忆里有错误回合污染下轮**
→ codrax 自动把错误回合用占位文替代,但若已经污染,`/clear` 一次。

**写模式 verify 老失败**
→ Auto Pilot 会把最新 typed verify failure 放进 P2 handoff 并自动小批量 replan;仍耗尽预算时,先看 `/workflow show` 的 report/diff/surface refs。需要更大预算再调高 `pipeline_write_retry_budget`;跳过验证或强行合入只作为显式人工覆盖。

## 8.3 性能

**REPL 启动慢**
→ 第一次启动会扫整个仓建 repomap 索引。后续走 cache(`cache_dir`),快得多。

**单次 Run 太慢 / token 太贵**
→ analyzer / extractor / chitchat_classifier 路由到便宜模型(5.1.6);或 `pipeline_max_steps` 调小。

**`repomap: tier degradation` WARN**
→ 某种语言的 tree-sitter 解析失败率偏高。问题不大,但建议升级 codrax 或反馈给团队。

**扫描超大仓库时进程被杀(`Killed` / dmesg 里有 OOM)**
→ 在内存偏小的机器上扫巨型仓库(如 Linux 内核)时,repomap 全量扫描可能耗尽宿主 RAM。codrax 默认已三管齐下缓解:启动设 GOMEMLIMIT 软上限、解析后立即回收内存、被中断的扫描下次自动从已落盘 chunk 续扫(见 5.2「大仓内存韧性」)。若仍被杀:① 显式设置每次运行的 `GOMEMLIMIT`,例如 `GOMEMLIMIT=6GiB codrax --repo /path/to/big-repo --request "..."`,并让它低于容器/宿主硬内存上限;② 临时加 swap 让首次扫描扛过峰值、把 cache 建出来;③ 用 `--repo` 指向更小的子目录而非整棵树;④ 内存极小的机器可调低 `memory_soft_limit_fraction` 或直接设置 `memory_soft_limit_bytes`。日志里的 `repo_map: resuming interrupted scan` 行说明续扫已生效。

**扫描超大仓库时 SSH 断连 / 远程终端卡死**
→ 全量扫描会占满每个 CPU 核心,可能饿死 sshd。把 `repomap_scan_reserve_cpus` 设为 `1`:扫描期间 codrax 把 `GOMAXPROCS` 压低一核,让**整个 Go 运行时(含 GC 线程)**都留一个空闲核心给 sshd。启动日志 `cpu: repomap scan reserves 1 core(s)` 可确认。注意:调度 nice 值管不住 Go 运行时自己的 GC 线程,所以这里用 `GOMAXPROCS` 上限作硬保证。该键默认 `0`(不改变默认行为、不损失扫描速度);小远程机掉 SSH 就设 `1`,仍卡顿设 `2`。

## 8.4 写模式特有

**`/mode write` 报 write 被禁用**
→ 你的 `codrax.yaml` 显式设置了 `write_enabled: false`(kill switch)。删除该行或改为 true,重启 codrax。

**`/approve` 报 `target ... is needs_init`**
→ 目标目录不是 git 仓。`/approve --auto-init-repo` 一次,或 yaml 里 `write_auto_init_repo: true` 长期允许。

**plan 模式在空目录卡住或报 "scaffold 授权"**
→ 空目录从零创建项目需要单独授权。在 yaml 里同时设 `write_auto_init_repo: true` + `write_scaffold_enabled: true`,或启动时同时加 `--auto-init-repo --allow-scaffold`。两者职责不同 — 前者授权初始化 git,后者授权凭空生成文件。

**runner 检测错了 / runner 不存在**
→ codrax 自动探测 12 种 runner(go / node(jest/vitest)/ pytest/unittest/Django runtests.py / cargo / mvn / gradle(含 Kotlin/Android)/ cmake(ctest)/ meson / make / cjpm / hvigor / rspec / swift)。`runner_missing` 信号识别"二进制没装"(`pytest: command not found` 等),`parser_error` 信号识别"runner 启动了但没有执行到真实用例或没产出结构化报告"(例如 pytest collection/import 阶段因环境不兼容中止、verification probe import/syntax 或顶层 probe 结构错误、或 unittest discover 只产出 `unittest.loader._FailedTest` loader 行),`no_tests` 信号识别"runner 没有执行到任何测试"(例如 selector / project harness 不匹配),三者都会自动跳过普通 verify→plan 重试,将 plan 标记为 `unverified` 而不是 `verify_failed`,并保留安装/环境提示和 failure summary。`ChangeReport.verification_status` 会归一为 `unavailable`,所以 REPL、controller、SWE-bench adapter 和持久化 report 使用同一个 typed verdict；durable workflow 还会写 `completion.verdict=unverified`,让审计方区分"控制器已完成"与"本地测试已证明"。若模型后续仍提出继续探索或重试,日志会透明显示该 toolcall,但 typed normalizer 会把它覆盖为 `finish(accept_unverified)` 并记录 `write controller decision normalized ...`;实际不会继续跑探索/重规划。若 `ChangePlan.verification_probes[]` 存在,verify 会优先运行 typed 探针;探针 pass/fail 会进入 `ChangeReport.TestResults` 和 `ExecutedCommands`,raw output 包含探针源码片段,供 controller/handoff 消费。探针通过时项目套件不会作为硬门执行,但会以 `TestSurface` 和 `suite_skipped` command evidence 留痕;探针里的 assertion/raise/throw/panic、显式非零退出或被测代码运行时异常会驱动 replan,探针 runtime 缺失、import/syntax 或顶层结构不可用才回落到项目 runner 或 `unverified`。pytest JSON/report 生成失败时会自动尝试禁用第三方插件的 verbose 文本 fallback;若第一个 fallback 使用裸 `pytest` 入口仍没有可解析用例行,执行器会再尝试同样禁用插件的 `python -m pytest -v`,覆盖 self-hosted pytest / editable install 后 console script 与源码 API 不一致的场景;缺少 report 会被描述为"pytest 未产出请求的 JSON report",可能来自 collection/import/startup 中止或插件缺失;只有拿到用例级执行行才把结果当作真实 pass/fail,collection/import 启动失败仍保持 `parser_error`。也可以在 verifier prompt 里声明 `runner=<choice>` + `working_dir` 显式指定,绕过自动探测;Python 场景只声明结构化 `framework=django|pytest|unittest` 也会隐式选择 `runner=python`;pytest `suite` 可以包含多个以空格分隔的 file/nodeid selector,执行器会在 selector token 形状明确时拆成多个 pytest argv;带空格的单个参数化 selector 会保留为一个 quoted argv;`suite` 若误填 typed TestSurface candidate id 会在执行前被拒绝,避免把候选编号误拼成 pytest selector。SWE-bench adapter 的 `--prepare-python-env` 会记录每个实例的 venv 准备过程,缺 `pkg_resources` 这类老项目运行期兼容包时会自动尝试 setuptools 兼容版本;若 `pyproject.toml` 声明 `[build-system].requires`,adapter 会用 TOML 结构化解析结果把 build requirements 安装到同一个 venv;若存在常见运行时/测试 requirements 文件,adapter 会 best-effort 安装并将 constraints 透传给 pip,没有测试专用文件时才使用 dev requirements 作为受控兜底;老项目在 `setup.py` 或 `setup.cfg` 声明依赖时,adapter 会分别用 Python AST / ConfigParser 结构化提取 `install_requires` / `setup_requires` 并 best-effort 安装;`setup.cfg` 的 `tests_require` 以及 `options.extras_require` 中 `test` / `tests` / `testing` extras 也会 best-effort 安装;若 editable install 在隔离构建环境里仍看不到 legacy setuptools,adapter 会再尝试一次 `pip install --no-build-isolation -e .`;如果 legacy `setuptools.dep_util` API 缺失且项目有 `setup.py`,adapter 会用结构化 API probe 收敛到 `setuptools>=64,<66` 并尝试 `python setup.py build_ext --inplace`;最后会对当前 checkout 的 Python import roots 做非硬门 import probe 并记录结果,同时把结构化发现的 `src/` / `lib/` / repo-root source roots 注入 adapter import probe 和 Codrax 子进程 `PYTHONPATH`。若仍失败,结果保持 `unverified`,不会把环境问题当作代码失败硬门;`env_prepare` 会同时在嵌套对象和 `results.jsonl` 顶层暴露稳定字段:`env_prepare_status` / `env_prepare_success` / `env_prepare_env_available` / `env_prepare_failure_kind` / `env_prepare_pytest_available` / `env_prepare_pytest_json_report_available` / `env_prepare_import_probe_ok` / `env_prepare_import_roots` / `env_prepare_source_roots` / `env_prepare_python_compat_constraints` / `env_prepare_venv_python` / `env_prepare_failed_step_names`。这些字段只用于审计和批量分析,不会参与代码成败硬判定。`results.jsonl` 还同步包含 `verify_status` / `verify_failure_kind` / `verify_failure_reason_code` / `verify_summary` / `verify_test_count`、`plan_target_paths` / `plan_change_paths` / `plan_test_change_paths` / `plan_verification_probe_count`,以及基于持久化 workflow/context pack typed 字段投影出的 `plan_context_paths` / `plan_context_covered_paths` / `plan_context_uncovered_paths` / `plan_context_coverage_ratio`,并记录 `exported_patch_paths` / `exported_patch_source_paths` / `exported_patch_test_paths` / `final_plan_source_paths` / `final_plan_test_only` / `final_plan_covers_exported_source_patch` / `prediction_verdict` / `prediction_local_confidence` / `prediction_blocks_local_acceptance`,方便审计环境不可用、测试改动漂移、probe 覆盖、planner handoff 覆盖、本地 verify 失败和最终 plan 与导出 source patch 是否一致。context coverage、final-plan/exported-patch coverage 和 prediction verdict 都只是审计遥测,不是 apply/verify 硬门,也不改变官方 predictions JSONL 形状。SWE-bench prediction 默认剥离仓内 test/spec 路径改动并记录到 `dropped_test_patch_paths`;adapter 也会软提示 planner 优先把本地行为检查写成 `verification_probes[]`,但硬保证仍是 typed exporter 的剥离与记录;调试 Codrax 生成的测试改动时可显式传 `--include-test-patches`。

SWE-bench adapter 的 `results.jsonl` 还会写 `plan_context_missing_source_paths`,用于审计哪些最终 source 改动没有 P0/P1 prior context 覆盖。

写模式终态还会在 plan 旁边持久化 `<plan-id>.final.json` 结构化交付报告,并让 SWE-bench adapter 投影出 `final_report_present` / `final_report_path` / `final_report_completion_verdict` / `final_report_verification_status` / `final_report_patch_language_families` / `final_report_verification_runner_families` / `final_report_patch_review_verdict` / `final_report_residual_risk_codes` / `final_report_handoff_evidence_refs` 等字段。这些字段只来自 workflow、ChangePlan、ChangeReport、patch review、impact analysis 和 context pack typed artifacts；不会解析 issue 文本、模型散文、终端日志或可见 `<think>` 来驱动逻辑。`codrax.out` 仍完整保留给用户透明审计,但批量验收优先看 `.final.json`。

SWE-bench adapter 还会在 ConfigParser 提取 `setup.cfg` 依赖后剥离 inline comment,避免把 `numpy>=1.21  # runtime floor` 这类合法写法传给 pip。明确 Python/pytest test surface 且仓内存在 pytest 配置时,pytest 零用例会保持 typed `no_tests/unavailable`,不会继续升级到 unittest discovery 产生无关 loader 噪声。仅靠 verification probe 通过的本地 verdict 若没有覆盖 required behavior contracts(包括 `expected_outcome_fallback`)或 changed symbols,会降低 `prediction_local_confidence`;这仍是审计遥测,不阻断官方 predictions JSONL 导出。Codrax 内部 `ChangeReport` 也会写 `verification_confidence[]`,把 `source_compile_ok`、`verification_probe_missing_required_contract_ref`、`verification_probe_missing_changed_symbol_ref`、`verification_probe_baseline_not_run`、`project_runner_unavailable` 等 typed reason 带入 context pack、verify-failure handoff 和 SWE-bench `verify_confidence_reason_codes`。

SWE-bench adapter 的 Codrax 子进程输出会边跑边写入每个实例的 `codrax.out`,长时间探索/verify 时还会按 `--codrax-progress-interval` 或环境变量 `CODRAX_PROGRESS_INTERVAL` 打印 typed workflow heartbeat(默认 30 秒,设 0 关闭)。heartbeat 只读取 `.codrax/plans/workflows/` 下的 durable JSON,展示 `workflow` 状态、active batch/slice 和最新 `progress_ledger` reason;它不会解析 stdout、模型散文、issue 文本或 `<think>` 来驱动逻辑。原始日志仍完整保留,用于用户透明审计。

**Source compile fallback**
→ verify 阶段会对 plan-touched source 跑 runner-aware compile fallback:Python `py_compile`/静态名称检查、Node JS `node --check`、Node TS `tsc --noEmit --pretty false`、Ruby `ruby -wc`、Java/Kotlin Maven/Gradle compile 或 bounded `kotlinc`、Swift Package `swift build --skip-build`,以及没有 `_test.go` 时的 Go package `go test -json` 编译兜底。这些只消费 ChangePlan 路径、runner 语言映射和编译器输出;语法/解析/编译失败归入 typed `build_failure` / `failed`,避免明显坏补丁被后续缺 pytest、缺依赖或 runner parser_error 降级成 `unverified`。缺少 pytest、第三方依赖、插件或测试 harness 仍按上面的 `unavailable` 处理,不会成为代码失败硬门。

**actual-diff 边界信号是不是只有 Python**
→ 不是。真实 diff 线形状的 PatchReview producer 走 provider registry,当前覆盖 Python、JS/TS、Ruby、Java/Kotlin、Go 的映射/容器默认边界信号;这些信号是 typed soft coverage obligation,会进入 P2 handoff 和 bounded follow-up。SWE-bench Lite 上有些 AST owner-boundary 审计看起来偏 Python,是因为评测集主要是 Python 仓,不是写模式架构只支持 Python。

**no_tests 状态**
→ runner 没有发现可执行测试时,持久化 `ChangeReport` 会同时写入 `verification_status=unavailable` 和 `failure_kind=no_tests`。`Passed=true` 只保留给旧 parser 兼容,controller、REPL、SWE-bench adapter 和 handoff 都应消费 typed verdict/reason,不要从布尔值猜验证成功。

**Verify failure handoff 不继承合成 suite**
→ verify 失败后 controller 会把 typed `TestResult` / `ExecutedCommands` 投影到下一轮 handoff。unittest 输出会保留 `FAIL:` / `ERROR:` block 和最终 `Ran ...` / `FAILED (...)` tail,避免大套件进度点把真正失败证据挤掉。下一轮只会继承真实且唯一的可复用 suite;如果失败用例分散但上一轮 `ExecutedCommand.Suite` 是真实 selector,会继承这个命令级 selector,不会把空 suite 当成"跑全量"。`unittest`、`unittest.loader._FailedTest...`、`verification_probe/*`、`runner_missing`、`py_compile`、`build`、`make-test` 等合成/环境/聚合标签只作为证据保留,不会被拼成下一次 runner selector。

**SWE-bench fair eval 要防未来历史泄漏**
→ 正常 Codrax 产品运行默认保留 git history 工具。只有 SWE-bench adapter 显式传 `--isolate-git-history` 时,adapter 才会把实例 checkout 固定在 `base_commit`、删除分支/标签 refs、过期 reflog、prune unreachable objects,并调用 Codrax 内部 eval-only flag `--eval-disable-git-history`。`eval/swebench/smoke_lite.sh` 为 fair smoke 默认设置 `SWEBENCH_ISOLATE_GIT_HISTORY=1`;直接运行 `run_codrax_swebench.py` 或普通读/写模式默认关闭。该 gate 只消费 typed flag、repo path/ref 结构和解析后的 git 子命令,不根据用户意图关键词、issue 文本或模型散文做硬路由。

**SWE-bench adapter source roots**
→ `--prepare-python-env` 会基于文件结构发现 Python source roots(`src/`、`lib/` 或包含 importable 模块的 repo root),注入 adapter import probe 和 Codrax 子进程 `PYTHONPATH`,并在 `env_prepare_source_roots` / `source_roots` 中记录。这仍只是审计遥测和本地验证辅助,不会成为 apply/verify 硬门。

**SWE-bench adapter legacy Python env**
→ adapter 会用结构化 parser 读取 `setup.py` / `setup.cfg` 声明的运行依赖，并读取 `setup.cfg` 的 `tests_require` 以及 `test` / `tests` / `testing` extras 作为 best-effort 测试依赖。若 editable install 失败且 legacy `setuptools.dep_util` API 缺失，会用 API probe 收敛到 `setuptools>=64,<66` 并尝试 `python setup.py build_ext --inplace`。这些步骤只改善本地验证沙箱；失败会记录到 `env_prepare.failed_step_names` 并保持 `unverified`，不会拦截 prediction 导出。

**`/merge` 说 "no worktree to merge from"**
→ apply 完 worktree 被清了。`codrax.yaml` 加 `pipeline_keep_worktree_on_success: true`,下次 apply 后 worktree 会保留。

---

## 附:学习路线建议

1. 先用读模式,问几个仓里熟悉的问题,体会 citation 验证流程
2. 加 `--log` 跑一次 panic 排查,体会 log_triage 的精确度
3. 多轮转换("换成表格""画 mermaid"),体会 turn-policy 路由
4. 用 `/write` 或 auto 路由做一个 1-3 文件的小重构,低/中风险让 Auto Pilot 自动 apply + verify,最后按需 `/merge`
5. 按需调 `providers.yaml` 把贵 agent 路由到大模型、便宜 agent 路由到小模型,把成本压下来

---

> 本指南覆盖 codrax v0.1.x 的全部用户面命令与配置。底层架构、调度算法、各种 gate 设计请参考 `docs/architecture.md` 与 `CLAUDE.md`。报告问题 / 建议改进:`hanssccv@gmail.com`。
