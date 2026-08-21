# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T14:53:09Z
- sweep_start_ts: 20260821-075308
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-075309 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 159s | 39 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 精确保留 2.000000..2.020000 用户窗、threadpool-400→network-300→cookie-200→app-100 四跳链、11.000ms 链上 IO 第一席、三个独立 1.000ms 调度/优先级候选、实际占时/规则可消双账户、邻近/背景隔离、自动补采与 Trace 因果投影；内部 caliber 枚举未泄漏，活跃流无固定年龄降级。模型把两个线程的“唤醒前累计 sleep”写成“被唤醒后 sleep”，但同页 typed 时间点与主结论正确，记模型措辞观察项，不据单次波动加答案硬门。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260821-075309 | answer_regex,answer_contains | none | 637s | 55 | read=15,repo_map=3,list=0,trace=0,source_lens=1 | midloop=12,inv=3/0,fin_reject=7,unavail=0,prune=0 | fail | Runner 假绿。第一次 diagram repair 后 opaque failure_ref 已是唯一选择器，但模型重复携带 block/match/body occurrence；冗余 relation_kind 在 live row 中为空且 JSON enum 不可表达空值，造成 4 轮同类硬失败，最终共 8 次成文。接受稿仍声明 Orchestrator、Analyzer、Explorer、Extractor、Finalizer、BusCtx，却用 Analyzer→explorer→extractor→finalizer，生成大小写重复隐式 actor 与多枚孤立声明；B1295 optional cleanup 未被模型选择。另有内部 executable_body=unproven 泄漏和自动枚举补充重复冗长。确认 B1296 ref-first canonicalization 与 B1297 explicit orphan disposition。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human verdict

- Runner: 2/2 PASS.
- Human correctness: 1/2 PASS.
- Production-positive: Trace explicit-window causal lane and active-stream no-age-degrade.
- Production-negative: read diagram repair convergence and orphan participant presentation.
- Next batches: B1296 first, then B1297; neither batch may author model relations, labels, ordering, layout, prose, or conclusions.
