# Selected Eval Manual Audit Scaffold

- date: 2026-09-01T00:28:47Z
- sweep_start_ts: 20260831-172846
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_java_call_chain | FAIL | eval/results/sr_java_call_chain-20260831-172847 | primary_answer | none | 142s | 42 | read=9,repo_map=3,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=2,unavail=0,prune=0 | fail | 调查证据和调用边正确，finalizer prompt 也发布了 `AuditLog.record -> System.out.println [ev-f632b19f56f19361]`。模型正确选择 `current_terminal_differs`，但工具执行侧因 AgentContext→ToolBusContext 丢失 EvidenceItems 而把同一 pair 拒绝为未发布；最终旧稿遂错误写成“打印标准输出，完成审计落库”。这是确定性的 schema/执行合同漂移 B1520，不是模型波动。图本轮保留 countOpenVisits，但把 stdout 本地副作用画成 Audit→Repo reply，作为关系表达观察项，不以本 case 特判。 |
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-172847 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 187s | 49 | read=0,repo_map=0,list=0,trace=9,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=1 | fail | 显式 10ms 窗、9 次 typed trace_query、唤醒链、链上根因排序、实际占时/规则可消双账户、业务 span、自动补采和 Trace 因果投影均完整；邻近/背景没有进入根因序数。模型却先把优先级反转/调度供给称为“链上确定性优化工作”，又正确披露 VerifyClass 0.285ms 只有 host→target 唤醒边、缺工作完成→目标等待绑定并判 relation_unproven，答案内部矛盾。typed 上下文与系统投影均准确，归为重复模型语义波动/上下文压力观察，不新增正文关键词硬门或系统代写。无固定 4ms/4m/活动流年龄降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
