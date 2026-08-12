# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T11:29:24Z
- sweep_start_ts: 20260812-042923
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h10_spantop_member_subrows | PASS | eval/results/real_trace_h10_spantop_member_subrows-20260812-042925 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 95s | 40 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B638 主修正向：Finalizer typed context 和模型正文都看见同进程 Jit thread pool 的 2 条成员，1.781ms/行5969..6114 与 0.607ms/行12611..12664；不再宣称全窗没有 JIT，并明确不是 CompThread 自身。残余两件：同一 semantic family 从探索/补齐各出现一次；模型把第二条表格名称误抄成第一条并从“非目标线程”越推为“不影响目标执行流”。因此用户要求的逐 span 名称仍未完全正确，不能判人工绿。 |
| 1 | real_trace_h11_cross_direction_overlap | PASS | eval/results/real_trace_h11_cross_direction_overlap-20260812-042925 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 156s | 45 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B637 主修正向：42.193ms broad-envelope 伪 overlap 已从模型上下文和正文消失。残余为决策上下文消费失败：摘要称四方向相互独立，关系段又说独立/重叠均未证；模型擅自求出锁方向18.853ms、IO方向8.622ms及跨方向约31.4ms，尽管 typed 合同没有这些 subtotal；还把 policy ceiling 存在描述成证明热/功耗治理约束。链上/邻近/背景及占用/规则计价双轴仍正确保留，但关系和修向值口径未达可交付标准。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
