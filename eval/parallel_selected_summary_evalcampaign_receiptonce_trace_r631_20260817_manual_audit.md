# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T17:17:51Z
- sweep_start_ts: 20260817-101749
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-101751 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 184s | 35 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式主窗与目标状态账保持 1.000000..1.010000/10.000ms，worker-200 的链上 #1 8.300ms、跨 CPU 唤醒与 Trace 因果投影均在；但 1.000000..1.011000 扩窗产生的 app-100 0.020ms 被系统同时作为 #2 主席、方向小计成员和树上徽章，形成 8.320ms，并与 `selected_window_authority` 的“窗外仅导航”及主窗 runnable=0 自相矛盾。模型据此进一步误写同 CPU、worker 持续运行和正常协作睡眠；另有 `priority_inversion_candidate`、`bounded_window_candidate`、`coverage=complete` 等内部枚举泄漏。确认 B993，非模型波动。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-101751 | answer_regex,answer_contains,mermaid_edge_count | none | 308s | 42 | read=15,repo_map=2,list=0,trace=0,source_lens=0 | midloop=11,inv=5/0,fin_reject=2,unavail=0,prune=2 | fail | B991 生产正证：系统未再把一个 typed receipt 重复映射给两个断开组件。新根因 B992：completion 只缺 BusContext 时，导航只匹配实参最终段，无法从 `o.busCtx.Mutable` 的外层 exact typed binding 看见 BusContext，随后回退到无关 validator；最终图只剩四阶段 precedence，BusContext/Mutable 孤立。修复仅扩展 parser-owned 完整成员路径的精确 segment 导航，不铸造关系。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion and batches

1. `B991-DIAGRAMTYPEDRECEIPTSINGLECONSUMER1=production-closed-r631`。本轮没有再出现唯一 receipt 被两个匿名组件重复消费后由系统自冲突拒绝的形状。
2. `B992-OUTERTYPEDCARRIERSEGMENTNAV1` 已由 `2bade8851` 修复并推送。只在完整 parser identity `o.busCtx.Mutable` 中匹配 exact binding segment；字符串、调用表达式、模糊名字仍不能导航，更不能生成关系。全语言生产形与 `go test ./internal/tool -count=1` 通过，待 r632 生产回放。
3. `B993-SELECTEDWINDOWRANKBOARDISOLATION1` 已由 `5fc054249` 修复并推送。只有独立 typed 目标状态账或已选唤醒链窗精确确认 principal window 时，已知异窗 rank board 才退出主席名册、可消除榜、方向小计、树徽章和代表窗；证据转入背景，不静默删除。普通多板/跨 trace 报告继续展示，旧回归与 types/agent/tool 全套通过。
4. 客户语言债继续单列 P2：JSON 字段仍用稳定 enum，用户可见正文应由模型使用当前语言解释；不得通过扫描用户输入或模型答案做硬替换，也不得由系统重写模型结论。
5. 两案活跃流持续远超 4ms 且均正常完成。没有固定累计年龄降级；合法终止仍只来自 caller deadline/cancel、首字节超时、字节停滞或 transport/decode 失败。
