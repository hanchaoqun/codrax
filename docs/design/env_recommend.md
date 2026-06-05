# 编译/测试环境诊断 + 推荐(env_recommend)设计

> 状态:**v2 设计冻结**(2026-04-28)。
>
> 决策点 1-5 已与客户确认。客户要求**全场景闭环**——8 阶段 ~9.5 周 ~8315 LOC,覆盖所有 12 runner + 裸仓 + LLM 兜底 + 完整文档。
>
> 范围:档 1(诊断 + 推荐)+ 档 2 LLM 兜底,合并为单次工程线但分 8 阶段渐进发布。**不**触及档 3(主动改宿主机环境)。
>
> v2 修订点(相对 v1):§14 工作量重估(基于实测代码量)、§15 落地节奏拆为 8 阶段、§22 复用清单和数据驱动决策点新增。

## 1. 背景与诉求

客户希望 codrax 在 verify 失败 / runner_missing / 缺依赖时,**比现在的硬编码 hint 更智能** —— 给出基于真实环境的精确推荐命令,而不是干瘪的"请安装 pytest"。

约束:codrax 是 read-only-by-default code-analysis tool。**不**自动改宿主机。所有推荐均以 `!cmd` 前缀的可复制命令呈现,执行权完全在用户。

## 2. 设计原则(红线)

| 原则 | 内核 |
|---|---|
| **R1 LLM 仅做兜底** | 主路径全确定性。LLM 仅在确定性返回空 / 未知时被叫。 |
| **R2 全 schema-bound 输出** | LLM 决策走 `emit_*` 风格结构化输出,不允许散文 |
| **R3 全部可缓存** | LLM 决策结果(包名翻译、入口推断)写到 `~/.codrax/cache/env-cache.json`,首次后零调用 |
| **R4 cheap model 路由** | 走 `providers.yaml :: agents.env_recommender`,默认 fallback 到 `chitchat_classifier` |
| **R5 完全可关** | yaml `env_recommend_llm_enabled` 默认 true 带 5s 超时;`false` 即纯确定性 |
| **R6 不破坏现有** | 关闭新功能时回退到当前 `runnerInstallHint` 的 hardcoded 串,字节级不变 |
| **R7 无副作用** | codrax 永远不**执行**推荐命令,只**渲染** `!` 前缀给用户 |
| **R8 全局/sudo 推荐降级** | `recommend_global_install` 默认 false。enabled 时仍带红色 sudo warning |

## 3. 客户决策(已确认)

| # | 决策点 | 选择 |
|---|---|---|
| 1 | 阶段 1 + 阶段 2 是否同时发 | **一起发** |
| 2 | LLM 仅作兜底原则 | **同意** |
| 3 | cheap model 路由可接受度 | **接受** |
| 4 | 缓存位置 | **`~/.codrax/cache/env-cache.json`** |

## 4. 三层架构

```
                ┌──────────────────────────────────────┐
                │  Layer 1: Diagnoser                  │
                │  - 已有 detectRunnerMissing 抽到此处  │
                │  - 输出 Diagnosis{Kind, Tool, Source}│
                └────────────┬─────────────────────────┘
                             ↓
                ┌──────────────────────────────────────┐
                │  Layer 2: EnvProbe                   │
                │  - PATH 工具 / 包管理器 / 项目元数据  │
                │  - 输出 EnvFacts (single struct)     │
                └────────────┬─────────────────────────┘
                             ↓
                ┌──────────────────────────────────────┐
                │  Layer 3: Recommender                │
                │  Stage 1: 确定性表(per-runner)       │
                │  └→ 命中?  返回 Recommendation list  │
                │  Stage 2: 缓存查询(env-cache.json)   │
                │  └→ 命中?  返回缓存结果              │
                │  Stage 3: LLM 兜底(限时 5s)          │
                │  └→ 写回缓存,返回结果                │
                │  Stage 4: 全部 miss → DocsLink only  │
                └────────────┬─────────────────────────┘
                             ↓
                ┌──────────────────────────────────────┐
                │  Renderer: 双语 + ! 前缀渲染          │
                │  写入 FailureSummary / ToolResult     │
                └──────────────────────────────────────┘
```

## 5. 数据契约

### 5.1 Diagnosis(Layer 1 输出)

```go
type DiagKind string

const (
    DiagRunnerMissing         DiagKind = "runner_missing"          // pytest / cargo / mvn 不在 PATH
    DiagDepsMissing           DiagKind = "deps_missing"            // node_modules 缺 / pyproject 依赖未装
    DiagBuildToolchainMissing DiagKind = "toolchain_missing"       // gcc / python3 / nodejs 本身缺
    DiagSystemLibMissing      DiagKind = "system_lib_missing"      // -lssl / libGL.so 缺
    DiagConfigMissing         DiagKind = "config_missing"          // 找不到 pyproject.toml / package.json
    DiagUnknown               DiagKind = "unknown"                 // 确定性 detector 未匹配 → LLM 兜底入口
)

type Diagnosis struct {
    Kind        DiagKind
    Runner      string  // python / node / go / java / ruby / rust / swift / cmake / meson / make / hvigor / cjpm / ""(系统级)
    Tool        string  // 具体缺什么:pytest / node_modules / cmake / openssl-dev
    Source      string  // exit_127 / pattern_match / file_stat / cargo_lock_drift / llm_inferred
    Excerpt     string  // 真实 stderr 切片(给 LLM 兜底用)
    Confidence  float32 // 1.0 = 确定性,< 1.0 = LLM 兜底打的分
}
```

### 5.2 EnvFacts(Layer 2 输出)

```go
type EnvFacts struct {
    OS              string                       // linux / darwin / windows
    Arch            string                       // amd64 / arm64
    OSFamily        string                       // debian / rhel / arch / alpine / darwin / windows
    OSVersion       string                       // "ubuntu 22.04" / "macos 14.3"
    Shell           string                       // bash / zsh / fish / pwsh
    PathTools       map[string]string            // bin name → resolved path
    PkgManagers     map[string]PkgManagerInfo    // apt / dnf / brew / pip / pipx / cargo / npm / pnpm / yarn / bun / gem / bundler / poetry / hatch / uv / pixi
    ProjectFiles    map[string]string            // 仓内 pyproject.toml / package.json / Cargo.toml / Gemfile / go.mod / pom.xml / build.gradle[.kts] 等的绝对路径
    Pythons         []PythonInterp               // 多个 python 解释器(系统/venv/pyenv/conda)+ 各自 site-packages 已装
    Nodes           []NodeInterp                 // node + npm/pnpm/yarn 配套
    Rusts           []RustToolchain
    Javas           []JavaToolchain              // jdk + maven + gradle wrapper
    Rubies          []RubyToolchain
    Network         NetworkStatus                // {Reachable bool; ProxySet bool; Probe time.Time}
    InsideContainer bool                         // /proc/1/cgroup heuristic
    ProbedAt        time.Time
}

type PkgManagerInfo struct {
    Path    string  // /usr/bin/apt
    Version string  // 解析失败留空
}

type PythonInterp struct {
    Path             string   // /usr/bin/python3
    Version          string   // 3.11.5
    Origin           string   // system / venv / pyenv / conda
    HasPytest        bool     // sys.modules pytest 直检
    HasJsonReport    bool     // pytest-json-report
    SitePackagesPath string
}
```

EnvFacts 探测仅在 BusContext 创建时跑**一次**(per Run),缓存到 `BusContext.Memory`(暂借)或新加 `BusContext.Env`。15-30ms 量级。

### 5.3 Recommendation(Layer 3 输出)

```go
type InstallStrategy int

const (
    StrategyVenvInstall    InstallStrategy = iota  // 项目隔离;最安全
    StrategyProjectInstall                          // 项目目录内(npm install / cargo fetch)
    StrategyUserInstall                             // ~/.local/bin / pip --user
    StrategyToolchainBootstrap                      // rustup-init / nvm / pyenv 等
    StrategyGlobalInstall                           // sudo apt / brew / npm install -g — 默认禁用
    StrategyDocsLink                                // 没好命令,只能给文档链接
)

type Recommendation struct {
    Strategy     InstallStrategy
    Command      string    // 带 ! 前缀,直接复制粘贴
    Why          string    // "你 PATH 上有 pip 且当前是 pyproject 项目"
    NeedsNetwork bool
    NeedsSudo    bool
    EstimatedSec int       // 5/30/60/300+
    Caveats      []string  // ["~/.local/bin 需要在 PATH 中","离线请用 mirror"]
    Priority     int       // 同诊断多候选时排序;低数字优先
    DiagRef      Diagnosis // 反向溯源
}
```

## 6. Layer 1: Diagnoser

### 6.1 现有可复用

`run_tests.go::detectRunnerMissing` 已经做了 runner_missing 的三信号检测(exit 127 / ErrNotFound / pattern)。**直接抽到 `internal/env/diag/runner_missing.go`**,run_tests 调新版 API。

### 6.2 新加的 Diagnoser

| Kind | 触发器 | 文件 |
|---|---|---|
| `DiagDepsMissing` | 解析 `pyproject.toml [project.dependencies]` / `package.json dependencies`,与 site-packages / node_modules 对比 | `diag/deps.go` |
| `DiagBuildToolchainMissing` | EnvFacts 上看 `Pythons`/`Nodes`/`Rusts`/`Javas` 全空 + runner 需要它 | `diag/toolchain.go` |
| `DiagSystemLibMissing` | stderr 含 `library not found for -l<lib>` / `<header>.h: No such file` / `lib*.so: cannot open` | `diag/syslib.go`(简单 pattern;复杂 case 转 LLM 兜底) |
| `DiagConfigMissing` | runner 需要 manifest 但 EnvFacts.ProjectFiles 缺对应键 | `diag/config.go` |
| `DiagUnknown` | 上述全 miss + 仍 build_failure → 进 LLM 兜底 | (no detector;由 dispatcher 进 LLM 路径) |

### 6.3 Diagnoser dispatcher

```go
// internal/env/diag/dispatch.go
func Diagnose(stderr string, runner string, env EnvFacts) Diagnosis
```

按上表顺序匹配,首个命中即返回。全 miss → `Diagnosis{Kind: DiagUnknown}`。

## 7. Layer 2: EnvProbe

### 7.1 探测顺序与超时

```
StartProbe() → 三个并发 goroutine:
  ├─ probeOSAndShell()        // 5ms
  ├─ probePkgManagers()       // 50ms (并发 LookPath 12 个)
  ├─ probeProjectFiles()      // 20ms (扫 repo root 一层)
join → 串行:
  └─ probePythons()           // 100ms (多解释器 + site-packages 检查)
  └─ probeNodes()             // 50ms
  └─ probeRusts() / Javas() / Rubies()  // 各 ~30ms
joinNetwork():
  └─ probeNetwork()           // 200ms,可关
                              // 总计 ~400ms;只跑一次,缓存到 BusContext
```

总体 ~400ms,只跑一次。`env_probe_network: false` 关网络探测节省 200ms。

### 7.2 文件布局

```
internal/env/probe/
  probe.go              // 入口 + 总线
  probe_os_unix.go      // Linux/Darwin OSFamily + version
  probe_os_windows.go   // Windows OSFamily + version
  probe_shell.go
  probe_pkgmgr.go       // apt/dnf/yum/brew/pacman/zypper/apk/pkg/winget/scoop/choco
  probe_python.go       // 抽自 run_tests.go::pythonInterpreter
  probe_node.go
  probe_rust.go
  probe_java.go
  probe_ruby.go
  probe_network.go      // ping 1.1.1.1 + DNS resolve test
  cache.go              // BusContext-级缓存
```

### 7.3 复用现有

- `pythonInterpreter`(`run_tests.go:1014`)直接抽进 `probe_python.go`。run_tests 改成调 `EnvFacts.Pythons[0]`。
- `linkProjectDeps`(`run_tests_link_deps.go`)的"找 .venv / node_modules"逻辑也属于 EnvFacts 的范畴,后续可统一。

## 8. Layer 3: Recommender

### 8.1 Stage 1 — 确定性表(per-runner)

每个 runner 一个文件,实现:

```go
type Recommender interface {
    Recommend(d Diagnosis, env EnvFacts) []Recommendation
}
```

注册:

```go
// internal/env/recommend/registry.go
var recommenders = map[string]Recommender{
    "python":  &pythonRecommender{},
    "node":    &nodeRecommender{},
    "rust":    &rustRecommender{},
    "java":    &javaRecommender{},
    "ruby":    &rubyRecommender{},
    "go":      &goRecommender{},
    "swift":   &swiftRecommender{},
    "cmake":   &cmakeRecommender{},
    "meson":   &mesonRecommender{},
    "make":    &makeRecommender{},
    "hvigor":  &hvigorRecommender{},
    "cjpm":    &cjpmRecommender{},
    "":        &systemRecommender{},  // 系统级(toolchain_missing / system_lib_missing 但 runner 未知)
}
```

每文件 ~80 LOC,以 Python 为例(伪代码):

```go
func (p *pythonRecommender) Recommend(d Diagnosis, env EnvFacts) []Recommendation {
    var out []Recommendation
    switch d.Kind {
    case DiagRunnerMissing:
        if d.Tool == "pytest" {
            // 候选 1:venv 隔离(优先)
            if env.HasPython() {
                out = append(out, Recommendation{
                    Strategy: StrategyVenvInstall,
                    Command: "!python3 -m venv .venv && .venv/bin/pip install pytest pytest-json-report",
                    Why: pythonVenvWhy(env),
                    NeedsNetwork: true,
                    EstimatedSec: 30,
                    Priority: 1,
                })
            }
            // 候选 2:user-install
            if env.HasPip() {
                out = append(out, Recommendation{
                    Strategy: StrategyUserInstall,
                    Command: "!pip install --user pytest pytest-json-report",
                    Why: "已有 pip;~/.local/bin 装到用户级",
                    NeedsNetwork: true,
                    EstimatedSec: 10,
                    Priority: 2,
                    Caveats: []string{"确保 ~/.local/bin 在 PATH 里"},
                })
            }
            // 候选 3:conda 环境(若 origin=conda)
            if hasCondaPython(env) {
                out = append(out, Recommendation{...})
            }
        }
    case DiagBuildToolchainMissing:
        if d.Tool == "python3" {
            out = append(out, perOSPython3Install(env)...)
        }
    case DiagDepsMissing:
        out = append(out, pythonDepsInstall(env)...)
    }
    return out
}
```

### 8.2 Stage 2 — 缓存

`~/.codrax/cache/env-cache.json` 结构:

```json
{
  "schema_version": 1,
  "entries": [
    {
      "key": "syslib_translate:debian:openssl",
      "value": {"package": "libssl-dev", "install_cmd": "!sudo apt install -y libssl-dev"},
      "source": "llm",
      "created_at": "2026-04-28T14:30:00Z",
      "ttl_days": 90
    }
  ]
}
```

- 路径:`os.UserHomeDir()` + `.codrax/cache/env-cache.json`(POSIX),Windows `%LocalAppData%\codrax\cache\env-cache.json`
- TTL:90 天,过期即重新查询
- 文件锁:`flock` 或 `os.Rename`(写入到 `.tmp` 再 rename)防并发损坏
- 缓存键 schema:
  - `syslib_translate:<os_family>:<lib>` → 包名 + 安装命令
  - `entry_inference:<sha8(project_files)>` → 项目入口推荐
  - `error_diagnosis:<sha8(stderr_norm)>` → 诊断结果(stderr 归一化后哈希)

归一化:把 stderr 里的绝对路径、PID、时间戳、git SHA 替换成占位符,保证同类错误命中同一缓存。

### 8.3 Stage 3 — LLM 兜底

#### 触发条件(三选一)

1. Stage 1 返回空 + Stage 2 miss → 用 LLM 推荐命令
2. `Diagnosis.Kind == DiagUnknown` → 用 LLM 重新分类
3. `Diagnosis.Kind == DiagSystemLibMissing` 且 OS 包名表 miss → 用 LLM 翻译

#### LLM 调用规范

- Provider:`providers.yaml :: agents.env_recommender`(新增);未配置则 fallback 到 `chitchat_classifier`(已有 cheap model 路由)
- 超时:5 秒
- 失败兜底:返回空 → Renderer 退化到 hardcoded `runnerInstallHint`
- 输出 schema 强制(类似 `emit_change_plan`):
  ```go
  // emit_env_recommendation 工具 schema
  {
    "diagnosis": {
      "kind": "system_lib_missing",   // enum
      "runner": "python",              // enum
      "tool": "openssl",
      "confidence": 0.85
    },
    "recommendations": [
      {
        "strategy": "global_install",  // enum
        "command": "!sudo apt install -y libssl-dev",
        "why": "你的 stderr 显示 ld 找不到 libssl",
        "needs_network": true,
        "needs_sudo": true,
        "estimated_sec": 30,
        "caveats": ["这是发行版级安装,影响所有项目"]
      }
    ]
  }
  ```
- schema 不合规即丢弃(同 `emit_change_plan` 11-stage 验证);系统不会用 LLM 写错的命令

#### LLM 调用的输入

```
[system]
你是 codrax 的环境诊断助手。给定环境画像 + stderr 切片,emit 一条 emit_env_recommendation 工具调用。

约束:
- strategy 优先 venv > project > user > toolchain_bootstrap > docs_link;global 仅在前面都不行时才用,且必须 needs_sudo=true
- command 必须以 ! 开头(用户复制粘贴入口)
- 不允许散文,只用结构化字段

[user]
环境画像:
- OS: linux 6.8 / debian-derived (ubuntu 22.04)
- PATH 工具: python3 (/usr/bin/python3,3.10.12), pip (/usr/bin/pip3),没装 pytest
- 项目: pyproject.toml (poetry-managed) / Pipfile.lock 也存在
- 网络: 可达

stderr 切片:
ld: library not found for -lssl
clang: error: linker command failed with exit code 1

请诊断并推荐。
```

### 8.4 Stage 4 — 全部 miss

兜底渲染:DocsLink + 现有 `runnerInstallHint` 字符串。**保证回归 = 现有行为**。

## 9. Renderer

```go
// internal/env/render.go
func RenderRecommendations(diag Diagnosis, recs []Recommendation, lang string) string
```

输出形态(zh):

```
✗ pytest 未在 PATH。
诊断:python runner;Python 3.10 解释器在 /usr/bin/python3,但 pytest 模块未安装。

推荐(命中你当前环境 = pip + pyproject.toml):
  !python3 -m venv .venv && .venv/bin/pip install pytest pytest-json-report  # ~30s,项目隔离,推荐
  !pip install --user pytest pytest-json-report                                # ~10s,用户级
                                                                              ⚠ ~/.local/bin 需在 PATH 里

复制以 ! 开头的命令到 codrax 提示符回车执行,或在主仓终端直接跑(去掉 !)。
```

英文版同步。沿用 `isZh()`。

写入路径:
- `internal/tool/run_tests.go::runnerMissingToolResultSummary` 切到 `RenderRecommendations`
- `internal/tool/run_tests.go::makeRunnerMissingReport`'s `FailureSummary` 字段同切

## 10. `/env` REPL 命令

```
> /env

Probed environment (2026-04-28 14:30, 387ms):
  OS:           linux 6.8 / amd64 / debian-derived
  Shell:        bash
  Network:      reachable

  Pythons:
    /usr/bin/python3.10           system     pytest=NO  json-report=NO
    .venv/bin/python (3.10.12)    venv       pytest=8.0.0  json-report=1.5.0  ← preferred

  PkgManagers:  apt(/usr/bin/apt 2.4), pip3(/usr/bin/pip3), poetry(1.7.1)

  Project:      pyproject.toml (poetry-managed)

  Cache:        ~/.codrax/cache/env-cache.json (12 entries, last write 2 days ago)
```

子命令:
- `/env` 等同 `/env show`
- `/env probe` 强制刷新缓存
- `/env explain` LLM 模式生成对话语气解释(场景 D)
- `/env cache list / clear` 缓存管理

## 11. 配置

### 11.1 codrax.yaml 新增 keys

```yaml
# Master switch — 完全可关,关掉就回到当前 hardcoded hint
# env_recommend_enabled: true

# 限时;LLM 路径在此时长内必须返回,否则丢弃回退
# env_recommend_llm_timeout_sec: 5

# LLM 兜底是否启用(R5 红线);false 即纯确定性
# env_recommend_llm_enabled: true

# 启用 sudo 全局安装类推荐(R8 红线);默认 false
# recommend_global_install: false

# 网络探测开关(可省 200ms 启动)
# env_probe_network: true

# 缓存 TTL(天)
# env_cache_ttl_days: 90
```

### 11.2 providers.yaml 新增

```yaml
llm:
  agents:
    env_recommender:
      model: gpt-4o-mini   # 或任意 cheap model
      think_aloud: false
      # 未配置时 fallback 到 chitchat_classifier
```

## 12. 文件布局总览

```
internal/env/
├── diag/                         # Layer 1
│   ├── dispatch.go
│   ├── runner_missing.go         # 抽自 run_tests.go::detectRunnerMissing
│   ├── deps.go
│   ├── toolchain.go
│   ├── syslib.go
│   └── config.go
├── probe/                        # Layer 2
│   ├── probe.go
│   ├── probe_os_unix.go
│   ├── probe_os_windows.go
│   ├── probe_shell.go
│   ├── probe_pkgmgr.go
│   ├── probe_python.go           # 抽自 run_tests.go::pythonInterpreter
│   ├── probe_node.go
│   ├── probe_rust.go
│   ├── probe_java.go
│   ├── probe_ruby.go
│   ├── probe_network.go
│   └── cache.go                  # ~/.codrax/cache/env-cache.json
├── recommend/                    # Layer 3
│   ├── registry.go
│   ├── recommend.go              # dispatcher (确定性 → 缓存 → LLM 兜底 → DocsLink)
│   ├── llm_fallback.go           # LLM 调用 + emit_env_recommendation schema
│   ├── llm_diagnose.go           # DiagUnknown → LLM 分类
│   ├── llm_pkg_translate.go      # syslib 跨发行版翻译
│   ├── strategy.go               # InstallStrategy + 优先级
│   ├── recommend_python.go
│   ├── recommend_node.go
│   ├── recommend_rust.go
│   ├── recommend_java.go
│   ├── recommend_ruby.go
│   ├── recommend_go.go
│   ├── recommend_swift.go
│   ├── recommend_cmake.go
│   ├── recommend_meson.go
│   ├── recommend_make.go
│   ├── recommend_hvigor.go
│   └── recommend_cjpm.go
├── render.go                     # 双语渲染
└── facts.go                      # EnvFacts struct + 共用类型
```

## 13. 测试方案

### 13.1 Diagnoser 单测

每个 Diagnoser 一个 `_test.go`,fixture 是预录的 stderr + 预期 Diagnosis。零 LLM 依赖。

### 13.2 Recommender 单测(per-runner)

mock EnvFacts → assert Recommendation list 包含期望命令 / 优先级 / 战略。

```go
func TestPython_RunnerMissing_VenvFirst(t *testing.T) {
    env := EnvFacts{
        OSFamily: "debian",
        PathTools: map[string]string{"python3": "/usr/bin/python3"},
        ProjectFiles: map[string]string{"pyproject.toml": "/abs"},
    }
    diag := Diagnosis{Kind: DiagRunnerMissing, Runner: "python", Tool: "pytest"}
    recs := (&pythonRecommender{}).Recommend(diag, env)
    if recs[0].Strategy != StrategyVenvInstall {
        t.Errorf("top1 should be VenvInstall; got %v", recs[0].Strategy)
    }
    if !strings.Contains(recs[0].Command, "python3 -m venv") {
        t.Errorf("expected venv command; got %q", recs[0].Command)
    }
}
```

### 13.3 缓存层测试

- 写入 → 读出,key/value 一致
- TTL 过期不返回旧值
- 并发写入不损坏文件
- schema_version 不匹配丢弃

### 13.4 LLM 兜底测试

- Mock LLM 返回合规 emit_env_recommendation → 正确解析
- Mock LLM 返回散文/格式不对 → 丢弃 + 兜底
- Mock LLM 超时 → 5s 后丢弃 + 兜底
- 多次同样输入 → 第二次走缓存(无 LLM 调用)

### 13.5 集成测试

- 真实仓 fixture(Python pyproject 项目)+ mock pytest 缺失 stderr → 端到端验证产出
- `/env` 命令端到端
- yaml 关闭 LLM 路径 → 退化到现有行为字节级一致

## 14. 工作量(v2 重估,基于实测代码量)

读了现有代码后,前一版工作量低估了 76%。差距来自:跨平台 Probe 的实际复杂度、缓存 corner cases、per-Recommender 测试覆盖、文档系统刷新四块。

### 14.1 已读代码基线

| 现有可复用组件 | 实测 LOC | 复用形式 |
|---|---|---|
| `run_tests.go::detectRunnerMissing` | 54 | 抽 → `internal/env/diag/runner_missing.go` |
| `run_tests.go::runnerInstallHint` | 69 | 重写为 Recommender 表 |
| `run_tests.go::pythonInterpreter` | 60 | 抽 → `internal/env/probe/probe_python.go`(run_tests 包装调用) |
| `run_tests_runner_missing_test.go` | 379 | 大部分平移到 `env/diag` 包 |
| `worktree.DetectRepoState` | 87 | 复用,包到 `env/diag/git_state.go` |
| `worktree.EnsureInitialCommit` | 51 | 复用,不动 |
| `worktree`三层 `--auto-init-repo` 授权门 | — | 完全不动 |
| `run_tests_link_deps.go::linkProjectDeps` | 128 | 复用,可扩到 EnvProbe.LinkedDeps |
| `memory.MEMORY.md.lock` 文件锁 pattern | — | 借鉴到 `disk_cache.go` |
| `cmd/root.go` chitchat_classifier resolver pattern | — | 复用到 env_recommender provider 解析 |

**纯复用 / 抽取约 470 LOC**——给 8 阶段实现节省约 1.5 周工作量。

### 14.2 总工作量(8 阶段累计)

| 阶段 | LOC(含测试) | 周期 | 累计 LOC | 累计周 |
|---|---|---|---|---|
| 阶段 0 MVP — 探针 + `/env show` | 1285 | 1.0 | 1285 | 1.0 |
| 阶段 1 Python+Node Recommender + 错误流接入 | 1760 | 2.0 | 3045 | 3.0 |
| 阶段 2 Rust+Ruby+Go | 810 | 1.0 | 3855 | 4.0 |
| 阶段 3 磁盘缓存层 | 620 | 1.0 | 4475 | 5.0 |
| 阶段 4 LLM 兜底(诊断 + 包名翻译) | 910 | 1.5 | 5385 | 6.5 |
| 阶段 5 git 状态(裸仓集成) | 510 | 0.5 | 5895 | 7.0 |
| 阶段 6 Java+Swift+CMake+Meson+Make+hvigor+cjpm | 1240 | 1.5 | 7135 | 8.5 |
| 阶段 7 `/env explain` + 集成测试 + 全文档闭环 | 1180 | 1.0 | **8315** | **9.5 周** |

**最终总量约 8315 LOC**,含约 3000 LOC 测试。

### 14.3 与 v1 估计差距来源

| 类别 | v1 估 | v2 实测 | 差因 |
|---|---|---|---|
| 测试覆盖 | 1200 | ~3000 | per-Recommender 单测 + 缓存 corner case + LLM stub + 集成测试 |
| 跨平台 Probe | 250 | ~470 | Windows / 容器 / 多 shell / 多 Python 解释器路径 |
| 缓存层 | 200 | ~620 | flock + atomic rename + TTL + schema_version + 跨平台 home 路径 + REPL 子命令 |
| 文档系统 | 200 | ~830 | user_guide / architecture / CLAUDE / yaml.example / 设计文档 v2 |

## 15. 落地节奏 — v2 全场景闭环(W1-W9.5)

每阶段交付**可发布、可测、可关闭**的增量;前一阶段不阻塞后一阶段。每阶段结束后客户可立即上手用,数据驱动后续节奏微调。

### 阶段 0(W1):MVP — 环境画像 + `/env show`

**范围**:把 Layer 2 EnvProbe 跑通,REPL 加 `/env` 命令展示画像。**不**接错误流。

| 模块 | LOC |
|---|---|
| `internal/env/facts.go` — EnvFacts 结构 + 共用类型 | 80 |
| `internal/env/probe/probe.go` — 入口 + goroutine 调度 | 90 |
| `internal/env/probe/probe_os_unix.go` | 110 |
| `internal/env/probe/probe_os_windows.go` | 70 |
| `internal/env/probe/probe_shell.go` | 30 |
| `internal/env/probe/probe_pkgmgr.go` — 22 个包管理器探测 | 130 |
| `internal/env/probe/probe_python.go` — 抽自 `pythonInterpreter` + 多解释器 + site-packages 检查 | 70 |
| `internal/env/probe/probe_node.go` | 50 |
| `internal/env/probe/cache_facts.go` — per-Run BusContext 缓存 | 60 |
| `internal/repl/handle_env.go` — `/env show` + 双语渲染 | 130 |
| `internal/types/conversation.go` 加 `/env` alias | 10 |
| `internal/repl/input.go` 加 slashCommand entry | 15 |
| `internal/repl/messages.go` 双语 messages(envProbeBanner / envSection) | 60 |
| 单测:Probe fixture + `/env` 渲染 | 380 |
| **小计** | **1285** |

**用户感知**:`/env` 命令展示完整环境画像,其它行为完全不变。

**风险**:低。新增只读子系统,不接现有错误流。

### 阶段 1(W2-W3):Python + Node Recommender + 错误流切换

**范围**:Layer 1 Diagnoser + Layer 3 主流两语言 Recommender + Renderer + run_tests 错误消息切换。覆盖现实 ~70% 场景。

| 模块 | LOC |
|---|---|
| `internal/env/diag/dispatch.go` | 80 |
| `internal/env/diag/runner_missing.go` — 抽自 `detectRunnerMissing` | 70 |
| `internal/env/diag/deps.go` — pyproject / package.json deps 比对 | 100 |
| `internal/env/diag/config.go` | 50 |
| `internal/env/recommend/registry.go` — 注册表 + dispatcher(含缓存 stub) | 80 |
| `internal/env/recommend/strategy.go` — InstallStrategy + 优先级 | 50 |
| `internal/env/recommend/recommend_python.go` | 130 |
| `internal/env/recommend/recommend_node.go` | 110 |
| `internal/env/recommend/recommend_system.go` — 系统级占位(toolchain / build-essential) | 70 |
| `internal/env/render.go` — 双语 RenderRecommendations | 130 |
| `internal/tool/run_tests.go` — runnerMissingToolResultSummary + makeRunnerMissingReport 切到 Renderer | 60 |
| `internal/types/config.go` — EnvRecommendSettings + Default + Resolve | 60 |
| `internal/config/runtime.go` — 6 个新 yaml key | 70 |
| `codrax.yaml.example` — 完整注释块 | 50 |
| 单测:per-Recommender + Render + run_tests 集成 | 580 |
| 文档刷新:user_guide.md + CLAUDE.md + architecture.md(阶段 1 范围) | 120 |
| **小计** | **1760** |

**用户感知**:Python / Node 项目缺 pytest / npm 时,错误消息从干瘪 hint 升级为基于环境的精确推荐命令。

**风险**:中。需要确保 `env_recommend_enabled: false` 关闭后字节级回退现行 hint——靠 yaml 旋钮 + 守护测试。

### 阶段 2(W4):Rust + Ruby + Go Recommender

**范围**:补齐另外三个主流语言,覆盖率到 ~85%。

| 模块 | LOC |
|---|---|
| `internal/env/probe/probe_rust.go` | 50 |
| `internal/env/probe/probe_ruby.go` | 50 |
| Go probe — `go env` 命令一次拿全,无独立文件 | 0 |
| `internal/env/recommend/recommend_rust.go` | 90 |
| `internal/env/recommend/recommend_ruby.go` | 90 |
| `internal/env/recommend/recommend_go.go` | 70 |
| `internal/env/diag/toolchain.go` — 编译器/runtime 缺失专用诊断 | 80 |
| 单测 | 380 |
| **小计** | **810** |

### 阶段 3(W5):磁盘缓存层

**范围**:为 LLM 兜底铺基础;同时即使 LLM 没接,缓存也能加速跨 Run 的 EnvProbe 重用。

| 模块 | LOC |
|---|---|
| `internal/env/cache/disk_cache.go` — 文件锁 + atomic rename + TTL + schema_version | 200 |
| `internal/env/cache/keys.go` — stderr / project_files 归一化哈希 | 50 |
| 跨 OS user home 路径 — UserHomeDir + 平台 fallback | 30 |
| Recommender 接缓存 — dispatcher 加 cache check 步骤 | 40 |
| `/env cache list / clear` REPL 子命令 | 80 |
| 单测:写入 / 读出 / TTL / 并发 / schema 不匹配 | 220 |
| **小计** | **620** |

**风险**:跨平台路径 + 文件锁的 corner case 需要稳。借鉴 `memory.MEMORY.md.lock`(已成熟 pattern)。

### 阶段 4(W6-W6.5):LLM 兜底

**范围**:未识别的 stderr / 跨发行版包名翻译走 cheap-model LLM,结果走缓存。

| 模块 | LOC |
|---|---|
| `internal/tool/emit_env_recommendation.go` — schema + 11-stage 验证(同 emit_change_plan 思路) | 180 |
| `internal/env/recommend/llm_diagnose.go` — DiagUnknown → LLM 重新分类 | 130 |
| `internal/env/recommend/llm_pkg_translate.go` — OS 包名翻译(缓存命中后零调用) | 120 |
| `internal/env/recommend/llm_dispatch.go` — adapter 调用 + 5s 超时 + 失败兜底 | 90 |
| `providers.yaml :: agents.env_recommender` 路由(沿用 chitchat_classifier resolver pattern) | 30 |
| `cmd/root.go` LLM adapter 解析 | 40 |
| 单测:mock LLM 合规 / 不合规 / 超时 / 缓存命中后零调用 | 320 |
| **小计** | **910** |

**关键架构**:所有 LLM 调用走 `emit_*` 风格 schema(R1/R2 红线);不合规即丢弃;缓存命中跳过 LLM——首次后零调用。

**风险**:中。LLM 可靠性需要 stub 完整覆盖;不合规输出必须 fail-safe 不影响主路径。

### 阶段 5(W7):git 状态(裸仓集成)

**范围**:把客户后补的"git_not_initialized"诉求按同框架做。详见 §19。

| 模块 | LOC |
|---|---|
| `internal/env/diag/git_state.go` — 复用 `worktree.DetectRepoState` + git PATH 探测 | 80 |
| `internal/env/recommend/recommend_git.go` — git 安装 / git init / 三层授权指引 | 120 |
| EnvFacts.GitRepoState 字段 + Probe 接入 | 40 |
| `Orchestrator.Run` 入口 INFO 日志(读模式提示"裸目录") | 30 |
| 写模式错误消息切到 Renderer — apply_pre_hook 现有 hardcoded prose 替换 | 60 |
| 单测:per-OS 包管理器 × 各 git state | 180 |
| **小计** | **510** |

**复用红利**:`worktree.DetectRepoState`(87 LOC)+ `EnsureInitialCommit`(51 LOC)+ 三层授权门完全不动。本阶段几乎纯增量诊断 + 渲染。

### 阶段 6(W8-W8.5):剩余 7 个 Runner

**范围**:Java / Swift / CMake / Meson / Make / hvigor / cjpm,覆盖到 100%。

| Runner | Probe LOC | Recommender LOC |
|---|---|---|
| Java | 70(jdk + maven + gradle wrapper) | 100 |
| Swift | 30 | 50 |
| CMake | 30 | 60 |
| Meson | 30 | 50 |
| Make | 20(仅 PATH 探测) | 40 |
| hvigor | 40 | 60 |
| cjpm | 30 | 50 |
| 系统库诊断(system_lib_missing pattern 表) | — | 100 |
| 单测 | — | 480 |
| **小计** | **250** | **990** |

**总计本阶段**:**1240 LOC**。

**风险**:低-中。hvigor / cjpm 需要本地 HarmonyOS / Cangjie 工具链才能集成测试,否则用 stub。

### 阶段 7(W9-W9.5):`/env explain` + 集成测试 + 全文档闭环

**范围**:LLM 长尾增强 + 真实仓集成测试 + 完整文档刷新。

| 模块 | LOC |
|---|---|
| `internal/repl/handle_env_explain.go` — LLM 生成对话语气解释 | 100 |
| 集成测试:真实 Python pyproject 仓 + Node monorepo + Rust 项目 | 350 |
| `docs/user_guide.md` §3.3.x 新章节(env_recommend 完整说明) | 200 |
| `docs/architecture.md` 加 §env_recommend(架构 + 数据流图) | 150 |
| `CLAUDE.md` memory_* 节加新 yaml keys 描述 | 80 |
| `codrax.yaml.example` 完整注释块 | 200 |
| `docs/design/env_recommend.md` 升级到 v3(实施后总结) | 100 |
| **小计** | **1180** |

**用户感知**:`/env explain` 给对话语气解释决策;真实项目的端到端集成测试守护未来重构;所有文档反映 env_recommend 完整能力。

## 16. 风险与红线

## 16. 风险与红线

| 风险 | 缓解 |
|---|---|
| LLM 推荐错误命令(幻觉 flag) | schema-bound 输出 + 缓存对人审 + 用户复制粘贴是最后一道关 |
| 缓存损坏 / 并发 | flock + 写入 .tmp 再 rename + schema_version 不匹配丢弃 |
| 非 POSIX 路径 | UserHomeDir + filepath.Join 跨平台 |
| 离线环境 | NeedsNetwork=true 标记;离线时降到 DocsLink |
| 公司 proxy | EnvProbe 探 HTTP_PROXY / HTTPS_PROXY,推荐命令带显式 proxy 提示 |
| 大依赖下载时长 | EstimatedSec 标注;> 60s 显眼提示 |
| 不同发行版包名差异 | LLM 翻译 + 缓存,首次后零调用 |
| `recommend_global_install` 误开 | 默认 false + 启用时强 warning |
| sudo 误推荐 | NeedsSudo=true 时双重 confirmation 文案 |

## 17. 决策已冻结

- ✅ 阶段 1 + 阶段 2 一起发(单次工程线,8 阶段渐进发布)
- ✅ LLM 仅做兜底
- ✅ cheap model 路由(`agents.env_recommender` → fallback `chitchat_classifier`)
- ✅ 缓存位置 `~/.codrax/cache/env-cache.json`
- ✅ `recommend_global_install` 默认 false(本设计补充)
- ✅ **v2 客户承诺:全场景闭环** — 阶段 0-7 完整跑完,所有 12 runner + 裸仓 + LLM + 完整文档,~9.5 周 ~8315 LOC

## 18. 下个 session 开工

按 §15 的阶段 0(W1)起手。先 §11 yaml + §12 文件骨架立起来,再填 Probe;从 `/env show` 开始让客户立即看到产出。

每个阶段结束都打 tag(`env-recommend-stage-N`)以便回滚 / 渐进发布。

文档此次仅设计冻结。设计文档版本号:v2.0,日期:2026-04-28。

## 19. 裸仓(无 git)场景 — 折叠到同一框架

客户补充诉求:**没有 git 管理的裸目录**也是一种"环境未就绪",应该走同一套诊断 + 推荐路径。

### 19.1 现状盘点

codrax 已有处理裸仓的机制(`internal/worktree/repo_state.go`),但仅在写模式 `/approve` 触发时才检查:

- `worktree.DetectRepoState(repoRoot)` 返回 `Ready / NotInitialized / NoCommits`
- `worktree.EnsureInitialCommit(repoRoot, msg)` 做 `git init` + 空首次 commit
- **三层授权门**(已存在,沿用):
  1. CLI flag `--auto-init-repo`(per-run)
  2. yaml `write_auto_init_repo: true`(deploy-wide)
  3. REPL `/approve` 前的交互式 y/N 确认
- 错误消息(已存在):"target X is not_initialized — codrax can scaffold..."

**问题 1**:这套机制只在 `/approve` 触发,**读模式 / plan 模式**等其它路径不会先告知用户"你这是裸目录"。用户在 plan 阶段就该知道,不用走到 apply 才发现。

**问题 2**:错误消息是 hardcoded 一段长 prose,跟新框架的"基于环境画像精确推荐"思路不一致。

### 19.2 折叠到 env_recommend 框架

**新增 DiagKind**:

```go
const (
    // ...
    DiagGitNotInstalled         DiagKind = "git_not_installed"          // git 二进制不在 PATH
    DiagGitRepoNotInitialized   DiagKind = "git_not_initialized"        // 不是 git 仓
    DiagGitRepoNoCommits        DiagKind = "git_no_commits"             // 是 git 仓但 HEAD 不存在
)
```

**新增 Diagnoser**:`internal/env/diag/git_state.go`
- 直接复用 `worktree.DetectRepoState`(已存在)
- 加一个 `git` 二进制 PATH 探测(可能 git 都没装)
- 输出 Diagnosis,源标注 `Source: "detect_repo_state"` 或 `"path_lookup"`

**新增 Recommender**:`internal/env/recommend/recommend_git.go`(系统级 runner=`""`)

| 诊断 | 候选(按优先级) |
|---|---|
| `DiagGitNotInstalled` + apt 可用 | `!sudo apt install -y git` (StrategyGlobalInstall, NeedsSudo) |
| `DiagGitNotInstalled` + brew 可用 | `!brew install git` (StrategyGlobalInstall, no sudo) |
| `DiagGitNotInstalled` + Windows | DocsLink: <https://git-scm.com/download/win> |
| `DiagGitRepoNotInitialized` + 任何 OS | 多个候选:<br>1. `!git init && git add -A && git commit -m "initial"`(StrategyProjectInstall,NeedsNetwork=false,EstimatedSec=2)<br>2. **codrax 自动**(指向现有三层授权):"REPL `/approve` 触发时已自动询问;CLI 加 `--auto-init-repo`;yaml 持久化设 `write_auto_init_repo: true`" |
| `DiagGitRepoNoCommits` | `!git -C <repo> commit --allow-empty -m "initial"`(StrategyProjectInstall) |

### 19.3 触发时机扩展

**变更**:把 `worktree.DetectRepoState` 的检测**前移到 Run 入口**,让所有模式(读 / plan / apply / verify)都能尽早看到诊断:

- `Orchestrator.Run` 起手 → `EnvProbe` 跑(已经在做的) + `git_state.Diagnose` 跑(新增)
- 诊断结果挂到 `EnvFacts.GitRepoState`(新加字段)+ `BusContext.Mutable.EnvDiagnostics`
- 读模式不会因此 fail-loud,但会在**首次 LLM 调用前**给一条 INFO 日志:"target is not a git repo;some features (repomap incremental, file blame, /worktree, write mode) will be limited or unavailable"
- 写模式 plan / apply 触发时,如果 `GitRepoNotInitialized` 仍然为真且未授权 → 走现有"三层授权门",但**错误消息切到 Renderer 输出格式**(双语 + ! 前缀的命令 + 三层授权各自的清晰说明)

**新错误消息形态**(取代现有 hardcoded prose):

```
✗ apply 阶段:目标 /tmp/foo 不是 git 仓,无法创建 worktree。
诊断:git_not_initialized(`git rev-parse --is-inside-work-tree` 返回 false)

推荐(选一种):
  1) codrax 自动 scaffold(推荐,带授权确认):
     - 单次:启动时加 --auto-init-repo
     - 长期:codrax.yaml 设 `write_auto_init_repo: true`
     - REPL:下次 /approve 前会弹出 y/N 确认
  2) 手动 init(2 秒,无副作用):
     !git -C /tmp/foo init && git -C /tmp/foo add -A && git -C /tmp/foo commit -m "initial"

完成后再次 /approve 或 --mode=write --write-phase=apply。
```

### 19.4 复用 LLM 兜底吗?

**不需要**。git 状态诊断 + 推荐完全确定性:
- 检测 `.git/` 目录是 syscall
- `git init && git commit` 命令是固定模板
- OS 包管理器 install git 已经在 §8.2 范畴内

**例外**:如果用户的目录其实是 hg / fossil / pijul 等其它 VCS,LLM 兜底可以识别("这看起来是 mercurial 仓库,codrax 当前只支持 git;考虑 `hg-git` 桥或转换")—— 但这是非常长尾的 case,可以**第二阶段再加**。

### 19.5 跟现有授权机制的关系

**重要**:env_recommend 框架**不取代**现有的三层授权 + `/approve` 确认门。它只是**改进诊断 + 错误消息**:

| 层 | 现状 | 变更 |
|---|---|---|
| 诊断 | `DetectRepoState` 已有 | 抽到 env/diag,添加 git binary 检测 |
| 推荐文案 | hardcoded 一段 prose | 切到 Renderer 输出双语 + `!` 前缀命令 |
| 授权门(--auto-init-repo / yaml / REPL prompt) | 三层已有,不动 | 不动。env_recommend 的"自动 scaffold"候选指向这套现有机制 |
| 实际执行 `git init` | `worktree.EnsureInitialCommit` 已有 | 不动 |

**红线 R7 不破坏**:codrax 永远不**自动**执行 `git init`,只在用户授权后才动。env_recommend 只是把诊断 + 文案做得更智能。

### 19.6 工作量增量

| 模块 | LOC |
|---|---|
| Diagnoser `git_state.go` | ~80 |
| Recommender `recommend_git.go` | ~120 |
| EnvFacts.GitRepoState 字段 + Probe 接入 | ~40 |
| Run 入口 INFO 日志 | ~30 |
| 写模式错误消息切到 Renderer | ~50 |
| 测试(per-OS 包管理器 + 各 git state) | ~150 |
| **小计** | **~470 LOC** |

总计从 §14 的 ~4230 LOC → **~4700 LOC**。

### 19.7 落地节奏微调

§15 的 W4-W5 之间插入"git state 诊断 + Recommender",优先于 hvigor / cjpm 等较冷门 runner 后:

- W4.5:git state Diagnoser + Recommender + 写模式错误消息切换
- W5:其它后置(`/env explain` + 全文档刷新)

## 20. 总览(更新后)

包含 git 状态在内的诊断范围,本框架现在覆盖:

| 类别 | DiagKind |
|---|---|
| Runner 二进制缺失 | `runner_missing`(已有) |
| 项目依赖缺失 | `deps_missing`(新) |
| 编译/runtime toolchain 缺失 | `toolchain_missing`(新) |
| 系统库 / 头文件缺失 | `system_lib_missing`(新) |
| 项目元数据缺失 | `config_missing`(新) |
| **git 二进制缺失** | **`git_not_installed`**(新,§19) |
| **git 仓未初始化** | **`git_not_initialized`**(新,§19) |
| **git 仓无 commits** | **`git_no_commits`**(新,§19) |
| 长尾未识别 | `unknown` → LLM 兜底 |

**单一框架 + 单一缓存 + 单一渲染管线**覆盖整个"环境就绪度"问题域。客户两次诉求(智能推荐 + 裸仓引导)落到同一架构里,不会有第二种 hint 系统跟它平行。

## 21. 数据驱动的阶段微调

每阶段结束都可发布并收集真实使用数据。后续阶段的范围可以按数据动态调整。

### 21.1 关键观察指标(每阶段都收)

| 指标 | 数据来源 | 用途 |
|---|---|---|
| `/env` 命令调用频次 | REPL log | 验证用户对环境画像的兴趣 |
| Recommender 命中分布(per-runner / per-DiagKind) | env_recommend 日志 | 识别哪类场景是真高频 |
| 推荐被复制粘贴执行的转化率 | 后续 turn 是否有 `!cmd` 紧跟 | 验证推荐质量 |
| LLM 兜底触发频次 | env_recommend 日志(stage 4 启用后) | 如果 < 5% → 确定性表已经足够 |
| 缓存命中率(LLM 调用) | 缓存日志 | 验证缓存价值 |
| `git_not_initialized` 误报率 | 用户 /reject 频次 | 验证 git 自动诊断准确性 |

### 21.2 可能的范围调整

**如果数据显示**:

- LLM 兜底触发率 < 5% → 阶段 4 后**减少 LLM 配置面**(不再扩 prompt 翻译能力,只保留诊断)
- hvigor / cjpm runner 半年内零命中 → 阶段 6 **跳过这两个**(保持现行 hardcoded hint),减约 200 LOC
- 用户从不用 `/env explain` → 阶段 7 **缩到只发集成测试 + 文档**,减约 100 LOC
- Java / Swift 命中率不到 3% → 把它们后置到 v2.x 维护版,从主 v2 移出

按数据反馈,**v2 完整覆盖最低可压到 ~7500 LOC / 8 周**。

### 21.3 中断点契约

每阶段结束有三种处置:
1. **继续下一阶段**(默认):前一阶段绿、用户反馈无重大 blocker
2. **暂停 + 数据收集**:阶段 N 发布后跑 1-2 周,根据 §21.1 指标决定 N+1
3. **返工修阶段 N**:发现重大设计问题,先迭代 N 再进 N+1

阶段间不强制连续,客户随时叫停。

## 22. 复用清单 + 模式借鉴

明确列出可以"不重复造轮子"的现有代码,以及新代码可参考的成熟 pattern。

### 22.1 可直接抽 / 复用的代码

| 现有代码 | 实测 LOC | 处置 |
|---|---|---|
| `run_tests.go::detectRunnerMissing` | 54 | **抽到** `env/diag/runner_missing.go`(run_tests.go 改成 thin wrapper) |
| `run_tests.go::pythonInterpreter` | 60 | **抽到** `env/probe/probe_python.go`(run_tests.go 调 EnvFacts.Pythons[0]) |
| `run_tests.go::runnerInstallHint` | 69 | **重写**为 Recommender 表(yaml 关闭 LLM 时仍有保底) |
| `run_tests_runner_missing_test.go` | 379 | **平移**:大部分测试搬到 `env/diag/runner_missing_test.go`,run_tests 留 thin 集成测试 |
| `worktree.DetectRepoState` | 87 | **复用**:在 `env/diag/git_state.go` 包一层 |
| `worktree.EnsureInitialCommit` | 51 | **复用**:不动,推荐文案指向它 |
| `worktree` 三层 `--auto-init-repo` 授权门 | — | **完全不动**:env_recommend 只改进诊断 + 文案 |
| `linkProjectDeps`(`run_tests_link_deps.go`) | 128 | **可扩**:EnvFacts.LinkedDeps 字段后续接它 |
| `run_tests.go::runnerHasNoTestWork` / `hasPythonTestInfrastructure` / `runPyCompileFallback` | ~280 | **不动**:这些是 verify pre-flight,跟 env_recommend 正交 |

**总复用约 1100 LOC,等效节省约 1.5 周**。

### 22.2 借鉴的成熟 Pattern

| 现有 Pattern | 借鉴目标 | 借鉴点 |
|---|---|---|
| `memory.MEMORY.md.lock` flock + atomic rename | `internal/env/cache/disk_cache.go` | 跨平台文件锁 + corruption-safe 写入 |
| `cmd/root.go` chitchat_classifier resolver | `cmd/root.go` env_recommender resolver | provider 配置缺失时 fallback 到默认 cheap model |
| `tool.EmitChangePlan` 11-stage 验证 | `tool.EmitEnvRecommendation` schema 验证 | LLM 输出不合规即丢弃,不进主路径 |
| `chitchat.go::RespondWithMemory` 有界 ReAct loop | `env/recommend/llm_dispatch.go` 单次 LLM 调用 + 5s 超时 | LLM 路径绝不无限循环;失败 fail-safe 兜底 |
| `runtime.go` 6 keys 模式(`memory_*`) | `runtime.go` 6 个新 yaml keys(`env_recommend_*`) | yaml 知 旋钮 `*int` / `*bool` + `Default*` / `Resolve*` 三件套 |
| `internal/repl/messages.go` 双语 helpers | `internal/env/render.go` 双语渲染 | `isZh()` 路由,中英两套独立维护 |

### 22.3 不复用的部分(必须新写)

| 新代码 | 原因 |
|---|---|
| 22 个包管理器 PATH 探测 | 现有代码只做了 python/node 两个解释器探测,扩到 22 个包管理器是新工作 |
| 跨发行版包名表(LLM 翻译用) | 现有代码无类似数据 |
| disk_cache 的 schema_version 演化 | 现有 memory 缓存没有这个需求 |
| `/env explain` 的 LLM 解释生成 | 全新 UX |
| 12 个 Recommender 的策略表 | 每个 runner 的安装策略需要逐个整理(每个 ~80 LOC) |

## 23. v2 验收清单(发布前最后一道关)

阶段 7 结束、v2 正式发布前,以下都要绿:

- [ ] `go test ./...` 全 packages 绿,含 ~3000 LOC 新测试
- [ ] `go vet ./...` 无 warning
- [ ] `make` build 无 warning(本仓默认 CGO)
- [ ] 真实仓集成测试 3 个项目全过(Python pyproject + Node monorepo + Rust workspace)
- [ ] 关闭 `env_recommend_enabled: false` 后字节级回退现行 hint(守护测试锁住)
- [ ] 关闭 `env_recommend_llm_enabled: false` 后无任何 LLM 调用 trace 进 env_recommend 路径
- [ ] 离线网络环境(`env_probe_network: false`)整个链路工作正常
- [ ] 跨 OS:macOS / Linux / Windows 至少在 CI 各跑一次
- [ ] yaml 知 旋钮文档完整(`codrax.yaml.example` 注释 + `docs/user_guide.md` + `CLAUDE.md`)
- [ ] 设计文档 v3(实施后总结)归档到 `docs/design/env_recommend.md`,记录实际 LOC、踩坑、与 v2 估计的差距
- [ ] 旧 `runnerInstallHint` 函数标记 deprecated,默认走 Recommender 路径(yaml 关闭时仍可达)
