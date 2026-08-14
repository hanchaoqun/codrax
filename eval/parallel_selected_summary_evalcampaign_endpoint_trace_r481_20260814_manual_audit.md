# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T11:00:55Z
- sweep_start_ts: 20260814-040054
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260814-040055 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 217s | 38 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 三次 trace_query 均继承同一 233.190ms 显式窗；模型正文与系统 Trace 因果投影同时在场，保留真实占时/规则可消双轴、链上根因、D-state/调度/算力/优先级、业务 span，邻近 CPU 压力仅作背景。缺口是模型把 12 条 blocked_reason 调用点记录与 11 段 D-state 直接解释成同一批 dma_fence 等待，并进一步推断 gpu-token 代表 fence 完成；typed 教学已明确禁止一一配对，系统附注也给出正确边界，故记模型服从性 partial，不新增正文关键词硬门。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260814-040055 | answer_regex,answer_contains | none | 248s | 32 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=4/1,fin_reject=1,unavail=0,prune=0 | partial | B786 消除了 `parallel_convergence` 词源，但本次 sink 只有 definition-only、状态为 endpoint_unresolved；系统仍任取 source_frontier 前 3 条兄弟调用作为主图参考，并把 HARD principal_path_edge/diagram_spine 计为 30 条。模型据此把 22 个同 caller 调用列成“按顺序的关键中间函数”。第一稿另将两条兄弟调用串成伪链，被严格关系门拒绝后仅修图，正文伪中间列表仍保留。确认 B787 系统载体 gap，非模型波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit conclusion

- Runner：`2 PASS / 2`；人工：`2 partial / 2`。
- H7 的显式窗、Trace 因果投影、自动补齐、链上-only 根因、实际耗时/规则可消双轴与业务线索均完整，说明本次关系载体施工没有回归 Trace 主能力。模型对 blocked_reason 的过度关联发生在精确反向教学已存在的情况下，先记 `B788-BLOCKEDREASONMODELADHERENCE1/P2-observe`；系统附注不改正文，只并置正确 typed 事实。
- Sequence 证明 B786 的 shared-callee 词面修复有效，但揭示 endpoint-unresolved 分支仍将任意 source frontier 和全量 facet count 升格为 principal authority。`B787-ENDPOINTUNRESOLVEDBOUNDARYSEED1/P1` 统一由 typed endpoint status 选边：shared/reverse/disjoint 只保留解释边界的精确边；unresolved/ambiguous/no-edges 的主边集合为空，显式图只给断开的精确端点；全量调用事实保留审计/support，不作为中间 hop。
- B787 不扫描用户或模型原文，不替模型写答案/结论/图；只收窄系统提供的 evidence authority、facet count 与 first-pass diagram seed。活跃流未发生 4ms/4m/固定总年龄降级。
