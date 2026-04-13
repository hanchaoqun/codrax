# repomap_v3 baseline — 2026-04-13T20:30:26Z

- repo: `/home/chatpp/codrax`
- files: 186, symbols: 1828, relations: 12399
- full scan: 0.18s

## Metrics

| metric | value |
|---|---|
| symbol precision (sample 500 / 500 checked) | 1.000 |
| symbol recall (125 / 125) | 1.000 |
| symbol recall by name only | 1.000 |
| import edge accuracy (7 / 21) | 0.333 |
| call edge unambiguous ratio (2381 / 10836) | 0.220 |
| task_map mean hit@k (23 / 35 perfect) | 0.729 |

## Import accuracy per language

| language | resolved / total | accuracy |
|---|---|---|
| go | 7 / 21 | 0.333 |

## Call edge ambiguity histogram

| ambiguity | count |
|---|---|
| 0 | 7442 |
| 1 | 2381 |
| 2 | 239 |
| 3 | 162 |
| 4 | 49 |
| 5 | 98 |
| 6 | 310 |
| 9 | 38 |
| 11 | 5 |
| 15 | 112 |

## Top ambiguous call targets

| symbol | call edges |
|---|---|
| Run | 220 |
| Error | 131 |
| String | 90 |
| Debug | 75 |
| Name | 68 |
| Close | 59 |
| Execute | 44 |
| Info | 36 |
| New | 36 |
| Register | 24 |
| TaskList | 21 |
| BuildInitialPrompt | 18 |
| Get | 15 |
| contains | 14 |
| Warning | 13 |
| ParseOutput | 12 |
| mergeEvidenceItems | 12 |
| mergeStrings | 12 |
| ContinuationPrompt | 10 |
| Resolve | 10 |

## task_map per-query hits

| query | hit | missing |
|---|---|---|
| `determineActivePolicy run policy` | 1.00 | — |
| `AnalysisIR RunPolicy` | 0.50 | internal/analysis/risk/policy.go |
| `AnalyzerClassification MutableState` | 0.50 | internal/tool/todo_write.go |
| `todo_write analyzer classification` | 0.50 | internal/tool/todo_write.go |
| `buildAnalysisIR buildRequestModel` | 1.00 | — |
| `ir_accessor irKeywords irEntities` | 1.00 | — |
| `propose_sub_agents tool` | 0.00 | internal/tool/propose_sub_agents.go |
| `subagent register default SubExplorer` | 1.00 | — |
| `extractSubAgentProposal orchestrator` | 1.00 | — |
| `explorer ShouldStop BaseAgent` | 1.00 | — |
| `ContinuationPrompt explorer evaluator` | 1.00 | — |
| `keyword search IDF ranking` | 1.00 | — |
| `extractConcreteValues evidence` | 0.00 | internal/agent/explorer.go |
| `EvidenceRequirement ERM` | 1.00 | — |
| `risk matrix derive policy` | 1.00 | — |
| `hypothesis planner Plan Bind` | 1.00 | — |
| `counterfactual expand ShouldExpand` | 1.00 | — |
| `quality gate report checks` | 1.00 | — |
| `normalizer term graph canonical` | 0.00 | internal/analysis/normalizer/normalizer.go |
| `compiler scenario compile` | 1.00 | — |
| `BuildGraph FileInfo Symbol Relation` | 0.50 | internal/tool/repomap/types.go |
| `RankGraph query score fan-out` | 1.00 | — |
| `repomap cache SaveCache LoadFileInfos` | 1.00 | — |
| `ScanFiles FileEntry special file` | 1.00 | — |
| `GenerateView task_map call_path` | 1.00 | — |
| `tree-sitter extractor go` | 0.50 | internal/tool/repomap/parser.go |
| `BuildAgentContext narrowed view` | 0.00 | internal/context/builder.go |
| `MutableState TaskList lock concurrency` | 1.00 | — |
| `GrepTool ReadFile tool builtin` | 1.00 | — |
| `logging rotation PID retention` | 1.00 | — |
| `memory store compact turn index` | 1.00 | — |
| `REPL slash command history` | 1.00 | — |
| `finalizer answer shape translation` | 0.00 | internal/agent/finalizer.go |
| `有多少个agent可以调用subagent` | 0.00 | internal/agent/subagent.go |
| `分析器如何派生 run policy` | 0.00 | internal/analysis/risk/policy.go, internal/agent/analyzer.go |
