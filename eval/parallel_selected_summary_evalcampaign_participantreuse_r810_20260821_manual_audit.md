# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T12:05:54Z
- sweep_start_ts: 20260821-050554
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-050554 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 207s | 37 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 系统面完整：显式 2.000..2.020s、4 节点唤醒链、链上 11.000ms IO 首席、三个各 1.000ms 反转候选、主要占时/规则可消双账、背景隔离与 Trace 因果投影均在；没有固定 4ms/4m 降级。模型也披露反转缺少锁持有证明、fscache 只定位调用点；但“下游链路整体延迟约 11ms”和“跨 CPU 传递本身带来调度开销”仍比 typed 关系/计量权限略强，按既有 B1269/B1271 软引导残余记 partial，不由系统改写结论。 |
| 1 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260821-050554 | answer_regex,answer_contains | none | 1279s | 68 | read=26,repo_map=4,list=0,trace=0,source_lens=0 | midloop=14,inv=4/0,fin_reject=20,unavail=0,prune=7 | fail | B1292 未获生产触发，流程先被新的 typed repair capability 自冲突卡死：同一 Phase→Fin body occurrence 同时发布 call/precedence 多个失败 ref，却禁止同事务消费；只消费一个又使另一失败残留。lease 还向非 diagram 的 ordered_list 发布 relation additions，而局部执行器仅接受 diagram carrier，整块替换又被 local-scope 门拒绝。19 次 patch/20 次拒绝后降级展示第一稿，图未经终验；前置探查和主探索还重复重建同一证据链，39 explorer iter、26 reads、68% context。活动模型流一直保留，失败不是固定时限降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
