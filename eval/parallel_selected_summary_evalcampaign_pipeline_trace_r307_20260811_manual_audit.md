# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T10:55:55Z
- sweep_start_ts: 20260811-035553
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260811-035555 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 122s | 37 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 主链、#1 IO、三个 runnable 席与背景权限正确；但 20.000ms 查询窗、20.020ms 附件范围、0.020ms 唤醒后调度延迟被混写，S 状态被无证据称为“正常等待”，且“每跳均 1ms”忽略最终 0.020ms。投影重复 E5(+2) 仍在。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260811-035555 | answer_regex,answer_contains | none | 742s | 42 | read=48,repo_map=3,list=0,trace=0,source_lens=0 | midloop=29,inv=12/1,fin_reject=4,unavail=0,prune=1 | fail | required sequence 已保留，B523 获生产正证；但图由多个断开实现片段组成，未表达完整 analyze→finalizer 时序，Finalizer/BusContext 仍是断开 unproven 节点。四次 reject 中精确 n14→n15 边因业务缩写标签覆盖 typed endpoint identity 被拒，最终删边。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Case audit: trace_query_wakeup_causal_io_chain

- 正确部分：typed 唤醒链仍为 `threadpool-400 -> network-300 -> cookie-200 -> app-100`；链上 #1 为
  `fscache_page_wait_on_page_bit` 的 11.000ms iowait，三个调度供给席各 1.000ms；邻近/资源背景没有越权加冕。
- `B521` 只有部分正效：正文不再写 14ms 完整分解 20ms，系统投影也明确还有 5 条未计价链上行；但模型仍在别处泛称上游等待之和等于目标 wakeup latency，缺少同口径加法权威。
- 新确认 `B525-TRACETIMEROLE1/P1-high`：perf triage 提供附件范围 2.000000..2.020020=20.020ms，trace query 选择窗为
  2.000000..2.020000=20.000ms，最后 wakeup→running 为 0.020ms。三种合法数值缺少足够靠前、同一 typed role 的消费约束，模型将目标 sleep 写成
  20.020ms、又称“每跳均 1ms”。最优方案是 typed `interval_role + start/end + duration` 同席发布，并在 synthesis handoff 按 query-window、state-residency、post-wakeup-latency 分栏；不是扫描答案数字或替模型改算术。
- 同一 S-state 仅证明可中断睡眠，不能证明“正常”“等待下游响应”。应把 `state_semantics=unproven` 与已有 blocked-reason absence 并置给模型；absence 不能反推出业务语义。
- `B522` 仍开放：同一 iowait 在三次 trace_query 结果中汇合为 `[E5(+2)]`，可见说明又称“2次同值”；target/intermediate sleep 也有跨面重复。需按 producer/query/view/interval/measurement-role 审计后做 identity-aware merge，禁止 subject+value 粗去重。

## Case audit: read_combo_pipeline_sequence_table

- `B523` 生产闭环：Analyzer 的无出处推断 participant 已不再连带删除 required sequence，最终答案确有 Mermaid 图。
- 最终图仍不是完整 read-mode 时序：多个独立 typed 关系分量被同时放入 sequence，`Finalizer`、`BusContext` 以 disconnected/unproven 出现；表格虽补齐阶段说明，图本身没有完成用户要求的 analyze→finalizer 主链。
- 四次 finalizer reject 暴露 `B524-DIAGRAMIDENTITYCARRIER1/P1-high`：copy-ready recipe 的 node ID/方向正确，但
  `DiagramEdgeAnchor` 只有 node ID 与 relation，validator 仍从可见 label 反解 endpoint。模型把
  `persistMergedAnswerDocumentWithAttachmentPolicy` 改写为业务友好的 `persistMergedAnswer` 后，系统把它当成证据身份变化并拒绝；随后模型删掉正确边。
- 最优方案是 edge anchor 可选携带 producer-supplied exact endpoint identity pair；validator 用该 pair 选择 typed evidence，visible label/message 只负责业务表达。identity pair 不能自证边，仍需同方向 citable relation evidence。
- 742s 全程仍产出模型答案，没有因约 4 分钟阈值发布系统降级答案；`B517` 获“长任务不回归”正证，但单次持续 hidden-reasoning 专项仍保留独立回放口径。

人工结果：runner 2/2，human 0/2。两例均非可忽略模型波动：一个是 typed 时间角色上下文不够精确，另一个是显示标签与证据身份仍被同一字段承载。
