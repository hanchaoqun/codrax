# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T03:50:38Z
- sweep_start_ts: 20260812-205037
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260812-205038 | answer_regex,answer_contains | none | 128s | 25 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Mermaid 图完整保留 12 条 production implementer→LoopController 关系，方向和语法正确；但用户明确要求的“每个实现类型所在文件”没有进入逐成员表格，表只有类型/职责两列。12 个路径仅存在引用区，引用证明不能替代可见位置列。Analyzer 将该维度铸成 `role=other`，使下游没有逐成员 location 合同。另有一次 `blocks` JSON-string 容错恢复，恢复完整且未降级；已有 JSON 教学明确禁止字符串包裹，按模型波动/恢复正证记录，不新增硬门。 |
| 1 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260812-205038 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 152s | 37 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 显式 10ms 窗、五态账、typed 唤醒链、优先级反转、调度/算力供给、VerifyClass、业务线索及实际占用/规则计价双轴均在；邻近/背景未晋升主因，frame absent 边界诚实。跨面一致性有一处确定性缺口：VerifyClass 0.285ms 已在根因榜、修向榜、树和证据面发布为规则可消，但专门“确定性语义优化”表显示 `—`；rank carrier 的有效归因没有回填到独立 semantic display copy。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
