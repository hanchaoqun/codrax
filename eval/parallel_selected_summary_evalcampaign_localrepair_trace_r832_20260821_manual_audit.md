# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T00:26:30Z
- sweep_start_ts: 20260821-172628
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-172630 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 154s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 窗、四线程三跳唤醒链、11.000ms 链上 IO 第一席、三个独立 1.000ms runnable/优先级候选、实际占时/规则可消双账户、业务下钻和完整 Trace 因果投影均在；邻近/背景没有升格为根因，活动流未按固定 4ms/4m 降级。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260821-172630 | answer_regex,answer_contains,mermaid_edge_count | none | 427s | 44 | read=16,repo_map=5,list=0,trace=0,source_lens=0 | midloop=14,inv=6/1,fin_reject=6,unavail=1,prune=0 | uncertain | Runner 通过但人工判 partial：拒绝由 r831 的 13 降至 6，大小写等价的阶段 addition 不再重复发布，证明 B1316 去重臂生产生效；最终图仍只保留阶段链和 BusContext→BuildAgentContext，Orchestrator/Mutable 关系表达偏薄。patch 已把条目切换到正确 evidence_ids，但继承 citation pool 仍渲染无人使用的伪造 `internal/types/context.go:51`，并出现同一 failure_ref 重复选择、代次更新后复用 stale ref、重复 remove 操作等修补心智浪费。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- B1316 production result: positive for case-insensitive exact existing-anchor suppression; the three duplicate stage additions seen in r831 disappeared. The missing-kind, evidence-negative remove-only, structural-replace hidden tuple, and multiline-label branches did not naturally trigger, so their production status remains test-closed rather than inferred from this run.
- New/cross-run citation witness: the initial model pool contained `internal/types/context.go:51` with an invented `type Orchestrator struct {` quote. Source verification correctly replaced the quote with the real line, and a later patch correctly rebound the visible item through evidence IDs, but the now-unused inherited citation slot remained in the final bibliography. This joins r830's `SetResult` item bound to the `SetExploreBudget` evidence ID: exact typed mismatch is detected and a unique correct candidate is available, but the current path only advises and still renders the wrong binding.
- General repair target B1315: for a single explicit evidence ID that conflicts with a symbol-like structured item label, rebind only when exactly one citable accepted evidence row matches that typed identity; ambiguity remains advisory/fail-closed. After a merged patch, compact unused citation slots and remap hidden indexes before persist. Neither arm edits model text, item selection, diagram relations, or conclusions.
- Do not hide the thin graph by system-authored edges. Remaining participant/relation expressiveness and exact duplicate patch operations stay separate observations for later heterogeneous replay.
