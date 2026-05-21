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
- 想让 codrax 真改代码 → 第 4 章 写模式 `plan → apply → verify`。
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
  - [3.2 附加性能 trace(HiTrace / atrace / systrace / perfetto)](#32-附加性能-tracehitrace--atrace--systrace--perfetto)
  - [3.3 闲聊与本地转换](#33-闲聊与本地转换)
  - [3.4 记忆与会话](#34-记忆与会话)
  - [3.5 一台机器多仓库](#35-一台机器多仓库)
  - [3.6 跨仓 workspace(multi-repo discovery)](#36-跨仓-workspacemulti-repo-discovery)
- [4. 写模式 — plan → apply → verify](#4-写模式--plan--apply--verify)
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
- **写模式(可选)**:在沙箱 git worktree 里产生改动 plan,通过你审批后才落地。主仓 HEAD 永不自动变。支持 12 种测试 runner 自动探测,verify 失败 → planner 自动重新规划(带早停守门)。
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
| DeepSeek | `https://api.deepseek.com/v1` | model 例:`deepseek-chat` |
| 阿里云 DashScope | `https://dashscope.aliyuncs.com/compatible-mode/v1` | model 例:`qwen-max` |
| Ollama 本地 | `http://localhost:11434/v1` | model 是你 `ollama pull` 拉过的名字 |
| vLLM 本地 | `http://localhost:8000/v1` | api_key 任写;model = `--served-model-name` |

## 1.5 第一个问题

> **codrax 把「你启动它时所在的目录」当作要分析的代码仓库。** 它不会弹窗问你「要分析哪个项目」—— 默认分析的就是当前工作目录(等价 `--repo .`)。所以使用前请务必先 `cd` 进你想分析的项目根目录,再运行 `codrax`;在别的目录启动,它就会去分析那个目录。

```bash
cd /path/to/your/repo    # ① 先进入你要分析的代码仓库(任何 git 仓)
codrax                   # ② 再启动;它会索引「当前目录」这个仓库
```

启动后 codrax 会在当前目录建一个 `.codrax/` 子目录,存放日志、对话记忆、缓存等运行产物;读模式只读你的源码、从不修改。想分析的不是当前目录时,用 `--repo /path/to/repo` 显式指定。

看到这个就是启动成功了(每一行都是真实输出,版本号和路径会按你的环境替换):

```
   CODRAX  v0.1.x  git:main  /help · /exit
   modes: read (write_enabled=false — /mode plan / apply / verify 已禁用) · /home/you/tools/codrax/codrax.yaml

[git:main]❯❯
```

提示符前会带 sticky 标签 `[git:<branch>]`,显示当前所在 git 分支(没在 git 仓里就不显示这一段)。

直接打你的问题、回车。提交后,你打的内容会以 `> ...` 形式回显在分隔线下方,然后下方开始打印进度:

```
─────────────────────────────────────
> 这个项目的入口函数在哪里?
✓ 1/6 已理解问题 · 第 N 轮 · X 次工具调用 · 本 Ys · 总 Zs
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
   CODRAX  v0.1.x  git:main  /help · /exit
   modes: read · plan · apply · verify (write_enabled=true) · /home/you/.../codrax.yaml

─────────────────────────────────────
> 这个项目的入口函数在哪里?
✓ 1/6 已理解问题 · 第 4 轮 · 2 次工具调用 · 本 7s · 总 8s
  💭 [analyzer-1] <think> ...                           ← LLM 实时推理摘要
  ⠏ 调用工具中 ▸ grep "func main"                       ← 当前在跑哪个工具
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
| `CODRAX  v0.1.x  git:main` | 版本 + 当前 git 分支(非 git 目录不显示 git:) |
| `modes:` 行 | 当前可用模式 + 配置文件路径(`write_enabled` 开/关一目了然) |
| `─────…` 分隔线 | 每轮请求开始前的视觉断点(在你的回显之上) |
| `>` 开头(青色) | 你刚提交的请求的回显(保留多行 paste 内容) |
| `[git:main]`、`[mode:plan]`、`[log]`、`[trace]`、`[plan]`、`[mem!]` | sticky 标签,提示当前粘滞状态(写模式 / 附加日志 / 待处理 plan / 记忆压力) |
| `K/N <stage 中文标签>` | dock 顶部的"K/N 进度 + 当前阶段"。读模式 N=6,plan-only N=2,apply N=4,verify N=2 |
| `💭 [agent-N]` | LLM 单次推理的一两句摘要(默认开) |
| `⠏ ▸ ...` 调用工具中 | 当前在执行哪个工具调用 |
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

附加日志 size 上限:50 MiB(`log_attach_max_bytes`),超过自动尾部截断 + 警告。

**自动检测**:如果你在普通问题里直接粘贴包含 panic/stack-frame 行的文本,REPL 会**一次性**自动把它转成附加日志(打印一行 `auto-attached log: N bytes`),只对当前这轮生效,不影响下轮。如果想阻止自动,先 `/log clear` 把粘性 log 占位即可。

**C/C++ 编译路径前缀**:CI build 出来的 stack frame 路径常是 `/build/src/foo.cpp:42`,你的 repo 是 `~/repo/foo.cpp:42`。用 `--log-source-prefix /build/src/` 让 codrax 把前缀剥掉再去仓库找文件。

## 3.2 附加性能 trace(HiTrace / atrace / systrace / perfetto)

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

trace 的 size 上限独立于 log:`trace_attach_max_bytes`(默认 50 MiB)。

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

REPL banner 立即提示:

```
   CODRAX  v0.1.X  /help · /exit
   modes: read · plan · apply · verify (write_enabled=true)
   🗂  multi-repo: 5 sub-repos (active cap=3); /repos for list / focus / refresh
```

### `/repos` 命令族

启动后任何时候可用:

| 命令 | 作用 |
|---|---|
| `/repos` | 列出已发现子仓 + active 状态 + cap + focus pin |
| `/repos focus <slug>` | 把子仓固定到 active 集合,跨 turn 不被 LRU 淘汰 |
| `/repos unfocus [slug]` | 释放固定(无参数 = 全释放) |
| `/repos refresh` | 强制重新探测父目录(子仓增删后用) |
| `/repos cap <N>` | 会话级覆盖 active cap(yaml 默认 2,硬上限 3 — 设更高自动 clamp) |

`/repos` 输出长这样:

```
multi-repo topology — parent=/home/user/workspace slug=workspace-1a2b3c4d sub-repos=5 cap=3
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

写模式(`/mode plan|apply|verify`)**禁止**跨子仓 ChangePlan。当前在父目录运行写模式时,plan 阶段如果 LLM emit 的 ChangePlan 触及多个子仓,会 fail-loud:

```
✗ write blocked: ChangePlan touches 2 sub-repos: api-go, web-frontend — multi-repo write banned by design
  Repair: split into 2 separate runs, one per sub-repo (cd into each sub-repo and re-issue the request,
          OR `/repos focus <slug>` then re-run with multi_repo_max_active=1)
```

正确做法:

```bash
cd ~/workspace/api-go && codrax --mode=apply --request "..."
cd ~/workspace/web-frontend && codrax --mode=apply --request "..."
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
multi_repo_max_active: 2                    # LRU 上限,默认 2,硬上限 3 (yaml > 3 自动 clamp)
multi_repo_inactive_preview_count: 2        # L0 prompt advisory 给 LLM 看几个 out-of-active 仓 (默认 2,硬上限 3)
multi_repo_discovery_depth: 4               # 父目录 BFS 深度
multi_repo_min_files: 1                     # 子仓 file count 下限,过滤空 .git fixture
```

**何时调整 cap**:
- 默认 2 覆盖典型跨仓场景(一个主仓 + 一个协作仓);3 子仓以上的跨仓调查较少
- 跨仓问题多 + 子仓总数 5-10 → 调到 3(硬上限,yaml 设更高也强制 3)
- LRU thrashing 警告(`multigraph: thrashing detected (>5 evictions/60s)`)出现时 → 加 cap 或 `/repos focus`

### `--focus` CLI flag(2026-05-08 新增)

在非 REPL / 脚本化 / eval 调用里,用 `--focus` 在启动时预 pin 子仓(等价启动后立刻跑 `/repos focus`):

```bash
# 单仓 pin
codrax --repo ~/workspace --focus repo-greet-go --request "..."

# 多 pin(repeatable 或逗号分隔)
codrax --repo ~/workspace --focus repo-go --focus repo-py --request "..."
codrax --repo ~/workspace --focus repo-go,repo-py --request "..."
```

每个值是子仓 **slug 或 RootRel 路径**,通过 `topology.Resolve` 解析,任一形态都可以。匹配不到的 token 会 Warning 提示并丢弃,不阻断 Run。**单仓 / 无 git workspace 静默忽略此 flag**(无 sub-repo 可匹配)。

### `--multi-repo` CLI flag(2026-05-09 新增)

per-Run 覆盖 `codrax.yaml :: multi_repo_enabled`,无需改 yaml:

```bash
# 临时启用(yaml 关 / 默认覆盖)
codrax --repo ~/workspace --multi-repo=true --request "..."

# 临时关闭 — 父目录扫到一堆子仓但本次只想用单仓 legacy 路径
codrax --repo ~/single --multi-repo=false --request "..."
```

只有显式传 `--multi-repo=true` 或 `--multi-repo=false` 才会覆盖;省略 flag 时 yaml(或 yaml 缺省值 true)继续生效。生效会在启动 INFO 日志输出 `[multi-repo] CLI override: enabled=...`,可直观确认。

`multi_repo_enabled=false` 时 `/repos focus|unfocus|refresh|cap` 会拒绝执行并提示同时给出 yaml 与 `--multi-repo=true` 两条启用方式;`/repos`(不带子命令)的列表仍可查阅,只是不会路由。

### 内存与性能预算

| 项 | 单仓 | cap=3 多仓 | 备注 |
|---|---|---|---|
| 启动开销 | ~50µs | warm cache 通常 ms 级; cold discovery 取决于子仓数 | BFS + 有界并行 per-sub-repo `git ls-files`(默认 4 路) |
| Active 内存 | ~100 MB / 万文件 | ~300 MB | 与 cap 线性 |
| 拓扑 cache 磁盘 | 0 | <100 KB | 100 子仓也只 1 MB |
| 跨仓 typed lane 查询 | n/a | O(active 子仓数) | LRU 命中 → ms 级,miss → 子仓全量 build |

100 子仓 × 1 万文件场景:active 仍只 hold 3 个,内存仍 ~300 MB,**与今天单仓 ~3×**。

### 故障排查

| 症状 | 原因 | 修复 |
|---|---|---|
| answer 漏掉某子仓的 entity | 子仓未在 active 集 | `/repos focus <slug>` 然后重跑(REPL)或 `--focus <slug>` 启动(scripted)|
| `partial_typed_lane=true` 出现频繁 | cap 太低 | yaml `multi_repo_max_active: 3` 或 `/repos cap 3`(硬上限 3)|
| `thrashing detected` Warning | 同上,LRU 抖动 | 同上 |
| 写模式跨仓 fail-loud | 设计限制 | cd 进具体子仓重跑 |
| 没看到 banner 多仓行 | 父目录是单 git 仓(不是 workspace) | 这是预期 — 单仓 quiet UX |
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

# 4. 写模式 — plan → apply → verify

写模式让 codrax **生成代码改动**(增删改文件),在沙箱 git worktree 里跑测试,只有你显式批准后才合回主仓。**默认关闭**。

### 工作区单一不变量(写模式的核心规则)

> **同一个项目同时只能有一个未结算的改动方案。**

未结算 = `pending_approval`(待批)/ `applied`(已批未合)/ `verify_failed`(验证失败)。结算 = `merged`(已合并)/ `rejected`(显式丢弃)/ `applied_failed`(apply 阶段崩了,自动终止)。

**为什么这条规则重要**:每个 plan 都是基于"当前主仓状态"生成的。如果一个 plan 已经在 worktree 里改了文件但没合回主仓,这时再生成第二个 plan,新的 plan 看不到第一个 plan 的改动 — 两个 plan 可能对同一文件给出冲突的修改,合并顺序也乱套。

实际行为:`/mode plan` 切换时如果存在未结算的 plan,直接拒绝并列出三选一菜单(merge / reject / clear);REPL 启动时也会在 banner 提醒未结算 plan;数据层(PlanStore)同样硬约束,任何写入路径都过不去。

```
[git:master]❯❯ /mode plan
  ✗ 切换被拒:已存在未结算的改动方案 plan-XXXX(状态:applied)。
    新方案要基于当前仓状态生成,先把上一个收尾再来:
      /merge         合并到主仓
      /reject        丢弃改动(保留事后审查记录)
      /plan clear    彻底删除(无审查记录)
    收尾后再敲 /mode plan。
```

三个结算命令:

| 命令 | 适用状态 | 文件 | 工作区 |
|---|---|---|---|
| `/merge` | applied / verify_failed(后者需 `--include-failed`) | 状态改 `merged`,保留供审查 | 自动 discard |
| `/reject [reason]` | 任何未结算状态 | 状态改 `rejected`,reason 写进文件,保留供审查 | 自动 discard |
| `/plan clear` | 任何未结算状态 | **直接删除** plan 文件 + report,无审查记录 | 自动 discard |

启动 banner 提示长这样(灰色单行):

```
   plan-XXXX 已 apply 但未合并 — /merge · /reject · /plan clear
```

按当前 plan 的 status 不同,提示文案也不同 — `pending_approval` 显示 `/plan show · /approve · /reject · /plan clear`,`verify_failed` 显示 `/approve <id> · /merge --include-failed · /reject · /plan clear`。

## 4.1 启用

在 `codrax.yaml` 里加一行:

```yaml
write_enabled: true
```

(默认值 false 时,任何 `/mode plan` / `/approve` / `--mode=apply` 都会被礼貌拒绝并指引你改 yaml。)

## 4.2 完整流程

写模式分三步:**plan(产出改动方案)**、**apply(在 worktree 里执行)**、**verify(跑测试)**。在 plan 之前还有两个隐式分类阶段——`analyze`(读模式 analyzer 复用作请求分类)+ `write_analyze`(写模式专属,产 `WriteAnalysisIR`:任务 kind/scope/risk/期望结果,可选多阶段拆分提议)。这两个阶段对用户不可见,直接喂 prompt 给 planner 帮它"开局就有上下文",不需要 planner 自己冷启动 5 轮探索。

REPL 实际流程:

### 第 1 步:`/mode plan`,描述要做的事

```
[git:main]❯❯ /mode plan
  ✓ 已切换到 plan 模式
  •   下一条请求会产生改动方案,不直接回答。
  •   之后:/plan show 看 diff · /approve 落地 · /reject 丢弃 · /mode read 回读模式

[git:main][mode:plan]❯❯ 把 internal/foo/bar.go 里 ParseConfig 拆成两个函数,逻辑保持等价
[planner 生成改动方案,~1-3 分钟]
✓ 改动方案已就绪: plan-abc123 (3 处改动)。
  /plan show · /approve · /approve --skip-verify · /reject · /mode read
```

更多虚构的 plan 请求示例(任何项目都能套用):

| 你想做的 | 一行写法 |
|---|---|
| 把一个长函数拆成两个 | `把 X 里 LongFn 拆成两个函数,逻辑保持等价` |
| 给现有函数加参数 + 改调用点 | `给 Foo() 加一个可选 timeout 参数,所有调用点默认传 5s` |
| 重命名一个对外符号 | `把 OldName 重命名为 NewName,同步改导出路径和文档` |
| 加一个新文件 + 接入注册表 | `新增 internal/mcp/sftp.go,在 cmd/root.go 的 mcp 注册表里挂上` |
| 改 yaml 默认值 | `把 codrax.yaml.example 里 pipeline_max_steps 默认值从 50 改成 80,加注释` |

### 第 2 步:`/plan show` 审 diff

```
[git:main][mode:plan][plan]❯❯ /plan show
[per-file unified diff,带颜色;每个文件独立段落]
- Summary: 拆分 ParseConfig...
- 文件 1/3: internal/foo/bar.go (modify, +24/-12)
[diff body...]
```

不满意:

```
[git:main][mode:plan][plan]❯❯ /reject 拆得不够小
  ✓ 已拒绝 plan plan-abc123 — 原因: 拆得不够小
```

`/reject` 把 plan 状态改成 `rejected` 但**保留文件**供事后 `/plan show <id>` 查看。如果不想留记录,用 `/plan clear` 直接删掉。两种命令都会自动 discard 对应的 worktree。

### 第 3 步:`/approve` 落地

```
[git:main][mode:plan][plan]❯❯ /approve
  是否批准 plan plan-abc123 (3 处改动)?将在 git worktree 中 apply + 跑 verify。
  > y
[在 .codrax/worktrees/<plan-id>/ 里 git apply + 跑测试]
✓ apply 完成,已自动切回 read 模式。继续改代码用 /mode plan。
```

注意:批准成功后会**自动切回 read 模式**,你的下一句话默认是问代码,不是再开 plan。要继续改代码:**先把这个 applied 的 plan 收尾**(`/merge` 合到主仓 / `/reject` 丢弃 / `/plan clear` 删除),再 `/mode plan` 才能生成下一个 plan(写模式工作区单一不变量,见上文)。

`/approve` 自动:

1. 创建临时 worktree(基于当前 branch)
2. 在 worktree 里 `apply_patch` 每个文件改动(支持 create / modify / delete / patch / rename)
3. 自动检测 runner 跑测试 — 12 种自动探测:go / node(jest/vitest)/ python(pytest)/ rust(cargo)/ java(maven 或 gradle,含 Kotlin/Android)/ ruby(rspec)/ swift / cmake(ctest)/ meson / make / hvigor(HarmonyOS ArkTS)/ cjpm(Cangjie)
4. 测试通过 → 标记 `applied`;失败 → 标记 `verify_failed`(可重试)

> verifier agent 也可以**绕过自动探测**,显式声明 `runner=<choice>` + `working_dir`(都在 worktree 内的白名单里);适用于多 manifest 仓 / 测试目录在子目录的场景。

> 测试失败时,`pipeline_write_retry_budget`(默认 3,硬上限 5)允许自动重新规划:把失败摘要 + top-3 失败测试 + 嫌疑文件清单喂回 planner,重 plan 再 apply 再 verify。**这一步不用你手动操作**。两条早停守门避免烧 budget:`runner_missing` 一等信号(`pytest: command not found` 等)直接 fall-through 给安装提示;fingerprint 比对(AppliedCount + VerifyPassed + VerifyFailed + FailureSummaryHash 完全相等 → 视为"无进展")跳过本轮 retry。

特殊场景:

| 场景 | 命令 |
|---|---|
| 跳过 verify(本地起不了集成测试) | `/approve --skip-verify` |
| 指定批某个非最新的 plan | `/approve <plan-id>` 或 `/approve --plan-id=<id>` |
| apply 通过后立刻合到某分支 | `/approve --merge-to=feature/xyz`(等价 approve + merge) |
| 已 apply 的 plan 重跑 verify(如改了环境) | `/verify <plan-id>` |

## 4.3 把改动合回主仓

`/approve` 通过后改动**只**在 worktree 里。要让它进主仓:

```
[git:main]❯❯ /merge --branch=feature/refactor-bar
  在主仓上拉新分支 feature/refactor-bar 并 cherry-pick 3 个 commit?
  > y
  ✓ 已在主仓创建分支 feature/refactor-bar,cherry-pick 3 个 commit。
  下一步:cd <主仓> && git push -u origin feature/refactor-bar,然后开 PR。
  已自动切回 read 模式 — 直接提问就行。再 /mode plan 进入 plan 模式即可继续改代码。
```

| `/merge` 选项 | 行为 |
|---|---|
| (默认) | fast-forward 当前 branch 到 worktree 头 |
| `--branch=<name>` | 在主仓拉新分支 + cherry-pick(标准 PR 流) |
| `--include-failed` 或 `--force` | 把验证失败的 plan 也纳入候选(适合环境/CI 类失败,你 review 后决定强合) |

`/merge` 成功后:
- plan 状态从 `applied` 改成 `merged`(终态,可以下一个 plan 了)
- worktree 自动 discard
- REPL 自动切回 read 模式

> `/merge` 需要 yaml 里 `pipeline_keep_worktree_on_success: true`,否则 worktree 在 apply 完就清掉了。

如果主仓只有 `.codrax/` 自己写入的文件(日志、记忆、blob 缓存)显示为 dirty,`/merge` 会自动把它们 `git rm --cached` 并补一条 `.gitignore` 提交,再继续合。也就是说第一次 `git init && git add -A` 误把 `.codrax/` 纳入 git 的人,不会被 `/merge` 拒绝。

## 4.4 失败排错

**apply 失败**(代码 patch 没打进去 / 写入冲突):
- 屏幕会打印失败原因 + worktree 保留(`/worktree list` 可看)
- 推荐做法:`/mode plan` + 把目标说更具体一点重发,planner 通过 `/history` 看到本轮失败摘要;或 `/reject` 弃掉这版重新规划
- 也可以直接 `cd` 进 worktree 路径手工调,然后 `/worktree discard <plan-id>` 清掉

**verify 失败**(测试不过):
- `pipeline_write_retry_budget`(默认 3)允许 planner 自动重新规划再 apply 再 verify;失败摘要会作为补丁提示喂回 planner
- 重试用尽仍不过 → plan 标记为"验证失败"。下次 `/approve <plan-id>` 仍可重试(常见于环境/CI 类抖动)
- 本地测试根本起不了(缺依赖、缺数据库等)→ `/approve --skip-verify` 跳过 verify,只 apply

**plan 阶段返回文字回答而不是改动方案**(planner 觉得这是咨询性问题):
- 屏幕打印一段二选一引导(咨询走 `/mode read`;真改代码就把目标说具体再发)
- 直接选你需要的路径继续

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
- 如果 planner 漏了,直接把报错信息原样贴进 codrax(它会用 LLM 推断该装哪个包),或 `/mode read` 后问 "这个 ModuleNotFoundError 怎么修?"

`/worktree list` / `/worktree discard <plan-id>` 管理保留的 worktree。

---

# 5. 配置参考

codrax 用两份 YAML,职责严格不重叠:

| 文件 | 职责 | 是否必填 |
|---|---|---|
| `providers.yaml` | LLM 凭证、每个 agent 用哪个模型 | **必填** |
| `codrax.yaml` | 语言、日志、流水线预算、各种阈值 | 可选,默认值开箱即用 |

## 5.1 providers.yaml(LLM 凭证)

### 5.1.1 必填的 4 个字段

```yaml
llm:
  default:
    provider: openai
    api_key: "sk-xxx"
    model: "your-model-id"
    base_url: "https://your-endpoint/v1"  # http:// 或 https:// 都行
```

缺任何一个都会**拒启动 + 打印缺哪个字段**,不会偷偷连公网。

### 5.1.2 流式开关

```yaml
llm:
  default:
    stream: true   # 默认 true:SSE 流式,REPL 里实时显示推理 / 答案
                   # 设 false:经典单次请求-响应,适合 byte-stable CI 或本地小模型
```

### 5.1.3 TLS / 自签证书

仅当 `base_url` 是 `https://`:

```yaml
llm:
  default:
    tls_ca_file: /etc/pki/corp-bundle.pem    # 公司自签 CA(typical 错误:x509: cert signed by unknown authority)
    # tls_insecure_skip_verify: true         # 核武器:完全跳过证书验证,启动时会打高亮警告
```

### 5.1.4 sizing / 超时 / 重试(每个字段都可选)

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
```

### 5.1.5 本地小模型 tool-call 兼容(可选)

对 OpenAI-compatible 本地 / 小模型，先只打开审计模式观察：

```yaml
llm:
  default:
    recover_text_tool_calls: true       # 文本里的完整工具调用 envelope → protocol tool_calls
    tool_param_compat:
      mode: audit                       # off | audit | repair; audit 不改执行
      split_string_arrays: true
```

确认日志里的 `[tool_param_compat] ... audit repairable` 都是机械类型错误后，再对需要的 provider / agent 切到 `repair`。它只按工具 JSON Schema 修确定性问题，例如 `"offset":"146"` → `146`、JSON 字符串里的 array/object 解包、`"a,b"` → `["a","b"]`；不会补缺失字段、猜 path、删 unknown 字段或修 prose。远程大模型不需要打开 `recover_text_tool_calls` 的宽兼容档；系统会默认处理“assistant content 正好是完整显式工具调用 JSON envelope”的安全运输修复。

### 5.1.6 每个 agent 用不同模型(可选)

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
| `output_dump_enabled` | `true` | 每次 read 模式答案落盘到 `<CWD>/.codrax/output/<时间戳>-<pid>.md`;两段 `# 问题` / `# 回答`,模型原文轻度排版 |
| `output_max_files` | 10 | output 目录保留最近 N 份,旧的按 mtime 自动删 |
| `markdown_preview_server` | `auto` | REPL 在答案落盘后懒启动本地 Markdown 预览服务并显示浏览器 URL;`off` 关闭 |
| `markdown_preview_host` | `0.0.0.0` | 预览服务监听地址;远程终端可保持默认全监听,本机私有可设 `127.0.0.1` |
| `markdown_preview_port` | `0` | 预览服务端口;`0` 表示系统随机分配可用高位端口 |
| `lang` | `zh` | 答案默认语言;`off` 关闭 |

### 大仓内存韧性

扫描超大仓库(如 Linux 内核:约 6.4 万个待解析源文件)时,repomap 全量扫描可能让进程内存超出宿主可用 RAM,被系统 OOM 杀掉。下面三个键默认即开,无需配置;低内存机器上扫巨型仓库才需要关注。详见 `docs/design/large_repo_memory_resilience.md`。

| 键 | 默认 | 作用 |
|---|---|---|
| `memory_soft_limit_enabled` | `true` | 启动时设软堆上限(GOMEMLIMIT),让 GC 在宿主内存耗尽前提前回收。环境变量 `GOMEMLIMIT` 优先于本组所有键 |
| `memory_soft_limit_fraction` | `0.8` | 目标占检测到的宿主 RAM 的比例,范围 `(0,1]`。仅 Linux(读 `/proc/meminfo`) |
| `memory_soft_limit_bytes` | `0` | 直接以字节指定软上限;`>0` 时跳过宿主 RAM 检测,低于 512 MiB 抬到该下限。非 Linux 平台用这个键 |
| `repomap_resume_interrupted_scan` | `true` | 全量扫描复用上次被中断(如被 OOM 杀掉)的扫描已落盘的 chunk,经内容哈希校验后跳过重解析;重试逐步收敛而非从零重来。覆盖全部 15 种语言 |

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
| `write_enabled` | `false` | **写模式总闸**;不设 true 任何写命令都拒绝 |
| `write_default_mode` | `read` | 启动默认模式 |
| `write_auto_init_repo` | `false` | 允许把目标目录初始化为 git 仓库(`git init` + 空 commit;等价 `--auto-init-repo`,持久版) |
| `write_scaffold_enabled` | `false` | 允许在空目录里凭空生成新文件(从零创建项目;等价 `--allow-scaffold`,持久版)。空目录场景需要和 `write_auto_init_repo` 同时开启 |
| `write_auto_approval` | `false` | 预留:批量工作流 / REPL `/approve` 默认开关 |
| `write_plan_dir` | `<runtime>/plans` | ChangePlan JSON 落盘目录 |
| `pipeline_write_retry_budget` | 3 | verify 失败后自动重 plan 的最大次数 |
| `pipeline_write_retry_budget_ceil` | 5 | 上面那个 budget 的硬上限 |
| `pipeline_baseline_capture_enabled` | `false` | 写模式开启前先跑一次基准测试,用于回归判定(双倍测试时间) |
| `pipeline_keep_worktree_on_success` | `false` | apply 通过后保留 worktree(`/merge` 必需) |
| `pipeline_lint_enabled` | `true` | emit_change_plan 时对新建文件跑 V5 静态检查(gofmt / ruff / node --check 等;工具链缺失会静默跳过) |
| `worktree_keep_ttl_hours` | 168 | 保留下来的 worktree 在启动时按 mtime 老化清理(0 关闭) |
| `worktree_keep_max_count` | 20 | 老化后还要按 LRU 配额裁剪(0 关闭) |
| `verify_mem_limit_mb` | 2048 | 每次 `run_tests` / `exec_command` 的内存硬上限(MiB);0 用 package 默认 |
| `verify_cpu_limit_seconds` | 600 | 同上,CPU 时间(秒);跟 wall 超时(默认 300)是两回事 |

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
| `log_attach_max_bytes` | `52428800`(50 MiB) | `--log` / `/log` / 自动检测的总字节上限 |
| `trace_attach_max_bytes` | `52428800`(50 MiB) | `--htrace` / `/htrace` 的字节上限(独立) |
| `log_triage_enabled` | `true` | log_triage 预阶段 |
| `log_triage_two_step_enabled` / `log_triage_two_step_bytes` / `log_triage_two_step_coverage` | `true` / 32 KiB / 0.3 | 大日志的两步 fallback |
| `log_triage_max_llm_calls` | 12 | 单次 Run log_triage LLM 调用上限 |
| `log_triage_source_prefix` | `""` | 等价 `--log-source-prefix`(yaml 持久版) |
| `perf_triage_enabled` | `true` | perf_triage 预阶段(同上结构) |

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

`--color={auto,always,never}` 命令行覆盖所有 yaml;`NO_COLOR=1` 环境变量永远强制关闭(no-color.org 标准)。

### 还有更多

`gate_*` / `analysis_*` / `explore_*` / `agent_*` / `cgec_*` / `evidence_*` / `summary_cap_*` / `memory_*` / `memory_policy_*` / `citation_quote_max_chars` / `agent_prior_conversation_policy` / `agent_investigation_complete_policy` / `agent_planner_soft_iter_cap` 等几十个调参旋钮 — 它们影响 LLM 的 internal heuristic 阈值,**新手通常不需要改**。完整字段见 `codrax.yaml.example`,每个都有行内注释说明。

## 5.3 配置查找顺序

启动时按这个顺序找 `codrax.yaml`,首个命中即用:

1. `$CODRAX_SETTINGS` 环境变量指向的路径
2. `<可执行文件目录>/codrax.yaml`(推荐)
3. `<可执行文件目录>/codrax/codrax.yaml`
4. 三个历史 `config/` 路径(已 deprecated,启动时打 WARN)

`providers.yaml` 默认在二进制同目录;用 `--providers /path/to/providers.yaml` 覆盖。

**优先级**(低到高):

```
代码默认值 < codrax.yaml < 命令行 flag
```

只有这些 flag 会覆盖 yaml:`--repo` / `--branch` / `--multi-repo` / `--lang` / `--log-level` / `--log-dir` / `--log-stdout` / `--memory-dir` / `--cache-dir` / `--pipeline-max-steps` / `--pipeline-max-retries` / `--pipeline-max-stage-visits` / `--max-prescan-rounds` / `--log` / `--log-text` / `--log-source-prefix` / `--htrace` / `--htrace-text` / `--atrace` / `--atrace-text` / `--chitchat-classifier` / `--mode` / `--auto-apply` / `--plan-out` / `--plan-file` / `--auto-init-repo` / `--allow-scaffold` / `--color` / `--mermaid-render`。

---

# 6. REPL 命令参考

REPL 启动后,任何以 `/` 开头的输入是斜杠命令;TAB 自动补全。`/help` 列出所有命令。

| 命令 | 用途 |
|---|---|
| `/help` | 列出所有命令 |
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

**写模式专用**(`write_enabled: true` 才能用):

| 命令 | 用途 |
|---|---|
| `/mode read` / `plan` / `apply` / `verify` | 切换粘滞模式 |
| `/plan show` | 渲染当前 pending plan(per-file diff,16 KB 上限) |
| `/plan show <id>` | 按 ID 渲染任意 plan |
| `/plan list` | 列出 PlanStore 里所有 plan |
| `/plan clear` | 丢弃当前 pending plan(不入 memory) |
| `/plan clear <id>` | 删除指定 plan |
| `/plan clear --all` / `--status=<state>` | 批量清(交互 y/N) |
| `/approve` | 批准当前 pending plan,在 worktree 内 apply + verify |
| `/approve <id>` 或 `/approve --plan-id=<id>` | 指定 plan ID |
| `/approve --skip-verify` | 仅 apply,跳过 verify |
| `/approve --merge-to=<branch>` | apply 通过后立即 merge |
| `/reject [reason]` | 拒绝当前 pending plan(理由记入 memory) |
| `/verify [<id>]` | 对已 apply 的 plan 重跑 verify |
| `/worktree list` | 列出保留的 worktree |
| `/worktree discard <id>` | 删除指定 worktree |
| `/merge` | 把 worktree 合回当前 branch(fast-forward) |
| `/merge --branch=<name>` | 在主仓拉新分支并 cherry-pick |
| `/merge --include-failed` / `--force` | 把 verify_failed plan 纳入候选 |
| `/baseline` | 显示当前 baseline 测试快照(`pipeline_baseline_capture_enabled` 打开时可用) |
| `/baseline clear` | 清掉 baseline 缓存 |
| `/phase` | 多阶段方案组的当前进度;每阶段独立 retry 预算,同 worktree 累积 |
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
| `--mode <read\|plan\|apply\|verify>` | `read` | 流水线模式;非 read 需 `write_enabled=true` |
| `--auto-apply` | `false` | 单次 `--mode=apply` 必须搭配,跳过交互确认 |
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

# 写模式:产 plan + 落盘
codrax --mode=plan -r "把 foo 拆成两个函数" --plan-out /tmp/plan.json

# 写模式:批准并执行已有 plan(单次,不开 REPL)
codrax --mode=apply --plan-file=/tmp/plan.json --auto-apply

# 写模式:重跑 verify
codrax --mode=verify --plan-file=/tmp/plan.json
```

CLI 单次模式输出:

- **stderr**: 进度 / spinner / 调试信息
- **stdout**: 最终答案纯文本(mermaid / markdown 都按源码输出,方便重定向到文件 / 转给其他工具)
- **`.codrax/output/<时间戳>-<pid>.md`**: 每次 read 模式问答的最终答案落盘留底,文件分两段 `# 问题` / `# 回答`,内容是模型原文的轻度排版版本(图表保持源码,不二次渲染)。REPL 多轮对话每轮一份。默认保留最近 10 份,旧的按 mtime 自动删。失败的中间重试不会写盘 — 只留用户实际看到的最后一版。写模式 plan / apply / verify **不生成**这种文件。开关 `output_dump_enabled`、份数 `output_max_files` 见 5.2 节。
- **REPL 浏览器预览**: 当 `markdown_preview_server: auto|on` 且 output dump 成功时,REPL 会在答案下方显示一个带随机 token 的本地 URL。预览服务只服务当前进程登记过的 `.md` 文件,默认监听 `0.0.0.0:0`(系统随机端口),可通过 `markdown_preview_host` / `markdown_preview_port` 固定。页面内嵌固定版本 Mermaid JS,适合查看终端里容易变形的 Mermaid、表格和长代码块。

---

# 8. 排错

## 8.1 启动时

**`providers config not found`** 或 `provider/api_key/model/base_url is required`
→ 5.1 节;4 个字段必填,缺一个就拒启动。

**`x509: certificate signed by unknown authority`**
→ 公司自签 CA。`tls_ca_file: /path/to/corp-bundle.pem` 写进 `providers.yaml`。

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

**最终答案空白 / `(no content rendered)`**
→ analyzer 拒绝了请求,或 LLM 返空。看 `<CWD>/.codrax/logs/codrax-*.log` 的 ERROR / WARN。

**记忆里有错误回合污染下轮**
→ codrax 自动把错误回合用占位文替代,但若已经污染,`/clear` 一次。

**写模式 verify 老失败**
→ 看 `/history` 的 verify 失败摘要;调高 `pipeline_write_retry_budget`(让 codrax 多重 plan 几次);或者 `/approve --skip-verify` 跳过测试,本地手动验证。

## 8.3 性能

**REPL 启动慢**
→ 第一次启动会扫整个仓建 repomap 索引。后续走 cache(`cache_dir`),快得多。

**单次 Run 太慢 / token 太贵**
→ analyzer / extractor / chitchat_classifier 路由到便宜模型(5.1.6);或 `pipeline_max_steps` 调小。

**`repomap: tier degradation` WARN**
→ 某种语言的 tree-sitter 解析失败率偏高。问题不大,但建议升级 codrax 或反馈给团队。

**扫描超大仓库时进程被杀(`Killed` / dmesg 里有 OOM)**
→ 在内存偏小的机器上扫巨型仓库(如 Linux 内核)时,repomap 全量扫描可能耗尽宿主 RAM。codrax 默认已三管齐下缓解:启动设 GOMEMLIMIT 软上限、解析后立即回收内存、被中断的扫描下次自动从已落盘 chunk 续扫(见 5.2「大仓内存韧性」)。若仍被杀:① 临时加 swap 让首次扫描扛过峰值、把 cache 建出来;② 用 `--repo` 指向更小的子目录而非整棵树;③ 内存极小的机器可调低 `memory_soft_limit_fraction`。日志里的 `repo_map: resuming interrupted scan` 行说明续扫已生效。

## 8.4 写模式特有

**`/mode plan` 报 `write_enabled is false`**
→ `codrax.yaml` 加 `write_enabled: true`,重启 codrax。

**`/approve` 报 `target ... is needs_init`**
→ 目标目录不是 git 仓。`/approve --auto-init-repo` 一次,或 yaml 里 `write_auto_init_repo: true` 长期允许。

**plan 模式在空目录卡住或报 "scaffold 授权"**
→ 空目录从零创建项目需要单独授权。在 yaml 里同时设 `write_auto_init_repo: true` + `write_scaffold_enabled: true`,或启动时同时加 `--auto-init-repo --allow-scaffold`。两者职责不同 — 前者授权初始化 git,后者授权凭空生成文件。

**runner 检测错了 / runner 不存在**
→ codrax 自动探测 12 种 runner(go / node(jest+vitest)/ pytest / cargo / mvn / gradle(含 Kotlin/Android)/ cmake(ctest)/ meson / make / cjpm / hvigor / rspec / swift)。`runner_missing` 信号识别"二进制没装"(`pytest: command not found` 等),自动跳过 verify→plan 重试,显示推荐安装命令(`env_recommend` 启动一条诊断 → 推荐 → 双语渲染管线;命令以 `!` 前缀给出方便复制)。也可以在 verifier prompt 里声明 `runner=<choice>` + `working_dir` 显式指定,绕过自动探测。

**`/merge` 说 "no worktree to merge from"**
→ apply 完 worktree 被清了。`codrax.yaml` 加 `pipeline_keep_worktree_on_success: true`,下次 apply 后 worktree 会保留。

---

## 附:学习路线建议

1. 先用读模式,问几个仓里熟悉的问题,体会 citation 验证流程
2. 加 `--log` 跑一次 panic 排查,体会 log_triage 的精确度
3. 多轮转换("换成表格""画 mermaid"),体会 turn-policy 路由
4. 开写模式,做一个 1-3 文件的小重构 plan → approve → merge
5. 按需调 `providers.yaml` 把贵 agent 路由到大模型、便宜 agent 路由到小模型,把成本压下来

---

> 本指南覆盖 codrax v0.1.x 的全部用户面命令与配置。底层架构、调度算法、各种 gate 设计请参考 `docs/architecture.md` 与 `CLAUDE.md`。报告问题 / 建议改进:`hanssccv@gmail.com`。
