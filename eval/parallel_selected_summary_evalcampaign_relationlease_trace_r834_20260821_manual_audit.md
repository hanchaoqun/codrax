# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T01:32:15Z
- sweep_start_ts: 20260821-183213
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-183215 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 292s | 39 | read=0,repo_map=0,list=0,trace=12,source_lens=0 | midloop=1,inv=3/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 窗、四线程三跳唤醒链、11.000ms 链上 IO 第一席、三个独立 1.000ms runnable/优先级候选、实际占时/规则可消双轴、业务下钻、背景隔离和完整 Trace 因果投影均在；0 次成文拒绝，未按固定 4ms/4m 或上下文比例降级。 |
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260821-183215 | answer_regex,answer_contains,mermaid_edge_count | none | 1228s | 78 | read=28,repo_map=4,list=0,trace=0,source_lens=0 | midloop=34,inv=11/0,fin_reject=20,unavail=0,prune=1 | fail | B1317 生产正证：混合 relation failures/current refs 已发布，不再出现 failures=null。新系统 GAP：一轮 patch 成功消费旧租约后，validator 发布新 addition-only 活动租约；下一轮使用旧 ref 时，工具正确返回当前活动租约完整 delta，但 evaluator 以当前草稿重建租约，候选因已在草稿出现而被过滤为空，随后丢弃工具原始 live delta，退回只说“重新读取”的通用提示。历史中存在多代 refs，模型无法辨认当前代，连续 stale ref 至 20 次拒绝并降级旧稿。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case conclusions

- `B1317-HALFIDENTITYRELATIONDELTA1` is production-positive: the read replay reached non-empty mixed relation/participant deltas with live failure/addition refs. The prior whole-generation atomic drop is gone.
- `B1318-LIVERELATIONDELTAFORWARD1/P1` is confirmed. `emit_answer_document_patch` already returns the exact active lease in `answer_doc_relation_repair_scope`; the evaluator must render that producer-owned delta directly. Reconstructing it from a different patch-base generation is a non-equivalent second authority and can erase an additions-only capability set.
- The repair must not select an addition, edge, endpoint, label, action, layout, or conclusion. It should only repeat the exact current tool-owned capability roster and continue to fail closed for malformed deltas.
- Trace remains the guard case: no source-diagram retry change may alter explicit-window selection, trace auto-supplement, on-chain-only ranking, dual-axis accounting, business clues, causal projection, or active-stream timeout behavior.
