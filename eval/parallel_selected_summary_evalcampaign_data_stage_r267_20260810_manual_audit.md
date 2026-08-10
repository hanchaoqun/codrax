# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T09:28:25Z
- sweep_start_ts: 20260810-022824
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260810-022825 | log_regex,answer_regex | none | 605s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 输出 `20,0,5`，应为 `17,0,5`。`filter_records(active != inactive)` 未把 `inactive` 与输入布尔值 `false` 归为同一值，r3 被错误纳入；后续 decision/contribution/reconcile 只验证错误候选集内自洽。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260810-022825 | answer_regex,answer_contains,mermaid_edge_count | none | 758s | 38 | read=10,repo_map=4,list=0,trace=0,source_lens=1 | midloop=8,inv=2/0,fin_reject=14,unavail=0,prune=0 | fail | runner 只数任意 Mermaid 边而假绿。主图仅有 `dispatchStage -> BuildAgentContext` 和阶段 precedence；BusContext 未连、Mutable 缺席，用户要求的数据流未形成。B456 已使 skipped item 局部重发；B452/B457 又被 `Analyzer agent` / `Mutable (in BusContext)` 等 typed 展示标签与 canonical code identity 不等价阻断。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- human correctness: **0 / 2**；runner correctness: **1 / 2**。QF 的 `mermaid_edge_count` 仍不能证明被点名参与者及其数据流已覆盖。
- data 根因是通用布尔域等价缺口，不是模型算术波动；`active/inactive` 与 `true/false` 必须在 typed filter 比较层等价，不能依赖 action purpose 或用户原文。
- QF 的 B456 production 正证成立：批内合法证据没有被重建，被跳过关系项获得局部修复并被模型重发。B452/B457 未闭环的共同前置是 typed participant 展示身份缺少到 typed entity/code identity 的软解析。
- “系统保留内容”明确标为未校验模型参考，且在严格成文最终稿之外；它实现此前约定的失败内容保全，不计作已校验证据，也没有替换模型最终稿。本轮不据此放宽关系门或新增系统结论。
