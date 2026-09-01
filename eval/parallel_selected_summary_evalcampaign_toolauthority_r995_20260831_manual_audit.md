# Selected Eval Manual Audit Scaffold

- date: 2026-09-01T00:51:56Z
- sweep_start_ts: 20260831-175154
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_java_call_chain | FAIL | eval/results/sr_java_call_chain-20260831-175156 | primary_answer | none | 234s | 48 | read=10,repo_map=2,list=0,trace=0,source_lens=1 | midloop=5,inv=2/0,fin_reject=6,unavail=0,prune=0 | fail | B1520 获生产正证：prompt 发布的 `AuditLog.record -> System.out.println [ev-f632b19f56f19361]` 在执行端不再报未发布，模型也明确选择 `current_terminal_differs`。最终仍回退错误旧稿，是随后 patch 同时用完整 `replace_blocks` 和被兼容层重映射的局部 receipt edit 修改 `summary-1`；两者携带完全相同 pair，却被结构层当冲突拒绝。确认为通用 B1521。另有 B1522：同一 `chain-1` 同时有关系锚失败与 item evidence_ids 失败时，关系租约禁整块替换，模型需先原子移除关系再下一轮整块修 evidence，教学未明确顺序，造成 4 轮额外重试；独立记账，不按 Java/println 特判。 |
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-175156 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 258s | 47 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=2,inv=2/2,fin_reject=1,unavail=0,prune=0 | fail | 显式 10ms 窗、typed 查询、唤醒链、链上根因排序、实际占用/规则可消双账户、业务 span、自动补采与最终 `Trace 因果投影` 均保留；邻近/背景没有进入根因排序，也无固定 4ms/4m/活动流年龄降级。模型正文却把裸唤醒链扩写成“上游工作完成依赖”，并从 `page_lock_timeout` 推到具体文件缓存竞争，超过系统同时发布的“调用点不证明对象/持有者/后端”和 semantic-work relation unproven 边界。typed 上下文与系统投影准确，判为模型语义过推；不增加正文关键词硬门或系统代写。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
