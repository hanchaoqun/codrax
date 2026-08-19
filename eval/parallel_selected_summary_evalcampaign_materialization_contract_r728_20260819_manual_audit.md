# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T10:34:44Z
- sweep_start_ts: 20260819-033442
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_nlohmann_long_double_symptom | PASS | eval/results/github_issue_nlohmann_long_double_symptom-20260819-033444 | write_apply,answer_regex | none | 140s | 26 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | 两个 header 的 `%.*lg -> %.*Lg` 与 strict build/run 均正确；但显式要求的普通 float/double 非回归未进入 typed expected outcome，测试仍只调用 long-double helper。B1159-A 教学已到达 planner，但上游 Write Analyzer 的“2-4 outcomes”容量教学先丢掉了该独立约束，确认 B1160。 |
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260819-033444 | answer_regex,answer_contains,mermaid_edge_count | none | 589s | 60 | read=18,repo_map=2,list=0,trace=0,source_lens=0 | midloop=16,inv=6/0,fin_reject=8,unavail=0,prune=0 | fail | exact mechanism-descent repair 读完 `internal/context/builder.go:26-288` 后恢复 14-tool broad surface；模型虽随后发出部分 EvidenceItem/AnswerSymbol/AnalysisIR，最终仍经历 8 次成文拒绝、7 次 patch，并回退上一版结构化草稿。B1158 v1 仅识别 flow-navigation origin，未覆盖其他 producer-selected exact materialization repair。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings

1. B1158 v1 的行为目标正确，但 typed owner 放错在消费端的 origin 分类：r728 的补读由
   `pre_complete.mechanism_semantic_descent.26` 产生，不属于 flow-navigation 前缀。精确范围读完并清债后，
   explorer 下一轮重新暴露 14 个工具，说明生产闭环失败，不能收账。
2. 通用修向是让选择精确窗口的 repair producer 显式携带 `MaterializationRequired`，经
   `RepairDirective -> PendingRead` 单调传播；消费端只读该 typed bit（旧 flow origin 仅作耐久状态兼容），
   不再猜 origin 字符串家族。机制下钻、子主题函数体、调用链端点/主跨度与 flow value/operation 都使用
   同一合同。读完窗口只清 pending debt，不清当前 dispatch latch；模型仍须 `emit_evidence` 或诚实完成。
3. C++ case 证明 B1159-A 不能弥补上游合同丢失。用户显式的普通浮点非回归只留在 workflow
   `known_constraints`，Write Analyzer 的四个 `expected_outcomes` 已满且没有该维度；planner 因而没有可逐项
   见证的 typed contract。B1160 应扩大有界 outcome 容量，并教学每个独立显式成功/非回归维度必须进入
   typed outcome/behavior contract；不得由系统扫描请求文字补造合同。
4. QF 的 8 次 finalizer 拒绝属于独立成文合同审计面，不能拿 B1158 的工具生命周期修复一并宣称解决；需逐次
   核对拒绝是否一致、提示是否可执行、patch 是否被旧关系状态反复污染。
5. 两案都没有 active-stream 4ms/固定 age 降级，也没有系统代写结论。Read/Trace 显式窗口、因果投影、自动
   补齐和链上-only 主因路径未改。
6. QF 八次拒绝逐轮拆解后，前五次分别是未证 Orchestrator→stage 边、参与者身份不可见、候选 exact identity
   未复制、技术标签未呈现参与者身份、已连参与者仍保留 stale boundary；这些拒绝各自有精确信号。后续三次
   才暴露通用 B1161：局部 incident edge 已让 boundary 变 stale，但整张图仍分成 stage 岛和 carrier 岛；
   component-split 门要求补 join，却只重复发布每参与者前三条普通 candidate，没有发布“确实跨当前两个可见
   组件”的 typed frontier。两条合同语义不矛盾，修复出口却不完备，导致模型在等价 node-id 形之间往返。
7. B1161 根修从完整 typed candidate 池计算 current-visible-component crossing，最多发布 4 条
   `typed_join_candidate`；本地 incident candidate 不得冒充 join。只有至少一个这种精确、可复制候选存在时，
   component-split 才可硬拒绝；没有可执行出口时不得逼模型猜桥。系统仍不自动选边、加边、改图或写结论。
