# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T04:36:48Z
- sweep_start_ts: 20260808-213646
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_jsonl_filter_count | PASS | eval/results/data_jsonl_filter_count-20260808-213648 | log_regex,answer_regex | none | 50s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | `instructions.md` 以 planner_distilled 完整进入计划，`events.jsonl` 由单个 `custom_transform` 消费；最终严格单行 `2`，零 repair/reject。该简单过滤计数不需要强制铸造 contributions/reconcile，未见 JSON 教学冲突。 |
| 1 | trace_query_frame_timeline_flow | PASS | eval/results/trace_query_frame_timeline_flow-20260808-213648 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 179s | 32 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=2,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | Mermaid 三条 temporal anchors 由模型首稿完整携带，`fin_reject=0/patch=0`，证明 B409 无重试车道成立，但未命中新 auto-repair positive branch。答案仍把 `app-20` 确认为 UI 主线程、把 `RSUniRenderThre` 确认为 RenderService 专用线程并扩写内部提交/硬件绘制职责；typed rows 只证明线程名、span 与时序，角色/内部工作 authority 均为 not_provided。另首次 completion 被源码 flow-operation 门 DOWNGRADED，外部 trace 被无意义要求源码 producer/transfer/consumer；第二次借 external-only waiver 才完成。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
