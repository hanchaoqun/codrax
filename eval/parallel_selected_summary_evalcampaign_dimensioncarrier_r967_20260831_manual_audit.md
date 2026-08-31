# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T11:00:09Z
- sweep_start_ts: 20260831-040007
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-040009 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 212s | 44 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | 显式 10ms 窗、目标状态分区、NetworkService→CookieMonster→目标的两跳 typed 唤醒链、链上可消除排序、实际占时/规则可消双账户、业务线索与邻近/背景隔离、系统自动补采及最终 Trace 因果投影均完整。VerifyClass 0.285ms 仅作为链上宿主线程的确定性工作线索展示，completion→target-wait 仍标未证且规则可消为 0.000ms，没有挤入根因排序。第一次成文只因模型把 summary-only 的 `trace_causal_claim_caliber` 放入 table 被精确拒绝，第二轮删除非法字段即接受；这不是互斥合同或固定时限降级。 |
| 1 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260831-040009 | typed_inventory_rowset,dimension_substring,answer_contains | none | 704s | 46 | read=0,repo_map=4,list=0,trace=0,source_lens=4 | midloop=5,inv=6/3,fin_reject=11,unavail=6,prune=0 | fail (system contract conflict) | B1484 的 requested-dimension 重复表格误导未复现；新 P0 B1485 确认。首稿确有把 native_add 重复算入 extend、形成 13 行的模型错误，校验器正确拒绝；模型随后已收敛到 exact 12 行（extend=2、foreign func=2、public class=8）。但 Animal/Service 的 authoritative principal row 均写明 `surface_family=public class`，per-member bucket gate 却连续 11 轮要求同一 row_id 显示内部通用桶 `source inventory principal rows`，导致一个正确答案不可能通过并进入 degraded fallback。根因是合成全局 aggregate 的 SetLabel 覆盖 typed principal-row family；修复改为只用 exact typed file:line+family 投影行局部 bucket，并保留真实的模型/用户 bucket。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
