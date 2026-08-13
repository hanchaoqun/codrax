# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T04:22:56Z
- sweep_start_ts: 20260812-212251
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260812-212257 | answer_regex,answer_contains | none | 183s | 26 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | B703 生产闭环：Analyzer 铸出 required `role=source_location`，current-source lane 接管“所在文件”；终稿保留 12 条 implementation→interface 边，且每个实现者自己的表格行都显示准确 `internal/agent/*.go:line`，不是只藏在引用区。第一次 completion 把方法行 evidence 与类型定义 support_ref 混槽，精确 grounding 校验要求重新提交后通过；这是正确拒绝，不是合同冲突，finalizer 零拒绝。 |
| 1 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260812-212256 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 219s | 44 | read=2,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=2/1,fin_reject=0,unavail=0,prune=0 | partial | B704 生产闭环：独立“确定性语义优化”表已显示 VerifyClass 原始墙钟 0.285ms、规则可消 0.285ms、2.8%，与根因 #2/修向/树/证据一致。显式 10ms 窗、typed 唤醒链、链上-only 排序、实际占用与规则可消双轴、frame absent 边界均正确；邻近/背景未晋升。新发现 B705：模型分页读取 trace_query 私有 spill 后，共享枚举权限把绝对 `.codrax/blob/.../trace_query-*.txt` 路径误当 `compacted_view/scope` 写入模型上下文和最终覆盖块；数值边界诚实但内部存储身份不应成为业务 view。一次 `emit_evidence` 对 runtime artifact 的拒绝正确，未导致答案降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
