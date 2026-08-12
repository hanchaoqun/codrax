# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T09:30:34Z
- sweep_start_ts: 20260812-023032
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260812-023034 | answer_regex,answer_contains | none | 183s | 26 | read=4,repo_map=0,list=0,trace=0,source_lens=0 | midloop=7,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | 源码是 `gate.Run -> RunWith`，`buildAnalysisIR` 直接调用 `gate.RunWith`；不存在题设方向的 `buildAnalysisIR -> ... -> gate.Run`。模型的图没有伪造该边，却在摘要和 gate.Run 条目两次倒置为 “RunWith 包装/内部调用 Run”。Analyzer 首轮明明发出两个有序精确端点，却误用 discover_path；wire gate 令其清空端点后，既有 exact reachability/no_directed_path 车道被旁路。机器 oracle 只检查表面锚和 Mermaid，未覆盖方向语义。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260812-023034 | answer_regex,answer_contains,mermaid_edge_count | none | 526s | 39 | read=17,repo_map=5,list=0,trace=0,source_lens=1 | midloop=13,inv=4/0,fin_reject=4,unavail=0,prune=2 | pass | B630c 已进入真实 repair payload：BusContext candidate 明确 `participant_endpoint_side=to`，模型最终复用同一 data_flow edge，把业务节点 `BusContext` 置于该端，没有另造 method/component 桥；Mutable 在缺少有向 operation 时保持可见 disconnected 并披露未证。首稿仍虚构 Orchestrator fan-out、BusContext→Mutable 箭头等 11 条关系，4 次精确拒绝后才收敛，且未采用已允许的 containment 分组，效率与表达仍有优化空间，但最终图/正文关系边界诚实。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case judgment

- `B630c` 可转 production-positive：candidate endpoint side 足以让模型把已有技术关系直接落到业务 participant，而不需系统生成图或新增桥边。4 次成文拒绝说明教学仍有模型服从成本，但最终没有无证关系。
- 立案 `B632-CALLCHAINENDPOINTMODE1/P0`：两个 ordered endpoint 字段与 `discover_path` 枚举自冲突时，当前接线直接要求清空端点，令本应进入 exact reachability/no_directed_path 的任务降格为 role-bound 探索；最终正文发生包装方向倒置。
- 两案均由模型持续 reasoning/tool/content 字节推进。526s 案越过四分钟仍正常完成；系统未以连接年龄、累计时长或“4 分钟尚无最终答案”为降级条件。逻辑案的软化只在四份已完成结构稿被精确关系合同拒绝后发生。
- 本批未改 Trace 车道；显式时间窗、链上-only 根因、因果投影、自动补齐和模型结论所有权均未受影响。
