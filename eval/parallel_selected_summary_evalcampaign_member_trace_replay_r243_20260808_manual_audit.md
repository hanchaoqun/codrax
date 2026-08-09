# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T06:32:16Z
- sweep_start_ts: 20260808-233215
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_frame_timeline_flow | PASS | eval/results/trace_query_frame_timeline_flow-20260808-233216 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 162s | 24 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | 两个 windowed typed trace_query 正常到达；答案正确保留 temporal-only、`causal=unproven`，没有把背景升级成根因，含多条 Note 的 Mermaid 首轮通过，证明 B417 生产生效。人工仍失败：item authority 明示 `owning_thread_role_authority=not_provided_by_this_item`，模型却把 app-20、RSUniRenderThre-2096、gpu-300 扩写为“UI 主线程/RenderService 渲染线程/GPU 硬件线程”，并推断未捕获 Fence/队列/回调；属既有 B403 精确信号到达后的模型语义越权，不新增 prose 硬门。该请求是 bounded frame flow，不要求 root-cause projection；因果能力未被关闭。 |
| 1 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260808-233216 | answer_regex,answer_contains | none | 409s | 41 | read=20,repo_map=4,list=0,trace=0,source_lens=0 | midloop=7,inv=2/0,fin_reject=6,unavail=0,prune=7 | fail | B416 使 Explorer 携带了逐成员 notes/refs，但 notes 仍过浅且部分 stage/ref 对齐错误，最终表继续出现错误输入/载体并保留 `列 2..列 5`，故只算 partial。更严重的是系统胶囊明确给 `relation_kind=assignment` 两边，严格 sequence body gate 却仍要求 call anchor；模型用 assignment 报 missing_call_anchor，改 call 报 call_edge_unproven，形成不可满足合同，连续 6 次成文拒绝后恢复 rejected draft、跳过 structured checks。系统补充保持分栏并声明不替代模型答案，但不能修复主稿事实与降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
