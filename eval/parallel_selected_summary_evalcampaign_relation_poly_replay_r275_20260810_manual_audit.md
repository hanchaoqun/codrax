# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T20:02:21Z
- sweep_start_ts: 20260810-130220
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260810-130222 | answer_regex | none | 201s | 22 | read=1,repo_map=2,list=1,trace=0,source_lens=1 | midloop=5,inv=2/0,fin_reject=2,unavail=0,prune=0 | fail | B472 的二态 guard 与 PyO3 方向性摘要保持正确；但注册桥仍未进入图，`py::tokenize_bytes` 与 `super::tokenize_bytes` 共用同一调用行，`_tokenize_slow` 又把调用行 22 误称为定义行。B474/B475 仍成立。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260810-130222 | answer_regex,answer_contains,mermaid_edge_count | none | 487s | 41 | read=26,repo_map=4,list=1,trace=0,source_lens=0 | midloop=12,inv=3/0,fin_reject=2,unavail=0,prune=4 | fail | 第二次 Explore 已真实 dispatch，但仍只产出两条内部辅助 call；最终请求主体 `Mutable/BusContext/analyzer/explorer/extractor/finalizer` 全部以 unproven 断点呈现，图未回答所求主数据流。mermaid_edge_count 只数到无关边，属于 oracle 假绿。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工裁定

### 1. B473 没有被本轮生产形直接收账

r275 的 QF 确有两次 `DISPATCH stage=explore`，但第二次由 `flow_operation_carrier / flow participant coverage` 的 completion repair 驱动，发生在首次 finalizer 之前。本轮最终稿保留两条辅助 call，外层 `required_diagram_edge_absent` 没有再次触发，因此没有构造 r274 那条“finalizer 精确 hard backtrack → accepted closure”生产路径。B473 的专项 pin 与全量回归有效，状态仍应写成 `implemented/full-suite-pass/pending-direct-production-witness`，不能借本轮双 Explore 虚报 production-closed。

### 2. B476-OPSUPPLY1 确认 P0

completion repair 已明确告诉 Explorer 查 producer、transfer/merge、consumer，并列出六个 typed participant；第二次 Explorer 也读到 `BuildAgentContext`、`applyStageOutput`、`recordTaskFinalize` 等真实操作面。但它仍主要发射 definition，最终只留下：

- `executeStageRequest -> dispatchStage`；
- `dispatchStage -> BuildAgentContext`。

这两条都是真 call，却不是用户要求的 analyzer/explorer/extractor/finalizer 与 BusContext/Mutable 之间的数据流。最终图把六个请求 participant 全部作为断开的 unproven 节点，正文却继续概括四阶段数据流，图文权威不一致。根因是 repair 仍给宽泛 `ExpandSearch`，没有把已经读过的候选文件/操作位点形成 typed、bounded 的操作补证任务；26 次 read 和 487 秒证明仅增加教学与预算不会泛化解决。

最优批次：让 flow repair 携带已读生产文件，并从 typed participant + current-source evidence/navigation carrier 形成只用于探索的 operation-site targets；要求读取包含真实调用/赋值/返回的 bounded body 后再发射 exact endpoints。targets 不授权答案边，participant roster 不铸权，找不到时继续诚实 unproven。

### 3. B474/B475 仍未关闭

多态答案保留了 B472 的正确结论：import 成功走 native，ImportError 走 Python fallback；但 registration 行仍未把“模块对象/导出 callable”两端稳定成可与 Python call target 精确 join 的身份，图因此缺失 `_fastlex.tokenize_bytes` 到 Rust wrapper 的注册桥。逐项列表又让 wrapper/core 共享 `core-rs/src/lib.rs:42`，并把 `_tokenize_slow` 的调用行 22 写成定义行。下一顺序保持 B476 → B474 → B475，不按 Python/PyO3 字面做特例。

### 4. 回归边界

本轮没有 runtime/trace 输入，Trace 显式时间窗、自动补齐、因果投影、链上根因选举与背景分层均未进入或改变。后续实现继续禁止扫描用户原文、模型 thinking 或最终 prose 作为 hard authority；系统只改善 typed 证据与修复定向，不生成关系、不替换模型结论。
