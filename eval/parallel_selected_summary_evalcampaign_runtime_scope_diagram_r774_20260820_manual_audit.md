# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T13:37:39Z
- sweep_start_ts: 20260820-063738
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-063740 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 223s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | B1238 获生产正证：首次 analysis 已选择 causal_diagnosis，只因错误携带 bounded fact_families 被精确拒绝一次，第二次保留因果 scope 并补 causal_attribution；最终 family=root_cause_trace，三次同窗 trace_query 覆盖 wakeup_chain/window_stats/root_cause_rank，11.000ms threadpool IO 席、三个 1.000ms runnable 席、显式窗、实际占时/规则可消双轴、链上-only 加冕、Trace 因果投影与自动补采均恢复，目标 20ms S 态保持症状。人工不记满分：模型把 typed `on_chain_prewakeup_work_candidate_only` 说成 IO 等待“逐级传导”，超出明确的 target-wait/completion/direct-blocking 未证上限；同时 root_cause_rank 同一背景 io_pressure 行发布 activity_index/cumulative=16 与 ranking impact=7，系统投影把两者并列成“窗口投影 7/链上累计 16”，而模型正文采用 16，形成 B1241 composite-score 口径冲突。禁止扫描正文硬门；先修 typed score identity/显示，再以软指导观察机理越界。持续活跃 223s，无 4ms/4m/首包/stall 降级。 |
| 2 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260820-063740 | answer_regex,answer_contains | none | 284s | 34 | read=13,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=1/0,fin_reject=3,unavail=0,prune=0 | pass-with-caveat | B1237/B1239 获生产正证：Finalizer 的 First-Pass reference 明确限定 completion-verified 12 席；source-role handoff 将 12 个 production 标 principal、3 个 test helper 标 support_only；首稿和终稿主表/主图均精确 12 席，方向为 implementer→LoopController，同侧 node/identity 一致，零 test helper 边。三次拒绝分别是 citation_ref→evidence_ids、table label/text→cells、`Summary-main` 大小写误写，均属模型 JSON/patch 失误而非合同矛盾，后续仍需低心智教学优化。新确定 B1240：legacy structural-enumeration V2 oracle 只读 item.label、不读结构化 table cells，且仍按 all-source roster；因此产生 1 条 soft advisory 并追加与 12 条 exact relation+citation 相反的“证据支持稍弱”系统 caveat。持续活跃 284s，无固定时长降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
