# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T09:58:35Z
- sweep_start_ts: 20260813-025834
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260813-025835 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 217s | 47 | read=1,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=1 | pass | B716 production-positive：5 次 trace_query 均严格使用 13762.791708..13763.024898 用户窗；Trace 因果投影和自动补齐在场，链上主席为自身 running 供给折算 65.912ms，其次 D-state 36.757ms，优先级反转/调度供给/D/IO/业务 span 均保留，邻近席位单列且不得加冕。实际占用与规则可消量双轴明确不可相加。本轮未再把 65.912+36.757 写成总可消量，B717 未复现。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260813-025835 | answer_regex,answer_contains,mermaid_edge_count | none | 493s | 40 | read=12,repo_map=2,list=1,trace=0,source_lens=0 | midloop=6,inv=4/0,fin_reject=4,unavail=0,prune=1 | fail | Runner 只钉图存在/边数而假绿。正文宣称四阶段经 BusContext/Mutable 完成数据传递，图却只含四阶段 precedence 与各阶段到 Mutable 的局部调用，BusContext 最终孤立并标 unproven。系统把仅覆盖四阶段的 checkout-verified precedence 误标为 complete requested_relation_spine；participant coverage 又把任意局部 incident operation 当成参与者完备，缺少精确的 BusContext owns Mutable 无箭头分组教学。4 次 finalizer reject 均围绕该自冲突消耗修补轮。确认 B718。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

1. `B716-TRACECANONICALWINDOWCONTEXT1` 在生产回放中闭环：主排序只消费显式用户窗，其它探查窗没有进入主账；Trace 因果投影、系统自动补齐、typed 链上-only 根因与实际占用/规则计价双轴全部保留。
2. `B717-TRACECROSSSEATPROSESUM1` 本轮未复现。65.912ms running 供给折算与 36.757ms D-state 被分别排序，没有求和成 102.669ms；当前证据支持“上一轮模型成文波动”，不支持新增答案词面或特定数字硬门。继续观察即可。
3. `B718-REQUESTEDFLOWTOPOLOGYCOMPLETENESS1` 是确定性系统 GAP：一个真实的 stage-order 子集不能因其自身完整就代表带载体参与者的完整 requested flow；局部 participant incidence 也不能证明所有请求参与者已经连成一条流。
4. 通用修向冻结为三层：精确区分 `requestScoped` 子集与覆盖全部 incident-required participants 的 `requestSpine`；把 `BusContext.Mutable *MutableState` 这类 checkout-verified ownership 发布为 no-arrow grouping recipe；把 unproven boundary 收窄为“请求的有向关系未证”，允许独立已证局部事实/包含关系共存，但参与者不得借此成为伪造的有向边端点。系统仍不画图、不写结论。
5. active stream 在 217s/493s 执行中持续有字节和工具进展，4ms 内没有完整 answer 时没有降级。4ms 仅属于非流式 terminal emit-only 校验预算；活跃流只能因 caller cancel/deadline、no-first-byte、真实 byte stall、terminal transport/decode failure 或重试耗尽进入有界恢复。
