# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T09:00:01Z
- sweep_start_ts: 20260822-015959
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-020001 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 190s | 38 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=2/1,fin_reject=1,unavail=0,prune=0 | pass | 显式 2.000–2.020s 窗、完整四线程唤醒链、11ms 链上 IO 第一席、三个独立 1ms 验证候选、实际占时/规则可消双轴、邻近/背景隔离和完整 Trace 因果投影均保留。唯一拒绝是首稿遗漏 typed principal summary 控制载体，模型局部追加后通过；可见正文自然语言未泄漏内部枚举，也未把候选宣称为掉帧/截止期已证原因。 |
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260822-020001 | answer_regex,answer_contains | none | 379s | 33 | read=7,repo_map=1,list=0,trace=0,source_lens=1 | midloop=10,inv=5/0,fin_reject=4,unavail=0,prune=0 | uncertain | B1330-E 生产转正：`repomap_dynamic_selector_return @ registry.py:34` 已进入 finalizer，typed 拒绝从 `return_unavailable` 前移为两个 `entry_unavailable`。入口 call 证据 `run_pipeline -> resolve` 同时存在，但 compiler 优先读取限定 OwnerSymbol 而非调用边 Subject，导致精确入口被遮蔽。模型因候选仍空连续 4 次修图并最终删除图；另有调查完成前 registration recovery 反复循环，作为独立高 ROI 上下文/证据合同 gap 留档。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
