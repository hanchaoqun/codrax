# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T22:52:42Z
- sweep_start_ts: 20260801-155241
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_c2_dstate_iowait | PASS | eval/results/real_trace_c2_dstate_iowait-20260801-155242 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 140s | 41 | read=1,repo_map=0,list=0,trace=7,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass_with_advisory | 主问题结论正确：请求主窗 3 段 io_wait，0.138+0.147+0.350=0.635ms，非 IO D-state=0，caller 为 sync_buffer_read_wi；没有错误注入 Trace 因果投影。模型把 udk-irq 行解释成“代为采样/内核跨线程采样”缺少 typed 权限，作为低优先级语义 advisory 登记，不由系统改写正文。 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260801-155242 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 159s | 34 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | ALIAS1 covered：同一附件只生成一套主投影/明细/指标/证据索引。PHASE1 partial：finalizer 已收到 impact_phase=pre_wakeup_dependency，旧的“RT 目标唤醒后被 CFS 线程抢占 11.103ms”误判消失；但模型仍把候选称作核心优先级反转。更严重的是系统把 typed missing_wakeup（仅表示窗内未找到匹配 sched_wakeup）发布成 principal_blocking，模型因而误写成“直接唤醒缺失/直接阻塞”。另有 D-state/io_wait/io_latency 跨口径相加、把 wait caller 写成“持有对象”的次级语义问题。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B26-ALIAS1`: covered by this replay. One physical attachment has one canonical projection set; exploration and system-supplement source lanes remain visible without duplicating the user-facing projection.
- `EVAL-B26-PHASE1`: partial. Typed phase transport is present and prevents the prior post-wakeup scheduler claim, but a candidate is still rhetorically promoted beyond its holder/waiter proof.
- New P0/P1 gap `EVAL-B27-MWAUTH1`: an absence observation (`missing_wakeup`) is admitted into the positive target-blocking wall-clock authority. “No matching row in the selected window” proves an evidence boundary, not a physically missing wakeup or a causal blocker identity.
- New P2 gap `EVAL-B27-REL1`: causally adjacent D-state, io_wait, and io_latency seats do not carry a sufficiently salient typed overlap/relation contract into synthesis; the model added unlike/overlapping measures despite the generic non-additivity instruction.
- New P2 gap `EVAL-B27-CALLER1`: `sched_blocked_reason.caller` is a wait-site/caller fact; no typed field authorizes “held object” or “sampling agent” semantics.
