# r617 人工审计：IO family 已选中但紧凑上下文丢主值

- 基线：`main@7516b2a2e` 加 B973 与 runtime fact-family 单源教学。
- 执行：`real_trace_h3_iofam_one_seat` 与 `trace_query_wakeup_causal_runnable`，并发恰好 2，单次回放。
- Runner：H3 FAIL；Trace 因果对照 PASS。
- 人工：H3 fail；Trace pass。

| case | runner | 人工 | 关键审计 |
|---|---|---|---|
| real_trace_h3_iofam_one_seat | FAIL | fail | Analyzer 已正确发 `io_latency + target_wait_occurrences + count_or_duration`，证明 ontology 接线生效；但 Finalizer 的通用 Observation Ledger 只有 10 行预算，目标的 8 个 per-CPU 状态行先占满，精确 IO pair/coverage 被挤掉。Explorer 看见 1.347/1.337/41.329，但终稿仍无法使用。另发现 Explorer 把“单请求 issue→complete 墙钟”误称非墙钟；真正非墙钟的是跨请求可重叠的 request·ms 累计。 |
| trace_query_wakeup_causal_runnable | PASS | pass | 显式 1.000..1.010s 因果诊断继续保留 typed 唤醒链、链上 runnable 延迟与优先级风险；有限 family 改动没有削弱完整 causal diagnosis。 |

## GAP 与修向

1. `B975-REQUESTEDFACTPROMPTSURVIVAL1/P1`：同一目标并不等于同一问题维度；per-CPU roster 可把用户明确请求的 exact family 挤出紧凑预算。根修是在已接受 ledger 内按 typed requested family 做 prompt-only 优先，不改事实、因果排序或答案。
2. `B976-IOLATENCYCALIBERAUTHORITY1/P1`：为有限 runtime fact 建独立有界 authority，从完整 ledger 取该请求选中的 family，并明确区分单请求墙钟、completion-closed 目标阻塞墙钟、跨请求 request·ms/综合分。系统只交事实和量纲，不生成结论。
