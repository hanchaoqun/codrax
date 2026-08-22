# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T11:36:15Z
- sweep_start_ts: 20260822-043614
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-043615 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 194s | 38 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 明确窗、四跳唤醒链、11.000ms 链上 IO 第一席、3 个独立 1.000ms 优先级反转候选、实际占时/规则可消双账和完整 Trace 因果投影均保留；邻近睡眠、CPU 占用与 IO 综合指数没有进入根因排序。模型把目标 sleep 称为“协作式等待”仍是未获独立机制证据的软措辞观察，不以正文关键词硬拒或由系统改写。 |
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260822-043615 | answer_regex,answer_contains | none | 242s | 30 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=3/0,fin_reject=3,unavail=0,prune=0 | uncertain | 最终 JsonPlugin、run_pipeline→resolve、注册表查找、MRO 与 executor callback 主结论基本正确；但模型连续把 return/动态选择误标为 call、把 callback 端点/类型猜错，耗 3 次拒绝才收敛。精确 typed recipe 已在上下文却未在局部错误提示中重投影，确认 B1339；本轮未画图，故未自然触发 B1337 同 patch lease 或 B1338 Note 引用活性，二者仍只有单测闭环。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
