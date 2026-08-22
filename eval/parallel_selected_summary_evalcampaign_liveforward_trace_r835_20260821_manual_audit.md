# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T02:05:48Z
- sweep_start_ts: 20260821-190548
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-190548 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 234s | 41 | read=0,repo_map=0,list=0,trace=9,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 窗、四线程三跳唤醒链、11.000ms 链上 IO 第一席、三个独立 1.000ms runnable/优先级候选、实际占时/规则可消双轴、业务下钻、邻近/背景隔离和完整 Trace 因果投影均在；帧因果未证单独披露，0 次成文拒绝，未按固定 4ms/4m 或上下文比例降级。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260821-190548 | answer_regex,answer_contains,mermaid_edge_count | none | 411s | 38 | read=16,repo_map=3,list=0,trace=0,source_lens=0 | midloop=10,inv=6/0,fin_reject=2,unavail=0,prune=0 | uncertain | 相对 r834 从 20 次拒绝/降级旧稿收敛到 2 次拒绝/正常成文；首轮 joint delta 一次处理关系+参与者，第二轮只删除一个 stale boundary ref，未出现 stale relation ref、patch_correct 或旧稿恢复。最终 Mermaid 合法，三条 stage precedence、BusContext→BuildAgentContext 参数传递、Mutable 传递关系均保留，未靠删空关系过关。B1318 的 scope-stale 形本轮未自然触发，故只有无回归正证。答案仍有既有 P2 citation 语义错配：Explorer 段声称“通过 extractorEvaluator.BuildInitialInstruction 间接感知”，引用却是 extractorToolObligationView 结构体结束行 extractor.go:79，源码无该调用；不据单样本新增 prose 硬门，归入已 filed EVAL-B14-RC1。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case conclusions

- `B1317` remains production-positive and r835 no longer loses mixed relation failures.
- `B1318` is test-positive and production-no-regression, but this replay did not naturally enter `answer_doc_relation_repair_scope` after a stale ref; do not overstate it as production-trigger-positive.
- Read convergence improved from r834's 1228s / 20 rejects / degraded old draft to 411s / 2 rejects / normal structured answer. The surviving second reject is a precise stale participant-boundary cleanup, not a relation-lease loop.
- The read answer's unsupported Explorer/extractor sentence is another witness for existing `EVAL-B14-RC1` (row-to-citation semantic alignment). It is model/citation grounding debt, not evidence to scan or rewrite final prose deterministically.
- Trace remains fully guarded: on-chain-only ranking, actual occupancy versus rule-eliminable accounting, business investigation clues, causal projection, and active-stream behavior are intact.
