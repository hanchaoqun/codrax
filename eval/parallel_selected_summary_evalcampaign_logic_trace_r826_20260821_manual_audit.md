# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T21:02:53Z
- sweep_start_ts: 20260821-140253
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-140254 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 256s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 3 次目标化查询覆盖 4 个维度族；显式窗、四节点链、逐跳 CPU、11.000ms 链上 IO 第一席、3 个独立 1.000ms 候选、实际占时/可消双账、链外背景隔离、自动补采和完整 Trace 因果投影均在。finalizer 0 拒绝、无固定时限降级。 |
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260821-140254 | answer_regex,answer_contains,mermaid_edge_count | none | 859s | 63 | read=16,repo_map=4,list=0,trace=0,source_lens=1 | midloop=26,inv=6/0,fin_reject=20,unavail=0,prune=0 | fail | 20 次成文拒绝后降级展示未通过校验的旧稿。参与者门要求把 typed candidate 的技术端点映射到可见 `Mutable` 且保留 anchor identity；通用 node/identity 门却把同一 `Mutable(bus.Mutable.Objective)` 形判为冲突，模型在 call/data_flow 候选间循环。确认为 B1311 跨门合同互斥，不是模型波动。B1310 的 no-lease 形本轮未形成生产正证；一次 stale ref 发生在另一 live generation。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- `r826=runner-pass-1/2,human-trace-pass+read-system-fail`。
- Trace 红线守护通过：根因仍只来自已证链上，优先级/调度/IO 与业务下钻信息未丢失，邻近和背景不参与加冕；系统未改写模型结论。
- P0 `B1311-PARTICIPANTCARRIERIDENTITYCONTRACT1`：同一 request-scoped typed candidate 在 participant coverage 门是必选合法形，在 generic node/identity 门却是必拒形。该矛盾烧尽 20 轮并触发恢复旧稿。
- 根修边界：只允许候选精确 relation/from/to/side 对应的 exact participant node ID 或 exact parsed participant label 作为显示 carrier；普通无候选边、反向 tuple、类型-only 节点和业务别名继续 fail-closed。系统不选边、不改方向、不写标签。
