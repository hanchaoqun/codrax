# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T03:14:28Z
- sweep_start_ts: 20260801-201427
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_c2_dstate_iowait | PASS | eval/results/real_trace_c2_dstate_iowait-20260801-201428 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 129s | 40 | read=4,repo_map=0,list=0,trace=4,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 关键事实正确：D=0，io_wait 恰 3 段、0.635ms，三段时间/时长/caller 全齐。typed `bounded_fact_set` 正确压掉最终因果投影；但 thread_timeline 文本预览只给前两段，模型为第三段回读 141KB JSON 4 次并调用 grep。另有 caller 角色漂移：模型写“由 sync_buffer_read_wi 引起”，系统交叉核验也旧称“等待对象”，而该字段只证明 kernel wait call-site。登记 WAITPREVIEW1/CALLERROLE1。 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260801-201428 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 142s | 40 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | uncertain | 显式窗 `causal_diagnosis` 正确保留 Trace 因果投影、自动补采、实际占用/规则内可消除双轴、`frame_causality=unproven`。模型也明示 unproven；但正文把 wakeup path 写成“同步阻塞/等待某线程”，把重叠的 23.994+19.041ms 写成“约43ms”后又称非叠加，且模型总结仍偏向复述席位，未充分给出 axis-A 新探索方向。前者是 typed 语义边界提示 gap，后者在已有 model-owned 双轴 handoff 下暂记模型波动/P2-watch，禁止系统代写或硬 gate。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case judgment

1. 范围权限没有回归。无窗有限事实请求由 `runtime_question_profile=bounded_fact_set` 收窄，`final_projection_blocks=0`；显式时间窗请求由 `explicit_time_window + causal_diagnosis` 保持完整 Trace 因果投影与确定性补采。两者形成所需正反对照。
2. `EVAL-B33-WAITPREVIEW1/P1` 是生产 gap：完整的小型 typed occurrence rowset 已在 payload/ledger 中，却未进入 head-safe 工具文本，导致重复查询和 blob archaeology。修复应前置最多 8 条的小清单；超过容量必须 typed incomplete + payload continuation，不能丢尾也不能伪称 complete。
3. `EVAL-B33-CALLERROLE1/P1` 是生产 gap：`sched_blocked_reason.caller` 被若干系统面显示为“等待对象/cause”，与已有 final prompt 的“kernel-reported wait call-site, not holder”自相矛盾。统一改为“内核调用点/kernel wait call-site”，不改变值、排序或 cause-family grouping。
4. `EVAL-B33-WAKEWORD1/P2` 是 prompt 语义 gap：typed wakeup path 证明边的唤醒/依赖关系，但不单独证明同步阻塞、锁/资源持有或全链连续阻塞。只补 model-owned soft guidance，不扫描或改写正文。
5. `EVAL-B33-MODELSYNTH1/P2-watch` 暂不改代码：finalizer 已收到“Model Owns The Conclusion”、两独立轴、不可跨行相加和 unproven ceiling；单次模型仍把重叠席相加并弱化 axis-A 综合。当前证据更像模型波动/提示消费不足，不能据单 case 加答案硬门，更不能让系统替模型写结论。等待不同 trace/model 的第二 witness。

## Implemented batches

- `e881ea321`：把目标等待的小型 typed 完整清单前置到 trace_query 文本面；大清单显式 incomplete，保留 payload continuation；同时补 wakeup-path 角色软提示。相关 `internal/tool` 全量 157.295s、`internal/agent` 全量 3.267s 通过。
- `4b3c538f4`（`B33-CALLERROLE1`）：统一系统交叉核验、Trace 投影、上下文提示的 caller 词面为 kernel wait call-site，并明确禁止从 caller 推断资源对象/owner/holder。此批不触碰模型 block wire、不新增 prose hard gate；context/orchestrator/tool 全量回归通过（0.907s / 9.399s / 160.584s）。
