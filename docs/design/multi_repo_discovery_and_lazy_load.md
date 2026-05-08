# Multi-Repo Discovery + Lazy Load (B+C 混合方案)

> **Baseline**: commit `b558c66` (`feat: dump final read-mode answers to .codrax/output/<ts-pid>.md`)
> **Status**: DESIGN v2 — 商用级实施计划,含 raw consumer audit。实施前请用 §1 速查表逐条 grep 校验行号未漂移。
> **Audience**: 陌生开发者(任意背景)应能直接读完落地,不要求事先了解 codrax 架构。
> **跨语言一等公民**: ArkTS / Cangjie / Java / Kotlin / Go / Python / JS/TS / Rust / C/C++ / Swift / Ruby / Lua / Proto **同父目录共存**是核心用例,不是 edge case。
> **v2 改动**: line drift 修;Slug 格式对齐 CacheDir 同源;`MainRepoRoot` 双 site 显式;raw consumer audit (§11) 与 phase-by-phase migration (§12) 落进 doc;cap thrashing 升级 fail-loud;REPL 3 注册点;R2' 6 处展开;Q1 改为 **MultiGraph carrier** Z+Y 混合(纯 Z 半成品)。

## 0. 目标 & 非目标

### 0.1 目标

用户从父目录(自身可能是 git 仓也可能不是)运行 `codrax --repo .`,父目录下有 N 个独立 git 仓(深度未知,语言未知,可异构),codrax 必须:

1. **自动发现** 全部子仓 + 每个子仓的 owning language 集合(无需用户配置)
2. **per-repo 隔离** 索引(不同子仓的同名 symbol/package 不再 collide,typed lane 永不静默错配)
3. **跨仓 typed-lane 查询**(`SymbolOracle.SymbolExists*` / `LookupSymbol`)聚合多子仓答案,但保留 owning-repo 元信息以便 LLM 引证
4. **内存可控**(默认 ≤ 3 个 active Graph 同时驻留,LRU evict)
5. **磁盘可控**(per-repo cache 独立,粒度内复用现有 `index.CacheDir` slug 机制)
6. **跨仓 ArkTS / Cangjie / runner 探测**全部止于 sub-repo 边界
7. **零回归**单仓用户:`multi_repo_enabled=false` (默认 **true**,但单仓走退化路径) → 行为字节级等价于今天

### 0.2 非目标

- 跨仓 import edge 解析(子仓间 Go module / Java pom / Cargo crate 是独立 namespace,跨仓 import 在语义上就是 unresolved,不予建模 — 见 §3.5)
- monorepo workspace 协议(`go.work` / `Cargo.toml [workspace]` / pnpm workspace / Gradle multi-project)— 这些是**单仓内的 multi-module**,不是 multi-repo,留待后续独立设计
- 写模式跨仓 plan/apply(write-mode 始终选定**单一** target sub-repo,plan/apply/verify 在该子仓的 worktree 内运行 — 见 §4.5.2)
- 跨仓 `RankGraph` / `subgraph.ComputeChains/Hubs/Bridges`:per sub-repo 计算 N 次。跨仓排名/链路无意义(评分尺度不可比)。

## 1. 速查表(防 line drift,实施前 grep 校验)

| 引用 | file:line | 校验命令 |
|---|---|---|
| `BusContext.RepoRoot` 字段 | `internal/types/context.go:3193` | `grep -n 'RepoRoot  string' internal/types/context.go` |
| `BusContext.MainRepoRoot` (主) | `internal/types/context.go:3290` | `grep -n 'MainRepoRoot string' internal/types/context.go` |
| `MainRepoRoot` 镜像 site | `internal/types/context.go:3438` | 同上(同 grep,两个命中) |
| `MutableState.SetRepoRoot` | `internal/types/context.go:770` | `grep -n 'func.*SetRepoRoot' internal/types/context.go` |
| `MutableState.exactContextRequiredFiles` | `internal/types/context.go:312` | `grep -n 'exactContextRequiredFiles' internal/types/context.go` |
| `flagRepo` 解析(单 root) | `cmd/root.go:1398` | `grep -n 'filepath.Abs(flagRepo)' cmd/root.go` |
| `ScanFiles` 入口 | `internal/tool/repomap/index/scanner.go:70` | `grep -n 'func ScanFiles' internal/tool/repomap/index/scanner.go` |
| `BuildGraph` 入口 | `internal/tool/repomap/index/build.go:17` | `grep -n 'func BuildGraph' internal/tool/repomap/index/build.go` |
| `BuildOrLoadGraph` 5 caller | `internal/agent/analyzer.go:342,1672,1771` `keyword_search.go:667` `sub_explorer.go:366` | `grep -rn 'BuildOrLoadGraph' internal/agent/` |
| `*types.Graph` 定义 | `internal/tool/repomap/types/types.go:389-402` | `grep -n 'type Graph struct' internal/tool/repomap/types/types.go` |
| `SymbolOracle` interface | `internal/types/symbol_oracle.go:53` | `grep -n 'type SymbolOracle' internal/types/symbol_oracle.go` |
| `graphOracle` impl | `internal/tool/repomap/oracle.go:24` | `grep -n 'type graphOracle' internal/tool/repomap/oracle.go` |
| `IsArkTSProject` ancestor walk | `internal/tool/repomap/types/lang.go:279` | `grep -n 'func IsArkTSProject' internal/tool/repomap/types/lang.go` |
| ArkTS scanner 调用 | `internal/tool/repomap/index/scanner.go:106` | `grep -n 'IsArkTSProject' internal/tool/repomap/index/scanner.go` |
| Go resolver shallowest-only | `internal/tool/repomap/index/resolver_go.go:37` | `grep -n 'shallowest go.mod' internal/tool/repomap/index/resolver_go.go` |
| Rust crate longest-prefix | `internal/tool/repomap/index/resolver_rust.go:131` | `grep -n 'fileCrate' internal/tool/repomap/index/resolver_rust.go` |
| `cacheSchemaVersion` | `internal/tool/repomap/index/cache.go:89` | `grep -n 'cacheSchemaVersion =' internal/tool/repomap/index/cache.go` |
| `extractorVersions` | `internal/tool/repomap/index/cache.go:99` | `grep -n 'extractorVersions =' internal/tool/repomap/index/cache.go` |
| `CacheDir` slug `<basename>-<8hex>` | `internal/tool/repomap/index/cache.go:51-74` | `grep -n 'func CacheDir' internal/tool/repomap/index/cache.go` |
| `RuntimeSettings` struct | `internal/config/runtime.go:34+` | `grep -n 'type RuntimeSettings' internal/config/runtime.go` |
| Worktree 启动 prune | `cmd/root.go:1424,1446` | `grep -n 'PruneDeadSessions\|InstallSignalHandler' cmd/root.go` |
| REPL `slashCommands` 列表 | `internal/repl/input.go:86` | `grep -n 'slashCommands' internal/repl/input.go` |
| REPL `handleSlash` switch | `internal/repl/repl.go:1972` | `grep -n 'func .*handleSlash' internal/repl/repl.go` |
| REPL `NormalizeREPLCommandAlias` | `internal/repl/repl.go` (TODO confirm line) | `grep -rn 'NormalizeREPLCommandAlias' internal/repl/` |
| LogTriage frame validate | `internal/analysis/logtriage/validate.go:81` | `grep -n 'validateFrame' internal/analysis/logtriage/validate.go` |
| `detectRunnerPlans` walk | `internal/tool/run_tests.go:677-738` | `grep -n 'detectRunnerPlans' internal/tool/run_tests.go` |
| `tool.NewGitCommand` helper | `internal/tool/repomap/index/scanner.go:115` | `grep -rn 'tool.NewGitCommand' internal/tool/` |
| `tool.IsExcludedDirName` 剪枝 | (multi-call site) | `grep -rn 'IsExcludedDirName' internal/tool/` |

> **保险**:任何上面引用 line drift > 5 行,实施前先 sync 这份 doc。

## 2. 现状缺陷(已审计,见附录 A 跨语言矩阵)

**HIGH 风险**(静默错配):

- **ArkTS leak** (`types/lang.go:279-302`):scanner 在 walk 父目录时,某子仓的 `oh-package.json5` 会被另一子仓的 `.ts` 文件 ancestor-walk 命中,LangTypeScript 被错误提升为 LangArkTS,后续 ArkTS resolver 试图 bundle resolution 拿到 garbage。

**MEDIUM-HIGH**:

- **`detectRunnerPlans`** (`run_tests.go:677-738`) `filepath.Walk(repoRoot)` 全树找 manifest,选 priority 最高的 — 多仓父目录下选错子仓的 build 系统。
- **Rust crate 名 collision**(两子仓各有 `Cargo.toml::name="utils"`)→ resolver 第一发现胜出,unresolved 但不错配。

**MEDIUM**:

- **JS/TS 多 tsconfig alias merge** — 不致命(longest-prefix 排序),但 LLM 可能拿到非预期 alias 路径。

**LOW(safe by design)**:

- Go(取最浅 go.mod,文档化为 Phase 2a 限制)、Java/Kotlin/Cangjie(package_clause AST,严格 namespace)、Python(path 推断,跨子仓自然分歧)、C/C++/Swift/Ruby/Lua/Proto(纯路径索引)。

**通用代价**(任何语言):

- 单 Graph 的 `SymbolDefs` first-wins 丢失重名定义
- `populateImplementers` O(N_iface × N_concrete) 已按 language 分组,多仓后 N 都涨,GB 量级 + scan 时间倍增
- cache 原子失效:任一子仓任一 extractor version bump → 全父树重扫(`cache.go:96` 注释承认 per-file invalidation 未实现)

## 3. 总体架构

### 3.1 组件分层

```
┌────────────────────────────────────────────────────────────────┐
│  cmd/root.go::Execute                                          │
│  └─ resolveRepoTopology(flagRepo) → *RepoTopology              │  Phase 1
└────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌────────────────────────────────────────────────────────────────┐
│  internal/tool/repomap/                                        │
│  ├─ topology/                  (NEW package)                   │  Phase 1+2
│  │   ├─ discover.go            ─ enumerate sub-repos           │
│  │   ├─ topology.go            ─ RepoTopology / SubRepo struct │
│  │   └─ manifest.go            ─ per-repo manifest fingerprint │
│  │                                                             │
│  ├─ multigraph/                (NEW package)                   │  Phase 3+4
│  │   ├─ multigraph.go          ─ MultiGraph carrier (Z+Y)      │
│  │   ├─ oracle.go              ─ multiGraphOracle (Z fan-out)  │
│  │   ├─ locator.go             ─ multiGraphLocator (Z fan-out) │
│  │   └─ lru.go                 ─ container/list-based LRU      │
│  │                                                             │
│  ├─ index/scanner.go           ─ 改造:接受 SubRepo 而非 RepoRoot │  Phase 5
│  ├─ types/lang.go              ─ IsArkTSProject 接受 subRepo   │  Phase 0
│  └─ facade.go                  ─ 新增 BuildOrLoadMultiGraph     │  Phase 4
└────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌────────────────────────────────────────────────────────────────┐
│  internal/types/context.go                                     │
│  ├─ BusContext.RepoRoot        ─ 保留,语义为"父根"(未变)        │  Phase 4
│  ├─ BusContext.SubRepos        ─ 新增 []SubRepo (snapshot)     │
│  ├─ BusContext.ActiveSubRepo   ─ 新增 *SubRepo (write-mode)    │
│  ├─ BusContext.PendingSubRepos ─ 新增 []string (cap 裁切名单)  │
│  ├─ BusContext.MainRepoRoot    ─ 双 site 同步 (3290 + 3438)    │
│  └─ BusContext.SetActiveSubRepo                                │
└────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌────────────────────────────────────────────────────────────────┐
│  internal/agent/* + analysis/*                                 │
│  ├─ analyzer.go                ─ 5 caller 改 *MultiGraph       │  Phase 4
│  ├─ keyword_search.go          ─ 跨子仓 grep + per-hit attribute│
│  ├─ sub_explorer.go            ─ 选定 sub-repo 后 EnsureLoaded │
│  ├─ explorer.go                ─ raw consumer 47 处 Y/Z 归类改 │  Phase 4
│  ├─ ground.go                  ─ raw consumer 15 处 Y/Z 归类改 │  Phase 4
│  ├─ subgraph.go                ─ per-subrepo 重算(Y 退化)     │  Phase 4
│  ├─ rank.go                    ─ per-subrepo 重算(Y 退化)     │  Phase 4
│  ├─ render.go                  ─ aggregate 视图(Y/退化)       │  Phase 4
│  └─ logtriage/validate.go      ─ frame.File → owning sub-repo │  Phase 4
└────────────────────────────────────────────────────────────────┘
```

### 3.2 关键数据结构

```go
// internal/tool/repomap/topology/topology.go (NEW)

// SubRepo 描述一个被 codrax 识别的子仓库。
// 来源:Phase 1 discovery walk 一次,持久化到 manifest cache。
type SubRepo struct {
    // Slug 复用 index.CacheDir 现行格式 "<basename>-<8hex>"
    // (例 "codrax-1a2b3c4d")。同源,避免双映射。
    Slug string `json:"slug"`

    // RootAbs 子仓的绝对路径(已 EvalSymlinks 规整)。
    RootAbs string `json:"root_abs"`

    // RootRel 相对父根的路径,用于面板展示和 LLM 引证。
    // 退化情况(父根=子根):RootRel = ".".
    RootRel string `json:"root_rel"`

    // GitMode 是 ".git/" 的形态:"dir"(独立仓)、"file"(submodule/worktree)、
    // ""(无 .git,目录被探测器接纳的特殊情况 — §3.3.2)。
    GitMode string `json:"git_mode"`

    // PrimaryLangs 是该子仓主要语言(按文件数 top-3),
    // discovery 阶段 Tier 1 轻量探测得到 — 不需要完整 BuildGraph。
    PrimaryLangs []string `json:"primary_langs"`

    // PrimaryLangsTier 1=Tier1(扩展名+special-file 启发);
    // 2=Tier2(EnsureLoaded 后 graph.Metadata 校准过)。
    PrimaryLangsTier int `json:"primary_langs_tier"`

    // ManifestFingerprint 是 special files 的 hash,用于检测
    // 子仓拓扑变更而触发 EnsureLoaded re-build。
    ManifestFingerprint string `json:"manifest_fp,omitempty"`

    // FileCount 是粗略文件数(用 git ls-files | wc 得到,O(1) cheap),
    // 用于 §3.4 active-set 的 "biggest first" 启发。
    FileCount int `json:"file_count,omitempty"`
}

// RepoTopology 是 discovery 一次扫描的产物,持久化到
// <runtime-anchor>/.codrax/cache/topology/<parent-slug>.json
type RepoTopology struct {
    ParentRoot   string    `json:"parent_root"` // == flagRepo abs
    ParentSlug   string    `json:"parent_slug"`
    Repos        []SubRepo `json:"repos"`        // sorted by RootRel
    DiscoveredAt time.Time `json:"discovered_at"`
}

// Lookup 用 RelPath(从父根起算)定位 owning sub-repo。
// 返回 nil 表示该路径不属于任何已发现子仓。
func (t *RepoTopology) Lookup(relPathFromParent string) *SubRepo
```

```go
// internal/tool/repomap/multigraph/multigraph.go (NEW)
//
// MultiGraph 是多子仓的统一查询面 + raw 字段聚合面。
// 既实现 Z 类(SymbolOracle/SymbolLocator fan-out),又暴露 Y 类
// (AllGraphs/GraphFor/Files/FileIndex/ImportEdges/...) 给原 raw consumer。
//
// 单仓场景:active 只 hold 1 个 *Graph,所有方法字节级等价于
// 直接操作那个 *Graph(零回归保障)。
type MultiGraph struct {
    topo *topology.RepoTopology

    // active LRU,key=Slug,value=*repomap.Graph。default cap 3。
    // EnsureLoaded(slug) 按需加载;eviction 仅丢内存,
    // per-repo cache 仍在磁盘,re-load 极廉。
    active *lru.LRU

    cap            int    // 来自 codrax.yaml::multi_repo_max_active
    discoveryQuery string // 创建时的 query 字符串,EnsureLoaded 时透传

    mu sync.Mutex // 保护 active(BuildOrLoadGraph 本身已 thread-safe)
}

// === 加载控制 ===
//
// EnsureLoaded 同步加载指定 slug 的 Graph(必要时触发 BuildOrLoad
// + LRU evict)。返回的 *Graph 在 evict 之前都安全。
func (m *MultiGraph) EnsureLoaded(slug string) (*types.Graph, error)

// EnsureLoadedFor(relPath) 是常用 wrapper:从父根起算的 RelPath
// → topology.Lookup → EnsureLoaded。
func (m *MultiGraph) EnsureLoadedFor(relPathFromParent string) (*types.Graph, *topology.SubRepo, error)

// EnsureMany(slugs) 批量加载。超过 cap fail-loud 兜底
// (正常路径 routing fold 已 pre-trim,这是 R3 红线兜底)。
func (m *MultiGraph) EnsureMany(slugs []string) error

// === Z 类:接口层 fan-out (审计 23 sites 走这里) ===

// Oracle 返回 multiGraphOracle,跨 active sub-repo fan-out
// 实现 types.SymbolOracle 全部方法。
func (m *MultiGraph) Oracle() types.SymbolOracle

// Locator 同上(types.SymbolLocator)。
func (m *MultiGraph) Locator() types.SymbolLocator

// === Y 类:raw 字段访问 (审计 58 sites 走这里) ===

// AllGraphs 返回 active sub-repo 的 graphs(slug → *Graph)。
// rank.go / subgraph.go 等 per-repo 计算遍历它,N 次独立 RankGraph/Compute*。
func (m *MultiGraph) AllGraphs() map[string]*types.Graph

// GraphFor 用 relPath(从父根起算)定位 owning *Graph。
// 用于"某 file 属于哪个 sub-repo 的 graph"场景。
// 若 owning sub-repo 未 active,返回 (nil, subRepo, false) — 调用方
// 决定是否 EnsureLoaded(避免隐式触发加载)。
func (m *MultiGraph) GraphFor(relPathFromParent string) (*types.Graph, *topology.SubRepo, bool)

// Files 返回所有 active sub-repo 的 *FileInfo flatten 视图。
// RelPath 是"父根起算的 RelPath"(子仓内 RelPath 加 sub-repo prefix)。
func (m *MultiGraph) Files() []*types.FileInfo

// FileInfoFor(relPathFromParent) 是 GraphFor + per-graph FileIndex lookup
// 一气呵成的便捷方法。
func (m *MultiGraph) FileInfoFor(relPathFromParent string) (*types.FileInfo, *topology.SubRepo, bool)

// ImportEdges / ReverseImportEdges 返回 flatten 视图。RelPath 加 sub-repo prefix
// 以便跨仓不撞名。Cross-repo edge 不存在(§3.5)。
func (m *MultiGraph) ImportEdges() map[string][]string
func (m *MultiGraph) ReverseImportEdges() map[string][]string

// ScoreFor / QueryScoreFor 用 owning sub-repo 的 score 表查询。
func (m *MultiGraph) ScoreFor(relPathFromParent string) float64
func (m *MultiGraph) QueryScoreFor(relPathFromParent string) float64

// === 退化类:repo-level invariant ===

// Root 返回父根(parent root,等于 flagRepo abs)。
func (m *MultiGraph) Root() string

// Metadata 跨 sub-repo aggregate(总 file count、Languages 合并、SpecialFiles 合并)。
func (m *MultiGraph) Metadata() types.Metadata

// === 单仓退化保障 ===

// IsSingle 表示 active 只有 1 个 graph(单仓退化)。
// 在此模式下,所有方法行为等价于直接操作 active[0]。
func (m *MultiGraph) IsSingle() bool

// Single 返回单仓退化模式下的唯一 *Graph(IsSingle()==true 时)。
// 用于"明确不需要多仓语义"的 legacy code path 兼容。
func (m *MultiGraph) Single() *types.Graph
```

**单仓字节级等价保障**:`IsSingle()==true` 时,`AllGraphs()` 返回 size=1 map,`Files()/ImportEdges()/...` 直接转发底层 `*Graph` 同名字段,**新代码与旧代码语义一致**。

### 3.3 Discovery(Phase 1)

#### 3.3.1 算法

```
INPUT: parentAbs (cmd/root.go:1398 已 EvalSymlinks 规整后的绝对路径)
OUTPUT: *RepoTopology

1. 探测 parentAbs/.git → 若存在(dir 或 file):
     单仓退化:输出 RepoTopology{ Repos: [SubRepo{ Slug=cacheDirSlug(parentAbs),
       RootAbs=parentAbs, RootRel=".", GitMode="dir"|"file" }] }
     立即返回(无需深 walk,行为与今天一致)
     额外开销:1 次 os.Stat + 1 次 sha256 = ~50µs

2. parentAbs 自身非 git 仓:启动 BFS walk
     - 限制:depth ≤ multi_repo_discovery_depth (default 4)
     - 排除:tool.IsExcludedDirName(part) 全部跳过(node_modules/.git/.codrax 等)
     - 每个 dir 看子项里有没有 ".git":
         发现 → 创建 SubRepo,**剪枝**:不再深入该 SubRepo 内部(避免吃下子仓的 nested submodule)
         未发现 → 继续 BFS

3. 每个 SubRepo:
     - Slug = cacheDirSlug(RootAbs)         # 复用 index.CacheDir 同源逻辑
     - PrimaryLangs:用 tool.NewGitCommand(nil, "-C", RootAbs, "ls-files", ...) ms 级
                    扩展名 top-3 + special-file 修正 (oh-package.json5/cjpm.toml/build.gradle/Cargo.toml)
                    PrimaryLangsTier = 1
     - FileCount:wc -l of git ls-files
     - ManifestFingerprint:hash 该子仓 root 下 special files 列表

4. min_files 过滤(default 1):FileCount < N 的 SubRepo 弃掉

5. 按 RootRel 字典序排序 → 持久化到 <runtime-anchor>/cache/topology/<parent-slug>.json
```

**Slug 同源**:`cacheDirSlug(abs)` 提到 `internal/tool/repomap/index/cache.go` 公开为 helper(目前内嵌在 `CacheDir` 里),Phase 1 顺手抽取。格式 `filepath.Base(abs) + "-" + hex.EncodeToString(sha256Sum256[:4])`,8 hex chars 后缀,含可读 basename 前缀。

#### 3.3.2 边界情况

- **嵌套子仓**(子仓里又有子仓):剪枝策略下被忽略。用户应 cd 到内层后重跑(显式选择)。
- **Submodule(`.git` 是 file)**:GitMode="file",当作独立 SubRepo 处理(submodule 在内容上确实独立)。
- **codrax 自家的 worktrees**(`<runtime-anchor>/worktrees/<id>/.git`):必须排除。检测路径前缀是 `runtimeAnchor + /worktrees/`(`cmd/root.go:1424` 附近)→ skip。
- **完全无 git 仓**(全是普通目录):RepoTopology.Repos=[]。codrax 报错 "no git repository found under <parent>",fail-loud。

#### 3.3.3 Discovery cache 失效

manifest cache 在父目录路径不变的情况下复用。失效条件:
- topology.json 不存在
- 父目录最后修改时间 > topology.DiscoveredAt(便宜路径,有限信任)
- 任一 SubRepo.RootAbs 已不存在(子仓被删)
- 用户显式触发 `/repos refresh`(REPL,§4.4)

### 3.4 Active-set 与 LRU(Phase 3)

```
ACTIVE 上限 cap = codrax.yaml::multi_repo_max_active (default 3)

EnsureLoaded(slug) 流程:
  if slug in active:
     LRU.MarkRecent(slug); return graph
  if len(active) >= cap:
     evicted = LRU.Pop()    ← 仅释放内存,磁盘 cache 保留
     telemetry.Log("multigraph: evicted slug=%s reason=lru_cap", evicted)
  graph := repomap.BuildOrLoadGraph(subrepo.RootAbs, query)
  active.Put(slug, graph); return graph

ENSURE-MANY(slugs):
  if len(slugs) > cap:
     return ErrTooManyActive        ← R3 红线兜底,正常路径 routing fold 已 pre-trim
  for each slug: EnsureLoaded
```

**EnsureMany cap 兜底**:正常路径 §3.5 routing fold 后送进来必然 ≤ cap。`ErrTooManyActive` 是 defense-in-depth — 万一 caller 跳过 routing 直接喂 slug 列表,fail-loud 暴露而非静默裁剪。

**为什么 fail-loud 拒绝超 cap?** R3 红线:静默少加载会让 typed lane 在用户不知情的情况下漏命中,产生"系统正确但答案不全"的不可诊断 bug。

**Eviction 副作用**:被 evict 的 *Graph 失去强引用,GC 回收。已发出的 *Graph 引用仍可用(调用方持有),但 MultiGraph 不再返回该 slug 的旧引用 — re-EnsureLoaded 会重新 Build/Load(命中磁盘 cache 时 ms 级)。

**Thrashing 检测(fail-loud,新)**:
```
recentEvicts := []time.Time{}     // sliding window 60s
on each evict:
   recentEvicts append now()
   trim recentEvicts older than 60s
   if len(recentEvicts) > 5:
      return ErrThrashingDetected(
         "multigraph: %d evictions in 60s — increase multi_repo_max_active",
         len(recentEvicts))
```
fail-loud 提示用户调高 cap 而不是默默 thrash。

### 3.5 Routing:哪些 sub-repo 应该 active?

**多通道融合,每通道产生 candidate slug 集,合并后由 cap 裁剪。**

| 通道 | 信号来源 | 类型 | 备注 |
|---|---|---|---|
| **A. 用户显式** | REPL `/repos focus <slug>` | 必须(precise) | UX 兜底 |
| **B. AnalyzerHints + RequiredFiles 路径** | analyzer 已 emit `MutableState.exactContextRequiredFiles` → topology.Lookup | 高(precise) | 主路径 |
| **C. AttachedLog frames** | logtriage `validateFrame` 解析 frame.File → topology.Lookup | 高(precise) | log/perf-triage 路径 |
| **D. Query keyword 与 SubRepo.PrimaryLangs 匹配** | "Go-only question" 命中 PrimaryLangs⊇{Go} 的 sub-repo | 中(noisy) | rank-only,不挡硬正确性 |
| **E. SubRepo.FileCount 降序** | 没有任何上述信号时,加载最大子仓做兜底 | 低(noisy) | 退化场景 |

**精确 vs 噪声边界(R3 红线对齐)**:通道 A/B/C 是 **precise** 信号(显式路径或 typed enum),通道 D/E 是 **noisy** rank 信号。**通道 D/E 决定 candidate rank,但不挡硬正确性** — 漏掉的 sub-repo 由 `partial_typed_lane=true` precise enum boolean fail-loud 暴露(LLM prompt 里显式承认"以下子仓未参与本次 typed-lane 查询")。

**流程**:analyzer Stage 启动时 fold 全部信号 → 候选集,按通道权重排序 → 取 top-cap slug → MultiGraph.EnsureMany。超 cap 被裁掉的 slug 在 `BusContext.PendingSubRepos` 列出,LLM prompt 显式承认。

**跨仓 Symbol 查询**(`SymbolOracle.SymbolExistsFlat` 经 `multiGraphOracle`):

```
multiGraphOracle.SymbolExistsFlat(name):
   for each (slug, graph) in active:
       found, tier = graphOracle{graph}.SymbolExistsFlat(name)
       if found:
           accumulator.Add(slug, tier)
   return (accumulator.Any(), accumulator.MinTier())

# 若 Any() == false 但 topology 有 inactive sub-repo 未查:
# 不"为了一次查询就加载全部子仓"(违背 cap)
# 转而:summary 标 partial_typed_lane=true + 列出未查的 slug
```

**Cross-repo import edge:不解析。** 子仓间 Go module / Java pom / Cargo crate 是独立 namespace,跨仓 import 在语义上就是 unresolved。每子仓自己的 `ImportGraph` / `ReverseImports` 仍正确;`MultiGraph.ImportEdges()` flatten 时各子仓独立 namespace(RelPath 加 sub-repo prefix),不提供 cross-repo edge view(避免假阳性)。

## 4. Phase-by-Phase 实施计划(本 session 满载执行)

### 4.1 Phase 0(1 commit) — ArkTS leak 修复

**与 multi-repo 无关的硬 bug**,先单独修。

修 `internal/tool/repomap/types/lang.go::IsArkTSProject`:

```go
// 旧: func IsArkTSProject(repoRoot, relPath string) bool
// 新签名(参数语义改: subRepoRoot 为 file 所在 sub-repo 的 root)
func IsArkTSProject(subRepoRoot, relPath string) bool
```

调用点 `scanner.go:106` 改为传入"file 所在 sub-repo 的 root"。Phase 0 阶段还没有 multi-repo,临时传 `repoRoot`,语义等价于今天。Phase 5 multi-repo 改造完成后,自然得到正确边界。

测试 fixture:`testdata/multirepo/arkts_leak_fix/parent/sub-a/oh-package.json5` + `parent/sub-b/foo.ts`,断言 `sub-b/foo.ts` 不被提升为 LangArkTS(unit test 直接调 `IsArkTSProject(filepath.Join(parent, "sub-b"), "foo.ts")` 返回 false)。

### 4.2 Phase 1(2 commits) — Discovery + Topology cache

- **Commit 1**:`internal/tool/repomap/topology/{discover,topology,manifest}.go` + `index/cache.go::CacheDirSlug` helper 暴露(`return filepath.Base(abs) + "-" + hex.EncodeToString(h[:4])`) + 单元测试
- **Commit 2**:`cmd/root.go::Execute` 内调用 `resolveRepoTopology(flagRepo)`,结果存到 `app.topology`(即使 `multi_repo_enabled=false` 也跑,无副作用 INFO 日志一行)

**测试 fixture**:`testdata/multirepo/parent/{repo-a,repo-b,repo-c}/.git/HEAD` 三子仓,断言 Discovery 找到 3 个 SubRepo + slug 稳定 + Lookup(rel) 正确。

### 4.3 Phase 2(2 commits) — yaml schema + REPL 探测命令

- **Commit 1**:`internal/config/runtime.go` 加 4 个新字段(全 pointer-typed,沿用 `WorktreeKeepTTLHours` 模式):

```go
MultiRepoEnabled         *bool   `yaml:"multi_repo_enabled"`         // default TRUE  (§9 #2)
MultiRepoMaxActive       *int    `yaml:"multi_repo_max_active"`      // default 3
MultiRepoDiscoveryDepth  *int    `yaml:"multi_repo_discovery_depth"` // default 4    (§9 #1)
MultiRepoMinFiles        *int    `yaml:"multi_repo_min_files"`       // default 1
```

- **Commit 2**:REPL `/repos` 命令 — 必须 **3 处同步注册**:
  1. `internal/repl/repl.go::handleSlash` switch 加 `case "/repos": r.handleReposCmd(line)`
  2. `internal/repl/input.go::slashCommands` 加 autocomplete 条目
  3. `internal/repl/repl.go::NormalizeREPLCommandAlias`(若有 alias 需求)
  
  子命令:`/repos`(列拓扑+active)/ `/repos focus <slug>` / `/repos refresh` / `/repos cap <N>`
  
  REPL 命令在 `multi_repo_enabled=false` 时仍工作(展示 hint:"multi_repo_enabled=false; edit codrax.yaml + restart, or stay single-repo"。**不支持运行时 toggle** — §9.2 一致性成本)。

### 4.4 Phase 3(3 commits) — MultiGraph carrier + LRU + oracle/locator

- **Commit 1**:`multigraph/lru.go` — `container/list`-based LRU(无第三方依赖)+ 并发安全 + thrashing 计数 + 单元测试(eviction order / cap respect / thrashing fail-loud)
- **Commit 2**:`multigraph/multigraph.go` — `MultiGraph` carrier 全部方法(§3.2 列表)+ 单仓退化 wrapper + 单元测试(单仓字节级等价 + 多仓 fan-out)
- **Commit 3**:`multigraph/oracle.go::multiGraphOracle` 实现 `types.SymbolOracle`(聚合 active graphs + `partial_typed_lane` precise gate)+ `multigraph/locator.go::multiGraphLocator` 实现 `types.SymbolLocator` + 单元测试

**关键不变量**:`multi_repo_enabled=false` 时 MultiGraph 退化为 size=1 wrapper,所有方法行为字节级等价于今天的 `graphOracle{graph}` / 直接 `*Graph` 字段访问。

### 4.5 Phase 4(4-5 commits) — BusContext + 5 caller migration + raw consumer 改造

#### 4.5.1 BusContext 改造(Commit 1)

`internal/types/context.go`:
- 加 `SubRepos []topology.SubRepo`(snapshot,不可变)
- 加 `ActiveSubRepo *topology.SubRepo`(write-mode 选定的单一目标)
- 加 `PendingSubRepos []string`(cap 裁切后未参与的 slug,LLM prompt 用 RootRel 暴露,**不暴露 slug**)
- `MainRepoRoot` 双 site(3290 主 site + 3438 镜像 site)同步加 `OwningSubRepoSlug` 字段

#### 4.5.2 5 BuildOrLoadGraph caller 改造(Commit 2)

| File:Line | 改造 |
|---|---|
| `analyzer.go:342` | `graph, err := repomap.BuildOrLoadGraph(repoRoot, query)` → `mg, err := repomap.BuildOrLoadMultiGraph(ctx, repoRoot, query)` |
| `analyzer.go:1672` | 同上 |
| `analyzer.go:1771` | 同上(返回 `*MultiGraph`,callsite 内部使用通过 `mg.AllGraphs()` 或 `mg.Single()` 视场景而定) |
| `keyword_search.go:667` | 同上,内部跨子仓 grep + per-hit slug 标注 |
| `sub_explorer.go:366` | 同上,选定子仓后 `mg.EnsureLoaded(slug)` 拿 *Graph |

新 `repomap.BuildOrLoadMultiGraph(ctx, parentRoot, query) *MultiGraph` 在 `facade.go`:
- 单仓(topology.Repos size 1) → 直接 `BuildOrLoadGraph(repoRoot, query)` 包成 `MultiGraph{IsSingle=true}`
- 多仓 → routing fold(§3.5)→ EnsureMany(top-cap slugs) → 返回 carrier

#### 4.5.3 Raw consumer 归类改造(Commit 3-4)

按 §11 audit 表格,逐个文件改:

**Z 类(via Oracle/Locator,代码不动或最小改动)**:
- `oracle.go::indexSymbols`、`locator.go::LocateSymbol` — 已是 single-graph 封装,新加 `multiGraphOracle/multiGraphLocator` fan-out 调用前者
- `ground.go:820/1761`、`taxonomy.go:244/299/395`、`analyzer.go:710/2290`、`explorer_erm.go:518`、`symbol_resolver.go:75/174`、`mechanism_scan.go:218`、`typed_relations.go:135/166`、`contract_check.go:873`、`keyword_search.go:687`、`explorer.go:2674/2816` — 改成调 `mg.Oracle().XXX(name)` 或 `mg.Locator().YYY(name)` 而不是直接 `g.SymbolDefs[name]` / `g.SymbolByID[id]`

**Y 类(per-graph 重算 / flatten / owner-aware lookup)**:
- `rank.go::RankGraph(g)` — 不动签名;`MultiGraph.Rank()` 在 `multigraph.go` 里 `for _, g := range AllGraphs() { rank.RankGraph(g) }` 各自计算
- `subgraph.go::ComputeChains/Hubs/Bridges(g)` — 同上,per-subrepo 计算
- `explorer.go` 25 个 FileIndex / 18 个 SymbolDefs:Z 类走 Oracle;FileIndex lookup 改 `mg.FileInfoFor(relPath)`;FileIndex 全量 iter 改 `mg.AllGraphs()` 双层循环
- `analyzer.go:2417 buildAnchorMap` — `for path, fi := range graph.FileIndex` 改 `for _, g := range mg.AllGraphs() { for path, fi := range g.FileIndex { ... } }`,path 加 sub-repo prefix
- `subgraph.go:107/186/353` (`for f := range g.FileIndex`) — 同上,per-subrepo 计算
- `keyword_search.go:665/685/677` — owner-aware lookup
- `exact_resolution_scope.go:780/836/718` — 同上(`findCrossRepoImports` 命名已暗示跨仓,真多仓后 owner-aware 自然)
- `token_classifier.go:181/194` — owner-aware
- `render.go:298/348/489/514/544/660/461/464/467` — render 时 aggregate(per-subrepo 渲染 + 父级合并,UX 标注 sub-repo 来源)
- `ground.go:690/847`、`emit_evidence.go:1148`(FileIndex import 解析)— owner-aware
- `analyzer.go:563/1954`(Scores/QueryScores filter)— owner-aware via `mg.ScoreFor(path)` / `mg.QueryScoreFor(path)`
- `explorer.go:4277/4278`(QueryScores)— owner-aware

**退化类**:
- `rank.go:48 g.Root` — `mg.Root()` 父根
- `Metadata` aggregate via `mg.Metadata()`

#### 4.5.4 logtriage validateFrame OwningRepoSlug(Commit 5,可与 4.5.3 合并)

`internal/analysis/logtriage/validate.go::validateFrame` 加 owning-repo 解析(`topology.Lookup(frame.File)`),frame 上挂 `OwningRepoSlug` 字段(扩展现有 BundleEntry 模式)。

#### 4.5.5 写模式跨仓 fail-loud + R2' 6 处同步

写模式下 `task.scope=micro` 强制 `kind=patch`(memory:Method M),plan/apply/verify 必须锁定**单一** target sub-repo。流程:

1. analyzer 在 read 阶段把 RequiredFiles routing 到候选 sub-repos(多个 OK)
2. 进入 plan 阶段前,`write_analyzer` 必须把 ChangePlan 收敛到**一个** sub-repo(多 sub-repo 写入禁止 — fail-loud)
3. apply 阶段 `SetRepoRoot(worktree.Path)`,target sub-repo 复制到 worktree
4. verify 阶段 `SetActiveSubRepo` 锁定不变,runner 探测在该 sub-repo 范围内进行

新增 `ViolKind = "WriteCrossSubRepoForbidden"`。**R2' 6 处同步**(MEMORY 红线):
1. `internal/types/violations.go` ViolKind 常量 + 详情结构体
2. `internal/types/violation_schema.go` JSON schema description
3. 写模式 skill prompt 提及该违规
4. retry hint 模板提示用户 split into separate runs
5. JSON decoder strict-decode error remap
6. cooccurrence rule + RepairLocus 映射

`MainRepoRoot` 保留语义为"原始 ActiveSubRepo.RootAbs"(双 site:`context.go:3290 + 3438` 同步)。

### 4.6 Phase 5(2 commits) — Scanner / Resolver per-subrepo 隔离

- **Commit 1**:`scanner.go::ScanFiles` 不变签名(仍 single-root 调用,因为 BuildOrLoadGraph 仍 per sub-repo);`scanner.go:106` 的 `IsArkTSProject` 在 Phase 0 已修签名
- **Commit 2**:`detectRunnerPlans(repoRoot)` 改 `detectRunnerPlans(repoRoot, subRepoRoot string)`,walk 范围限制在 `subRepoRoot` 子树。调用点(planner / verifier)从 `BusContext.ActiveSubRepo.RootAbs` 读取,无 ActiveSubRepo 时退化传 `repoRoot`(单仓兼容)。**12 runner 自动收益**(Go/Node/Python/Rust/Swift/Java/Ruby/CMake/Meson/Make/hvigor/cjpm)

每语言 resolver 不需要改造 — Phase 5 之后每个 sub-repo 各自走自己的 BuildOrLoadGraph,resolver 看到的 g.Files 都是单 sub-repo 范围,Go shallowest-only / Rust longest-prefix 等单仓假设自动成立。

### 4.7 Phase 6(2 commits) — Telemetry + docs/architecture.md

- **Commit 1**:`telemetry`:每 Run 输出 `multigraph: discovered=%d active=%d evicted=%d pending=%d thrashing=%v`;Discovery 一次性 INFO `multi-repo: parent=<parentRoot> n_subrepos=<N> langs=<...>`
- **Commit 2**:`docs/architecture.md` 加 §multi-repo 章节(链接到本设计稿);structural test fixture(testdata/multirepo/...)pin 进 repo;最终 `go build / go vet / go test ./...` 全绿,如有失败逐个修复

**实施总计 14-18 commits / 单 session 满载执行**。LLM eval 不在本 session 范围(下个 session 入口)。

### 4.8 Phase 4 finalization 重新分类(2026-05-08 收尾审计)

P4.F routing fold 落地后,消费者侧的语义重新理解:5 个 BuildOrLoadGraph caller 不再返回"最大子仓"半成品,而是 **routing fold 选定的当前 query 最相关 sub-repo** 的 `*Graph`。下游 raw 消费点(rank、subgraph、taxonomy、ground、explorer 内 ~70 处 FileIndex/SymbolDefs)在该 *Graph 上操作,**语义对已路由的子仓正确**。

设计 §11 audit 原始 80+ 点不再全部需要"迁移到 fan-out":

| 类别 | 数量 | 处理 |
|---|---|---|
| 5 BuildOrLoadGraph caller | 5 | ✅ Phase 4.3 走 GraphFromXxxOrLoad + routing fold |
| 写模式跨仓 fail-loud | 1 hook | ✅ Phase 4.G ValidateChangePlanScope |
| Producer-side per-sub-repo helper(rank/subgraph/taxonomy 等) | ~30 | **保留 `*Graph` 签名**(架构正确,跨仓 rank 尺度不可比;跨仓 chain 语义未定义) |
| Per-sub-repo Z 查询(ground evidence resolver、analyzer hint validation 等) | ~20 | **保留单 graph**(routing fold 已选对子仓;fan-out 反而引入跨仓噪声) |
| 跨子仓 opt-in fan-out 入口(已 wired 但消费方按需) | — | `mg.Oracle() / Locator() / LookupSymbol / IterateSymbolDefs / AllGraphs / Files / FileInfoFor / ImportEdges` 全部已实现 |

**100% 完整 = 架构件全 ship + 路由正确 + 写模式 fail-loud + 跨仓 API wired**(消费侧按需消费)。剩余"raw consumer 迁移"是 cross-sub-repo opt-in 增强,触发条件:用户提的问题确实跨子仓(`/repos` 列表 N≥2 + 问题 entity 跨多 RootRel),此时 partial_typed_lane disclosure 自动暴露,用户决定是否 `/repos focus` 或拆 Run。

## 5. 内存预算

| 项 | 单仓量级 | cap=3 时多仓量级 | 说明 |
|---|---|---|---|
| FileInfo + Symbols | 50 MB / 10K 文件 | 150 MB | 与 cap 线性 |
| SymbolByID / MethodIndex | 30 MB | 90 MB | per-graph 独立 |
| ImportGraph + Reverse | 20 MB | 60 MB | per-graph 独立 |
| Topology snapshot | 10 KB / 子仓 | 1 MB / 100 子仓 | 常驻 |
| **Total active mem** | ~100 MB | ~300 MB | 与今天单仓 ×3 |
| **Topology overhead** | 0 | 1 MB | 持久化到磁盘 |

100 子仓 × 1 万文件场景:active mem 仍 ~300 MB(只 hold 3 个),topology snapshot 1 MB。**与今天单仓内存 ~3×**,可控。

## 6. 风险与缓解

| 风险 | 概率 | 影响 | 缓解 |
|---|---|---|---|
| Discovery walk 遇大目录(node_modules 等)慢 | 中 | scan 数秒延迟 | `tool.IsExcludedDirName` 已剪枝;限制 depth ≤ 4 |
| LRU thrashing(query 触发反复 evict-load) | 低 | scan 时间放大 | **fail-loud** ErrThrashingDetected 5 evict/60s 触发,提示调高 cap(§3.4) |
| 用户 SIGKILL 时 LRU 内存不写回磁盘 | — | 无影响(per-repo cache 已写) | per-repo BuildOrLoadGraph 内部已经 SaveCache;evict 只丢 RAM |
| ArkTS leak Phase 0 修复回归 | 低 | TS 项目误为 LangTypeScript | 测试 fixture 覆盖 + 24 case eval rerun |
| 写模式跨仓 ChangePlan(§4.5.5) | 中 | 用户 confused | `ViolKind=WriteCrossSubRepoForbidden` fail-loud + REPL hint "split into separate runs per sub-repo" |
| Cross-repo Symbol 同名漏报 | 中 | typed-lane partial | `partial_typed_lane=true` precise enum 标记进 LLM prompt(R3 红线) |
| Discovery cache stale | 低 | active sub-repo 不全 | `/repos refresh` UX 兜底 + 父目录 mtime 探测 |
| Raw consumer 归类漏改 | 中 | 多仓静默错配 | §11 audit 列出 80+ 消费点 + Phase 4 逐文件改 + structural test |

## 7. 红线 checklist(实施时逐条勾)

- [ ] **L1**:read-mode byte-preserved(`runReadSchedulerLoop` 不变 — multi-repo 改造在调用点之外)
- [ ] **L2**:`write_enabled=false` 默认 + multi-repo 不引入新写路径
- [ ] **L3**:multi-repo write skill 不调用 `ground.BuildContext`
- [ ] **L5**:worktree 清理:Phase 4.5.5 后 worktree 来源于 ActiveSubRepo,`worktree.DiscardByPath` 不变,outer defer 路径完整
- [ ] **L7-L8**:render/mermaid 失败路径(与本设计无关,保留)
- [ ] **R3**(precise vs noisy):`partial_typed_lane` 是 typed enum boolean(precise),用作 hard gate;sub-repo routing 通道 D/E(noisy)只作 rank-soft 引导
- [ ] **R2'**(typed signal 6 处同步):`OwningRepoSlug` + `WriteCrossSubRepoForbidden` 6 处同步:struct + schema desc + skill prompt + retry hint + JSON decoder remap + cooccurrence rule/RepairLocus
- [ ] **R6**(无 internal pipeline info in LLM prompts):`PendingSubRepos` 列表暴露给 LLM 时只用 RootRel(用户可读),不暴露 slug
- [ ] **R4**(无 over-fitted 实现):discovery walk / LRU / Routing 都通用化,不绑定 codrax 自身路径假设
- [ ] **No backward-compat shim**:`multi_repo_enabled=false` 不引入新 code path 维护成本(MultiGraph 内部退化即可)
- [ ] **不允许半成品**:Phase 0-6 全 commit 后 `go build / go vet / go test ./...` 必须全绿

## 8. 跨语言矩阵(附录,Phase 5 必读)

| 语言 | 跨仓风险(改造前) | per-subrepo 隔离后 | 改造点 | 注 |
|---|---|---|---|---|
| Go | LOW(取最浅 go.mod 文档化) | 自动正确 | 0 | resolver_go.go:37 注释保留 |
| Java | LOW(package_clause AST) | 自动正确 | 0 | extract_java.go |
| Kotlin | LOW(package_header AST) | 自动正确 | 0 | extract_kotlin.go |
| Cangjie | LOW(L-Cangjie-2 红线) | 自动正确 | 0 | extract_cangjie.go |
| **ArkTS** | **HIGH** | 自动正确 | **Phase 0** | types/lang.go:279 单独修 |
| Python | LOW(path 推断,跨仓自然分歧) | 自动正确 | 0 | extract_python.go |
| JS/TS | LOW(longest-prefix tsconfig) | 自动正确 | 0 | resolver_javascript.go |
| Rust | MEDIUM(crate 名 collision) | 自动正确 | 0 | resolver_rust.go:131 |
| C/C++ | LOW(suffix index) | 自动正确 | 0 | resolver_cpp.go |
| CUDA(`.cu`/`.cuh`) | LOW(走 Cpp resolver) | 自动正确 | 0 | extToLang 别名 |
| Obj-C(`.m`/`.mm`) | LOW(走 C/Cpp resolver) | 自动正确 | 0 | extToLang 别名 |
| Swift / Ruby / Lua / Proto | LOW(纯路径) | 自动正确 | 0 | — |
| **run_tests.go** | **MEDIUM-HIGH** | Phase 5 改造 | 1 处签名扩展 | run_tests.go:677-738 (12 runner) |

**全 15 语言 + 2 别名**(.ets/.cj/.kt/.kts/.go/.py/.pyi/.js/.jsx/.mjs/.ts/.tsx/.java/.rs/.c/.h/.cc/.cpp/.cxx/.hpp/.hh/.cu/.cuh/.m/.mm/.rb/.swift/.lua/.proto)覆盖 — 核对自 `internal/tool/repomap/types/lang.go::extToLang` 权威映射。

## 9. 决策记录(2026-05-08 用户确认)

| # | Question | Decision | Rationale / 实施约束 |
|---|---|---|---|
| 1 | Discovery depth 默认 | **4** | 覆盖 `mono/{frontend,backend,sdk}/<repo>` 三级 + 父根,深于 4 罕见;用户可 yaml override |
| 2 | `multi_repo_enabled` 默认 | **true** | 单仓启动开销 ≤ 5ms;详见 §9.1 |
| 3 | 写模式跨仓 ChangePlan | **fail-loud,禁止** | ViolKind `WriteCrossSubRepoForbidden`,REPL hint "split into separate runs per sub-repo" |
| 4 | REPL `/repos` 在 `enabled=false` 时 | **命令保留,只展示 hint** | handler 5 行;**不支持运行时 toggle**(§9.2) |
| 5 | PrimaryLangs 探测 | **双保险:Tier 1 fast + Tier 2 lazy** | §9.3 |
| 6 | `BuildOrLoadGraph` caller 多仓返回类型 | **`*MultiGraph` carrier(Z+Y 混合)** | 纯 Z 半成品(raw 字段消费点 silent miss);纯 Y 改动面失控。MultiGraph 接口层 fan-out + raw 字段 flatten/owner-aware,单仓退化等价 |
| 7 | Slug 格式 | **复用 `index.CacheDir` 现行 `<basename>-<8hex>`** | 真同源,不重复造轮子,telemetry 可读 |

### 9.1 单仓退化路径(默认开启的代价)

`multi_repo_enabled=true` 默认下,单仓用户每次启动经过 §3.3.1 step 1:

```
parentAbs/.git 探测:
  存在 → 直接生成 RepoTopology{Repos: [SubRepo{Slug=cacheDirSlug(parentAbs), RootRel="."}]}
        立即返回(无 BFS walk)
        额外开销:1 次 os.Stat + 1 次 sha256 = ~50µs
  不存在 → BFS walk 启动(进入多仓路径)
```

单仓用户的额外成本是 **~50µs** + 一次 topology.json 写盘(~1KB)。可忽略。

**反过来**:多仓用户从 `multi_repo_enabled=false`(若默认是 false)的世界过渡时,会遭遇 §2 列出的所有 HIGH/MEDIUM-HIGH leak。**默认 true 是把"正确性"放在第一,把"启动 50µs"放在第二**。

### 9.2 为什么不支持 `/repos enable` 运行时 toggle

运行时从 disabled → enabled 需要:(a) 重建 MultiGraph;(b) 替换 `BusContext.SubRepos / ActiveSubRepo`(已 in-flight stage 持有旧引用);(c) 已 emit 的 RequiredFiles re-routing;(d) baseline cache / Failure Taxonomy 已按 single-graph slug 写入。

(b)(c)(d) 在 stage 边界外做安全,边界内做需要 lock + checkpoint — 等于半个重启。**重启更便宜也更可信**。

### 9.3 双保险 PrimaryLangs 流程

```
Tier 1(Discovery 启动期,§3.3.1 step 3):
  tool.NewGitCommand(nil, "-C", subRepoRoot, "ls-files", "--cached", "--others", "--exclude-standard")
  扩展名分桶 → top-3 langs
  special-file 修正:
    oh-package.json5  → 强制加 ArkTS
    cjpm.toml         → 强制加 Cangjie
    build.gradle*     → 当 .kt 多于 .java 时标 Kotlin,反之 Java
    Cargo.toml        → 强制加 Rust
  得 SubRepo.PrimaryLangs (粗,routing 通道 D 用)
  得 SubRepo.PrimaryLangsTier = 1

Tier 2(EnsureLoaded 后,multigraph.go EnsureLoaded 末尾):
  graph := repomap.BuildOrLoadGraph(...)
  if graph.Metadata.Languages != nil && SubRepo.PrimaryLangsTier < 2:
      newPrimaryLangs := top3ByCount(graph.Metadata.Languages)
      if !equalSlice(newPrimaryLangs, oldPrimaryLangs):
          telemetry.Log("multigraph: %s primary_langs Tier1=%v Tier2=%v",
                         slug, oldPrimaryLangs, newPrimaryLangs)
      SubRepo.PrimaryLangs = newPrimaryLangs
      SubRepo.PrimaryLangsTier = 2
      topology.Save()    // 持久化到 <runtime-anchor>/cache/topology/<parent-slug>.json

下次启动:Tier 2 已校准的 sub-repo 直接从 topology.json 读 ground truth
         冷子仓仍是 Tier 1 (足够给 routing 用,EnsureLoaded 时再升级)
```

## 10. 速查表 — 实施时新增/改动文件清单

```
NEW:
  internal/tool/repomap/topology/discover.go       ~150 LOC
  internal/tool/repomap/topology/topology.go       ~80 LOC
  internal/tool/repomap/topology/manifest.go       ~60 LOC
  internal/tool/repomap/multigraph/multigraph.go   ~280 LOC  (Z+Y 全方法)
  internal/tool/repomap/multigraph/oracle.go       ~80 LOC
  internal/tool/repomap/multigraph/locator.go      ~80 LOC
  internal/tool/repomap/multigraph/lru.go          ~80 LOC   (含 thrashing 检测)
  testdata/multirepo/parent/{a,b,c}/...            fixtures
  testdata/multirepo/arkts_leak_fix/...            Phase 0 fixture
  testdata/eval/multirepo/                         eval fixtures (下个 session)

CHANGED:
  internal/tool/repomap/types/lang.go              ~10 LOC (Phase 0)
  internal/tool/repomap/index/cache.go             ~5  LOC (CacheDirSlug helper)
  internal/tool/repomap/index/scanner.go           ~5  LOC
  internal/tool/repomap/facade.go                  ~50 LOC (BuildOrLoadMultiGraph)
  internal/tool/repomap/oracle.go                  ~5  LOC (no-op,multiGraphOracle 用 graphOracle)
  internal/tool/repomap/locator.go                 ~5  LOC (同上)
  internal/types/context.go                        ~30 LOC (SubRepos / ActiveSubRepo / PendingSubRepos / 双 site)
  internal/types/violations.go                     ~15 LOC (新 ViolKind)
  internal/types/violation_schema.go               ~10 LOC (schema desc)
  internal/config/runtime.go                       ~30 LOC (4 fields + defaults)
  cmd/root.go                                      ~25 LOC (resolveRepoTopology call)
  internal/repl/input.go                           ~10 LOC (slashCommands /repos)
  internal/repl/repl.go                            ~80 LOC (handleSlash + handleReposCmd + alias?)
  internal/agent/analyzer.go                       ~100 LOC (5 callers + raw consumer routing)
  internal/agent/keyword_search.go                 ~30 LOC
  internal/agent/sub_explorer.go                   ~15 LOC
  internal/agent/explorer.go                       ~80 LOC (47 raw consumer 改造)
  internal/agent/ground.go                         ~30 LOC (15 raw consumer)
  internal/agent/explorer_erm.go                   ~15 LOC
  internal/agent/symbol_resolver.go                ~10 LOC
  internal/agent/taxonomy.go                       ~15 LOC
  internal/agent/mechanism_scan.go                 ~10 LOC
  internal/agent/token_classifier.go               ~10 LOC
  internal/agent/exact_resolution_scope.go         ~15 LOC
  internal/agent/emit_evidence.go                  ~10 LOC
  internal/contract/typed_relations.go             ~10 LOC
  internal/orchestrator/contract_check.go          ~10 LOC
  internal/tool/repomap/oracle_render/render.go    ~30 LOC (aggregate 渲染)
  internal/tool/repomap/oracle_render/rank.go      ~10 LOC (per-subrepo)
  internal/tool/repomap/oracle_render/subgraph.go  ~10 LOC (per-subrepo)
  internal/analysis/logtriage/validate.go          ~15 LOC
  internal/tool/run_tests.go                       ~20 LOC (subRepoRoot 参数,12 runner 收益)

≈ 850 LOC NEW + ~720 LOC CHANGED + tests
```

## 11. Raw consumer audit(Phase 4 实施依据)

`*types.Graph` 共 11 个 public 字段。下表是 audit 总览,**每行落地到 §4.5.3 具体改造**。详细 file:line 列表见 doc 历史 / git blame audit commit。

| 字段 | 消费 sites | 主类型 | Phase 4 策略 |
|---|---|---|---|
| `Files []*FileInfo` | 13 | **Y** | rank.go/subgraph.go per-subrepo 重算;explorer/render flatten |
| `FileIndex map[string]*FileInfo` | **20**(explorer 主导) | **Y** | lookup → `mg.FileInfoFor`;iter → `mg.AllGraphs()` 双层 |
| `SymbolDefs map[string][]*Symbol` | 18 | **Z** | `mg.Oracle()` fan-out(`graphOracle` 套 `multiGraphOracle`) |
| `SymbolByID map[SymbolID]*Symbol` | 4 | **Z** | `mg.Oracle().LookupSymbolByID` |
| `MethodIndex map[MethodKey]*Symbol` | 1 internal | **Z** | `mg.Oracle().ResolveCallTarget` wrapper |
| `ImportGraph map[string][]string` | 6 | **Y** | subgraph per-subrepo;render owner-aware |
| `ReverseImports map[string][]string` | 6 | **Y** | rank per-subrepo;subgraph per-subrepo |
| `Scores map[string]float64` | 7 | **Y** | rank per-subrepo;`mg.ScoreFor(path)` owner-aware 读 |
| `QueryScores map[string]float64` | 6 | **Y** | rank per-subrepo;`mg.QueryScoreFor(path)` owner-aware 读 |
| `Metadata Metadata` | 5 | **退化** | `mg.Metadata()` aggregate |
| `Root string` | 1 | **退化** | `mg.Root()` 父根 |

**热点文件**:
- `explorer.go` — 47 处(FileIndex 25 / SymbolDefs 18 / SymbolByID 1 / QueryScores 2 / Files 1)
- `rank.go` — 21 处(producer 侧,per-subrepo 计算自然正确)
- `analyzer.go` — 18 处(FileIndex 7 / SymbolDefs 2 / Scores 1 / QueryScores 1 / 其他)
- `ground.go` — 15 处(FileIndex 9 / SymbolDefs 3 / 其他)
- `subgraph.go` — 14 处(FileIndex 8 / ImportGraph 4 / ReverseImports 2)
- `render.go` — ~15 处(混合)

## 12. 实施顺序提交计划(commit-by-commit)

| # | Phase | Commit | 影响 | Build/Test |
|---|---|---|---|---|
| 1 | — | docs: design v2 落盘 | doc only | n/a |
| 2 | 0 | fix(repomap): IsArkTSProject 接受 sub-repo root | scanner + lang | go test ./internal/tool/repomap/types/... |
| 3 | 1 | feat(topology): discover + manifest + topology types | topology pkg | go test ./internal/tool/repomap/topology/... |
| 4 | 1 | feat(cmd): cmd/root resolveRepoTopology | cmd/root | go build,go vet |
| 5 | 2 | feat(config): 4 multi_repo_* yaml 字段 | runtime.go | go test ./internal/config/... |
| 6 | 2 | feat(repl): /repos slash command (3 处注册) | repl | go test ./internal/repl/... |
| 7 | 3 | feat(multigraph): LRU + thrashing 检测 | multigraph/lru.go | go test ./internal/tool/repomap/multigraph/... |
| 8 | 3 | feat(multigraph): MultiGraph carrier (Z+Y) | multigraph/multigraph.go | go test ... |
| 9 | 3 | feat(multigraph): oracle/locator fan-out + partial_typed_lane | multigraph/oracle.go + locator.go | go test ... |
| 10 | 4 | feat(types): BusContext SubRepos/ActiveSubRepo/PendingSubRepos | context.go(双 site) | go vet,go test ./internal/types/... |
| 11 | 4 | feat(repomap): facade BuildOrLoadMultiGraph | facade.go | go test ... |
| 12 | 4 | refactor(agent): 5 caller → MultiGraph + Z 类 oracle 改造 | analyzer/keyword_search/sub_explorer + Z hotspots | go build,go vet |
| 13 | 4 | refactor(agent): Y 类 raw consumer flatten/owner-aware | explorer/ground/subgraph/render/rank/render(per-subrepo)/analyzer 全量 | go build,go vet,go test ./... |
| 14 | 4 | feat(write): WriteCrossSubRepoForbidden + R2' 6 处同步 | violations + schema + skill prompt + retry hint + decoder + cooccurrence | go test ./internal/contract/... |
| 15 | 4 | feat(logtriage): validateFrame OwningRepoSlug | validate.go | go test ./internal/analysis/logtriage/... |
| 16 | 5 | refactor(run_tests): detectRunnerPlans 加 subRepoRoot 参数(12 runner 收益) | run_tests.go | go test ./internal/tool/... |
| 17 | 6 | feat(telemetry): multigraph 事件 + thrashing 升级 | telemetry pkg | go test ... |
| 18 | 6 | docs: architecture.md §multi-repo + structural test pin | docs + testdata/multirepo | **go build / go vet / go test ./...** 全绿 |

实施每 commit 跑 `go build ./... && go vet ./...`,关键 commit 跑 `go test ./<affected>/...`。最终 commit #18 跑全套 `go test ./...`。

---
**Doc 起草**:2026-05-07/08;v2 改动:2026-05-08(audit + 6 fix-ups + Z+Y carrier 落定)。
