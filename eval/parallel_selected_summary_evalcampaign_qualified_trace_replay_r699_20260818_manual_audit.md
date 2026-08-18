# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T18:55:08Z
- sweep_start_ts: 20260818-115507
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260818-115508 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 176s | 31 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B1102 生产正证：Finalizer 的 Claim Binding 只保留 validator-owned 优先级/时间单位记录，前置零-authority dependency/阻塞候选已从成文上下文遮蔽；模型最终只作条件性锁验证建议，未宣称已证持锁/等待完成。精确窗、链上 worker-200、8.300/9.000ms 双口径、Trace 因果投影和自动补齐完整，无固定 4ms 降级。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260818-115508 | answer_regex,answer_contains | none | 346s | 30 | read=7,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=2/0,fin_reject=2,unavail=0,prune=0 | partial | B1101 生产正证：两轮 patch 后 5 项“关键中间函数”清单仍完整，端点边界列表不能再冒名 member_set。新 B1103：系统给出的 copy-ready 两条汇聚边没有携带同一硬门要求的 no-path participant boundary rows；模型照抄后连续两次被拒，并被迫在正文显示怪异“未证关系边界”。这是 initial teaching 与 emit gate 跨 relation-axis 不一致，不是模型波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工审计结论

### `trace_query_wakeup_causal_runnable`

1. 精确主窗仍为 `1.000000..1.010000`。目标 app-100 的 10.000ms S 态是自身等待症状；唯一链上排序席
   为 worker-200，链上累计 9.000ms、有效归因 8.300ms，邻近/背景没有晋升主因。实际占用与规则可消
   两轴、`Trace 因果投影`、确定性补采均在场。
2. B1102 获生产正证。前置 `emit_perf_trace` 确实生成了零 authority 的 dependency、blocking candidate
   和错误优先级解释；Finalizer 的 Claim Binding/Observation 成文视图中均未出现这些模型候选，只保留
   deterministic validator 产生的平台优先级语义和时间单位。最终答案没有再断言 worker 已持锁、目标
   在等 work 完成或 CFS 抢占 RT，只将锁关系作为后续验证条件。
3. 活跃字节流没有因 4ms 未形成答案而恢复旧稿、降级或空答。系统没有改写模型结论。审计附录仍有
   `tier`、`causality`、`predicate` 等内部控制值，这是既有 reader-language 投影债，不把它误记成
   B1102 回归，也不能用扫描最终 prose 的方式修补。

### `qf_sequence_analyzer_gate`

1. B1101 获生产正证：最终答案在两轮 patch 后仍完整保留 `compiler.Compile`、`hdp.Plan`、
   `compiler.RecomputeBudget`、`binder.BindByRelevance`、`gate.RunWith` 五项关键中间函数；另一个
   `principal_path_edge` 端点列表没有替代该 `member_set`。最终图正确保持
   `buildAnalysisIR -> gate.RunWith <- gate.Run`，没有伪造 source→sink 路径。
2. Runner PASS 仍只能判 partial：首稿已完全照用系统发布的两条 copy-ready typed 边和 anchors，却被
   participant gate 要求为 `buildAnalysisIR`、`gate.Run` 增加 `unproven` boundary；第二轮修补只改
   清单/端点仍再次收到相同拒绝。第三轮才加入隐藏 boundary metadata，并额外把“未证关系边界”写进
   读者正文，总计 2 次 Finalizer reject、346s。
3. 根因是跨轴合同不一致：pre-emit validator 对所有非 Trace-root-cause 必需图执行 participant
   completeness；初始 Diagram Contract 的 exact `boundary_recipe` 却仅在 `PredicateAxis=flow` 且
   `Intent!=trace` 时发布。源码 call-chain 合法使用 `IntentTrace + AxisCall`，因此恰好被教学面排除，
   同时 copy-ready skeleton 只带局部 arrows/anchors，不带仍未证的请求级 relation boundary。
4. 立案 `B1103-BOUNDARYRECIPEAXISPARITY1`：boundary recipe 的适用域与硬门统一为“所有 required、
   非 QFRootCauseTrace 图”；同一 typed coverage producer 同时给 Diagram Contract 和 copy-ready carrier
   输出精确 block-level `participant_boundaries_json`，并明确该 JSON 不得显示为读者语言。系统不创建
   边、不代写图/结论，也不扫描请求或最终正文。
