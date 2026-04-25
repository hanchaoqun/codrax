# HarmonyOS / Android / ArkTS / Cangjie / Kotlin 全栈支持 — 设计文档

**状态**：已实现，与 main 分支代码同步。

**覆盖目标**：
- HarmonyOS 生态：ArkTS UI 代码 + Cangjie 原生代码 + hilog 运行时日志 + HiTrace 性能 trace + hvigor 测试运行
- Android 生态：Kotlin / Java 代码 + logcat 日志 + atrace / systrace / perfetto 性能 trace + Gradle 测试运行
- 通用能力：repomap 抽取 / 问题分析 / 写模式代码生成 / runtime 故障归因

代码层面所有改动落在 `internal/` 与 `cmd/`，无新顶层包。新依赖 0（json5 / TOML 解析为本地手写实现，~520 LOC）。

---

## 目录

- [1. 系统架构总览](#1-系统架构总览)
- [2. Repomap 语言扩展](#2-repomap-语言扩展)
  - [2.1 ArkTS 抽取器](#21-arkts-抽取器)
  - [2.2 Cangjie 抽取器](#22-cangjie-抽取器)
  - [2.3 Kotlin 抽取器](#23-kotlin-抽取器)
  - [2.4 Tier 折扣 + Banner 阈值](#24-tier-折扣--banner-阈值)
  - [2.5 Resolver 设计](#25-resolver-设计)
- [3. Log Triage 通道扩展](#3-log-triage-通道扩展)
- [4. Perf Triage 独立通道（详细设计）](#4-perf-triage-独立通道详细设计)
  - [4.1 数据流总览](#41-数据流总览)
  - [4.2 PerfBundle 数据结构](#42-perfbundle-数据结构)
  - [4.3 emit_perf_trace 工具契约](#43-emit_perf_trace-工具契约)
  - [4.4 perf_triager Agent 控制器](#44-perf_triager-agent-控制器)
  - [4.5 Two-step Fallback 详细设计](#45-two-step-fallback-详细设计)
  - [4.6 PerfBundle 合并算法](#46-perfbundle-合并算法)
  - [4.7 阈值与启发式](#47-阈值与启发式)
  - [4.8 Skill Prompt 设计](#48-skill-prompt-设计)
  - [4.9 与 LogTriage 的关系](#49-与-logtriage-的关系)
- [5. 测试运行器集成（hvigor / cjpm）](#5-测试运行器集成hvigor--cjpm)
- [6. 写模式 Skill Prompt](#6-写模式-skill-prompt)
- [7. 多文件附加 + 字节上限](#7-多文件附加--字节上限)
- [8. 配置 Knob 总览](#8-配置-knob-总览)
- [9. 红线（不变量）](#9-红线不变量)

---

## 1. 系统架构总览

```
                       ┌────────────────────────────────────┐
       --log / /log    │                                    │
       (StringArrayVar │  BusContext.AttachedLog (text)     │
        + concat)      │                                    │
   ────────────────────►                                    │
                       │                                    │
       --htrace        │  BusContext.AttachedHitrace (text) │
       --atrace alias  │                                    │
       /htrace /atrace │                                    │
   ────────────────────►                                    │
                       └─────┬──────────────────────────────┘
                             │
                             ▼  (orchestrator preStages 顺序触发)
                   ┌──────────────────────────────┐
                   │  log_triage (条件)           │
                   │  Guard: AttachedLog != ""    │
                   │  Agent: log_triager          │
                   │  Output: Mutable.LogTriage() │
                   └──────────────┬───────────────┘
                                  │
                                  ▼
                   ┌──────────────────────────────┐
                   │  perf_triage (条件)          │
                   │  Guard: AttachedHitrace != ""│
                   │  Agent: perf_triager         │
                   │  Output: Mutable.PerfTrace() │
                   └──────────────┬───────────────┘
                                  │
                                  ▼
                  analyze → explore → extract → finalize
                  (主流水线，4 阶段 × 4 Agent)
```

两条 pre-stage 互相独立，可以同时触发或单独触发。失败均为 advisory，不阻塞主流水线（`Mutable.LogTriage()` / `Mutable.PerfTrace()` 停留 `nil`，下游消费者全部 nil-safe）。

**Analyzer 同时消费两份 bundle**（`internal/agent/analyzer.go`）：
- `logtriage.MergeEntities` 把 LogBundle.Entities + PerfBundle.Entities union 进 `AnalyzerHints.Entities`
- `analyzerRequiredFiles` 取 `LogBundle.ResolvedFiles ∪ PerfBundle.ResolvedFiles`（cap 10）作 first-class 文件锚点

---

## 2. Repomap 语言扩展

`internal/tool/repomap/types/lang.go` 注册 4 个新语言常量 + 扩展名映射；`internal/tool/repomap/index/` 新增对应抽取器与 resolver。

### 2.1 ArkTS 抽取器

`internal/tool/repomap/index/extract_arkts.go`

**实现策略**：复用既有 `tree-sitter-typescript`（已是依赖）+ ArkTS 后处理 pass。理由：
- TS 是 ArkTS 严格模式的近超集（class / function / module / decorators 全部共享）
- ArkTS-specific surface（`struct` 关键字 / 21 装饰器 / ArkUI 链式调用 + trailing block）走 regex 后处理
- 无需自维护 tree-sitter-arkts 的 grammar.js + parser.c（外发版本管理风险高，无 tree-sitter CLI 不可生成）

**ArkTS 装饰器白名单**（21 个，单一权威列表）：
- ArkUI 结构：`@Component @Entry @Preview @CustomDialog @Observed @Reusable`
- 函数级 UI：`@Builder @BuilderParam @Styles @Extend`
- 状态管理：`@State @Prop @Link @Provide @Consume @Watch @ObjectLink @StorageLink @StorageProp @LocalStorageLink @LocalStorageProp`

**抽取规则**：

| Symbol.Kind | 触发模式 | 备注 |
|---|---|---|
| `component` | `(@Decorator(\([^)]*\))? \s+)* struct <Name>` | 装饰器可同行可跨行 |
| `ui-entry` | struct body 内 `build()` 方法 | Parent 设为外围 component |
| `state-field` | `@State <name>: <type>` | 同样路径处理 @Prop / @Link / @Provide / @Consume / @ObjectLink |
| `prop-field` | `@Prop <name>: <type>` | |
| `link-field` | `@Link <name>: <type>` | |
| `provide-field` / `consume-field` | `@Provide <name>` / `@Consume <name>` | |
| `object-link-field` | `@ObjectLink <name>` | |
| `storage-field` | `@StorageLink` / `@StorageProp` | 两类合并到一个 kind |
| `local-storage-field` | `@LocalStorageLink` / `@LocalStorageProp` | |
| `watch-binding` | `@Watch('handler') <name>` | |
| `builder` | `@Builder function <name>(` 或同行变体 | |
| `styles` | `@Styles function <name>()` | |
| `extend` | `@Extend(<Type>) function <name>()` | Parent = 被扩展类型 |

**.ts 文件的归属**：scanner 探测到任意祖先目录有 `oh-package.json5` 时把 `LangTypeScript` 升级为 `LangArkTS`。`types.IsArkTSProject(repoRoot, relPath)` 是单一权威函数，向上 12 层目录扫描。纯 TypeScript 仓库不会被污染（红线 L-ArkTS-2）。

### 2.2 Cangjie 抽取器

`internal/tool/repomap/index/cangjie_lexer.go` + `cangjie_parser.go`

**实现策略**：手写 token 流 + 递归下降 parser。理由：
- 仓颉无近邻 tree-sitter grammar 可复用（Rust / Swift / Java grammar surface 偏差太大）
- 仓颉 surface 中等复杂（`func / class / struct / interface / enum / extend / match / operator / foreign`）
- 自维护 tree-sitter-cangjie 需 Rust CLI + 持续 grammar 维护，ROI 低
- `parser.go` 的 BaseAgent 路由层留出"`GetSitterLanguage` 返回 nil 时走 native"的口子

**Lexer**（`cangjie_lexer.go`）：

输入字节流，输出 `cangjieToken{ Kind, Text, Offset, Line }` 流。
- 跳过单行 `//` 与块 `/* */` 注释（保留换行符以维持行号）
- 字符串 `"..."`（含 `\` 转义）+ Rune `'...'`（Cangjie 单引号是 Rune 而非 String）整段消化
- 标点：`{ } ( ) [ ] , : ; . @ < > = ->`
- 关键字白名单：`package import class struct interface enum extend func init main operator foreign public private protected internal open static sealed abstract override redef mut const unsafe let var as where`
- 其余文字归 `cjTokIdent` 或 `cjTokOther`

**Parser**（`cangjie_parser.go`）：

递归下降 + 状态栈 + 平衡 token 跳过。

```
run() → consumeDecorators() → consumeModifiers() → 关键字 dispatch
  ├── parsePackage()       // 写 FileInfo.Package（红线 L-Cangjie-2）
  ├── parseImport()
  ├── parseTypeDecl(class|struct|interface|enum, parent)
  │     └── parseBody(name) → 递归下降处理 body 内 nested decl
  ├── parseExtendDecl()        // extend Type [<: Trait]* { ... }
  ├── parseFuncDecl(parent)    // func Name<G>(p: T): R where C? body?
  ├── parseForeignFunc()       // foreign func Name(...)
  ├── parseOperatorFunc(parent) // operator func + / >> / [] / ==
  ├── parseInit(parent)        // init(...) (Cangjie 构造函数无 func)
  └── parseMainEntry()         // main(): Int64 (无 func 修饰)
```

**关键设计点**：
1. **enclosing-type 栈**：`parseTypeDecl` 调用 `parseBody(typeName)`，body 内的 `func` / `init` / `operator` 全部带上 `parent=typeName` → Symbol.Parent 正确写到所属类
2. **`<:` 与 generic `<` 区分**：`skipGenerics()` peek 下一 token，若是 `:` 则视为 `<:` 继承运算符 left here，否则进入泛型平衡跳过
3. **operator 名识别**：`+ / >> / == / [] / .` 都是合法 operator 字符，token 流可能产出 `<`/`>` 也被接受
4. **return type 跳过**：`skipReturnType()` 在 `:` 后吞类型表达式直到遇到 body 起点 `{` / `=` / `where` / 上层 `)` / `}`
5. **`<:` 双形态**：lexer 看到 `<` 紧跟 `:` 输出两 token，parser 同时处理 `<:` 与不带 `<` 的 `:` 继承形式

**Cangjie 红线**：
- `FileInfo.Package` 必须从 `package_clause` 来，禁止路径推断（L-Cangjie-2，`TestExtractCangjie_PackageMustComeFromClause` 守护）
- `.cjo` 编译产物 scanner 显式 deny（L-Cangjie-1，`TestApplyHarmonyOSPostProcess_CjoDeny` 守护）

### 2.3 Kotlin 抽取器

`internal/tool/repomap/index/extract_kotlin.go`

**实现策略**：tree-sitter-kotlin（已 vendored 在 `github.com/smacker/go-tree-sitter/kotlin`）+ AST walk。无后处理 regex（grammar 已完整）。

**节点 → Symbol 映射**（实测 vendored grammar）：

| AST 节点 | Symbol.Kind 推导 | Modifier 细化 |
|---|---|---|
| `package_header` | 写 `FileInfo.Package` | — |
| `import_header` | `types.Import`（path / alias / wildcard `.*`） | — |
| `class_declaration` | `class` / `data-class` / `sealed-class` / `enum` / `annotation` / `interface`（细化由 modifiers + `interface` 字面 token 决定） | — |
| `object_declaration` | `object` | — |
| `companion_object` | `companion-object`，匿名时合成 `Companion` 名 | — |
| `function_declaration` | `function` / `method` / `suspend-function` / `operator` / `extension-function` | extension fn 检测：第一个 named child 是 `user_type` / `nullable_type` 在 `simple_identifier` 之前 |
| `property_declaration` | `field` / `var` / `val` / `const` | name 来自 `variable_declaration > simple_identifier` |
| `type_alias` | `type` | — |

**Inheritance 边**：`delegation_specifier` 子节点穿透 — `constructor_invocation > user_type` 是 class 父类构造形态（`: ComponentActivity()`），裸 `user_type` 是 interface 实现，`explicit_delegation` 是 `by xxx`。三种都产 `Relation{Kind:"inheritance"}`。

**关键 quirks**：
- tree-sitter-kotlin 不暴露 `class_declaration > body` 字段名（`ChildByFieldName("body")` 返回 nil）。代码迭代 `NamedChildren` 找 `class_body` / `enum_class_body`
- `companion_object` 节点不属于 `object_declaration` switch case，单独处理
- `property_declaration` 的 name 在 `variable_declaration > simple_identifier`，不是直接子 simple_identifier

### 2.4 Tier 折扣 + Banner 阈值

`internal/tool/repomap/index/parse_fallback.go`

```go
func TierDiscount(tier int) float64 {
    case 0, 1: return 1.0
    case 2:    return 0.85
    case 3:    return 0.6
    default:   return 0.3   // 4+
}
```

`retrieve.rank.go::parseTierDiscount` 内联同曲线（避免跨包 import cycle），rank score 末尾 `score *= parseTierDiscount(fi.ParseTier)`。Tier 2+ 文件不可能在相同证据下超过 Tier 1 同名兄弟。

**Repo 级 banner 阈值**：

| Lang | 阈值 | 触发条件 |
|---|---|---|
| ArkTS | 0.4 | Tier-2 占比 > 40% → 启动一次 WARN，提示 grammar 升级 |
| Cangjie | 0.5 | 同上 0.5 阈值（仓颉 surface 演进快，更宽容）|

`reportFallbackRatios(files []*types.FileInfo)` 在 `BuildGraph` 后调用，每个 lang 至多一行 WARN。

### 2.5 Resolver 设计

`internal/tool/repomap/index/resolver_arkts.go` + `resolver_cangjie.go` + `resolver_kotlin.go`

**ArkTS Resolver**（`arkTSImportResolver`）：

- 包装既有 `jsImportResolver`（共享 tsconfig alias 路径 + relative 解析逻辑）
- 5 类 builtin 黑洞前缀：`@ohos.*` / `@kit.*` / `@hms.*` / `@arkui.*` / `@system.*` —— 这些是 HarmonyOS 运行时 API，永不映射仓内文件
- `@bundle:<name>[/sub]` 解析：`Prepare` 扫所有 `oh-package.json5`（用本地 `json5_parser.go`），建 `bundleMap[name] = manifestDir`，`Resolve` 时 `bundleMap[bundle] + sub` → 走 `resolveJsCandidate` 探 `.ets/.ts/.d.ts/.js` 扩展 + `Index.ets/index.ets` 目录入口

**Cangjie Resolver**（`cangjieImportResolver`）：

- 5 类 builtin 黑洞：`std.*` / `core.*` / `runtime.*` / `ohos.*`（仓颉侧的 ohos 互操作，与 ArkTS `@ohos.*` 区分）
- `Prepare` 扫所有 `cjpm.toml`（用本地 `toml_parser.go`），建 `externalDeps`（`[dependencies]` 段声明的依赖名 → 视为外部不解析）+ `modulePkgs`（`[package].name → manifestDir` 映射）
- `Resolve` 优先级：external dep 黑洞 → `PkgToFiles[exact path]` → 模块名匹配（取目录下所有 `.cj`）→ 包前缀逐级缩短匹配 → 最后 `BasenameIndex` 末段匹配（限 `.cj`）

**Kotlin Resolver**（`kotlinImportResolver`）：

- 与 Java resolver 同形态：`Prepare` 建 `packageIndex[Package] → []RelPath` + `declInFile[pkg+"."+name] → []RelPath`
- `Resolve` 优先精确 decl 命中，其次同包目录，wildcard import (`a.b.*`) 整包返回
- 与 Java resolver 共存于同一 graph（混合 Android 项目同时含 Java + Kotlin 文件，两 resolver 各自处理自己 Lang 的文件）

**JSON5 / TOML 解析器**（`json5_parser.go` + `toml_parser.go`）：

stdlib-only 手写实现。理由：
- 唯一使用点是 `oh-package.json5` / `cjpm.toml`，feature surface 窄
- 第三方依赖增加供应链风险，得益薄

JSON5 支持：行 / 块注释、尾逗号、未引号 key、单引号 string、hex 数、leading-dot float、`\u` 转义。
TOML 子集支持：`[section]` 头、`key = value`、字符串/整型/浮点/布尔/扁平数组、行尾注释、`'literal'` string。多行 string 与 inline table 显式不支持（cjpm.toml 不需要）。

---

## 3. Log Triage 通道扩展

`internal/analysis/logtriage/`

**新增 lang 枚举**（`emit_log_triage.go::langEnum`）：在 `go / java / cpp / python / node / rust / ruby / csharp` 之上加 `kotlin / arkts / cangjie`。skill prompt 教 LLM 三种新栈格式：
- ArkTS：V8 风格 `at IndexPage.build (entry/src/main/ets/pages/Index.ets:128:9)`
- Cangjie：JVM-like `at demo.cart.Cart.itemAt(src/cart/Cart.cj:78)`
- Kotlin：JVM 同 Java 但扩展名 `.kt` `at com.example.Foo$bar(Foo.kt:42)`

**hilog / logcat 行格式**（同构）：
```
<MM-DD HH:MM:SS.mmm> <PID> <TID> <LEVEL> <DOMAIN/TAG>: <body>
```
- HarmonyOS：`01-26 11:01:06.870 1051 1051 W 00201/test: Failed to ...`
- Android：`04-15 14:32:18.421  5821  5821 E JsApp: ...`

skill 教 LLM 同一处解析；message 体进 frame.raw，DOMAIN/TAG 进 entity 候选。

**新增 ResolveFile**：

| 函数 | Tier 1 | Tier 2 | Tier 3 |
|---|---|---|---|
| `ResolveArkTSFile(baseName, candidates)` | `entry/src/main/ets/...` Stage Model 路径 | `<module>/src/main/ets/...` 子模块 / `commons/...` | basename baseline |
| `ResolveCangjieFile(pkg, baseName, candidates)` | dir 后缀匹配 `pkg→pkgPath` 的精确 | `src/main/cangjie/` / `src/cangjie/` / `cangjie/` / `src/` 源根 + 目录后缀 | baseline |
| `ResolveKotlinFile(pkg, baseName, candidates)` | dir 后缀精确匹配 | Android Gradle 路径（`src/main/kotlin/`、`src/main/java/`、JB MPP `commonMain/`、`androidMain/`、`jvmMain/`、`test/kotlin/`） | baseline |

`validate.go::validateFrame` 按 lang 分发（`f.Lang == "arkts"` / `isArkTSBasename(f.File)` 等启发式 + 路径形态判断），basename-only 走 `GlobByBasename` + 对应 Resolve；带路径的走 `StripBuildPathPrefix` + `ResolveFrameFile`。

**多文件附加边界识别**：log-triage-skill prompt 教 LLM 把 `# codrax-source: <path>` 行视为独立 capture 边界（不同进程 panic / 不同时间窗口）。

---

## 4. Perf Triage 独立通道（详细设计）

### 4.1 数据流总览

```
        ┌──────────────────────────────────────────────────┐
        │  CLI / REPL 输入                                 │
        │  --htrace / --atrace / --htrace-text / --atrace-text │
        │  /htrace / /atrace + append + show + clear       │
        │  (StringArrayVar 可重复，多文件加 # codrax-source 头) │
        └──────────────────────┬───────────────────────────┘
                               │
                               ▼
        ┌──────────────────────────────────────────────────┐
        │  cmd/root.go::loadAttachedTrace                  │
        │  - 互斥校验 (htrace vs atrace 别名 + file vs text)│
        │  - stdin 全局只读一次                            │
        │  - loadMultiPathSlice 拼接 + 头分隔               │
        │  - truncateAttachedToCap(maxAttachedTraceBytes)  │
        └──────────────────────┬───────────────────────────┘
                               │
                               ▼
        Orchestrator.SetAttachedHitrace(body)
        → busCtx.AttachedHitrace
        → AgentContext.AttachedHitrace（context/builder.go 镜像）
                               │
                               ▼
        ┌──────────────────────────────────────────────────┐
        │  StagePerfTriage 前置阶段                         │
        │  Guard: bus.AttachedHitrace != ""                │
        │  Agent: perf_triager                             │
        │  Skill: perf-triage-skill (单段) or              │
        │         perf-segmentation-skill + 重调          │
        │         (两步)                                   │
        └──────────────────────┬───────────────────────────┘
                               │
                               ▼
        ┌──────────────────────────────────────────────────┐
        │  emit_perf_trace 工具                            │
        │  - 解析 PerfBundle Layer 1-3                     │
        │  - derivePerfLayer4 (entities / signals /        │
        │    ResolvedFiles / IntentHint)                   │
        │  - Mutable.SetPerfTrace(bundle)                  │
        └──────────────────────┬───────────────────────────┘
                               │
                               ▼
        ┌──────────────────────────────────────────────────┐
        │  Analyzer 消费                                   │
        │  - rm.AnalyzerHints.Entities ∪ perf.Entities     │
        │  - analyzerRequiredFiles ∪ perf.ResolvedFiles    │
        │  - log.LogBundle.IntentHint OR perf.IntentHint   │
        └──────────────────────────────────────────────────┘
```

### 4.2 PerfBundle 数据结构

`internal/types/perf_bundle.go`

```go
type PerfBundle struct {
    Meta     PerfMeta      `json:"meta"`
    Frames   []PerfFrame   `json:"frames,omitempty"`
    Janks    []PerfJank    `json:"janks,omitempty"`
    Stalls   []PerfStall   `json:"stalls,omitempty"`
    Startup  *PerfStartup  `json:"startup,omitempty"`
    Residue  []string      `json:"residue,omitempty"`
    Coverage float64       `json:"coverage,omitempty"`

    // Layer 4 — 验证器派生，LLM 不可写
    ResolvedFiles []string `json:"resolved_files,omitempty"`
    Entities      []string `json:"entities,omitempty"`
    IntentHint    string   `json:"intent_hint,omitempty"`
}

type PerfMeta struct {
    Source     string   // hitrace / atrace / systrace / perfetto / unknown
    DurationMs float64  // 0 = unknown
    AppPID     int      // 0 = not detectable
    Signals    []string // jank / cold-start-slow / main-thread-stall / io-block / gc-pause / render-miss
    Summary    string   // ≤ 200 chars
}

type PerfFrame struct {
    FrameNo    int
    TsMs       float64
    DurationMs float64
    Phase      string // measure / layout / draw / composite / ""
    Janky      bool
}

type PerfJank struct {
    StartTsMs   float64
    DurationMs  float64
    TriggerSpan string   // 最内层 B|...|tag 的 tag
    Reason      string   // io / lock / sync-call / heavy-compute / ""
    Tags        []string // 完整 tag 栈，外层到内层
}

type PerfStall struct {
    StartTsMs  float64
    DurationMs float64
    Kind       string // io / lock / sync-rpc / native-call / ""
    Symbol     string // 函数符号
    File       string // 解析后的源文件（可空）
    Line       int
}

type PerfStartup struct {
    Mode          string  // cold / warm / hot
    AppLaunchMs   float64
    AbilityInitMs float64 // HarmonyOS Stage Model 专有
    FirstFrameMs  float64
}
```

**阈值常量**（公开导出，渲染端可读取）：

```go
PerfFrameBudget60HzMs = 16.67  // 60fps 帧预算
PerfStartupSlowColdMs = 1200.0 // 慢冷启动门
PerfMainThreadStallMs = 100.0  // 主线程阻塞门
```

### 4.3 emit_perf_trace 工具契约

`internal/tool/emit_perf_trace.go`

**JSON Schema**（`additionalProperties: false` 全层强制）：

- `meta` object，required `source`（enum 5 值），optional `duration_ms` / `app_pid` / `signals[]`（最多 8，enum 6 值）/ `summary`（≤200）
- `frames[]` 最多 200 项，每项 required `duration_ms`
- `janks[]` 最多 50 项，每项 required `start_ts_ms` + `duration_ms`，`tags[]` 最多 16
- `stalls[]` 最多 50 项，每项 required `start_ts_ms` + `duration_ms`
- `startup` optional，required `mode`（enum 3）
- `residue[]` 最多 8 项，每项 ≤500 字节

Schema **拒绝** Layer-4 字段（`resolved_files / entities / intent_hint / coverage`）—— LLM 不可写。

**Execute 流程**：
1. `json.NewDecoder(...).DisallowUnknownFields()` 解参数
2. 跨字段 sanity：`frames + janks + stalls + startup` 不可全空（meta-only 视为空 emission，强制重试）
3. `toPerfBundle` 复制成 PerfBundle，`Frames` 自动按 `PerfFrameBudget60HzMs` 标 Janky
4. `derivePerfLayer4` 派生 Layer 4
5. `Mutable.SetPerfTrace(bundle)`
6. ToolResult Summary 渲染 frames/janks/stalls/signals 计数 + IntentHint

**`derivePerfLayer4` 算法**（同步逻辑，非 LLM）：
- `IntentHint = "performance"` 当 `len(Janks) > 0 || len(Stalls) > 0 || (Startup != nil && Startup.AppLaunchMs > PerfStartupSlowColdMs)`
- `Entities`（cap 32）：jank.TriggerSpan + jank.Tags[] + stall.Symbol + stall.Kind + (startup.Mode + "-start")，去重，trim
- 自动 signals 增补：
  - `len(Janks) > 0` → push `"jank"`
  - `any(stall.DurationMs >= PerfMainThreadStallMs)` → push `"main-thread-stall"`
  - `Startup.Mode == "cold" && Startup.AppLaunchMs > PerfStartupSlowColdMs` → push `"cold-start-slow"`
- `ResolvedFiles`（cap 10）：unique stall.File（非空）

### 4.4 perf_triager Agent 控制器

`internal/agent/perf_triager.go`

**Settings**（`PerfTriageSettings`，全部 yaml 可调）：

| 字段 | 默认 | 作用 |
|---|---|---|
| `Enabled` | true | 总开关 |
| `MinBytes` | 200 | 小于此字节直接跳过（200 字节以下 trace 通常只是 banner） |
| `MaxRetries` | 1 | 单次 emit 失败重试次数 |
| `TwoStepEnabled` | true | 是否允许两步升级 |
| `TwoStepBytes` | 65536 | trace ≥ 此值直接走两步（不试单段） |
| `TwoStepCoverage` | 0.3 | 单段返回 coverage < 此值升级 |
| `MaxLLMCalls` | 12 | 总 LLM 调用上限（1 单段 + 1 分段 + 10 per-segment） |

**Execute 决策树**：

```
if !Enabled: return skipped("disabled")
if AttachedHitrace == "": return skipped("empty")
if len(AttachedHitrace) < MinBytes: return skipped("too short")

if TwoStepEnabled && len(AttachedHitrace) >= TwoStepBytes:
    return runTwoStep("oversized")

# 单段尝试
out := base.Execute(ctx, perf-triage-skill)
bundle := Mutable.PerfTrace()

if bundle != nil && bundle.Coverage >= TwoStepCoverage:
    return out  # 单段成功

if !TwoStepEnabled:
    return out  # 单段结果（即使低覆盖）

# 升级两步
Mutable.SetPerfTrace(nil)  # 清单段结果，让两步从空状态合并
return runTwoStep(...)
```

**ReAct 循环**：BaseAgent + `perfTriagerEvaluator`，`MaxIterations=6`（与 log_triager 同级）。`Observe` hook 监听 `emit_perf_trace` 或 `emit_perf_segmentation` 任一成功即停。

### 4.5 Two-step Fallback 详细设计

#### Step A：分段

调用 `perf-segmentation-skill` 让 LLM emit 一次 `emit_perf_segmentation`：

```json
{
  "segments": [
    {"byte_start": 0,    "byte_end": 1200, "kind": "context",      "hint": "..."},
    {"byte_start": 1200, "byte_end": 4500, "kind": "startup",      "hint": "Application#onCreate cold start"},
    {"byte_start": 4500, "byte_end": 7800, "kind": "frame_window", "hint": "Choreographer doFrame 16ms"},
    {"byte_start": 7800, "byte_end": 9500, "kind": "jank_region",  "hint": "performTraversals 85ms"}
  ]
}
```

**Kind enum** 6 值（专为 perf 设计，与 log segmentation 6 值正交）：
- `frame_window` — 完整渲染帧 envelope
- `jank_region` — 连续多帧 jank 或 stall 区段
- `startup` — 进程/Activity 启动窗口
- `thread_run` — 长时间 CPU/IO 单线程占用
- `context` — header / banner / 元数据
- `noise` — 不相关 / 非 actionable

Schema 限制：≤10 段，按 `byte_start` 升序，禁止重叠（`byte_end[N] ≤ byte_start[N+1]`）。

`emit_perf_segmentation.go::Execute` 校验：每段 `0 ≤ start < end ≤ len(AttachedHitrace)`，违规段静默丢弃。合法段写入 `Mutable.SetPerfSegments(rawJSON)`。

#### Step B：逐段抽取

```go
perSegBudget := MaxLLMCalls - 2  // 减去 1 次 single-shot + 1 次 segmentation
origTrace := ctx.AttachedHitrace
for _, seg := range segments:
    if calls >= perSegBudget: break
    if !IsExtractablePerfSegment(seg.Kind): continue  // 跳过 context / noise
    
    ctx.AttachedHitrace = origTrace[seg.ByteStart:seg.ByteEnd]
    Mutable.SetPerfTrace(nil)  # 给本段空白槽
    eval.emitSeen = false
    
    subOut := base.Execute(ctx, perf-triage-skill)
    calls++
    if subOut.OK && Mutable.PerfTrace() != nil:
        partials = append(partials, Mutable.PerfTrace())
ctx.AttachedHitrace = origTrace  # 恢复
Mutable.SetPerfSegments(nil)
```

**为什么不并行 fan-out**：BaseAgent + LLM 适配器是单线程友好；并行需要 N 份 BusContext.Mutable 实例隔离 + merge — 复杂度收益不匹配。串行 + budget cap 已能覆盖绝大多数场景。

#### Step C：合并

`internal/analysis/perftriage.MergePerfBundles(parts, rawTraceBytes)` 输出最终 PerfBundle，写入 `Mutable.SetPerfTrace`。详见 §4.6。

#### 降级路径

| 失败点 | 行为 |
|---|---|
| Segmenter 失败 / 0 段 | 返回 `StageReport: "two-step produced no segments — degraded"`，bundle 为 nil，主流水线继续 |
| Segments JSON 解析失败 | `"two-step malformed segmentation — degraded"` |
| 所有 segments emit 全失败 | `"two-step produced no per-segment bundles — degraded"` |
| Merge 返回 nil | `"two-step merge produced nil bundle — degraded"` |
| Skill 未注册（perf-segmentation-skill / perf-triage-skill 缺失）| WARN log + degraded report，不 panic |

每条降级路径都是 advisory（StageOutput.Error 为空，StageReport 写明状态）—— 主流水线 analyzer 看 `Mutable.PerfTrace() == nil` 自然跳过 perf 路径。

### 4.6 PerfBundle 合并算法

`internal/analysis/perftriage/merge.go`

**输入**：`parts []*PerfBundle` + `rawTraceBytes int`（原始 trace 总字节数，用于 Coverage 派生）

**合并策略**：

| 字段 | 策略 |
|---|---|
| `Meta.Source` | 多数票；并列则取首次出现 |
| `Meta.DurationMs` | 所有非零值的最大值（trace 实际跨度） |
| `Meta.AppPID` | 多数票，并列首次 |
| `Meta.Signals` | 集合并集，按字典序排序（保证幂等） |
| `Meta.Summary` | 所有非空摘要 `; ` 拼接，截 ≤200 字节 |
| `Frames` | 按 `(FrameNo, TsMs, DurationMs)` 三元组去重，保留输入序 |
| `Janks` | 按 `(StartTsMs, DurationMs, TriggerSpan)` 去重 |
| `Stalls` | 按 `(StartTsMs, DurationMs, Symbol)` 去重 |
| `Startup` | 取 `AppLaunchMs` 最大的（Cold 启动比 Warm 慢，"最大"即最确定 cold 信号）|
| `Residue` | 全字符串相等去重 |
| Layer 4 | 基于合并后的 Layer 1-3 重新 derive（`mergedDeriveEntities` / `mergedDeriveResolvedFiles` / `mergedDeriveIntentHint` / `mergedDeriveCoverage`） |

**Coverage 公式**：
```
Coverage = 1 - sum(len(residue[i])) / rawTraceBytes
```
clamp 到 `[0, 1]`。`rawTraceBytes == 0` 退化为 `1.0`（无信息时不主动报低覆盖）。

**幂等性**：相同输入 parts（顺序无关）→ 相同输出（signals 排序、startup 选最大、frames/janks/stalls 唯一签名）。`merge_test.go::TestMergePerfBundles_SignalsUnion_FramesConcatDedup` 守护。

**N=1 短路**：单段输入直接 pass-through 同一指针（避免无意义复制）。

### 4.7 阈值与启发式

| 常量 | 值 | 用途 |
|---|---|---|
| `PerfFrameBudget60HzMs` | 16.67 ms | 帧预算；`emit_perf_trace.toPerfBundle` 自动 set Janky；skill prompt 教 LLM 同阈值识别 jank_region 段 |
| `PerfStartupSlowColdMs` | 1200 ms | 慢冷启动；`derivePerfLayer4` 触发 `cold-start-slow` signal + `IntentHint=performance` |
| `PerfMainThreadStallMs` | 100 ms | 主线程阻塞；`derivePerfLayer4` 触发 `main-thread-stall` signal |
| `TwoStepBytes` | 64 KiB | 两步升级阈值；超过单段 LLM context 已力不从心 |
| `TwoStepCoverage` | 0.3 | 单段覆盖率门；低于此说明 trace 没读懂 |
| `MaxLLMCalls` | 12 | 总预算（1+1+10 配 segmenter cap） |
| `MinBytes` | 200 | 启动门；< 200 字节 trace 通常只是 banner |

为什么不让 LLM 输出阈值：阈值是设备/部署语义（60Hz/120Hz/144Hz、Tier-1 SoC vs 低端机），不该 per-call 漂移。yaml 静态配置，prompt 不打。

### 4.8 Skill Prompt 设计

#### perf-triage-skill

`internal/skill/defaults.go`

**Goal**：单次 `emit_perf_trace` 调用产出 PerfBundle。

**Workflow**（11 条）核心要点：
1. 读 "Attached Performance Trace" section；多文件 `# codrax-source:` 头视为独立 capture
2. 识别 source：`# tracer: nop` ftrace header / `TASK-PID CPU# TIMESTAMP FUNCTION` header → hitrace/atrace/systrace；perfetto banner → perfetto
3. 配对 `tracing_mark_write: B|<pid>|<tag>` 与 `E|<pid>` end，δ ≥ 16.67ms 标 Janky
4. UI-thread tag 启发：HarmonyOS `H:RenderService:DoFrame` / `H:Layout:measure` / `H:DataLoader:fetchSync`；Android `Choreographer#doFrame` / `performTraversals` / `RenderThread`
5. 为 jank 帧产 `PerfJank`：trigger_span = 最内层 active tag；tags[] = 完整 outer→inner 栈；reason = io/lock/sync-call/heavy-compute 启发
6. 主线程阻塞 > 100 ms 产 `PerfStall`：kind / symbol / file / line（tag 含路径时抽）
7. cold-start 检测：HarmonyOS `ActivityTaskManager / AppInit / WindowStage.loadContent`；Android `ActivityThread / Application#onCreate` → 产 PerfStartup
8. residue[] 装不可结构化的 chunks，≤8 项 ≤500 字节
9. 空信号 trace：frames/janks/stalls 全空 + meta.summary 解释 → 验证器视为"无可报告"非失败
10. 严禁 emit Layer-4 字段（schema 拒绝）
11. 一次 dispatch 一次 emit

**Prohibitions**（4 条）：
- 不要把 trace 当 panic log 解析（这是时序 span 数据）
- 不用 grep / list_files / repo_map（未在 allowlist）
- 不要伪造 spans（每个 PerfJank 必须对应可观察的 B/E 对）
- 不要多次 emit_perf_trace（第二次替换第一次）

#### perf-segmentation-skill

**Goal**：单次 `emit_perf_segmentation` 切 ≤10 段。

**Workflow**（5 条）：
1. 读 trace（blob 时用 read_file 翻全文，分段需要全局坐标）
2. Top-down 走 trace，识别 6 类区段：
   - `frame_window`：单 B|...|H:RenderService:DoFrame ... E| 帧 envelope
   - `jank_region`：连续 jank 帧或 stall
   - `startup`：进程 / Activity 启动窗口
   - `thread_run`：长 CPU/IO 单线程占用
   - `context`：header / 元数据
   - `noise`：不相关
3. ≤10 段，禁重叠（按 byte_start 升序）
4. `hint` 字段 ≤80 字符，per-segment 标签（"cold-start ActivityTaskManager 1.2s"）
5. context / noise 段 Step B 跳过，仍保留供诊断

**Prohibitions**：
- 不调 `emit_perf_trace`（那是 Step B 工作）
- 不重叠段
- 不超 10 段（粒度过细就 coarsen）
- kind enum 外的值会被 schema 拒绝

### 4.9 与 LogTriage 的关系

| 维度 | log_triage | perf_triage |
|---|---|---|
| 触发条件 | `AttachedLog != ""` | `AttachedHitrace != ""` |
| BusContext 字段 | `AttachedLog string` | `AttachedHitrace string` |
| Mutable 槽 | `LogTriage() *LogBundle` | `PerfTrace() *PerfBundle` |
| 主要 emit 工具 | `emit_log_triage` | `emit_perf_trace` |
| 分段工具 | `emit_log_segmentation` | `emit_perf_segmentation` |
| Skill | `log-triage-skill` + `log-segmentation-skill` | `perf-triage-skill` + `perf-segmentation-skill` |
| 段 kind enum | stack / caused_by / header / context / trace / noise | frame_window / jank_region / startup / thread_run / context / noise |
| 两步阈值 | 32 KiB / coverage 0.3 / max 12 calls | 64 KiB / coverage 0.3 / max 12 calls |
| 合并函数 | `logtriage.MergeBundles` | `perftriage.MergePerfBundles` |
| Analyzer 消费 | Entities ∪ + RequiredFiles ∪ + IntentHint=root_cause | Entities ∪ + RequiredFiles ∪ + IntentHint=performance |

两通道**结构同形态**但**数据正交**：log 是 chronological text 含 stack frames + cause chains，perf 是 timestamp-indexed span events。所以 segmentation kind 不复用，merge 算法不复用，但**控制流形态完全一致**（preStage Guard → BaseAgent + Evaluator → emit_* tool → derive Layer 4 → Mutable.Set*Trace → analyzer consume）。

---

## 5. 测试运行器集成（hvigor / cjpm）

`internal/tool/run_tests.go`

**Detector 优先级**（前几条已加 HarmonyOS / Cangjie 优先）：

```
go.mod                       → go
oh-package.json5             → hvigor
build-profile.json5          → hvigor
hvigorfile.ts                → hvigor
cjpm.toml                    → cjpm
package.json                 → node
pyproject.toml / pytest.ini  → python
Cargo.toml                   → rust
pom.xml                      → java
build.gradle / *.gradle.kts  → java (Gradle 路径同 Java，Kotlin 项目走这里)
Gemfile                      → ruby
CMakeLists.txt               → cmake
meson.build                  → meson
Makefile                     → make
```

**hvigor 命令构造**：
- 检测 `hvigorw` 在 repo 根 → 用本地 wrapper；否则降级到 PATH `hvigor`
- 命令：`<wrapper> --no-daemon test [--tests <filter>]`
- 输出格式：JUnit XML（DevEco Testing 基于 JUnit5），输出目录与 Gradle 同 → 复用 `parseJUnitXMLDir`
- Build 失败检测：`locateJUnitReportDir` 返回空 → `synthesizeBuildFailureReport(..., "hvigor-build", "hvigor", output)`，stdout 抽错误行进 ChangeReport.FailureDetail

**cjpm 命令构造**：
- 命令：`cjpm test [--filter <selector>]`
- 输出格式：cargo 风格文本（`test result: ok. X passed; Y failed`）→ 复用 `parseCargoTestText`
- 不假设 `--json` 标志（官方 1.0.0 LTS 未文档化）

**Parser 路由**（`run_tests_parsers.go::parseRunnerOutput`）：

```go
case "hvigor":
    return parseJUnitXMLDir(extraFile, stdout)  // 直接复用 Java 路径
case "cjpm":
    return parseCargoTestText(stdout)            // 直接复用 Cargo 路径
```

零新 parser，零新依赖。

---

## 6. 写模式 Skill Prompt

`internal/skill/defaults.go::code-write-skill::Workflow`

新增 11+10+10 条三语言风格 checklist 作为第一条 workflow 项。LLM 在 apply 阶段对照 ChangePlan.target_paths 文件扩展名选择对应风格集。

**ArkTS 严格模式 11 条**：
- 禁 `any` / `unknown` / `as` 强制类型转换 / 索引签名 / `Function` 类型
- import 必须含扩展名 (`.ets` / `.ts`)
- class field 必须显式类型
- 装饰器仅用 21 白名单
- struct 是 ArkUI 组件载体，不能用 `new`
- build() 返回必须单根
- 不用过期 `@ohos.commonEvent`

**Cangjie 风格 10 条**：
- `func` 而非 `function`
- 首行 `package xxx.yyy` 必含
- `public` / `open` 显式标记 export
- `extend` 是类型扩展，非 trait impl
- `match` 而非 cascaded if/else
- trait-like `interface` 非 Java interface
- operator overload 用 `operator func`
- FFI 用 `foreign func` + `@CJOH`
- `var` 可变 / `let` 不可变
- `@CallingThread` 主线程检查

**Kotlin 风格 10 条**：
- 默认 public，跨模块用 `internal` / `private`
- 优先 `val` over `var`
- `data class` value carriers
- `sealed class` closed hierarchies
- `suspend fun` 不要包阻塞 IO
- 优先扩展函数 over utility class
- `when` over switch
- Activity/Fragment 引用包在 lifecycle-aware scope

---

## 7. 多文件附加 + 字节上限

### 7.1 CLI 多文件

`cmd/root.go`

```go
flagAttachLog       []string  // StringArrayVar
flagAttachLogText   string

flagAttachHitrace     []string  // StringArrayVar
flagAttachHitraceText string
flagAttachAtrace      []string  // alias of --htrace
flagAttachAtraceText  string
```

`loadMultiPathSlice(kind, paths, inlineText, cap)` 读每个 path（或 inline），按 CLI 顺序拼接，文件之间插入 `# codrax-source: <path>\n` 头。

**stdin 全局唯一**：`enforceStdinExclusivity()` 计数 `len(slice == "-")` 跨 log + htrace + atrace 三组，>1 报错 "only one of --log/--htrace/--atrace can consume stdin per run"。

**互斥矩阵**：
- `--log` ⊕ `--log-text`
- `--htrace` ⊕ `--htrace-text`
- `--atrace` ⊕ `--atrace-text`
- (`--htrace` ∨ `--htrace-text`) ⊕ (`--atrace` ∨ `--atrace-text`) — 别名整体互斥（避免歧义优先级）

### 7.2 REPL 多文件

`internal/repl/repl.go`

```
/log <path>          替换
/log append <path>   追加（同样的 # codrax-source: 头）
/log show
/log clear
/htrace <path>       替换
/htrace append <path>
/htrace show
/htrace clear
/atrace ...          完全等价 /htrace（NormalizeREPLCommandAlias 归一化）
```

`replCommandAliases["/atrace"] = "/htrace"`（含 `\atrace`），`handleSlash` 只 case `"/htrace"`，drift-guard 测试同时验证 dispatch + autocomplete 一致。

### 7.3 字节上限

**两通道独立 cap**：

```go
maxAttachedLogBytes   = defaultAttachedLogMaxBytes  // 50 MiB
maxAttachedTraceBytes = defaultAttachedLogMaxBytes  // 默认继承 log
```

**继承逻辑**（`cmd/root.go::initApp`）：

```go
if rs.LogAttachMaxBytes != nil && *rs.LogAttachMaxBytes > 0:
    maxAttachedLogBytes = clamp(*rs.LogAttachMaxBytes, ..., maxAttachedLogHardCeiling)
maxAttachedTraceBytes = maxAttachedLogBytes  // 默认对称
if rs.TraceAttachMaxBytes != nil && *rs.TraceAttachMaxBytes > 0:
    maxAttachedTraceBytes = clamp(*rs.TraceAttachMaxBytes, ..., maxAttachedLogHardCeiling)
```

显式 0 / 负数 / 未设 → 走默认（防止意外把 cap 调成 0 后所有 attach 静默失效）。

**1 GiB 硬顶**（`maxAttachedLogHardCeiling`）：超过则 WARN clamp 到 1 GiB，防 OOM。

**REPL 同形态**：`Config.AttachedTraceMaxBytes` 0 → 继承 `AttachedLogMaxBytes`（先解析 log，再 fallback trace）。

### 7.4 字节计入点

`truncateAttachedToCap(s string, cap int, kind string)` 是单一入口。`kind` 字符串区分 WARN 消息：
- `[cmd] attached log truncated: N → M bytes`
- `[cmd] attached trace truncated: N → M bytes`

stdin 读取用 `io.LimitReader(os.Stdin, int64(cap)+1)` 确保进程内存上限严格。

---

## 8. 配置 Knob 总览

所有 knob 都是 `*T` pointer 字段（保持"absent vs explicit zero"语义），全部 yaml 可调，全部走既有 RuntimeSettings 机制，无新加载代码。

### Attach（接入侧，triage 之前生效）

| YAML 键 | 默认 | 作用 |
|---|---|---|
| `log_attach_max_bytes` | 52428800 (50 MiB) | 日志通道字节上限 |
| `trace_attach_max_bytes` | 0 → 继承 log | 性能 trace 通道字节上限 |

### Log Triage

| YAML 键 | 默认 | 作用 |
|---|---|---|
| `log_triage_enabled` | true | 总开关 |
| `log_triage_source_prefix` | "" | CI 绝对路径前缀剥离（`StripBuildPathPrefix` extra 列表）|
| `log_triage_min_bytes` | 50 | 触发下限 |
| `log_triage_max_retries` | 1 | 单次 emit 重试 |
| `log_triage_two_step_enabled` | true | 两步开关 |
| `log_triage_two_step_bytes` | 32768 (32 KiB) | 直转两步阈值 |
| `log_triage_two_step_coverage` | 0.3 | 升级两步的覆盖率门 |
| `log_triage_max_llm_calls` | 12 | 总 LLM 调用上限 |

### Perf Triage

| YAML 键 | 默认 | 作用 |
|---|---|---|
| `perf_triage_enabled` | true | 总开关 |
| `perf_triage_min_bytes` | 200 | 触发下限 |
| `perf_triage_max_retries` | 1 | 单次 emit 重试 |
| `perf_triage_two_step_enabled` | true | 两步开关 |
| `perf_triage_two_step_bytes` | 65536 (64 KiB) | 直转两步阈值 |
| `perf_triage_two_step_coverage` | 0.3 | 升级两步的覆盖率门 |
| `perf_triage_max_llm_calls` | 12 | 总 LLM 调用上限 |

### Repomap Fallback

| 常量（代码） | 值 | 作用 |
|---|---|---|
| `parseTier1Weight` / `parseTier2Weight` / `parseTier3Weight` / `parseTier4Weight` | 1.0 / 0.85 / 0.6 / 0.3 | rank score 折扣曲线 |
| `fallbackBannerThreshold[LangArkTS]` | 0.4 | Tier-2 占比超此值启动 banner |
| `fallbackBannerThreshold[LangCangjie]` | 0.5 | 同上（仓颉宽容） |

### Provider Routing

`providers.yaml :: agents.<name>` 对所有 agent 名都生效，包括新增的 `perf_triager`：

```yaml
llm:
  agents:
    perf_triager:
      model: "cheaper-model"  # HiTrace 解析结构简单，可路便宜模型
```

---

## 9. 红线（不变量）

代码里的红线均有结构性测试守护。任何改动违反红线 → CI 红。

| 红线 | 守护测试 |
|---|---|
| **L-ArkTS-2** `.ts → LangArkTS` 仅在 `IsArkTSProject` 探测到 `oh-package.json5` 时切换 | `TestApplyHarmonyOSPostProcess_PromotesTSInArkTSProject` |
| **L-ArkTS-3** ArkTS Tier 1 不放宽语法（严格 grammar reject 即 reject）| extract_arkts_test 的 NonArkTSFallback 场景 |
| **L-Cangjie-1** `.cjo` 编译产物 scanner 显式 deny | `TestApplyHarmonyOSPostProcess_CjoDeny` |
| **L-Cangjie-2** `FileInfo.Package` 必须从 `package_clause` 来 | `TestExtractCangjie_PackageMustComeFromClause` |
| **L-Fallback-1** 所有降级写 WARN log | `recordFallback` 内联，结构层 `parse_fallback_test.go` |
| **L-Fallback-2** Tier ≥ 2 文件 rank 强折扣 | `TestTierDiscount` 钉曲线 |
| **L-Fallback-3** 仓库级 Tier 2 占比超阈值启动 banner | `TestFallbackBannerThresholds` 钉值 |
| **L-Perf-1** PerfBundle Layer 4 字段（resolved_files / entities / intent_hint / coverage）schema 显式拒绝 LLM 写入 | `emit_perf_trace.go::buildEmitPerfTraceSchema` 不列；测试覆盖 |
| **L-Perf-2** 两步降级路径全部 advisory（StageOutput.Error 为空，主流水线不阻塞） | `perf_triager.go::runTwoStep` 各失败分支 + log_triage 同形态 |
| **L-Perf-3** stdin 全局唯一消费者（log + htrace + atrace 三组中至多一个 `-`） | `TestEnforceStdinExclusivity` |
| **L-Multi-1** 多文件 attach 必含 `# codrax-source: <path>` 头分隔 | `TestLoadMultiPathSlice_HeaderedConcat` |
| **L-Cap-1** 用户配置的 cap 超 1 GiB 硬顶则 clamp + WARN | `TestHardCeilingClamp` |
| **L-Drift-1** REPL 命令 alias 注册 ↔ handleSlash dispatch ↔ slashCommands autocomplete 三处一致 | `TestHandleSlashDispatchMatchesRegistry` + `TestSlashCommandsMatchCanonicalRegistry` |

---

## 附录 A — 代码索引

- 主体语言抽取：`internal/tool/repomap/index/extract_arkts.go` / `cangjie_lexer.go` + `cangjie_parser.go` / `extract_kotlin.go`
- Resolver：`internal/tool/repomap/index/resolver_arkts.go` / `resolver_cangjie.go` / `resolver_kotlin.go`
- 嵌入式解析器：`internal/tool/repomap/index/json5_parser.go` / `toml_parser.go`
- Fallback 框架：`internal/tool/repomap/index/parse_fallback.go`
- Log triage 扩展：`internal/analysis/logtriage/resolver_harmonyos.go`（ArkTS/Cangjie/Kotlin Resolve 全在此）
- Perf 通道核心：`internal/types/perf_bundle.go` + `internal/tool/emit_perf_trace.go` + `internal/tool/emit_perf_segmentation.go` + `internal/agent/perf_triager.go` + `internal/analysis/perftriage/merge.go`
- Skill：`internal/skill/defaults.go`（perf-triage-skill / perf-segmentation-skill / log-triage-skill 教学 / code-write-skill ArkTS+Cangjie+Kotlin checklist）
- Pipeline 接入：`internal/orchestrator/topology.go::preStages` + `internal/types/enums.go` 的 `StagePerfTriage` / `AgentPerfTriager`
- CLI / REPL：`cmd/root.go` 多文件 attach + cap inheritance + alias merge；`internal/repl/repl.go` `/htrace` + `/atrace` 处理；`internal/types/conversation.go` 命令 alias 表
- Verifier runner：`internal/tool/run_tests.go`（hvigor / cjpm 探测 + 命令构造）+ `run_tests_parsers.go`（路由复用既有 parser）

---

## 附录 B — 用户面命令速查

```bash
# 基础读模式
codrax --request "...question..."

# 单日志附加
codrax --log /tmp/panic.txt --request "..."

# 多日志附加（panic 对比）
codrax --log a.log --log b.log --request "对比两次崩溃"

# stdin 日志（kubectl / docker）
kubectl logs pod/api | codrax --log - --request "..."

# 单 trace 附加
codrax --htrace /tmp/perf.atrace --request "where is the jank?"
codrax --atrace /tmp/perf.atrace --request "..."  # 别名

# 多 trace 附加
codrax --htrace boot1.trace --htrace boot2.trace --request "对比冷启动"

# 双通道（panic + perf）
codrax --log /tmp/hilog.txt --htrace /tmp/jank.atrace --request "卡顿后 crash 的根因"

# 写模式（HarmonyOS ArkTS 工程）
codrax --mode=plan --request "把这个页面改成响应式布局"
codrax --mode=apply --plan-file=.codrax/plans/<id>.json --auto-apply
```

REPL 同等命令通过 `/log` / `/htrace` / `/atrace` 实现，支持 `append` 子命令多文件累加。
