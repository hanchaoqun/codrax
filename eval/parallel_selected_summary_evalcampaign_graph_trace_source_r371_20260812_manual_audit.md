# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T07:22:06Z
- sweep_start_ts: 20260812-002204
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260812-002206 | answer_regex,answer_contains,mermaid_edge_count | none | 291s | 37 | read=9,repo_map=5,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=1,unavail=0,prune=1 | fail | B622 正确阻止断开的 BusContext 借技术边连通，但 Analyzer 把明确关系成员 Mutable 标为 context_only；最终图只画四阶段，BusContext 断开、Mutable 缺席。立案并修复 B623。 |
| 2 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260812-002206 | trace_attachment,answer_regex | perf_triage+trace_query | 306s | 49 | read=7,repo_map=1,list=0,trace=2,source_lens=0 | midloop=2,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | B621 正向生效：未把 scheduler 缺失写成 On-CPU。源码机制仍错误：把 emitter TID 键说成 payload PID，并把 H: 错说成 isInstanceTag 会剥离；line-range 锚只证明定义存在，却携带模型合成伪代码与过度摘要。立案 B624。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工结论

Runner `2/2`，人工 `0/2`。两案分别运行 291s、306s，均超过四分钟且在模型流仍活跃时正常收到并渲染最终答案；未观察按 elapsed age 切旧稿、空答案或降级回答，继续为 B615 的生产正证。降级只能由精确的 transport/byte-silence/caller cancel/safety/decode 信号触发，不能由“已运行四分钟”触发。

### 1. qf_logic_view_read_pipeline

- B622 生效：系统不再把一条内部技术边与另一处断开的 `BusContext` 展示节点拼成“已连接”，因此最终稿诚实保留了未证边界。
- 但上游 Analyzer 在第六次 emit 中接受 `Mutable(role=context_only, source_quote="Mutable/BusContext")`。`Mutable/BusContext` 只是两个 typed 参与者的名单，不能证明 Mutable 是关系外背景；Explorer 因而只探索四阶段 precedence，最终 Mermaid 没有 Mutable，BusContext 也完全断开。
- 这不是 Mermaid edge-count oracle 能发现的问题。机器只看到了足够数量的边，没有验证用户点名的数据载体是否真实参与数据流。
- B623 泛化修复：required source-flow 的 `context_only` provenance 既不能是裸 identity，也不能是 delimiter-only typed participant roster。真正的周边边界仍需更宽、明确的 current-request boundary phrase。硬门只读 schema 化 identity/role/source_quote/entity roster，不扫描请求或答案原文，也不替模型补边。

### 2. read_combo_trace_current_source_explanation

- B621 获得生产正证：附件无 scheduler rows，答案没有再宣称连续 On-CPU、无阻塞或 CPU-bound；只保留 86.111ms wall-clock 与诊断边界。
- 机制解释仍有三处确定错误：同步栈实际 key 是 physical source identity + emitter event PID/TID，不是 B payload 的 `SpanPID`；`SpanPID` 是 namespace/process membership 载体。`isInstanceTag` 只识别精确 `<UPPER><digits>`（如 I38/M0538），不会剥离 `H:`；B 的完整未标记尾部会保留为 span name。答案还自行举了 60fps/16.67ms，typed deadline 权威并不存在。
- 根因不是源码未读：Explorer 确实读到了相关函数。但它在 `emit_evidence` 中提交了并非源码逐字内容的伪代码 snippet 和错误 summary；definition/line-range grounding 只核实文件、行和 symbol 存在，随后 Finalizer handoff 只带位置与模型摘要，无法区分“定义锚存在”和“所述操作逐行已证”。最终错误摘要被当作 grounded mechanism 复述。
- 立案 B624：源码证据的 locator authority 与 operation-content authority 必须拆分。定义锚可证明 symbol/入口存在；correlation key、分支、栈、reset、malformed/fail-closed 等操作，只能由 exact source gutter 或结构化 operation fact 承载。不能继续让模型合成 snippet 因 locator grounded 而获得正文权限。

## 不变量

- 未修改显式时间窗 Trace 查询、链上根因排序、双维度占时/可消除量、因果投影或自动补齐。
- 未扫描用户原始输入、模型 reasoning 或最终答案做硬门。
- 系统未删除、替换模型结论，也未生成 Mermaid 关系边。
