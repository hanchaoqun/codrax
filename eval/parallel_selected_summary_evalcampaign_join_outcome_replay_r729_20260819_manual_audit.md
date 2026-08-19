# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T11:24:41Z
- sweep_start_ts: 20260819-042439
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_nlohmann_long_double_symptom | PASS | eval/results/github_issue_nlohmann_long_double_symptom-20260819-042441 | write_apply,answer_regex | none | 174s | 27 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=1,prune=0 | partial | B1160 生产正证：Write Analyzer 首次发射四个独立 expected outcomes（严格编译、long double 非空、两头文件同步、普通浮点非回归），四项均进入 planner/controller。补丁与 long-double 测试正确且项目套件通过；但 `tests/long_double_format.cpp` 仍没有普通 float/double 断言，controller 仅凭一次 `make check` 与 changed-path target_behavior 就把四项全部签为已验证。确认 B1159-B：typed 合同已到达，仍缺 criterion→实际执行观察的结构化回执。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-042441 | answer_regex,answer_contains,mermaid_edge_count | none | 429s | 43 | read=10,repo_map=3,list=0,trace=0,source_lens=0 | midloop=10,inv=8/0,fin_reject=2,unavail=0,prune=0 | partial | B1158-B 生产正证：机制语义精确补读后多轮 dispatch 只暴露 3 个 closure 工具，未恢复 broad navigation。B1161 正证：成文由 r728 的 8 reject/7 patch/降级失败降为 2 reject/2 patch/正常答案，component-split 不再在无 executable join 时强迫猜边。终稿 Mermaid 合法，四阶段 precedence 与 BusContext 参数流均有 typed anchor，Mutable 缺完整关系时诚实披露未证；但 `Extractor` 与 `AgentExtractor` 仍分裂为两个展示身份，阶段岛与载体岛未形成业务连通，用户请求的完整数据流只部分回答，另立 B1162 typed identity binding gap。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual findings

1. 两个 runner 均 PASS，但不能据此把两项都判为人工正确。C++ 的信息合同已经修复，执行证明合同尚未修复；
   QF 的拒绝循环已经显著收敛，关系证据仍不完整。
2. C++ 首次 `emit_write_analysis` 已保留全部四个独立 outcome，证明 B1160 不是只在 schema 单测中成立。
   第四项“普通浮点序列化不受影响”到达 planner 的 fallback behavior contract；然而计划仍只修改两份发布头，
   现有测试只调用 long-double helpers。一次 project runner 总绿没有提供“第四项实际被哪个断言执行”的回执，
   不得继续由 path-level `covered/project_runner/target_behavior` 代签。
3. QF 在机制语义 exact-read repair 后连续使用 `read_file + emit_evidence + emit_investigation_complete` 三工具面，
   说明 B1158-B 的 producer-owned typed bit 已在真实非-flow origin 上生效。调查仍有 19 个 explorer 迭代、8 次
   completion 发射，后续可继续优化 closure 心智，但没有再出现“读完立即恢复 14-tool broad surface”的旧回归。
4. QF 第一次成文拒绝准确指出无证 `Orchestrator→Analyzer/BusContext` 边、stale anchor 和 BusContext incident
   缺失；第二次准确指出 Mutable/BuildAgentContext 的无证桥和技术身份映射错误。第三稿直接通过，没有再进入
   `typed_requested_component_not_connected` 三轮死循环，也没有回退“上一版结构化答案草稿”。这验证 B1161
   的 hard-gate escape lane，而不是降低既有 relation/anchor 校验。
5. 终稿图仍把 `extractor[Extractor]` 与 `n9[AgentExtractor]` 画成两个节点。现有 typed row 只证明
   `types.AgentExtractor → ctxbuilder.BuildAgentContext` 的参数流，没有一条结构化 identity-binding/ownership row
   许可系统或模型把两者当作同一展示参与者。因此正确处置只能是 partial + 未证边界，不能靠字符串相似、
   request/answer 扫描或系统自动补桥。新立 B1162，候选根修应由 producer 发射 typed participant↔technical
   endpoint binding，再让模型选择如何展示。
6. 本轮未观察 active stream 以 4ms/累计 age 降级，也未改 Trace 显式窗、因果投影或自动补齐；系统没有
   代写模型结论或 Mermaid 图。
