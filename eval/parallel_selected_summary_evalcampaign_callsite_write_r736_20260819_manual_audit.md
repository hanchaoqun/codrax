# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T16:33:14Z
- sweep_start_ts: 20260819-093313
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-093314 | log_regex,write_apply,answer_regex,answer_contains | none | 636s | 26 | read=8,repo_map=8,list=0,trace=0,source_lens=1 | midloop=2,inv=0/0,fin_reject=0,unavail=1,prune=0 | fail（确定性终局矛盾） | 原改动已经存在，planner 的行为探针与 `make check` 均通过；最终 controller 上下文明确显示三个 batch 全部 `state=complete`、最新报告 `passed_results=3/failed=0`，模型据此发出 `finish_disposition=all_verified`。但终局仍把早先 `batch-1-cumulative-review` 的 `verification_proof_incomplete` 当作永久未闭合，输出“未完全验证”。更早的精确 proof-planning normalizer 没接管模型的 `append_batch`：`controllerActionInterruptsUnverifiedCompletion` 漏掉 append/split，使模型自建普通验证批、丢失 controller-owned purpose/depends_on，并被 schema 逼出一个无意义 source patch。这是 B1174，不得用降低验证杆或按任务文字特判修复。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-093314 | answer_regex,answer_contains,mermaid_edge_count | none | 809s | 39 | read=16,repo_map=2,list=1,trace=0,source_lens=0 | midloop=7,inv=5/0,fin_reject=2,unavail=0,prune=1 | partial | B1171 后读取量从 r735 的 25 降到 16、Explorer 迭代从 39 降到 21，但 requested spine 仍为 `unproven`。最终图依然只有四阶段 precedence 与 `BusContext -> dispatchStage`、`Mutable -> answerDocumentV2` 等局部片段；真正 `dispatchStage` 内 `agentCtx -> Execute -> output -> applyStageOutput` 的跨阶段共享状态流没有进入证据/答案。日志显示 landing 仍只追加读取 `internal/context/builder.go`，早期 grounded body/handoff fast path 在新 callsite continuation 排序前返回；runner 只验名词和最小边数而假绿。B1171 记 production-failed-at-objective，下一步把同一个 parser-owned quality rank 接到所有抢先返回的调用点候选，不造边、不代写答案。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工结论与下一批

1. `B1171-CALLSITEWHOLEVALUECONTINUATIONRANK1` 的单测实现正确，但 r736 没有闭环用户请求。新排序只覆盖末级候选集；`flowNavigationGroundedBodyOperationCallerHandoffReadTarget` / `flowNavigationGroundedHandoffCalleeOperationReadTarget` 可在它之前返回，继续选中局部 builder/precheck。根修是让所有同 callee 调用点选择复用一份 parser-owned continuation quality，不按函数名、文件名、语言或 case 特判。
2. `B1174-WRITEPROOFFOLLOWUPTERMINALRESOLUTION1` 为本批最高优先级。第一层：typed proof-planning decision 必须优先于模型自拟 append/split，防丢 controller-owned `purpose/depends_on/changes=[]` 权威。第二层：一个严格依赖于未闭合 verify-only proof batch 的后继 proof batch，只有在自己的 typed proof ledger 为 verified 且 missing/unavailable/failed 均为 0 时，才能把该直接前驱标为“被后继补证闭合”；普通批、无依赖、不同 purpose 或弱证明一律不能洗绿。
3. 写模式的 3 项测试是真通过，终局矛盾也是真 gap；修复不得把任意“后续测试绿”当成覆盖旧失败。必须依靠 `DependsOn + controller-owned proof purpose + exact ledger` 三重精确信号。
4. QF 最终 Mermaid 合法且系统没有铸边，但模型把阶段职责箭头当数据流展示，人工只能 partial。系统应继续提供精准关系材料和缺口边界，不能替模型画完整架构图。
5. 两案均无 active-stream 4ms/固定累计 age 降级；QF 的 `answer_document_blocks_string_recovery_events=1` 被有损恢复但最终有答案，另列容错观察，不作为本批硬门。Read/Write 本批均未触碰 Trace 查询或因果投影。
