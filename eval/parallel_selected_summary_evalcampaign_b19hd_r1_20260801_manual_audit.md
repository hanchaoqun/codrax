# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T11:30:01Z
- sweep_start_ts: 20260801-042959
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_c2_dstate_iowait | PASS | eval/results/real_trace_c2_dstate_iowait-20260801-043001 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 143s | 30 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=1,prune=0 | pass | Typed 主结论与完整清单一致：3 段、0.635ms、caller 均正确；D-state/io_wait/S-state IO 为互斥记账分栏，并明确不推导内核标签包含关系。模型探索中的 19.671ms 错算没有进入最终答案；窄答案未发布因果投影、根因榜、全量钻取或缺失唤醒者 caveat。系统补采 provenance 与输出维度核对仅作来源/展示说明，不扩张结论。 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260801-043001 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 156s | 38 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式时间窗仍走完整报告：typed unproven 主结论、主要占用/新修向与现规则可消除量双轴、板内根因候选排序、唤醒链、代表窗、Trace 因果投影、证据索引和系统自动补采均保留。模型草稿中的确定性因果过 claim 未进入最终答案；B19h-d 的窄事实接管和 caveat 过滤没有外溢到该车道。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
