# HarmonyOS / ArkTS / 仓颉(Cangjie) 全面支持 —— 设计文档

**状态**：设计审查（rev3 已锁）。**未开始实现**。
**目标 PR 范围**：`PR-(-1) 调研` + `PR-0 ArkTS grammar` + `PR-0c 仓颉 grammar` + `PR-A log_triage 扩展` + `PR-B HiTrace 性能` + `PR-C runner+skill`，共 ~76 commits / 约 7-9 周。
**场景**：客户大量使用鸿蒙系统（HarmonyOS Stage Model + 仓颉 Native 模块），需要 codrax 在 ArkTS 与仓颉两种语言上提供与现有 Go/Java/TS 等价的 first-class 能力 —— repomap、问题分析、代码生成；并消化 hdc 抓回的 hilog 日志、HiTrace 性能数据。

---

## 0. 目录

- [1. 目标 / 非目标](#1-目标--非目标)
- [2. 现状审计](#2-现状审计)
- [3. 决策记录（5 项已锁）](#3-决策记录5-项已锁)
- [4. 架构总览](#4-架构总览)
- [5. PR-(-1) — 公开资料调研 + 设计落定](#5-pr-1--公开资料调研--设计落定)
- [6. PR-0 — tree-sitter-arkts 内置 + 4 层 fallback](#6-pr-0--tree-sitter-arkts-内置--4-层-fallback)
- [7. PR-0c — tree-sitter-cangjie 内置 + 3 层 fallback](#7-pr-0c--tree-sitter-cangjie-内置--3-层-fallback)
- [8. PR-A — log_triage 扩展（hilog + 双语言栈帧）](#8-pr-a--log_triage-扩展hilog--双语言栈帧)
- [9. PR-B — HiTrace + emit_perf_trace 性能分析](#9-pr-b--hitrace--emit_perf_trace-性能分析)
- [10. PR-C — hvigor / cjpm runner + 双语言 skill prompt](#10-pr-c--hvigor--cjpm-runner--双语言-skill-prompt)
- [11. 全栈 fallback 规范](#11-全栈-fallback-规范)
- [12. 红线（5 条）](#12-红线5-条)
- [13. 测试计划](#13-测试计划)
- [14. 开工顺序与里程碑](#14-开工顺序与里程碑)
- [15. 未决项 / 风险](#15-未决项--风险)

---

## 1. 目标 / 非目标

### 目标

1. **ArkTS 严格模式 first-class**：自维护 tree-sitter-arkts grammar；repomap 抽 `@Component struct` / `@Builder` / `@State` 等 21 装饰器与 ArkUI 链式调用；写模式通过既有 `git apply -` 路径直接可用，附 ArkTS 严格风格 skill prompt
2. **仓颉(Cangjie) 全面支持**：自维护 tree-sitter-cangjie grammar；repomap 抽 `func` / `class` / `struct` / `interface` / `extend` / `foreign func` / `enum`；包路径走 `package_clause` 而非路径推断
3. **hdc 日志消化**：log_triage 识别 hilog 行格式 + ArkTS V8 栈帧 + 仓颉 JVM-like 栈帧 + ArkTS↔Cangjie 互调链
4. **HiTrace 性能分析**：新工具 `emit_perf_trace` 抽 `PerfBundle{ Frames, JankSpans, ColdStart, MainThreadStalls }`；与 LogBundle 平行的独立通道
5. **hvigor / cjpm runner**：写模式 verifier 识别 ArkTS / 仓颉工程；标准输出走 JUnit XML / cjpm JSON，缺失时 exit-code 兜底（复用既有 CMake/Cargo 路径）
6. **全栈 fallback 退化**：每一层都有跨技术栈降级路径；降级非静默（WARN 日志 + tier 折扣 + 启动 banner）

### 非目标（本系列 PR 明确不做）

- 自维护 grammar 仓外发独立项目（grammar 内置 codrax 仓，决策 #2）
- ArkTS 1.0 / 1.1 lenient 兼容（仅严格模式，决策 #1）
- HarmonyOS Native (C/C++) 专项支持（既有 C/Cpp grammar 已覆盖；本系列不动）
- DevEco Studio 项目模板生成（codrax 不替代 IDE）
- HiTrace 二进制格式 native 解析（decode 走 perfetto 离线，codrax 不嚼二进制）
- ArkTS 跨平台输出（ArkUI-X 等）的特殊处理（grammar surface 一致即可）
- MCP / 外部 skill overlay（已在 `mcp_integration.md` 单独立项，与本系列并行不阻塞）

---

## 2. 现状审计

### 已有支撑面

| 组件 | 现状 | 鸿蒙缺口 |
|---|---|---|
| repomap tree-sitter | Go/Python/JS/TS/Java/Rust/C/C++ 8 语言 | `.ets` 未注册；ArkTS 装饰器未识别；仓颉零支持 |
| repomap resolver | Go/JS/Python/Rust/Java/C++ 解析包路径与 import | oh-package.json5 未支持；cjpm.toml 未支持 |
| log_triage `lang` 枚举 | go/java/python/rust/javascript/typescript/c/cpp/ruby/other | arkts/cangjie 未列；hilog 行格式未教过 LLM |
| log_triage 验证器 | `ResolveJavaFile` basename glob | ArkTS / 仓颉 basename glob 缺失 |
| verifier runner | 9 个（Go/Node/Python/Rust/Java/Ruby/CMake/Meson/Make）| hvigor / cjpm 缺失 |
| 写模式 coder | `git apply -` 与语言无关，透明工作 | skill prompt 无 ArkTS 严格 / 仓颉风格指引 |
| `log_attach_max_bytes` | 1 MB 默认，覆盖所有 attach 入口 | hdc 日志可能 GB 级，需评估默认是否抬高 |

### 现有依赖

- tree-sitter Go binding：`github.com/smacker/go-tree-sitter`（已在 `internal/tool/repomap/types/lang.go` 引入）
- TypeScript grammar：`github.com/smacker/go-tree-sitter/typescript/typescript`（PR-0 Tier2 fallback 复用，无需新引入）
- 唯一非 stdlib YAML 依赖：`gopkg.in/yaml.v3`

### 本系列新增依赖

- `github.com/titanous/json5`（解析 oh-package.json5；轻量 ~200 LOC）
- `github.com/pelletier/go-toml/v2`（解析 cjpm.toml；标准 TOML，活跃维护）

---

## 3. 决策记录（5 项已锁）

| # | 议题 | 选中 | 论据 |
|---|---|---|---|
| 1 | ArkTS 版本目标 | **仅严格模式（API 12+ Stable）**；禁 `any` / `as` / `index_signature` / `Function` 类型；grammar 层 reject 不放宽 | 严格模式是华为推力方向；lenient 兜底反而模糊降级触发点；遇到旧语法走 Tier2 显式降级而非 Tier1 内放过 |
| 2 | grammar 仓归属 | **内置 codrax 仓**：`internal/thirdparty/tree-sitter-arkts/` + `internal/thirdparty/tree-sitter-cangjie/` | 不外发独立仓简化分发；产物（parser.c/scanner.c）checkin 避免下游用户装 tree-sitter CLI；版本与 codrax 主体同步 |
| 3 | 仓颉版本目标 | **Cangjie 1.0.0 LTS (cjnative)** —— 已锁；cangjie-lang.cn 官方 LTS 版本，VS Code 插件 1.0.0；编译器后端基于 LLVM；Ubuntu/macOS x86_64+aarch64 支持 | 公开最新 LTS；用户只指 "公开最新"；锁版后写死避免 grammar 反复返工。后续若 1.1+ 发布且引入新 surface，单独评估升级 |
| 4 | 真实样本来源 | **公开资料**（developer.huawei.com / gitee openharmony 工程）；不要客户脱敏样本 | 客户回复 "根据网上公开资料查询"；公开样本足以驱动 corpus；后续若客户工程命中 fallback 高比例可二次校准 |
| 5 | Fallback 策略 | **全栈 4 层退化**：grammar Tier1→Tier2→Tier3→Tier4 / resolver 二级 / log_triage 三级 / HiTrace 三级 / runner exit-code 兜底 | 客户明确要求；鸿蒙生态演进快，grammar 不可能永远跟得上；降级是稳态运营前提 |

### 论据补充（避免下 session 忘了为什么）

- **为什么 ArkTS 不复用 TS grammar 走 fallback 而要自维护**：TS grammar 的 `class_declaration` 与 ArkTS `struct` 是不同节点类型，ArkUI 链式调用 + trailing block（`Column() { ... }.width('100%')`）TS grammar 语法上不接受 —— 不自维护就抽不到 build() / @State / 装饰器实参。Tier2 用 TS grammar 是降级（拿不到 ArkUI 特性），不是平替。
- **为什么仓颉无 Tier2 grammar**：仓颉 surface 与 Rust/Swift/Java 都不同（`func` / `package_clause` 首行 / `extend` / `match` 模式 / trait-like `interface`）；任何既有 grammar 强行套用都会大量 parse error，不如直接走 regex Tier2。
- **为什么 grammar 产物 checkin 而非 go generate**：tree-sitter CLI 是 Rust 二进制 + npm 依赖，强制下游用户装太重；checkin parser.c 后用户只需 `go build` 一行；开发者改 grammar 时手动 `go generate ./internal/thirdparty/...`。

---

## 4. 架构总览

### 4.1 数据流（鸿蒙工程读模式）

```
用户 hdc 抓日志 ─┐
                 ├─→ codrax --log <file> --request "..."
ArkTS+Cangjie 工程 ─┘
                              ↓
                    [log_triage 阶段]
                    ├─ Tier1: emit_log_triage (ArkTS V8 栈 / 仓颉 JVM-like 栈)
                    ├─ Tier2: log-segmentation-skill 切片再 triage
                    └─ Tier3: regex 抽 (\S+\.(ets|cj)):(\d+) 兜底
                              ↓
                    [analyze 阶段] —— 用 LogBundle.ResolvedFiles 种 EvidencePlan.RequiredFiles
                              ↓
                    [explore 阶段] —— repomap.ParseWithFallback
                                        ├─ ArkTS: arkts(strict) → typescript → regex → path-only
                                        └─ Cangjie: cangjie → regex → path-only
                              ↓
                    [extract 阶段] —— 含 ArkTS 装饰器 / 仓颉 extend 关系
                              ↓
                    [finalize 阶段]
```

### 4.2 数据流（鸿蒙工程写模式 plan→apply→verify）

```
analyze (classifier) ─→ runPlanPhase (planner agent + ArkTS/仓颉 skill prompt)
                          ↓
                       runApplyPhase (worktree + git apply -)
                          ↓
                       runVerifyPhase
                       ├─ detectLanguage:
                       │    oh-package.json5 / build-profile.json5 / hvigorfile.ts → arkts
                       │    cjpm.toml                                              → cangjie
                       │    既有 9 detector                                          → 既有路径
                       ├─ runner:
                       │    arkts: hvigorw test → JUnit XML 解析 (复用 Java JUnit 路径)
                       │    cangjie: cjpm test --json → 解析；缺失走 cargo 文本路径
                       └─ exit-code 兜底（runner 全失败时）
```

### 4.3 性能分析数据流（HiTrace）

```
hdc shell hitrace ─→ trace 文件
                       ↓
                    codrax --htrace <file>
                       ↓
                    [log_triage 阶段] — 无关；HiTrace 走独立通道
                       ↓
                    [perf_triage 阶段（新增）]
                    ├─ Tier1: emit_perf_trace → PerfBundle
                    ├─ Tier2: 退到 log_triage 走文本路径（损失结构）
                    └─ Tier3: 二进制无法 utf-8 解码 → 提示用户 perfetto 离线
                       ↓
                    Mutable.PerfTrace() 通道
                       ↓
                    [analyze] 性能 hypothesis 模板（jank/cold-start/main-thread-stall）
                       ↓
                    [explore]→[extract]→[finalize]
```

---

## 5. PR-(-1) — 公开资料调研 + 设计落定

**估时**：2-3 天 / 1 commit。
**依赖**：无。

### 5.1 调研产出清单

| 文件 | 内容 | 来源 |
|---|---|---|
| `docs/design/harmonyos_arkts_cangjie_support.md` | 本文档（合并设计） | 本次 session 写就 |
| `internal/thirdparty/tree-sitter-arkts/corpus/*.txt` | 30 例严格模式样本 | gitee openharmony / developer.huawei.com codelabs |
| `internal/thirdparty/tree-sitter-cangjie/corpus/*.txt` | 40 例样本 | cangjie-lang.cn / gitee cangjie SIG |
| `eval/cases/harmony/hilog_arkts_panic.case` | ArkTS panic 端到端 | OpenHarmony forum 真实崩溃 |
| `eval/cases/harmony/hilog_cangjie_panic.case` | 仓颉 panic 端到端 | 同上 |
| `eval/cases/harmony/hitrace_jank.case` | jank trace | 调研期手抓 |
| `eval/cases/harmony/arkts_repomap.case` | ArkTS repomap | 拉公开 ArkTS 工程 |
| `eval/cases/harmony/cangjie_repomap.case` | 仓颉 repomap | 拉公开仓颉工程 |

### 5.2 调研已锁事实（rev3 完成）

- [x] **仓颉公开最新版本**：Cangjie 1.0.0 LTS (cjnative)；编译器后端 LLVM；Ubuntu/macOS x86_64+aarch64；VS Code 插件 1.0.0 → §3 决策 #3 已写入
- [x] **仓颉栈帧格式**（公开样本归纳）：`at <package>.<func>(<file.cj>:<line>)` JVM-like → §8.2 已写入
- [x] **hilog 行格式**（来自 openharmony/hiviewdfx_hilog README.md）：`MM-DD HH:MM:SS.mmm PID TID LEVEL DOMAIN/TAG: MESSAGE` → §8.1 已写入；literal 例 `01-26 11:01:06.870 1051 1051 W 00201/test: Failed to visit ...`
- [x] **HiTrace 文本格式**：底层 ftrace + bytrace/hiTraceMeter CLI；输出格式与 Linux ftrace 兼容（含 `tracing_mark_write: B|<pid>|<tag>` / `E|<pid>` 事件）→ §9.2 已写入
- [x] **hvigor 测试输出**：DevEco Testing 基于 JUnit5；输出含 `junit.xml` + `report.html` → §10.1 已写入；JUnit XML 复用既有 Java 路径
- [x] **cjpm `--json`**：官方文档未列出 `--json` flag；主路径走文本解析（cargo 风格）；future-proof 在 PR-C 启动时 `cjpm test --help` 探测 → §10.2 已写入
- [x] **`oh-package.json5`**：实际工程多含 json5 注释 + 单引号 + 尾逗号 → 必须引 json5 库；strict-JSON 兜底走 Tier2 fallback；§6.5 已写入

### 5.3 公开资料证据链（来源 URL）

ArkTS：
- https://github.com/HarmonyOS-Next/awesome-harmonyos/blob/main/Adaptation_rules_from_TypeScript_to_ArkTS.md（严格模式禁 any/as 规则）
- https://developer.huawei.com/consumer/en/doc/harmonyos-guides/arkts-overview（Application Framework）
- https://docs.oniroproject.org/application-development/basic-concepts/introduction-to-arkts/（基础语法）

Cangjie：
- https://cangjie-lang.cn/en（官方）
- https://docs.cangjie-lang.cn/cjnative/user_manual/（语言文档）
- https://github.com/Cangjie-Pub/cangjie_compiler（编译器源码）
- https://docs.cangjie-lang.cn/cjnative/user_manual/source_zh_cn/Compile-And-Build/cjpm_usage_OHOS.html（cjpm 用法）

hilog / HiTrace：
- https://github.com/openharmony/hiviewdfx_hilog/blob/master/README.md（hilog 行格式）
- https://github.com/openharmony/hiviewdfx_hitrace（HiTrace 实现）

hvigor / DevEco Testing：
- https://dev.to/moyantianwang/deveco-testing-official-automation-testing-tool-for-harmonyos-applications-5g95（JUnit XML + report.html 输出）

### 5.3 调研期间 grammar.js 不动手

PR-(-1) 不写 grammar 代码；只产出调研结果与 corpus。grammar.js 写代码是 PR-0 / PR-0c 的事，**前提是 corpus 充分**。

---

## 6. PR-0 — tree-sitter-arkts 内置 + 4 层 fallback

**估时**：~14 commits / 2.5 周。
**依赖**：PR-(-1) v0.1 corpus。

### 6.1 grammar 仓内布局

```
internal/thirdparty/tree-sitter-arkts/
├── grammar.js             # tree-sitter grammar DSL
├── src/
│   ├── parser.c           # tree-sitter generate 产物（checkin）
│   ├── scanner.c          # 自定义 scanner（处理 ArkUI trailing block）
│   ├── tree_sitter/       # 头文件
│   └── grammar.json       # generate 产物
├── corpus/
│   └── *.txt              # 测试用例（来自 PR-(-1)）
├── package.json           # tree-sitter 元信息（仅给 generate 用）
├── README.md              # 何时升级 grammar / 如何 generate
└── go_binding.go          # cgo wrapper，暴露 GetLanguage()
```

`go generate` 注释写在 `internal/thirdparty/tree-sitter-arkts/generate.go`（仅文档作用）：
```go
//go:build generate
//go:generate tree-sitter generate
package arkts
```

### 6.2 grammar.js 严格模式 surface

**核心节点**：

```js
struct_declaration:           // 替代 class
  // @Component / @Entry / @Preview / @Observed 必含至少一个
  // body 必含 build() 函数（@Component 强制）
  decorator+ 'struct' identifier (extends_clause)? class_body

build_method:                 // ArkUI 渲染入口
  'build' '(' ')' '{' ui_call+ '}'

ui_call:                      // ArkUI 链式 + trailing block
  call_expression ('.' identifier '(' argument_list ')')* trailing_block?
trailing_block:
  '{' (ui_call | if_else | foreach | lazy_foreach)* '}'

decorator_with_arg:           // 21 装饰器白名单
  '@' decorator_name ('(' decorator_arg ')')?
decorator_name:
  // ArkUI 装饰器
  'Component' | 'Entry' | 'Preview' | 'CustomDialog' | 'Observed' |
  'Reusable' | 'Builder' | 'BuilderParam' | 'Styles' | 'Extend' |
  // 状态管理
  'State' | 'Prop' | 'Link' | 'Provide' | 'Consume' | 'Watch' |
  'ObjectLink' | 'StorageLink' | 'StorageProp' |
  'LocalStorageLink' | 'LocalStorageProp'
  // 非白名单 → parse error

styled_block:                 // @Styles 内属性方法链
  '@Styles' 'function' identifier '(' ')' '{' style_attr_call+ '}'

// 严格门：以下规则不存在 / 显式 reject
//   any_type, as_expression, index_signature, function_type
```

**严格门实现**：在 grammar 中不为 `any` 关键字定义 token；`as` 表达式不进 `expression` choice；遇到立即 ERROR 节点。

### 6.3 corpus 用例分布（30 例）

- 10 例 @Component struct + build()
- 5 例 @Builder / @BuilderParam
- 5 例 @State / @Prop / @Link / @Provide / @Consume 状态管理
- 3 例 @Styles / @Extend
- 3 例 LazyForEach / ForEach 嵌套 UI
- 2 例 Stage Model 入口（EntryAbility）
- 2 例 import / export（含 oh-package 路径）

### 6.4 Go binding（`internal/thirdparty/tree-sitter-arkts/binding.go`）

```go
package arkts

// #cgo CFLAGS: -std=c11 -fPIC
// #include "src/parser.c"
// #include "src/scanner.c"
// extern TSLanguage *tree_sitter_arkts(void);
import "C"
import (
    "unsafe"
    sitter "github.com/smacker/go-tree-sitter"
)

func GetLanguage() *sitter.Language {
    ptr := unsafe.Pointer(C.tree_sitter_arkts())
    return sitter.NewLanguage(ptr)
}
```

### 6.5 codrax 接入

**`internal/tool/repomap/types/lang.go`**：
```go
const LangArkTS = "arkts"

// extToLang[".ets"] = LangArkTS
// extToLang[".ts"]  在 ArkTS 工程探测到时由 build.go 动态归类
```

**`internal/tool/repomap/index/parser.go`** switch 加：
```go
case types.LangArkTS:
    return arkts.GetLanguage(), extractArkTS
```

**`internal/tool/repomap/index/extract_arkts.go`**（新）抽：
- `struct_declaration` → `Symbol{ Kind: "component", Name: ... }`
- `build_method` → `Symbol{ Kind: "ui-entry", Name: "build", Parent: <component> }`
- `decorator_with_arg` → `Symbol.Decorators []string`
- `@State` / `@Prop` 等字段 → `Symbol{ Kind: "state-field" / "prop-field" }` 独立 SymbolKind
- import → `types.Import{ Path: <oh-package 解析后> }`

**`internal/tool/repomap/index/resolver_arkts.go`**（新）：
- 找仓库根 `oh-package.json5` / 子模块 `oh-package.json5`
- 解析依赖：`@ohos.*` / `@kit.*` / `@hms.*` 进 builtin 黑洞跳过
- `@bundle:<bundle>` 走 bundle map
- `../` 相对路径走 fs（与 JS resolver 复用 `pickFsCandidate` 风格）
- json5 解析失败 → 退到 strict-JSON 二次尝试 → 再失败 → basename glob 兜底

**`internal/tool/repomap/index/build.go`** 扫描扩展加 `.ets`；探测到 `oh-package.json5` 时 `.ts` 文件归 `LangArkTS`（红线 L-ArkTS-2）。

**`internal/tool/repomap/retrieve/rank.go`**：
- 入口名加权：`Index.ets` / `EntryAbility.ts` / `entry/src/main/ets/` 路径前缀
- 装饰器加权：含 `@Entry` 文件 +0.15

### 6.6 4 层 fallback chain（`internal/tool/repomap/index/parse_fallback.go` 新文件）

```go
type ParseAttempt struct {
    Lang     string
    Tier     int    // 1..4
    Symbols  []types.Symbol
    Imports  []types.Import
    Pkg      string
    Reason   string // 降级原因首行（不超过 200 字符）
}

func ParseWithFallback(path string, content []byte) ParseAttempt {
    // 仅 .ets/.ts (in ArkTS 工程) 走此 chain；其它语言保持原 parser.go 路径

    // Tier 1: tree-sitter-arkts 严格
    if attempt, ok := tryParseTier1ArkTS(path, content); ok {
        return attempt
    }

    // Tier 2: tree-sitter-typescript lenient
    logger.Warnf("[repomap] %s arkts→typescript (tier 1→2): %s", path, lastErr.Reason)
    if attempt, ok := tryParseTier2TS(path, content); ok {
        attempt.Tier = 2
        attempt.Reason = lastErr.Reason
        return attempt
    }

    // Tier 3: regex 抽取器
    logger.Warnf("[repomap] %s arkts→regex (tier 2→3): %s", path, lastErr.Reason)
    if attempt, ok := tryParseTier3Regex(path, content); ok {
        attempt.Tier = 3
        return attempt
    }

    // Tier 4: 仅文件路径
    logger.Warnf("[repomap] %s arkts→path-only (tier 3→4): %s", path, lastErr.Reason)
    return ParseAttempt{Lang: "arkts", Tier: 4, Reason: lastErr.Reason}
}
```

**Tier 3 regex 抽取**（覆盖率目标 70%+ 关键 symbol）：
- `@(\w+)\s*\([^)]*\)\s*struct\s+(\w+)` → component
- `@State\s+(\w+):` / `@Prop\s+(\w+):` / `@Link\s+(\w+):` → state/prop/link
- `function\s+build\s*\(\s*\)` → ui-entry
- `import\s+\{([^}]+)\}\s+from\s+['"]([^'"]+)['"]` → imports
- `@Builder\s+function?\s*(\w+)` → builder

**`FileInfo.ParseTier int`**（新字段）：rank.go 折扣
```go
const (
    parseTier1Weight = 1.0
    parseTier2Weight = 0.85
    parseTier3Weight = 0.6
    parseTier4Weight = 0.3
)
```

**仓库级阈值**：`build.go` 扫描完后统计 ArkTS 文件 Tier2 占比 > 0.4 → 启动 banner 一次：
```
[repomap] WARN: 47% of .ets files fell back to TypeScript grammar — consider tree-sitter-arkts grammar update
```

### 6.7 PR-0 commits 拆分（14 个）

1. `feat(repomap): add LangArkTS constant + .ets ext registration`
2. `feat(thirdparty): vendor tree-sitter-arkts grammar.js skeleton`
3. `feat(thirdparty): tree-sitter-arkts decorator + struct rules`
4. `feat(thirdparty): tree-sitter-arkts build() + ui_call_chain`
5. `feat(thirdparty): tree-sitter-arkts strict gates (any/as/index_signature)`
6. `feat(thirdparty): tree-sitter-arkts checkin parser.c + scanner.c + Go binding`
7. `feat(repomap): extract_arkts.go — struct/build/decorators/state-fields`
8. `feat(repomap): resolver_arkts.go — oh-package.json5 + @ohos black-hole`
9. `feat(repomap): build.go scan .ets + dynamic .ts→ArkTS in ArkTS projects`
10. `feat(repomap): rank.go — ArkTS entry + decorator weighting`
11. `feat(repomap): parse_fallback.go — Tier 1→2→3→4 chain + ParseTier field`
12. `feat(repomap): rank discount by ParseTier + repo-level banner`
13. `test(repomap): TestParseFallback_Tier1_Strict / Tier2_TSGrammar / Tier3_Regex / Tier4_PathOnly`
14. `docs: update CLAUDE.md + architecture.md with ArkTS support`

---

## 7. PR-0c — tree-sitter-cangjie 内置 + 3 层 fallback

**估时**：~16 commits / 3 周。
**依赖**：PR-(-1) v0.1 corpus；与 PR-0 并行。

### 7.1 grammar.js 关键节点（surface）

```js
package_clause:               // 首行 package xxx.yyy
  'package' qualified_identifier

import_declaration:
  'import' qualified_identifier ('as' identifier)?

function_declaration:
  modifier* 'func' identifier generic_params? '(' parameter_list ')' return_type? where_clause? block?

class_declaration:
  modifier* 'class' identifier generic_params? superclass_clause? interface_list? where_clause? class_body
struct_declaration:
  modifier* 'struct' identifier generic_params? interface_list? struct_body
interface_declaration:
  modifier* 'interface' identifier generic_params? superinterface_list? where_clause? interface_body
enum_declaration:
  modifier* 'enum' identifier generic_params? where_clause? enum_body

extend_declaration:           // 关键 —— 扩展类型方法
  'extend' type_expression interface_list? where_clause? extend_body

modifier:
  'public' | 'private' | 'protected' | 'internal' |
  'open' | 'static' | 'operator' | 'sealed' | 'abstract' |
  'foreign' | 'override' | 'redef' | 'mut' | 'const' | 'unsafe'

decorator:
  '@' identifier ('(' argument_list ')')?
  // 白名单：@When / @CallingThread / @CJOH / 用户自定义不限制

pattern_match:                // match 表达式
  'match' '(' expression ')' '{' match_arm+ '}'

operator_overload:
  modifier* 'operator' 'func' operator_token '(' parameter_list ')' return_type? block

foreign_block:
  'foreign' 'func' identifier '(' parameter_list ')' return_type?
```

### 7.2 corpus 用例分布（40 例）

- 8 例 func / class 基础
- 5 例 struct / interface / enum
- 5 例 extend 关键字（含 extend Trait for Type）
- 4 例 generic + where_clause
- 4 例 pattern_match
- 4 例 operator overload
- 3 例 foreign func（含 @CJOH 与 ArkTS 互调）
- 3 例 modifier 组合（open / sealed / abstract 等）
- 2 例 package + import
- 2 例 lambda + closure

### 7.3 Go binding

同 PR-0 §6.4 风格，路径 `internal/thirdparty/tree-sitter-cangjie/binding.go`。

### 7.4 codrax 接入

**`types/lang.go`**：
```go
const LangCangjie = "cangjie"
extToLang[".cj"] = LangCangjie
// .cjo 不进 extToLang —— 编译产物，scanner 显式 deny-list (L-Cangjie-1)
```

**`extract_cangjie.go`**：
- `package_clause` → `FileInfo.Package`（**红线 L-Cangjie-2**：必须从此读，禁止路径推断）
- `function_declaration` / `class_declaration` / `struct_declaration` / `interface_declaration` / `enum_declaration` → 各对应 `Symbol.Kind`
- `extend_declaration` → 特殊 SymbolKind=`extend`，附 `ExtendsType` 边到被扩展类型
- `operator_overload` → `Symbol.Kind="operator"`
- `foreign_block` → `Symbol.Kind="foreign-func"`
- `modifier` 含 `public` / `open` / `protected` → `Exported=true`

**`resolver_cangjie.go`**：
- 找根 `cjpm.toml` 或子包 `cjpm.toml`
- 解析 `[dependencies]` → 包名 → 路径
- `import std.*` / `import core.*` 进 builtin 黑洞
- toml 解析失败 → line-by-line `[dependencies]` 段抓取 → 再失败 → basename glob

**`build.go`**：扫描扩展加 `.cj`；scanner 黑名单 `*.cjo` / `target/` / `.cangjie-cache/`。
**`rank.go`**：`main.cj` / `src/main/cangjie/` 路径加权。
**`IsExported` 加 LangCangjie 分支**：modifier 含 `public` / `open` → 否则 false（与 Java 不同 —— 仓颉默认包内可见）。

### 7.5 3 层 fallback chain

```go
// internal/tool/repomap/index/parse_fallback_cangjie.go
func parseCangjieWithFallback(path string, content []byte) ParseAttempt {
    if attempt, ok := tryParseTier1Cangjie(path, content); ok {
        return attempt
    }
    logger.Warnf("[repomap] %s cangjie→regex (tier 1→2): %s", path, lastErr.Reason)
    if attempt, ok := tryParseTier2RegexCangjie(path, content); ok {
        attempt.Tier = 2
        return attempt
    }
    logger.Warnf("[repomap] %s cangjie→path-only (tier 2→3): %s", path, lastErr.Reason)
    return ParseAttempt{Lang: "cangjie", Tier: 3, Reason: lastErr.Reason}
}
```

**Tier 2 regex**：
- `^package\s+([\w.]+)` → package
- `^\s*(public|open|protected|internal)?\s*func\s+(\w+)` → function（modifier 抓 export 状态）
- `^\s*(public|open|protected|internal)?\s*(class|struct|interface|enum)\s+(\w+)` → 类型
- `^\s*extend\s+(\w+)` → extend
- `^\s*foreign\s+func\s+(\w+)` → foreign-func
- `^\s*import\s+([\w.]+)` → import

**仓库级阈值**：Tier2 占比 > 0.5 → 启动 banner（仓颉无 Tier2 grammar，阈值更宽容）。

### 7.6 PR-0c commits 拆分（16 个）

1. `feat(repomap): add LangCangjie constant + .cj ext + .cjo deny-list`
2. `feat(thirdparty): vendor tree-sitter-cangjie grammar.js skeleton`
3. `feat(thirdparty): cangjie package_clause + import + modifier`
4. `feat(thirdparty): cangjie function/class/struct/interface/enum`
5. `feat(thirdparty): cangjie extend_declaration + operator_overload`
6. `feat(thirdparty): cangjie generic + where_clause`
7. `feat(thirdparty): cangjie pattern_match + foreign_block`
8. `feat(thirdparty): cangjie checkin parser.c + Go binding`
9. `feat(repomap): extract_cangjie.go — package/func/types/extend/foreign`
10. `feat(repomap): IsExported cangjie branch (public/open modifier)`
11. `feat(repomap): resolver_cangjie.go — cjpm.toml + std/core black-hole`
12. `feat(repomap): build.go scan .cj + .cjo/target/.cangjie-cache deny`
13. `feat(repomap): rank.go cangjie entry weighting`
14. `feat(repomap): parse_fallback_cangjie.go — Tier 1→2→3 chain`
15. `test(repomap): TestParseFallback_Cangjie_Tier1/Tier2/Tier3 + IsExported cangjie`
16. `docs: update CLAUDE.md with Cangjie support + L-Cangjie-1/2 red lines`

---

## 8. PR-A — log_triage 扩展（hilog + 双语言栈帧）

**估时**：~14 commits。
**依赖**：PR-0 + PR-0c（需要 ArkTS / 仓颉 grammar 已上线，validator 才能 ResolveFile 找到真文件）。

### 8.1 hilog 行格式（已锁，来自 openharmony/hiviewdfx_hilog README.md）

```
MM-DD HH:MM:SS.mmm PID TID LEVEL DOMAIN/TAG: MESSAGE
```

literal 例：
```
01-26 11:01:06.870 1051 1051 W 00201/test: Failed to visit <private>, reason:503.
```

字段：
- `MM-DD HH:MM:SS.mmm` —— 时间戳（月-日 时:分:秒.毫秒）
- `PID` / `TID` —— 进程 / 线程 ID
- `LEVEL` —— 单字符 D/I/W/E/F（Debug/Info/Warn/Error/Fatal）
- `DOMAIN/TAG` —— 4 位 hex 域 + 应用自定 tag
- `MESSAGE` —— 应用日志消息；release 模式中 `%{public}` 标记的字段保留，未标记字段被替换为 `<private>`

`emit_log_triage` prompt 加教 LLM：
- 识别 hilog 行格式，把 tag + level 写入 `LogBundle.Meta.Tags[]` / 严重度推 `signals`
- ArkTS 应用日志通常 tag 形如 `OHRunner` / `ace_napi` / 应用包名

### 8.2 ArkTS 栈帧（V8 格式，复用既有 JS 路径）

```
ErrorMessage: ...
  at Foo.bar (entry/src/main/ets/pages/Index.ets:42:10)
  at FooComponent.build (entry/src/main/ets/pages/Index.ets:30:5)
  at __ELEMENT_invoke (...)
```

LLM 直接抽 `(file:line:col)`；validator 走 `ResolveArkTSFile`：
```go
// internal/analysis/logtriage/resolve_arkts.go
func ResolveArkTSFile(repoRoot, raw string) (string, bool) {
    // 1. 严格匹配 entry/src/main/ets/<...>.ets
    // 2. basename glob 在 entry/src/main/ets/ 下
    // 3. basename glob 在 commons/*/src/main/ets/ 下
    // 4. fallback: 任意位置 <basename>
}
```

### 8.3 仓颉栈帧（已锁，公开样本归纳）

```
panic: <msg>
  at <package>.<func>(<file.cj>:<line>)
  at <package>.<func>(<file.cj>:<line>)
  at <native>
```

literal 例（同 `eval/cases/harmony/hilog_cangjie_panic.case`）：
```
04-15 14:35:42.108 6128 6128 F A0c0d/CjApp: panic: index out of bounds: index=5, size=3
04-15 14:35:42.108 6128 6128 F A0c0d/CjApp:     at demo.cart.Cart.itemAt(src/cart/Cart.cj:78)
04-15 14:35:42.108 6128 6128 F A0c0d/CjApp:     at demo.cart.Cart.checkout(src/cart/Cart.cj:42)
```

特征：JVM-like 栈帧（`<package>.<func>` 点分），与 Java 栈帧近似但路径含 `.cj` 扩展名。`ResolveCangjieFile` 抓取 `(\S+\.cj):(\d+)` 即可。

`ResolveCangjieFile`：
```go
// internal/analysis/logtriage/resolve_cangjie.go
func ResolveCangjieFile(repoRoot, raw string) (string, bool) {
    // 1. basename glob 在 src/ 下
    // 2. basename glob 在 cangjie/ 下
    // 3. 任意位置 <basename>
}
```

### 8.4 lang 枚举扩展（`internal/analysis/logtriage/types.go`）

```go
const (
    LangArkTS    = "arkts"
    LangCangjie  = "cangjie"
    // ... 既有 9 个
)
```

`ValidateBundle` switch 加：
```go
case LangArkTS:
    if path, ok := ResolveArkTSFile(repoRoot, f.File); ok { ... }
case LangCangjie:
    if path, ok := ResolveCangjieFile(repoRoot, f.File); ok { ... }
```

### 8.5 Tier 3 regex 兜底（仓颉栈帧 LLM 失败时）

`emit_log_triage` 失败 + 重试用尽 → 系统旁路 regex `(\S+\.cj):(\d+)` 抓所有 cj:line 对，写入 `LogBundle.Errors[].Frames[]`，`Coverage` 标 0.3，IntentHint 留空。

### 8.6 PR-A commits 拆分（14 个）

1. `feat(logtriage): lang enum +arkts +cangjie`
2. `feat(logtriage): resolve_arkts.go — entry/src/main/ets glob`
3. `feat(logtriage): resolve_cangjie.go — src/cangjie glob`
4. `feat(logtriage): ValidateBundle switch arkts/cangjie`
5. `feat(logtriage): hilog formatter prompt update`
6. `feat(logtriage): ArkTS V8 stack frame prompt examples`
7. `feat(logtriage): cangjie stack frame prompt examples`
8. `feat(logtriage): regex Tier3 fallback for cangjie frames`
9. `test(logtriage): TestResolveArkTSFile + TestResolveCangjieFile`
10. `test(logtriage): TestValidateBundle_HilogArkTS`
11. `test(logtriage): TestValidateBundle_HilogCangjie`
12. `test(logtriage): TestRegexFallback_CangjieFrames`
13. `feat(eval): cases/harmony/hilog_arkts_panic.case + hilog_cangjie_panic.case`
14. `docs: update CLAUDE.md log_triage section with hilog + arkts/cangjie`

---

## 9. PR-B — HiTrace + emit_perf_trace 性能分析

**估时**：~10 commits。
**依赖**：PR-A。

### 9.1 PerfBundle 数据结构

```go
// internal/types/perf_bundle.go
type PerfBundle struct {
    Meta       PerfMeta       `json:"meta"`
    Frames     []FrameInfo    `json:"frames"`         // 渲染帧
    JankSpans  []JankSpan     `json:"jank_spans"`     // 卡顿区段
    ColdStart  *ColdStartInfo `json:"cold_start,omitempty"`
    MainStalls []StallSpan    `json:"main_stalls"`    // 主线程 block
    Residue    []string       `json:"residue"`        // 未结构化的原始片段
    Coverage   float64        `json:"coverage"`
}

type PerfMeta struct {
    Source   string  `json:"source"`    // hitrace / perfetto / systrace
    Duration float64 `json:"duration"`  // ms
    AppPID   int     `json:"app_pid,omitempty"`
}

type FrameInfo struct {
    Timestamp float64 `json:"timestamp"`
    Duration  float64 `json:"duration_ms"`
    Janky     bool    `json:"janky"`
    Phase     string  `json:"phase"` // measure/layout/draw/composite
}

type JankSpan struct {
    StartMs    float64  `json:"start_ms"`
    DurationMs float64  `json:"duration_ms"`
    Cause      string   `json:"cause"`
    Frames     []string `json:"frames"` // 触发帧（call site）
}

type ColdStartInfo struct {
    AppLaunchMs   float64 `json:"app_launch_ms"`
    AbilityInitMs float64 `json:"ability_init_ms"`
    FirstDrawMs   float64 `json:"first_draw_ms"`
}

type StallSpan struct {
    StartMs    float64 `json:"start_ms"`
    DurationMs float64 `json:"duration_ms"`
    Reason     string  `json:"reason"` // io/lock/sync-call/heavy-compute
    Symbol     string  `json:"symbol,omitempty"`
}
```

### 9.2 HiTrace 文本格式（已锁，来自 openharmony/hiviewdfx_hitrace + ftrace 兼容子集）

`hdc shell hitrace -t <duration>` 输出 ftrace 兼容文本，关键事件：
```
   <task>-<pid> [<cpu>] ...1 <ts>: tracing_mark_write: B|<pid>|<tag>   # begin span
   <task>-<pid> [<cpu>] ...1 <ts>: tracing_mark_write: E|<pid>          # end span
   <task>-<pid> [<cpu>] ...1 <ts>: tracing_mark_write: C|<pid>|<name>|<value>  # counter
   <task>-<pid> [<cpu>] ...1 <ts>: tracing_mark_write: S|<pid>|<tag>|<cookie>  # async start
```

literal 例（同 `eval/cases/harmony/hitrace_jank.case`）：
```
   com.example-5821 [001] ...1 12345.678901: tracing_mark_write: B|5821|H:RenderService:DoFrame
   com.example-5821 [001] ...1 12345.679512: tracing_mark_write: B|5821|H:Layout:measure
   com.example-5821 [001] ...1 12345.681203: tracing_mark_write: B|5821|H:DataLoader:fetchSync
   com.example-5821 [001] ...1 12345.764812: tracing_mark_write: E|5821
```

`emit_perf_trace` LLM prompt 教模型：
- 配对 `B|...|<tag>` 与 `E|...`，提取 span 名 + 持续时间
- `H:RenderService:DoFrame` 为帧边界；超过 16.67ms 的帧标 Janky=true
- 同步嵌套 span 在主线程上 = MainStalls

### 9.3 emit_perf_trace 工具

新增 `internal/tool/emit_perf_trace.go`：与 `emit_log_triage` 同形态，schema 暴露给 `perf_triager` agent。

新 agent `internal/agent/perf_triager.go`：复用 `BaseAgent` ReAct loop；LLM 读 trace 文本，emit PerfBundle。

### 9.4 attach 入口

CLI：`--htrace <file|->` / `--htrace-text <inline>`，与 `--log` 同形态。
REPL：`/htrace <path>` / `/htrace clear` / `/htrace show`。
共享 `log_attach_max_bytes` cap（不重复定义）。

### 9.5 Mutable.PerfTrace() 通道

新增 setter / getter 与 `LogTriage()` 平行：
```go
func (m *MutableState) SetPerfTrace(b *PerfBundle)
func (m *MutableState) PerfTrace() *PerfBundle
```

analyzer 读 PerfTrace 与 LogTriage 同等：注入 RequiredFiles + Entities + IntentHint=`IntentPerformance`（新 IntentKind）。

### 9.6 性能 hypothesis 模板

`internal/analysis/compiler` 新 scenario template `ScenarioPerformance`：
- 默认 hypotheses：`H_main_thread_block` / `H_layout_thrash` / `H_cold_start_io` / `H_render_overdraw`
- 默认 RequiredEvidence：`StallSpan.Symbol` 反查 repomap

### 9.7 3 层 fallback

- Tier 1：`emit_perf_trace` 成功
- Tier 2：失败/超时 → 把 trace 当文本走 `log_triage`（loss 性能结构）
- Tier 3：trace utf-8 解码失败 → 用户提示 `use perfetto offline; codrax does not parse binary trace`，pipeline 跳过

### 9.8 PR-B commits 拆分（10 个）

1. `feat(types): PerfBundle + FrameInfo + JankSpan + ColdStart + StallSpan`
2. `feat(tool): emit_perf_trace.go + schema`
3. `feat(agent): perf_triager.go + perf-triage-skill`
4. `feat(state): Mutable.SetPerfTrace + PerfTrace`
5. `feat(cmd): --htrace + --htrace-text + REPL /htrace`
6. `feat(analysis): IntentPerformance + ScenarioPerformance + 4 default hypotheses`
7. `feat(perf): Tier2 fallback (perf→log_triage text) + Tier3 utf-8 fail surface`
8. `test(perf): TestEmitPerfTrace_HappyPath + TestPerfFallback_Tier2_LogTriage`
9. `feat(eval): cases/harmony/hitrace_jank.case`
10. `docs: architecture.md HiTrace section + CLAUDE.md PerfTrace channel`

---

## 10. PR-C — hvigor / cjpm runner + 双语言 skill prompt

**估时**：~8 commits。
**依赖**：PR-A（与 PR-B 并行）。

### 10.1 hvigor runner（ArkTS 写模式）

`internal/tool/run_tests.go` 新增 detector：
```go
{"oh-package.json5",     "arkts"},
{"build-profile.json5",  "arkts"},
{"hvigorfile.ts",        "arkts"},
```

测试命令（待 §5 调研确认）：
```go
case "arkts":
    return "hvigorw test", junitXMLPath
```

JUnit XML 输出复用既有 Java 路径（`internal/tool/run_tests.go::parseJUnitXML`）。

**Tier 2 兜底**：JUnit XML 缺失 → exit-code only PASS/FAIL（复用 CMake/Meson 路径）。

### 10.2 cjpm runner（仓颉写模式，已锁）

```go
{"cjpm.toml", "cangjie"},
```

测试命令（cjpm 1.0.0 LTS 公开文档**未列 `--json` flag**；主路径走文本解析，启动时 `cjpm test --help` 一次性探测 future-proof）：
```go
case "cangjie":
    if probedHasJSONReporter {
        return "cjpm test --json", "" // future-proof：仓颉 1.1+ 若加 --json 自动启用
    }
    return "cjpm test", "" // 默认走文本输出，cargo 风格解析（test result: ok. X passed, Y failed）
```

**Tier 2 兜底**：JSON / 文本均失败 → exit-code only（复用 CMake 路径）。

### 10.3 ArkTS 严格模式 skill prompt（11 条 checklist）

`internal/skill/code_write.go` 加 ArkTS 段：
1. 不写 `any` 类型，全部显式标注
2. 不写 `as` 强制类型转换
3. 不写索引签名 `{[k:string]: T}`
4. 不写函数式 `Function` 类型
5. import 必须含扩展名（`.ets` / `.ts`）
6. class field 必须显式类型
7. `Object.keys()` 等返回 `string[]` 不能反向索引未声明字段
8. struct 不是 class，不能用 `new`
9. 装饰器仅限 21 白名单
10. UI build() 返回必须是单根（Column/Row/Stack）
11. 不引 `@ohos.commonEvent` 老 API（用 `@ohos.commonEventManager`）

### 10.4 仓颉风格 skill prompt

1. `func` 而非 `function`
2. 包声明必须（首行 `package xxx.yyy`）
3. `public` 显式标记 export
4. `extend` 用法（不是 trait impl 也不是 class）
5. `@CallingThread` 在主线程检查
6. 模式匹配用 `match` 而非 `switch`
7. trait-like `interface`（不是 Java interface）
8. operator overload 用 `operator func`
9. FFI 用 `foreign func` + `@CJOH`
10. `var` 可变 / `let` 不可变（与 Rust 一致）

### 10.5 PR-C commits 拆分（8 个）

1. `feat(verifier): detect oh-package.json5/build-profile.json5/hvigorfile.ts → arkts`
2. `feat(verifier): hvigorw test runner + JUnit XML reuse`
3. `feat(verifier): detect cjpm.toml → cangjie + cjpm test runner`
4. `feat(verifier): cjpm JSON parser + cargo-style text fallback`
5. `feat(verifier): exit-code Tier2 fallback for both runners`
6. `feat(skill): ArkTS strict-mode 11-rule checklist in code_write_skill`
7. `feat(skill): Cangjie style guide in code_write_skill`
8. `test(verifier): TestRunTests_Arkts_Hvigor + TestRunTests_Cangjie_Cjpm + Tier2 exit-code`

---

## 11. 全栈 fallback 规范

### 11.1 5 个 fallback 域

| 域 | Tier 数 | 主路径 | 降级路径 |
|---|---|---|---|
| ArkTS grammar | 4 | tree-sitter-arkts | tree-sitter-typescript → regex → path-only |
| 仓颉 grammar | 3 | tree-sitter-cangjie | regex → path-only |
| 配置文件 resolver | 3 | json5/toml | strict-JSON / line-by-line → basename glob |
| log_triage stack frames | 3 | LLM emit_log_triage | log-segmentation-skill → regex |
| HiTrace 性能 | 3 | LLM emit_perf_trace | text→log_triage → 提示 perfetto |
| runner | 2 | JUnit XML / JSON | exit-code only |

### 11.2 共享原则（红线 L-Fallback-*）

1. **每次降级必须打 WARN 日志**：`[<subsystem>] %s X→Y (tier N→M): %s`，含文件名 + tier 转移 + 原因首行
2. **Tier3/Tier4 文件 rank 强折扣**：防"语法越烂越优先"反激励
3. **仓库级阈值告警**：grammar 整体降级占比超阈值 → 启动 banner 一次（不阻塞，不重复）
4. **fail-loud 不被违反**：fallback 是"主路径失败时降级保持可用"，不是"主路径偷偷不跑"。Tier1 的 LLM 必须严格 fail-loud；其失败才触发 Tier2

### 11.3 监控埋点（不入 PR-0/-0c，留 §15 待办）

未来可加：每次启动汇总 `<lang> tier1: N% / tier2: N% / tier3: N% / tier4: N%`，写 metrics 文件供运维看。本系列 PR 不实现，仅留 hook 点。

---

## 12. 红线（5 条）

### L-ArkTS-2：`.ts` 语言归属动态切换
`extToLang[".ts"] → LangArkTS` 仅在 `build.go` 探测到工程根 / 任意子目录 `oh-package.json5` 时生效。纯 TypeScript 工程必须保持 `LangTypeScript`，不污染。
**测试**：`TestBuild_PureTSProject_KeepsTypeScript` + `TestBuild_ArkTSProject_PromotesTSToArkTS`。

### L-ArkTS-3：grammar Tier1 不放宽
tree-sitter-arkts grammar 严格模式不接受 `any` / `as` / `index_signature` / `Function` 类型；遇到立即 ERROR 节点；Tier2 切换是显式降级（必有 WARN），不是 Tier1 内偷偷放过。
**测试**：`TestArkTSGrammar_RejectsAnyType` + `TestArkTSGrammar_RejectsAsExpression`。

### L-Cangjie-1：`.cjo` 编译产物不进 repomap
scanner 显式 deny-list：`*.cjo` / `target/` / `.cangjie-cache/` / `build/`。
**测试**：`TestScanner_CjoFiles_Excluded`。

### L-Cangjie-2：包路径必须从 package_clause 读
仓颉 `FileInfo.Package` 不允许从路径推断（与 Go/Java 不同 —— 路径不是仓颉的包路径来源）。`extract_cangjie.go` 必须先扫顶部 `package xxx.yyy` 写入 `FileInfo.Package`；resolver 跨文件 import 用此字段。
**测试**：`TestExtractCangjie_PackageFromClause` + `TestExtractCangjie_NoPackageClause_EmptyPkg`（不允许 fallback 到路径）。

### L-Fallback-1/2/3：降级非静默
1. 每次降级必有 WARN 日志（含 tier 转移 + 原因）
2. Tier3/Tier4 强折扣（防反激励）
3. 仓库级 Tier2 占比超阈值启动 banner（grammar 该升级的告警）
**测试**：`TestParseFallback_LogsWarnOnDegrade` + `TestRank_DiscountByParseTier` + `TestBuild_BannerOnHighFallbackRatio`。

---

## 13. 测试计划

### 13.1 单元测试

| 文件 | 用例数 | 覆盖 |
|---|---|---|
| `extract_arkts_test.go` | ~20 | struct/build/装饰器/状态字段/import |
| `extract_cangjie_test.go` | ~25 | func/class/extend/foreign/operator/match |
| `resolver_arkts_test.go` | ~10 | oh-package.json5 + 黑洞 + json5→strict-JSON 降级 |
| `resolver_cangjie_test.go` | ~10 | cjpm.toml + std/core 黑洞 + 降级 |
| `parse_fallback_test.go` | ~15 | ArkTS 4 tier + 仓颉 3 tier + WARN 验证 |
| `logtriage/resolve_arkts_test.go` | ~8 | basename glob 各种路径形态 |
| `logtriage/resolve_cangjie_test.go` | ~8 | 同上 |
| `agent/perf_triager_test.go` | ~6 | PerfBundle 结构 + Tier2/3 fallback |
| `verifier_arkts_test.go` | ~5 | hvigor JUnit XML + exit-code |
| `verifier_cangjie_test.go` | ~5 | cjpm JSON / 文本 / exit-code |

### 13.2 集成测试（eval cases）

| 用例 | 期望 |
|---|---|
| `eval/cases/harmony/arkts_repomap.case` | repomap 抽出 N 个 @Component / N 个 @Builder |
| `eval/cases/harmony/cangjie_repomap.case` | repomap 抽出 N 个 func / N 个 extend |
| `eval/cases/harmony/hilog_arkts_panic.case` | LogBundle.ResolvedFiles 含 entry/src/main/ets/...ets |
| `eval/cases/harmony/hilog_cangjie_panic.case` | LogBundle.ResolvedFiles 含 .cj |
| `eval/cases/harmony/hitrace_jank.case` | PerfBundle.JankSpans 非空 |

### 13.3 红线结构测试（go/ast scan）

`internal/tool/repomap/index/red_lines_arkts_test.go`：
- L-ArkTS-2：grep `extToLang[".ts"] = LangArkTS` 必须在 `build.go` 的 hasOhPackage 分支内
- L-Cangjie-1：grep scanner 黑名单含 `.cjo`
- L-Cangjie-2：grep `extract_cangjie.go` 必须有 `FileInfo.Package = ` 来自 package_clause 的路径

### 13.4 性能测试（benchmark）

```go
BenchmarkParseArkTS_Tier1     // 严格 grammar 解 100 个 .ets 文件
BenchmarkParseArkTS_Tier2     // 全降到 TS grammar
BenchmarkParseArkTS_Tier3     // 全 regex
BenchmarkParseCangjie_Tier1
BenchmarkParseCangjie_Tier2
```

目标：Tier1 < 10× 单 .go 文件解析耗时；Tier2/3 不能比 Tier1 慢（避免反向激励）。

---

## 14. 开工顺序与里程碑

```
Week 1     PR-(-1) 调研 + 设计落定 (本文档 + corpus + eval cases)
Week 1.5–4 PR-0  ArkTS grammar + repomap + 4 层 fallback        ┐
Week 1.5–4 PR-0c 仓颉 grammar + repomap + 3 层 fallback         ┘ 并行
Week 4–6   PR-A  log_triage 扩展 (hilog + 双语言栈)
Week 6–7   PR-B  HiTrace + emit_perf_trace                       ┐
Week 6–7   PR-C  hvigor + cjpm runner + skill prompt             ┘ 并行
Week 7–9   缓冲 / 客户验证 / 调优
```

总估时：~7-9 周，76 commits。

### 阶段验收标准

- **PR-(-1) 完成**：corpus 30+40 全部为真实公开样本；eval cases 5 个能在 mock LLM 下跑通；本文档 §3 决策 #3 仓颉版本号锁定
- **PR-0 完成**：跑客户/公开 ArkTS 工程 `repomap`，Tier1 占比 > 80%；fallback 日志可观察；Tier2 banner 不误报
- **PR-0c 完成**：跑公开仓颉工程 `repomap`，Tier1 占比 > 75%；package_clause 全部解析；extend 关系正确挂回
- **PR-A 完成**：hilog ArkTS panic + 仓颉 panic 各 1 个 eval case 端到端绿
- **PR-B 完成**：jank trace 能抽出 PerfBundle.JankSpans；perfetto 提示对二进制 trace 触发
- **PR-C 完成**：写模式对 ArkTS 工程能 plan→apply→verify 跑通；exit-code 兜底验证

---

## 15. 未决项 / 风险

### 15.1 待 PR-(-1) 调研锁定的事实

- 仓颉公开最新版本号（语言规范 + 编译器）
- 仓颉栈帧实际格式
- HiTrace 文本输出格式（systrace 兼容 / 独立 / json）
- hvigor 测试输出格式（JUnit XML 路径 / 私有 JSON / 文本）
- cjpm `--json` 是否可用
- `oh-package.json5` 实际是否多用 strict-JSON 子集

### 15.2 已知风险

- **R1 ArkTS 演进快**：API 13/14 引入新装饰器或语法 → grammar 频繁返工。**缓解**：Tier2 TS grammar 兜底；启动 banner 提示升级
- **R2 仓颉社区资料少**：corpus 来源主要靠官方文档与少量 SIG 工程 → grammar 真实场景覆盖不足。**缓解**：Tier2 regex 兜底；仓库级阈值告警放宽到 50%
- **R3 hdc 日志 GB 级**：`log_attach_max_bytes` 默认 1MB 直接被截 → 客户感知差。**缓解**：调研期评估默认值是否抬高（如 8MB），写模式启动时 banner 提示 `--log-source-prefix` 用法
- **R4 ArkTS↔Cangjie 互调链栈帧**：mixed 工程崩溃同时含两种格式栈 → LLM 可能漏抽。**缓解**：prompt 加 mixed example；regex Tier3 兜底
- **R5 grammar 产物 checkin 体积**：parser.c 单文件可能 2-5MB。**缓解**：单文件不影响 git；如果超 10MB 再考虑 git LFS

### 15.3 留待后续（不在本系列 PR）

- ArkUI-X 跨平台特殊处理（Android/iOS 输出）
- HarmonyOS Native (C/C++) 与 ArkTS NAPI 桥接专项
- DevEco Studio 项目模板生成
- HiTrace 二进制 native 解析
- MCP server 提供华为内部诊断工具（在 mcp_integration.md 体系中扩展，不在本系列）

---

## 附录 A — 文件结构总览（实施后）

```
docs/design/
├── mcp_integration.md
└── harmonyos_arkts_cangjie_support.md     # 本文档

internal/thirdparty/                        # 新顶级目录
├── tree-sitter-arkts/
│   ├── grammar.js
│   ├── src/{parser,scanner}.c + tree_sitter/
│   ├── corpus/*.txt
│   ├── package.json
│   ├── README.md
│   └── binding.go
└── tree-sitter-cangjie/
    └── (同上结构)

internal/tool/repomap/
├── index/
│   ├── extract_arkts.go               # 新
│   ├── extract_cangjie.go             # 新
│   ├── resolver_arkts.go              # 新
│   ├── resolver_cangjie.go            # 新
│   ├── parse_fallback.go              # 新（ArkTS 4 tier）
│   ├── parse_fallback_cangjie.go      # 新（仓颉 3 tier）
│   └── (既有文件 build.go / parser.go / rank.go 修改)
└── types/lang.go                      # 加 LangArkTS / LangCangjie

internal/analysis/logtriage/
├── resolve_arkts.go                   # 新
├── resolve_cangjie.go                 # 新
└── validate.go                        # switch 加 arkts/cangjie

internal/types/
├── perf_bundle.go                     # 新（PerfBundle 等）
└── mutable_state.go                   # 加 SetPerfTrace / PerfTrace

internal/agent/
├── perf_triager.go                    # 新

internal/tool/
├── emit_perf_trace.go                 # 新
└── run_tests.go                       # 加 arkts/cangjie detector

internal/skill/
└── code_write.go                      # 加 ArkTS 严格 11 条 + 仓颉风格

eval/cases/harmony/                    # 新目录
├── arkts_repomap.case
├── cangjie_repomap.case
├── hilog_arkts_panic.case
├── hilog_cangjie_panic.case
└── hitrace_jank.case
```

---

**文档版本**：rev3.1（2026-04-25，6 项调研事实已锁）
**已交付**（PR-(-1).b 进行中）：
- corpus 种子：`internal/thirdparty/tree-sitter-arkts/corpus/sources/` 6 文件 + `tree-sitter-cangjie/corpus/sources/` 8 文件
- eval cases：`eval/cases/harmony/` 6 个（hilog ArkTS + Cangjie + mixed + HiTrace + ArkTS repomap + Cangjie repomap）
- 决策 #3 仓颉版本号已锁 1.0.0 LTS；hilog/HiTrace/cjpm/hvigor 格式全部锁定

**下一步**：PR-(-1).b 收尾（corpus 扩到 30+40 全量；本 session 先种 6+8）→ commit → PR-0 / PR-0c 并行开工。
