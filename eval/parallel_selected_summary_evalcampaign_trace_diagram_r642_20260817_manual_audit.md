# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T21:12:23Z
- sweep_start_ts: 20260817-141221
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260817-141223 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 282s | 38 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 用户指定主窗为 2.000000..2.020000s，但三个根因视图均查询到 2.021000s。精确窗树正确把宽窗排名席降为 context_only，因而总览显示“无同尺持值行”、E6/E9 不参与排序；头行却从未过滤的 PrimaryRootCauses 回退重新加冕同一 11.000ms，形成确定性自相矛盾。Trace 因果投影、typed 唤醒链和活跃流均在，问题不是模型波动或 4ms 降级，而是精确窗补齐缺席加发布面不同源。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-141223 | answer_regex,answer_contains,mermaid_edge_count | none | 619s | 37 | read=27,repo_map=4,list=0,trace=0,source_lens=0 | midloop=16,inv=7/0,fin_reject=2,unavail=0,prune=0 | partial | B1012 在生产回放中生效：BusContext 的真实 argument_flow 候选进入最终图，未再误导到 forcedReadCancelled/getter。模型首稿反向绘制 Mutable 实参边并增加无证边，后两次修补最终保留 BusContext 已证边和 Mutable 未证边界；正文仍声称 Mutable 贯穿数据流而图中断开。门的两次拒绝均有结构理由，尚不能证明合同自冲突，但 619s、16 次中途提示和两次成文拒绝说明关系组合与模型心智负担仍高。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
