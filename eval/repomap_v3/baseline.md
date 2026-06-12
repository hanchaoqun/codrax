# repomap_v3 baseline — 2026-06-12T09:26:53Z

- repo: `/Users/han/opt/claude/codrax`
- files: 1758, symbols: 39014, relations: 254219
- full scan: 0.78s

## Metrics

| metric | value |
|---|---|
| symbol precision (sample 500 / 500 checked) | 1.000 |
| symbol recall (125 / 125) | 1.000 |
| symbol recall by name only | 1.000 |
| import edge accuracy (1792 / 6353) | 0.282 |
| call edge unambiguous ratio (57260 / 236613) | 0.242 |
| call receiver capture ratio (107142 / 236613) | 0.453 |
| unambiguous by receiver-type (4386 / 236613) | 0.019 |
| **drift calls resolved (1335 / 23893)** | **0.056** |
| task_map mean hit@k (32 / 35 perfect) | 0.929 |

## Import accuracy per language

| language | resolved / total | accuracy |
|---|---|---|
| arkts | 0 / 5 | 0.000 |
| go | 1792 / 6342 | 0.283 |
| python | 0 / 6 | 0.000 |

## Call edge ambiguity histogram

| ambiguity | count |
|---|---|
| 0 | 148708 |
| 1 | 57260 |
| 2 | 13505 |
| 3 | 3442 |
| 4 | 830 |
| 5 | 1418 |
| 6 | 908 |
| 7 | 208 |
| 8 | 1282 |
| 9 | 562 |
| 10 | 26 |
| 11 | 296 |
| 12 | 454 |
| 13 | 117 |
| 14 | 65 |
| 15 | 179 |
| 16 | 16 |
| 17 | 328 |
| 18 | 2911 |
| 19 | 508 |
| 20 | 19 |
| 21 | 201 |
| 23 | 116 |
| 24 | 4 |
| 25 | 3 |
| 26 | 40 |
| 27 | 65 |
| 29 | 117 |
| 31 | 34 |
| 39 | 1 |
| 41 | 72 |
| 45 | 28 |
| 48 | 864 |
| 50 | 1193 |
| 56 | 20 |
| 64 | 170 |
| 69 | 43 |
| 71 | 22 |
| 77 | 2 |
| 82 | 8 |
| 113 | 520 |
| 120 | 9 |
| 149 | 39 |

## Top ambiguous call targets

| symbol | call edges |
|---|---|
| Contains | 7575 |
| String | 1561 |
| Error | 1193 |
| Run | 1130 |
| add | 1004 |
| Execute | 864 |
| Name | 520 |
| Index | 508 |
| New | 498 |
| Warning | 491 |
| Info | 489 |
| firstNonEmptyString | 436 |
| emit | 372 |
| max | 336 |
| Split | 324 |
| isZh | 324 |
| Add | 291 |
| Debug | 286 |
| info | 279 |
| insert | 256 |

## task_map per-query hits

| query | hit | missing |
|---|---|---|
| `runReadSchedulerLoop task graph scheduler` | 1.00 | — |
| `AnalysisIR RequestModel EvidencePlan` | 1.00 | — |
| `buildAnalysisIR buildRequestModel` | 1.00 | — |
| `propose_sub_agents tool` | 1.00 | — |
| `subagent register default SubExplorer` | 1.00 | — |
| `explorer ShouldStop BaseAgent` | 1.00 | — |
| `keyword search IDF ranking` | 1.00 | — |
| `EvidenceRequirement matrix explorer` | 1.00 | — |
| `answer document evaluator block shape` | 1.00 | — |
| `risk matrix Evaluate RiskMatrix` | 1.00 | — |
| `hypothesis planner Plan Bind` | 1.00 | — |
| `gate Thresholds coverage weights` | 1.00 | — |
| `Normalize TermGraph SymbolResolver` | 1.00 | — |
| `log triage bug class registry` | 1.00 | — |
| `dataflow lowerer producer evidence` | 1.00 | — |
| `BuildGraph FileInfo Symbol Relation` | 0.50 | internal/tool/repomap/index/build.go |
| `RankGraph query score fan-out` | 1.00 | — |
| `repomap cache SaveCache LoadFileInfos` | 1.00 | — |
| `ScanFiles FileEntry special file` | 1.00 | — |
| `GenerateView task_map call_path` | 1.00 | — |
| `tree-sitter extractor go` | 0.00 | internal/tool/repomap/index/extract_go.go, internal/tool/repomap/index/parser.go |
| `gin route literal handler resolver` | 1.00 | — |
| `RepoSizeTier DefaultTopN budget tier` | 1.00 | — |
| `PruneOrphanedCacheDirs orphaned cache sweep` | 1.00 | — |
| `multigraph LRU EnsureLoaded sub-repo` | 1.00 | — |
| `scoped projection rebase clone graph` | 1.00 | — |
| `trace event parse ftrace index cache` | 1.00 | — |
| `binder IPC edge confidence` | 1.00 | — |
| `StreamEventSearch bounded discovery` | 1.00 | — |
| `BuildAgentContext narrowed view` | 1.00 | — |
| `MutableState TaskList lock concurrency` | 1.00 | — |
| `GrepTool ReadFile tool builtin` | 1.00 | — |
| `REPL slash command history` | 1.00 | — |
| `有多少个agent可以调用subagent` | 1.00 | — |
| `分析器如何构建请求模型` | 0.00 | internal/agent/analyzer.go |
