# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T21:28:41Z
- sweep_start_ts: 20260808-142840
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260808-142841 | answer_regex,answer_contains,mermaid_edge_count | none | 422s | 34 | read=8,repo_map=2,list=0,trace=0,source_lens=0 | midloop=6,inv=3/0,fin_reject=7,unavail=0,prune=0 | fail | Analyzer 的 `diagram_hint` 完全漏发新 participants 字段，Explorer 因而没有收到六个用户点名组件的 typed 调查义务。最终正文宣称完整四阶段 agent→Mutable/BusContext 数据流，图中却只有 `runTaskGraph→runReadSchedulerLoop` 和 `dispatchStage→BuildAgentContext` 两条互不连通的局部 call，Analyzer/Explorer/Extractor/Finalizer/Mutable/BusContext 全部是孤立节点。7 次拒绝均在无证 stage/容器桥和删边之间消耗；runner 的“一条边”oracle 再次误签。 |
| 1 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260808-142841 | answer_regex,answer_contains,mermaid_edge_count | none | 739s | 34 | read=4,repo_map=1,list=0,trace=0,source_lens=1 | midloop=8,inv=2/0,fin_reject=11,unavail=0,prune=0 | fail | Analyzer 虽发 participants，却把未点名的抽象数量造为 `Stage 1`…`Stage 4`，并把 `codrax read-mode pipeline` 标为 context-only；typed carrier 安全地没有据此生成边，但也无法引导真实 `StageAnalyze`…`StageFinalize` 取证。最终图只剩 `runAnalyzePhase→dispatchStage` 一条局部 call，四阶段主链缺席；11 次拒绝在同一批无证 `dispatch→stage`、call/precedence 重标和删边之间振荡。系统还附出近乎重复的第一稿，B369 保持开放。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch verdict

- Runner 2/2 PASS，人工 0/2；`mermaid_edge_count>=1` 再次证明只能判图非空，不能证明用户要求的主关系被表达。
- `EVAL-B379-DIAGRAMMULTISURFACEIDENTITY1` 保持生产闭环：本批所有 validator endpoint 都使用 canonical identity，没有显示标签污染。
- `EVAL-B377-FLOWPARTICIPANTROLE1` 的载体传递与“不自造边”边界正确，但 Analyzer 教学未与 schema 同步：一案漏发，一案发占位身份。确认 `EVAL-B380-DIAGRAMPARTICIPANTTEACHING1=P1/HIGH`，应把可选 participant 规则并入既有 `diagram_hint` 单一教学源；只复制当前请求明确点名的 identity，未点名时省略，禁止 `Stage 1` / `Actor A` 等占位符。
- 确认 `EVAL-B381-RELATIONRETRYOSCILLATION1=P1/HIGH`：同一个 unsupported endpoint pair 在 `call` / `precedence` / missing-anchor 间重标不会产生新证据，却被当作新修复持续重试。最优修复应基于 typed violation key 记录本轮已失败的 pair+relation family，重复时明确要求删除该可见边或继续取证；不得扫描模型原文、不得自动删图或放松证据门。
- 两案均无 malformed JSON、无 Trace 查询、无系统替换模型答案，也没有请求/思考/摘要/答案关键词硬门。系统补充仅并置 typed stage binding；Trace 显式窗、自动补齐、因果投影与“主因只能来自 typed on-chain，链外仅背景”均未进入本批路径。
