# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T08:12:16Z
- sweep_start_ts: 20260809-011215
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_background_demotion | PASS | eval/results/trace_query_wakeup_background_demotion-20260809-011216 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 158s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=3/2,fin_reject=1,unavail=0,prune=0 | pass | 显式 2.000..2.020 窗、自动补采、因果投影、目标五态与两类优化维度均保留。模型只把已证链 `threadpool-400 -> network-300 -> cookie-200 -> app-100` 上的 11ms io_wait 加冕 #1，并列三段各 1ms runnable 作为调度供给方向；off-chain `logger-900` 19.5ms 明确归 background、无 wakeup 边，不升格主因。一次 Finalizer reject 是缺 required summary 后一次 patch 成功。Explorer 另白耗两轮：无可复制 authority 时仍尝试可选 `relation_claims`，先混入 string、再自造 authority_id，记 B424。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260809-011216 | answer_regex,answer_contains | none | 262s | 34 | read=5,repo_map=4,list=0,trace=0,source_lens=1 | midloop=10,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | B423 生产闭环：mixed short/qualified endpoint 不再误拒，首稿唯一 reject 只针对四条真实无证 Agent/return 边，一次 patch 保留四条已证 call 后通过；7→1 rejects、513→262s。人工仍 FAIL：模型表只列入口/Analyze/Finalize，遗漏 StageExplore 与 StageExtract；正文把 FinalizerAgent 误称为 AnalyzeAgent 角色变化。Finalizer 前已有精确 `canonical_read_main_sequence=analyze -> explore -> extract -> finalize`，系统末尾核对表也正确但不替代模型答案，故 B422 仍开放。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
