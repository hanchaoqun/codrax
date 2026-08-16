# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T05:36:29Z
- sweep_start_ts: 20260815-223628
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260815-223629 | answer_regex,answer_contains | none | 141s | 26 | read=3,repo_map=1,list=1,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | B880 stable：首稿关系门拒绝后，修补被 typed candidate 引导为 `Name() -> string literal` 两条 return 边；图可渲染但未表达两个工具与 finalizer 的真实注册、选择、首次输出/重试切换关系。正文仍无证据地声称 LoopPolicy 直接调度两工具，且末尾系统附注承认主路径关系未完整呈现。runner PASS 不能代表关系语义正确。 |
| 1 | read_combo_loose_multi_question_units | PASS | eval/results/read_combo_loose_multi_question_units-20260815-223629 | answer_regex,answer_contains | none | 222s | 39 | read=6,repo_map=3,list=0,trace=0,source_lens=1 | midloop=5,inv=2/0,fin_reject=0,unavail=0,prune=2 | fail | B879c 精确条件未满足：最终 mechanism roster 是 outcome 枚举而非 callable，因而没有误扩域；但 Explorer 已用 typed mechanism-definition 选择 Render/Sanitize 等定义，文件首段读取未覆盖真正普通失败路径的 `maybeReplaceMermaidFence`/`mermaidFallbackFence`。最终把 degraded-draft sanitizer 错接到普通 REPL 主链，并误述 FallbackRune、UnsupportedKind 与 LibraryRejected。属于上下文闭包 gap，不是可接受模型波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
