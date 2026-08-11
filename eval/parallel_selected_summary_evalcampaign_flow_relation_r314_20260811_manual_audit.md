# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T13:40:02Z
- sweep_start_ts: 20260811-064001
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_ts_workspace_chain | PASS | eval/results/sr_ts_workspace_chain-20260811-064003 | answer_regex,answer_contains | none | 147s | 23 | read=9,repo_map=1,list=4,trace=0,source_lens=0 | midloop=3,inv=3/0,fin_reject=1,unavail=0,prune=0 | fail | B530 的 principal-roster 臂再次生效：completion 从已读 AST 找到并要求补交 `dispatchOnce -> fetch`，模型随后发出该 typed edge。此次 Analyzer 却把“完整调用链”的 `CompletenessObligation.required` 发成 false，因此同语句 sibling 臂没有要求 `send -> sleep`；正文虽提到 sleep，最终自称“完整”的 6 条有向边没有这条调用。r312/r313 曾发 true，本轮为模型 typed 分类波动；暂记 B533/P2-watch，先加强软教学与异构复放，禁止扫描原始请求/答案中的“完整”做硬门，也不把所有 sibling call 无差别升成主链。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260811-064003 | answer_regex,answer_contains,mermaid_edge_count | none | 279s | 39 | read=12,repo_map=4,list=0,trace=0,source_lens=0 | midloop=7,inv=4/0,fin_reject=1,unavail=0,prune=2 | fail | B531 生产正证：Analyzer 发出的 Analyzer/Explorer/Extractor/Finalizer/Mutable/BusContext 六个 participant 全部保留，日志不再出现系统删席。Explorer 已读到 `Mutable: types.NewMutableState(request)`，却把成员初始化发成 `anchor_kind=assignment`；该行被 grounded 为普通证据，但 `AssignmentEvidenceEndpoints` 按 assignment 无法解析冒号端点，因而既无 flow authority，也没有 B529a 的分类提示。completion 最终诚实披露 Mutable/BusContext unproven，Finalizer 首稿伪边被拒后留下两个孤点，未满足用户要求的数据流。确认 B532/FLOWANCHORKIND1：必须从 typed AST 行特征给出 initializer/assignment 的精确重发形，不得自动升格或代画。279s 活跃流跨过四分钟后仍返回模型答案，B517 无降级正证。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
