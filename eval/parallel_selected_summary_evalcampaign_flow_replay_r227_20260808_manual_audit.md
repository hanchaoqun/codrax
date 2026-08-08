# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T22:05:56Z
- sweep_start_ts: 20260808-150555
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260808-150556 | answer_regex,answer_contains,mermaid_edge_count | none | 222s | 33 | read=14,repo_map=3,list=0,trace=0,source_lens=0 | midloop=11,inv=2/0,fin_reject=2,unavail=0,prune=0 | fail | B380 的 named-only 教学部分生效：Analyzer 发出六个真实 identity、无占位符；但把用户明确要求画出数据流的 analyzer/explorer/extractor/finalizer/Mutable/BusContext 全部标成 `context_only`，typed coverage 因而不要求任何 incident relation。最终正文仍宣称完整 agent→Mutable/BusContext 流，图只剩 `runAnalyzePhase→dispatchStage→BuildAgentContext` 两条局部 call。拒绝 7→2、耗时 422→222s，但答案仍不完整。第二轮相同 alias pair 的 canonical symbol 被括号/引号污染，B381 仅 canonical-symbol 指纹未命中。 |
| 1 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260808-150556 | answer_regex,answer_contains,mermaid_edge_count | none | 412s | 36 | read=29,repo_map=3,list=0,trace=0,source_lens=1 | midloop=18,inv=9/0,fin_reject=1,unavail=0,prune=0 | fail | 主任务的四阶段 Mermaid 主链和逐阶段职责已正确，且只用 1 次 surgical reject 删除无证 pre-stage 支线；但答案头部引用 `internal/types/enums.go:3` 的过期注释，错误宣称“2026-04-14 后 codrax 是只读分析工具”，与当前 write Auto Pilot 架构矛盾。Analyzer 也把四个关系主节点全标 `context_only`，并从预扫描推导了当前请求未逐字点名的身份，说明 participant role/identity 教学仍只有软约束。整体答案含架构事实错误，人工不签绿。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch verdict

- Runner 2/2 PASS，人工 0/2。diagram 主链已真正恢复，但一案仍缺所求数据流，另一案被当前源码中的过期架构注释污染；不能只按图边与关键词签绿。
- B380 部分生产生效：没有再出现 `Stage 1`/`Actor A` 占位符，明确点名案也稳定发出六个真实 identity；但两案把所有关系主节点错标为 `context_only`。确认 `EVAL-B382-DIAGRAMPARTICIPANTROLEBOUNDARY1=P1/HIGH`：单源教学必须明确“只要当前所求关系/数据流要求展示某 participant 的连接，它就是 incident_required，即使它也是 component/state container；context_only 仅用于明确不要求连接的外围边界”。仍只作 soft planning，不加请求原文硬门。
- B381 为 partial：拒绝总数 18→3、总耗时 1161→634s，但没有观察到 repeat guidance。日志证明同一 Mermaid alias pair 在两轮中保持稳定，而 canonical symbol 因显示括号/引号变化漂移。最优补强是同时铸造 `canonical-symbol pair` 与 `verbatim parsed alias pair` 两个独立 typed hash，任一重复即可提示；不得模糊匹配标签文本。
- 新确认 `EVAL-B383-STALEARCHITECTURECOMMENTAUTHORITY1=P0/HIGH`：`internal/types/enums.go` 和若干 2026-04-14 遗留注释把“read-mode 主流水线只读”写成“Codrax 整体 read-only”，Explorer 将其作为 direct evidence，模型据此在答案头部做出与当前 write mode 冲突的结论。这不是模型幻觉；应统一改为 scope-accurate 的 read-mode 描述，并加当前 write-mode existence pin，避免源码注释继续污染模型上下文。
- 本批无 malformed JSON、无系统替写模型答案、无 Trace 查询、无原始请求/思考/摘要/答案关键词硬门。系统 stage-binding 补充保持并置且不改模型结论；Trace 显式窗、自动补齐、因果投影、链上主因和链外背景边界均未进入本批路径。
