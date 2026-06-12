# repomap_v3 baseline — 2026-06-12T06:58:24Z

- repo: `/Users/han/opt/claude/codrax`
- files: 1734, symbols: 38680, relations: 252254
- full scan: 2.51s

## Metrics

| metric | value |
|---|---|
| symbol precision (sample 500 / 500 checked) | 1.000 |
| symbol recall (17 / 125) | 0.136 |
| symbol recall by name only | 0.728 |
| import edge accuracy (1767 / 6247) | 0.283 |
| call edge unambiguous ratio (56803 / 234878) | 0.242 |
| call receiver capture ratio (106148 / 234878) | 0.452 |
| unambiguous by receiver-type (4381 / 234878) | 0.019 |
| **drift calls resolved (1331 / 23685)** | **0.056** |
| task_map mean hit@k (6 / 35 perfect) | 0.186 |

## Import accuracy per language

| language | resolved / total | accuracy |
|---|---|---|
| arkts | 0 / 5 | 0.000 |
| go | 1767 / 6236 | 0.283 |
| python | 0 / 6 | 0.000 |

## Call edge ambiguity histogram

| ambiguity | count |
|---|---|
| 0 | 147665 |
| 1 | 56803 |
| 2 | 13422 |
| 3 | 3487 |
| 4 | 757 |
| 5 | 1414 |
| 6 | 892 |
| 7 | 366 |
| 8 | 1122 |
| 9 | 542 |
| 10 | 52 |
| 11 | 267 |
| 12 | 451 |
| 13 | 117 |
| 14 | 65 |
| 15 | 179 |
| 16 | 16 |
| 17 | 327 |
| 18 | 2904 |
| 19 | 506 |
| 20 | 179 |
| 23 | 113 |
| 24 | 4 |
| 25 | 3 |
| 26 | 40 |
| 27 | 65 |
| 29 | 117 |
| 30 | 34 |
| 39 | 1 |
| 41 | 66 |
| 45 | 28 |
| 48 | 855 |
| 50 | 1193 |
| 56 | 20 |
| 64 | 169 |
| 69 | 43 |
| 71 | 22 |
| 77 | 2 |
| 80 | 8 |
| 113 | 515 |
| 120 | 9 |
| 148 | 38 |

## Top ambiguous call targets

| symbol | call edges |
|---|---|
| Contains | 7522 |
| String | 1556 |
| Error | 1193 |
| Run | 1128 |
| add | 1001 |
| Execute | 855 |
| Name | 515 |
| Index | 506 |
| New | 497 |
| Info | 489 |
| Warning | 489 |
| firstNonEmptyString | 436 |
| emit | 372 |
| max | 336 |
| isZh | 324 |
| Split | 323 |
| Add | 291 |
| Debug | 286 |
| info | 278 |
| insert | 256 |

## task_map per-query hits

| query | hit | missing |
|---|---|---|
| `determineActivePolicy run policy` | 0.00 | internal/orchestrator/orchestrator.go |
| `AnalysisIR RunPolicy` | 0.00 | internal/types/analysis_ir.go, internal/analysis/risk/policy.go |
| `AnalyzerClassification MutableState` | 0.00 | internal/types/context.go, internal/tool/todo_write.go |
| `todo_write analyzer classification` | 0.50 | internal/tool/todo_write.go |
| `buildAnalysisIR buildRequestModel` | 1.00 | — |
| `ir_accessor irKeywords irEntities` | 0.00 | internal/agent/ir_accessor.go |
| `propose_sub_agents tool` | 0.00 | internal/tool/propose_sub_agents.go |
| `subagent register default SubExplorer` | 0.00 | internal/agent/subagent.go |
| `extractSubAgentProposal orchestrator` | 0.00 | internal/orchestrator/orchestrator.go |
| `explorer ShouldStop BaseAgent` | 1.00 | — |
| `ContinuationPrompt explorer evaluator` | 0.00 | internal/agent/explorer.go |
| `keyword search IDF ranking` | 1.00 | — |
| `extractConcreteValues evidence` | 0.00 | internal/agent/explorer.go |
| `EvidenceRequirement ERM` | 0.00 | internal/agent/erm.go |
| `risk matrix derive policy` | 0.00 | internal/analysis/risk/policy.go, internal/analysis/risk/matrix.go |
| `hypothesis planner Plan Bind` | 0.00 | internal/analysis/hdp/planner.go |
| `counterfactual expand ShouldExpand` | 1.00 | — |
| `quality gate report checks` | 0.00 | internal/analysis/gate/gate.go |
| `normalizer term graph canonical` | 0.00 | internal/analysis/normalizer/normalizer.go |
| `compiler scenario compile` | 1.00 | — |
| `BuildGraph FileInfo Symbol Relation` | 0.00 | internal/tool/repomap/index/build.go, internal/tool/repomap/types/types.go |
| `RankGraph query score fan-out` | 0.00 | internal/tool/repomap/retrieve/rank.go |
| `repomap cache SaveCache LoadFileInfos` | 0.00 | internal/tool/repomap/index/cache.go |
| `ScanFiles FileEntry special file` | 0.00 | internal/tool/repomap/index/scanner.go |
| `GenerateView task_map call_path` | 0.00 | internal/tool/repomap/render/render.go |
| `tree-sitter extractor go` | 0.00 | internal/tool/repomap/index/extract_go.go, internal/tool/repomap/index/parser.go |
| `BuildAgentContext narrowed view` | 0.00 | internal/context/builder.go |
| `MutableState TaskList lock concurrency` | 0.00 | internal/types/context.go |
| `GrepTool ReadFile tool builtin` | 0.00 | internal/tool/builtin.go |
| `logging rotation PID retention` | 1.00 | — |
| `memory store compact turn index` | 0.00 | internal/memory/store.go |
| `REPL slash command history` | 0.00 | internal/repl/repl.go |
| `finalizer answer shape translation` | 0.00 | internal/agent/finalizer.go |
| `有多少个agent可以调用subagent` | 0.00 | internal/agent/subagent.go |
| `分析器如何派生 run policy` | 0.00 | internal/analysis/risk/policy.go, internal/agent/analyzer.go |

## Missing symbols (recall gaps)

- TaskItem (no definitions extracted)
- TaskList (no definitions extracted)
- TaskState @ internal/types/task.go:36 (found at internal/types/context.go:13, internal/types/context.go:5348)
- MutableState @ internal/types/context.go:32 (found at internal/types/context.go:71)
- AnalyzerClassification (no definitions extracted)
- StageReport @ internal/types/context.go:226 (found at internal/agent/agent.go:131, internal/types/context.go:5091)
- RepoFact @ internal/types/context.go:233 (found at internal/types/context.go:5098)
- ToolResult @ internal/types/context.go:242 (found at internal/types/context.go:5121)
- MCPResponse @ internal/types/context.go:251 (found at internal/types/context.go:5131)
- ExecutionSignals @ internal/types/context.go:261 (found at internal/types/context.go:5272)
- PolicyContext (no definitions extracted)
- BusContext @ internal/types/context.go:287 (found at internal/orchestrator/orchestrator.go:8370, internal/types/context.go:5341)
- AgentContext @ internal/types/context.go:340 (found at internal/types/context.go:5661)
- AnalyzerHints @ internal/types/analysis_ir.go:73 (found at internal/types/analysis_ir.go:205, internal/types/analysis_ir.go:706)
- Intent @ internal/types/analysis_ir.go:80 (found at internal/render/structured_tool_summary.go:742, internal/skill/skill.go:83, internal/tool/emit_analysis.go:52, internal/types/analysis_ir.go:50, internal/types/analysis_ir.go:888)
- Scenario @ internal/types/analysis_ir.go:95 (found at internal/orchestrator/answer_reviewer.go:54, internal/render/structured_tool_summary.go:743, internal/tool/emit_analysis.go:53, internal/types/analysis_ir.go:51, internal/types/analysis_ir.go:908)
- Complexity @ internal/types/analysis_ir.go:107 (found at internal/analysis/budget/budget.go:21, internal/render/structured_tool_summary.go:744, internal/tool/emit_analysis.go:54, internal/types/analysis_ir.go:52, internal/types/analysis_ir.go:924)
- TaskGraph @ internal/types/analysis_ir.go:156 (found at internal/analysis/compiler/compile.go:24, internal/types/analysis_ir.go:33, internal/types/analysis_ir.go:1015)
- TaskNode @ internal/types/analysis_ir.go:162 (found at internal/types/analysis_ir.go:1021)
- AnswerContract @ internal/types/analysis_ir.go:244 (found at internal/analysis/compiler/compile.go:26, internal/types/analysis_ir.go:35, internal/types/analysis_ir.go:1281, internal/types/observation_ledger.go:292)
- AnswerShape (no definitions extracted)
- Hypothesis @ internal/types/analysis_ir.go:278 (found at internal/types/analysis_ir.go:1501)
- HypothesisStatus @ internal/types/analysis_ir.go:287 (found at internal/types/analysis_ir.go:1510)
- RiskMatrix @ internal/types/analysis_ir.go:297 (found at internal/types/analysis_ir.go:55, internal/types/analysis_ir.go:1569)
- RunPolicy (no definitions extracted)
- AgentName @ internal/types/enums.go:42 (found at internal/types/context.go:5662, internal/types/context.go:6054, internal/types/enums.go:127)
- TaskStatus (no definitions extracted)
- MissingPiece @ internal/types/enums.go:91 (found at internal/agent/agent.go:88, internal/types/context.go:5786, internal/types/enums.go:184)
- StageConfig (no definitions extracted)
- Transition (no definitions extracted)
- EvidenceRequirement @ internal/agent/erm.go:25 (found at internal/agent/explorer_erm.go:38)
- EvidenceItem @ internal/types/evidence.go:29 (found at internal/types/evidence.go:552)
- FlowFindingDigest @ internal/types/evidence.go:49 (found at internal/types/evidence.go:876)
- AnswerSymbol @ internal/types/evidence.go:91 (found at internal/types/evidence.go:1340)
- SubAgentRuntime @ internal/agent/subagent_runtime.go:86 (found at internal/agent/subagent_runtime.go:188)
- Import @ internal/tool/repomap/types/types.go:94 (found at internal/tool/repomap/types/types.go:157)
- Relation @ internal/tool/repomap/types/types.go:125 (found at internal/orchestrator/contract_check_block.go:597, internal/tool/emit_investigation_complete.go:2669, internal/tool/repomap/types/types.go:241, internal/types/analysis_ir.go:1009, internal/types/typed_relation_hint.go:33, internal/types/typed_relation_hint.go:478)
- FileInfo @ internal/tool/repomap/types/types.go:140 (found at internal/tool/repomap/types/types.go:268)
- Graph @ internal/tool/repomap/types/types.go:165 (found at internal/agent/keyword_search.go:49, internal/tool/ground/ground.go:86, internal/tool/repomap/types/types.go:474)
- Metadata @ internal/tool/repomap/types/types.go:180 (found at internal/tool/repomap/multigraph/multigraph.go:676, internal/tool/repomap/types/types.go:264, internal/tool/repomap/types/types.go:486, internal/tool/repomap/types/types.go:582, internal/types/context.go:5118)
- ViewParams @ internal/tool/repomap/types/types.go:202 (found at internal/tool/repomap/types/types.go:604)
- RepoMapV2 @ internal/tool/repomap/tool.go:17 (found at internal/tool/repomap/tool.go:27)
- TodoWrite (no definitions extracted)
- ExecCommand @ internal/tool/builtin.go:32 (found at internal/tool/builtin.go:406)
- GrepTool @ internal/tool/builtin.go:115 (found at internal/tool/builtin.go:1015)
- ReadFile @ internal/tool/builtin.go:375 (found at internal/tool/builtin.go:2797)
- ListFiles @ internal/tool/builtin.go:493 (found at internal/tool/builtin.go:3095)
- determineActivePolicy (no definitions extracted)
- decideNextStage (no definitions extracted)
- applyStageOutput @ internal/orchestrator/orchestrator.go:588 (found at internal/orchestrator/orchestrator.go:8189)
- dispatchStage @ internal/orchestrator/orchestrator.go:449 (found at internal/orchestrator/orchestrator.go:7751)
- nextPendingTask (no definitions extracted)
- recordTaskFinalize @ internal/orchestrator/orchestrator.go:396 (found at internal/orchestrator/orchestrator.go:7685)
- runTaskPipeline (no definitions extracted)
- runTaskPhase @ internal/orchestrator/orchestrator.go:240 (found at internal/orchestrator/orchestrator.go:3016)
- runAnalyzePhase @ internal/orchestrator/orchestrator.go:219 (found at internal/orchestrator/orchestrator.go:2404)
- Run @ internal/orchestrator/orchestrator.go:127 (found at internal/agent/sub_explorer.go:36, internal/agent/subagent_runtime.go:220, internal/analysis/gate/gate.go:127, internal/dataquery/action_runner.go:595, internal/dataquery/dataquery.go:1788, internal/env/probe/probe.go:35, internal/orchestrator/orchestrator.go:1173, internal/orchestrator/orchestrator.go:1635, internal/repl/approve_reject_test.go:28, internal/repl/approve_reject_test.go:443, internal/repl/repl_test.go:24, internal/repl/repl_test.go:38, internal/repl/repl_test.go:51, internal/repl/repl_test.go:63, internal/repl/repl_test.go:79, internal/repl/repl_test.go:88, internal/repl/turn_policy_test.go:210, internal/tracequery/query.go:11)
- extractSubAgentProposal @ internal/orchestrator/orchestrator.go:841 (found at internal/orchestrator/orchestrator.go:8378)
- BuildAgentContext @ internal/context/builder.go:14 (found at internal/context/builder.go:23)
- buildAnalysisIR @ internal/agent/analyzer.go:240 (found at internal/agent/analyzer.go:1730)
- buildRequestModel (no definitions extracted)
- mapIntent (no definitions extracted)
- mapComplexity (no definitions extracted)
- mapAnswerShape (no definitions extracted)
- irQuestionKind @ internal/agent/ir_accessor.go:27 (found at internal/agent/ir_accessor.go:76)
- irAnswerShape (no definitions extracted)
- extractConcreteValues @ internal/agent/explorer.go:3921 (found at internal/agent/explorer.go:15695)
- keywordSearch @ internal/agent/keyword_search.go:58 (found at internal/agent/keyword_search.go:230)
- buildTodoItems (no definitions extracted)
- pickClassification (no definitions extracted)
- pickCurrentTaskID (no definitions extracted)
- normalizeQuestionKind @ internal/tool/todo_write.go:269 (found at internal/tool/emit_analysis.go:3663)
- normalizeAnswerShape (no definitions extracted)
- normalizeComplexity @ internal/tool/todo_write.go:333 (found at internal/tool/emit_analysis.go:3646)
- BuildOrLoadGraph @ internal/tool/repomap/tool.go:128 (found at internal/tool/repomap/tool.go:737)
- BuildGraph @ internal/tool/repomap/facade.go:69 (found at internal/tool/repomap/facade.go:96, internal/tool/repomap/index/build.go:19)
- RankGraph @ internal/tool/repomap/facade.go:128 (found at internal/tool/repomap/facade.go:183, internal/tool/repomap/retrieve/rank.go:61)
- SymbolKey @ internal/tool/repomap/facade.go:85 (found at internal/analysis/dataflow/types.go:77, internal/tool/repomap/facade.go:112, internal/tool/repomap/types/graph.go:53)
- ScanFiles @ internal/tool/repomap/facade.go:74 (found at internal/tool/repomap/facade.go:101, internal/tool/repomap/index/scanner.go:77)
- ParseFiles @ internal/tool/repomap/facade.go:80 (found at internal/tool/repomap/facade.go:107, internal/tool/repomap/index/parser.go:40)
- GenerateView @ internal/tool/repomap/facade.go:108 (found at internal/tool/repomap/facade.go:163, internal/tool/repomap/render/views.go:16)
- IsSpecialFile @ internal/tool/repomap/facade.go:100 (found at internal/tool/repomap/facade.go:155, internal/tool/repomap/index/scanner.go:258)
- CacheDir @ internal/tool/repomap/index/cache.go:33 (found at internal/config/runtime.go:50, internal/tool/repomap/index/cache.go:125)
- SaveCache @ internal/tool/repomap/index/cache.go:108 (found at internal/tool/repomap/index/cache.go:302)
- LoadFileInfos @ internal/tool/repomap/index/cache.go:182 (found at internal/tool/repomap/index/cache.go:388)
- ChangedFiles @ internal/tool/repomap/index/cache.go:371 (found at internal/tool/repomap/index/cache.go:1083, internal/types/change_plan.go:186, internal/types/repomap_scan.go:58)
- NeedsFullRescan @ internal/tool/repomap/index/cache.go:445 (found at internal/tool/repomap/index/cache.go:1202)
- DerivePolicy (no definitions extracted)
- Normalize @ internal/analysis/normalizer/normalizer.go:64 (found at internal/analysis/normalizer/normalizer.go:72, internal/dataquery/dataquery.go:43, internal/repl/user_mode.go:32, internal/tool/answer_document_json_repair.go:20, internal/toolparam/normalize.go:78, internal/types/pipeline_mode.go:97)
- Plan @ internal/analysis/hdp/planner.go:37 (found at internal/analysis/hdp/planner.go:45, internal/dataworkflow/action_input.go:14, internal/dataworkflow/admission.go:10, internal/dataworkflow/admission.go:20, internal/dataworkflow/deferred.go:19, internal/dataworkflow/deferred.go:57, internal/dataworkflow/evaluation.go:168, internal/dataworkflow/evaluation.go:189, internal/dataworkflow/failure.go:81, internal/dataworkflow/failure.go:98, internal/dataworkflow/failure.go:142, internal/dataworkflow/failure.go:152, internal/dataworkflow/fallback_plan.go:70, internal/dataworkflow/fallback_plan.go:86, internal/dataworkflow/plan_guard.go:44, internal/dataworkflow/plan_guard.go:367, internal/dataworkflow/prefix_fallback.go:10, internal/dataworkflow/prefix_fallback.go:15, internal/dataworkflow/prefix_fallback.go:22, internal/dataworkflow/process_event.go:15, internal/dataworkflow/record.go:8, internal/dataworkflow/runtime.go:36, internal/dataworkflow/runtime.go:62, internal/dataworkflow/runtime.go:72, internal/dataworkflow/runtime.go:82, internal/dataworkflow/stage.go:194, internal/dataworkflow/stage.go:252, internal/dataworkflow/stage.go:265, internal/dataworkflow/stage.go:312, internal/dataworkflow/stage.go:326, internal/dataworkflow/stage.go:435, internal/dataworkflow/stage.go:517, internal/dataworkflow/stage.go:533, internal/operation/plan.go:89, internal/operation/workflow.go:74, internal/repl/data_task_workflow.go:81, internal/repl/data_task_workflow.go:86, internal/repl/repl.go:5099, internal/repl/repl.go:5105, internal/repl/repl.go:5114, internal/repl/repl.go:5118, internal/types/change_plan.go:1058, internal/types/config.go:1182, internal/writeflow/evaluator.go:25, internal/writeflow/risk.go:75)
- Bind (no definitions extracted)
- CurrentTask (no definitions extracted)
- IsZero @ internal/types/context.go:69 (found at internal/dataworkflow/state_snapshot.go:131, internal/operation/workflow.go:226, internal/types/turn_route_hint.go:22)
- Classification (no definitions extracted)
- SetClassification (no definitions extracted)
- TaskList (no definitions extracted)
- SetTaskList (no definitions extracted)
- UpdateTaskStatus (no definitions extracted)
- UpdateTaskResult (no definitions extracted)
- SetCurrentTask (no definitions extracted)
- SetEmitter @ internal/orchestrator/orchestrator.go:47 (found at internal/orchestrator/orchestrator.go:576)
- SetMaxSteps @ internal/orchestrator/orchestrator.go:55 (found at internal/orchestrator/orchestrator.go:695)
- SetLanguage @ internal/orchestrator/orchestrator.go:64 (found at internal/orchestrator/orchestrator.go:704)
- BusContext @ internal/orchestrator/orchestrator.go:833 (found at internal/orchestrator/orchestrator.go:8370, internal/types/context.go:5341)
- Execute @ internal/agent/agent.go:317 (found at cmd/root.go:597, internal/agent/agent.go:1743, internal/agent/agent_executetool_busctx_propagation_test.go:25, internal/agent/agent_test.go:1290, internal/agent/agent_test.go:1423, internal/agent/agent_tool_context_test.go:30, internal/agent/agent_tool_context_test.go:54, internal/agent/explorer.go:18299, internal/agent/log_triager.go:297, internal/agent/perf_triager.go:198, internal/agent/write_analyzer.go:123, internal/agent/write_controller.go:106, internal/operation/executor.go:27, internal/orchestrator/orchestrator_test.go:28, internal/tool/apply_patch.go:100, internal/tool/builtin.go:432, internal/tool/builtin.go:1112, internal/tool/builtin.go:2848, internal/tool/builtin.go:3121, internal/tool/builtin.go:3290, internal/tool/builtin.go:3411, internal/tool/builtin.go:3551, internal/tool/builtin.go:3712, internal/tool/emit_analysis.go:905, internal/tool/emit_answer_document.go:161, internal/tool/emit_answer_document_patch.go:140, internal/tool/emit_answer_symbol.go:169, internal/tool/emit_change_plan.go:198, internal/tool/emit_evidence.go:501, internal/tool/emit_hypothesis_verdict.go:124, internal/tool/emit_investigation_complete.go:1015, internal/tool/emit_log_segmentation.go:101, internal/tool/emit_log_triage.go:270, internal/tool/emit_multi_repo_focus.go:58, internal/tool/emit_perf_segmentation.go:110, internal/tool/emit_perf_trace.go:138, internal/tool/emit_plan_change.go:82, internal/tool/emit_plan_skeleton.go:123, internal/tool/emit_test_results.go:107, internal/tool/emit_write_analysis.go:104, internal/tool/emit_write_workflow_decision.go:43, internal/tool/list_memory.go:70, internal/tool/propose_sub_agents.go:78, internal/tool/recall_memory.go:70, internal/tool/repomap/tool.go:159, internal/tool/run_tests.go:214, internal/tool/tool.go:119, internal/tool/trace_query.go:100)
- Name @ internal/agent/agent.go:219 (found at cmd/operation_provider_test.go:14, internal/agent/agent.go:525, internal/agent/agent_executetool_busctx_propagation_test.go:22, internal/agent/agent_test.go:1285, internal/agent/agent_test.go:1416, internal/agent/agent_tool_context_test.go:24, internal/agent/agent_tool_context_test.go:41, internal/agent/agent_tool_context_test.go:65, internal/agent/answer_document_evaluator.go:1634, internal/agent/explorer.go:18295, internal/agent/log_triager.go:293, internal/agent/perf_triager.go:191, internal/agent/sub_explorer.go:32, internal/agent/write_analyzer.go:121, internal/agent/write_controller.go:104, internal/analysis/dataflow/types.go:50, internal/analysis/gate/hard_gate.go:56, internal/env/probe/probe_pkgmgr.go:15, internal/hitraceconv/eventformat.go:14, internal/hitraceconv/eventformat.go:22, internal/llm/factory.go:187, internal/llm/factory.go:195, internal/llm/llm.go:11, internal/llm/llm.go:19, internal/llm/oauth_polling.go:147, internal/llm/oauth_polling.go:309, internal/llm/openai.go:1144, internal/llm/openai.go:1168, internal/mcp/mcp.go:16, internal/mcp/mcp.go:24, internal/mcp/mcp.go:31, internal/mcp/stdio.go:415, internal/operation/capability.go:37, internal/operation/capability.go:44, internal/operation/plan.go:27, internal/operation/plan.go:49, internal/orchestrator/orchestrator_test.go:26, internal/repl/command_operation_e2e_test.go:277, internal/repl/input.go:60, internal/repl/input.go:80, internal/skill/skill.go:22, internal/tool/apply_patch.go:65, internal/tool/builtin.go:416, internal/tool/builtin.go:1033, internal/tool/builtin.go:2809, internal/tool/builtin.go:3105, internal/tool/builtin.go:3272, internal/tool/builtin.go:3390, internal/tool/builtin.go:3528, internal/tool/builtin.go:3691, internal/tool/emit_analysis.go:322, internal/tool/emit_answer_document.go:35, internal/tool/emit_answer_document_patch.go:47, internal/tool/emit_answer_symbol.go:86, internal/tool/emit_answer_symbol.go:103, internal/tool/emit_answer_symbol.go:622, internal/tool/emit_change_plan.go:98, internal/tool/emit_change_plan.go:1821, internal/tool/emit_change_plan.go:2118, internal/tool/emit_change_plan.go:2190, internal/tool/emit_evidence.go:218, internal/tool/emit_hypothesis_verdict.go:80, internal/tool/emit_investigation_complete.go:59, internal/tool/emit_investigation_complete.go:2671, internal/tool/emit_investigation_complete.go:7031, internal/tool/emit_investigation_complete_relation_authority.go:13, internal/tool/emit_log_segmentation.go:43, internal/tool/emit_log_triage.go:84, internal/tool/emit_multi_repo_focus.go:21, internal/tool/emit_perf_segmentation.go:52, internal/tool/emit_perf_trace.go:105, internal/tool/emit_plan_change.go:45, internal/tool/emit_plan_skeleton.go:79, internal/tool/emit_test_results.go:56, internal/tool/emit_write_analysis.go:72, internal/tool/emit_write_workflow_decision.go:22, internal/tool/ground/claim_citation.go:20, internal/tool/list_memory.go:39, internal/tool/multipath/decision.go:303, internal/tool/propose_sub_agents.go:28, internal/tool/recall_memory.go:33, internal/tool/repomap/render/render.go:337, internal/tool/repomap/tool.go:51, internal/tool/repomap/types/types.go:74, internal/tool/repomap/types/types.go:181, internal/tool/repomap/types/types.go:470, internal/tool/repomap/types/types.go:515, internal/tool/run_tests.go:165, internal/tool/run_tests_parsers.go:1373, internal/tool/run_tests_parsers.go:1387, internal/tool/source_inventory_reconcile.go:81, internal/tool/trace_query.go:64, internal/tracequery/thread_selector.go:11, internal/tracequery/types.go:58, internal/tracequery/types.go:613, internal/tracequery/types.go:623, internal/tracequery/types.go:632, internal/tracequery/types.go:749, internal/tracequery/types.go:775, internal/tracequery/types.go:804, internal/types/analysis_ir.go:1616, internal/types/answer_aggregate_fact.go:123, internal/types/answer_surface_plan.go:241, internal/types/answer_taxonomy.go:40, internal/types/context.go:5172, internal/types/context.go:5203, internal/types/context.go:5234, internal/types/evidence.go:1341, internal/types/failure_taxonomy.go:34, internal/types/source_inventory_observation.go:69, internal/types/source_inventory_observation.go:87, internal/types/typed_relation_hint.go:57, internal/types/typed_relation_hint.go:168)
- CallersOf (no definitions extracted)
- TransitiveDeps @ internal/tool/repomap/types/graph.go:213 (found at internal/tool/repomap/types/graph.go:348)
