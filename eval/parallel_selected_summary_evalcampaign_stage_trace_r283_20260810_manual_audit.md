# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T00:38:57Z
- sweep_start_ts: 20260810-173856
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260810-173857 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 206s | 34 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | 显式 5.000..5.007s 窗、自动补采、因果投影、链上 class verification 4.600ms #1、runnable 0.800ms #2、actual span 5.000ms、frame-unproven 与非链背景均保留；但答案把 CPU2 上的 worker-200 写成与 CPU1 上 app-100“同一 CPU”，又把 cross-CPU 唤醒依赖写成 worker 切出后 app 才能运行的直接同核竞争。原始行有 header CPU2 与 target_cpu=1，WakeupEdge typed 载体却未透传 CPU 拓扑，确认 B489-WAKECPU1。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260810-173857 | answer_regex,answer_contains,mermaid_edge_count | none | 231s | 35 | read=7,repo_map=3,list=0,trace=0,source_lens=0 | midloop=9,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | B486 正证：Analyzer→Explorer→Extractor→Finalizer 三条 precedence 全保留；B487 正证：completion/finalizer 收敛到 2 次完成调用和 1 次 reject。用户要求的 Mutable/BusContext 数据流仍未形成，图将二者留作未证独立节点，正文却继续声称四阶段共享读写。Explorer 把字段声明 `Mutable *MutableState` 错发成 initializer，grounder 正确拒绝，随后没有转向 SetTurnAArtifacts/TurnAArtifacts 等真实 producer/consumer operation，确认 B488-CARRIERFLOW1。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- runner: 2/2 PASS；human: 0/2。
- B486/B487 的目标均获生产正证：阶段别名不再删除真实 precedence，稳定 typed blocker 显著减少重复完成尝试。
- B488 是跨语言/跨组件 flow 的通用证据规划缺口：类型/字段定义不能代替 producer/transfer/consumer operation；应引导模型读取并发射真实 member call/assignment/return，而不是放宽 relation gate 或由系统补边。
- B489 是 Trace typed 上下文缺口：wakeup row 的 source/header CPU 与 target_cpu 已被 parser 精确持有，却在 WakeupEdge/observation/handoff 丢失。应透传 `waker_cpu`、`wakee_target_cpu`、`cpu_relation` 并给 soft 语义边界；不改变链构造、排名、可消除量或模型结论所有权。

## Post-audit implementation status

- B489 已在 `69f3aab9a` 贯通 wakeup CPU topology，registry golden 漏钉由 `e046ae3c1` 补齐；状态为 package-suite-pass、待 r284 生产回放。
- B488 已在 Analyzer participant planning 与 Explorer/completion 共用 flow-operation teaching 两层根修：被请求连接的 carrier 不再降为 context_only，载体必须取证真实 writer/reader operation，declaration-only field 不得当 initializer。hard relation authority、模型结论所有权与 Trace 路径均未变；状态为 package-suite-pass、待 r284 生产回放。
