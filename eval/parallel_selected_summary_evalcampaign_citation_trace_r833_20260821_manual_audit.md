# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T00:55:00Z
- sweep_start_ts: 20260821-175500
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-175500 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 188s | 39 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 窗、四线程三跳唤醒链、11.000ms 链上 IO 第一席、三个独立 1.000ms runnable/优先级候选、实际占时/规则可消双轴、链上业务下钻和完整 Trace 因果投影均保留；邻近/背景未升为根因，活动流未按固定 4ms/4m 降级。 |
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260821-175500 | answer_regex,answer_contains,mermaid_edge_count | none | 1147s | 70 | read=31,repo_map=2,list=0,trace=0,source_lens=0 | midloop=23,inv=9/0,fin_reject=20,unavail=1,prune=0 | fail | B1315 生产正证：最终引用只保留真实在用坐标，旧 `SetResult→4602` 与 `context.go:51` 均消失。但同一图关系合同经历 20 次拒绝/19 次 patch 后降级旧稿。精确复盘确认：混合 `typed_anchor_without_visible_edge` 与 `missing_relation_anchor` 中，一条 body-edge 只解析出单侧 technical identity；producer 把该半 identity 填入 failure locator，整份 relation delta 因 fail-closed 合并规则被清空，只剩 participant addition。系统随后宣称 preserve-all 会刷新 lease，却再次只给 participant 车道，模型进入无 ref 的旧式坐标修补循环。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit conclusion

- Runner 结果为 1/2，整批不能收账为通过；read 的降级答案只是旧结构稿恢复，不代表关系合同闭环。
- `B1315-CITATIONSUBJECTLINECONSISTENCY1` 已获生产正证：单条错 evidence ID 与无人使用的 inherited citation 都没有再进入最终文档。
- 新确认 `B1317-HALFIDENTITYRELATIONDELTA1/P1`：可见 body edge 有完整 node pair、但 validator 只解析到一侧 technical identity 时，半 identity 使整代 relation delta 原子失效，连同其他完全可执行的 stale/missing-anchor failures 一并丢失。这是 typed carrier 构造 GAP，不是模型波动。
- 最小泛化修向：仅当完整 node pair 没有任何 prior anchor candidate 且 technical identity 恰为半对时，producer 删除这两个可选 identity 字段，把能力收窄为 node-local remove-only；不补全、猜测、反向或新建关系。其他不完整 locator 继续 fail-closed。然后保留现有 joint-delta/lease 路由，使 preserve-all 重验能真正发布当前代 refs。
