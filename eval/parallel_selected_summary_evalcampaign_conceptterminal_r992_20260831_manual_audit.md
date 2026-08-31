# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T22:58:50Z
- sweep_start_ts: 20260831-155848
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-155850 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 173s | 44 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | 显式 10ms 窗、typed 查询、链上排序、双账户、Trace 因果投影和自动补齐均保留；但模型可见回答漏掉被单独询问的 VerifyClass 关系结论。模型随后提交精确 `runtime_work_relation` receipt 的原子字段修补，被 `field_not_published` 拒绝；系统投影虽正确披露 relation-only/unproven 与规则可消 0，也不能代替模型完成该子问。确认 B1518：富 typed receipt 缺少保留其余 block 字节的原子 patch 通道。 |
| 1 | sr_java_call_chain | FAIL | eval/results/sr_java_call_chain-20260831-155850 | primary_answer | none | 289s | 46 | read=8,repo_map=1,list=0,trace=0,source_lens=0 | midloop=11,inv=4/0,fin_reject=7,unavail=0,prune=0 | fail | B1515 在生产触发，模型独立选择 `AuditLog.record -> System.out.println` 及概念终点结论；但该精确 parser row 的 producer 是 `repomap_selected_callable_body_call`，而 schema/prompt 只发布 `repomap_terminal_body_call`，receipt 被 exact-pair 门拒绝。更深层是 body enrichment 被误并入主调用拓扑，反而把 AuditLog 从 leaf owner 清单移除。最终答案虽列出 stdout hop，却没有明确完成“stdout 并非持久化终点”的结论；模型还出现证据与所选 conclusion 自相矛盾。确认 B1516 候选拓扑污染与 prompt/contract 候选谓词漂移；7 次成文拒绝中的 guard/relation/diagram 抖动另记 B1517，不与本批拟合修复。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
